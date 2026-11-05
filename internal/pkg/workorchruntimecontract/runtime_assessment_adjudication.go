package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeAssessmentAuthorityInput is the complete pre-seal assessment input.
// It is immutable SM-3 dispatch authority.  In particular, owner-fact
// satisfaction is re-evaluated from this catalog snapshot rather than from a
// later mutable Runtime resource projection.
type RuntimeAssessmentAuthorityInput struct {
	AuthorityInput           RuntimeEquivalentAuthorityInputIdentity
	Purpose                  RuntimeAssessmentPurpose
	SourceSliceID            string
	SourceScope              string
	SourceID                 string
	SourceText               string
	Focus                    CoverageFocus
	OperationAdmissibilities []RuntimeOperationAdmissibility
	// Immutable SM-2 observation prefix at admission. Later
	// observations cannot change a historical assessment's classification.
	ApplicabilityObservations []RuntimeEvidenceApplicabilityObservation `json:",omitempty"`
	// Original generation at assessment admission; later Resume cannot upgrade
	// old observations, while historical classification retains this fence.
	ApplicabilityReconciliationRequestHash string `json:",omitempty"`
}

type RuntimeAssessmentPurpose string

const (
	RuntimeAssessmentPurposeDefinitionProof      RuntimeAssessmentPurpose = "definition_proof"
	RuntimeAssessmentPurposeOwnerFactAcquisition RuntimeAssessmentPurpose = "owner_fact_acquisition"
)

// RuntimeAssessmentRequiresExecutableBehaviorEvidence applies the immutable
// Definition quality gate only to terminal Definition proof. An owner-fact
// acquisition is an intermediate transaction whose sealed binding proves the
// current state-change obligation; it must reopen the CWI for a successor
// terminal-proof generation instead of requiring that state-change receipt to
// masquerade as executable-behavior evidence.
func RuntimeAssessmentRequiresExecutableBehaviorEvidence(purpose RuntimeAssessmentPurpose, focus CoverageFocus) bool {
	purpose = RuntimeAssessmentPurpose(strings.TrimSpace(string(purpose)))
	if purpose == RuntimeAssessmentPurposeOwnerFactAcquisition {
		return false
	}
	return focus.Normalized().RequireExecutableBehaviorEvidence
}

func (a RuntimeAssessmentAuthorityInput) Normalized() RuntimeAssessmentAuthorityInput {
	a.AuthorityInput = a.AuthorityInput.Normalized()
	a.Purpose = RuntimeAssessmentPurpose(strings.TrimSpace(string(a.Purpose)))
	a.SourceSliceID = strings.TrimSpace(a.SourceSliceID)
	a.SourceScope = strings.TrimSpace(a.SourceScope)
	a.SourceID = strings.TrimSpace(a.SourceID)
	a.SourceText = strings.TrimSpace(a.SourceText)
	a.Focus = a.Focus.Normalized()
	a.OperationAdmissibilities = NormalizeRuntimeOperationAdmissibilities(a.OperationAdmissibilities)
	a.ApplicabilityObservations = CloneRuntimeEvidenceApplicabilityObservations(a.ApplicabilityObservations)
	a.ApplicabilityReconciliationRequestHash = strings.TrimSpace(a.ApplicabilityReconciliationRequestHash)
	return a
}

func (a RuntimeAssessmentAuthorityInput) Clone() RuntimeAssessmentAuthorityInput {
	a = a.Normalized()
	a.OperationAdmissibilities = CloneRuntimeOperationAdmissibilities(a.OperationAdmissibilities)
	return a
}

func (a RuntimeAssessmentAuthorityInput) Validate(transaction AuthorityTransaction, bindings []CoverageEvidenceBinding) error {
	_, err := validateAndNormalizeRuntimeAssessmentAuthorityInput(a, transaction, bindings)
	return err
}

