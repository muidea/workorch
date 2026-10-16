package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCompleteAdmissionDisposition is the bounded result of the final,
// pure SM-3 dispatch-admission fence. It is a value decision only: the
// Orchestrator owner maps it to a private internal event and remains the sole
// component permitted to mutate the Runtime aggregate.
type RuntimeCompleteAdmissionDisposition string

const (
	RuntimeCompleteAdmissionAdmitted                RuntimeCompleteAdmissionDisposition = "admitted"
	RuntimeCompleteAdmissionCorrectableOnce         RuntimeCompleteAdmissionDisposition = "correctable_once"
	RuntimeCompleteAdmissionPrerequisiteBlocked     RuntimeCompleteAdmissionDisposition = "prerequisite_blocked"
	RuntimeCompleteAdmissionOwnerPrerequisite       RuntimeCompleteAdmissionDisposition = "owner_prerequisite_required"
	RuntimeCompleteAdmissionOwnerStale              RuntimeCompleteAdmissionDisposition = "owner_stale"
	RuntimeCompleteAdmissionOwnerInfeasible         RuntimeCompleteAdmissionDisposition = "owner_infeasible"
	RuntimeCompleteAdmissionDuplicateNoop           RuntimeCompleteAdmissionDisposition = "duplicate_noop"
	RuntimeCompleteAdmissionStaleConflict           RuntimeCompleteAdmissionDisposition = "stale_conflict"
	RuntimeCompleteAdmissionTerminalContractFailure RuntimeCompleteAdmissionDisposition = "terminal_contract_failure"
)

// RuntimeCompleteAdmissionInput is an invocation-local detached view of
// already-persisted canonical records. It is not persisted and deliberately
// introduces no second CWI, binding, request, dispatch, or receipt model.
type RuntimeCompleteAdmissionInput struct {
	Definition          RuntimeDefinitionContext
	ProcessedEventIDs   []string
	Facts               RuntimeFacts
	Decision            RuntimeDecision
	SemanticPlanning    RuntimeSemanticPlanningLedger
	ImplementationWork  RuntimeImplementationWorkLedger
	Reconciliation      RuntimeCWIReconciliationLedger
	Manifest            RuntimeCandidateContractManifest
	Candidate           RuntimeCandidate
	Attempt             RuntimeCandidateAttemptRecord
	ActiveDispatch      *RuntimeDispatchRecord
	CompletedDispatches []RuntimeDispatchRecord
	OperationReceipts   []RuntimeOperationReceipt
	// Original read-only source ledgers required to replay historical ordinary
	// settlements after rebind. These are not reconstructed from current status.
	ProofLineage     []RuntimeCoverageProofLineage
	Governance       []RuntimeOperationGovernanceBinding
	Budget           RuntimeExecutionBudgetLedger
	EvidenceSequence uint64
	Resolutions      []RuntimeGovernanceResolutionRecord
	Attempts         []RuntimeCandidateAttemptRecord
	Sessions         []RuntimeCandidateRequestSession
	Confirmations    []RuntimeRepairSurfaceConfirmationRecord
}

