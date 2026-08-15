// Package candidateadmission contains stateless SM-3 candidate replay and
// fingerprint rules. It does not own an aggregate, attempt ledger, retry
// budget, transition or EventHub call.
package candidateadmission

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	CodeRejectedShape     = "candidate_replays_rejected_shape"
	CodePartialReplay     = "candidate_partially_replays_rejected_shape"
	CodeRejectedCandidate = "candidate_replays_rejected_candidate_shape"
	CodeNoProgressShape   = "candidate_replays_no_progress_shape"

	SignatureRejectedShape     = "candidate-replays-rejected-shape"
	SignaturePartialReplay     = "candidate-partially-replays-rejected-shape"
	SignatureRejectedCandidate = "candidate-replays-rejected-candidate-shape"
	SignatureNoProgressShape   = "candidate-replays-no-progress-shape"
)

// Finding is a factual first-failure result. The caller maps it to its
// owner-local CandidateAdmissionResult and decides whether the rejection is
// correctable or terminal.
type Finding struct {
	Code      string
	Signature string
	Reason    string
}

func (f Finding) Valid() bool { return strings.TrimSpace(f.Code) != "" }

// Operations returns the executable operation values from a candidate. It is
// deliberately limited to state-change and validation candidates; assessment
// and governance candidates have no operation replay surface.
func Operations(candidate runtimecontract.RuntimeCandidate) []runtimecontract.RuntimeOperation {
	switch {
	case candidate.StateChange != nil:
		ret := make([]runtimecontract.RuntimeOperation, 0, len(candidate.StateChange.Operations))
		for _, value := range candidate.StateChange.Operations {
			ret = append(ret, value.Operation)
		}
		return ret
	case candidate.Validation != nil:
		ret := make([]runtimecontract.RuntimeOperation, 0, len(candidate.Validation.Operations))
		for _, value := range candidate.Validation.Operations {
			ret = append(ret, value.Operation)
		}
		return ret
	default:
		return nil
	}
}

// RejectedShapeReplay compares the candidate's semantic operation shapes with
// the active rejected-shape facts. It preserves the original all/partial
// distinction without interpreting owner messages.
func RejectedShapeReplay(candidate runtimecontract.RuntimeCandidate, shapes []runtimecontract.RuntimeRejectedOperationShape) (Finding, bool) {
	shapes = runtimecontract.NormalizeRuntimeRejectedOperationShapes(shapes)
	if len(shapes) == 0 {
		return Finding{}, false
	}
	blocked := make(map[string]struct{}, len(shapes))
	for _, shape := range shapes {
		blocked[shape.Hash] = struct{}{}
	}
	operations := Operations(candidate)
	if len(operations) == 0 {
		return Finding{}, false
	}
	replayCount := 0
	for _, operation := range operations {
		if _, found := blocked[runtimecontract.StableRuntimeOperationShape(operation)]; found {
			replayCount++
		}
	}
	switch {
	case replayCount == 0:
		return Finding{}, false
	case replayCount == len(operations):
		return Finding{Code: CodeRejectedShape, Signature: SignatureRejectedShape, Reason: "runtime candidate replays active rejected operation shape(s); change kind, targets, or structural parameters before retry"}, true
	default:
		return Finding{Code: CodePartialReplay, Signature: SignaturePartialReplay, Reason: fmt.Sprintf("runtime candidate includes %d operation(s) that replay active rejected shape(s); remove or rewrite those operations", replayCount)}, true
	}
}

// NoProgressReplay compares against the persisted no-progress shape set. It
// is intentionally separate from ordinary rejected receipts: both facts are
// owner-neutral but have different audit lineage.
func NoProgressReplay(candidate runtimecontract.RuntimeCandidate, shapes []runtimecontract.RuntimeRejectedOperationShape) (Finding, bool) {
	shapes = runtimecontract.NormalizeRuntimeRejectedOperationShapes(shapes)
	if len(shapes) == 0 {
		return Finding{}, false
	}
	blocked := make(map[string]struct{}, len(shapes))
	for _, shape := range shapes {
		blocked[shape.Hash] = struct{}{}
	}
	for _, operation := range Operations(candidate) {
		if _, found := blocked[runtimecontract.StableRuntimeOperationShape(operation)]; found {
			return Finding{Code: CodeNoProgressShape, Signature: SignatureNoProgressShape, Reason: "runtime candidate replays an accepted operation shape that did not resolve the active gap; change kind, targets, or structural parameters before retry"}, true
		}
	}
	return Finding{}, false
}

// CandidateSemanticReplay compares one complete candidate semantic shape with
// prior failed proposal shapes from the same authority contract. The caller
// performs authority/attempt filtering because those are aggregate facts.
func CandidateSemanticReplay(candidate runtimecontract.RuntimeCandidate, failedProposalShapes []string) (Finding, bool) {
	wanted := runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	if wanted == "" {
		return Finding{}, false
	}
	for _, shape := range failedProposalShapes {
		if strings.TrimSpace(shape) == wanted {
			return Finding{Code: CodeRejectedCandidate, Signature: SignatureRejectedCandidate, Reason: "runtime candidate replays the prior rejected complete candidate shape; change kind, targets, coverage, dependencies, or structural parameters before retry"}, true
		}
	}
	return Finding{}, false
}
