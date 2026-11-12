package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"

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
	onToolCall      func(name, arguments string)
	onText          func(text string)
	systemPrompt    string
	maxSteps        int // max LLM round-trips per Run; 0 means unlimited
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
	OnToolCall      func(name, arguments string)
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

	a := &Agent{
		client:          opts.Client,
		registry:        registry,
		toolDefs:        defs,
		model:           opts.Model,
		permissiveShell: opts.PermissiveShell,
		tavilyAPIKey:    opts.TavilyAPIKey,
		logger:          opts.Logger,
		onToolCall:      opts.OnToolCall,
	}

	if wd, err := os.Getwd(); err == nil {
		a.systemPrompt = loadAgentsMD(wd, opts.Logger)
	}

	// Sub-agents get every tool registered except delegate_task
	subRegistry := make(map[string]tools.Tool, len(registry))
	maps.Copy(subRegistry, registry)
	subDefs := append([]openai.ChatCompletionToolUnionParam(nil), defs...)

	d := delegateTool{parent: a, registry: subRegistry, defs: subDefs}
	a.registry[d.Name()] = d
	a.toolDefs = append(a.toolDefs, d.Definition())

	return a
}

// Sets a callback for tool calls
func (a *Agent) SetToolCallCallback(fn func(name, arguments string)) {
	a.onToolCall = fn
}

// Sets a callback for assistant text
func (a *Agent) SetTextCallback(fn func(text string)) {
	a.onText = fn
}

func (a *Agent) Run(ctx context.Context, userPrompt string) error {
	if len(a.messages) == 0 && a.systemPrompt != "" {
		a.messages = append(a.messages, openai.SystemMessage(a.systemPrompt))
	}
	a.messages = append(a.messages, openai.UserMessage(userPrompt))

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(a.model),
		Messages: a.messages,
		Tools:    a.toolDefs,
	}

	for step := 0; ; step++ {
		if a.maxSteps > 0 && step >= a.maxSteps {
			return fmt.Errorf("exceeded %d steps without finishing", a.maxSteps)
		}
		a.logger.Println("Sending messages to LLM...")
		params.Messages = a.messages
		resp, err := a.client.Chat.Completions.New(ctx, params)
		if err != nil {
			return fmt.Errorf("chat completion failed: %w", err)
		}

		msg := resp.Choices[0].Message
		a.messages = append(a.messages, msg.ToParam())

		if msg.Content != "" && a.onText != nil {
			a.onText(msg.Content)
		}

		if len(msg.ToolCalls) == 0 {
			break
		}

		for _, toolCall := range msg.ToolCalls {
			name := toolCall.Function.Name
			arguments := toolCall.Function.Arguments
			a.logger.Printf("Executing tool: %s\n", name)

			// Notify TUI about tool call
			if a.onToolCall != nil {
				a.onToolCall(name, arguments)
			}

			toolResult := a.executeToolCall(ctx, name, arguments)
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

// Returns a copy of the conversation history
func (a *Agent) GetMessages() []openai.ChatCompletionMessageParamUnion {
	messages := make([]openai.ChatCompletionMessageParamUnion, len(a.messages))
	copy(messages, a.messages)
	return messages
}

// Returns the content of the last assistant message
func (a *Agent) GetLastAssistantMessage() string {
	for _, msg := range slices.Backward(a.messages) {
		if msg.OfAssistant != nil {
			if msg.OfAssistant.Content.OfString.Valid() {
				return msg.OfAssistant.Content.OfString.Value
			}
		}
	}
	return ""
}
