package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Validate remains the single complete aggregate invariant gate. The private
// validators are ordered to preserve the historical fail-fast sequence while
// making each state-machine responsibility reviewable in isolation.
func (a RuntimeAggregate) Validate() error {
	if err := validateRuntimeAggregateSM2Authority(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM3ActiveDispatch(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateActivationFence(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM3CandidateTransaction(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateGovernanceLedgers(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM4DeliveryAcknowledgements(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM2RepairLedgers(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM4DriveLease(a); err != nil {
		return err
	}
	return validateRuntimeAggregateCrossMachineFences(a)
}

func validateRuntimeAggregateSM2Authority(a RuntimeAggregate) error {
	if a.Facts.WorkspaceID == "" || a.Facts.RunID == "" {
		return fmt.Errorf("runtime aggregate requires workspace and run scope")
	}
	if err := runtimecontract.ValidateRuntimeStateCWILedger(a.Facts, a.Reconciliation); err != nil {
		return err
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	completed := sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
	if err := runtimecontract.ValidateRuntimeCWIBindingRetirementsAgainstExecution(a.Facts, active, completed, a.OperationReceipts); err != nil {
		return runtimecontract.NewRuntimeStateCWILedgerIncompatibilityError(err)
	}
	if err := a.Facts.Validate(); err != nil {
		return err
	}
	if err := a.SemanticPlanning.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate semantic planning ledger: %w", err)
	}
	for idx, history := range a.SemanticPlanning.History {
		if history.Snapshot.Manifest == nil {
			// The typed ledger validator already rejects this shape; retain an
			// aggregate-level guard so a future history representation cannot
			// cross workspace/run ownership silently.
			return fmt.Errorf("runtime aggregate semantic generation %d has no manifest", idx)
		}
		if history.Snapshot.Manifest.WorkspaceID != a.Facts.WorkspaceID || history.Snapshot.Manifest.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate semantic generation %d scope mismatch", idx)
		}
	}
	if a.SemanticPlanning.DefinitionSeed.Hash != a.Facts.DefinitionSeed.Hash {
		return fmt.Errorf("runtime aggregate semantic planning seed does not match facts")
	}
	if err := a.Reconciliation.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate reconciliation ledger: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeCoverageProofLineage(a.CoverageProofLineage); err != nil {
		return fmt.Errorf("runtime aggregate coverage proof lineage: %w", err)
	}
	if err := a.Definition.Validate(); err != nil {
		return err
	}
	if a.Definition.Normalized().Revision != a.Facts.Normalized().DefinitionRevision {
		return fmt.Errorf("runtime aggregate definition revision mismatch")
	}
	if err := a.Decision.Validate(); err != nil {
		return err
	}
	if a.Decision.AuthoritySnapshot.WorkspaceID != a.Facts.WorkspaceID || a.Decision.AuthoritySnapshot.RunID != a.Facts.RunID {
		return fmt.Errorf("runtime aggregate decision scope mismatch")
	}
	return nil
}

func validateRuntimeAggregateSM3ActiveDispatch(a RuntimeAggregate) error {
	if a.ActiveDispatch == nil {
		return nil
	}
	if err := a.ActiveDispatch.ValidateWithActivation(a.Decision, a.Facts.Activation); err != nil {
		return fmt.Errorf("runtime aggregate active dispatch: %w", err)
	}
	return nil
}

func validateRuntimeAggregateActivationFence(a RuntimeAggregate) error {
	if a.Termination == nil {
		return nil
	}
	if err := a.Termination.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate termination: %w", err)
	}
	return nil
}

func validateRuntimeAggregateSM3CandidateTransaction(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeCandidateAttemptRecordsForScope(a.Facts.WorkspaceID, a.Facts.RunID, a.CandidateAttempts); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeCandidateRequestSessions(a.CandidateRequestSessions); err != nil {
		return err
	}
	if err := validateRuntimeCandidateAttemptSessionBindings(a.CandidateAttempts, a.CandidateRequestSessions); err != nil {
		return err
	}
	if (isTerminalConvergenceState(a.Decision.State) || a.Decision.Terminal != nil) && hasPendingCandidateAttempt(a.CandidateAttempts) {
		return fmt.Errorf("terminal runtime aggregate cannot retain a pending candidate attempt")
	}
	if hasUnreducedCurrentCandidateRetryFailure(a.Decision, a.CandidateAttempts) {
		return fmt.Errorf("current runtime authority retains a failed candidate attempt 1 without reducer transition")
	}
	return validateAcceptedCandidateDispatchBindings(a)
}

func validateRuntimeCandidateAttemptSessionBindings(attempts []runtimecontract.RuntimeCandidateAttemptRecord, sessions []runtimecontract.RuntimeCandidateRequestSession) error {
	for _, attempt := range attempts {
		matched := false
		for _, session := range sessions {
			if session.LogicalRequestID != attempt.LogicalRequestID {
				continue
			}
			if session.Transaction != attempt.Transaction || session.StablePrefixHash != attempt.StablePrefixHash || session.ManifestHash != attempt.ManifestHash || session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
				return fmt.Errorf("runtime candidate attempt request-session binding does not match its complete document")
			}
			matched = true
			break
		}
		if !matched {
			return fmt.Errorf("runtime candidate attempt has no complete request-session binding")
		}
	}
	return nil
}

func validateAcceptedCandidateDispatchBindings(a RuntimeAggregate) error {
	for _, attempt := range a.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := a.ActiveDispatch != nil && a.ActiveDispatch.Transaction == attempt.Transaction
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if dispatch.Transaction == attempt.Transaction {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted candidate attempt has no sealed dispatch")
		}
	}
	return nil
}

func validateRuntimeAggregateGovernanceLedgers(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeGovernanceResolutionRecords(a.GovernanceResolutions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeGovernanceRequestRecords(a.GovernanceRequests); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceBindings(a.OperationGovernanceBindings); err != nil {
		return err
	}
	return runtimecontract.ValidateRuntimeOperationReceipts(a.OperationReceipts)
}

func validateRuntimeAggregateSM4DeliveryAcknowledgements(a RuntimeAggregate) error {
	for _, acknowledgement := range a.DeliveryAcknowledgements {
		if err := acknowledgement.Validate(); err != nil {
			return err
		}
		if acknowledgement.WorkspaceID != a.Facts.WorkspaceID || acknowledgement.RunID != a.Facts.RunID {
			return fmt.Errorf("delivery acknowledgement scope mismatch")
		}
	}
	return nil
}

func validateRuntimeAggregateSM2RepairLedgers(a RuntimeAggregate) error {
	for _, invalidation := range a.PrerequisiteInvalidations {
		if err := invalidation.Validate(); err != nil {
			return fmt.Errorf("runtime aggregate prerequisite invalidation: %w", err)
		}
	}
	if err := runtimecontract.ValidateRuntimeRepairSurfaceConfirmationRecords(a.RepairSurfaceConfirmations); err != nil {
		return fmt.Errorf("runtime aggregate repair surface confirmations: %w", err)
	}
	for _, confirmation := range a.RepairSurfaceConfirmations {
		if confirmation.Confirmation.Observation.WorkspaceID != a.Facts.WorkspaceID || confirmation.Confirmation.Observation.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate repair surface confirmation scope mismatch")
		}
	}
	return nil
}

func validateRuntimeAggregateSM4DriveLease(a RuntimeAggregate) error {
	if a.DriveLease == nil {
		return nil
	}
	if err := a.DriveLease.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate drive lease: %w", err)
	}
	if a.DriveLease.WorkspaceID != a.Facts.WorkspaceID || a.DriveLease.RunID != a.Facts.RunID {
		return fmt.Errorf("runtime aggregate drive lease scope mismatch")
	}
	return nil
}

func validateRuntimeAggregateCrossMachineFences(a RuntimeAggregate) error {
	if err := validateRuntimeGovernanceRequestDispatchBindings(a); err != nil {
		return err
	}
	if err := validateRuntimeOperationGovernanceBindingFences(a); err != nil {
		return err
	}
	return validateRuntimeGovernanceResolutionBindings(a)
}

func validateRuntimeGovernanceRequestDispatchBindings(a RuntimeAggregate) error {
	for _, request := range a.GovernanceRequests {
		matched := a.ActiveDispatch != nil && governancecheck.RequestDispatchMatches(request, governancecheck.DispatchFacts{
			DispatchID: a.ActiveDispatch.ID, AuthoritySnapshotID: a.ActiveDispatch.Transaction.AuthoritySnapshotID,
			CandidateGovernance: a.ActiveDispatch.Candidate.Governance != nil,
		})
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if governancecheck.RequestDispatchMatches(request, governancecheck.DispatchFacts{
					DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
					CandidateGovernance: dispatch.Candidate.Governance != nil,
				}) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("runtime aggregate governance request has no governed dispatch")
		}
	}
	return nil
}

func validateRuntimeOperationGovernanceBindingFences(a RuntimeAggregate) error {
	for _, rawBinding := range a.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *SealedDispatch
		for idx := range a.CompletedDispatches {
			if a.CompletedDispatches[idx].ID == binding.OperationDispatchID {
				value := a.CompletedDispatches[idx]
				source = &value
				break
			}
		}
		if source == nil {
			return fmt.Errorf("operation governance binding source dispatch %q is not completed", binding.OperationDispatchID)
		}
		execution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
		if !found || !governancecheck.BindingMatchesExecution(binding, execution) {
			return fmt.Errorf("operation governance binding source execution does not match sealed receipt")
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, binding.ExecutionKey, binding.Fence)
		if !found || !governancecheck.BindingMatchesReceipt(binding, receipt) {
			return fmt.Errorf("operation governance binding has no matching governance receipt")
		}
		if binding.State == runtimecontract.RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		bound := false
		for _, request := range a.GovernanceRequests {
			if governancecheck.BindingMatchesRequest(binding, request) {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("operation governance binding has no matching ticket request")
		}
	}
	return nil
}

func validateRuntimeGovernanceResolutionBindings(a RuntimeAggregate) error {
	for _, resolution := range a.GovernanceResolutions {
		matched := false
		bound := false
		for _, request := range a.GovernanceRequests {
			if governancecheck.ResolutionRecordMatchesRequest(resolution, request) {
				bound = true
				break
			}
		}
		for _, dispatch := range a.CompletedDispatches {
			if dispatch.State == DispatchStateCommitted && governancecheck.ResolutionRecordMatchesDispatch(resolution, governancecheck.DispatchFacts{
				DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
				CandidateGovernance: dispatch.Candidate.Governance != nil,
			}) {
				matched = true
				break
			}
		}
		if !matched || !bound {
			return fmt.Errorf("runtime aggregate governance resolution has no completed governed dispatch")
		}
	}
	return nil
}

func hasPendingCandidateAttempt(values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	for _, value := range values {
		if value.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return true
		}
	}
	return false
}

func hasUnreducedCurrentCandidateRetryFailure(decision runtimecontract.RuntimeDecision, values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	current := decision.AuthorityTransaction()
	for _, value := range values {
		if value.Attempt == 1 && value.State == runtimecontract.RuntimeCandidateAttemptStateFailed && runtimecontract.SameAuthorityContract(value.Transaction, current) {
			return true
		}
	}
	return false
}
