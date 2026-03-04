package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nzkbuild/PhantomClaw/internal/llm"
	"github.com/nzkbuild/PhantomClaw/internal/skills"
)

type staticProvider struct {
	name string
}

func (s *staticProvider) Name() string { return s.name }
func (s *staticProvider) Chat(context.Context, []llm.Message) (string, error) {
	return "", nil
}
func (s *staticProvider) StreamChat(context.Context, []llm.Message, llm.StreamCallback) error {
	return nil
}
func (s *staticProvider) ToolCall(context.Context, []llm.Message, []llm.Tool) (*llm.ToolResult, error) {
	return &llm.ToolResult{Decision: `{"action":"HOLD","reason":"ok"}`}, nil
}

func TestBuildToolDefsForProviderRespectsPolicy(t *testing.T) {
	reg := skills.NewRegistry()
	reg.Register(&skills.Skill{
		Name:        "alpha",
		Description: "allowed",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ json.RawMessage) (string, error) {
			return `{"ok":true}`, nil
		},
	})
	reg.Register(&skills.Skill{
		Name:        "beta",
		Description: "blocked",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ json.RawMessage) (string, error) {
			return `{"ok":true}`, nil
		},
	})

	a := New(Deps{
		LLM:    &staticProvider{name: "router:groq"},
		Skills: reg,
		ToolPolicy: map[string][]string{
			"groq": {"alpha"},
		},
	})

	tools := a.buildToolDefs()
	if len(tools) != 1 {
		t.Fatalf("expected exactly 1 allowed tool, got %d", len(tools))
	}
	if tools[0].Name != "alpha" {
		t.Fatalf("expected allowed tool alpha, got %q", tools[0].Name)
	}
}
