package main

import (
	"context"
	"flag"
	"log"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/config"
	"github.com/eswar-7116/agent-gopher/internal/llm"
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
}
