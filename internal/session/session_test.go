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

// Closes the writer on cleanup since Windows cannot remove open files
func newTestWriter(t *testing.T, id string) *Writer {
	t.Helper()
	w := NewWriter(&State{ID: id, ProjectPath: "/tmp/proj", Model: "m1", Mode: "normal"})
	t.Cleanup(func() { w.Close("normal") })
	return w
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
	want := filepath.Join(home, ".agent-gopher", "projects", "tmp-proj", "abc.jsonl")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestUnarmedWriterNeverCreatesAFile(t *testing.T) {
	home := isolateHome(t)
	w := newTestWriter(t, "unarmed")
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
	if _, err := os.Stat(filepath.Join(home, ".agent-gopher", "projects")); !os.IsNotExist(err) {
		t.Fatalf("session root exists for a writer never armed: %v", err)
	}
}

func TestArmingFlushesWhatWasBufferedBeforeIt(t *testing.T) {
	isolateHome(t)
	w := newTestWriter(t, "buffered")
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
	w := newTestWriter(t, "appendonly")
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
	w := newTestWriter(t, "empty")
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
	w := newTestWriter(t, "roundtrip")
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
	w := newTestWriter(t, "torn")
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
	w := newTestWriter(t, "closed")
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
	w := newTestWriter(t, "concurrent")
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

func TestReopenAppendsToTheSameLog(t *testing.T) {
	isolateHome(t)
	w := newTestWriter(t, "resumable")
	w.Arm()
	w.AppendEntry("user", "hello", time.Time{})
	if err := w.Close("plan"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r := Reopen(w.Path())
	r.AppendEntry("user", "again", time.Time{})
	if err := r.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	s, err := Load(w.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ID != "resumable" || len(s.Entries) != 2 || s.Entries[1].Content != "again" {
		t.Fatalf("reopened log = %+v", s)
	}
	if s.Complete {
		t.Fatal("Complete = true with records after the old end record")
	}
	if err := r.Close("normal"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, _ := os.ReadFile(w.Path())
	if bytes.Count(data, []byte(`"type":"meta"`)) != 1 {
		t.Fatal("reopening wrote a second meta line")
	}
	if s, _ := Load(w.Path()); !s.Complete || s.Mode != "normal" {
		t.Fatalf("after second close: complete %v, mode %q", s.Complete, s.Mode)
	}
}

func TestReopenStartsAfterATornLine(t *testing.T) {
	isolateHome(t)
	w := newTestWriter(t, "tornresume")
	w.Arm()
	w.AppendEntry("user", "hello", time.Time{})
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	f, _ := os.OpenFile(w.Path(), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"entry","role":"assis`)
	f.Close()

	r := Reopen(w.Path())
	t.Cleanup(func() { r.Close("normal") })
	r.AppendEntry("user", "after crash", time.Time{})
	if err := r.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	s, err := Load(w.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Entries) != 2 || s.Entries[1].Content != "after crash" {
		t.Fatalf("entry after the torn line was lost: %+v", s.Entries)
	}
}

func TestFindByPrefixExactMatch(t *testing.T) {
	isolateHome(t)
	w := newTestWriter(t, "findme12345")
	w.Arm()
	w.AppendEntry("assistant", "hi", time.Time{})
	if err := w.Close("normal"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	path, err := FindByPrefix("findme12345")
	if err != nil {
		t.Fatalf("FindByPrefix exact: %v", err)
	}
	if path != w.Path() {
		t.Fatalf("got %q, want %q", path, w.Path())
	}
}

func TestFindByPrefixPartialMatch(t *testing.T) {
	isolateHome(t)
	w := newTestWriter(t, "uniqueprefix999")
	w.Arm()
	w.AppendEntry("assistant", "hi", time.Time{})
	if err := w.Close("normal"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	path, err := FindByPrefix("uniqueprefix")
	if err != nil {
		t.Fatalf("FindByPrefix prefix: %v", err)
	}
	if path != w.Path() {
		t.Fatalf("got %q, want %q", path, w.Path())
	}
}

func TestFindByPrefixNoMatch(t *testing.T) {
	isolateHome(t)
	_, err := FindByPrefix("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestFindByPrefixAmbiguous(t *testing.T) {
	isolateHome(t)
	for _, id := range []string{"ambig_aaa", "ambig_bbb"} {
		w := newTestWriter(t, id)
		w.Arm()
		w.AppendEntry("assistant", "hi", time.Time{})
		if err := w.Close("normal"); err != nil {
			t.Fatalf("Close %s: %v", id, err)
		}
	}
	_, err := FindByPrefix("ambig_")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %q, want ambiguous", err.Error())
	}
}

func TestResetDropsEarlierMessages(t *testing.T) {
	isolateHome(t)
	w := newTestWriter(t, "compacted")
	w.Arm()
	w.AppendMessage(json.RawMessage(`{"role":"user","content":"old"}`))
	w.AppendReset()
	w.AppendMessage(json.RawMessage(`{"role":"user","content":"summary"}`))
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	s, err := Load(Path("compacted"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Messages) != 1 || !strings.Contains(string(s.Messages[0]), "summary") {
		t.Fatalf("messages = %s, want only the summary", s.Messages)
	}
}
