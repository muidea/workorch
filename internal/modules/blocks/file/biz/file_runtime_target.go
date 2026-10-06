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
// workspace-deliverable selector has both an SM-2 binding-sealed creation form for
// an empty inventory and a bounded complete-scope form for multiple entries.
// File never derives a path: it only verifies the binding-sealed prospective path
// and operation admissibility validates every operation target.
type runtimeTargetSelectionBinding struct {
	ownerRef                             string
	subjectRef                           string
	candidateRefs                        []string
	prospectiveTargetAllOfConstraintRefs []string
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
	if len(request.binding.prospectiveTargetAllOfConstraintRefs) != 0 &&
		!runtimemodel.SameRuntimeProspectiveTargetAllOfConstraintRefs(request.binding.prospectiveTargetAllOfConstraintRefs, runtimeFileProspectiveTargetAllOfConstraintRefs()) {
		return unsupportedRuntimeTargetSelection(request)
	}
	observation, err := a.acquireWorkspaceManifestObservation(request.workspaceDirectory)
	if err != nil {
		return runtimemodel.RuntimeTargetSelection{}, err
	}
	manifest := observation.manifest
	request.manifestEntryCount = observation.entryCount
	switch request.selectorRef {
	case runtimeFileSelectorWorkspaceRelativeNewTarget:
		return acquireProspectiveRuntimeTargetSelection(request, manifest)
	case runtimeFileTargetSelectorUniqueWorkspaceEntry:
		return acquireUniqueWorkspaceEntryRuntimeTargetSelection(request, manifest)
	case runtimeFileTargetSelectorProfileCandidates:
		return acquireBoundedCandidateTargetSelection(
			request.workspaceDirectory, request.prerequisiteID, request.targetRole, request.selectorRef,
			request.prospectiveTargetRef, request.maxItems, observation.entryCount, manifest, request.binding,
		)
	default:
		return acquireManifestRuntimeTargetSelection(request, manifest)
	}
}

// acquireUniqueWorkspaceEntryRuntimeTargetSelection keeps the unique selector
// strictly observational. Unlike the explicit prospective selectors, it never
// authorizes a caller-declared path: combining the two concepts is a typed
// unsupported admission even when the current manifest happens to contain
// zero or one matching entry.
func acquireUniqueWorkspaceEntryRuntimeTargetSelection(request runtimeTargetSelectionRequest, manifest runtimemodel.WorkspaceManifest) (runtimemodel.RuntimeTargetSelection, error) {
	if request.prospectiveTargetRef != "" {
		selection := runtimeTargetSelectionForManifest(request, manifest)
		selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
		return sealRuntimeTargetSelection(selection)
	}
	return acquireManifestRuntimeTargetSelection(request, manifest)
}

