package runtime

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
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
	completed := sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
	if err := runtimecontract.ValidateRuntimeDispatchHistoryShape(a.Facts.WorkspaceID, a.Facts.RunID, active, completed); err != nil {
		return fmt.Errorf("runtime aggregate dispatch history: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeEvidenceLedgerSequence(a.EvidenceSequence, a.Facts, active, completed, a.GovernanceResolutions); err != nil {
		return fmt.Errorf("runtime aggregate evidence ledger: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeAssessmentAuthoritySources(a.Facts, active, completed); err != nil {
		return fmt.Errorf("runtime aggregate assessment authority: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeAssessmentAdjudications(a.Facts, completed, a.CoverageProofLineage); err != nil {
		return fmt.Errorf("runtime aggregate assessment adjudication: %w", err)
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
	if err := validateRuntimeAggregateOperationRecoveryAuthorizations(a); err != nil {
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

func validateRuntimeAggregateOperationRecoveryAuthorizations(a RuntimeAggregate) error {
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeOperationRecoveryAuthorizations(
		a.Facts.WorkspaceID,
		a.Facts.RunID,
		active,
		sealedDispatchesToRuntimeRecords(a.CompletedDispatches),
		a.OperationReceipts,
		a.OperationRecoveryAuthorizations,
		a.Facts.Recovery,
		a.StateRevision,
	); err != nil {
		return fmt.Errorf("runtime aggregate operation recovery authorization: %w", err)
	}
	return nil
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
	if err := validateRuntimeCWISemanticRebindingFocusProjections(a.Facts); err != nil {
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
	if err := validateRuntimeSemanticBindingPreimages(a); err != nil {
		return err
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
		focus, found := focuses[ref.FocusID]
		if !found {
			return fmt.Errorf("runtime aggregate intermediate-purpose CWI %q has no bound focus", item.ID)
		}
		switch purpose {
		case runtimecontract.RuntimeSemanticEvidenceObligationOwnerFactAcquisition:
			if !focus.RequireAssessment {
				return fmt.Errorf("runtime aggregate acquisition CWI %q requires assessment before terminal credit", item.ID)
			}
			if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied || item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled {
				return fmt.Errorf("runtime aggregate acquisition CWI %q cannot retain terminal coverage credit", item.ID)
			}
		case runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair:
			binding, bindingFound := coverageEvidenceBindingForCoverage(focus, item.ID)
			pendingFileTransaction, err := pendingConfirmedRepairTransactionCovers(a, item.ID)
			if err != nil {
				return err
			}
			persistedDirective := a.Facts.RepairDirective != nil &&
				a.Facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedStateChange &&
				a.Facts.RepairDirective.Closure != nil &&
				sameRuntimeTargetRefs(a.Facts.RepairDirective.TargetRefs, binding.ProspectiveTargetRefs)
			if !bindingFound || len(binding.ProspectiveTargetRefs) == 0 ||
				(!persistedDirective && !pendingFileTransaction) ||
				item.State == runtimecontract.RuntimeCoverageItemStateSatisfied || item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled {
				return fmt.Errorf("runtime aggregate validation-failure repair CWI %q gained terminal credit or lost its exact repair directive", item.ID)
			}
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
	if a.Termination.Revision > a.StateRevision {
		return fmt.Errorf("runtime aggregate termination revision exceeds aggregate revision")
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
	if err := validateRuntimeCandidateFeasibilityBindings(a); err != nil {
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
	completed := sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
	if err := runtimecontract.ValidateRuntimeDispatchAdmissionLineage(
		active,
		completed,
		a.CandidateAttempts,
		a.OperationGovernanceBindings,
	); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeAcceptedCandidateManifestLineage(active, completed, a.CandidateAttempts, a.CandidateRequestSessions); err != nil {
		return err
	}
	return runtimecontract.ValidateRuntimeAcceptedAssessmentPurposePreimages(a.SemanticPlanning, active, completed, a.CandidateAttempts, a.CandidateRequestSessions)
}

// validateRuntimeCandidateFeasibilityBindings is the fresh-load reverse proof
// for every retained pre-seal owner probe fact. Shape validation alone is
// insufficient: after restart each result must still belong to the exact
// proposal, manifest, operation owner and current owner-authored catalog that
// the live admission path checked before persisting it. Feasible facts may
// remain on a pending attempt; a candidate-constraint violation remains on its
// atomically failed attempt as correction lineage.
func validateRuntimeCandidateFeasibilityBindings(a RuntimeAggregate) error {
	for _, attempt := range a.CandidateAttempts {
		if len(attempt.Feasibilities) == 0 {
			continue
		}
		if attempt.Proposal == nil {
			return fmt.Errorf("runtime candidate feasibility ledger has no persisted proposal")
		}
		for _, feasibility := range attempt.Feasibilities {
			operation, found := candidateOperationByID(*attempt.Proposal, feasibility.OperationID)
			if !found {
				return fmt.Errorf("runtime candidate feasibility operation %q is absent from its proposal", feasibility.OperationID)
			}
			operation = operation.Normalized()
			if feasibility.CandidateManifestHash != attempt.ManifestHash {
				return fmt.Errorf("runtime candidate feasibility operation %q has stale manifest identity", operation.ID)
			}
			descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
			if !known || descriptor.OwnerModuleID == "" {
				return fmt.Errorf("runtime candidate feasibility operation %q has no canonical owner", operation.ID)
			}
			if !runtimecontract.RuntimeOperationAdmissibilityRequiredForOperation(a.Facts.Prerequisites, descriptor.OwnerModuleID, operation.Kind) {
				return fmt.Errorf("runtime candidate feasibility operation %q has no current admissibility requirement", operation.ID)
			}
			inventory, found := runtimeOperationAdmissibilityForKind(a.Facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, operation.Kind)
			if !found || inventory.SnapshotID != feasibility.AdmissibilitySnapshotID || inventory.AcquisitionBinding != feasibility.AcquisitionBinding {
				return fmt.Errorf("runtime candidate feasibility operation %q does not match current owner admissibility", operation.ID)
			}
			if feasibility.Observation == nil {
				return fmt.Errorf("runtime candidate feasibility operation %q has no owner observation", operation.ID)
			}
			observation := feasibility.Observation.Normalized()
			ownerPolicy := observation.OwnerPolicy.Normalized()
			if ownerPolicy.OwnerModuleID != "" || ownerPolicy.PolicyRevision != "" || ownerPolicy.CatalogSnapshotID != "" {
				if err := ownerPolicy.Validate(); err != nil || ownerPolicy.OwnerModuleID != descriptor.OwnerModuleID || ownerPolicy.CatalogSnapshotID != inventory.SnapshotID {
					return fmt.Errorf("runtime candidate feasibility operation %q owner policy does not match its descriptor and catalog", operation.ID)
				}
			}
			if observation.Feedback != nil {
				feedback := observation.Feedback.Normalized()
				if feedback.WorkspaceID != a.Facts.WorkspaceID || feedback.RunID != a.Facts.RunID || feedback.OriginalDiagnostic == nil ||
					feedback.OriginalDiagnostic.OwnerModuleID != descriptor.OwnerModuleID {
					return fmt.Errorf("runtime candidate feasibility operation %q feedback does not match aggregate scope and owner", operation.ID)
				}
			}
			if observation.Stale != nil && observation.Stale.Normalized().OwnerModuleID != descriptor.OwnerModuleID {
				return fmt.Errorf("runtime candidate feasibility operation %q stale fact has the wrong owner", operation.ID)
			}
			if observation.PrerequisiteRequest != nil {
				request := observation.PrerequisiteRequest.Normalized()
				if request.OwnerModuleID != descriptor.OwnerModuleID || request.OperationID != operation.ID {
					return fmt.Errorf("runtime candidate feasibility operation %q prerequisite request has the wrong identity", operation.ID)
				}
			}
			if observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated {
				manifest, err := persistedCandidateAttemptManifest(a, attempt)
				if err != nil {
					return fmt.Errorf("runtime candidate applicability feasibility manifest: %w", err)
				}
				if err := validateCandidateFeasibilityObservationBinding(a, operation, feasibility, manifest); err != nil {
					return err
				}
			}
			if feasibility.ReplayProof != nil {
				proof := feasibility.ReplayProof.Normalized()
				if proof.OwnerModuleID != descriptor.OwnerModuleID || proof.OperationID != operation.ID ||
					proof.AdmissibilitySnapshotID != inventory.SnapshotID || proof.AcquisitionBinding != inventory.AcquisitionBinding ||
					!sameStringSet(proof.TargetRefs, operation.TargetRefs) {
					return fmt.Errorf("runtime candidate feasibility operation %q replay proof does not match sealed owner facts", operation.ID)
				}
			}
		}
	}
	return nil
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
			if session.Transaction != attempt.Transaction || session.StablePrefixHash != attempt.StablePrefixHash || session.ManifestHash != attempt.ManifestHash {
				return fmt.Errorf("runtime candidate attempt request-session binding does not match its complete document")
			}
			if session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
				return fmt.Errorf("runtime candidate attempt request-session binding does not match its complete document")
			}
			if attempt.Failure != nil && runtimecontract.RuntimeCandidateFailureDiagnosticRequiresWireRule(attempt.Failure.Class, attempt.Failure.Diagnostic) && attempt.Failure.Diagnostic.WireRule == nil {
				return fmt.Errorf("runtime candidate rule-selected schema diagnostic is missing exact wire identity")
			}
			if attempt.Failure != nil && attempt.Failure.Diagnostic != nil && attempt.Failure.Diagnostic.WireRule != nil {
				if session.CandidateManifest == nil {
					return fmt.Errorf("runtime candidate wire diagnostic has no persisted manifest preimage")
				}
				if err := runtimecontract.ValidateRuntimeCandidateFailureDiagnosticWireRule(*session.CandidateManifest, attempt.Failure.Diagnostic); err != nil {
					return fmt.Errorf("runtime candidate wire diagnostic lineage: %w", err)
				}
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
		matched := a.ActiveDispatch != nil && runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, sealedDispatchToRuntimeRecord(*a.ActiveDispatch))
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, sealedDispatchToRuntimeRecord(dispatch)) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted candidate attempt has no exact sealed dispatch target-authorization projection")
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
	for _, receipt := range a.OperationReceipts {
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil {
			continue
		}
		dispatch, err := operationReceiptFeedbackDispatch(a, receipt)
		if err != nil {
			return fmt.Errorf("runtime aggregate operation receipt diagnostic execution projection: %w", err)
		}
		if err := validateOperationReceiptFeedbackAgainstDispatch(a, dispatch, a.Facts.WorkspaceID, a.Facts.RunID, receipt); err != nil {
			return fmt.Errorf("runtime aggregate operation receipt diagnostic: %w", err)
		}
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, receipt.OperationID)
		if !found {
			return fmt.Errorf("runtime aggregate rejected operation receipt has no execution projection")
		}
		if execution.State == runtimecontract.OperationExecutionEvidenceCommitted {
			operation, found := candidateOperationByID(dispatch.Candidate, receipt.OperationID)
			if !found {
				return fmt.Errorf("runtime aggregate rejected operation receipt has no sealed operation")
			}
			if err := validateRuntimeReceiptEvidenceRejectedLineage(a, dispatch, operation, execution, receipt, runtimecontract.StableRuntimeOperationReceiptHash(receipt)); err != nil {
				return fmt.Errorf("runtime aggregate rejected operation receipt evidence projection: %w", err)
			}
		}
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeOperationReceiptExecutionLineage(active, sealedDispatchesToRuntimeRecords(a.CompletedDispatches), a.OperationReceipts); err != nil {
		return err
	}
	return validateRuntimeAggregateEvidenceCommitLedger(a)
}

type runtimeEvidenceCommitFactProjection struct {
	fact     runtimecontract.EvidenceFact
	rejected bool
}

// validateRuntimeAggregateEvidenceCommitLedger is the fresh-load/recovery
// reverse proof for the mutable SM-3 evidence commit. Every evidence-committed
// execution must point to one exact Orchestrator-owned fact and processed-event
// fence. Governance clones may repeat that projection, so the fact's own
// DispatchID resolves the dispatch that originally committed it.
func validateRuntimeAggregateEvidenceCommitLedger(a RuntimeAggregate) error {
	dispatches := make([]SealedDispatch, 0, len(a.CompletedDispatches)+1)
	if a.ActiveDispatch != nil {
		dispatches = append(dispatches, *a.ActiveDispatch)
	}
	dispatches = append(dispatches, a.CompletedDispatches...)
	factsByDispatch := map[string][]runtimecontract.EvidenceFact{}
	seenFacts := map[string]struct{}{}
	referencedFacts := map[common.EvidenceRecordID]int{}
	for _, dispatch := range dispatches {
		for _, execution := range dispatch.OperationExecutions {
			execution = execution.Normalized()
			if execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
				continue
			}
			projection, err := runtimeEvidenceCommitFactProjectionForID(a, execution.EvidenceFactID)
			if err != nil {
				return fmt.Errorf("evidence-committed operation %q: %w", execution.OperationID, err)
			}
			if execution.EvidenceFactHash != runtimecontract.StableRuntimeEvidenceFactHash(projection.fact) {
				return fmt.Errorf("evidence-committed operation %q evidence fact hash mismatch", execution.OperationID)
			}
			if err := validateRuntimeAggregateEvidenceCommitFact(a, dispatches, execution, projection); err != nil {
				return fmt.Errorf("evidence-committed operation %q: %w", execution.OperationID, err)
			}
			referencedFacts[projection.fact.ID]++
			factID := string(projection.fact.ID)
			if _, duplicate := seenFacts[factID]; duplicate {
				continue
			}
			seenFacts[factID] = struct{}{}
			factsByDispatch[projection.fact.DispatchID] = append(factsByDispatch[projection.fact.DispatchID], projection.fact)
		}
	}
	if err := validateRuntimeAggregateOperationEvidenceFactsReferenced(a, referencedFacts); err != nil {
		return err
	}
	for dispatchID, facts := range factsByDispatch {
		dispatch, found := sealedDispatchByID(dispatches, dispatchID)
		if !found {
			return fmt.Errorf("evidence commit fact dispatch %q is not persisted", dispatchID)
		}
		sort.Slice(facts, func(i, j int) bool { return facts[i].Revision < facts[j].Revision })
		previous := dispatch.Transaction.EvidenceRevision
		for _, fact := range facts {
			if !confirmedRepairConsecutiveEvidenceRevision(fact.Revision, previous) {
				return fmt.Errorf("evidence commit fact %q revision does not form the exact dispatch evidence chain", fact.ID)
			}
			previous = fact.Revision
		}
	}
	return nil
}

// validateRuntimeAggregateOperationEvidenceFactsReferenced closes the other
// direction of the SM-3 commit proof. Assessment and governance facts have
// their own owner ledgers; every ordinary operation-scoped fact must instead
// be reachable from an evidence_committed execution. Otherwise a persisted
// orphan could affect coverage or the reducer without ever crossing the owner
// receipt transition.
func validateRuntimeAggregateOperationEvidenceFactsReferenced(a RuntimeAggregate, referenced map[common.EvidenceRecordID]int) error {
	validate := func(ledger string, values []runtimecontract.EvidenceFact) error {
		for _, fact := range values {
			kind := strings.TrimSpace(fact.Kind)
			if kind == runtimecontract.EvidenceKindAssessment || kind == runtimecontract.EvidenceKindGovernance {
				continue
			}
			if strings.TrimSpace(fact.FocusID) == "" || strings.TrimSpace(fact.DispatchID) == "" || strings.TrimSpace(fact.OperationID) == "" {
				return fmt.Errorf("ordinary %s evidence fact %q requires focus, dispatch and operation lineage", ledger, fact.ID)
			}
			if referenced[fact.ID] == 0 {
				return fmt.Errorf("operation-scoped %s evidence fact %q has no exact evidence-committed execution", ledger, fact.ID)
			}
		}
		return nil
	}
	if err := validate("accepted", a.Facts.Evidence.Accepted); err != nil {
		return err
	}
	return validate("rejected", a.Facts.Evidence.Rejected)
}

func runtimeEvidenceCommitFactProjectionForID(a RuntimeAggregate, id common.EvidenceRecordID) (runtimeEvidenceCommitFactProjection, error) {
	if id == "" {
		return runtimeEvidenceCommitFactProjection{}, fmt.Errorf("evidence fact id is empty")
	}
	acceptedRaw, rejectedRaw := 0, 0
	for _, fact := range a.Facts.Evidence.Accepted {
		if fact.ID == id {
			acceptedRaw++
		}
	}
	for _, fact := range a.Facts.Evidence.Rejected {
		if fact.ID == id {
			rejectedRaw++
		}
	}
	if acceptedRaw+rejectedRaw != 1 {
		return runtimeEvidenceCommitFactProjection{}, fmt.Errorf("evidence fact %q must exist exactly once across accepted/rejected ledgers", id)
	}
	facts := a.Facts.Normalized().Evidence
	projection := runtimeEvidenceCommitFactProjection{rejected: rejectedRaw == 1}
	values := facts.Accepted
	if projection.rejected {
		values = facts.Rejected
	}
	for _, fact := range values {
		if fact.ID == id {
			projection.fact = fact
			break
		}
	}
	processed := 0
	for _, eventID := range a.ProcessedEventIDs {
		if eventID == string(id) {
			processed++
		}
	}
	if projection.fact.ID == "" || processed != 1 {
		return runtimeEvidenceCommitFactProjection{}, fmt.Errorf("evidence fact %q requires one processed-event fence", id)
	}
	return projection, nil
}

func validateRuntimeAggregateEvidenceCommitFact(
	a RuntimeAggregate,
	dispatches []SealedDispatch,
	execution runtimecontract.RuntimeOperationExecutionRecord,
	projection runtimeEvidenceCommitFactProjection,
) error {
	fact := projection.fact
	dispatch, found := sealedDispatchByID(dispatches, fact.DispatchID)
	if !found {
		return fmt.Errorf("evidence fact dispatch %q is not persisted", fact.DispatchID)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, fact.OperationID)
	if !found || fact.FocusID != dispatch.Transaction.FocusID || fact.SubjectRef != operation.ID || execution.OperationID != operation.ID ||
		fact.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID || fact.Revision == "" {
		return fmt.Errorf("evidence fact does not match its sealed dispatch/focus/authority/operation")
	}
	ownerExecution, found := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
	if !found || ownerExecution.State != runtimecontract.OperationExecutionEvidenceCommitted ||
		ownerExecution.EvidenceFactID != fact.ID || ownerExecution.EvidenceFactHash != execution.EvidenceFactHash ||
		ownerExecution.ExecutionKey != execution.ExecutionKey || ownerExecution.Fence != execution.Fence ||
		ownerExecution.OwnerReceiptRef != execution.OwnerReceiptRef || ownerExecution.OwnerReceiptHash != execution.OwnerReceiptHash {
		return fmt.Errorf("evidence fact has no exact evidence-committed execution in its owning dispatch")
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, ownerExecution.ExecutionKey, ownerExecution.Fence)
	if !found || receipt.OperationID != operation.ID || ownerExecution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey ||
		ownerExecution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
		return fmt.Errorf("evidence fact has no exact owner receipt")
	}
	expectedStatus, expectedFailure, err := runtimeOperationReceiptEvidenceJudgment(
		runtimecontract.RuntimeDecision{State: dispatch.Transaction.State},
		ownerExecution,
		operation,
		receipt,
	)
	if err != nil {
		return fmt.Errorf("evidence fact owner receipt judgment: %w", err)
	}
	expectedRejected := expectedStatus == ExecutionEvidenceStatusRejected
	if projection.rejected != expectedRejected {
		return fmt.Errorf("evidence fact accepted/rejected ledger does not match its exact owner receipt judgment")
	}
	if !runtimeEvidenceFactFailureMatches(fact, expectedFailure) {
		return fmt.Errorf("evidence fact failure projection does not match its exact owner receipt judgment")
	}
	expectedKind, err := adjudicateEvidenceKind(
		runtimecontract.RuntimeDecision{State: dispatch.Transaction.State},
		ownerExecution.ExpectedEffectScope,
		true,
		ExecutionEvidence{Status: expectedStatus, Receipt: &receipt},
	)
	if err != nil || fact.Kind != expectedKind || !sameRuntimeTargetRefs(fact.TargetRefs, operation.TargetRefs) {
		return fmt.Errorf("evidence fact kind/targets do not match its sealed operation")
	}
	receiptEvidence := receipt.Normalized().Evidence
	if fact.Summary != receiptEvidence.Summary || fact.ContentHash != receiptEvidence.ContentHash ||
		!reflect.DeepEqual(fact.ContentAddress, receiptEvidence.ContentAddress) || !reflect.DeepEqual(fact.ExitCode, receiptEvidence.ExitCode) ||
		!sameRuntimeTargetRefs(fact.EvidenceRefs, receiptEvidence.EvidenceRefs) || fact.Quality != receiptEvidence.Quality ||
		fact.ZeroWork != receiptEvidence.ZeroWork || !sameRuntimeTargetRefs(fact.EvidenceTags, receiptEvidence.EvidenceTags) ||
		!reflect.DeepEqual(fact.Validation, receiptEvidence.Validation) {
		return fmt.Errorf("evidence fact payload does not match its exact owner receipt")
	}
	credit := findExecutionEvidenceCredit(dispatch.CoverageBindings, operation, expectedKind, &receipt)
	repairCausationOnly := !expectedRejected && confirmedRepairCausationOnlyBinding(
		dispatch,
		dispatch.CoverageBindings,
		operation,
		expectedKind,
	)
	if !sameRuntimeStringSet(fact.BindingRefs, operation.BindingRefs) {
		return fmt.Errorf("evidence fact binding refs do not match its sealed operation")
	}
	if expectedRejected {
		if len(fact.CoverageWorkItemIDs) != 0 {
			return fmt.Errorf("rejected evidence fact cannot carry coverage credit")
		}
	} else if repairCausationOnly {
		if len(fact.CoverageWorkItemIDs) != 0 {
			return fmt.Errorf("confirmed repair causation-only evidence fact cannot carry coverage credit")
		}
	} else if !sameCoverageIDs(fact.CoverageWorkItemIDs, credit.CoverageWorkItemIDs) {
		return fmt.Errorf("accepted evidence fact coverage does not match its sealed binding/receipt projection")
	}
	if fact.EvidenceReason != receiptEvidence.EvidenceReason {
		return fmt.Errorf("evidence fact reason does not match its exact owner receipt")
	}
	if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
		receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		if !projection.rejected || fact.ID != common.EvidenceRecordID("runtime-receipt|"+receiptHash) || fact.SourceRef != "runtime-receipt/"+receipt.ExecutionKey ||
			fact.EvidenceReason != receiptEvidence.EvidenceReason {
			return fmt.Errorf("rejected evidence fact does not match its canonical receipt/binding/non-credit projection")
		}
	} else {
		receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		recoveryEventID := common.EvidenceRecordID("runtime-receipt|" + receiptHash)
		recoverySourceRef := "runtime-receipt/" + receipt.ExecutionKey
		if fact.ID != recoveryEventID || fact.SourceRef != recoverySourceRef {
			return fmt.Errorf("non-rejected receipt evidence fact has no canonical persisted-receipt identity")
		}
	}
	return nil
}

func runtimeEvidenceFactFailureMatches(fact runtimecontract.EvidenceFact, expected *runtimecontract.FailureFact) bool {
	if expected == nil {
		return fact.FailureClass == "" && fact.FailureCode == "" && fact.FailureSignature == "" && fact.FailureMessage == ""
	}
	want := expected.Normalized()
	return fact.FailureClass == want.Class && fact.FailureCode == want.Code &&
		fact.FailureSignature == want.Signature && fact.FailureMessage == want.Message
}

func sealedDispatchByID(values []SealedDispatch, id string) (SealedDispatch, bool) {
	var matched SealedDispatch
	found := false
	for _, dispatch := range values {
		if dispatch.ID != id {
			continue
		}
		if found {
			return SealedDispatch{}, false
		}
		matched, found = dispatch, true
	}
	return matched, found
}

// operationReceiptFeedbackDispatch resolves the dispatch that actually owns a
// rejected receipt from its committed execution projection. Governance resume
// keeps the source ExecutionKey as immutable lineage while advancing Fence and
// binding the owner call to a new dispatch; the key origin therefore cannot be
// used as the receipt's current dispatch identity. Origin/grant legality remains
// the responsibility of the canonical execution-lineage validators.
func operationReceiptFeedbackDispatch(a RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt) (SealedDispatch, error) {
	receipt = receipt.Normalized()
	if receipt.Feedback == nil {
		return SealedDispatch{}, fmt.Errorf("rejected receipt has no diagnostic feedback")
	}
	feedbackDispatchID := receipt.Feedback.Normalized().DispatchID
	receiptRef := "runtime-receipt/" + receipt.ExecutionKey
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)

	projected := make([]SealedDispatch, 0, 1)
	collect := func(dispatch SealedDispatch) {
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, receipt.OperationID)
		if !found {
			return
		}
		execution = execution.Normalized()
		if execution.ExecutionKey != receipt.ExecutionKey || execution.Fence != receipt.Fence ||
			(execution.State != runtimecontract.OperationExecutionOwnerCommitted && execution.State != runtimecontract.OperationExecutionEvidenceCommitted) ||
			execution.OwnerReceiptRef != receiptRef || execution.OwnerReceiptHash != receiptHash {
			return
		}
		projected = append(projected, dispatch)
	}
	if a.ActiveDispatch != nil {
		collect(*a.ActiveDispatch)
	}
	for _, dispatch := range a.CompletedDispatches {
		collect(dispatch)
	}
	if len(projected) == 0 {
		return SealedDispatch{}, fmt.Errorf("rejected receipt has no exact committed execution projection")
	}

	var matched SealedDispatch
	exact := 0
	for _, dispatch := range projected {
		if dispatch.ID != feedbackDispatchID {
			continue
		}
		matched = dispatch
		exact++
	}
	if exact == 0 {
		return SealedDispatch{}, fmt.Errorf("feedback dispatch %q does not match an exact committed execution projection", feedbackDispatchID)
	}
	if exact != 1 {
		return SealedDispatch{}, fmt.Errorf("feedback dispatch %q has ambiguous committed execution projections", feedbackDispatchID)
	}
	return matched, nil
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
	if err := runtimecontract.ValidateRuntimeRepairSurfaceConfirmationRecords(a.RepairSurfaceConfirmations); err != nil {
		return fmt.Errorf("runtime aggregate repair surface confirmations: %w", err)
	}
	for _, confirmation := range a.RepairSurfaceConfirmations {
		value := confirmation.Confirmation.Normalized()
		if value.Observation.WorkspaceID != a.Facts.WorkspaceID || value.Observation.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate repair surface confirmation scope mismatch")
		}
		if !sealedValidationFailureObservationMatches(a, value.Observation) {
			return fmt.Errorf("runtime aggregate repair surface confirmation does not match a sealed completed dispatch")
		}
		if !runtimeValidationFailureObservationHasPersistedReceipt(a, value.Observation) {
			return fmt.Errorf("runtime aggregate repair surface confirmation has no exact persisted owner receipt")
		}
		expectedOwner, err := runtimeExpectedRepairSurfaceOwner(a, value.Observation)
		if err != nil || value.OwnerModuleID != expectedOwner {
			return fmt.Errorf("runtime aggregate repair surface confirmation owner does not match sealed repair provenance")
		}
	}
	if err := validateRuntimeFailureTargetObservationOwnerAuthorities(a); err != nil {
		return err
	}
	if err := validateRuntimeUnobservedFailureTargetAuthority(a); err != nil {
		return err
	}
	if err := validateRuntimeReceiptEvidenceFailureTargetAuthorities(a); err != nil {
		return err
	}
	if err := validateRuntimeFailureObservationReceiptAuthority(a, "current failure", a.Facts.Failure); err != nil {
		return err
	}
	if a.Facts.RepairDirective != nil {
		if err := validateRuntimeFailureObservationReceiptAuthority(a, "repair directive", a.Facts.RepairDirective.CurrentFailure); err != nil {
			return err
		}
		if observation := runtimeValidationFailureObservationFromFailure(a.Facts.RepairDirective.CurrentFailure); observation != nil && len(a.Facts.RepairDirective.TargetRefs) != 0 {
			if _, err := runtimeConfirmedRepairSurfaceForTargets(a, *observation, a.Facts.RepairDirective.TargetRefs); err != nil {
				return fmt.Errorf("runtime aggregate repair directive target authority: %w", err)
			}
		}
	}
	if observation := runtimeValidationFailureObservationFromFailure(a.Facts.Failure); observation != nil && a.Facts.Resources.FailureTarget != nil {
		confirmation, err := runtimeConfirmedRepairSurfaceForTargets(a, *observation, a.Facts.Resources.FailureTarget.TargetRefs)
		if err != nil {
			return fmt.Errorf("runtime aggregate failure target authority: %w", err)
		}
		if a.Facts.Resources.FailureTarget.Signature != observation.FailureSignature || !runtimeTargetObservationMatchesConfirmation(a.Facts.Resources.FailureTarget.TargetObservation, confirmation) {
			return fmt.Errorf("runtime aggregate failure target authority does not match owner confirmation")
		}
	}
	for _, invalidation := range a.PrerequisiteInvalidations {
		if err := invalidation.Validate(); err != nil {
			return fmt.Errorf("runtime aggregate prerequisite invalidation: %w", err)
		}
	}
	if err := validatePendingConfirmedRepairTransaction(a); err != nil {
		return err
	}
	if err := validateConfirmedRepairFreshValidationContinuation(a); err != nil {
		return err
	}
	return nil
}

// validateRuntimeUnobservedFailureTargetAuthority proves the two legitimate
// source of a target resource that has no discovered-target observation: an
// exact rejected receipt whose diagnostic refs are a subset of its sealed
// operation. This keeps
// deletion of a receipt-evidence/owner-confirmed observation from downgrading discovered
// targets into generic repair authority.
func validateRuntimeUnobservedFailureTargetAuthority(a RuntimeAggregate) error {
	target := a.Facts.Resources.FailureTarget
	if target == nil || target.TargetObservation != nil {
		return nil
	}
	failure := a.Facts.Failure
	if failure == nil || failure.Signature != target.Signature || !sameRuntimeTargetRefs(failure.TargetRefs, target.TargetRefs) {
		return fmt.Errorf("runtime aggregate unobserved failure target does not match the current failure")
	}

	validReceiptLineages := 0
	var receiptLineageErr error
	for _, rawReceipt := range a.OperationReceipts {
		receipt := rawReceipt.Normalized()
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil ||
			receipt.Feedback.OriginalDiagnostic.TargetObservation != nil ||
			!sameRuntimeTargetRefs(receipt.Feedback.OriginalDiagnostic.TargetRefs, target.TargetRefs) {
			continue
		}
		expected := failureFactFromOperationReceiptFeedback(receipt)
		if expected == nil || !runtimeFailureTargetAuthorityCoreEqual(failure.Normalized(), expected.Normalized()) {
			continue
		}
		dispatch, err := operationReceiptFeedbackDispatch(a, receipt)
		if err != nil {
			receiptLineageErr = err
			continue
		}
		operation, found := candidateOperationByID(dispatch.Candidate, receipt.OperationID)
		descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found || !descriptorFound || descriptor.FailureTargetProtocol == runtimecontract.RuntimeFailureTargetProtocolValidationCandidates ||
			!runtimeTargetRefsWithin(target.TargetRefs, operation.TargetRefs) {
			continue
		}
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, receipt.OperationID)
		if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || dispatch.State != DispatchStateCommitted ||
			!containsString(dispatch.CompletedOperationIDs, receipt.OperationID) {
			continue
		}
		receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		if execution.ExecutionKey != receipt.ExecutionKey || execution.Fence != receipt.Fence ||
			execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || execution.OwnerReceiptHash != receiptHash {
			continue
		}
		if err := validateRuntimeReceiptEvidenceRejectedLineage(a, dispatch, operation, execution, receipt, receiptHash); err != nil {
			receiptLineageErr = err
			continue
		}
		validReceiptLineages++
	}
	if validReceiptLineages == 1 {
		return nil
	}
	if validReceiptLineages > 1 {
		return fmt.Errorf("runtime aggregate unobserved failure target has multiple exact rejected receipt lineages")
	}

	if receiptLineageErr != nil {
		return fmt.Errorf("runtime aggregate unobserved failure target receipt lineage: %w", receiptLineageErr)
	}
	return fmt.Errorf("runtime aggregate unobserved failure target has no exact sealed receipt lineage")
}

// validateRuntimeFailureTargetObservationOwnerAuthorities is the aggregate
// fresh-load fence for concrete repair targets. Receipt ingress already admits
// receipt-derived observations only through a descriptor-declared protocol,
// while a resource owner may create an observation only through exact
// repair-surface confirmation. Persisted state must
// replay the same rule instead of treating the DTO's generic OwnerModuleID as
// authority for another capability owner.
func validateRuntimeFailureTargetObservationOwnerAuthorities(a RuntimeAggregate) error {
	failures := []struct {
		subject string
		failure *runtimecontract.FailureFact
	}{
		{subject: "current failure", failure: a.Facts.Failure},
		{subject: "repair directive", failure: runtimeRepairDirectiveCurrentFailure(a.Facts.RepairDirective)},
	}
	for _, item := range failures {
		if item.failure == nil {
			continue
		}
		failure := item.failure.Normalized()
		var nested *runtimecontract.RuntimeFailureTargetObservation
		if failure.OriginalDiagnostic != nil {
			diagnostic := failure.OriginalDiagnostic.Normalized()
			if diagnostic != nil {
				nested = diagnostic.TargetObservation
			}
		}
		if validationObservation := runtimeValidationFailureObservationFromFailure(&failure); validationObservation != nil && failure.TargetObservation != nil {
			confirmation, err := runtimeConfirmedRepairSurfaceForTargets(a, *validationObservation, failure.TargetRefs)
			if err != nil || nested != nil || !runtimeTargetObservationMatchesConfirmation(failure.TargetObservation, confirmation) {
				return fmt.Errorf("runtime aggregate %s validation target authority must come from its exact owner confirmation", item.subject)
			}
			continue
		}
		if (failure.TargetObservation == nil) != (nested == nil) ||
			(failure.TargetObservation != nil && !sameFailureTargetObservationBinding(failure.TargetObservation, nested)) {
			return fmt.Errorf("runtime aggregate %s receipt-evidence target observation copies must both be present and identical", item.subject)
		}
	}

	target := a.Facts.Resources.FailureTarget
	if target == nil || target.TargetObservation == nil {
		return nil
	}
	if runtimeFailureTargetResourceMatchesConfirmedFailure(a, *target) {
		return nil
	}
	// A receipt-evidence target may intentionally outlive the mutable current
	// failure while a repair receipt awaits fresh validation. The independent
	// reverse-provenance fence below binds this resource directly to exactly one
	// immutable rejected receipt and its current owner-catalog dependency; do
	// not require a second mutable FailureFact copy here.
	return nil
}

func runtimeFailureTargetResourceMatchesConfirmedFailure(a RuntimeAggregate, target runtimecontract.FailureTarget) bool {
	for _, rawFailure := range []*runtimecontract.FailureFact{
		a.Facts.Failure,
		runtimeRepairDirectiveCurrentFailure(a.Facts.RepairDirective),
	} {
		if rawFailure == nil {
			continue
		}
		failure := rawFailure.Normalized()
		observation := runtimeValidationFailureObservationFromFailure(&failure)
		if observation == nil || failure.TargetObservation == nil ||
			failure.Signature != target.Signature || !sameRuntimeTargetRefs(failure.TargetRefs, target.TargetRefs) ||
			!sameFailureTargetObservationBinding(failure.TargetObservation, target.TargetObservation) {
			continue
		}
		confirmation, err := runtimeConfirmedRepairSurfaceForTargets(a, *observation, failure.TargetRefs)
		if err == nil && runtimeTargetObservationMatchesConfirmation(failure.TargetObservation, confirmation) {
			return true
		}
	}
	return false
}

// validateRuntimeReceiptEvidenceFailureTargetAuthorities closes the reverse edge
// from durable SM-2 target authority back to the one rejected owner execution
// transaction that produced it. Receipt validation by itself proves only that
// an owner result was committed; it must not let a copied TargetObservation or
// FailureTarget survive after its evidence, dispatch or mutable catalog
// authority has disappeared.
func validateRuntimeReceiptEvidenceFailureTargetAuthorities(a RuntimeAggregate) error {
	if err := validateRuntimeReceiptEvidenceFailureTargetAuthorityDowngrades(a); err != nil {
		return err
	}
	for _, item := range []struct {
		subject string
		failure *runtimecontract.FailureFact
	}{
		{subject: "current failure", failure: a.Facts.Failure},
		{subject: "repair directive", failure: runtimeRepairDirectiveCurrentFailure(a.Facts.RepairDirective)},
	} {
		observation, carriesReceiptAuthority, err := runtimeReceiptEvidenceFailureTargetObservationCopies(item.failure)
		if err != nil {
			return fmt.Errorf("runtime aggregate %s receipt-evidence target authority: %w", item.subject, err)
		}
		if !carriesReceiptAuthority {
			continue
		}
		failure := item.failure.Normalized()
		if err := validateRuntimeReceiptEvidenceTargetProvenance(a, item.subject, observation, func(expected runtimecontract.FailureFact) bool {
			return reflect.DeepEqual(failure, expected.Normalized())
		}); err != nil {
			return err
		}
	}

	target := a.Facts.Resources.FailureTarget
	if target == nil || target.TargetObservation == nil || runtimeFailureTargetResourceMatchesConfirmedFailure(a, *target) {
		return nil
	}
	observation := target.TargetObservation.Normalized()
	if err := validateRuntimeReceiptEvidenceTargetProvenance(a, "failure target resource", observation, func(expected runtimecontract.FailureFact) bool {
		expected = expected.Normalized()
		return expected.TargetObservation != nil && expected.Signature == target.Signature &&
			sameRuntimeTargetRefs(expected.TargetRefs, target.TargetRefs) &&
			sameFailureTargetObservationBinding(expected.TargetObservation, target.TargetObservation)
	}); err != nil {
		return err
	}
	return nil
}

// validateRuntimeReceiptEvidenceFailureTargetAuthorityDowngrades closes the
// forward edge from an immutable rejected owner receipt to mutable Runtime
// repair authority.
// Removing every promoted observation copy must not turn that same receipt-
// derived target into an apparently generic, unobserved FailureTarget.
func validateRuntimeReceiptEvidenceFailureTargetAuthorityDowngrades(a RuntimeAggregate) error {
	expectedFailures := make([]runtimecontract.FailureFact, 0)
	for _, rawReceipt := range a.OperationReceipts {
		receipt := rawReceipt.Normalized()
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil ||
			receipt.Feedback.OriginalDiagnostic.TargetObservation == nil {
			continue
		}
		dispatch, err := operationReceiptFeedbackDispatch(a, receipt)
		if err != nil {
			return err
		}
		operation, found := candidateOperationByID(dispatch.Candidate, receipt.OperationID)
		descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found || !descriptorFound || descriptor.FailureTargetProtocol != runtimecontract.RuntimeFailureTargetProtocolReceiptEvidence {
			continue
		}
		expected := failureFactFromOperationReceiptFeedback(receipt)
		if expected != nil {
			expectedFailures = append(expectedFailures, expected.Normalized())
		}
	}
	if len(expectedFailures) == 0 {
		return nil
	}
	for _, item := range []struct {
		subject string
		failure *runtimecontract.FailureFact
	}{
		{subject: "current failure", failure: a.Facts.Failure},
		{subject: "repair directive", failure: runtimeRepairDirectiveCurrentFailure(a.Facts.RepairDirective)},
	} {
		if item.failure == nil {
			continue
		}
		failure := item.failure.Normalized()
		for _, expected := range expectedFailures {
			if !runtimeFailureTargetAuthorityCoreEqual(failure, expected) {
				continue
			}
			if failure.TargetObservation == nil {
				return fmt.Errorf("runtime aggregate %s cannot downgrade receipt-evidence target authority to an unobserved failure", item.subject)
			}
		}
	}
	if target := a.Facts.Resources.FailureTarget; target != nil && len(target.TargetRefs) != 0 {
		for _, expected := range expectedFailures {
			if target.Signature != expected.Signature || !sameRuntimeTargetRefs(target.TargetRefs, expected.TargetRefs) {
				continue
			}
			if target.TargetObservation == nil {
				return fmt.Errorf("runtime aggregate failure target resource cannot downgrade receipt-evidence target authority to an unobserved target")
			}
		}
	}
	return nil
}

func runtimeFailureTargetAuthorityCoreEqual(left, right runtimecontract.FailureFact) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.Class == right.Class && left.Code == right.Code && left.Signature == right.Signature && left.Message == right.Message &&
		sameRuntimeTargetRefs(left.TargetRefs, right.TargetRefs)
}

