package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/eswar-7116/agent-gopher/internal/tools"
)

// Resolves the pending permission request and restores normal input.
func (m *Model) answer(choice, alt string) {
	if m.perm == nil {
		return
	}
	if m.perm.Kind == tools.PermWrite {
		verb := map[string]string{"y": "allowed", "a": "allowed (all writes this session)", "n": "denied"}[choice]
		m.add(roleInfo, fmt.Sprintf("Write %s: %s", verb, m.perm.Path))
	} else {
		verb := map[string]string{"y": "allowed", "a": "always allowed", "n": "denied"}[choice]
		m.add(roleInfo, fmt.Sprintf("Command %s: %s", verb, truncate(m.perm.Command, 200)))
	}
	m.perm.reply <- permReply{choice, alt}
	m.perm, m.askingAlt = nil, false
	m.input.Reset()
	m.input.Placeholder = "Message the agent…"
	m.layout()
}

// Sends all queued messages to the agent as one run.
func (m *Model) drainQueue() tea.Cmd {
	if len(m.queue) == 0 {
		return nil
	}
	q := m.queue
	m.queue = nil
	for _, t := range q {
		m.add(roleUser, t)
	}
	m.layout()
	return m.start(strings.Join(q, "\n\n"))
}

// Moves queued messages back into the input after a failed or cancelled run.
func (m *Model) restoreQueue() {
	if len(m.queue) == 0 {
		return
	}
	q := append(m.queue, m.input.Value())
	m.queue = nil
	m.input.SetValue(strings.TrimSpace(strings.Join(q, " ")))
	m.input.CursorEnd()
	m.layout()
}

// Compacts the conversation
func (m *Model) startCompact() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.running = cancel, true
	ag, p := m.agent, m.prog
	go func() {
		p.Send(compactDoneMsg{err: ag.Compact(ctx)})
	}()
}

// Runs the agent on text in a goroutine and reports completion via doneMsg.
func (m *Model) start(text string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.running = cancel, true
	ag, p := m.agent, m.prog
	go func() {
		p.Send(doneMsg{err: ag.Run(ctx, text)})
	}()
	return nil
}