func validateAndNormalizeRuntimeAssessmentAuthorityInput(a RuntimeAssessmentAuthorityInput, transaction AuthorityTransaction, bindings []CoverageEvidenceBinding) (RuntimeAssessmentAuthorityInput, error) {
	canonical := a.Normalized()
	if !reflect.DeepEqual(a, canonical) {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("runtime assessment authority input must be raw canonical")
	}
	a = canonical
	if transaction.State != ConvergenceStateNeedAssessment || a.Focus.ID == "" || a.Focus.ID != transaction.FocusID {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("runtime assessment authority input requires its assessment transaction focus")
	}
	if a.Purpose != RuntimeAssessmentPurposeDefinitionProof && a.Purpose != RuntimeAssessmentPurposeOwnerFactAcquisition {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("runtime assessment authority input has unsupported purpose %q", a.Purpose)
	}
	if a.SourceSliceID == "" || a.SourceScope == "" || a.SourceID == "" || a.SourceText == "" {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("runtime assessment authority input requires exact definition source")
	}
	if err := a.AuthorityInput.Validate(); err != nil || StableRuntimeEquivalentAuthorityInputID(a.AuthorityInput) != transaction.AuthorityInputID {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("runtime assessment authority input identity mismatch")
	}
	if StableRuntimeAuthorityFocusHash(a.Focus) != a.AuthorityInput.FocusHash ||
		StableRuntimeOperationAdmissibilityCatalogRevisionHash(a.OperationAdmissibilities) != a.AuthorityInput.CatalogRevisionHash {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("runtime assessment authority input preimage mismatch")
	}
	if !RuntimeCoverageEvidenceBindingProjectionsEqual(a.Focus.CoverageEvidenceBindings, bindings) {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("runtime assessment authority input binding preimage mismatch")
	}
	for _, observation := range a.ApplicabilityObservations {
		if err := observation.ValidateRaw(); err != nil {
			return RuntimeAssessmentAuthorityInput{}, err
		}
		if observation.Request.EvidenceRevision > transaction.EvidenceRevision {
			return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("assessment contains future applicability observation")
		}
	}
	if StableRuntimeEvidenceApplicabilitySemanticHash(a.ApplicabilityObservations) != a.AuthorityInput.ApplicabilityObservationHash {
		return RuntimeAssessmentAuthorityInput{}, fmt.Errorf("assessment applicability observations do not match sealed authority identity")
	}
	return a, nil
}

func StableRuntimeAssessmentAuthorityInputHash(value RuntimeAssessmentAuthorityInput) string {
	// 保持原 JSON 编码，复用同次 marshal 缓冲容纳域前缀，不另建前缀＋正文缓冲。
	const prefix = "runtime-assessment-authority-input/v1\x00"
	value = value.Normalized()
	payload, _ := json.Marshal(value)
	bodyLength := len(payload)
	payload = append(payload, prefix...)
	copy(payload[len(prefix):], payload[:bodyLength])
	copy(payload, prefix)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// RuntimeAssessmentAdjudicationProof is one accepted fact selected by the
// canonical assessment classifier.  FactHash is independently rooted in the
// owning completed operation dispatch by the global evidence invariant.
type RuntimeAssessmentAdjudicationProof struct {
	CoverageWorkItemID common.CoverageWorkItemID
	EvidenceKind       string
	EvidenceFactID     common.EvidenceRecordID
	EvidenceFactHash   string
	// 原选中 evidence 的完整读取依赖；不代表 assessment 的全部依赖。
	OwnerRevisionSet          *RuntimeOwnerRevisionSet                  `json:",omitempty"`
	ApplicabilityDependencies *RuntimeEvidenceApplicabilityDependencies `json:",omitempty"`
}

func (p RuntimeAssessmentAdjudicationProof) Normalized() RuntimeAssessmentAdjudicationProof {
	p.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(p.CoverageWorkItemID)))
	p.EvidenceKind = strings.TrimSpace(p.EvidenceKind)
	p.EvidenceFactID = common.EvidenceRecordID(strings.TrimSpace(string(p.EvidenceFactID)))
	p.EvidenceFactHash = strings.TrimSpace(p.EvidenceFactHash)
	p.OwnerRevisionSet = CloneRuntimeOwnerRevisionSet(p.OwnerRevisionSet)
	p.ApplicabilityDependencies = CloneRuntimeEvidenceApplicabilityDependencies(p.ApplicabilityDependencies)
	return p
}

// RuntimeAssessmentAdjudication is the post-dispatch SM-2 classification
// proof.  It is deliberately excluded from StableRuntimeDispatchAuthorityHash:
// the immutable pre-seal AssessmentAuthorityInput is dispatch authority, while
// this record is written atomically with RuntimeAssessmentFact at commit.
type RuntimeAssessmentAdjudication struct {
	DispatchID          string
	AuthorityInputID    string
	FocusID             string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Proofs              []RuntimeAssessmentAdjudicationProof
	OwnerFactSources    []RuntimeAssessmentOwnerFactSource `json:",omitempty"`
	Disposition         AssessmentDisposition
}

