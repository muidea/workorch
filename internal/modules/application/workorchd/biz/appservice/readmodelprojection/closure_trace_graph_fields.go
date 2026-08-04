package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func applyClosureTraceGraphFields(trace *workorchprojectioncontract.ClosureTrace, input map[string]any, output map[string]any) {
	trace.GraphFollowUp = readModelBoolValue(input["graphFollowUp"])
	trace.AssessmentOnly = readModelBoolValue(input["assessmentOnly"]) || readModelBoolValue(input["providerRetryAssessmentOnly"])
	trace.GraphExpansionLimitFollowUp = readModelBoolValue(input["graphExpansionLimitFollowUp"])
	trace.GraphFollowUpExpansionLimitExceeded = readModelBoolValue(output["graphFollowUpExpansionLimitExceeded"])
	trace.GraphFollowUpExpansionCount = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionCount"]),
		readModelIntValue(input["graphFollowUpExpansionCount"]),
	)
	trace.GraphFollowUpExpansionLimit = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionLimit"]),
		readModelIntValue(input["graphFollowUpExpansionLimit"]),
	)
}
