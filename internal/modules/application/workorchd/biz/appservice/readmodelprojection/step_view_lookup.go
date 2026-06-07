package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func stepByID(steps []presenter.StepView, stepID string) *presenter.StepView {
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
