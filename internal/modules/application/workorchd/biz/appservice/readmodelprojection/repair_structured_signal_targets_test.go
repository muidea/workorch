package readmodelprojection

import (
	"testing"
)

func TestReadModelFallbackFailureTargetRefsUsesProgressFailureTargetScope(t *testing.T) {
	output := map[string]any{
		"currentExecutionProgress": map[string]any{
			"lastObservedFailure": map[string]any{
				"failureTargetScope": "failureTargets=target.txt",
			},
		},
	}

	targets := readModelFallbackFailureTargetRefs(output, "class=runtime_failed")

	if len(targets) != 1 || targets[0] != "target.txt" {
		t.Fatalf("targetRefs = %#v, want target from progress failure target scope", targets)
	}
}

func TestReadModelFailureTargetScopeUsesStructuredScope(t *testing.T) {
	step := &StepView{
		Output: map[string]any{
			"failureTargetScope": "failureTargets=target.txt",
		},
	}

	if got := readModelFailureTargetScopeFromStep(step); got != "target.txt" {
		t.Fatalf("failureTargetScope = %q, want target from structured scope", got)
	}
}
