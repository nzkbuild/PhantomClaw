package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
	"github.com/nzkbuild/PhantomClaw/internal/risk"
	"github.com/nzkbuild/PhantomClaw/internal/safety"
	"github.com/nzkbuild/PhantomClaw/internal/scheduler"
	"github.com/nzkbuild/PhantomClaw/internal/skills"
)

// Dependencies holds references to subsystems for command handlers.
type Dependencies struct {
	Safety    *safety.Manager
	Risk      *risk.Engine
	Scheduler *scheduler.Scheduler
	Memory    *memory.DB
	Diary     *memory.DiaryWriter
	Strategy  *memory.StrategyManager
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

	b.RegisterHandler(bot.HandlerTypeMessageText, "/status", bot.MatchTypePrefix, tb.handleStatus)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/halt", bot.MatchTypePrefix, tb.handleHalt)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/mode", bot.MatchTypePrefix, tb.handleMode)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/report", bot.MatchTypePrefix, tb.handleReport)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/pairs", bot.MatchTypePrefix, tb.handlePairs)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/pending", bot.MatchTypePrefix, tb.handlePending)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/confidence", bot.MatchTypePrefix, tb.handleConfidence)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/config", bot.MatchTypePrefix, tb.handleConfig)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/rollback", bot.MatchTypePrefix, tb.handleRollback)
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

	var diaryText string
	if tb.deps.Diary != nil {
		entries, err := tb.deps.Diary.GetToday()
		if err == nil && len(entries) > 0 {
			var sb strings.Builder
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("• [%s] %s\n", e.EntryType, e.Content))
			}
			diaryText = sb.String()
		}
	}
	if diaryText == "" {
		diaryText = "_No diary entries today_"
	}

	msg := fmt.Sprintf("📊 *Daily Report*\n\n"+
		"Daily P&L: -$%.2f\n"+
		"Open Positions: %d\n"+
		"Profitable Trades: %d\n"+
		"Session: %s\n\n"+
		"*Today's Diary:*\n%s",
		stats.DailyLoss,
		stats.OpenPositions,
		stats.ProfitableTrades,
		tb.deps.Scheduler.CurrentSession(),
		diaryText,
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handlePairs(ctx context.Context, b *bot.Bot, update *models.Update) {
	var msg strings.Builder
	msg.WriteString("📈 *Active Pairs*\n\n")

	if tb.deps.Memory != nil {
		rows, err := tb.deps.Memory.QueryRows(
			"SELECT symbol, bias, win_rate_7d, avg_pnl_7d FROM pair_state ORDER BY updated_at DESC LIMIT 10",
		)
		if err == nil {
			defer rows.Close()
			found := false
			for rows.Next() {
				var symbol, bias string
				var winRate, avgPnl float64
				rows.Scan(&symbol, &bias, &winRate, &avgPnl)
				msg.WriteString(fmt.Sprintf("• %s: %s (WR: %.0f%%, PnL: $%.2f)\n",
					symbol, bias, winRate*100, avgPnl))
				found = true
			}
			if !found {
				msg.WriteString("_No pair state data yet — trades needed_\n")
			}
		}
	} else {
		msg.WriteString("XAUUSD, EURUSD, USDJPY, GBPUSD\n")
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg.String(),
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handlePending(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "📋 *Pending Orders*\n\n_Pending orders are managed by the EA. Check MT5 terminal for live status._",
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleConfidence(ctx context.Context, b *bot.Bot, update *models.Update) {
	conf := skills.ScoreConfidence(skills.ConfidenceInput{
		Session: string(tb.deps.Scheduler.CurrentSession()),
	})

	msg := fmt.Sprintf("🎯 *Confidence Score*\n\n"+
		"Score: %d/100\n"+
		"Action: %s\n"+
		"Lot Factor: %.1fx\n\n"+
		"_Score based on current session quality. Full 7-factor scoring requires live market data._",
		conf.Score, conf.Action, conf.LotFactor,
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	stats := tb.deps.Risk.Stats()
	msg := fmt.Sprintf("⚙️ *Risk Config*\n\n"+
		"Max Open Positions: %d\n"+
		"Ramp-up Target: %d trades\n"+
		"Halted: %v",
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

func (tb *Bot) handleRollback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if tb.deps.Strategy == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Strategy manager not configured",
		})
		return
	}

	versions, err := tb.deps.Strategy.ListVersions(5)
	if err != nil || len(versions) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "📜 No strategy versions available yet.",
		})
		return
	}

	var sb strings.Builder
	sb.WriteString("📜 *Strategy Versions*\n\n")
	for _, v := range versions {
		sb.WriteString(fmt.Sprintf("v%d — %s (%s)\n", v.Version, v.PatchType, v.Reason))
	}
	sb.WriteString("\n_Use `/rollback N` to revert to version N_")

	// Check if a version number was provided
	parts := strings.Fields(update.Message.Text)
	if len(parts) >= 2 {
		var targetVersion int
		fmt.Sscanf(parts[1], "%d", &targetVersion)
		if targetVersion > 0 {
			if err := tb.deps.Strategy.Rollback(targetVersion); err != nil {
				b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: update.Message.Chat.ID,
					Text:   fmt.Sprintf("❌ Rollback failed: %v", err),
				})
				return
			}
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    update.Message.Chat.ID,
				Text:      fmt.Sprintf("✅ Rolled back to strategy v%d", targetVersion),
				ParseMode: models.ParseModeMarkdown,
			})
			return
		}
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      sb.String(),
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := "🤖 *PhantomClaw Commands*\n\n" +
		"`/status` — Mode, positions, PnL, session\n" +
		"`/mode observe|suggest|auto|halt` — Switch mode\n" +
		"`/halt` — Emergency stop\n" +
		"`/report` — Daily summary + diary\n" +
		"`/pairs` — Active pairs + bias\n" +
		"`/pending` — Pending orders (check MT5)\n" +
		"`/confidence` — Current confidence score\n" +
		"`/rollback [N]` — Strategy versions / rollback\n" +
		"`/config` — Risk config\n" +
		"`/help` — This message"

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
	})
}

func (tb *Bot) handleUnknown(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Silently ignore non-command messages
}
