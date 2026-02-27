package llm

import "context"

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"` // "system", "user", "assistant", "tool"
	Content string `json:"content"`
}

// Tool describes a tool/function the LLM can call.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// ToolCall represents the LLM's decision to call a tool.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolResult holds the result of a tool execution.
type ToolResult struct {
	Decision  string     `json:"decision"`   // The LLM's final text decision
	ToolCalls []ToolCall `json:"tool_calls"` // Tools the LLM wants to invoke
}

// StreamCallback is called for each chunk of a streaming response.
type StreamCallback func(chunk string)

// Provider defines the interface all LLM providers must implement.
// Swap providers via config with zero code change (PRD §12).
type Provider interface {
	// Name returns the provider identifier (e.g. "claude", "openai").
	Name() string

	// Chat sends messages and returns the full response.
	Chat(ctx context.Context, messages []Message) (string, error)

	// StreamChat sends messages and streams the response via callback.
	StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error

	// ToolCall sends messages with available tools and returns the LLM's tool decisions.
	ToolCall(ctx context.Context, messages []Message, tools []Tool) (*ToolResult, error)
}

// ProviderConfig holds credentials for a single provider.
type ProviderConfig struct {
	APIKey string
	Model  string
}
