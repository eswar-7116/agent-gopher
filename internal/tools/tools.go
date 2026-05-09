package tools

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

type Tool interface {
	Name() string
	Definition() openai.ChatCompletionToolUnionParam
	Execute(ctx context.Context, args map[string]any) (any, error)
}

// Registry of all available tools
func Registry(permissiveShell bool) map[string]Tool {
	tools := []Tool{
		ReadFileTool{},
		WriteFileTool{},
		ShellCmdTool{Permissive: permissiveShell},
		ReadLogsTool{},
		KillProcessTool{},
	}

	registry := make(map[string]Tool, len(tools))
	for _, t := range tools {
		registry[t.Name()] = t
	}
	return registry
}

// Returns the OpenAI tool definitions for all registered tools
func Definitions(permissiveShell bool) []openai.ChatCompletionToolUnionParam {
	defs := make([]openai.ChatCompletionToolUnionParam, 0)
	for _, t := range Registry(permissiveShell) {
		defs = append(defs, t.Definition())
	}
	return defs
}

func newFunctionTool(name, description string, params openai.FunctionParameters) openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        name,
		Description: openai.String(description),
		Parameters:  params,
	})
}
