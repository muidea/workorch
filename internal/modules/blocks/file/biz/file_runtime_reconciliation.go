package file

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// fileRuntimeReconciliationTargetFact is a File-private, post-effect
// observation. It deliberately does not reuse RuntimeTargetSelection: that
// type describes pre-effect admission, where an existing prospective target
// must remain unsupported for create-only operations.
type fileRuntimeReconciliationTargetFact struct {
	status            runtimecontract.RuntimeCWIReconciliationObservationStatus
	observedRevision  string
	targetRefs        []string
	diagnosticCode    string
	diagnosticMessage string
}

// observeRuntimeReconciliationTargets reports only the current File-owned
// resource fact for a sealed reconciliation binding. It never selects a CWI
// disposition and never mutates the workspace.
func (a *RuntimeAdapter) observeRuntimeReconciliationTargets(workspaceDirectory string, binding runtimecontract.RuntimeCWIReconciliationBindingRef) (fileRuntimeReconciliationTargetFact, error) {
	if a == nil || strings.TrimSpace(workspaceDirectory) == "" {
		return fileRuntimeReconciliationTargetFact{}, common.ErrInvalidArgument
	}
	binding = binding.Normalized()
	if err := binding.Validate(); err != nil {
		return fileRuntimeReconciliationTargetFact{}, err
	}
	manifest, err := a.AcquireWorkspaceManifest(workspaceDirectory)
	if err != nil {
		return fileRuntimeReconciliationTargetFact{}, err
	}
	known := make(map[string]struct{}, len(manifest.TargetRefs))
	for _, targetRef := range manifest.TargetRefs {
		known[targetRef] = struct{}{}
	}
	if invalidProspective := invalidFileReconciliationProspectiveSelector(binding, manifest.Revision); invalidProspective != nil {
		return *invalidProspective, nil
	}

	if len(binding.TargetRefs) != 0 {
		return observeSealedFileTargets(workspaceDirectory, manifest.Revision, known, binding.TargetRefs, false), nil
	}
	if len(binding.ArtifactRefs) != 0 {
		return observeSealedFileTargets(workspaceDirectory, manifest.Revision, known, binding.ArtifactRefs, true), nil
	}
	return observeSealedFileSelector(workspaceDirectory, manifest, known, binding), nil
}

func invalidFileReconciliationProspectiveSelector(binding runtimecontract.RuntimeCWIReconciliationBindingRef, manifestRevision string) *fileRuntimeReconciliationTargetFact {
	if binding.ProspectiveTargetRef == "" {
		return nil
	}
	var fact fileRuntimeReconciliationTargetFact
	switch binding.TargetSelectorRef {
	case runtimeFileSelectorWorkspaceRelativeNewTarget, runtimeFileTargetSelectorWorkspaceDeliverable:
		return nil
	case runtimeFileTargetSelectorProfileCandidates:
		if binding.TargetSelectorOwnerRef == "" || binding.TargetSelectorSubjectRef == "" || len(binding.TargetSelectorCandidateRefs) == 0 {
			fact = unverifiableFileReconciliationFact(manifestRevision, "profile File target selector has no complete sealed owner scope", "target_selector_scope_incomplete", nil)
		} else if !containsString(binding.TargetSelectorCandidateRefs, binding.ProspectiveTargetRef) {
			fact = unverifiableFileReconciliationFact(manifestRevision, "sealed prospective File target is outside the declared selector candidates", "prospective_target_outside_selector", nil)
		} else {
			return nil
		}
	default:
		fact = unverifiableFileReconciliationFact(manifestRevision, "sealed File target selector does not authorize a prospective target", "prospective_target_unsupported_for_selector", nil)
	}
	return &fact
}

func observeSealedFileSelector(workspaceDirectory string, manifest runtimecontract.WorkspaceManifest, known map[string]struct{}, binding runtimecontract.RuntimeCWIReconciliationBindingRef) fileRuntimeReconciliationTargetFact {
	switch binding.TargetSelectorRef {
	case runtimeFileSelectorWorkspaceRelativeNewTarget:
		return observeWorkspaceRelativeNewFileSelector(workspaceDirectory, manifest.Revision, known, binding)
	case runtimeFileTargetSelectorUniqueWorkspaceEntry:
		return observeUniqueWorkspaceEntrySelector(manifest, binding)
	case runtimeFileTargetSelectorWorkspaceDeliverable:
		return observeWorkspaceDeliverableSelector(workspaceDirectory, manifest, known, binding)
	case runtimeFileTargetSelectorProfileCandidates:
		return observeProfileDeclaredFileSelector(workspaceDirectory, manifest, known, binding)
	default:
		return unverifiableFileReconciliationFact(manifest.Revision, "sealed File target selector is unsupported", "target_selector_unsupported", nil)
	}
}