func (a RuntimeAssessmentAdjudication) Normalized() RuntimeAssessmentAdjudication {
	a.DispatchID = strings.TrimSpace(a.DispatchID)
	a.AuthorityInputID = strings.TrimSpace(a.AuthorityInputID)
	a.FocusID = strings.TrimSpace(a.FocusID)
	a.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(a.CoverageWorkItemIDs)
	proofs := make([]RuntimeAssessmentAdjudicationProof, 0, len(a.Proofs))
	seen := make(map[string]struct{}, len(a.Proofs))
	for _, raw := range a.Proofs {
		proof := raw.Normalized()
		key := string(proof.CoverageWorkItemID) + "\x00" + proof.EvidenceKind + "\x00" + string(proof.EvidenceFactID)
		if proof.CoverageWorkItemID == "" || proof.EvidenceKind == "" || proof.EvidenceFactID == "" || proof.EvidenceFactHash == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		proofs = append(proofs, proof)
	}
	sort.Slice(proofs, func(i, j int) bool {
		if proofs[i].CoverageWorkItemID != proofs[j].CoverageWorkItemID {
			return proofs[i].CoverageWorkItemID < proofs[j].CoverageWorkItemID
		}
		if proofs[i].EvidenceKind != proofs[j].EvidenceKind {
			return proofs[i].EvidenceKind < proofs[j].EvidenceKind
		}
		return proofs[i].EvidenceFactID < proofs[j].EvidenceFactID
	})
	a.Proofs = nilIfEmpty(proofs)
	a.OwnerFactSources = cloneRuntimeAssessmentOwnerFactSources(a.OwnerFactSources)
	a.Disposition = a.Disposition.Normalized()
	return a
}

func (a RuntimeAssessmentAdjudication) Clone() RuntimeAssessmentAdjudication {
	a = a.Normalized()
	a.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), a.CoverageWorkItemIDs...)
	a.Proofs = append([]RuntimeAssessmentAdjudicationProof(nil), a.Proofs...)
	return a
}

func (a RuntimeAssessmentAdjudication) ValidateRaw() error {
	_, err := validateAndNormalizeRuntimeAssessmentAdjudication(a)
	return err
}

func validateAndNormalizeRuntimeAssessmentAdjudication(a RuntimeAssessmentAdjudication) (RuntimeAssessmentAdjudication, error) {
	canonical := a.Normalized()
	if !reflect.DeepEqual(a, canonical) {
		return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment adjudication must be raw canonical")
	}
	a = canonical
	if a.DispatchID == "" || a.AuthorityInputID == "" || a.FocusID == "" || len(a.CoverageWorkItemIDs) == 0 {
		return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment adjudication requires dispatch, authority, focus and coverage")
	}
	if a.Disposition != AssessmentDispositionSatisfied && a.Disposition != AssessmentDispositionRepairableGap {
		return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment adjudication has non-canonical disposition %q", a.Disposition)
	}
	if a.Disposition == AssessmentDispositionSatisfied && len(a.Proofs) == 0 {
		return RuntimeAssessmentAdjudication{}, fmt.Errorf("satisfied runtime assessment adjudication requires proof")
	}
	for _, proof := range a.Proofs {
		if err := ValidateRuntimeEvidencePurposeKind(proof.ApplicabilityDependencies, proof.EvidenceKind); err != nil {
			return RuntimeAssessmentAdjudication{}, err
		}
		if !reflect.DeepEqual(proof, proof.Normalized()) || proof.CoverageWorkItemID == "" || proof.EvidenceKind == "" || proof.EvidenceFactID == "" || proof.EvidenceFactHash == "" {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment adjudication contains malformed proof")
		}
		if err := ValidateRuntimeEvidenceDependencyPair(proof.OwnerRevisionSet, proof.ApplicabilityDependencies, ""); err != nil {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment proof owner dependencies: %w", err)
		}
	}
	seenRequirements := make(map[RuntimeOwnerFactRequirement]struct{}, len(a.OwnerFactSources))
	for _, source := range a.OwnerFactSources {
		if source.Requirement != source.Requirement.Normalized() || source.Requirement.Validate() != nil {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment owner-fact source has malformed requirement")
		}
		if _, duplicate := seenRequirements[source.Requirement]; duplicate {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment owner-fact source duplicates requirement")
		}
		seenRequirements[source.Requirement] = struct{}{}
		if source.CatalogSnapshotID != strings.TrimSpace(source.CatalogSnapshotID) || (source.CatalogSnapshotID == "") != (source.OwnerRevisionSet == nil) {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment owner-fact source catalog identity mismatch")
		}
		if source.OwnerRevisionSet != nil && len(source.OwnerRevisionSet.Entries) != 1 {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment owner-fact source must identify one exact observation")
		}
		if err := validateRuntimeOwnerRevisionSetRaw(source.OwnerRevisionSet); err != nil {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment owner-fact source dependencies: %w", err)
		}
	}
	return a, nil
}

