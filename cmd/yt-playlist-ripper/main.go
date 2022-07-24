package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/handlers"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/logging"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/settings"
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
	logger.Info("yt-playlist-ripper init...")

	settings, err := settings.InitSettings()
	if err != nil {
		logger.Fatal("could not init settings", zap.Error(err))
	}

	lockMap := make(map[string]*sync.Mutex, len(settings.Playlists))

	ytdlClient := ytdl.New(logger, lockMap)

	c := cron.New()
	for _, playlist := range settings.Playlists {
		lockMap[playlist] = &sync.Mutex{}
		logger.Info("adding playlist to cron", zap.String("playlist", playlist))
		_, err := c.AddFunc(settings.CronString, ytdlClient.Run(playlist))
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
