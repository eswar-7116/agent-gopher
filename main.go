package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/config"
	"github.com/eswar-7116/agent-gopher/internal/llm"
	"github.com/eswar-7116/agent-gopher/internal/mcpclient"
	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/eswar-7116/agent-gopher/internal/tui"
)

func main() {
	debug := slices.Contains(os.Args[1:], "--debug")

	var debugLogger *log.Logger
	if debug {
		f, err := os.OpenFile("agent-gopher-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.Fatalf("failed to open debug log: %v", err)
		}
		defer f.Close()
		debugLogger = log.New(f, "DEBUG: ", log.Ltime)
	} else {
		debugLogger = log.New(io.Discard, "", 0)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cleanup()
		os.Exit(1)
	}()

	cfg := config.Load()
	client := llm.NewClient(cfg.OpenRouterAPIKey, cfg.BaseURL)

	// Connect to configured MCP servers
	ctx := context.Background()
	var mcpServers []*mcpclient.MCPServer
	for i := range cfg.MCPServers {
		server, err := mcpclient.Connect(ctx, &cfg.MCPServers[i])
		if err != nil {
			log.Printf("Warning: failed to connect to MCP server %q: %v\n", cfg.MCPServers[i].Name, err)
			continue
		}
		debugLogger.Printf("Connected to MCP server %q (%d tools)\n", server.Name, len(server.Tools))
		mcpServers = append(mcpServers, server)
	}

	onWhitelist := func(cmd string) {
		cfg.WhitelistedCmds = append(cfg.WhitelistedCmds, cmd)
		if err := config.Save(cfg); err != nil {
			log.Printf("Warning: failed to save whitelisted command to config: %v\n", err)
		}
	}

	agentGopher := agent.NewAgent(agent.AgentOptions{
		Client:          &client,
		Model:           cfg.Model,
		PermissiveShell: cfg.PermissiveShell,
		TavilyAPIKey:    cfg.TavilyAPIKey,
		MCPServers:      mcpServers,
		Logger:          debugLogger,
		WhitelistedCmds: cfg.WhitelistedCmds,
		OnWhitelist:     onWhitelist,
	})

	if err := tui.Run(agentGopher); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}

	cleanup()
}

func cleanup() {
	mcpclient.Close()

	tools.BgProcessesMu.Lock()
	defer tools.BgProcessesMu.Unlock()

	if len(tools.BgProcesses) > 0 {
		fmt.Println("\nWarning: background processes still running:")
		for _, p := range tools.BgProcesses {
			fmt.Printf("  PID %d - %s (logs: %s)\n", p.PID, p.Cmd, p.LogFile)
		}
	} else {
		fmt.Println("\nNo background processes running.")
	}
}
