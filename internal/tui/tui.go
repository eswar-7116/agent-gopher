package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/tools"
)

type role int

const (
	roleUser role = iota
	roleAssistant
	roleTool
	roleError
	roleInfo
)

type entry struct {
	role    role
	content string
}

var (
	userStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	agentStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	promptStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("214")).Padding(0, 1)
	inputBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

// Messages sent from the agent goroutine
type (
	textMsg     string
	toolCallMsg struct{ name, args string }
	doneMsg     struct{ err error }
	permReqMsg  struct {
		cmd        string
		background bool
		reply      chan permReply
	}
)

type permReply struct{ choice, alt string }

type Model struct {
	agent    *agent.Agent
	prog     *tea.Program
	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	entries []entry
	history []string
	histIdx int

	running bool
	cancel  context.CancelFunc

	// permission request
	perm      *permReqMsg
	askingAlt bool

	sugIdx int // selected slash-command suggestion

	width, height int
	ready         bool
}

type slashCmd struct{ name, desc string }

var slashCmds = []slashCmd{
	{"/exit", "Quit Agent Gopher"},
	{"/quit", "Quit Agent Gopher"},
}

func (m *Model) suggestions() []slashCmd {
	v := m.input.Value()
	if m.perm != nil || !strings.HasPrefix(v, "/") || strings.ContainsAny(v, " \t") {
		return nil
	}
	var out []slashCmd
	for _, c := range slashCmds {
		if strings.HasPrefix(c.name, v) {
			out = append(out, c)
		}
	}
	return out
}

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
		m.add(roleAssistant, string(msg))
		return m, nil

	case toolCallMsg:
		m.add(roleTool, fmt.Sprintf("%s %s", msg.name, truncate(msg.args, 200)))
		return m, nil

	case doneMsg:
		m.running, m.cancel = false, nil
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

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c":
		if m.perm != nil {
			m.answer("n", "")
		}
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	// Permission prompt takes over input until answered
	if m.perm != nil {
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

	if sugs := m.suggestions(); len(sugs) > 0 {
		m.sugIdx = min(m.sugIdx, len(sugs)-1)
		switch key {
		case "up":
			m.sugIdx = (m.sugIdx + len(sugs) - 1) % len(sugs)
			return m, nil
		case "down":
			m.sugIdx = (m.sugIdx + 1) % len(sugs)
			return m, nil
		case "tab":
			m.input.SetValue(sugs[m.sugIdx].name)
			m.input.CursorEnd()
			m.layout()
			return m, nil
		case "enter":
			if m.input.Value() != sugs[m.sugIdx].name {
				m.input.SetValue(sugs[m.sugIdx].name)
				m.input.CursorEnd()
				m.layout()
				return m, nil
			}
		case "esc":
			m.input.Reset()
			m.layout()
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
		text := strings.TrimSpace(m.input.Value())
		if text == "/exit" || text == "/quit" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if text == "" || m.running {
			return m, nil
		}
		m.input.Reset()
		m.layout()
		m.history = append(m.history, text)
		m.histIdx = len(m.history)
		m.add(roleUser, text)
		return m, m.start(text)
	case "up", "down":
		if len(m.history) == 0 {
			return m, nil
		}
		if key == "up" && m.histIdx > 0 {
			m.histIdx--
		} else if key == "down" && m.histIdx < len(m.history) {
			m.histIdx++
		}
		if m.histIdx == len(m.history) {
			m.input.Reset()
		} else {
			m.input.SetValue(m.history[m.histIdx])
			m.input.CursorEnd()
		}
		m.layout()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.sugIdx = 0
	m.layout()
	return m, cmd
}

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

func (m *Model) start(text string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.running = cancel, true
	ag, p := m.agent, m.prog
	go func() {
		p.Send(doneMsg{err: ag.Run(ctx, text)})
	}()
	return nil
}

func (m *Model) add(r role, content string) {
	m.entries = append(m.entries, entry{r, content})
	m.refresh()
}

func (m *Model) layout() {
	if !m.ready {
		return
	}
	m.input.SetWidth(max(m.width-6, 10))
	footer := 3 + 1 // input box + status line
	if m.perm != nil {
		footer += 4
	}
	footer += len(m.suggestions())
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(max(m.height-footer-1, 3))
	m.refresh()
}

func (m *Model) refresh() {
	if !m.ready {
		return
	}
	atBottom := m.viewport.AtBottom()
	w := max(m.width-2, 10)
	var b strings.Builder
	for _, e := range m.entries {
		var label string
		var style lipgloss.Style
		switch e.role {
		case roleUser:
			label, style = "You", userStyle
		case roleAssistant:
			label, style = "Agent", agentStyle
		case roleTool:
			b.WriteString(toolStyle.Width(w).Render("⚙ " + e.content))
			b.WriteString("\n\n")
			continue
		case roleError:
			b.WriteString(errorStyle.Width(w).Render("✗ " + e.content))
			b.WriteString("\n\n")
			continue
		default:
			b.WriteString(dimStyle.Width(w).Render(e.content))
			b.WriteString("\n\n")
			continue
		}
		b.WriteString(style.Render(label))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Width(w).Render(e.content))
		b.WriteString("\n\n")
	}
	if m.running && m.perm == nil {
		b.WriteString(dimStyle.Render(m.spinner.View() + " working…"))
	}
	m.viewport.SetContent(b.String())
	if atBottom || len(m.entries) > 0 && m.entries[len(m.entries)-1].role == roleUser {
		m.viewport.GotoBottom()
	}
}

func (m *Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if !m.ready {
		v.SetContent("Loading…")
		return v
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("🤖 Agent Gopher"))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	status := "/ commands · Enter send · ↑/↓ history · PgUp/PgDn scroll · Ctrl+C quit"
	if m.running {
		status = "Esc cancel · Ctrl+C quit"
	}
	if m.perm != nil {
		kind := "command"
		if m.perm.background {
			kind = "background command"
		}
		if m.askingAlt {
			status = "Enter send alternative · Esc skip"
		} else {
			status = "y allow · a always allow · n deny"
		}
		b.WriteString(promptStyle.Width(max(m.width-2, 10)).Render(
			toolStyle.Render("Run "+kind+"?") + "\n" + truncate(m.perm.cmd, 400)))
		b.WriteString("\n")
	}
	for i, c := range m.suggestions() {
		line := fmt.Sprintf("%-8s %s", c.name, c.desc)
		if i == m.sugIdx {
			line = userStyle.Render("▸ " + line)
		} else {
			line = dimStyle.Render("  " + line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(inputBox.Width(max(m.width-2, 10)).Render(m.input.View()))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(status))
	v.SetContent(b.String())
	return v
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
