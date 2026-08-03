package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeOwnerFeasibilityOutcome is the complete, factual outcome of a
// side-effect-free capability preflight. It is deliberately distinct from a
// RuntimeCandidateFailure: capability owners report facts, while the
// Orchestrator alone chooses the next authority or terminal state.
type RuntimeOwnerFeasibilityOutcome string

const (
	RuntimeOwnerFeasibilityFeasible             RuntimeOwnerFeasibilityOutcome = "feasible"
	RuntimeOwnerFeasibilityStale                RuntimeOwnerFeasibilityOutcome = "stale"
	RuntimeOwnerFeasibilityPrerequisiteRequired RuntimeOwnerFeasibilityOutcome = "prerequisite_required"
	RuntimeOwnerFeasibilityInfeasible           RuntimeOwnerFeasibilityOutcome = "infeasible"
	// RuntimeOwnerFeasibilityContractDefect is a typed owner-boundary fact:
	// the capability emitted a failure code that is absent from its own total
	// registry. It is not an ordinary infeasibility and must never enter
	// candidate correction or prerequisite recovery.
	RuntimeOwnerFeasibilityContractDefect RuntimeOwnerFeasibilityOutcome = "owner_contract_defect"
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
	Outcome             RuntimeOwnerFeasibilityOutcome
	Feedback            *RuntimeFeedback
	Stale               *RuntimePrerequisiteStale
	PrerequisiteRequest *RuntimeOperationPrerequisiteRequest
}

func (o RuntimeOwnerFeasibilityObservation) Normalized() RuntimeOwnerFeasibilityObservation {
	o.Outcome = RuntimeOwnerFeasibilityOutcome(strings.TrimSpace(string(o.Outcome)))
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
	switch o.Outcome {
	case RuntimeOwnerFeasibilityFeasible:
		if o.Feedback != nil || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("feasible owner feasibility observation cannot carry failure detail")
		}
	case RuntimeOwnerFeasibilityStale:
		if o.Stale == nil || o.Feedback != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("stale owner feasibility observation requires only stale detail")
		}
		if err := o.Stale.Validate(); err != nil {
			return err
		}
	case RuntimeOwnerFeasibilityPrerequisiteRequired:
		if o.PrerequisiteRequest == nil || o.Feedback == nil || o.Stale != nil {
			return fmt.Errorf("prerequisite-required owner feasibility observation requires category, failure identity and prerequisite")
		}
		if err := o.PrerequisiteRequest.Validate(); err != nil {
			return err
		}
	case RuntimeOwnerFeasibilityInfeasible:
		if o.Feedback == nil || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("infeasible owner feasibility observation requires category and failure identity")
		}
	case RuntimeOwnerFeasibilityContractDefect:
		if o.Feedback == nil || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("owner-contract-defect feasibility observation requires original failure identity and feedback")
		}
	default:
		return fmt.Errorf("unknown owner feasibility outcome %q", o.Outcome)
	}
	if o.Feedback != nil {
		if err := o.Feedback.Validate(); err != nil {
			return err
		}
		feedback := o.Feedback.Normalized()
		if feedback.Domain != RuntimeFeedbackDomainFeasibility || !ValidRuntimeOwnerFeasibilityCategory(RuntimeOwnerFeasibilityCategory(feedback.StableCategory)) || feedback.OriginalDiagnostic == nil {
			return fmt.Errorf("owner feasibility feedback does not match persisted observation")
		}
		if o.Outcome == RuntimeOwnerFeasibilityContractDefect && feedback.StableCategory != RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryOwnerContractDefect) {
			return fmt.Errorf("owner-contract-defect feasibility observation requires contract-defect feedback")
		}
		if o.Outcome != RuntimeOwnerFeasibilityContractDefect && feedback.StableCategory == RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryOwnerContractDefect) {
			return fmt.Errorf("owner contract-defect feedback requires contract-defect outcome")
		}
	}
	return nil
}

// FailureCategory returns the stable receiver-facing category from the one
// canonical feedback value. It never reconstructs information from text.
func (o RuntimeOwnerFeasibilityObservation) FailureCategory() RuntimeOwnerFeasibilityCategory {
	if o.Feedback == nil {
		return ""
	}
	return RuntimeOwnerFeasibilityCategory(o.Feedback.Normalized().StableCategory)
}

func (o RuntimeOwnerFeasibilityObservation) Clone() RuntimeOwnerFeasibilityObservation {
	return o.Normalized()
}

// RuntimeOwnerFeasibilityObservationFromOutcome is the only conversion from a
// cross-owner factual outcome into the persisted neutral observation contract.
// It is pure and intentionally does not derive a candidate correction
// directive, retry, or terminal disposition.
func RuntimeOwnerFeasibilityObservationFromOutcome(outcome RuntimeOwnerFeasibilityOutcome, feedback *RuntimeFeedback, stale *RuntimePrerequisiteStale, prerequisiteRequest *RuntimeOperationPrerequisiteRequest) (RuntimeOwnerFeasibilityObservation, error) {
	observation := RuntimeOwnerFeasibilityObservation{
		Outcome: outcome, Feedback: feedback, Stale: stale, PrerequisiteRequest: prerequisiteRequest,
	}
	return observation, observation.Validate()
}
