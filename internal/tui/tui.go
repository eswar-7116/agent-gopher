package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/tools"
)

type Model struct {
	agent    *agent.Agent
	prog     *tea.Program
	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	entries     []entry
	IsStreaming bool
	history     []string
	histIdx     int

	running bool
	cancel  context.CancelFunc

	// permission request
	perm      *permReqMsg
	askingAlt bool

	sugIdx int // selected slash-command suggestion

	width, height int
	ready         bool
}

// Starts the TUI and blocks until it exits
func Run(ag *agent.Agent) error {
	ti := textinput.New()
	ti.Placeholder = "Message the agent…"
	ti.CharLimit = 10000
	ti.Focus()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	m := &Model{agent: ag, input: ti, spinner: sp, viewport: viewport.New()}
	m.entries = append(m.entries, entry{roleInfo, "Welcome to Agent Gopher. Enter sends · PgUp/PgDn scroll · Esc cancels a running task · Ctrl+C or /exit quits."})

	p := tea.NewProgram(m)
	m.prog = p

	ag.SetToolCallCallback(func(name, args string) { p.Send(toolCallMsg{name, args}) })
	ag.SetTextCallback(func(t string) { p.Send(textMsg(t)) })
	tools.PromptPermission = func(cmd string, bg bool) (string, string) {
		reply := make(chan permReply, 1)
		p.Send(permReqMsg{cmd: cmd, background: bg, reply: reply})
		r := <-reply
		return r.choice, r.alt
	}
	defer func() { tools.PromptPermission = nil }()

	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.layout()
		m.refresh()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.running {
			m.refresh()
		}
		return m, cmd

	case textMsg:
		if m.IsStreaming && len(m.entries) > 0 {
			m.entries[len(m.entries)-1].content += string(msg)
			m.refresh()
		} else {
			m.add(roleAssistant, string(msg))
			m.IsStreaming = true
		}
		return m, nil

	case toolCallMsg:
		m.IsStreaming = false
		m.add(roleTool, fmt.Sprintf("%s %s", msg.name, truncate(msg.args, 200)))
		return m, nil

	case doneMsg:
		m.running, m.cancel, m.IsStreaming = false, nil, false
		if msg.err != nil && msg.err != context.Canceled && !strings.Contains(msg.err.Error(), "context canceled") {
			m.add(roleError, msg.err.Error())
		} else if msg.err != nil {
			m.add(roleInfo, "Cancelled.")
		}
		m.refresh()
		return m, nil

	case permReqMsg:
		m.perm = &msg
		m.layout()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
