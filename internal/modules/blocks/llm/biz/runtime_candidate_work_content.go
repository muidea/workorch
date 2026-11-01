package llm

import (
	"encoding/json"
	"fmt"
	"reflect"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateRuntimeCandidateWorkContentIntegrity(c contract.RuntimeCandidateContext, m contract.RuntimeCandidateContractManifest, prompt *llmmodels.PromptContext) error {
	expected, err := contract.ProjectRuntimeCandidateWorkContent(c, m)
	if err != nil {
		return err
	}
	if prompt == nil {
		return fmt.Errorf("P4.implementation_work_content: missing prompt")
	}
	count := 0
	for _, section := range prompt.Sections {
		if promptSectionGroup(section.ID) != "C4" {
			continue
		}
		var actual runtimeCandidateExecutionTailProjection
		if json.Unmarshal([]byte(section.Content), &actual) != nil || !reflect.DeepEqual(actual.ImplementationWorkContent, expected) {
			return fmt.Errorf("P4.implementation_work_content: rendered source differs from frozen scope")
		}
		count++
	}
	if count != 1 {
		return fmt.Errorf("P4.implementation_work_content: missing or duplicate execution section")
	}
	return nil
}
