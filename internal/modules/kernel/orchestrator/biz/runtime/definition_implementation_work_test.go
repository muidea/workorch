package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerRejectsInjectedDefinitionImplementationWorkWithoutWrite(t *testing.T) {
	policy := runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations:     []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileWrite}},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		TargetSelectorRef:     "work", TargetSelectorOwnerRef: "profile", TargetSelectorSubjectRef: "implementation",
		TargetSelectorCandidateRefs: []string{"resource-a"}, MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1,
		RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	facts, definition := controllerFacts(), controllerDefinition()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.initialize(context.Background(), facts, definition, nil); err != nil {
		t.Fatal(err)
	}
	// A valid policy inserted only into persisted Definition cannot bypass the
	// absent sealed Profile source, even though no execution gate sees it there.
	repository.aggregate.Facts.DefinitionFocuses[0].ImplementationWorkPolicy = &policy
	if err := repository.aggregate.Facts.Validate(); err == nil || !strings.Contains(err.Error(), "implementation work differs from its exact sealed Profile source") {
		t.Fatalf("facts load accepted an injected work declaration: %v", err)
	}
	before, err := json.Marshal(repository.aggregate)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		restarted, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		_, err = restarted.initialize(context.Background(), facts, definition, nil)
		if !errors.Is(err, common.ErrConflict) {
			t.Fatalf("injected work declaration survived restart: %v", err)
		}
		// Even matching the caller's copy to the corrupted Definition cannot
		// satisfy the independent Profile-source check before the load/CAS.
		injected := facts.Normalized()
		injected.DefinitionFocuses[0].ImplementationWorkPolicy = &policy
		_, err = restarted.initialize(context.Background(), injected, definition, nil)
		if err == nil || !strings.Contains(err.Error(), "implementation work differs from its exact sealed Profile source") {
			t.Fatalf("matching caller copy bypassed sealed Profile source: %v", err)
		}
		after, err := json.Marshal(repository.aggregate)
		if err != nil {
			t.Fatal(err)
		}
		if repository.saves != 1 || string(before) != string(after) {
			t.Fatal("restart rewrote state or reserved work for an injected policy")
		}
	}
}
