package agent

import (
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
)

// Controls what the agent is allowed to do.
type Mode int

const (
	ModeNormal Mode = iota
	ModePlan        // Restricts the agent to read-only tools and asks it to produce a plan instead of making changes
)

func (m Mode) String() string {
	if m == ModePlan {
		return "plan"
	}
	return "normal"
}

// Converts a user-supplied name into a Mode.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "normal", "build":
		return ModeNormal, nil
	case "plan":
		return ModePlan, nil
	}
	return ModeNormal, fmt.Errorf("unknown mode %q (use normal or plan)", s)
}

const planModePrompt = "You are in PLAN MODE. Do not modify anything: you may only use read-only tools to investigate " +
	"(reading files and logs, searching the web, delegating research). Tools that write files, run commands or " +
	"kill processes are disabled. Explore as needed, then reply with a clear, step-by-step implementation plan " +
	"naming the files and changes involved. The user will switch out of plan mode to have it executed."

// Tools usable in plan mode (MCP tools are excluded)
var planModeTools = map[string]bool{
	"read_file":      true,
	"read_logs":      true,
	"web_search":     true,
	delegateToolName: true,
}

func toolAllowed(mode Mode, name string) bool {
	return mode != ModePlan || planModeTools[name]
}

func filterDefs(mode Mode, defs []openai.ChatCompletionToolUnionParam) []openai.ChatCompletionToolUnionParam {
	if mode != ModePlan {
		return defs
	}
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(defs))
	for _, d := range defs {
		if fn := d.GetFunction(); fn != nil && planModeTools[fn.Name] {
			out = append(out, d)
		}
	}
	return out
}

func (a *Agent) Mode() Mode { return a.mode }

// Switches the mode
// The switch is recorded in the conversation so the model knows the rules changed
func (a *Agent) SetMode(m Mode) {
	if m == a.mode {
		return
	}
	a.mode = m
	if len(a.messages) == 0 {
		return // the prompt is added when the conversation starts
	}
	if m == ModePlan {
		a.messages = append(a.messages, openai.SystemMessage(planModePrompt))
	} else {
		a.messages = append(a.messages, openai.SystemMessage("Plan mode is off. All tools are available again; you may now carry out changes."))
	}
}
