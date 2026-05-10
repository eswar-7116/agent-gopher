package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/config"
	"github.com/eswar-7116/agent-gopher/internal/llm"
	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cleanup()
		os.Exit(1)
	}()

	cfg := config.Load()
	client := llm.NewClient(cfg.OpenRouterAPIKey, cfg.BaseURL)
	agentGopher := agent.NewAgent(&client, cfg.PermissiveShell, cfg.TavilyAPIKey)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nUser: ")
		os.Stdout.Sync()

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/exit" || input == "/quit" {
			break
		}

		ctx := context.Background()
		if err := agentGopher.Run(ctx, input); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	cleanup()
}

func cleanup() {
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
