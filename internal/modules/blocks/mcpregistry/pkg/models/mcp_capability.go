package mcp

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	CapabilityKindTool     = "tool"
	CapabilityKindResource = "resource"
	CapabilityKindPrompt   = "prompt"

	ValidationProofApplicabilityDirect      = "direct"
	ValidationProofApplicabilityAuxiliary   = "auxiliary"
	ValidationProofApplicabilityNotDeclared = "not_declared"

	CapabilitySourceUser            = "user"
	CapabilitySourceBuiltin         = "builtin"
	CapabilitySourceServerAllowlist = "server_allowlist"
)

func ValidValidationProofApplicability(value string) bool {
	switch value {
	case ValidationProofApplicabilityDirect, ValidationProofApplicabilityAuxiliary, ValidationProofApplicabilityNotDeclared:
		return true
	default:
		return false
	}
}

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
	// advisory RiskLevel or tool name by Runtime.
	EffectScope      string `json:"effectScope,omitempty"`
	IdempotencyClass string `json:"idempotencyClass,omitempty"`
	// ValidationProofApplicability is an explicit owner declaration. Runtime
	// never derives direct-proof authority from Name, Category, tags or prose.
	ValidationProofApplicability string                       `json:"validationProofApplicability,omitempty"`
	Schema                       workorcheventpayload.Payload `json:"schema,omitempty"`
	URI                          string                       `json:"uri,omitempty"`
	Enabled                      bool                         `json:"enabled"`
	ApprovalPolicy               string                       `json:"approvalPolicy,omitempty"`
	Source                       string                       `json:"source"`
	SourceRef                    string                       `json:"sourceRef"`
	Revision                     uint64                       `json:"revision"`
}

// CloneCapability returns a detached owner value suitable for crossing a
// read-result boundary. Capability is already the canonical immutable value;
// callers must not receive its mutable slices or payload storage by reference.
func CloneCapability(item *Capability) *Capability {
	if item == nil {
		return nil
	}
	ret := *item
	ret.Tags = append([]string(nil), item.Tags...)
	ret.StageHints = append([]string(nil), item.StageHints...)
	ret.Schema = item.Schema.Clone()
	return &ret
}

func CloneCapabilities(items []*Capability) []*Capability {
	ret := make([]*Capability, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneCapability(item))
	}
	return ret
}
