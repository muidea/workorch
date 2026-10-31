package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeValidationInputReviewProtocol          = "validation/input-consistency"
	RuntimeValidationInputConsumer                = "validation_input"
	RuntimeValidationInputReviewMaxFiles          = 16
	RuntimeValidationInputReviewMaxBytes          = 64 * 1024
	RuntimeValidationInputReviewMaxReasonBytes    = 2048
	RuntimeValidationInputReviewMaxScopeBytes     = 4096
	RuntimeValidationInputReviewMaxObligations    = 16
	RuntimeValidationInputReviewMaxReportBytes    = 32 * 1024
	RuntimeValidationInputReviewInformationAction = "provide_validation_input_facts"
	RuntimeValidationInputReviewFailureClass      = "validation_input_review"
)

// These are provider observations, never Runtime dispositions or transitions.
const (
	RuntimeValidationInputConsistent   = "consistent"
	RuntimeValidationInputConflict     = "conflict"
	RuntimeValidationInputInconclusive = "inconclusive"
	RuntimeValidationInputCovered      = "covered"
	RuntimeValidationInputMissing      = "missing"
	RuntimeValidationInputConflicting  = "conflicting"
	RuntimeValidationInputUnknown      = "unknown"
)

// RuntimeValidationInputReviewSubject is a bounded, exact owner observation.
// Its content is input to an independent review, not a provider-generated
// assertion of compliance. The enclosing owner binding seals the immutable
// requirement, native checks, recipe and catalog revision.
type RuntimeValidationInputReviewSubject struct {
	ProtocolRef string
	Hash        string
	Files       []RuntimeValidationInputReviewFile
	// Optional owner-authored limits on what these inputs execute/observe. A
	// reviewer must not infer a wider proof scope from recipe names or paths.
	ScopeContractRef            string                              `json:",omitempty"`
	ScopeRequirements           string                              `json:",omitempty"`
	LineageAssertions           []RuntimeLineageAssertion           `json:",omitempty"`
	DiagnosticSourceContractRef string                              `json:",omitempty"`
	DiagnosticSources           []RuntimeValidationDiagnosticSource `json:",omitempty"`
}

type RuntimeValidationInputReviewFile struct {
	TargetRef   string `json:"targetRef"`
	ContentHash string `json:"contentHash"`
	Content     string `json:"content"`
}

// RuntimeValidationInputReview is deliberately a closed per-check report.
// There is no overall approval bit, alternative criterion or replacement DoD.
type RuntimeValidationInputReview struct {
	SubjectHash       string                                    `json:"subjectHash"`
	Checks            []RuntimeValidationInputCheckReview       `json:"checks"`
	DiagnosticSources []RuntimeValidationDiagnosticSourceReview `json:"diagnosticSources,omitempty"`
}

type RuntimeValidationInputCheckReview struct {
	RequiredCheckRef string   `json:"requiredCheckRef"`
	Conclusion       string   `json:"conclusion"`
	Reason           string   `json:"reason"`
	InputTargetRefs  []string `json:"inputTargetRefs"`
	// Decompose the immutable criterion into exact excerpts and describe
	// input coverage. These are observations, not new acceptance criteria or a
	// repair authorization.
	Obligations []RuntimeValidationInputObligationReview `json:"obligations,omitempty"`
}

type RuntimeValidationInputObligationReview struct {
	CriterionText     string   `json:"criterionText"`
	Finding           string   `json:"finding"`
	Reason            string   `json:"reason"`
	InputTargetRefs   []string `json:"inputTargetRefs"`
	ProofConsumerRefs []string `json:"proofConsumerRefs,omitempty"`
}

// Information is a Decision-owner resolution fact, not a criterion, a
// consistency verdict or an execution grant. The next manifest may include
// it only for the same exact owner input subject.
type RuntimeValidationInputReviewInformation struct {
	SubjectHash  string              `json:"subjectHash"`
	ReviewRef    string              `json:"reviewRef"`
	TicketID     common.TicketID     `json:"ticketId"`
	ResolutionID common.ResolutionID `json:"resolutionId"`
	Summary      string              `json:"summary"`
}

func (i RuntimeValidationInputReviewInformation) Validate() error {
	if i.SubjectHash == "" || i.ReviewRef == "" || i.TicketID == "" || i.ResolutionID == "" ||
		i.Summary == "" || strings.TrimSpace(i.Summary) != i.Summary || !utf8.ValidString(i.Summary) || len(i.Summary) > RuntimeValidationInputReviewMaxReasonBytes {
		return fmt.Errorf("validation input review information requires exact source identity and bounded facts")
	}
	return nil
}