func runtimeRepairDirectiveCurrentFailure(directive *runtimecontract.RuntimeRepairDirective) *runtimecontract.FailureFact {
	if directive == nil {
		return nil
	}
	return directive.CurrentFailure
}

// runtimeReceiptEvidenceFailureTargetObservationCopies requires the promoted failure copy
// and the immutable owner diagnostic copy to remain identical. A top-level
// observation alone is mutable Runtime state; a nested copy alone is audit
// detail. Neither one independently authorizes a repair target.
func runtimeReceiptEvidenceFailureTargetObservationCopies(failure *runtimecontract.FailureFact) (runtimecontract.RuntimeFailureTargetObservation, bool, error) {
	if failure == nil {
		return runtimecontract.RuntimeFailureTargetObservation{}, false, nil
	}
	value := failure.Normalized()
	var nested *runtimecontract.RuntimeFailureTargetObservation
	if value.OriginalDiagnostic != nil {
		diagnostic := value.OriginalDiagnostic.Normalized()
		if diagnostic != nil {
			nested = diagnostic.TargetObservation
		}
	}
	if nested == nil {
		return runtimecontract.RuntimeFailureTargetObservation{}, false, nil
	}
	if value.TargetObservation == nil || !sameFailureTargetObservationBinding(value.TargetObservation, nested) {
		return runtimecontract.RuntimeFailureTargetObservation{}, true, fmt.Errorf("promoted and original receipt-evidence target observations must both be present and identical")
	}
	return value.TargetObservation.Clone(), true, nil
}

