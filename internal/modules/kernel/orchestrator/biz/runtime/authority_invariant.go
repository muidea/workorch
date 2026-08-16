package runtime

import (
	"sort"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeAuthorityInvariantCode identifies a deterministic SM-2/SM-3
// authority violation. It is intentionally separate from provider and owner
// failures: the report is read-only and never chooses a recovery action.
type RuntimeAuthorityInvariantCode string

const (
	RuntimeAuthorityInvariantActiveFocusScope     RuntimeAuthorityInvariantCode = "INV-FR-01"
	RuntimeAuthorityInvariantActiveRetiredOverlap RuntimeAuthorityInvariantCode = "INV-FR-02"
	RuntimeAuthorityInvariantObservationLifecycle RuntimeAuthorityInvariantCode = "INV-FR-03"
	RuntimeAuthorityInvariantSelectionBinding     RuntimeAuthorityInvariantCode = "INV-FR-04"
	RuntimeAuthorityInvariantDecisionFocus        RuntimeAuthorityInvariantCode = "INV-CA-01"
	// RuntimeAuthorityInvariantProviderCeiling binds the candidate action to
	// the activation-sealed provider prerequisite. It is an SM-2 fact defect,
	// never a candidate/provider failure: reconciliation must return to owner
	// prerequisite acquisition before an SM-3 attempt is created.
	RuntimeAuthorityInvariantProviderCeiling RuntimeAuthorityInvariantCode = "INV-CA-02"
)

// RuntimeAuthorityInvariantViolation is audit metadata for one persisted
// aggregate revision. It does not expose owner payloads or mutate runtime
// facts.
type RuntimeAuthorityInvariantViolation struct {
	Code           RuntimeAuthorityInvariantCode
	StateRevision  uint64
	FocusID        string
	PrerequisiteID string
	ObservationID  string
	AuthorityID    string
	Message        string
}

// RuntimeAuthorityInvariantReport is a bounded, read-only SM-2/SM-3 snapshot
// intended for deterministic fixtures and audit projection adapters. It must
// not be used by the reducer as a second authority.
type RuntimeAuthorityInvariantReport struct {
	StateRevision                    uint64
	FocusID                          string
	TargetSelectionPrerequisiteID    string
	ActivePrerequisiteIDs            []string
	RetiredPrerequisiteIDs           []string
	ActivePrerequisiteObservationIDs []string
	RetiredObservationIDs            []string
	CandidateAttemptCount            int
	ActiveDispatchID                 string
	TerminalCode                     string
	RepairAttempts                   int
	PrerequisiteRefreshes            int
	Violations                       []RuntimeAuthorityInvariantViolation
}

// InspectRuntimeAuthorityInvariants reports inconsistencies in an already
// persisted aggregate. It is safe to call during failure analysis because it
// only reads the aggregate and does not invoke owners, the provider, or the
// reducer.
func InspectRuntimeAuthorityInvariants(aggregate RuntimeAggregate) RuntimeAuthorityInvariantReport {
	facts := aggregate.Facts.Normalized()
	report := newRuntimeAuthorityInvariantReport(aggregate, facts)
	active, satisfied := report.inspectActivePrerequisites(facts)
	retired := report.inspectRetiredPrerequisites(facts, active)
	report.inspectPrerequisiteObservations(facts, satisfied, retired)
	report.inspectTargetSelection(facts, satisfied)
	report.inspectDecisionAndDispatch(aggregate, facts)
	report.inspectProviderCeiling(aggregate, facts)
	report.sort()
	return report
}

func newRuntimeAuthorityInvariantReport(aggregate RuntimeAggregate, facts runtimecontract.RuntimeFacts) RuntimeAuthorityInvariantReport {
	report := RuntimeAuthorityInvariantReport{
		StateRevision: aggregate.StateRevision, FocusID: facts.Focus.ID,
		CandidateAttemptCount: len(aggregate.CandidateAttempts), RepairAttempts: facts.Repair.RepairAttempts,
		PrerequisiteRefreshes: facts.Recovery.PrerequisiteRefreshes,
	}
	if facts.Resources.TargetSelection != nil {
		report.TargetSelectionPrerequisiteID = facts.Resources.TargetSelection.PrerequisiteID
	}
	if aggregate.ActiveDispatch != nil {
		report.ActiveDispatchID = aggregate.ActiveDispatch.ID
	}
	if aggregate.Decision.Terminal != nil {
		report.TerminalCode = aggregate.Decision.Terminal.Code
	}
	return report
}

func (r *RuntimeAuthorityInvariantReport) inspectActivePrerequisites(facts runtimecontract.RuntimeFacts) (map[string]runtimecontract.RuntimePrerequisite, map[string]runtimecontract.RuntimePrerequisite) {
	active := make(map[string]runtimecontract.RuntimePrerequisite, len(facts.Prerequisites.Pending)+len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...), facts.Prerequisites.Satisfied...) {
		active[prerequisite.ID] = prerequisite
		r.ActivePrerequisiteIDs = append(r.ActivePrerequisiteIDs, prerequisite.ID)
		if prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID != facts.Focus.ID {
			r.addViolation(RuntimeAuthorityInvariantActiveFocusScope, prerequisite.ID, "", "", "active focus-scoped prerequisite is bound to a different focus")
		}
	}
	satisfied := make(map[string]runtimecontract.RuntimePrerequisite, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		satisfied[prerequisite.ID] = prerequisite
	}
	return active, satisfied
}