func RuntimeSemanticInputReviewInformationForCatalogs(values []RuntimeValidationInputReviewInformation, catalogs []RuntimeOperationAdmissibility) []RuntimeValidationInputReviewInformation {
	subjects := make(map[string]bool)
	for _, catalog := range catalogs {
		for _, surface := range catalog.ValidationSurfaces {
			for _, binding := range surface.ProofRequirementBindings {
				if binding.InputReview != nil {
					subjects[binding.InputReview.Hash] = true
				}
			}
		}
	}
	var ret []RuntimeValidationInputReviewInformation
	for _, value := range values {
		if subjects[value.SubjectHash] {
			ret = append(ret, value)
		}
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].SubjectHash < ret[j].SubjectHash })
	return ret
}

func validateRuntimeSemanticInputReviewInformation(m RuntimeSemanticPlanManifest) error {
	if len(m.InputReviewInformation) == 0 {
		return nil
	}
	if !reflect.DeepEqual(m.InputReviewInformation, RuntimeSemanticInputReviewInformationForCatalogs(m.InputReviewInformation, m.OwnerCatalogs)) {
		return fmt.Errorf("semantic input review information must bind current subjects in canonical order")
	}
	previous := ""
	for _, value := range m.InputReviewInformation {
		if err := value.Validate(); err != nil {
			return err
		}
		if value.SubjectHash <= previous {
			return fmt.Errorf("semantic input review permits only one information resolution per exact subject")
		}
		previous = value.SubjectHash
	}
	return nil
}

func writeRuntimeSemanticInputReviewInformationHash(h interface{ Write([]byte) (int, error) }, values []RuntimeValidationInputReviewInformation) {
	if len(values) == 0 {
		return
	}
	writeHashString(h, "runtime-input-review-information/v1")
	for _, value := range values {
		writeHashString(h, value.SubjectHash)
		writeHashString(h, value.ReviewRef)
		writeHashString(h, string(value.TicketID))
		writeHashString(h, string(value.ResolutionID))
		writeHashString(h, value.Summary)
	}
}

func (s RuntimeValidationInputReviewSubject) Clone() RuntimeValidationInputReviewSubject {
	s.Files = append([]RuntimeValidationInputReviewFile(nil), s.Files...)
	s.LineageAssertions = slices.Clone(s.LineageAssertions)
	s.DiagnosticSources = slices.Clone(s.DiagnosticSources)
	return s
}

func (r RuntimeValidationInputReview) Clone() RuntimeValidationInputReview {
	r.DiagnosticSources = slices.Clone(r.DiagnosticSources)
	for i := range r.DiagnosticSources {
		r.DiagnosticSources[i].InputTargetRefs = slices.Clone(r.DiagnosticSources[i].InputTargetRefs)
	}
	r.Checks = cloneRuntimeStateRawSlice(r.Checks, func(check RuntimeValidationInputCheckReview) RuntimeValidationInputCheckReview { return check })
	for i := range r.Checks {
		r.Checks[i].InputTargetRefs = cloneRuntimeStateRawSlice(r.Checks[i].InputTargetRefs, func(value string) string { return value })
		r.Checks[i].Obligations = cloneRuntimeStateRawSlice(r.Checks[i].Obligations, func(value RuntimeValidationInputObligationReview) RuntimeValidationInputObligationReview {
			return value
		})
		for j := range r.Checks[i].Obligations {
			r.Checks[i].Obligations[j].ProofConsumerRefs = slices.Clone(r.Checks[i].Obligations[j].ProofConsumerRefs)
			r.Checks[i].Obligations[j].InputTargetRefs = cloneRuntimeStateRawSlice(r.Checks[i].Obligations[j].InputTargetRefs, func(value string) string { return value })
		}
	}
	return r
}

func RuntimeValidationInputContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// StableRuntimeValidationInputReviewSubjectHash also binds the complete native
// mapping. Changing a required check, supplement, recipe or criterion identity
// invalidates a report even if every input byte stayed unchanged.
func StableRuntimeValidationInputReviewSubjectHash(s RuntimeValidationInputReviewSubject, b RuntimeValidationProofRequirementBinding) string {
	h := sha256.New()
	writeHashString(h, s.ProtocolRef)
	writeHashString(h, b.RequirementHash)
	writeHashString(h, b.DefinitionRevision)
	writeHashString(h, string(b.CoverageWorkItemID))
	writeHashString(h, b.DeclarationRef)
	writeHashString(h, b.ValidationRecipeID)
	writeHashStrings(h, b.RequiredCheckRefs)
	for _, check := range b.CheckAttestations {
		writeHashString(h, check.RequiredCheckRef)
		writeHashString(h, check.ObservationProtocolRef)
		writeHashString(h, check.NativeCheckRef)
		writeHashString(h, check.SupplementalNativeCheckRef)
		writeRuntimeValidationExecutionRequirementHash(h, check)
	}
	for _, file := range s.Files {
		writeHashString(h, file.TargetRef)
		writeHashString(h, file.ContentHash)
	}
	if s.ScopeContractRef != "" || s.ScopeRequirements != "" {
		writeHashString(h, "validation-input-review-scope/v1")
		writeHashStrings(h, []string{s.ScopeContractRef, s.ScopeRequirements})
	}
	if len(s.LineageAssertions) != 0 {
		payload, _ := json.Marshal(s.LineageAssertions)
		writeHashString(h, "validation-proof-composition/v1")
		writeHashString(h, string(payload))
	}
	writeRuntimeDiagnosticSourceSubjectHash(h, s)
	return "runtime-input-review-" + hex.EncodeToString(h.Sum(nil))
}

