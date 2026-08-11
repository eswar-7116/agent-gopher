package tools

import (
	"context"
	"os"
	"testing"
)

func BenchmarkRegistryInitialization(b *testing.B) {
	for b.Loop() {
		_ = Registry(true, "test-key")
	}
}

func BenchmarkReadFile_Small(b *testing.B) {
	tool := ReadFileTool{}
	ctx := context.Background()

	f, _ := os.CreateTemp("", "bench_small")
	f.WriteString("hello world")
	f.Close()
	defer os.Remove(f.Name())

	args := map[string]any{"path": f.Name()}

	for b.Loop() {
		_, _ = tool.Execute(ctx, args)
	}
}

func BenchmarkWriteFile_Small(b *testing.B) {
	tool := WriteFileTool{}
	ctx := context.Background()

	f := os.TempDir() + "/bench_write_small.txt"
	defer os.Remove(f)

	args := map[string]any{
		"path":    f,
		"content": "hello world",
	}

	for b.Loop() {
		_, _ = tool.Execute(ctx, args)
	}
}

func BenchmarkShellCmd_Echo(b *testing.B) {
	tool := ShellCmdTool{Permissive: true}
	ctx := context.Background()
	args := map[string]any{"cmd": "echo hello"}

	for b.Loop() {
		_, _ = tool.Execute(ctx, args)
	}
}
