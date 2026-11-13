package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func (m *Model) add(r role, content string) {
	now := time.Now()
	m.stateMu.Lock()
	m.sealLocked()
	e := entry{r, content}
	m.entries = append(m.entries, e)
	if r == roleAssistant {
		m.open, m.openAt = true, now
	} else {
		m.logEntryLocked(e, now)
	}
	m.stateMu.Unlock()
	m.refresh()
}

// Shows an entry without saving it to the session log
func (m *Model) show(r role, content string) {
	m.stateMu.Lock()
	m.sealLocked()
	m.entries = append(m.entries, entry{r, content})
	m.stateMu.Unlock()
	m.refresh()
}

// Appends streamed text to the entry in progress
func (m *Model) appendToLast(content string) bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if len(m.entries) == 0 {
		return false
	}
	m.entries[len(m.entries)-1].content += content
	return true
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
	footer += len(m.suggestions()) + len(m.queue)
	if m.argHint() != "" {
		footer++
	}
	if m.themeMenu != nil {
		footer += len(m.themeMenu) + 1
	}
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
	m.stateMu.RLock()
	entries := m.entries
	lastRole, hasEntries := roleInfo, len(entries) > 0
	if hasEntries {
		lastRole = entries[len(entries)-1].role
	}
	seen := map[string]bool{}
	for _, e := range entries {
		var label string
		var style lipgloss.Style
		switch e.role {
		case roleUser:
			label, style = "You", userLabel
		case roleAssistant:
			label, style = "Agent", agentLabel
		case roleTool:
			b.WriteString(toolStyle.Width(w).Render("⚙ " + e.content))
			b.WriteString("\n\n")
			continue
		case roleError:
			b.WriteString(errorStyle.Width(w).Render("✗ " + e.content))
			b.WriteString("\n\n")
			continue
		case roleDiff:
			b.WriteString(renderDiff(e.content, w))
			b.WriteString("\n\n")
			continue
		default:
			b.WriteString(dimStyle.Width(w).Render(e.content))
			b.WriteString("\n\n")
			continue
		}
		b.WriteString(style.Render(label))
		b.WriteString("\n")
		if e.role == roleAssistant {
			seen[e.content] = true
			b.WriteString(m.md.render(e.content, w))
		} else {
			b.WriteString(lipgloss.NewStyle().Width(w).Render(e.content))
		}
		b.WriteString("\n\n")
	}
	m.stateMu.RUnlock()
	m.md.prune(seen)

	if m.running && m.perm == nil {
		b.WriteString(dimStyle.Render(m.spinner.View() + " working…"))
	}
	m.viewport.SetContent(b.String())
	if atBottom || hasEntries && lastRole == roleUser {
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
	b.WriteString(titleStyle.Render("Agent Gopher"))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	status := m.statusLine()
	if m.perm != nil {
		b.WriteString(m.permPrompt())
		b.WriteString("\n")
	}
	b.WriteString(m.queueList())
	b.WriteString(m.suggestionList())
	if hint := m.argHint(); hint != "" {
		b.WriteString(dimStyle.Render("  " + hint))
		b.WriteString("\n")
	}
	b.WriteString(m.themeMenuView())
	b.WriteString(inputBox.Width(max(m.width-2, 10)).Render(m.input.View()))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(status))
	v.SetContent(b.String())
	return v
}

// Returns the hint line shown under the input box
func (m *Model) statusLine() string {
	if m.themeMenu != nil {
		return "↑/↓ navigate · Enter select · Esc cancel"
	}
	if m.perm != nil {
		if m.askingAlt {
			return "Enter send alternative · Esc skip"
		}
		if m.perm.Kind == tools.PermWrite {
			return "y allow · a allow all writes this session · n deny · PgUp/PgDn review diff"
		}
		if m.commandOverflows() {
			return "y allow · a always allow · n deny · ←/→ scroll command"
		}
		return "y allow · a always allow · n deny"
	}
	modeTag := ""
	if m.agent.Mode() == agent.ModePlan {
		modeTag = "[PLAN] "
	}
	var hint string
	switch {
	case m.urgent:
		hint = "Interrupting…"
	case m.running:
		hint = "Enter queue · Alt+Enter interrupt & send · Esc/Ctrl+C cancel"
	default:
		hint = "/ commands · Enter send · ↑/↓ history · PgUp/PgDn scroll · Ctrl+C quit"
	}
	return modeTag + hint + m.contextUsage()
}

