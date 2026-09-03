package file

import (
	"os"
	"path/filepath"
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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

// fileOperationCandidateApplicabilityViolation translates only the File-
// private in-memory unified-diff context miss into the manifest-declared,
// provider-safe owner fact. It deliberately carries no retry or terminal
// choice; those belong to Orchestrator's private SM-3 transition.
func fileOperationCandidateApplicabilityViolation(operation runtimemodel.RuntimeOperation, authorization runtimemodel.RuntimeCandidateOperationManifest, inventory runtimemodel.RuntimeOperationAdmissibility, failure *runtimemodel.FailureFact) *runtimemodel.RuntimeOperationCandidateConstraintViolation {
	operation = operation.Normalized()
	if failure == nil || strings.TrimSpace(failure.Code) != "unified_diff_apply_conflict" {
		return nil
	}
	_, constraints, err := runtimemodel.RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
	if err != nil {
		return nil
	}
	for _, constraint := range constraints {
		if constraint.Kind != runtimemodel.RuntimeOperationCandidateConstraintTargetApplicability ||
			constraint.RequiredShape != runtimemodel.RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget {
			continue
		}
		return &runtimemodel.RuntimeOperationCandidateConstraintViolation{
			ConstraintRef: constraint.Ref, ParameterName: constraint.ParameterName,
			RequiredShape: constraint.RequiredShape,
			ActualShape:   runtimemodel.RuntimeOperationCandidateActualShapeObservedTargetContextMismatch,
			TargetRef:     fileOperationTargetRef(operation), ObservedRevision: inventory.Normalized().ObservedRevision,
		}
	}
	return nil
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
	case "operation_replay_proof_unavailable", "patch_text_replace_conflict", "patch_unified_diff_conflict", "unified_diff_apply_conflict", "patch_basehash_mismatch", "operation_base_hash_mismatch":
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
