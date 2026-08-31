package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeFailureAuthorityPolicy makes replacement of the mutable SM-2 failure
// explicit about the repair authority that was derived from its predecessor.
// A failure is not an isolated field: FailureTarget and RepairDirective can
// authorize the next SM-3 transaction, so a replacement must either preserve
// that authority for a bounded interlude, retire it, or replace it with the
// exact owner fact admitted by the current receipt/confirmation transaction.
type runtimeFailureAuthorityPolicy uint8

const (
	runtimeFailureAuthorityRetainInterlude runtimeFailureAuthorityPolicy = iota + 1
	runtimeFailureAuthorityRetire
	runtimeFailureAuthorityReplaceFromExactOwnerFact
)

func replaceRuntimeFailureAuthority(
	facts *runtimecontract.RuntimeFacts,
	failure runtimecontract.FailureFact,
	policy runtimeFailureAuthorityPolicy,
	ownerTarget *runtimecontract.FailureTarget,
) error {
	if facts == nil {
		return common.ErrInvalidArgument
	}
	failure = failure.Clone()
	if failure.Class == "" || strings.TrimSpace(failure.Message) == "" {
		return fmt.Errorf("%w: failure authority requires class and message", common.ErrInvalidArgument)
	}

	switch policy {
	case runtimeFailureAuthorityRetainInterlude:
		if ownerTarget != nil {
			return fmt.Errorf("%w: retained failure interlude cannot replace owner target authority", common.ErrInvalidArgument)
		}
	case runtimeFailureAuthorityRetire:
		if ownerTarget != nil {
			return fmt.Errorf("%w: retiring failure authority cannot carry an owner target", common.ErrInvalidArgument)
		}
		facts.Resources.FailureTarget = nil
		facts.RepairDirective = nil
	case runtimeFailureAuthorityReplaceFromExactOwnerFact:
		target, err := exactOwnerFailureTargetForReplacement(failure, ownerTarget)
		if err != nil {
			return err
		}
		facts.Resources.FailureTarget = target
		// A target replacement is a new owner-fact generation. The reducer may
		// build its successor directive, but an older directive must not bridge
		// the two failure lineages.
		facts.RepairDirective = nil
	default:
		return fmt.Errorf("%w: unknown failure authority replacement policy", common.ErrInvalidArgument)
	}

	facts.Failure = &failure
	return nil
}

func exactOwnerFailureTargetForReplacement(
	failure runtimecontract.FailureFact,
	raw *runtimecontract.FailureTarget,
) (*runtimecontract.FailureTarget, error) {
	if raw == nil {
		return nil, fmt.Errorf("%w: exact owner failure replacement requires a target", common.ErrInvalidArgument)
	}
	target := &runtimecontract.FailureTarget{
		Signature:  strings.TrimSpace(raw.Signature),
		TargetRefs: normalizeRuntimeTargetRefs(raw.TargetRefs),
	}
	if raw.TargetObservation != nil {
		observation := raw.TargetObservation.Clone()
		target.TargetObservation = &observation
	}
	if target.Signature == "" || len(target.TargetRefs) == 0 ||
		target.Signature != failure.Signature || !sameRuntimeTargetRefs(target.TargetRefs, failure.TargetRefs) {
		return nil, fmt.Errorf("%w: exact owner failure target does not match replacement failure", common.ErrConflict)
	}
	if (target.TargetObservation == nil) != (failure.TargetObservation == nil) ||
		(target.TargetObservation != nil && !sameFailureTargetObservationBinding(target.TargetObservation, failure.TargetObservation)) {
		return nil, fmt.Errorf("%w: exact owner failure target observation does not match replacement failure", common.ErrConflict)
	}
	return target, nil
}
