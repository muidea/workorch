package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func closureTraceFromStep(step presenter.StepView) (readmodelcore.ClosureTrace, bool) {
	input := step.Input
	output := step.Output
	trace := readmodelcore.ClosureTrace{
		StepID: step.ID,
		Kind:   llmcommon.CanonicalRuntimeBlockKind(step.Kind),
		State:  step.State,
	}
	applyClosureTraceGraphFields(&trace, input, output)
	trace.IgnoredExecutionGraph = readModelBoolValue(output["ignoredExecutionGraph"])
	trace.Reason = strings.TrimSpace(firstNonEmptyTimelineString(
		readModelStringValue(input["assessmentOnlyReason"]),
		readModelStringValue(output["error"]),
		readModelStringValue(output["reason"]),
	))
	trace.MissingDoDItems = MissingDoDItemsFromStepOutput(output)
	trace.EvidenceRefs = compactUniqueStrings([]string{step.ID})
	ok := trace.GraphFollowUp ||
		trace.AssessmentOnly ||
		trace.GraphExpansionLimitFollowUp ||
		trace.GraphFollowUpExpansionLimitExceeded ||
		trace.IgnoredExecutionGraph ||
		len(trace.MissingDoDItems) > 0
	return trace, ok
}
