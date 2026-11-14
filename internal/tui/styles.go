package tui

import "github.com/charmbracelet/lipgloss"

type role int

const (
	roleUser role = iota
	roleAssistant
	roleTool
	roleError
	roleInfo
	roleDiff
	roleThinking
)

type entry struct {
	role    role
	content string
}

// Styles are set by applyTheme()
var (
	userStyle   lipgloss.Style
	userLabel   lipgloss.Style
	agentLabel  lipgloss.Style
	toolStyle   lipgloss.Style
	errorStyle  lipgloss.Style
	dimStyle    lipgloss.Style
	titleStyle  lipgloss.Style
	promptStyle lipgloss.Style
	inputBox    lipgloss.Style
	addStyle    lipgloss.Style
	delStyle    lipgloss.Style
)

func init() {
	applyTheme()
}
