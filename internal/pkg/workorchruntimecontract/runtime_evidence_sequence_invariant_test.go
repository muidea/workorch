package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateRuntimeEvidenceLedgerSequenceAcceptsTypedGlobalChainAndResumeClone(t *testing.T) {
	sequence, facts, active, completed, resolutions := runtimeEvidenceSequenceFixture(t)
	if err := ValidateRuntimeEvidenceLedgerSequence(sequence, facts, active, completed, resolutions); err != nil {
		t.Fatalf("valid global evidence ledger: %v", err)
	}

	t.Run("active partial own fact", func(t *testing.T) {
		partialFacts := facts.Normalized()
		partial := active.Clone()
		operation := RuntimeOperation{ID: "operation-active", Kind: StepKindFileWrite}
		partial.Candidate = RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: operation}}}}
		fact := EvidenceFact{
			ID: "fact-active", Kind: EvidenceKindStateChange, SubjectRef: operation.ID,
			FocusID:    "focus-1",
			DispatchID: partial.ID, OperationID: operation.ID,
			Revision: runtimeEvidenceRevisionForSequence(sequence + 1),
		}
		partialFacts.Evidence.Accepted = normalizeEvidenceFacts(append(partialFacts.Evidence.Accepted, fact))
		partialFacts.EvidenceRevision = fact.Revision
		partial.OperationExecutions = []RuntimeOperationExecutionRecord{{
			OperationID: operation.ID, State: OperationExecutionEvidenceCommitted,
			EvidenceFactID: fact.ID, EvidenceFactHash: StableRuntimeEvidenceFactHash(fact),
		}}
		if err := ValidateRuntimeEvidenceLedgerSequence(sequence+1, partialFacts, &partial, completed, resolutions); err != nil {
			t.Fatalf("active partial evidence ledger: %v", err)
		}
	})

	t.Run("rejected ordinary fact", func(t *testing.T) {
		rejected := facts.Normalized()
		ordinary := rejected.Evidence.Accepted[0]
		rejected.Evidence.Accepted = normalizeEvidenceFacts(rejected.Evidence.Accepted[1:])
		rejected.Evidence.Rejected = []EvidenceFact{ordinary}
		if err := ValidateRuntimeEvidenceLedgerSequence(sequence, rejected, active, completed, resolutions); err != nil {
			t.Fatalf("rejected ordinary fact global ledger: %v", err)
		}
	})
}

func TestValidateRuntimeEvidenceLedgerSequenceRejectsGlobalRevisionTamper(t *testing.T) {
	sequence, facts, active, completed, resolutions := runtimeEvidenceSequenceFixture(t)
	tests := []struct {
		name   string
		mutate func(*uint64, *RuntimeFacts, **RuntimeDispatchRecord, *[]RuntimeDispatchRecord)
	}{
		{name: "sequence ahead", mutate: func(value *uint64, _ *RuntimeFacts, _ **RuntimeDispatchRecord, _ *[]RuntimeDispatchRecord) {
			(*value)++
		}},
		{name: "sequence behind", mutate: func(value *uint64, _ *RuntimeFacts, _ **RuntimeDispatchRecord, _ *[]RuntimeDispatchRecord) {
			(*value)--
		}},
		{name: "facts revision", mutate: func(_ *uint64, value *RuntimeFacts, _ **RuntimeDispatchRecord, _ *[]RuntimeDispatchRecord) {
			value.EvidenceRevision = runtimeEvidenceRevisionForSequence(99)
		}},
		{name: "fact gap", mutate: func(_ *uint64, value *RuntimeFacts, _ **RuntimeDispatchRecord, _ *[]RuntimeDispatchRecord) {
			value.Evidence.Accepted[0].Revision = runtimeEvidenceRevisionForSequence(2)
		}},
		{name: "duplicate revision", mutate: func(_ *uint64, value *RuntimeFacts, _ **RuntimeDispatchRecord, _ *[]RuntimeDispatchRecord) {
			value.Evidence.Accepted[1].Revision = runtimeEvidenceRevisionForSequence(1)
		}},
		{name: "dispatch baseline", mutate: func(_ *uint64, _ *RuntimeFacts, _ **RuntimeDispatchRecord, values *[]RuntimeDispatchRecord) {
			(*values)[1].Transaction.EvidenceRevision = runtimeEvidenceRevisionForSequence(0)
		}},
		{name: "completed order", mutate: func(_ *uint64, _ *RuntimeFacts, _ **RuntimeDispatchRecord, values *[]RuntimeDispatchRecord) {
			(*values)[0], (*values)[1] = (*values)[1], (*values)[0]
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSequence := sequence
			gotFacts := facts.Normalized()
			gotActiveValue := active.Clone()
			gotActive := &gotActiveValue
			gotCompleted := cloneRuntimeDispatchRecords(completed)
			tc.mutate(&gotSequence, &gotFacts, &gotActive, &gotCompleted)
			if err := ValidateRuntimeEvidenceLedgerSequence(gotSequence, gotFacts, gotActive, gotCompleted, resolutions); err == nil {
				t.Fatal("global evidence revision tamper was accepted")
			}
		})
	}
}

