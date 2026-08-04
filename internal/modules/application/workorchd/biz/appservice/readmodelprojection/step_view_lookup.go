package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func stepByID(steps []readmodelcore.StepView, stepID string) *readmodelcore.StepView {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil
	}
	for idx := range steps {
		if steps[idx].ID == stepID {
			return &steps[idx]
		}
	}
	return nil
}