func (r *RuntimeAuthorityInvariantReport) inspectRetiredPrerequisites(facts runtimecontract.RuntimeFacts, active map[string]runtimecontract.RuntimePrerequisite) map[string]runtimecontract.RuntimePrerequisite {
	retired := make(map[string]runtimecontract.RuntimePrerequisite, len(facts.Prerequisites.Retired))
	for _, prerequisite := range facts.Prerequisites.Retired {
		retired[prerequisite.ID] = prerequisite
		r.RetiredPrerequisiteIDs = append(r.RetiredPrerequisiteIDs, prerequisite.ID)
		if _, exists := active[prerequisite.ID]; exists {
			r.addViolation(RuntimeAuthorityInvariantActiveRetiredOverlap, prerequisite.ID, "", "", "prerequisite is both active and retired")
		}
		if prerequisite.Scope != runtimecontract.RuntimePrerequisiteScopeFocus || prerequisite.FocusID == "" {
			r.addViolation(RuntimeAuthorityInvariantActiveRetiredOverlap, prerequisite.ID, "", "", "retired prerequisite is not focus-scoped")
		}
	}
	return retired
}

func (r *RuntimeAuthorityInvariantReport) inspectPrerequisiteObservations(facts runtimecontract.RuntimeFacts, satisfied, retired map[string]runtimecontract.RuntimePrerequisite) {
	for _, observation := range facts.Resources.PrerequisiteObservations {
		r.ActivePrerequisiteObservationIDs = append(r.ActivePrerequisiteObservationIDs, observation.ObservationID)
		prerequisite, exists := satisfied[observation.PrerequisiteID]
		if !exists || prerequisite.Kind != observation.Kind || prerequisite.OwnerModuleID != observation.OwnerModuleID {
			r.addViolation(RuntimeAuthorityInvariantObservationLifecycle, observation.PrerequisiteID, observation.ObservationID, "", "active observation has no matching satisfied prerequisite")
		}
	}
	for _, observation := range facts.Resources.RetiredPrerequisiteObservations {
		r.RetiredObservationIDs = append(r.RetiredObservationIDs, observation.ObservationID)
		prerequisite, exists := retired[observation.PrerequisiteID]
		if !exists || prerequisite.Kind != observation.Kind || prerequisite.OwnerModuleID != observation.OwnerModuleID {
			r.addViolation(RuntimeAuthorityInvariantObservationLifecycle, observation.PrerequisiteID, observation.ObservationID, "", "retired observation has no matching retired prerequisite")
		}
	}
}

func (r *RuntimeAuthorityInvariantReport) inspectTargetSelection(facts runtimecontract.RuntimeFacts, satisfied map[string]runtimecontract.RuntimePrerequisite) {
	if facts.Resources.TargetSelection == nil {
		return
	}
	selection := facts.Resources.TargetSelection.Normalized()
	prerequisite, exists := satisfied[selection.PrerequisiteID]
	if !exists || prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSelection || prerequisite.Scope != runtimecontract.RuntimePrerequisiteScopeFocus || prerequisite.FocusID != facts.Focus.ID || prerequisite.OwnerModuleID != selection.OwnerModuleID {
		r.addViolation(RuntimeAuthorityInvariantSelectionBinding, selection.PrerequisiteID, "", "", "target selection is not bound to the active current-focus prerequisite")
	}
}

func (r *RuntimeAuthorityInvariantReport) inspectDecisionAndDispatch(aggregate RuntimeAggregate, facts runtimecontract.RuntimeFacts) {
	if aggregate.Decision.Focus.ID != "" && aggregate.Decision.Focus.ID != facts.Focus.ID {
		r.addViolation(RuntimeAuthorityInvariantDecisionFocus, "", "", aggregate.Decision.AuthoritySnapshot.ID, "sealed decision focus does not match persisted facts focus")
	}
	if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Transaction.FocusID != facts.Focus.ID {
		r.addViolation(RuntimeAuthorityInvariantDecisionFocus, "", "", aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID, "active dispatch focus does not match persisted facts focus")
	}
}

