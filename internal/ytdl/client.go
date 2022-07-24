package ytdl

import (
	"os/exec"

	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
)

type YTDLClient struct {
	logger *zap.Logger
}

func New(logger *zap.Logger) *YTDLClient {
	return &YTDLClient{logger: logger}
}

func (ytdl *YTDLClient) Run(playlist string) func() {
	return func() {
		zapWriter := zapio.Writer{
			Log:   ytdl.logger.With(zap.String("from", "ytdl")),
			Level: zap.InfoLevel,
		}
		ytdlCommand := exec.Command("youtube-dl",
			"--no-call-home",
			"--no-progress",
			"--write-thumbnail",
			"--yes-playlist",
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
	}
}
