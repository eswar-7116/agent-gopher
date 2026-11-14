package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// viewport starts below the title
const viewportTop = 1

var selStyle = lipgloss.NewStyle().Reverse(true)

// position in viewport content
type selPos struct{ line, col int }

type selection struct {
	start, end selPos
	dragging   bool
	active     bool
}

func (s selection) bounds() (a, b selPos) {
	a, b = s.start, s.end
	if b.line < a.line || b.line == a.line && b.col < a.col {
		a, b = b, a
	}
	return
}

func (s selection) empty() bool { return !s.active || s.start == s.end }

func (m *Model) contentPos(x, y int) selPos {
	row := min(max(y-viewportTop, 0), m.viewport.Height()-1)
	line := min(m.viewport.YOffset()+row, max(len(m.lines)-1, 0))
	return selPos{line, max(x, 0)}
}

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	ms := msg.Mouse()
	switch msg.(type) {
	case tea.MouseClickMsg:
		if ms.Button != tea.MouseLeft {
			return nil
		}
		if ms.Y < viewportTop || ms.Y >= viewportTop+m.viewport.Height() {
			m.clearSelection()
			return nil
		}
		p := m.contentPos(ms.X, ms.Y)
		m.sel = selection{start: p, end: p, dragging: true, active: true}
		m.applySelection()
	case tea.MouseMotionMsg:
		if !m.sel.dragging {
			return nil
		}
		// auto-scroll when dragging past the edges
		if ms.Y < viewportTop {
			m.viewport.ScrollUp(1)
		} else if ms.Y >= viewportTop+m.viewport.Height() {
			m.viewport.ScrollDown(1)
		}
		m.sel.end = m.contentPos(ms.X, ms.Y)
		m.applySelection()
	case tea.MouseReleaseMsg:
		if !m.sel.dragging {
			return nil
		}
		m.sel.dragging = false
		if m.sel.empty() {
			m.clearSelection()
			return nil
		}
		if t := m.selectedText(); t != "" {
			return tea.SetClipboard(t)
		}
	}
	return nil
}

func (m *Model) clearSelection() {
	if !m.sel.active {
		return
	}
	m.sel = selection{}
	m.applySelection()
}

func (m *Model) selectedText() string {
	a, b := m.sel.bounds()
	var out []string
	for i := a.line; i <= b.line && i < len(m.lines); i++ {
		l, r := lineRange(i, a, b, ansi.StringWidth(m.lines[i]))
		out = append(out, strings.TrimRight(ansi.Strip(ansi.Cut(m.lines[i], l, r)), " "))
	}
	return strings.Join(out, "\n")
}

// selected cell range [l, r) on line i
func lineRange(i int, a, b selPos, w int) (int, int) {
	l, r := 0, w
	if i == a.line {
		l = a.col
	}
	if i == b.line {
		r = b.col + 1
	}
	return min(l, w), min(max(r, l), w)
}

func (m *Model) applySelection() {
	off := m.viewport.YOffset()
	if m.sel.empty() {
		m.viewport.SetContentLines(m.lines)
	} else {
		a, b := m.sel.bounds()
		lines := make([]string, len(m.lines))
		copy(lines, m.lines)
		for i := a.line; i <= b.line && i < len(lines); i++ {
			s := m.lines[i]
			w := ansi.StringWidth(s)
			l, r := lineRange(i, a, b, w)
			if l < r {
				lines[i] = ansi.Cut(s, 0, l) + selStyle.Render(ansi.Strip(ansi.Cut(s, l, r))) + ansi.Cut(s, r, w)
			}
		}
		m.viewport.SetContentLines(lines)
	}
	m.viewport.SetYOffset(off)
}
