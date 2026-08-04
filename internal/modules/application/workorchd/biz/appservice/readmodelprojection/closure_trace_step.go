package readmodelprojection

import (
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func closureTraceFromStep(step workorchprojectioncontract.StepView) (workorchprojectioncontract.ClosureTrace, bool) {
	input := step.Input
	output := step.Output
	trace := workorchprojectioncontract.ClosureTrace{
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
