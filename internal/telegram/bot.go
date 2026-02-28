package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/nzkbuild/PhantomClaw/internal/risk"
	"github.com/nzkbuild/PhantomClaw/internal/safety"
	"github.com/nzkbuild/PhantomClaw/internal/scheduler"
)

// Dependencies holds references to subsystems for command handlers.
type Dependencies struct {
	Safety    *safety.Manager
	Risk      *risk.Engine
	Scheduler *scheduler.Scheduler
}

// Bot wraps the Telegram bot and handles command dispatch.
type Bot struct {
	b      *bot.Bot
	chatID int64
	deps   Dependencies
}

// New creates a Telegram bot listener.
func New(token string, chatID int64, deps Dependencies) (*Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram: token is required")
	}

	tb := &Bot{chatID: chatID, deps: deps}

	opts := []bot.Option{
		bot.WithDefaultHandler(tb.handleUnknown),
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: init error: %w", err)
	}

	// Register command handlers
	b.RegisterHandler(bot.HandlerTypeMessageText, "/status", bot.MatchTypePrefix, tb.handleStatus)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/halt", bot.MatchTypePrefix, tb.handleHalt)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/mode", bot.MatchTypePrefix, tb.handleMode)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/report", bot.MatchTypePrefix, tb.handleReport)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/pairs", bot.MatchTypePrefix, tb.handlePairs)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/pending", bot.MatchTypePrefix, tb.handlePending)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/confidence", bot.MatchTypePrefix, tb.handleConfidence)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/config", bot.MatchTypePrefix, tb.handleConfig)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypePrefix, tb.handleHelp)

	tb.b = b
	return tb, nil
}

// Start begins polling for Telegram updates (blocking).
func (tb *Bot) Start(ctx context.Context) {
	log.Printf("telegram: bot started, chat_id=%d", tb.chatID)
	tb.b.Start(ctx)
}

// Send sends a message to the configured chat.
func (tb *Bot) Send(ctx context.Context, text string) {
	if tb.chatID == 0 {
		return
	}
	tb.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    tb.chatID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	})
}

// --- Command Handlers ---

func (tb *Bot) handleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	stats := tb.deps.Risk.Stats()
	session := tb.deps.Scheduler.CurrentSession()

	msg := fmt.Sprintf("🤖 *PhantomClaw Status*\n\n"+
		"Mode: %s\n"+
		"Session: %s\n"+
		"Open Positions: %d/%d\n"+
		"Daily Loss: $%.2f\n"+
		"Ramp-up: %d/%d profitable trades\n"+
		"Weekend: %v",
		tb.deps.Safety.StatusText(),
		session,
		stats.OpenPositions, stats.MaxPositions,
		stats.DailyLoss,
		stats.ProfitableTrades, stats.RampUpTarget,
		tb.deps.Scheduler.IsWeekend(),
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleHalt(ctx context.Context, b *bot.Bot, update *models.Update) {
	tb.deps.Safety.SetMode(safety.ModeHalt)
	tb.deps.Risk.SetHalted(true)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "🛑 *HALT ACTIVATED*\nAll trading frozen. Pending orders will be cancelled.\nUse `/mode auto` to resume.",
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleMode(ctx context.Context, b *bot.Bot, update *models.Update) {
	parts := strings.Fields(update.Message.Text)
	if len(parts) < 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      "Usage: `/mode observe|suggest|auto|halt`",
			ParseMode: models.ParseModeMarkdown,
		})
		return
	}

	mode, err := safety.ParseMode(parts[1])
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ %s", err),
		})
		return
	}

	tb.deps.Safety.SetMode(mode)
	if mode == safety.ModeHalt {
		tb.deps.Risk.SetHalted(true)
	} else {
		tb.deps.Risk.SetHalted(false)
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ Mode switched to: %s", tb.deps.Safety.StatusText()),
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleReport(ctx context.Context, b *bot.Bot, update *models.Update) {
	stats := tb.deps.Risk.Stats()
	msg := fmt.Sprintf("📊 *Daily Report*\n\n"+
		"Daily P&L: -$%.2f (loss tracked)\n"+
		"Open Positions: %d\n"+
		"Profitable Trades: %d\n"+
		"Session: %s\n\n"+
		"_Full report coming in Phase 2 (trade journal + lessons)_",
		stats.DailyLoss,
		stats.OpenPositions,
		stats.ProfitableTrades,
		tb.deps.Scheduler.CurrentSession(),
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handlePairs(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "📈 *Active Pairs*\n\nXAUUSD, EURUSD, USDJPY, GBPUSD\n\n_Pair state + LRU ranking coming in Phase 2_",
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handlePending(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "📋 *Pending Orders*\n\nNo active pending orders (stub)\n\n_Live pending order tracking coming in Phase 2_",
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleConfidence(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "🎯 *Confidence Scores*\n\nNo active analysis (stub)\n\n_Live confidence breakdown per pair coming in Phase 2_",
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	stats := tb.deps.Risk.Stats()
	msg := fmt.Sprintf("⚙️ *Risk Config (read-only)*\n\n"+
		"Max Open Positions: %d\n"+
		"Ramp-up Target: %d trades\n"+
		"Halted: %v\n\n"+
		"_Full config display coming in Phase 2_",
		stats.MaxPositions,
		stats.RampUpTarget,
		stats.Halted,
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := "🤖 *PhantomClaw Commands*\n\n" +
		"`/status` — Mode, positions, PnL, session\n" +
		"`/mode observe|suggest|auto|halt` — Switch mode\n" +
		"`/halt` — Emergency stop\n" +
		"`/report` — Daily summary\n" +
		"`/pairs` — Active pairs + bias\n" +
		"`/pending` — Pending orders\n" +
		"`/confidence` — Confidence scores\n" +
		"`/config` — Risk config\n" +
		"`/help` — This message"

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleUnknown(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Silently ignore non-command messages for now
	// Phase 3 will wire /ask [question] to LLM conversation
}