func validateRuntimeReceiptEvidenceTargetProvenance(
	a RuntimeAggregate,
	subject string,
	observation runtimecontract.RuntimeFailureTargetObservation,
	matchesFailure func(runtimecontract.FailureFact) bool,
) error {
	var lineageErr error
	matchedReceipt := false
	validLineages := 0
	for _, rawReceipt := range a.OperationReceipts {
		receipt := rawReceipt.Normalized()
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil {
			continue
		}
		expected := failureFactFromOperationReceiptFeedback(receipt)
		if expected == nil || expected.TargetObservation == nil || !sameFailureTargetObservationBinding(expected.TargetObservation, &observation) || !matchesFailure(*expected) {
			continue
		}
		matchedReceipt = true
		if err := validateRuntimeReceiptEvidenceTargetLineage(a, receipt, observation); err != nil {
			lineageErr = err
			continue
		}
		validLineages++
	}
	if validLineages == 1 {
		return nil
	}
	if validLineages > 1 {
		return fmt.Errorf("runtime aggregate %s receipt-evidence target authority has multiple exact rejected owner receipts", subject)
	}
	if matchedReceipt && lineageErr != nil {
		return fmt.Errorf("runtime aggregate %s receipt-evidence target authority: %w", subject, lineageErr)
	}
	return fmt.Errorf("runtime aggregate %s receipt-evidence target authority has no exact rejected owner receipt", subject)
}

