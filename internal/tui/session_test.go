package tui

import (
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/session"
	"github.com/openai/openai-go/v3"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func testModel(t *testing.T) *Model {
	t.Helper()
	client := openai.Client{}
	ag := agent.NewAgent(agent.AgentOptions{
		Client: &client,
		Model:  "test-model",
		Logger: log.New(io.Discard, "", 0),
	})
	return &Model{agent: ag, sessionID: "testsession0001", projectPath: "/tmp/proj"}
}

func TestRoleNamesCoverEveryRole(t *testing.T) {
	for r := roleUser; r <= roleInfo; r++ {
		if _, ok := roleNames[r]; !ok {
			t.Fatalf("role %d has no persisted name", r)
		}
	}
}

func TestSnapshotCapturesTranscriptAndHistory(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.entries = []entry{
		{roleInfo, "welcome"},
		{roleUser, "hello"},
		{roleAssistant, "hi there"},
		{roleTool, "read_file {}"},
		{roleError, "boom"},
	}
	m.history = []string{"hello"}

	s := m.snapshot()
	if s.ID != "testsession0001" || s.ProjectPath != "/tmp/proj" {
		t.Fatalf("identity not captured: %+v", s)
	}
	if s.Model != "test-model" || s.Mode != "normal" {
		t.Fatalf("model/mode = %q/%q", s.Model, s.Mode)
	}
	wantRoles := []string{"info", "user", "assistant", "tool", "error"}
	if len(s.Entries) != len(wantRoles) {
		t.Fatalf("got %d entries, want %d", len(s.Entries), len(wantRoles))
	}
	for i, want := range wantRoles {
		if s.Entries[i].Role != want {
			t.Fatalf("entry %d role = %q, want %q", i, s.Entries[i].Role, want)
		}
	}
	if s.Entries[2].Content != "hi there" {
		t.Fatalf("content = %q", s.Entries[2].Content)
	}
	if len(s.InputHistory) != 1 || s.InputHistory[0] != "hello" {
		t.Fatalf("input history = %+v", s.InputHistory)
	}
}

func TestSnapshotCopiesSoLaterAppendsDoNotLeak(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.entries = []entry{{roleUser, "one"}}
	m.history = []string{"one"}

	s := m.snapshot()
	m.add(roleUser, "two")
	m.history = append(m.history, "two")

	if len(s.Entries) != 1 || len(s.InputHistory) != 1 {
		t.Fatalf("snapshot aliased live state: %+v", s)
	}
}

func TestSaveSessionWritesUnderProjectDir(t *testing.T) {
	home := isolateHome(t)
	m := testModel(t)
	m.entries = []entry{{roleUser, "hello"}}
	m.history = []string{"hello"}

	path, err := m.saveSession()
	if err != nil {
		t.Fatalf("saveSession: %v", err)
	}
	want := filepath.Join(home, session.RootDirName, "tmp-proj", "testsession0001"+session.Ext)
	if path != want {
		t.Fatalf("saved to %q, want %q", path, want)
	}

	got, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Content != "hello" {
		t.Fatalf("saved entries = %+v", got.Entries)
	}
}

func TestSaveSessionSkipsAnEmptyConversation(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	// Only the opening banner, as on a session the user never typed into.
	m.entries = []entry{{roleInfo, "welcome"}}

	path, err := m.saveSession()
	if err != nil {
		t.Fatalf("saveSession: %v", err)
	}
	if path != "" {
		t.Fatalf("wrote %q for an empty conversation, want no file", path)
	}
}

func TestSaveSessionSkipsWhenNoIDCouldBeGenerated(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.sessionID = ""
	m.entries = []entry{{roleUser, "hello"}}

	path, err := m.saveSession()
	if err != nil || path != "" {
		t.Fatalf("saveSession = (%q, %v), want (\"\", nil)", path, err)
	}
}

func TestSaveActiveSessionWithNoRunningTUI(t *testing.T) {
	setActive(nil)
	path, err := SaveActiveSession()
	if err != nil || path != "" {
		t.Fatalf("SaveActiveSession = (%q, %v), want (\"\", nil)", path, err)
	}
}

func TestSaveActiveSessionSavesTheRegisteredModel(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.entries = []entry{{roleUser, "hello"}}
	setActive(m)
	t.Cleanup(func() { setActive(nil) })

	path, err := SaveActiveSession()
	if err != nil {
		t.Fatalf("SaveActiveSession: %v", err)
	}
	if path == "" {
		t.Fatal("SaveActiveSession wrote nothing")
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != m.sessionID {
		t.Fatalf("saved id = %q, want %q", got.ID, m.sessionID)
	}
}

func TestNewSessionIDIsUsable(t *testing.T) {
	if id := newSessionID(); id == "" {
		t.Fatal("newSessionID returned an empty id")
	}
}
