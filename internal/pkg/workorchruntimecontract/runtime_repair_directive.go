package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeGovernanceKind unifies decision/approval/configuration/external dependency (RVC-R11 prep).
type RuntimeGovernanceKind string

const (
	GovernanceDecision      RuntimeGovernanceKind = "decision"
	GovernanceApproval      RuntimeGovernanceKind = "approval"
	GovernanceConfiguration RuntimeGovernanceKind = "configuration"
	GovernanceExternal      RuntimeGovernanceKind = "external_dependency"
)

// RuntimeGovernanceRequirement is returned by owners before side effects when
// approval/decision/config/external dependency is required (INV-06).
type RuntimeGovernanceRequirement struct {
	Kind            RuntimeGovernanceKind
	SubjectRef      string
	Reason          string
	RequestedAction string
	RiskLevel       string
}

func (k RuntimeGovernanceKind) Normalized() RuntimeGovernanceKind {
	return RuntimeGovernanceKind(strings.TrimSpace(string(k)))
}

func ValidRuntimeGovernanceKind(kind RuntimeGovernanceKind) bool {
	switch kind.Normalized() {
	case GovernanceDecision, GovernanceApproval, GovernanceConfiguration, GovernanceExternal:
		return true
	default:
		return false
	}
}

func (r RuntimeGovernanceRequirement) Normalized() RuntimeGovernanceRequirement {
	r.Kind = r.Kind.Normalized()
	r.SubjectRef = strings.TrimSpace(r.SubjectRef)
	r.Reason = strings.TrimSpace(r.Reason)
	r.RequestedAction = strings.TrimSpace(r.RequestedAction)
	r.RiskLevel = strings.TrimSpace(r.RiskLevel)
	return r
}

func (r RuntimeGovernanceRequirement) Validate() error {
	r = r.Normalized()
	if !ValidRuntimeGovernanceKind(r.Kind) {
		return fmt.Errorf("governance requirement has invalid kind %q", r.Kind)
	}
	if r.SubjectRef == "" || r.Reason == "" {
		return fmt.Errorf("governance requirement requires subject ref and reason")
	}
	return nil
}

func (r RuntimeGovernanceRequirement) Clone() RuntimeGovernanceRequirement {
	return r.Normalized()
}

// RuntimeTerminationKind is the administrative terminal kind for SM-1 cancel/supersede.
type RuntimeTerminationKind string

const (
	RuntimeTerminationCancelled  RuntimeTerminationKind = "cancelled"
	RuntimeTerminationSuperseded RuntimeTerminationKind = "superseded"
)

// RuntimeTerminationRecord is CAS-persisted before SM-1 projects cancelled/superseded (INV-08).
// completed/failed remain SM-2 sealed terminal only and do not use this record.
type RuntimeTerminationRecord struct {
	Kind        RuntimeTerminationKind
	RequestedBy string
	Reason      string
	Revision    uint64
	RequestedAt string
}

func (k RuntimeTerminationKind) Normalized() RuntimeTerminationKind {
	return RuntimeTerminationKind(strings.TrimSpace(string(k)))
}

func ValidRuntimeTerminationKind(kind RuntimeTerminationKind) bool {
	switch kind.Normalized() {
	case RuntimeTerminationCancelled, RuntimeTerminationSuperseded:
		return true
	default:
		return false
	}
}

func (r RuntimeTerminationRecord) Normalized() RuntimeTerminationRecord {
	r.Kind = r.Kind.Normalized()
	r.RequestedBy = strings.TrimSpace(r.RequestedBy)
	r.Reason = strings.TrimSpace(r.Reason)
	r.RequestedAt = strings.TrimSpace(r.RequestedAt)
	return r
}

func (r RuntimeTerminationRecord) Validate() error {
	r = r.Normalized()
	if !ValidRuntimeTerminationKind(r.Kind) {
		return fmt.Errorf("termination record has invalid kind %q", r.Kind)
	}
	if r.Reason == "" {
		return fmt.Errorf("termination record requires reason")
	}
	if r.Revision == 0 {
		return fmt.Errorf("termination record requires revision")
	}
	return nil
}

func (r RuntimeTerminationRecord) Clone() RuntimeTerminationRecord {
	return r.Normalized()
}

// RuntimeRepairTrigger classifies why a repair directive was created.
type RuntimeRepairTrigger string

const (
	RuntimeRepairTriggerRejectedValidation  RuntimeRepairTrigger = "rejected_validation"
	RuntimeRepairTriggerAssessmentGap       RuntimeRepairTrigger = "assessment_gap"
	RuntimeRepairTriggerExecutionRejected   RuntimeRepairTrigger = "execution_rejected"
	RuntimeRepairTriggerCandidateInfeasible RuntimeRepairTrigger = "candidate_infeasible"
	RuntimeRepairTriggerEffectMismatch      RuntimeRepairTrigger = "effect_mismatch"
	RuntimeRepairTriggerMissingEvidence     RuntimeRepairTrigger = "missing_evidence"
)

// RuntimeRepairDirective is the single persisted next-action authority for a focus (INV-05).
// Candidate renderer, anti-replay, and owners must obey RequiredAction; they cannot switch it.
type RuntimeRepairDirective struct {
	ID                         string
	FocusID                    string
	Trigger                    RuntimeRepairTrigger
	CurrentFailure             *FailureFact
	RequiredAction             RuntimeAction
	RequiredEffectScopes       []RuntimeEffectScope
	RequiredEvidenceKinds      []string
	AllowedOperationKinds      []string
	RejectedOperationShapes    []RuntimeRejectedOperationShape
	CoverageWorkItemIDs        []common.CoverageWorkItemID
	TargetRefs                 []string
	SupersedesEvidenceRevision string
	RepairAttempt              int
}

