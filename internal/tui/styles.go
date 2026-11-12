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
