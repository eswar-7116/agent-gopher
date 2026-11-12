package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/config"
	"github.com/eswar-7116/agent-gopher/internal/llm"
	"github.com/eswar-7116/agent-gopher/internal/mcpclient"
	"github.com/eswar-7116/agent-gopher/internal/paths"
	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/eswar-7116/agent-gopher/internal/tui"
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		saveChatState()
		cleanup()
		os.Exit(1)
	}()

	if err := paths.Init(); err != nil {
		log.Fatalf("error resolving paths: %v", err)
	}
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
		Logger:          log.New(io.Discard, "", 0),
		WhitelistedCmds: cfg.WhitelistedCmds,
		OnWhitelist:     onWhitelist,
	})

	if err := tui.Run(agentGopher); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}

	cleanup()
}

// Persists the chat state of the running TUI
func saveChatState() {
	path, err := tui.SaveActiveSession()
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "\nWarning: failed to save chat state: %v\n", err)
	case path != "":
		fmt.Printf("\nChat state saved to %s\n", path)
	}
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
