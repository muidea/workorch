package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

const DefinitionAuthoringMappingProtocol = "workorch.authoring.sealed-ref-mapping/v1"

// DefinitionAuthoringMapping 是唯一当前创作提案 wire；provider 仅提出分组与声明，不能改写封存来源事实。
type DefinitionAuthoringMapping struct {
	Protocol                    string
	InputHash                   string
	TemplateSnapshotHash        string
	ResourceSnapshotHash        string
	GoalRequirement             DefinitionAuthoringGoalMapping
	Dimensions                  []DefinitionAuthoringDimensionMapping
	Requirements                []DefinitionAuthoringRequirementMapping
	Obligations                 []DefinitionAuthoringObligationMapping
	ExecutionProofMappings      []GoalExecutionProofMapping            `json:",omitempty"`
	EvidencePurposeMappings     []GoalEvidencePurposeMapping           `json:",omitempty"`
	ProfileAcceptanceSelections []DefinitionProfileAcceptanceSelection `json:",omitempty"`
}
type DefinitionProfileAcceptanceSelection struct {
	SourceRef         string
	AcceptanceItemRef string
}

type DefinitionAuthoringGoalMapping struct {
	DimensionRef string
	ResourceIDs  []string
}
type DefinitionAuthoringDimensionMapping struct {
	Ref         string
	ResourceIDs []string
}
type DefinitionAuthoringRequirementMapping struct {
	Ref            string
	DimensionRef   string
	SourceItemRefs []string
	ResourceIDs    []string
	Text           string
}
type DefinitionAuthoringObligationMapping struct {
	CandidateItemRef string
	RequirementRef   string
	Verification     DefinitionAuthoringVerificationMapping
}
type DefinitionAuthoringVerificationMapping struct {
	Declaration           string
	Status                string
	ProfileID             string
	EvidenceRequirementID string
	Scope                 string
}

func (m DefinitionAuthoringMapping) Clone() DefinitionAuthoringMapping {
	m.ProfileAcceptanceSelections = slices.Clone(m.ProfileAcceptanceSelections)
	m.EvidencePurposeMappings = slices.Clone(m.EvidencePurposeMappings)
	m.ExecutionProofMappings = slices.Clone(m.ExecutionProofMappings)
	m.GoalRequirement.ResourceIDs = slices.Clone(m.GoalRequirement.ResourceIDs)
	m.Dimensions = slices.Clone(m.Dimensions)
	for i := range m.Dimensions {
		m.Dimensions[i].ResourceIDs = slices.Clone(m.Dimensions[i].ResourceIDs)
	}
	m.Requirements = slices.Clone(m.Requirements)
	for i := range m.Requirements {
		m.Requirements[i].SourceItemRefs = slices.Clone(m.Requirements[i].SourceItemRefs)
		m.Requirements[i].ResourceIDs = slices.Clone(m.Requirements[i].ResourceIDs)
	}
	m.Obligations = slices.Clone(m.Obligations)
	return m
}

