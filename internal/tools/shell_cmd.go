package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
)

type BackgroundProcess struct {
	ID      string
	Cmd     string
	LogFile string
	cancel  context.CancelFunc
}

var (
	bgProcesses   = map[string]*BackgroundProcess{}
	bgProcessesMu sync.Mutex
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
			"background": map[string]any{
				"type":        "boolean",
				"description": "Run the command in the background (for servers, watchers, long-running processes, etc.). Logs will be streamed asynchronously. Do NOT use for interactive processes that require user input.",
				"default":     false,
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
	background, _ := args["background"].(bool)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	// Ask user permission
	kind := "command"
	if background {
		kind = "background command"
	}
	fmt.Printf("Agent wants to execute %s %q in your shell (y/N): ", kind, cmdStr)
	var permission string
	_, err := fmt.Scanln(&permission)

	if err != nil || len(permission) == 0 {
		return nil, fmt.Errorf("permission denied (defaulted to No)")
	}
	if strings.ToLower(permission) != "y" {
		return nil, fmt.Errorf("user denied the permission")
	}

	if background {
		return runBackground(cmdStr, shell)
	}
	return runForeground(ctx, cmdStr, shell)
}

func runForeground(ctx context.Context, cmdStr, shell string) (any, error) {
	cmd := exec.CommandContext(ctx, shell, "-c", cmdStr)
	cmd.Cancel = func() error {
		return cmd.Process.Kill()
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runBackground(cmdStr, shell string) (any, error) {
	id := fmt.Sprintf("bg_%d", time.Now().UnixNano())
	logFileName := fmt.Sprintf("agent-gopher-%s.log", id)
	logFilePath := path.Join(os.TempDir(), logFileName)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %v", err)
	}

	bgCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(bgCtx, shell, "-c", cmdStr)

	cmd.Stdin = os.NewFile(0, os.DevNull)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		cancel()
		logFile.Close()
		return nil, fmt.Errorf("failed to start background process: %v", err)
	}

	bgProcessesMu.Lock()
	bgProcesses[id] = &BackgroundProcess{
		ID:      id,
		Cmd:     cmdStr,
		LogFile: logFilePath,
		cancel:  cancel,
	}
	bgProcessesMu.Unlock()

	go func() {
		cmd.Wait()
		logFile.Close()
		bgProcessesMu.Lock()
		delete(bgProcesses, id)
		bgProcessesMu.Unlock()
	}()

	return map[string]any{
		"id":       id,
		"log_file": logFile,
		"message":  fmt.Sprintf("Process started in background. Logs at %s", logFilePath),
	}, nil
}
