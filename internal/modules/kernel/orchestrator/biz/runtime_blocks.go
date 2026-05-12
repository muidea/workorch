package orchestrator

import (
	"context"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	decisionContextKeyScope                 = "scope"
	decisionContextScopeWorkorchRuntime     = "workorch_runtime"
	decisionContextScopeTargetWorkspace     = "target_workspace"
	decisionContextScopeExternalDependency  = "external_dependency"
	decisionContextKeyDependency            = "dependency"
	decisionContextDependencyLLMProvider    = "llm_provider"
	decisionContextKeyRecoveryPolicy        = "recoveryPolicy"
	decisionContextRecoveryPolicyConfigOnly = "system_config_required"
	decisionContextRecoveryPolicyRepairPlan = "runtime_repair_exhausted_requires_strategy"
	decisionContextKeyResultPolicy          = "resultPolicy"
	decisionContextResultPolicyNone         = "none"
	decisionContextResultPolicyRejected     = "rejected"
)

type runtimeDecisionBlockSpec struct {
	Reason       string
	TicketID     common.TicketID
	DecisionKind string
	Question     string
	Context      map[string]any
}

type runtimeApprovalBlockSpec struct {
	Reason     string
	ApprovalID common.ApprovalID
	Action     string
	Context    map[string]any
}

func decisionBlockRequiresRejectedResult(spec runtimeDecisionBlockSpec) bool {
	policy := strings.TrimSpace(stringFromMap(spec.Context, decisionContextKeyResultPolicy))
	switch policy {
	case decisionContextResultPolicyRejected:
		return true
	case decisionContextResultPolicyNone:
		return false
	}
	return strings.TrimSpace(stringFromMap(spec.Context, decisionContextKeyRecoveryPolicy)) == decisionContextRecoveryPolicyRepairPlan
}

func (o *RuntimeOrchestrator) recordRejectedResultForDecisionBlock(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
	spec runtimeDecisionBlockSpec,
) error {
	if !decisionBlockRequiresRejectedResult(spec) {
		return nil
	}
	return o.recordRejectedCompletionDoDAssessment(ctx, run, step, output)
}

func decisionBlockSpec(output map[string]any) runtimeDecisionBlockSpec {
	if output == nil {
		return runtimeDecisionBlockSpec{}
	}
	reason, _ := output[stepOutputKeyBlockDecisionReason].(string)
	ticketID, _ := output[stepOutputKeyBlockDecisionTicketID].(string)
	decisionKind, _ := output[stepOutputKeyBlockDecisionKind].(string)
	question, _ := output[stepOutputKeyBlockDecisionQuestion].(string)
	return runtimeDecisionBlockSpec{
		Reason:       reason,
		TicketID:     common.TicketID(ticketID),
		DecisionKind: decisionKind,
		Question:     question,
		Context:      mapFromValue(output[stepOutputKeyBlockDecisionContext]),
	}
}

func approvalBlockSpec(output map[string]any) runtimeApprovalBlockSpec {
	if output == nil {
		return runtimeApprovalBlockSpec{}
	}
	reason, _ := output[stepOutputKeyBlockApprovalReason].(string)
	approvalID, _ := output[stepOutputKeyBlockApprovalID].(string)
	action, _ := output[stepOutputKeyBlockApprovalAction].(string)
	return runtimeApprovalBlockSpec{
		Reason:     reason,
		ApprovalID: common.ApprovalID(approvalID),
		Action:     action,
		Context:    mapFromValue(output[stepOutputKeyBlockApprovalContext]),
	}
}

func assessmentOnlyDecisionShouldRepair(step *runModel.Step, spec runtimeDecisionBlockSpec) bool {
	if step == nil || spec.Reason == "" {
		return false
	}
	if !boolValue(step.Input["providerRetryAssessmentOnly"]) &&
		!boolValue(step.Input[stepInputKeyAssessmentOnly]) &&
		!boolValue(step.Input[stepInputKeyGraphLimitFollowUp]) {
		return false
	}
	if strings.TrimSpace(spec.DecisionKind) != "" &&
		strings.TrimSpace(spec.DecisionKind) != decisioncommon.ResolutionTypeProvideMissingInfo {
		return false
	}
	if continuableCandidateContext(spec.Context) {
		return true
	}
	for _, field := range stringListFromValue(spec.Context["missingFields"]) {
		if isExecutionTargetField(field) {
			return true
		}
	}
	for _, field := range stringListFromValue(spec.Context["requiresHumanFields"]) {
		if isExecutionTargetField(field) {
			return true
		}
	}
	return false
}

