package ytdl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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

	ArchiveModePerChannel = "per-channel"
	ArchiveModeGlobal     = "global"

	// bootstrapDir holds per-channel sentinel files. Presence of
	// {OUTPUT_ROOT}/{bootstrapDir}/{channelID}.done means we've already
	// fetched the channel-level info.json and don't need to call YouTube
	// again on subsequent ticks. Delete the file to force a re-bootstrap.
	bootstrapDir = ".bootstrap"
)

// PlaylistLockKey and ChannelLockKey namespace lockmap entries so a playlist
// ID and channel ID that happen to share a string can't collide.
func PlaylistLockKey(id string) string { return "playlist:" + id }
func ChannelLockKey(id string) string  { return "channel:" + id }

func New(ctx context.Context, lockMap *lockmap.LockMap, c *config.Config, n *notifier.Notifier) *YTDLPClient {
	return &YTDLPClient{
		LockMap:  lockMap,
		ctx:      ctx,
		c:        c,
		notifier: n,
	}
}

// execResult captures the outcome of a single yt-dlp invocation.
type execResult struct {
	waitErr        error
	sawError       bool
	capturedIssues []string
}

// execute starts yt-dlp with the given Command, scans stdout/stderr for
// ERROR:/WARNING: lines, and waits for it to exit. label is used for slog only.
func (ytdlClient *YTDLPClient) execute(ctx context.Context, label string, command *Command) execResult {
	r, w := io.Pipe()

	ytdlCommand := exec.CommandContext(ctx, command.bin, command.args...)
	ytdlCommand.Stdout = w
	ytdlCommand.Stderr = w

	var (
		scannerWG sync.WaitGroup
		result    execResult
	)
	scannerWG.Go(func() {
		scanner := bufio.NewScanner(r)

		// 1MB buffer size for scanner
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			slog.Info("yt-dlp output", slog.String("output", line))
			switch {
			case strings.HasPrefix(line, "ERROR:"):
				result.sawError = true
				result.capturedIssues = append(result.capturedIssues, line)
			case strings.HasPrefix(line, "WARNING:"):
				result.capturedIssues = append(result.capturedIssues, line)
			default:
				continue
			}
			if len(result.capturedIssues) > maxCapturedIssues {
				result.capturedIssues = result.capturedIssues[len(result.capturedIssues)-maxCapturedIssues:]
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Error("yt-dlp output error", slog.String("error", err.Error()))
		}
	})

	if !ytdlClient.c.Quiet {
		slog.Info("command run", slog.String("command", ytdlCommand.String()), slog.String("target", label))
	}

	if err := ytdlCommand.Start(); err != nil {
		_ = w.Close()
		scannerWG.Wait()
		result.waitErr = fmt.Errorf("start: %w", err)
		return result
	}

	result.waitErr = ytdlCommand.Wait()
	if closeErr := w.Close(); closeErr != nil {
		slog.Error("failed to close pipe writer", slog.String("error", closeErr.Error()))
	}
	scannerWG.Wait()
	return result
}

// reportResult logs and (on failure) notifies based on an execResult.
func (ytdlClient *YTDLPClient) reportResult(ctx context.Context, label string, r execResult) {
	switch {
	case r.waitErr != nil:
		slog.Error("yt-dlp command failed to run", slog.String("error", r.waitErr.Error()), slog.String("target", label))
		body := fmt.Sprintf("%s failed (%s)", label, r.waitErr.Error())
		if summary := summarizeIssues(r.capturedIssues); summary != "" {
			body = body + "\n\n" + summary
		}
		ytdlClient.notifier.Send(ctx, "yt-playlist-ripper", body)
	case r.sawError:
		slog.Warn("yt-dlp command finished with errors", slog.String("target", label))
		body := fmt.Sprintf("%s finished with errors", label)
		if summary := summarizeIssues(r.capturedIssues); summary != "" {
			body = body + "\n\n" + summary
		}
		ytdlClient.notifier.Send(ctx, "yt-playlist-ripper", body)
	default:
		slog.Info("yt-dlp command finished", slog.String("target", label))
	}
}

