package ytdl

import (
	"os/exec"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
)

type YTDLPClient struct {
	logger  *zap.Logger
	LockMap map[string]*sync.Mutex
}

func New(logger *zap.Logger, lockMap map[string]*sync.Mutex) *YTDLPClient {
	return &YTDLPClient{logger: logger, LockMap: lockMap}
}

func (ytdl *YTDLPClient) Run(playlist string) func() {
	return func() {
		ytdl.LockMap[playlist].Lock()
		zapWriter := zapio.Writer{
			Log:   ytdl.logger.With(zap.String("from", "ytdlp")),
			Level: zap.InfoLevel,
		}
		ytdlCommand := exec.Command("yt-dlp",
			"--no-call-home",
			"--no-progress",
			"--write-thumbnail",
			"--yes-playlist",
			"-f", "'bv[height=1080][ext=mp4]+ba[ext=m4a]'",
			"-o", "/downloads/%(channel)s/%(title)s",
			"--download-archive", "/config/archive.txt",
			playlist)
		ytdlCommand.Stdout = &zapWriter
		err := ytdlCommand.Start()
		if err != nil {
			ytdl.logger.Error("yt-playlist-ripper failed to run ytdl", zap.Error(err))
		}

		err = ytdlCommand.Wait()
		if err != nil {
			ytdl.logger.Error("yt-playlist-ripper failed to exit successfully", zap.Error(err))
		}
		ytdl.LockMap[playlist].Unlock()
	}
}
