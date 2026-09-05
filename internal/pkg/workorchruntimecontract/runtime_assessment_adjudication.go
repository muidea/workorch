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
	return a
}

func (a RuntimeAssessmentAuthorityInput) Clone() RuntimeAssessmentAuthorityInput {
	a = a.Normalized()
	a.OperationAdmissibilities = CloneRuntimeOperationAdmissibilities(a.OperationAdmissibilities)
	return a
}

func (a RuntimeAssessmentAuthorityInput) Validate(transaction AuthorityTransaction, bindings []CoverageEvidenceBinding) error {
	if !reflect.DeepEqual(a, a.Normalized()) {
		return fmt.Errorf("runtime assessment authority input must be raw canonical")
	}
	a = a.Normalized()
	if transaction.State != ConvergenceStateNeedAssessment || a.Focus.ID == "" || a.Focus.ID != transaction.FocusID {
		return fmt.Errorf("runtime assessment authority input requires its assessment transaction focus")
	}
	if a.Purpose != RuntimeAssessmentPurposeDefinitionProof && a.Purpose != RuntimeAssessmentPurposeOwnerFactAcquisition {
		return fmt.Errorf("runtime assessment authority input has unsupported purpose %q", a.Purpose)
	}
	if a.SourceSliceID == "" || a.SourceScope == "" || a.SourceID == "" || a.SourceText == "" {
		return fmt.Errorf("runtime assessment authority input requires exact definition source")
	}
	if err := a.AuthorityInput.Validate(); err != nil || StableRuntimeEquivalentAuthorityInputID(a.AuthorityInput) != transaction.AuthorityInputID {
		return fmt.Errorf("runtime assessment authority input identity mismatch")
	}
	if StableRuntimeAuthorityFocusHash(a.Focus) != a.AuthorityInput.FocusHash ||
		StableRuntimeOperationAdmissibilityCatalogRevisionHash(a.OperationAdmissibilities) != a.AuthorityInput.CatalogRevisionHash {
		return fmt.Errorf("runtime assessment authority input preimage mismatch")
	}
	if !RuntimeCoverageEvidenceBindingProjectionsEqual(a.Focus.CoverageEvidenceBindings, bindings) {
		return fmt.Errorf("runtime assessment authority input binding preimage mismatch")
	}
	return nil
}

func StableRuntimeAssessmentAuthorityInputHash(value RuntimeAssessmentAuthorityInput) string {
	value = value.Normalized()
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(append([]byte("runtime-assessment-authority-input/v1\x00"), payload...))
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
}

func (p RuntimeAssessmentAdjudicationProof) Normalized() RuntimeAssessmentAdjudicationProof {
	p.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(p.CoverageWorkItemID)))
	p.EvidenceKind = strings.TrimSpace(p.EvidenceKind)
	p.EvidenceFactID = common.EvidenceRecordID(strings.TrimSpace(string(p.EvidenceFactID)))
	p.EvidenceFactHash = strings.TrimSpace(p.EvidenceFactHash)
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
	if !reflect.DeepEqual(a, a.Normalized()) {
		return fmt.Errorf("runtime assessment adjudication must be raw canonical")
	}
	a = a.Normalized()
	if a.DispatchID == "" || a.AuthorityInputID == "" || a.FocusID == "" || len(a.CoverageWorkItemIDs) == 0 {
		return fmt.Errorf("runtime assessment adjudication requires dispatch, authority, focus and coverage")
	}
	if a.Disposition != AssessmentDispositionSatisfied && a.Disposition != AssessmentDispositionRepairableGap {
		return fmt.Errorf("runtime assessment adjudication has non-canonical disposition %q", a.Disposition)
	}
	if a.Disposition == AssessmentDispositionSatisfied && len(a.Proofs) == 0 {
		return fmt.Errorf("satisfied runtime assessment adjudication requires proof")
	}
	for _, proof := range a.Proofs {
		if proof != proof.Normalized() || proof.CoverageWorkItemID == "" || proof.EvidenceKind == "" || proof.EvidenceFactID == "" || proof.EvidenceFactHash == "" {
			return fmt.Errorf("runtime assessment adjudication contains malformed proof")
		}
	}
	return nil
}

func StableRuntimeAssessmentAdjudicationHash(value RuntimeAssessmentAdjudication) string {
	value = value.Normalized()
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(append([]byte("runtime-assessment-adjudication/v1\x00"), payload...))
	return "assessment-adjudication-" + hex.EncodeToString(digest[:])
}

