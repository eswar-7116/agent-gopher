package tools_test

import (
	"strings"
	"testing"
	"time"

	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func TestKillProcess_Name(t *testing.T) {
	tool := tools.KillProcessTool{}
	if tool.Name() != "kill_process" {
		t.Errorf("expected 'kill_process', got %q", tool.Name())
	}
}

func TestKillProcess_Execute(t *testing.T) {
	// start a background process
	shellTool := tools.ShellCmdTool{}
	simulateStdin(t, "y\n")

	// Start a process that sleeps for 10 seconds
	res, err := shellTool.Execute(t.Context(), map[string]any{
		"cmd":        "sleep 10",
		"background": true,
	})
	if err != nil {
		t.Fatalf("expected no error starting bg process, got: %v", err)
	}

	resMap, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be map[string]any, got %T", res)
	}

	id, ok := resMap["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected valid id in result, got: %v", resMap["id"])
	}

	// Try to kill it
	killTool := tools.KillProcessTool{}
	killRes, killErr := killTool.Execute(t.Context(), map[string]any{
		"id": id,
	})

	if killErr != nil {
		t.Fatalf("expected no error killing process, got: %v", killErr)
	}

	killMsg := killRes.(string)
	if !strings.Contains(killMsg, "killed process "+id) {
		t.Errorf("unexpected kill message: %s", killMsg)
	}

	// Poll until the background goroutine cleans up the process
	success := false
	for i := 0; i < 5; i++ {
		_, errAgain := killTool.Execute(t.Context(), map[string]any{"id": id})
		if errAgain != nil && strings.Contains(errAgain.Error(), "no background process found") {
			success = true
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	if !success {
		t.Errorf("expected process to be removed from registry after kill, but it persisted")
	}
}

func TestKillProcess_Execute_InvalidArgs(t *testing.T) {
	tool := tools.KillProcessTool{}

	_, err := tool.Execute(t.Context(), map[string]any{"wrong_key": "val"})
	if err == nil {
		t.Fatal("expected error for missing 'id' argument")
	}
}
