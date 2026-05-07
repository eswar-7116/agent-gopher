package llm

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func NewClient(apiKey string) openai.Client {
	return openai.NewClient(
		option.WithBaseURL("https://openrouter.ai/api/v1"),
		option.WithAPIKey(apiKey),
	)
}
