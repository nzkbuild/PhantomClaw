package llm

import (
	"context"
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAI implements Provider using the OpenAI SDK (GPT-4o).
type OpenAI struct {
	client *openai.Client
	model  string
}

// NewOpenAI creates a GPT-4o provider.
func NewOpenAI(cfg ProviderConfig) (*OpenAI, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm/openai: API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = openai.GPT4o
	}

	client := openai.NewClient(cfg.APIKey)

	return &OpenAI{client: client, model: cfg.Model}, nil
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Chat(ctx context.Context, messages []Message) (string, error) {
	oaiMsgs := toOpenAIMessages(messages)

	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    o.model,
		Messages: oaiMsgs,
	})
	if err != nil {
		return "", fmt.Errorf("llm/openai: chat error: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("llm/openai: no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}

func (o *OpenAI) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error {
	oaiMsgs := toOpenAIMessages(messages)

	stream, err := o.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    o.model,
		Messages: oaiMsgs,
	})
	if err != nil {
		return fmt.Errorf("llm/openai: stream error: %w", err)
	}
	defer stream.Close()

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if len(resp.Choices) > 0 && resp.Choices[0].Delta.Content != "" {
			callback(resp.Choices[0].Delta.Content)
		}
	}
	return nil
}

func (o *OpenAI) ToolCall(ctx context.Context, messages []Message, tools []Tool) (*ToolResult, error) {
	oaiMsgs := toOpenAIMessages(messages)

	// Convert tools to OpenAI function definitions
	var oaiTools []openai.Tool
	for _, t := range tools {
		paramsJSON, _ := json.Marshal(t.Parameters)
		oaiTools = append(oaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  json.RawMessage(paramsJSON),
			},
		})
	}

	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    o.model,
		Messages: oaiMsgs,
		Tools:    oaiTools,
	})
	if err != nil {
		return nil, fmt.Errorf("llm/openai: tool call error: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm/openai: no choices returned")
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

func toOpenAIMessages(messages []Message) []openai.ChatCompletionMessage {
	var oaiMsgs []openai.ChatCompletionMessage
	for _, m := range messages {
		role := m.Role
		if role == "tool" {
			role = "assistant" // OpenAI uses assistant for tool results in simplified mode
		}
		oaiMsgs = append(oaiMsgs, openai.ChatCompletionMessage{
			Role:    role,
			Content: m.Content,
		})
	}
	return oaiMsgs
}