func (i RuntimeCompleteAdmissionInput) Clone() RuntimeCompleteAdmissionInput {
	i.Definition = i.Definition.Normalized()
	i.ProcessedEventIDs = append([]string(nil), i.ProcessedEventIDs...)
	i.Facts = i.Facts.Normalized()
	i.Decision = cloneRuntimeDecision(i.Decision)
	i.SemanticPlanning = i.SemanticPlanning.Clone()
	i.ImplementationWork = i.ImplementationWork.Clone()
	i.Reconciliation = i.Reconciliation.Clone()
	i.Manifest = i.Manifest.Clone()
	i.Candidate = cloneRuntimeCandidate(i.Candidate)
	i.Attempt = cloneRuntimeCompleteAdmissionAttempt(i.Attempt)
	i.Budget = i.Budget.Clone()
	i.ProofLineage = append([]RuntimeCoverageProofLineage(nil), i.ProofLineage...)
	for n := range i.ProofLineage {
		i.ProofLineage[n] = i.ProofLineage[n].Clone()
	}
	i.Governance = append([]RuntimeOperationGovernanceBinding(nil), i.Governance...)
	for n := range i.Governance {
		i.Governance[n] = i.Governance[n].Clone()
	}
	i.Resolutions = append([]RuntimeGovernanceResolutionRecord(nil), i.Resolutions...)
	for n := range i.Resolutions {
		i.Resolutions[n] = i.Resolutions[n].Clone()
	}
	i.Attempts = append([]RuntimeCandidateAttemptRecord(nil), i.Attempts...)
	for n := range i.Attempts {
		i.Attempts[n] = i.Attempts[n].Clone()
	}
	i.Sessions = append([]RuntimeCandidateRequestSession(nil), i.Sessions...)
	for n := range i.Sessions {
		i.Sessions[n] = i.Sessions[n].Clone()
	}
	i.Confirmations = append([]RuntimeRepairSurfaceConfirmationRecord(nil), i.Confirmations...)
	for n := range i.Confirmations {
		i.Confirmations[n] = i.Confirmations[n].Clone()
	}
	return i.cloneExecutionFacts()
}

func cloneRuntimeCompleteAdmissionAttempt(value RuntimeCandidateAttemptRecord) RuntimeCandidateAttemptRecord {
	return value.Clone()
}

func (i RuntimeCompleteAdmissionInput) cloneExecutionFacts() RuntimeCompleteAdmissionInput {
	if i.ActiveDispatch != nil {
		value := i.ActiveDispatch.Clone()
		i.ActiveDispatch = &value
	}
	i.CompletedDispatches = cloneRuntimeDispatchRecords(i.CompletedDispatches)
	i.OperationReceipts = NormalizeRuntimeOperationReceipts(i.OperationReceipts)
	return i
}

