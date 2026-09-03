package orchestrator

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimeMainlineHasNoSecondStageCandidateFeasibilityTransition(t *testing.T) {
	payload, err := os.ReadFile("runtime_mainline.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(payload)
	for _, forbidden := range []string{"runtimeCandidateStale(", "runtimeCandidatePrerequisite(", "runtimeOwnerFeasibility("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("runtime mainline retains split candidate feasibility transition %q", forbidden)
		}
	}
	if !strings.Contains(source, "runtimeRecordCandidateFeasibility(") {
		t.Fatal("runtime mainline no longer submits the single candidate feasibility fact")
	}
}
