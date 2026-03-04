package logging

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger and exposes console level control.
type Logger struct {
	*zap.Logger
	consoleLevel zap.AtomicLevel
}

// SetConsoleLevel changes the minimum log level shown on the console.
// Use this to suppress INFO during startup, then restore it afterwards.
func (l *Logger) SetConsoleLevel(level zapcore.Level) {
	l.consoleLevel.SetLevel(level)
}

// New creates a structured JSON logger with daily rotation.
// Writes to both stdout and a log file.
// The returned Logger allows runtime control of the console log level.
func New(logDir, level string) (*Logger, error) {
	// Parse log level
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	// Encoder config — structured JSON
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// File output — JSON format
	logFile := filepath.Join(logDir, "phantomclaw.log")
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	// Console output — human-readable, with dynamic level control
	consoleCfg := encoderCfg
	consoleCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleLevel := zap.NewAtomicLevelAt(zapLevel)

	core := zapcore.NewTee(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderCfg),
			zapcore.AddSync(file),
			zapLevel,
		),
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleCfg),
			zapcore.AddSync(os.Stdout),
			consoleLevel,
		),
	)

	return &Logger{
		Logger:       zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)),
		consoleLevel: consoleLevel,
	}, nil
}

// TradeFields creates structured fields for trade logging.
func TradeFields(symbol, direction, action string, lot float64, confidence int) []zap.Field {
	return []zap.Field{
		zap.String("symbol", symbol),
		zap.String("direction", direction),
		zap.String("action", action),
		zap.Float64("lot", lot),
		zap.Int("confidence", confidence),
	}
}

// RiskFields creates structured fields for risk engine logging.
func RiskFields(dailyLoss float64, openPos, maxPos int, halted bool) []zap.Field {
	return []zap.Field{
		zap.Float64("daily_loss", dailyLoss),
		zap.Int("open_positions", openPos),
		zap.Int("max_positions", maxPos),
		zap.Bool("halted", halted),
	}
}
