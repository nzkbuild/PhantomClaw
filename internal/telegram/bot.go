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

func logInbound(command string, update *models.Update) {
	if update == nil || update.Message == nil {
		log.Printf("telegram: inbound %s (no message payload)", command)
		return
	}
	log.Printf("telegram: inbound %s chat_id=%d user_id=%d text=%q", command, update.Message.Chat.ID, update.Message.From.ID, update.Message.Text)
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
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, tb.handleStart)
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
	tb.sendReply(ctx, tb.b, tb.chatID, text, true)
}

// sendReply sends a Telegram message and logs failures.
// If markdown send fails, it retries once as plain text.
func (tb *Bot) sendReply(ctx context.Context, b *bot.Bot, chatID int64, text string, markdown bool) {
	params := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}
	if markdown {
		params.ParseMode = models.ParseModeMarkdown
	}

	if _, err := b.SendMessage(ctx, params); err != nil {
		log.Printf("telegram: send failed chat_id=%d markdown=%v err=%v text=%q", chatID, markdown, err, text)
		if markdown {
			// Retry once in plain text in case Markdown formatting caused API rejection.
			params.ParseMode = ""
			if _, err2 := b.SendMessage(ctx, params); err2 != nil {
				log.Printf("telegram: send retry failed chat_id=%d err=%v", chatID, err2)
			}
		}
	}
}

// --- Command Handlers ---

func (tb *Bot) handleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/status", update)
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

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/start", update)
	msg := "✅ *PhantomClaw connected*\n\nSend `/status` to check runtime state or `/help` for all commands."
	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleHalt(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/halt", update)
	tb.deps.Safety.SetMode(safety.ModeHalt)
	tb.deps.Risk.SetHalted(true)

	tb.sendReply(ctx, b, update.Message.Chat.ID, "🛑 *HALT ACTIVATED*\nAll trading frozen. Pending orders will be cancelled.\nUse `/mode auto` to resume.", true)
}

func (tb *Bot) handleMode(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/mode", update)
	parts := strings.Fields(update.Message.Text)
	if len(parts) < 2 {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "Usage: `/mode observe|suggest|auto|halt`", true)
		return
	}

	mode, err := safety.ParseMode(parts[1])
	if err != nil {
		tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("❌ %s", err), false)
		return
	}

	tb.deps.Safety.SetMode(mode)
	if mode == safety.ModeHalt {
		tb.deps.Risk.SetHalted(true)
	} else {
		tb.deps.Risk.SetHalted(false)
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ Mode switched to: %s", tb.deps.Safety.StatusText()), true)
}

func (tb *Bot) handleReport(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/report", update)
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

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handlePairs(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/pairs", update)
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

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg.String(), true)
}

func (tb *Bot) handlePending(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/pending", update)
	tb.sendReply(ctx, b, update.Message.Chat.ID, "📋 *Pending Orders*\n\n_Pending orders are managed by the EA. Check MT5 terminal for live status._", true)
}

func (tb *Bot) handleConfidence(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/confidence", update)
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

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/config", update)
	stats := tb.deps.Risk.Stats()
	msg := fmt.Sprintf("⚙️ *Risk Config*\n\n"+
		"Max Open Positions: %d\n"+
		"Ramp-up Target: %d trades\n"+
		"Halted: %v",
		stats.MaxPositions,
		stats.RampUpTarget,
		stats.Halted,
	)

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleRollback(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/rollback", update)
	if tb.deps.Strategy == nil {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "❌ Strategy manager not configured", false)
		return
	}

	versions, err := tb.deps.Strategy.ListVersions(5)
	if err != nil || len(versions) == 0 {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "📜 No strategy versions available yet.", false)
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
				tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("❌ Rollback failed: %v", err), false)
				return
			}
			tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ Rolled back to strategy v%d", targetVersion), true)
			return
		}
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, sb.String(), true)
}

func (tb *Bot) handleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/help", update)
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

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleUnknown(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("unknown", update)
	if update.Message == nil {
		return
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, "🤖 Bot is online. Use /help to see available commands.", false)
}
