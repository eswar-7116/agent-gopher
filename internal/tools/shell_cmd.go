package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/openai/openai-go/v3"
)

// ShellCmdTool implements Tool for executing shell commands
type ShellCmdTool struct{}

func (ShellCmdTool) Name() string {
	return "shell_cmd"
}

func (s ShellCmdTool) Definition() openai.ChatCompletionToolUnionParam {
	return newFunctionTool(s.Name(), "Execute shell commands", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cmd": map[string]string{
				"type":        "string",
				"description": "The shell command to be executed",
			},
		},
		"required": []string{"cmd"},
	})
}

func (ShellCmdTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	cmdStr, ok := args["cmd"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'cmd' argument")
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	fmt.Printf("Agent wants to executed %q command in your shell (y/N): ", cmdStr)
	var permission string
	_, err := fmt.Scanln(&permission)

	if err != nil || len(permission) == 0 {
		return nil, fmt.Errorf("permission denied (defaulted to No)")
	}
	if strings.ToLower(permission)[0] != 'y' {
		return nil, fmt.Errorf("user denied the permission")
	}

	cmd := exec.CommandContext(ctx, shell, "-c", cmdStr)

	cmd.Cancel = func() error {
		return cmd.Process.Kill()
	}

	out, err := cmd.CombinedOutput()
	return string(out), err
}