func validateRuntimeReceiptEvidenceTargetLineage(a RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt, observation runtimecontract.RuntimeFailureTargetObservation) error {
	dispatch, err := operationReceiptFeedbackDispatch(a, receipt)
	if err != nil {
		return err
	}
	operation, found := candidateOperationByID(dispatch.Candidate, receipt.OperationID)
	descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || !descriptorFound || descriptor.FailureTargetProtocol != runtimecontract.RuntimeFailureTargetProtocolReceiptEvidence ||
		descriptor.OwnerModuleID != observation.OwnerModuleID {
		return fmt.Errorf("rejected receipt has no exact descriptor-authorized target-observation operation")
	}
	if observation.TargetRole != dispatch.FocusTargetRole {
		return fmt.Errorf("receipt-evidence target observation role does not match the sealed dispatch focus role")
	}
	if err := validateReceiptEvidenceTargetObservation(descriptor, receipt.Evidence, observation); err != nil {
		return fmt.Errorf("failure target observation/evidence provenance: %w", err)
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, receipt.OperationID)
	if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
		return fmt.Errorf("receipt-evidence target authority requires an evidence-committed execution")
	}
	if dispatch.State != DispatchStateCommitted || !containsString(dispatch.CompletedOperationIDs, receipt.OperationID) {
		return fmt.Errorf("receipt-evidence target authority requires a completed committed dispatch")
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	if execution.ExecutionKey != receipt.ExecutionKey || execution.Fence != receipt.Fence ||
		execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || execution.OwnerReceiptHash != receiptHash {
		return fmt.Errorf("receipt-evidence target authority receipt ref/hash drift")
	}
	if err := validateRuntimeReceiptEvidenceRejectedLineage(a, dispatch, operation, execution, receipt, receiptHash); err != nil {
		return err
	}
	if !runtimeReceiptEvidenceFailureTargetHasCurrentOwnerFreshness(a, dispatch, operation, execution, observation) {
		return fmt.Errorf("receipt-evidence target authority requires the current exact owner catalog and prerequisite freshness")
	}
	return nil
}

