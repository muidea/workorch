package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeOwnerFeasibilityDisposition is the complete, factual outcome of a
// side-effect-free capability preflight. It is deliberately distinct from a
// RuntimeCandidateFailure: capability owners report facts, while the
// Orchestrator alone chooses the next authority or terminal state.
type RuntimeOwnerFeasibilityDisposition string

const (
	RuntimeOwnerFeasibilityFeasible             RuntimeOwnerFeasibilityDisposition = "feasible"
	RuntimeOwnerFeasibilityStale                RuntimeOwnerFeasibilityDisposition = "stale"
	RuntimeOwnerFeasibilityPrerequisiteRequired RuntimeOwnerFeasibilityDisposition = "prerequisite_required"
	RuntimeOwnerFeasibilityInfeasible           RuntimeOwnerFeasibilityDisposition = "infeasible"
	// RuntimeOwnerFeasibilityContractDefect is a typed owner-boundary fact:
	// the capability emitted a failure code that is absent from its own total
	// registry. It is not an ordinary infeasibility and must never enter
	// candidate correction or prerequisite recovery.
	RuntimeOwnerFeasibilityContractDefect RuntimeOwnerFeasibilityDisposition = "owner_contract_defect"
)

func ValidRuntimeOwnerFeasibilityCategory(category RuntimeOwnerFeasibilityCategory) bool {
	switch RuntimeOwnerFeasibilityCategory(strings.TrimSpace(string(category))) {
	case RuntimeOwnerFeasibilityCategoryResourceAbsent,
		RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable,
		RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
		RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
		RuntimeOwnerFeasibilityCategoryTargetNotRegular,
		RuntimeOwnerFeasibilityCategoryObservationUnknown,
		RuntimeOwnerFeasibilityCategoryOwnerContractDefect:
		return true
	default:
		return false
	}
}

// RuntimeOwnerFeasibilityCategory is an owner-neutral taxonomy. Owner codes
// retain diagnostic precision but never enter the candidate correction matrix.
type RuntimeOwnerFeasibilityCategory string

const (
	RuntimeOwnerFeasibilityCategoryResourceAbsent              RuntimeOwnerFeasibilityCategory = "resource_absent"
	RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable RuntimeOwnerFeasibilityCategory = "validation_recipe_unavailable"
	RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable      RuntimeOwnerFeasibilityCategory = "environment_unavailable"
	RuntimeOwnerFeasibilityCategoryPolicyDisallowed            RuntimeOwnerFeasibilityCategory = "policy_disallowed"
	RuntimeOwnerFeasibilityCategoryTargetNotRegular            RuntimeOwnerFeasibilityCategory = "target_not_regular"
	RuntimeOwnerFeasibilityCategoryObservationUnknown          RuntimeOwnerFeasibilityCategory = "observation_unknown"
	// RuntimeOwnerFeasibilityCategoryOwnerContractDefect is reserved for an
	// owner whose private emitted-code registry is incomplete. The original
	// owner code remains solely in RuntimeFeedback.OriginalDiagnostic.
	RuntimeOwnerFeasibilityCategoryOwnerContractDefect RuntimeOwnerFeasibilityCategory = "owner_contract_defect"
)

// RuntimeOwnerFeasibilityObservation is a pure persisted value. It carries no
// Runtime action, retry, terminal, governance or aggregate mutation fields.
type RuntimeOwnerFeasibilityObservation struct {
	Disposition         RuntimeOwnerFeasibilityDisposition
	Category            RuntimeOwnerFeasibilityCategory
	Code                string
	Signature           string
	Reason              string
	TargetRefs          []string
	Feedback            *RuntimeFeedback
	Stale               *RuntimePrerequisiteStale
	PrerequisiteRequest *RuntimeOperationPrerequisiteRequest
}

func (o RuntimeOwnerFeasibilityObservation) Normalized() RuntimeOwnerFeasibilityObservation {
	o.Disposition = RuntimeOwnerFeasibilityDisposition(strings.TrimSpace(string(o.Disposition)))
	o.Category = RuntimeOwnerFeasibilityCategory(strings.TrimSpace(string(o.Category)))
	o.Code = strings.TrimSpace(o.Code)
	o.Signature = strings.TrimSpace(o.Signature)
	o.Reason = strings.TrimSpace(o.Reason)
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	if o.Feedback != nil {
		feedback := o.Feedback.CloneOriginal()
		o.Feedback = &feedback
	}
	if o.Stale != nil {
		copyValue := o.Stale.Normalized()
		o.Stale = &copyValue
	}
	if o.PrerequisiteRequest != nil {
		copyValue := o.PrerequisiteRequest.Normalized()
		o.PrerequisiteRequest = &copyValue
	}
	return o
}