type runtimeTargetSelectionRequest struct {
	workspaceDirectory   string
	prerequisiteID       string
	targetRole           string
	selectorRef          string
	prospectiveTargetRef string
	maxItems             int
	manifestEntryCount   int
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
		request.binding.prospectiveTargetAllOfConstraintRefs = runtimemodel.NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(bindings[0].prospectiveTargetAllOfConstraintRefs)
		if err := runtimemodel.ValidateRuntimeProspectiveTargetAllOfConstraintRefs(request.binding.prospectiveTargetAllOfConstraintRefs); err != nil {
			return runtimeTargetSelectionRequest{}, common.ErrInvalidArgument
		}
		if len(request.binding.prospectiveTargetAllOfConstraintRefs) != 0 && (request.binding.ownerRef == "" || request.binding.subjectRef == "") {
			return runtimeTargetSelectionRequest{}, common.ErrInvalidArgument
		}
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
		SelectorCandidateRefs: request.binding.candidateRefs, ProspectiveTargetAllOfConstraintRefs: request.binding.prospectiveTargetAllOfConstraintRefs, ProspectiveTargetRef: request.prospectiveTargetRef,
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
	if !runtimeFileDeliverableTargetEligible(request.prospectiveTargetRef) {
		selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
		return sealRuntimeTargetSelection(selection)
	}
	if runtimeFileProspectiveTargetWouldOverflowManifest(request.workspaceDirectory, manifest, request.manifestEntryCount, request.prospectiveTargetRef) {
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
	var prospectiveTarget string
	if request.prospectiveTargetRef != "" {
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
		if !runtimeFileDeliverableTargetEligible(request.prospectiveTargetRef) {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
			return sealRuntimeTargetSelection(selection)
		}
		if runtimeFileProspectiveTargetWouldOverflowManifest(request.workspaceDirectory, manifest, request.manifestEntryCount, request.prospectiveTargetRef) {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
			return sealRuntimeTargetSelection(selection)
		}
		prospectiveTarget = target
		// workspace_deliverable is the complete bounded File-owner scope.  An
		// eligible absent prospective path remains an explicit part of that scope
		// admission even when other deliverables already exist; it is not inferred
		// from, or silently replaced by, one of the current manifest entries.
	}
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
		if _, err := os.Lstat(prospectiveTarget); err == nil {
			return runtimemodel.RuntimeTargetSelection{}, fmt.Errorf("%w: prospective target %q already exists", common.ErrInvalidArgument, request.prospectiveTargetRef)
		} else if !os.IsNotExist(err) {
			return runtimemodel.RuntimeTargetSelection{}, err
		}
		selection.Disposition = runtimemodel.RuntimeTargetSelectionCreate
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
		SelectorCandidateRefs: request.binding.candidateRefs, ProspectiveTargetAllOfConstraintRefs: request.binding.prospectiveTargetAllOfConstraintRefs, ProspectiveTargetRef: request.prospectiveTargetRef,
		ObservedRevision: manifest.Revision,
	}
}

func sealRuntimeTargetSelection(selection runtimemodel.RuntimeTargetSelection) (runtimemodel.RuntimeTargetSelection, error) {
	// owner 在封存事实前统一合同规定的集合表示；hash 与返回前像必须完全一致。
	selection = selection.Normalized()
	selection.SnapshotID = runtimemodel.StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err != nil {
		return runtimemodel.RuntimeTargetSelection{}, err
	}
	return selection, nil
}

