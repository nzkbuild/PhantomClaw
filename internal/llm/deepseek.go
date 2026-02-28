package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DeepSeek implements Provider using the OpenAI-compatible API endpoint.
type DeepSeek struct {
	apiKey  string
	model   string
	baseURL string
}

// NewDeepSeek creates a DeepSeek provider via OpenAI-compatible endpoint.
func NewDeepSeek(cfg ProviderConfig) (*DeepSeek, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm/deepseek: API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	return &DeepSeek{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: "https://api.deepseek.com/v1",
	}, nil
}

func (d *DeepSeek) Name() string { return "deepseek" }

func (d *DeepSeek) Chat(ctx context.Context, messages []Message) (string, error) {
	body := map[string]any{
		"model":    d.model,
		"messages": toGenericMessages(messages),
	}
	return d.post(ctx, "/chat/completions", body)
}

func (d *DeepSeek) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error {
	// Simplified: use non-streaming and call callback once
	result, err := d.Chat(ctx, messages)
	if err != nil {
		return err
	}
	callback(result)
	return nil
}

func (d *DeepSeek) ToolCall(ctx context.Context, messages []Message, tools []Tool) (*ToolResult, error) {
	var toolDefs []map[string]any
	for _, t := range tools {
		toolDefs = append(toolDefs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}

	body := map[string]any{
		"model":    d.model,
		"messages": toGenericMessages(messages),
		"tools":    toolDefs,
	}

	respBody, err := d.rawPost(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("llm/deepseek: parse error: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm/deepseek: no choices")
	}

	result := &ToolResult{Decision: resp.Choices[0].Message.Content}
	for _, tc := range resp.Choices[0].Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return result, nil
}

func (d *DeepSeek) post(ctx context.Context, endpoint string, body map[string]any) (string, error) {
	respBody, err := d.rawPost(ctx, endpoint, body)
	if err != nil {
		return "", err
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("llm/deepseek: parse error: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("llm/deepseek: no choices")
	}
	return resp.Choices[0].Message.Content, nil
}

func (d *DeepSeek) rawPost(ctx context.Context, endpoint string, body map[string]any) ([]byte, error) {
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", d.baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llm/deepseek: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm/deepseek: HTTP error: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("llm/deepseek: API error %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func toGenericMessages(messages []Message) []map[string]string {
	var result []map[string]string
	for _, m := range messages {
		role := m.Role
		if role == "tool" {
			role = "assistant"
		}
		result = append(result, map[string]string{
			"role":    role,
			"content": m.Content,
		})
	}
	return result
}
