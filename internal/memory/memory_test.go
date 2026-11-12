package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddListRemove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	if got := List(dir); got != nil {
		t.Fatalf("expected none for missing dir, got %v", got)
	}
	if Content(dir, ScopeProject) != "" {
		t.Fatal("expected empty content")
	}

	for _, fact := range []string{
		"Tests use testify",
		"Handlers use service/repository layering",
		"Never modify generated/*.go",
	} {
		if added, err := Add(dir, fact); err != nil || !added {
			t.Fatalf("Add(%q): added=%v err=%v", fact, added, err)
		}
	}
	if added, _ := Add(dir, "tests use TESTIFY"); added {
		t.Error("duplicate (case-insensitive) should not be added")
	}
	if _, err := Add(dir, "  "); err == nil {
		t.Error("expected error for empty fact")
	}

	got := List(dir)
	want := []string{
		"Tests use testify",
		"Handlers use service/repository layering",
		"Never modify generated/*.go",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %v want %v", i, got[i], want[i])
		}
	}
	if c := Content(dir, ScopeProject); !strings.Contains(c, "Project memory") || !strings.Contains(c, "- Tests use testify") {
		t.Errorf("bad content: %q", c)
	}

	if _, err := Remove(dir, "e"); err == nil || !strings.Contains(err.Error(), "match") {
		t.Errorf("expected ambiguity error, got %v", err)
	}
	if _, err := Remove(dir, "nope"); err == nil {
		t.Error("expected no-match error")
	}
	if _, err := Remove(dir, "9"); err == nil {
		t.Error("expected out-of-range error")
	}
	if fact, err := Remove(dir, "testify"); err != nil || fact != "Tests use testify" {
		t.Fatalf("Remove by text: %q %v", fact, err)
	}
	if fact, err := Remove(dir, "1"); err != nil || fact != "Handlers use service/repository layering" {
		t.Fatalf("Remove by number: %q %v", fact, err)
	}
	if got := List(dir); len(got) != 1 || got[0] != "Never modify generated/*.go" {
		t.Errorf("after removes: %v", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, memoryFileName))
	if !strings.HasPrefix(string(data), "# Memory\n") {
		t.Errorf("heading lost: %q", data)
	}
}

func TestListReadsOnlyFactsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("- not stored memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := List(dir); len(got) != 0 {
		t.Fatalf("unexpected facts from another markdown file: %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, memoryFileName), []byte("# Memory\n\n- keep only this fact\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := List(dir); len(got) != 1 || got[0] != "keep only this fact" {
		t.Fatalf("unexpected facts: %v", got)
	}
}

func TestTool(t *testing.T) {
	globalDir := filepath.Join(t.TempDir(), "global")
	projectDir := filepath.Join(t.TempDir(), "project")
	tool := Tool{GlobalDir: globalDir, ProjectDir: projectDir}
	args := map[string]any{"scope": ScopeGlobal, "fact": "Use Bun for PostgreSQL"}
	if r, err := tool.Execute(context.Background(), args); err != nil || r != "Saved to memory." {
		t.Fatalf("got %v %v", r, err)
	}
	if r, _ := tool.Execute(context.Background(), args); r != "Already in memory." {
		t.Errorf("got %v", r)
	}
	args["scope"] = ScopeProject
	if r, err := tool.Execute(context.Background(), args); err != nil || r != "Saved to memory." {
		t.Fatalf("project scope got %v %v", r, err)
	}
	if len(List(globalDir)) != 1 || len(List(projectDir)) != 1 {
		t.Fatal("tool did not keep scopes separate")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"fact": "missing scope"}); err == nil {
		t.Error("expected error for missing scope")
	}
}

func TestMemoryDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wantRoot := filepath.Join(home, configDirName)
	if got := GlobalDir(); got != wantRoot {
		t.Fatalf("GlobalDir() = %q, want %q", got, wantRoot)
	}
	for _, test := range []struct {
		projectPath string
		wantName    string
	}{
		{`C:\Users\eswar\MyProj`, "c-users-eswar-myproj"},
		{"/home/eswar/MyProj", "home-eswar-myproj"},
	} {
		if got := NormalizeProjectPath(test.projectPath); got != test.wantName {
			t.Errorf("NormalizeProjectPath(%q) = %q, want %q", test.projectPath, got, test.wantName)
		}
		if got, want := ProjectDir(test.projectPath), filepath.Join(wantRoot, test.wantName); got != want {
			t.Errorf("ProjectDir(%q) = %q, want %q", test.projectPath, got, want)
		}
	}
}
