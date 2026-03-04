package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillMetadata describes a skill loaded from a skill.yaml file.
type SkillMetadata struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Enabled     bool     `json:"enabled"`
	Tags        []string `json:"tags"`
}

// DiscoverSkills scans a directory for skill definitions (skill.json files)
// and returns metadata for each discovered skill.
func DiscoverSkills(dir string) ([]SkillMetadata, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no skills directory yet
		}
		return nil, fmt.Errorf("skills: discover: %w", err)
	}

	var skills []SkillMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(dir, entry.Name(), "skill.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue // no metadata file, skip
		}

		var meta SkillMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue // malformed metadata, skip
		}
		if meta.ID == "" {
			meta.ID = entry.Name()
		}
		skills = append(skills, meta)
	}
	return skills, nil
}

// FormatSkillList returns a human-readable list of discovered skills for Telegram.
func FormatSkillList(skills []SkillMetadata, registeredNames []string) string {
	var sb strings.Builder
	sb.WriteString("🧩 *Skills*\n\n")

	sb.WriteString("*Active (built-in):*\n")
	for _, name := range registeredNames {
		sb.WriteString(fmt.Sprintf("• %s\n", name))
	}

	if len(skills) > 0 {
		sb.WriteString("\n*Discovered (pluggable):*\n")
		for _, s := range skills {
			status := "✅"
			if !s.Enabled {
				status = "⏸"
			}
			sb.WriteString(fmt.Sprintf("%s %s — %s\n", status, s.Name, s.Description))
		}
	}

	return sb.String()
}
