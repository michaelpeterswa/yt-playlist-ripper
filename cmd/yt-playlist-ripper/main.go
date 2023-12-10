package main

import (
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

	logger.Info("yt-playlist-ripper init...", zap.Strings("playlists", strings.Split(config.String(configClient.PlaylistList), ",")), zap.String("cron", config.String(configClient.CronString)))

	ytdlClient := ytdl.New(logger, lockmap.New())

	c := cron.New()
	for _, playlist := range strings.Split(config.String(configClient.PlaylistList), ",") {
		err := ytdlClient.LockMap.Add(playlist)
		if err != nil {
			logger.Error("could not add playlist to lockmap", zap.Error(err), zap.String("playlist", playlist))
		}

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
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		logger.Fatal("could not start http server", zap.Error(err))
	}
}