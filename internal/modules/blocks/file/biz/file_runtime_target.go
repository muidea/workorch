package file

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	request, err := normalizeRuntimeTargetSelectionRequest(
		workspaceDirectory, prerequisiteID, targetRole, selectorRef, prospectiveTargetRef, maxItems, bindings,
	)
	if err != nil {
		return runtimemodel.RuntimeTargetSelection{}, err
	}
	if !runtimeFileTargetSelectorSupported(request.selectorRef) {
		return unsupportedRuntimeTargetSelection(request)
	}
	manifest, err := a.AcquireWorkspaceManifest(request.workspaceDirectory)
	if err != nil {
		return runtimemodel.RuntimeTargetSelection{}, err
	}
	switch request.selectorRef {
	case runtimeFileSelectorWorkspaceRelativeNewTarget:
		return acquireProspectiveRuntimeTargetSelection(request, manifest)
	case runtimeFileTargetSelectorProfileCandidates:
		return acquireProfileDeclaredTargetSelection(
			request.workspaceDirectory, request.prerequisiteID, request.targetRole, request.selectorRef,
			request.prospectiveTargetRef, request.maxItems, manifest, request.binding,
		)
	default:
		return acquireManifestRuntimeTargetSelection(request, manifest)
	}
}

type runtimeTargetSelectionRequest struct {
	workspaceDirectory   string
	prerequisiteID       string
	targetRole           string
	selectorRef          string
	prospectiveTargetRef string
	maxItems             int
	binding              runtimeTargetSelectionBinding
}

func normalizeRuntimeTargetSelectionRequest(workspaceDirectory, prerequisiteID, targetRole, selectorRef, prospectiveTargetRef string, maxItems int, bindings []runtimeTargetSelectionBinding) (runtimeTargetSelectionRequest, error) {
	request := runtimeTargetSelectionRequest{
		workspaceDirectory:   workspaceDirectory,
		prerequisiteID:       prerequisiteID,
		targetRole:           targetRole,
		selectorRef:          strings.TrimSpace(selectorRef),
		prospectiveTargetRef: strings.TrimSpace(prospectiveTargetRef),
		maxItems:             maxItems,
	}
	if strings.TrimSpace(request.prerequisiteID) == "" || strings.TrimSpace(request.targetRole) == "" || request.selectorRef == "" || request.maxItems < 1 || request.maxItems > runtimemodel.RuntimeOwnerInventoryItemsMaxCount {
		return runtimeTargetSelectionRequest{}, common.ErrInvalidArgument
	}
	if len(bindings) > 1 {
		return runtimeTargetSelectionRequest{}, common.ErrInvalidArgument
	}
	if len(bindings) == 1 {
		request.binding.ownerRef = strings.TrimSpace(bindings[0].ownerRef)
		request.binding.subjectRef = strings.TrimSpace(bindings[0].subjectRef)
		request.binding.candidateRefs = normalizeRuntimeFileTargetRefs(bindings[0].candidateRefs)
	}
	return request, nil
}

func runtimeFileTargetSelectorSupported(selectorRef string) bool {
	switch selectorRef {
	case runtimeFileSelectorWorkspaceRelativeNewTarget,
		runtimeFileTargetSelectorUniqueWorkspaceEntry,
		runtimeFileTargetSelectorWorkspaceDeliverable,
		runtimeFileTargetSelectorProfileCandidates:
		return true
	default:
		return false
	}
}

func unsupportedRuntimeTargetSelection(request runtimeTargetSelectionRequest) (runtimemodel.RuntimeTargetSelection, error) {
	selection := runtimemodel.RuntimeTargetSelection{
		PrerequisiteID: request.prerequisiteID, OwnerModuleID: runtimemodel.RuntimeModuleFile,
		TargetRole: request.targetRole, SelectorRef: request.selectorRef,
		SelectorOwnerRef: request.binding.ownerRef, SelectorSubjectRef: request.binding.subjectRef,
		SelectorCandidateRefs: request.binding.candidateRefs, ProspectiveTargetRef: request.prospectiveTargetRef,
		Disposition:      runtimemodel.RuntimeTargetSelectionUnsupported,
		ObservedRevision: "unsupported-selector:" + request.selectorRef,
	}
	return sealRuntimeTargetSelection(selection)
}

