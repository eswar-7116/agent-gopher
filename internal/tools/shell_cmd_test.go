package tools_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func init() {
	tools.UseStdinForPrompt = true
}

func simulateStdin(t *testing.T, input string) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Replace Stdin
	oldStdin := os.Stdin
	os.Stdin = r

	// Restore Stdin when test finishes
	t.Cleanup(func() {
		os.Stdin = oldStdin
		r.Close()
	})

	// Write simulated user input
	go func() {
		defer w.Close()
		w.Write([]byte(input))
	}()
}

func TestShellCmd_Name(t *testing.T) {
	tool := tools.ShellCmdTool{}
	if tool.Name() != "shell_cmd" {
		t.Errorf("expected 'shell_cmd', got %q", tool.Name())
	}
}

func TestShellCmd_Execute_Success(t *testing.T) {
	tool := tools.ShellCmdTool{Permissive: true}

	result, err := tool.Execute(t.Context(), map[string]any{
		"cmd": "echo 'hello world'",
	})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	out := strings.TrimSpace(result.(string))
	if out != "hello world" {
		t.Errorf("expected 'hello world', got %q", out)
	}
}

func TestShellCmd_Execute_PermissionDenied(t *testing.T) {
	tool := tools.ShellCmdTool{Permissive: false}

	simulateStdin(t, "n\n\n")

	_, err := tool.Execute(t.Context(), map[string]any{
		"cmd": "echo 'should not run'",
	})

	if err == nil {
		t.Fatal("expected an error due to permission denied, but got nil")
	}

	if !strings.Contains(err.Error(), "user denied permission for command(echo 'should not run')") {
		t.Errorf("expected permission denied error message, got: %v", err)
	}
}

func TestShellCmd_Execute_PermissionDeniedWithAlt(t *testing.T) {
	tool := tools.ShellCmdTool{Permissive: false}

	// Simulate user typing 'n' and providing an alternative instruction
	simulateStdin(t, "n\nplease do something else instead\n")

	_, err := tool.Execute(context.Background(), map[string]any{
		"cmd": "echo 'bad command'",
	})

	if err == nil {
		t.Fatal("expected an error due to permission denied, but got nil")
	}

	if !strings.Contains(err.Error(), "The following instruction was provided by the user as an alternative: please do something else instead") {
		t.Errorf("expected alternative instruction in error message, got: %v", err)
	}
}

func TestShellCmd_WhitelistBypass(t *testing.T) {
	tool := tools.ShellCmdTool{
		Permissive: false,
		WhitelistedCmds: map[string]bool{
			"echo": true,
		},
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"cmd": "echo 'hello world'",
	})
	if err != nil {
		t.Fatalf("expected no error (should bypass prompt), got: %v", err)
	}

	out := strings.TrimSpace(result.(string))
	if out != "hello world" {
		t.Errorf("expected 'hello world', got %q", out)
	}
}

func TestShellCmd_AlwaysAllow(t *testing.T) {
	var savedCmd string
	tool := tools.ShellCmdTool{
		Permissive:      false,
		WhitelistedCmds: map[string]bool{},
		OnWhitelist: func(cmd string) {
			savedCmd = cmd
		},
	}

	// Simulate user typing 'a' to always allow
	simulateStdin(t, "a\n")

	result, err := tool.Execute(context.Background(), map[string]any{
		"cmd": "echo 'allowed command'",
	})

	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	out := strings.TrimSpace(result.(string))
	if out != "allowed command" {
		t.Errorf("expected 'allowed command', got %q", out)
	}

	if savedCmd != "echo" {
		t.Errorf("expected OnWhitelist to receive 'echo', got %q", savedCmd)
	}
}

func TestShellCmd_Subcommand_AlwaysAllow(t *testing.T) {
	var savedCmd string
	tool := tools.ShellCmdTool{
		Permissive:      false,
		WhitelistedCmds: map[string]bool{},
		OnWhitelist: func(cmd string) {
			savedCmd = cmd
		},
	}

	// Simulate user typing 'a' to always allow
	simulateStdin(t, "a\n")

	_, err := tool.Execute(context.Background(), map[string]any{
		"cmd": "git status -s",
	})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if savedCmd != "git status" {
		t.Errorf("expected OnWhitelist to receive 'git status', got %q", savedCmd)
	}
}

func TestShellCmd_Subcommand_WhitelistBypass(t *testing.T) {
	tool := tools.ShellCmdTool{
		Permissive: false,
		WhitelistedCmds: map[string]bool{
			"git status": true,
		},
	}

	// Should bypass prompt for git status with args
	_, err := tool.Execute(context.Background(), map[string]any{
		"cmd": "git status -s",
	})
	if err != nil {
		t.Fatalf("expected no error for whitelisted 'git status', got: %v", err)
	}

	// Should NOT bypass prompt for git log
	simulateStdin(t, "n\n\n")
	_, err = tool.Execute(context.Background(), map[string]any{
		"cmd": "git log",
	})
	if err == nil {
		t.Fatal("expected error because 'git log' is not whitelisted, but got nil")
	}
}

func TestShellCmd_Execute_InvalidArgs(t *testing.T) {
	tool := tools.ShellCmdTool{}

	_, err := tool.Execute(t.Context(), map[string]any{"wrong_key": "val"})
	if err == nil {
		t.Fatal("expected error for missing 'cmd' argument")
	}
}

func TestShellCmd_Execute_PermissionGranted(t *testing.T) {
	tool := tools.ShellCmdTool{Permissive: false}

	// Simulate user typing 'y' and pressing Enter
	simulateStdin(t, "y\n")

	result, err := tool.Execute(t.Context(), map[string]any{
		"cmd": "echo 'hello world'",
	})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	out := strings.TrimSpace(result.(string))
	if out != "hello world" {
		t.Errorf("expected 'hello world', got %q", out)
	}
}

func TestShellCmd_Execute_CommandError(t *testing.T) {
	tool := tools.ShellCmdTool{Permissive: true}

	_, err := tool.Execute(t.Context(), map[string]any{
		"cmd": "false",
	})

	if err == nil {
		t.Fatal("expected error for failing command (exit 1), but got nil")
	}
}

func TestShellCmd_Execute_Background(t *testing.T) {
	tool := tools.ShellCmdTool{Permissive: true}

	res, err := tool.Execute(t.Context(), map[string]any{
		"cmd":        "echo 'background process'",
		"background": true,
	})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	resMap, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}

	id, ok := resMap["id"].(string)
	if !ok || id == "" {
		t.Errorf("expected valid id, got %v", resMap["id"])
	}

	logFile, ok := resMap["log_file"].(*os.File)
	if !ok || logFile == nil {
		t.Errorf("expected valid *os.File for log_file, got %v", resMap["log_file"])
	}

	// Poll until the background process writes output
	var content []byte
	success := false
	for i := 0; i < 5; i++ {
		content, err = os.ReadFile(logFile.Name())
		if err == nil && strings.Contains(string(content), "background process") {
			success = true
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	if !success {
		t.Errorf("expected log file to contain 'background process', got %q", string(content))
	}
}
