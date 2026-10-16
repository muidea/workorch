package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type runtimeEvidenceSequenceEntry struct {
	revision        string
	evidenceFactID  common.EvidenceRecordID
	assessmentID    string
	resolutionIndex int
}

// ValidateRuntimeEvidenceLedgerSequence is the fresh-load invariant for the
// single Orchestrator evidence ledger. Completed dispatch order, followed by
// the optional active dispatch, is the only chronology source. Operation,
// governance and assessment projections all consume that one sequence; a
// governance-resume clone keeps its source fact but never consumes it again.
//
// This validator deliberately accepts raw persisted values. It rejects any
// representation that normalization would trim, sort or de-duplicate before
// proving the ledger, so callers must invoke it before a Clone/Normalized
// boundary can erase malformed recovery state.
func ValidateRuntimeEvidenceLedgerSequence(
	sequence uint64,
	facts RuntimeFacts,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	resolutions []RuntimeGovernanceResolutionRecord,
) error {
	if !reflect.DeepEqual(facts.Evidence.Accepted, normalizeEvidenceFacts(facts.Evidence.Accepted)) ||
		!reflect.DeepEqual(facts.Evidence.Rejected, normalizeEvidenceFacts(facts.Evidence.Rejected)) {
		return fmt.Errorf("runtime evidence ledger requires canonical raw facts")
	}
	if !reflect.DeepEqual(facts.Assessments, normalizeRuntimeAssessmentFacts(facts.Assessments)) {
		return fmt.Errorf("runtime assessment ledger requires canonical raw facts")
	}
	if err := ValidateRuntimeGovernanceResolutionRecords(resolutions); err != nil {
		return err
	}

	factsByID := make(map[common.EvidenceRecordID]EvidenceFact, len(facts.Evidence.Accepted)+len(facts.Evidence.Rejected))
	for ledger, values := range map[string][]EvidenceFact{
		"accepted": facts.Evidence.Accepted,
		"rejected": facts.Evidence.Rejected,
	} {
		for _, fact := range values {
			if err := ValidateRuntimeEvidencePurposeKind(fact.ApplicabilityDependencies, fact.Kind); err != nil {
				return err
			}
			if err := ValidateRuntimeEvidenceDependencyPair(fact.OwnerRevisionSet, fact.ApplicabilityDependencies, facts.DefinitionRevision); err != nil {
				return fmt.Errorf("runtime %s evidence dependency pair: %w", ledger, err)
			}
			if err := validateRuntimeOwnerRevisionSetRaw(fact.OwnerRevisionSet); err != nil {
				return fmt.Errorf("runtime %s evidence owner dependencies: %w", ledger, err)
			}
			if fact.OwnerRevisionSet != nil && fact.OwnerRevisionSet.DefinitionRevision != facts.DefinitionRevision {
				return fmt.Errorf("runtime %s evidence owner dependencies definition mismatch", ledger)
			}
			if fact.ID == "" || fact.Revision == "" {
				return fmt.Errorf("runtime %s evidence fact requires identity and revision", ledger)
			}
			if fact.Kind == EvidenceKindAssessment {
				return fmt.Errorf("runtime %s evidence fact %q cannot use assessment kind; assessments require their typed ledger", ledger, fact.ID)
			}
			if fact.Kind != EvidenceKindGovernance && fact.FocusID == "" {
				return fmt.Errorf("ordinary runtime %s evidence fact %q requires immutable focus lineage", ledger, fact.ID)
			}
			if _, duplicate := factsByID[fact.ID]; duplicate {
				return fmt.Errorf("runtime evidence fact %q is duplicated across accepted/rejected ledgers", fact.ID)
			}
			factsByID[fact.ID] = fact
		}
	}
	assessmentsByID := make(map[string]RuntimeAssessmentFact, len(facts.Assessments))
	for _, assessment := range facts.Assessments {
		if assessment.ID == "" || assessment.Revision == "" {
			return fmt.Errorf("runtime assessment fact requires identity and revision")
		}
		if _, duplicate := assessmentsByID[assessment.ID]; duplicate {
			return fmt.Errorf("runtime assessment fact %q is duplicated", assessment.ID)
		}
		assessmentsByID[assessment.ID] = assessment
	}

	consumedFacts := make(map[common.EvidenceRecordID]int, len(factsByID))
	consumedAssessments := make(map[string]int, len(assessmentsByID))
	consumedResolutions := make(map[int]int, len(resolutions))
	currentSequence := uint64(0)
	consumeDispatch := func(dispatch RuntimeDispatchRecord, activeDispatch bool) error {
		expectedBaseline := runtimeEvidenceRevisionForSequence(currentSequence)
		if dispatch.Transaction.EvidenceRevision != expectedBaseline {
			return fmt.Errorf("runtime dispatch %q evidence baseline %q does not match global ledger %q", dispatch.ID, dispatch.Transaction.EvidenceRevision, expectedBaseline)
		}
		entries, err := runtimeEvidenceSequenceEntriesForDispatch(
			dispatch,
			activeDispatch,
			facts,
			factsByID,
			assessmentsByID,
			resolutions,
			consumedFacts,
		)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].revision != entries[j].revision {
				return entries[i].revision < entries[j].revision
			}
			if entries[i].evidenceFactID != entries[j].evidenceFactID {
				return entries[i].evidenceFactID < entries[j].evidenceFactID
			}
			return entries[i].assessmentID < entries[j].assessmentID
		})
		for _, entry := range entries {
			if currentSequence == ^uint64(0) {
				return fmt.Errorf("runtime evidence sequence overflow")
			}
			currentSequence++
			expectedRevision := runtimeEvidenceRevisionForSequence(currentSequence)
			if entry.revision != expectedRevision {
				return fmt.Errorf("runtime evidence revision %q does not match global consecutive revision %q", entry.revision, expectedRevision)
			}
			consumed := false
			if entry.evidenceFactID != "" {
				consumedFacts[entry.evidenceFactID]++
				consumed = true
			}
			if entry.assessmentID != "" {
				consumedAssessments[entry.assessmentID]++
				consumed = true
			}
			if entry.resolutionIndex >= 0 {
				consumedResolutions[entry.resolutionIndex]++
				consumed = true
			}
			if !consumed {
				return fmt.Errorf("runtime evidence sequence entry has no typed ledger identity")
			}
		}
		return nil
	}

	for _, dispatch := range completed {
		if err := consumeDispatch(dispatch, false); err != nil {
			return err
		}
	}
	if active != nil {
		if err := consumeDispatch(*active, true); err != nil {
			return err
		}
	}

	for id := range factsByID {
		if consumedFacts[id] != 1 {
			return fmt.Errorf("runtime evidence fact %q must be consumed exactly once by its persisted dispatch, got %d", id, consumedFacts[id])
		}
	}
	for id := range assessmentsByID {
		if consumedAssessments[id] != 1 {
			return fmt.Errorf("runtime assessment fact %q must be consumed exactly once by its committed dispatch, got %d", id, consumedAssessments[id])
		}
	}
	for idx, resolution := range resolutions {
		if consumedResolutions[idx] != 1 {
			return fmt.Errorf("runtime governance resolution %q must be consumed exactly once by its committed dispatch, got %d", resolution.ResolutionID, consumedResolutions[idx])
		}
	}
	if sequence != currentSequence {
		return fmt.Errorf("runtime evidence sequence %d does not match globally consumed fact count %d", sequence, currentSequence)
	}
	expectedFinalRevision := runtimeEvidenceRevisionForSequence(currentSequence)
	if facts.EvidenceRevision != expectedFinalRevision {
		return fmt.Errorf("runtime facts evidence revision %q does not match global ledger %q", facts.EvidenceRevision, expectedFinalRevision)
	}
	return nil
}