func cloneRuntimeDispatchRecords(values []RuntimeDispatchRecord) []RuntimeDispatchRecord {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeDispatchRecord, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

// RuntimeCompleteAdmissionDecision is intentionally small and owner-neutral.
// Code identifies the failed matrix row; Message is diagnostic only and must
// never be parsed to select a transition.
type RuntimeCompleteAdmissionDecision struct {
	Disposition RuntimeCompleteAdmissionDisposition
	Code        string
	Message     string
}

func (d RuntimeCompleteAdmissionDecision) Admitted() bool {
	return d.Disposition == RuntimeCompleteAdmissionAdmitted
}

func completeAdmissionFailure(disposition RuntimeCompleteAdmissionDisposition, code string, err error) RuntimeCompleteAdmissionDecision {
	decision := RuntimeCompleteAdmissionDecision{Disposition: disposition, Code: strings.TrimSpace(code)}
	if err != nil {
		decision.Message = strings.TrimSpace(err.Error())
	}
	return decision
}

// EvaluateRuntimeCompleteAdmission validates the complete final-seal matrix
// using only detached canonical values. It performs no I/O, mutation, action
// selection, correction accounting, or state transition.
func EvaluateRuntimeCompleteAdmission(raw RuntimeCompleteAdmissionInput) RuntimeCompleteAdmissionDecision {
	input := raw.Clone()
	if err := input.Facts.Validate(); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "facts_invalid", err)
	}
	if err := input.Decision.Validate(); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "decision_invalid", err)
	}
	transaction := input.Decision.AuthorityTransaction()
	if input.Facts.WorkspaceID != input.Decision.AuthoritySnapshot.WorkspaceID ||
		input.Facts.RunID != input.Decision.AuthoritySnapshot.RunID ||
		input.Facts.DefinitionRevision != transaction.DefinitionRevision ||
		input.Facts.EvidenceRevision != transaction.EvidenceRevision ||
		input.Facts.Focus.ID != transaction.FocusID {
		return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "snapshot_identity_mismatch", fmt.Errorf("runtime admission facts do not match the sealed authority transaction"))
	}
	if err := ValidateRuntimeCWILedger(input.Facts); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "cwi_ledger_invalid", err)
	}
	if err := ValidateRuntimeHistoricalBehaviorCompletions(RuntimeLineageAssertionSources{
		Definition: input.Definition, Facts: input.Facts, Semantic: input.SemanticPlanning, Completed: input.CompletedDispatches,
		Receipts: input.OperationReceipts, ProcessedEventIDs: input.ProcessedEventIDs, Reconciliation: input.Reconciliation,
		ProofLineage: input.ProofLineage, Work: input.ImplementationWork, Active: input.ActiveDispatch,
		Governance: input.Governance, Budget: input.Budget, EvidenceSequence: input.EvidenceSequence,
		Resolutions: input.Resolutions, Attempts: input.Attempts, Sessions: input.Sessions, Confirmations: input.Confirmations,
	}); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "historical_behavior_lineage_invalid", err)
	}
	if err := ValidateRuntimeCWIFocusBindings(input.Facts); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "cwi_binding_invalid", err)
	}
	if err := ValidateRuntimeCWIReconciliationLedger(input.Facts, input.Reconciliation); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "reconciliation_generation_stale", err)
	}
	if blocked := runtimeCurrentFocusBlockedPrerequisites(input.Facts); len(blocked) != 0 {
		return completeAdmissionFailure(RuntimeCompleteAdmissionPrerequisiteBlocked, "cwi_prerequisite_blocked", fmt.Errorf("runtime current CWI prerequisites are not settled: %s", strings.Join(blocked, ",")))
	}
	workPurpose := input.Decision.RequiredOutput.ImplementationWorkBindingHash != ""
	if workPurpose {
		if _, err := ValidateRuntimeImplementationWorkAdmission(input.Facts, input.Decision, input.SemanticPlanning, input.ImplementationWork, input.Manifest); err != nil {
			return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "implementation_work_authority_invalid", err)
		}
	} else if disposition, err := validateRuntimeCompleteAdmissionSemanticAuthority(input.Facts, input.SemanticPlanning); err != nil {
		code := "semantic_authority_invalid"
		if disposition == RuntimeCompleteAdmissionStaleConflict {
			code = "semantic_authority_unsealed"
		}
		return completeAdmissionFailure(disposition, code, err)
	}
	ownerFacts := RuntimeCandidateManifestOwnerFactsFromContext(RuntimeCandidateContextForDecision(input.Facts, input.Decision, nil))
	if workPurpose {
		binding := input.Manifest.ImplementationWorkingBinding.Clone()
		ownerFacts.ImplementationWorkingBinding = &binding
		ownerFacts.TargetSelection = nil
		ownerFacts.OperationAdmissibilities = []RuntimeOperationAdmissibility{binding.OwnerCatalog.Clone()}
	}
	if input.Decision.State == ConvergenceStateNeedAssessment {
		focus := input.Decision.Focus.Normalized()
		if _, err := RuntimeAssessmentSourceProjection(input.Facts, focus.ID, focus.CoverageWorkItemIDs); err != nil {
			return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "assessment_definition_preimage_invalid", err)
		}
		preimage, err := ResolveRuntimeAssessmentPurposePreimage(input.SemanticPlanning, focus.CoverageEvidenceBindings)
		if err != nil {
			return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "assessment_purpose_preimage_invalid", err)
		}
		ownerFacts.AssessmentPurpose = preimage.Purpose
		ownerFacts.AssessmentPurposeManifestHash = preimage.SemanticManifestHash
	}
	if err := ValidateRuntimeCandidateContractManifestForDecision(input.Manifest, input.Decision, ownerFacts); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "candidate_manifest_stale", err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(input.Manifest, input.Candidate); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionCorrectableOnce, "candidate_manifest_violation", err)
	}
	if err := input.Candidate.ValidateWithActivation(input.Decision, RuntimeActivationForFacts(input.Facts)); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionCorrectableOnce, "candidate_contract_invalid", err)
	}
	contractID := StableAuthorityContractID(input.Facts.WorkspaceID, input.Facts.RunID, transaction)
	if input.Attempt.AuthorityContractID != contractID || !SameAuthorityContract(input.Attempt.Transaction, transaction) ||
		input.Attempt.ManifestHash != input.Manifest.Hash || strings.TrimSpace(input.Attempt.LogicalRequestID) == "" ||
		strings.TrimSpace(input.Attempt.StablePrefixHash) == "" || input.Attempt.Attempt < 0 || input.Attempt.Attempt > 1 {
		return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "candidate_attempt_identity_mismatch", fmt.Errorf("runtime candidate attempt does not match current authority"))
	}
	if input.Attempt.State != RuntimeCandidateAttemptStatePending {
		return completeAdmissionFailure(RuntimeCompleteAdmissionDuplicateNoop, "candidate_attempt_not_pending", fmt.Errorf("runtime candidate attempt is already %s", input.Attempt.State))
	}
	if input.Attempt.Proposal == nil || StableRuntimeCandidateHash(*input.Attempt.Proposal) != StableRuntimeCandidateHash(input.Candidate) {
		return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "candidate_proposal_mismatch", fmt.Errorf("runtime candidate differs from the persisted proposal"))
	}
	if input.ActiveDispatch != nil {
		if input.ActiveDispatch.Transaction == transaction && input.ActiveDispatch.CandidateHash == StableRuntimeCandidateHash(input.Candidate) {
			return RuntimeCompleteAdmissionDecision{Disposition: RuntimeCompleteAdmissionDuplicateNoop, Code: "dispatch_already_sealed"}
		}
		return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "active_dispatch_conflict", fmt.Errorf("runtime authority already has a different active dispatch"))
	}
	if feasibility := evaluateRuntimeCompleteAdmissionFeasibility(input.Facts, input.Manifest, input.Candidate, input.Attempt.Feasibilities); !feasibility.Admitted() {
		return feasibility
	}
	if err := ValidateRuntimeCWIBindingRetirementsAgainstExecution(input.Facts, input.ActiveDispatch, input.CompletedDispatches, input.OperationReceipts); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "binding_retirement_execution_conflict", err)
	}
	return RuntimeCompleteAdmissionDecision{Disposition: RuntimeCompleteAdmissionAdmitted, Code: "admitted"}
}