func (s RuntimeValidationInputReviewSubject) ValidateAgainstBinding(b RuntimeValidationProofRequirementBinding) error {
	if err := validateRuntimeDiagnosticSourceSubject(s); err != nil {
		return err
	}
	if err := validateRuntimeValidationCompositionSubject(s, b); err != nil {
		return err
	}
	maxFiles, maxBytes, _ := RuntimeValidationInputReviewBounds(s)
	if !RuntimeValidationInputReviewProtocolSupported(s.ProtocolRef) || len(s.Files) == 0 || len(s.Files) > maxFiles {
		return fmt.Errorf("validation input review requires a supported protocol and complete bounded inputs")
	}
	if (s.ScopeContractRef == "") != (s.ScopeRequirements == "") || len(s.ScopeContractRef) > 512 || len(s.ScopeRequirements) > RuntimeValidationInputReviewMaxScopeBytes ||
		strings.TrimSpace(s.ScopeContractRef) != s.ScopeContractRef || strings.TrimSpace(s.ScopeRequirements) != s.ScopeRequirements || !utf8.ValidString(s.ScopeContractRef+s.ScopeRequirements) ||
		strings.IndexFunc(s.ScopeContractRef+s.ScopeRequirements, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return fmt.Errorf("validation input review requires a complete bounded owner scope contract")
	}
	bytes := 0
	previous := ""
	for _, file := range s.Files {
		if !runtimeProspectiveTargetRefValid(file.TargetRef) || file.TargetRef != strings.TrimSpace(file.TargetRef) || file.TargetRef <= previous ||
			!utf8.ValidString(file.Content) || file.ContentHash != RuntimeValidationInputContentHash(file.Content) {
			return fmt.Errorf("validation input review requires canonical unique targets and exact complete content")
		}
		bytes += len(file.Content)
		if bytes > maxBytes {
			return fmt.Errorf("validation input review input capacity exceeded")
		}
		previous = file.TargetRef
	}
	if s.Hash != StableRuntimeValidationInputReviewSubjectHash(s, b) {
		return fmt.Errorf("validation input review subject does not bind the exact criterion and input identity")
	}
	return nil
}

func (r RuntimeValidationInputReview) ValidateAgainstBinding(b RuntimeValidationProofRequirementBinding) error {
	if b.InputReview == nil || r.SubjectHash != b.InputReview.Hash {
		return fmt.Errorf("validation input review has no exact owner subject")
	}
	if err := b.InputReview.ValidateAgainstBinding(b); err != nil {
		return err
	}
	if err := validateRuntimeDiagnosticSourceReview(r, *b.InputReview); err != nil {
		return err
	}
	if len(r.Checks) != len(b.RequiredCheckRefs) {
		return fmt.Errorf("validation input review must cover every original required check")
	}
	seen := make(map[string]bool, len(r.Checks))
	for _, check := range r.Checks {
		if !containsString(b.RequiredCheckRefs, check.RequiredCheckRef) || seen[check.RequiredCheckRef] ||
			check.Reason != strings.TrimSpace(check.Reason) || check.Reason == "" || !utf8.ValidString(check.Reason) ||
			len(check.Reason) > RuntimeValidationInputReviewMaxReasonBytes {
			return fmt.Errorf("validation input review has missing, duplicate or invalid per-check reasoning")
		}
		seen[check.RequiredCheckRef] = true
		switch check.Conclusion {
		case RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive:
		default:
			return fmt.Errorf("validation input review has an unsupported conclusion")
		}
		refs := normalizeStrings(check.InputTargetRefs)
		if len(refs) == 0 || !reflect.DeepEqual(refs, check.InputTargetRefs) {
			return fmt.Errorf("validation input review requires canonical exact input references for every check")
		}
		for _, ref := range refs {
			idx := sort.Search(len(b.InputReview.Files), func(i int) bool { return b.InputReview.Files[i].TargetRef >= ref })
			if idx == len(b.InputReview.Files) || b.InputReview.Files[idx].TargetRef != ref {
				return fmt.Errorf("validation input review references an unobserved input")
			}
		}
		if err := validateRuntimeValidationInputObligations(check); err != nil {
			return err
		}

		if err := validateRuntimeValidationCompositionCheck(check, *b.InputReview); err != nil {
			return err
		}
	}
	// Every current report observes the same explicit capacity bound.

	encoded, err := json.Marshal(r)
	_, _, maxReport := RuntimeValidationInputReviewBounds(*b.InputReview)
	if err != nil || len(encoded) > maxReport {
		return fmt.Errorf("validation input review report capacity exceeded")
	}

	return nil
}

