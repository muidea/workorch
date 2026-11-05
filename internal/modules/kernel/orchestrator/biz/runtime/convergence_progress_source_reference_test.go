package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Frozen full-input progress algorithm. This is an offline oracle only; it
// cannot participate in Runtime decisions or rebuild production authority.
func referenceFullInputConvergenceProgressSignature(aggregate RuntimeAggregate) string {
	facts := aggregate.Facts.Normalized()
	decision := aggregate.Decision
	progress := struct {
		Focus                     rc.CoverageFocus
		RemainingFocuses          []rc.CoverageFocus
		CompletedFocuses          []rc.CoverageFocus
		EvidenceRevision          string
		Failure                   *rc.FailureFact
		Prerequisites             rc.PrerequisiteState
		Repair                    rc.RuntimeRepairLedger
		RepairDirective           *rc.RuntimeRepairDirective
		PrerequisiteObservations  []rc.RuntimePrerequisiteObservation
		PrerequisiteInvalidations []rc.RuntimePrerequisiteInvalidationRecord
		CoverageProofLineage      []rc.RuntimeCoverageProofLineage
		Coverage                  []rc.RuntimeCoverageItemFact
		Feasibility               *rc.RuntimeFeasibilityFact
		State                     rc.ConvergenceState
		RequiredOutput            rc.RequiredOutputContract
		Terminal                  *rc.RuntimeTerminalDecision
		SemanticPlanning          rc.RuntimeSemanticPlanningLedger
		ImplementationWork        rc.RuntimeImplementationWorkLedger
		CandidateAdmissions       []runtimeConvergenceCandidateAdmission
		ActiveDispatch            *runtimeConvergenceDispatchProgress
		CompletedDispatches       []runtimeConvergenceDispatchProgress
		GovernanceRequests        []rc.RuntimeGovernanceRequestRecord
		GovernanceResolutions     []rc.RuntimeGovernanceResolutionRecord
	}{
		Focus: facts.Focus, RemainingFocuses: facts.RemainingFocuses, CompletedFocuses: facts.CompletedFocuses,
		EvidenceRevision: facts.EvidenceRevision, Failure: facts.Failure,
		Prerequisites: facts.Prerequisites, Repair: facts.Repair, RepairDirective: facts.RepairDirective,
		PrerequisiteObservations:  facts.Resources.PrerequisiteObservations,
		PrerequisiteInvalidations: append([]rc.RuntimePrerequisiteInvalidationRecord(nil), aggregate.PrerequisiteInvalidations...),
		CoverageProofLineage:      rc.NormalizeRuntimeCoverageProofLineage(aggregate.CoverageProofLineage),
		Coverage:                  facts.Coverage, Feasibility: facts.Feasibility,
		State:          decision.State,
		RequiredOutput: decision.RequiredOutput.Normalized(), Terminal: decision.Terminal,
		SemanticPlanning:      aggregate.SemanticPlanning.CloneRaw(),
		ImplementationWork:    aggregate.ImplementationWork.Clone(),
		GovernanceRequests:    append([]rc.RuntimeGovernanceRequestRecord(nil), aggregate.GovernanceRequests...),
		GovernanceResolutions: rc.CloneRuntimeGovernanceResolutionRecords(aggregate.GovernanceResolutions),
	}
	if aggregate.ActiveDispatch != nil {
		value := runtimeConvergenceDispatchProgressFrom(*aggregate.ActiveDispatch)
		progress.ActiveDispatch = &value
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		progress.CompletedDispatches = append(progress.CompletedDispatches, runtimeConvergenceDispatchProgressFrom(dispatch))
	}
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.Proposal == nil && len(attempt.Feasibilities) == 0 {
			continue
		}
		value := runtimeConvergenceCandidateAdmission{
			AuthorityContractID: attempt.AuthorityContractID, Transaction: attempt.Transaction,
			Feasibilities: append([]rc.RuntimeCandidateFeasibilityRecord(nil), attempt.Feasibilities...),
		}
		if attempt.Proposal != nil {
			value.ProposalHash = rc.StableRuntimeCandidateHash(*attempt.Proposal)
		}
		progress.CandidateAdmissions = append(progress.CandidateAdmissions, value)
	}
	payload, _ := json.Marshal(progress)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
