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
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/telegram"
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

	ctx := context.Background()

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
		_ = shutdown(ctx)
	}()

	telegramClient, err := telegram.NewTelegramClient(c.TelegramEnabled, c.TelegramBotToken, c.TelegramChatID)
	if err != nil {
		slog.Error("could not create telegram client", slog.String("error", err.Error()))
		os.Exit(1)
	}
	go telegramClient.Start(ctx)

	slog.Info("yt-playlist-ripper init", slog.Any("playlists", c.PlaylistList), slog.String("cron", c.CronString))

	ytdlClient := ytdl.New(lockmap.New(), c, telegramClient)

	for _, playlist := range strings.Split(c.PlaylistList, ",") {
		err := ytdlClient.LockMap.Add(playlist)
		if err != nil {
			slog.Error("could not add playlist to lockmap", slog.String("playlist", playlist), slog.String("error", err.Error()))
		} else {
			slog.Info("added playlist to lockmap", slog.String("playlist", playlist))
		}
	}

	if c.RunOnStart {
		for _, playlist := range strings.Split(c.PlaylistList, ",") {
			ytdlClient.Run(playlist)()
		}
	}

	cronClient := cron.New()
	for _, playlist := range strings.Split(c.PlaylistList, ",") {
		slog.Info("adding cron job", slog.String("playlist", playlist), slog.String("cron", c.CronString))
		_, err = cronClient.AddFunc(c.CronString, ytdlClient.Run(playlist))
		if err != nil {
			slog.Error("could not add cron job", slog.String("playlist", playlist), slog.String("error", err.Error()))
		}
	}
	cronClient.Start()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("yt-playlist-ripper started", slog.String("pid", fmt.Sprintf("%d", os.Getpid())))
	slog.Info("waiting for signal")

	<-ctx.Done()
	slog.Info("shutting down")

}
