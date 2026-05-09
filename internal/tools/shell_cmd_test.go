package tools_test

import (
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

	// Simulate user typing 'n' to deny permission
	simulateStdin(t, "n\n")

	_, err := tool.Execute(t.Context(), map[string]any{
		"cmd": "echo 'should not run'",
	})

	if err == nil {
		t.Fatal("expected an error due to permission denied, but got nil")
	}

	if !strings.Contains(err.Error(), "user denied the permission") {
		t.Errorf("expected permission denied error message, got: %v", err)
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
