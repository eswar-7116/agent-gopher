package tools

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
)

type KillProcessTool struct{}

func (KillProcessTool) Name() string {
	return "kill_process"
}

func (k KillProcessTool) Definition() openai.ChatCompletionToolUnionParam {
	return newFunctionTool(k.Name(), "Kill a background process started by shell_cmd", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]string{
				"type":        "string",
				"description": "The process ID returned by shell_cmd background mode",
			},
		},
		"required": []string{"id"},
	})
}

func (KillProcessTool) Execute(_ context.Context, args map[string]any) (any, error) {
	id, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'id' argument")
	}

	BgProcessesMu.Lock()
	proc, exists := BgProcesses[id]
	BgProcessesMu.Unlock()

	if !exists {
		return nil, fmt.Errorf("no background process found with id %q (may have already exited)", id)
	}

	proc.cancel()
	return fmt.Sprintf("killed process %s (%s)", id, proc.Cmd), nil
}
