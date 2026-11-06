package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateRuntimeSemanticRenderedIntegrity(schema string, context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest, attempt int, previous *runtimecontract.RuntimeSemanticCorrectionObservation, directive *runtimecontract.RuntimeSemanticCorrectionDirective, budget *llmmodels.ProviderContextBudget, prompt *llmmodels.PromptContext) error {
	if budget == nil || prompt == nil {
		return fmt.Errorf("runtime semantic context integrity requires prompt and budget metadata")
	}
	missing := make([]string, 0, 6)
	if budget.RenderContractRef != runtimecontract.RuntimeSemanticContextEncoding {
		missing = append(missing, "P0.context_encoding")
	}
	contents := make(map[string]string, len(prompt.Sections))
	for _, section := range prompt.Sections {
		contents[promptSectionGroup(section.ID)] += strings.TrimSpace(section.Content)
	}
	for _, group := range []string{"C0", "C1", "C2", "C3", "C4"} {
		if contents[group] == "" {
			missing = append(missing, group)
		}
	}
	if context.Definition.GoalDefinition == "" || context.Definition.DoDDefinition == "" || len(manifest.CoverageItems) == 0 {
		missing = append(missing, "P0.contract")
	}
	if context.DefinitionSeed.Hash == "" || manifest.Hash == "" {
		missing = append(missing, "P2.coverage_scope")
	}
	if context.ProviderCeiling == nil {
		missing = append(missing, "P4.current_snapshots")
	}
	if attempt == 1 && (previous == nil || directive == nil) {
		missing = append(missing, "P1.current_failure")
	}
	if len(budget.RedactedPaths) > 0 && budget.RedactionApplied {
		missing = append(missing, "protected_context_redacted")
	}
	if context.Definition.SourceObligations != nil {
		var definition runtimeCandidateDefinitionProjection
		if json.Unmarshal([]byte(contents["C1"]), &definition) != nil || !runtimecontract.SameRuntimeSourceObligationInventory(definition.SourceObligations, context.Definition.SourceObligations) {
			missing = append(missing, "P0.source_obligation_inventory")
		}
	}
	expectedWorkContext := runtimeSemanticVisibleWorkContext(context, manifest)
	var tail runtimeSemanticTailProjection
	var authority runtimeSemanticAuthorityProjection
	tailErr := json.Unmarshal([]byte(contents["C4"]), &tail)
	authorityErr := json.Unmarshal([]byte(contents["C3"]), &authority)
	if authorityErr != nil || !samePromptSchemaJSON(authority.Schema, compactSemanticPlanPromptSchema([]byte(schema))) {
		missing = append(missing, "P0.semantic_output_schema")
	}
	if authorityErr != nil || authority.ContextEncoding != runtimecontract.RuntimeSemanticContextEncoding || tail.WorkContext != nil || (expectedWorkContext != nil && tail.WorkContextRef != semanticWorkContextRef) || (expectedWorkContext == nil && tail.WorkContextRef != "") {
		missing = append(missing, "P4.semantic_work_reference")
	} else {
		expanded, err := expandSemanticWorkContext(authority.WorkContext, tail.ExecutionHistory)
		if err != nil {
			missing = append(missing, "P4.semantic_work_reference")
		} else {
			tail.WorkContext = expanded
		}
	}

	var workContent *runtimecontract.RuntimeImplementationWorkContent
	if authority.WorkContext != nil {
		workContent = authority.WorkContext.Content
	}
	if expandChangeReviewFiles(tail.ChangeReview, tail.ChangeReviewFileRefs, changeReviewBodySources(workContent, "C3.workContext.content", authority.Manifest.ValidationGenerationContexts, "C3.manifest.validationGenerationContexts")) != nil {
		missing = append(missing, "P4.change_review_reference")
	}
	for _, field := range []struct {
		name  string
		equal bool
	}{
		{"P2.semantic_coverage_progress", reflect.DeepEqual(tail.CoverageProgress, context.CoverageProgress)},
		{"P1.semantic_execution_feedback", reflect.DeepEqual(tail.ExecutionFeedback, context.ExecutionFeedback)},
		{"P3.semantic_execution_history", reflect.DeepEqual(tail.ExecutionHistory, runtimeSemanticVisibleHistory(context, manifest))},
		{"P3.semantic_execution_budget", reflect.DeepEqual(tail.ExecutionBudget, context.ExecutionBudget)},
		{"P4.change_review", reflect.DeepEqual(tail.ChangeReview, context.ChangeReview)},
		{"P4.semantic_work_context", reflect.DeepEqual(tail.WorkContext, expectedWorkContext)},
	} {
		if tailErr != nil || !field.equal {
			missing = append(missing, field.name)
		}
	}
	for _, obligation := range manifest.ProofObligations {
		if obligation.ImplementationWork == nil {
			continue
		}
		// work choice 与 proof 两支均独立反证完整 C3 投影；模型不得猜测遗漏的 scope。
		projection, err := runtimeSemanticManifestForProvider(manifest, context.PlanStep, context.BatchCoverageWorkItemIDs)
		if err != nil {
			return err
		}
		var actual runtimeSemanticAuthorityProjection
		if json.Unmarshal([]byte(contents["C3"]), &actual) != nil || !reflect.DeepEqual(actual.Manifest, projection) {
			missing = append(missing, "P0.implementation_work_preimage")
		}
		if contents["C0"] != runtimeSemanticPlanContractCatalog() {
			missing = append(missing, "P0.implementation_work_static_contract")
		}
		break
	}
	if authorityErr == nil && authority.OutputInstruction != runtimeSemanticSharedWorkOutputInstruction(context, manifest)+runtimeChangeReviewInstruction(context.ChangeReview) {
		missing = append(missing, "P0.semantic_output_instruction")
	}
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
		projection, err := runtimeSemanticManifestForProvider(manifest, context.PlanStep, context.BatchCoverageWorkItemIDs)
		if err != nil {
			return err
		}
		var actual runtimeSemanticAuthorityProjection
		if json.Unmarshal([]byte(contents["C3"]), &actual) != nil {
			missing = append(missing, "P0.semantic_authority")
		} else {
			if !reflect.DeepEqual(actual.Manifest.ValidationGenerationContexts, projection.ValidationGenerationContexts) {
				missing = append(missing, "P4.validation_generation_context")
			}
			for _, obligation := range projection.ProofObligations {
				if obligation.ValidationProofRequirement == nil || (len(obligation.ValidationProofRequirement.InputReviews) == 0 && len(obligation.ValidationProofRequirement.SourceObligationChecks) == 0) {
					continue
				}
				found := false
				for _, rendered := range actual.Manifest.ProofObligations {
					if rendered.CoverageWorkItemID == obligation.CoverageWorkItemID && reflect.DeepEqual(rendered.ValidationProofRequirement, obligation.ValidationProofRequirement) {
						found = true
					}
				}
				if !found {
					missing = append(missing, "P0.validation_input_review_preimage")
				}
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	budget.IntegrityWarnings = uniqueStrings(append(budget.IntegrityWarnings, missing...))
	return fmt.Errorf("runtime semantic context integrity missing or conflicted: %s", strings.Join(missing, ", "))
}

func runtimeRequestProviderCeiling(request llmmodels.RuntimeCandidateRequest) *runtimecontract.ProviderCeiling {
	if request.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return request.SemanticContext.ProviderCeiling
	}
	return request.CandidateContext.Resources.ProviderCeiling
}

func runtimeRequestScope(request llmmodels.RuntimeCandidateRequest) (string, string) {
	if request.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return string(request.SemanticContext.WorkspaceID), string(request.SemanticContext.RunID)
	}
	return string(request.CandidateContext.WorkspaceID), string(request.CandidateContext.RunID)
}

// Object member order is a rendering choice; every schema value and array
// position must still equal the owner-compiled prompt schema.
func samePromptSchemaJSON(actual, expected []byte) bool {
	decode := func(raw []byte) (any, error) {
		var value any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		if err := d.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("trailing schema JSON")
		}
		return value, nil
	}
	a, err := decode(actual)
	if err != nil {
		return false
	}
	b, err := decode(expected)
	return err == nil && reflect.DeepEqual(a, b)
}
