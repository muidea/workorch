package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	defaultMaxReadBytes                           = 64 * 1024
	defaultMaxEntries                             = 500
	runtimeFileSelectorWorkspaceRelativeNewTarget = "workspace-relative-new-file-target"
	runtimeFileTargetSelectorUniqueWorkspaceEntry = "file/unique_workspace_entry"
	runtimeFileTargetSelectorWorkspaceDeliverable = "file/workspace_deliverable"
	runtimeFileTargetSelectorProfileCandidates    = "file/profile_declared_candidates"
)

type RuntimeAdapter struct{}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{}
}

func (a *RuntimeAdapter) AcquireTargetSnapshot(ctx context.Context, workspaceDirectory, targetRef string, maxBytes int) (runtimemodel.TargetSnapshot, error) {
	result, err := a.ReadFile(ctx, FileReadRequest{
		WorkspaceDirectory: workspaceDirectory,
		Path:               targetRef,
		MaxBytes:           int64(maxBytes),
	})
	if err != nil {
		return runtimemodel.TargetSnapshot{}, err
	}
	snapshot := runtimemodel.TargetSnapshot{
		TargetRef:   result.TargetRef,
		Content:     result.Content,
		ContentHash: result.ContentHash,
		Size:        result.Size,
		LineCount:   int(result.LineCount),
	}
	switch {
	case !result.Exists:
		snapshot.ContentState = runtimemodel.SnapshotContentStateMissing
	case result.Binary:
		snapshot.ContentState = runtimemodel.SnapshotContentStateBinary
	case result.Redacted:
		snapshot.ContentState = runtimemodel.SnapshotContentStateRedacted
	case result.Truncated:
		snapshot.ContentState = runtimemodel.SnapshotContentStateTruncated
	case result.ContentComplete:
		snapshot.ContentState = runtimemodel.SnapshotContentStateComplete
	}
	if !snapshot.Complete() {
		return snapshot, fmt.Errorf("%w: target snapshot %q is not complete (%s)", common.ErrInvalidArgument, snapshot.TargetRef, snapshot.ContentState)
	}
	return snapshot, nil
}

func (a *RuntimeAdapter) AcquireWorkspaceManifest(workspaceDirectory string) (runtimemodel.WorkspaceManifest, error) {
	result, err := a.ListFiles(FileListRequest{
		WorkspaceDirectory: workspaceDirectory,
		Path:               ".",
		Recursive:          true,
	})
	if err != nil {
		return runtimemodel.WorkspaceManifest{}, err
	}
	if result.Truncated {
		return runtimemodel.WorkspaceManifest{}, fmt.Errorf("%w: workspace manifest exceeds bounded file listing", common.ErrInvalidArgument)
	}
	targetRefs := make([]string, 0, len(result.Entries))
	h := sha256.New()
	for _, entry := range result.Entries {
		if entry.Kind != "file" {
			continue
		}
		ref := strings.TrimSpace(entry.Path)
		if ref == "" {
			continue
		}
		targetRefs = append(targetRefs, ref)
		_, _ = fmt.Fprintf(h, "%d:%s|%s|%d|%s;", len(ref), ref, entry.ContentHash, entry.Size, entry.ModifiedAt)
	}
	return runtimemodel.WorkspaceManifest{
		Revision:   hex.EncodeToString(h.Sum(nil)),
		TargetRefs: targetRefs,
	}, nil
}

// AcquireRuntimeTargetSelection applies the File owner's neutral cardinality
// rule to a bounded workspace inventory. It does not inspect source syntax,
// file names, suffixes, or a profile's semantic TargetRole. The
// workspace-deliverable selector has both a profile-declared creation form for
// an empty inventory and a bounded complete-scope form for multiple entries.
// File never derives a path: it only verifies the declared prospective path
// and operation admissibility validates every operation target.
type runtimeTargetSelectionBinding struct {
	ownerRef      string
	subjectRef    string
	candidateRefs []string
}