// verbosityOption appends --verbose or --quiet according to config, mirroring
// how the legacy runner picked between them.
func (ytdlClient *YTDLPClient) verbosityOption() (CommandOption, bool) {
	switch {
	case ytdlClient.c.Verbose && !ytdlClient.c.Quiet:
		return WithVerbose(), true
	case ytdlClient.c.Quiet && !ytdlClient.c.Verbose:
		return WithQuiet(), true
	}
	return nil, false
}

// jellyfinSyncOptions returns the shared CommandOption set used for any
// Jellyfin-shaped sync (per-playlist or per-channel). The caller appends the
// target URL last.
func (ytdlClient *YTDLPClient) jellyfinSyncOptions(archiveFile string) []CommandOption {
	videoTemplate := filepath.Join(ytdlClient.c.OutputRoot, ytdlClient.c.JellyfinVideoTemplate)
	opts := []CommandOption{
		WithFormat(ytdlClient.c.Format),
		WithForceIPv4(),
		WithSleepRequests(ytdlClient.c.SleepRequests),
		WithSleepInterval(ytdlClient.c.SleepInterval),
		WithMaxSleepInterval(ytdlClient.c.MaxSleepInterval),
		WithIgnoreErrors(),
		WithNoContinue(),
		WithNoOverwrites(),
		WithNoProgress(),
		WithRestrictFilenames(),
		WithDownloadArchive(archiveFile),
		WithAddMetadata(),
		WithEmbedChapters(),
		WithParseMetadata(ParseMetadataTitleMetaTitle),
		WithParseMetadata(ParseMetadataUploaderMetaArtist),
		WithWriteInfoJSON(),
		WithWriteThumbnail(),
		WithConvertThumbnails("jpg"),
		WithWriteSubs(),
		WithSubLangs(ytdlClient.c.SubLangs),
		WithSubFormat(ytdlClient.c.SubFormat),
		WithEmbedSubs(),
		WithCheckFormats(),
		WithConcurrentFragments(ytdlClient.c.ConcurrentFragments),
		WithMatchFilter(MatchFilter),
		WithOutputTemplate(videoTemplate),
		WithMergeOutputFormat(ytdlClient.c.MergeOutputFormat),
		WithThrottledRate(ytdlClient.c.ThrottledRate),
		WithJSRuntime("bun"),
	}
	if ytdlClient.c.WriteAutoSubs {
		opts = append(opts, WithWriteAutoSubs())
	}
	if opt, ok := ytdlClient.verbosityOption(); ok {
		opts = append(opts, opt)
	}
	return opts
}

// bootstrapSentinel returns the path to the per-channel "already bootstrapped"
// marker file.
func (ytdlClient *YTDLPClient) bootstrapSentinel(channelID string) string {
	return filepath.Join(ytdlClient.c.OutputRoot, bootstrapDir, channelID+".done")
}

func (ytdlClient *YTDLPClient) channelBootstrapped(channelID string) bool {
	_, err := os.Stat(ytdlClient.bootstrapSentinel(channelID))
	return err == nil
}

func (ytdlClient *YTDLPClient) markChannelBootstrapped(channelID string) {
	path := ytdlClient.bootstrapSentinel(channelID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Warn("could not create bootstrap dir", slog.String("path", filepath.Dir(path)), slog.String("error", err.Error()))
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Warn("could not write bootstrap sentinel", slog.String("path", path), slog.String("error", err.Error()))
		return
	}
	_ = f.Close()
}

