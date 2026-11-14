package tui

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/paths"
	"github.com/eswar-7116/agent-gopher/internal/session"
	"github.com/openai/openai-go/v3"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	paths.InitWithBase(home, "/tmp/proj")
	t.Cleanup(func() { paths.InitWithBase("", "") })
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
	m := &Model{agent: ag}
	w := session.NewWriter(&session.State{ID: "testsession0001", ProjectPath: "/tmp/proj", Model: ag.Model()})
	// Windows cannot remove open files
	t.Cleanup(func() { w.Close("normal") })
	m.log = w
	return m
}

// Streams a reply the way Update does
func streamReply(m *Model, deltas ...string) {
	for i, d := range deltas {
		if i == 0 {
			m.add(roleAssistant, d)
		} else {
			m.appendToLast(roleAssistant, d)
		}
	}
}

func TestRoleNamesCoverEveryRole(t *testing.T) {
	for r := roleUser; r <= roleDiff; r++ {
		if _, ok := roleNames[r]; !ok {
			t.Fatalf("role %d has no persisted name", r)
		}
	}
}

func TestNothingIsWrittenUntilTheAgentReplies(t *testing.T) {
	cases := []struct {
		name string
		run  func(m *Model)
	}{
		{"only the opening banner", func(m *Model) {
			m.add(roleInfo, "welcome")
		}},
		{"only slash commands", func(m *Model) {
			m.add(roleInfo, "welcome")
			m.log.AppendInput("/plan")
			m.add(roleInfo, "Switched to plan mode.")
		}},
		{"asked, but the reply failed", func(m *Model) {
			m.add(roleUser, "hello")
			m.add(roleError, "chat completion failed")
		}},
		{"tool ran but the agent never spoke", func(m *Model) {
			m.add(roleUser, "read the file")
			m.add(roleTool, "read_file {}")
			m.add(roleError, "chat completion failed")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateHome(t)
			m := testModel(t)
			tc.run(m)

			if path, err := m.flushSession(); err != nil || path != "" {
				t.Fatalf("flushSession = (%q, %v), want nothing written", path, err)
			}
			if path, err := m.closeSession(); err != nil || path != "" {
				t.Fatalf("closeSession = (%q, %v), want nothing written", path, err)
			}
			if _, err := os.Stat(filepath.Join(home, ".agent-gopher", "projects")); !os.IsNotExist(err) {
				t.Fatalf("session root was created anyway: %v", err)
			}
		})
	}
}

func TestReplyWritesTheWholeSessionIncludingWhatCameBefore(t *testing.T) {
	home := isolateHome(t)
	m := testModel(t)
	m.add(roleInfo, "welcome")
	m.add(roleUser, "hello")
	streamReply(m, "hi", " there")

	path, err := m.flushSession()
	if err != nil {
		t.Fatalf("flushSession: %v", err)
	}
	want := filepath.Join(home, ".agent-gopher", "projects", "tmp-proj", "testsession0001"+session.Ext)
	if path != want {
		t.Fatalf("wrote %q, want %q", path, want)
	}

	s, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var roles []string
	for _, e := range s.Entries {
		roles = append(roles, e.Role)
	}
	if got := len(roles); got != 3 || roles[0] != "info" || roles[1] != "user" || roles[2] != "assistant" {
		t.Fatalf("roles = %v, want info, user, assistant", roles)
	}
	if s.Entries[2].Content != "hi there" {
		t.Fatalf("streamed reply logged as %q, want the whole reply", s.Entries[2].Content)
	}
}

func TestStreamedReplyIsLoggedOnceWhenSealedByTheNextEntry(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.add(roleUser, "do it")
	streamReply(m, "Running ", "the tool.")
	m.add(roleTool, "shell_cmd ls") // seals the reply
	streamReply(m, "Done.")

	path, err := m.closeSession()
	if err != nil {
		t.Fatalf("closeSession: %v", err)
	}
	s, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var got []string
	for _, e := range s.Entries {
		got = append(got, e.Role+":"+e.Content)
	}
	want := []string{"user:do it", "assistant:Running the tool.", "tool:shell_cmd ls", "assistant:Done."}
	if len(got) != len(want) {
		t.Fatalf("entries = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries = %q, want %q", got, want)
		}
	}
	if !s.Complete {
		t.Fatal("Complete = false after closeSession")
	}
}

func TestRepeatedFlushesDoNotDuplicate(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.add(roleUser, "one")
	streamReply(m, "reply one")
	path, err := m.flushSession()
	if err != nil {
		t.Fatalf("flush 1: %v", err)
	}
	before, _ := os.ReadFile(path)

	if _, err := m.flushSession(); err != nil {
		t.Fatalf("flush 2: %v", err)
	}
	same, _ := os.ReadFile(path)
	if !bytes.Equal(before, same) {
		t.Fatal("a flush with nothing new changed the log")
	}

	m.add(roleUser, "two")
	streamReply(m, "reply two")
	if _, err := m.flushSession(); err != nil {
		t.Fatalf("flush 3: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.HasPrefix(after, before) {
		t.Fatal("a later flush rewrote earlier lines")
	}

	s, _ := session.Load(path)
	if len(s.Entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(s.Entries))
	}
}

func TestNilLogIsANoOp(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.log = nil
	m.add(roleUser, "hello")
	streamReply(m, "hi")
	if path, err := m.flushSession(); err != nil || path != "" {
		t.Fatalf("flushSession = (%q, %v)", path, err)
	}
	if path, err := m.closeSession(); err != nil || path != "" {
		t.Fatalf("closeSession = (%q, %v)", path, err)
	}
}

func TestSaveActiveSessionWithNoRunningTUI(t *testing.T) {
	setActive(nil)
	path, err := SaveActiveSession()
	if err != nil || path != "" {
		t.Fatalf("SaveActiveSession = (%q, %v), want (\"\", nil)", path, err)
	}
}

func TestSaveActiveSessionFlushesMidReply(t *testing.T) {
	isolateHome(t)
	m := testModel(t)
	m.add(roleUser, "hello")
	streamReply(m, "partial ans") // killed while still streaming
	setActive(m)
	t.Cleanup(func() { setActive(nil) })

	path, err := SaveActiveSession()
	if err != nil {
		t.Fatalf("SaveActiveSession: %v", err)
	}
	s, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Entries) != 2 || s.Entries[1].Content != "partial ans" {
		t.Fatalf("entries = %+v, want the partial reply kept", s.Entries)
	}
	if s.Complete {
		t.Fatal("Complete = true for a session that was not closed")
	}
}
