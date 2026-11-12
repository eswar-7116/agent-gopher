package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestNewIDUsesNanoIDAlphabet(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		if len(id) != idLength {
			t.Fatalf("id %q: got length %d, want %d", id, len(id), idLength)
		}
		for _, r := range id {
			if !strings.ContainsRune(idAlphabet, r) {
				t.Fatalf("id %q contains %q, outside the alphabet", id, r)
			}
		}
		if seen[id] {
			t.Fatalf("NewID returned duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestDirUsesNormalizedProjectPath(t *testing.T) {
	home := isolateHome(t)
	dir := Dir("/home/eswar/agent-gopher")
	want := filepath.Join(home, RootDirName, "home-eswar-agent-gopher")
	if dir != want {
		t.Fatalf("Dir = %q, want %q", dir, want)
	}
}

func TestSaveWritesStateAndLoadReadsItBack(t *testing.T) {
	isolateHome(t)
	s := &State{
		ID:           "abc123",
		ProjectPath:  "/tmp/proj",
		Model:        "some-model",
		Mode:         "normal",
		Entries:      []Entry{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}},
		InputHistory: []string{"hi"},
		Messages:     json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}

	path, err := Save(s)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := filepath.Base(path); got != "abc123"+Ext {
		t.Fatalf("file name = %q, want %q", got, "abc123"+Ext)
	}
	if s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() {
		t.Fatal("Save did not stamp CreatedAt/UpdatedAt")
	}
	if s.Version != Version {
		t.Fatalf("Version = %d, want %d", s.Version, Version)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != s.ID || got.Model != s.Model || got.Mode != s.Mode {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got.Entries) != 2 || got.Entries[1].Content != "hello" {
		t.Fatalf("entries = %+v", got.Entries)
	}
	if len(got.InputHistory) != 1 || got.InputHistory[0] != "hi" {
		t.Fatalf("input history = %+v", got.InputHistory)
	}
	var msgs []map[string]any
	if err := json.Unmarshal(got.Messages, &msgs); err != nil {
		t.Fatalf("messages are not valid JSON: %v", err)
	}
	if len(msgs) != 1 || msgs[0]["content"] != "hi" {
		t.Fatalf("messages = %+v", msgs)
	}
}

func TestSaveTwiceRewritesTheSameFile(t *testing.T) {
	isolateHome(t)
	s := &State{ID: "sameid", ProjectPath: "/tmp/proj", Entries: []Entry{{Role: "user", Content: "one"}}}

	first, err := Save(s)
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	created := s.CreatedAt

	s.Entries = append(s.Entries, Entry{Role: "user", Content: "two"})
	second, err := Save(s)
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if first != second {
		t.Fatalf("second save went to %q, want %q", second, first)
	}
	if !s.CreatedAt.Equal(created) {
		t.Fatal("second save overwrote CreatedAt")
	}

	entries, err := os.ReadDir(filepath.Dir(first))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d files in the session dir, want 1: %v", len(entries), entries)
	}

	got, err := Load(first)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("reloaded %d entries, want 2", len(got.Entries))
	}
}

func TestSaveRejectsStateWithoutID(t *testing.T) {
	isolateHome(t)
	if _, err := Save(&State{ProjectPath: "/tmp/proj"}); err == nil {
		t.Fatal("Save with no id: want error, got nil")
	}
	if _, err := Save(nil); err == nil {
		t.Fatal("Save(nil): want error, got nil")
	}
}

func TestListReturnsNewestFirstAndSkipsJunk(t *testing.T) {
	isolateHome(t)
	proj := "/tmp/proj"

	older := &State{ID: "older", ProjectPath: proj}
	if _, err := Save(older); err != nil {
		t.Fatalf("Save older: %v", err)
	}
	// Save() stamps UpdatedAt with time.Now(); force a gap the sort can see.
	older.UpdatedAt = time.Now().Add(-time.Hour)
	data, _ := json.Marshal(older)
	if err := os.WriteFile(Path(proj, "older"), data, 0o600); err != nil {
		t.Fatalf("rewrite older: %v", err)
	}

	if _, err := Save(&State{ID: "newer", ProjectPath: proj}); err != nil {
		t.Fatalf("Save newer: %v", err)
	}

	dir := Dir(proj)
	if err := os.WriteFile(filepath.Join(dir, "broken"+Ext), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write broken: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	list, err := List(proj)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(list), list)
	}
	if list[0].ID != "newer" || list[1].ID != "older" {
		t.Fatalf("order = %q, %q; want newer, older", list[0].ID, list[1].ID)
	}
}

func TestListOnMissingDirIsEmpty(t *testing.T) {
	isolateHome(t)
	list, err := List("/tmp/never-used")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("got %d sessions, want 0", len(list))
	}
}

func TestSaveLeavesNoTempFilesBehind(t *testing.T) {
	isolateHome(t)
	path, err := Save(&State{ID: "clean", ProjectPath: "/tmp/proj"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
