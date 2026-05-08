package tools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openai/openai-go/v3"
)

type BackgroundProcess struct {
	ID      string
	Cmd     string
	LogFile string
	PID     int
	cancel  context.CancelFunc
}

var (
	BgProcesses   = map[string]*BackgroundProcess{}
	BgProcessesMu sync.Mutex
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
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return nil, fmt.Errorf("failed to open tty: %v", err)
	}
	defer tty.Close()

	kind := "command"
	if background {
		kind = "background command"
	}
	fmt.Printf("Agent wants to execute %s %q in your shell (y/N): ", kind, cmdStr)

	var permission string
	ttyScanner := bufio.NewScanner(tty)
	if ttyScanner.Scan() {
		permission = strings.TrimSpace(ttyScanner.Text())
	}

	if len(permission) == 0 {
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

	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)

	err := cmd.Run()
	return buf.String(), err
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

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // new process group
	}

	cmd.Stdin = os.NewFile(0, os.DevNull)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		cancel()
		logFile.Close()
		return nil, fmt.Errorf("failed to start background process: %v", err)
	}

	BgProcessesMu.Lock()
	BgProcesses[id] = &BackgroundProcess{
		ID:      id,
		Cmd:     cmdStr,
		LogFile: logFilePath,
		PID:     cmd.Process.Pid,
		cancel:  cancel,
	}
	BgProcessesMu.Unlock()

	go func() {
		cmd.Wait()
		logFile.Close()
		BgProcessesMu.Lock()
		delete(BgProcesses, id)
		BgProcessesMu.Unlock()
	}()

	return map[string]any{
		"id":       id,
		"log_file": logFile,
		"message":  fmt.Sprintf("Process started in background. Logs at %s", logFilePath),
	}, nil
}