func TestValidateRuntimeEvidenceLedgerSequenceRejectsOrphanDuplicateAndAssessmentKind(t *testing.T) {
	sequence, facts, active, completed, resolutions := runtimeEvidenceSequenceFixture(t)

	t.Run("orphan ordinary", func(t *testing.T) {
		drifted := facts.Normalized()
		drifted.Evidence.Accepted = normalizeEvidenceFacts(append(drifted.Evidence.Accepted, EvidenceFact{
			ID: "orphan", Kind: EvidenceKindValidation, DispatchID: completed[0].ID,
			OperationID: "orphan-operation", Revision: runtimeEvidenceRevisionForSequence(sequence + 1),
		}))
		if err := ValidateRuntimeEvidenceLedgerSequence(sequence, drifted, active, completed, resolutions); err == nil {
			t.Fatal("orphan ordinary fact was accepted")
		}
	})

	t.Run("raw duplicate", func(t *testing.T) {
		drifted := facts
		drifted.Evidence.Accepted = append(drifted.Evidence.Accepted, drifted.Evidence.Accepted[0])
		if err := ValidateRuntimeEvidenceLedgerSequence(sequence, drifted, active, completed, resolutions); err == nil || !strings.Contains(err.Error(), "canonical raw") {
			t.Fatalf("raw duplicate error = %v", err)
		}
	})

	t.Run("generic assessment kind", func(t *testing.T) {
		drifted := facts.Normalized()
		drifted.Evidence.Accepted[0].Kind = EvidenceKindAssessment
		if err := ValidateRuntimeEvidenceLedgerSequence(sequence, drifted, active, completed, resolutions); err == nil || !strings.Contains(err.Error(), "typed ledger") {
			t.Fatalf("generic assessment error = %v", err)
		}
	})
}

