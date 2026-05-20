package ytdl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/config"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"github.com/michaelpeterswa/yt-playlist-ripper/internal/notifier"
)

const (
	maxCapturedIssues = 50
	maxNotifyBytes    = 900
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

		var (
			scannerWG      sync.WaitGroup
			capturedIssues []string
		)
		scannerWG.Go(func() {
			scanner := bufio.NewScanner(r)

			// 1MB buffer size for scanner
			buf := make([]byte, 0, 64*1024)
			scanner.Buffer(buf, 1024*1024)

			for scanner.Scan() {
				line := scanner.Text()
				slog.Info("yt-dlp output", slog.String("output", line))
				if strings.HasPrefix(line, "ERROR:") || strings.HasPrefix(line, "WARNING:") {
					capturedIssues = append(capturedIssues, line)
					if len(capturedIssues) > maxCapturedIssues {
						capturedIssues = capturedIssues[len(capturedIssues)-maxCapturedIssues:]
					}
				}
			}
			if err := scanner.Err(); err != nil {
				slog.Error("yt-dlp output error", slog.String("error", err.Error()))
			}
		})

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
			body := fmt.Sprintf("playlist %s failed (%s)", playlist, waitErr.Error())
			if summary := summarizeIssues(capturedIssues); summary != "" {
				body = body + "\n\n" + summary
			}
			ytdlClient.notifier.Send(ctx, "yt-playlist-ripper", body)
			return
		}
		slog.Info("yt-dlp command finished", slog.String("playlist", playlist))
	}
}

// summarizeIssues dedupes a slice of yt-dlp ERROR/WARNING lines into a stable
// first-occurrence order with occurrence counts, and trims to maxNotifyBytes
// (favouring the tail so the freshest context survives Pushover's body limit).
func summarizeIssues(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	seen := map[string]int{}
	order := make([]string, 0, len(lines))
	for _, l := range lines {
		if _, ok := seen[l]; !ok {
			order = append(order, l)
		}
		seen[l]++
	}

	out := make([]string, 0, len(order))
	for _, l := range order {
		if seen[l] > 1 {
			out = append(out, fmt.Sprintf("%s (×%d)", l, seen[l]))
		} else {
			out = append(out, l)
		}
	}

	summary := strings.Join(out, "\n")
	if len(summary) <= maxNotifyBytes {
		return summary
	}
	for len(summary) > maxNotifyBytes {
		idx := strings.IndexByte(summary, '\n')
		if idx < 0 {
			return summary[len(summary)-maxNotifyBytes:]
		}
		summary = summary[idx+1:]
	}
	return "…\n" + summary
}
