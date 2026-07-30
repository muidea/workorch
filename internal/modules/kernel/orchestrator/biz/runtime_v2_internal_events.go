package orchestrator

import (
	"context"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// submitRuntimeV2InternalEvent is the only state-transition call available to
// Orchestrator mainline and adapters. It deliberately lives in Biz instead of
// a shared pkg: internal events are implementation details of the
// Orchestrator owner, not a cross-owner protocol.
func (m *Biz) submitRuntimeV2InternalEvent(ctx context.Context, event runtimev2.RuntimeInternalEvent) (runtimev2.RuntimeInternalEventResult, error) {
	if m == nil || m.bizPtr == nil || event.WorkspaceID == "" || event.RunID == "" {
		return runtimev2.RuntimeInternalEventResult{}, common.ErrInvalidArgument
	}
	return m.bizPtr.ApplyRuntimeV2InternalEvent(ctx, event)
}

func (m *Biz) runtimeV2RecordFailure(ctx context.Context, aggregate runtimev2.RuntimeAggregate, fact runtimecontract.FailureFact) (runtimev2.RuntimeAggregate, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventFailureObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, FailureFact: &fact,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeV2RejectCandidate(ctx context.Context, aggregate runtimev2.RuntimeAggregate, attempt int, failure runtimecontract.RuntimeCandidateFailure) (runtimev2.RuntimeAggregate, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventCandidateRejected, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, CandidateFailure: &failure,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeV2CandidateStale(ctx context.Context, aggregate runtimev2.RuntimeAggregate, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (runtimev2.RuntimeAggregate, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventCandidatePrerequisiteStaleObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, Stale: &stale,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeV2CandidatePrerequisite(ctx context.Context, aggregate runtimev2.RuntimeAggregate, attempt int, request runtimecontract.RuntimeOperationPrerequisiteRequest) (runtimev2.RuntimeAggregate, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventCandidatePrerequisiteObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, PrerequisiteRequest: &request,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeV2OwnerFeasibility(ctx context.Context, aggregate runtimev2.RuntimeAggregate, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtimev2.RuntimeAggregate, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventOwnerFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, OwnerFeasibility: &observation,
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeV2EnsureCandidateRequest(ctx context.Context, aggregate runtimev2.RuntimeAggregate, stablePrefixHash, manifestHash, correctionHash string) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateRequestEnsured, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, StablePrefixHash: stablePrefixHash, ManifestHash: manifestHash, CorrectionHash: correctionHash})
	return result.Aggregate, result.CandidateSession, err
}

func (m *Biz) runtimeV2CandidateSession(ctx context.Context, aggregate runtimev2.RuntimeAggregate, kind runtimev2.RuntimeInternalEventKind, logicalRequestID string, sequence int, physicalCallID string, outcome *runtimecontract.RuntimeCandidateRequestOutcome, fragment *runtimecontract.RuntimeCandidateRequestFragment, stale *runtimecontract.RuntimePrerequisiteStale) (runtimev2.RuntimeAggregate, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: kind, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID, RequestOutcome: outcome, RequestFragment: fragment, Stale: stale})
	return result.Aggregate, err
}

func (m *Biz) runtimeV2CandidateDocument(ctx context.Context, aggregate runtimev2.RuntimeAggregate, kind runtimev2.RuntimeInternalEventKind, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate *runtimecontract.RuntimeCandidate, failure *runtimecontract.RuntimeCandidateFailure) (runtimev2.RuntimeInternalEventResult, error) {
	return m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: kind, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: candidate, CandidateFailure: failure})
}

func (m *Biz) runtimeV2RecordCandidateFeasibility(ctx context.Context, aggregate runtimev2.RuntimeAggregate, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (runtimev2.RuntimeAggregate, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Attempt: attempt, CandidateFeasibility: &record})
	return result.Aggregate, err
}

func (m *Biz) runtimeV2ConsumeRecoveryBudget(ctx context.Context, aggregate runtimev2.RuntimeAggregate, kind runtimecontract.RuntimeRecoveryBudgetKind) (runtimev2.RuntimeAggregate, bool, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventRecoveryBudgetConsumed, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, RecoveryBudgetKind: kind})
	return result.Aggregate, result.BudgetConsumed, err
}

// The following helpers keep SM-4 retry mutations behind the one owner-local
// internal-event dispatcher. The persisted lease is the retry authority;
// callers receive only its immutable identity, never repository/CAS access.
func (m *Biz) runtimeV2ScheduleDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, lease runtimecontract.RuntimeDriveLease) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventDriveLeaseScheduled, WorkspaceID: workspaceID, RunID: runID, DriveLease: &lease,
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseChanged, err
	}
	return result.DriveLease.Clone(), result.LeaseChanged, err
}

func (m *Biz) runtimeV2ClaimDriveLease(ctx context.Context, lease runtimecontract.RuntimeDriveLease, now string) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventDriveLeaseClaimed, WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, DriveLease: &lease, LeaseNow: now,
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseClaimed, err
	}
	return result.DriveLease.Clone(), result.LeaseClaimed, err
}

func (m *Biz) runtimeV2ReleaseDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, generation uint64) (bool, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventDriveLeaseReleased, WorkspaceID: workspaceID, RunID: runID, LeaseGeneration: generation,
	})
	return result.LeaseChanged, err
}

func (m *Biz) runtimeV2RecoverDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dueAt string) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventDriveLeaseRecovered, WorkspaceID: workspaceID, RunID: runID, LeaseNow: dueAt,
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseChanged, err
	}
	return result.DriveLease.Clone(), result.LeaseChanged, err
}
