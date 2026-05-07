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

	tool := tools.ReadFileTool{}
	result, err := tool.Execute(map[string]any{"filepath": testFilePath})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	content, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if content != expected {
		t.Fatalf("expected %q, got %q", expected, content)
	}
}

func TestReadFile_NonExistentFile(t *testing.T) {
	tool := tools.ReadFileTool{}
	_, err := tool.Execute(map[string]any{"filepath": "/does/not/exist.txt"})
	if err == nil {
		t.Fatal("expected an error for non-existent file, but got nil")
	}
}

func TestReadFile_MissingFilepathArg(t *testing.T) {
	tool := tools.ReadFileTool{}
	_, err := tool.Execute(map[string]any{})
	if err == nil {
		t.Fatal("expected an error for missing 'filepath' arg, but got nil")
	}
}

func TestReadFile_WrongTypeFilepathArg(t *testing.T) {
	tool := tools.ReadFileTool{}
	_, err := tool.Execute(map[string]any{"filepath": 42})
	if err == nil {
		t.Fatal("expected an error for wrong-typed 'filepath' arg, but got nil")
	}
}
