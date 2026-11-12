package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/eswar-7116/agent-gopher/internal/mcpclient"
	"github.com/eswar-7116/agent-gopher/internal/memory"
	"github.com/eswar-7116/agent-gopher/internal/skills"
	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

type Agent struct {
	client *openai.Client

	msgMu    sync.RWMutex
	messages []openai.ChatCompletionMessageParamUnion

	registry        map[string]tools.Tool
	toolDefs        []openai.ChatCompletionToolUnionParam
	model           string
	permissiveShell bool
	tavilyAPIKey    string
	logger          *log.Logger

	onToolCall func(name, arguments string)
	onText     func(text string)

	systemPrompt string
	maxSteps     int // max LLM round-trips per Run; 0 means unlimited

	mode Mode

	globalMemoryDir  string
	projectMemoryDir string
	memorySeen       string
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

	var promptParts []string
	var projectPath string
	if wd, err := os.Getwd(); err == nil {
		projectPath = wd
		if p := loadAgentsMD(wd, opts.Logger); p != "" {
			promptParts = append(promptParts, p)
		}
	}
	a.globalMemoryDir = memory.GlobalDir()
	if projectPath != "" {
		a.projectMemoryDir = memory.ProjectDir(projectPath)
	}
	if a.globalMemoryDir != "" || a.projectMemoryDir != "" {
		// Register before copying the sub-agent registry so sub-agents can save memories too.
		t := memory.Tool{GlobalDir: a.globalMemoryDir, ProjectDir: a.projectMemoryDir}
		a.registry[t.Name()] = t
		a.toolDefs = append(a.toolDefs, t.Definition())
		promptParts = append(promptParts, memory.Guidance)
	}

	// Register before copying the sub-agent registry so sub-agents can use skills too.
	if dir := skills.Dir(); dir != "" {
		if loaded := skills.Load(dir, opts.Logger); len(loaded) > 0 {
			promptParts = append(promptParts, skills.Prompt(loaded))
			t := skills.NewTool(loaded)
			a.registry[t.Name()] = t
			a.toolDefs = append(a.toolDefs, t.Definition())
		}
	}
	a.systemPrompt = strings.Join(promptParts, "\n\n")

	// Sub-agents get every tool registered except delegate_task
	subRegistry := make(map[string]tools.Tool, len(a.registry))
	for name, t := range a.registry {
		subRegistry[name] = t
	}
	subDefs := append([]openai.ChatCompletionToolUnionParam(nil), a.toolDefs...)

	d := delegateTool{parent: a, registry: subRegistry, defs: subDefs}
	a.registry[d.Name()] = d
	a.toolDefs = append(a.toolDefs, d.Definition())

	return a
}

// Sets a callback for tool calls
func (a *Agent) SetToolCallCallback(fn func(name, arguments string)) {
	a.onToolCall = fn
}

// Sets a callback to receive assistant text deltas as they stream in
func (a *Agent) SetTextCallback(fn func(delta string)) {
	a.onText = fn
}

func (a *Agent) MemoryDir(scope string) string {
	if scope == memory.ScopeGlobal {
		return a.globalMemoryDir
	}
	return a.projectMemoryDir
}

func (a *Agent) currentMemory() string {
	global := memory.Content(a.globalMemoryDir, memory.ScopeGlobal)
	project := memory.Content(a.projectMemoryDir, memory.ScopeProject)
	if global == "" {
		return project
	}
	if project == "" {
		return global
	}
	return global + "\n\n" + project
}

