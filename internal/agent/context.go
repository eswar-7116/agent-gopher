package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

const (
	basePrompt = "You are Agent Gopher, an agent that helps users with their tasks using your tools and skills."

	maxToolOutput    = 30000 // chars of a tool result kept in the conversation
	compactPercent   = 80    // compact once the context is this full
	summaryMsgLimit  = 4000  // chars of each message fed to the summarizer
	charsPerToken    = 4     // rough estimate when the provider reports no usage
	compactPrompt    = "You compact conversations between a user and a coding agent. Summarize the conversation below so the agent can continue the work without it. Keep the user's goals and instructions, decisions made, files read or changed and how, commands run and their outcomes, errors and how they were fixed, and what remains to be done. Be concise but complete. Reply with the summary only."
	summaryPrefix    = "Summary of the conversation so far (earlier messages were compacted to save context):\n\n"
	planShellReadErr = "only read-only commands (ls, cat, grep, rg, find, git status/log/diff/show, etc.) without redirection or chaining are allowed in plan mode"
)

var ErrNothingToCompact = errors.New("nothing to compact yet")

// Keeps head and tail of oversized tool output
func capOutput(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	half := maxToolOutput / 2
	return strings.ToValidUTF8(s[:half], "") +
		fmt.Sprintf("\n\n[... %d characters truncated. Narrow the command or use read_file with offset/limit ...]\n\n", len(s)-maxToolOutput) +
		strings.ToValidUTF8(s[len(s)-half:], "")
}

// Rejects shell commands that could change anything while in plan mode
func checkPlanShell(mode Mode, name string, args map[string]any) error {
	if mode != ModePlan || name != "shell_cmd" {
		return nil
	}
	if bg, _ := args["background"].(bool); bg {
		return errors.New("background commands are disabled in plan mode")
	}
	if cmd, _ := args["cmd"].(string); !tools.ReadOnlyCommand(cmd) {
		return errors.New(planShellReadErr)
	}
	return nil
}

// Tracks how much of the context window the conversation uses
type usageTracker struct {
	mu       sync.Mutex
	reported int // tokens the provider reported for the last response
	pending  int // reported by the response being recorded
	at       int // message count the reported tokens cover
	estimate int // current context size in tokens
}

func (u *usageTracker) observe(tokens int) {
	u.mu.Lock()
	u.pending = tokens
	u.mu.Unlock()
}

// Binds the last observed usage to the first n messages
func (u *usageTracker) record(n int) {
	u.mu.Lock()
	if u.pending > 0 {
		u.reported, u.at, u.pending = u.pending, n, 0
	}
	u.mu.Unlock()
}

func (u *usageTracker) reset() {
	u.mu.Lock()
	u.reported, u.at, u.pending = 0, 0, 0
	u.mu.Unlock()
}

// Returns the reported tokens and the message count they cover
func (u *usageTracker) base() (int, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.reported, u.at
}

func (u *usageTracker) set(n int) {
	u.mu.Lock()
	u.estimate = n
	u.mu.Unlock()
}

func (u *usageTracker) get() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.estimate
}

// Recomputes the context size from reported usage plus estimated newer messages
func (a *Agent) updateContextEstimate() {
	reported, at := a.usage.base()
	a.msgMu.RLock()
	if at > len(a.messages) {
		reported, at = 0, 0
	}
	chars := 0
	for _, m := range a.messages[at:] {
		if b, err := json.Marshal(m); err == nil {
			chars += len(b)
		}
	}
	a.msgMu.RUnlock()
	a.usage.set(reported + chars/charsPerToken)
}

// Returns the estimated tokens in the conversation
func (a *Agent) ContextTokens() int { return a.usage.get() }

// Returns the context window size; 0 means unknown
func (a *Agent) ContextWindow() int { return a.contextWindow }

func (a *Agent) needsCompaction() bool {
	return a.contextWindow > 0 && a.ContextTokens() >= a.contextWindow*compactPercent/100
}

// Builds the leading system messages of a conversation
func (a *Agent) preamble() []openai.ChatCompletionMessageParamUnion {
	var p []openai.ChatCompletionMessageParamUnion
	if a.systemPrompt != "" {
		p = append(p, openai.SystemMessage(a.systemPrompt))
	}
	if a.memorySeen != "" {
		p = append(p, openai.SystemMessage(a.memorySeen))
	}
	if a.mode == ModePlan {
		p = append(p, openai.SystemMessage(planModePrompt))
	}
	return p
}

// Replaces the history with a summary to free context
func (a *Agent) Compact(ctx context.Context) error {
	msgs := a.GetMessages()
	start := 0
	for start < len(msgs) && msgs[start].OfSystem != nil {
		start++
	}
	if len(msgs)-start < 2 {
		return ErrNothingToCompact
	}
	before := a.ContextTokens()

	summary, err := a.summarize(ctx, msgs[start:])
	if err != nil {
		return fmt.Errorf("compact: %w", err)
	}

	if a.globalMemoryDir != "" || a.projectMemoryDir != "" {
		a.memorySeen = a.currentMemory()
	}
	fresh := append(a.preamble(), openai.UserMessage(summaryPrefix+summary))
	a.msgMu.Lock()
	a.messages = append(fresh, a.messages[len(msgs):]...)
	a.epoch++
	a.msgMu.Unlock()

	a.usage.reset()
	a.updateContextEstimate()
	if a.onNotice != nil {
		a.onNotice(fmt.Sprintf("Context compacted (~%d → ~%d tokens).", before, a.ContextTokens()))
	}
	return nil
}

// Asks the model for a summary of msgs
func (a *Agent) summarize(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion) (string, error) {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(messageText(m))
		b.WriteString("\n\n")
	}
	transcript := b.String()
	// Keep the newest part if the transcript alone would overflow the window
	if limit := a.contextWindow * charsPerToken * compactPercent / 100; limit > 0 && len(transcript) > limit {
		transcript = "[earlier messages omitted]\n\n" + strings.ToValidUTF8(transcript[len(transcript)-limit:], "")
	}

	msg, _, err := a.complete(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(a.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(compactPrompt),
			openai.UserMessage(transcript),
		},
	}, false)
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(msg.Content)
	if summary == "" {
		return "", errors.New("empty summary")
	}
	return summary, nil
}

// Renders a message as plain text for the summarizer
func messageText(m openai.ChatCompletionMessageParamUnion) string {
	var v struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	raw, err := json.Marshal(m)
	if err != nil || json.Unmarshal(raw, &v) != nil {
		return ""
	}

	var content string
	if json.Unmarshal(v.Content, &content) != nil {
		var parts []struct {
			Text string `json:"text"`
		}
		json.Unmarshal(v.Content, &parts)
		for _, p := range parts {
			content += p.Text
		}
	}

	var b strings.Builder
	b.WriteString(v.Role)
	b.WriteString(": ")
	b.WriteString(clip(content, summaryMsgLimit))
	for _, tc := range v.ToolCalls {
		b.WriteString(fmt.Sprintf("\n[called %s %s]", tc.Function.Name, clip(tc.Function.Arguments, summaryMsgLimit)))
	}
	return b.String()
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + " [...]"
}
