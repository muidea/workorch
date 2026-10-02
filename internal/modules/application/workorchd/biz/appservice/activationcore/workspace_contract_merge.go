package activationcore

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func MergeTemplateWorkspaceContextHintsForExecutionContract(templateHints map[string]any, workspaceHints map[string]any) map[string]any {
	return runtimeprofiledecode.MergeTemplateWorkspaceContextHintsForExecutionContract(templateHints, workspaceHints)
}

func MergeTemplateWorkspaceProfileContracts(templateProfiles map[string]any, workspaceProfiles map[string]any) map[string]any {
	return runtimeprofiledecode.MergeTemplateWorkspaceProfileContracts(templateProfiles, workspaceProfiles)
}

// MergeTemplateWorkspaceEvidencePolicies preserves the template policies while
// letting a workspace-owned declaration replace a policy with the same ID.
// The result is deterministic and retains policies without an ID in their
// original owner order. It is a pure configuration merge: no Run, activation,
// or Runtime state is read or changed here.
func MergeTemplateWorkspaceEvidencePolicies(templatePolicies, workspacePolicies []workorcheventpayload.Payload) []workorcheventpayload.Payload {
	merged := make(map[string]workorcheventpayload.Payload, len(templatePolicies)+len(workspacePolicies))
	unnamed := make([]workorcheventpayload.Payload, 0, len(templatePolicies)+len(workspacePolicies))
	for _, policy := range append(workorcheventpayload.CloneSlice(templatePolicies), workorcheventpayload.CloneSlice(workspacePolicies)...) {
		id, _ := policy.String("id")
		id = strings.TrimSpace(id)
		if id == "" {
			unnamed = append(unnamed, policy.Clone())
			continue
		}
		merged[id] = policy.Clone()
	}
	ids := make([]string, 0, len(merged))
	for id := range merged {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]workorcheventpayload.Payload, 0, len(ids)+len(unnamed))
	for _, id := range ids {
		ret = append(ret, merged[id].Clone())
	}
	return append(ret, unnamed...)
}

func MergeTemplateWorkspaceProfileContract(templateProfile map[string]any, workspaceProfile map[string]any) map[string]any {
	return runtimeprofiledecode.MergeTemplateWorkspaceProfileContract(templateProfile, workspaceProfile)
}

func contextMapSliceValue(value any) []map[string]any {
	return runtimeprofiledecode.MapSlice(value)
}
