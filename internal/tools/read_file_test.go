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
	result, err := tool.Execute(t.Context(), map[string]any{"filepath": testFilePath})
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

func TestReadFile_Range(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "lines.txt")
	if err := os.WriteFile(testFilePath, []byte("one\ntwo\nthree\nfour\nfive"), 0644); err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"offset and limit", map[string]any{"offset": float64(2), "limit": float64(2)}, "two\nthree\n"},
		{"offset only", map[string]any{"offset": float64(4)}, "four\nfive"},
		{"limit only", map[string]any{"limit": float64(1)}, "one\n"},
		{"limit past EOF", map[string]any{"offset": float64(5), "limit": float64(10)}, "five"},
	}
	tool := tools.ReadFileTool{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.args["filepath"] = testFilePath
			result, err := tool.Execute(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("expected no error, but got: %v", err)
			}
			if result != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, result)
			}
		})
	}
}

func TestReadFile_InvalidRange(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "lines.txt")
	if err := os.WriteFile(testFilePath, []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	tool := tools.ReadFileTool{}
	for _, args := range []map[string]any{
		{"offset": float64(3)},
		{"offset": float64(0)},
		{"limit": float64(-1)},
		{"offset": 1.5},
		{"limit": "10"},
	} {
		args["filepath"] = testFilePath
		if _, err := tool.Execute(t.Context(), args); err == nil {
			t.Fatalf("expected an error for args %v, but got nil", args)
		}
	}
}

func TestReadFile_NonExistentFile(t *testing.T) {
	tool := tools.ReadFileTool{}
	_, err := tool.Execute(t.Context(), map[string]any{"filepath": "/does/not/exist.txt"})
	if err == nil {
		t.Fatal("expected an error for non-existent file, but got nil")
	}
}

func TestReadFile_MissingFilepathArg(t *testing.T) {
	tool := tools.ReadFileTool{}
	_, err := tool.Execute(t.Context(), map[string]any{})
	if err == nil {
		t.Fatal("expected an error for missing 'filepath' arg, but got nil")
	}
}

func TestReadFile_WrongTypeFilepathArg(t *testing.T) {
	tool := tools.ReadFileTool{}
	_, err := tool.Execute(t.Context(), map[string]any{"filepath": 42})
	if err == nil {
		t.Fatal("expected an error for wrong-typed 'filepath' arg, but got nil")
	}
}
