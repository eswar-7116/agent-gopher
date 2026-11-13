package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eswar-7116/agent-gopher/internal/paths"
)

var (
	activeTheme      = builtinThemes["default"]
	themeWarning     string
	sortedThemeNames []string
)

func ActiveTheme() Theme { return activeTheme }

// Returns and clears any warning from theme loading
func ThemeWarning() string {
	w := themeWarning
	themeWarning = ""
	return w
}

// Writes built-in themes as .theme files to the themes directory if not written
func InitThemes() error {
	if err := os.MkdirAll(paths.ThemesDir, 0755); err != nil {
		return err
	}
	for name, t := range builtinThemes {
		path := filepath.Join(paths.ThemesDir, name+".theme")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := writeThemeFile(path, t); err != nil {
			return err
		}
	}
	rebuildThemeList()
	return nil
}

func writeThemeFile(path string, t Theme) error {
	lines := []string{
		"# Theme: " + t.Name,
		"user=" + t.User,
		"agent=" + t.Agent,
		"tool=" + t.Tool,
		"error=" + t.Error,
		"dim=" + t.Dim,
		"title=" + t.Title,
		"border=" + t.Border,
		"prompt=" + t.Prompt,
		"input_border=" + t.InputBorder,
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func LoadTheme(name string) error {
	if t, ok := builtinThemes[name]; ok {
		activeTheme = t
		applyTheme()
		return nil
	}
	t, err := readThemeFile(name)
	if err != nil {
		themeWarning = fmt.Sprintf("Theme %q not found, using default.", name)
		activeTheme = builtinThemes["default"]
		applyTheme()
		return nil
	}
	activeTheme = t
	applyTheme()
	return nil
}

func readThemeFile(name string) (Theme, error) {
	path := filepath.Join(paths.ThemesDir, name+".theme")
	f, err := os.Open(path)
	if err != nil {
		return Theme{}, fmt.Errorf("theme %q not found (looked in %s)", name, paths.ThemesDir)
	}
	defer f.Close()

	t := builtinThemes["default"]
	t.Name = name

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "user":
			t.User = val
		case "agent":
			t.Agent = val
		case "tool":
			t.Tool = val
		case "error":
			t.Error = val
		case "dim":
			t.Dim = val
		case "title":
			t.Title = val
		case "border":
			t.Border = val
		case "prompt":
			t.Prompt = val
		case "input_border":
			t.InputBorder = val
		}
	}
	return t, scanner.Err()
}

func applyTheme() {
	t := activeTheme
	userStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.User))
	agentStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Agent))
	toolStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Tool))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Error))
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Dim))
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Title))
	promptStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Prompt)).Padding(0, 1)
	inputBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.InputBorder)).Padding(0, 1)
	addStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.User))
	delStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Error))
}

func rebuildThemeList() {
	seen := make(map[string]bool)
	var names []string
	for name := range builtinThemes {
		seen[name] = true
		names = append(names, name)
	}
	entries, _ := os.ReadDir(paths.ThemesDir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".theme") {
			continue
		}
		base := strings.TrimSuffix(n, ".theme")
		if !seen[base] {
			names = append(names, base)
		}
	}
	sort.Strings(names)
	sortedThemeNames = names
}

func AvailableThemes() []string {
	return sortedThemeNames
}
