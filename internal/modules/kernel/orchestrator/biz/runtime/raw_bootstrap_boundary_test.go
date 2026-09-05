package runtime

import (
	"context"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerInitializeRejectsRawActivationBeforeNormalizeOrPersist(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{
		ID: "policy-1", AppliesTo: runtimecontract.StepKindFileWrite,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "dod-1", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation, ""},
		}},
	}}
	_, err = controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("Runtime initialization accepted noncanonical activation authority: %v", err)
	}
	if repository.saves != 0 || repository.aggregate != nil {
		t.Fatalf("rejected Runtime initialization persisted aggregate: saves=%d aggregate=%+v", repository.saves, repository.aggregate)
	}
}