func continuableDecisionShouldRepair(step *runModel.Step, spec runtimeDecisionBlockSpec) bool {
	if step == nil || spec.Reason == "" {
		return false
	}
	if strings.TrimSpace(spec.DecisionKind) != "" &&
		strings.TrimSpace(spec.DecisionKind) != decisioncommon.ResolutionTypeProvideMissingInfo {
		return false
	}
	return continuableCandidateContext(spec.Context)
}

func continuableCandidateContext(context map[string]any) bool {
	candidates := mapFromValue(context["candidateValues"])
	for _, value := range candidates {
		if continuableCandidateValue(value) {
			return true
		}
	}
	return false
}

func continuableCandidateValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return isContinuableCandidateText(typed)
	case []string:
		for _, item := range typed {
			if isContinuableCandidateText(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if continuableCandidateValue(item) {
				return true
			}
		}
	case map[string]any:
		for _, nested := range typed {
			if continuableCandidateValue(nested) {
				return true
			}
		}
	}
	return false
}

func isContinuableCandidateText(value string) bool {
	normalized := normalizedDecisionRepairText(value)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"init", "initialize", "bootstrap", "fromscratch", "newproject", "currentworkspace", "currentdirectory",
		"初始化", "从零", "新项目", "当前工作区", "当前目录",
	} {
		if strings.Contains(normalized, normalizedDecisionRepairText(marker)) {
			return true
		}
	}
	return false
}

func isExecutionTargetField(value string) bool {
	normalized := normalizedDecisionRepairText(value)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{"executiontarget", "targetmode", "workspacepath", "projectlocation", "projectcontent", "workingdirectory", "执行目标", "项目路径", "工作目录"} {
		if strings.Contains(normalized, normalizedDecisionRepairText(marker)) {
			return true
		}
	}
	return false
}

func normalizedDecisionRepairText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "_", "", "/", "", "\\", "", ".", "")
	return replacer.Replace(value)
}

func stringListFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				ret = append(ret, item)
			}
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				ret = append(ret, strings.TrimSpace(text))
			}
		}
		return ret
	default:
		return nil
	}
}

func assessmentOnlyDecisionRepairAssessment(run *runModel.Run, step *runModel.Step, spec runtimeDecisionBlockSpec) map[string]any {
	items := assessmentRepairDoDItems(run, step, spec)
	return map[string]any{
		"satisfied":             false,
		"summary":               "The assessment-only step requested a decision for work that can continue through runtime repair or replan.",
		"goalOutcome":           "The requested work is incomplete and should continue through automated implementation or verification work.",
		"implementationSummary": "No implementation evidence was produced before the decision request.",
		"verificationSummary":   "Current evidence is insufficient for DoD satisfaction.",
		"items":                 items,
		"residualRisks": []string{
			"The requested work has not yet produced implementation or verification evidence.",
		},
		"nextActions": []string{
			"Replan the missing implementation or bootstrap work using the resolved Goal and DoD.",
			"Run concrete verification after implementation and produce evidence refs for every DoD item.",
		},
	}
}

func assessmentOnlyContractViolationRepairAssessment(run *runModel.Run, step *runModel.Step, reason string) map[string]any {
	items := assessmentRepairDoDItems(run, step, runtimeDecisionBlockSpec{Reason: reason})
	return map[string]any{
		"satisfied":             false,
		"summary":               "The assessment-only step returned executable work after a contract correction request.",
		"goalOutcome":           "The requested work is incomplete and must continue through automated repair or replan.",
		"implementationSummary": "No executable graph from the assessment-only response was applied.",
		"verificationSummary":   "Current evidence is insufficient for DoD satisfaction because the validation response did not provide a valid assessment.",
		"items":                 items,
		"residualRisks": []string{
			"The validation model attempted to plan more executable work instead of evaluating the completed evidence.",
		},
		"nextActions": []string{
			"Continue through runtime repair with concrete implementation or verification steps.",
			"Return a complete item-level DoD assessment once evidence is available.",
		},
	}
}

