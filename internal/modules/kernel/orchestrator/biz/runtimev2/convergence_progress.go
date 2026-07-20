package runtimev2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeConvergenceProgressSignature captures persisted SM-2/SM-3 semantic
// progress while deliberately excluding StateRevision, authority attempt and
// candidate retry bookkeeping. A drive transition that changes only those
// bookkeeping fields is not positive convergence.
func RuntimeConvergenceProgressSignature(aggregate RuntimeAggregate) string {
	facts := aggregate.Facts.Normalized()
	decision := aggregate.Decision
	progress := struct {
		Focus                      runtimecontract.CoverageFocus
		RemainingFocuses           []runtimecontract.CoverageFocus
		EvidenceRevision           string
		Failure                    *runtimecontract.FailureFact
		Prerequisites              runtimecontract.PrerequisiteState
		Repair                     runtimecontract.RuntimeRepairLedger
		RepairDirective            *runtimecontract.RuntimeRepairDirective
		Recovery                   runtimecontract.RuntimeRecoveryLedger
		FreshnessReacquireCoverage []common.CoverageWorkItemID
		PrerequisiteObservations   []runtimecontract.RuntimePrerequisiteObservation
		PrerequisiteInvalidations  []runtimecontract.RuntimePrerequisiteInvalidationRecord
		Coverage                   []runtimecontract.RuntimeCoverageItemFact
		Feasibility                *runtimecontract.RuntimeFeasibilityFact
		State                      runtimecontract.ConvergenceState
		Action                     runtimecontract.RuntimeAction
		RequiredOutput             runtimecontract.RequiredOutputContract
		Terminal                   *runtimecontract.TerminalOutcome
		CandidateAdmissions        []runtimeConvergenceCandidateAdmission
		ActiveDispatch             *runtimeConvergenceDispatchProgress
		CompletedDispatches        []runtimeConvergenceDispatchProgress
		GovernanceRequests         []runtimecontract.RuntimeGovernanceRequestRecord
		GovernanceResolutions      []runtimecontract.RuntimeGovernanceResolutionRecord
	}{
		Focus: facts.Focus, RemainingFocuses: facts.RemainingFocuses,
		EvidenceRevision: facts.EvidenceRevision, Failure: facts.Failure,
		Prerequisites: facts.Prerequisites, Repair: facts.Repair, RepairDirective: facts.RepairDirective, Recovery: facts.Recovery,
		FreshnessReacquireCoverage: facts.FreshnessReacquireCoverage,
		PrerequisiteObservations:   facts.Resources.PrerequisiteObservations,
		PrerequisiteInvalidations:  append([]runtimecontract.RuntimePrerequisiteInvalidationRecord(nil), aggregate.PrerequisiteInvalidations...),
		Coverage:                   facts.Coverage, Feasibility: facts.Feasibility,
		State: decision.State, Action: decision.Action,
		RequiredOutput: decision.RequiredOutput.Normalized(), Terminal: decision.Terminal,
		GovernanceRequests:    append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), aggregate.GovernanceRequests...),
		GovernanceResolutions: append([]runtimecontract.RuntimeGovernanceResolutionRecord(nil), aggregate.GovernanceResolutions...),
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
			Feasibilities: append([]runtimecontract.RuntimeCandidateFeasibilityRecord(nil), attempt.Feasibilities...),
		}
		if attempt.Proposal != nil {
			value.ProposalHash = runtimecontract.StableRuntimeCandidateHash(*attempt.Proposal)
		}
		progress.CandidateAdmissions = append(progress.CandidateAdmissions, value)
	}
	payload, _ := json.Marshal(progress)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

type runtimeConvergenceDispatchProgress struct {
	CandidateHash         string
	State                 DispatchState
	CompletedOperationIDs []string
}

// runtimeConvergenceCandidateAdmission includes the persisted SM-3 proposal
// and owner feasibility evidence, while deliberately excluding attempt number,
// state and provider failure bookkeeping.
type runtimeConvergenceCandidateAdmission struct {
	AuthorityContractID string
	Transaction         runtimecontract.AuthorityTransaction
	ProposalHash        string
	Feasibilities       []runtimecontract.RuntimeCandidateFeasibilityRecord
}

func runtimeConvergenceDispatchProgressFrom(dispatch SealedDispatch) runtimeConvergenceDispatchProgress {
	return runtimeConvergenceDispatchProgress{
		CandidateHash: dispatch.CandidateHash, State: dispatch.State,
		CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
	}
}
