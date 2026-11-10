package workorchruntimecontract

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeCWISuspensionV1 = "runtime/cwi-suspension/v1"

// RuntimeCWISuspension retains the exact bound successor, not a new binding or
// execution authorization. There is at most one unresolved pause. Original
// fields are immutable; the resume proof can be filled exactly once by SM-2.
type RuntimeCWISuspension struct {
	ProtocolRef               string
	SuspendedAtStateRevision  uint64
	SuspendedEvidenceRevision string
	FrozenFocus               CoverageFocus
	FrozenBinding             RuntimeCWIBindingRef
	EffectEvidenceID          common.EvidenceRecordID
	PredecessorID             common.CoverageWorkItemID
	ReopenSource              RuntimeCWIReopenLineage
	Repair                    RuntimeRepairLedger
	RepairDirective           *RuntimeRepairDirective          `json:",omitempty"`
	Failure                   *FailureFact                     `json:",omitempty"`
	FrozenPrerequisites       []RuntimePrerequisite            `json:",omitempty"`
	FrozenObservations        []RuntimePrerequisiteObservation `json:",omitempty"`
	ResumeBinding             *RuntimeCWIBindingRef            `json:",omitempty"`
	ResumeAssessmentID        string                           `json:",omitempty"`
	ResumeEvidenceRevision    string                           `json:",omitempty"`
	ResumeObservationCount    int                              `json:",omitempty"`
}

func (s RuntimeCWISuspension) CloneRaw() RuntimeCWISuspension {
	s.FrozenFocus = cloneCoverageFocusRaw(s.FrozenFocus)
	s.Repair = cloneRuntimeRepairLedgerRaw(s.Repair)
	s.RepairDirective = cloneRuntimeStateRawPointer(s.RepairDirective, cloneRuntimeRepairDirectiveRaw)
	s.Failure = cloneRuntimeStateRawPointer(s.Failure, cloneFailureFactRaw)
	s.FrozenPrerequisites = cloneRuntimeStateRawSlice(s.FrozenPrerequisites, cloneRuntimePrerequisiteRaw)
	s.FrozenObservations = cloneRuntimeStateRawSlice(s.FrozenObservations, cloneRuntimePrerequisiteObservationRaw)
	s.ResumeBinding = cloneRuntimeStateRawPointer(s.ResumeBinding, func(v RuntimeCWIBindingRef) RuntimeCWIBindingRef { return v })
	return s
}

func CloneRuntimeCWISuspensions(values []RuntimeCWISuspension) []RuntimeCWISuspension {
	return cloneRuntimeStateRawSlice(values, func(v RuntimeCWISuspension) RuntimeCWISuspension { return v.CloneRaw() })
}

// RuntimeCWISuspensionOrdinaryPredecessorSupported deliberately requires an
// existing validation-only Definition. A state-change obligation without a
// declared ordinary consumer cannot be turned into a current proof or replayed.
func RuntimeCWISuspensionOrdinaryPredecessorSupported(facts RuntimeFacts, id common.CoverageWorkItemID) bool {
	matches := 0
	for _, declaration := range facts.DefinitionFocuses {
		for _, coverageID := range declaration.CoverageWorkItemIDs {
			if coverageID != id {
				continue
			}
			matches++
			if !declaration.RequireAssessment || len(declaration.RequiredEvidenceKinds) != 1 || declaration.RequiredEvidenceKinds[0] != EvidenceKindValidation {
				return false
			}
		}
	}
	return matches == 1
}

// RuntimeCWIRevalidationConsumer resolves only the declared acceptance edge.
// Historical behavior remains complete: its original ordinary consumer owns
// current acceptance, including after credit has been revoked.
func RuntimeCWIRevalidationConsumer(facts RuntimeFacts, dependency RuntimeCoverageItemFact) (RuntimeCoverageItemFact, bool) {
	id := dependency.ID
	if dependency.HistoricalBehavior != nil {
		id = dependency.HistoricalBehavior.Source.AcceptanceCoverageWorkItemID
	}
	for _, row := range facts.Coverage {
		if row.ID == id && row.CWIConvergenceState == RuntimeCWIConvergenceStateReopened && len(row.ReopenLineage) > 0 {
			return row, true
		}
	}
	return RuntimeCoverageItemFact{}, false
}

