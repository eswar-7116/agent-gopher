package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
)

type ReadLogsTool struct{}

func (ReadLogsTool) Name() string {
	return "read_logs"
}

func (r ReadLogsTool) Definition() openai.ChatCompletionToolUnionParam {
	return newFunctionTool(r.Name(), "Read logs from a background process log file", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"log_file": map[string]string{
				"type":        "string",
				"description": "Path to the log file returned by shell_cmd background mode",
			},
			"tail": map[string]any{
				"type":        "integer",
				"description": "Number of lines to read from the end of the file. Reads all lines if not specified.",
			},
			"filter": map[string]any{
				"type":        "string",
				"description": "Optional substring to filter lines by (e.g. 'error', 'warn')",
			},
		},
		"required": []string{"log_file"},
	})
}

func (ReadLogsTool) Execute(_ context.Context, args map[string]any) (any, error) {
	logFile, ok := args["log_file"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'log_file' argument")
	}

	filter, _ := args["filter"].(string)
	tail, _ := args["tail"].(float64)

	f, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("could not open log file: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if filter == "" || strings.Contains(line, filter) {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	if tail > 0 && int(tail) < len(lines) {
		lines = lines[len(lines)-int(tail):]
	}

	if len(lines) == 0 {
		return "(no output yet)", nil
	}

	return strings.Join(lines, "\n"), nil
}
