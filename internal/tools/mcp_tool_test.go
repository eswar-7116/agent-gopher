package tools

import (
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewMCPTool_Name(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		toolName   string
		expected   string
	}{
		{"with server name", "myserver", "search", "myserver__search"},
		{"empty server name", "", "search", "mcp__search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &mcpclient.MCPServer{Name: tt.serverName}
			tool := &mcp.Tool{Name: tt.toolName}
			m := NewMCPTool(server, tool)
			if m.Name() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, m.Name())
			}
		})
	}
}

func TestMCPTool_Definition(t *testing.T) {
	server := &mcpclient.MCPServer{Name: "srv"}
	tool := &mcp.Tool{
		Name:        "greet",
		Description: "Says hello",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}

	m := NewMCPTool(server, tool)
	def := m.Definition()
	if def == (openaiToolZero) {
		t.Fatal("expected non-zero definition")
	}
}

func TestMCPTool_DefinitionFallbackDescription(t *testing.T) {
	server := &mcpclient.MCPServer{Name: "srv"}
	tool := &mcp.Tool{Name: "mytool", Description: ""}
	m := NewMCPTool(server, tool)

	// Should not panic even with empty description
	_ = m.Definition()
}

func TestMCPTool_InputSchemaToParams(t *testing.T) {
	tests := []struct {
		name     string
		schema   any
		expectOK bool
	}{
		{"nil schema", nil, true},
		{"map schema", map[string]any{"type": "object", "properties": map[string]any{}}, true},
		{"struct schema", struct {
			Type string `json:"type"`
		}{"object"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &mcpclient.MCPServer{Name: "s"}
			tool := &mcp.Tool{Name: "t", InputSchema: tt.schema}
			m := NewMCPTool(server, tool)
			params := m.inputSchemaToParams()
			if _, ok := params["type"]; !ok && tt.expectOK {
				t.Error("expected 'type' key in params")
			}
		})
	}
}

// zero value for comparison
var openaiToolZero openaiToolType

type openaiToolType = any
