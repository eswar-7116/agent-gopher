package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	Permissive bool
	Whitelist  *Whitelist
}

func (ShellCmdTool) Name() string {
	return "shell_cmd"
}

func (s ShellCmdTool) Definition() openai.ChatCompletionToolUnionParam {
	return NewFunctionTool(s.Name(), fmt.Sprintf("Execute shell commands. Commands run on %s via %s, so use syntax valid for that shell.", runtime.GOOS, shellName()), map[string]any{
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

var shellOperators = []string{";", "&", "|", "`", "$(", ">", "<", "\n", "\r"}

func hasShellOperators(cmdStr string) bool {
	for _, op := range shellOperators {
		if strings.Contains(cmdStr, op) {
			return true
		}
	}
	return false
}

func isWhitelisted(cmdStr string, whitelistedCmds map[string]bool) bool {
	if len(whitelistedCmds) == 0 || hasShellOperators(cmdStr) {
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

// Commands that only read (for plan mode)
var readOnlyCmds = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true, "pwd": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "find": true, "tree": true,
	"file": true, "stat": true, "du": true, "which": true, "diff": true, "cut": true,
	"sort": true, "basename": true, "dirname": true, "realpath": true, "echo": true,
}

var readOnlyGitCmds = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "blame": true,
	"ls-files": true, "grep": true, "rev-parse": true,
}

// Flags that make an otherwise read-only command write or execute
var unsafeFlags = map[string][]string{
	"find": {"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls"},
	"rg":   {"--pre"},
	"sort": {"-o", "--output"},
	"tree": {"-o"},
	"git":  {"--output", "-o"},
}

// Reports whether cmdStr only reads
func ReadOnlyCommand(cmdStr string) bool {
	if strings.Contains(cmdStr, "||") || hasShellOperators(strings.ReplaceAll(cmdStr, "|", "")) {
		return false
	}
	for seg := range strings.SplitSeq(cmdStr, "|") {
		fields := strings.Fields(seg)
		if len(fields) == 0 || !readOnlyCmds[fields[0]] && fields[0] != "git" {
			return false
		}
		if fields[0] == "git" && !readOnlyGitCmds[gitSubcommand(fields)] {
			return false
		}
		for _, f := range fields[1:] {
			for _, bad := range unsafeFlags[fields[0]] {
				if f == bad || strings.HasPrefix(f, bad+"=") || len(bad) == 2 && strings.HasPrefix(f, bad) {
					return false
				}
			}
		}
	}
	return true
}

// Returns the first non-flag argument of a git command
func gitSubcommand(fields []string) string {
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, "-") {
			return f
		}
	}
	return ""
}

func (s ShellCmdTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	cmdStr, ok := args["cmd"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'cmd' argument")
	}
	background, _ := args["background"].(bool)

	if !s.Permissive && !s.Whitelist.Allows(cmdStr) {
		choice, alt, err := askPermission(PermissionRequest{Kind: PermShell, Command: cmdStr, Background: background})
		if err != nil {
			return nil, err
		}
		switch choice {
		case "a":
			s.Whitelist.Add(cmdStr)
		case "y":
		default:
			return nil, deniedError("command("+cmdStr+")", alt)
		}
	}

	if background {
		return runBackground(cmdStr)
	}
	return runForeground(ctx, cmdStr)
}

// Bounds how long Wait blocks on pipes held open by orphaned children.
const waitDelay = 2 * time.Second

func runForeground(ctx context.Context, cmdStr string) (any, error) {
	cmd := shellCommand(ctx, cmdStr)
	cmd.WaitDelay = waitDelay

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	return buf.String(), err
}

func runBackground(cmdStr string) (any, error) {
	id := fmt.Sprintf("bg_%d", time.Now().UnixNano())
	logFileName := fmt.Sprintf("agent-gopher-%s.log", id)
	logFilePath := filepath.Join(os.TempDir(), logFileName)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %v", err)
	}

	bgCtx, cancel := context.WithCancel(context.Background())
	cmd := shellCommand(bgCtx, cmdStr)
	detach(cmd)

	// Stdin is left nil so the process reads from the null device
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
		"log_file": logFilePath,
		"message":  fmt.Sprintf("Process started in background. Logs at %s", logFilePath),
	}, nil
}