// RuntimeCWIReopenHasOwnerSource consumes either formal invalidation ingress.
// It matches the exact source identity; it does not rerun the owner's judgment.
func RuntimeCWIReopenHasOwnerSource(facts RuntimeFacts, reconciliation RuntimeCWIReconciliationLedger, id common.CoverageWorkItemID, cause RuntimeCWIReopenLineage, revision uint64) bool {
	for _, o := range facts.EvidenceApplicabilityObservations {
		if o.Request.RequestID != cause.TriggerRef || o.RecordedAtStateRevision > revision {
			continue
		}
		for _, fact := range facts.Evidence.Accepted {
			if fact.ID == o.Request.SourceEvidenceID && fact.Kind == EvidenceKindValidation && containsStringValue(fact.BindingRefs, cause.PreviousBindingRef.BindingID) && RuntimeEvidenceHasStaleApplicability(fact, []RuntimeEvidenceApplicabilityObservation{o}, o.Request.EvidenceRevision) {
				return true
			}
		}
	}
	match := func(request *RuntimeCWIReconciliationRequest, batch *RuntimeCWIReconciliationBatch, results []RuntimeCWIReconciliationResult, reopened []RuntimeCWIReconciliationReopenedLineage) bool {
		if request == nil || batch == nil || request.WorkspaceID != facts.WorkspaceID || request.RunID != facts.RunID || request.DefinitionRevision != facts.DefinitionRevision {
			return false
		}
		bindingFound := false
		for _, b := range request.Bindings {
			bindingFound = bindingFound || b.CoverageWorkItemID == id && b.BindingID == cause.PreviousBindingRef.BindingID && b.BindingHash == cause.PreviousBindingRef.BindingHash
		}
		if !bindingFound {
			return false
		}
		for _, source := range reopened {
			if source.WorkspaceID != request.WorkspaceID || source.RunID != request.RunID || source.RequestID != request.RequestID || source.Generation != request.Generation || source.BatchHash != batch.Hash || source.CoverageWorkItemID != id || source.BindingID != cause.PreviousBindingRef.BindingID || source.ObservationRef != cause.TriggerRef {
				continue
			}
			for _, result := range results {
				if result.Disposition == RuntimeCWIReconciliationDispositionReopen && result.RequestID == source.RequestID && result.WorkspaceID == source.WorkspaceID && result.RunID == source.RunID && result.Generation == source.Generation && result.BatchHash == source.BatchHash && result.CoverageItemID == id && result.BindingID == source.BindingID && result.ObservationRef == source.ObservationRef {
					return true
				}
			}
		}
		return false
	}
	if match(reconciliation.Request, reconciliation.Batch, reconciliation.Results, reconciliation.Reopened) {
		return true
	}
	for _, h := range reconciliation.History {
		if match(&h.Request, h.Batch, h.Results, h.Reopened) {
			return true
		}
	}
	return false
}

