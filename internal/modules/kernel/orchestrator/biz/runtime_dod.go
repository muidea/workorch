package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func completionDoDAssessmentError(run *runModel.Run, output map[string]any) error {
	_, _, err := validatedCompletionDoDAssessment(run, output)
	return err
}

func validatedCompletionDoDAssessment(run *runModel.Run, output map[string]any) (map[string]any, string, error) {
	if !requiresDoDAssessmentCompletionGate(run) {
		return nil, "", nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return nil, "", fmt.Errorf("%w: dodAssessment is required for llm-driven completion", common.ErrInvalidArgument)
	}
	satisfied, ok := assessment["satisfied"].(bool)
	if !ok {
		return nil, "", fmt.Errorf("%w: dodAssessment.satisfied is required", common.ErrInvalidArgument)
	}
	if !satisfied {
		return nil, "", fmt.Errorf("%w: dodAssessment is not satisfied", common.ErrInvalidArgument)
	}
	summary, _ := assessment["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, "", fmt.Errorf("%w: dodAssessment.summary is required for llm-driven completion", common.ErrInvalidArgument)
	}
	if err := validateDoDAssessmentItems(assessment["items"]); err != nil {
		return nil, "", err
	}
	return assessment, summary, nil
}

func requiresDoDAssessmentCompletionGate(run *runModel.Run) bool {
	return run != nil && run.DoDID != "" && initialStepKind(run) == llmWorkspaceExecutionStepKind
}

func doDAssessmentFromOutput(output map[string]any) map[string]any {
	if output == nil {
		return nil
	}
	if assessment := mapFromValue(output[stepOutputKeyDoDAssessment]); assessment != nil {
		return assessment
	}
	decision := mapFromValue(output[stepOutputKeyLLMDecision])
	if decision == nil {
		return nil
	}
	return mapFromValue(decision[stepOutputKeyDoDAssessment])
}

func validateDoDAssessmentItems(value any) error {
	items, ok := doDAssessmentItemMaps(value)
	if !ok || len(items) == 0 {
		return fmt.Errorf("%w: dodAssessment.items are required", common.ErrInvalidArgument)
	}
	for _, item := range items {
		name, _ := item["item"].(string)
		state, _ := item["state"].(string)
		name = strings.TrimSpace(name)
		state = strings.TrimSpace(state)
		if name == "" || state == "" {
			return fmt.Errorf("%w: dodAssessment item and state are required", common.ErrInvalidArgument)
		}
		if state == "satisfied" && len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
			return fmt.Errorf("%w: satisfied dodAssessment item requires evidenceRefs", common.ErrInvalidArgument)
		}
	}
	return nil
}

func doDAssessmentItemMaps(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneStepPayload(item))
		}
		return ret, true
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			ret = append(ret, cloneStepPayload(mapped))
		}
		return ret, true
	default:
		return nil, false
	}
}

func evidenceRefsFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			text = strings.TrimSpace(text)
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}
