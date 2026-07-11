package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type CreateSoulCommand struct {
	SoulID                      common.SoulID
	Name                        string
	Description                 string
	BusinessDomains             []string
	SoulMD                      string
	AllowedSkillRefs            []string
	AllowedMCPServerRefs        []string
	AllowedCapabilityRefs       []string
	AllowedCapabilityCategories []string
	AllowedToolKinds            []string
}

type UpdateSoulCommand = CreateSoulCommand

type DeleteSoulCommand struct {
	SoulID common.SoulID
}

type ValidateCapabilitySelectionCommand struct {
	SoulID               common.SoulID
	SkillRefs            []string
	MCPServerRefs        []string
	CapabilityRefs       []string
	CapabilityCategories []string
	ToolKinds            []string
}