func runtimeEvidenceSequenceEntriesForDispatch(
	dispatch RuntimeDispatchRecord,
	active bool,
	facts RuntimeFacts,
	factsByID map[common.EvidenceRecordID]EvidenceFact,
	assessmentsByID map[string]RuntimeAssessmentFact,
	resolutions []RuntimeGovernanceResolutionRecord,
	consumedFacts map[common.EvidenceRecordID]int,
) ([]runtimeEvidenceSequenceEntry, error) {
	switch {
	case dispatch.Candidate.Governance != nil:
		return runtimeGovernanceEvidenceSequenceEntries(dispatch, active, factsByID, resolutions)
	case dispatch.Candidate.Assessment != nil:
		return runtimeAssessmentEvidenceSequenceEntries(dispatch, active, facts, assessmentsByID)
	case dispatch.Candidate.StateChange != nil || dispatch.Candidate.Validation != nil:
		return runtimeOperationEvidenceSequenceEntries(dispatch, factsByID, consumedFacts)
	default:
		return nil, fmt.Errorf("runtime dispatch %q has no supported candidate for evidence sequencing", dispatch.ID)
	}
}

func runtimeOperationEvidenceSequenceEntries(
	dispatch RuntimeDispatchRecord,
	factsByID map[common.EvidenceRecordID]EvidenceFact,
	consumedFacts map[common.EvidenceRecordID]int,
) ([]runtimeEvidenceSequenceEntry, error) {
	entries := make([]runtimeEvidenceSequenceEntry, 0, len(dispatch.OperationExecutions))
	seenOwnFacts := make(map[common.EvidenceRecordID]struct{})
	for _, rawExecution := range dispatch.OperationExecutions {
		execution := rawExecution.Normalized()
		if execution.State != OperationExecutionEvidenceCommitted {
			continue
		}
		if execution.EvidenceFactID == "" || execution.EvidenceFactHash == "" {
			return nil, fmt.Errorf("runtime dispatch %q evidence-committed operation %q has no fact identity", dispatch.ID, execution.OperationID)
		}
		fact, found := factsByID[execution.EvidenceFactID]
		if !found {
			return nil, fmt.Errorf("runtime dispatch %q evidence-committed operation %q has no persisted fact %q", dispatch.ID, execution.OperationID, execution.EvidenceFactID)
		}
		if fact.Kind == EvidenceKindGovernance || fact.Kind == EvidenceKindAssessment {
			return nil, fmt.Errorf("runtime dispatch %q operation %q references reserved evidence kind %q", dispatch.ID, execution.OperationID, fact.Kind)
		}
		if fact.OperationID != execution.OperationID || execution.EvidenceFactHash != StableRuntimeEvidenceFactHash(fact) {
			return nil, fmt.Errorf("runtime dispatch %q operation %q evidence fact projection mismatch", dispatch.ID, execution.OperationID)
		}
		if fact.DispatchID != dispatch.ID {
			// A governance resume may carry an already-completed sibling execution.
			// Its source fact must already have been consumed by the earlier source
			// dispatch and never advances the sequence at this hop.
			if consumedFacts[fact.ID] != 1 {
				return nil, fmt.Errorf("runtime dispatch %q operation %q clones evidence fact %q before its owning dispatch", dispatch.ID, execution.OperationID, fact.ID)
			}
			continue
		}
		if _, duplicate := seenOwnFacts[fact.ID]; duplicate {
			return nil, fmt.Errorf("runtime dispatch %q consumes evidence fact %q more than once", dispatch.ID, fact.ID)
		}
		seenOwnFacts[fact.ID] = struct{}{}
		entries = append(entries, runtimeEvidenceSequenceEntry{revision: fact.Revision, evidenceFactID: fact.ID, resolutionIndex: -1})
	}
	return entries, nil
}

