package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
)

const GoalSpecificationVersionCurrent uint32 = 1
const (
	GoalRequirementUserExplicit        = "USER_EXPLICIT"
	GoalRequirementTemplateRequired    = "TEMPLATE_REQUIRED"
	GoalRequirementTemplateConditional = "TEMPLATE_CONDITIONAL"
	GoalRequirementDerived             = "DERIVED"
	GoalRequirementDiscovered          = "DISCOVERED"
	GoalReviewDimensionCoverage        = "dimension_coverage"
	GoalReviewNegativePath             = "negative_path"
	GoalReviewRegression               = "regression"
	GoalReviewResourceEnvironment      = "resource_environment"
	GoalReviewTraceability             = "traceability"
	GoalReviewVerifierFeasibility      = "verifier_feasibility"
	GoalReviewPass                     = "PASS"
	GoalReviewFail                     = "FAIL"
	GoalReviewUnknown                  = "UNKNOWN"
	GoalVerificationDeclared           = "DECLARED"
	GoalVerificationUnverified         = "UNVERIFIED"
)

// GoalSpecification is an optional, versioned Workspace admission proposal, not
// a Runtime authority or a proof of natural-language semantic completeness.
// Reviews report externally authored judgments; static validation checks their
// structure, provenance, exact identity and complete required review scope.
type GoalSpecification struct {
	Version                 uint32
	GoalItemRef             string
	GoalTextHash            string
	Resources               []GoalSpecificationResource
	Dimensions              []GoalSpecificationDimension
	Requirements            []GoalSpecificationRequirement
	Obligations             []GoalSpecificationObligation
	Verifications           []GoalSpecificationVerification
	Reviews                 []GoalSpecificationCompletenessReview
	ExecutionProofMappings  []GoalExecutionProofMapping  `json:",omitempty"`
	EvidencePurposeMappings []GoalEvidencePurposeMapping `json:",omitempty"`
}

// Candidate refs, not provider-chosen Runtime CWI IDs. Confirmation seals the
// exact mapping together with the original specification and all six reviews.
type GoalExecutionProofMapping struct {
	ContractRef      string
	ConsumerItemRef  string
	RuleRef          string
	SourceItemRef    string
	ConsumerGoalText string `json:",omitempty"`
}

// The original historical criterion and its unique ordinary acceptance source
// are confirmed together. This value does not supply evidence or CWI credit.
type GoalEvidencePurposeMapping struct {
	ContractRef             string
	HistoricalItemRef       string
	AcceptanceItemRef       string
	HistoricalProfileID     string `json:",omitempty"`
	HistoricalDeliverableID string `json:",omitempty"`
	HistoricalProfileHash   string `json:",omitempty"`
	HistoricalText          string `json:",omitempty"`
}

type GoalSpecificationResource struct {
	ID           string
	Ref          string
	SnapshotHash string
	Required     bool
	Available    bool
}

type GoalSpecificationDimension struct {
	ID          string
	Required    bool
	Activated   bool
	ResourceIDs []string
}

type GoalSpecificationRequirement struct {
	ID             string
	DimensionID    string
	Required       bool
	SourceKind     string
	SourceItemRefs []string
	ResourceIDs    []string
	Text           string
}

type GoalSpecificationObligation struct {
	ID            string
	RequirementID string
	DoDItemRefs   []string
	Text          string
}

// DECLARED binds an exact existing Profile evidence declaration. UNVERIFIED
// explicitly records that the proposal has no such binding; free text alone
// cannot become proof of capability availability or runtime feasibility.
type GoalSpecificationVerification struct {
	ID                    string
	ObligationID          string
	Declaration           string
	Status                string
	ProfileID             string
	EvidenceRequirementID string
	Scope                 string
}

type GoalSpecificationCompletenessReview struct {
	Kind              string
	Status            string
	SpecificationHash string
	Reason            string
	EvidenceRefs      []string
	RequirementIDs    []string
}

