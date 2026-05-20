// Package notifier sends ripper failure events through the
// pulsar-notifcation-pipeline writer, which dispatches to Pushover.
package notifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/michaelpeterswa/pulsar-notifcation-pipeline/pkg/notificationclient"
)

type Notifier struct {
	enabled         bool
	client          *notificationclient.Client
	pushoverUserKey string
}

// New constructs a Notifier. An empty pulsarURL disables it (no-op Send).
// When pulsarURL is set, pushoverUserKey is required.
func New(pulsarURL, bearerToken, pushoverUserKey string) (*Notifier, error) {
	if pulsarURL == "" {
		slog.Info("notifier disabled (PULSAR_URL is empty)")
		return &Notifier{enabled: false}, nil
	}
	if pushoverUserKey == "" {
		return nil, errors.New("notifier: PULSAR_PUSHOVER_USER_KEY is required when PULSAR_URL is set")
	}

	opts := []notificationclient.Option{
		notificationclient.WithUserAgent("yt-playlist-ripper"),
	}
	if bearerToken != "" {
		opts = append(opts, notificationclient.WithBearerToken(bearerToken))
	}

	c, err := notificationclient.New(pulsarURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("notifier: %w", err)
	}

	return &Notifier{
		enabled:         true,
		client:          c,
		pushoverUserKey: pushoverUserKey,
	}, nil
}

func (n *Notifier) Send(ctx context.Context, title, body string) {
	if !n.enabled {
		return
	}

	_, err := n.client.Submit(ctx, notificationclient.NewPushoverRequest(n.pushoverUserKey, title, body))
	if err != nil {
		slog.Error("notifier: submit failed",
			slog.String("title", title),
			slog.String("error", err.Error()),
		)
	}
}
