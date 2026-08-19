package llm

import (
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Soul allowlists constrain provider-visible activation context for the
// current stage. They do not create Runtime operation authority: the sealed
// manifest and capability-owner facts remain the only dispatch authority.
func runtimeCandidateSkillsForSoul(values []runtimecontract.RuntimeSkillContext, soul *runtimecontract.RuntimeSoulContext) []runtimecontract.RuntimeSkillContext {
	if soul == nil {
		return append([]runtimecontract.RuntimeSkillContext(nil), values...)
	}
	ret := make([]runtimecontract.RuntimeSkillContext, 0, len(values))
	for _, value := range values {
		if !runtimeSoulAllowsValue(string(value.ID), soul.AllowedSkillRefs) ||
			!runtimeSoulAllowsValues(value.RequiredToolKinds, soul.AllowedToolKinds) ||
			!runtimeSoulAllowsValues(value.RequiredMCPServerRefs, soul.AllowedMCPServerRefs) ||
			!runtimeSoulAllowsValues(value.RequiredCapabilityRefs, soul.AllowedCapabilityRefs) {
			continue
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCandidateMCPCapabilitiesForSoul(values []runtimecontract.RuntimeMCPCapabilityContext, soul *runtimecontract.RuntimeSoulContext) []runtimecontract.RuntimeMCPCapabilityContext {
	if soul == nil {
		return append([]runtimecontract.RuntimeMCPCapabilityContext(nil), values...)
	}
	ret := make([]runtimecontract.RuntimeMCPCapabilityContext, 0, len(values))
	for _, value := range values {
		if !runtimeSoulAllowsValue(string(value.ServerID), soul.AllowedMCPServerRefs) ||
			!runtimeSoulAllowsValue(string(value.ID), soul.AllowedCapabilityRefs) ||
			!runtimeSoulAllowsValue(value.Category, soul.AllowedCapabilityCategories) ||
			!runtimeSoulAllowsValue(value.OperationKind, soul.AllowedToolKinds) {
			continue
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCandidateMCPServerIDsForSoul(values []common.MCPServerID, soul *runtimecontract.RuntimeSoulContext) []common.MCPServerID {
	if soul == nil {
		return append([]common.MCPServerID(nil), values...)
	}
	ret := make([]common.MCPServerID, 0, len(values))
	for _, value := range values {
		if runtimeSoulAllowsValue(string(value), soul.AllowedMCPServerRefs) {
			ret = append(ret, value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeSoulAllowsValues(values []string, allowed []string) bool {
	for _, value := range values {
		if !runtimeSoulAllowsValue(value, allowed) {
			return false
		}
	}
	return true
}

func runtimeSoulAllowsValue(value string, allowed []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return slices.Contains(allowed, "*") || slices.Contains(allowed, value)
}

func runtimeCandidateSkillBudget(values []runtimecontract.RuntimeSkillContext, sealed *runtimecontract.RuntimeSkillContextBudget) *runtimecontract.RuntimeSkillContextBudget {
	if len(values) == 0 || sealed == nil {
		return nil
	}
	ret := *sealed
	ret.OriginalInstructionBytes = 0
	ret.IncludedInstructionBytes = 0
	ret.MetadataBytes = 0
	ret.TruncatedSkills = 0
	for _, value := range values {
		ret.OriginalInstructionBytes += value.OriginalInstructionBytes
		ret.IncludedInstructionBytes += len(value.Instruction)
		ret.MetadataBytes += runtimeCandidateSkillMetadataBytes(value)
		if value.InstructionTruncated {
			ret.TruncatedSkills++
		}
	}
	return &ret
}

func runtimeCandidateSkillMetadataBytes(value runtimecontract.RuntimeSkillContext) int {
	total := len(value.ID) + len(value.Name) + len(value.Version) + len(value.Description)
	for _, values := range [][]string{value.TriggerRules, value.ReferenceRefs, value.RequiredToolKinds, value.RequiredMCPServerRefs, value.RequiredCapabilityRefs, value.Constraints} {
		for _, item := range values {
			total += len(item)
		}
	}
	provenance := value.Provenance.Normalized()
	return total + len(provenance.Source) + len(provenance.Scope) + len(provenance.WorkspaceID) + len(provenance.ContentHash)
}