func runtimeGovernanceEvidenceSequenceEntries(
	dispatch RuntimeDispatchRecord,
	active bool,
	factsByID map[common.EvidenceRecordID]EvidenceFact,
	resolutions []RuntimeGovernanceResolutionRecord,
) ([]runtimeEvidenceSequenceEntry, error) {
	resolutionIndexes := make([]int, 0, 1)
	for idx, resolution := range resolutions {
		if resolution.DispatchID == dispatch.ID {
			resolutionIndexes = append(resolutionIndexes, idx)
		}
	}
	factIDs := make([]common.EvidenceRecordID, 0, 1)
	for id, fact := range factsByID {
		if fact.Kind == EvidenceKindGovernance && fact.DispatchID == dispatch.ID {
			factIDs = append(factIDs, id)
		}
	}
	committed := !active && dispatch.State == RuntimeDispatchStateCommitted
	if !committed {
		if len(resolutionIndexes) != 0 || len(factIDs) != 0 {
			return nil, fmt.Errorf("non-committed runtime governance dispatch %q cannot carry resolution evidence", dispatch.ID)
		}
		return nil, nil
	}
	if len(resolutionIndexes) != 1 || len(factIDs) != 1 {
		return nil, fmt.Errorf("committed runtime governance dispatch %q requires exactly one resolution and fact", dispatch.ID)
	}
	resolutionIndex := resolutionIndexes[0]
	resolution := resolutions[resolutionIndex].Resolution()
	fact := factsByID[factIDs[0]]
	if !RuntimeGovernanceEvidenceFactMatchesResolution(fact, resolution) {
		return nil, fmt.Errorf("runtime governance dispatch %q fact is not the exact canonical resolution projection", dispatch.ID)
	}
	return []runtimeEvidenceSequenceEntry{{revision: fact.Revision, evidenceFactID: fact.ID, resolutionIndex: resolutionIndex}}, nil
}

