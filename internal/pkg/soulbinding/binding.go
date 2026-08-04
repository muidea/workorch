// Package soulbinding defines the owner-neutral value used to bind a workflow
// stage to a Soul. It owns no workspace, Soul, or Runtime state.
package soulbinding

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Binding is one immutable stage-to-Soul association.
type Binding struct {
	Stage  string
	SoulID common.SoulID
}

func FromSoulIDMap(input map[string]common.SoulID) []Binding {
	ret := make([]Binding, 0, len(input))
	for stage, soulID := range input {
		ret = append(ret, Binding{Stage: stage, SoulID: soulID})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Stage < ret[j].Stage })
	return ret
}

func FromStringMap(input map[string]string) []Binding {
	if input == nil {
		return nil
	}
	ret := make([]Binding, 0, len(input))
	for stage, soulID := range input {
		ret = append(ret, Binding{Stage: stage, SoulID: common.SoulID(soulID)})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Stage < ret[j].Stage })
	return ret
}

func Validate(input []Binding) error {
	seen := make(map[string]struct{}, len(input))
	for _, binding := range input {
		stage := strings.TrimSpace(binding.Stage)
		if stage == "" || binding.SoulID == "" {
			return common.ErrInvalidArgument
		}
		if _, ok := seen[stage]; ok {
			return common.ErrInvalidArgument
		}
		seen[stage] = struct{}{}
	}
	return nil
}

func SoulIDMap(input []Binding) map[string]common.SoulID {
	if input == nil {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for _, binding := range input {
		ret[binding.Stage] = binding.SoulID
	}
	return ret
}

func StringMap(input []Binding) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for _, binding := range input {
		ret[binding.Stage] = string(binding.SoulID)
	}
	return ret
}
