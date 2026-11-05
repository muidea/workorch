package workorchruntimecontract

import (
	"fmt"
	"reflect"
)

// Explicit offline audit only. Replays the original full classifier, while
// ordinary production reads consume the already committed owner result.
func auditCommittedAssessmentAdjudication(
	dispatch RuntimeDispatchRecord,
	fact RuntimeAssessmentFact,
	facts RuntimeFacts,
	completed []RuntimeDispatchRecord,
	lineage []RuntimeCoverageProofLineage,
	lineageSources RuntimeLineageAssertionSources,
) error {
	return auditCommittedAssessmentAdjudicationInSession(dispatch, fact, facts, completed, lineage, lineageSources, &runtimeHistoricalPreimageSession{})
}

func auditCommittedAssessmentAdjudicationInSession(dispatch RuntimeDispatchRecord, fact RuntimeAssessmentFact, facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, lineageSources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) error {
	if dispatch.AssessmentAdjudication == nil || dispatch.AssessmentAdjudicationHash == "" || fact.AdjudicationHash == "" {
		return fmt.Errorf("committed runtime assessment dispatch %q requires double-entry adjudication", dispatch.ID)
	}
	actual, err := validateAndNormalizeRuntimeAssessmentAdjudication(*dispatch.AssessmentAdjudication)
	if err != nil {
		return err
	}
	if dispatch.AssessmentAdjudicationHash != StableRuntimeAssessmentAdjudicationHash(actual) || fact.AdjudicationHash != dispatch.AssessmentAdjudicationHash {
		return fmt.Errorf("runtime assessment dispatch %q adjudication hash mismatch", dispatch.ID)
	}
	if dispatch.AssessmentAuthorityInput == nil {
		return fmt.Errorf("runtime assessment adjudication requires assessment dispatch authority")
	}
	authority := dispatch.AssessmentAuthorityInput.Normalized()
	source, err := RuntimeAssessmentSourceProjection(facts, fact.FocusID, fact.CoverageWorkItemIDs)
	if err != nil {
		return err
	}
	if authority.SourceSliceID != source.SliceID || authority.SourceScope != source.AssessmentScope ||
		authority.SourceID != source.AssessmentID || authority.SourceText != source.AssessmentText ||
		!sameCoverageWorkItemIDs(authority.Focus.CoverageWorkItemIDs, fact.CoverageWorkItemIDs) {
		return fmt.Errorf("runtime assessment dispatch %q authority source does not match definition seed", dispatch.ID)
	}
	expected, err := deriveRuntimeAssessmentAdjudicationInSession(dispatch, facts, completed, lineage, lineageSources, session)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) || fact.Disposition != expected.Disposition {
		return fmt.Errorf("runtime assessment dispatch %q adjudication does not match immutable proof history", dispatch.ID)
	}
	return nil
}

// auditCommittedAssessmentAdjudications enforces the completed-dispatch side
// of the assessment double entry after the global evidence ledger has proved
// the one-to-one fact/dispatch chronology.
func auditCommittedAssessmentAdjudications(facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, lineageSources RuntimeLineageAssertionSources) error {
	return auditCommittedAssessmentAdjudicationsInSession(facts, completed, lineage, lineageSources, &runtimeHistoricalPreimageSession{})
}

func auditCommittedAssessmentAdjudicationsInSession(facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, lineageSources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) error {
	return runtimeProcessingRegion("assessment_adjudications", func() error {
		return auditCommittedAssessmentAdjudicationsComplete(facts, completed, lineage, lineageSources, session)
	})
}

func auditCommittedAssessmentAdjudicationsComplete(facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, lineageSources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) error {
	byID := make(map[string]RuntimeAssessmentFact, len(facts.Assessments))
	for _, fact := range facts.Assessments {
		byID[fact.ID] = fact
	}
	for _, dispatch := range completed {
		// Invalidated assessment dispatches are durable SM-3 history, but they
		// never crossed the SM-2 assessment commit boundary and therefore must
		// not have a fact or adjudication. Dispatch intrinsic validation and the
		// evidence-sequence invariant enforce that negative side of the double
		// entry; only committed dispatches enter the positive adjudication proof.
		if dispatch.Candidate.Assessment == nil || dispatch.State != RuntimeDispatchStateCommitted {
			continue
		}
		fact, found := byID[dispatch.ID]
		if !found {
			return fmt.Errorf("committed runtime assessment dispatch %q has no assessment fact", dispatch.ID)
		}
		if err := auditCommittedAssessmentAdjudicationInSession(dispatch, fact, facts, completed, lineage, lineageSources, session); err != nil {
			return err
		}
	}
	return nil
}

// DeriveRuntimeAssessmentAdjudication deterministically replays the canonical
// classification from the assessment dispatch's pre-seal input, the immutable
// completed-dispatch history, accepted receipt facts and proof lineage.
func DeriveRuntimeAssessmentAdjudication(
	dispatch RuntimeDispatchRecord,
	facts RuntimeFacts,
	completed []RuntimeDispatchRecord,
	lineage []RuntimeCoverageProofLineage,
	lineageSources RuntimeLineageAssertionSources,
) (RuntimeAssessmentAdjudication, error) {
	return deriveRuntimeAssessmentAdjudicationInSession(dispatch, facts, completed, lineage, lineageSources, &runtimeHistoricalPreimageSession{})
}

func deriveRuntimeAssessmentAdjudicationInSession(dispatch RuntimeDispatchRecord, facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, lineageSources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) (RuntimeAssessmentAdjudication, error) {
	value, err := compileRuntimeAssessmentEvaluationInSession(dispatch, facts, completed, lineage, lineageSources, session)
	return value.Adjudication, err
}
