package config

import (
	"os"
	"strings"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/paths"
)

func TestAddWhitelistedDedupesAndSkipsEnv(t *testing.T) {
	paths.InitWithBase(t.TempDir(), "/tmp/proj")
	t.Cleanup(func() { paths.InitWithBase("", "") })
	if err := os.MkdirAll(paths.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Save(Config{APIKey: "file-key", BaseURL: "u"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPHER_API_KEY", "env-secret")
	cfg := Load()
	if cfg.APIKey != "env-secret" {
		t.Errorf("env override not applied: %q", cfg.APIKey)
	}

	for range 2 {
		if err := AddWhitelisted("echo"); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(paths.ConfigFile)
	if strings.Contains(string(data), "env-secret") {
		t.Error("env value written to config file")
	}
	if n := strings.Count(string(data), `"echo"`); n != 1 {
		t.Errorf("echo saved %d times, want 1", n)
	}
	if st, _ := os.Stat(paths.ConfigFile); st.Mode().Perm() != 0o600 {
		t.Errorf("config mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestEnvOverridesConfig(t *testing.T) {
	paths.InitWithBase(t.TempDir(), "/tmp/proj")
	t.Cleanup(func() { paths.InitWithBase("", "") })
	os.MkdirAll(paths.ConfigDir, 0o755)
	if err := Save(Config{APIKey: "fk", BaseURL: "fu", Model: "fm", TavilyAPIKey: "ft"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPHER_API_KEY", "ek")
	t.Setenv("GOPHER_BASE_URL", "eu")
	t.Setenv("GOPHER_MODEL", "em")
	t.Setenv("TAVILY_API_KEY", "et")

	cfg := Load()
	if cfg.APIKey != "ek" || cfg.BaseURL != "eu" || cfg.Model != "em" || cfg.TavilyAPIKey != "et" {
		t.Errorf("env did not take priority: %+v", cfg)
	}
}
