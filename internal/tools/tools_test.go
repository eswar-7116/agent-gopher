package tools

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := Registry(Options{})

	for name, tool := range reg {
		if tool == nil {
			t.Errorf("tool %q is nil in registry", name)
			continue
		}

		if tool.Name() != name {
			t.Errorf("registry key %q does not match tool.Name() %q", name, tool.Name())
		}

		// Check if the definition can be generated with no errors
		_ = tool.Definition()
	}
}

func TestRegistryWebSearchNeedsKey(t *testing.T) {
	if _, ok := Registry(Options{})["web_search"]; ok {
		t.Error("web_search registered without a Tavily key")
	}
	if _, ok := Registry(Options{TavilyAPIKey: "k"})["web_search"]; !ok {
		t.Error("web_search missing with a Tavily key")
	}
}

func TestDefinitions(t *testing.T) {
	reg := Registry(Options{})
	defs := Definitions(reg)

	if len(defs) == 0 {
		t.Errorf("expected Definitions to not be empty")
	}

	if len(defs) != len(reg) {
		t.Errorf("expected Definitions length %d to match Registry length %d", len(defs), len(reg))
	}
	for i := 1; i < len(defs); i++ {
		if defs[i-1].GetFunction().Name > defs[i].GetFunction().Name {
			t.Fatalf("definitions not sorted: %q before %q", defs[i-1].GetFunction().Name, defs[i].GetFunction().Name)
		}
	}
}