func (a *RuntimeAdapter) AcquireRuntimeTargetSelection(workspaceDirectory, prerequisiteID, targetRole, selectorRef, prospectiveTargetRef string, maxItems int, bindings ...runtimeTargetSelectionBinding) (runtimemodel.RuntimeTargetSelection, error) {
	selectorRef = strings.TrimSpace(selectorRef)
	prospectiveTargetRef = strings.TrimSpace(prospectiveTargetRef)
	if strings.TrimSpace(prerequisiteID) == "" || strings.TrimSpace(targetRole) == "" || selectorRef == "" || maxItems < 1 || maxItems > runtimemodel.RuntimeOwnerInventoryItemsMaxCount {
		return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
	}
	binding := runtimeTargetSelectionBinding{}
	if len(bindings) > 1 {
		return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
	}
	if len(bindings) == 1 {
		binding.ownerRef = strings.TrimSpace(bindings[0].ownerRef)
		binding.subjectRef = strings.TrimSpace(bindings[0].subjectRef)
		binding.candidateRefs = normalizeRuntimeFileTargetRefs(bindings[0].candidateRefs)
	}
	if selectorRef != runtimeFileTargetSelectorUniqueWorkspaceEntry && selectorRef != runtimeFileTargetSelectorWorkspaceDeliverable && selectorRef != runtimeFileTargetSelectorProfileCandidates {
		selection := runtimemodel.RuntimeTargetSelection{
			PrerequisiteID: prerequisiteID, OwnerModuleID: runtimemodel.RuntimeModuleFile, TargetRole: targetRole, SelectorRef: selectorRef,
			SelectorOwnerRef: binding.ownerRef, SelectorSubjectRef: binding.subjectRef, SelectorCandidateRefs: binding.candidateRefs,
			ProspectiveTargetRef: prospectiveTargetRef, Disposition: runtimemodel.RuntimeTargetSelectionUnsupported,
			ObservedRevision: "unsupported-selector:" + selectorRef,
		}
		selection.SnapshotID = runtimemodel.StableRuntimeTargetSelectionSnapshotID(selection)
		return selection, selection.Validate()
	}
	manifest, err := a.AcquireWorkspaceManifest(workspaceDirectory)
	if err != nil {
		return runtimemodel.RuntimeTargetSelection{}, err
	}
	if selectorRef == runtimeFileTargetSelectorProfileCandidates {
		return acquireProfileDeclaredTargetSelection(workspaceDirectory, prerequisiteID, targetRole, selectorRef, prospectiveTargetRef, maxItems, manifest, binding)
	}
	candidates := boundedAdmissibilityTargetRefs(nil, manifest.TargetRefs, maxItems)
	selection := runtimemodel.RuntimeTargetSelection{
		PrerequisiteID: prerequisiteID, OwnerModuleID: runtimemodel.RuntimeModuleFile, TargetRole: targetRole, SelectorRef: selectorRef,
		SelectorOwnerRef: binding.ownerRef, SelectorSubjectRef: binding.subjectRef, SelectorCandidateRefs: binding.candidateRefs,
		CandidateTargetRefs: candidates, ProspectiveTargetRef: prospectiveTargetRef, CandidateCount: len(manifest.TargetRefs), ObservedRevision: manifest.Revision,
	}
	switch selection.CandidateCount {
	case 0:
		if selectorRef == runtimeFileTargetSelectorWorkspaceDeliverable {
			if prospectiveTargetRef == "" {
				// A profile may explicitly authorize the owner-grounded empty
				// workspace scope. File still does not derive a path: SM-3 must
				// propose a concrete File target and the normal owner feasibility
				// gate validates it before any side effect. Unbound selectors retain
				// the deterministic absent result and never authorize a provider
				// filename guess.
				if binding.ownerRef != "" && binding.subjectRef != "" {
					selection.Disposition = runtimemodel.RuntimeTargetSelectionScope
				} else {
					selection.Disposition = runtimemodel.RuntimeTargetSelectionAbsent
				}
				break
			}
			if strings.Contains(prospectiveTargetRef, "\\") {
				return runtimemodel.RuntimeTargetSelection{}, fmt.Errorf("%w: prospective target must use canonical workspace-relative separators", common.ErrInvalidArgument)
			}
			_, target, relative, err := guardedWorkspacePath(workspaceDirectory, prospectiveTargetRef, false)
			if err != nil {
				return runtimemodel.RuntimeTargetSelection{}, err
			}
			if canonical := filepath.ToSlash(relative); canonical != prospectiveTargetRef {
				return runtimemodel.RuntimeTargetSelection{}, fmt.Errorf("%w: prospective target %q is not canonical (want %q)", common.ErrInvalidArgument, prospectiveTargetRef, canonical)
			}
			if _, err := os.Lstat(target); err == nil {
				return runtimemodel.RuntimeTargetSelection{}, fmt.Errorf("%w: prospective target %q already exists", common.ErrInvalidArgument, filepath.ToSlash(relative))
			} else if !os.IsNotExist(err) {
				return runtimemodel.RuntimeTargetSelection{}, err
			}
			selection.Disposition = runtimemodel.RuntimeTargetSelectionCreate
			selection.ProspectiveTargetRef = filepath.ToSlash(relative)
		} else {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionAbsent
		}
	case 1:
		selection.Disposition = runtimemodel.RuntimeTargetSelectionSelected
		selection.TargetRefs = append([]string(nil), candidates...)
	default:
		if selectorRef == runtimeFileTargetSelectorWorkspaceDeliverable && len(candidates) == selection.CandidateCount {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionScope
			selection.TargetRefs = append([]string(nil), candidates...)
		} else {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionAmbiguous
		}
	}
	selection.SnapshotID = runtimemodel.StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err != nil {
		return runtimemodel.RuntimeTargetSelection{}, err
	}
	return selection, nil
}