func validateRuntimeReceiptEvidenceRejectedLineage(a RuntimeAggregate, dispatch SealedDispatch, operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord, receipt runtimecontract.RuntimeOperationReceipt, receiptHash string) error {
	facts := a.Facts.Normalized()
	var evidence runtimecontract.EvidenceFact
	evidenceMatches := 0
	for _, fact := range a.Facts.Evidence.Rejected {
		if fact.ID == execution.EvidenceFactID {
			evidenceMatches++
		}
	}
	for _, fact := range facts.Evidence.Rejected {
		if fact.ID == execution.EvidenceFactID {
			evidence = fact
			break
		}
	}
	acceptedMatches := 0
	for _, fact := range a.Facts.Evidence.Accepted {
		if fact.ID == execution.EvidenceFactID {
			acceptedMatches++
		}
	}
	eventID := string(evidence.ID)
	processedMatches := 0
	for _, value := range a.ProcessedEventIDs {
		if value == eventID {
			processedMatches++
		}
	}
	if execution.EvidenceFactID == "" || execution.EvidenceFactHash == "" || evidenceMatches != 1 || acceptedMatches != 0 || processedMatches != 1 {
		return fmt.Errorf("rejected receipt requires one receipt-derived rejected evidence fact bound to its execution ledger and processed-event fence (rejected=%d accepted=%d processed=%d event=%q)", evidenceMatches, acceptedMatches, processedMatches, eventID)
	}
	recoveryEventID := "runtime-receipt|" + receiptHash
	recoverySourceRef := "runtime-receipt/" + receipt.ExecutionKey
	expectedFailure := failureFactFromOperationReceiptFeedback(receipt)
	expectedKind, kindErr := runtimecontract.EvidenceKindFromReceipt(dispatch.Transaction.State, execution.ExpectedEffectScope, receipt)
	if expectedFailure == nil || kindErr != nil || eventID != recoveryEventID || evidence.SourceRef != recoverySourceRef || evidence.Kind != expectedKind || evidence.FocusID != dispatch.Transaction.FocusID || evidence.Revision == "" || evidence.SubjectRef != operation.ID ||
		evidence.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID || evidence.DispatchID != dispatch.ID || evidence.OperationID != operation.ID ||
		evidence.FailureClass != expectedFailure.Class || evidence.FailureCode != expectedFailure.Code ||
		evidence.FailureSignature != expectedFailure.Signature || evidence.FailureMessage != expectedFailure.Message ||
		!sameRuntimeTargetRefs(evidence.TargetRefs, operation.TargetRefs) {
		return fmt.Errorf("receipt-derived rejected evidence does not match its exact receipt and sealed operation")
	}
	receiptEvidence := receipt.Normalized().Evidence
	if evidence.Summary != receiptEvidence.Summary || evidence.ContentHash != receiptEvidence.ContentHash ||
		!reflect.DeepEqual(evidence.ContentAddress, receiptEvidence.ContentAddress) ||
		!reflect.DeepEqual(evidence.ExitCode, receiptEvidence.ExitCode) || !sameRuntimeTargetRefs(evidence.EvidenceRefs, receiptEvidence.EvidenceRefs) ||
		evidence.Quality != receiptEvidence.Quality || evidence.ZeroWork != receiptEvidence.ZeroWork ||
		evidence.EvidenceReason != receiptEvidence.EvidenceReason || !sameRuntimeTargetRefs(evidence.EvidenceTags, receiptEvidence.EvidenceTags) || !reflect.DeepEqual(evidence.Validation, receiptEvidence.Validation) {
		return fmt.Errorf("receipt-derived rejected evidence payload drifted from its owner receipt")
	}
	if execution.EvidenceFactID != evidence.ID || execution.EvidenceFactHash != runtimecontract.StableRuntimeEvidenceFactHash(evidence) {
		return fmt.Errorf("receipt-derived rejected evidence does not match its execution-ledger fact id/hash")
	}
	return nil
}

