package skills

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Skill is a callable tool that the agent can invoke (PRD §14).
type Skill struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema for LLM tool definition
	Execute     func(args json.RawMessage) (string, error)
}

// Registry holds all registered skills and dispatches tool calls by name.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
	}
}

// Register adds a skill to the registry.
func (r *Registry) Register(s *Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[s.Name] = s
}

// Execute dispatches a tool call by name with the given arguments.
func (r *Registry) Execute(name string, args json.RawMessage) (string, error) {
	r.mu.RLock()
	skill, ok := r.skills[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("skills: unknown skill %q", name)
	}
	return skill.Execute(args)
}

// List returns all registered skills as LLM tool definitions.
func (r *Registry) List() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := make([]map[string]any, 0, len(names))
	for _, name := range names {
		s := r.skills[name]
		tools = append(tools, map[string]any{
			"name":        s.Name,
			"description": s.Description,
			"parameters":  s.Parameters,
		})
	}
	return tools
}

// Names returns the names of all registered skills.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
