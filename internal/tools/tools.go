package tools

import (
	"context"
	"maps"
	"slices"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

type Tool interface {
	Name() string
	Definition() openai.ChatCompletionToolUnionParam
	Execute(ctx context.Context, args map[string]any) (any, error)
}

type Options struct {
	PermissiveShell bool
	PermissiveWrite bool
	TavilyAPIKey    string
	WhitelistedCmds []string
	OnWhitelist     func(string)
}

// Registry of all available tools
func Registry(opts Options) map[string]Tool {
	gate := NewWriteGate(opts.PermissiveWrite)
	tools := []Tool{
		ReadFileTool{},
		WriteFileTool{Gate: gate},
		EditFileTool{Gate: gate},
		ShellCmdTool{
			Permissive: opts.PermissiveShell,
			Whitelist:  NewWhitelist(opts.WhitelistedCmds, opts.OnWhitelist),
		},
		ReadLogsTool{},
		KillProcessTool{},
	}
	if opts.TavilyAPIKey != "" {
		tools = append(tools, NewWebSearchTool(opts.TavilyAPIKey))
	}

	registry := make(map[string]Tool, len(tools))
	for _, t := range tools {
		registry[t.Name()] = t
	}
	return registry
}

// Returns the OpenAI tool definitions sorted by name
func Definitions(registry map[string]Tool) []openai.ChatCompletionToolUnionParam {
	names := slices.Sorted(maps.Keys(registry))
	defs := make([]openai.ChatCompletionToolUnionParam, 0, len(names))
	for _, n := range names {
		defs = append(defs, registry[n].Definition())
	}
	return defs
}

func NewFunctionTool(name, description string, params openai.FunctionParameters) openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        name,
		Description: openai.String(description),
		Parameters:  params,
	})
}