func runtimeReceiptEvidenceFailureTargetHasCurrentOwnerFreshness(
	a RuntimeAggregate,
	dispatch SealedDispatch,
	operation runtimecontract.RuntimeOperation,
	execution runtimecontract.RuntimeOperationExecutionRecord,
	observation runtimecontract.RuntimeFailureTargetObservation,
) bool {
	facts := a.Facts.Normalized()
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || descriptor.FailureTargetProtocol != runtimecontract.RuntimeFailureTargetProtocolReceiptEvidence || descriptor.OwnerModuleID != observation.OwnerModuleID {
		return false
	}
	sealedCatalog := runtimeOwnerCatalogForOperation(
		dispatch.OperationAdmissibilities,
		dispatch.OperationAdmissibilityObservations,
		operation,
		execution.AdmissibilitySnapshotID,
		execution.AcquisitionBinding,
	)
	if sealedCatalog == nil || sealedCatalog.OwnerModuleID != descriptor.OwnerModuleID ||
		sealedCatalog.ObservedRevision != observation.ObservedRevision || !containsString(sealedCatalog.SelectorRefs, observation.SubjectRef) {
		return false
	}
	sealedObservation := runtimeOwnerCatalogObservationForOperation(dispatch.OperationAdmissibilityObservations, sealedCatalog)
	if sealedObservation == nil || !runtimeFailureTargetObservationHasExactOwnerCatalogDependency(*sealedObservation, observation.CatalogDependency) {
		return false
	}
	currentCatalog, found := currentOperationAdmissibility(facts, *sealedCatalog)
	if !found || currentCatalog.SnapshotID != sealedCatalog.SnapshotID ||
		currentCatalog.AcquisitionBinding != sealedCatalog.AcquisitionBinding || currentCatalog.ObservedRevision != observation.ObservedRevision ||
		!containsString(currentCatalog.SelectorRefs, observation.SubjectRef) {
		return false
	}
	currentObservation := runtimeOwnerCatalogObservationForOperation(facts.Resources.PrerequisiteObservations, &currentCatalog)
	if currentObservation == nil || !runtimeFailureTargetObservationHasExactOwnerCatalogDependency(*currentObservation, observation.CatalogDependency) {
		return false
	}
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.ID == currentCatalog.PrerequisiteID && prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility &&
			prerequisite.OwnerModuleID == descriptor.OwnerModuleID {
			return true
		}
	}
	return false
}

