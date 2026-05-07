package tools

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := Registry()

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

func TestDefinitions(t *testing.T) {
	defs := Definitions()

	if len(defs) == 0 {
		t.Errorf("expected Definitions to not be empty")
	}

	if len(defs) != len(Registry()) {
		t.Errorf("expected Definitions length %d to match Registry length %d", len(defs), len(Registry()))
	}
}
