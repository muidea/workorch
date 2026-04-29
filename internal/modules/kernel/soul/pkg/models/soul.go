package soul

import "github.com/muidea/workorch/internal/pkg/common"

type Soul struct {
	ID                   common.SoulID
	Name                 string
	Description          string
	BusinessDomains      []string
	SoulMD               string
	AllowedSkillRefs     []string
	AllowedMCPServerRefs []string
	AllowedToolKinds     []string
}
