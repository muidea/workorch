package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

type taskLifecycleMetadata struct {
	sourceOutputState string
	retryOf           string
	retryAttempt      int
	violationType     string
	violatedRule      string
	failureClass      string
	failureSignature  string
	targetScope       string
	blockingTicketID  string
	evidenceRefs      []string
}

func taskLifecycleMetadataFromStep(step StepView) taskLifecycleMetadata {
	output := taskMapValue(step.Output)
	input := taskMapValue(step.Input)
	recovery := taskMapValue(input["contractViolationRecovery"])
	violation := firstNonEmptyMap(
		taskMapValue(output["contractViolation"]),
		recovery,
		taskMapValue(taskMapValue(output["runtimeDiagnostics"])["contractViolation"]),
	)
	ret := taskLifecycleMetadata{
		sourceOutputState: sourceOutputStateFromStep(step, violation),
		retryOf:           firstNonEmptyString(readModelStringValue(input["retryOf"]), readModelStringValue(input["sourceStepId"]), readModelStringValue(recovery["sourceStepId"]), readModelStringValue(recovery["stepId"])),
		retryAttempt:      readModelIntValue(firstNonNilValue(input["retryAttempt"], violation["attempt"])),
		violationType:     firstNonEmptyString(readModelStringValue(output["contractViolationType"]), readModelStringValue(violation["violationType"])),
		violatedRule:      firstNonEmptyString(readModelStringValue(output["contractViolationRule"]), readModelStringValue(violation["violatedRule"])),
		failureClass:      firstNonEmptyString(readModelStringValue(output["failureClass"]), readModelStringValue(violation["failureClass"])),
		failureSignature:  firstNonEmptyString(readModelStringValue(output["failureSignature"]), readModelStringValue(violation["failureSignature"])),
		targetScope:       firstNonEmptyString(readModelStringValue(output["targetScope"]), readModelStringValue(violation["targetScope"])),
		blockingTicketID:  readModelStringValue(output["blockingTicketId"]),
		evidenceRefs:      compactUniqueStrings([]string{step.ID}),
	}
	if ret.violationType == "" && ret.violatedRule != "" {
		ret.violationType = ret.violatedRule
	}
	if ret.retryOf == "" {
		ret.retryOf = firstNonEmptyString(readModelStringValue(input["repairOfStepId"]), readModelStringValue(input["repairOfStepID"]))
	}
	if ret.retryAttempt == 0 {
		ret.retryAttempt = readModelIntValue(firstNonNilValue(input["attempt"], input["repairAttempt"], output["attempt"]))
	}
	if refs := readModelStringSliceValue(output["evidenceRefs"]); len(refs) > 0 {
		ret.evidenceRefs = append(ret.evidenceRefs, refs...)
	}
	if refs := readModelStringSliceValue(violation["sourceRefs"]); len(refs) > 0 {
		ret.evidenceRefs = append(ret.evidenceRefs, refs...)
	}
	ret.evidenceRefs = compactUniqueStrings(ret.evidenceRefs)
	return ret
}

func sourceOutputStateFromStep(step StepView, violation map[string]any) string {
	if len(violation) > 0 {
		if step.State == string(common.StepStateFailed) {
			return "rejected"
		}
		return "retrying"
	}
	if len(taskMapValue(taskMapValue(step.Input)["contractViolationRecovery"])) > 0 {
		return "retrying"
	}
	switch strings.TrimSpace(step.State) {
	case string(common.StepStateSucceeded):
		return "accepted"
	case string(common.StepStateFailed):
		return "rejected"
	case string(common.StepStateExecuting):
		return "running"
	default:
		return "proposed"
	}
}

func taskLifecycleMetadataForNode(node readmodelcore.TaskDecompositionNode, meta taskLifecycleMetadata) taskLifecycleMetadata {
	if strings.TrimSpace(node.StepID) == "" {
		switch meta.sourceOutputState {
		case "rejected", "retrying":
			return meta
		default:
			return taskLifecycleMetadata{}
		}
	}
	return meta
}

