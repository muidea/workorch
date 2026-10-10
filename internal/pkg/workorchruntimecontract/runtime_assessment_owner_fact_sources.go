package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// 保存 classifier 实际消费的 sealed catalog 来源；缺少匹配 catalog 时不补造 absence fact。
type RuntimeAssessmentOwnerFactSource struct {
	Requirement       RuntimeOwnerFactRequirement
	CatalogSnapshotID string                   `json:",omitempty"`
	OwnerRevisionSet  *RuntimeOwnerRevisionSet `json:",omitempty"`
}

func cloneRuntimeAssessmentOwnerFactSources(values []RuntimeAssessmentOwnerFactSource) []RuntimeAssessmentOwnerFactSource {
	return cloneRuntimeStateRawSlice(values, func(value RuntimeAssessmentOwnerFactSource) RuntimeAssessmentOwnerFactSource {
		value.OwnerRevisionSet = CloneRuntimeOwnerRevisionSet(value.OwnerRevisionSet)
		return value
	})
}

func deriveRuntimeAssessmentOwnerFactSources(definition string, workspace string, authority RuntimeAssessmentAuthorityInput) ([]RuntimeAssessmentOwnerFactSource, error) {
	if len(authority.Focus.RequiredOwnerFacts) == 0 {
		return nil, nil
	}
	if definition == "" || definition != strings.TrimSpace(definition) || workspace == "" || workspace != strings.TrimSpace(workspace) {
		return nil, fmt.Errorf("assessment owner-fact sources require exact definition and workspace")
	}
	if err := ValidateRuntimeOwnerFactRequirements(authority.Focus.RequiredOwnerFacts); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(authority.Focus.RequiredOwnerFacts, NormalizeRuntimeOwnerFactRequirements(authority.Focus.RequiredOwnerFacts)) {
		return nil, fmt.Errorf("assessment owner-fact source requirements must be raw canonical")
	}
	seenOwners := make(map[string]struct{}, len(authority.OperationAdmissibilities))
	seenPrerequisites := make(map[string]struct{}, len(authority.OperationAdmissibilities))
	for _, catalog := range authority.OperationAdmissibilities {
		if !reflect.DeepEqual(catalog, catalog.Normalized()) {
			return nil, fmt.Errorf("assessment owner-fact source catalog must be raw canonical")
		}
		if err := catalog.Validate(); err != nil {
			return nil, err
		}
		if _, duplicate := seenOwners[catalog.OwnerModuleID]; duplicate {
			return nil, fmt.Errorf("assessment owner-fact source duplicates owner")
		}
		if _, duplicate := seenPrerequisites[catalog.PrerequisiteID]; duplicate {
			return nil, fmt.Errorf("assessment owner-fact source duplicates prerequisite")
		}
		seenOwners[catalog.OwnerModuleID] = struct{}{}
		seenPrerequisites[catalog.PrerequisiteID] = struct{}{}
	}
	result := make([]RuntimeAssessmentOwnerFactSource, 0, len(authority.Focus.RequiredOwnerFacts))
	for _, requirement := range authority.Focus.RequiredOwnerFacts {
		source := RuntimeAssessmentOwnerFactSource{Requirement: requirement}
		for _, catalog := range authority.OperationAdmissibilities {
			if catalog.OwnerModuleID != requirement.OwnerModuleID || !runtimeOperationKindsContain(catalog.OperationKinds, requirement.OperationKind) {
				continue
			}
			// 来源 snapshot 独立保全；依赖 revision 只绑定 postcondition 读取的字段，不使用整 catalog hash。
			selector := struct {
				WorkspaceID string
				Requirement RuntimeOwnerFactRequirement
			}{workspace, requirement}
			body, err := json.Marshal(selector)
			if err != nil {
				return nil, err
			}
			digest := sha256.Sum256(append([]byte("runtime-assessment-owner-fact-selector/v1\x00"), body...))
			selectorHash := hex.EncodeToString(digest[:])
			observations, _ := observeRuntimeOwnerFactRequirementCatalog(requirement, catalog)
			body, err = json.Marshal(observations)
			if err != nil {
				return nil, err
			}
			revision := sha256.Sum256(append([]byte("runtime-assessment-owner-fact-observation/v1\x00"), body...))
			set := (RuntimeOwnerRevisionSet{DefinitionRevision: definition, Entries: []RuntimeOwnerRevisionRef{{
				OwnerKind: "capability", OwnerInstanceID: catalog.OwnerModuleID + "/" + workspace,
				ResourceKind: "owner_fact_requirement", ResourceID: selectorHash,
				Revision: hex.EncodeToString(revision[:]), DependencyHash: selectorHash,
			}}}).WithHash()
			if err := set.Validate(); err != nil {
				return nil, err
			}
			source.CatalogSnapshotID = catalog.SnapshotID
			source.OwnerRevisionSet = &set
		}
		result = append(result, source)
	}
	return result, nil
}
