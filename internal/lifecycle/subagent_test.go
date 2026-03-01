package lifecycle

import (
	"testing"
)

func TestBltAgentTypes_ContainsExpected(t *testing.T) {
	expected := []string{
		"paivot-graph:business-analyst",
		"paivot-graph:designer",
		"paivot-graph:architect",
	}
	for _, agentType := range expected {
		if !bltAgentTypes[agentType] {
			t.Errorf("expected %q in bltAgentTypes", agentType)
		}
	}
}

func TestBltAgentTypes_RejectsNonBLT(t *testing.T) {
	nonBLT := []string{
		"paivot-graph:developer",
		"paivot-graph:sr-pm",
		"paivot-graph:pm",
		"paivot-graph:anchor",
		"paivot-graph:retro",
		"general-purpose",
	}
	for _, agentType := range nonBLT {
		if bltAgentTypes[agentType] {
			t.Errorf("did not expect %q in bltAgentTypes", agentType)
		}
	}
}
