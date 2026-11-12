package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

const (
	delegateToolName     = "delegate_task"
	subAgentMaxSteps     = 25 // Bounds LLM round-trips of a sub-agent so a confused one can't loop forever
	subAgentSystemPrompt = "You are a sub-agent working on a single task delegated by a parent agent. " +
		"You cannot ask the user questions. Complete the task using your tools, then reply with a concise " +
		"final report containing everything the parent needs (findings, file paths, results). " +
		"The parent sees only your final message, not your intermediate steps."
)

type delegateTool struct {
	parent   *Agent
	registry map[string]tools.Tool
	defs     []openai.ChatCompletionToolUnionParam
}

func (delegateTool) Name() string { return delegateToolName }

func (d delegateTool) Definition() openai.ChatCompletionToolUnionParam {
	return tools.NewFunctionTool(
		delegateToolName,
		"Delegate a self-contained task to a sub-agent that has its own fresh context and the same tools "+
			"(except delegate_task). Use it for work that is research- or exploration-heavy, or that would "+
			"clutter your context (e.g. searching a codebase, investigating a bug, summarizing many files). "+
			"The sub-agent knows nothing about this conversation, so the task must include all needed context. "+
			"You receive only its final report.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]string{
					"type":        "string",
					"description": "Complete, self-contained instructions for the sub-agent, including any file paths, constraints and the expected shape of the report",
				},
			},
			"required": []string{"task"},
		},
	)
}

func (d delegateTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	task, ok := args["task"].(string)
	if !ok || strings.TrimSpace(task) == "" {
		return nil, fmt.Errorf("missing or invalid 'task' argument")
	}

	p := d.parent
	sub := &Agent{
		client:       p.client,
		registry:     d.registry,
		toolDefs:     d.defs,
		model:        p.model,
		logger:       p.logger,
		systemPrompt: subAgentSystemPrompt,
		maxSteps:     subAgentMaxSteps,
		mode:         p.mode,
		onToolCall: func(name, arguments string) {
			if p.onToolCall != nil {
				p.onToolCall("subagent:"+name, arguments)
			}
		},
	}

	p.logger.Printf("Delegating task to sub-agent: %s\n", task)
	if err := sub.Run(ctx, task); err != nil {
		return nil, fmt.Errorf("sub-agent failed: %w", err)
	}

	report := strings.TrimSpace(sub.GetLastAssistantMessage())
	if report == "" {
		return nil, fmt.Errorf("sub-agent finished without a report")
	}
	return report, nil
}
