package file

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeFileConstraintWorkspaceRelativePath          = "workspace-relative-path"
	runtimeFileConstraintExistingTargetListed           = "existing-target-must-be-listed"
	runtimeFileConstraintNewTargetRequiresOwnerSelector = "new-target-requires-owner-selector"
	runtimeFileConstraintRegularExistingTarget          = "regular-file-required-for-read-edit-and-existing-patch-target"
)

func runtimeFileProspectiveTargetAllOfConstraintRefs() []string {
	return []string{
		runtimeFileConstraintWorkspaceRelativePath,
		runtimeFileConstraintNewTargetRequiresOwnerSelector,
	}
}

// AcquireRuntimeOperationAdmissibility returns the bounded File-owner view used
// to render a candidate. It reads metadata only and never writes a receipt or
// workspace file.
func (a *RuntimeAdapter) AcquireRuntimeOperationAdmissibility(workspaceDirectory, prerequisiteID string, operationKinds, requiredTargetRefs []string, maxItems int) (runtimemodel.RuntimeOperationAdmissibility, error) {
	manifest, err := a.AcquireWorkspaceManifest(workspaceDirectory)
	if err != nil {
		return runtimemodel.RuntimeOperationAdmissibility{}, err
	}
	if maxItems < 1 || maxItems > runtimemodel.RuntimeOwnerInventoryItemsMaxCount {
		return runtimemodel.RuntimeOperationAdmissibility{}, common.ErrInvalidArgument
	}
	inventory := runtimemodel.RuntimeOperationAdmissibility{
		PrerequisiteID:    prerequisiteID,
		OwnerModuleID:     runtimemodel.RuntimeModuleFile,
		OperationKinds:    append([]string(nil), operationKinds...),
		OperationVariants: fileAdmissibilityOperationVariants(operationKinds),
		TargetRefs:        boundedAdmissibilityTargetRefs(requiredTargetRefs, manifest.TargetRefs, maxItems),
		SelectorRefs:      fileAdmissibilitySelectorRefs(operationKinds),
		ConstraintRefs: []string{
			runtimeFileConstraintWorkspaceRelativePath,
			runtimeFileConstraintExistingTargetListed,
			runtimeFileConstraintNewTargetRequiresOwnerSelector,
			runtimeFileConstraintRegularExistingTarget,
		},
		ObservedRevision: manifest.Revision,
	}
	inventory.SnapshotID = runtimemodel.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		return runtimemodel.RuntimeOperationAdmissibility{}, err
	}
	return inventory, nil
}

func fileAdmissibilitySelectorRefs(operationKinds []string) []string {
	newTarget := false
	existingTarget := false
	for _, kind := range operationKinds {
		switch runtimemodel.CanonicalRuntimeBlockKind(kind) {
		case runtimemodel.StepKindFileWrite, runtimemodel.RuntimeBlockKindPatchApply, runtimemodel.RuntimeBlockKindPatchPreview:
			newTarget = true
		case runtimemodel.StepKindFileEdit, runtimemodel.StepKindFileRead, runtimemodel.RuntimeBlockKindPatchRevert:
			existingTarget = true
		}
	}
	ret := make([]string, 0, 2)
	if existingTarget {
		ret = append(ret, runtimeFileSelectorWorkspaceRelativeExistingTarget)
	}
	if newTarget {
		ret = append(ret, runtimeFileSelectorWorkspaceRelativeNewTarget)
	}
	return ret
}

