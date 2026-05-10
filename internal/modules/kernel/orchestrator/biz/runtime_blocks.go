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
