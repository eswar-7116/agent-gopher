package tools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func TestReadLogs_Name(t *testing.T) {
	tool := tools.ReadLogsTool{}
	if tool.Name() != "read_logs" {
		t.Errorf("expected 'read_logs', got %q", tool.Name())
	}
}

func TestReadLogs_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nerror: something went wrong\nline 4\nwarning: slow\nline 6\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	tool := tools.ReadLogsTool{}

	tests := []struct {
		name        string
		args        map[string]any
		expectError bool
		expectOut   string
	}{
		{
			name:        "missing log_file",
			args:        map[string]any{},
			expectError: true,
		},
		{
			name:        "read all lines",
			args:        map[string]any{"log_file": logFile},
			expectError: false,
			expectOut:   strings.TrimSpace(content),
		},
		{
			name:        "read with tail",
			args:        map[string]any{"log_file": logFile, "tail": float64(2)},
			expectError: false,
			expectOut:   "warning: slow\nline 6",
		},
		{
			name:        "read with filter",
			args:        map[string]any{"log_file": logFile, "filter": "error"},
			expectError: false,
			expectOut:   "error: something went wrong",
		},
		{
			name:        "read with filter and tail",
			args:        map[string]any{"log_file": logFile, "filter": "line", "tail": float64(2)},
			expectError: false,
			expectOut:   "line 4\nline 6",
		},
		{
			name:        "non-existent log file",
			args:        map[string]any{"log_file": filepath.Join(tmpDir, "non_existent.log")},
			expectError: true,
		},
		{
			name:        "no output yet",
			args:        map[string]any{"log_file": logFile, "filter": "nonexistent"},
			expectError: false,
			expectOut:   "(no output yet)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tool.Execute(t.Context(), tt.args)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			out := res.(string)
			if out != tt.expectOut {
				t.Errorf("expected %q, got %q", tt.expectOut, out)
			}
		})
	}
}