// ValidateRuntimeCWISuspensionSources rechecks original receipt/evidence,
// semantic binding, exact direct edge, reopen and stale owner observation.
// Enclosing aggregate gates independently reverse-prove complete dispatch,
// assessment and semantic provider/owner preimages. No self-reported pause hash
// can replace any of those sources.
func ValidateRuntimeCWISuspensionSources(facts RuntimeFacts, completed []RuntimeDispatchRecord, revision uint64, reconciliation RuntimeCWIReconciliationLedger) error {
	active := 0
	var previousPauseRevision uint64
	seenSources := make(map[string]bool, len(facts.CWISuspensions))
	rows := make(map[common.CoverageWorkItemID]RuntimeCoverageItemFact, len(facts.Coverage))
	for _, row := range facts.Coverage {
		rows[row.ID] = row
	}
	for _, s := range facts.CWISuspensions {
		if s.ProtocolRef != RuntimeCWISuspensionV1 || s.SuspendedAtStateRevision == 0 || s.SuspendedAtStateRevision > revision || s.SuspendedEvidenceRevision == "" || EvidenceRevisionAfter(s.SuspendedEvidenceRevision, facts.EvidenceRevision) {
			return fmt.Errorf("runtime CWI suspension protocol/revision mismatch")
		}
		sourceKey := s.FrozenBinding.BindingID + "\x00" + s.FrozenBinding.SemanticManifestHash + "\x00" + string(s.PredecessorID) + "\x00" + s.ReopenSource.TriggerRef
		if s.SuspendedAtStateRevision <= previousPauseRevision || seenSources[sourceKey] {
			return fmt.Errorf("runtime CWI suspension duplicates or reorders original pause source")
		}
		previousPauseRevision, seenSources[sourceKey] = s.SuspendedAtStateRevision, true
		if !reflect.DeepEqual(s.FrozenBinding, s.FrozenBinding.Normalized()) || !reflect.DeepEqual(s.ReopenSource, s.ReopenSource.Normalized()) || (s.ResumeBinding != nil && !reflect.DeepEqual(*s.ResumeBinding, s.ResumeBinding.Normalized())) {
			return fmt.Errorf("runtime CWI suspension source must be raw canonical")
		}
		archived := make(map[string]RuntimePrerequisite)
		for _, prerequisite := range s.FrozenPrerequisites {
			if prerequisite.ID == "" || prerequisite.Scope != RuntimePrerequisiteScopeFocus || prerequisite.FocusID != s.FrozenFocus.ID {
				return fmt.Errorf("runtime CWI suspension archived prerequisite scope drift")
			}
			if _, duplicate := archived[prerequisite.ID]; duplicate {
				return fmt.Errorf("runtime CWI suspension duplicates archived prerequisite")
			}
			archived[prerequisite.ID] = prerequisite
		}
		observed := make(map[string]bool)
		for _, observation := range s.FrozenObservations {
			if err := observation.Validate(); err != nil {
				return err
			}
			prerequisite, exists := archived[observation.PrerequisiteID]
			if !exists || observation.OwnerModuleID != prerequisite.OwnerModuleID || observation.Kind != prerequisite.Kind || observed[observation.PrerequisiteID] {
				return fmt.Errorf("runtime CWI suspension archived owner observation drift")
			}
			observed[observation.PrerequisiteID] = true
		}
		if len(s.FrozenFocus.CoverageWorkItemIDs) != 1 || len(s.FrozenFocus.CoverageEvidenceBindings) != 1 {
			return fmt.Errorf("runtime CWI suspension requires one exact frozen binding")
		}
		binding := s.FrozenFocus.CoverageEvidenceBindings[0]
		if err := ValidateCoverageEvidenceBindingsForFocus(s.FrozenFocus, facts.DefinitionRevision); err != nil {
			return err
		}
		if !SameRuntimeCWIBindingRef(s.FrozenBinding, RuntimeCWIBindingRefForCoverageBinding(binding, s.FrozenBinding.SemanticManifestHash)) {
			return fmt.Errorf("runtime CWI suspension frozen binding drift")
		}
		if err := s.ReopenSource.Validate(); err != nil {
			return err
		}
		frozen, found := rows[binding.CoverageWorkItemID]
		predecessor, predecessorFound := rows[s.PredecessorID]
		if !found || !predecessorFound || !RuntimeCWISuspensionOrdinaryPredecessorSupported(facts, s.PredecessorID) {
			return fmt.Errorf("runtime CWI suspension has no ordinary predecessor declaration")
		}
		direct := false
		for _, id := range frozen.PrerequisiteCoverageWorkItemIDs {
			dependency := rows[id]
			target := dependency.ID
			if dependency.HistoricalBehavior != nil {
				target = dependency.HistoricalBehavior.Source.AcceptanceCoverageWorkItemID
			}
			direct = direct || target == s.PredecessorID
		}
		reopened := false
		for _, source := range predecessor.ReopenLineage {
			reopened = reopened || reflect.DeepEqual(source, s.ReopenSource)
		}
		if !direct || !reopened {
			return fmt.Errorf("runtime CWI suspension has no exact direct reopen source")
		}
		if !RuntimeCWIReopenHasOwnerSource(facts, reconciliation, s.PredecessorID, s.ReopenSource, s.SuspendedAtStateRevision) {
			return fmt.Errorf("runtime CWI suspension has no original owner invalidation")
		}
		effect := false
		for _, fact := range facts.Evidence.Accepted {
			if fact.ID != s.EffectEvidenceID || EvidenceRevisionAfter(fact.Revision, s.SuspendedEvidenceRevision) || fact.Kind != EvidenceKindStateChange || !EvidenceAppliesToCoverageWorkItem(s.FrozenFocus, fact, binding.CoverageWorkItemID) {
				continue
			}
			for _, dispatch := range completed {
				if dispatch.ID != fact.DispatchID {
					continue
				}
				for _, execution := range dispatch.OperationExecutions {
					if execution.OperationID == fact.OperationID && execution.State == OperationExecutionEvidenceCommitted && execution.EvidenceFactID == fact.ID && execution.EvidenceFactHash == StableRuntimeEvidenceFactHash(fact) && RuntimeEffectIsDurable(execution.ExpectedEffectScope) {
						effect = true
					}
				}
			}
		}
		if !effect && s.EffectEvidenceID != "" {
			return fmt.Errorf("runtime CWI suspension has no completed durable effect source")
		}
		if s.EffectEvidenceID == "" {
			for _, fact := range facts.Evidence.Accepted {
				if fact.Kind == EvidenceKindStateChange && !EvidenceRevisionAfter(fact.Revision, s.SuspendedEvidenceRevision) && EvidenceAppliesToCoverageWorkItem(s.FrozenFocus, fact, binding.CoverageWorkItemID) {
					return fmt.Errorf("runtime CWI suspension dropped an existing effect")
				}
			}
		}
		if s.Repair.FocusID != "" && s.Repair.FocusID != s.FrozenFocus.ID || s.RepairDirective != nil && s.RepairDirective.FocusID != s.FrozenFocus.ID {
			return fmt.Errorf("runtime CWI suspension repair scope drift")
		}
		if s.ResumeBinding == nil {
			active++
			if s.ResumeAssessmentID != "" || s.ResumeEvidenceRevision != "" || s.ResumeObservationCount != 0 || frozen.CurrentBindingRef == nil || !SameRuntimeCWIBindingRef(*frozen.CurrentBindingRef, s.FrozenBinding) || frozen.CWIConvergenceState != RuntimeCWIConvergenceStateBound || facts.Focus.ID == s.FrozenFocus.ID {
				return fmt.Errorf("runtime CWI suspension lost its frozen transaction")
			}
			matches := 0
			for _, focus := range facts.RemainingFocuses {
				if reflect.DeepEqual(focus, s.FrozenFocus) {
					matches++
				}
			}
			if matches != 1 || len(facts.Focus.CoverageWorkItemIDs) != 1 || facts.Focus.CoverageWorkItemIDs[0] != s.PredecessorID {
				return fmt.Errorf("runtime CWI suspension cursor is not its exact predecessor")
			}
			continue
		}
		if s.ResumeEvidenceRevision == "" || s.ResumeAssessmentID == "" || s.ResumeObservationCount <= 0 || s.ResumeObservationCount > len(facts.EvidenceApplicabilityObservations) || SameRuntimeCWIBindingRef(*s.ResumeBinding, s.ReopenSource.PreviousBindingRef) {
			return fmt.Errorf("runtime CWI suspension resume proof is incomplete or reused")
		}
		assessmentFound := false
		for _, assessment := range facts.Assessments {
			if assessment.ID == s.ResumeAssessmentID && assessment.FocusID == s.ResumeBinding.FocusID && assessment.Disposition == AssessmentDispositionSatisfied && !EvidenceRevisionAfter(assessment.Revision, s.ResumeEvidenceRevision) {
				for _, id := range assessment.CoverageWorkItemIDs {
					assessmentFound = assessmentFound || id == s.PredecessorID
				}
			}
		}
		proofFound := false
		for _, dispatch := range completed {
			if dispatch.ID != s.ResumeAssessmentID || dispatch.AssessmentAdjudication == nil || dispatch.AssessmentAuthorityInput == nil || dispatch.AssessmentAdjudication.Disposition != AssessmentDispositionSatisfied || dispatch.Transaction.FocusID != s.ResumeBinding.FocusID {
				continue
			}
			for _, b := range dispatch.CoverageBindings {
				if b.ID != s.ResumeBinding.BindingID || b.Hash != s.ResumeBinding.BindingHash || b.CoverageWorkItemID != s.PredecessorID {
					continue
				}
				for _, fact := range facts.Evidence.Accepted {
					selected := false
					for _, proof := range dispatch.AssessmentAdjudication.Proofs {
						selected = selected || proof.EvidenceFactID == fact.ID && proof.EvidenceFactHash == StableRuntimeEvidenceFactHash(fact) && proof.CoverageWorkItemID == s.PredecessorID
					}
					if selected && fact.ApplicabilityDependencies != nil && fact.Kind == EvidenceKindValidation && containsStringValue(fact.BindingRefs, b.ID) && !EvidenceRevisionAfter(fact.Revision, s.ResumeEvidenceRevision) &&
						RuntimeEvidenceHasFreshApplicability(fact, facts.EvidenceApplicabilityObservations[:s.ResumeObservationCount], s.ResumeEvidenceRevision, facts.EvidenceApplicabilityObservations[s.ResumeObservationCount-1].Request.ReconciliationRequestHash) {
						proofFound = true
					}
				}
			}
		}
		if !assessmentFound || !proofFound {
			return fmt.Errorf("runtime CWI suspension resume has no fresh ordinary assessment/owner proof: assessment=%t proof=%t", assessmentFound, proofFound)
		}
	}
	if active > 1 {
		return fmt.Errorf("runtime CWI suspension cannot nest frozen transactions")
	}
	return nil
}