// fileAdmissibilityOperationVariants is the File owner's sole declaration of
// variant-specific target semantics.  The broad selector/constraint union on
// RuntimeOperationAdmissibility remains useful as bounded provider vocabulary,
// but it is never sufficient to authorize a concrete variant when this typed
// slice is present.
func fileAdmissibilityOperationVariants(operationKinds []string) []runtimemodel.RuntimeOperationAdmissibilityVariant {
	policiesExisting := []runtimemodel.RuntimeOperationTargetPolicy{
		runtimemodel.RuntimeOperationTargetPolicyExactTarget,
		runtimemodel.RuntimeOperationTargetPolicyExistingOwnerScope,
	}
	policiesCreate := append(append([]runtimemodel.RuntimeOperationTargetPolicy(nil), policiesExisting...), runtimemodel.RuntimeOperationTargetPolicyProspectiveOwnerTarget)
	newTargetSelector := []string{runtimeFileSelectorWorkspaceRelativeNewTarget}
	newTargetConstraints := runtimeFileProspectiveTargetAllOfConstraintRefs()
	existingTargetSelector := []string{runtimeFileSelectorWorkspaceRelativeExistingTarget}
	existingTargetConstraints := []string{
		runtimeFileConstraintWorkspaceRelativePath,
		runtimeFileConstraintExistingTargetListed,
		runtimeFileConstraintRegularExistingTarget,
	}
	ret := make([]runtimemodel.RuntimeOperationAdmissibilityVariant, 0)
	for _, raw := range operationKinds {
		kind := runtimemodel.CanonicalRuntimeBlockKind(raw)
		switch kind {
		case runtimemodel.StepKindFileWrite:
			// file/write is the File owner's create-only operation. Existing
			// targets are intentionally absent from this fact so the semantic
			// manifest, JSON schema and operation feasibility probe all consume
			// the same owner policy. Revision-bound replacement remains available
			// through file/edit or an admissible patch/apply variant.
			ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{
				Kind: kind,
				AllowedTargetPolicies: []runtimemodel.RuntimeOperationTargetPolicy{
					runtimemodel.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				},
				SelectorRefs: newTargetSelector, ConstraintRefs: newTargetConstraints,
			})
		case runtimemodel.StepKindFileEdit:
			ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{
				Kind: kind, AllowedTargetPolicies: policiesExisting,
				SelectorRefs: existingTargetSelector, ConstraintRefs: existingTargetConstraints,
			})
		case runtimemodel.RuntimeBlockKindPatchApply:
			for _, variantID := range []string{patchTypeFileWrite, patchTypeTextReplace, patchTypeUnifiedDiff} {
				policies := policiesExisting
				variantSelectors := []string(nil)
				variantConstraints := []string{runtimeFileConstraintWorkspaceRelativePath, runtimeFileConstraintExistingTargetListed}
				if variantID == patchTypeFileWrite || variantID == patchTypeUnifiedDiff {
					policies = policiesCreate
					variantSelectors = newTargetSelector
					variantConstraints = newTargetConstraints
				}
				ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{Kind: kind, VariantID: variantID, AllowedTargetPolicies: policies, SelectorRefs: variantSelectors, ConstraintRefs: variantConstraints})
			}
		case runtimemodel.RuntimeBlockKindPatchPreview:
			for _, variantID := range []string{patchTypeFileWrite, patchTypeTextReplace, patchTypeUnifiedDiff} {
				policies := policiesExisting
				variantSelectors := []string(nil)
				variantConstraints := []string{runtimeFileConstraintWorkspaceRelativePath, runtimeFileConstraintExistingTargetListed}
				if variantID == patchTypeFileWrite || variantID == patchTypeUnifiedDiff {
					policies = policiesCreate
					variantSelectors = newTargetSelector
					variantConstraints = newTargetConstraints
				}
				ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{Kind: kind, VariantID: variantID, AllowedTargetPolicies: policies, SelectorRefs: variantSelectors, ConstraintRefs: variantConstraints})
			}
		case runtimemodel.StepKindFileRead:
			ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{Kind: kind, AllowedTargetPolicies: policiesExisting})
		case runtimemodel.StepKindFileList:
			ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{Kind: kind, AllowedTargetPolicies: []runtimemodel.RuntimeOperationTargetPolicy{runtimemodel.RuntimeOperationTargetPolicyExactTarget}})
		case runtimemodel.RuntimeBlockKindPatchRevert:
			ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{Kind: kind, AllowedTargetPolicies: policiesExisting})
		}
	}
	return ret
}

func boundedAdmissibilityTargetRefs(required, available []string, maxItems int) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, maxItems)
	appendRef := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || len(ret) >= maxItems {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	for _, value := range required {
		appendRef(value)
	}
	for _, value := range available {
		appendRef(value)
	}
	return ret
}
