package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/eswar-7116/agent-gopher/internal/agent"
)

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

	status := m.statusLine()
	if m.perm != nil {
		b.WriteString(m.permPrompt())
		b.WriteString("\n")
	}
	b.WriteString(m.suggestionList())
	b.WriteString(inputBox.Width(max(m.width-2, 10)).Render(m.input.View()))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(status))
	v.SetContent(b.String())
	return v
}

// Returns the hint line shown under the input box
func (m *Model) statusLine() string {
	if m.perm != nil {
		if m.askingAlt {
			return "Enter send alternative · Esc skip"
		}
		return "y allow · a always allow · n deny"
	}
	modeTag := ""
	if m.agent.Mode() == agent.ModePlan {
		modeTag = "[PLAN] "
	}
	if m.running {
		return modeTag + "Esc cancel · Ctrl+C quit"
	}
	return modeTag + "/ commands · Enter send · ↑/↓ history · PgUp/PgDn scroll · Ctrl+C quit"
}

// Renders the pending permission request box
func (m *Model) permPrompt() string {
	kind := "command"
	if m.perm.background {
		kind = "background command"
	}
	return promptStyle.Width(max(m.width-2, 10)).Render(
		toolStyle.Render("Run "+kind+"?") + "\n" + truncate(m.perm.cmd, 400))
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

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
