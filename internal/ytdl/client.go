package ytdl

import (
	"fmt"
	"os/exec"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
)

type YTDLPClient struct {
	logger         *zap.Logger
	LockMap        *lockmap.LockMap
	VideoQuality   string
	ArchiveFile    string
	OutputTemplate string
}

func New(logger *zap.Logger, lockMap *lockmap.LockMap, videoQuality string, archiveFile string, outputTemplate string) *YTDLPClient {
	return &YTDLPClient{
		logger:         logger,
		LockMap:        lockmap.New(),
		VideoQuality:   videoQuality,
		ArchiveFile:    archiveFile,
		OutputTemplate: outputTemplate,
	}
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
			"-S", ytdl.VideoQuality,
			"--recode-video", "mp4",
			"-o", ytdl.OutputTemplate,
			"--download-archive", ytdl.ArchiveFile,
			fmt.Sprintf("https://www.youtube.com/playlist?list=%s", playlist))
		ytdlCommand.Stdout = &zapWriter
		ytdlCommand.Stderr = &zapWriter

		ytdl.logger.Info("command run", zap.String("command", ytdlCommand.String()))

		err = ytdlCommand.Start()
		if err != nil {
			ytdl.logger.Error("yt-playlist-ripper failed to run ytdl", zap.Error(err))
			return
		}

		err = ytdlCommand.Wait()
		if err != nil {
			ytdl.logger.Error("yt-playlist-ripper failed to exit successfully", zap.Error(err))
		}
	}
}
