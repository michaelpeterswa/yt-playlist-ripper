// Command migrate-to-jellyfin relocates a yt-playlist-ripper library from
// the legacy on-disk layout
//
//	{playlist} - ({uploader})/{YYYYMMDD} - {title}/{YYYYMMDD} - {title} [{id}].{ext}
//
// to the Jellyfin-compatible layout
//
//	{uploader}/Season {YYYY}/{uploader} - {YYYYMMDD} - {title} [{id}].{ext}
//
// expected by the jellyfin-youtube-metadata-plugin.
//
// The transform is purely a rename: each per-video info.json is located,
// its uploader and upload_date are read, and every sibling file sharing
// the legacy base name (video, info.json, thumbnail, description,
// subtitle tracks) is moved together so the plugin's "byte-identical base
// name" invariant is preserved.
//
// Run dry-run first:
//
//	go run ./cmd/migrate-to-jellyfin --root /downloads
//
// Then apply:
//
//	go run ./cmd/migrate-to-jellyfin --root /downloads --apply
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// videoInfo captures the fields we need from a per-video .info.json. We
// deliberately do NOT consume Title/Ext: preserving the existing base
// filename verbatim sidesteps having to perfectly mirror yt-dlp's title
// sanitization rules.
type videoInfo struct {
	ID         string `json:"id"`
	Uploader   string `json:"uploader"`
	UploadDate string `json:"upload_date"` // YYYYMMDD
}

// migration is a single video's planned relocation.
type migration struct {
	src      string   // current info.json path (drives parent-dir discovery)
	parent   string   // dir containing src and its siblings
	dstDir   string   // target {root}/{uploader}/Season {YYYY}/
	oldBase  string   // basename(src) minus ".info.json"
	uploader string   // sanitized uploader, used as both the show-root dir and the prepended name prefix
	siblings []string // absolute paths to all files in `parent` sharing oldBase
}

// targetInfoJSON returns where this plan would write its info.json — used
// as the dedup key when two legacy paths resolve to the same Jellyfin path.
func (m migration) targetInfoJSON() string {
	return filepath.Join(m.dstDir, m.uploader+" - "+m.oldBase+".info.json")
}

// planResult bundles the dedup outcome from planMigrations. Each entry in
// duplicates is a slice of source info.json paths that all want the same
// target; the first is kept (and is also present in plans), the rest are
// reported and skipped.
type planResult struct {
	plans      []migration
	duplicates [][]string
}

