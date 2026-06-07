package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"

func readModelStructuredFailureSignalsFromStep(step *presenter.StepView, failureClass string, failureSignature string) map[string]any {
	if step == nil {
		return nil
	}
	if signals := readModelMapValue(step.Output["structuredFailureSignals"]); len(signals) > 0 {
		return signals
	}
	return readModelFallbackStructuredFailureSignals(step.Output, failureClass, failureSignature)
}