func applyTaskLifecycleToNode(node *readmodelcore.TaskDecompositionNode, meta taskLifecycleMetadata) {
	if node == nil {
		return
	}
	if meta.retryOf != "" {
		node.RetryOf = meta.retryOf
	}
	if meta.retryAttempt > 0 {
		node.RetryAttempt = meta.retryAttempt
	}
	if meta.violationType != "" {
		node.ViolationType = meta.violationType
	}
	if meta.violatedRule != "" {
		node.ViolatedRule = meta.violatedRule
	}
	if meta.failureClass != "" {
		node.FailureClass = meta.failureClass
	}
	if meta.failureSignature != "" {
		node.FailureSignature = meta.failureSignature
	}
	if meta.targetScope != "" {
		node.TargetScope = meta.targetScope
	}
	if meta.blockingTicketID != "" {
		node.BlockingTicketID = meta.blockingTicketID
	}
	if len(meta.evidenceRefs) > 0 {
		node.EvidenceRefs = compactUniqueStrings(append(node.EvidenceRefs, meta.evidenceRefs...))
	}
	if meta.sourceOutputState == "retrying" && node.State != "completed" {
		node.State = "retrying"
	}
	if meta.sourceOutputState == "rejected" && node.State == "pending" {
		node.State = "rejected"
	}
}

func applyTaskLifecycleToPlan(plan *readmodelcore.TaskPlan, meta taskLifecycleMetadata) {
	if plan == nil {
		return
	}
	plan.SourceOutputState = meta.sourceOutputState
	if meta.retryOf != "" {
		plan.RetryOf = meta.retryOf
	}
	if meta.retryAttempt > 0 {
		plan.RetryAttempt = meta.retryAttempt
	}
	if meta.violationType != "" {
		plan.ViolationType = meta.violationType
	}
	if meta.violatedRule != "" {
		plan.ViolatedRule = meta.violatedRule
	}
	if meta.failureClass != "" {
		plan.FailureClass = meta.failureClass
	}
	if meta.failureSignature != "" {
		plan.FailureSignature = meta.failureSignature
	}
	if meta.targetScope != "" {
		plan.TargetScope = meta.targetScope
	}
	if meta.blockingTicketID != "" {
		plan.BlockingTicketID = meta.blockingTicketID
	}
	if len(meta.evidenceRefs) > 0 {
		plan.EvidenceRefs = compactUniqueStrings(append(plan.EvidenceRefs, meta.evidenceRefs...))
	}
	switch meta.sourceOutputState {
	case "retrying":
		if plan.State != "accepted" {
			plan.State = "retrying"
		}
	case "rejected":
		if plan.State == "proposed" || plan.State == "revised" {
			plan.State = "rejected"
		}
	}
}

func taskCoverageTargetsFromInput(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	var ret []string
	for _, key := range []string{"coverageTargets", "coverageTargetRefs", "coverageWorkItemIds", "targetRefs", "affectedTargets", "relatedFailureTargets"} {
		ret = append(ret, readModelStringSliceValue(input[key])...)
		if value := strings.TrimSpace(readModelStringValue(input[key])); value != "" {
			ret = append(ret, value)
		}
	}
	return compactUniqueStrings(ret)
}

func taskBlockingTicketByStep(snapshot readmodelcore.RunSnapshot) map[string]string {
	ret := map[string]string{}
	for _, item := range snapshot.Decisions {
		if IsResolvedDecisionState(item.State) {
			continue
		}
		stepID := strings.TrimSpace(item.StepID)
		if stepID == "" {
			continue
		}
		ret[stepID] = item.ID
	}
	return ret
}

func firstNonEmptyMap(items ...map[string]any) map[string]any {
	for _, item := range items {
		if len(item) > 0 {
			return item
		}
	}
	return nil
}

func taskMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func firstNonNilValue(items ...any) any {
	for _, item := range items {
		if item != nil {
			return item
		}
	}
	return nil
}
