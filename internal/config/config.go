package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
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
	OpsAlerts OpsAlertsConfig `mapstructure:"ops_alerts"`
	Dashboard DashboardConfig `mapstructure:"dashboard"`
	Pairs     []string        `mapstructure:"pairs"`
}

// DashboardConfig holds web dashboard settings.
type DashboardConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	AuthUser string `mapstructure:"auth_user"`
	AuthPass string `mapstructure:"auth_pass"`
}

// HeartbeatConfig holds periodic health check settings.
type HeartbeatConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	IntervalMin int  `mapstructure:"interval_min"`
}

// OpsAlertsConfig controls operational-truth Telegram alerting behavior.
type OpsAlertsConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	PollIntervalSec   int  `mapstructure:"poll_interval_sec"`
	ProbeTimeoutMs    int  `mapstructure:"probe_timeout_ms"`
	DegradeForSec     int  `mapstructure:"degrade_for_sec"`
	RepeatEverySec    int  `mapstructure:"repeat_every_sec"`
	UpdateCooldownSec int  `mapstructure:"update_cooldown_sec"`
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
	Primary       string             `mapstructure:"primary"`        // provider name or alias
	StickyPrimary bool               `mapstructure:"sticky_primary"` // lock to primary provider unless changed by user
	Providers     []LLMProviderEntry `mapstructure:"providers"`      // ordered list of providers
	Aliases       map[string]string  `mapstructure:"aliases"`        // "fast" → "groq"
}

// LLMProviderEntry holds config for a single LLM provider.
type LLMProviderEntry struct {
	Name         string   `mapstructure:"name"` // unique identifier (e.g. "groq", "ollama")
	Type         string   `mapstructure:"type"` // "openai_compat" | "claude" | "openai"
	BaseURL      string   `mapstructure:"base_url"`
	APIKey       string   `mapstructure:"api_key"`
	Model        string   `mapstructure:"model"`
	AllowedTools []string `mapstructure:"allowed_tools"` // optional allow-list; empty = all tools allowed
}

// BridgeConfig holds MT5 EA REST bridge settings.
type BridgeConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	AuthToken       string `mapstructure:"auth_token"`
	SignalTimeoutMs int    `mapstructure:"signal_timeout_ms"`
}

