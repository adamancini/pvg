package lifecycle

import (
	"testing"
)

func TestTrackedAgentTypes_ContainsExpected(t *testing.T) {
	expected := []string{
		"paivot-graph:business-analyst",
		"paivot-graph:designer",
		"paivot-graph:architect",
		"paivot-graph:sr-pm",
		"paivot-graph:developer",
		"paivot-graph:pm",
	}
	for _, agentType := range expected {
		if !trackedAgentTypes[agentType] {
			t.Errorf("expected %q in trackedAgentTypes", agentType)
		}
	}
}

func TestTrackedAgentTypes_RejectsUntracked(t *testing.T) {
	untracked := []string{
		"paivot-graph:anchor",
		"paivot-graph:retro",
		"general-purpose",
	}
	for _, agentType := range untracked {
		if trackedAgentTypes[agentType] {
			t.Errorf("did not expect %q in trackedAgentTypes", agentType)
		}
	}
}
