package orchestrator

import (
	"context"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// submitRuntimeInternalEvent is the only state-transition call available to
// Orchestrator mainline and adapters. It deliberately lives in Biz instead of
// a shared pkg: internal events are implementation details of the
// Orchestrator owner, not a cross-owner protocol.
func (m *Biz) submitRuntimeInternalEvent(ctx context.Context, event runtime.RuntimeInternalEvent) (runtime.RuntimeInternalEventResult, error) {
	if m == nil || m.runtimeController == nil || event.Scope.WorkspaceID == "" || event.Scope.RunID == "" {
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
		Kind: runtime.RuntimeInternalEventFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{FailureFact: &fact},
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeRejectCandidate(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, failure runtimecontract.RuntimeCandidateFailure) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateRejected, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, CandidateFailure: &failure},
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeCandidateStale(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidatePrerequisiteStaleObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, Stale: &stale},
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeCandidatePrerequisite(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, request runtimecontract.RuntimeOperationPrerequisiteRequest) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidatePrerequisiteObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, PrerequisiteRequest: &request},
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeOwnerFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtimeOwnerFeasibilityInternalEventKind(observation), Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{Attempt: attempt, OwnerFeasibility: &observation},
	})
	return result.Aggregate, err
}

func runtimeOwnerFeasibilityInternalEventKind(observation runtimecontract.RuntimeOwnerFeasibilityObservation) runtime.RuntimeInternalEventKind {
	if observation.SealedTargetAuthorizationContradicted() {
		return runtime.RuntimeInternalEventSemanticBindingContradictionObserved
	}
	return runtime.RuntimeInternalEventOwnerFeasibilityObserved
}

func (m *Biz) runtimeEnsureCandidateRequestWithExpected(ctx context.Context, aggregate runtime.RuntimeAggregate, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string, expected runtimecontract.RuntimeExpectedDocumentContract) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateRequestEnsured, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{DocumentKind: documentKind, StablePrefixHash: stablePrefixHash, ManifestHash: manifestHash, CorrectionHash: correctionHash, ExpectedDocument: &expected},
	})
	if err != nil || result.PendingAuthorityFailure == nil {
		return result.Aggregate, result.CandidateSession, err
	}
	terminal, terminalErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventFailureObserved,
		Scope: runtime.RuntimeInternalEventScope{
			WorkspaceID: aggregate.Facts.WorkspaceID,
			RunID:       aggregate.Facts.RunID,
		},
		Authority: &runtime.RuntimeInternalAuthorityPayload{FailureFact: result.PendingAuthorityFailure},
	})
	return terminal.Aggregate, runtimecontract.RuntimeCandidateRequestSession{}, terminalErr
}

func (m *Biz) runtimeCandidateSession(ctx context.Context, aggregate runtime.RuntimeAggregate, kind runtime.RuntimeInternalEventKind, logicalRequestID string, sequence int, physicalCallID string, stale *runtimecontract.RuntimePrerequisiteStale, budgetSettlement ...*runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: kind, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID, Stale: stale, ProviderBudgetSettlement: firstRuntimeProviderBudgetSettlement(budgetSettlement)}})
	return result.Aggregate, err
}

func (m *Biz) runtimeCoordinateProviderRequestTerminal(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID string, input runtime.RuntimeProviderRequestTerminalInput, budgetSettlement ...*runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, error) {
	fact, err := runtime.NormalizeRuntimeProviderRequestTerminal(input)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:  runtime.RuntimeInternalEventProviderRequestTerminalCoordinated,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &runtime.RuntimeInternalTransactionPayload{
			LogicalRequestID: logicalRequestID, ProviderRequestTerminal: &fact,
			ProviderBudgetSettlement: firstRuntimeProviderBudgetSettlement(budgetSettlement),
		},
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeCoordinateProviderRequestRecovery(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID, physicalCallID string, input runtime.RuntimeProviderRequestRecoveryInput, budgetSettlement ...*runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, error) {
	fact, err := runtime.NormalizeRuntimeProviderRequestRecovery(input)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventProviderRequestRecoveryCoordinated, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID, ProviderRequestRecovery: &fact, ProviderBudgetSettlement: firstRuntimeProviderBudgetSettlement(budgetSettlement)},
	})
	return result.Aggregate, err
}

