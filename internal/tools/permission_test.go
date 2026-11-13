package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Answers permission prompts with choice and counts them
func stubPrompt(t *testing.T, choice string) *[]PermissionRequest {
	var seen []PermissionRequest
	PromptPermission = func(req PermissionRequest) (string, string) {
		seen = append(seen, req)
		return choice, ""
	}
	t.Cleanup(func() { PromptPermission = nil })
	return &seen
}

func TestWhitelistRefusesChainedCommands(t *testing.T) {
	wl := map[string]bool{"git status": true, "echo": true}
	for _, cmd := range []string{
		"git status; rm -rf ~",
		"git status && rm x",
		"git status || rm x",
		"echo hi | sh",
		"echo $(rm x)",
		"echo `rm x`",
		"echo hi > ~/.bashrc",
		"echo hi < /etc/passwd",
		"echo hi & rm x",
		"echo hi\nrm x",
	} {
		if isWhitelisted(cmd, wl) {
			t.Errorf("isWhitelisted(%q) = true, want false", cmd)
		}
	}
	if !isWhitelisted("git status -s", wl) {
		t.Error("plain whitelisted command was refused")
	}
}

func TestReadOnlyCommand(t *testing.T) {
	ok := []string{
		"ls -la",
		"grep -rn foo .",
		"rg foo | head -20",
		"find . -name '*.go'",
		"git status",
		"git log --oneline -5",
		"git diff HEAD~1",
		"cat main.go | wc -l",
	}
	bad := []string{
		"rm -rf x",
		"ls; rm x",
		"ls || rm x",
		"cat x > y",
		"find . -delete",
		"find . -exec rm {} ;",
		"git commit -m x",
		"git diff --output=x",
		"sort -o out in",
		"echo $(rm x)",
		"ls | sh",
		"",
	}
	for _, c := range ok {
		if !ReadOnlyCommand(c) {
			t.Errorf("ReadOnlyCommand(%q) = false, want true", c)
		}
	}
	for _, c := range bad {
		if ReadOnlyCommand(c) {
			t.Errorf("ReadOnlyCommand(%q) = true, want false", c)
		}
	}
}

