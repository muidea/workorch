package orchestrator

// Owner Biz prerequisite tests.

import (
	"strings"
	"testing"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestLocalPrerequisiteAdaptersCommitOwnerFacts(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus:              runtimecontract.CoverageFocus{ID: "cwi-1"},
		Failure: &runtimecontract.FailureFact{
			Signature:  "build-failure",
			TargetRefs: []string{"main.go"},
		},
	}
	requirements := []runtimev2.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindFailureTarget},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch},
	}
	facts = runtimev2.PreflightRuntimeFacts(facts, requirements)
	for _, item := range append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...) {
		var err error
		switch item.Kind {
		case runtimecontract.PrerequisiteKindFailureTarget:
			facts, err = ResolveFailureTargetPrerequisite(facts, item.ID)
		case runtimecontract.PrerequisiteKindValidationScratch:
			facts, err = AcquireValidationScratchPrerequisite(facts, item.ID)
		}
		if err != nil {
			t.Fatalf("resolve %s: %v", item.Kind, err)
		}
	}
	if facts.Resources.FailureTarget == nil || len(facts.Resources.FailureTarget.TargetRefs) != 1 {
		t.Fatalf("failure target = %+v", facts.Resources.FailureTarget)
	}
	if facts.Resources.ValidationScratch == nil || !strings.Contains(facts.Resources.ValidationScratch.Reference, "workorch-validation-scratch-v2") {
		t.Fatalf("validation scratch = %+v", facts.Resources.ValidationScratch)
	}
	facts = runtimev2.PreflightRuntimeFacts(facts, requirements)
	if len(facts.Prerequisites.Pending) != 0 {
		t.Fatalf("resolved prerequisites scheduled again: %+v", facts.Prerequisites.Pending)
	}
}