func (t RuntimeRepairTrigger) Normalized() RuntimeRepairTrigger {
	return RuntimeRepairTrigger(strings.TrimSpace(string(t)))
}

func ValidRuntimeRepairTrigger(trigger RuntimeRepairTrigger) bool {
	switch trigger.Normalized() {
	case RuntimeRepairTriggerRejectedValidation, RuntimeRepairTriggerAssessmentGap,
		RuntimeRepairTriggerExecutionRejected, RuntimeRepairTriggerCandidateInfeasible, RuntimeRepairTriggerEffectMismatch,
		RuntimeRepairTriggerMissingEvidence:
		return true
	default:
		return false
	}
}

func (d RuntimeRepairDirective) Normalized() RuntimeRepairDirective {
	d.ID = strings.TrimSpace(d.ID)
	d.FocusID = strings.TrimSpace(d.FocusID)
	d.Trigger = d.Trigger.Normalized()
	if d.CurrentFailure != nil {
		failure := *d.CurrentFailure
		failure.Class = FailureClass(strings.TrimSpace(string(failure.Class)))
		failure.Code = strings.TrimSpace(failure.Code)
		failure.Signature = strings.TrimSpace(failure.Signature)
		failure.Message = strings.TrimSpace(failure.Message)
		failure.TargetRefs = normalizeStrings(failure.TargetRefs)
		d.CurrentFailure = &failure
	}
	d.RequiredAction = RuntimeAction(strings.TrimSpace(string(d.RequiredAction)))
	d.RequiredEffectScopes = NormalizeRuntimeEffectScopes(d.RequiredEffectScopes)
	d.RequiredEvidenceKinds = normalizeStrings(d.RequiredEvidenceKinds)
	d.AllowedOperationKinds = normalizeStrings(d.AllowedOperationKinds)
	d.RejectedOperationShapes = NormalizeRuntimeRejectedOperationShapes(d.RejectedOperationShapes)
	d.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(d.CoverageWorkItemIDs)
	d.TargetRefs = normalizeStrings(d.TargetRefs)
	d.SupersedesEvidenceRevision = strings.TrimSpace(d.SupersedesEvidenceRevision)
	return d
}

func (d RuntimeRepairDirective) Validate() error {
	d = d.Normalized()
	if d.ID == "" || d.FocusID == "" {
		return fmt.Errorf("repair directive requires id and focus id")
	}
	if !ValidRuntimeRepairTrigger(d.Trigger) {
		return fmt.Errorf("repair directive has invalid trigger %q", d.Trigger)
	}
	switch d.RequiredAction {
	case RuntimeActionProduceStateChange, RuntimeActionProduceValidation, RuntimeActionRequestGovernance:
		// allowed
	default:
		return fmt.Errorf("repair directive required action %q is not a repairable action", d.RequiredAction)
	}
	if err := ValidateEffectScopesCompatibleWithAction(d.RequiredAction, d.RequiredEffectScopes); err != nil {
		return fmt.Errorf("repair directive effect scopes: %w", err)
	}
	if d.RepairAttempt < 0 {
		return fmt.Errorf("repair directive attempt cannot be negative")
	}
	if d.CurrentFailure != nil {
		if d.CurrentFailure.Class == "" || d.CurrentFailure.Message == "" {
			return fmt.Errorf("repair directive current failure requires class and message")
		}
	}
	return nil
}

func (d RuntimeRepairDirective) Clone() RuntimeRepairDirective {
	ret := d.Normalized()
	if d.CurrentFailure != nil {
		failure := *ret.CurrentFailure
		ret.CurrentFailure = &failure
	}
	if len(ret.RejectedOperationShapes) > 0 {
		shapes := make([]RuntimeRejectedOperationShape, len(ret.RejectedOperationShapes))
		copy(shapes, ret.RejectedOperationShapes)
		ret.RejectedOperationShapes = shapes
	}
	return ret
}

func SameRuntimeRepairDirective(left *RuntimeRepairDirective, right *RuntimeRepairDirective) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	l := left.Normalized()
	r := right.Normalized()
	if l.ID != r.ID || l.FocusID != r.FocusID || l.Trigger != r.Trigger || l.RequiredAction != r.RequiredAction ||
		l.SupersedesEvidenceRevision != r.SupersedesEvidenceRevision || l.RepairAttempt != r.RepairAttempt {
		return false
	}
	if !SameRuntimeEffectScopes(l.RequiredEffectScopes, r.RequiredEffectScopes) ||
		!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) ||
		!sameStrings(l.AllowedOperationKinds, r.AllowedOperationKinds) ||
		!sameCoverageWorkItemIDs(l.CoverageWorkItemIDs, r.CoverageWorkItemIDs) ||
		!sameStrings(l.TargetRefs, r.TargetRefs) {
		return false
	}
	if (l.CurrentFailure == nil) != (r.CurrentFailure == nil) {
		return false
	}
	if l.CurrentFailure != nil {
		if l.CurrentFailure.Class != r.CurrentFailure.Class || l.CurrentFailure.Code != r.CurrentFailure.Code ||
			l.CurrentFailure.Signature != r.CurrentFailure.Signature || l.CurrentFailure.Message != r.CurrentFailure.Message ||
			!sameStrings(l.CurrentFailure.TargetRefs, r.CurrentFailure.TargetRefs) {
			return false
		}
	}
	if len(l.RejectedOperationShapes) != len(r.RejectedOperationShapes) {
		return false
	}
	for idx := range l.RejectedOperationShapes {
		if l.RejectedOperationShapes[idx].Hash != r.RejectedOperationShapes[idx].Hash {
			return false
		}
	}
	return true
}
