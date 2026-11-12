package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSkills(t *testing.T) {
	root := t.TempDir()
	if got := Load(filepath.Join(root, "missing"), nil); got != nil {
		t.Fatalf("expected nil for missing dir, got %v", got)
	}

	writeSkill(t, root, "review-pr", "---\nname: pr-review\ndescription: \"Review a pull request\"\n---\n# Review\nDo the review.\n")
	writeSkill(t, root, "release", "# Release\n\nCut a release carefully.\n")
	writeSkill(t, root, "empty", "  \n")
	os.MkdirAll(filepath.Join(root, "no-skill-file"), 0o755)
	os.WriteFile(filepath.Join(root, "stray.txt"), []byte("x"), 0o644)

	skills := Load(root, nil)
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %+v", skills)
	}
	if skills[0].Name != "pr-review" || skills[0].Description != "Review a pull request" ||
		skills[0].Body != "# Review\nDo the review." {
		t.Errorf("bad frontmatter skill: %+v", skills[0])
	}
	if skills[1].Name != "release" || skills[1].Description != "Cut a release carefully." {
		t.Errorf("bad fallback skill: %+v", skills[1])
	}
}

func TestUseSkillTool(t *testing.T) {
	skills := []Skill{{Name: "a", Description: "desc a", Body: "body a"}}
	if p := Prompt(skills); !strings.Contains(p, "- a: desc a") || !strings.Contains(p, ToolName) {
		t.Errorf("bad prompt: %q", p)
	}
	if Prompt(nil) != "" {
		t.Error("expected empty prompt for no skills")
	}

	tool := NewTool(skills)
	got, err := tool.Execute(context.Background(), map[string]any{"name": "a"})
	if err != nil || got != "body a" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"name": "zzz"}); err == nil {
		t.Error("expected error for unknown skill")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil {
		t.Error("expected error for missing name")
	}
}
