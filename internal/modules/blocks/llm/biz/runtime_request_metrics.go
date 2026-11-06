package llm

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Byte accounting is an observation of the existing request, never a token
// estimate, budget allocation, input selection or raw-content log.
func runtimeProviderSectionMetrics(s models.PromptSection) models.ProviderCacheSectionHash {
	m := models.ProviderCacheSectionHash{BodyBytes: len(s.Content), ExpandedBytes: len(s.Content)}
	switch promptSectionGroup(s.ID) {
	case "C0":
		m.Applicability = "runtime_contract"
	case "C1":
		m.Applicability = "sealed_definition_and_original_sources"
	case "C2":
		m.Applicability = "declarative_capability_contract"
	case "C3":
		m.Applicability = "current_request_authority"
	case "C4":
		m.Applicability = "execution_and_correction_facts"
	case "C5":
		m.Applicability = "acceptance_references"
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(s.Content), &fields) != nil {
		return m
	}
	switch s.ID {
	case "C1.derivation-input", "C1.specification-source", "C1.specification-snapshot":
		m.Applicability = "exact_unconfirmed_authoring_sources"
	case "C2.specification-profiles":
		m.Applicability = "source_applicable_authoring_declarations"
	case "C3.specification-request":
		m.Applicability = "current_authoring_task"
	}
	keys := make([]string, 0, len(fields))
	allowed := runtimeMetricComponentNames(s.ID)
	for key := range fields {
		if allowed[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		m.Components = append(m.Components, models.ProviderContextComponent{Path: key, Bytes: len(fields[key]), Applicability: "included", Reason: "current_phase_input", Source: s.ID})
	}
	switch s.ID {
	case "C1.goal-dod":
		var wire runtimeDefinitionWire
		var p runtimeCandidateDefinitionProjection
		if json.Unmarshal([]byte(s.Content), &wire) == nil && json.Unmarshal([]byte(s.Content), &p) == nil {
			raw, _ := json.Marshal(runtimeDefinitionInline(p))
			m.ExpandedBytes = len(raw)
			m.LocalReferences = len(wire.TextRefs)
		}
	case "C3.semantic-authority":
		var wire runtimeSemanticAuthorityWire
		var p runtimeSemanticAuthorityProjection
		if json.Unmarshal([]byte(s.Content), &wire) == nil && json.Unmarshal([]byte(s.Content), &p) == nil {
			raw, _ := json.Marshal(runtimeSemanticAuthorityInline(p))
			m.ExpandedBytes = len(raw)
			m.LocalReferences = len(wire.FileRefs) + len(wire.IntentRefs) + len(wire.GenerationRefs)
		}
	}
	return m
}

// Audit field names come only from the owner's closed projection, never from
// arbitrary JSON keys in business text or another prompt family.
func runtimeMetricComponentNames(section string) map[string]bool {
	ret := map[string]bool{}
	var t reflect.Type
	switch section {
	case "C1.goal-dod":
		t = reflect.TypeFor[runtimeDefinitionWire]()
	case "C3.semantic-authority":
		t = reflect.TypeFor[runtimeSemanticAuthorityWire]()
	case "C4.semantic-tail":
		t = reflect.TypeFor[runtimeSemanticTailProjection]()
	case "C4.execution-tail":
		t = reflect.TypeFor[runtimeCandidateExecutionTailProjection]()
	case "C3.runtime-authority":
		for _, name := range []string{"decision", "transaction", "outputContract", "context"} {
			ret[name] = true
		}
	case "C1.derivation-input", "C1.specification-source", "C1.specification-snapshot", "C2.specification-profiles", "C3.specification-request":
		for _, name := range []string{"goal", "userRequirements", "templateRequirements", "candidateAcceptance", "inputHash", "templateHash", "resourceSnapshotHash", "resources", "expectedResources", "goalTextHash", "profiles", "sourceApplicableVerificationDeclarations", "profileSnapshotHashes", "requiredEvidencePurposes", "wireContractRef", "wireContractHash", "outputShape", "reviewEvidenceRefs", "proposalHash", "specification", "requiredRequirementIDs", "kind"} {
			ret[name] = true
		}
	case "C2.capability-contract":
		for _, name := range []string{"kinds", "capabilityKinds", "executionContract"} {
			ret[name] = true
		}
	}
	if t != nil {
		for _, field := range reflect.VisibleFields(t) {
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" && !field.Anonymous {
				ret[name] = true
			}
		}
	}
	return ret
}

