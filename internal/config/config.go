package config

import (
	"fmt"

	"github.com/caarlos0/env"
)

type Config struct {
	LogLevel string `env:"LOG_LEVEL" envDefault:"error"`

	RunOnStart     bool   `env:"RUN_ON_START" envDefault:"true"`
	PlaylistList   string `env:"PLAYLIST_LIST" envDefault:""`
	CronString     string `env:"CRON_STRING" envDefault:"0 */12 * * *"`
	VideoQuality   string `env:"VIDEO_QUALITY" envDefault:"height:1080"`
	ArchiveFile    string `env:"ARCHIVE_FILE" envDefault:"/downloads/archive.txt"`
	OutputTemplate string `env:"OUTPUT_TEMPLATE" envDefault:"/downloads/%(playlist)s/%(channel)s/%(title)s"`

	MetricsEnabled bool `env:"METRICS_ENABLED" envDefault:"true"`
	MetricsPort    int  `env:"METRICS_PORT" envDefault:"8081"`

	Local bool `env:"LOCAL" envDefault:"false"`

	TracingEnabled    bool    `env:"TRACING_ENABLED" envDefault:"false"`
	TracingSampleRate float64 `env:"TRACING_SAMPLERATE" envDefault:"0.01"`
	TracingService    string  `env:"TRACING_SERVICE" envDefault:"katalog-agent"`
	TracingVersion    string  `env:"TRACING_VERSION"`
}

func NewConfig() (*Config, error) {
	var cfg Config

	err := env.Parse(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}