// acquireProfileDeclaredTargetSelection only intersects the profile-provided,
// bounded candidate set with the File manifest. The profile binding supplies
// all semantics; File does not inspect a name, suffix, content or role.
func acquireProfileDeclaredTargetSelection(workspaceDirectory, prerequisiteID, targetRole, selectorRef, prospectiveTargetRef string, maxItems int, manifest runtimemodel.WorkspaceManifest, binding runtimeTargetSelectionBinding) (runtimemodel.RuntimeTargetSelection, error) {
	if binding.ownerRef == "" || binding.subjectRef == "" || len(binding.candidateRefs) == 0 || len(binding.candidateRefs) > maxItems {
		return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
	}
	for _, candidate := range binding.candidateRefs {
		if strings.Contains(candidate, "\\") {
			return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
		}
		_, _, relative, err := guardedWorkspacePath(workspaceDirectory, candidate, false)
		if err != nil || filepath.ToSlash(relative) != candidate {
			return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
		}
	}
	declared := make(map[string]struct{}, len(binding.candidateRefs))
	for _, candidate := range binding.candidateRefs {
		declared[candidate] = struct{}{}
	}
	matched := make([]string, 0, len(binding.candidateRefs))
	for _, target := range manifest.TargetRefs {
		if _, ok := declared[target]; ok {
			matched = append(matched, target)
		}
	}
	matched = boundedAdmissibilityTargetRefs(nil, matched, maxItems)
	selection := runtimemodel.RuntimeTargetSelection{
		PrerequisiteID: prerequisiteID, OwnerModuleID: runtimemodel.RuntimeModuleFile, TargetRole: targetRole, SelectorRef: selectorRef,
		SelectorOwnerRef: binding.ownerRef, SelectorSubjectRef: binding.subjectRef, SelectorCandidateRefs: binding.candidateRefs,
		CandidateTargetRefs: matched, ProspectiveTargetRef: prospectiveTargetRef, CandidateCount: len(matched), ObservedRevision: manifest.Revision,
	}
	switch selection.CandidateCount {
	case 0:
		if prospectiveTargetRef == "" {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionAbsent
			break
		}
		if _, ok := declared[prospectiveTargetRef]; !ok || strings.Contains(prospectiveTargetRef, "\\") {
			return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
		}
		_, target, relative, err := guardedWorkspacePath(workspaceDirectory, prospectiveTargetRef, false)
		if err != nil || filepath.ToSlash(relative) != prospectiveTargetRef {
			return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
		}
		if _, err := os.Lstat(target); err == nil || !os.IsNotExist(err) {
			return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
		}
		selection.Disposition = runtimemodel.RuntimeTargetSelectionCreate
	case 1:
		selection.Disposition = runtimemodel.RuntimeTargetSelectionSelected
		selection.TargetRefs = append([]string(nil), matched...)
	default:
		selection.Disposition = runtimemodel.RuntimeTargetSelectionAmbiguous
	}
	selection.SnapshotID = runtimemodel.StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err != nil {
		return runtimemodel.RuntimeTargetSelection{}, err
	}
	return selection, nil
}

