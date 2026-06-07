package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func readModelFailureTargetScopeFromStep(step *presenter.StepView) string {
	if step == nil {
		return ""
	}
	signature := readModelStringValue(step.Output["failureSignature"])
	if scope := readModelFailureTargetScope(signature); scope != "" {
		return scope
	}
	return strings.TrimSpace(readModelFirstNonEmptyString(
		readModelStringValue(step.Output["targetRef"]),
		readModelStringValue(step.Output["path"]),
	))
}

func readModelFailureTargetScope(signature string) string {
	for _, target := range readModelTargetsFromFailureSignature(signature) {
		return target
	}
	return ""
}