func TestValidateRuntimeEvidenceLedgerSequenceRejectsGovernanceCreditPollution(t *testing.T) {
	sequence, facts, active, completed, resolutions := runtimeEvidenceSequenceFixture(t)
	governanceID := common.EvidenceRecordID(resolutions[0].Resolution().EventID())
	tests := []struct {
		name   string
		mutate func(*EvidenceFact)
	}{
		{name: "operation", mutate: func(f *EvidenceFact) { f.OperationID = "operation-injected" }},
		{name: "targets", mutate: func(f *EvidenceFact) { f.TargetRefs = []string{"target.txt"} }},
		{name: "bindings", mutate: func(f *EvidenceFact) { f.BindingRefs = []string{"binding-injected"} }},
		{name: "coverage", mutate: func(f *EvidenceFact) { f.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"} }},
		{name: "quality", mutate: func(f *EvidenceFact) { f.Quality = common.EvidenceQualityValid }},
		{name: "zero work", mutate: func(f *EvidenceFact) { f.ZeroWork = true }},
		{name: "reason", mutate: func(f *EvidenceFact) { f.EvidenceReason = "injected" }},
		{name: "tags", mutate: func(f *EvidenceFact) { f.EvidenceTags = []string{"injected"} }},
		{name: "validation", mutate: func(f *EvidenceFact) { f.Validation = &RuntimeValidationObservation{RecipeID: "recipe-injected"} }},
		{name: "content address", mutate: func(f *EvidenceFact) {
			f.ContentAddress = &RuntimeEvidenceContentAddress{Algorithm: RuntimeEvidenceDigestSHA256, Digest: strings.Repeat("a", 64), Ref: "artifact://injected"}
		}},
		{name: "exit code", mutate: func(f *EvidenceFact) { code := int64(0); f.ExitCode = &code }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			drifted := facts.Normalized()
			for idx := range drifted.Evidence.Accepted {
				if drifted.Evidence.Accepted[idx].ID == governanceID {
					tc.mutate(&drifted.Evidence.Accepted[idx])
				}
			}
			if err := ValidateRuntimeEvidenceLedgerSequence(sequence, drifted, active, completed, resolutions); err == nil {
				t.Fatal("polluted governance fact was accepted")
			}
		})
	}
}

func TestValidateRuntimeEvidenceLedgerSequenceRejectsAssessmentTamper(t *testing.T) {
	sequence, facts, active, completed, resolutions := runtimeEvidenceSequenceFixture(t)
	tests := []struct {
		name   string
		mutate func(*RuntimeFacts)
	}{
		{name: "missing revision", mutate: func(value *RuntimeFacts) { value.Assessments[0].Revision = "" }},
		{name: "orphan", mutate: func(value *RuntimeFacts) {
			orphan := value.Assessments[0]
			orphan.ID = "assessment-orphan"
			orphan.Revision = runtimeEvidenceRevisionForSequence(sequence + 1)
			value.Assessments = append(value.Assessments, orphan)
		}},
		{name: "raw duplicate", mutate: func(value *RuntimeFacts) { value.Assessments = append(value.Assessments, value.Assessments[0]) }},
		{name: "dispatch id", mutate: func(value *RuntimeFacts) { value.Assessments[0].ID = "assessment-other" }},
		{name: "candidate payload", mutate: func(value *RuntimeFacts) { value.Assessments[0].Summary = "coordinated rewrite" }},
		{name: "coverage source", mutate: func(value *RuntimeFacts) { value.Assessments[0].RequirementText = "rewritten requirement" }},
		{name: "owner disposition", mutate: func(value *RuntimeFacts) { value.Assessments[0].Disposition = AssessmentDispositionGovernanceRequired }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			drifted := facts
			drifted.Evidence.Accepted = append([]EvidenceFact(nil), facts.Evidence.Accepted...)
			drifted.Assessments = append([]RuntimeAssessmentFact(nil), facts.Assessments...)
			tc.mutate(&drifted)
			if tc.name != "raw duplicate" && tc.name != "orphan" {
				drifted.Assessments = normalizeRuntimeAssessmentFacts(drifted.Assessments)
			}
			if err := ValidateRuntimeEvidenceLedgerSequence(sequence, drifted, active, completed, resolutions); err == nil {
				t.Fatal("assessment ledger tamper was accepted")
			}
		})
	}
}

