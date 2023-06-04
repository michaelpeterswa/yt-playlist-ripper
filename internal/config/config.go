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