// CompileDefinitionAuthoringMapping 是 owner 纯编译；Profile 适用性仍由 receiving owner 独立校验，不授予证据 credit。
func CompileDefinitionAuthoringMapping(d DefinitionDerivationRequest, m DefinitionAuthoringMapping) (GoalSpecification, error) {
	if d.Metadata().ValidateView() != nil || d.Proposal == nil {
		return GoalSpecification{}, fmt.Errorf("authoring mapping requires complete sealed derivation")
	}
	c, err := newAuthoringSpecificationContext(d.Input, &d.Template, *d.Proposal, d.Resources)
	if err != nil {
		return GoalSpecification{}, err
	}
	if m.Protocol != DefinitionAuthoringMappingProtocol || m.InputHash != d.Input.InputHash() || m.TemplateSnapshotHash != WorkspaceTemplateSnapshotHash(&d.Template) || m.ResourceSnapshotHash != d.ResourceSnapshotHash {
		return GoalSpecification{}, mappingError("sealed_identity", "", "$", "mapping does not bind sealed input/template/resources")
	}
	if len(m.Dimensions) == 0 || len(m.Dimensions) > 64 || len(m.Requirements) == 0 || len(m.Requirements) > 255 || len(m.Obligations) != len(c.items) {
		return GoalSpecification{}, mappingError("mapping_cardinality", "", "$", "dimensions/requirements/obligations cardinality invalid")
	}
	validRef := func(s string) bool {
		return s != "" && len(s) <= 128 && strings.TrimSpace(s) == s && !strings.ContainsRune(s, 0)
	}
	resources := map[string]bool{}
	for _, r := range c.resources {
		resources[r.ID] = true
	}
	checkResources := func(refs []string) bool {
		return len(refs) <= 128 && exactMappingRefs(refs, func(s string) bool { return resources[s] })
	}
	dims := map[string]DefinitionAuthoringDimensionMapping{}
	for i, dim := range m.Dimensions {
		if !validRef(dim.Ref) || dims[dim.Ref].Ref != "" || !checkResources(dim.ResourceIDs) {
			return GoalSpecification{}, mappingError("dimension_mapping", "", fmt.Sprintf("$.Dimensions[%d]", i), "invalid/duplicate dimension or resource ref")
		}
		dims[dim.Ref] = dim
	}
	if dims[m.GoalRequirement.DimensionRef].Ref == "" || !checkResources(m.GoalRequirement.ResourceIDs) {
		return GoalSpecification{}, mappingError("goal_mapping", "", "$.GoalRequirement", "Goal dimension/resource refs must exist")
	}
	candidates := map[string]DefinitionCandidateItem{}
	for _, item := range c.items {
		candidates[item.CandidateItemRef] = item
	}
	reqs := map[string]DefinitionAuthoringRequirementMapping{}
	reqIDs := map[string]string{}
	reqKinds := map[string]string{}
	assigned := map[string]string{}
	dimMembers := map[string][]string{}
	goalID := mappingCanonicalID("requirement", d.Input.InputHash(), DefinitionAuthoringGoalItemRef)
	dimMembers[m.GoalRequirement.DimensionRef] = append(dimMembers[m.GoalRequirement.DimensionRef], goalID)
	for i, req := range m.Requirements {
		path := fmt.Sprintf("$.Requirements[%d]", i)
		if !validRef(req.Ref) || reqs[req.Ref].Ref != "" || dims[req.DimensionRef].Ref == "" || len(req.SourceItemRefs) == 0 || len(req.SourceItemRefs) > 32 || req.Text == "" || len(req.Text) > 4096 || !checkResources(req.ResourceIDs) {
			return GoalSpecification{}, mappingError("requirement_mapping", "", path, "invalid/duplicate requirement, dimension, text or resource refs")
		}
		kind := ""
		for _, ref := range req.SourceItemRefs {
			item, ok := candidates[ref]
			if !ok || assigned[ref] != "" {
				return GoalSpecification{}, mappingError("source_mapping", ref, path+".SourceItemRefs", "unknown/duplicate source, or Goal mixed with candidates")
			}
			actualKind := authoringItemKind(item)
			if kind != "" && kind != actualKind {
				return GoalSpecification{}, mappingError("mixed_source_kind", ref, path+".SourceItemRefs", "requirement cannot mix sealed source kinds")
			}
			kind = actualKind
			assigned[ref] = req.Ref
		}
		refs := slices.Clone(req.SourceItemRefs)
		sort.Strings(refs)
		id := mappingCanonicalID("requirement", append([]string{d.Input.InputHash()}, refs...)...)
		reqIDs[req.Ref], reqKinds[req.Ref], reqs[req.Ref] = id, kind, req
		dimMembers[req.DimensionRef] = append(dimMembers[req.DimensionRef], id)
	}
	if len(assigned) != len(c.items) {
		return GoalSpecification{}, mappingError("missing_source", "", "$.Requirements", "every sealed candidate must be explicitly grouped exactly once")
	}
	dimIDs := map[string]string{}
	for _, dim := range m.Dimensions {
		ref := dim.Ref
		members := slices.Clone(dimMembers[ref])
		sort.Strings(members)
		if len(members) == 0 {
			return GoalSpecification{}, mappingError("unused_dimension", "", "$.Dimensions", "dimension has no explicit requirement")
		}
		rs := slices.Clone(dim.ResourceIDs)
		sort.Strings(rs)
		fields := []string{"input", d.Input.InputHash(), "requirements", fmt.Sprint(len(members))}
		fields = append(fields, members...)
		fields = append(fields, "resources", fmt.Sprint(len(rs)))
		fields = append(fields, rs...)
		dimIDs[ref] = mappingCanonicalID("dimension", fields...)
	}
	spec := GoalSpecification{Version: GoalSpecificationVersionCurrent, GoalItemRef: DefinitionAuthoringGoalItemRef, GoalTextHash: StableDefinitionTextHash(c.goal.Text), Resources: c.resources, Dimensions: []GoalSpecificationDimension{}, Requirements: []GoalSpecificationRequirement{}, Obligations: []GoalSpecificationObligation{}, Verifications: []GoalSpecificationVerification{}, Reviews: []GoalSpecificationCompletenessReview{}}
	spec.ExecutionProofMappings = slices.Clone(m.ExecutionProofMappings)
	spec.EvidencePurposeMappings, err = compileAuthoringEvidencePurposes(d, m, c.items)
	if err != nil {
		return GoalSpecification{}, err
	}
	for ref, dim := range dims {
		spec.Dimensions = append(spec.Dimensions, GoalSpecificationDimension{ID: dimIDs[ref], Required: true, Activated: true, ResourceIDs: slices.Clone(dim.ResourceIDs)})
	}
	spec.Requirements = append(spec.Requirements, GoalSpecificationRequirement{ID: goalID, DimensionID: dimIDs[m.GoalRequirement.DimensionRef], Required: true, SourceKind: GoalRequirementUserExplicit, SourceItemRefs: []string{DefinitionAuthoringGoalItemRef}, ResourceIDs: slices.Clone(m.GoalRequirement.ResourceIDs), Text: c.goal.Text})
	for ref, req := range reqs {
		spec.Requirements = append(spec.Requirements, GoalSpecificationRequirement{ID: reqIDs[ref], DimensionID: dimIDs[req.DimensionRef], Required: true, SourceKind: reqKinds[ref], SourceItemRefs: slices.Clone(req.SourceItemRefs), ResourceIDs: slices.Clone(req.ResourceIDs), Text: req.Text})
	}
	mapped := map[string]bool{}
	for i, om := range m.Obligations {
		item, ok := candidates[om.CandidateItemRef]
		path := fmt.Sprintf("$.Obligations[%d]", i)
		if !ok || mapped[om.CandidateItemRef] || assigned[om.CandidateItemRef] != om.RequirementRef || reqIDs[om.RequirementRef] == "" {
			return GoalSpecification{}, mappingError("obligation_mapping", om.CandidateItemRef, path, "unknown/duplicate candidate or dangling/wrong requirement")
		}
		mapped[om.CandidateItemRef] = true
		id := mappingCanonicalID("obligation", d.Input.InputHash(), item.CandidateItemRef, item.SourceIdentity)
		spec.Obligations = append(spec.Obligations, GoalSpecificationObligation{ID: id, RequirementID: reqIDs[om.RequirementRef], DoDItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		v := om.Verification
		spec.Verifications = append(spec.Verifications, GoalSpecificationVerification{ID: mappingCanonicalID("verification", id), ObligationID: id, Declaration: v.Declaration, Status: v.Status, ProfileID: v.ProfileID, EvidenceRequirementID: v.EvidenceRequirementID, Scope: v.Scope})
	}
	// 排序是显式 canonical 编译，不改写或规范化原 wire。
	sort.Slice(spec.Dimensions, func(i, j int) bool { return spec.Dimensions[i].ID < spec.Dimensions[j].ID })
	sort.Slice(spec.Requirements, func(i, j int) bool { return spec.Requirements[i].ID < spec.Requirements[j].ID })
	for i := range spec.Requirements {
		sort.Strings(spec.Requirements[i].SourceItemRefs)
		sort.Strings(spec.Requirements[i].ResourceIDs)
	}
	for i := range spec.Dimensions {
		sort.Strings(spec.Dimensions[i].ResourceIDs)
	}
	sort.Slice(spec.Obligations, func(i, j int) bool { return spec.Obligations[i].ID < spec.Obligations[j].ID })
	sort.Slice(spec.Verifications, func(i, j int) bool { return spec.Verifications[i].ID < spec.Verifications[j].ID })
	if err := c.validate(spec); err != nil {
		return GoalSpecification{}, err
	}
	return spec, nil
}

func exactMappingRefs(refs []string, exists func(string) bool) bool {
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref == "" || seen[ref] || !exists(ref) {
			return false
		}
		seen[ref] = true
	}
	return true
}
func mappingCanonicalID(kind string, fields ...string) string {
	h := sha256.New()
	writeDefinitionHash(h, DefinitionAuthoringMappingProtocol, "node-kind", kind, "fields", fmt.Sprint(len(fields)))
	writeDefinitionHash(h, fields...)
	return "authoring-" + kind + ":" + hex.EncodeToString(h.Sum(nil))
}

type DefinitionAuthoringMappingError struct {
	Code             string
	CandidateItemRef string
	Path             string
	Reason           string
}

func (e *DefinitionAuthoringMappingError) Error() string {
	return fmt.Sprintf("authoring mapping %s candidate=%q path=%s: %s", e.Code, e.CandidateItemRef, e.Path, e.Reason)
}
func mappingError(code, candidate, path, reason string) error {
	return &DefinitionAuthoringMappingError{code, candidate, path, reason}
}

func RecompileDefinitionAuthoringProposal(d DefinitionDerivationRequest, r DefinitionAuthoringSpecificationRequest) (GoalSpecification, error) {
	if r.Mapping == nil || r.SuccessAudit == nil || r.SuccessAudit.Validate() != nil {
		return GoalSpecification{}, fmt.Errorf("authoring proposal lacks current mapping and complete success byte audit")
	}
	decoded, err := DecodeAuthoringMappingFor(d, r.SuccessAudit.Document.Content)
	if err != nil {
		return GoalSpecification{}, err
	}
	if !reflect.DeepEqual(decoded, *r.Mapping) {
		return GoalSpecification{}, fmt.Errorf("authoring typed mapping differs from exact provider document")
	}
	compiled, err := CompileDefinitionAuthoringMapping(d, decoded)
	if err != nil {
		return GoalSpecification{}, err
	}
	if r.Proposal == nil || !reflect.DeepEqual(compiled, *r.Proposal) {
		return GoalSpecification{}, fmt.Errorf("authoring canonical proposal differs from sealed-source compilation")
	}
	return compiled, nil
}