func TestRuntimeAssessmentDoubleEntryAppliesOnlyToCommittedDispatches(t *testing.T) {
	t.Run("invalidated dispatch has no fact or adjudication", func(t *testing.T) {
		_, facts, active, completed, resolutions := runtimeEvidenceSequenceFixture(t)
		completed[2].State = RuntimeDispatchStateInvalidated
		completed[2].AssessmentAdjudication = nil
		completed[2].AssessmentAdjudicationHash = ""
		facts.Assessments = nil
		facts.EvidenceRevision = runtimeEvidenceRevisionForSequence(2)
		active.Transaction.EvidenceRevision = runtimeEvidenceRevisionForSequence(2)

		if err := ValidateRuntimeEvidenceLedgerSequence(2, facts, active, completed, resolutions); err != nil {
			t.Fatalf("invalidated assessment dispatch required a fact: %v", err)
		}
		if err := ValidateRuntimeAssessmentAdjudications(facts, completed, nil); err != nil {
			t.Fatalf("invalidated assessment dispatch required adjudication: %v", err)
		}
	})

	t.Run("committed dispatch requires fact", func(t *testing.T) {
		_, facts, _, completed, _ := runtimeEvidenceSequenceFixture(t)
		facts.Assessments = nil
		if err := ValidateRuntimeAssessmentAdjudications(facts, completed, nil); err == nil || !strings.Contains(err.Error(), "no assessment fact") {
			t.Fatalf("committed assessment missing-fact error = %v", err)
		}
	})

	t.Run("committed dispatch requires adjudication", func(t *testing.T) {
		_, facts, _, completed, _ := runtimeEvidenceSequenceFixture(t)
		if err := ValidateRuntimeAssessmentAdjudications(facts, completed, nil); err == nil || !strings.Contains(err.Error(), "double-entry adjudication") {
			t.Fatalf("committed assessment missing-adjudication error = %v", err)
		}
	})
}

func TestValidateRuntimeEvidenceLedgerSequenceRejectsAssessmentItemNilEmptyDrift(t *testing.T) {
	sequence, facts, active, completed, resolutions := runtimeEvidenceSequenceFixture(t)
	completed[2].Candidate.Assessment.Items = make([]RuntimeAssessmentItemFact, 0)
	if completed[2].Candidate.Assessment.Items == nil || facts.Assessments[0].Items != nil {
		t.Fatalf("fixture does not preserve sealed empty versus fact nil: candidate=%#v fact=%#v", completed[2].Candidate.Assessment.Items, facts.Assessments[0].Items)
	}

	err := ValidateRuntimeEvidenceLedgerSequence(sequence, facts, active, completed, resolutions)
	if err == nil || !strings.Contains(err.Error(), "does not match its exact sealed candidate") {
		t.Fatalf("assessment nil/empty drift error = %v", err)
	}
}