func runtimeFailureTargetObservationHasExactOwnerCatalogDependency(observation runtimecontract.RuntimePrerequisiteObservation, expected *runtimecontract.RuntimeFreshnessDependency) bool {
	if expected == nil {
		return false
	}
	observation = observation.Normalized()
	want := expected.Normalized()
	for _, dependency := range observation.Dependencies {
		if dependency.Normalized() == want {
			return true
		}
	}
	return false
}

func validateRuntimeFailureObservationReceiptAuthority(a RuntimeAggregate, subject string, failure *runtimecontract.FailureFact) error {
	if failure == nil {
		return nil
	}
	normalizedFailure := failure.Normalized()
	var nested *runtimecontract.RuntimeValidationFailureObservation
	var nestedDiagnostic *runtimecontract.ValidationFailureDiagnostic
	if normalizedFailure.OriginalDiagnostic != nil {
		diagnostic := normalizedFailure.OriginalDiagnostic.Normalized()
		if diagnostic != nil {
			nested = diagnostic.ValidationFailureObservation
			nestedDiagnostic = diagnostic.ValidationDiagnostic
		}
	}
	if (normalizedFailure.ValidationFailureObservation == nil) != (nested == nil) {
		return fmt.Errorf("runtime aggregate %s validation failure observation copies must both be present", subject)
	}
	if normalizedFailure.ValidationFailureObservation != nil && !runtimecontract.SameRuntimeValidationFailureObservation(*normalizedFailure.ValidationFailureObservation, *nested) {
		return fmt.Errorf("runtime aggregate %s validation failure observation disagrees with original diagnostic", subject)
	}
	if normalizedFailure.ValidationFailureObservation != nil {
		if normalizedFailure.ValidationDiagnostic == nil || nestedDiagnostic == nil {
			return fmt.Errorf("runtime aggregate %s validation diagnostic copies must both be present", subject)
		}
		if !runtimecontract.SameValidationFailureDiagnostic(normalizedFailure.ValidationDiagnostic, nestedDiagnostic) {
			return fmt.Errorf("runtime aggregate %s validation diagnostic disagrees with original diagnostic", subject)
		}
	}
	for _, observation := range []*runtimecontract.RuntimeValidationFailureObservation{normalizedFailure.ValidationFailureObservation, nested} {
		if observation == nil {
			continue
		}
		if normalizedFailure.Signature != observation.FailureSignature {
			return fmt.Errorf("runtime aggregate %s validation failure observation signature mismatch", subject)
		}
		if !sealedValidationFailureObservationMatches(a, *observation) {
			return fmt.Errorf("runtime aggregate %s validation failure observation does not match a sealed completed dispatch", subject)
		}
		if !runtimeValidationFailureObservationHasPersistedReceipt(a, *observation) {
			return fmt.Errorf("runtime aggregate %s validation failure observation has no exact persisted validation-owner receipt", subject)
		}
	}
	observation := runtimeValidationFailureObservationFromFailure(&normalizedFailure)
	if observation != nil {
		if latest, found := runtimeLatestRepairSurfaceConfirmation(a, *observation); found && latest.Disposition == runtimecontract.RuntimeRepairSurfaceConfirmed {
			if len(normalizedFailure.TargetRefs) == 0 || !runtimeTargetObservationMatchesConfirmation(normalizedFailure.TargetObservation, latest) {
				return fmt.Errorf("runtime aggregate %s does not retain the latest confirmed owner repair surface", subject)
			}
		}
	}
	if observation != nil && (len(normalizedFailure.TargetRefs) != 0 || normalizedFailure.TargetObservation != nil) {
		confirmation, err := runtimeConfirmedRepairSurfaceForTargets(a, *observation, normalizedFailure.TargetRefs)
		if err != nil {
			return fmt.Errorf("runtime aggregate %s target authority: %w", subject, err)
		}
		if !runtimeTargetObservationMatchesConfirmation(normalizedFailure.TargetObservation, confirmation) {
			return fmt.Errorf("runtime aggregate %s target authority does not match owner confirmation", subject)
		}
	}
	return nil
}

func runtimeValidationFailureObservationFromFailure(failure *runtimecontract.FailureFact) *runtimecontract.RuntimeValidationFailureObservation {
	if failure == nil {
		return nil
	}
	value := failure.Normalized()
	if value.ValidationFailureObservation != nil {
		observation := value.ValidationFailureObservation.Clone()
		return &observation
	}
	if value.OriginalDiagnostic != nil {
		diagnostic := value.OriginalDiagnostic.Normalized()
		if diagnostic != nil && diagnostic.ValidationFailureObservation != nil {
			observation := diagnostic.ValidationFailureObservation.Clone()
			return &observation
		}
	}
	return nil
}

func runtimeConfirmedRepairSurfaceForTargets(a RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation, targetRefs []string) (runtimecontract.RuntimeRepairSurfaceConfirmation, error) {
	confirmation, found := runtimeLatestRepairSurfaceConfirmation(a, observation)
	expectedOwner, ownerErr := runtimeExpectedRepairSurfaceOwner(a, observation)
	if ownerErr == nil && found && confirmation.OwnerModuleID == expectedOwner &&
		confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceConfirmed &&
		sameRuntimeTargetRefs(confirmation.ConfirmedTargetRefs, targetRefs) {
		return confirmation, nil
	}
	return runtimecontract.RuntimeRepairSurfaceConfirmation{}, fmt.Errorf("validation failure target authority requires an exact confirmed owner repair surface")
}

// runtimeExpectedRepairSurfaceOwner derives the only capability owner allowed
// to confirm a validation failure's concrete repair surface. A matching sealed
// Profile convergence is authoritative when present; otherwise the exact
// source operation's immutable coverage bindings must agree on one
// state-change operation owner. The confirmation itself never selects owner.
func runtimeExpectedRepairSurfaceOwner(a RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation) (string, error) {
	observation = observation.Normalized()
	var sourceDispatch *SealedDispatch
	for index := range a.CompletedDispatches {
		dispatch := &a.CompletedDispatches[index]
		if dispatch.ID != observation.DispatchID || dispatch.State != DispatchStateCommitted ||
			!containsString(dispatch.CompletedOperationIDs, observation.OperationID) {
			continue
		}
		if sourceDispatch != nil {
			return "", fmt.Errorf("validation failure repair owner has multiple source dispatches")
		}
		sourceDispatch = dispatch
	}
	if sourceDispatch == nil || !sealedValidationFailureObservationMatchesDispatch(*sourceDispatch, observation) {
		return "", fmt.Errorf("validation failure repair owner has no exact sealed source")
	}
	operation, found := candidateOperationByID(sourceDispatch.Candidate, observation.OperationID)
	if !found {
		return "", fmt.Errorf("validation failure repair owner source operation is not sealed")
	}
	triggerDescriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || triggerDescriptor.FailureTargetProtocol != runtimecontract.RuntimeFailureTargetProtocolValidationCandidates {
		return "", fmt.Errorf("validation failure repair owner source operation has no candidate-target protocol")
	}

	bindingIDs := normalizeRuntimeTargetRefs(operation.BindingRefs)
	if len(bindingIDs) == 0 {
		return "", fmt.Errorf("validation failure repair owner source operation has no sealed binding")
	}
	sourceBindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(bindingIDs))
	coverageIDs := append([]common.CoverageWorkItemID(nil), operation.CoverageWorkItemIDs...)
	for _, bindingID := range bindingIDs {
		matches := 0
		var binding runtimecontract.CoverageEvidenceBinding
		for index := range sourceDispatch.CoverageBindings {
			candidate := sourceDispatch.CoverageBindings[index].Normalized()
			if candidate.ID != bindingID {
				continue
			}
			binding, matches = candidate, matches+1
		}
		if matches != 1 {
			return "", fmt.Errorf("validation failure repair owner binding %q does not have one exact source projection", bindingID)
		}
		sourceBindings = append(sourceBindings, binding)
		if len(operation.CoverageWorkItemIDs) == 0 {
			coverageIDs = append(coverageIDs, binding.CoverageWorkItemID)
		} else if !coverageIDsIntersect(operation.CoverageWorkItemIDs, []common.CoverageWorkItemID{binding.CoverageWorkItemID}) {
			return "", fmt.Errorf("validation failure repair owner binding %q is outside its source operation coverage", bindingID)
		}
	}
	coverageIDs = normalizeCoverageIDs(coverageIDs)
	if len(coverageIDs) == 0 {
		return "", fmt.Errorf("validation failure repair owner has no sealed coverage")
	}
	profileOwner := ""
	profileMatches := 0
	diagnostic, diagnosticFound := runtimeValidationFailurePersistedDiagnostic(a, observation)
	if !diagnosticFound {
		return "", fmt.Errorf("validation failure repair owner has no exact persisted typed diagnostic")
	}
	diagnostic = diagnostic.Normalized()
	if diagnostic.Class != observation.DiagnosticClass {
		return "", fmt.Errorf("validation failure repair owner diagnostic conflicts with observation")
	}
	for _, coverageID := range coverageIDs {
		declaration, found, err := runtimeDefinitionFocusForCoverage(a.Facts.DefinitionFocuses, coverageID)
		if err != nil {
			return "", err
		}
		if !found || declaration.ValidationFailureRepairConvergence == nil {
			continue
		}
		convergence := declaration.ValidationFailureRepairConvergence.Normalized()
		route, matched := convergence.RouteForTrigger(
			triggerDescriptor.OwnerModuleID,
			operation.Kind,
			diagnostic.Class,
			diagnostic.RecommendedEvidenceTargetType,
			diagnostic.OwnerCode,
		)
		if !matched {
			return "", fmt.Errorf("validation failure repair owner does not match sealed Profile trigger")
		}
		if profileOwner != "" && profileOwner != route.RepairOwnerModuleID {
			return "", fmt.Errorf("validation failure repair owner is ambiguous across sealed Profile declarations")
		}
		profileOwner = route.RepairOwnerModuleID
		profileMatches++
	}
	if profileMatches != 0 {
		if profileMatches != len(coverageIDs) || profileOwner == "" {
			return "", fmt.Errorf("validation failure repair owner has incomplete sealed Profile coverage")
		}
		return profileOwner, nil
	}

	bindingOwner := ""
	for _, binding := range sourceBindings {
		proofMatched := false
		for _, proof := range binding.Proofs {
			proof = proof.Normalized()
			if proof.EvidenceKind != runtimecontract.EvidenceKindStateChange {
				continue
			}
			if proofMatched || len(proof.AllowedOperationKinds) == 0 {
				return "", fmt.Errorf("validation failure repair owner binding %q has ambiguous state-change proof", binding.ID)
			}
			proofMatched = true
			for _, kind := range proof.AllowedOperationKinds {
				descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
				if !found || !runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(kind, runtimecontract.EvidenceKindStateChange) {
					return "", fmt.Errorf("validation failure repair owner binding %q has invalid state-change operation %q", binding.ID, kind)
				}
				if bindingOwner != "" && bindingOwner != descriptor.OwnerModuleID {
					return "", fmt.Errorf("validation failure repair owner is ambiguous across sealed operations")
				}
				bindingOwner = descriptor.OwnerModuleID
			}
		}
		if !proofMatched {
			return "", fmt.Errorf("validation failure repair owner binding %q has no state-change proof", binding.ID)
		}
	}
	if bindingOwner == "" {
		return "", fmt.Errorf("validation failure repair owner has incomplete binding provenance")
	}
	return bindingOwner, nil
}

