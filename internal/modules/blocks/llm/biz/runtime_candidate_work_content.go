package llm

import (
	"encoding/json"
	"fmt"
	"reflect"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Expand the execution section once at admission. Work integrity and causal
// review checks consume this same value, not separate reference decoders.
func runtimeCandidateExecutionTailForIntegrity(c contract.RuntimeCandidateContext, m contract.RuntimeCandidateContractManifest, prompt *llmmodels.PromptContext) (runtimeCandidateExecutionTailProjection, error) {
	var actual runtimeCandidateExecutionTailProjection
	expected, err := contract.ProjectRuntimeCandidateWorkContent(c, m)
	if err != nil {
		return actual, err
	}
	if prompt == nil {
		return actual, fmt.Errorf("P4.implementation_work_content: missing prompt")
	}
	count := 0
	for _, section := range prompt.Sections {
		if promptSectionGroup(section.ID) != "C4" {
			continue
		}
		if err := json.Unmarshal([]byte(section.Content), &actual); err != nil {
			return actual, fmt.Errorf("P4.causal_execution_tail: invalid execution section")
		}
		if err := expandRuntimeCandidateFiles(&actual, prompt); err != nil {
			return actual, fmt.Errorf("P4.request_local_files: %w", err)
		}
		if !reflect.DeepEqual(actual.ImplementationWorkContent, expected) {
			return actual, fmt.Errorf("P4.implementation_work_content: rendered source differs from frozen scope")
		}
		count++
	}
	if count != 1 {
		return actual, fmt.Errorf("P4.implementation_work_content: missing or duplicate execution section")
	}
	return actual, nil
}
