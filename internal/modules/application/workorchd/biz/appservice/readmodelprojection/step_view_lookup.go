package readmodelprojection

import (
	"strings"
)

func stepByID(steps []StepView, stepID string) *StepView {
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
