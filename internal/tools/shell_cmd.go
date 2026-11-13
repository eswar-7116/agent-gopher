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
	"mvdan.cc/sh/v3/syntax"
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

// Returns the command name, plus its subcommand for tools like git
func prefixOf(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	if subcommandTools[fields[0]] {
		for _, f := range fields[1:] {
			if !strings.HasPrefix(f, "-") {
				return fields[0] + " " + f
			}
		}
	}
	return fields[0]
}

// Returns the prefix of the first command in cmdStr
func extractPrefix(cmdStr string) string {
	f := parseShell(cmdStr)
	if f == nil {
		return ""
	}
	var fields []string
	found := false
	syntax.Walk(f, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if found || !ok {
			return !found
		}
		found = true
		for _, w := range call.Args {
			lit, ok := literal(w)
			if !ok {
				break
			}
			fields = append(fields, lit)
		}
		return false
	})
	return prefixOf(fields)
}

var shellOperators = []string{";", "&", "|", "`", "$(", ">", "<", "\n", "\r"}

// cmd.exe doesn't honour POSIX quoting
func windowsOperators(cmdStr string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	for _, op := range shellOperators {
		if strings.Contains(cmdStr, op) {
			return true
		}
	}
	return false
}

// Parses cmdStr as a shell script
func parseShell(cmdStr string) *syntax.File {
	f, err := syntax.NewParser().Parse(strings.NewReader(cmdStr), "")
	if err != nil {
		return nil
	}
	return f
}

// Returns the word's value if the shell would pass it through unchanged
func literal(w *syntax.Word) (string, bool) {
	if syntax.SplitBraces(w) {
		return "", false
	}
	var b strings.Builder
	unquotedGlob := false
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.Contains(p.Value, `\`) {
				return "", false
			}
			unquotedGlob = unquotedGlob || strings.ContainsAny(p.Value, "*?[")
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				return "", false
			}
			for _, dp := range p.Parts {
				lit, ok := dp.(*syntax.Lit)
				if !ok || strings.Contains(lit.Value, `\`) {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	s := b.String()

	if unquotedGlob && strings.HasPrefix(s, "-") {
		return "", false
	}
	return s, true
}

// Reports whether s has no redirects, backgrounding or negation
func plainStmt(s *syntax.Stmt) bool {
	return len(s.Redirs) == 0 && !s.Background && !s.Coprocess && !s.Disown && !s.Negated
}

// Returns the words of a simple command made only of literal words
func callWords(s *syntax.Stmt) ([]string, bool) {
	call, ok := s.Cmd.(*syntax.CallExpr)
	if !ok || !plainStmt(s) || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return nil, false
	}
	fields := make([]string, len(call.Args))
	for i, w := range call.Args {
		lit, ok := literal(w)
		if !ok {
			return nil, false
		}
		fields[i] = lit
	}
	return fields, true
}

// Returns the words of cmdStr if it is a single simple command
func singleCommand(cmdStr string) ([]string, bool) {
	f := parseShell(cmdStr)
	if f == nil || len(f.Stmts) != 1 {
		return nil, false
	}
	return callWords(f.Stmts[0])
}

func isWhitelisted(cmdStr string, whitelistedCmds map[string]bool) bool {
	if len(whitelistedCmds) == 0 || windowsOperators(cmdStr) {
		return false
	}
	fields, ok := singleCommand(cmdStr)
	if !ok {
		return false
	}
	return whitelistedCmds[fields[0]] || whitelistedCmds[prefixOf(fields)]
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
	if windowsOperators(strings.ReplaceAll(cmdStr, "|", "")) {
		return false
	}
	f := parseShell(cmdStr)
	if f == nil || len(f.Stmts) != 1 {
		return false
	}
	return readOnlyStmt(f.Stmts[0])
}

// Reports whether s is a read-only command or a pipeline of them
func readOnlyStmt(s *syntax.Stmt) bool {
	if bin, ok := s.Cmd.(*syntax.BinaryCmd); ok {
		return plainStmt(s) && bin.Op == syntax.Pipe && readOnlyStmt(bin.X) && readOnlyStmt(bin.Y)
	}
	fields, ok := callWords(s)
	if !ok || !readOnlyCmds[fields[0]] && fields[0] != "git" {
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
