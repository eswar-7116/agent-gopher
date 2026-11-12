package tui

import (
	"os"
	"sync"

	"github.com/eswar-7116/agent-gopher/internal/session"
)

var roleNames = map[role]string{
	roleUser:      "user",
	roleAssistant: "assistant",
	roleTool:      "tool",
	roleError:     "error",
	roleInfo:      "info",
}

var (
	activeMu sync.Mutex
	active   *Model
)

func setActive(m *Model) {
	activeMu.Lock()
	defer activeMu.Unlock()
	active = m
}

// Writes the running TUI's chat state
func SaveActiveSession() (string, error) {
	activeMu.Lock()
	m := active
	activeMu.Unlock()
	if m == nil {
		return "", nil
	}
	return m.saveSession()
}

// Builds the state to persist from the current conversation
func (m *Model) snapshot() *session.State {
	m.stateMu.RLock()
	entries := make([]session.Entry, 0, len(m.entries))
	for _, e := range m.entries {
		name, ok := roleNames[e.role]
		if !ok {
			name = "info"
		}
		entries = append(entries, session.Entry{Role: name, Content: e.content})
	}
	history := append([]string(nil), m.history...)
	m.stateMu.RUnlock()

	s := &session.State{
		ID:           m.sessionID,
		Version:      session.Version,
		ProjectPath:  m.projectPath,
		CreatedAt:    m.startedAt,
		Model:        m.agent.Model(),
		Mode:         m.agent.Mode().String(),
		Entries:      entries,
		InputHistory: history,
	}
	if msgs, err := m.agent.MessagesJSON(); err == nil {
		s.Messages = msgs
	}
	return s
}

// Persists the chat state
func (m *Model) saveSession() (string, error) {
	if m.sessionID == "" {
		return "", nil
	}
	s := m.snapshot()
	if !worthSaving(s) {
		return "", nil
	}
	return session.Save(s)
}

// Reports whether a state holds anything but the opening banner
func worthSaving(s *session.State) bool {
	if len(s.InputHistory) > 0 || len(s.Messages) > 0 {
		return true
	}
	for _, e := range s.Entries {
		if e.Role != "info" {
			return true
		}
	}
	return false
}

func newSessionID() string {
	id, err := session.NewID()
	if err != nil {
		return ""
	}
	return id
}

func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// Saves the chat state mid-session, after each completed agent run
func (m *Model) persist() {
	if _, err := m.saveSession(); err != nil && !m.saveFailed {
		m.saveFailed = true
		m.add(roleError, "Could not save chat state: "+err.Error())
	}
}