// acquireBoundedCandidateTargetSelection intersects the Definition-declared,
// bounded candidate set with the File manifest. An exact prospective target,
// when present, has already been sealed by SM-2 binding. File only evaluates
// path and inventory facts; it does not inspect a name, suffix, content or role.
func acquireBoundedCandidateTargetSelection(workspaceDirectory, prerequisiteID, targetRole, selectorRef, prospectiveTargetRef string, maxItems int, manifestEntryCount int, manifest runtimemodel.WorkspaceManifest, binding runtimeTargetSelectionBinding) (runtimemodel.RuntimeTargetSelection, error) {
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
	selection := runtimemodel.RuntimeTargetSelection{
		PrerequisiteID: prerequisiteID, OwnerModuleID: runtimemodel.RuntimeModuleFile, TargetRole: targetRole, SelectorRef: selectorRef,
		SelectorOwnerRef: binding.ownerRef, SelectorSubjectRef: binding.subjectRef, SelectorCandidateRefs: binding.candidateRefs,
		ProspectiveTargetRef: prospectiveTargetRef, ObservedRevision: manifest.Revision,
	}
	var prospectiveTarget string
	if prospectiveTargetRef != "" {
		if _, ok := declared[prospectiveTargetRef]; !ok || strings.Contains(prospectiveTargetRef, "\\") {
			return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
		}
		_, target, relative, err := guardedWorkspacePath(workspaceDirectory, prospectiveTargetRef, false)
		if err != nil || filepath.ToSlash(relative) != prospectiveTargetRef {
			return runtimemodel.RuntimeTargetSelection{}, common.ErrInvalidArgument
		}
		if !runtimeFileDeliverableTargetEligible(prospectiveTargetRef) {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
			return sealRuntimeTargetSelection(selection)
		}
		if runtimeFileProspectiveTargetWouldOverflowManifest(workspaceDirectory, manifest, manifestEntryCount, prospectiveTargetRef) {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
			return sealRuntimeTargetSelection(selection)
		}
		prospectiveTarget = target
		// The binding-sealed prospective target must participate in the bounded
		// current candidate set. A different current match cannot make an absent
		// target admissible by cardinality alone.
		if len(matched) != 0 && !containsString(matched, prospectiveTargetRef) {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionUnsupported
			return sealRuntimeTargetSelection(selection)
		}
	}
	matched = boundedAdmissibilityTargetRefs(nil, matched, maxItems)
	selection.CandidateTargetRefs = matched
	selection.CandidateCount = len(matched)
	switch selection.CandidateCount {
	case 0:
		if prospectiveTargetRef == "" {
			selection.Disposition = runtimemodel.RuntimeTargetSelectionAbsent
			break
		}
		if _, err := os.Lstat(prospectiveTarget); err == nil || !os.IsNotExist(err) {
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

// runtimeFileProspectiveTargetWouldOverflowManifest prevents an admitted
// create from making the next owner observation structurally impossible. The
// full File manifest is bounded independently from the smaller candidate
// context projection, so capacity is checked against the complete inventory.
func runtimeFileProspectiveTargetWouldOverflowManifest(workspaceDirectory string, manifest runtimemodel.WorkspaceManifest, entryCount int, prospectiveTargetRef string) bool {
	if containsString(manifest.TargetRefs, prospectiveTargetRef) {
		return false
	}
	additional, err := runtimeFileProspectiveTargetAdditionalEntries(workspaceDirectory, prospectiveTargetRef)
	return err != nil || additional < 1 || entryCount+additional > defaultMaxEntries
}

// runtimeFileProspectiveTargetAdditionalEntries counts the exact File-list
// entries a create may add: the target plus every currently missing parent
// directory below the workspace root. This is owner-local capacity evidence;
// it does not authorize the operation or choose a Runtime transition.
func runtimeFileProspectiveTargetAdditionalEntries(workspaceDirectory, prospectiveTargetRef string) (int, error) {
	root, target, _, err := guardedWorkspacePath(workspaceDirectory, prospectiveTargetRef, false)
	if err != nil {
		return 0, err
	}
	additional := 0
	for current := target; current != root; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		switch {
		case statErr == nil:
			// An admitted create may not traverse even an internal symlink alias:
			// the sealed prospective ref must name one canonical workspace tree
			// location. Existing parents must be real directories; otherwise the
			// later mkdir/write effect cannot implement the admitted topology.
			if info.Mode()&os.ModeSymlink != 0 {
				return 0, fmt.Errorf("%w: prospective target %q traverses symbolic link %q", common.ErrInvalidArgument, prospectiveTargetRef, filepath.Base(current))
			}
			if current == target || !info.IsDir() {
				return 0, fmt.Errorf("%w: prospective target %q has an invalid existing path component", common.ErrInvalidArgument, prospectiveTargetRef)
			}
			return additional, nil
		case os.IsNotExist(statErr):
			additional++
		default:
			return 0, statErr
		}
	}
	return additional, nil
}

// runtimeFileProspectiveTargetRefsAdditionalEntries counts the union of files
// and missing parent directories that one bounded batch would add. Shared
// parents are counted once, so admission is deterministic against the same
// manifest observation regardless of target ordering.
func runtimeFileProspectiveTargetRefsAdditionalEntries(workspaceDirectory string, prospectiveTargetRefs []string) (int, error) {
	if len(prospectiveTargetRefs) == 0 || len(prospectiveTargetRefs) > runtimemodel.RuntimeProspectiveTargetRefsMaxCount {
		return 0, common.ErrInvalidArgument
	}
	additionalEntries := make(map[string]struct{})
	for _, prospectiveTargetRef := range prospectiveTargetRefs {
		root, target, _, err := guardedWorkspacePath(workspaceDirectory, prospectiveTargetRef, false)
		if err != nil {
			return 0, err
		}
	targetPath:
		for current := target; current != root; current = filepath.Dir(current) {
			info, statErr := os.Lstat(current)
			switch {
			case statErr == nil:
				if info.Mode()&os.ModeSymlink != 0 || current == target || !info.IsDir() {
					return 0, fmt.Errorf("%w: prospective target %q has an invalid existing path component", common.ErrInvalidArgument, prospectiveTargetRef)
				}
				break targetPath
			case os.IsNotExist(statErr):
				additionalEntries[current] = struct{}{}
			default:
				return 0, statErr
			}
		}
	}
	return len(additionalEntries), nil
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
