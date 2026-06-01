// Command backfill-images walks a Jellyfin-layout yt-playlist-ripper library
// and writes channel artwork (poster.* + backdrop.*) into each show root,
// derived from the channel-level info.json already on disk.
//
// For every "{uploader} - NA - {playlist_title} [{channel_id}].info.json"
// the bootstrap pass wrote, it reads the `thumbnails` array, selects the
// best square avatar (-> poster) and widest banner (-> backdrop), downloads
// them, and drops them next to the info.json. Jellyfin's built-in local
// image provider then displays them with no plugin or API quota involved.
//
// Run dry-run first:
//
//	go run ./cmd/backfill-images --root /downloads
//
// Then apply:
//
//	go run ./cmd/backfill-images --root /downloads --apply
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/channelimage"
)

// channelInfoMarker is the " - NA - " infix the bootstrap puts in
// channel-level info.json filenames (from JELLYFIN_INFO_TEMPLATE), which
// distinguishes them from per-video info.json files.
const channelInfoMarker = " - NA - "

func main() {
	var (
		root  = flag.String("root", "/downloads", "downloads root to scan")
		apply = flag.Bool("apply", false, "actually download images; otherwise dry-run")
		force = flag.Bool("force", false, "overwrite existing poster/backdrop images")
	)
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if *root == "" {
		slog.Error("--root is required")
		os.Exit(2)
	}
	if _, err := os.Stat(*root); err != nil {
		slog.Error("--root not accessible", slog.String("root", *root), slog.String("error", err.Error()))
		os.Exit(2)
	}

	infos, err := findChannelInfos(*root)
	if err != nil {
		slog.Error("scan failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if len(infos) == 0 {
		slog.Info("no channel info.json files found; nothing to do")
		return
	}

	hc := &http.Client{Timeout: 30 * time.Second}
	ctx := context.Background()

	var planned, posters, backdrops, skipped int
	for _, infoPath := range infos {
		destDir := filepath.Dir(infoPath)
		thumbs, err := channelimage.ReadThumbnails(infoPath)
		if err != nil {
			slog.Warn("could not read thumbnails", slog.String("info_json", infoPath), slog.String("error", err.Error()))
			continue
		}

		poster, hasPoster := channelimage.SelectPoster(thumbs)
		backdrop, hasBackdrop := channelimage.SelectBackdrop(thumbs)
		if !hasPoster && !hasBackdrop {
			slog.Warn("no usable thumbnails in info.json", slog.String("info_json", infoPath))
			continue
		}
		planned++

		if !*apply {
			if hasPoster {
				dryRun(destDir, channelimage.PosterStem, poster.URL, *force)
			}
			if hasBackdrop {
				dryRun(destDir, channelimage.BackdropStem, backdrop.URL, *force)
			}
			continue
		}

		if hasPoster {
			if download(ctx, hc, poster.URL, destDir, channelimage.PosterStem, *force) {
				posters++
			} else {
				skipped++
			}
		}
		if hasBackdrop {
			if download(ctx, hc, backdrop.URL, destDir, channelimage.BackdropStem, *force) {
				backdrops++
			} else {
				skipped++
			}
		}
	}

	if !*apply {
		slog.Info("dry-run complete; pass --apply to download", slog.Int("channels", planned))
		return
	}
	slog.Info("backfill complete",
		slog.Int("channels", planned),
		slog.Int("posters_written", posters),
		slog.Int("backdrops_written", backdrops),
		slog.Int("skipped_existing", skipped),
	)
}

func dryRun(destDir, stem, url string, force bool) {
	if existing := channelimage.Existing(destDir, stem); existing != "" && !force {
		slog.Info("DRY-RUN skip (exists)", slog.String("path", existing))
		return
	}
	slog.Info("DRY-RUN write", slog.String("dest", filepath.Join(destDir, stem+".*")), slog.String("from", url))
}

func download(ctx context.Context, hc *http.Client, url, destDir, stem string, force bool) bool {
	path, written, err := channelimage.Download(ctx, hc, url, destDir, stem, force)
	if err != nil {
		slog.Warn("download failed", slog.String("stem", stem), slog.String("url", url), slog.String("error", err.Error()))
		return false
	}
	if written {
		slog.Info("wrote image", slog.String("path", path))
	} else {
		slog.Debug("skipped existing image", slog.String("path", path))
	}
	return written
}

// findChannelInfos walks root for channel-level info.json files, skipping the
// runtime's state directories.
func findChannelInfos(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", slog.String("path", path), slog.String("error", err.Error()))
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); name == ".bootstrap" || name == ".archives" {
				return fs.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".info.json") && strings.Contains(base, channelInfoMarker) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
