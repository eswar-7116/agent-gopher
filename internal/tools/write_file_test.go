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

func TestWriteFile_InvalidDirectory(t *testing.T) {
	tool := tools.WriteFileTool{}
	_, err := tool.Execute(map[string]any{
		"filepath": "/does/not/exist/file.txt",
		"contents": "hello",
	})
	if err == nil {
		t.Fatal("expected an error for invalid directory, but got nil")
	}
}

func TestWriteFile_MissingFilepathArg(t *testing.T) {
	tool := tools.WriteFileTool{}
	_, err := tool.Execute(map[string]any{"contents": "hello"})
	if err == nil {
		t.Fatal("expected an error for missing 'filepath' arg, but got nil")
	}
}

func TestWriteFile_MissingContentsArg(t *testing.T) {
	tool := tools.WriteFileTool{}
	_, err := tool.Execute(map[string]any{"filepath": "/tmp/x.txt"})
	if err == nil {
		t.Fatal("expected an error for missing 'contents' arg, but got nil")
	}
}

func TestWriteFile_ResultIsMap(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "result.txt")
	tool := tools.WriteFileTool{}
	result, err := tool.Execute(map[string]any{
		"filepath": testFilePath,
		"contents": "data",
	})
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result, got %T", result)
	}
	if m["success"] != true {
		t.Fatalf("expected success=true, got %v", m["success"])
	}
	if m["filepath"] != testFilePath {
		t.Fatalf("expected filepath=%q, got %v", testFilePath, m["filepath"])
	}
}
