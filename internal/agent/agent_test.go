package agent

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// mock implementation of tools.Tool
type mockTool struct {
	name    string
	execute func(ctx context.Context, args map[string]any) (any, error)
}

func (m mockTool) Name() string { return m.name }
func (m mockTool) Definition() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name: m.name,
	})
}
func (m mockTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	if m.execute != nil {
		return m.execute(ctx, args)
	}
	return nil, nil
}

func TestSerialize(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name: "map",
			input: map[string]any{
				"key": "value",
			},
			expected: "{\n  \"key\": \"value\"\n}",
		},
		{
			name:     "int",
			input:    42,
			expected: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serialize(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExecuteToolCall(t *testing.T) {
	agent := &Agent{
		logger: log.New(io.Discard, "", 0),
		registry: map[string]tools.Tool{
			"mock_tool": mockTool{
				name: "mock_tool",
				execute: func(ctx context.Context, args map[string]any) (any, error) {
					if fail, ok := args["fail"].(bool); ok && fail {
						return nil, errors.New("tool error")
					}
					return "success", nil
				},
			},
			"complex_tool": mockTool{
				name: "complex_tool",
				execute: func(ctx context.Context, args map[string]any) (any, error) {
					return map[string]any{"result": "ok"}, nil
				},
			},
		},
	}

	tests := []struct {
		name      string
		toolName  string
		arguments string
		expected  string
	}{
		{
			name:      "unknown tool",
			toolName:  "unknown",
			arguments: `{}`,
			expected:  `Error: unknown tool "unknown"`,
		},
		{
			name:      "invalid arguments json",
			toolName:  "mock_tool",
			arguments: `{invalid json}`,
			expected:  `Error: failed to parse arguments: invalid character 'i' looking for beginning of object key string`,
		},
		{
			name:      "tool error",
			toolName:  "mock_tool",
			arguments: `{"fail": true}`,
			expected:  `Error: tool error`,
		},
		{
			name:      "successful string result",
			toolName:  "mock_tool",
			arguments: `{"fail": false}`,
			expected:  `success`,
		},
		{
			name:      "successful complex result",
			toolName:  "complex_tool",
			arguments: `{}`,
			expected:  "{\n  \"result\": \"ok\"\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.executeToolCall(t.Context(), tt.toolName, tt.arguments)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestNewAgent(t *testing.T) {
	client := openai.NewClient(option.WithAPIKey("dummy-key"))
	agent := NewAgent(&client, "test-model", false, "test-key", nil, log.New(io.Discard, "", 0))

	if agent == nil {
		t.Fatal("expected agent to not be nil")
	}

	if agent.client != &client {
		t.Error("expected client to be set")
	}

	if agent.registry == nil {
		t.Error("expected registry to be initialized")
	}
}