func StableRuntimeAssessmentAdjudicationHash(value RuntimeAssessmentAdjudication) string {
	value = value.Normalized()
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(append([]byte("runtime-assessment-adjudication/v1\x00"), payload...))
	return "assessment-adjudication-" + hex.EncodeToString(digest[:])
}

// RuntimeAssessmentEvaluation is a detached submission-time product, not a
// persisted PASS credential. Coverage, owner-fact routing and the adjudication
// consume this same classification in the one owner transaction.
type RuntimeAssessmentCoverageEvaluation struct {
	CoverageWorkItemID common.CoverageWorkItemID
	EvidenceRefs       []string
	ProofComplete      bool
}
type RuntimeAssessmentEvaluation struct {
	Adjudication      RuntimeAssessmentAdjudication
	Coverage          []RuntimeAssessmentCoverageEvaluation
	MissingOwnerFacts []RuntimeOwnerFactRequirement
}

func (e RuntimeAssessmentEvaluation) ProofCompleteWithoutOwnerFacts() bool {
	if len(e.Coverage) == 0 {
		return false
	}
	for _, item := range e.Coverage {
		if !item.ProofComplete {
			return false
		}
	}
	return true
}
func CompileRuntimeAssessmentEvaluation(dispatch RuntimeDispatchRecord, facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, sources RuntimeLineageAssertionSources) (RuntimeAssessmentEvaluation, error) {
	return compileRuntimeAssessmentEvaluationInSession(dispatch, facts, completed, lineage, sources, &runtimeHistoricalPreimageSession{})
}

func compileRuntimeAssessmentEvaluationInSession(dispatch RuntimeDispatchRecord, facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, lineageSources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) (RuntimeAssessmentEvaluation, error) {
	if dispatch.Candidate.Assessment == nil || dispatch.AssessmentAuthorityInput == nil {
		return RuntimeAssessmentEvaluation{}, fmt.Errorf("runtime assessment adjudication requires assessment dispatch authority")
	}
	authority, err := validateAndNormalizeRuntimeAssessmentAuthorityInput(*dispatch.AssessmentAuthorityInput, dispatch.Transaction, dispatch.CoverageBindings)
	if err != nil {
		return RuntimeAssessmentEvaluation{}, err
	}
	candidate := dispatch.Candidate.Assessment.Normalized()
	ownerFactSources, err := deriveRuntimeAssessmentOwnerFactSourcesInSession(facts.DefinitionRevision, string(facts.WorkspaceID), *dispatch.AssessmentAuthorityInput, session.semanticSession())
	if err != nil {
		return RuntimeAssessmentEvaluation{}, err
	}
	result := RuntimeAssessmentAdjudication{
		DispatchID: dispatch.ID, AuthorityInputID: dispatch.Transaction.AuthorityInputID,
		FocusID: dispatch.Transaction.FocusID, CoverageWorkItemIDs: candidate.CoverageWorkItemIDs,
		Disposition: AssessmentDispositionSatisfied, OwnerFactSources: ownerFactSources,
	}
	// The assessment dispatch is sealed after the operation receipt and any
	// owner-fact re-observation. Its catalog snapshot is therefore the immutable
	// answer to whether the required postcondition was present at assessment
	// admission. Acquisition purpose does not waive that postcondition: doing so
	// would let the same accepted operation be classified satisfied while the
	// sealed owner snapshot still proves the required fact absent.
	evaluation := RuntimeAssessmentEvaluation{MissingOwnerFacts: RuntimeMissingOwnerFactRequirements(authority.Focus.RequiredOwnerFacts, authority.OperationAdmissibilities)}
	if len(evaluation.MissingOwnerFacts) != 0 {
		result.Disposition = AssessmentDispositionRepairableGap
	}
	for _, coverageID := range candidate.CoverageWorkItemIDs {
		binding, found := assessmentBindingForCoverage(authority.Focus.CoverageEvidenceBindings, coverageID)
		if !found {
			result.Disposition = AssessmentDispositionRepairableGap
			evaluation.Coverage = append(evaluation.Coverage, RuntimeAssessmentCoverageEvaluation{CoverageWorkItemID: coverageID})
			continue
		}
		coverageProofs, complete := deriveAssessmentCoverageProofs(dispatch, authority.Purpose, authority.Focus, binding, facts.Evidence.Accepted, completed, lineage)
		if authority.Purpose == RuntimeAssessmentPurposeDefinitionProof {
			if !reflect.DeepEqual(lineageSources.Facts, facts) {
				return RuntimeAssessmentEvaluation{}, fmt.Errorf("lineage assertion facts differ from assessment source")
			}
			if !runtimeExecutionLineageAssertionsInSession(binding, lineageSources, dispatch.Transaction.EvidenceRevision, session) {
				complete = false
			}
		}
		refs := make([]string, 0, len(coverageProofs))
		selected := make([]EvidenceFact, 0, len(coverageProofs))
		for _, proof := range coverageProofs {
			refs = append(refs, string(proof.EvidenceFactID))
			for _, fact := range facts.Evidence.Accepted {
				if fact.ID == proof.EvidenceFactID {
					selected = append(selected, fact)
					break
				}
			}
		}
		if authority.Purpose == RuntimeAssessmentPurposeDefinitionProof && !RuntimeVerificationEvidencePolicySatisfied(binding, selected, completed, lineageSources.Receipts) {
			complete = false
		}
		if !complete {
			refs = nil
		}
		evaluation.Coverage = append(evaluation.Coverage, RuntimeAssessmentCoverageEvaluation{CoverageWorkItemID: coverageID, EvidenceRefs: normalizeStrings(refs), ProofComplete: complete})
		result.Proofs = append(result.Proofs, coverageProofs...)
		if !complete {
			result.Disposition = AssessmentDispositionRepairableGap
		}
	}
	result = result.Normalized()
	for _, proof := range result.Proofs {
		if err := ValidateRuntimeEvidenceDependencyPair(proof.OwnerRevisionSet, proof.ApplicabilityDependencies, facts.DefinitionRevision); err != nil {
			return RuntimeAssessmentEvaluation{}, fmt.Errorf("runtime assessment selected proof dependencies: %w", err)
		}
		if proof.OwnerRevisionSet != nil && proof.OwnerRevisionSet.DefinitionRevision != facts.DefinitionRevision {
			return RuntimeAssessmentEvaluation{}, fmt.Errorf("runtime assessment selected proof dependencies definition mismatch")
		}
	}
	evaluation.Adjudication = result
	return evaluation, nil
}

