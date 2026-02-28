package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/llm"
)

// StrategyManager handles master strategy doc generation, patches, and rollback (PRD §14).
type StrategyManager struct {
	db      *DB
	llmProv llm.Provider
}

// NewStrategyManager creates a strategy versioning manager.
func NewStrategyManager(db *DB, provider llm.Provider) *StrategyManager {
	return &StrategyManager{db: db, llmProv: provider}
}

// StrategyPatch represents a numbered strategy update.
type StrategyPatch struct {
	ID        int64
	Version   int
	PatchType string // "add_rule" | "modify_rule" | "remove_rule" | "weight_adjust"
	Content   string
	Reason    string
	CreatedAt time.Time
}

// GetCurrentStrategy loads the master strategy document (latest version).
func (sm *StrategyManager) GetCurrentStrategy() (string, int, error) {
	var content string
	var version int
	err := sm.db.QueryRow(`
		SELECT content, version FROM strategy_patches 
		WHERE patch_type = 'master' 
		ORDER BY version DESC LIMIT 1`,
	).Scan(&content, &version)
	if err != nil {
		return "", 0, nil // No strategy yet — not an error
	}
	return content, version, nil
}

// ApplyPatch writes a new strategy patch and increments the version.
func (sm *StrategyManager) ApplyPatch(patchType, content, reason string, prevVersion int) (int, error) {
	newVersion := prevVersion + 1
	_, err := sm.db.conn.Exec(`
		INSERT INTO strategy_patches (version, patch_type, content, reason) 
		VALUES (?, ?, ?, ?)`,
		newVersion, patchType, content, reason,
	)
	if err != nil {
		return 0, fmt.Errorf("strategy: apply patch: %w", err)
	}
	return newVersion, nil
}

// Rollback reverts to a previous strategy version.
func (sm *StrategyManager) Rollback(targetVersion int) error {
	// Get the target version content
	var content string
	err := sm.db.QueryRow(`
		SELECT content FROM strategy_patches 
		WHERE version = ? AND patch_type = 'master'`,
		targetVersion,
	).Scan(&content)
	if err != nil {
		return fmt.Errorf("strategy: version %d not found", targetVersion)
	}

	// Create rollback patch
	_, currentVersion, _ := sm.GetCurrentStrategy()
	_, err = sm.ApplyPatch("rollback", content,
		fmt.Sprintf("rollback from v%d to v%d", currentVersion, targetVersion),
		currentVersion,
	)
	return err
}

// ListVersions returns all strategy patch versions.
func (sm *StrategyManager) ListVersions(limit int) ([]StrategyPatch, error) {
	rows, err := sm.db.conn.Query(`
		SELECT id, version, patch_type, reason, created_at 
		FROM strategy_patches 
		ORDER BY version DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patches []StrategyPatch
	for rows.Next() {
		var p StrategyPatch
		if err := rows.Scan(&p.ID, &p.Version, &p.PatchType, &p.Reason, &p.CreatedAt); err != nil {
			return nil, err
		}
		patches = append(patches, p)
	}
	return patches, rows.Err()
}

// RebuildMasterStrategy generates a new master strategy doc using LLM analysis of lessons.
// Called nightly during LEARNING mode (00:00-08:00 MYT).
func (sm *StrategyManager) RebuildMasterStrategy(ctx context.Context) error {
	// Gather all high-weight lessons
	rows, err := sm.db.conn.Query(`
		SELECT symbol, lesson, weight FROM lessons 
		WHERE weight >= 1.0 
		ORDER BY weight DESC, created_at DESC LIMIT 50`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lessonsText strings.Builder
	for rows.Next() {
		var symbol, lesson string
		var weight float64
		rows.Scan(&symbol, &lesson, &weight)
		lessonsText.WriteString(fmt.Sprintf("- [%s, w=%.1f] %s\n", symbol, weight, lesson))
	}

	if lessonsText.Len() == 0 {
		return nil // No lessons yet
	}

	// Get current strategy
	currentStrat, currentVersion, _ := sm.GetCurrentStrategy()

	prompt := fmt.Sprintf(
		"You are PhantomClaw's strategy architect. Rebuild the master trading strategy document based on these lessons:\n\n"+
			"%s\n\n"+
			"Current strategy (v%d):\n%s\n\n"+
			"Write a clear, actionable strategy document with:\n"+
			"1. Key rules per pair\n2. Session preferences\n3. Risk adjustments\n4. Known patterns to exploit\n5. Mistakes to avoid",
		lessonsText.String(), currentVersion, currentStrat,
	)

	newStrategy, err := sm.llmProv.Chat(ctx, []llm.Message{
		{Role: "system", Content: "You are a trading strategy architect. Write precise, actionable rules."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return fmt.Errorf("strategy: LLM rebuild error: %w", err)
	}

	_, err = sm.ApplyPatch("master", strings.TrimSpace(newStrategy),
		"nightly rebuild from lessons", currentVersion)
	return err
}
