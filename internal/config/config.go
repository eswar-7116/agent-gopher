package config

import (
	"encoding/json"
	"log"
	"os"

	"github.com/eswar-7116/agent-gopher/internal/paths"
)

type MCPServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Config struct {
	OpenRouterAPIKey string            `json:"api_key"`
	BaseURL          string            `json:"base_url"`
	Model            string            `json:"model"`
	PermissiveShell  bool              `json:"permissive_shell"`
	WhitelistedCmds  []string          `json:"whitelisted_cmds"`
	TavilyAPIKey     string            `json:"tavily_api_key"`
	MCPServers       []MCPServerConfig `json:"mcp_servers"`
}

func Load() Config {
	configPath := paths.ConfigFile
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config
			err = os.MkdirAll(paths.ConfigDir, 0755)
			if err != nil {
				log.Fatalf("error creating config directory: %v", err)
			}
			defaultConfig := Config{
				BaseURL:          "",
				OpenRouterAPIKey: "",
				Model:            "openrouter/free",
				PermissiveShell:  false,
				WhitelistedCmds:  []string{},
				TavilyAPIKey:     "",
			}
			data, err = json.MarshalIndent(defaultConfig, "", "  ")
			if err != nil {
				log.Fatalf("error marshaling default config: %v", err)
			}
			err = os.WriteFile(configPath, data, 0600)
			if err != nil {
				log.Fatalf("error writing default config: %v", err)
			}
			log.Fatalf("Config file not found. A default one has been created at %s. Please fill it out and run again.", configPath)
		} else {
			log.Fatalf("error reading config file: %v", err)
		}
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("error parsing config file: %v", err)
	}

	// Validation
	if config.OpenRouterAPIKey == "" {
		log.Fatal("api_key not set in config.json")
	}
	if config.BaseURL == "" {
		log.Fatal("base_url not set in config.json")
	}
	if config.Model == "" {
		config.Model = "openrouter/free"
	}
	if config.WhitelistedCmds == nil {
		config.WhitelistedCmds = []string{}
	}

	return config
}

func Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(paths.ConfigFile, data, 0644)
}
