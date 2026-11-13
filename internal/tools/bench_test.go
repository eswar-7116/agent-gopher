package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// A write/edit tool call diffs the whole file before asking for approval;
// this is the cost paid on every such prompt for a realistically large file.
func BenchmarkUnifiedDiff_5000Lines(b *testing.B) {
	var before, after strings.Builder
	for i := range 5000 {
		fmt.Fprintf(&before, "line %d\n", i)
		if i%100 == 0 {
			fmt.Fprintf(&after, "edited %d\n", i)
		} else {
			fmt.Fprintf(&after, "line %d\n", i)
		}
	}
	beforeS, afterS := before.String(), after.String()
	for b.Loop() {
		UnifiedDiff("f", beforeS, afterS)
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
