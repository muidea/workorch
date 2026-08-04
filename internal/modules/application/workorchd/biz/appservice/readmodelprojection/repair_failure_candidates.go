package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func repairFailureProjectionFromCheckpoint(checkpoint projectioncontract.RuntimeCheckpoint) *repairFailureProjection {
	class := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureClass"]))
	signature := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureSignature"]))
	event := strings.TrimSpace(readModelStringValue(checkpoint.Data["event"]))
	if class == "" && signature == "" && event != "step_failed" {
		return nil
	}
	return &repairFailureProjection{
		stepID:      checkpoint.CurrentStepID,
		class:       class,
		signature:   signature,
		occurredAt:  checkpoint.OccurredAt,
		evidenceRef: checkpoint.ID,
	}
}

func repairFailureProjectionFromStep(step readmodelcore.StepView) *repairFailureProjection {
	class := strings.TrimSpace(readModelStringValue(step.Output["failureClass"]))
	signature := strings.TrimSpace(readModelStringValue(step.Output["failureSignature"]))
	if class == "" && signature == "" {
		return nil
	}
	return &repairFailureProjection{
		stepID:      step.ID,
		class:       class,
		signature:   signature,
		evidenceRef: step.ID,
	}
}
