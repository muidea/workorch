package soul

import "github.com/muidea/workorch/internal/pkg/common"

type Soul struct {
	ID                   common.SoulID
	Name                 string
	Description          string
	BusinessDomains      []string
	SystemInstruction    string
	PlanningPolicy       string
	DecisionPolicy       string
	ApprovalPolicy       string
	ValidationPolicy     string
	AllowedSkillRefs     []string
	AllowedMCPServerRefs []string
	AllowedToolKinds     []string
	OutputContract       string
}