func (a *Agent) Run(ctx context.Context, userPrompt string) error {
	if a.globalMemoryDir != "" || a.projectMemoryDir != "" {
		cur := a.currentMemory()
		if a.messageCount() > 0 && cur != a.memorySeen {
			update := cur
			if update == "" {
				update = "Global memory is now empty."
			} else {
				update = "Global memory was updated. Current contents:\n" + update
			}
			a.appendMessages(openai.SystemMessage(update))
		}
		a.memorySeen = cur

		defer func() { a.memorySeen = a.currentMemory() }()
	}

	if a.messageCount() == 0 {
		var preamble []openai.ChatCompletionMessageParamUnion
		if a.systemPrompt != "" {
			preamble = append(preamble, openai.SystemMessage(a.systemPrompt))
		}
		if a.memorySeen != "" {
			preamble = append(preamble, openai.SystemMessage(a.memorySeen))
		}
		if a.mode == ModePlan {
			preamble = append(preamble, openai.SystemMessage(planModePrompt))
		}
		a.appendMessages(preamble...)
	}
	a.appendMessages(openai.UserMessage(userPrompt))

	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(a.model),
		Tools: filterDefs(a.mode, a.toolDefs),
	}

	for step := 0; ; step++ {
		if a.maxSteps > 0 && step >= a.maxSteps {
			return fmt.Errorf("exceeded %d steps without finishing", a.maxSteps)
		}
		a.logger.Println("Sending messages to LLM...")
		params.Messages = a.GetMessages()
		msg, err := a.complete(ctx, params)
		if err != nil {
			return err
		}
		a.appendMessages(msg.ToParam())

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
			a.appendMessages(openai.ToolMessage(toolResult, toolCall.ID))
		}
	}

	return nil
}

// complete performs one streamed LLM round-trip, forwarding text deltas to onText as they arrive.
func (a *Agent) complete(ctx context.Context, params openai.ChatCompletionNewParams) (openai.ChatCompletionMessage, error) {
	stream := a.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var acc openai.ChatCompletionAccumulator
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if a.onText != nil && len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			a.onText(chunk.Choices[0].Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		return openai.ChatCompletionMessage{}, fmt.Errorf("chat completion failed: %w", err)
	}
	if len(acc.Choices) == 0 {
		return openai.ChatCompletionMessage{}, fmt.Errorf("chat completion failed: empty response")
	}
	return acc.Choices[0].Message, nil
}

func (a *Agent) executeToolCall(ctx context.Context, name, arguments string) string {
	if !toolAllowed(a.mode, name) {
		return fmt.Sprintf("Error: tool %q is disabled in plan mode", name)
	}
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

// Returns the model the agent talks to
func (a *Agent) Model() string { return a.model }

// Appends to the conversation history
func (a *Agent) appendMessages(msgs ...openai.ChatCompletionMessageParamUnion) {
	if len(msgs) == 0 {
		return
	}
	a.msgMu.Lock()
	defer a.msgMu.Unlock()
	a.messages = append(a.messages, msgs...)
}

// Returns the number of messages in the conversation history
func (a *Agent) messageCount() int {
	a.msgMu.RLock()
	defer a.msgMu.RUnlock()
	return len(a.messages)
}

// Returns a copy of the conversation history
func (a *Agent) GetMessages() []openai.ChatCompletionMessageParamUnion {
	a.msgMu.RLock()
	defer a.msgMu.RUnlock()
	messages := make([]openai.ChatCompletionMessageParamUnion, len(a.messages))
	copy(messages, a.messages)
	return messages
}

// Returns the messages from a given index, each encoded as JSON
func (a *Agent) MessagesJSONFrom(from int) ([]json.RawMessage, error) {
	a.msgMu.RLock()
	var tail []openai.ChatCompletionMessageParamUnion
	if from < len(a.messages) {
		tail = append(tail, a.messages[max(from, 0):]...)
	}
	a.msgMu.RUnlock()

	out := make([]json.RawMessage, 0, len(tail))
	for _, msg := range tail {
		b, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("encode message: %w", err)
		}
		out = append(out, b)
	}
	return out, nil
}

// Returns the content of the last assistant message
func (a *Agent) GetLastAssistantMessage() string {
	a.msgMu.RLock()
	defer a.msgMu.RUnlock()
	for _, msg := range slices.Backward(a.messages) {
		if msg.OfAssistant != nil {
			if msg.OfAssistant.Content.OfString.Valid() {
				return msg.OfAssistant.Content.OfString.Value
			}
		}
	}
	return ""
}
