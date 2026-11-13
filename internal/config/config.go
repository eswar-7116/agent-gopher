package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/eswar-7116/agent-gopher/internal/paths"
)

const (
	DefaultModel         = "openrouter/free"
	DefaultContextWindow = 128000
)

type MCPServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Config struct {
	APIKey          string            `json:"api_key"`
	BaseURL         string            `json:"base_url"`
	Model           string            `json:"model"`
	Theme           string            `json:"theme"`
	ContextWindow   int               `json:"context_window"`
	PermissiveShell bool              `json:"permissive_shell"`
	PermissiveWrite bool              `json:"permissive_write"`
	WhitelistedCmds []string          `json:"whitelisted_cmds"`
	TavilyAPIKey    string            `json:"tavily_api_key"`
	MCPServers      []MCPServerConfig `json:"mcp_servers"`
}

// Env vars that override config values
var envOverrides = []struct {
	name string
	set  func(*Config, string)
}{
	{"GOPHER_API_KEY", func(c *Config, v string) { c.APIKey = v }},
	{"GOPHER_BASE_URL", func(c *Config, v string) { c.BaseURL = v }},
	{"GOPHER_MODEL", func(c *Config, v string) { c.Model = v }},
	{"TAVILY_API_KEY", func(c *Config, v string) { c.TavilyAPIKey = v }},
}

func Load() Config {
	cfg, err := read()
	created := false
	if os.IsNotExist(err) {
		cfg, err = createDefault()
		created = true
	}
	if err != nil {
		log.Fatal(err)
	}

	for _, o := range envOverrides {
		if v := os.Getenv(o.name); v != "" {
			o.set(&cfg, v)
		}
	}

	if cfg.APIKey == "" || cfg.BaseURL == "" {
		if created {
			log.Fatalf("Config file not found. A default one has been created at %s. Fill in api_key and base_url (or set GOPHER_API_KEY and GOPHER_BASE_URL) and run again.", paths.ConfigFile)
		}
		if cfg.APIKey == "" {
			log.Fatalf("api_key not set in %s or GOPHER_API_KEY", paths.ConfigFile)
		}
		log.Fatalf("base_url not set in %s or GOPHER_BASE_URL", paths.ConfigFile)
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Theme == "" {
		cfg.Theme = "default"
	}
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = DefaultContextWindow
	}
	if cfg.WhitelistedCmds == nil {
		cfg.WhitelistedCmds = []string{}
	}

	return cfg
}

// Reads the config file without env overrides or defaults
func read() (Config, error) {
	var cfg Config
	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, err
		}
		return cfg, fmt.Errorf("error reading config file: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("error parsing config file: %w", err)
	}
	return cfg, nil
}

func createDefault() (Config, error) {
	cfg := Config{
		Model:           DefaultModel,
		Theme:           "default",
		ContextWindow:   DefaultContextWindow,
		WhitelistedCmds: []string{},
		MCPServers:      []MCPServerConfig{},
	}
	if err := os.MkdirAll(paths.ConfigDir, 0755); err != nil {
		return cfg, fmt.Errorf("error creating config directory: %w", err)
	}
	if err := Save(cfg); err != nil {
		return cfg, fmt.Errorf("error writing default config: %w", err)
	}
	return cfg, nil
}

func Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(paths.ConfigFile, data, 0600)
}

// Adds a command to the config
func AddWhitelisted(cmd string) error {
	cfg, err := read()
	if err != nil {
		return err
	}
	if slices.Contains(cfg.WhitelistedCmds, cmd) {
		return nil
	}
	cfg.WhitelistedCmds = append(cfg.WhitelistedCmds, cmd)
	return Save(cfg)
}