// DeriveRuntimeAssessmentAdjudication deterministically replays the canonical
// classification from the assessment dispatch's pre-seal input, the immutable
// completed-dispatch history, accepted receipt facts and proof lineage.
func DeriveRuntimeAssessmentAdjudication(
	dispatch RuntimeDispatchRecord,
	facts RuntimeFacts,
	completed []RuntimeDispatchRecord,
	lineage []RuntimeCoverageProofLineage,
) (RuntimeAssessmentAdjudication, error) {
	if dispatch.Candidate.Assessment == nil || dispatch.AssessmentAuthorityInput == nil {
		return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment adjudication requires assessment dispatch authority")
	}
	if err := dispatch.AssessmentAuthorityInput.Validate(dispatch.Transaction, dispatch.CoverageBindings); err != nil {
		return RuntimeAssessmentAdjudication{}, err
	}
	candidate := dispatch.Candidate.Assessment.Normalized()
	authority := dispatch.AssessmentAuthorityInput.Normalized()
	result := RuntimeAssessmentAdjudication{
		DispatchID: dispatch.ID, AuthorityInputID: dispatch.Transaction.AuthorityInputID,
		FocusID: dispatch.Transaction.FocusID, CoverageWorkItemIDs: candidate.CoverageWorkItemIDs,
		Disposition: AssessmentDispositionSatisfied,
	}
	// The assessment dispatch is sealed after the operation receipt and any
	// owner-fact re-observation. Its catalog snapshot is therefore the immutable
	// answer to whether the required postcondition was present at assessment
	// admission. Acquisition purpose does not waive that postcondition: doing so
	// would let the same accepted operation be classified satisfied while the
	// sealed owner snapshot still proves the required fact absent.
	if len(RuntimeMissingOwnerFactRequirements(authority.Focus.RequiredOwnerFacts, authority.OperationAdmissibilities)) != 0 {
		result.Disposition = AssessmentDispositionRepairableGap
	}
	for _, coverageID := range candidate.CoverageWorkItemIDs {
		binding, found := assessmentBindingForCoverage(authority.Focus.CoverageEvidenceBindings, coverageID)
		if !found {
			result.Disposition = AssessmentDispositionRepairableGap
			continue
		}
		coverageProofs, complete := deriveAssessmentCoverageProofs(dispatch, authority.Purpose, authority.Focus, binding, facts.Evidence.Accepted, completed, lineage)
		result.Proofs = append(result.Proofs, coverageProofs...)
		if !complete {
			result.Disposition = AssessmentDispositionRepairableGap
		}
	}
	return result.Normalized(), nil
}

func ValidateRuntimeAssessmentAdjudication(
	dispatch RuntimeDispatchRecord,
	fact RuntimeAssessmentFact,
	facts RuntimeFacts,
	completed []RuntimeDispatchRecord,
	lineage []RuntimeCoverageProofLineage,
) error {
	if dispatch.AssessmentAdjudication == nil || dispatch.AssessmentAdjudicationHash == "" || fact.AdjudicationHash == "" {
		return fmt.Errorf("committed runtime assessment dispatch %q requires double-entry adjudication", dispatch.ID)
	}
	actual := dispatch.AssessmentAdjudication.Normalized()
	if err := dispatch.AssessmentAdjudication.ValidateRaw(); err != nil {
		return err
	}
	if dispatch.AssessmentAdjudicationHash != StableRuntimeAssessmentAdjudicationHash(actual) || fact.AdjudicationHash != dispatch.AssessmentAdjudicationHash {
		return fmt.Errorf("runtime assessment dispatch %q adjudication hash mismatch", dispatch.ID)
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
	expected, err := DeriveRuntimeAssessmentAdjudication(dispatch, facts, completed, lineage)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) || fact.Disposition != expected.Disposition {
		return fmt.Errorf("runtime assessment dispatch %q adjudication does not match immutable proof history", dispatch.ID)
	}
	return nil
}

// ValidateRuntimeAssessmentAdjudications enforces the completed-dispatch side
// of the assessment double entry after the global evidence ledger has proved
// the one-to-one fact/dispatch chronology.
func ValidateRuntimeAssessmentAdjudications(facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage) error {
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
		if err := ValidateRuntimeAssessmentAdjudication(dispatch, fact, facts, completed, lineage); err != nil {
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
	proofs := make([]RuntimeAssessmentAdjudicationProof, 0, len(selected))
	for _, fact := range selected {
		proofs = append(proofs, RuntimeAssessmentAdjudicationProof{
			CoverageWorkItemID: binding.CoverageWorkItemID, EvidenceKind: fact.Kind,
			EvidenceFactID: fact.ID, EvidenceFactHash: StableRuntimeEvidenceFactHash(fact),
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