var goalReviewKinds = []string{GoalReviewDimensionCoverage, GoalReviewNegativePath, GoalReviewRegression, GoalReviewResourceEnvironment, GoalReviewTraceability, GoalReviewVerifierFeasibility}

func (s GoalSpecification) Clone() GoalSpecification {
	s.EvidencePurposeMappings = slices.Clone(s.EvidencePurposeMappings)
	s.ExecutionProofMappings = slices.Clone(s.ExecutionProofMappings)
	s.Resources = slices.Clone(s.Resources)
	s.Dimensions = slices.Clone(s.Dimensions)
	for i := range s.Dimensions {
		s.Dimensions[i].ResourceIDs = slices.Clone(s.Dimensions[i].ResourceIDs)
	}
	s.Requirements = slices.Clone(s.Requirements)
	for i := range s.Requirements {
		s.Requirements[i].SourceItemRefs = slices.Clone(s.Requirements[i].SourceItemRefs)
		s.Requirements[i].ResourceIDs = slices.Clone(s.Requirements[i].ResourceIDs)
	}
	s.Obligations = slices.Clone(s.Obligations)
	for i := range s.Obligations {
		s.Obligations[i].DoDItemRefs = slices.Clone(s.Obligations[i].DoDItemRefs)
	}
	s.Verifications = slices.Clone(s.Verifications)
	s.Reviews = slices.Clone(s.Reviews)
	for i := range s.Reviews {
		s.Reviews[i].EvidenceRefs = slices.Clone(s.Reviews[i].EvidenceRefs)
		s.Reviews[i].RequirementIDs = slices.Clone(s.Reviews[i].RequirementIDs)
	}
	return s
}

func (s GoalSpecification) Normalized() GoalSpecification {
	s = s.Clone()
	s.GoalItemRef = strings.TrimSpace(s.GoalItemRef)
	s.GoalTextHash = strings.TrimSpace(s.GoalTextHash)
	for i := range s.Resources {
		r := &s.Resources[i]
		r.ID = strings.TrimSpace(r.ID)
		r.Ref = strings.TrimSpace(r.Ref)
		r.SnapshotHash = strings.TrimSpace(r.SnapshotHash)
	}
	sort.Slice(s.Resources, func(i, j int) bool { return s.Resources[i].ID < s.Resources[j].ID })
	for i := range s.Dimensions {
		d := &s.Dimensions[i]
		d.ID = strings.TrimSpace(d.ID)
		d.ResourceIDs = normalizedSpecRefs(d.ResourceIDs)
	}
	sort.Slice(s.Dimensions, func(i, j int) bool { return s.Dimensions[i].ID < s.Dimensions[j].ID })
	for i := range s.Requirements {
		r := &s.Requirements[i]
		r.ID = strings.TrimSpace(r.ID)
		r.DimensionID = strings.TrimSpace(r.DimensionID)
		r.SourceKind = strings.TrimSpace(r.SourceKind)
		r.SourceItemRefs = normalizedSpecRefs(r.SourceItemRefs)
		r.ResourceIDs = normalizedSpecRefs(r.ResourceIDs)
		r.Text = strings.TrimSpace(r.Text)
	}
	sort.Slice(s.Requirements, func(i, j int) bool { return s.Requirements[i].ID < s.Requirements[j].ID })
	for i := range s.Obligations {
		o := &s.Obligations[i]
		o.ID = strings.TrimSpace(o.ID)
		o.RequirementID = strings.TrimSpace(o.RequirementID)
		o.DoDItemRefs = normalizedSpecRefs(o.DoDItemRefs)
		o.Text = strings.TrimSpace(o.Text)
	}
	sort.Slice(s.Obligations, func(i, j int) bool { return s.Obligations[i].ID < s.Obligations[j].ID })
	for i := range s.Verifications {
		v := &s.Verifications[i]
		v.ID = strings.TrimSpace(v.ID)
		v.ObligationID = strings.TrimSpace(v.ObligationID)
		v.Declaration = strings.TrimSpace(v.Declaration)
		v.Status = strings.TrimSpace(v.Status)
		v.ProfileID = strings.TrimSpace(v.ProfileID)
		v.EvidenceRequirementID = strings.TrimSpace(v.EvidenceRequirementID)
		v.Scope = strings.TrimSpace(v.Scope)
	}
	sort.Slice(s.Verifications, func(i, j int) bool { return s.Verifications[i].ID < s.Verifications[j].ID })
	for i := range s.Reviews {
		r := &s.Reviews[i]
		r.Kind = strings.TrimSpace(r.Kind)
		r.Status = strings.TrimSpace(r.Status)
		r.SpecificationHash = strings.TrimSpace(r.SpecificationHash)
		r.Reason = strings.TrimSpace(r.Reason)
		r.EvidenceRefs = normalizedSpecRefs(r.EvidenceRefs)
		r.RequirementIDs = normalizedSpecRefs(r.RequirementIDs)
	}
	sort.Slice(s.Reviews, func(i, j int) bool { return s.Reviews[i].Kind < s.Reviews[j].Kind })
	sort.Slice(s.ExecutionProofMappings, func(i, j int) bool {
		left, right := s.ExecutionProofMappings[i], s.ExecutionProofMappings[j]
		if left.ConsumerItemRef != right.ConsumerItemRef {
			return left.ConsumerItemRef < right.ConsumerItemRef
		}
		return left.RuleRef < right.RuleRef
	})
	sort.Slice(s.EvidencePurposeMappings, func(i, j int) bool {
		return s.EvidencePurposeMappings[i].HistoricalItemRef < s.EvidencePurposeMappings[j].HistoricalItemRef
	})
	return s
}

