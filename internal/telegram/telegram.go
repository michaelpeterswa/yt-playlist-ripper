package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
)

type TelegramClient struct {
	Enabled  bool
	BotToken string
	ChatID   string
	bot      *bot.Bot
}

func NewTelegramClient(enabled bool, botToken string, chatID string, opts ...bot.Option) (*TelegramClient, error) {
	b, err := bot.New(botToken, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &TelegramClient{
		bot:      b,
		Enabled:  enabled,
		BotToken: botToken,
		ChatID:   chatID,
	}, nil
}

func (t *TelegramClient) Start(ctx context.Context) {
	t.bot.Start(ctx)
}

func (t *TelegramClient) SendMessage(ctx context.Context, message string) {
	if !t.Enabled {
		return
	}

	_, err := t.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: t.ChatID,
		Text:   message,
	})

	if err != nil {
		slog.Error("failed to send telegram message", slog.String("error", err.Error()))
		return
	}
}