// bootstrapChannel runs the metadata-only pass that writes a channel-level
// info.json at the show root. It is a no-op if the per-channel sentinel
// already exists. Returns true if the pass actually ran without yt-dlp
// reporting any errors (so the caller can decide whether to write the
// sentinel).
func (ytdlClient *YTDLPClient) bootstrapChannel(ctx context.Context, channelID, label string) bool {
	if ytdlClient.channelBootstrapped(channelID) {
		slog.Debug("bootstrap skipped (sentinel present)", slog.String("channel", channelID))
		return false
	}

	bootstrapTemplate := filepath.Join(ytdlClient.c.OutputRoot, ytdlClient.c.JellyfinInfoTemplate)
	opts := []CommandOption{
		WithForceIPv4(),
		WithSleepRequests(ytdlClient.c.SleepRequests),
		WithIgnoreErrors(),
		WithNoOverwrites(),
		WithNoProgress(),
		WithSkipDownload(),
		WithWriteInfoJSON(),
		// Also write the channel banner/avatar alongside the info.json so
		// the jellyfin-youtube-metadata-plugin's local image provider
		// finds it. The plugin walks the show root for *.jpg / *.webp
		// matching a 24-char YouTube channel ID inside [brackets], which
		// is exactly what our JellyfinInfoTemplate produces.
		WithWriteThumbnail(),
		WithConvertThumbnails("jpg"),
		WithPlaylistItems("0"),
		WithRestrictFilenames(),
		WithOutputTemplate(bootstrapTemplate),
		WithJSRuntime("bun"),
		WithString(fmt.Sprintf("https://www.youtube.com/channel/%s/videos", channelID)),
	}
	if opt, ok := ytdlClient.verbosityOption(); ok {
		opts = append(opts, opt)
	}

	res := ytdlClient.execute(ctx, label, NewCommand("yt-dlp", opts...))
	ytdlClient.reportResult(ctx, label, res)
	if res.waitErr == nil && !res.sawError {
		ytdlClient.markChannelBootstrapped(channelID)
		return true
	}
	return false
}