func firstRuntimeProviderBudgetSettlement(values []*runtime.RuntimeProviderBudgetSettlement) *runtime.RuntimeProviderBudgetSettlement {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func runtimeProviderBudgetSettlement(workspaceID, runID, logicalRequestID, physicalCallID string, audit *runtimecontract.ProviderRequestAuditView, usage *llmaudit.ProviderUsage) (*runtime.RuntimeProviderBudgetSettlement, error) {
	value, err := runtime.NewRuntimeProviderBudgetSettlementFromAudit(workspaceID, runID, logicalRequestID, physicalCallID, audit, usage)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (m *Biz) runtimeCandidateDocument(ctx context.Context, aggregate runtime.RuntimeAggregate, kind runtime.RuntimeInternalEventKind, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate *runtimecontract.RuntimeCandidate, failure *runtimecontract.RuntimeCandidateFailure) (runtime.RuntimeInternalEventResult, error) {
	return m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: kind, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: candidate, CandidateFailure: failure}})
}

func (m *Biz) runtimeRecordCandidateFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, CandidateFeasibility: &record}})
	return result.Aggregate, err
}

func (m *Biz) runtimeConsumeRecoveryBudget(ctx context.Context, aggregate runtime.RuntimeAggregate, kind runtimecontract.RuntimeRecoveryBudgetKind) (runtime.RuntimeAggregate, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventRecoveryBudgetConsumed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryBudgetKind: kind}})
	return result.Aggregate, result.BudgetConsumed, err
}

// The following helpers keep SM-4 retry mutations behind the one owner-local
// internal-event dispatcher. The persisted lease is the retry authority;
// callers receive only its immutable identity, never repository/CAS access.
func (m *Biz) runtimeScheduleDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, lease runtimecontract.RuntimeDriveLease) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseScheduled, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{DriveLease: &lease},
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseChanged, err
	}
	return result.DriveLease.Clone(), result.LeaseChanged, err
}

func (m *Biz) runtimeClaimDriveLease(ctx context.Context, lease runtimecontract.RuntimeDriveLease, now string) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseClaimed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: lease.WorkspaceID, RunID: lease.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{DriveLease: &lease, LeaseNow: now},
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseClaimed, err
	}
	return result.DriveLease.Clone(), result.LeaseClaimed, err
}

func (m *Biz) runtimeReleaseDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, generation uint64) (bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseReleased, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{LeaseGeneration: generation},
	})
	return result.LeaseChanged, err
}

func (m *Biz) runtimeRecoverDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dueAt string) (runtimecontract.RuntimeDriveLease, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveLeaseRecovered, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{LeaseNow: dueAt},
	})
	if result.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, result.LeaseChanged, err
	}
	return result.DriveLease.Clone(), result.LeaseChanged, err
}

func (m *Biz) runtimeOpenRecoverableWait(ctx context.Context, wait runtimecontract.RuntimeRecoverableWaitRecord) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventRecoverableWaitOpened, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: wait.WorkspaceID, RunID: wait.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoverableWait: &wait},
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeClaimRecoverableWaitProbe(ctx context.Context, aggregate runtime.RuntimeAggregate, waitID string, leaseGeneration uint64, now string) (runtime.RuntimeAggregate, bool, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventRecoverableWaitProbeDue, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoverableWaitID: waitID, LeaseGeneration: leaseGeneration, LeaseNow: now},
	})
	return result.Aggregate, result.LeaseClaimed, err
}

func (m *Biz) runtimeDeferRecoverableWaitProbe(ctx context.Context, aggregate runtime.RuntimeAggregate, waitID, now string) (runtime.RuntimeAggregate, error) {
	probeResultHash := runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(waitID, runtimeRecoverableWaitProbeRequestHash(aggregate, waitID))
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventRecoverableWaitProbeDeferred, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoverableWaitID: waitID, RecoverableWaitProbeResultHash: probeResultHash, LeaseNow: now},
	})
	return result.Aggregate, err
}

func (m *Biz) runtimeExhaustRecoverableWait(ctx context.Context, aggregate runtime.RuntimeAggregate, waitID, code string) (runtime.RuntimeAggregate, error) {
	probeResultHash := runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(waitID, runtimeRecoverableWaitProbeRequestHash(aggregate, waitID))
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventRecoverableWaitExhausted, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoverableWaitID: waitID, RecoverableWaitProbeResultHash: probeResultHash, Reason: code},
	})
	return result.Aggregate, err
}

func runtimeRecoverableWaitProbeRequestHash(aggregate runtime.RuntimeAggregate, waitID string) string {
	for _, wait := range aggregate.RecoverableWaits {
		if wait.WaitID == waitID {
			return wait.ProbeRequestHash
		}
	}
	return ""
}

func (m *Biz) runtimeRetireRecoverableWait(ctx context.Context, aggregate runtime.RuntimeAggregate, waitID string) (runtime.RuntimeAggregate, error) {
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventRecoverableWaitRetired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoverableWaitID: waitID},
	})
	return result.Aggregate, err
}