// SignalTimeoutDuration returns the configured bridge signal timeout.
func (b BridgeConfig) SignalTimeoutDuration() time.Duration {
	if b.SignalTimeoutMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(b.SignalTimeoutMs) * time.Millisecond
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

var allowedModes = []string{"observe", "suggest", "auto", "halt"}
var allowedLogLevels = []string{"debug", "info", "warn", "error"}
var allowedProviderTypes = []string{"claude", "openai", "openai_compat"}
var allowedFailPolicies = []string{"fail_open", "fail_closed"}

// Load reads configuration from file and environment variables.
// Environment variables use PHANTOM_ prefix (e.g. PHANTOM_TELEGRAM_TOKEN).
func Load(path string) (*Config, error) {
	v, secretsPath := buildViper(path)
	if err := loadSecretsFile(secretsPath); err != nil {
		return nil, fmt.Errorf("secrets read error: %w", err)
	}
	if err := readConfigFile(v); err != nil {
		return nil, err
	}
	cfg, err := decode(v)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// Watch registers a config watcher that re-reads and validates settings before applying.
// If the configured file does not exist, this is a no-op and returns nil.
func Watch(path string, onChange func(*Config) error, onError func(error)) error {
	v, secretsPath := buildViper(path)
	if err := loadSecretsFile(secretsPath); err != nil {
		return fmt.Errorf("secrets read error: %w", err)
	}
	if err := readConfigFile(v); err != nil {
		return err
	}
	if _, err := decode(v); err != nil {
		return err
	}
	if strings.TrimSpace(v.ConfigFileUsed()) == "" {
		return nil
	}

	v.OnConfigChange(func(_ fsnotify.Event) {
		if err := loadSecretsFile(secretsPath); err != nil {
			if onError != nil {
				onError(fmt.Errorf("secrets reload failed: %w", err))
			}
			return
		}
		next, err := decode(v)
		if err != nil {
			if onError != nil {
				onError(fmt.Errorf("config reload rejected: %w", err))
			}
			return
		}
		if onChange != nil {
			if err := onChange(next); err != nil && onError != nil {
				onError(fmt.Errorf("config apply failed: %w", err))
			}
		}
	})
	v.WatchConfig()
	return nil
}

// SecretWarnings reports secret-like values still embedded directly in config fields.
func SecretWarnings(cfg *Config) []string {
	var out []string
	if strings.TrimSpace(cfg.Telegram.Token) != "" {
		out = append(out, "telegram.token is set in config; move to PHANTOM_TELEGRAM_TOKEN or .secrets")
	}
	if strings.TrimSpace(cfg.Bridge.AuthToken) != "" {
		out = append(out, "bridge.auth_token is set in config; move to PHANTOM_BRIDGE_AUTH_TOKEN or .secrets")
	}
	for _, p := range cfg.LLM.Providers {
		if strings.TrimSpace(p.APIKey) != "" {
			out = append(out, fmt.Sprintf("llm.providers[%s].api_key is set in config; move to PHANTOM_LLM_PROVIDERS_*_API_KEY or .secrets", p.Name))
		}
	}
	return out
}

// Validate performs strict config checks before startup or runtime apply.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Bot.Mode))
	if !slices.Contains(allowedModes, mode) {
		return fmt.Errorf("bot.mode must be one of %v, got %q", allowedModes, cfg.Bot.Mode)
	}
	logLevel := strings.ToLower(strings.TrimSpace(cfg.Bot.LogLevel))
	if !slices.Contains(allowedLogLevels, logLevel) {
		return fmt.Errorf("bot.log_level must be one of %v, got %q", allowedLogLevels, cfg.Bot.LogLevel)
	}
	if _, err := time.LoadLocation(strings.TrimSpace(cfg.Bot.Timezone)); err != nil {
		return fmt.Errorf("bot.timezone is invalid: %w", err)
	}

	if cfg.Bridge.Port <= 0 || cfg.Bridge.Port > 65535 {
		return fmt.Errorf("bridge.port must be between 1 and 65535")
	}
	if cfg.Bridge.SignalTimeoutMs < 100 || cfg.Bridge.SignalTimeoutMs > 120000 {
		return fmt.Errorf("bridge.signal_timeout_ms must be between 100 and 120000")
	}

	failPolicy := strings.ToLower(strings.TrimSpace(cfg.Market.FailPolicy))
	if !slices.Contains(allowedFailPolicies, failPolicy) {
		return fmt.Errorf("market.fail_policy must be one of %v, got %q", allowedFailPolicies, cfg.Market.FailPolicy)
	}

	if cfg.Risk.MaxLotSize <= 0 || cfg.Risk.MaxLotSize > 100 {
		return fmt.Errorf("risk.max_lot_size must be > 0 and <= 100")
	}
	if cfg.Risk.MaxDailyLossUSD <= 0 {
		return fmt.Errorf("risk.max_daily_loss_usd must be > 0")
	}
	if cfg.Risk.MaxOpenPositions <= 0 || cfg.Risk.MaxOpenPositions > 100 {
		return fmt.Errorf("risk.max_open_positions must be between 1 and 100")
	}
	if cfg.Risk.MaxDrawdownPct <= 0 || cfg.Risk.MaxDrawdownPct > 100 {
		return fmt.Errorf("risk.max_drawdown_pct must be > 0 and <= 100")
	}
	if cfg.Risk.SuggestTimeoutSec < 15 || cfg.Risk.SuggestTimeoutSec > 3600 {
		return fmt.Errorf("risk.suggest_timeout_sec must be between 15 and 3600")
	}
	if _, err := time.ParseDuration(strings.TrimSpace(cfg.Risk.MinTradeInterval)); err != nil {
		return fmt.Errorf("risk.min_trade_interval invalid duration: %w", err)
	}
	if cfg.Risk.RampUpTrades < 0 || cfg.Risk.RampUpTrades > 10000 {
		return fmt.Errorf("risk.ramp_up_trades must be between 0 and 10000")
	}
	if cfg.Risk.RampUpLotPct < 0 || cfg.Risk.RampUpLotPct > 1 {
		return fmt.Errorf("risk.ramp_up_lot_pct must be between 0 and 1")
	}

	if err := validateClock("sessions.tokyo_open", cfg.Sessions.TokyoOpen); err != nil {
		return err
	}
	if err := validateClock("sessions.pre_london", cfg.Sessions.PreLondon); err != nil {
		return err
	}
	if err := validateClock("sessions.london_open", cfg.Sessions.LondonOpen); err != nil {
		return err
	}
	if err := validateClock("sessions.ny_overlap_end", cfg.Sessions.NYOverlapEnd); err != nil {
		return err
	}
	if err := validateClock("sessions.learning_start", cfg.Sessions.LearningStart); err != nil {
		return err
	}
	if err := validateClock("sessions.learning_end", cfg.Sessions.LearningEnd); err != nil {
		return err
	}
	if err := validateOrderedClock(
		[]string{"sessions.tokyo_open", "sessions.pre_london", "sessions.london_open", "sessions.ny_overlap_end"},
		[]string{cfg.Sessions.TokyoOpen, cfg.Sessions.PreLondon, cfg.Sessions.LondonOpen, cfg.Sessions.NYOverlapEnd},
	); err != nil {
		return err
	}
	if err := validateOrderedClock(
		[]string{"sessions.learning_start", "sessions.learning_end"},
		[]string{cfg.Sessions.LearningStart, cfg.Sessions.LearningEnd},
	); err != nil {
		return err
	}

	if strings.TrimSpace(cfg.Memory.DBPath) == "" {
		return fmt.Errorf("memory.db_path is required")
	}
	if strings.TrimSpace(cfg.Memory.LogDir) == "" {
		return fmt.Errorf("memory.log_dir is required")
	}
	if strings.TrimSpace(cfg.Memory.SessionsDir) == "" {
		return fmt.Errorf("memory.sessions_dir is required")
	}

	if cfg.Heartbeat.Enabled {
		if cfg.Heartbeat.IntervalMin <= 0 || cfg.Heartbeat.IntervalMin > 1440 {
			return fmt.Errorf("heartbeat.interval_min must be between 1 and 1440 when heartbeat.enabled=true")
		}
	}
	if cfg.OpsAlerts.Enabled {
		if cfg.OpsAlerts.PollIntervalSec < 2 || cfg.OpsAlerts.PollIntervalSec > 3600 {
			return fmt.Errorf("ops_alerts.poll_interval_sec must be between 2 and 3600 when ops_alerts.enabled=true")
		}
		if cfg.OpsAlerts.ProbeTimeoutMs < 100 || cfg.OpsAlerts.ProbeTimeoutMs > 10000 {
			return fmt.Errorf("ops_alerts.probe_timeout_ms must be between 100 and 10000 when ops_alerts.enabled=true")
		}
		if cfg.OpsAlerts.DegradeForSec < 5 || cfg.OpsAlerts.DegradeForSec > 3600 {
			return fmt.Errorf("ops_alerts.degrade_for_sec must be between 5 and 3600 when ops_alerts.enabled=true")
		}
		if cfg.OpsAlerts.RepeatEverySec < 30 || cfg.OpsAlerts.RepeatEverySec > 86400 {
			return fmt.Errorf("ops_alerts.repeat_every_sec must be between 30 and 86400 when ops_alerts.enabled=true")
		}
		if cfg.OpsAlerts.UpdateCooldownSec < 1 || cfg.OpsAlerts.UpdateCooldownSec > 3600 {
			return fmt.Errorf("ops_alerts.update_cooldown_sec must be between 1 and 3600 when ops_alerts.enabled=true")
		}
	}

	if len(cfg.Pairs) == 0 {
		return fmt.Errorf("pairs must contain at least one symbol")
	}

	if strings.TrimSpace(cfg.LLM.Primary) == "" {
		return fmt.Errorf("llm.primary is required")
	}
	seenProvider := make(map[string]struct{})
	for i, entry := range cfg.LLM.Providers {
		name := strings.TrimSpace(strings.ToLower(entry.Name))
		if name == "" {
			return fmt.Errorf("llm.providers[%d].name is required", i)
		}
		if _, exists := seenProvider[name]; exists {
			return fmt.Errorf("llm.providers[%d].name duplicates %q", i, name)
		}
		seenProvider[name] = struct{}{}

		providerType := strings.TrimSpace(strings.ToLower(entry.Type))
		if !slices.Contains(allowedProviderTypes, providerType) {
			return fmt.Errorf("llm.providers[%d].type must be one of %v", i, allowedProviderTypes)
		}
		if strings.TrimSpace(entry.Model) == "" {
			return fmt.Errorf("llm.providers[%d].model is required", i)
		}
		if providerType == "openai_compat" {
			baseURL := strings.TrimSpace(entry.BaseURL)
			u, err := url.Parse(baseURL)
			if err != nil || !u.IsAbs() {
				return fmt.Errorf("llm.providers[%d].base_url must be a valid absolute URL", i)
			}
		}
	}

	resolvedPrimary := strings.ToLower(strings.TrimSpace(cfg.LLM.Primary))
	if aliasTarget, ok := cfg.LLM.Aliases[resolvedPrimary]; ok {
		resolvedPrimary = strings.ToLower(strings.TrimSpace(aliasTarget))
	}
	if _, ok := seenProvider[resolvedPrimary]; !ok {
		return fmt.Errorf("llm.primary %q does not resolve to a configured provider", cfg.LLM.Primary)
	}

	for alias, target := range cfg.LLM.Aliases {
		targetName := strings.ToLower(strings.TrimSpace(target))
		if targetName == "" {
			return fmt.Errorf("llm.aliases[%q] target is empty", alias)
		}
		if _, ok := seenProvider[targetName]; !ok {
			return fmt.Errorf("llm.aliases[%q] targets unknown provider %q", alias, target)
		}
	}

	return nil
}

func buildViper(path string) (*viper.Viper, string) {
	v := viper.New()

	// Defaults (PRD §10 balanced defaults)
	v.SetDefault("bot.name", "PhantomClaw")
	v.SetDefault("bot.mode", "AUTO")
	v.SetDefault("bot.timezone", "Asia/Kuala_Lumpur")
	v.SetDefault("bot.log_level", "info")
	v.SetDefault("telegram.token", "")
	v.SetDefault("telegram.chat_id", 0)

	v.SetDefault("bridge.host", "127.0.0.1")
	v.SetDefault("bridge.port", 8765)
	v.SetDefault("bridge.auth_token", "")
	v.SetDefault("bridge.signal_timeout_ms", 30000)
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

	v.SetDefault("ops_alerts.enabled", true)
	v.SetDefault("ops_alerts.poll_interval_sec", 10)
	v.SetDefault("ops_alerts.probe_timeout_ms", 1500)
	v.SetDefault("ops_alerts.degrade_for_sec", 20)
	v.SetDefault("ops_alerts.repeat_every_sec", 900)
	v.SetDefault("ops_alerts.update_cooldown_sec", 120)

	v.SetDefault("llm.primary", "claude")
	v.SetDefault("llm.sticky_primary", true)

	v.SetDefault("dashboard.host", "127.0.0.1")
	v.SetDefault("dashboard.port", 8080)
	v.SetDefault("dashboard.auth_user", "")
	v.SetDefault("dashboard.auth_pass", "")

	v.SetDefault("pairs", []string{"XAUUSD", "EURUSD", "USDJPY", "GBPUSD"})

	// Environment variable binding
	v.SetEnvPrefix("PHANTOM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// File
	configPath := strings.TrimSpace(path)
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("C:\\PhantomClaw\\")
		configPath = "config.yaml"
	}

	return v, resolveSecretsPath(configPath)
}

func readConfigFile(v *viper.Viper) error {
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("config read error: %w", err)
		}
	}
	return nil
}

