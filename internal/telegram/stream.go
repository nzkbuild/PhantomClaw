package telegram

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// handleStreamingChat sends a placeholder message, then progressively edits it
// as chunks arrive from the LLM. This makes long replies feel instant.
func (tb *Bot) handleStreamingChat(ctx context.Context, b *bot.Bot, update *models.Update, streamer StreamChatResponder) {
	chatID := update.Message.Chat.ID

	// Send initial placeholder
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      "⏳ _Thinking..._",
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		tb.sendReply(ctx, b, chatID, "❌ Failed to send message", false)
		return
	}

	// Accumulate chunks and edit periodically
	var mu sync.Mutex
	var accumulated strings.Builder
	lastEdit := time.Now()
	editInterval := 800 * time.Millisecond // Edit at most every 800ms to avoid Telegram rate limits

	onChunk := func(chunk string) {
		mu.Lock()
		accumulated.WriteString(chunk)
		current := accumulated.String()
		now := time.Now()
		shouldEdit := now.Sub(lastEdit) >= editInterval
		mu.Unlock()

		if shouldEdit && len(strings.TrimSpace(current)) > 0 {
			mu.Lock()
			lastEdit = time.Now()
			mu.Unlock()

			b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    chatID,
				MessageID: sent.ID,
				Text:      current + " ▍",
			})
		}
	}

	reply, err := streamer.HandleChatStream(ctx, update.Message.Text, onChunk)
	if err != nil {
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: sent.ID,
			Text:      "❌ Chat error: " + err.Error(),
		})
		return
	}

	// Final edit with complete reply (remove cursor)
	finalText := strings.TrimSpace(reply)
	if finalText == "" {
		finalText = "I don't have a useful response right now."
	}
	b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: sent.ID,
		Text:      finalText,
	})
}
