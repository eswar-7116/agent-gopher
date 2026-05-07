package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/config"
	"github.com/eswar-7116/agent-gopher/internal/llm"
	"github.com/eswar-7116/agent-gopher/internal/tools"
)

func main() {
	var userPrompt string
	flag.StringVar(&userPrompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	if userPrompt == "" {
		log.Fatal("Prompt must not be empty (use -p flag)")
	}

	cfg := config.Load()
	client := llm.NewClient(cfg.OpenRouterAPIKey)
	agentGopher := agent.NewAgent(&client)

	ctx := context.Background()
	if err := agentGopher.Run(ctx, userPrompt); err != nil {
		log.Fatal(err)
	}

	defer func() {
		tools.BgProcessesMu.Lock()
		if len(tools.BgProcesses) > 0 {
			fmt.Println("\nWarning: background processes still running:")
			for _, p := range tools.BgProcesses {
				fmt.Printf("  PID %d - %s (logs: %s)\n", p.PID, p.Cmd, p.LogFile)
			}
		}
		tools.BgProcessesMu.Unlock()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		os.Exit(1)
	}()
}