func decode(v *viper.Viper) (*Config, error) {
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config unmarshal error: %w", err)
	}
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation error: %w", err)
	}
	return &cfg, nil
}

func resolveSecretsPath(configPath string) string {
	if override := strings.TrimSpace(os.Getenv("PHANTOM_SECRETS_FILE")); override != "" {
		return override
	}
	dir := "."
	if strings.TrimSpace(configPath) != "" {
		dir = filepath.Dir(configPath)
		if strings.TrimSpace(dir) == "" {
			dir = "."
		}
	}
	return filepath.Join(dir, ".secrets")
}

func loadSecretsFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		value = strings.Trim(value, `"'`)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func validateClock(label, value string) error {
	if _, err := time.Parse("15:04", strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("%s must be HH:MM 24-hour format, got %q", label, value)
	}
	return nil
}

func validateOrderedClock(labels, values []string) error {
	if len(labels) != len(values) || len(values) == 0 {
		return fmt.Errorf("invalid ordered clock validator inputs")
	}
	mins := make([]int, len(values))
	for i, v := range values {
		parsed, err := time.Parse("15:04", strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("%s must be HH:MM 24-hour format", labels[i])
		}
		mins[i] = parsed.Hour()*60 + parsed.Minute()
	}

	offset := 0
	prev := mins[0]
	for i := 1; i < len(mins); i++ {
		current := mins[i] + offset
		if current <= prev {
			offset += 24 * 60
			current = mins[i] + offset
		}
		if current <= prev {
			return fmt.Errorf("%s must be later than %s (supports day rollover)", labels[i], labels[i-1])
		}
		prev = current
	}
	return nil
}
