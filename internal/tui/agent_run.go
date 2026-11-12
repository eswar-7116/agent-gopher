package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// Resolves the pending permission request and restores normal input.
func (m *Model) answer(choice, alt string) {
	if m.perm == nil {
		return
	}
	verb := map[string]string{"y": "allowed", "a": "always allowed", "n": "denied"}[choice]
	m.add(roleInfo, fmt.Sprintf("Command %s: %s", verb, truncate(m.perm.cmd, 200)))
	m.perm.reply <- permReply{choice, alt}
	m.perm, m.askingAlt = nil, false
	m.input.Reset()
	m.input.Placeholder = "Message the agent…"
	m.layout()
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
