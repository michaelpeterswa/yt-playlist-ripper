package ytdl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/config"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/telegram"
)

type YTDLPClient struct {
	LockMap        *lockmap.LockMap
	c              *config.Config
	telegramClient *telegram.TelegramClient
}

const (
	ParseMetadataTitleMetaTitle     = "%(title)s:%(meta_title)s"
	ParseMetadataUploaderMetaArtist = "%(uploader)s:%(meta_artist)s"
	MatchFilter                     = "!is_live & !live"
)

func New(lockMap *lockmap.LockMap, c *config.Config, tc *telegram.TelegramClient) *YTDLPClient {
	return &YTDLPClient{
		LockMap:        lockmap.New(),
		c:              c,
		telegramClient: tc,
	}
}

func (ytdlClient *YTDLPClient) Run(playlist string) func() {
	ctx := context.Background()

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

		r, w := io.Pipe()

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
			WithAllSubs(),
			WithEmbedSubs(),
			WithCheckFormats(),
			WithConcurrentFragments(ytdlClient.c.ConcurrentFragments),
			WithMatchFilter(MatchFilter),
			WithOutputTemplate(ytdlClient.c.OutputTemplate),
			WithMergeOutputFormat(ytdlClient.c.MergeOutputFormat),
			WithThrottledRate(ytdlClient.c.ThrottledRate),
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

		ytdlCommand := exec.Command(command.bin, command.args...)
		ytdlCommand.Stdout = w
		ytdlCommand.Stderr = w

		go func() {
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

		defer func() {
			err := w.Close()
			if err != nil {
				slog.Error("failed to close pipe writer", slog.String("error", err.Error()))
			}
		}()

		if !ytdlClient.c.Quiet {
			slog.Info("command run", slog.String("command", ytdlCommand.String()), slog.String("playlist", playlist))
		}

		ytdlClient.telegramClient.SendMessage(ctx, telegram.MessageString(telegram.Bold("playlist "), telegram.Code(playlist), telegram.Bold(" is running")))
		err = ytdlCommand.Start()
		if err != nil {
			slog.Error("yt-dlp command failed to start", slog.String("error", err.Error()), slog.String("command", ytdlCommand.String()))
			ytdlClient.telegramClient.SendMessage(ctx, telegram.MessageString(telegram.Bold("playlist "), telegram.Code(playlist), telegram.Bold(" has failed to start")))
			return
		}

		err = ytdlCommand.Wait()
		if err != nil {
			slog.Error("yt-dlp command failed to run", slog.String("error", err.Error()), slog.String("command", ytdlCommand.String()))
			ytdlClient.telegramClient.SendMessage(ctx, telegram.MessageString(telegram.Bold("playlist "), telegram.Code(playlist), telegram.Bold(" has failed")))
			return
		}
		slog.Info("yt-dlp command finished", slog.String("playlist", playlist))
		ytdlClient.telegramClient.SendMessage(ctx, telegram.MessageString(telegram.Bold("playlist "), telegram.Code(playlist), telegram.Bold(" has finished")))
	}
}