func normalizeRuntimeFileTargetRefs(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
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
			"workspace-relative-path",
			"existing-target-must-be-listed",
			"new-target-requires-owner-selector",
			"regular-file-required-for-read-edit-and-existing-patch-target",
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
	for _, kind := range operationKinds {
		switch runtimemodel.CanonicalRuntimeBlockKind(kind) {
		case runtimemodel.StepKindFileWrite, runtimemodel.RuntimeBlockKindPatchApply, runtimemodel.RuntimeBlockKindPatchPreview:
			return []string{runtimeFileSelectorWorkspaceRelativeNewTarget}
		}
	}
	return nil
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
	selector := []string{runtimeFileSelectorWorkspaceRelativeNewTarget}
	constraints := []string{"workspace-relative-path", "new-target-requires-owner-selector"}
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
				SelectorRefs: selector, ConstraintRefs: constraints,
			})
		case runtimemodel.StepKindFileEdit:
			ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{Kind: kind, AllowedTargetPolicies: policiesExisting})
		case runtimemodel.RuntimeBlockKindPatchApply:
			for _, variantID := range []string{patchTypeFileWrite, patchTypeTextReplace, patchTypeUnifiedDiff} {
				policies := policiesExisting
				variantSelectors := []string(nil)
				variantConstraints := []string{"workspace-relative-path", "existing-target-must-be-listed"}
				if variantID == patchTypeFileWrite || variantID == patchTypeUnifiedDiff {
					policies = policiesCreate
					variantSelectors = selector
					variantConstraints = constraints
				}
				ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{Kind: kind, VariantID: variantID, AllowedTargetPolicies: policies, SelectorRefs: variantSelectors, ConstraintRefs: variantConstraints})
			}
		case runtimemodel.RuntimeBlockKindPatchPreview:
			for _, variantID := range []string{patchTypeFileWrite, patchTypeTextReplace, patchTypeUnifiedDiff} {
				policies := policiesExisting
				variantSelectors := []string(nil)
				variantConstraints := []string{"workspace-relative-path", "existing-target-must-be-listed"}
				if variantID == patchTypeFileWrite || variantID == patchTypeUnifiedDiff {
					policies = policiesCreate
					variantSelectors = selector
					variantConstraints = constraints
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

// ProbeRuntimeOperationFeasibility checks target admissibility without applying
// a patch, writing a file, or creating an execution receipt.
func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibility(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation) *runtimemodel.FailureFact {
	failure, _ := a.ProbeRuntimeOperationFeasibilityWithObservation(workspaceDirectory, inventory, operation, nil)
	return failure
}

// ProbeRuntimeOperationFeasibilityWithReplayProof returns the ordinary
// side-effect-free feasibility result plus an optional owner-authored effect
// proof. Only patch/apply has a File-owned durable postcondition that can be
// proven by previewing the exact sealed operation; all other kinds deliberately
// return a nil proof and retain their normal recovery policy.
func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibilityWithReplayProof(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation, observation *runtimemodel.RuntimePrerequisiteObservation) (*runtimemodel.FailureFact, *runtimemodel.RuntimePrerequisiteStale, *runtimemodel.RuntimeOperationReplayProof) {
	failure, stale := a.ProbeRuntimeOperationFeasibilityWithObservation(workspaceDirectory, inventory, operation, observation)
	if failure != nil || stale != nil || operation.Normalized().Kind != runtimemodel.RuntimeBlockKindPatchApply {
		return failure, stale, nil
	}
	request, ok := patchApplyRequestFromRuntimeOperation(workspaceDirectory, operation)
	if !ok {
		return fileOperationInfeasible(operation, "operation_replay_proof_invalid", "runtime patch operation cannot form an owner replay proof", operation.TargetRefs), nil, nil
	}
	preview, err := a.PreviewPatch(request)
	if err != nil || preview.BeforeHash == preview.AfterHash || strings.TrimSpace(preview.AfterHash) == "" {
		message := "runtime patch operation cannot prove a durable postcondition"
		if err != nil {
			message = err.Error()
		}
		return fileOperationInfeasible(operation, "operation_replay_proof_unavailable", message, operation.TargetRefs), nil, nil
	}
	proof := runtimemodel.RuntimeOperationReplayProof{
		OwnerModuleID: ModuleID, OperationID: operation.Normalized().ID,
		TargetRefs: []string{strings.TrimSpace(preview.TargetRef)}, BeforeHash: preview.BeforeHash, AfterHash: preview.AfterHash,
		AdmissibilitySnapshotID: inventory.Normalized().SnapshotID, AcquisitionBinding: inventory.Normalized().AcquisitionBinding,
	}.Normalized()
	if err := proof.Validate(); err != nil {
		return fileOperationInfeasible(operation, "operation_replay_proof_invalid", err.Error(), operation.TargetRefs), nil, nil
	}
	return nil, nil, &proof
}

// ProbeRuntimeOperationFeasibilityWithObservation is the owner read-only
// admission gate. A changed inventory revision is a freshness transition, not
// a candidate-contract failure, when Orchestrator supplied the persisted
// observation bound to that inventory.
func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibilityWithObservation(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation, observation *runtimemodel.RuntimePrerequisiteObservation) (*runtimemodel.FailureFact, *runtimemodel.RuntimePrerequisiteStale) {
	operation = operation.Normalized()
	inventory = inventory.Normalized()
	manifest, manifestErr := a.AcquireWorkspaceManifest(workspaceDirectory)
	if manifestErr != nil {
		return fileOperationInfeasible(operation, "operation_admissibility_unavailable", manifestErr.Error(), nil), nil
	}
	if inventory.ObservedRevision != manifest.Revision {
		if observation != nil {
			value := observation.Normalized()
			for _, dependency := range value.Dependencies {
				if dependency.Domain == runtimemodel.RuntimeResourceWorkspaceTree && dependency.OwnerModuleID == runtimemodel.RuntimeModuleFile && dependency.SubjectRef == "workspace_deliverable" && dependency.Revision == inventory.ObservedRevision {
					return nil, &runtimemodel.RuntimePrerequisiteStale{
						PrerequisiteID: value.PrerequisiteID, ObservationID: value.ObservationID,
						Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef,
						ExpectedRevision: dependency.Revision, ObservedRevision: manifest.Revision,
						Reason: "runtime file operation admissibility snapshot is stale",
					}
				}
			}
		}
		return fileOperationInfeasible(operation, "operation_admissibility_stale", "runtime file operation admissibility snapshot is stale", nil), nil
	}
	targetRef := fileOperationTargetRef(operation)
	if targetRef == "" {
		return fileOperationInfeasible(operation, "operation_target_missing", "runtime file operation requires a concrete workspace-relative target", nil), nil
	}
	_, target, rel, err := guardedWorkspacePath(workspaceDirectory, targetRef, false)
	if err != nil {
		return fileOperationInfeasible(operation, "operation_target_invalid", err.Error(), []string{targetRef}), nil
	}
	info, statErr := os.Lstat(target)
	requiresRegular := fileOperationRequiresRegularTarget(operation)
	if statErr == nil {
		policy := evaluateFileOwnerFeasibilityPolicy(fileOwnerFeasibilityPolicyInput{
			OperationKind: operation.Kind, VariantID: runtimeOperationParameterString(operation, "patchType"),
			TargetPolicy: runtimemodel.RuntimeOperationTargetPolicyExactTarget, TargetRef: filepath.ToSlash(rel), TargetPath: target,
		})
		if !policy.Feasible() {
			return fileOperationInfeasible(operation, policy.Code, policy.Message, policy.TargetRefs), nil
		}
		if !fileInventoryContainsTarget(inventory, targetRef) {
			return fileOperationInfeasible(operation, "operation_target_not_admissible", "runtime file operation target is absent from the owner admissibility inventory", []string{targetRef}), nil
		}
		if requiresRegular && !info.Mode().IsRegular() {
			return fileOperationInfeasible(operation, "operation_target_not_regular", "runtime file operation target is not a regular file", []string{filepath.ToSlash(rel)}), nil
		}
		if failure := verifyFileWriteReplayPrecondition(target, operation, filepath.ToSlash(rel)); failure != nil {
			return failure, nil
		}
		if failure := a.probePatchFeasibility(workspaceDirectory, operation, filepath.ToSlash(rel)); failure != nil {
			return failure, nil
		}
		return nil, nil
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return fileOperationInfeasible(operation, "operation_target_unreadable", statErr.Error(), []string{filepath.ToSlash(rel)}), nil
	}
	if fileOperationRequiresExistingTarget(operation) {
		return fileOperationInfeasible(operation, "operation_target_missing", "runtime file operation requires an existing regular file", []string{filepath.ToSlash(rel)}), nil
	}
	policy := evaluateFileOwnerFeasibilityPolicy(fileOwnerFeasibilityPolicyInput{
		OperationKind: operation.Kind, VariantID: runtimeOperationParameterString(operation, "patchType"),
		TargetPolicy: runtimemodel.RuntimeOperationTargetPolicyProspectiveOwnerTarget, TargetRef: filepath.ToSlash(rel), TargetPath: target,
	})
	if !policy.Feasible() {
		return fileOperationInfeasible(operation, policy.Code, policy.Message, policy.TargetRefs), nil
	}
	if !fileInventoryContainsTarget(inventory, targetRef) && !fileInventoryAllowsNewTargetForOperation(inventory, operation) {
		return fileOperationInfeasible(operation, "operation_target_not_admissible", "runtime file operation target is absent from the owner admissibility inventory", []string{targetRef}), nil
	}
	if failure := a.probePatchFeasibility(workspaceDirectory, operation, filepath.ToSlash(rel)); failure != nil {
		return failure, nil
	}
	return nil, nil
}

// verifyFileWriteReplayPrecondition binds an optional file_write baseHash to
// the owner-observed preflight revision. ApplyPatch can later use that sealed
// revision to prove a lost-receipt replay reached the same postcondition; a
// provider cannot invent an arbitrary prior hash and obtain such a receipt.
func verifyFileWriteReplayPrecondition(target string, operation runtimemodel.RuntimeOperation, targetRef string) *runtimemodel.FailureFact {
	if operation.Kind != runtimemodel.RuntimeBlockKindPatchApply && operation.Kind != runtimemodel.RuntimeBlockKindPatchPreview {
		return nil
	}
	if runtimeOperationParameterString(operation, "patchType") != patchTypeFileWrite {
		return nil
	}
	baseHash := runtimeOperationParameterString(operation, "baseHash")
	if baseHash == "" {
		return fileOperationInfeasible(operation, "operation_base_hash_required", "patch/apply file_write requires the owner-observed baseHash when replacing an existing file", []string{targetRef})
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return fileOperationInfeasible(operation, "operation_base_hash_unavailable", err.Error(), []string{targetRef})
	}
	if !strings.EqualFold(sha256Hex(content), baseHash) {
		return fileOperationInfeasible(operation, "operation_base_hash_mismatch", "runtime file operation baseHash does not match the owner-observed target revision", []string{targetRef})
	}
	return nil
}

// probePatchFeasibility uses the same patch parser and in-memory apply path as
// execution, but never writes or creates a receipt. This lets the File owner
// reject an invalid unified diff before SM-3 seals a durable dispatch.
func (a *RuntimeAdapter) probePatchFeasibility(workspaceDirectory string, operation runtimemodel.RuntimeOperation, targetRef string) *runtimemodel.FailureFact {
	if operation.Kind != runtimemodel.RuntimeBlockKindPatchApply && operation.Kind != runtimemodel.RuntimeBlockKindPatchPreview {
		return nil
	}
	patchType := runtimeOperationParameterString(operation, "patchType")
	if patchType != patchTypeUnifiedDiff {
		return nil
	}
	preview, err := a.PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspaceDirectory, TargetRef: targetRef, PatchType: patchType,
		Diff: runtimeOperationParameterString(operation, "diff"), BaseHash: runtimeOperationParameterString(operation, "baseHash"),
		Summary: runtimeOperationParameterString(operation, "summary"),
	})
	if err == nil {
		return nil
	}
	code := strings.TrimSpace(preview.FailureCode)
	if code == "" {
		code = "unified_diff_infeasible"
	}
	message := strings.TrimSpace(preview.FailureMessage)
	if message == "" {
		message = err.Error()
	}
	return fileOperationInfeasible(operation, code, message, []string{targetRef})
}

