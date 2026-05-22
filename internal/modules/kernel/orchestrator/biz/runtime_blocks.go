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
	decisionContextDependencyContextGate    = "runtime_context_gate"
	decisionContextKeyRecoveryPolicy        = "recoveryPolicy"
	decisionContextRecoveryPolicyConfigOnly = "system_config_required"
	decisionContextRecoveryPolicyRepairPlan = "runtime_repair_exhausted_requires_strategy"
	decisionContextRecoveryPolicyContext    = "runtime_context_required"
	decisionContextKeyResultPolicy          = "resultPolicy"
	decisionContextResultPolicyNone         = "none"
	decisionContextResultPolicyRejected     = "rejected"
	decisionContextKeyQuestion              = "decisionQuestion"
	decisionContextKeyReason                = "decisionReason"
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
	return continuableCandidateContext(spec.Context) ||
		knownReadableWorkspaceResourceDecisionSpec(spec) ||
		mechanicallyRepairableDecisionShouldRepair(step, spec)
}

func continuableCandidateContext(context map[string]any) bool {
	for _, key := range []string{"runtimeCanContinue", "continuableByRuntime", "workspaceActionable"} {
		if boolValue(context[key]) {
			return true
		}
	}
	return false
}

func mechanicallyRepairableDecisionShouldRepair(step *runModel.Step, spec runtimeDecisionBlockSpec) bool {
	if !stateChangingRepairContractActive(step) {
		return false
	}
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	if !repairActionPackageRequiresStateChangingOutput(actionPackage) {
		return false
	}
	if !stateChangingRepairHasConcreteTargetEvidence(step.Input, actionPackage) {
		return false
	}
	if decisionBlockDeclaresExternalOrRuntimeDependency(spec.Context) {
		return false
	}
	return true
}

func decisionBlockDeclaresExternalOrRuntimeDependency(context map[string]any) bool {
	switch strings.TrimSpace(stringFromMap(context, decisionContextKeyScope)) {
	case decisionContextScopeExternalDependency, decisionContextScopeWorkorchRuntime:
		return true
	}
	switch strings.TrimSpace(stringFromMap(context, decisionContextKeyDependency)) {
	case decisionContextDependencyLLMProvider, decisionContextDependencyContextGate:
		return true
	}
	switch strings.TrimSpace(stringFromMap(context, decisionContextKeyRecoveryPolicy)) {
	case decisionContextRecoveryPolicyConfigOnly, decisionContextRecoveryPolicyContext, decisionContextRecoveryPolicyRepairPlan:
		return true
	}
	return decisionBlockRequiresRejectedResult(runtimeDecisionBlockSpec{Context: context})
}

func knownReadableWorkspaceResourceDecisionContext(context map[string]any) bool {
	resources := knownReadableWorkspaceResourceNames(context)
	if len(resources) == 0 {
		return false
	}
	for _, text := range readableResourceReferenceTexts(context) {
		if decisionTextReferencesKnownResource(text, resources) {
			return true
		}
	}
	return false
}

func knownReadableWorkspaceResourceDecisionSpec(spec runtimeDecisionBlockSpec) bool {
	context := cloneStepPayload(spec.Context)
	if context == nil {
		context = map[string]any{}
	}
	if strings.TrimSpace(spec.Question) != "" {
		context[decisionContextKeyQuestion] = spec.Question
	}
	if strings.TrimSpace(spec.Reason) != "" {
		context[decisionContextKeyReason] = spec.Reason
	}
	return knownReadableWorkspaceResourceDecisionContext(context)
}

func knownReadableWorkspaceResourceNames(context map[string]any) []string {
	if len(context) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = strings.TrimSpace(strings.TrimPrefix(value, "./"))
		if !isDecisionKnownResourceName(value) || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	collect := func(source map[string]any) {
		for _, item := range stringListFromValue(source["workspaceFiles"]) {
			add(item)
		}
		for _, item := range stringListFromValue(source["existingFiles"]) {
			add(item)
		}
		for _, item := range stringListFromValue(source["readableResources"]) {
			add(item)
		}
		for _, item := range stringListFromValue(source["knownResources"]) {
			add(item)
		}
		for _, item := range mapSliceFromValue(source[stepInputKeyWorkspaceFileManifest]) {
			add(stringFromMap(item, "path"))
		}
	}
	collect(context)
	collect(mapFromValue(context["knownValues"]))
	return ret
}