// Preserve cardinality, including duplicates, for validation; never silently
// erase missing or repeated identities while normalizing.
func normalizedSpecRefs(values []string) []string {
	result := slices.Clone(values)
	for i := range result {
		result[i] = strings.TrimSpace(result[i])
	}
	sort.Strings(result)
	return result
}

func specHashFields(h definitionHashWriter, values ...string) {
	writeDefinitionHash(h, fmt.Sprintf("%d", len(values)))
	writeDefinitionHash(h, values...)
}

// ContentHash excludes Reviews to avoid a self-referential review hash.
func (s GoalSpecification) ContentHash() string {
	s = s.Normalized()
	h := sha256.New()
	specHashFields(h, fmt.Sprintf("%d", s.Version), s.GoalItemRef, s.GoalTextHash)
	specHashFields(h, fmt.Sprintf("%d", len(s.Resources)))
	for _, r := range s.Resources {
		specHashFields(h, r.ID, r.Ref, r.SnapshotHash, fmt.Sprintf("%t", r.Required), fmt.Sprintf("%t", r.Available))
	}
	specHashFields(h, fmt.Sprintf("%d", len(s.Dimensions)))
	for _, d := range s.Dimensions {
		specHashFields(h, d.ID, fmt.Sprintf("%t", d.Required), fmt.Sprintf("%t", d.Activated))
		specHashFields(h, d.ResourceIDs...)
	}
	specHashFields(h, fmt.Sprintf("%d", len(s.Requirements)))
	for _, r := range s.Requirements {
		specHashFields(h, r.ID, r.DimensionID, fmt.Sprintf("%t", r.Required), r.SourceKind, r.Text)
		specHashFields(h, r.SourceItemRefs...)
		specHashFields(h, r.ResourceIDs...)
	}
	specHashFields(h, fmt.Sprintf("%d", len(s.Obligations)))
	for _, o := range s.Obligations {
		specHashFields(h, o.ID, o.RequirementID, o.Text)
		specHashFields(h, o.DoDItemRefs...)
	}
	specHashFields(h, fmt.Sprintf("%d", len(s.Verifications)))
	for _, v := range s.Verifications {
		specHashFields(h, v.ID, v.ObligationID, v.Declaration, v.Status, v.ProfileID, v.EvidenceRequirementID, v.Scope)
	}
	// Empty opt-in extension preserves all existing specification hashes.
	if len(s.ExecutionProofMappings) != 0 {
		specHashFields(h, "runtime/dod-source-execution-proof", fmt.Sprint(len(s.ExecutionProofMappings)))
		for _, mapping := range s.ExecutionProofMappings {
			specHashFields(h, mapping.ContractRef, mapping.ConsumerItemRef, mapping.RuleRef, mapping.SourceItemRef)
			if mapping.ConsumerGoalText != "" {
				specHashFields(h, "runtime/goal-source-execution-proof", mapping.ConsumerGoalText)
			}
		}
	}
	if len(s.EvidencePurposeMappings) != 0 {
		specHashFields(h, "runtime/cross-cwi-evidence-purpose", fmt.Sprint(len(s.EvidencePurposeMappings)))
		for _, mapping := range s.EvidencePurposeMappings {
			specHashFields(h, mapping.ContractRef, mapping.HistoricalItemRef, mapping.AcceptanceItemRef)
			if mapping.HistoricalProfileID != "" || mapping.HistoricalDeliverableID != "" || mapping.HistoricalProfileHash != "" || mapping.HistoricalText != "" {
				specHashFields(h, "profile-deliverable-source/v2", mapping.HistoricalProfileID, mapping.HistoricalDeliverableID, mapping.HistoricalProfileHash, mapping.HistoricalText)
			}
		}
	}
	return "goal-specification-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// AdmissionHash seals both the exact specification content and its independent
// review judgments. It is appended only when Specification is non-nil.
func (s GoalSpecification) AdmissionHash() string {
	s = s.Normalized()
	h := sha256.New()
	specHashFields(h, s.ContentHash(), fmt.Sprintf("%d", len(s.Reviews)))
	for _, r := range s.Reviews {
		specHashFields(h, r.Kind, r.Status, r.SpecificationHash, r.Reason)
		specHashFields(h, r.EvidenceRefs...)
		specHashFields(h, r.RequirementIDs...)
	}
	return "goal-specification-admission-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Validate checks bounded identities and structural traceability only. It does
// not infer whether the original natural-language Goal is fully represented.
func (s GoalSpecification) Validate(goal DefinitionCandidateItem, dod []DefinitionCandidateItem) error {
	if err := validateGoalEvidencePurposeMappings(s.EvidencePurposeMappings, dod); err != nil {
		return err
	}
	s = s.Normalized()
	if s.Version != GoalSpecificationVersionCurrent || s.GoalItemRef != goal.CandidateItemRef || s.GoalTextHash != StableDefinitionTextHash(goal.Text) {
		return fmt.Errorf("goal specification version or exact Goal binding mismatch")
	}
	if len(s.Resources) > 128 || len(s.Dimensions) > 64 || len(s.Requirements) > 256 || len(s.Obligations) > 512 || len(s.Verifications) > 512 || len(s.Reviews) != len(goalReviewKinds) {
		return fmt.Errorf("goal specification exceeds bounds or lacks required declarations/reviews")
	}
	items := map[string]bool{goal.CandidateItemRef: true}
	dodRefs := map[string]bool{}
	for _, item := range dod {
		items[item.CandidateItemRef] = true
		dodRefs[item.CandidateItemRef] = true
	}
	if len(s.ExecutionProofMappings) > 512 {
		return fmt.Errorf("execution proof mappings exceed bound")
	}
	proofKeys := map[string]bool{}
	goalProofSource := ""
	for _, mapping := range s.ExecutionProofMappings {
		key := mapping.ConsumerItemRef + "\x00" + mapping.RuleRef
		validConsumer := mapping.ContractRef == "runtime/dod-source-execution-proof" && dodRefs[mapping.ConsumerItemRef] && mapping.ConsumerGoalText == ""
		validConsumer = validConsumer || (mapping.ContractRef == "runtime/goal-source-execution-proof" && mapping.ConsumerItemRef == goal.CandidateItemRef && mapping.ConsumerGoalText == goal.Text && strings.TrimSpace(goal.Text) != "" && len(goal.Text) <= 4096)
		if !validConsumer || !dodRefs[mapping.SourceItemRef] || mapping.ConsumerItemRef == mapping.SourceItemRef || mapping.RuleRef == "" || strings.TrimSpace(mapping.RuleRef) != mapping.RuleRef || len(mapping.RuleRef) > 512 || strings.ContainsRune(mapping.RuleRef, 0) || proofKeys[key] {
			return fmt.Errorf("execution proof mapping requires unique exact original consumer/rule/DoD source")
		}
		proofKeys[key] = true
		if mapping.ContractRef == "runtime/goal-source-execution-proof" {
			if goalProofSource != "" && goalProofSource != mapping.SourceItemRef {
				return fmt.Errorf("Goal mappings have ambiguous original execution sources")
			}
			goalProofSource = mapping.SourceItemRef
		}
	}
	// Review evidence refs are unqualified IDs. They must resolve to exactly
	// one candidate item or specification object, never an ambiguous namespace.
	identities := map[string]bool{}
	for id := range items {
		identities[id] = true
	}
	claimID := func(id string) bool {
		if id == "" || identities[id] {
			return false
		}
		identities[id] = true
		return true
	}
	resources := map[string]GoalSpecificationResource{}
	for _, r := range s.Resources {
		if !claimID(r.ID) || r.Ref == "" || len(r.Ref) > 1024 || len(r.SnapshotHash) > 256 || len(r.ID) > 128 {
			return fmt.Errorf("goal specification resource is incomplete or duplicated: %q", r.ID)
		}
		resources[r.ID] = r
	}
	dimensions := map[string]GoalSpecificationDimension{}
	for _, d := range s.Dimensions {
		if !claimID(d.ID) || len(d.ID) > 128 || !specRefsExist(d.ResourceIDs, func(ref string) bool { _, ok := resources[ref]; return ok }) {
			return fmt.Errorf("goal specification dimension is invalid: %q", d.ID)
		}
		dimensions[d.ID] = d
	}
	requirements := map[string]GoalSpecificationRequirement{}
	for _, r := range s.Requirements {
		_, hasDimension := dimensions[r.DimensionID]
		textLimit := 4096
		// 独立 Goal 保留输入的既有字节边界；分组、改写或重新标记的文本不能借用该边界。
		if r.Required && r.SourceKind == GoalRequirementUserExplicit && slices.Equal(r.SourceItemRefs, []string{goal.CandidateItemRef}) && r.Text == goal.Text {
			textLimit = 32 * 1024
		}
		if !claimID(r.ID) || len(r.ID) > 128 || !hasDimension || len(r.Text) > textLimit || len(r.SourceItemRefs) > 32 || len(r.ResourceIDs) > 128 || !specRefsExist(r.SourceItemRefs, func(ref string) bool { return items[ref] }) || !specRefsExist(r.ResourceIDs, func(ref string) bool { _, ok := resources[ref]; return ok }) {
			return fmt.Errorf("goal specification requirement is incomplete: %q", r.ID)
		}
		switch r.SourceKind {
		case GoalRequirementUserExplicit, GoalRequirementTemplateRequired, GoalRequirementTemplateConditional, GoalRequirementDerived, GoalRequirementDiscovered:
		default:
			return fmt.Errorf("goal specification requirement %q has unknown source kind", r.ID)
		}
		requirements[r.ID] = r
	}
	obligations := map[string]GoalSpecificationObligation{}
	for _, o := range s.Obligations {
		_, hasRequirement := requirements[o.RequirementID]
		if !claimID(o.ID) || len(o.ID) > 128 || !hasRequirement || len(o.Text) > 4096 || len(o.DoDItemRefs) > 32 || !specRefsExist(o.DoDItemRefs, func(ref string) bool { return dodRefs[ref] }) {
			return fmt.Errorf("goal specification obligation is incomplete: %q", o.ID)
		}
		obligations[o.ID] = o
	}
	verifications := map[string]bool{}
	for _, v := range s.Verifications {
		_, hasObligation := obligations[v.ObligationID]
		if !claimID(v.ID) || len(v.ID) > 128 || !hasObligation || len(v.Declaration) > 4096 {
			return fmt.Errorf("goal specification verification is incomplete: %q", v.ID)
		}
		switch v.Status {
		case GoalVerificationDeclared:
			if v.ProfileID == "" || v.EvidenceRequirementID == "" || v.Scope == "" {
				return fmt.Errorf("goal specification verification %q lacks exact profile declaration", v.ID)
			}
		case GoalVerificationUnverified:
			if v.ProfileID != "" || v.EvidenceRequirementID != "" || v.Scope != "" {
				return fmt.Errorf("unverified goal specification verification %q claims profile binding", v.ID)
			}
		default:
			return fmt.Errorf("goal specification verification %q has unknown status", v.ID)
		}
		verifications[v.ID] = true
	}
	allowedReview := map[string]bool{}
	for _, kind := range goalReviewKinds {
		allowedReview[kind] = true
	}
	context := goalSpecificationReviewContext{contentHash: s.ContentHash(), evidence: identities, requirements: requirements}
	reviews := map[string]bool{}
	for _, r := range s.Reviews {
		if !allowedReview[r.Kind] || reviews[r.Kind] {
			return fmt.Errorf("goal specification review %q has invalid identity or provenance", r.Kind)
		}
		if err := context.validate(r); err != nil {
			return err
		}
		reviews[r.Kind] = true
	}
	return nil
}

// 仅在同次校验中复用已通过结构检查的 body，不跨 record/revision 保存。
type goalSpecificationReviewContext struct {
	contentHash  string
	evidence     map[string]bool
	requirements map[string]GoalSpecificationRequirement
}

func (c goalSpecificationReviewContext) validate(r GoalSpecificationCompletenessReview) error {
	r.Kind = strings.TrimSpace(r.Kind)
	r.Status = strings.TrimSpace(r.Status)
	r.SpecificationHash = strings.TrimSpace(r.SpecificationHash)
	r.Reason = strings.TrimSpace(r.Reason)
	r.EvidenceRefs = normalizedSpecRefs(r.EvidenceRefs)
	r.RequirementIDs = normalizedSpecRefs(r.RequirementIDs)
	if !GoalSpecificationReviewKind(r.Kind) || r.SpecificationHash != c.contentHash || r.Status == "" || len(r.Reason) > 4096 || len(r.EvidenceRefs) > 64 || len(r.RequirementIDs) > 256 || !specRefsExist(r.EvidenceRefs, func(ref string) bool { return c.evidence[ref] }) || !specRefsExist(r.RequirementIDs, func(ref string) bool { _, ok := c.requirements[ref]; return ok }) {
		return fmt.Errorf("goal specification review %q has invalid identity or provenance", r.Kind)
	}
	switch r.Status {
	case GoalReviewPass:
		if r.Reason == "" || len(r.EvidenceRefs) == 0 || len(r.RequirementIDs) == 0 {
			return fmt.Errorf("goal specification PASS review %q lacks traceable evidence", r.Kind)
		}
		scope := GoalSpecification{Requirements: make([]GoalSpecificationRequirement, 0, len(c.requirements))}
		for _, requirement := range c.requirements {
			scope.Requirements = append(scope.Requirements, requirement)
		}
		if err := scope.ValidateReviewCoverage(r); err != nil {
			return err
		}
	case GoalReviewFail, GoalReviewUnknown:
	default:
		return fmt.Errorf("goal specification review %q has unknown status", r.Kind)
	}
	return nil
}

func specRefsExist(refs []string, exists func(string) bool) bool {
	if len(refs) > 512 {
		return false
	}
	last := ""
	for i, ref := range refs {
		if ref == "" || len(ref) > 1024 || (i > 0 && ref == last) || !exists(ref) {
			return false
		}
		last = ref
	}
	return true
}
