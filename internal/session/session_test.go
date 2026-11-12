package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eswar-7116/agent-gopher/internal/paths"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	paths.InitWithBase(home, "/tmp/proj")
	t.Cleanup(func() { paths.InitWithBase("", "") })
	return home
}

func newTestWriter(id string) *Writer {
	return NewWriter(&State{ID: id, ProjectPath: "/tmp/proj", Model: "m1", Mode: "normal"})
}

func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line is not JSON: %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
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

func TestPathUsesNormalizedProjectDirAndJSONL(t *testing.T) {
	home := isolateHome(t)
	got := Path("abc")
	want := filepath.Join(home, ".agent-memory", "tmp-proj", "abc.jsonl")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestUnarmedWriterNeverCreatesAFile(t *testing.T) {
	home := isolateHome(t)
	w := newTestWriter("unarmed")
	w.AppendEntry("info", "welcome", time.Time{})
	w.AppendInput("/plan")
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := w.Close("plan"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if w.Created() {
		t.Fatal("Created() = true for a writer never armed")
	}
	if _, err := os.Stat(filepath.Join(home, ".agent-memory")); !os.IsNotExist(err) {
		t.Fatalf("session root exists for a writer never armed: %v", err)
	}
}

func TestArmingFlushesWhatWasBufferedBeforeIt(t *testing.T) {
	isolateHome(t)
	w := newTestWriter("buffered")
	w.AppendEntry("user", "hello", time.Time{})
	if err := w.Flush(); err != nil { // not armed yet
		t.Fatalf("Flush: %v", err)
	}
	w.AppendEntry("assistant", "hi", time.Time{})
	w.Arm()
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	lines := readLines(t, w.Path())
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want meta + 2 entries", len(lines))
	}
	if lines[0]["type"] != "meta" || lines[0]["id"] != "buffered" {
		t.Fatalf("first line is not this session's meta: %v", lines[0])
	}
	if lines[1]["content"] != "hello" || lines[2]["content"] != "hi" {
		t.Fatalf("entries out of order: %v, %v", lines[1], lines[2])
	}
}

func TestFlushOnlyAppends(t *testing.T) {
	isolateHome(t)
	w := newTestWriter("appendonly")
	w.Arm()
	w.AppendEntry("assistant", "one", time.Time{})
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	before, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	w.AppendEntry("assistant", "two", time.Time{})
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	after, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.HasPrefix(after, before) {
		t.Fatal("second flush rewrote earlier bytes instead of appending")
	}
	if n := bytes.Count(after[len(before):], []byte("\n")); n != 1 {
		t.Fatalf("second flush appended %d lines, want 1", n)
	}
	if bytes.Count(after, []byte(`"type":"meta"`)) != 1 {
		t.Fatal("meta written more than once")
	}
}

func TestEmptyFlushWritesNothing(t *testing.T) {
	isolateHome(t)
	w := newTestWriter("empty")
	w.Arm()
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if w.Created() {
		t.Fatal("an armed flush with nothing queued created the file")
	}
}

func TestLoadFoldsTheLogBack(t *testing.T) {
	isolateHome(t)
	w := newTestWriter("roundtrip")
	w.Arm()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	w.AppendEntry("user", "hello", at)
	w.AppendInput("hello")
	w.AppendInput("/plan")
	w.AppendMessage(json.RawMessage(`{"role":"user","content":"hello"}`))
	w.AppendEntry("assistant", "hi there", time.Time{})
	w.AppendMessage(json.RawMessage(`{"role":"assistant","content":"hi there"}`))
	if err := w.Close("plan"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s, err := Load(w.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ID != "roundtrip" || s.ProjectPath != "/tmp/proj" || s.Model != "m1" || s.Version != Version {
		t.Fatalf("meta not folded: %+v", s)
	}
	if s.Mode != "plan" {
		t.Fatalf("Mode = %q, want the final mode from the end record", s.Mode)
	}
	if !s.Complete {
		t.Fatal("Complete = false after Close")
	}
	if s.CreatedAt.IsZero() || s.UpdatedAt.Before(s.CreatedAt) {
		t.Fatalf("timestamps: created %v, updated %v", s.CreatedAt, s.UpdatedAt)
	}
	if len(s.Entries) != 2 || s.Entries[1].Content != "hi there" {
		t.Fatalf("entries = %+v", s.Entries)
	}
	if !s.Entries[0].At.Equal(at) {
		t.Fatalf("entry time = %v, want the time it was appended with, %v", s.Entries[0].At, at)
	}
	if strings.Join(s.InputHistory, "|") != "hello|/plan" {
		t.Fatalf("input history = %v", s.InputHistory)
	}
	if len(s.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(s.Messages))
	}
	var m map[string]string
	if err := json.Unmarshal(s.Messages[1], &m); err != nil || m["content"] != "hi there" {
		t.Fatalf("message 1 = %s (%v)", s.Messages[1], err)
	}
}

func TestLoadSurvivesATornLastLine(t *testing.T) {
	isolateHome(t)
	w := newTestWriter("torn")
	w.Arm()
	w.AppendEntry("user", "hello", time.Time{})
	w.AppendEntry("assistant", "hi", time.Time{})
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Simulate a kill mid-write
	f, err := os.OpenFile(w.Path(), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	f.WriteString(`{"type":"entry","role":"assis`)
	f.Close()

	s, err := Load(w.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Entries) != 2 {
		t.Fatalf("got %d entries, want the 2 before the tear", len(s.Entries))
	}
	if s.Complete {
		t.Fatal("Complete = true for a log that never got its end record")
	}
}

func TestCloseIsFinal(t *testing.T) {
	isolateHome(t)
	w := newTestWriter("closed")
	w.Arm()
	w.AppendEntry("assistant", "hi", time.Time{})
	if err := w.Close("normal"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := w.Close("normal"); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	before, _ := os.ReadFile(w.Path())

	w.AppendEntry("assistant", "after close", time.Time{})
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush after Close: %v", err)
	}
	after, _ := os.ReadFile(w.Path())
	if !bytes.Equal(before, after) {
		t.Fatal("writing after Close changed the log")
	}
	if bytes.Count(after, []byte(`"type":"end"`)) != 1 {
		t.Fatal("end record written more or less than once")
	}
}

func TestConcurrentAppendAndFlush(t *testing.T) {
	isolateHome(t)
	w := newTestWriter("concurrent")
	w.Arm()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				w.AppendEntry("assistant", strings.Repeat("x", i+1), time.Time{})
				if err := w.Flush(); err != nil {
					t.Errorf("Flush: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	s, err := Load(w.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Entries) != 8*25 {
		t.Fatalf("got %d entries, want %d", len(s.Entries), 8*25)
	}
}

func TestListSummarizesNewestFirstAndSkipsJunk(t *testing.T) {
	isolateHome(t)
	for _, id := range []string{"older", "newer"} {
		w := newTestWriter(id)
		w.Arm()
		w.AppendEntry("assistant", "hi", time.Time{})
		if err := w.Close("normal"); err != nil {
			t.Fatalf("Close %s: %v", id, err)
		}
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(Path("older"), old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	dir := paths.SessionsDir
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600)
	os.WriteFile(filepath.Join(dir, "old-format.json"), []byte("{}"), 0o600)

	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(list), list)
	}
	if list[0].ID != "newer" || list[1].ID != "older" {
		t.Fatalf("order = %q, %q; want newer, older", list[0].ID, list[1].ID)
	}
	if list[0].Model != "m1" || list[0].ProjectPath != "/tmp/proj" || list[0].CreatedAt.IsZero() {
		t.Fatalf("summary not read from meta: %+v", list[0])
	}
}

func TestListOnMissingDirIsEmpty(t *testing.T) {
	paths.InitWithBase(t.TempDir(), "/tmp/never-used")
	t.Cleanup(func() { paths.InitWithBase("", "") })
	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("got %d sessions, want 0", len(list))
	}
}
