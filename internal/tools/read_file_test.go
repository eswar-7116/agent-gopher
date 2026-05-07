package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func TestReadFile(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "test.txt")
	expected := "This is a test file"

	err := os.WriteFile(testFilePath, []byte(expected), 0644)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	content, err := tools.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if content != expected {
		t.Fatalf("expected %q, got %q", expected, content)
	}
}