// acquireProspectiveRuntimeTargetSelection verifies one caller-declared path.
// It never derives a target from the workspace inventory.
func acquireProspectiveRuntimeTargetSelection(request runtimeTargetSelectionRequest, manifest runtimemodel.WorkspaceManifest) (runtimemodel.RuntimeTargetSelection, error) {
	selection := runtimeTargetSelectionForManifest(request, manifest)
	if request.prospectiveTargetRef == "" || strings.Contains(request.prospectiveTargetRef, "\\") {
		selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
		return sealRuntimeTargetSelection(selection)
	}
	_, target, relative, pathErr := guardedWorkspacePath(request.workspaceDirectory, request.prospectiveTargetRef, false)
	if pathErr != nil || filepath.ToSlash(relative) != request.prospectiveTargetRef {
		selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
		return sealRuntimeTargetSelection(selection)
	}
	if _, statErr := os.Lstat(target); statErr == nil {
		selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
	} else if !os.IsNotExist(statErr) {
		return runtimemodel.RuntimeTargetSelection{}, statErr
	} else {
		selection.Disposition = runtimemodel.RuntimeTargetSelectionCreate
	}
	return sealRuntimeTargetSelection(selection)
}

func acquireManifestRuntimeTargetSelection(request runtimeTargetSelectionRequest, manifest runtimemodel.WorkspaceManifest) (runtimemodel.RuntimeTargetSelection, error) {
	candidates := boundedAdmissibilityTargetRefs(nil, manifest.TargetRefs, request.maxItems)
	selection := runtimeTargetSelectionForManifest(request, manifest)
	selection.CandidateTargetRefs = candidates
	selection.CandidateCount = len(manifest.TargetRefs)
	switch selection.CandidateCount {
	case 0:
		if request.selectorRef != runtimeFileTargetSelectorWorkspaceDeliverable {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionAbsent
			break
		}
		if request.prospectiveTargetRef == "" {
			if request.binding.ownerRef != "" && request.binding.subjectRef != "" {
				selection.Disposition = runtimemodel.RuntimeTargetSelectionScope
			} else {
				selection.Disposition = runtimemodel.RuntimeTargetSelectionAbsent
			}
			break
		}
		if strings.Contains(request.prospectiveTargetRef, "\\") {
			return runtimemodel.RuntimeTargetSelection{}, fmt.Errorf("%w: prospective target must use canonical workspace-relative separators", common.ErrInvalidArgument)
		}
		_, target, relative, err := guardedWorkspacePath(request.workspaceDirectory, request.prospectiveTargetRef, false)
		if err != nil {
			return runtimemodel.RuntimeTargetSelection{}, err
		}
		if canonical := filepath.ToSlash(relative); canonical != request.prospectiveTargetRef {
			return runtimemodel.RuntimeTargetSelection{}, fmt.Errorf("%w: prospective target %q is not canonical (want %q)", common.ErrInvalidArgument, request.prospectiveTargetRef, canonical)
		}
		if _, err := os.Lstat(target); err == nil {
			return runtimemodel.RuntimeTargetSelection{}, fmt.Errorf("%w: prospective target %q already exists", common.ErrInvalidArgument, filepath.ToSlash(relative))
		} else if !os.IsNotExist(err) {
			return runtimemodel.RuntimeTargetSelection{}, err
		}
		selection.Disposition = runtimemodel.RuntimeTargetSelectionCreate
		selection.ProspectiveTargetRef = filepath.ToSlash(relative)
	case 1:
		selection.Disposition = runtimemodel.RuntimeTargetSelectionSelected
		selection.TargetRefs = append([]string(nil), candidates...)
	default:
		if request.selectorRef == runtimeFileTargetSelectorWorkspaceDeliverable && len(candidates) == selection.CandidateCount {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionScope
			selection.TargetRefs = append([]string(nil), candidates...)
		} else {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionAmbiguous
		}
	}
	return sealRuntimeTargetSelection(selection)
}

func runtimeTargetSelectionForManifest(request runtimeTargetSelectionRequest, manifest runtimemodel.WorkspaceManifest) runtimemodel.RuntimeTargetSelection {
	return runtimemodel.RuntimeTargetSelection{
		PrerequisiteID: request.prerequisiteID, OwnerModuleID: runtimemodel.RuntimeModuleFile,
		TargetRole: request.targetRole, SelectorRef: request.selectorRef,
		SelectorOwnerRef: request.binding.ownerRef, SelectorSubjectRef: request.binding.subjectRef,
		SelectorCandidateRefs: request.binding.candidateRefs, ProspectiveTargetRef: request.prospectiveTargetRef,
		ObservedRevision: manifest.Revision,
	}
}

func sealRuntimeTargetSelection(selection runtimemodel.RuntimeTargetSelection) (runtimemodel.RuntimeTargetSelection, error) {
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
