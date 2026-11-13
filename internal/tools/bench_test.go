package tools

import (
	"context"
	"os"
	"testing"
)

func BenchmarkRegistryInitialization(b *testing.B) {
	for b.Loop() {
		_ = Registry(Options{})
	}
}

func BenchmarkReadFile_Small(b *testing.B) {
	tool := ReadFileTool{}
	ctx := context.Background()

	f, _ := os.CreateTemp("", "bench_small")
	f.WriteString("hello world")
	f.Close()
	defer os.Remove(f.Name())

	args := map[string]any{"filepath": f.Name()}

	if _, err := tool.Execute(ctx, args); err != nil {
		b.Fatal(err)
	}
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
		"filepath": f,
		"contents": "hello world",
	}

	if _, err := tool.Execute(ctx, args); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		_, _ = tool.Execute(ctx, args)
	}
}

func BenchmarkShellCmd_Echo(b *testing.B) {
	tool := ShellCmdTool{Permissive: true}
	ctx := context.Background()
	args := map[string]any{"cmd": "echo hello"}

	if _, err := tool.Execute(ctx, args); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		_, _ = tool.Execute(ctx, args)
	}
}