// validateRuntimeCompleteAdmissionSemanticAuthority distinguishes the
// provider/session generation from the binding authority it produced. A later
// catalog invalidation may retire the current semantic session, but it must not
// revoke an already-sealed CWI binding. The binding keeps the immutable
// semantic-manifest lineage required by final admission.
func validateRuntimeCompleteAdmissionSemanticAuthority(facts RuntimeFacts, ledger RuntimeSemanticPlanningLedger) (RuntimeCompleteAdmissionDisposition, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeCompleteAdmissionTerminalContractFailure, err
	}
	if ledger.Phase == RuntimeSemanticPlanningSealed {
		if ledger.Manifest == nil || ledger.SealedBindingSetHash == "" || ledger.SealedBindingSetHash != facts.ReconciliationContract.BindingSetHash {
			return RuntimeCompleteAdmissionTerminalContractFailure, fmt.Errorf("runtime sealed semantic authority does not match the current binding set")
		}
		return RuntimeCompleteAdmissionAdmitted, nil
	}
	byCoverage := make(map[common.CoverageWorkItemID]RuntimeCoverageItemFact, len(facts.Coverage))
	for _, item := range facts.Coverage {
		byCoverage[item.ID] = item
	}
	for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
		item, found := byCoverage[coverageID]
		if !found || item.CurrentBindingRef == nil || strings.TrimSpace(item.CurrentBindingRef.SemanticManifestHash) == "" {
			return RuntimeCompleteAdmissionStaleConflict, fmt.Errorf("runtime current CWI has no sealed semantic binding lineage")
		}
	}
	return RuntimeCompleteAdmissionAdmitted, nil
}