func observeWorkspaceRelativeNewFileSelector(workspaceDirectory, manifestRevision string, known map[string]struct{}, binding runtimecontract.RuntimeCWIReconciliationBindingRef) fileRuntimeReconciliationTargetFact {
	if binding.ProspectiveTargetRef == "" {
		return unverifiableFileReconciliationFact(manifestRevision, "prospective File target selector has no sealed target path", "prospective_target_missing", nil)
	}
	return observeSealedFileTargets(workspaceDirectory, manifestRevision, known, []string{binding.ProspectiveTargetRef}, false)
}

func observeUniqueWorkspaceEntrySelector(manifest runtimecontract.WorkspaceManifest, binding runtimecontract.RuntimeCWIReconciliationBindingRef) fileRuntimeReconciliationTargetFact {
	if binding.ProspectiveTargetRef != "" {
		return unverifiableFileReconciliationFact(manifest.Revision, "unique workspace entry selector does not authorize a prospective target", "prospective_target_unsupported_for_selector", nil)
	}
	return observeFileSelectorCardinality(manifest.Revision, manifest.TargetRefs, false)
}

func observeWorkspaceDeliverableSelector(workspaceDirectory string, manifest runtimecontract.WorkspaceManifest, known map[string]struct{}, binding runtimecontract.RuntimeCWIReconciliationBindingRef) fileRuntimeReconciliationTargetFact {
	if binding.ProspectiveTargetRef != "" {
		prospective := observeSealedFileTargets(workspaceDirectory, manifest.Revision, known, []string{binding.ProspectiveTargetRef}, false)
		if prospective.status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
			return prospective
		}
	}
	// workspace_deliverable is a complete bounded owner scope, not a unique
	// selector. Every current manifest target is therefore part of one factual
	// satisfied observation; cardinality greater than one is not ambiguity.
	return observeFileSelectorCardinality(manifest.Revision, manifest.TargetRefs, true)
}

func observeProfileDeclaredFileSelector(workspaceDirectory string, manifest runtimecontract.WorkspaceManifest, known map[string]struct{}, binding runtimecontract.RuntimeCWIReconciliationBindingRef) fileRuntimeReconciliationTargetFact {
	if binding.TargetSelectorOwnerRef == "" || binding.TargetSelectorSubjectRef == "" || len(binding.TargetSelectorCandidateRefs) == 0 {
		return unverifiableFileReconciliationFact(manifest.Revision, "profile File target selector has no complete sealed owner scope", "target_selector_scope_incomplete", nil)
	}
	candidates := make([]string, 0, len(binding.TargetSelectorCandidateRefs))
	for _, candidate := range binding.TargetSelectorCandidateRefs {
		canonical, _, ok := canonicalFileReconciliationTarget(workspaceDirectory, candidate)
		if !ok {
			return unverifiableFileReconciliationFact(manifest.Revision, "profile File target selector contains a non-canonical or out-of-workspace path", "target_outside_workspace", nil)
		}
		if !runtimeFileDeliverableTargetEligible(canonical) {
			return unverifiableFileReconciliationFact(manifest.Revision, "profile File target selector contains a path outside the deliverable manifest", "target_outside_manifest", nil)
		}
		if _, found := known[canonical]; found {
			candidates = append(candidates, canonical)
		}
	}
	if len(candidates) > 1 {
		return unverifiableFileReconciliationFact(manifest.Revision, "sealed File target selector resolves to multiple current workspace paths", "target_selection_ambiguous", nil)
	}
	if binding.ProspectiveTargetRef != "" {
		if !containsString(binding.TargetSelectorCandidateRefs, binding.ProspectiveTargetRef) {
			return unverifiableFileReconciliationFact(manifest.Revision, "sealed prospective File target is outside the declared selector candidates", "prospective_target_outside_selector", nil)
		}
		return observeSealedFileTargets(workspaceDirectory, manifest.Revision, known, []string{binding.ProspectiveTargetRef}, false)
	}
	return observeFileSelectorCardinality(manifest.Revision, candidates, false)
}

