package soul

import "github.com/muidea/workorch/internal/pkg/common"

type Soul struct {
	ID                          common.SoulID
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

func CloneSoul(item *Soul) *Soul {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.BusinessDomains = append([]string(nil), item.BusinessDomains...)
	cloned.AllowedSkillRefs = append([]string(nil), item.AllowedSkillRefs...)
	cloned.AllowedMCPServerRefs = append([]string(nil), item.AllowedMCPServerRefs...)
	cloned.AllowedCapabilityRefs = append([]string(nil), item.AllowedCapabilityRefs...)
	cloned.AllowedCapabilityCategories = append([]string(nil), item.AllowedCapabilityCategories...)
	cloned.AllowedToolKinds = append([]string(nil), item.AllowedToolKinds...)
	return &cloned
}

func CloneSouls(items []*Soul) []*Soul {
	if items == nil {
		return nil
	}
	ret := make([]*Soul, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneSoul(item))
	}
	return ret
}
