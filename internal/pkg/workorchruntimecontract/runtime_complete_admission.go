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

// RuntimeCompleteAdmissionInput borrows read-only, already-admitted owner
// values for the current seal. Proposal is the persisted admission product;
// unaccepted external input must enter candidate admission first. Historical
// business judgments are not inputs to this transaction fence.
type RuntimeCompleteAdmissionInput struct {
	Facts              RuntimeFacts
	Decision           RuntimeDecision
	SemanticPlanning   RuntimeSemanticPlanningLedger
	ImplementationWork RuntimeImplementationWorkLedger
	Manifest           RuntimeCandidateContractManifest
	Candidate          RuntimeCandidate
	Attempt            RuntimeCandidateAttemptRecord
	ActiveDispatch     *RuntimeDispatchRecord
}

// RuntimeCompleteAdmissionDecision is intentionally small and owner-neutral.
// Code identifies the failed matrix row; Message is diagnostic only and must
// never be parsed to select a transition.
type RuntimeCompleteAdmissionDecision struct {
	Disposition RuntimeCompleteAdmissionDisposition
	Code        string
	Message     string
}

// RuntimeCompleteAdmissionRejection preserves the final owner decision across
// the drive boundary. It never grants a retry or weakens the final gate.
type RuntimeCompleteAdmissionRejection struct {
	Decision RuntimeCompleteAdmissionDecision
	Cause    error
}

func (e *RuntimeCompleteAdmissionRejection) Error() string {
	message := e.Decision.Message
	if message == "" && e.Cause != nil {
		message = e.Cause.Error()
	}
	return fmt.Sprintf("complete candidate admission %s/%s: %s", e.Decision.Disposition, e.Decision.Code, message)
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

// EvaluateRuntimeCompleteAdmission joins the exact persisted proposal to the
// current authority and owner feasibility facts. It never repeats proposal or
// historical aggregate admission, copies history, or mutates the borrowed input.
func EvaluateRuntimeCompleteAdmission(input RuntimeCompleteAdmissionInput) RuntimeCompleteAdmissionDecision {
	transaction := input.Decision.AuthorityTransaction()
	if input.Facts.WorkspaceID != input.Decision.AuthoritySnapshot.WorkspaceID ||
		input.Facts.RunID != input.Decision.AuthoritySnapshot.RunID ||
		input.Facts.DefinitionRevision != transaction.DefinitionRevision ||
		input.Facts.EvidenceRevision != transaction.EvidenceRevision ||
		input.Facts.Focus.ID != transaction.FocusID {
		return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "snapshot_identity_mismatch", fmt.Errorf("runtime admission facts do not match the sealed authority transaction"))
	}
	if blocked := runtimeCurrentFocusBlockedPrerequisites(input.Facts); len(blocked) != 0 {
		return completeAdmissionFailure(RuntimeCompleteAdmissionPrerequisiteBlocked, "cwi_prerequisite_blocked", fmt.Errorf("runtime current CWI prerequisites are not settled: %s", strings.Join(blocked, ",")))
	}
	workPurpose := input.Decision.RequiredOutput.ImplementationWorkBindingHash != ""
	if workPurpose {
		if _, err := ValidateRuntimeImplementationWorkAdmission(input.Facts, input.Decision, input.ImplementationWork, input.Manifest); err != nil {
			return completeAdmissionFailure(RuntimeCompleteAdmissionStaleConflict, "implementation_work_authority_invalid", err)
		}
	} else if disposition, err := validateRuntimeCompleteAdmissionSemanticAuthority(input.Facts, input.SemanticPlanning); err != nil {
		code := "semantic_authority_invalid"
		if disposition == RuntimeCompleteAdmissionStaleConflict {
			code = "semantic_authority_unsealed"
		}
		return completeAdmissionFailure(disposition, code, err)
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
	if feasibility := evaluateRuntimeCompleteAdmissionFeasibility(input.Facts, input.Manifest, input.Candidate, input.Attempt.Feasibilities, input.Decision.AuthorityTransaction()); !feasibility.Admitted() {
		return feasibility
	}
	return RuntimeCompleteAdmissionDecision{Disposition: RuntimeCompleteAdmissionAdmitted, Code: "admitted"}
}

// validateRuntimeCompleteAdmissionSemanticAuthority distinguishes the
// provider/session generation from the binding authority it produced. A later
// catalog invalidation may retire the current semantic session, but it must not
// revoke an already-sealed CWI binding. The binding keeps the immutable
// semantic-manifest lineage required by final admission.
func validateRuntimeCompleteAdmissionSemanticAuthority(facts RuntimeFacts, ledger RuntimeSemanticPlanningLedger) (RuntimeCompleteAdmissionDisposition, error) {
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

func evaluateRuntimeCompleteAdmissionFeasibility(facts RuntimeFacts, manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate, values []RuntimeCandidateFeasibilityRecord, transaction AuthorityTransaction) RuntimeCompleteAdmissionDecision {
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
		if err := ValidateRuntimeInputStructureRecord(manifest, operation, observation, transaction); err != nil {
			return completeAdmissionFailure(RuntimeCompleteAdmissionTerminalContractFailure, "input_structure_invalid", err)
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
