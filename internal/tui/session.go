package tui

import (
	"sync"
	"time"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/paths"
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

// Flushes the running TUI's chat state
func SaveActiveSession() (string, error) {
	activeMu.Lock()
	m := active
	activeMu.Unlock()
	if m == nil {
		return "", nil
	}
	return m.flushSession()
}

// Starts the session log; nil if no id could be generated
func newSessionLog(ag *agent.Agent) *session.Writer {
	id, err := session.NewID()
	if err != nil {
		return nil
	}
	return session.NewWriter(&session.State{
		ID:          id,
		ProjectPath: paths.ProjectPath,
		CreatedAt:   time.Now(),
		Model:       ag.Model(),
		Mode:        ag.Mode().String(),
	})
}

// Queues a finished transcript line; caller holds stateMu
func (m *Model) logEntryLocked(e entry, at time.Time) {
	if m.log == nil {
		return
	}
	name, ok := roleNames[e.role]
	if !ok {
		name = roleNames[roleInfo]
	}
	m.log.AppendEntry(name, e.content, at)
	// First reply makes the session worth keeping
	if e.role == roleAssistant {
		m.log.Arm()
	}
}

// Queues the streamed reply once complete; caller holds stateMu
func (m *Model) sealLocked() {
	if !m.open || len(m.entries) == 0 {
		return
	}
	m.open = false
	m.logEntryLocked(m.entries[len(m.entries)-1], m.openAt)
}

// Appends new records to the session log
func (m *Model) flushSession() (string, error) {
	if m.log == nil {
		return "", nil
	}
	m.flushMu.Lock()
	defer m.flushMu.Unlock()

	m.stateMu.Lock()
	m.sealLocked()
	m.stateMu.Unlock()

	msgs, err := m.agent.MessagesJSONFrom(m.loggedMessages)
	if err != nil {
		return "", err
	}
	for _, msg := range msgs {
		m.log.AppendMessage(msg)
	}
	m.loggedMessages += len(msgs)

	if err := m.log.Flush(); err != nil {
		return "", err
	}
	if !m.log.Created() {
		return "", nil
	}
	return m.log.Path(), nil
}

// Flushes and closes the session log on exit
func (m *Model) closeSession() (string, error) {
	if m.log == nil {
		return "", nil
	}
	path, err := m.flushSession()
	if cerr := m.log.Close(m.agent.Mode().String()); err == nil {
		err = cerr
	}
	return path, err
}

// Saves the chat state after each completed agent run
func (m *Model) persist() {
	if _, err := m.flushSession(); err != nil && !m.saveFailed {
		m.saveFailed = true
		m.add(roleError, "Could not save chat state: "+err.Error())
	}
}