// Selection metadata uses closed owner fields/reasons only. It neither logs
// business keys/text nor claims that included observations passed acceptance.
func runtimeProviderContextSectionMetrics(s models.PromptSection, prompt *models.PromptContext) models.ProviderCacheSectionHash {
	m := runtimeProviderSectionMetrics(s)
	if prompt == nil {
		return m
	}
	add := func(path, source, reason string, present bool, value any) {
		raw, _ := json.Marshal(value)
		state, size := "included", len(raw)
		if !present {
			state, size = "excluded", 0
		}
		component := models.ProviderContextComponent{Path: path, Bytes: size, Applicability: state, Reason: reason, Source: source}
		for i := range m.Components {
			if m.Components[i].Path == path {
				m.Components[i] = component
				return
			}
		}
		m.Components = append(m.Components, component)
	}
	switch s.ID {
	case "C3.runtime-authority":
		var p struct {
			Context runtimeCandidateAuthorityProjection `json:"context"`
		}
		if json.Unmarshal([]byte(s.Content), &p) == nil {
			work := p.Context.RequiredState == rc.ConvergenceStateNeedStateChange
			reason := "sealed_state_change_source_and_dependencies"
			if !work {
				reason = "sealed_task_consumes_bindings_and_observations_not_generation"
			}
			corpusReason, preparationReason := reason, reason
			if work && len(p.Context.ValidationGenerationContexts) == 0 {
				corpusReason = "no_matching_owner_generation_declaration"
			}
			if work && len(p.Context.ValidationProofPreparations) == 0 {
				preparationReason = "no_preparation_for_selected_prospective_targets"
			}
			add("context.validationGenerationContexts", "owner.operationAdmissibilities.validationSurfaces", corpusReason, len(p.Context.ValidationGenerationContexts) > 0, p.Context.ValidationGenerationContexts)
			add("context.validationProofPreparations", "owner.focus.prospectiveTargets", preparationReason, len(p.Context.ValidationProofPreparations) > 0, p.Context.ValidationProofPreparations)
			add("context.activation.memories", "owner.activation.memories", "explicit_activation_snapshot_no_stage_selector", len(p.Context.Activation.Memories) > 0, p.Context.Activation.Memories)
		}
	case "C4.semantic-tail":
		var p runtimeSemanticTailProjection
		if json.Unmarshal([]byte(s.Content), &p) == nil {
			add("executionHistory", "owner.semantic.executionHistory", "work_and_proof_causal_receipts_ordering_excluded", p.ExecutionHistory.Hash != "", p.ExecutionHistory)
			m.LocalReferences = len(p.ChangeReviewFileRefs) + len(p.ChangeReviewValueRefs)
			var authority runtimeSemanticAuthorityProjection
			for _, section := range prompt.Sections {
				if section.ID != "C3.semantic-authority" {
					continue
				}
				if json.Unmarshal([]byte(section.Content), &authority) != nil {
					break
				}
				var content *rc.RuntimeImplementationWorkContent
				if authority.WorkContext != nil {
					content = authority.WorkContext.Content
				}
				if expandChangeReviewFiles(p.ChangeReview, p.ChangeReviewFileRefs, changeReviewBodySources(content, "C3.workContext.content", authority.Manifest.ValidationGenerationContexts, "C3.manifest.validationGenerationContexts")) == nil && expandChangeReviewValues(p.ChangeReview, p.ChangeReviewValueRefs) == nil {
					p.ChangeReviewValueRefs = nil
					p.ChangeReviewFileRefs = nil
					raw, _ := json.Marshal(p)
					m.ExpandedBytes = len(raw)
				}
			}
			add("changeReview", "owner.committedDispatches_and_scoped_observation", "exact_current_target_mutation_corpus", p.ChangeReview != nil, p.ChangeReview)
		}
	case "C4.execution-tail":
		var tail runtimeCandidateExecutionTailProjection
		if json.Unmarshal([]byte(s.Content), &tail) == nil {
			m.LocalReferences = len(tail.WorkFileRefs) + len(tail.ChangeReviewFileRefs) + len(tail.ChangeReviewValueRefs)
			var authority struct {
				Context runtimeCandidateAuthorityProjection `json:"context"`
			}
			for _, section := range prompt.Sections {
				if section.ID == "C3.runtime-authority" {
					json.Unmarshal([]byte(section.Content), &authority)
				}
			}
			if expandRuntimeCandidateWorkFiles(&tail, prompt) == nil && expandChangeReviewFiles(tail.ChangeReview, tail.ChangeReviewFileRefs, changeReviewBodySources(nil, "", authority.Context.ValidationGenerationContexts, "C3.context.validationGenerationContexts")) == nil && expandChangeReviewValues(tail.ChangeReview, tail.ChangeReviewValueRefs) == nil {
				tail.ChangeReviewValueRefs = nil
				tail.ChangeReviewFileRefs = nil
				tail.WorkFileRefs, tail.ReferenceRule = nil, ""
				raw, _ := json.Marshal(tail)
				m.ExpandedBytes = len(raw)
			}
			add("latestAcceptedEvidence", "owner.evidence.accepted", "current_focus_cwi_allowed_refs_or_failure", len(tail.AcceptedEvidence) > 0, tail.AcceptedEvidence)
			add("latestRejectedEvidence", "owner.evidence.rejected", "current_focus_cwi_or_exact_failure_no_recent_cutoff", len(tail.RejectedEvidence) > 0, tail.RejectedEvidence)
			add("recentAssessments", "owner.assessments", "current_focus_or_cwi", len(tail.Assessments) > 0, tail.Assessments)
		}
	}
	return m
}
