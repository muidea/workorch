package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeConvergenceProgressPayload struct {
	Focus                     runtimecontract.CoverageFocus
	RemainingFocuses          []runtimecontract.CoverageFocus
	CompletedFocuses          []runtimecontract.CoverageFocus
	EvidenceRevision          string
	Failure                   *runtimecontract.FailureFact
	Prerequisites             runtimecontract.PrerequisiteState
	Repair                    runtimecontract.RuntimeRepairLedger
	RepairDirective           *runtimecontract.RuntimeRepairDirective
	PrerequisiteObservations  []runtimecontract.RuntimePrerequisiteObservation
	PrerequisiteInvalidations []runtimecontract.RuntimePrerequisiteInvalidationRecord
	CoverageProofLineage      []runtimecontract.RuntimeCoverageProofLineage
	Coverage                  []runtimecontract.RuntimeCoverageItemFact
	Feasibility               *runtimecontract.RuntimeFeasibilityFact
	State                     runtimecontract.ConvergenceState
	RequiredOutput            runtimecontract.RequiredOutputContract
	Terminal                  *runtimecontract.RuntimeTerminalDecision
	SemanticPlanning          runtimecontract.RuntimeSemanticPlanningLedger
	ImplementationWork        runtimecontract.RuntimeImplementationWorkLedger
	CandidateAdmissions       []runtimeConvergenceCandidateAdmission
	ActiveDispatch            *runtimeConvergenceDispatchProgress
	CompletedDispatches       []runtimeConvergenceDispatchProgress
	GovernanceRequests        []runtimecontract.RuntimeGovernanceRequestRecord
	GovernanceResolutions     []runtimecontract.RuntimeGovernanceResolutionRecord
}

func runtimeConvergenceProgressValue(aggregate RuntimeAggregate) runtimeConvergenceProgressPayload {
	// A progress observation consumes only these fields. Full Facts ownership
	// does not make unrelated catalogs, evidence bodies or Definition inputs
	// part of this query. The canonical field rules remain in one normalizer.
	facts := (runtimecontract.RuntimeFacts{
		Focus: aggregate.Facts.Focus, RemainingFocuses: aggregate.Facts.RemainingFocuses,
		CompletedFocuses: aggregate.Facts.CompletedFocuses, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		Failure: aggregate.Facts.Failure, Prerequisites: aggregate.Facts.Prerequisites,
		Repair: aggregate.Facts.Repair, RepairDirective: aggregate.Facts.RepairDirective,
		Resources: runtimecontract.RuntimeResourceFacts{PrerequisiteObservations: aggregate.Facts.Resources.PrerequisiteObservations},
		Coverage:  aggregate.Facts.Coverage, Feasibility: aggregate.Facts.Feasibility,
	}).Normalized()
	decision := aggregate.Decision
	progress := runtimeConvergenceProgressPayload{
		Focus: facts.Focus, RemainingFocuses: facts.RemainingFocuses, CompletedFocuses: facts.CompletedFocuses,
		EvidenceRevision: facts.EvidenceRevision, Failure: facts.Failure,
		Prerequisites: facts.Prerequisites, Repair: facts.Repair, RepairDirective: facts.RepairDirective,
		PrerequisiteObservations:  facts.Resources.PrerequisiteObservations,
		PrerequisiteInvalidations: append([]runtimecontract.RuntimePrerequisiteInvalidationRecord(nil), aggregate.PrerequisiteInvalidations...),
		CoverageProofLineage:      runtimecontract.NormalizeRuntimeCoverageProofLineage(aggregate.CoverageProofLineage),
		Coverage:                  facts.Coverage, Feasibility: facts.Feasibility,
		State:          decision.State,
		RequiredOutput: decision.RequiredOutput.Normalized(), Terminal: decision.Terminal,
		SemanticPlanning:      aggregate.SemanticPlanning,
		ImplementationWork:    aggregate.ImplementationWork.Clone(),
		GovernanceRequests:    append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), aggregate.GovernanceRequests...),
		GovernanceResolutions: runtimecontract.CloneRuntimeGovernanceResolutionRecords(aggregate.GovernanceResolutions),
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
	return progress
}

type trimTrailingNewlineHashWriter struct {
	hasher     hash.Hash
	hasPending bool
	pending    byte
}

func (w *trimTrailingNewlineHashWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.hasPending {
		if _, err := w.hasher.Write([]byte{w.pending}); err != nil {
			return 0, err
		}
	}
	last := len(p) - 1
	if last > 0 {
		if _, err := w.hasher.Write(p[:last]); err != nil {
			return 0, err
		}
	}
	w.pending = p[last]
	w.hasPending = true
	return len(p), nil
}

func (w *trimTrailingNewlineHashWriter) finish() {
	if w.hasPending && w.pending != '\n' {
		_, _ = w.hasher.Write([]byte{w.pending})
	}
	w.hasPending = false
}

