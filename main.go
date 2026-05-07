package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY not defined in the environment")
	}

	client := openai.NewClient(
		option.WithBaseURL("https://openrouter.ai/api/v1"),
		option.WithAPIKey(apiKey),
	)

	var userPrompt string
	flag.StringVar(&userPrompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	if userPrompt == "" {
		panic("Prompt must not be empty")
	}

	params := openai.ChatCompletionNewParams{
		Model: "openrouter/free",
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(userPrompt),
					},
				},
			},
		},
	}
	ctx := context.Background()

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp.Choices[0].Message.Content)
}