func TestAlwaysAllowAppliesImmediatelyAndOnce(t *testing.T) {
	seen := stubPrompt(t, "a")
	var saved []string
	tool := ShellCmdTool{Whitelist: NewWhitelist(nil, func(c string) { saved = append(saved, c) })}

	for range 2 {
		if _, err := tool.Execute(t.Context(), map[string]any{"cmd": "echo hi"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(*seen) != 1 {
		t.Errorf("prompted %d times, want 1", len(*seen))
	}
	if len(saved) != 1 || saved[0] != "echo" {
		t.Errorf("saved = %v, want [echo]", saved)
	}
}

func TestShellErrorKeepsOutput(t *testing.T) {
	out, err := ShellCmdTool{Permissive: true}.Execute(t.Context(), map[string]any{"cmd": "echo boom; exit 3"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out.(string), "boom") {
		t.Errorf("output = %q, want it to contain boom", out)
	}
}

func TestWriteGatePromptsWithDiff(t *testing.T) {
	seen := stubPrompt(t, "n")
	path := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(path, []byte("old\n"), 0o644)

	tool := WriteFileTool{Gate: NewWriteGate(false)}
	_, err := tool.Execute(context.Background(), map[string]any{"filepath": path, "contents": "new\n"})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err = %v, want denial", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "old\n" {
		t.Errorf("file changed despite denial: %q", data)
	}
	if len(*seen) != 1 || (*seen)[0].Kind != PermWrite || !strings.Contains((*seen)[0].Diff, "-old\n+new") {
		t.Errorf("unexpected request: %+v", *seen)
	}
}

func TestWriteGateAllowAll(t *testing.T) {
	seen := stubPrompt(t, "a")
	dir := t.TempDir()
	gate := NewWriteGate(false)
	tool := WriteFileTool{Gate: gate}
	for _, n := range []string{"a", "b"} {
		if _, err := tool.Execute(t.Context(), map[string]any{"filepath": filepath.Join(dir, n), "contents": "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(*seen) != 1 {
		t.Errorf("prompted %d times, want 1", len(*seen))
	}
}

func TestPermissiveWriteSkipsPrompt(t *testing.T) {
	seen := stubPrompt(t, "n")
	tool := WriteFileTool{Gate: NewWriteGate(true)}
	if _, err := tool.Execute(t.Context(), map[string]any{"filepath": filepath.Join(t.TempDir(), "f"), "contents": "x"}); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 0 {
		t.Errorf("prompted %d times, want 0", len(*seen))
	}
}

func TestEditFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.go")
	write := func(s string) { os.WriteFile(path, []byte(s), 0o640) }
	read := func() string { b, _ := os.ReadFile(path); return string(b) }
	edit := func(args map[string]any) error {
		args["filepath"] = path
		_, err := EditFileTool{}.Execute(t.Context(), args)
		return err
	}

	write("a\nb\na\n")
	if err := edit(map[string]any{"old_string": "a", "new_string": "x"}); err == nil || !strings.Contains(err.Error(), "matches 2") {
		t.Errorf("ambiguous edit err = %v", err)
	}
	if err := edit(map[string]any{"old_string": "zzz", "new_string": "x"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing edit err = %v", err)
	}
	if err := edit(map[string]any{"old_string": "b", "new_string": "B"}); err != nil || read() != "a\nB\na\n" {
		t.Errorf("unique edit: err=%v content=%q", err, read())
	}
	if err := edit(map[string]any{"old_string": "a", "new_string": "x", "replace_all": true}); err != nil || read() != "x\nB\nx\n" {
		t.Errorf("replace_all: err=%v content=%q", err, read())
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", st.Mode().Perm())
	}
}

func TestEditFileDeniedLeavesFile(t *testing.T) {
	stubPrompt(t, "n")
	path := filepath.Join(t.TempDir(), "f")
	os.WriteFile(path, []byte("keep"), 0o644)
	_, err := EditFileTool{Gate: NewWriteGate(false)}.Execute(t.Context(), map[string]any{"filepath": path, "old_string": "keep", "new_string": "lose"})
	if err == nil {
		t.Fatal("expected denial")
	}
	if b, _ := os.ReadFile(path); string(b) != "keep" {
		t.Errorf("file changed: %q", b)
	}
}

func TestUnifiedDiff(t *testing.T) {
	before := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	after := "1\n2\n3\n4\nfive\n6\n7\n8\n9\n10\n"
	want := "--- a/f\n+++ b/f\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8"
	if got := UnifiedDiff("f", before, after); got != want {
		t.Errorf("diff =\n%s\nwant\n%s", got, want)
	}
	if UnifiedDiff("f", "same", "same") != "" {
		t.Error("expected empty diff for identical input")
	}
	if a, r := DiffStat(UnifiedDiff("f", "", "x\ny\n")); a != 2 || r != 0 {
		t.Errorf("new file stat = +%d -%d, want +2 -0", a, r)
	}
}

func TestUnifiedDiffSeparateHunks(t *testing.T) {
	var before, after strings.Builder
	for i := range 30 {
		line := strings.Repeat("x", i+1) + "\n"
		before.WriteString(line)
		if i == 2 || i == 25 {
			line = "changed\n"
		}
		after.WriteString(line)
	}
	d := UnifiedDiff("f", before.String(), after.String())
	if n := strings.Count(d, "@@ -"); n != 2 {
		t.Errorf("got %d hunks, want 2:\n%s", n, d)
	}
	if !strings.Contains(d, "@@ -23,7 +23,7 @@") {
		t.Errorf("second hunk header wrong:\n%s", d)
	}
}

func TestSanitizeToolName(t *testing.T) {
	cases := map[string]string{
		"srv__ok-name_1":        "srv__ok-name_1",
		"my server__a.b":        "my_server__a_b",
		strings.Repeat("a", 80): strings.Repeat("a", 64),
		"":                      "mcp_tool",
	}
	for in, want := range cases {
		if got := SanitizeToolName(in); got != want {
			t.Errorf("SanitizeToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnifiedDiffLargeFileStaysMinimal(t *testing.T) {
	var before, after strings.Builder
	for i := range 5000 {
		fmt.Fprintf(&before, "line %d\n", i)
		if i%100 == 0 {
			fmt.Fprintf(&after, "edited %d\n", i)
		} else {
			fmt.Fprintf(&after, "line %d\n", i)
		}
	}
	a, r := DiffStat(UnifiedDiff("f", before.String(), after.String()))
	if a != 50 || r != 50 {
		t.Errorf("stat = +%d -%d, want +50 -50", a, r)
	}
}