func main() {
	var (
		root     = flag.String("root", "/downloads", "downloads root to scan")
		apply    = flag.Bool("apply", false, "actually move files; otherwise dry-run")
		restrict = flag.Bool("restrict-filenames", true, "apply yt-dlp-style restricted (ASCII) sanitization to the uploader directory name; should match the runtime's RESTRICT setting")
		keep     = flag.Bool("keep-empty-dirs", false, "do not remove now-empty legacy per-video or playlist directories")
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

	result, err := planMigrations(*root, *restrict)
	if err != nil {
		slog.Error("planning failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	plans := result.plans
	sort.Slice(plans, func(i, j int) bool { return plans[i].src < plans[j].src })

	// Report duplicate sources up-front (visible in both dry-run and apply).
	for _, dup := range result.duplicates {
		slog.Warn("duplicate sources resolve to the same target; keeping the first and leaving the rest at their legacy locations",
			slog.String("kept", dup[0]),
			slog.Any("left_in_place", dup[1:]),
		)
	}

	if len(plans) == 0 && len(result.duplicates) == 0 {
		slog.Info("nothing to migrate")
		return
	}

	if !*apply {
		for _, p := range plans {
			for _, s := range p.siblings {
				newName := p.uploader + " - " + filepath.Base(s)
				fmt.Printf("DRY-RUN  %s\n     ->  %s\n", s, filepath.Join(p.dstDir, newName))
			}
		}
		slog.Info("dry-run complete; pass --apply to commit",
			slog.Int("videos", len(plans)),
			slog.Int("duplicate_groups", len(result.duplicates)),
		)
		return
	}

	var moved, skipped, conflicts int
	emptyDirs := map[string]struct{}{}
	for _, p := range plans {
		// Pre-flight: if any sibling target already exists, skip the whole
		// plan. Moving only some siblings would leave the plugin unable to
		// satisfy its "byte-identical base name" invariant.
		var blockers []string
		for _, s := range p.siblings {
			dst := filepath.Join(p.dstDir, p.uploader+" - "+filepath.Base(s))
			if s == dst {
				continue
			}
			if _, err := os.Stat(dst); err == nil {
				blockers = append(blockers, dst)
			}
		}
		if len(blockers) > 0 {
			slog.Warn("destination already exists; skipping plan to keep siblings together",
				slog.String("info_json_src", p.src),
				slog.Any("existing_targets", blockers),
			)
			conflicts++
			continue
		}

		if err := os.MkdirAll(p.dstDir, 0o755); err != nil {
			slog.Error("mkdir failed", slog.String("dir", p.dstDir), slog.String("error", err.Error()))
			skipped++
			continue
		}
		ok := true
		for _, s := range p.siblings {
			dst := filepath.Join(p.dstDir, p.uploader+" - "+filepath.Base(s))
			if s == dst {
				continue
			}
			if err := os.Rename(s, dst); err != nil {
				slog.Error("move failed", slog.String("src", s), slog.String("dst", dst), slog.String("error", err.Error()))
				ok = false
				break
			}
		}
		if !ok {
			skipped++
			continue
		}
		moved++
		emptyDirs[p.parent] = struct{}{}
	}

	if !*keep {
		// Tear down empty legacy per-video subdirs and, if they end up
		// empty, the playlist dir above them. We collect parent dirs first
		// and remove deepest-first so cascading cleanup works.
		dirs := make([]string, 0, len(emptyDirs))
		for d := range emptyDirs {
			dirs = append(dirs, d)
		}
		// Also try the grandparents (playlist-level dirs) after.
		sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
		for _, d := range dirs {
			tryRemoveIfEmpty(d)
			tryRemoveIfEmpty(filepath.Dir(d))
		}
	}

	slog.Info("migration complete",
		slog.Int("moved", moved),
		slog.Int("skipped", skipped),
		slog.Int("conflicts", conflicts),
		slog.Int("duplicate_groups", len(result.duplicates)),
	)
}

func planMigrations(root string, restrict bool) (planResult, error) {
	var raw []migration
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Don't abort the whole walk for one unreadable subtree.
			slog.Warn("walk error", slog.String("path", path), slog.String("error", err.Error()))
			return nil
		}
		if d.IsDir() {
			// Skip files we wrote ourselves to track state.
			base := d.Name()
			if base == ".bootstrap" || base == ".archives" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".info.json") {
			return nil
		}
		// Skip channel-level info.json files (these use " - NA - " in the
		// name from JELLYFIN_INFO_TEMPLATE; the plugin's local provider
		// expects them, so leave them alone).
		if strings.Contains(filepath.Base(path), " - NA - ") {
			return nil
		}
		// Skip files already in the Jellyfin layout (parent is
		// "Season YYYY"). Catches re-runs and partial migrations.
		parent := filepath.Dir(path)
		if strings.HasPrefix(filepath.Base(parent), "Season ") {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("could not read info.json", slog.String("path", path), slog.String("error", err.Error()))
			return nil
		}
		var info videoInfo
		if err := json.Unmarshal(b, &info); err != nil {
			slog.Warn("could not parse info.json", slog.String("path", path), slog.String("error", err.Error()))
			return nil
		}
		if info.Uploader == "" || len(info.UploadDate) < 4 || info.ID == "" {
			slog.Warn("info.json missing required fields",
				slog.String("path", path),
				slog.String("uploader", info.Uploader),
				slog.String("upload_date", info.UploadDate),
				slog.String("id", info.ID),
			)
			return nil
		}

		uploaderDir := info.Uploader
		if restrict {
			uploaderDir = sanitizeRestricted(uploaderDir)
		} else {
			uploaderDir = sanitizeDefault(uploaderDir)
		}
		if uploaderDir == "" {
			slog.Warn("uploader sanitized to empty; skipping", slog.String("path", path), slog.String("uploader", info.Uploader))
			return nil
		}

		year := info.UploadDate[:4]
		oldBase := strings.TrimSuffix(filepath.Base(path), ".info.json")
		dstDir := filepath.Join(root, uploaderDir, "Season "+year)

		// Collect siblings: every file in the same dir whose name starts
		// with `{oldBase}.` (covers .mkv/.mp4/.webm, .info.json, .jpg/.webp,
		// .description, .en.srt, .en.vtt, .part, etc.).
		entries, err := os.ReadDir(parent)
		if err != nil {
			slog.Warn("could not read parent dir", slog.String("dir", parent), slog.String("error", err.Error()))
			return nil
		}
		var siblings []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.HasPrefix(e.Name(), oldBase+".") {
				continue
			}
			siblings = append(siblings, filepath.Join(parent, e.Name()))
		}
		if len(siblings) == 0 {
			return nil
		}

		raw = append(raw, migration{
			src:      path,
			parent:   parent,
			dstDir:   dstDir,
			oldBase:  oldBase,
			uploader: uploaderDir,
			siblings: siblings,
		})
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return planResult{}, err
	}

	// Dedup by target info.json path. When multiple legacy sources resolve
	// to the same Jellyfin target (e.g. the same video sitting under two
	// different playlist trees), keep the lexicographically-first source
	// and surface the rest as duplicates so the operator can resolve them
	// manually.
	groups := map[string][]int{}
	var order []string
	for i, p := range raw {
		k := p.targetInfoJSON()
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}

	var out planResult
	for _, k := range order {
		indices := groups[k]
		if len(indices) == 1 {
			out.plans = append(out.plans, raw[indices[0]])
			continue
		}
		sort.SliceStable(indices, func(i, j int) bool { return raw[indices[i]].src < raw[indices[j]].src })
		out.plans = append(out.plans, raw[indices[0]])
		group := make([]string, 0, len(indices))
		for _, i := range indices {
			group = append(group, raw[i].src)
		}
		out.duplicates = append(out.duplicates, group)
	}

	return out, nil
}

func tryRemoveIfEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) != 0 {
		return
	}
	if err := os.Remove(dir); err != nil {
		slog.Debug("could not remove empty dir", slog.String("dir", dir), slog.String("error", err.Error()))
	}
}

// sanitizeDefault approximates yt-dlp's sanitize_filename(s, restricted=False).
// Good enough for typical YouTube channel names that contain at most
// punctuation that yt-dlp's default mode rewrites.
func sanitizeDefault(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '?':
			// dropped
		case '"':
			b.WriteRune('\'')
		case ':':
			b.WriteString(" -")
		case '\\', '/', '|', '*', '<', '>':
			b.WriteRune('_')
		case '\n':
			b.WriteRune(' ')
		default:
			if r >= 0x20 && r != 0x7f {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// sanitizeRestricted approximates yt-dlp's sanitize_filename(s, restricted=True):
// ASCII-only, allowed chars = alphanumeric and `_-.`, anything else collapses
// to a single underscore, leading/trailing underscores trimmed.
func sanitizeRestricted(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		var w rune
		switch {
		case r > unicode.MaxASCII:
			w = '_'
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			w = r
		case r == '-' || r == '.':
			w = r
		default:
			w = '_'
		}
		if w == '_' {
			if prevUnderscore {
				continue
			}
			prevUnderscore = true
		} else {
			prevUnderscore = false
		}
		b.WriteRune(w)
	}
	return strings.Trim(b.String(), "_-")
}