func (r *RuntimeAuthorityInvariantReport) inspectProviderCeiling(aggregate RuntimeAggregate, facts runtimecontract.RuntimeFacts) {
	if runtimeDecisionRequestsCandidate(aggregate.Decision.State) && activeProviderCeilingRequirementUnsatisfied(facts) {
		r.addViolation(RuntimeAuthorityInvariantProviderCeiling, providerCeilingPrerequisiteID(facts), "", aggregate.Decision.AuthoritySnapshot.ID, "candidate state has an unsatisfied activation-sealed provider ceiling prerequisite")
	}
}

func (r *RuntimeAuthorityInvariantReport) sort() {
	sort.Strings(r.ActivePrerequisiteIDs)
	sort.Strings(r.RetiredPrerequisiteIDs)
	sort.Strings(r.ActivePrerequisiteObservationIDs)
	sort.Strings(r.RetiredObservationIDs)
	sort.Slice(r.Violations, func(left, right int) bool {
		if r.Violations[left].Code != r.Violations[right].Code {
			return r.Violations[left].Code < r.Violations[right].Code
		}
		if r.Violations[left].PrerequisiteID != r.Violations[right].PrerequisiteID {
			return r.Violations[left].PrerequisiteID < r.Violations[right].PrerequisiteID
		}
		return r.Violations[left].ObservationID < r.Violations[right].ObservationID
	})
}

func runtimeDecisionRequestsCandidate(state runtimecontract.ConvergenceState) bool {
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange,
		runtimecontract.ConvergenceStateNeedValidation,
		runtimecontract.ConvergenceStateNeedAssessment,
		runtimecontract.ConvergenceStateNeedGovernance:
		return true
	default:
		return false
	}
}

// activeProviderCeilingRequirements returns only prerequisites already sealed
// in the aggregate. It deliberately does not invent a fallback byte ceiling:
// bootstrap is the sole source that declares the provider requirement.
func activeProviderCeilingRequirements(facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeRequirement {
	facts = facts.Normalized()
	values := make([]runtimecontract.RuntimeRequirement, 0, 1)
	seen := make(map[string]struct{})
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...), facts.Prerequisites.Satisfied...) {
		if prerequisite.Kind != runtimecontract.PrerequisiteKindProviderCeiling {
			continue
		}
		requirement := runtimecontract.RuntimeRequirement{
			Kind: prerequisite.Kind, Scope: prerequisite.Scope, FocusID: prerequisite.FocusID,
			OwnerModuleID: prerequisite.OwnerModuleID, SubjectRef: prerequisite.SubjectRef,
			MinimumBytes: prerequisite.MinimumBytes, Reason: prerequisite.Reason,
		}
		key := runtimePrerequisiteID(requirement)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, requirement)
	}
	return normalizeRuntimeRequirements(values)
}

func activeProviderCeilingRequirementUnsatisfied(facts runtimecontract.RuntimeFacts) bool {
	for _, requirement := range activeProviderCeilingRequirements(facts) {
		if !runtimeRequirementSatisfied(facts, requirement) {
			return true
		}
	}
	return false
}

func providerCeilingPrerequisiteID(facts runtimecontract.RuntimeFacts) string {
	values := activeProviderCeilingRequirements(facts)
	if len(values) == 0 {
		return ""
	}
	return runtimePrerequisiteID(values[0])
}

func (r *RuntimeAuthorityInvariantReport) addViolation(code RuntimeAuthorityInvariantCode, prerequisiteID, observationID, authorityID, message string) {
	r.Violations = append(r.Violations, RuntimeAuthorityInvariantViolation{
		Code: code, StateRevision: r.StateRevision, FocusID: r.FocusID, PrerequisiteID: prerequisiteID,
		ObservationID: observationID, AuthorityID: authorityID, Message: message,
	})
}

// FirstRuntimeAuthorityInvariantViolation finds the earliest bad persisted
// revision from an audit snapshot sequence. The input is copied and sorted, so
// callers may retain repository order for their own audit output.
func FirstRuntimeAuthorityInvariantViolation(history []RuntimeAggregate) *RuntimeAuthorityInvariantViolation {
	ordered := append([]RuntimeAggregate(nil), history...)
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].StateRevision < ordered[right].StateRevision })
	for _, aggregate := range ordered {
		report := InspectRuntimeAuthorityInvariants(aggregate)
		if len(report.Violations) != 0 {
			violation := report.Violations[0]
			return &violation
		}
	}
	return nil
}
