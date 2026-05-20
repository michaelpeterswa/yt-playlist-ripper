package ytdl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/config"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/notifier"
)

type YTDLPClient struct {
	LockMap  *lockmap.LockMap
	ctx      context.Context
	c        *config.Config
	notifier *notifier.Notifier
}

const (
	ParseMetadataTitleMetaTitle     = "%(title)s:%(meta_title)s"
	ParseMetadataUploaderMetaArtist = "%(uploader)s:%(meta_artist)s"
	MatchFilter                     = "!is_live & !live"
)

func New(ctx context.Context, lockMap *lockmap.LockMap, c *config.Config, n *notifier.Notifier) *YTDLPClient {
	return &YTDLPClient{
		LockMap:  lockMap,
		ctx:      ctx,
		c:        c,
		notifier: n,
	}
}

func (ytdlClient *YTDLPClient) Run(playlist string) func() {
	return func() {
		err := ytdlClient.LockMap.Lock(playlist)
		if err != nil {
			slog.Error("failed to acquire lock", slog.String("playlist", playlist), slog.String("error", err.Error()))
			return
		}
		defer func() {
			err := ytdlClient.LockMap.Unlock(playlist)
			if err != nil {
				slog.Error("failed to release lock", slog.String("playlist", playlist), slog.String("error", err.Error()))
			}
		}()

		ctx := ytdlClient.ctx

		commandOptions := []CommandOption{
			WithFormat(ytdlClient.c.Format),
			WithForceIPv4(),
			WithSleepRequests(ytdlClient.c.SleepRequests),
			WithSleepInterval(ytdlClient.c.SleepInterval),
			WithMaxSleepInterval(ytdlClient.c.MaxSleepInterval),
			WithIgnoreErrors(),
			WithNoContinue(),
			WithNoOverwrites(),
			WithNoProgress(),
			WithDownloadArchive(ytdlClient.c.ArchiveFile),
			WithAddMetadata(),
			WithParseMetadata(ParseMetadataTitleMetaTitle),
			WithParseMetadata(ParseMetadataUploaderMetaArtist),
			WithWriteDescription(),
			WithWriteInfoJSON(),
			WithWriteThumbnail(),
			WithEmbedThumbnail(),
			WithWriteSubs(),
			WithSubLangs("all"),
			WithEmbedSubs(),
			WithCheckFormats(),
			WithConcurrentFragments(ytdlClient.c.ConcurrentFragments),
			WithMatchFilter(MatchFilter),
			WithOutputTemplate(ytdlClient.c.OutputTemplate),
			WithMergeOutputFormat(ytdlClient.c.MergeOutputFormat),
			WithThrottledRate(ytdlClient.c.ThrottledRate),
			WithJSRuntime("bun"),
			WithString(fmt.Sprintf("https://www.youtube.com/playlist?list=%s", playlist)),
		}

		if ytdlClient.c.Verbose && !ytdlClient.c.Quiet {
			commandOptions = append(commandOptions, WithVerbose())
		} else if ytdlClient.c.Quiet && !ytdlClient.c.Verbose {
			commandOptions = append(commandOptions, WithQuiet())
		}

		command := NewCommand(
			"yt-dlp",
			commandOptions...,
		)

		r, w := io.Pipe()

		ytdlCommand := exec.CommandContext(ctx, command.bin, command.args...)
		ytdlCommand.Stdout = w
		ytdlCommand.Stderr = w

		var scannerWG sync.WaitGroup
		scannerWG.Add(1)
		go func() {
			defer scannerWG.Done()
			scanner := bufio.NewScanner(r)

			// 1MB buffer size for scanner
			buf := make([]byte, 0, 64*1024)
			scanner.Buffer(buf, 1024*1024)

			for scanner.Scan() {
				slog.Info("yt-dlp output", slog.String("output", scanner.Text()))
			}
			if err := scanner.Err(); err != nil {
				slog.Error("yt-dlp output error", slog.String("error", err.Error()))
			}
		}()

		if !ytdlClient.c.Quiet {
			slog.Info("command run", slog.String("command", ytdlCommand.String()), slog.String("playlist", playlist))
		}

		err = ytdlCommand.Start()
		if err != nil {
			_ = w.Close()
			scannerWG.Wait()
			slog.Error("yt-dlp command failed to start", slog.String("error", err.Error()), slog.String("command", ytdlCommand.String()))
			ytdlClient.notifier.Send(ctx, "yt-playlist-ripper", fmt.Sprintf("playlist %s failed to start: %s", playlist, err.Error()))
			return
		}

		waitErr := ytdlCommand.Wait()
		if closeErr := w.Close(); closeErr != nil {
			slog.Error("failed to close pipe writer", slog.String("error", closeErr.Error()))
		}
		scannerWG.Wait()

		if waitErr != nil {
			slog.Error("yt-dlp command failed to run", slog.String("error", waitErr.Error()), slog.String("command", ytdlCommand.String()))
			ytdlClient.notifier.Send(ctx, "yt-playlist-ripper", fmt.Sprintf("playlist %s failed: %s", playlist, waitErr.Error()))
			return
		}
		slog.Info("yt-dlp command finished", slog.String("playlist", playlist))
	}
}