func runtimeOperationParameterString(operation runtimemodel.RuntimeOperation, name string) string {
	for _, parameter := range operation.Parameters {
		if parameter.Name == name && parameter.String != nil {
			// Operation parameter values may be executable payloads. In
			// particular, patch content, old/new text and unified diffs are
			// byte-sensitive: trimming here would make feasibility preview (and
			// its sealed replay proof) describe a different effect from the
			// operation eventually executed by the typed File handler. Callers
			// that consume identifiers or enums normalize those fields at their
			// own contract boundary.
			return *parameter.String
		}
	}
	return ""
}

func runtimeOperationParameterInteger(operation runtimemodel.RuntimeOperation, name string) int64 {
	for _, parameter := range operation.Parameters {
		if parameter.Name == name && parameter.Integer != nil {
			return *parameter.Integer
		}
	}
	return 0
}

func runtimeOperationHasStringParameter(operation runtimemodel.RuntimeOperation, name string) bool {
	for _, parameter := range operation.Parameters {
		if parameter.Name == name && parameter.String != nil {
			return true
		}
	}
	return false
}

func patchApplyRequestFromRuntimeOperation(workspaceDirectory string, operation runtimemodel.RuntimeOperation) (PatchApplyRequest, bool) {
	operation = operation.Normalized()
	if operation.Kind != runtimemodel.RuntimeBlockKindPatchApply {
		return PatchApplyRequest{}, false
	}
	targetRef := fileOperationTargetRef(operation)
	patchType := runtimeOperationParameterString(operation, "patchType")
	if targetRef == "" || patchType == "" {
		return PatchApplyRequest{}, false
	}
	return PatchApplyRequest{
		WorkspaceDirectory: workspaceDirectory, TargetRef: targetRef, PatchType: patchType,
		Content: runtimeOperationParameterString(operation, "content"), ContentPresent: runtimeOperationHasStringParameter(operation, "content"),
		Diff: runtimeOperationParameterString(operation, "diff"), OldText: runtimeOperationParameterString(operation, "oldText"),
		NewText: runtimeOperationParameterString(operation, "newText"), NewTextPresent: runtimeOperationHasStringParameter(operation, "newText"),
		BaseHash: runtimeOperationParameterString(operation, "baseHash"), Occurrence: runtimeOperationParameterInteger(operation, "occurrence"),
		CoverageTargets: append([]string(nil), runtimeOperationParameterStrings(operation, "coverageTargets")...), Summary: runtimeOperationParameterString(operation, "summary"),
	}, true
}

