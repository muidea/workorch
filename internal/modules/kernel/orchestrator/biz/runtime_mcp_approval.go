package orchestrator

import (
	"context"
	"fmt"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) blockMCPToolCallForOperatorApproval(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	workspace *workspaceModel.Workspace,
) (bool, error) {
	if run == nil || step == nil || strings.TrimSpace(step.Kind) != "mcp/tool_call" {
		return false, nil
	}
	if o.approvals == nil {
		return false, nil
	}
	serverID := strings.TrimSpace(stringFromMap(step.Input, "serverId"))
	tool := strings.TrimSpace(stringFromMap(step.Input, "tool"))
	if serverID == "" || tool == "" || !mcpServerRequiresOperatorApproval(run.ExecutionContext, serverID, tool) {
		return false, nil
	}
	if mcpStepHasApprovedOperatorApproval(ctx, o.approvals, step) {
		return false, nil
	}
	approvalID := common.ApprovalID(runtimeChildID(step.ID, "mcp-operator-approval"))
	existing, err := o.approvals.Get(ctx, approvalID)
	if err == nil && existing != nil {
		return true, o.blockOnApprovalAtStep(ctx, run, workspace, approvalID, step.ID)
	}
	if err != nil && err != common.ErrNotFound {
		return true, err
	}
	reason := fmt.Sprintf("MCP tool %s on server %s requires operator approval before execution.", tool, serverID)
	action := fmt.Sprintf("Approve MCP tool call %s", tool)
	approvalContext := map[string]any{
		"approvalPolicy": "operator",
		"operation":      "mcp/tool_call",
		"serverId":       serverID,
		"tool":           tool,
		"stepId":         string(step.ID),
		"preflight":      true,
	}
	if err := o.createApproval(ctx, run, step, approvalID, reason, action, approvalContext); err != nil {
		return true, err
	}
	return true, o.blockOnApprovalAtStep(ctx, run, workspace, approvalID, step.ID)
}

func mcpStepHasApprovedOperatorApproval(ctx context.Context, approvals approvalReader, step *runModel.Step) bool {
	if approvals == nil || step == nil {
		return false
	}
	if approvalID := common.ApprovalID(strings.TrimSpace(stringFromMap(step.Input, "approvalId"))); approvalID != "" {
		if approval, err := approvals.Get(ctx, approvalID); err == nil && approval != nil && approval.State == common.ApprovalStateApproved {
			return true
		}
	}
	resolution := mapFromValue(step.Input["approvalResolution"])
	if strings.TrimSpace(stringFromMap(resolution, "state")) == string(common.ApprovalStateApproved) {
		context := mapFromValue(resolution["context"])
		if strings.TrimSpace(stringFromMap(context, "approvalPolicy")) == "operator" &&
			strings.TrimSpace(stringFromMap(context, "operation")) == "mcp/tool_call" {
			return true
		}
	}
	return false
}

type approvalReader interface {
	Get(context.Context, common.ApprovalID) (*approvalmodel.Approval, error)
}

func mcpServerRequiresOperatorApproval(executionContext map[string]any, serverID, tool string) bool {
	if strings.TrimSpace(serverID) == "" {
		return false
	}
	for _, server := range mapSliceFromValue(executionContext["mcpServers"]) {
		if strings.TrimSpace(stringFromMap(server, "id")) != serverID {
			continue
		}
		if len(stringSliceFromValue(server["allowedTools"])) > 0 && !stringSliceContains(stringSliceFromValue(server["allowedTools"]), tool) {
			return false
		}
		return normalizedMCPApprovalPolicy(stringFromMap(server, "defaultApprovalPolicy")) == "operator"
	}
	for _, capability := range mapSliceFromValue(executionContext["mcpCapabilities"]) {
		if strings.TrimSpace(stringFromMap(capability, "serverId")) != serverID {
			continue
		}
		if strings.TrimSpace(stringFromMap(capability, "name")) != "" && strings.TrimSpace(stringFromMap(capability, "name")) != tool {
			continue
		}
		if normalizedMCPApprovalPolicy(stringFromMap(capability, "approvalPolicy")) == "operator" {
			return true
		}
	}
	return false
}

func normalizedMCPApprovalPolicy(policy string) string {
	policy = strings.ToLower(strings.TrimSpace(policy))
	policy = strings.ReplaceAll(policy, "-", "_")
	policy = strings.ReplaceAll(policy, " ", "_")
	switch policy {
	case "operator", "human", "manual", "requires_operator", "operator_required":
		return "operator"
	default:
		return policy
	}
}
