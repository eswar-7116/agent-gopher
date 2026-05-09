package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenRouterAPIKey string
	BaseURL          string
	PermissiveShell  bool
}

func Load() Config {
	config := Config{}

	// Load .env
	err := godotenv.Load()
	if err != nil {
		log.Fatal("error loading .env")
	}

	// Set API key
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY not set")
	}
	config.OpenRouterAPIKey = apiKey

	// Set base URL
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		log.Fatal("BASE_URL not set")
	}
	config.BaseURL = baseURL

	// Set permissive shell
	permissive := os.Getenv("PERMISSIVE_SHELL")
	config.PermissiveShell = strings.ToLower(permissive) == "true" || permissive == "1"

	return config
}
