package orchestrator

import (
	"context"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This proves inventory acquisition only, not work dispatch or CWI acceptance.
func TestImplementationWorkInventoryAcquiresRealFileOwnerFacts(t *testing.T) {
	policy := runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations:     []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileWrite}},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		TargetSelectorRef:     "work", TargetSelectorOwnerRef: "profile", TargetSelectorSubjectRef: "implementation",
		TargetSelectorCandidateRefs: []string{"resource-a"}, MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1,
		RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
	focuses := []runtimecontract.RuntimeDefinitionFocus{{ID: "focus-1", AssessmentScope: "dodItem", AssessmentID: "dod-1",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, ImplementationWorkPolicy: &policy}}
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "dod-1", Scope: "dodItem", ImplementationWorkPolicy: &policy,
	}}}}
	requirements, err := runtimecontract.CompileRuntimeImplementationWorkCapabilityRequirements(focuses, profiles)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner := filemodule.New()
	if err := owner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Stop(ctx)
	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-work", RunID: "run-work", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}, requirements)
	if len(facts.Prerequisites.Pending) != 1 {
		t.Fatalf("unexpected prerequisites: %#v", facts.Prerequisites)
	}
	prerequisite := facts.Prerequisites.Pending[0]
	committed, err := AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx, hub, facts, runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1", PrerequisiteID: prerequisite.ID,
		OwnerModuleID: prerequisite.OwnerModuleID, CapabilityClass: prerequisite.CapabilityClass, RequiredKinds: prerequisite.RequiredCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(committed.Resources.PrerequisiteObservations, prerequisite.ID)
	if !ok || observation.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		!reflect.DeepEqual(committed.Capabilities.StateChangeKinds, []string{runtimecontract.StepKindFileWrite}) || len(committed.Capabilities.ValidationKinds) != 0 {
		t.Fatalf("work capability acquisition crossed owner/class boundary: %#v", committed.Capabilities)
	}
}
