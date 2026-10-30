package runtime

import (
	"context"
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
)

// CompileRuntimeSemanticRequestRepairProof reads only the exact persisted
// legacy transaction. It cannot repair an ordinary semantic ambiguity, alter
// effects, reset budgets or infer a request from projection/diagnostic text.
func CompileRuntimeSemanticRequestRepairProof(a RuntimeAggregate) (runtimecontract.RuntimeSemanticRequestRepairProof, error) {
	s, l := a.DefinitionClarification, a.SemanticPlanning
	if s == nil || s.Dispatch == nil || s.Request == nil || l.Manifest == nil || l.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen || a.ActiveDispatch != nil || a.Termination != nil || a.Facts.Failure != nil || a.Facts.RepairDirective != nil || a.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || l.Attempt != 0 || len(a.GovernanceResolutions) != 0 || len(l.History) >= runtimecontract.RuntimeSemanticPlanningHistoryLimit {
		return runtimecontract.RuntimeSemanticRequestRepairProof{}, common.ErrConflict
	}
	if s.Observation.WorkspaceID != a.Facts.WorkspaceID || s.Observation.RunID != a.Facts.RunID || s.Observation.DefinitionRevision != a.Definition.Revision || s.Observation.StartupInputHash != a.Definition.StartupInputHash || s.Dispatch.AuthoritySnapshotID != a.Decision.AuthoritySnapshot.ID {
		return runtimecontract.RuntimeSemanticRequestRepairProof{}, common.ErrConflict
	}
	var expected *runtimecontract.RuntimeExpectedDocumentContract
	for _, session := range a.CandidateRequestSessions {
		if session.ProviderSubmitted && session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
			return runtimecontract.RuntimeSemanticRequestRepairProof{}, common.ErrConflict
		}
		if session.LogicalRequestID == s.Observation.LogicalRequestID {
			if expected != nil || session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.ExpectedDocument == nil || session.Transaction.AuthoritySnapshotID != s.Observation.AuthoritySnapshotID || session.ManifestHash != l.Manifest.Hash || session.LogicalRequestID != l.LogicalRequestID {
				return runtimecontract.RuntimeSemanticRequestRepairProof{}, common.ErrConflict
			}
			v := session.ExpectedDocument.Clone()
			expected = &v
		}
	}
	if expected == nil {
		return runtimecontract.RuntimeSemanticRequestRepairProof{}, common.ErrConflict
	}
	p := runtimecontract.RuntimeSemanticRequestRepairProof{ProtocolRef: runtimecontract.RuntimeSemanticRequestRepairV1, Manifest: l.Manifest.Clone(), Expected: *expected, Observation: s.Observation.Clone(), Dispatch: s.Dispatch.Clone(), Request: *s.Request}
	p.Hash = runtimecontract.StableRuntimeSemanticRequestRepairProofHash(p)
	if err := p.Validate(); err != nil {
		return p, err
	}
	if err := p.ValidateRequestIntent(l); err != nil {
		return p, err
	}
	round, err := CurrentRuntimeImplementationWorkReservation(a)
	if err != nil || round == nil || round.Hash != p.Manifest.ProofObligations[0].ImplementationWork.RoundHash || round.WorkingBinding != nil || round.DispatchID != "" || round.CompletedReceiptSetHash != "" || round.FailedReceiptHash != "" || round.StartupInputHash != a.Definition.StartupInputHash {
		return p, common.ErrConflict
	}
	if _, current := semanticPlanningCatalogsCurrent(a, p.Manifest); !current {
		return p, common.ErrConflict
	}
	for _, allocation := range a.ExecutionBudget.Allocations {
		if allocation.State == runtimecontract.RuntimeBudgetAllocationReserved || allocation.State == runtimecontract.RuntimeBudgetAllocationUnknown {
			return p, common.ErrConflict
		}
	}
	for _, dimension := range []runtimecontract.RuntimeBudgetDimension{runtimecontract.RuntimeBudgetDimensionProviderCall, runtimecontract.RuntimeBudgetDimensionProviderInputToken, runtimecontract.RuntimeBudgetDimensionProviderOutputToken, runtimecontract.RuntimeBudgetDimensionSemanticGeneration} {
		remaining, e := runtimecontract.RuntimeBudgetRemaining(a.ExecutionBudget, dimension)
		if e != nil {
			return p, e
		}
		if remaining <= 0 {
			return p, &runtimecontract.RuntimeBudgetExhaustedError{Dimension: dimension, Required: 1, Remaining: remaining}
		}
	}
	if limits := a.Facts.Activation.ExecutionContract.ExecutionLimits; limits != nil && limits.MaxProofBatchesPerCWI > 0 && runtimecontract.RuntimeSemanticProofBatchUsage(l)[round.CoverageWorkItemID] >= limits.MaxProofBatchesPerCWI {
		return p, common.ErrConflict
	}
	for _, g := range l.History {
		if g.RequestContractRepair != nil && g.RequestContractRepair.Proof.Expected.LogicalRequestID == p.Expected.LogicalRequestID {
			return p, common.ErrConflict
		}
	}
	return p, nil
}