func runtimeCurrentFocusBlockedPrerequisites(facts RuntimeFacts) []string {
	facts = facts.Normalized()
	byID := make(map[string]RuntimeCoverageItemFact, len(facts.Coverage))
	for _, item := range facts.Coverage {
		byID[string(item.ID)] = item
	}
	blocked := make([]string, 0)
	for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
		item, found := byID[string(coverageID)]
		if !found {
			blocked = append(blocked, string(coverageID))
			continue
		}
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			prerequisite, found := byID[string(prerequisiteID)]
			if !found || !RuntimeCWIPrerequisiteReadyForConsumer(facts, prerequisite, coverageID) {
				blocked = append(blocked, string(prerequisiteID))
			}
		}
	}
	return normalizeStrings(blocked)
}

func evaluateRuntimeCompleteAdmissionFeasibility(facts RuntimeFacts, manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate, values []RuntimeCandidateFeasibilityRecord) RuntimeCompleteAdmissionDecision {
	if err := ValidateRuntimeCandidateFeasibilityRecords(values); err != nil {
		return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "owner_feasibility_invalid", err)
	}
	byOperation := make(map[string]RuntimeCandidateFeasibilityRecord, len(values))
	for _, value := range values {
		byOperation[strings.TrimSpace(value.OperationID)] = value
	}
	for _, operation := range runtimeCandidateAdmissionOperations(candidate) {
		descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
		if !known || descriptor.OwnerModuleID == "" {
			return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "operation_owner_unknown", fmt.Errorf("candidate operation %q has no canonical owner", operation.Kind))
		}
		inventory, required := runtimeAdmissionAdmissibilityForOperation(facts, descriptor.OwnerModuleID, operation.Kind)
		if !required {
			// A declared dynamic constraint cannot silently take the static
			// capability seal lane when the owner prerequisite is absent.
			for _, authorization := range manifest.Operations {
				if authorization.Kind != operation.Kind {
					continue
				}
				_, constraints, err := RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
				if err != nil {
					return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "owner_constraint_invalid", err)
				}
				for _, constraint := range constraints {
					if constraint.Kind == RuntimeOperationCandidateConstraintTargetApplicability {
						return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "owner_catalog_missing", fmt.Errorf("candidate operation %q has dynamic constraints without owner admissibility", operation.ID))
					}
				}
			}
			continue
		}
		if inventory.SnapshotID == "" {
			return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "owner_catalog_missing", fmt.Errorf("candidate operation %q has no persisted owner admissibility inventory", operation.ID))
		}
		observation, found := byOperation[operation.ID]
		if !found || observation.AdmissibilitySnapshotID != inventory.SnapshotID || observation.Observation == nil {
			return completeAdmissionFailure(RuntimeCompleteAdmissionOwnerInfeasible, "owner_feasibility_incomplete", fmt.Errorf("candidate operation %q has no current persisted feasibility result", operation.ID))
		}
		if observation.CandidateManifestHash != strings.TrimSpace(manifest.Hash) {
			return completeAdmissionFailure(RuntimeCompleteAdmissionOwnerStale, "owner_feasibility_manifest_stale", fmt.Errorf("candidate operation %q feasibility manifest identity is stale", operation.ID))
		}
		if observation.AcquisitionBinding != inventory.AcquisitionBinding {
			return completeAdmissionFailure(RuntimeCompleteAdmissionOwnerStale, "owner_feasibility_catalog_stale", fmt.Errorf("candidate operation %q feasibility acquisition binding is stale", operation.ID))
		}
		if observation.ReplayProof != nil {
			proof := observation.ReplayProof.Normalized()
			if proof.OwnerModuleID != descriptor.OwnerModuleID || proof.OperationID != operation.ID ||
				proof.AdmissibilitySnapshotID != inventory.SnapshotID || proof.AcquisitionBinding != inventory.AcquisitionBinding ||
				!sameRuntimeStringSet(proof.TargetRefs, operation.TargetRefs) {
				return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "owner_replay_proof_mismatch", fmt.Errorf("candidate operation %q replay proof does not match sealed owner facts", operation.ID))
			}
		}
		switch observation.Observation.Outcome {
		case RuntimeOwnerFeasibilityFeasible:
			// Continue only after every required owner has returned an exact fact.
		case RuntimeOwnerFeasibilityStale:
			return completeAdmissionFailure(RuntimeCompleteAdmissionOwnerStale, "owner_stale", fmt.Errorf("candidate operation %q owner fact is stale", operation.ID))
		case RuntimeOwnerFeasibilityPrerequisiteRequired:
			return completeAdmissionFailure(RuntimeCompleteAdmissionOwnerPrerequisite, "owner_prerequisite_required", fmt.Errorf("candidate operation %q requires an owner prerequisite", operation.ID))
		case RuntimeOwnerFeasibilityCandidateConstraintViolated:
			return completeAdmissionFailure(RuntimeCompleteAdmissionOwnerInfeasible, "candidate_constraint_violated", fmt.Errorf("candidate operation %q violates a sealed owner candidate constraint", operation.ID))
		case RuntimeOwnerFeasibilityInfeasible:
			return completeAdmissionFailure(RuntimeCompleteAdmissionOwnerInfeasible, "owner_infeasible", fmt.Errorf("candidate operation %q is not owner-feasible", operation.ID))
		case RuntimeOwnerFeasibilityContractDefect:
			return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "owner_contract_defect", fmt.Errorf("candidate operation %q owner contract is defective", operation.ID))
		default:
			return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "owner_feasibility_unknown", fmt.Errorf("candidate operation %q has unknown feasibility outcome %q", operation.ID, observation.Observation.Outcome))
		}
	}
	return RuntimeCompleteAdmissionDecision{Disposition: RuntimeCompleteAdmissionAdmitted, Code: "owner_feasibility_admitted"}
}

