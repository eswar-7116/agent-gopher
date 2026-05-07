package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func TestWriteFile(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "test.txt")
	contents := "This is a test file"

	tool := tools.WriteFileTool{}
	_, err := tool.Execute(map[string]any{
		"filepath": testFilePath,
		"contents": contents,
		"append":   false,
	})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	data, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if string(data) != contents {
		t.Fatalf("expected %q, got %q", contents, string(data))
	}
}

func TestAppendFile(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "append_test.txt")
	writeContent := "Hello "
	appendContent := "World"
	expected := writeContent + appendContent

	err := os.WriteFile(testFilePath, []byte(writeContent), 0644)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	tool := tools.WriteFileTool{}
	_, err = tool.Execute(map[string]any{
		"filepath": testFilePath,
		"contents": appendContent,
		"append":   true,
	})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	data, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}
