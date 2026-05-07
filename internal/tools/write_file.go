package tools

import (
	"fmt"
	"os"

	"github.com/openai/openai-go/v3"
)

// WriteFileTool implements Tool for writing/appending files.
type WriteFileTool struct{}

func (WriteFileTool) Name() string { return "write_file" }

func (WriteFileTool) Definition() openai.ChatCompletionToolUnionParam {
	return newFunctionTool("write_file", "Write or append content to a file", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filepath": map[string]string{
				"type":        "string",
				"description": "The path to the file to be written",
			},
			"contents": map[string]string{
				"type":        "string",
				"description": "The content to write to the file",
			},
			"append": map[string]string{
				"type":        "boolean",
				"description": "Whether to append to the file instead of overwriting",
			},
		},
		"required": []string{"filepath", "contents"},
	})
}

func (WriteFileTool) Execute(args map[string]any) (any, error) {
	fpath, ok := args["filepath"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'filepath' argument")
	}
	contents, ok := args["contents"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'contents' argument")
	}
	appendFlag, _ := args["append"].(bool)

	flag := os.O_CREATE | os.O_WRONLY
	if appendFlag {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	file, err := os.OpenFile(fpath, flag, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	_, err = file.WriteString(contents)
	if err != nil {
		return nil, err
	}

	return map[string]any{"success": true, "filepath": fpath}, nil
}