// Formats context usage for the status line
func (m *Model) contextUsage() string {
	used, window := m.agent.ContextTokens(), m.agent.ContextWindow()
	if used == 0 {
		return ""
	}
	if window <= 0 {
		return " · ctx " + formatTokens(used)
	}
	return fmt.Sprintf(" · ctx %s/%s (%d%%)", formatTokens(used), formatTokens(window), used*100/window)
}

func formatTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + "k"
}

// Renders the pending permission request box
func (m *Model) permPrompt() string {
	box := promptStyle.Width(max(m.width-2, 10))
	if m.perm.Kind == tools.PermWrite {
		added, removed := tools.DiffStat(m.perm.Diff)
		return box.Render(toolStyle.Render("Write file?") + "\n" +
			truncate(fmt.Sprintf("%s (+%d −%d, diff shown above)", m.perm.Path, added, removed), m.commandWidth()))
	}
	kind := "command"
	if m.perm.Background {
		kind = "background command"
	}
	return box.Render(toolStyle.Render("Run "+kind+"?") + "\n" + marquee(flattenCommand(m.perm.Command), m.commandWidth(), m.marqueePos()))
}

// Width available for the command inside the prompt box
func (m *Model) commandWidth() int {
	return max(m.width-6, 10)
}

func (m *Model) commandOverflows() bool {
	return m.perm != nil && len([]rune(flattenCommand(m.perm.Command))) > m.commandWidth()
}

const marqueePause = 15

// Returns the marquee offset
func (m *Model) marqueePos() int {
	if m.scrubbed {
		return m.marquee
	}
	n := len([]rune(flattenCommand(m.perm.Command))) + len([]rune(marqueeGap))
	pos := m.marquee % (n + marqueePause)
	return max(pos-marqueePause, 0)
}

const marqueeGap = "   ⋯   "

// Shows newlines and tabs visibly so nothing in the command is hidden
func flattenCommand(cmd string) string {
	return strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\r", " ⏎ ", "\t", " ").Replace(cmd)
}

// Returns a width-wide window of s starting at pos, wrapping around
func marquee(s string, width, pos int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	loop := append(r, []rune(marqueeGap)...)
	out := make([]rune, width)
	for i := range out {
		out[i] = loop[(pos+i)%len(loop)]
	}
	return string(out)
}

// Max diff lines shown in the transcript
const maxDiffLines = 400

// Colors a unified diff line by line
func renderDiff(diff string, w int) string {
	lines := strings.Split(diff, "\n")
	extra := 0
	if len(lines) > maxDiffLines {
		extra = len(lines) - maxDiffLines
		lines = lines[:maxDiffLines]
	}
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		style := dimStyle
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"), strings.HasPrefix(l, "@@"):
		case strings.HasPrefix(l, "+"):
			style = addStyle
		case strings.HasPrefix(l, "-"):
			style = delStyle
		}
		b.WriteString(style.Width(w).Render(l))
	}
	if extra > 0 {
		fmt.Fprintf(&b, "\n%s", dimStyle.Render(fmt.Sprintf("… %d more lines", extra)))
	}
	if diff == "" {
		b.WriteString(dimStyle.Render("(no changes)"))
	}
	return b.String()
}

// Renders messages waiting for the current run to finish
func (m *Model) queueList() string {
	var b strings.Builder
	for _, t := range m.queue {
		b.WriteString(dimStyle.Render("⏳ " + truncate(t, max(m.width-6, 10))))
		b.WriteString("\n")
	}
	return b.String()
}

// Renders the slash-command suggestions
func (m *Model) suggestionList() string {
	var b strings.Builder
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
	return b.String()
}

func (m *Model) themeMenuView() string {
	if m.themeMenu == nil {
		return ""
	}
	current := ActiveTheme().Name
	var b strings.Builder
	b.WriteString(dimStyle.Render("Select a theme:"))
	b.WriteString("\n")
	for i, name := range m.themeMenu {
		label := name
		if name == current {
			label += " (current)"
		}
		if i == m.themeIdx {
			b.WriteString(userStyle.Render("▸ " + label))
		} else {
			b.WriteString(dimStyle.Render("  " + label))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
