package mcpclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/eswar-7116/agent-gopher/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	mcpClient *mcp.Client
	once      sync.Once

	servers   []*MCPServer
	serversMu sync.Mutex
)

func getMCPClient() *mcp.Client {
	once.Do(func() {
		mcpClient = mcp.NewClient(&mcp.Implementation{
			Name:    "Agent Gopher",
			Version: "1.0.0",
		}, &mcp.ClientOptions{})
	})
	return mcpClient
}

type MCPServer struct {
	clientSession *mcp.ClientSession
	Name          string
	Tools         []*mcp.Tool
}

func Connect(ctx context.Context, server *config.MCPServerConfig) (*MCPServer, error) {
	cli := getMCPClient()

	mcpServer := &MCPServer{
		Name: server.Name,
	}

	cmd := exec.Command(server.Command, server.Args...)
	cs, err := cli.Connect(ctx, &mcp.CommandTransport{
		Command: cmd,
	}, &mcp.ClientSessionOptions{})
	if err != nil {
		return nil, err
	}
	mcpServer.clientSession = cs

	err = mcpServer.loadTools(ctx)
	if err != nil {
		return nil, err
	}

	serversMu.Lock()
	servers = append(servers, mcpServer)
	serversMu.Unlock()

	return mcpServer, nil
}

func (m *MCPServer) loadTools(ctx context.Context) error {
	res, err := m.clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return err
	}

	m.Tools = res.Tools

	return nil
}

// CallTool invokes a tool on this server and returns the text result.
func (m *MCPServer) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	result, err := m.clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("MCP CallTool %q failed: %w", name, err)
	}

	if result.IsError {
		return extractContent(result), fmt.Errorf("MCP tool %q returned an error", name)
	}

	return extractContent(result), nil
}

// extractContent collects text and saves images from a CallToolResult.
func extractContent(result *mcp.CallToolResult) string {
	var parts []string
	for _, c := range result.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		case *mcp.ImageContent:
			path, err := saveImage(v)
			if err != nil {
				parts = append(parts, fmt.Sprintf("[image save failed: %v]", err))
			} else {
				parts = append(parts, fmt.Sprintf("[image saved to %s]", path))
			}
		}
	}
	return strings.Join(parts, "\n")
}

// saveImage writes ImageContent to a temp file and returns the path.
func saveImage(img *mcp.ImageContent) (string, error) {
	ext := ".bin"
	switch img.MIMEType {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	f, err := os.CreateTemp("", "mcp-image-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Write(img.Data); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// GetAllServers returns all connected MCP servers.
func GetAllServers() []*MCPServer {
	serversMu.Lock()
	defer serversMu.Unlock()
	cp := make([]*MCPServer, len(servers))
	copy(cp, servers)
	return cp
}

func Close() {
	serversMu.Lock()
	defer serversMu.Unlock()
	for _, s := range servers {
		if s.clientSession != nil {
			s.clientSession.Close()
		}
	}
}
