package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	configClient "github.com/michaelpeterswa/yt-playlist-ripper/internal/config"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/handlers"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/logging"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/ytdl"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

func main() {
	logger, err := logging.InitZap()
	if err != nil {
		log.Fatalf("could not acquire zap logger: %s", err.Error())
	}

	config, err := configClient.Get()
	if err != nil {
		logger.Fatal("could not get config", zap.Error(err))
	}
	err = configClient.SetDefaults(config)
	if err != nil {
		logger.Fatal("could not set defaults", zap.Error(err))
	}

	logger.Info("yt-playlist-ripper init...", zap.Strings("playlists", strings.Split(config.String(configClient.PlaylistList), ",")), zap.String("cron", config.String(configClient.CronString)), zap.String("httpPort", config.String(configClient.HTTPPort)), zap.String("videoQuality", config.String(configClient.VideoQuality)), zap.String("archiveFile", config.String(configClient.ArchiveFile)), zap.String("outputTemplate", config.String(configClient.OutputTemplate)))

	ytdlClient := ytdl.New(logger, lockmap.New(), config.String(configClient.VideoQuality), config.String(configClient.ArchiveFile), config.String(configClient.OutputTemplate))

	for _, playlist := range strings.Split(config.String(configClient.PlaylistList), ",") {
		err := ytdlClient.LockMap.Add(playlist)
		if err != nil {
			logger.Error("could not add playlist to lockmap", zap.Error(err), zap.String("playlist", playlist))
		}
	}

	if config.Bool(configClient.RunOnStart) {
		for _, playlist := range strings.Split(config.String(configClient.PlaylistList), ",") {
			ytdlClient.Run(playlist)()
		}
	}

	c := cron.New()
	for _, playlist := range strings.Split(config.String(configClient.PlaylistList), ",") {
		logger.Info("adding playlist to cron", zap.String("playlist", playlist))
		_, err = c.AddFunc(config.String(configClient.CronString), ytdlClient.Run(playlist))
		if err != nil {
			logger.Error("could not add cron job", zap.Error(err))
		}
	}
	c.Start()

	r := mux.NewRouter()
	r.HandleFunc("/healthcheck", handlers.HealthcheckHandler)
	r.Handle("/metrics", promhttp.Handler())
	http.Handle("/", r)

	httpPort := config.String("http.port")
	logger.Info("starting http server", zap.String("port", httpPort))
	err = http.ListenAndServe(fmt.Sprintf(":%s", httpPort), nil)
	if err != nil {
		logger.Fatal("could not start http server", zap.Error(err))
	}

}
