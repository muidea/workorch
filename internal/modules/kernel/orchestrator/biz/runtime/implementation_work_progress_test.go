package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestImplementationWorkScopePrivateCASChangesConvergenceProgress(t *testing.T) {
	controller, repository, before := implementationWorkPurposeController(t)
	request, err := NextRuntimeImplementationWorkScopeRequest(before)
	if err != nil || request == nil {
		t.Fatalf("initial work scope: request=%v err=%v", request, err)
	}
	result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:      RuntimeInternalEventImplementationWorkScopeRequired,
		Scope:     RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: request},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != 2 || result.Aggregate.StateRevision != before.StateRevision+1 {
		t.Fatal("scope reservation did not use one aggregate CAS")
	}
	if RuntimeConvergenceProgressSignature(before) == RuntimeConvergenceProgressSignature(result.Aggregate) {
		t.Fatal("persisted work scope reservation was mistaken for retry bookkeeping")
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: request.RequestID, OwnerModuleID: request.Policy.OwnerModuleID, TargetRole: "implementation_work",
		SelectorRef: request.Policy.TargetSelectorRef, SelectorOwnerRef: request.Policy.TargetSelectorOwnerRef, SelectorSubjectRef: request.Policy.TargetSelectorSubjectRef,
		SelectorCandidateRefs: request.Policy.TargetSelectorCandidateRefs, Disposition: runtimecontract.RuntimeTargetSelectionSelected,
		CandidateCount: 1, CandidateTargetRefs: []string{"target.txt"}, TargetRefs: []string{"target.txt"}, ObservedRevision: request.ManifestBinding.Manifest.Revision,
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := request.ManifestBinding.Observation
	observation.PrerequisiteID, observation.Kind, observation.ContentHash = request.RequestID, runtimecontract.PrerequisiteKindTargetSelection, selection.SnapshotID
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	scope := runtimecontract.RuntimeImplementationWorkScopeObservation{Request: *request, Selection: selection, Observation: observation}
	observed, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:      RuntimeInternalEventImplementationWorkScopeObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeObservation: &scope},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != 3 || observed.Aggregate.StateRevision != result.Aggregate.StateRevision+1 {
		t.Fatal("scope observation did not use one aggregate CAS")
	}
	if RuntimeConvergenceProgressSignature(result.Aggregate) == RuntimeConvergenceProgressSignature(observed.Aggregate) {
		t.Fatal("persisted owner scope observation was mistaken for retry bookkeeping")
	}
	fresh, err := controller.Load(context.Background(), before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil || !reflect.DeepEqual(fresh, observed.Aggregate) || RuntimeConvergenceProgressSignature(fresh) != RuntimeConvergenceProgressSignature(observed.Aggregate) {
		t.Fatalf("same-version reload changed scope progress: %v", err)
	}
}
