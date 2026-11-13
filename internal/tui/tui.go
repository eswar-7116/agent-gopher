package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"os"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/session"
	"github.com/eswar-7116/agent-gopher/internal/tools"
)

type Model struct {
	agent    *agent.Agent
	prog     *tea.Program
	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	stateMu     sync.RWMutex
	entries     []entry
	history     []string
	IsStreaming bool
	histIdx     int

	log            *session.Writer
	open           bool      // last entry is an assistant reply still streaming
	openAt         time.Time // when that reply started
	flushMu        sync.Mutex
	loggedMessages int // agent messages already queued to the log
	loggedEpoch    int // agent epoch those messages belong to
	saveFailed     bool

	running bool
	cancel  context.CancelFunc
	queue   []string // messages sent while running
	urgent  bool     // run cancelled by Alt+Enter

	// permission request
	perm      *permReqMsg
	askingAlt bool
	marquee   int  // scroll position of a long command
	scrubbed  bool // user moved the marquee by hand

	sugIdx int // selected slash-command suggestion

	md mdRenderer // markdown renderer

	// theme picker
	themeMenu []string
	themeIdx  int

	// last applied wheel scroll, used to collapse bursts of wheel events.
	lastWheel    time.Time
	lastWheelDir tea.MouseButton

	width, height int
	ready         bool
}

// Starts the TUI and blocks until it exits.
// If resumeID is non-empty the previous session is loaded and its conversation restored.
// startupErrors are shown as errors once the TUI opens.
func Run(ag *agent.Agent, resumeID string, startupErrors []string) error {
	ti := textinput.New()
	ti.Placeholder = "Message the agent…"
	ti.CharLimit = 10000
	ti.Focus()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	vp := viewport.New()
	// Wheel scrolling is handled in Update
	vp.MouseWheelEnabled = false

	m := &Model{
		agent:    ag,
		input:    ti,
		spinner:  sp,
		viewport: vp,
	}

	var resumeErr error
	if resumeID != "" {
		resumeErr = m.resume(resumeID)
	}
	if resumeID == "" || resumeErr != nil {
		m.log = newSessionLog(ag)
		m.add(roleInfo, "Welcome to Agent Gopher. Enter sends · PgUp/PgDn scroll · Esc cancels a running task · Ctrl+C or /exit quits.")
		if resumeErr != nil {
			m.add(roleError, "Could not resume session: "+resumeErr.Error()+". Started a new chat.")
		}
	}
	if w := ThemeWarning(); w != "" {
		m.add(roleError, w)
	}
	for _, e := range startupErrors {
		m.add(roleError, e)
	}

	// Lets a signal handler save the chat state
	setActive(m)
	defer setActive(nil)

	p := tea.NewProgram(m)
	m.prog = p

	ag.SetToolCallCallback(func(name, args string) { p.Send(toolCallMsg{name, args}) })
	ag.SetTextCallback(func(t string) { p.Send(textMsg(t)) })
	ag.SetInjectedCallback(func(t []string) { p.Send(injectedMsg(t)) })
	ag.SetNoticeCallback(func(t string) { p.Send(noticeMsg(t)) })
	tools.PromptPermission = func(req tools.PermissionRequest) (string, string) {
		reply := make(chan permReply, 1)
		p.Send(permReqMsg{PermissionRequest: req, reply: reply})
		r := <-reply
		return r.choice, r.alt
	}
	defer func() { tools.PromptPermission = nil }()

	_, err := p.Run()

	if path, saveErr := m.closeSession(); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save chat state: %v\n", saveErr)
	} else if path != "" {
		fmt.Println(ResumeHint(path))
	}
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
		if m.perm != nil && !m.scrubbed {
			m.marquee++
		}
		if m.running {
			m.refresh()
		}
		return m, cmd

	case textMsg:
		if m.IsStreaming && m.appendToLast(string(msg)) {
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

	case noticeMsg:
		m.IsStreaming = false
		m.add(roleInfo, string(msg))
		return m, nil

	case compactDoneMsg:
		m.running, m.cancel = false, nil
		switch {
		case errors.Is(msg.err, agent.ErrNothingToCompact):
			m.add(roleInfo, "Nothing to compact yet.")
		case msg.err != nil && !errors.Is(msg.err, context.Canceled):
			m.add(roleError, msg.err.Error())
		case msg.err != nil:
			m.add(roleInfo, "Cancelled.")
		}
		m.persist()
		return m, m.drainQueue()

	case injectedMsg:
		m.queue = m.queue[min(len(msg), len(m.queue)):]
		m.IsStreaming = false
		for _, t := range msg {
			m.add(roleUser, t)
		}
		m.layout()
		return m, nil

	case doneMsg:
		m.running, m.cancel, m.IsStreaming = false, nil, false
		if m.urgent {
			m.urgent = false
			m.add(roleInfo, "Interrupted.")
			m.persist()
			return m, m.drainQueue()
		}
		if msg.err != nil && msg.err != context.Canceled && !strings.Contains(msg.err.Error(), "context canceled") {
			m.add(roleError, msg.err.Error())
		} else if msg.err != nil {
			m.add(roleInfo, "Cancelled.")
		}
		m.refresh()
		m.persist()
		if msg.err != nil {
			m.restoreQueue()
			return m, nil
		}
		return m, m.drainQueue()

	case permReqMsg:
		m.perm = &msg
		m.marquee, m.scrubbed = 0, false
		if msg.Kind == tools.PermWrite {
			m.IsStreaming = false
			m.add(roleDiff, msg.Diff)
		}
		m.layout()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		m.handleWheel(msg)
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

const wheelInterval = 16 * time.Millisecond

func (m *Model) handleWheel(msg tea.MouseWheelMsg) {
	dir := msg.Mouse().Button
	now := time.Now()
	if dir == m.lastWheelDir && now.Sub(m.lastWheel) < wheelInterval {
		return
	}
	m.lastWheel, m.lastWheelDir = now, dir
	switch dir {
	case tea.MouseWheelUp:
		m.viewport.ScrollUp(1)
	case tea.MouseWheelDown:
		m.viewport.ScrollDown(1)
	}
}
