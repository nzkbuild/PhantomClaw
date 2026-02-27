package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Claude implements Provider using the Anthropic SDK (v1.26+).
type Claude struct {
	client *anthropic.Client
	model  anthropic.Model
}

// NewClaude creates a new Claude provider with the given API key and model.
func NewClaude(cfg ProviderConfig) (*Claude, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm/claude: API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-20250514"
	}

	client := anthropic.NewClient(option.WithAPIKey(cfg.APIKey))

	return &Claude{
		client: &client,
		model:  anthropic.Model(cfg.Model),
	}, nil
}

func (c *Claude) Name() string { return "claude" }

// toMessages converts our Message slice to Anthropic SDK types.
// Returns system prompt separately (Anthropic's API takes it as a top-level field).
func toAnthropicMessages(messages []Message) (string, []anthropic.MessageParam) {
	var system string
	var params []anthropic.MessageParam

	for _, m := range messages {
		switch m.Role {
		case "system":
			system = m.Content
		case "user":
			params = append(params, anthropic.MessageParam{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{{
					OfText: &anthropic.TextBlockParam{Text: m.Content},
				}},
			})
		case "assistant":
			params = append(params, anthropic.MessageParam{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{{
					OfText: &anthropic.TextBlockParam{Text: m.Content},
				}},
			})
		}
	}
	return system, params
}

// Chat sends messages to Claude and returns the full text response.
func (c *Claude) Chat(ctx context.Context, messages []Message) (string, error) {
	system, anthropicMsgs := toAnthropicMessages(messages)

	params := anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 4096,
		Messages:  anthropicMsgs,
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}

	resp, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("llm/claude: chat error: %w", err)
	}

	// Extract text from response content blocks
	var result string
	for _, block := range resp.Content {
		if block.Type == "text" {
			result += block.Text
		}
	}
	return result, nil
}

// StreamChat sends messages and streams the response via callback using Accumulate.
func (c *Claude) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error {
	system, anthropicMsgs := toAnthropicMessages(messages)

	params := anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 4096,
		Messages:  anthropicMsgs,
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}

	stream := c.client.Messages.NewStreaming(ctx, params)
	acc := anthropic.Message{}

	for stream.Next() {
		event := stream.Current()
		if err := acc.Accumulate(event); err != nil {
			return fmt.Errorf("llm/claude: accumulate error: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("llm/claude: stream error: %w", err)
	}

	// Send accumulated text via callback
	for _, block := range acc.Content {
		if block.Type == "text" {
			callback(block.Text)
		}
	}
	return nil
}

// ToolCall sends messages with tool definitions and returns tool call decisions.
func (c *Claude) ToolCall(ctx context.Context, messages []Message, tools []Tool) (*ToolResult, error) {
	system, anthropicMsgs := toAnthropicMessages(messages)

	// Convert our tools to Anthropic tool params
	var anthropicTools []anthropic.ToolUnionParam
	for _, t := range tools {
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: t.Parameters,
				},
			},
		})
	}

	params := anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 4096,
		Messages:  anthropicMsgs,
		Tools:     anthropicTools,
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}

	resp, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm/claude: tool call error: %w", err)
	}

	result := &ToolResult{}
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Decision += block.Text
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(argsJSON),
			})
		}
	}
	return result, nil
}
