package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// driveRuntimeImplementationWorkScope re-drives only a durable SM-2 scope
// reservation. It never selects a CWI, creates a round or authorizes effects.
func (m *Biz) driveRuntimeImplementationWorkScope(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, bool, error) {
	round, err := runtime.CurrentRuntimeImplementationWorkReservation(aggregate)
	if err != nil {
		return runtime.RuntimeAggregate{}, false, err
	}
	preview := aggregate.ImplementationWork.ScopePreview
	if round == nil && preview == nil {
		request, requestErr := runtime.NextRuntimeImplementationWorkScopeRequest(aggregate)
		if requestErr != nil || request == nil {
			return aggregate, false, requestErr
		}
		// SM-4 只递送 SM-2 的只读 scope 需求；先 CAS，再 owner query。
		updated, reserveErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:      runtime.RuntimeInternalEventImplementationWorkScopeRequired,
			Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
			Authority: &runtime.RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: request},
		})
		return updated.Aggregate, true, reserveErr
	}
	var request runtimecontract.RuntimeImplementationWorkScopeRequest
	if preview != nil {
		if preview.Observation != nil {
			return aggregate, false, nil
		}
		request = preview.Request
	} else {
		if round.ScopeObservation != nil {
			return aggregate, false, nil
		}
		request = round.ScopeRequest
	}
	sources, err := runtime.RuntimeImplementationWorkScopeForCurrentSources(aggregate, request.CoverageWorkItemID)
	if err != nil {
		return runtime.RuntimeAggregate{}, true, err
	}
	workspace, err := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, true, err
	}
	observation, err := acquireRuntimeImplementationWorkScope(ctx, m.EventHub(), workspace.DeliverableDirectory, request, sources)
	if err != nil {
		return runtime.RuntimeAggregate{}, true, err
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:      runtime.RuntimeInternalEventImplementationWorkScopeObserved,
		Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{ImplementationWorkScopeObservation: &observation},
	})
	return updated.Aggregate, true, err
}

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

	// Capture full bytes within the original complete selection, then re-read the
	// manifest. No partial/truncated body may be labelled complete. The private
	// scope CAS freezes this same input for attempt 1 and process recovery.
	if result.Selection.Disposition == runtimecontract.RuntimeTargetSelectionSelected || result.Selection.Disposition == runtimecontract.RuntimeTargetSelectionScope {
		content := &runtimecontract.RuntimeImplementationWorkContent{ProtocolRef: runtimecontract.RuntimeImplementationWorkContentV1, ManifestRevision: result.Selection.ObservedRevision}
		remaining := runtimecontract.RuntimeImplementationWorkContentMaxBytes
		for _, target := range result.Selection.TargetRefs {
			if remaining <= 0 {
				return observation, fmt.Errorf("implementation_work_content_capacity_exceeded: full target content not acquired")
			}
			snapshot, err := AcquireTargetSnapshotPrerequisite(ctx, hub, fileevents.AcquireTargetSnapshotCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID, PrerequisiteID: request.RequestID, WorkspaceDeliverableDirectory: directory, TargetRef: target, MaxBytes: remaining})
			if err != nil {
				return observation, fmt.Errorf("implementation_work_content_unavailable: %w", err)
			}
			content.Snapshots = append(content.Snapshots, snapshot.Snapshot)
			remaining -= len(snapshot.Snapshot.Content)
		}
		fresh, err := AcquireWorkspaceManifestPrerequisite(ctx, hub, fileevents.AcquireWorkspaceManifestCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID, PrerequisiteID: request.RequestID, WorkspaceDeliverableDirectory: directory})
		if err != nil {
			return observation, err
		}
		if fresh.Manifest.Revision != content.ManifestRevision {
			return observation, runtimecontract.ErrRuntimeImplementationWorkScopeStale
		}
		observation.Content = content
		if err := observation.ValidateForSources(sources); err != nil {
			return observation, err
		}
	}
	return observation.Clone(), nil
}
