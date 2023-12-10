package config

import (
	"fmt"
	"strings"

	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

const (
	PlaylistList = "playlist.list"
	CronString   = "cron.string"
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

	if !k.Exists(CronString) {
		k.Set("cron.string", "0 */12 * * *")
	}

	if !k.Exists("http.port") {
		k.Set("http.port", "8081")
	}

	if !k.Exists("video.quality") {
		k.Set("video.quality", "height:1080")
	}

	if !k.Exists("archive.file") {
		k.Set("archive.file", "/downloads/archive.txt")
	}

	return nil
}
