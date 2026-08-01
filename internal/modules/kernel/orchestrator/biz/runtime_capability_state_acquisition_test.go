package orchestrator

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCapabilityStateAcquisitionGenerationTracksAffectedPrerequisiteOnly(t *testing.T) {
	const prerequisiteID = "repository-state"
	baseGeneration := runtimeCapabilityStateAcquisitionGeneration(nil, prerequisiteID)
	baseKey := stableRuntimeCapabilityStateAcquisitionKey("ws-v2", "run-v2", "repository-state", baseGeneration)
	if baseGeneration != 0 || baseKey != "capability-state/ws-v2/run-v2/repository-state/0" {
		t.Fatalf("base generation=%d key=%q", baseGeneration, baseKey)
	}

	stale := []runtimecontract.RuntimePrerequisiteInvalidationRecord{{Signature: "stale-1", PrerequisiteIDs: []string{prerequisiteID}, ObservationIDs: []string{"observation-1"}, Stale: &runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisiteID, ObservationID: "observation-1", Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit, SubjectRef: "workspace_deliverable", ExpectedRevision: "v1", ObservedRevision: "v2",
	}}}
	if got := runtimeCapabilityStateAcquisitionGeneration(stale, prerequisiteID); got != baseGeneration+1 {
		t.Fatalf("stale generation=%d, want %d", got, baseGeneration+1)
	}

	resourceChange := []runtimecontract.RuntimePrerequisiteInvalidationRecord{{Signature: "resource-change-1", PrerequisiteIDs: []string{prerequisiteID}, ObservationIDs: []string{"observation-1"}, ReceiptHash: "receipt-1", ResourceChanges: []runtimecontract.RuntimeResourceChange{{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit, SubjectRef: "workspace_deliverable", AfterRevision: "v2"}}}}
	if got := runtimeCapabilityStateAcquisitionGeneration(resourceChange, prerequisiteID); got != baseGeneration+1 {
		t.Fatalf("resource-change generation=%d, want %d", got, baseGeneration+1)
	}

	both := append(stale, resourceChange...)
	if got := runtimeCapabilityStateAcquisitionGeneration(both, prerequisiteID); got != 2 {
		t.Fatalf("combined generation=%d, want 2", got)
	}

	unrelated := []runtimecontract.RuntimePrerequisiteInvalidationRecord{{Signature: "resource-change-other", PrerequisiteIDs: []string{"other-prerequisite"}, ObservationIDs: []string{"other-observation"}, ReceiptHash: "receipt-other", ResourceChanges: []runtimecontract.RuntimeResourceChange{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", AfterRevision: "v2"}}}}
	if got := runtimeCapabilityStateAcquisitionGeneration(unrelated, prerequisiteID); got != baseGeneration {
		t.Fatalf("unrelated recovery changed generation to %d", got)
	}
}