func runtimeAdmissionAdmissibilityForOperation(facts RuntimeFacts, owner, kind string) (RuntimeOperationAdmissibility, bool) {
	required := RuntimeOperationAdmissibilityRequiredForOperation(facts.Prerequisites, owner, kind)
	if !required {
		return RuntimeOperationAdmissibility{}, false
	}
	for _, inventory := range facts.Resources.OperationAdmissibilities {
		if inventory.OwnerModuleID != owner {
			continue
		}
		for _, allowed := range inventory.OperationKinds {
			if CanonicalRuntimeBlockKind(allowed) == CanonicalRuntimeBlockKind(kind) {
				return inventory, true
			}
		}
	}
	return RuntimeOperationAdmissibility{}, true
}

// RuntimeOperationAdmissibilityRequiredForOperation is the pure owner/kind
// scope rule used by both prerequisite fixtures and complete admission.
func RuntimeOperationAdmissibilityRequiredForOperation(prerequisites PrerequisiteState, owner, kind string) bool {
	owner = strings.TrimSpace(owner)
	kind = CanonicalRuntimeBlockKind(kind)
	for _, prerequisite := range append(append([]RuntimePrerequisite(nil), prerequisites.Pending...), prerequisites.Satisfied...) {
		if prerequisite.Kind != PrerequisiteKindOperationAdmissibility || strings.TrimSpace(prerequisite.OwnerModuleID) != owner {
			continue
		}
		if len(prerequisite.RequiredCapabilities) == 0 {
			return true
		} else {
			for _, capability := range prerequisite.RequiredCapabilities {
				if CanonicalRuntimeBlockKind(capability) == kind {
					return true
				}
			}
		}
	}
	return false
}

func runtimeCandidateAdmissionOperations(candidate RuntimeCandidate) []RuntimeOperation {
	ret := make([]RuntimeOperation, 0)
	if candidate.StateChange != nil {
		for _, value := range candidate.StateChange.Operations {
			ret = append(ret, value.Operation.Normalized())
		}
	}
	if candidate.Validation != nil {
		for _, value := range candidate.Validation.Operations {
			ret = append(ret, value.Operation.Normalized())
		}
	}
	return ret
}
