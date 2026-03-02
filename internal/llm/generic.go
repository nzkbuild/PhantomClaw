package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GenericProvider implements Provider for any OpenAI-compatible API endpoint.
// Works with: OpenRouter, Ollama, Groq, Mistral, Together AI, DeepSeek,
// and any provider that speaks the /v1/chat/completions format.
type GenericProvider struct {
	name    string
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// GenericConfig holds configuration for a generic OpenAI-compatible provider.
type GenericConfig struct {
	Name    string // Provider identifier (e.g. "groq", "ollama", "openrouter")
	BaseURL string // API base URL (e.g. "https://api.groq.com/openai/v1")
	APIKey  string
	Model   string
}

// NewGeneric creates a provider for any OpenAI-compatible endpoint.
func NewGeneric(cfg GenericConfig) (*GenericProvider, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("llm/generic: provider name is required")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("llm/generic: base URL is required for %s", cfg.Name)
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("llm/generic: model is required for %s", cfg.Name)
	}

	// Trim trailing slash for consistent URL building
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	return &GenericProvider{
		name:    cfg.Name,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: cfg.BaseURL,
		client:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (g *GenericProvider) Name() string { return g.name }

func (g *GenericProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	body := map[string]any{
		"model":    g.model,
		"messages": toGenericMessages(messages),
	}
	return g.post(ctx, "/chat/completions", body)
}

func (g *GenericProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error {
	body := map[string]any{
		"model":    g.model,
		"messages": toGenericMessages(messages),
		"stream":   true,
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", g.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("llm/%s: stream request error: %w", g.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("llm/%s: stream HTTP error: %w", g.name, err)
	}
	defer resp.Body.Close()

	// Some OpenAI-compatible backends do not support SSE streaming.
	// Fall back to non-streaming completion in that case.
	if resp.StatusCode != http.StatusOK {
		result, err := g.Chat(ctx, messages)
		if err != nil {
			respBody, _ := io.ReadAll(resp.Body)
			return &APIError{
				Provider:   g.name,
				StatusCode: resp.StatusCode,
				Body:       string(respBody),
			}
		}
		callback(result)
		return nil
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("llm/%s: stream read error: %w", g.name, err)
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Text    string `json:"text"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		text := chunk.Choices[0].Delta.Content
		if text == "" {
			text = chunk.Choices[0].Text
		}
		if text == "" {
			text = chunk.Choices[0].Message.Content
		}
		if text != "" {
			callback(text)
		}
	}

	return nil
}

func (g *GenericProvider) ToolCall(ctx context.Context, messages []Message, tools []Tool) (*ToolResult, error) {
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
		"model":    g.model,
		"messages": toGenericMessages(messages),
		"tools":    toolDefs,
	}

	respBody, err := g.rawPost(ctx, "/chat/completions", body)
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
		return nil, fmt.Errorf("llm/%s: parse error: %w", g.name, err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm/%s: no choices returned", g.name)
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

// --- HTTP helpers ---

func (g *GenericProvider) post(ctx context.Context, endpoint string, body map[string]any) (string, error) {
	respBody, err := g.rawPost(ctx, endpoint, body)
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
		return "", fmt.Errorf("llm/%s: parse error: %w", g.name, err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("llm/%s: no choices returned", g.name)
	}
	return resp.Choices[0].Message.Content, nil
}

func (g *GenericProvider) rawPost(ctx context.Context, endpoint string, body map[string]any) ([]byte, error) {
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", g.baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llm/%s: request error: %w", g.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm/%s: HTTP error: %w", g.name, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, &APIError{
			Provider:   g.name,
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}
	return respBody, nil
}

// APIError represents an HTTP API error with status code for error classification.
type APIError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("llm/%s: API error %d: %s", e.Provider, e.StatusCode, e.Body)
}

func toGenericMessages(messages []Message) []map[string]string {
	var result []map[string]string
	for _, m := range messages {
		result = append(result, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	return result
}