func assessmentRepairDoDItems(run *runModel.Run, step *runModel.Step, spec runtimeDecisionBlockSpec) []map[string]any {
	names := effectiveDoDItemTexts(run)
	if len(names) == 0 {
		names = []string{"effective DoD"}
	}
	ret := make([]map[string]any, 0, len(names))
	evidenceRef := ""
	if step != nil {
		evidenceRef = string(step.ID)
	}
	for _, name := range names {
		item := map[string]any{
			"item":   name,
			"state":  "failed",
			"reason": firstNonEmptyString(spec.Reason, "Assessment-only decision did not produce implementation or verification evidence."),
			"impact": "The runtime cannot mark this DoD item satisfied without concrete execution evidence.",
		}
		if evidenceRef != "" {
			item["evidenceRefs"] = []string{evidenceRef}
		}
		ret = append(ret, item)
	}
	return ret
}

func effectiveDoDItemTexts(run *runModel.Run) []string {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	dod := mapFromValue(run.ExecutionContext["dod"])
	return stringListFromValue(dod["items"])
}

func (o *RuntimeOrchestrator) createDecisionTicket(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	spec runtimeDecisionBlockSpec,
) error {
	if run == nil || step == nil || spec.TicketID == "" || spec.Reason == "" {
		return common.ErrInvalidArgument
	}
	if o.tickets == nil {
		return common.ErrInvalidArgument
	}

	ticket := &decisionmodel.Ticket{
		ID:           spec.TicketID,
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		StepID:       step.ID,
		Reason:       common.RedactSensitiveString(spec.Reason),
		DecisionKind: spec.DecisionKind,
		Question:     common.RedactSensitiveString(spec.Question),
		Context:      common.RedactSensitiveMap(cloneStepPayload(spec.Context)),
		State:        common.DecisionTicketStateOpen,
	}
	if err := o.tickets.Create(ctx, ticket); err != nil {
		return err
	}
	if o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicTicketCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(ticket.ID),
		WorkspaceID:  string(ticket.WorkspaceID),
		RunID:        string(ticket.RunID),
		StepID:       string(ticket.StepID),
		Payload:      events.DecisionTicketPayload(ticket.Reason, ticket.State),
	})
}

func (o *RuntimeOrchestrator) createApproval(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	approvalID common.ApprovalID,
	reason string,
	action string,
	approvalContext map[string]any,
) error {
	if run == nil || step == nil || approvalID == "" || reason == "" {
		return common.ErrInvalidArgument
	}
	if o.approvals == nil {
		return common.ErrInvalidArgument
	}

	approval := &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      step.ID,
		Reason:      common.RedactSensitiveString(reason),
		Action:      common.RedactSensitiveString(action),
		Context:     common.RedactSensitiveMap(cloneStepPayload(approvalContext)),
		State:       common.ApprovalStatePending,
	}
	if err := o.approvals.Create(ctx, approval); err != nil {
		return err
	}
	if o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicApprovalCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(approval.ID),
		WorkspaceID:  string(approval.WorkspaceID),
		RunID:        string(approval.RunID),
		StepID:       string(approval.StepID),
		Payload:      events.ApprovalPayload(approval.Reason, approval.Action, approval.Context, approval.State),
	})
}

func decisionResolutionInput(ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) map[string]any {
	if ticket == nil || resolution == nil {
		return nil
	}
	return map[string]any{
		stepInputKeySoulStage: soulStageDecision,
		"decisionResolution": map[string]any{
			"ticketId":          string(ticket.ID),
			"stepId":            string(ticket.StepID),
			"decisionKind":      ticket.DecisionKind,
			"question":          ticket.Question,
			"reason":            ticket.Reason,
			"resolutionId":      string(resolution.ID),
			"resolutionType":    resolution.Type,
			"resolutionPayload": cloneStepPayload(resolution.Payload),
		},
	}
}

func effectiveDecisionResolutionType(ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) string {
	if resolution == nil {
		return ""
	}
	resolutionType := strings.TrimSpace(resolution.Type)
	if ticket == nil || ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		return resolutionType
	}
	switch strings.TrimSpace(stringFromMap(resolution.Payload, "recoveryAction")) {
	case "replan":
		return decisioncommon.ResolutionTypeReplan
	case "terminate":
		return decisioncommon.ResolutionTypeRejectCurrentPath
	default:
		return resolutionType
	}
}

func approvalResolutionInput(approval *approvalmodel.Approval) map[string]any {
	if approval == nil {
		return nil
	}
	return map[string]any{
		stepInputKeySoulStage: soulStageApproval,
		"approvalResolution": map[string]any{
			"approvalId": string(approval.ID),
			"stepId":     string(approval.StepID),
			"reason":     approval.Reason,
			"action":     approval.Action,
			"context":    cloneStepPayload(approval.Context),
			"state":      string(approval.State),
		},
	}
}
