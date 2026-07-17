package runtimev2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

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
		Focus                 runtimecontract.CoverageFocus
		RemainingFocuses      []runtimecontract.CoverageFocus
		EvidenceRevision      string
		Failure               *runtimecontract.FailureFact
		Prerequisites         runtimecontract.PrerequisiteState
		Repair                runtimecontract.RuntimeRepairLedger
		Coverage              []runtimecontract.RuntimeCoverageItemFact
		Feasibility           *runtimecontract.RuntimeFeasibilityFact
		State                 runtimecontract.ConvergenceState
		Action                runtimecontract.RuntimeAction
		RequiredOutput        runtimecontract.RequiredOutputContract
		Terminal              *runtimecontract.TerminalOutcome
		ActiveDispatch        *runtimeConvergenceDispatchProgress
		CompletedDispatches   []runtimeConvergenceDispatchProgress
		GovernanceRequests    []runtimecontract.RuntimeGovernanceRequestRecord
		GovernanceResolutions []runtimecontract.RuntimeGovernanceResolutionRecord
	}{
		Focus: facts.Focus, RemainingFocuses: facts.RemainingFocuses,
		EvidenceRevision: facts.EvidenceRevision, Failure: facts.Failure,
		Prerequisites: facts.Prerequisites, Repair: facts.Repair,
		Coverage: facts.Coverage, Feasibility: facts.Feasibility,
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
	payload, _ := json.Marshal(progress)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

type runtimeConvergenceDispatchProgress struct {
	CandidateHash         string
	State                 DispatchState
	CompletedOperationIDs []string
}

func runtimeConvergenceDispatchProgressFrom(dispatch SealedDispatch) runtimeConvergenceDispatchProgress {
	return runtimeConvergenceDispatchProgress{
		CandidateHash: dispatch.CandidateHash, State: dispatch.State,
		CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
	}
}
