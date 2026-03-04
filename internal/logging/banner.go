package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

// StatusOK, StatusWarn, StatusFail indicate startup step outcomes.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// Banner writes pretty startup output to the console.
type Banner struct {
	w        io.Writer
	maxLabel int
}

// NewBanner creates a banner writer. Pass nil to use os.Stdout.
func NewBanner(w io.Writer) *Banner {
	if w == nil {
		w = os.Stdout
	}
	return &Banner{w: w, maxLabel: 12}
}

// Header prints the startup banner.
func (b *Banner) Header(version string) {
	fmt.Fprintln(b.w)
	fmt.Fprintf(b.w, "  %s🐾 PhantomClaw v%s%s\n", colorBold+colorCyan, version, colorReset)
	b.Divider()
}

// Step prints a formatted status line.
//
//	name:   left-aligned label (e.g. "Config", "Bridge")
//	status: StatusOK / StatusWarn / StatusFail
//	detail: description text
func (b *Banner) Step(name, status, detail string) {
	icon, color := b.resolveStatus(status)
	padding := b.maxLabel - len(name)
	if padding < 1 {
		padding = 1
	}
	fmt.Fprintf(b.w, "  %s%s%s %s%s  %s%s\n",
		colorDim, name, colorReset,
		strings.Repeat(" ", padding),
		color+icon+colorReset,
		detail,
		colorReset,
	)
}

// Divider prints a visual separator line.
func (b *Banner) Divider() {
	fmt.Fprintf(b.w, "  %s%s%s\n", colorDim, strings.Repeat("─", 38), colorReset)
}

// Ready prints the final "ready" message.
func (b *Banner) Ready(message string) {
	b.Divider()
	fmt.Fprintf(b.w, "  %s%s%s\n\n", colorGreen, message, colorReset)
}

func (b *Banner) resolveStatus(status string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusOK:
		return "✓", colorGreen
	case StatusWarn:
		return "⚠", colorYellow
	case StatusFail:
		return "✗", colorRed
	default:
		return "·", colorDim
	}
}