func runtimeAssessmentEvidenceSequenceEntries(
	dispatch RuntimeDispatchRecord,
	active bool,
	facts RuntimeFacts,
	assessmentsByID map[string]RuntimeAssessmentFact,
) ([]runtimeEvidenceSequenceEntry, error) {
	fact, found := assessmentsByID[dispatch.ID]
	committed := !active && dispatch.State == RuntimeDispatchStateCommitted
	if !committed {
		if found {
			return nil, fmt.Errorf("non-committed runtime assessment dispatch %q cannot carry an assessment fact", dispatch.ID)
		}
		return nil, nil
	}
	if !found {
		return nil, fmt.Errorf("committed runtime assessment dispatch %q requires exactly one assessment fact", dispatch.ID)
	}
	if err := runtimeAssessmentFactMatchesDispatch(facts, dispatch, fact); err != nil {
		return nil, err
	}
	return []runtimeEvidenceSequenceEntry{{revision: fact.Revision, assessmentID: fact.ID, resolutionIndex: -1}}, nil
}

func runtimeAssessmentFactMatchesDispatch(facts RuntimeFacts, dispatch RuntimeDispatchRecord, fact RuntimeAssessmentFact) error {
	if dispatch.Candidate.Assessment == nil {
		return fmt.Errorf("runtime assessment fact %q has no assessment candidate", fact.ID)
	}
	candidate := dispatch.Candidate.Assessment.Normalized()
	if fact.ID != dispatch.ID || fact.FocusID != dispatch.Transaction.FocusID || fact.FocusID != candidate.FocusID ||
		!sameCoverageWorkItemIDs(fact.CoverageWorkItemIDs, candidate.CoverageWorkItemIDs) ||
		fact.ClaimedDisposition != candidate.Disposition || !reflect.DeepEqual(fact.Items, candidate.Items) ||
		!reflect.DeepEqual(fact.Gap, candidate.Gap) || fact.Summary != candidate.Summary ||
		!sameStrings(fact.EvidenceRefs, candidate.EvidenceRefs) || !sameStrings(fact.ArtifactRefs, candidate.ArtifactRefs) {
		return fmt.Errorf("runtime assessment fact %q does not match its exact sealed candidate", fact.ID)
	}
	if fact.Disposition != AssessmentDispositionSatisfied && fact.Disposition != AssessmentDispositionRepairableGap {
		return fmt.Errorf("runtime assessment fact %q has non-canonical owner disposition %q", fact.ID, fact.Disposition)
	}
	sourceFocus, err := RuntimeAssessmentSourceProjection(facts, fact.FocusID, candidate.CoverageWorkItemIDs)
	if err != nil {
		return fmt.Errorf("runtime assessment fact %q: %w", fact.ID, err)
	}
	expectedSlice := candidate.SliceID
	if expectedSlice == "" {
		expectedSlice = sourceFocus.SliceID
	}
	if fact.SliceID != expectedSlice || fact.Scope != sourceFocus.AssessmentScope || fact.RequirementID != sourceFocus.AssessmentID || fact.RequirementText != sourceFocus.AssessmentText {
		return fmt.Errorf("runtime assessment fact %q does not match its immutable focus projection", fact.ID)
	}
	return nil
}