func isDecisionKnownResourceName(value string) bool {
	if value == "" || strings.Contains(value, "..") {
		return false
	}
	if strings.ContainsAny(value, "/\\.") {
		return true
	}
	return len([]rune(value)) >= 6
}

func decisionFieldReferencesKnownResource(field string, resources []string) bool {
	return decisionTextReferencesKnownResource(field, resources)
}

func decisionTextReferencesKnownResource(text string, resources []string) bool {
	normalizedText := normalizedDecisionRepairText(text)
	if normalizedText == "" {
		return false
	}
	for _, resource := range resources {
		normalizedResource := normalizedDecisionRepairText(resource)
		if normalizedResource == "" || len(normalizedResource) < 4 {
			continue
		}
		if strings.Contains(normalizedText, normalizedResource) {
			return true
		}
	}
	return false
}

func readableResourceReferenceTexts(context map[string]any) []string {
	if len(context) == 0 {
		return nil
	}
	ret := append(stringListFromValue(context["missingFields"]), stringListFromValue(context["requiresHumanFields"])...)
	for _, key := range []string{decisionContextKeyQuestion, decisionContextKeyReason, "question", "reason"} {
		if text := strings.TrimSpace(stringFromMap(context, key)); text != "" {
			ret = append(ret, text)
		}
	}
	return ret
}

func convertedDecisionPayload(spec runtimeDecisionBlockSpec) map[string]any {
	payload := map[string]any{
		"reason":       spec.Reason,
		"decisionKind": spec.DecisionKind,
		"question":     spec.Question,
	}
	if spec.TicketID != "" {
		payload["ticketId"] = string(spec.TicketID)
	}
	if len(spec.Context) > 0 {
		payload["context"] = cloneStepPayload(spec.Context)
	}
	return payload
}

func decisionOutputFromSpec(spec runtimeDecisionBlockSpec) map[string]any {
	return map[string]any{
		stepOutputKeyBlockDecisionReason:   spec.Reason,
		stepOutputKeyBlockDecisionTicketID: string(spec.TicketID),
		stepOutputKeyBlockDecisionKind:     spec.DecisionKind,
		stepOutputKeyBlockDecisionQuestion: spec.Question,
		stepOutputKeyBlockDecisionContext:  cloneStepPayload(spec.Context),
	}
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
		"goalOutcome":           "The requested work is incomplete and should continue through automated state-changing or verification work.",
		"implementationSummary": "No state-changing or deliverable evidence was produced before the decision request.",
		"verificationSummary":   "Current evidence is insufficient for DoD satisfaction.",
		"items":                 items,
		"residualRisks": []string{
			"The requested work has not yet produced state-changing, deliverable, or verification evidence.",
		},
		"nextActions": []string{
			"Replan the missing state-changing or deliverable work using the resolved Goal and DoD.",
			"Run concrete verification after the required state change and produce evidence refs for every DoD item.",
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
			"Continue through runtime repair with concrete state-changing or verification steps.",
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
			"reason": firstNonEmptyString(spec.Reason, "Assessment-only decision did not produce state-changing or verification evidence."),
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
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, map[string]any{"llmOutput": output})
	if !ShouldCreateDecisionTicket(step.Input, output, contract) {
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
	ret := map[string]any{
		stepInputKeySoulStage:    soulStageDecision,
		stepInputKeyRuntimeStage: runtimeStageDecision,
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
		"ticketContext": cloneStepPayload(ticket.Context),
	}
	if action := strings.TrimSpace(stringFromMap(resolution.Payload, "recoveryAction")); action != "" {
		ret["decisionRecoveryAction"] = action
		if action == "retry_same_step" && ticket.Context[decisionContextKeyRecoveryPolicy] == decisionContextRecoveryPolicyConfigOnly {
			ret["providerRetryDecisionRetry"] = true
		}
	}
	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": ret}, map[string]any{"ticketContext": ticket.Context, "resolution": resolution.Payload})
	ret = ApplyConvergenceContract(ret, contract)
	return ret
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
		stepInputKeySoulStage:    soulStageApproval,
		stepInputKeyRuntimeStage: runtimeStageApproval,
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