func (o RuntimeOwnerFeasibilityObservation) Validate() error {
	o = o.Normalized()
	switch o.Disposition {
	case RuntimeOwnerFeasibilityFeasible:
		if o.Category != "" || o.Code != "" || o.Signature != "" || o.Reason != "" || len(o.TargetRefs) != 0 || o.Feedback != nil || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("feasible owner feasibility observation cannot carry failure detail")
		}
	case RuntimeOwnerFeasibilityStale:
		if o.Stale == nil || o.Category != "" || o.Code != "" || o.Signature != "" || o.Feedback != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("stale owner feasibility observation requires only stale detail")
		}
		if err := o.Stale.Validate(); err != nil {
			return err
		}
	case RuntimeOwnerFeasibilityPrerequisiteRequired:
		if o.PrerequisiteRequest == nil || o.Code == "" || o.Signature == "" || o.Reason == "" || o.Category == "" || o.Feedback == nil || o.Stale != nil {
			return fmt.Errorf("prerequisite-required owner feasibility observation requires category, failure identity and prerequisite")
		}
		if err := o.PrerequisiteRequest.Validate(); err != nil {
			return err
		}
	case RuntimeOwnerFeasibilityInfeasible:
		if o.Category == "" || o.Code == "" || o.Signature == "" || o.Reason == "" || o.Feedback == nil || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("infeasible owner feasibility observation requires category and failure identity")
		}
	case RuntimeOwnerFeasibilityContractDefect:
		if o.Category != RuntimeOwnerFeasibilityCategoryOwnerContractDefect || o.Code == "" || o.Signature == "" || o.Reason == "" || o.Feedback == nil || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("owner-contract-defect feasibility observation requires original failure identity and feedback")
		}
	default:
		return fmt.Errorf("unknown owner feasibility disposition %q", o.Disposition)
	}
	if o.Feedback != nil {
		if err := o.Feedback.Validate(); err != nil {
			return err
		}
		feedback := o.Feedback.Normalized()
		if feedback.Domain != RuntimeFeedbackDomainFeasibility || feedback.StableCategory != RuntimeFeedbackCategory(o.Category) || feedback.OriginalDiagnostic == nil ||
			feedback.OriginalDiagnostic.OriginalCode != o.Code || feedback.OriginalDiagnostic.Signature != o.Signature || feedback.OriginalDiagnostic.OriginalMessage != o.Reason {
			return fmt.Errorf("owner feasibility feedback does not match persisted observation")
		}
	}
	return nil
}

func (o RuntimeOwnerFeasibilityObservation) Clone() RuntimeOwnerFeasibilityObservation {
	return o.Normalized()
}

// RuntimeOwnerFeasibilityObservationFromFailure is the only conversion from
// an owner FailureFact into the neutral observation contract. It is pure and
// intentionally does not derive a candidate correction directive.
func RuntimeOwnerFeasibilityObservationFromFailure(category RuntimeOwnerFeasibilityCategory, feedback *RuntimeFeedback, failure *FailureFact, stale *RuntimePrerequisiteStale, prerequisiteRequest *RuntimeOperationPrerequisiteRequest) (RuntimeOwnerFeasibilityObservation, error) {
	if stale != nil {
		observation := RuntimeOwnerFeasibilityObservation{Disposition: RuntimeOwnerFeasibilityStale, Stale: stale}
		return observation, observation.Validate()
	}
	if failure == nil {
		return RuntimeOwnerFeasibilityObservation{}, fmt.Errorf("owner feasibility requires failure, stale, or feasible disposition")
	}
	category = RuntimeOwnerFeasibilityCategory(strings.TrimSpace(string(category)))
	if !ValidRuntimeOwnerFeasibilityCategory(category) {
		return RuntimeOwnerFeasibilityObservation{}, fmt.Errorf("owner feasibility requires a declared factual category")
	}
	disposition := RuntimeOwnerFeasibilityInfeasible
	if prerequisiteRequest != nil {
		disposition = RuntimeOwnerFeasibilityPrerequisiteRequired
	} else if category == RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
		disposition = RuntimeOwnerFeasibilityContractDefect
	}
	observation := RuntimeOwnerFeasibilityObservation{
		Disposition: disposition, Category: category, Code: failure.Code, Signature: failure.Signature,
		Reason: failure.Message, TargetRefs: append([]string(nil), failure.TargetRefs...), Feedback: feedback, PrerequisiteRequest: prerequisiteRequest,
	}
	return observation, observation.Validate()
}
