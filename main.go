package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
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
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        "read_file",
				Description: openai.String("Get the contents of the given file"),
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"filepath": map[string]string{
							"type":        "string",
							"description": "The path to the file to be read, e.g., 'config.json' or 'main.go'",
						},
					},
					"required": []string{"filepath"},
				},
			}),
		},
	}
	ctx := context.Background()

	log.Println("Sending messages to LLM...")
	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal(err)
	}

	toolCalls := resp.Choices[0].Message.ToolCalls
	if len(toolCalls) == 0 {
		log.Printf("No function call")
	} else {
		for _, toolCall := range toolCalls {
			if toolCall.Function.Name == "read_file" {
				var args map[string]any
				err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
				if err != nil {
					log.Fatalln(err)
				}

				fpath := args["filepath"].(string)
				log.Println("Reading", fpath)
				contents, err := readFile(fpath)
				params.Messages = append(params.Messages, openai.ToolMessage(contents, toolCall.ID))
			}
		}
	}

	log.Println("Sending tool responses to LLM...")
	resp, err = client.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println(resp.Choices[0].Message.Content)
}