// RuntimeAssessmentSourceProjection returns the immutable requirement fields
// copied into a typed assessment fact. DefinitionSeed is the source root;
// DefinitionFocuses is an independently persisted activation projection that
// must agree exactly with the seed. Mutable focus queues are never source
// authority for a committed assessment.
func RuntimeAssessmentSourceProjection(facts RuntimeFacts, focusID string, coverageWorkItemIDs []common.CoverageWorkItemID) (CoverageFocus, error) {
	coverageWorkItemIDs = normalizeCoverageWorkItemIDs(coverageWorkItemIDs)
	if len(coverageWorkItemIDs) != 1 {
		return CoverageFocus{}, fmt.Errorf("assessment source requires exactly one definition coverage item")
	}
	coverageID := coverageWorkItemIDs[0]
	var seedItem RuntimeCoverageItem
	seedMatches := 0
	for _, item := range facts.DefinitionSeed.Clone().CoverageItems {
		if item.ID == coverageID {
			seedItem = item
			seedMatches++
		}
	}
	if seedMatches != 1 {
		return CoverageFocus{}, fmt.Errorf("assessment source requires one definition seed item, got %d", seedMatches)
	}
	definitionMatches := make([]RuntimeDefinitionFocus, 0, 1)
	for _, declaration := range facts.DefinitionFocuses {
		declaration = declaration.Normalized()
		if sameCoverageWorkItemIDs(declaration.CoverageWorkItemIDs, []common.CoverageWorkItemID{coverageID}) {
			definitionMatches = append(definitionMatches, declaration)
		}
	}
	if len(definitionMatches) != 1 {
		return CoverageFocus{}, fmt.Errorf("assessment source requires one immutable Definition projection, got %d", len(definitionMatches))
	}
	declaration := definitionMatches[0]
	if strings.TrimSpace(focusID) == "" || declaration.SliceID != seedItem.SliceID ||
		declaration.AssessmentScope != seedItem.SourceScope || declaration.AssessmentID != seedItem.SourceID ||
		declaration.AssessmentText != seedItem.TargetText {
		return CoverageFocus{}, fmt.Errorf("assessment source Definition projection does not exactly match its seed item")
	}
	return CoverageFocus{
		ID: strings.TrimSpace(focusID), SliceID: seedItem.SliceID,
		AssessmentScope: seedItem.SourceScope, AssessmentID: seedItem.SourceID, AssessmentText: seedItem.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID},
	}.Normalized(), nil
}

func runtimeEvidenceRevisionForSequence(sequence uint64) string {
	return fmt.Sprintf("runtime-evidence-%020d", sequence)
}