func (c *authorityComponent) observeRuntimeSemanticRequestRepair(ctx context.Context, ws common.WorkspaceID, run common.RunID, proof runtimecontract.RuntimeSemanticRequestRepairProof, resolution *runtimecontract.RuntimeSemanticRequestRepairResolution, leaseTemplate *runtimecontract.RuntimeDriveLease) (RuntimeAggregate, error) {
	if err := proof.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if proof.Manifest.WorkspaceID != ws || proof.Manifest.RunID != run {
		return RuntimeAggregate{}, common.ErrConflict
	}
	a, err := c.repository.Load(ctx, ws, run)
	if err != nil {
		return a, err
	}
	if resolution != nil {
		if err := validateSemanticRequestRepairLease(proof, *resolution, leaseTemplate); err != nil {
			return a, err
		}
		for _, g := range a.SemanticPlanning.History {
			if g.RequestContractRepair != nil && g.RequestContractRepair.Proof.Hash == proof.Hash {
				if g.RequestContractRepair.Resolution == *resolution {
					return a, nil
				}
				return a, common.ErrConflict
			}
		}
	}
	expected, err := CompileRuntimeSemanticRequestRepairProof(a)
	if err != nil {
		return a, err
	}
	if !reflect.DeepEqual(expected, proof) {
		return a, common.ErrConflict
	}
	revision := a.StateRevision
	if resolution == nil {
		if a.DefinitionClarification.RequestContractRepair != nil {
			if reflect.DeepEqual(*a.DefinitionClarification.RequestContractRepair, proof) {
				return a, nil
			}
			return a, common.ErrConflict
		}
		v := proof.Clone()
		a.DefinitionClarification.RequestContractRepair = &v
	} else {
		if a.DefinitionClarification.RequestContractRepair == nil || !reflect.DeepEqual(*a.DefinitionClarification.RequestContractRepair, proof) {
			return a, common.ErrConflict
		}
		ledger := a.SemanticPlanning.Clone()
		snapshot := ledger.Clone()
		snapshot.History = nil
		record := runtimecontract.RuntimeSemanticRequestRepairRecord{Proof: proof.Clone(), Resolution: *resolution}
		generation := runtimecontract.RuntimeSemanticPlanningGeneration{RetiredAtStateRevision: revision + 1, Reason: runtimecontract.RuntimeSemanticPlanningRetirementRequestContractRepair, RequestContractRepair: &record, Snapshot: snapshot}
		if err := generation.Validate(); err != nil {
			return a, err
		}
		if err := retireSemanticPlanningRequestSessions(&a, ledger, "semantic_request_contract_repaired"); err != nil {
			return a, err
		}
		a.SemanticPlanning = runtimecontract.RuntimeSemanticPlanningLedger{SchemaVersion: ledger.SchemaVersion, Phase: runtimecontract.RuntimeSemanticPlanningRequired, DefinitionSeed: ledger.DefinitionSeed.Clone(), CatalogRefreshAttempts: ledger.CatalogRefreshAttempts, FeasibilityGaps: ledger.FeasibilityGaps, History: append(ledger.History, generation)}
		a.DefinitionClarification = nil
		facts, decision, e := reduceRuntimeAggregateWithDirective(a, a.Facts, nil)
		if e != nil {
			return a, fmt.Errorf("reduce request contract repair: %w", e)
		}
		a.Facts, a.Decision = facts, decision
		lease := leaseTemplate.Clone()
		lease.AggregateRevision, lease.RecoveryGeneration, lease.State = revision+1, 1, runtimecontract.RuntimeDriveLeaseScheduled
		if a.DriveLease != nil {
			lease.RecoveryGeneration = a.DriveLease.RecoveryGeneration + 1
		}
		a.DriveLease = &lease
	}
	a.StateRevision++
	if err := c.transitions.Commit(ctx, a, revision); err != nil {
		return RuntimeAggregate{}, err
	}
	return a, nil
}

// SemanticRequestRepairDriveLeaseSignature joins SM-4 delivery to the original
// clarification fence, rather than the newly reduced semantic authority.
func SemanticRequestRepairDriveLeaseSignature(p runtimecontract.RuntimeSemanticRequestRepairProof, r runtimecontract.RuntimeSemanticRequestRepairResolution) string {
	return fmt.Sprintf("lifecycle-owner-delivery/decision_continuation/%s/%s/%s//%s/runtime-governance/%s/%s", p.Dispatch.AuthoritySnapshotID, p.Dispatch.DispatchID, r.TicketID, r.ResolutionID, p.Dispatch.AuthoritySnapshotID, p.Dispatch.DispatchID)
}
func validateSemanticRequestRepairLease(p runtimecontract.RuntimeSemanticRequestRepairProof, r runtimecontract.RuntimeSemanticRequestRepairResolution, lease *runtimecontract.RuntimeDriveLease) error {
	if err := r.ValidateFor(p); err != nil {
		return err
	}
	if lease == nil || lease.WorkspaceID != p.Manifest.WorkspaceID || lease.RunID != p.Manifest.RunID || lease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry || lease.FailureSignature != SemanticRequestRepairDriveLeaseSignature(p, r) || lease.AggregateRevision != 0 || lease.RecoveryGeneration != 0 || lease.State != "" {
		return common.ErrInvalidArgument
	}
	probe := lease.Clone()
	probe.AggregateRevision, probe.RecoveryGeneration, probe.State = 1, 1, runtimecontract.RuntimeDriveLeaseScheduled
	return probe.Validate()
}
