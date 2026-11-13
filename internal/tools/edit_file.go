package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
)

// EditFileTool implements Tool for search/replace edits.
type EditFileTool struct {
	Gate *WriteGate
}

func (EditFileTool) Name() string { return "edit_file" }

func (e EditFileTool) Definition() openai.ChatCompletionToolUnionParam {
	return NewFunctionTool(e.Name(), "Edit a file by replacing exact text. 'old_string' must match the file exactly (including whitespace) and be unique unless 'replace_all' is set; include enough surrounding lines to make it unique. Prefer this over write_file for changing existing files.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filepath": map[string]string{
				"type":        "string",
				"description": "The path to the file to edit",
			},
			"old_string": map[string]string{
				"type":        "string",
				"description": "The exact text to replace",
			},
			"new_string": map[string]string{
				"type":        "string",
				"description": "The text to replace it with",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace every occurrence instead of requiring a unique match",
				"default":     false,
			},
		},
		"required": []string{"filepath", "old_string", "new_string"},
	})
}

func (e EditFileTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	fpath, ok := args["filepath"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'filepath' argument")
	}
	oldStr, ok := args["old_string"].(string)
	if !ok || oldStr == "" {
		return nil, fmt.Errorf("missing or invalid 'old_string' argument")
	}
	newStr, ok := args["new_string"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'new_string' argument")
	}
	replaceAll, _ := args["replace_all"].(bool)
	if oldStr == newStr {
		return nil, fmt.Errorf("'old_string' and 'new_string' are identical")
	}

	info, err := os.Stat(fpath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(fpath)
	if err != nil {
		return nil, err
	}
	before := string(data)

	n := strings.Count(before, oldStr)
	switch {
	case n == 0:
		return nil, fmt.Errorf("'old_string' not found in %s", fpath)
	case n > 1 && !replaceAll:
		return nil, fmt.Errorf("'old_string' matches %d places in %s; add more context or set replace_all", n, fpath)
	}
	if !replaceAll {
		n = 1
	}
	after := strings.Replace(before, oldStr, newStr, n)

	if err := e.Gate.Approve(fpath, UnifiedDiff(fpath, before, after)); err != nil {
		return nil, err
	}
	if err := os.WriteFile(fpath, []byte(after), info.Mode().Perm()); err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "filepath": fpath, "replacements": n}, nil
}
