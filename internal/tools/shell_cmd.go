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
	BgProcesses       = map[string]*BackgroundProcess{}
	BgProcessesMu     sync.Mutex
	UseStdinForPrompt = false // used for testing
)

var subcommandTools = map[string]bool{
	"git":       true,
	"docker":    true,
	"kubectl":   true,
	"go":        true,
	"npm":       true,
	"bun":       true,
	"cargo":     true,
	"yarn":      true,
	"pnpm":      true,
	"gh":        true,
	"terraform": true,
	"aws":       true,
	"pip":       true,
	"systemctl": true,
}

// ShellCmdTool implements Tool for executing shell commands
type ShellCmdTool struct {
	Permissive      bool
	WhitelistedCmds map[string]bool
	OnWhitelist     func(cmd string)
}

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

func extractPrefix(cmdStr string) string {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return ""
	}

	if subcommandTools[fields[0]] && len(fields) >= 2 {
		for i := 1; i < len(fields); i++ {
			if !strings.HasPrefix(fields[i], "-") {
				return fields[0] + " " + fields[i]
			}
		}
	}

	return fields[0]
}

func isWhitelisted(cmdStr string, whitelistedCmds map[string]bool) bool {
	if len(whitelistedCmds) == 0 {
		return false
	}

	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return false
	}

	if whitelistedCmds[fields[0]] {
		return true
	}

	prefix := extractPrefix(cmdStr)
	if prefix != fields[0] && whitelistedCmds[prefix] {
		return true
	}

	return false
}

func (s ShellCmdTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	cmdStr, ok := args["cmd"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'cmd' argument")
	}
	background, _ := args["background"].(bool)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	if s.Permissive || isWhitelisted(cmdStr, s.WhitelistedCmds) {
		if background {
			return runBackground(cmdStr, shell)
		}
		return runForeground(ctx, cmdStr, shell)
	}

	// Ask user permission
	var reader io.Reader
	if UseStdinForPrompt {
		reader = os.Stdin
	} else {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return nil, fmt.Errorf("failed to open tty: %v", err)
		}
		defer tty.Close()
		reader = tty
	}

	kind := "command"
	if background {
		kind = "background command"
	}
	fmt.Printf("Agent wants to execute %s %q in your shell. [y]es / [N]o / [a]lways allow: ", kind, cmdStr)

	var permission string
	ttyScanner := bufio.NewScanner(reader)
	if ttyScanner.Scan() {
		permission = strings.TrimSpace(strings.ToLower(ttyScanner.Text()))
	}

	if permission == "a" || permission == "always" {
		if s.OnWhitelist != nil {
			s.OnWhitelist(extractPrefix(cmdStr))
		}
		if background {
			return runBackground(cmdStr, shell)
		}
		return runForeground(ctx, cmdStr, shell)
	}

	if permission != "y" && permission != "yes" {
		fmt.Print("Command denied. What should the agent do instead? (Press Enter to skip): ")
		var altInstruction string
		if ttyScanner.Scan() {
			altInstruction = strings.TrimSpace(ttyScanner.Text())
		}
		if altInstruction != "" {
			return nil, fmt.Errorf("user denied permission for command(%s). The following instruction was provided by the user as an alternative: %s", cmdStr, altInstruction)
		}
		return nil, fmt.Errorf("user denied permission for command(%s)", cmdStr)
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