func runtimeOperationParameterStrings(operation runtimemodel.RuntimeOperation, name string) []string {
	for _, parameter := range operation.Parameters {
		if parameter.Name == name {
			return append([]string(nil), parameter.StringList...)
		}
	}
	return nil
}

func fileInventoryContainsTarget(inventory runtimemodel.RuntimeOperationAdmissibility, targetRef string) bool {
	targetRef = strings.TrimSpace(targetRef)
	for _, value := range inventory.Normalized().TargetRefs {
		if value == targetRef {
			return true
		}
	}
	return false
}

func fileInventoryAllowsNewTargetForOperation(inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation) bool {
	inventory = inventory.Normalized()
	variant, found, err := runtimemodel.RuntimeOperationAdmissibilityVariantForOperation(inventory, operation)
	if err != nil {
		return false
	}
	if found {
		if !containsString(inventory.SelectorRefs, runtimeFileSelectorWorkspaceRelativeNewTarget) {
			return false
		}
		for _, policy := range variant.AllowedTargetPolicies {
			if policy == runtimemodel.RuntimeOperationTargetPolicyProspectiveOwnerTarget && containsString(variant.SelectorRefs, runtimeFileSelectorWorkspaceRelativeNewTarget) {
				return true
			}
		}
		return false
	}
	// Prospective creation is never authorized by the broad selector union.
	// File must publish an exact kind/variant fact, including the variant-less
	// file/write fact, before a missing target can be considered feasible.
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(wanted) {
			return true
		}
	}
	return false
}

