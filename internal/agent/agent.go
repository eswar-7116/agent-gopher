package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/eswar-7116/agent-gopher/internal/mcpclient"
	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

type Agent struct {
	client          *openai.Client
	messages        []openai.ChatCompletionMessageParamUnion
	registry        map[string]tools.Tool
	toolDefs        []openai.ChatCompletionToolUnionParam
	model           string
	permissiveShell bool
	tavilyAPIKey    string
	logger          *log.Logger
}

type AgentOptions struct {
	Client          *openai.Client
	Model           string
	PermissiveShell bool
	TavilyAPIKey    string
	MCPServers      []*mcpclient.MCPServer
	Logger          *log.Logger
	WhitelistedCmds []string
	OnWhitelist     func(string)
}

func NewAgent(opts AgentOptions) *Agent {
	registry := tools.Registry(opts.PermissiveShell, opts.TavilyAPIKey, opts.WhitelistedCmds, opts.OnWhitelist)
	defs := tools.Definitions(opts.PermissiveShell, opts.TavilyAPIKey, opts.WhitelistedCmds, opts.OnWhitelist)

	// Register MCP tools
	for _, server := range opts.MCPServers {
		for _, mcpTool := range server.Tools {
			t := tools.NewMCPTool(server, mcpTool)
			registry[t.Name()] = t
			defs = append(defs, t.Definition())
		}
	}

	return &Agent{
		client:          opts.Client,
		registry:        registry,
		toolDefs:        defs,
		model:           opts.Model,
		permissiveShell: opts.PermissiveShell,
		tavilyAPIKey:    opts.TavilyAPIKey,
		logger:          opts.Logger,
	}
}

func (a *Agent) Run(ctx context.Context, userPrompt string) error {
	a.messages = append(a.messages, openai.UserMessage(userPrompt))

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(a.model),
		Messages: a.messages,
		Tools:    a.toolDefs,
	}

	for {
		a.logger.Println("Sending messages to LLM...")
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
			a.logger.Printf("Executing tool: %s\n", name)

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
		a.logger.Printf("Tool %s returned an error: %v\n", name, err)
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
