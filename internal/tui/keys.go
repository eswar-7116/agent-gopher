package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c":
		if m.perm != nil {
			m.answer("n", "")
		}
		if m.cancel != nil {
			m.cancel()
			return m, nil
		}
		return m, tea.Quit
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	// Permission prompt takes over input until answered.
	if m.perm != nil {
		return m.handlePermKey(msg, key)
	}

	if sugs := m.suggestions(); len(sugs) > 0 {
		if handled := m.handleSuggestionKey(key, sugs); handled {
			return m, nil
		}
	}

	switch key {
	case "esc":
		if m.cancel != nil {
			m.cancel()
		}
		return m, nil
	case "enter":
		return m.submit()
	case "alt+enter":
		return m.sendNow()
	case "up", "down":
		m.navigateHistory(key)
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.sugIdx = 0
	m.layout()
	return m, cmd
}

// Processes input while a permission prompt is active.
func (m *Model) handlePermKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	if m.askingAlt {
		switch key {
		case "enter":
			m.answer("n", strings.TrimSpace(m.input.Value()))
			return m, nil
		case "esc":
			m.answer("n", "")
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	switch key {
	case "y", "Y", "enter":
		m.answer("y", "")
	case "a", "A":
		m.answer("a", "")
	case "n", "N", "esc":
		m.askingAlt = true
		m.input.Reset()
		m.input.Placeholder = "Alternative instruction for the agent (Enter to skip)"
		m.layout()
	}
	return m, nil
}

// Processes keys while slash-command suggestions are shown
func (m *Model) handleSuggestionKey(key string, sugs []slashCmd) bool {
	m.sugIdx = min(m.sugIdx, len(sugs)-1)
	switch key {
	case "up":
		m.sugIdx = (m.sugIdx + len(sugs) - 1) % len(sugs)
		return true
	case "down":
		m.sugIdx = (m.sugIdx + 1) % len(sugs)
		return true
	case "tab":
		m.input.SetValue(sugs[m.sugIdx].name)
		m.input.CursorEnd()
		m.layout()
		return true
	case "enter":
		if m.input.Value() != sugs[m.sugIdx].name {
			m.input.SetValue(sugs[m.sugIdx].name)
			m.input.CursorEnd()
			m.layout()
			return true
		}
	case "esc":
		m.input.Reset()
		m.layout()
		return true
	}
	return false
}

// Handles Enter: quits, runs a slash command, or starts an agent run.
func (m *Model) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "/exit" || text == "/quit" {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	if text == "" || m.running && strings.HasPrefix(text, "/") {
		return m, nil
	}
	m.input.Reset()
	m.layout()
	m.recordInput(text)
	if m.running {
		m.queue = append(m.queue, text)
		// Rejected only when the run is ending; doneMsg sends it then
		m.agent.Inject(text)
		m.layout()
		return m, nil
	}
	if strings.HasPrefix(text, "/") && m.runSlash(text) {
		return m, nil
	}
	m.add(roleUser, text)
	return m, m.start(text)
}

// Handles Alt+Enter: interrupts the running agent and sends the queue and input.
func (m *Model) sendNow() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if !m.running || strings.HasPrefix(text, "/") {
		return m.submit()
	}
	if text != "" {
		m.input.Reset()
		m.recordInput(text)
		m.queue = append(m.queue, text)
	}
	if len(m.queue) == 0 {
		return m, nil
	}
	m.urgent = true
	m.cancel()
	m.layout()
	return m, nil
}

// Saves submitted text to input history and the session log
func (m *Model) recordInput(text string) {
	m.stateMu.Lock()
	m.history = append(m.history, text)
	m.histIdx = len(m.history)
	m.stateMu.Unlock()
	if m.log != nil {
		m.log.AppendInput(text)
	}
}

// Moves through previously submitted inputs.
func (m *Model) navigateHistory(key string) {
	m.stateMu.RLock()
	history := m.history
	m.stateMu.RUnlock()
	if len(history) == 0 {
		return
	}
	if key == "up" && m.histIdx > 0 {
		m.histIdx--
	} else if key == "down" && m.histIdx < len(history) {
		m.histIdx++
	}
	if m.histIdx >= len(history) {
		m.input.Reset()
	} else {
		m.input.SetValue(history[m.histIdx])
		m.input.CursorEnd()
	}
	m.layout()
}