// enumeratePlaylistChannels lists the unique channel IDs that appear in a
// playlist, using yt-dlp's --flat-playlist mode (single fast call, no
// downloads). Returns nil on error.
func (ytdlClient *YTDLPClient) enumeratePlaylistChannels(ctx context.Context, playlistURL string) []string {
	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--flat-playlist",
		"--no-warnings",
		"--print", "%(channel_id)s",
		playlistURL,
	)
	out, err := cmd.Output()
	if err != nil {
		slog.Warn("could not enumerate channels", slog.String("playlist_url", playlistURL), slog.String("error", err.Error()))
		return nil
	}

	seen := map[string]struct{}{}
	var ids []string
	for line := range strings.SplitSeq(string(out), "\n") {
		id := strings.TrimSpace(line)
		if id == "" || id == "NA" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func (ytdlClient *YTDLPClient) Run(playlist string) func() {
	return func() {
		key := PlaylistLockKey(playlist)
		if err := ytdlClient.LockMap.Lock(key); err != nil {
			slog.Error("failed to acquire lock", slog.String("playlist", playlist), slog.String("error", err.Error()))
			return
		}
		defer func() {
			if err := ytdlClient.LockMap.Unlock(key); err != nil {
				slog.Error("failed to release lock", slog.String("playlist", playlist), slog.String("error", err.Error()))
			}
		}()

		ctx := ytdlClient.ctx
		playlistURL := fmt.Sprintf("https://www.youtube.com/playlist?list=%s", playlist)

		if ytdlClient.c.JellyfinLayout {
			ytdlClient.runPlaylistJellyfin(ctx, playlist, playlistURL)
			return
		}
		ytdlClient.runPlaylistLegacy(ctx, playlist, playlistURL)
	}
}

// runPlaylistLegacy preserves the historical layout: bucketed by
// "{playlist} - ({uploader})", one subdir per video.
func (ytdlClient *YTDLPClient) runPlaylistLegacy(ctx context.Context, playlist, playlistURL string) {
	opts := []CommandOption{
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
		WithString(playlistURL),
	}
	if opt, ok := ytdlClient.verbosityOption(); ok {
		opts = append(opts, opt)
	}

	label := fmt.Sprintf("playlist %s", playlist)
	ytdlClient.reportResult(ctx, label, ytdlClient.execute(ctx, label, NewCommand("yt-dlp", opts...)))
}

// runPlaylistJellyfin emits a layout the jellyfin-youtube-metadata-plugin can
// consume: one Show per uploader, Season {YYYY} subfolders. The single
// playlist may contain videos from many channels — each unique uploader gets
// its own show root, and the channel-level info.json is bootstrapped per
// channel (gated by a sentinel so we don't re-hit YouTube on every tick).
func (ytdlClient *YTDLPClient) runPlaylistJellyfin(ctx context.Context, playlist, playlistURL string) {
	if err := os.MkdirAll(filepath.Dir(ytdlClient.c.ArchiveFile), 0o755); err != nil {
		slog.Warn("could not create archive dir", slog.String("path", filepath.Dir(ytdlClient.c.ArchiveFile)), slog.String("error", err.Error()))
	}

	// Bootstrap every channel that appears in the playlist (skipping ones
	// we've already bootstrapped). This produces the show-root info.json
	// the plugin's local provider looks for.
	channelIDs := ytdlClient.enumeratePlaylistChannels(ctx, playlistURL)
	slog.Info("playlist channels enumerated",
		slog.String("playlist", playlist),
		slog.Int("unique_channels", len(channelIDs)),
	)
	for _, channelID := range channelIDs {
		label := fmt.Sprintf("playlist %s -> channel %s (bootstrap)", playlist, channelID)
		ytdlClient.bootstrapChannel(ctx, channelID, label)
	}

	// Main sync — same flag set as RunChannel, just pointed at the playlist
	// URL. yt-dlp uses %(uploader)s in the template to split videos into
	// the correct show roots automatically.
	opts := ytdlClient.jellyfinSyncOptions(ytdlClient.c.ArchiveFile)
	opts = append(opts, WithString(playlistURL))
	label := fmt.Sprintf("playlist %s (jellyfin)", playlist)
	ytdlClient.reportResult(ctx, label, ytdlClient.execute(ctx, label, NewCommand("yt-dlp", opts...)))
}

// RunChannel returns a cron-compatible func that mirrors a single channel
// directly (no playlist intermediary) into the Jellyfin-shaped layout.
func (ytdlClient *YTDLPClient) RunChannel(channelID string) func() {
	return func() {
		key := ChannelLockKey(channelID)
		if err := ytdlClient.LockMap.Lock(key); err != nil {
			slog.Error("failed to acquire lock", slog.String("channel", channelID), slog.String("error", err.Error()))
			return
		}
		defer func() {
			if err := ytdlClient.LockMap.Unlock(key); err != nil {
				slog.Error("failed to release lock", slog.String("channel", channelID), slog.String("error", err.Error()))
			}
		}()

		ctx := ytdlClient.ctx
		channelURL := fmt.Sprintf("https://www.youtube.com/channel/%s/videos", channelID)
		archiveFile := ytdlClient.channelArchiveFile(channelID)
		if err := os.MkdirAll(filepath.Dir(archiveFile), 0o755); err != nil {
			slog.Warn("could not create archive dir", slog.String("path", filepath.Dir(archiveFile)), slog.String("error", err.Error()))
		}

		bootstrapLabel := fmt.Sprintf("channel %s (bootstrap)", channelID)
		ytdlClient.bootstrapChannel(ctx, channelID, bootstrapLabel)

		opts := ytdlClient.jellyfinSyncOptions(archiveFile)
		opts = append(opts, WithString(channelURL))
		syncLabel := fmt.Sprintf("channel %s (sync)", channelID)
		ytdlClient.reportResult(ctx, syncLabel, ytdlClient.execute(ctx, syncLabel, NewCommand("yt-dlp", opts...)))
	}
}

func (ytdlClient *YTDLPClient) channelArchiveFile(channelID string) string {
	if ytdlClient.c.ArchiveMode == ArchiveModeGlobal {
		return ytdlClient.c.ArchiveFile
	}
	return filepath.Join(ytdlClient.c.OutputRoot, ".archives", channelID+".txt")
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
