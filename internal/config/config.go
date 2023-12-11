package config

import (
	"fmt"
	"strings"

	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

const (
	RunOnStart     = "run.on.start"
	PlaylistList   = "playlist.list"
	CronString     = "cron.string"
	HTTPPort       = "http.port"
	VideoQuality   = "video.quality"
	ArchiveFile    = "archive.file"
	OutputTemplate = "output.template"
)

func Get() (*koanf.Koanf, error) {
	k := koanf.New(".")

	err := k.Load(env.Provider("YTPR_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "YTPR_")), "_", ".", -1)
	}), nil)

	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return k, nil
}

func SetDefaults(k *koanf.Koanf) error {
	// At a minimum, playlist list is required

	if !k.Exists(PlaylistList) {
		return fmt.Errorf("playlist list environment variable is required")
	}

	if !k.Exists(OutputTemplate) {
		_ = k.Set("output.template", "/downloads/%(playlist)s/%(channel)s/%(title)s")
	}

	if !k.Exists(CronString) {
		_ = k.Set("cron.string", "0 */12 * * *")
	}

	if !k.Exists(HTTPPort) {
		_ = k.Set("http.port", "8081")
	}

	if !k.Exists(VideoQuality) {
		_ = k.Set("video.quality", "height:1080")
	}

	if !k.Exists(ArchiveFile) {
		_ = k.Set("archive.file", "/downloads/archive.txt")
	}

	if !k.Exists(RunOnStart) {
		_ = k.Set("run.on.start", "true")
	}

	return nil
}
