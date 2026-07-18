package mcp

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	CapabilityKindTool     = "tool"
	CapabilityKindResource = "resource"
	CapabilityKindPrompt   = "prompt"
)

type Capability struct {
	ID            common.MCPCapabilityID `json:"id"`
	ServerID      common.MCPServerID     `json:"serverId"`
	Kind          string                 `json:"kind"`
	Name          string                 `json:"name"`
	DisplayName   string                 `json:"displayName"`
	Description   string                 `json:"description,omitempty"`
	Category      string                 `json:"category,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	UsageHint     string                 `json:"usageHint,omitempty"`
	InputSummary  string                 `json:"inputSummary,omitempty"`
	OutputSummary string                 `json:"outputSummary,omitempty"`
	StageHints    []string               `json:"stageHints,omitempty"`
	RiskLevel     string                 `json:"riskLevel,omitempty"`
	// EffectScope and IdempotencyClass are capability-owner declarations used when
	// activation seals a concrete mcp/tool_call. They are not inferred from the
	// advisory RiskLevel or tool name by Runtime V2.
	EffectScope      string                       `json:"effectScope,omitempty"`
	IdempotencyClass string                       `json:"idempotencyClass,omitempty"`
	Schema           workorcheventpayload.Payload `json:"schema,omitempty"`
	URI              string                       `json:"uri,omitempty"`
	Enabled          bool                         `json:"enabled"`
	ApprovalPolicy   string                       `json:"approvalPolicy,omitempty"`
	Status           string                       `json:"status,omitempty"`
}
