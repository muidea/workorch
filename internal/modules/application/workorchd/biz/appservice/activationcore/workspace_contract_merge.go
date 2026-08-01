package activationcore

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func MergeTemplateWorkspaceContextHintsForExecutionContract(templateHints map[string]any, workspaceHints map[string]any) map[string]any {
	if len(templateHints) == 0 {
		return cloneContextMapDeep(workspaceHints)
	}
	ret := cloneContextMapDeep(templateHints)
	for key, value := range workspaceHints {
		if key == "profileContracts" {
			ret[key] = MergeTemplateWorkspaceProfileContracts(contextMapValue(templateHints[key]), contextMapValue(value))
			continue
		}
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func MergeTemplateWorkspaceProfileContracts(templateProfiles map[string]any, workspaceProfiles map[string]any) map[string]any {
	if len(templateProfiles) == 0 {
		if cloned := cloneContextValue(workspaceProfiles); cloned != nil {
			if mapped, ok := cloned.(map[string]any); ok {
				return mapped
			}
		}
		return nil
	}
	ret := map[string]any{}
	for profileID, profile := range templateProfiles {
		ret[profileID] = cloneContextValue(profile)
	}
	for profileID, workspaceProfile := range workspaceProfiles {
		templateProfile, exists := ret[profileID]
		if !exists {
			ret[profileID] = cloneContextValue(workspaceProfile)
			continue
		}
		ret[profileID] = MergeTemplateWorkspaceProfileContract(contextMapValue(templateProfile), contextMapValue(workspaceProfile))
	}
	return ret
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
	if len(templateProfile) == 0 {
		if cloned := cloneContextValue(workspaceProfile); cloned != nil {
			if mapped, ok := cloned.(map[string]any); ok {
				return mapped
			}
		}
		return nil
	}
	ret := map[string]any{}
	for key, value := range templateProfile {
		ret[key] = cloneContextValue(value)
	}
	for key, value := range workspaceProfile {
		if profileContractCollectionKey(key) {
			ret[key] = mergeProfileContractCollection(contextMapSliceValue(templateProfile[key]), contextMapSliceValue(value), profileContractCollectionIdentityFields(key))
			continue
		}
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	return ret
}

// A workspace may add an exact semantic proof declaration to the template
// profile that selected it.  The template's wildcard entries remain quality
// gates, while an exact (scope,id) declaration is the only form that can be
// compiled into CWI credit.  Treating profile collections as indivisible made
// that input unreachable: a workspace overlay with evidenceRequirements was
// silently discarded because the template already had that key.
//
// This is still activation-only configuration merge.  It does not infer a
// target, recipe, check, or CWI from Goal/DoD text, and it does not mutate any
// owner state.  Workspace items replace same-identity template items so an
// explicit profile can refine a declared requirement without creating two
// competing definitions for one proof identity.
func profileContractCollectionKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "acceptanceDimensions", "deliverableExpectations", "evidenceRequirements", "coverageDependencies", "bootstrapRecipes":
		return true
	default:
		return false
	}
}

func profileContractCollectionIdentityFields(key string) []string {
	if strings.TrimSpace(key) == "evidenceRequirements" {
		return []string{"scope", "id"}
	}
	return []string{"id"}
}

func contextMapSliceValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return CloneContextMapSlice(typed)
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ret = append(ret, cloneContextMapDeep(mapped))
		}
		return ret
	default:
		return nil
	}
}

func mergeProfileContractCollection(template, workspace []map[string]any, identityFields []string) []map[string]any {
	merged := make(map[string]map[string]any, len(template)+len(workspace))
	for _, item := range template {
		if key := profileContractCollectionIdentity(item, identityFields); key != "" {
			merged[key] = cloneContextMapDeep(item)
		}
	}
	for _, item := range workspace {
		if key := profileContractCollectionIdentity(item, identityFields); key != "" {
			merged[key] = cloneContextMapDeep(item)
		}
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, merged[key])
	}
	return ret
}

func profileContractCollectionIdentity(item map[string]any, fields []string) string {
	if len(item) == 0 || len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		value, ok := item[field].(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			return ""
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\x00")
}

func contextMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}
