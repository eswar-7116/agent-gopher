package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenRouterAPIKey string
}

func Load() Config {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("error loading .env")
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY not set")
	}

	return Config{
		OpenRouterAPIKey: apiKey,
	}
}
