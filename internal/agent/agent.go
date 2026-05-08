package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

type Agent struct {
	client   *openai.Client
	messages []openai.ChatCompletionMessageParamUnion
	registry map[string]tools.Tool
}

func NewAgent(client *openai.Client) *Agent {
	return &Agent{
		client:   client,
		registry: tools.Registry(),
	}
}

func (a *Agent) Run(ctx context.Context, userPrompt string) error {
	a.messages = append(a.messages, openai.UserMessage(userPrompt))

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("openrouter/free"),
		Messages: a.messages,
		Tools:    tools.Definitions(),
	}

	for {
		log.Println("Sending messages to LLM...")
		params.Messages = a.messages
		resp, err := a.client.Chat.Completions.New(ctx, params)
		if err != nil {
			return fmt.Errorf("chat completion failed: %w", err)
		}

		msg := resp.Choices[0].Message
		a.messages = append(a.messages, msg.ToParam())

		if len(msg.ToolCalls) == 0 {
			fmt.Println("\nAgent:", msg.Content)
			break
		}

		for _, toolCall := range msg.ToolCalls {
			name := toolCall.Function.Name
			log.Printf("Executing tool: %s\n", name)

			toolResult := a.executeToolCall(ctx, name, toolCall.Function.Arguments)
			a.messages = append(a.messages, openai.ToolMessage(toolResult, toolCall.ID))
		}
	}

	return nil
}

func (a *Agent) executeToolCall(ctx context.Context, name, arguments string) string {
	tool, ok := a.registry[name]
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q", name)
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error: failed to parse arguments: %v", err)
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		log.Printf("Tool %s returned an error: %v\n", name, err)
		return fmt.Sprintf("Error: %v", err)
	}

	return serialize(result)
}

func serialize(v any) string {
	if s, ok := v.(string); ok {
		return s
	}

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
