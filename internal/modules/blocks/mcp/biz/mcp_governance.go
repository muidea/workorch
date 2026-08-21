package mcp

import (
	"fmt"
	"strings"

	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mcpApprovalRequirement(operation runtimecontract.RuntimeOperation, operationHash string, sealed mcpevents.RuntimeExecutionAuthorization, authorized authorizedRuntimeOperation) (*runtimecontract.RuntimeGovernanceRequirement, error) {
	if authorized.server == nil || authorized.capability == nil {
		return nil, fmt.Errorf("runtime MCP governance requires an authorized capability")
	}
	policy := strings.ToLower(strings.TrimSpace(authorized.capability.ApprovalPolicy))
	if policy == "" {
		policy = strings.ToLower(strings.TrimSpace(authorized.server.DefaultApprovalPolicy))
	}
	switch policy {
	case "", "auto", "none", "not_required", "not-required":
		return nil, nil
	}
	risk := strings.TrimSpace(authorized.capability.RiskLevel)
	if risk == "" {
		risk = "external"
	}
	value := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: string(authorized.capability.ID),
		Reason:          "MCP capability policy requires explicit approval before external execution",
		RequestedAction: operation.Kind, RiskLevel: risk,
		AuthorizationContext: workorcheventpayload.New(
			workorcheventpayload.StringField("capabilityId", string(authorized.capability.ID)),
			workorcheventpayload.StringField("serverId", string(authorized.server.ID)),
			workorcheventpayload.StringField("operationKind", operation.Kind),
			workorcheventpayload.StringField("operationHash", strings.TrimSpace(operationHash)),
			workorcheventpayload.StringField("catalogSnapshotId", strings.TrimSpace(sealed.CatalogSnapshotID)),
			workorcheventpayload.StringField("registryRevision", strings.TrimSpace(sealed.RegistryRevision)),
			workorcheventpayload.StringField("capabilityRevision", fmt.Sprintf("%d", authorized.capability.Revision)),
			workorcheventpayload.StringField("approvalPolicy", policy),
			workorcheventpayload.StringField("effectScope", string(sealed.Policy.EffectScope)),
			workorcheventpayload.StringField("idempotencyClass", string(sealed.Policy.IdempotencyClass)),
		),
	}.Normalized()
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("runtime MCP approval requirement: %w", err)
	}
	return &value, nil
}

func validateMCPApprovalGrant(grant *runtimecontract.RuntimeOperationGovernanceGrant, requirement runtimecontract.RuntimeGovernanceRequirement) error {
	requirement = requirement.Normalized()
	if err := requirement.Validate(); err != nil {
		return err
	}
	if grant == nil {
		return fmt.Errorf("runtime MCP approval grant is required")
	}
	value := grant.Normalized()
	if value.Validate() != nil || value.ApprovalID == "" || value.TicketID != "" ||
		value.DependencyKind != string(requirement.Kind) || value.SubjectRef != requirement.SubjectRef ||
		value.RequestedAction != requirement.RequestedAction || value.RiskLevel != requirement.RiskLevel ||
		value.AuthorizationHash != runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext) {
		return fmt.Errorf("runtime MCP approval grant does not match sealed capability authorization")
	}
	return nil
}