// RuntimeConvergenceProgressSignature captures persisted SM-2/SM-3 semantic
// progress while deliberately excluding StateRevision, authority attempt and
// candidate retry bookkeeping. A drive transition that changes only those
// bookkeeping fields is not positive convergence.
func RuntimeConvergenceProgressSignature(aggregate RuntimeAggregate) string {
	progress := runtimeConvergenceProgressValue(aggregate)
	hw := &trimTrailingNewlineHashWriter{hasher: sha256.New()}
	enc := json.NewEncoder(hw)
	_ = enc.Encode(progress)
	hw.finish()
	return hex.EncodeToString(hw.hasher.Sum(nil))
}

// RuntimeConvergenceProgressEqual reports whether left and right have equal
// convergence progress without marshaling large SemanticPlanning or dispatch
// ledgers when fast structural checks already establish inequality or equality.
func RuntimeConvergenceProgressEqual(left, right RuntimeAggregate) bool {
	if left.Decision.State != right.Decision.State ||
		left.SemanticPlanning.Phase != right.SemanticPlanning.Phase ||
		left.SemanticPlanning.ActiveBatchID != right.SemanticPlanning.ActiveBatchID ||
		len(left.SemanticPlanning.History) != len(right.SemanticPlanning.History) ||
		len(left.SemanticPlanning.Batches) != len(right.SemanticPlanning.Batches) ||
		len(left.SemanticPlanning.BatchDocuments) != len(right.SemanticPlanning.BatchDocuments) ||
		len(left.SemanticPlanning.Feasibilities) != len(right.SemanticPlanning.Feasibilities) ||
		len(left.SemanticPlanning.BatchFeasibilityResults) != len(right.SemanticPlanning.BatchFeasibilityResults) ||
		(left.SemanticPlanning.Manifest == nil) != (right.SemanticPlanning.Manifest == nil) ||
		(left.SemanticPlanning.CoverageOrderDocument == nil) != (right.SemanticPlanning.CoverageOrderDocument == nil) ||
		(left.SemanticPlanning.MergedDocument == nil) != (right.SemanticPlanning.MergedDocument == nil) ||
		(left.ActiveDispatch == nil) != (right.ActiveDispatch == nil) ||
		len(left.CompletedDispatches) != len(right.CompletedDispatches) ||
		len(left.GovernanceRequests) != len(right.GovernanceRequests) ||
		len(left.GovernanceResolutions) != len(right.GovernanceResolutions) ||
		len(left.PrerequisiteInvalidations) != len(right.PrerequisiteInvalidations) ||
		len(left.ImplementationWork.Rounds) != len(right.ImplementationWork.Rounds) ||
		len(left.ImplementationWork.PurposeChoices) != len(right.ImplementationWork.PurposeChoices) ||
		(left.ImplementationWork.ScopePreview == nil) != (right.ImplementationWork.ScopePreview == nil) ||
		strings.TrimSpace(left.Facts.EvidenceRevision) != strings.TrimSpace(right.Facts.EvidenceRevision) ||
		(left.Facts.Failure == nil) != (right.Facts.Failure == nil) ||
		(left.Facts.RepairDirective == nil) != (right.Facts.RepairDirective == nil) ||
		(left.Facts.Feasibility == nil) != (right.Facts.Feasibility == nil) ||
		(left.Decision.Terminal == nil) != (right.Decision.Terminal == nil) {
		return false
	}
	if !runtimeConvergenceDispatchesEqual(left.ActiveDispatch, right.ActiveDispatch, left.CompletedDispatches, right.CompletedDispatches) {
		return false
	}
	if !runtimeConvergenceCandidateAdmissionsEqual(left.CandidateAttempts, right.CandidateAttempts) {
		return false
	}
	if !runtimecontract.RuntimeRawEqual(left.Decision.Terminal, right.Decision.Terminal) ||
		!runtimecontract.RuntimeRawEqual(left.SemanticPlanning, right.SemanticPlanning) ||
		!runtimecontract.RuntimeRawEqual(left.ImplementationWork, right.ImplementationWork) ||
		!runtimeEmptyOrRawEqualSlice(left.PrerequisiteInvalidations, right.PrerequisiteInvalidations) ||
		!runtimeEmptyOrRawEqualSlice(left.GovernanceRequests, right.GovernanceRequests) ||
		!runtimecontract.RuntimeRawEqual(left.GovernanceResolutions, right.GovernanceResolutions) {
		return false
	}
	if !runtimeNormalizedFieldEqual(left.Decision.RequiredOutput, right.Decision.RequiredOutput, func(v runtimecontract.RequiredOutputContract) runtimecontract.RequiredOutputContract {
		return v.Normalized()
	}) ||
		!runtimeNormalizedFieldEqual(left.CoverageProofLineage, right.CoverageProofLineage, runtimecontract.NormalizeRuntimeCoverageProofLineage) ||
		!runtimeNormalizedFieldEqual(left.Facts.Focus, right.Facts.Focus, func(v runtimecontract.CoverageFocus) runtimecontract.CoverageFocus { return v.Normalized() }) {
		return false
	}
	if !runtimeNormalizedFactsEqual(left.Facts, right.Facts) {
		return false
	}
	return true
}

func runtimeEmptyOrRawEqualSlice[T any](left, right []T) bool {
	if len(left) == 0 && len(right) == 0 {
		return true
	}
	return runtimecontract.RuntimeRawEqual(left, right)
}

