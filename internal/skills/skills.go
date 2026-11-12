package skills

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
)

const (
	skillsDirName = "skills"
	skillFileName = "SKILL.md"
	ToolName      = "use_skill"
	configDirName = ".agent-gopher"
)

// Skill is a reusable set of instructions stored in <skills dir>/<dir name>/SKILL.md
type Skill struct {
	Name        string
	Description string
	Body        string
}

func Load(dir string, logger *log.Logger) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) && logger != nil {
			logger.Printf("failed to read skills dir %s: %v", dir, err)
		}
		return nil
	}

	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(dir, e.Name(), skillFileName)
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) && logger != nil {
				logger.Printf("failed to read %s: %v", path, err)
			}
			continue
		}
		s := parseSkill(e.Name(), string(data))
		if s.Body == "" {
			continue
		}
		if logger != nil {
			logger.Printf("loaded skill %q from %s", s.Name, path)
		}
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills
}

func parseSkill(dirName, content string) Skill {
	s := Skill{Name: dirName}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	body := content

	if rest, ok := strings.CutPrefix(content, "---\n"); ok {
		if fm, after, found := strings.Cut(rest, "\n---"); found {
			// The closing delimiter must be on its own line.
			if tail, ok := strings.CutPrefix(after, "\n"); ok || after == "" {
				body = tail
				for _, line := range strings.Split(fm, "\n") {
					key, val, ok := strings.Cut(line, ":")
					if !ok {
						continue
					}
					val = strings.Trim(strings.TrimSpace(val), `"'`)
					switch strings.TrimSpace(key) {
					case "name":
						if val != "" {
							s.Name = val
						}
					case "description":
						s.Description = val
					}
				}
			}
		}
	}

	s.Body = strings.TrimSpace(body)
	if s.Description == "" {
		for _, line := range strings.Split(s.Body, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				s.Description = line
				break
			}
		}
	}
	return s
}

// Lists the available skills to include in the system prompt
func Prompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("You have access to skills: specialized instructions for particular kinds of tasks. " +
		"When a task matches a skill's description, call the " + ToolName +
		" tool with its name to load the full instructions, then follow them.\n\nAvailable skills:\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}

// tool returns the full instructions of a named skill
type tool struct {
	skills map[string]Skill
	names  []string
}

func NewTool(skills []Skill) tool {
	t := tool{skills: make(map[string]Skill, len(skills))}
	for _, s := range skills {
		t.skills[s.Name] = s
		t.names = append(t.names, s.Name)
	}
	return t
}

func (tool) Name() string { return ToolName }

func (t tool) Definition() openai.ChatCompletionToolUnionParam {
	return tools.NewFunctionTool(
		ToolName,
		"Load the full instructions of a skill listed in the system prompt. Call it when the current task matches the skill's description, then follow the returned instructions.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the skill to load",
					"enum":        t.names,
				},
			},
			"required": []string{"name"},
		},
	)
}

func (t tool) Execute(_ context.Context, args map[string]any) (any, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("missing or invalid 'name' argument")
	}
	s, ok := t.skills[name]
	if !ok {
		return nil, fmt.Errorf("unknown skill %q (available: %s)", name, strings.Join(t.names, ", "))
	}
	return s.Body, nil
}

// Dir returns ~/.agent-gopher/skills
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDirName, skillsDirName)
}
