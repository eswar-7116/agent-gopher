package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

const ToolName = "save_memory"

const Guidance = "You have two persistent memory scopes. Save user preferences and environment details to global memory; " +
	"save facts that only apply to the current project to project memory. Use the scope argument with the " + ToolName + " tool. " +
	"Save short, self-contained facts; don't save things obvious from the code, or secrets. " +
	"Don't re-save facts already in the selected memory."

type Tool struct {
	GlobalDir  string
	ProjectDir string
}

func (Tool) Name() string { return ToolName }

func (t Tool) Definition() openai.ChatCompletionToolUnionParam {
	return tools.NewFunctionTool(
		ToolName,
		"Save a durable fact to global or current-project persistent memory.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scope": map[string]any{
					"type":        "string",
					"enum":        []string{ScopeGlobal, ScopeProject},
					"description": "Use global for user preferences or environment details; project for facts specific to this project",
				},
				"fact": map[string]any{
					"type":        "string",
					"description": "One short, self-contained fact",
				},
			},
			"required": []string{"scope", "fact"},
		},
	)
}

func (t Tool) Execute(_ context.Context, args map[string]any) (any, error) {
	scope, ok := args["scope"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'scope' argument")
	}
	fact, ok := args["fact"].(string)
	if !ok || strings.TrimSpace(fact) == "" {
		return nil, fmt.Errorf("missing or invalid 'fact' argument")
	}
	var dir string
	switch scope {
	case ScopeGlobal:
		dir = t.GlobalDir
	case ScopeProject:
		dir = t.ProjectDir
	default:
		return nil, fmt.Errorf("invalid memory scope %q", scope)
	}
	if dir == "" {
		return nil, fmt.Errorf("memory scope %q is unavailable", scope)
	}
	added, err := Add(dir, fact)
	if err != nil {
		return nil, err
	}
	if !added {
		return "Already in memory.", nil
	}
	return "Saved to memory.", nil
}
