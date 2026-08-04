package activationcore

import (
	"strings"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func RequiredToolKinds(skills []workorchmanagementcontract.Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, kind := range skill.RequiredToolKinds {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			ret = append(ret, kind)
		}
	}
	return ret
}

func RequiredMCPServerRefs(skills []workorchmanagementcontract.Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, id := range skill.RequiredMCPServerRefs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ret = append(ret, id)
		}
	}
	return ret
}

func RequiredCapabilityRefs(skills []workorchmanagementcontract.Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, id := range skill.RequiredCapabilityRefs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ret = append(ret, id)
		}
	}
	return ret
}

func CapabilityCategories(items []*mcpmodel.Capability) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil {
			continue
		}
		category := strings.TrimSpace(item.Category)
		if category == "" {
			continue
		}
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		ret = append(ret, category)
	}
	return ret
}
