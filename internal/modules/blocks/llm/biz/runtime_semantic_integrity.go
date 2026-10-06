package llm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateRuntimeSemanticContextIntegrity(context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest, attempt int, previous *runtimecontract.RuntimeSemanticCorrectionObservation, directive *runtimecontract.RuntimeSemanticCorrectionDirective, budget *llmmodels.ProviderContextBudget, prompt *llmmodels.PromptContext) error {
	if budget == nil || prompt == nil {
		return fmt.Errorf("runtime semantic context integrity requires prompt and budget metadata")
	}
	missing := make([]string, 0, 6)
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
	var tail runtimeSemanticTailProjection
	if json.Unmarshal([]byte(contents["C4"]), &tail) != nil || !reflect.DeepEqual(tail.CoverageProgress, context.CoverageProgress) {
		missing = append(missing, "P2.semantic_coverage_progress")
	}
	if json.Unmarshal([]byte(contents["C4"]), &tail) != nil || !reflect.DeepEqual(tail.ExecutionFeedback, context.ExecutionFeedback) {
		missing = append(missing, "P1.semantic_execution_feedback")
	}
	if json.Unmarshal([]byte(contents["C4"]), &tail) != nil || !reflect.DeepEqual(tail.ExecutionHistory, context.ExecutionHistory) {
		missing = append(missing, "P3.semantic_execution_history")
	}
	if json.Unmarshal([]byte(contents["C4"]), &tail) != nil || !reflect.DeepEqual(tail.ExecutionBudget, context.ExecutionBudget) {
		missing = append(missing, "P3.semantic_execution_budget")
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
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
		projection, err := runtimeSemanticManifestForProvider(manifest, context.PlanStep, context.BatchCoverageWorkItemIDs)
		if err != nil {
			return err
		}
		var actual runtimeSemanticAuthorityProjection
		if json.Unmarshal([]byte(contents["C3"]), &actual) != nil {
			missing = append(missing, "P0.semantic_authority")
		} else {
			for _, obligation := range projection.ProofObligations {
				if obligation.ValidationProofRequirement == nil || len(obligation.ValidationProofRequirement.InputReviews) == 0 {
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
