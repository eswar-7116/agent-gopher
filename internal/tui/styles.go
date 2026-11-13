package tui

import "github.com/charmbracelet/lipgloss"

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

// Styles are set by applyTheme()
var (
	userStyle   lipgloss.Style
	agentStyle  lipgloss.Style
	toolStyle   lipgloss.Style
	errorStyle  lipgloss.Style
	dimStyle    lipgloss.Style
	titleStyle  lipgloss.Style
	promptStyle lipgloss.Style
	inputBox    lipgloss.Style
)

func init() {
	applyTheme()
}
