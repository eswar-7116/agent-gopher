package memory

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	configDirName   = ".agent-gopher"
	memoryFileName  = "facts.md"
	maxContentBytes = 16 * 1024 //  Bounds how much memory is placed in the prompt
	maxEntryRunes   = 500       // Bounds a single remembered fact
)

const (
	ScopeGlobal  = "global"
	ScopeProject = "project"
)

func GlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDirName)
}

func NormalizeProjectPath(projectPath string) string {
	cleaned := path.Clean(strings.ReplaceAll(projectPath, `\`, "/"))
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.ReplaceAll(cleaned, ":", "")
	normalized := strings.ToLower(strings.ReplaceAll(cleaned, "/", "-"))
	if normalized == "" || normalized == "." {
		return "root"
	}
	return normalized
}

func ProjectDir(projectPath string) string {
	root := GlobalDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, NormalizeProjectPath(projectPath))
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(strings.TrimPrefix(s, "- "))
	if r := []rune(s); len(r) > maxEntryRunes {
		s = string(r[:maxEntryRunes])
	}
	return s
}

func parseFacts(data []byte) []string {
	var facts []string
	for line := range strings.SplitSeq(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		fact, ok := strings.CutPrefix(line, "- ")
		if !ok || strings.TrimSpace(fact) == "" {
			continue
		}
		fact = strings.TrimSpace(fact)
		duplicate := false
		for _, existing := range facts {
			if strings.EqualFold(existing, fact) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			facts = append(facts, fact)
		}
	}
	return facts
}

func writeFacts(dir string, facts []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var content strings.Builder
	content.WriteString("# Memory\n\n")
	for _, fact := range facts {
		content.WriteString("- ")
		content.WriteString(fact)
		content.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(dir, memoryFileName), []byte(content.String()), 0o644)
}

// Returns the stored facts.
func List(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, memoryFileName))
	if err != nil {
		return nil
	}
	return parseFacts(data)
}

// Stores a fact and reports false if it already exists.
func Add(dir, text string) (bool, error) {
	text = oneLine(text)
	if text == "" {
		return false, fmt.Errorf("nothing to remember")
	}
	facts := List(dir)
	for _, fact := range facts {
		if strings.EqualFold(fact, text) {
			return false, nil
		}
	}
	facts = append(facts, text)
	return true, writeFacts(dir, facts)
}

// Deletes one fact selected by its 1-based number or a unique case-insensitive substring.
func Remove(dir, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("specify a number or text to forget")
	}
	entries := List(dir)
	var targetIndex int
	if n, err := strconv.Atoi(query); err == nil {
		if n < 1 || n > len(entries) {
			return "", fmt.Errorf("no memory #%d (there are %d)", n, len(entries))
		}
		targetIndex = n - 1
	} else {
		var matches []int
		for i, fact := range entries {
			if strings.Contains(strings.ToLower(fact), strings.ToLower(query)) {
				matches = append(matches, i)
			}
		}
		switch len(matches) {
		case 0:
			return "", fmt.Errorf("no memory matches %q", query)
		case 1:
			targetIndex = matches[0]
		default:
			return "", fmt.Errorf("%d memories match %q; use a number or more specific text", len(matches), query)
		}
	}

	removed := entries[targetIndex]
	remaining := append(entries[:targetIndex], entries[targetIndex+1:]...)
	return removed, writeFacts(dir, remaining)
}

// Renders facts numbered as Remove expects.
func Format(entries []string) string {
	var b strings.Builder
	for i, fact := range entries {
		fmt.Fprintf(&b, "%d. %s\n", i+1, fact)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Renders the stored facts for the system prompt.
func Content(dir, scope string) string {
	entries := List(dir)
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	if scope == ScopeGlobal {
		b.WriteString("Global memory (facts for any project):\n")
	} else {
		b.WriteString("Project memory (facts for this project):\n")
	}
	for _, fact := range entries {
		line := "- " + fact + "\n"
		if b.Len()+len(line) > maxContentBytes {
			b.WriteString("- (further memories omitted; see ")
			b.WriteString(dir)
			b.WriteString(")\n")
			break
		}
		b.WriteString(line)
	}
	return strings.TrimRight(b.String(), "\n")
}
