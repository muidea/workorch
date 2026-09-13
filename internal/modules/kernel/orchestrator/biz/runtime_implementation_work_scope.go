package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// acquireRuntimeImplementationWorkScope is a read-only owner adapter. It does
// not commit terminal-proof TargetSelection, select work, or bypass activation.
// The working-binding private event/consumer remains a separate requirement.
func acquireRuntimeImplementationWorkScope(ctx context.Context, hub magicevent.Hub, directory string, request runtimecontract.RuntimeImplementationWorkScopeRequest, sources runtimecontract.RuntimeImplementationWorkScopeSources) (runtimecontract.RuntimeImplementationWorkScopeObservation, error) {
	if err := request.ValidateForSources(sources); err != nil {
		return runtimecontract.RuntimeImplementationWorkScopeObservation{}, err
	}
	// Concrete routing is adapter-owned; Core does not specialize on File.
	if request.Policy.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		return runtimecontract.RuntimeImplementationWorkScopeObservation{}, fmt.Errorf("implementation work scope owner %q has no target-selection adapter", request.Policy.OwnerModuleID)
	}
	p := request.Policy
	result, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, hub, fileevents.AcquireRuntimeTargetSelectionCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID,
		PrerequisiteID: request.RequestID, WorkspaceDeliverableDirectory: directory, TargetRole: "implementation_work",
		TargetSelectorRef: p.TargetSelectorRef, TargetSelectorOwnerRef: p.TargetSelectorOwnerRef, TargetSelectorSubjectRef: p.TargetSelectorSubjectRef,
		TargetSelectorCandidateRefs:          append([]string(nil), p.TargetSelectorCandidateRefs...),
		ProspectiveTargetAllOfConstraintRefs: append([]string(nil), p.ProspectiveTargetAllOfConstraintRefs...),
		// Work target cardinality is not an observation limit. Truncating to the
		// per-round limit could turn an ambiguous scope into a unique target.
		MaxItems: runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
	})
	if err != nil {
		return runtimecontract.RuntimeImplementationWorkScopeObservation{}, err
	}
	observation := runtimecontract.RuntimeImplementationWorkScopeObservation{
		Request: request.Clone(), Selection: result.Selection, Observation: result.Observation,
	}
	if err := observation.ValidateForSources(sources); err != nil {
		return runtimecontract.RuntimeImplementationWorkScopeObservation{}, err
	}
	return observation.Clone(), nil
}