func observeFileSelectorCardinality(manifestRevision string, candidates []string, completeScope bool) fileRuntimeReconciliationTargetFact {
	if len(candidates) == 0 {
		return fileRuntimeReconciliationTargetFact{
			status:            runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied,
			observedRevision:  manifestRevision,
			diagnosticCode:    "target_selection_unsatisfied",
			diagnosticMessage: "sealed File target selection is absent from the current workspace",
		}
	}
	if len(candidates) == 1 || completeScope {
		return fileRuntimeReconciliationTargetFact{
			status:           runtimecontract.RuntimeCWIReconciliationObservationSatisfied,
			observedRevision: manifestRevision,
			targetRefs:       append([]string(nil), candidates...),
		}
	}
	return unverifiableFileReconciliationFact(manifestRevision, "sealed File target selector resolves to multiple current workspace paths", "target_selection_ambiguous", nil)
}

func observeSealedFileTargets(workspaceDirectory, manifestRevision string, known map[string]struct{}, targetRefs []string, artifactRefs bool) fileRuntimeReconciliationTargetFact {
	canonicalRefs := make([]string, 0, len(targetRefs))
	missing := false
	for _, targetRef := range targetRefs {
		canonical, absolute, ok := canonicalFileReconciliationTarget(workspaceDirectory, targetRef)
		if !ok {
			code := "target_outside_workspace"
			message := "sealed File target is non-canonical or outside the workspace"
			if artifactRefs {
				code = "artifact_scope_unverifiable"
				message = "binding artifact reference is not a directly verifiable File target"
			}
			return unverifiableFileReconciliationFact(manifestRevision, message, code, nil)
		}
		if !runtimeFileDeliverableTargetEligible(canonical) {
			code := "target_outside_manifest"
			message := "sealed File target is outside the deliverable manifest scope"
			if artifactRefs {
				code = "artifact_scope_unverifiable"
				message = "binding artifact reference is outside the directly verifiable File manifest"
			}
			return unverifiableFileReconciliationFact(manifestRevision, message, code, nil)
		}
		canonicalRefs = append(canonicalRefs, canonical)
		if _, found := known[canonical]; found {
			continue
		}
		info, err := os.Lstat(absolute)
		switch {
		case os.IsNotExist(err):
			missing = true
		case err != nil:
			return unverifiableFileReconciliationFact(manifestRevision, "sealed File target cannot be inspected", "target_observation_unavailable", canonicalRefs)
		case !info.Mode().IsRegular():
			return unverifiableFileReconciliationFact(manifestRevision, "sealed File target is not a regular manifest file", "target_outside_manifest", canonicalRefs)
		default:
			// A regular file that is intentionally outside the deliverable
			// manifest (for example an ignored owner-internal path) is not a
			// missing deliverable and cannot be treated as satisfied.
			return unverifiableFileReconciliationFact(manifestRevision, "sealed File target is outside the current deliverable manifest", "target_outside_manifest", canonicalRefs)
		}
	}
	if missing {
		return fileRuntimeReconciliationTargetFact{
			status:            runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied,
			observedRevision:  manifestRevision,
			targetRefs:        canonicalRefs,
			diagnosticCode:    "target_missing",
			diagnosticMessage: "sealed File target is absent from the current workspace",
		}
	}
	return fileRuntimeReconciliationTargetFact{
		status:           runtimecontract.RuntimeCWIReconciliationObservationSatisfied,
		observedRevision: manifestRevision,
		targetRefs:       canonicalRefs,
	}
}

func canonicalFileReconciliationTarget(workspaceDirectory, targetRef string) (string, string, bool) {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" || strings.Contains(targetRef, "\\") {
		return "", "", false
	}
	_, absolute, relative, err := guardedWorkspacePath(workspaceDirectory, targetRef, false)
	if err != nil {
		return "", "", false
	}
	canonical := filepath.ToSlash(relative)
	if canonical == "." || canonical != targetRef {
		return "", "", false
	}
	return canonical, absolute, true
}

func unverifiableFileReconciliationFact(revision, message, code string, targetRefs []string) fileRuntimeReconciliationTargetFact {
	return fileRuntimeReconciliationTargetFact{
		status:            runtimecontract.RuntimeCWIReconciliationObservationUnverifiable,
		observedRevision:  revision,
		targetRefs:        append([]string(nil), targetRefs...),
		diagnosticCode:    code,
		diagnosticMessage: message,
	}
}