func fileOperationTargetRef(operation runtimemodel.RuntimeOperation) string {
	if len(operation.TargetRefs) == 1 {
		return strings.TrimSpace(operation.TargetRefs[0])
	}
	return ""
}

func fileOperationRequiresRegularTarget(operation runtimemodel.RuntimeOperation) bool {
	switch operation.Kind {
	case runtimemodel.StepKindFileRead, runtimemodel.StepKindFileEdit, runtimemodel.StepKindFileWrite, runtimemodel.RuntimeBlockKindPatchApply, runtimemodel.RuntimeBlockKindPatchPreview:
		return true
	default:
		return false
	}
}

func fileOperationRequiresExistingTarget(operation runtimemodel.RuntimeOperation) bool {
	if operation.Kind == runtimemodel.StepKindFileRead || operation.Kind == runtimemodel.StepKindFileEdit {
		return true
	}
	if operation.Kind != runtimemodel.RuntimeBlockKindPatchApply && operation.Kind != runtimemodel.RuntimeBlockKindPatchPreview {
		return false
	}
	for _, parameter := range operation.Parameters {
		if parameter.Name == "patchType" && parameter.String != nil {
			return strings.TrimSpace(*parameter.String) == "text_replace"
		}
	}
	return false
}

func fileOperationInfeasible(operation runtimemodel.RuntimeOperation, code, message string, targets []string) *runtimemodel.FailureFact {
	return &runtimemodel.FailureFact{
		Class:      runtimemodel.FailureClassCandidateContractFailure,
		Code:       code,
		Signature:  "operation-admissibility/" + operation.Kind + "/" + code,
		Message:    message,
		TargetRefs: targets,
	}
}

// fileOperationSnapshotPrerequisiteRequest classifies only File-owner
// feasibility facts. It does not infer a repair action: when a content-bound
// operation was rejected before the candidate received any current target
// snapshot, it asks Orchestrator to acquire that exact owner fact and seal a
// new authority. Rejections after a snapshot was already supplied remain
// ordinary candidate failures, preserving the SM-3 anti-replay fence.
func fileOperationSnapshotPrerequisiteRequest(operation runtimemodel.RuntimeOperation, failure *runtimemodel.FailureFact, current *runtimemodel.TargetSnapshot) *runtimemodel.RuntimeOperationPrerequisiteRequest {
	if failure == nil || !runtimemodel.RuntimeOperationRequiresCurrentTargetSnapshot(operation) {
		return nil
	}
	targetRef := fileOperationTargetRef(operation)
	if targetRef == "" {
		return nil
	}
	if current != nil && current.Complete() && strings.TrimSpace(current.TargetRef) == targetRef {
		return nil
	}
	switch strings.TrimSpace(failure.Code) {
	case "operation_replay_proof_unavailable", "patch_text_replace_conflict", "patch_unified_diff_conflict", "patch_basehash_mismatch", "operation_base_hash_mismatch":
		// These codes are emitted by the File owner only after it has compared a
		// content-bound operation against the current regular-file content.
	default:
		return nil
	}
	return &runtimemodel.RuntimeOperationPrerequisiteRequest{
		OperationID: operation.Normalized().ID, Kind: runtimemodel.PrerequisiteKindTargetSnapshot,
		OwnerModuleID: ModuleID, SubjectRef: targetRef,
		Reason: "content-bound file operation requires a current owner target snapshot",
	}
}

func guardedWorkspacePath(deliverableDirectory string, inputPath string, requireExisting bool) (string, string, string, error) {
	if strings.TrimSpace(deliverableDirectory) == "" || strings.TrimSpace(inputPath) == "" {
		return "", "", "", common.ErrInvalidArgument
	}
	if filepath.IsAbs(inputPath) {
		return "", "", "", common.ErrInvalidArgument
	}
	root, err := filepath.Abs(deliverableDirectory)
	if err != nil {
		return "", "", "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", "", err
	}
	clean := filepath.Clean(inputPath)
	if clean == "." {
		// ok
	} else if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", "", common.ErrInvalidArgument
	}
	target := filepath.Join(root, clean)
	if requireExisting {
		target, err = filepath.EvalSymlinks(target)
		if err != nil {
			return "", "", "", err
		}
	} else if existing := nearestExistingPath(target); existing != "" {
		resolved, evalErr := filepath.EvalSymlinks(existing)
		if evalErr != nil {
			return "", "", "", evalErr
		}
		if !pathInside(root, resolved) {
			return "", "", "", common.ErrInvalidArgument
		}
	}
	if !pathInside(root, target) {
		return "", "", "", common.ErrInvalidArgument
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", "", "", err
	}
	return root, target, rel, nil
}

func nearestExistingPath(path string) string {
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			return current
		}
		next := filepath.Dir(current)
		if next == current {
			return ""
		}
		current = next
	}
}

func pathInside(root string, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}