// ValidateRuntimeAssessmentAdjudication consumes a formally committed SM-2 result.
// It checks original identity, double-entry integrity and referenced evidence;
// new assessments are classified only at the owner submission boundary.
func ValidateRuntimeAssessmentAdjudication(dispatch RuntimeDispatchRecord, fact RuntimeAssessmentFact, facts RuntimeFacts) error {
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
	// A committed adjudication is the owner's result for its original window.
	// Reading it checks identity and exact referenced bytes, never re-runs the
	// historical classifier against the growing current source graph.
	if dispatch.State != RuntimeDispatchStateCommitted || dispatch.Candidate.Assessment == nil ||
		dispatch.WorkspaceID != facts.WorkspaceID || dispatch.RunID != facts.RunID ||
		actual.DispatchID != dispatch.ID || fact.ID != dispatch.ID ||
		actual.AuthorityInputID != dispatch.Transaction.AuthorityInputID ||
		actual.FocusID != dispatch.Transaction.FocusID || actual.FocusID != fact.FocusID ||
		!sameCoverageWorkItemIDs(actual.CoverageWorkItemIDs, fact.CoverageWorkItemIDs) ||
		!sameCoverageWorkItemIDs(actual.CoverageWorkItemIDs, dispatch.Candidate.Assessment.CoverageWorkItemIDs) ||
		fact.Disposition != actual.Disposition {
		return fmt.Errorf("runtime assessment dispatch %q committed result identity mismatch", dispatch.ID)
	}
	for _, proof := range actual.Proofs {
		var referenced EvidenceFact
		matches := 0
		for _, original := range facts.Evidence.Accepted {
			if original.ID == proof.EvidenceFactID {
				referenced, matches = original, matches+1
			}
		}
		if matches != 1 || referenced.Kind != proof.EvidenceKind ||
			!coverageWorkItemIDInAssessment(actual.CoverageWorkItemIDs, proof.CoverageWorkItemID) ||
			!coverageWorkItemIDInAssessment(referenced.CoverageWorkItemIDs, proof.CoverageWorkItemID) ||
			StableRuntimeEvidenceFactHash(referenced) != proof.EvidenceFactHash ||
			!reflect.DeepEqual(referenced.OwnerRevisionSet, proof.OwnerRevisionSet) ||
			!reflect.DeepEqual(referenced.ApplicabilityDependencies, proof.ApplicabilityDependencies) ||
			EvidenceRevisionAfter(referenced.Revision, dispatch.Transaction.EvidenceRevision) {
			return fmt.Errorf("runtime assessment dispatch %q committed proof reference integrity mismatch", dispatch.ID)
		}
	}
	// Catalog snapshots are immutable inputs of this result. Check their exact
	// source identities; do not ask current resources to re-establish an old
	// postcondition or reclassify its historical absence.
	if len(actual.OwnerFactSources) != len(authority.Focus.RequiredOwnerFacts) {
		return fmt.Errorf("runtime assessment dispatch %q committed owner-fact source membership mismatch", dispatch.ID)
	}
	for _, referenced := range actual.OwnerFactSources {
		declared := false
		for _, requirement := range authority.Focus.RequiredOwnerFacts {
			declared = declared || requirement == referenced.Requirement
		}
		if !declared {
			return fmt.Errorf("runtime assessment dispatch %q committed owner-fact requirement mismatch", dispatch.ID)
		}
		if referenced.CatalogSnapshotID == "" {
			continue
		}
		matches := 0
		for _, catalog := range authority.OperationAdmissibilities {
			if catalog.SnapshotID == referenced.CatalogSnapshotID && catalog.OwnerModuleID == referenced.Requirement.OwnerModuleID {
				matches++
			}
		}
		if matches != 1 || referenced.OwnerRevisionSet == nil || referenced.OwnerRevisionSet.DefinitionRevision != facts.DefinitionRevision {
			return fmt.Errorf("runtime assessment dispatch %q committed catalog source integrity mismatch", dispatch.ID)
		}
		selector, err := runtimeAssessmentOwnerFactSelectorHash(string(facts.WorkspaceID), referenced.Requirement)
		if err != nil {
			return err
		}
		entry := referenced.OwnerRevisionSet.Entries[0]
		if entry.OwnerKind != "capability" || entry.OwnerInstanceID != referenced.Requirement.OwnerModuleID+"/"+string(facts.WorkspaceID) ||
			entry.ResourceKind != "owner_fact_requirement" || entry.ResourceID != selector || entry.DependencyHash != selector {
			return fmt.Errorf("runtime assessment dispatch %q committed catalog source identity mismatch", dispatch.ID)
		}
	}
	return nil
}