func runtimeEvidenceSequenceFixture(t *testing.T) (uint64, RuntimeFacts, *RuntimeDispatchRecord, []RuntimeDispatchRecord, []RuntimeGovernanceResolutionRecord) {
	t.Helper()
	operation := RuntimeOperation{ID: "operation-1", Kind: StepKindFileWrite}
	operationFact := EvidenceFact{
		ID: "fact-operation", Kind: EvidenceKindStateChange, SubjectRef: operation.ID,
		FocusID:    "focus-1",
		DispatchID: "dispatch-operation", OperationID: operation.ID,
		Revision: runtimeEvidenceRevisionForSequence(1),
	}
	operationDispatch := RuntimeDispatchRecord{
		ID: "dispatch-operation", Transaction: AuthorityTransaction{FocusID: operationFact.FocusID, EvidenceRevision: runtimeEvidenceRevisionForSequence(0)},
		Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: operation}}}},
		State:     RuntimeDispatchStateCommitted,
		OperationExecutions: []RuntimeOperationExecutionRecord{{
			OperationID: operation.ID, State: OperationExecutionEvidenceCommitted,
			EvidenceFactID: operationFact.ID, EvidenceFactHash: StableRuntimeEvidenceFactHash(operationFact),
		}},
	}

	resolution := RuntimeGovernanceResolution{
		ResolutionID: "resolution-1", ApprovalID: "approval-1", WorkspaceID: "workspace-1", RunID: "run-1",
		AuthoritySnapshotID: "snapshot-governance", DispatchID: "dispatch-governance",
		DependencyKind: string(GovernanceApproval), SubjectRef: "subject-1",
		Outcome: RuntimeGovernanceResolutionOutcomeContinue, Summary: "approved", EvidenceRefs: []string{"approval://1"},
	}
	governanceFact, err := RuntimeGovernanceEvidenceFactForResolution(resolution, runtimeEvidenceRevisionForSequence(2))
	if err != nil {
		t.Fatal(err)
	}
	governanceDispatch := RuntimeDispatchRecord{
		ID: "dispatch-governance", Transaction: AuthorityTransaction{EvidenceRevision: runtimeEvidenceRevisionForSequence(1)},
		Candidate: RuntimeCandidate{Governance: &GovernanceCandidate{ExternalDependency: ExternalDependency{
			Kind: resolution.DependencyKind, SubjectRef: resolution.SubjectRef, Reason: "approval required",
		}}},
		State: RuntimeDispatchStateCommitted,
	}

	assessmentCandidate := AssessmentCandidate{
		FocusID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: AssessmentDispositionSatisfied, Summary: "satisfied",
	}
	assessmentDispatch := RuntimeDispatchRecord{
		ID: "dispatch-assessment", Transaction: AuthorityTransaction{FocusID: assessmentCandidate.FocusID, EvidenceRevision: runtimeEvidenceRevisionForSequence(2)},
		Candidate: RuntimeCandidate{Assessment: &assessmentCandidate}, State: RuntimeDispatchStateCommitted,
	}
	assessmentFact := RuntimeAssessmentFact{
		ID: assessmentDispatch.ID, Revision: runtimeEvidenceRevisionForSequence(3), FocusID: assessmentCandidate.FocusID,
		SliceID: assessmentCandidate.SliceID, Scope: "dodItem", RequirementID: "dod-1", RequirementText: "must work",
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), assessmentCandidate.CoverageWorkItemIDs...),
		ClaimedDisposition:  assessmentCandidate.Disposition, Disposition: AssessmentDispositionSatisfied,
		Summary: assessmentCandidate.Summary,
	}

	cloneDispatch := RuntimeDispatchRecord{
		ID: "dispatch-resume", Transaction: AuthorityTransaction{FocusID: operationFact.FocusID, EvidenceRevision: runtimeEvidenceRevisionForSequence(3)},
		Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: operation}}}},
		State:     RuntimeDispatchStateSealed,
		OperationExecutions: []RuntimeOperationExecutionRecord{{
			OperationID: operation.ID, State: OperationExecutionEvidenceCommitted,
			EvidenceFactID: operationFact.ID, EvidenceFactHash: StableRuntimeEvidenceFactHash(operationFact),
		}},
	}
	facts := RuntimeFacts{
		EvidenceRevision:  runtimeEvidenceRevisionForSequence(3),
		DefinitionSeed:    RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "must work"}}},
		DefinitionFocuses: []RuntimeDefinitionFocus{{ID: assessmentCandidate.FocusID, SliceID: assessmentCandidate.SliceID, AssessmentScope: "dodItem", AssessmentID: "dod-1", AssessmentText: "must work", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}, EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-1")}},
		Focus: CoverageFocus{
			ID: assessmentCandidate.FocusID, SliceID: assessmentCandidate.SliceID,
			CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), assessmentCandidate.CoverageWorkItemIDs...),
			AssessmentScope:     "dodItem", AssessmentID: "dod-1", AssessmentText: "must work",
		},
		Evidence:    EvidenceLedger{Accepted: normalizeEvidenceFacts([]EvidenceFact{operationFact, governanceFact})},
		Assessments: normalizeRuntimeAssessmentFacts([]RuntimeAssessmentFact{assessmentFact}),
		Coverage: []RuntimeCoverageItemFact{{
			ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "must work", State: RuntimeCoverageItemStateOpen,
		}},
	}
	return 3, facts, &cloneDispatch, []RuntimeDispatchRecord{operationDispatch, governanceDispatch, assessmentDispatch}, []RuntimeGovernanceResolutionRecord{RuntimeGovernanceResolutionRecordFromResolution(resolution)}
}