func shouldSkipWorkspaceListPath(path string) bool {
	base := filepath.Base(path)
	switch base {
	case ".git", "node_modules", "vendor", "dist", "build", ".workorch-data":
		return true
	default:
		return false
	}
}

func fileKind(info os.FileInfo) string {
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "directory"
	}
	return "file"
}

func fileHashAndBinary(path string, sampleLimit int) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	hash := sha256.Sum256(data)
	if len(data) > sampleLimit {
		data = data[:sampleLimit]
	}
	return hex.EncodeToString(hash[:]), looksBinary(data)
}

func looksBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(data)
}

func lineRange(content string, startLine int, endLine int) string {
	lines := strings.SplitAfter(content, "\n")
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 || endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine || startLine > len(lines) {
		return ""
	}
	return strings.Join(lines[startLine-1:endLine], "")
}

func replaceOccurrence(content string, oldText string, newText string, occurrence int) string {
	if occurrence <= 0 {
		occurrence = 1
	}
	start := 0
	for idx := 1; idx <= occurrence; idx++ {
		pos := strings.Index(content[start:], oldText)
		if pos < 0 {
			return content
		}
		absolute := start + pos
		if idx == occurrence {
			return content[:absolute] + newText + content[absolute+len(oldText):]
		}
		start = absolute + len(oldText)
	}
	return content
}

func unifiedDiffForEdit(path string, before string, after string) string {
	beforeLines, _ := splitEditTextLines(before)
	afterLines, _ := splitEditTextLines(after)
	prefix := 0
	for prefix < len(beforeLines) && prefix < len(afterLines) && beforeLines[prefix] == afterLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(beforeLines)-prefix &&
		suffix < len(afterLines)-prefix &&
		beforeLines[len(beforeLines)-1-suffix] == afterLines[len(afterLines)-1-suffix] {
		suffix++
	}
	const contextLines = 3
	contextStart := prefix - contextLines
	if contextStart < 0 {
		contextStart = 0
	}
	beforeChangeEnd := len(beforeLines) - suffix
	afterChangeEnd := len(afterLines) - suffix
	beforeEnd := beforeChangeEnd + contextLines
	if beforeEnd > len(beforeLines) {
		beforeEnd = len(beforeLines)
	}
	afterEnd := afterChangeEnd + contextLines
	if afterEnd > len(afterLines) {
		afterEnd = len(afterLines)
	}
	oldCount := beforeEnd - contextStart
	newCount := afterEnd - contextStart
	oldStart := contextStart + 1
	newStart := contextStart + 1
	var b strings.Builder
	b.WriteString("--- a/" + path + "\n")
	b.WriteString("+++ b/" + path + "\n")
	b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
	for i := contextStart; i < prefix; i++ {
		b.WriteString(" " + beforeLines[i] + "\n")
	}
	for i := prefix; i < beforeChangeEnd; i++ {
		b.WriteString("-" + beforeLines[i] + "\n")
	}
	for i := prefix; i < afterChangeEnd; i++ {
		b.WriteString("+" + afterLines[i] + "\n")
	}
	for i := beforeChangeEnd; i < beforeEnd; i++ {
		b.WriteString(" " + beforeLines[i] + "\n")
	}
	return b.String()
}

func splitEditTextLines(content string) ([]string, bool) {
	if content == "" {
		return nil, false
	}
	finalNewline := strings.HasSuffix(content, "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return []string{""}, finalNewline
	}
	return strings.Split(content, "\n"), finalNewline
}

func diffLineStats(diff string) (int, int) {
	added := 0
	deleted := 0
	for _, line := range strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			deleted++
		}
	}
	return added, deleted
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func containsRuntimeRedactionMarker(value string) bool {
	value = strings.ToUpper(value)
	return strings.Contains(value, "[REDACTED]") ||
		strings.Contains(value, "[SECRET]") ||
		strings.Contains(value, "[MASKED]")
}

// editApprovalRequired identifies paths whose mutation crosses a trust
// boundary. Change volume is deliberately not a governance signal: a large,
// workspace-local source edit remains an ordinary Runtime state change and
// must be allowed to converge without manufacturing a human decision ticket.
//
// Owners may still report ordinary patch conflicts or invalid inputs, which
// stay in the typed execution-repair path. Only this explicit sensitive-path
// classification may produce approval_required and a governance receipt.
func editApprovalRequired(path string) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(strings.TrimSpace(path))))
	base := filepath.Base(clean)
	switch {
	case clean == ".git" || strings.HasPrefix(clean, ".git/"):
		return true
	case clean == ".ssh" || strings.HasPrefix(clean, ".ssh/"):
		return true
	case clean == ".github/workflows" || strings.HasPrefix(clean, ".github/workflows/"):
		return true
	case base == ".env" || strings.HasPrefix(base, ".env."):
		return true
	case strings.Contains(base, "id_rsa") || strings.Contains(base, "id_ed25519"):
		return true
	case strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".crt"):
		return true
	default:
		return false
	}
}
