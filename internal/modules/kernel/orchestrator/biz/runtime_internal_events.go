package orchestrator

import (
	"context"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// submitRuntimeInternalEvent is the only state-transition call available to
// Orchestrator mainline and adapters. It deliberately lives in Biz instead of
// a shared pkg: internal events are implementation details of the
// Orchestrator owner, not a cross-owner protocol.
func (m *Biz) submitRuntimeInternalEvent(ctx context.Context, event runtime.RuntimeInternalEvent) (runtime.RuntimeInternalEventResult, error) {
	if m == nil || m.runtimeController == nil || event.WorkspaceID == "" || event.RunID == "" {
		return runtime.RuntimeInternalEventResult{}, common.ErrInvalidArgument
	}
	result, err := m.runtimeController.ApplyInternalEvent(ctx, event)
	if err == nil && result.Aggregate.Facts.WorkspaceID != "" {
		// The reducer remains the sole Runtime state owner. This immutable
		// observation only invalidates application read projections so TUI/SSE
		// consumers can follow committed internal progress during a long drive.
		m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
			WorkspaceID:       result.Aggregate.Facts.WorkspaceID,
			RunID:             result.Aggregate.Facts.RunID,
			Kind:              orchestratorevents.RuntimeDriveObservationStateCommitted,
			AggregateRevision: result.Aggregate.StateRevision,
			Source:            "runtime_internal_event",
		})
	}
	return result, err
}

func (m *Biz) runtimeRecordFailure(ctx context.Context, aggregate runtime.RuntimeAggregate, fact runtimecontract.FailureFact) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventFailureObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, FailureFact: &fact,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeRejectCandidate(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, failure runtimecontract.RuntimeCandidateFailure) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateRejected, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, CandidateFailure: &failure,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeCandidateStale(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidatePrerequisiteStaleObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, Stale: &stale,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeCandidatePrerequisite(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, request runtimecontract.RuntimeOperationPrerequisiteRequest) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidatePrerequisiteObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, PrerequisiteRequest: &request,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeOwnerFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventOwnerFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, OwnerFeasibility: &observation,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeEnsureCandidateRequestWithExpected(ctx context.Context, aggregate runtime.RuntimeAggregate, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string, expected runtimecontract.RuntimeExpectedDocumentContract) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateRequestEnsured, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DocumentKind: documentKind, StablePrefixHash: stablePrefixHash, ManifestHash: manifestHash, CorrectionHash: correctionHash, ExpectedDocument: &expected,
	})
	return result.Aggregate, result.CandidateSession, err
}

func (m *Biz) runtimeCandidateSession(ctx context.Context, aggregate runtime.RuntimeAggregate, kind runtime.RuntimeInternalEventKind, logicalRequestID string, sequence int, physicalCallID string, outcome *runtimecontract.RuntimeCandidateRequestOutcome, stale *runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: kind, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID, RequestOutcome: outcome, Stale: stale})
	return result.Aggregate, err
}

func (m *Biz) runtimeCandidateRequestRecoveryRequired(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID, physicalCallID string, outcome runtimecontract.RuntimeProviderDocumentOutcome) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventCandidateRequestRecoveryRequired,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID, ProviderDocumentOutcome: &outcome,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeCandidateDocument(ctx context.Context, aggregate runtime.RuntimeAggregate, kind runtime.RuntimeInternalEventKind, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate *runtimecontract.RuntimeCandidate, failure *runtimecontract.RuntimeCandidateFailure) (runtime.RuntimeInternalEventResult, error) {
	return m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: kind, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: candidate, CandidateFailure: failure})
}

func (m *Biz) runtimeRecordCandidateFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, CandidateFeasibility: &record})
	return result.Aggregate, err
}

func (m *Biz) runtimeConsumeRecoveryBudget(ctx context.Context, aggregate runtime.RuntimeAggregate, kind runtimecontract.RuntimeRecoveryBudgetKind) (runtime.RuntimeAggregate, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventRecoveryBudgetConsumed, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, RecoveryBudgetKind: kind})
	return result.Aggregate, result.BudgetConsumed, err
}

// The following helpers keep SM-4 retry mutations behind the one owner-local
// internal-event dispatcher. The persisted lease is the retry authority;
// callers receive only its immutable identity, never repository/CAS access.
func (m *Biz) runtimeScheduleDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, lease runtimecontract.RuntimeDriveLease) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseScheduled, WorkspaceID: workspaceID, RunID: runID, DriveLease: &lease,
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseChanged, err
	}
	return result.DriveLease.Clone(), result.LeaseChanged, err
}

func (m *Biz) runtimeClaimDriveLease(ctx context.Context, lease runtimecontract.RuntimeDriveLease, now string) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseClaimed, WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, DriveLease: &lease, LeaseNow: now,
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseClaimed, err
	}
	return result.DriveLease.Clone(), result.LeaseClaimed, err
}

func (m *Biz) runtimeReleaseDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, generation uint64) (bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseReleased, WorkspaceID: workspaceID, RunID: runID, LeaseGeneration: generation,
	})
	return result.LeaseChanged, err
}

func (m *Biz) runtimeRecoverDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dueAt string) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseRecovered, WorkspaceID: workspaceID, RunID: runID, LeaseNow: dueAt,
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseChanged, err
	}
	return result.DriveLease.Clone(), result.LeaseChanged, err
}