// CurrentRuntimeRepairSurfaceConfirmationContract resolves the exact
// receiver-owned read protocol for a persisted validation observation. The
// owner still comes from the sealed Profile route (or its exact source binding
// fallback); the operation descriptor only proves that this owner has a
// concrete confirmation adapter. No capability name is selected by SM-2.
func CurrentRuntimeRepairSurfaceConfirmationContract(a RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation) (string, runtimecontract.RuntimeRepairSurfaceConfirmationProtocol, error) {
	ownerModuleID, err := runtimeExpectedRepairSurfaceOwner(a, observation)
	if err != nil {
		return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, err
	}
	observation = observation.Normalized()
	var sourceDispatch *SealedDispatch
	for index := range a.CompletedDispatches {
		dispatch := &a.CompletedDispatches[index]
		if dispatch.ID != observation.DispatchID || dispatch.State != DispatchStateCommitted ||
			!containsString(dispatch.CompletedOperationIDs, observation.OperationID) {
			continue
		}
		if sourceDispatch != nil {
			return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation has multiple source dispatches")
		}
		sourceDispatch = dispatch
	}
	if sourceDispatch == nil {
		return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation has no exact sealed source")
	}
	operation, found := candidateOperationByID(sourceDispatch.Candidate, observation.OperationID)
	if !found {
		return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation source operation is not sealed")
	}
	diagnostic, found := runtimeValidationFailurePersistedDiagnostic(a, observation)
	if !found {
		return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation has no exact persisted typed diagnostic")
	}
	diagnostic = diagnostic.Normalized()
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found {
		return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation source operation is unknown")
	}

	bindingIDs := normalizeRuntimeTargetRefs(operation.BindingRefs)
	sourceBindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(bindingIDs))
	coverageIDs := append([]common.CoverageWorkItemID(nil), operation.CoverageWorkItemIDs...)
	for _, bindingID := range bindingIDs {
		matches := 0
		var binding runtimecontract.CoverageEvidenceBinding
		for index := range sourceDispatch.CoverageBindings {
			candidate := sourceDispatch.CoverageBindings[index].Normalized()
			if candidate.ID == bindingID {
				binding, matches = candidate, matches+1
			}
		}
		if matches != 1 {
			return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation binding %q does not have one exact source projection", bindingID)
		}
		sourceBindings = append(sourceBindings, binding)
		if len(operation.CoverageWorkItemIDs) == 0 {
			coverageIDs = append(coverageIDs, binding.CoverageWorkItemID)
		}
	}
	coverageIDs = normalizeCoverageIDs(coverageIDs)
	profileMatches := 0
	protocol := runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone
	for _, coverageID := range coverageIDs {
		declaration, declarationFound, declarationErr := runtimeDefinitionFocusForCoverage(a.Facts.DefinitionFocuses, coverageID)
		if declarationErr != nil {
			return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, declarationErr
		}
		if !declarationFound || declaration.ValidationFailureRepairConvergence == nil {
			continue
		}
		route, matched := declaration.ValidationFailureRepairConvergence.RouteForTrigger(
			descriptor.OwnerModuleID,
			operation.Kind,
			diagnostic.Class,
			diagnostic.RecommendedEvidenceTargetType,
			diagnostic.OwnerCode,
		)
		if !matched || route.RepairOwnerModuleID != ownerModuleID {
			return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation does not match sealed Profile route")
		}
		candidate, supported := runtimecontract.RuntimeRepairSurfaceConfirmationProtocolForOperationKinds(route.RepairOwnerModuleID, route.RepairOperationKinds)
		if !supported || (protocol != runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone && protocol != candidate) {
			return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation route has no unique owner protocol")
		}
		protocol = candidate
		profileMatches++
	}
	if profileMatches != 0 {
		if profileMatches != len(coverageIDs) || protocol == runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone {
			return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair confirmation has incomplete sealed Profile coverage")
		}
		return ownerModuleID, protocol, nil
	}

	operationKinds := make([]string, 0)
	for _, binding := range sourceBindings {
		for _, proof := range binding.Proofs {
			proof = proof.Normalized()
			if proof.EvidenceKind == runtimecontract.EvidenceKindStateChange {
				operationKinds = append(operationKinds, proof.AllowedOperationKinds...)
			}
		}
	}
	protocol, supported := runtimecontract.RuntimeRepairSurfaceConfirmationProtocolForOperationKinds(ownerModuleID, operationKinds)
	if !supported {
		return "", runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone, fmt.Errorf("validation failure repair owner %q has no concrete repair-surface confirmation protocol", ownerModuleID)
	}
	return ownerModuleID, protocol, nil
}

func runtimeLatestRepairSurfaceConfirmation(a RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation) (runtimecontract.RuntimeRepairSurfaceConfirmation, bool) {
	for idx := len(a.RepairSurfaceConfirmations) - 1; idx >= 0; idx-- {
		record := a.RepairSurfaceConfirmations[idx]
		if record.Validate() != nil {
			continue
		}
		confirmation := record.Confirmation.Normalized()
		if runtimecontract.SameRuntimeValidationFailureObservation(confirmation.Observation, observation) {
			return confirmation, true
		}
	}
	return runtimecontract.RuntimeRepairSurfaceConfirmation{}, false
}

func runtimeTargetObservationMatchesConfirmation(observation *runtimecontract.RuntimeFailureTargetObservation, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) bool {
	if observation == nil {
		return false
	}
	value := observation.Normalized()
	return value.OwnerModuleID == confirmation.OwnerModuleID && value.SubjectRef == confirmation.SubjectRef &&
		value.TargetRole == confirmation.Observation.TargetRole && value.ObservedRevision == confirmation.ObservedRevision &&
		value.DiagnosticClass == "" && len(value.EvidenceRefs) == 0 && value.ContentHash == "" &&
		sameRuntimeTargetRefs(value.TargetRefs, confirmation.ConfirmedTargetRefs)
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
	if a.Termination != nil && a.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry {
		return fmt.Errorf("administratively terminated runtime aggregate can retain only a delivery-retry lease")
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
