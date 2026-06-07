package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func applyClosureTraceGraphFields(trace *readmodelcore.ClosureTrace, input map[string]any, output map[string]any) {
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
