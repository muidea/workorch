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
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeDispatchHistoryShape(a.Facts.WorkspaceID, a.Facts.RunID, active, sealedDispatchesToRuntimeRecords(a.CompletedDispatches)); err != nil {
		return fmt.Errorf("runtime aggregate dispatch history: %w", err)
	}
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
	if err := validateRuntimeAggregateExecutionBudget(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateRecoverableWaits(a); err != nil {
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

func validateRuntimeAggregateRecoverableWaits(a RuntimeAggregate) error {
	seen := make(map[string]struct{}, len(a.RecoverableWaits))
	active := 0
	for _, raw := range a.RecoverableWaits {
		wait := raw.Clone()
		if err := wait.Validate(a.ExecutionBudget); err != nil {
			return fmt.Errorf("runtime aggregate recoverable wait: %w", err)
		}
		if _, duplicate := seen[wait.WaitID]; duplicate {
			return fmt.Errorf("runtime aggregate recoverable wait %q is duplicated", wait.WaitID)
		}
		seen[wait.WaitID] = struct{}{}
		if !wait.Active() {
			continue
		}
		active++
		if wait.WorkspaceID != a.Facts.WorkspaceID || wait.RunID != a.Facts.RunID || wait.DefinitionRevision != a.Facts.DefinitionRevision ||
			wait.AuthorityTransactionID != runtimecontract.StableAuthorityContractID(a.Facts.WorkspaceID, a.Facts.RunID, a.Decision.AuthorityTransaction()) {
			return fmt.Errorf("active runtime recoverable wait does not match current authority")
		}
		allocation, _ := runtimecontract.RuntimeBudgetAllocationForID(a.ExecutionBudget.Allocations, wait.BudgetAllocationID)
		if (wait.Status == runtimecontract.RuntimeRecoverableWaitWaiting || wait.Status == runtimecontract.RuntimeRecoverableWaitProbeClaimed) &&
			allocation.State != runtimecontract.RuntimeBudgetAllocationReserved && allocation.State != runtimecontract.RuntimeBudgetAllocationUnknown {
			return fmt.Errorf("active runtime recoverable wait requires a reserved or unknown probe allocation")
		}
	}
	if active > 1 {
		return fmt.Errorf("runtime aggregate has multiple active recoverable waits")
	}
	if active != 0 && (isTerminalConvergenceState(a.Decision.State) || a.Termination != nil) {
		return fmt.Errorf("terminal runtime aggregate cannot retain an active recoverable wait")
	}
	return nil
}

func validateRuntimeAggregateExecutionBudget(a RuntimeAggregate) error {
	if err := a.ExecutionBudget.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate execution budget: %w", err)
	}
	if !runtimecontract.SameRuntimeExecutionBudgetPolicy(a.ExecutionBudget.Policy, a.Facts.ExecutionBudgetPolicy) {
		return fmt.Errorf("runtime aggregate execution budget policy does not match activation facts")
	}
	return nil
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
		return err
	}
	if err := validateRuntimeProspectiveCoverageCollectiveCredit(a); err != nil {
		return err
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
	if err := validateRuntimeSemanticBindingPurposeInvariant(a); err != nil {
		return err
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

// validateRuntimeSemanticBindingPurposeInvariant prevents an intermediate
// owner-fact acquisition proof from becoming terminal Definition credit.  A
// current binding may refer to the active manifest retained while the next
// proof batch is prepared, or to a sealed manifest moved into immutable
// generation history after a resource refresh.  Those manifests are read here
// only to recover the exact per-CWI purpose; they never authorize a candidate,
// operation, dispatch, or transition.
func validateRuntimeSemanticBindingPurposeInvariant(a RuntimeAggregate) error {
	type bindingPurposeKey struct {
		manifestHash string
		coverageID   string
	}

	purposes := make(map[bindingPurposeKey]runtimecontract.RuntimeSemanticEvidenceObligationPurpose)
	addManifest := func(manifest runtimecontract.RuntimeSemanticPlanManifest) error {
		manifest = manifest.Normalized()
		for _, obligation := range manifest.ProofObligations {
			resolved, err := runtimecontract.ResolveRuntimeSemanticEvidenceObligation(obligation)
			if err != nil {
				return fmt.Errorf("runtime aggregate semantic binding purpose: %w", err)
			}
			key := bindingPurposeKey{manifestHash: manifest.Hash, coverageID: string(obligation.CoverageWorkItemID)}
			if existing, found := purposes[key]; found && existing != resolved.Purpose {
				return fmt.Errorf("runtime aggregate semantic binding purpose conflicts for manifest %q coverage %q", manifest.Hash, obligation.CoverageWorkItemID)
			}
			purposes[key] = resolved.Purpose
		}
		return nil
	}

	// The active manifest remains immutable lineage after one batch settles and
	// the ledger returns to an order-accepted phase for the next batch.
	if a.SemanticPlanning.Manifest != nil {
		if err := addManifest(*a.SemanticPlanning.Manifest); err != nil {
			return err
		}
	}
	for _, generation := range a.SemanticPlanning.History {
		// A generation can return to order-accepted while preparing its next
		// batch, then be retired after an eligibility or catalog change.  Its
		// exact manifest still explains an already-sealed binding; only that
		// binding-purpose classification is consumed here.
		if generation.Snapshot.Manifest == nil {
			continue
		}
		if err := addManifest(*generation.Snapshot.Manifest); err != nil {
			return err
		}
	}
	// Activation/reducer-only aggregates have no semantic generation yet and
	// therefore no purpose graph to cross-check.  Once any manifest lineage is
	// present, every current binding must resolve exactly within that graph.
	if len(purposes) == 0 {
		return nil
	}

	focuses := make(map[string]runtimecontract.CoverageFocus, 1+len(a.Facts.RemainingFocuses)+len(a.Facts.CompletedFocuses))
	for _, focus := range append(append([]runtimecontract.CoverageFocus{a.Facts.Focus}, a.Facts.RemainingFocuses...), a.Facts.CompletedFocuses...) {
		focus = focus.Normalized()
		focuses[focus.ID] = focus
	}
	for _, item := range a.Facts.Coverage {
		if item.CurrentBindingRef == nil {
			continue
		}
		ref := item.CurrentBindingRef.Normalized()
		key := bindingPurposeKey{manifestHash: ref.SemanticManifestHash, coverageID: string(item.ID)}
		purpose, found := purposes[key]
		if !found {
			activeManifestHash := ""
			if a.SemanticPlanning.Manifest != nil {
				activeManifestHash = a.SemanticPlanning.Manifest.Hash
			}
			return fmt.Errorf("runtime aggregate CWI %q current binding manifest %q has no exact semantic purpose lineage (active=%q history=%d)", item.ID, ref.SemanticManifestHash, activeManifestHash, len(a.SemanticPlanning.History))
		}
		if purpose != runtimecontract.RuntimeSemanticEvidenceObligationOwnerFactAcquisition {
			continue
		}
		focus, found := focuses[ref.FocusID]
		if !found {
			return fmt.Errorf("runtime aggregate acquisition CWI %q has no bound focus", item.ID)
		}
		if !focus.RequireAssessment {
			return fmt.Errorf("runtime aggregate acquisition CWI %q requires assessment before terminal credit", item.ID)
		}
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied || item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled {
			return fmt.Errorf("runtime aggregate acquisition CWI %q cannot retain terminal coverage credit", item.ID)
		}
	}
	return nil
}

func validateRuntimeAggregateSM3ActiveDispatch(a RuntimeAggregate) error {
	if a.ActiveDispatch == nil {
		return nil
	}
	if err := a.ActiveDispatch.ValidateWithActivation(a.Decision, runtimecontract.RuntimeActivationForFacts(a.Facts)); err != nil {
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
	if err := validateRuntimeDefinitionClarificationAggregate(a); err != nil {
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
	if err := validateAcceptedCandidateDispatchBindings(a); err != nil {
		return err
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	return runtimecontract.ValidateRuntimeDispatchAdmissionLineage(
		active,
		sealedDispatchesToRuntimeRecords(a.CompletedDispatches),
		a.CandidateAttempts,
		a.OperationGovernanceBindings,
	)
}

func validateRuntimeDefinitionClarificationAggregate(a RuntimeAggregate) error {
	if a.DefinitionClarification == nil {
		return nil
	}
	if err := a.DefinitionClarification.Validate(); err != nil {
		return fmt.Errorf("runtime Definition clarification: %w", err)
	}
	clarification := a.DefinitionClarification.Clone()
	definition := a.Definition.Normalized()
	if clarification.Observation.WorkspaceID != a.Facts.WorkspaceID || clarification.Observation.RunID != a.Facts.RunID ||
		clarification.Observation.DefinitionRevision != definition.Revision || clarification.Observation.StartupInputHash != definition.StartupInputHash ||
		a.Decision.State != runtimecontract.ConvergenceStateNeedGovernance ||
		!sameRuntimeStrings(a.Decision.RequiredOutput.Normalized().AcceptedKinds, []string{runtimecontract.RuntimeDefinitionClarificationRequiredOutputKind}) ||
		a.ActiveDispatch != nil {
		return fmt.Errorf("runtime Definition clarification aggregate binding mismatch")
	}
	if clarification.Dispatch != nil && clarification.Dispatch.AuthoritySnapshotID != a.Decision.AuthoritySnapshot.ID {
		return fmt.Errorf("runtime Definition clarification dispatch authority mismatch")
	}
	for _, session := range a.CandidateRequestSessions {
		if session.LogicalRequestID == clarification.Observation.LogicalRequestID && session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete &&
			session.Transaction.AuthoritySnapshotID == clarification.Observation.AuthoritySnapshotID {
			return nil
		}
	}
	return fmt.Errorf("runtime Definition clarification has no complete semantic request session")
}

func sameRuntimeStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
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
		matched := a.ActiveDispatch != nil && a.ActiveDispatch.Transaction == attempt.Transaction &&
			a.ActiveDispatch.ID == attempt.DispatchID && a.ActiveDispatch.CandidateHash == attempt.CandidateHash &&
			a.ActiveDispatch.DispatchAuthorityHash == attempt.DispatchAuthorityHash
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if dispatch.Transaction == attempt.Transaction && dispatch.ID == attempt.DispatchID &&
					dispatch.CandidateHash == attempt.CandidateHash && dispatch.DispatchAuthorityHash == attempt.DispatchAuthorityHash {
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
	if err := runtimecontract.ValidateRuntimeOperationReceipts(a.OperationReceipts); err != nil {
		return err
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	return runtimecontract.ValidateRuntimeOperationReceiptExecutionLineage(active, sealedDispatchesToRuntimeRecords(a.CompletedDispatches), a.OperationReceipts)
}

func validateRuntimeAggregateSM4DeliveryAcknowledgements(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeStateTerminalDeliveryLedger(a.Decision, a.Termination, a.DeliveryAcknowledgements, a.Facts.Recovery); err != nil {
		return err
	}
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
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	return runtimecontract.ValidateRuntimeGovernanceRequestDispatchLineage(a.GovernanceRequests, active, sealedDispatchesToRuntimeRecords(a.CompletedDispatches))
}

func validateRuntimeOperationGovernanceBindingFences(a RuntimeAggregate) error {
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeDispatchExecutionKeyLineage(
		a.Facts.WorkspaceID,
		a.Facts.RunID,
		active,
		sealedDispatchesToRuntimeRecords(a.CompletedDispatches),
		a.OperationGovernanceBindings,
	); err != nil {
		return err
	}
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
		if source == nil || source.State != DispatchStateCommitted {
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
	governedExecutions := make([]runtimecontract.RuntimeOperationExecutionRecord, 0)
	if a.ActiveDispatch != nil {
		governedExecutions = append(governedExecutions, a.ActiveDispatch.OperationExecutions...)
	}
	for _, dispatch := range a.CompletedDispatches {
		governedExecutions = append(governedExecutions, dispatch.OperationExecutions...)
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceExecutionBindings(a.OperationGovernanceBindings, governedExecutions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceGrantLineage(a.OperationGovernanceBindings, a.GovernanceResolutions, a.Facts.Evidence.Accepted, a.Facts.Evidence.Rejected); err != nil {
		return err
	}
	return nil
}

func validateRuntimeGovernanceResolutionBindings(a RuntimeAggregate) error {
	for _, resolution := range a.GovernanceResolutions {
		resolution = resolution.Normalized()
		if resolution.WorkspaceID != a.Facts.WorkspaceID || resolution.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate governance resolution scope mismatch")
		}
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
