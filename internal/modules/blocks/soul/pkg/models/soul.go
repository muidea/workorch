package soul

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	SoulSourceUserManaged = "user_managed"
	SoulSourceBuiltinSeed = "builtin_seed"
)

func ValidSoulSourceProvenance(value string) bool {
	switch value {
	case SoulSourceUserManaged, SoulSourceBuiltinSeed:
		return true
	default:
		return false
	}
}

type Soul struct {
	ID                          common.SoulID
	Revision                    uint64
	ContentHash                 string
	SourceProvenance            string
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

func StableSoulContentHash(item *Soul) string {
	if item == nil {
		return ""
	}
	h := sha256.New()
	write := func(values ...string) {
		for _, value := range values {
			_, _ = fmt.Fprintf(h, "%08x%s", len(value), value)
		}
	}
	write("workorch-soul-content-v1", string(item.ID), item.Name, item.Description, item.SoulMD)
	for _, values := range [][]string{item.BusinessDomains, item.AllowedSkillRefs, item.AllowedMCPServerRefs, item.AllowedCapabilityRefs, item.AllowedCapabilityCategories, item.AllowedToolKinds} {
		write(values...)
	}
	return "soul-content-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func CloneSoul(item *Soul) *Soul {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.BusinessDomains = slices.Clone(item.BusinessDomains)
	cloned.AllowedSkillRefs = slices.Clone(item.AllowedSkillRefs)
	cloned.AllowedMCPServerRefs = slices.Clone(item.AllowedMCPServerRefs)
	cloned.AllowedCapabilityRefs = slices.Clone(item.AllowedCapabilityRefs)
	cloned.AllowedCapabilityCategories = slices.Clone(item.AllowedCapabilityCategories)
	cloned.AllowedToolKinds = slices.Clone(item.AllowedToolKinds)
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
