package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eswar-7116/agent-gopher/internal/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/openai-go/v3"
)

// MCPTool adapts an MCP tool behind the tools.Tool interface
type MCPTool struct {
	server   *mcpclient.MCPServer
	tool     *mcp.Tool
	toolName string
}

func NewMCPTool(server *mcpclient.MCPServer, tool *mcp.Tool) MCPTool {
	prefix := server.Name
	if prefix == "" {
		prefix = "mcp"
	}
	return MCPTool{
		server:   server,
		tool:     tool,
		toolName: prefix + "__" + tool.Name,
	}
}

func (m MCPTool) Name() string {
	return m.toolName
}

func (m MCPTool) Definition() openai.ChatCompletionToolUnionParam {
	params := m.inputSchemaToParams()
	description := m.tool.Description
	if description == "" {
		description = fmt.Sprintf("MCP tool %q from server %q", m.tool.Name, m.server.Name)
	}
	return newFunctionTool(m.toolName, description, params)
}

func (m MCPTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	result, err := m.server.CallTool(ctx, m.tool.Name, args)
	if err != nil {
		return result, err
	}
	return result, nil
}

// inputSchemaToParams converts MCP InputSchema to openai.FunctionParameters.
func (m MCPTool) inputSchemaToParams() openai.FunctionParameters {
	if m.tool.InputSchema == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	// InputSchema is typically map[string]any from the wire; fall back to re-marshal.
	if schema, ok := m.tool.InputSchema.(map[string]any); ok {
		return schema
	}


	data, err := json.Marshal(m.tool.InputSchema)
	if err != nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return schema
}