func runtimeNormalizedFieldEqual[T any](left, right T, norm func(T) T) bool {
	if runtimecontract.RuntimeRawEqual(left, right) {
		return true
	}
	return runtimecontract.RuntimeRawEqual(norm(left), norm(right))
}

func runtimeConvergenceDispatchEqual(left, right runtimecontract.RuntimeDispatchRecord) bool {
	return left.CandidateHash == right.CandidateHash &&
		left.State == right.State &&
		runtimeEmptyOrRawEqualSlice(left.CompletedOperationIDs, right.CompletedOperationIDs)
}

func runtimeConvergenceDispatchesEqual(leftActive, rightActive *runtimecontract.RuntimeDispatchRecord, leftCompleted, rightCompleted []runtimecontract.RuntimeDispatchRecord) bool {
	if leftActive != nil && !runtimeConvergenceDispatchEqual(*leftActive, *rightActive) {
		return false
	}
	for i := range leftCompleted {
		if !runtimeConvergenceDispatchEqual(leftCompleted[i], rightCompleted[i]) {
			return false
		}
	}
	return true
}

func runtimeConvergenceCandidateAdmissionsEqual(left, right []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	li, ri := 0, 0
	for {
		for li < len(left) && left[li].Proposal == nil && len(left[li].Feasibilities) == 0 {
			li++
		}
		for ri < len(right) && right[ri].Proposal == nil && len(right[ri].Feasibilities) == 0 {
			ri++
		}
		if li == len(left) || ri == len(right) {
			return li == len(left) && ri == len(right)
		}
		la, ra := &left[li], &right[ri]
		if la.AuthorityContractID != ra.AuthorityContractID ||
			!runtimecontract.RuntimeRawEqual(la.Transaction, ra.Transaction) ||
			(la.Proposal == nil) != (ra.Proposal == nil) ||
			!runtimeEmptyOrRawEqualSlice(la.Feasibilities, ra.Feasibilities) {
			return false
		}
		if la.Proposal != nil && !runtimecontract.RuntimeRawEqual(la.Proposal, ra.Proposal) {
			if runtimecontract.StableRuntimeCandidateHash(*la.Proposal) != runtimecontract.StableRuntimeCandidateHash(*ra.Proposal) {
				return false
			}
		}
		li++
		ri++
	}
}

func runtimeNormalizedFactsEqual(left, right runtimecontract.RuntimeFacts) bool {
	if runtimecontract.RuntimeRawEqual(left.RemainingFocuses, right.RemainingFocuses) &&
		runtimecontract.RuntimeRawEqual(left.CompletedFocuses, right.CompletedFocuses) &&
		runtimecontract.RuntimeRawEqual(left.Failure, right.Failure) &&
		runtimecontract.RuntimeRawEqual(left.Prerequisites, right.Prerequisites) &&
		runtimecontract.RuntimeRawEqual(left.Repair, right.Repair) &&
		runtimecontract.RuntimeRawEqual(left.RepairDirective, right.RepairDirective) &&
		runtimecontract.RuntimeRawEqual(left.Resources.PrerequisiteObservations, right.Resources.PrerequisiteObservations) &&
		runtimecontract.RuntimeRawEqual(left.Coverage, right.Coverage) &&
		runtimecontract.RuntimeRawEqual(left.Feasibility, right.Feasibility) {
		return true
	}
	ln := (runtimecontract.RuntimeFacts{
		Focus: left.Focus, RemainingFocuses: left.RemainingFocuses, CompletedFocuses: left.CompletedFocuses,
		EvidenceRevision: left.EvidenceRevision, Failure: left.Failure, Prerequisites: left.Prerequisites,
		Repair: left.Repair, RepairDirective: left.RepairDirective,
		Resources: runtimecontract.RuntimeResourceFacts{PrerequisiteObservations: left.Resources.PrerequisiteObservations},
		Coverage:  left.Coverage, Feasibility: left.Feasibility,
	}).Normalized()
	rn := (runtimecontract.RuntimeFacts{
		Focus: right.Focus, RemainingFocuses: right.RemainingFocuses, CompletedFocuses: right.CompletedFocuses,
		EvidenceRevision: right.EvidenceRevision, Failure: right.Failure, Prerequisites: right.Prerequisites,
		Repair: right.Repair, RepairDirective: right.RepairDirective,
		Resources: runtimecontract.RuntimeResourceFacts{PrerequisiteObservations: right.Resources.PrerequisiteObservations},
		Coverage:  right.Coverage, Feasibility: right.Feasibility,
	}).Normalized()
	return runtimecontract.RuntimeRawEqual(ln, rn)
}

type runtimeConvergenceDispatchProgress struct {
	CandidateHash         string
	State                 runtimecontract.RuntimeDispatchState
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

func runtimeConvergenceDispatchProgressFrom(dispatch runtimecontract.RuntimeDispatchRecord) runtimeConvergenceDispatchProgress {
	return runtimeConvergenceDispatchProgress{
		CandidateHash: dispatch.CandidateHash, State: dispatch.State,
		CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
	}
}
