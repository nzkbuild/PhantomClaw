package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all PhantomClaw configuration.
type Config struct {
	Bot       BotConfig       `mapstructure:"bot"`
	Telegram  TelegramConfig  `mapstructure:"telegram"`
	LLM       LLMConfig       `mapstructure:"llm"`
	Bridge    BridgeConfig    `mapstructure:"bridge"`
	Market    MarketConfig    `mapstructure:"market"`
	Risk      RiskConfig      `mapstructure:"risk"`
	Sessions  SessionConfig   `mapstructure:"sessions"`
	Memory    MemoryConfig    `mapstructure:"memory"`
	Heartbeat HeartbeatConfig `mapstructure:"heartbeat"`
	Pairs     []string        `mapstructure:"pairs"`
}

// HeartbeatConfig holds periodic health check settings.
type HeartbeatConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	IntervalMin int  `mapstructure:"interval_min"`
}

// BotConfig holds general bot settings.
type BotConfig struct {
	Name     string `mapstructure:"name"`
	Mode     string `mapstructure:"mode"`     // OBSERVE | SUGGEST | AUTO | HALT
	Timezone string `mapstructure:"timezone"` // e.g. "Asia/Kuala_Lumpur"
	LogLevel string `mapstructure:"log_level"`
}

// TelegramConfig holds Telegram bot credentials.
type TelegramConfig struct {
	Token  string `mapstructure:"token"`
	ChatID int64  `mapstructure:"chat_id"`
}

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	Primary   string             `mapstructure:"primary"`   // provider name or alias
	Providers []LLMProviderEntry `mapstructure:"providers"` // ordered list of providers
	Aliases   map[string]string  `mapstructure:"aliases"`   // "fast" → "groq"
}

// LLMProviderEntry holds config for a single LLM provider.
type LLMProviderEntry struct {
	Name    string `mapstructure:"name"`     // unique identifier (e.g. "groq", "ollama")
	Type    string `mapstructure:"type"`     // "openai_compat" | "claude" | "openai"
	BaseURL string `mapstructure:"base_url"` // API base URL (required for openai_compat)
	APIKey  string `mapstructure:"api_key"`
	Model   string `mapstructure:"model"`
}

// BridgeConfig holds MT5 EA REST bridge settings.
type BridgeConfig struct {
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	AuthToken string `mapstructure:"auth_token"`
}

// MarketConfig holds market-data behavior settings.
type MarketConfig struct {
	// FailPolicy controls behavior when market data cannot be fetched/parsed.
	// Values: "fail_open" | "fail_closed"
	FailPolicy string `mapstructure:"fail_policy"`
}

// RiskConfig holds hard-coded risk guardrails (PRD §10).
type RiskConfig struct {
	MaxLotSize        float64 `mapstructure:"max_lot_size"`
	MaxDailyLossUSD   float64 `mapstructure:"max_daily_loss_usd"`
	MaxOpenPositions  int     `mapstructure:"max_open_positions"`
	MaxDrawdownPct    float64 `mapstructure:"max_drawdown_pct"`
	SuggestTimeoutSec int     `mapstructure:"suggest_timeout_sec"`
	MinTradeInterval  string  `mapstructure:"min_trade_interval"` // duration string e.g. "15m"
	RampUpTrades      int     `mapstructure:"ramp_up_trades"`
	RampUpLotPct      float64 `mapstructure:"ramp_up_lot_pct"`
}

// MinTradeIntervalDuration parses the interval as a time.Duration.
func (r RiskConfig) MinTradeIntervalDuration() time.Duration {
	d, err := time.ParseDuration(r.MinTradeInterval)
	if err != nil {
		return 15 * time.Minute // safe default
	}
	return d
}

// SessionConfig holds MYT session window definitions (PRD §6).
type SessionConfig struct {
	TokyoOpen     string `mapstructure:"tokyo_open"`     // "08:00"
	PreLondon     string `mapstructure:"pre_london"`     // "14:45"
	LondonOpen    string `mapstructure:"london_open"`    // "15:00"
	NYOverlapEnd  string `mapstructure:"ny_overlap_end"` // "00:00"
	LearningStart string `mapstructure:"learning_start"` // "00:00"
	LearningEnd   string `mapstructure:"learning_end"`   // "08:00"
}

// MemoryConfig holds SQLite database settings.
type MemoryConfig struct {
	DBPath      string `mapstructure:"db_path"`
	LogDir      string `mapstructure:"log_dir"`
	SessionsDir string `mapstructure:"sessions_dir"`
}

// Load reads configuration from file and environment variables.
// Environment variables use PHANTOM_ prefix (e.g. PHANTOM_TELEGRAM_TOKEN).
func Load(path string) (*Config, error) {
	v := viper.New()

	// Defaults (PRD §10 balanced defaults)
	v.SetDefault("bot.name", "PhantomClaw")
	v.SetDefault("bot.mode", "AUTO")
	v.SetDefault("bot.timezone", "Asia/Kuala_Lumpur")
	v.SetDefault("bot.log_level", "info")

	v.SetDefault("bridge.host", "127.0.0.1")
	v.SetDefault("bridge.port", 8765)
	v.SetDefault("bridge.auth_token", "")
	v.SetDefault("market.fail_policy", "fail_open")

	v.SetDefault("risk.max_lot_size", 0.10)
	v.SetDefault("risk.max_daily_loss_usd", 100.0)
	v.SetDefault("risk.max_open_positions", 3)
	v.SetDefault("risk.max_drawdown_pct", 10.0)
	v.SetDefault("risk.suggest_timeout_sec", 180)
	v.SetDefault("risk.min_trade_interval", "15m")
	v.SetDefault("risk.ramp_up_trades", 10)
	v.SetDefault("risk.ramp_up_lot_pct", 0.50)

	v.SetDefault("sessions.tokyo_open", "08:00")
	v.SetDefault("sessions.pre_london", "14:45")
	v.SetDefault("sessions.london_open", "15:00")
	v.SetDefault("sessions.ny_overlap_end", "00:00")
	v.SetDefault("sessions.learning_start", "00:00")
	v.SetDefault("sessions.learning_end", "08:00")

	v.SetDefault("memory.db_path", "data/phantom.db")
	v.SetDefault("memory.log_dir", "data/logs")
	v.SetDefault("memory.sessions_dir", "data/sessions")

	v.SetDefault("heartbeat.enabled", false)
	v.SetDefault("heartbeat.interval_min", 5)

	v.SetDefault("llm.primary", "claude")

	v.SetDefault("pairs", []string{"XAUUSD", "EURUSD", "USDJPY", "GBPUSD"})

	// Environment variable binding
	v.SetEnvPrefix("PHANTOM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// File
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("C:\\PhantomClaw\\")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("config read error: %w", err)
		}
		// Config file not found — use defaults + env vars only
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config unmarshal error: %w", err)
	}

	return &cfg, nil
}