// RuntimeValidationInputReviewConclusion only summarizes already validated
// facts. A conflict cannot be hidden by another check being inconclusive.
func RuntimeValidationInputReviewConclusion(r RuntimeValidationInputReview) string {
	ret := RuntimeValidationInputConsistent
	for _, source := range r.DiagnosticSources {
		if source.Conclusion == RuntimeValidationInputConflict {
			return RuntimeValidationInputConflict
		}
		if source.Conclusion != RuntimeValidationInputConsistent {
			ret = RuntimeValidationInputInconclusive
		}
	}
	for _, check := range r.Checks {
		if check.Conclusion == RuntimeValidationInputConflict {
			return RuntimeValidationInputConflict
		}
		if check.Conclusion != RuntimeValidationInputConsistent {
			ret = RuntimeValidationInputInconclusive
		}
	}
	if len(r.Checks) == 0 {
		return ""
	}
	return ret
}

func writeRuntimeValidationInputReviewHash(h interface{ Write([]byte) (int, error) }, r *RuntimeValidationInputReview) {
	if r == nil {
		return
	}
	writeHashString(h, RuntimeValidationInputReviewProtocol)
	writeHashString(h, r.SubjectHash)
	writeRuntimeDiagnosticSourceReviewHash(h, r.DiagnosticSources)
	for _, check := range r.Checks {
		writeHashString(h, check.RequiredCheckRef)
		writeHashString(h, check.Conclusion)
		writeHashString(h, check.Reason)
		writeHashStrings(h, check.InputTargetRefs)
		if len(check.Obligations) != 0 {
			writeHashString(h, "obligations")
			for _, obligation := range check.Obligations {
				writeHashString(h, obligation.CriterionText)
				writeHashString(h, obligation.Finding)
				writeHashString(h, obligation.Reason)
				writeHashStrings(h, obligation.InputTargetRefs)
				if len(obligation.ProofConsumerRefs) != 0 {
					writeHashString(h, "proof-consumers")
					writeHashStrings(h, obligation.ProofConsumerRefs)
				}
			}
		}
	}
}

func RuntimeSemanticValidationInputReviewBinding(p RuntimeSemanticProofProposal, catalogs []RuntimeOperationAdmissibility) (RuntimeValidationProofRequirementBinding, bool) {
	if !containsString(p.RequiredEvidenceKinds, EvidenceKindValidation) {
		return RuntimeValidationProofRequirementBinding{}, false
	}
	for _, catalog := range catalogs {
		if catalog.OwnerModuleID != p.OwnerModuleID {
			continue
		}
		for _, surface := range catalog.ValidationSurfaces {
			if surface.ID != p.ValidationRecipeID || surface.OperationKind != p.OperationKind {
				continue
			}
			for _, binding := range surface.ProofRequirementBindings {
				if binding.CoverageWorkItemID == p.CoverageWorkItemID && binding.DeclarationRef == p.DeclarationRef && binding.InputReview != nil {
					return binding.Clone(), true
				}
			}
		}
	}
	return RuntimeValidationProofRequirementBinding{}, false
}

func validateRuntimeSemanticInputReview(p RuntimeSemanticProofProposal, catalog RuntimeOperationAdmissibility) error {
	binding, required := RuntimeSemanticValidationInputReviewBinding(p, []RuntimeOperationAdmissibility{catalog})
	if !required {
		if p.ValidationInputReview != nil {
			return fmt.Errorf("semantic proposal cannot introduce an undeclared input review")
		}
		return nil
	}
	if p.ValidationInputReview == nil {
		return fmt.Errorf("semantic proposal requires an independent complete input review")
	}
	return p.ValidationInputReview.ValidateAgainstBinding(binding)
}

// ValidateRuntimeSemanticInputReviewConsistency is a binding guard, not
// document admission. Complete conflict/inconclusive reports remain valid
// observations, but neither can be compiled into proof authority.
func ValidateRuntimeSemanticInputReviewConsistency(d RuntimeSemanticPlanDocument) error {
	for _, proposal := range d.Proposals {
		if proposal.ValidationInputReview != nil && RuntimeValidationInputReviewConclusion(*proposal.ValidationInputReview) != RuntimeValidationInputConsistent {
			return fmt.Errorf("validation input consistency is not established for coverage %q", proposal.CoverageWorkItemID)
		}
	}
	return nil
}
