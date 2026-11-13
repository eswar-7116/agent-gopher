package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"

	"github.com/eswar-7116/agent-gopher/internal/mcpclient"
	"github.com/eswar-7116/agent-gopher/internal/memory"
	"github.com/eswar-7116/agent-gopher/internal/paths"
	"github.com/eswar-7116/agent-gopher/internal/skills"
	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

type Agent struct {
	client *openai.Client

	msgMu    sync.RWMutex
	messages []openai.ChatCompletionMessageParamUnion

	registry map[string]tools.Tool
	toolDefs []openai.ChatCompletionToolUnionParam
	model    string
	logger   *log.Logger

	onToolCall func(name, arguments string)
	onText     func(text string)
	onInjected func(texts []string)
	onNotice   func(text string)

	injectMu sync.Mutex
	running  bool     // Run is active and accepting injected messages
	injected []string // user messages waiting for the next step

	systemPrompt string
	maxSteps     int // max LLM round-trips per Run; 0 means unlimited

	mode Mode

	globalMemoryDir  string
	projectMemoryDir string
	memorySeen       string

	contextWindow int // tokens; 0 disables compaction
	usage         usageTracker
	epoch         int // bumped when compaction replaces the history
}

type AgentOptions struct {
	Client          *openai.Client
	Model           string
	PermissiveShell bool
	PermissiveWrite bool
	TavilyAPIKey    string
	ContextWindow   int
	MCPServers      []*mcpclient.MCPServer
	Logger          *log.Logger
	WhitelistedCmds []string
	OnWhitelist     func(string)
	OnToolCall      func(name, arguments string)
}

func NewAgent(opts AgentOptions) *Agent {
	registry := tools.Registry(tools.Options{
		PermissiveShell: opts.PermissiveShell,
		PermissiveWrite: opts.PermissiveWrite,
		TavilyAPIKey:    opts.TavilyAPIKey,
		WhitelistedCmds: opts.WhitelistedCmds,
		OnWhitelist:     opts.OnWhitelist,
	})
	defs := tools.Definitions(registry)

	// Register MCP tools
	for _, server := range opts.MCPServers {
		for _, mcpTool := range server.Tools {
			t := tools.NewMCPTool(server, mcpTool)
			if _, dup := registry[t.Name()]; dup {
				opts.Logger.Printf("Skipping MCP tool %q: name already registered\n", t.Name())
				continue
			}
			registry[t.Name()] = t
			defs = append(defs, t.Definition())
		}
	}

	a := &Agent{
		client:        opts.Client,
		registry:      registry,
		toolDefs:      defs,
		model:         opts.Model,
		logger:        opts.Logger,
		onToolCall:    opts.OnToolCall,
		contextWindow: opts.ContextWindow,
	}

	promptParts := []string{basePrompt}
	if paths.ProjectPath != "" {
		if p := loadAgentsMD(paths.ProjectPath, opts.Logger); p != "" {
			promptParts = append(promptParts, p)
		}
	}
	a.globalMemoryDir, a.projectMemoryDir = paths.GlobalMemoryDir, paths.ProjectMemoryDir
	if a.globalMemoryDir != "" || a.projectMemoryDir != "" {
		// Register before copying the sub-agent registry so sub-agents can save memories too.
		t := memory.Tool{GlobalDir: a.globalMemoryDir, ProjectDir: a.projectMemoryDir}
		a.registry[t.Name()] = t
		a.toolDefs = append(a.toolDefs, t.Definition())
		promptParts = append(promptParts, memory.Guidance)
	}

	// Register before copying the sub-agent registry so sub-agents can use skills too.
	if paths.SkillsDir != "" {
		if loaded := skills.Load(paths.SkillsDir, opts.Logger); len(loaded) > 0 {
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

// Sets a callback for status notices such as compaction
func (a *Agent) SetNoticeCallback(fn func(text string)) {
	a.onNotice = fn
}

// Sets a callback fired when injected messages enter the conversation
func (a *Agent) SetInjectedCallback(fn func(texts []string)) {
	a.onInjected = fn
}

// Queues a user message for the running Run; false if no run is active
func (a *Agent) Inject(text string) bool {
	a.injectMu.Lock()
	defer a.injectMu.Unlock()
	if !a.running {
		return false
	}
	a.injected = append(a.injected, text)
	return true
}

// Appends injected messages; stop also closes injection when none are pending
func (a *Agent) takeInjected(stop bool) bool {
	a.injectMu.Lock()
	texts := a.injected
	a.injected = nil
	if stop && len(texts) == 0 {
		a.running = false
	}
	a.injectMu.Unlock()
	if len(texts) == 0 {
		return false
	}
	for _, t := range texts {
		a.appendMessages(openai.UserMessage(t))
	}
	if a.onInjected != nil {
		a.onInjected(texts)
	}
	return true
}

func (a *Agent) Run(ctx context.Context, userPrompt string) error {
	a.injectMu.Lock()
	a.running, a.injected = true, nil
	a.injectMu.Unlock()
	defer func() {
		a.injectMu.Lock()
		a.running, a.injected = false, nil
		a.injectMu.Unlock()
	}()

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
		a.appendMessages(a.preamble()...)
	}
	a.appendMessages(openai.UserMessage(userPrompt))

	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(a.model),
		Tools: filterDefs(a.mode, a.toolDefs),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}

	for step := 0; ; step++ {
		if a.maxSteps > 0 && step >= a.maxSteps {
			return fmt.Errorf("exceeded %d steps without finishing", a.maxSteps)
		}
		a.updateContextEstimate()
		if a.needsCompaction() {
			if err := a.Compact(ctx); err != nil {
				if ctx.Err() != nil {
					return err
				}
				a.logger.Printf("Compaction failed: %v\n", err)
			}
		}
		a.logger.Println("Sending messages to LLM...")
		params.Messages = a.GetMessages()
		msg, tokens, err := a.complete(ctx, params, true)
		if err != nil {
			// Keep the partial reply so the model knows where it was interrupted
			if ctx.Err() != nil && msg.Content != "" {
				a.appendMessages(openai.AssistantMessage(msg.Content))
			}
			return err
		}
		a.appendMessages(msg.ToParam())
		if tokens > 0 {
			a.usage.observe(tokens)
		}
		a.usage.record(a.messageCount())
		a.updateContextEstimate()

		if len(msg.ToolCalls) == 0 {
			// Keep going if messages were injected while answering
			if a.takeInjected(true) {
				continue
			}
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
		a.updateContextEstimate()
		if ctx.Err() == nil {
			a.takeInjected(false)
		}
	}

	return nil
}

// Performs one streamed LLM round-trip, forwarding text deltas to onText if emit; also returns reported total tokens
func (a *Agent) complete(ctx context.Context, params openai.ChatCompletionNewParams, emit bool) (openai.ChatCompletionMessage, int, error) {
	stream := a.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var acc openai.ChatCompletionAccumulator
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if emit && a.onText != nil && len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			a.onText(chunk.Choices[0].Delta.Content)
		}
	}
	tokens := int(acc.Usage.TotalTokens)
	if err := stream.Err(); err != nil {
		var partial openai.ChatCompletionMessage
		if len(acc.Choices) > 0 {
			partial.Content = acc.Choices[0].Message.Content
		}
		return partial, tokens, fmt.Errorf("chat completion failed: %w", err)
	}
	if len(acc.Choices) == 0 {
		return openai.ChatCompletionMessage{}, tokens, fmt.Errorf("chat completion failed: empty response")
	}
	return acc.Choices[0].Message, tokens, nil
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
	if err := checkPlanShell(a.mode, name, args); err != nil {
		return "Error: " + err.Error()
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		a.logger.Printf("Tool %s returned an error: %v\n", name, err)
		// Keep output alongside the error so the model can see why it failed
		if out := strings.TrimSpace(serializeResult(result)); out != "" {
			return capOutput(fmt.Sprintf("Error: %v\n\nOutput:\n%s", err, out))
		}
		return fmt.Sprintf("Error: %v", err)
	}

	return capOutput(serialize(result))
}

