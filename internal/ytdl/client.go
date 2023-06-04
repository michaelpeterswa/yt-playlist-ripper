package ytdl

import (
	"fmt"
	"os/exec"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
)

type YTDLPClient struct {
	logger  *zap.Logger
	LockMap *lockmap.LockMap
}

func New(logger *zap.Logger, lockMap *lockmap.LockMap) *YTDLPClient {
	return &YTDLPClient{logger: logger, LockMap: lockmap.New()}
}

func (ytdl *YTDLPClient) Run(playlist string) func() {
	return func() {
		err := ytdl.LockMap.Lock(playlist)
		if err != nil {
			ytdl.logger.Error("failed to acquire lock", zap.Error(err), zap.String("playlist", playlist))
			return
		}
		defer func() {
			err := ytdl.LockMap.Unlock(playlist)
			if err != nil {
				ytdl.logger.Error("failed to release lock", zap.Error(err), zap.String("playlist", playlist))
			}
		}()

		zapWriter := zapio.Writer{
			Log:   ytdl.logger.With(zap.String("from", "ytdlp")),
			Level: zap.InfoLevel,
		}
		ytdlCommand := exec.Command("yt-dlp",
			"--no-call-home",
			"--no-progress",
			"--write-thumbnail",
			"--yes-playlist",
			"-S", "height:1080",
			"--recode-video", "mp4",
			"-o", "/downloads/%(channel)s/%(title)s",
			"--download-archive", "/config/archive.txt",
			fmt.Sprintf("https://www.youtube.com/playlist?list=%s", playlist))
		ytdlCommand.Stdout = &zapWriter
		ytdlCommand.Stderr = &zapWriter
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