// ValidateRuntimeAssessmentAdjudications enforces the completed-dispatch side
// of the assessment double entry after the global evidence ledger has proved
// the one-to-one fact/dispatch chronology.
func ValidateRuntimeAssessmentAdjudications(facts RuntimeFacts, completed []RuntimeDispatchRecord) error {
	return runtimeProcessingRegion("assessment_adjudications", func() error {
		return validateRuntimeCommittedAssessmentResults(facts, completed)
	})
}

func validateRuntimeCommittedAssessmentResults(facts RuntimeFacts, completed []RuntimeDispatchRecord) error {
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
		if err := ValidateRuntimeAssessmentAdjudication(dispatch, fact, facts); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRuntimeAssessmentAuthoritySources roots every persisted assessment
// dispatch (including the still-active pre-commit dispatch) in DefinitionSeed.
// The dispatch-local source copy therefore cannot be coordinated with a
// mutable DefinitionFocus projection to manufacture different assessment
// authority across restart.
func ValidateRuntimeAssessmentAuthoritySources(facts RuntimeFacts, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord) error {
	validate := func(dispatch RuntimeDispatchRecord) error {
		if dispatch.Candidate.Assessment == nil {
			return nil
		}
		if dispatch.AssessmentAuthorityInput == nil {
			return fmt.Errorf("runtime assessment dispatch %q has no authority source", dispatch.ID)
		}
		candidate := dispatch.Candidate.Assessment.Normalized()
		source, err := RuntimeAssessmentSourceProjection(facts, dispatch.Transaction.FocusID, candidate.CoverageWorkItemIDs)
		if err != nil {
			return err
		}
		authority := dispatch.AssessmentAuthorityInput.Normalized()
		// Evidence revision is not a temporal cutoff: an invalidated assessment
		// can be followed by new observations without a new evidence fact. Match
		// the original append-only prefix and its independently sealed semantic
		// authority instead of pulling future observations into historical proof.
		count := len(authority.ApplicabilityObservations)
		if count > len(facts.EvidenceApplicabilityObservations) {
			return fmt.Errorf("runtime assessment dispatch %q applicability prefix is missing", dispatch.ID)
		}
		var originalObservations []RuntimeEvidenceApplicabilityObservation
		if count > 0 {
			originalObservations = facts.EvidenceApplicabilityObservations[:count]
		}
		if !reflect.DeepEqual(authority.ApplicabilityObservations, originalObservations) {
			return fmt.Errorf("runtime assessment dispatch %q applicability prefix does not match original SM-2 observations", dispatch.ID)
		}
		if StableRuntimeEvidenceApplicabilitySemanticHash(originalObservations) != authority.AuthorityInput.ApplicabilityObservationHash {
			return fmt.Errorf("runtime assessment dispatch %q applicability authority identity mismatch", dispatch.ID)
		}
		if authority.SourceSliceID != source.SliceID || authority.SourceScope != source.AssessmentScope ||
			authority.SourceID != source.AssessmentID || authority.SourceText != source.AssessmentText {
			return fmt.Errorf("runtime assessment dispatch %q authority source does not match definition seed", dispatch.ID)
		}
		return nil
	}
	if active != nil {
		if err := validate(*active); err != nil {
			return err
		}
	}
	for _, dispatch := range completed {
		if err := validate(dispatch); err != nil {
			return err
		}
	}
	return nil
}

// The generation remains an original read fence after Resume. It must resolve
// in the same immutable reconciliation ledger, never in caller-supplied state.
func ValidateRuntimeAssessmentApplicabilityGenerations(ledger RuntimeCWIReconciliationLedger, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord) error {
	dispatches := append([]RuntimeDispatchRecord(nil), completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	for _, dispatch := range dispatches {
		if dispatch.AssessmentAuthorityInput == nil {
			continue
		}
		hash := dispatch.AssessmentAuthorityInput.ApplicabilityReconciliationRequestHash
		if hash == "" {
			continue // Original pre-Resume generation; not currentness by itself.
		}
		found := ledger.Request != nil && ledger.RequestHash == hash
		for _, historical := range ledger.History {
			found = found || historical.RequestHash == hash
		}
		if !found {
			return fmt.Errorf("assessment applicability has no original reconciliation generation")
		}
	}
	return nil
}

func assessmentBindingForCoverage(values []CoverageEvidenceBinding, coverageID common.CoverageWorkItemID) (CoverageEvidenceBinding, bool) {
	var ret CoverageEvidenceBinding
	found := false
	for _, value := range NormalizeCoverageEvidenceBindings(values) {
		if value.CoverageWorkItemID != coverageID {
			continue
		}
		if found {
			return CoverageEvidenceBinding{}, false
		}
		ret, found = value, true
	}
	return ret, found
}

func deriveAssessmentCoverageProofs(
	assessmentDispatch RuntimeDispatchRecord,
	purpose RuntimeAssessmentPurpose,
	focus CoverageFocus,
	binding CoverageEvidenceBinding,
	accepted []EvidenceFact,
	completed []RuntimeDispatchRecord,
	lineage []RuntimeCoverageProofLineage,
) ([]RuntimeAssessmentAdjudicationProof, bool) {
	byKind := make(map[string][]EvidenceFact)
	for _, fact := range normalizeEvidenceFacts(accepted) {
		if fact.Revision > assessmentDispatch.Transaction.EvidenceRevision || assessmentEvidenceWeak(fact) ||
			!RuntimeEvidenceHasFreshProofApplicability(fact, assessmentDispatch.AssessmentAuthorityInput.ApplicabilityObservations, assessmentDispatch.Transaction.EvidenceRevision, assessmentDispatch.AssessmentAuthorityInput.ApplicabilityReconciliationRequestHash) ||
			!coverageWorkItemIDInAssessment(fact.CoverageWorkItemIDs, binding.CoverageWorkItemID) {
			continue
		}
		if !assessmentEvidenceMatchesBinding(fact, binding, lineage) {
			continue
		}
		operation, found := assessmentOperationForEvidence(completed, fact)
		if !found || !assessmentOperationAllowedForFocus(focus, operation) {
			continue
		}
		byKind[fact.Kind] = append(byKind[fact.Kind], fact)
	}
	selected := make([]EvidenceFact, 0)
	for _, kind := range binding.EvidenceKinds() {
		eligible := byKind[kind]
		if len(eligible) == 0 {
			return nil, false
		}
		if len(binding.ProspectiveTargetRefs) > 1 {
			var chosen []EvidenceFact
			byDispatch := make(map[string][]EvidenceFact)
			for _, fact := range eligible {
				byDispatch[fact.DispatchID] = append(byDispatch[fact.DispatchID], fact)
			}
			dispatchIDs := make([]string, 0, len(byDispatch))
			for id := range byDispatch {
				dispatchIDs = append(dispatchIDs, id)
			}
			sort.Strings(dispatchIDs)
			for _, id := range dispatchIDs {
				if assessmentProspectiveTargetsComplete(binding.ProspectiveTargetRefs, byDispatch[id]) {
					chosen = byDispatch[id]
					break
				}
			}
			if len(chosen) == 0 {
				return nil, false
			}
			selected = append(selected, chosen...)
		} else {
			selected = append(selected, eligible...)
		}
	}
	if RuntimeAssessmentRequiresExecutableBehaviorEvidence(purpose, focus) {
		executable := false
		for _, fact := range selected {
			if assessmentValidationShowsExecutableBehavior(binding, fact) {
				executable = true
				break
			}
		}
		if !executable {
			return nil, false
		}
	}
	if purpose == RuntimeAssessmentPurposeDefinitionProof && !RuntimeHistoricalEffectsHaveCurrentAcceptanceProof(selected) {
		return nil, false
	}
	proofs := make([]RuntimeAssessmentAdjudicationProof, 0, len(selected))
	for _, fact := range selected {
		proofs = append(proofs, RuntimeAssessmentAdjudicationProof{
			CoverageWorkItemID: binding.CoverageWorkItemID, EvidenceKind: fact.Kind,
			EvidenceFactID: fact.ID, EvidenceFactHash: StableRuntimeEvidenceFactHash(fact),
			OwnerRevisionSet:          CloneRuntimeOwnerRevisionSet(fact.OwnerRevisionSet),
			ApplicabilityDependencies: CloneRuntimeEvidenceApplicabilityDependencies(fact.ApplicabilityDependencies),
		})
	}
	return proofs, true
}

func assessmentEvidenceWeak(fact EvidenceFact) bool {
	quality := common.EvidenceQuality(strings.TrimSpace(string(fact.Quality)))
	return fact.ZeroWork || quality == common.EvidenceQualityInvalid || quality == common.EvidenceQualityInconclusive || quality == common.EvidenceQualitySkipped
}

func assessmentEvidenceMatchesBinding(fact EvidenceFact, binding CoverageEvidenceBinding, lineage []RuntimeCoverageProofLineage) bool {
	for _, ref := range fact.BindingRefs {
		if strings.TrimSpace(ref) == binding.ID {
			return true
		}
	}
	for _, source := range lineage {
		source = source.Normalized()
		if source.Kind != RuntimeCoverageProofLineageKindSource || source.SourceEvidenceID != fact.ID || source.CoverageWorkItemID != binding.CoverageWorkItemID || source.EvidenceKind != fact.Kind {
			continue
		}
		for _, resolution := range lineage {
			resolution = resolution.Normalized()
			if resolution.Kind == RuntimeCoverageProofLineageKindResolution && resolution.SourceLineageID == source.ID &&
				resolution.CurrentBindingRef == binding.ID && resolution.CurrentBindingHash == binding.Hash &&
				resolution.Disposition == RuntimeCoverageProofLineageDispositionEquivalent {
				return true
			}
		}
	}
	return false
}

func assessmentOperationForEvidence(completed []RuntimeDispatchRecord, fact EvidenceFact) (RuntimeOperation, bool) {
	for _, dispatch := range completed {
		if dispatch.ID != fact.DispatchID {
			continue
		}
		for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
			operation = operation.Normalized()
			if operation.ID == fact.OperationID {
				return operation, true
			}
		}
	}
	return RuntimeOperation{}, false
}

func assessmentOperationAllowedForFocus(focus CoverageFocus, operation RuntimeOperation) bool {
	kind := CanonicalRuntimeBlockKind(operation.Kind)
	for _, rejected := range focus.RejectedStepKinds {
		if CanonicalRuntimeBlockKind(rejected) == kind {
			return false
		}
	}
	if len(focus.AcceptedStepKinds) == 0 {
		return true
	}
	for _, accepted := range focus.AcceptedStepKinds {
		if CanonicalRuntimeBlockKind(accepted) == kind {
			return true
		}
	}
	return false
}

func assessmentValidationShowsExecutableBehavior(binding CoverageEvidenceBinding, fact EvidenceFact) bool {
	if fact.Validation == nil {
		return false
	}
	proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	if !found {
		return false
	}
	return RuntimeValidationObservationSatisfiesCoverageProof(proof, *fact.Validation)
}

func assessmentProspectiveTargetsComplete(required []string, facts []EvidenceFact) bool {
	seen := make(map[string]struct{})
	for _, fact := range facts {
		for _, ref := range fact.TargetRefs {
			seen[strings.TrimSpace(ref)] = struct{}{}
		}
	}
	for _, ref := range required {
		if _, found := seen[strings.TrimSpace(ref)]; !found {
			return false
		}
	}
	return true
}

func coverageWorkItemIDInAssessment(values []common.CoverageWorkItemID, expected common.CoverageWorkItemID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
