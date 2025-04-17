package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
		ChatID:    t.ChatID,
		Text:      message,
		ParseMode: models.ParseModeHTML,
	})

	if err != nil {
		slog.Error("failed to send telegram message", slog.String("error", err.Error()))
		return
	}
}

func Bold(text string) string {
	return fmt.Sprintf("<b>%s</b>", text)
}

func Italic(text string) string {
	return fmt.Sprintf("<i>%s</i>", text)
}

func Underline(text string) string {
	return fmt.Sprintf("<u>%s</u>", text)
}

func Code(text string) string {
	return fmt.Sprintf("<code>%s</code>", text)
}

func Strikethrough(text string) string {
	return fmt.Sprintf("<s>%s</s>", text)
}

func Preformatted(language string, text string) string {
	return fmt.Sprintf(`<pre language="%s">%s</pre>`, language, text)
}

func MessageString(texts ...string) string {
	return strings.Join(texts, "")
}
