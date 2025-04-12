package ytdl

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os/exec"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
)

type YTDLPClient struct {
	LockMap        *lockmap.LockMap
	VideoQuality   string
	ArchiveFile    string
	OutputTemplate string
}

func New(lockMap *lockmap.LockMap, videoQuality string, archiveFile string, outputTemplate string) *YTDLPClient {
	return &YTDLPClient{
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
			slog.Error("failed to acquire lock", slog.String("playlist", playlist), slog.String("error", err.Error()))
			return
		}
		defer func() {
			err := ytdl.LockMap.Unlock(playlist)
			if err != nil {
				slog.Error("failed to release lock", slog.String("playlist", playlist), slog.String("error", err.Error()))
			}
		}()

		r, w := io.Pipe()

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
		ytdlCommand.Stdout = w
		ytdlCommand.Stderr = w

		go func() {
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				slog.Info("yt-dlp output", slog.String("output", scanner.Text()))
			}
			if err := scanner.Err(); err != nil {
				slog.Error("yt-dlp output error", slog.String("error", err.Error()))
			}
		}()

		defer func() {
			err := w.Close()
			if err != nil {
				slog.Error("failed to close pipe writer", slog.String("error", err.Error()))
			}
		}()

		slog.Info("command run", slog.String("command", ytdlCommand.String()), slog.String("playlist", playlist))

		err = ytdlCommand.Start()
		if err != nil {
			slog.Error("yt-dlp command failed to start", slog.String("error", err.Error()), slog.String("command", ytdlCommand.String()))
			return
		}

		err = ytdlCommand.Wait()
		if err != nil {
			slog.Error("yt-dlp command failed to run", slog.String("error", err.Error()), slog.String("command", ytdlCommand.String()))
			return
		}
	}
}
