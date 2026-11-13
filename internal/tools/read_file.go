package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
)

// ReadFileTool implements Tool for reading files.
type ReadFileTool struct{}

func (ReadFileTool) Name() string {
	return "read_file"
}

func (r ReadFileTool) Definition() openai.ChatCompletionToolUnionParam {
	return NewFunctionTool(r.Name(), "Get the contents of the given file. Use 'offset' and 'limit' to read only a range of lines from large files.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filepath": map[string]string{
				"type":        "string",
				"description": "The path to the file to be read, e.g., 'config.json' or 'main.go'",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "1-indexed line number to start reading from. Defaults to 1.",
				"minimum":     1,
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to read. Defaults to reading until the end of the file.",
				"minimum":     1,
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

	offset, err := intArg(args, "offset", 1)
	if err != nil {
		return nil, err
	}
	limit, err := intArg(args, "limit", 0)
	if err != nil {
		return nil, err
	}

	if offset == 1 && limit == 0 {
		data, err := os.ReadFile(fpath)
		if err != nil {
			return nil, err
		}
		return string(data), nil
	}

	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var sb strings.Builder
	lineNum, read := 0, 0
	for limit == 0 || read < limit {
		line, err := reader.ReadString('\n')
		if line != "" {
			lineNum++
			if lineNum >= offset {
				sb.WriteString(line)
				read++
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	if lineNum < offset {
		return nil, fmt.Errorf("offset %d is beyond end of file (%d lines)", offset, lineNum)
	}
	return sb.String(), nil
}

// Extracts a positive integer argument
func intArg(args map[string]any, key string, def int) (int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return def, nil
	}
	var n int
	switch x := v.(type) {
	case float64:
		if x != float64(int(x)) {
			return 0, fmt.Errorf("'%s' must be an integer", key)
		}
		n = int(x)
	case int:
		n = x
	default:
		return 0, fmt.Errorf("invalid '%s' argument", key)
	}
	if n < 1 {
		return 0, fmt.Errorf("'%s' must be >= 1", key)
	}
	return n, nil
}