// Serializes a possibly nil tool result
func serializeResult(v any) string {
	if v == nil {
		return ""
	}
	return serialize(v)
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

// Replaces the conversation history with messages saved by LogTail
func (a *Agent) Restore(raw []json.RawMessage) error {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(raw))
	for i, r := range raw {
		var msg openai.ChatCompletionMessageParamUnion
		if err := json.Unmarshal(r, &msg); err != nil {
			return fmt.Errorf("decode message %d: %w", i, err)
		}
		msgs = append(msgs, msg)
	}
	a.msgMu.Lock()
	a.messages = msgs
	a.msgMu.Unlock()
	a.usage.reset()
	a.updateContextEstimate()

	// Announce memory on the next run only if it changed since the model last saw it
	a.memorySeen = ""
	cur := a.currentMemory()
	for _, msg := range msgs {
		if msg.OfSystem == nil {
			continue
		}
		c := msg.OfSystem.Content.OfString.Value
		if c == cur || c == "Global memory was updated. Current contents:\n"+cur {
			a.memorySeen = cur
		}
	}
	return nil
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

// Returns messages from index from, each encoded as JSON, and the current epoch;
// if epoch is stale the history was compacted, so all messages are returned
func (a *Agent) LogTail(epoch, from int) ([]json.RawMessage, int, error) {
	a.msgMu.RLock()
	cur := a.epoch
	if epoch != cur {
		from = 0
	}
	var tail []openai.ChatCompletionMessageParamUnion
	if from < len(a.messages) {
		tail = append(tail, a.messages[max(from, 0):]...)
	}
	a.msgMu.RUnlock()

	out := make([]json.RawMessage, 0, len(tail))
	for _, msg := range tail {
		b, err := json.Marshal(msg)
		if err != nil {
			return nil, cur, fmt.Errorf("encode message: %w", err)
		}
		out = append(out, b)
	}
	return out, cur, nil
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
