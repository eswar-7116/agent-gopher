package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go/v3"
)

// ReadFileTool implements Tool for reading files.
type ReadFileTool struct{}

func (ReadFileTool) Name() string {
	return "read_file"
}

func (r ReadFileTool) Definition() openai.ChatCompletionToolUnionParam {
	return newFunctionTool(r.Name(), "Get the contents of the given file", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filepath": map[string]string{
				"type":        "string",
				"description": "The path to the file to be read, e.g., 'config.json' or 'main.go'",
			},
		},
		"required": []string{"filepath"},
	})
}

func (ReadFileTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	fpath, ok := args["filepath"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'filepath' argument")
	}

	data, err := os.ReadFile(fpath)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}
