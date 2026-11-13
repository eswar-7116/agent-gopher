package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// Returns executable name (agent-gopher if 'go run' is used)
func ExecName() string {
	exe := os.Args[0]

	if cache := goCacheDir(); strings.HasPrefix(exe, os.TempDir()) || cache != "" && strings.HasPrefix(exe, cache) {
		exe = filepath.Base(exe)
	}

	return exe
}

// Returns where go run caches built binaries; empty if unknown
func goCacheDir() string {
	if dir := os.Getenv("GOCACHE"); dir != "" && dir != "off" {
		return dir
	}
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "go-build")
	}
	return ""
}

// Returns the command that resumes the session logged at path
func ResumeHint(path string) string {
	id := strings.TrimSuffix(filepath.Base(path), session.Ext)
	return fmt.Sprintf("Resume this session with: %s --resume %s", ExecName(), id)
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

// Continues a saved session in place
func (m *Model) resume(idOrPrefix string) error {
	path, err := session.FindByPrefix(idOrPrefix)
	if err != nil {
		return err
	}
	prev, err := session.Load(path)
	if err != nil {
		return err
	}

	orig := m.agent.Mode()
	if mode, err := agent.ParseMode(prev.Mode); err == nil {
		m.agent.SetMode(mode)
	}
	if err := m.agent.Restore(prev.Messages); err != nil {
		m.agent.SetMode(orig)
		return fmt.Errorf("session %s: %w", prev.ID, err)
	}

	m.log = session.Reopen(path)
	m.loggedMessages = len(prev.Messages)
	for _, e := range prev.Entries {
		m.entries = append(m.entries, entry{roleFromName(e.Role), e.Content})
	}
	m.history = append(m.history, prev.InputHistory...)
	m.histIdx = len(m.history)
	m.add(roleInfo, "Resumed session "+prev.ID+".")
	return nil
}

func roleFromName(name string) role {
	for r, n := range roleNames {
		if n == name {
			return r
		}
	}
	return roleInfo
}
