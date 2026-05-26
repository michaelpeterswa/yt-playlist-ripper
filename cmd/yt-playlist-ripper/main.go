package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"alpineworks.io/ootel"
	configClient "github.com/michaelpeterswa/yt-playlist-ripper/internal/config"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/logging"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/notifier"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/ytdl"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
)

func main() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "error"
	}

	slogLevel, err := logging.LogLevelToSlogLevel(logLevel)
	if err != nil {
		log.Fatalf("could not convert log level: %s", err)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})))

	c, err := configClient.NewConfig()
	if err != nil {
		slog.Error("could not create config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	exporterType := ootel.ExporterTypePrometheus
	if c.Local {
		exporterType = ootel.ExporterTypeOTLPGRPC
	}

	ootelClient := ootel.NewOotelClient(
		ootel.WithMetricConfig(
			ootel.NewMetricConfig(
				c.MetricsEnabled,
				exporterType,
				c.MetricsPort,
			),
		),
		ootel.WithTraceConfig(
			ootel.NewTraceConfig(
				c.TracingEnabled,
				c.TracingSampleRate,
				c.TracingService,
				c.TracingVersion,
			),
		),
	)

	shutdown, err := ootelClient.Init(ctx)
	if err != nil {
		slog.Error("could not create ootel client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	err = runtime.Start(runtime.WithMinimumReadMemStatsInterval(5 * time.Second))
	if err != nil {
		slog.Error("could not create runtime metrics", slog.String("error", err.Error()))
		os.Exit(1)
	}

	err = host.Start()
	if err != nil {
		slog.Error("could not create host metrics", slog.String("error", err.Error()))
		os.Exit(1)
	}

	defer func() {
		_ = shutdown(context.Background())
	}()

	notifierClient, err := notifier.New(c.PulsarURL, c.PulsarBearerToken, c.PulsarPushoverUserKey)
	if err != nil {
		slog.Error("could not create notifier", slog.String("error", err.Error()))
		os.Exit(1)
	}

	playlists := splitCSV(c.PlaylistList)
	channels := splitCSV(c.ChannelList)

	slog.Info("yt-playlist-ripper init",
		slog.Any("playlists", playlists),
		slog.Any("channels", channels),
		slog.String("cron", c.CronString),
	)

	ytdlClient := ytdl.New(ctx, lockmap.New(), c, notifierClient)

	for _, playlist := range playlists {
		key := ytdl.PlaylistLockKey(playlist)
		err := ytdlClient.LockMap.Add(key)
		if err != nil {
			slog.Error("could not add playlist to lockmap", slog.String("playlist", playlist), slog.String("error", err.Error()))
		} else {
			slog.Info("added playlist to lockmap", slog.String("playlist", playlist))
		}
	}

	for _, channel := range channels {
		key := ytdl.ChannelLockKey(channel)
		err := ytdlClient.LockMap.Add(key)
		if err != nil {
			slog.Error("could not add channel to lockmap", slog.String("channel", channel), slog.String("error", err.Error()))
		} else {
			slog.Info("added channel to lockmap", slog.String("channel", channel))
		}
	}

	if c.RunOnStart {
		for _, playlist := range playlists {
			ytdlClient.Run(playlist)()
		}
		for _, channel := range channels {
			ytdlClient.RunChannel(channel)()
		}
	}

	cronClient := cron.New()
	for _, playlist := range playlists {
		slog.Info("adding cron job", slog.String("playlist", playlist), slog.String("cron", c.CronString))
		_, err = cronClient.AddFunc(c.CronString, ytdlClient.Run(playlist))
		if err != nil {
			slog.Error("could not add cron job", slog.String("playlist", playlist), slog.String("error", err.Error()))
		}
	}
	for _, channel := range channels {
		slog.Info("adding cron job", slog.String("channel", channel), slog.String("cron", c.CronString))
		_, err = cronClient.AddFunc(c.CronString, ytdlClient.RunChannel(channel))
		if err != nil {
			slog.Error("could not add cron job", slog.String("channel", channel), slog.String("error", err.Error()))
		}
	}
	cronClient.Start()

	slog.Info("yt-playlist-ripper started", slog.String("pid", fmt.Sprintf("%d", os.Getpid())))
	slog.Info("waiting for signal")

	<-ctx.Done()
	slog.Info("shutting down")

	cronStopCtx := cronClient.Stop()
	<-cronStopCtx.Done()
}

func splitCSV(list string) []string {
	if list == "" {
		return nil
	}
	parts := strings.Split(list, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
