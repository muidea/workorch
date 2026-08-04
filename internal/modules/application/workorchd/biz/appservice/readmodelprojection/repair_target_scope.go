package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func readModelFailureTargetScopeFromStep(step *readmodelcore.StepView) string {
	if step == nil {
		return ""
	}
	signature := readModelStringValue(step.Output["failureSignature"])
	if scope := readModelFailureTargetScope(signature); scope != "" {
		return scope
	}
	for _, scope := range []string{
		readModelStringValue(step.Output["failureTargetScope"]),
		readModelStringValue(step.Output["targetScope"]),
	} {
		if target := readModelFailureTargetScope(scope); target != "" {
			return target
		}
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
