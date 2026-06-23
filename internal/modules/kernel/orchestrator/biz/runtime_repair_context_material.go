package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (r *repairSubsystem) runtimeRepairPatchContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	if !shouldIncludePatchRepairContext(failedStep) {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	targetRefs := patchRepairTargetRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(targetRefs), 3))
	for idx, targetRef := range targetRefs {
		if idx >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, targetRef)
		if len(item) > 0 {
			ret = append(ret, item)
		}
	}
	return ret
}

func (r *repairSubsystem) runtimeRepairPatchSuggestions(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	targetRefs := patchRepairTargetRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	patches := patchRepairRequestsFromStep(failedStep)
	if len(patches) == 0 {
		return nil
	}
	allowedTargets := map[string]bool{}
	for _, targetRef := range targetRefs {
		allowedTargets[strings.TrimSpace(targetRef)] = true
	}
	var ret []map[string]any
	for _, request := range patches {
		if len(ret) >= 5 {
			break
		}
		if len(allowedTargets) > 0 && !allowedTargets[request.TargetRef] {
			continue
		}
		switch request.PatchType {
		case "unified_diff":
			if strings.TrimSpace(request.Diff) == "" {
				continue
			}
			ret = append(ret, patchRepairSuggestionsForDiff(workspace.DeliverableDirectory, request.TargetRef, request.Diff)...)
		case "text_replace":
			if suggestion := patchRepairSuggestionForTextReplace(workspace.DeliverableDirectory, request); len(suggestion) > 0 {
				ret = append(ret, suggestion)
			}
		}
	}
	if len(ret) > 5 {
		return ret[:5]
	}
	return ret
}

func (r *repairSubsystem) runtimeRepairPatchRefreshableSignals(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	if !runtimeRepairPatchFailureNeedsCurrentTargetRefresh(failedStep) {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	targetRefs := patchRepairTargetRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	patches := patchRepairRequestsFromStep(failedStep)
	if len(patches) == 0 {
		return nil
	}
	allowedTargets := map[string]bool{}
	for _, targetRef := range targetRefs {
		allowedTargets[strings.TrimSpace(targetRef)] = true
	}
	var ret []map[string]any
	for _, request := range patches {
		if len(ret) >= 3 {
			break
		}
		if len(allowedTargets) > 0 && !allowedTargets[request.TargetRef] {
			continue
		}
		failedBaseHash := strings.TrimSpace(request.BaseHash)
		if failedBaseHash == "" {
			continue
		}
		snapshot := patchRepairTargetSnapshot(workspace.DeliverableDirectory, request.TargetRef)
		currentHash := strings.TrimSpace(stringFromMap(snapshot, "contentHash"))
		if len(snapshot) == 0 || !boolValue(snapshot["exists"]) || boolValue(snapshot["isBinary"]) || currentHash == "" {
			continue
		}
		if strings.EqualFold(failedBaseHash, currentHash) {
			continue
		}
		ret = append(ret, map[string]any{
			"targetRef":          request.TargetRef,
			"patchType":          request.PatchType,
			"failedBaseHash":     failedBaseHash,
			"currentContentHash": currentHash,
			"contentComplete":    boolValue(snapshot["contentComplete"]),
			"safeForFileWrite":   boolValue(snapshot["safeForFileWrite"]),
			"reason":             "current_target_hash_differs_from_failed_patch_base_hash",
			"guidance":           "The failed patch used a stale baseHash. Rebuild the next patch from patchRepairContext/current target snapshot and copy currentContentHash as baseHash if baseHash is used.",
			"mechanicalSignal":   true,
			"businessJudgment":   false,
			"matchedCurrentFile": true,
		})
	}
	return ret
}

func (r *repairSubsystem) runtimeRepairPatchAlreadySatisfiedSignals(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	targetRefs := patchRepairTargetRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	patches := patchRepairRequestsFromStep(failedStep)
	if len(patches) == 0 {
		return nil
	}
	allowedTargets := map[string]bool{}
	for _, targetRef := range targetRefs {
		allowedTargets[strings.TrimSpace(targetRef)] = true
	}
	var ret []map[string]any
	for _, request := range patches {
		if len(ret) >= 3 {
			break
		}
		if len(allowedTargets) > 0 && !allowedTargets[request.TargetRef] {
			continue
		}
		switch request.PatchType {
		case "unified_diff":
			if strings.TrimSpace(request.Diff) == "" {
				continue
			}
			if signal := patchRepairAlreadySatisfiedSignalForDiff(workspace.DeliverableDirectory, request.TargetRef, request.Diff); len(signal) > 0 {
				ret = append(ret, signal)
			}
		case "text_replace":
			if signal := patchRepairAlreadySatisfiedSignalForTextReplace(workspace.DeliverableDirectory, request); len(signal) > 0 {
				ret = append(ret, signal)
			}
		}
	}
	return ret
}

func runtimeRepairPatchFailureNeedsCurrentTargetRefresh(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if strings.TrimSpace(step.Kind) != "patch/apply" &&
		strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) != failureClassPatchApply &&
		!strings.HasPrefix(strings.TrimSpace(stringFromMap(step.Output, "stage")), "patch_") {
		return false
	}
	textValues := repairForbiddenReuseTextValues(step.Input, step.Output)
	if diagnostics := mapFromValue(step.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		textValues = append(textValues,
			stringFromMap(diagnostics, "error"),
			stringFromMap(diagnostics, "errorDetail"),
			stringFromMap(diagnostics, "guidance"),
		)
	}
	text := strings.ToLower(strings.Join(textValues, "\n"))
	return strings.Contains(text, "basehash mismatch") ||
		(strings.Contains(text, "basehash") && strings.Contains(text, "mismatch"))
}

func (r *repairSubsystem) runtimeRepairFailureTargetContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	files, err := workspaceManifestFiles(workspace.DeliverableDirectory)
	if err != nil {
		files = nil
	}
	targetRefs := append([]string{}, failureTargetRepairRefs(failedStep)...)
	targetRefs = append(targetRefs, manifestGroundedFailureSignalTargetRefs(failedStep, files)...)
	targetRefs = compactUniqueStrings(targetRefs)
	if len(targetRefs) == 0 {
		return nil
	}
	targetRefs = resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, targetRefs)
	targetRefs = compactUniqueStrings(targetRefs)
	targetRefs = filterAdmissibleRuntimeRepairTargetRefs(targetRefs)
	ret := make([]map[string]any, 0, min(len(targetRefs), 3))
	for idx, targetRef := range targetRefs {
		if idx >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, targetRef)
		if len(item) == 0 || !runtimeRepairTargetRefAdmissible(targetRef) || boolValue(item["isBinary"]) {
			continue
		}
		item["source"] = "failure_target_current_workspace_file"
		item["guidance"] = "Use this current failure target snapshot for repair; failed stderr/stdout location is the authoritative target signal."
		ret = append(ret, item)
	}
	return ret
}

func (r *repairSubsystem) runtimeRepairResourceShapeAlignmentContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil || failedStep == nil || !runtimeRepairNeedsResourceShapeAlignment(failedStep) {
		return nil
	}
	targetRefs := failureTargetRepairRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	files, err := workspaceManifestFiles(workspace.DeliverableDirectory)
	if err != nil || len(files) == 0 {
		return nil
	}
	targetRefs = resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, targetRefs)
	failedSet := map[string]bool{}
	failedDirs := map[string]bool{}
	failedStems := map[string]bool{}
	for _, target := range targetRefs {
		target = normalizeRepairPatchTargetRef(target)
		if target == "" {
			continue
		}
		failedSet[target] = true
		failedDirs[filepath.Dir(target)] = true
		failedStems[normalizedPathStem(target)] = true
	}
	if len(failedDirs) == 0 {
		return nil
	}
	candidates := make([]workspaceDeliverableFile, 0, len(files))
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if path == "" || failedSet[path] || !sameDirectoryResourceCandidate(path, failedDirs) {
			continue
		}
		candidates = append(candidates, file)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftScore := resourceShapeAlignmentCandidateScore(candidates[i].Path, failedStems)
		rightScore := resourceShapeAlignmentCandidateScore(candidates[j].Path, failedStems)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if candidates[i].Size != candidates[j].Size {
			return candidates[i].Size < candidates[j].Size
		}
		return candidates[i].Path < candidates[j].Path
	})
	ret := make([]map[string]any, 0, min(len(candidates), 3))
	for _, candidate := range candidates {
		if len(ret) >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, candidate.Path)
		if len(item) == 0 || !boolValue(item["exists"]) {
			continue
		}
		item["source"] = "same_directory_resource_for_shape_conflict"
		item["guidance"] = "Use this current same-directory resource to reconcile resource or contract shape before inventing names, attributes, or contract entries."
		ret = append(ret, item)
	}
	return ret
}

func runtimeRepairNeedsResourceShapeAlignment(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	for _, signals := range inheritedResourceShapeSignalMaps(failedStep) {
		if structuredFailureSignalsContainShapeConflict(signals) {
			return true
		}
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	if signals := structuredFailureSignalsFromFailureOutput(failedStep.Output, nil, class, signature); structuredFailureSignalsContainShapeConflict(signals) {
		return true
	}
	for _, key := range []string{"stderr", "stdout", "error", "reason", "summary", "message"} {
		if textContainsShapeConflictDiagnostic(stringFromMap(failedStep.Output, key)) {
			return true
		}
	}
	if failedOutput := mapFromValue(failedStep.Output["failedStepOutput"]); len(failedOutput) > 0 {
		for _, key := range []string{"stderr", "stdout", "error", "reason", "summary", "message"} {
			if textContainsShapeConflictDiagnostic(stringFromMap(failedOutput, key)) {
				return true
			}
		}
	}
	return false
}

func inheritedResourceShapeSignalMaps(step *runModel.Step) []map[string]any {
	if step == nil {
		return nil
	}
	var ret []map[string]any
	add := func(signals map[string]any) {
		if len(signals) == 0 {
			return
		}
		ret = append(ret, signals)
	}
	add(mapFromValue(step.Input["structuredFailureSignals"]))
	add(mapFromValue(step.Input["primaryDiagnosticRepair"]))
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	add(mapFromValue(actionPackage["structuredFailureSignals"]))
	add(mapFromValue(actionPackage["primaryDiagnosticRepair"]))
	return ret
}

func sameDirectoryResourceCandidate(path string, failedDirs map[string]bool) bool {
	path = normalizeRepairPatchTargetRef(path)
	if path == "" || len(failedDirs) == 0 {
		return false
	}
	return failedDirs[filepath.Dir(path)]
}

func resourceShapeAlignmentCandidateScore(path string, failedStems map[string]bool) int {
	path = normalizeRepairPatchTargetRef(path)
	if path == "" {
		return 0
	}
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, ext)
	score := 50
	if ext != "" {
		score += 5
	}
	for failedStem := range failedStems {
		if failedStem == "" {
			continue
		}
		switch {
		case stem == failedStem:
			score += 30
		case strings.HasPrefix(failedStem, stem+"_") || strings.HasPrefix(failedStem, stem+"-") || strings.HasPrefix(stem, failedStem+"_") || strings.HasPrefix(stem, failedStem+"-"):
			score += 25
		case strings.Contains(failedStem, stem) || strings.Contains(stem, failedStem):
			score += 10
		}
	}
	if !strings.Contains(stem, "_") && !strings.Contains(stem, "-") && !strings.Contains(stem, ".") {
		score += 5
	}
	return score
}

func normalizedPathStem(path string) string {
	base := strings.ToLower(filepath.Base(normalizeRepairPatchTargetRef(path)))
	if base == "." || base == "" {
		return ""
	}
	return strings.TrimSuffix(base, strings.ToLower(filepath.Ext(base)))
}

func (r *repairSubsystem) runtimeRepairRootCauseCandidateContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	reassessment := repairStrategyReassessmentFromInput(map[string]any{
		"targetRepairAttempts":           r.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, run.ID, strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)), runtimeFailureTargetScopeFromStep(failedStep), runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep)),
		stepInputKeyRuntimeRepairAttempt: int64FromValue(failedStep.Input[stepInputKeyRuntimeRepairAttempt]),
		"failureTargetScope":             runtimeFailureTargetScopeFromStep(failedStep),
		failureOutputKeyClass:            strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)),
		failureOutputKeySignature:        strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)),
		"failedStepKind":                 failedStep.Kind,
	})
	if !strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		if inherited := mapFromValue(failedStep.Input["strategyReassessment"]); strategyReassessmentRequiresRootCauseTargetShift(inherited) {
			reassessment = inherited
		}
	}
	includeSurfaceCandidates := runtimeRepairShouldIncludeRootCauseCandidates(failedStep)
	includePrerequisiteCandidates := runtimeRepairShouldIncludePrerequisiteManifestCandidates(failedStep)
	if (len(reassessment) == 0 || !strategyReassessmentRequiresRootCauseTargetShift(reassessment)) && !includeSurfaceCandidates && !includePrerequisiteCandidates {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	files, err := workspaceManifestFiles(workspace.DeliverableDirectory)
	if err != nil || len(files) == 0 {
		return nil
	}
	failedTargets := resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, failureTargetRepairRefs(failedStep))
	if len(failedTargets) == 0 {
		failedTargets = resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, failureTargetRepairRefsFromStrategyReassessment(reassessment))
	}
	if len(failedTargets) == 0 {
		if includePrerequisiteCandidates {
			return runtimeRepairPrerequisiteManifestCandidateContext(workspace.DeliverableDirectory, files, runtimeRepairPrerequisiteManifestTargetPatterns(run, workspace, failedStep))
		}
		return nil
	}
	failedSet := map[string]bool{}
	failedDirs := map[string]bool{}
	failedStems := map[string]bool{}
	for _, target := range failedTargets {
		target = normalizeRepairPatchTargetRef(target)
		if target == "" {
			continue
		}
		failedSet[target] = true
		failedDirs[filepath.Dir(target)] = true
		if stem := normalizedPathStem(target); stem != "" {
			failedStems[stem] = true
		}
	}
	if len(failedSet) == 0 {
		return nil
	}
	var ret []map[string]any
	addCandidate := func(path string, source string) {
		path = normalizeRepairPatchTargetRef(path)
		if path == "" || failedSet[path] || !runtimeRepairTargetRefAdmissible(path) {
			return
		}
		for _, existing := range ret {
			if normalizeRepairPatchTargetRef(stringFromMap(existing, "targetRef")) == path {
				return
			}
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, path)
		if len(item) == 0 || !boolValue(item["exists"]) || boolValue(item["isBinary"]) {
			return
		}
		item["source"] = source
		item["guidance"] = "Use this as a grounded root-cause candidate resource when repairing the failed surface requires changing an adjacent or referenced current workspace resource."
		ret = append(ret, item)
	}
	if includePrerequisiteCandidates {
		for _, item := range runtimeRepairPrerequisiteManifestCandidateContext(workspace.DeliverableDirectory, files, runtimeRepairPrerequisiteManifestTargetPatterns(run, workspace, failedStep)) {
			if len(ret) >= 6 {
				break
			}
			addCandidate(stringFromMap(item, "targetRef"), "prerequisite_manifest_candidate")
		}
	}
	signals := structuredFailureSignalsFromFailureOutput(failedStep.Output, nil, strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)), strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)))
	for _, target := range stringSliceFromValue(signals["targetRefs"]) {
		addCandidate(resolveWorkspaceTargetRefFromManifest(files, target), "structured_failure_target_ref")
	}
	for _, item := range runtimeRepairContextItemsFromStep(failedStep) {
		addCandidate(stringFromMap(item, "targetRef"), "patch_repair_context")
	}
	sameDirectoryCandidates := make([]workspaceDeliverableFile, 0)
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if path == "" || failedSet[path] {
			continue
		}
		if failedDirs[filepath.Dir(path)] {
			sameDirectoryCandidates = append(sameDirectoryCandidates, file)
		}
	}
	sort.SliceStable(sameDirectoryCandidates, func(i, j int) bool {
		leftScore := resourceShapeAlignmentCandidateScore(sameDirectoryCandidates[i].Path, failedStems)
		rightScore := resourceShapeAlignmentCandidateScore(sameDirectoryCandidates[j].Path, failedStems)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if sameDirectoryCandidates[i].Size != sameDirectoryCandidates[j].Size {
			return sameDirectoryCandidates[i].Size < sameDirectoryCandidates[j].Size
		}
		return sameDirectoryCandidates[i].Path < sameDirectoryCandidates[j].Path
	})
	for _, file := range sameDirectoryCandidates {
		addCandidate(file.Path, "same_directory_resource")
		if len(ret) >= 6 {
			break
		}
	}
	if len(ret) > 6 {
		return ret[:6]
	}
	return ret
}

func runtimeRepairShouldIncludeRootCauseCandidates(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	if strings.TrimSpace(string(failedStep.Kind)) != "shell/command" {
		return false
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	if class != "" && class != failureClassShellCommand {
		return false
	}
	return len(failureTargetRepairRefs(failedStep)) > 0
}

func runtimeRepairShouldIncludePrerequisiteManifestCandidates(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	if strings.TrimSpace(string(failedStep.Kind)) != "shell/command" {
		return false
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	if class != "" && class != failureClassShellCommand {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(failedStep.Output, "stderr"),
		stringFromMap(failedStep.Output, "stdout"),
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
		stringFromMap(failedStep.Output, failureOutputKeySignature),
	}, "\n"))
	if strings.TrimSpace(text) == "" {
		return false
	}
	prerequisiteSignals := []string{
		"prerequisite", "dependency", "dependencies",
		"manifest", "metadata", "checksum", "lock", "requirements", "resolve",
		"download", "version", "registry", "repository", "network", "timeout",
		"unavailable",
	}
	for _, signal := range prerequisiteSignals {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

func runtimeRepairPrerequisiteManifestCandidateContext(deliverableDirectory string, files []workspaceDeliverableFile, targetPatterns []string) []map[string]any {
	if strings.TrimSpace(deliverableDirectory) == "" || len(files) == 0 {
		return nil
	}
	candidates := make([]workspaceDeliverableFile, 0)
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if path == "" || !runtimeRepairTargetRefAdmissible(path) || runtimeRepairPrerequisiteManifestCandidateScore(path, targetPatterns) <= 0 {
			continue
		}
		candidates = append(candidates, file)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftScore := runtimeRepairPrerequisiteManifestCandidateScore(candidates[i].Path, targetPatterns)
		rightScore := runtimeRepairPrerequisiteManifestCandidateScore(candidates[j].Path, targetPatterns)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		leftDepth := strings.Count(normalizeRepairPatchTargetRef(candidates[i].Path), "/")
		rightDepth := strings.Count(normalizeRepairPatchTargetRef(candidates[j].Path), "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		if candidates[i].Size != candidates[j].Size {
			return candidates[i].Size < candidates[j].Size
		}
		return candidates[i].Path < candidates[j].Path
	})
	ret := make([]map[string]any, 0, min(len(candidates), 4))
	for _, candidate := range candidates {
		if len(ret) >= 4 {
			break
		}
		item := patchRepairTargetSnapshot(deliverableDirectory, candidate.Path)
		if len(item) == 0 || !boolValue(item["exists"]) || boolValue(item["isBinary"]) {
			continue
		}
		item["source"] = "prerequisite_manifest_candidate"
		item["guidance"] = "Use this current dependency/configuration manifest candidate only when the failed prerequisite can be repaired by a minimal workspace metadata change; external endpoints remain diagnostics, not repair targets."
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeRepairPrerequisiteManifestCandidateScore(path string, targetPatterns []string) int {
	path = strings.ToLower(normalizeRepairPatchTargetRef(path))
	if path == "" {
		return 0
	}
	if len(targetPatterns) > 0 && !runtimeRepairPathMatchesAnyPattern(path, targetPatterns) {
		return 0
	}
	base := filepath.Base(path)
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	stem := strings.TrimSuffix(base, "."+ext)
	score := 0
	if runtimeRepairPathMatchesAnyPattern(path, targetPatterns) {
		score += 200
	}
	for _, token := range []string{"manifest", "metadata", "dependency", "dependencies", "requirements", "checksum", "lock", "sum"} {
		if stem == token || strings.Contains(stem, token) || ext == token {
			score += 80
		}
	}
	for _, token := range []string{"project", "workspace", "config", "settings"} {
		if stem == token || strings.Contains(stem, token) || ext == token {
			score += 45
		}
	}
	if strings.Count(path, "/") == 0 {
		score += 10
	}
	return score
}

func runtimeRepairPrerequisiteManifestTargetPatterns(run *runModel.Run, workspace *workspaceModel.Workspace, failedStep *runModel.Step) []string {
	var ret []string
	addFrom := func(value any) {
		ret = append(ret, runtimeRepairPrerequisiteManifestTargetPatternsFromValue(value)...)
	}
	if workspace != nil {
		addFrom(workspace.ContextHints)
	}
	if run != nil {
		addFrom(run.ExecutionContext)
		addFrom(mapFromValue(run.ExecutionContext["executionContract"]))
	}
	if failedStep != nil {
		addFrom(failedStep.Input)
		addFrom(mapFromValue(failedStep.Input["executionContract"]))
		pkg := mapFromValue(failedStep.Input[stepInputKeyExecutionContextPackage])
		addFrom(pkg)
		addFrom(mapFromValue(pkg["executionContract"]))
	}
	return compactUniqueStrings(ret)
}

func runtimeRepairPrerequisiteManifestTargetPatternsFromValue(value any) []string {
	return runtimeRepairPrerequisiteManifestTargetPatternsFromMap(mapFromValue(value), 0)
}

func runtimeRepairPrerequisiteManifestTargetPatternsFromMap(value map[string]any, depth int) []string {
	if len(value) == 0 || depth > 5 {
		return nil
	}
	var ret []string
	for _, key := range []string{"prerequisiteManifestTargetPatterns", "prerequisiteManifestPatterns"} {
		for _, pattern := range stringSliceFromValue(value[key]) {
			if normalized := normalizeRepairPatchTargetRef(pattern); normalized != "" && runtimeRepairTargetRefAdmissible(normalized) {
				ret = append(ret, normalized)
			}
		}
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ret = append(ret, runtimeRepairPrerequisiteManifestTargetPatternsFromMap(mapFromValue(value[key]), depth+1)...)
	}
	return compactUniqueStrings(ret)
}

func runtimeRepairPathMatchesAnyPattern(path string, patterns []string) bool {
	path = strings.ToLower(normalizeRepairPatchTargetRef(path))
	if path == "" || len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.ToLower(normalizeRepairPatchTargetRef(pattern))
		if pattern == "" {
			continue
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		if !strings.Contains(pattern, "/") {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				return true
			}
		}
	}
	return false
}

func failureTargetRepairRefsFromStrategyReassessment(reassessment map[string]any) []string {
	if len(reassessment) == 0 {
		return nil
	}
	var ret []string
	for _, key := range []string{"failureTargetScope", "failedSurface"} {
		ret = append(ret, repairPatchTargetRefsFromScopeText(stringFromMap(reassessment, key))...)
	}
	return compactUniqueStrings(ret)
}

func runtimeRepairContextItemsFromStep(step *runModel.Step) []map[string]any {
	if step == nil || step.Input == nil {
		return nil
	}
	ret := mapSliceFromValue(step.Input["patchRepairContext"])
	ret = append(ret, mapSliceFromValue(step.Input["failureTargetRepairContext"])...)
	return ret
}

func resolveWorkspaceTargetRefFromManifest(files []workspaceDeliverableFile, targetRef string) string {
	targetRef = normalizedFailureTarget(targetRef)
	if targetRef == "" {
		return ""
	}
	for _, file := range files {
		if normalizeRepairPatchTargetRef(file.Path) == normalizeRepairPatchTargetRef(targetRef) {
			return file.Path
		}
	}
	var match string
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if strings.HasSuffix(path, "/"+targetRef) || filepath.Base(path) == filepath.Base(targetRef) {
			if match != "" {
				return ""
			}
			match = file.Path
		}
	}
	return match
}

func manifestGroundedFailureSignalTargetRefs(step *runModel.Step, files []workspaceDeliverableFile) []string {
	if step == nil || len(files) == 0 {
		return nil
	}
	class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
	texts := repairFailureSignalTexts(step.Output, nil, class, signature)
	if len(texts) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		resolved := resolveWorkspaceTargetRefFromManifest(files, value)
		resolved = normalizeRepairPatchTargetRef(resolved)
		if resolved == "" || seen[resolved] {
			return
		}
		seen[resolved] = true
		ret = append(ret, resolved)
	}
	for _, text := range texts {
		for _, target := range extractStructuredFailureTargetRefs(text) {
			add(target)
		}
	}
	return ret
}

func resolveWorkspaceTargetRefs(deliverableDirectory string, targetRefs []string) []string {
	if strings.TrimSpace(deliverableDirectory) == "" || len(targetRefs) == 0 {
		return targetRefs
	}
	files, err := workspaceManifestFiles(deliverableDirectory)
	if err != nil || len(files) == 0 {
		return targetRefs
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(targetRefs))
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, targetRef := range targetRefs {
		resolved := resolveWorkspaceTargetRef(files, targetRef)
		if resolved == "" {
			resolved = targetRef
		}
		add(resolved)
	}
	return ret
}

func resolveWorkspaceTargetRef(files []workspaceDeliverableFile, targetRef string) string {
	targetRef = normalizedFailureTarget(targetRef)
	if targetRef == "" {
		return ""
	}
	for _, file := range files {
		if normalizeRepairPatchTargetRef(file.Path) == normalizeRepairPatchTargetRef(targetRef) {
			return file.Path
		}
	}
	var match string
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if path == "" {
			continue
		}
		if strings.HasSuffix(path, "/"+targetRef) || strings.HasSuffix(targetRef, "/"+path) {
			if match != "" {
				return targetRef
			}
			match = file.Path
		}
	}
	if match != "" {
		return match
	}
	return targetRef
}

func workspaceManifestFilesFromMaps(manifest []map[string]any) []workspaceDeliverableFile {
	if len(manifest) == 0 {
		return nil
	}
	ret := make([]workspaceDeliverableFile, 0, len(manifest))
	for _, item := range manifest {
		path := normalizeRepairPatchTargetRef(stringFromMap(item, "path"))
		if path == "" {
			continue
		}
		ret = append(ret, workspaceDeliverableFile{
			Path:       path,
			Size:       int64FromValue(item["size"]),
			Executable: boolValue(item["executable"]),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func canonicalRepairTargetRefFromManifest(manifest []map[string]any, targetRef string) (string, bool) {
	files := workspaceManifestFilesFromMaps(manifest)
	if len(files) == 0 {
		normalized := normalizeRepairPatchTargetRef(targetRef)
		return normalized, normalized != ""
	}
	normalized := normalizeRepairPatchTargetRef(targetRef)
	if normalized == "" {
		return "", false
	}
	resolved := normalizeRepairPatchTargetRef(resolveWorkspaceTargetRef(files, normalized))
	if resolved == "" {
		return "", false
	}
	if resolved != normalized {
		return resolved, true
	}
	for _, file := range files {
		if normalizeRepairPatchTargetRef(file.Path) == normalized {
			return normalized, true
		}
	}
	return normalized, false
}

func failureTargetRepairRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	suppressOutputSignatureTargets := externalCapabilityFailureSuppressesTextTargets(step.Output)
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" || !runtimeRepairTargetRefAdmissible(value) || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, target := range failureTargetsForSignature(step.Output) {
		add(target)
	}
	if !suppressOutputSignatureTargets {
		for _, target := range failureTargetsFromSignatureText(stringFromMap(step.Output, failureOutputKeySignature)) {
			add(target)
		}
	}
	if !currentFailureIsCoverageAssessmentOnly(step) {
		if !suppressOutputSignatureTargets {
			for _, target := range failureTargetsFromSignatureText(stringFromMap(step.Input, stepInputKeyRepairOfFailureSig)) {
				add(target)
			}
			for _, target := range failureTargetsFromSignatureText(stringFromMap(step.Input, failureOutputKeySignature)) {
				add(target)
			}
			for _, target := range failureTargetsFromSignatureText(stringFromMap(step.Input, "failureTargetScope")) {
				add(target)
			}
		}
	}
	if reassessment := mapFromValue(step.Input["strategyReassessment"]); len(reassessment) > 0 {
		for _, target := range failureTargetRepairRefsFromStrategyReassessment(reassessment) {
			add(target)
		}
	}
	failedOutput := mapFromValue(step.Output["failedStepOutput"])
	suppressFailedOutputSignatureTargets := externalCapabilityFailureSuppressesTextTargets(failedOutput)
	for _, target := range failureTargetsForSignature(failedOutput) {
		add(target)
	}
	if !suppressFailedOutputSignatureTargets {
		for _, target := range failureTargetsFromSignatureText(stringFromMap(failedOutput, failureOutputKeySignature)) {
			add(target)
		}
	}
	class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
	if signals := structuredFailureSignalsFromFailureOutput(step.Output, nil, class, signature); len(signals) > 0 {
		for _, target := range stringSliceFromValue(signals["targetRefs"]) {
			add(target)
		}
	}
	for _, signals := range inheritedResourceShapeSignalMaps(step) {
		for _, target := range stringSliceFromValue(signals["targetRefs"]) {
			add(target)
		}
	}
	for _, value := range []any{step.Input["currentExecutionProgress"], step.Output["currentExecutionProgress"]} {
		if signals := structuredFailureSignalsFromProgressEvidence(mapFromValue(value)); len(signals) > 0 {
			for _, target := range stringSliceFromValue(signals["targetRefs"]) {
				add(target)
			}
		}
	}
	return ret
}

func externalCapabilityFailureSuppressesTextTargets(output map[string]any) bool {
	return externalCapabilityFailureHasCallIdentity(output) && len(explicitFailureTargetsForSignature(output)) == 0
}

func failureTargetsFromSignatureText(signature string) []string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return nil
	}
	segments := strings.Split(signature, "|")
	var ret []string
	for _, segment := range segments {
		key, value, ok := strings.Cut(segment, "=")
		if !ok || strings.TrimSpace(key) != "failureTargets" {
			continue
		}
		for _, target := range strings.Split(value, ",") {
			if normalized := normalizedFailureTarget(target); normalized != "" {
				ret = append(ret, normalized)
			}
		}
	}
	return ret
}

func shouldIncludePatchRepairContext(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "patch/preview", "patch/apply", "file/edit":
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(stringFromMap(step.Output, "stage")), "patch_") {
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(stringFromMap(step.Output, "stage")), "file_edit") {
		return true
	}
	if strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) == failureClassPatchApply {
		return true
	}
	failedOutput := mapFromValue(step.Output["failedStepOutput"])
	if strings.HasPrefix(strings.TrimSpace(stringFromMap(failedOutput, "stage")), "patch_") {
		return true
	}
	failedInput := mapFromValue(step.Output["failedStepInput"])
	switch strings.TrimSpace(stringFromMap(failedInput, "kind")) {
	case "patch/preview", "patch/apply", "file/edit":
		return true
	}
	return false
}

func patchRepairTargetRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	seen := map[string]bool{}
	add := func(value string) []string {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return nil
		}
		seen[value] = true
		return []string{value}
	}
	var ret []string
	ret = append(ret, add(stringFromMap(step.Input, "targetRef"))...)
	ret = append(ret, add(stringFromMap(step.Input, "path"))...)
	ret = append(ret, add(stringFromMap(step.Output, "targetRef"))...)
	ret = append(ret, add(stringFromMap(step.Output, "path"))...)
	if patches := mapSliceFromValue(step.Output["patches"]); len(patches) > 0 {
		for _, patch := range patches {
			ret = append(ret, add(stringFromMap(patch, "targetRef"))...)
		}
	}
	if failedInput := mapFromValue(step.Output["failedStepInput"]); len(failedInput) > 0 {
		ret = append(ret, add(stringFromMap(failedInput, "targetRef"))...)
		ret = append(ret, add(stringFromMap(failedInput, "path"))...)
	}
	if failedOutput := mapFromValue(step.Output["failedStepOutput"]); len(failedOutput) > 0 {
		ret = append(ret, add(stringFromMap(failedOutput, "targetRef"))...)
		ret = append(ret, add(stringFromMap(failedOutput, "path"))...)
		if patches := mapSliceFromValue(failedOutput["patches"]); len(patches) > 0 {
			for _, patch := range patches {
				ret = append(ret, add(stringFromMap(patch, "targetRef"))...)
			}
		}
	}
	return ret
}

func patchRepairTargetSnapshot(deliverableDirectory string, targetRef string) map[string]any {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return map[string]any{
			"targetRef": targetRef,
			"error":     err.Error(),
		}
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return map[string]any{
			"targetRef": targetRef,
			"exists":    false,
			"error":     err.Error(),
		}
	}
	item := map[string]any{
		"targetRef":   targetRef,
		"exists":      true,
		"size":        len(data),
		"contentHash": sha256HexBytes(data),
		"source":      "current_workspace_file",
		"guidance":    "Use this current target snapshot for repair; failed patch text may be stale or syntactically invalid.",
	}
	if patchRepairLooksBinary(data) {
		item["isBinary"] = true
		item["contentComplete"] = false
		item["safeForFileWrite"] = false
		return item
	}
	content := string(data)
	contentComplete := true
	if len(content) > maxPatchRepairSnapshotBytes {
		content = content[:maxPatchRepairSnapshotBytes]
		item["truncated"] = true
		contentComplete = false
	}
	item["content"] = content
	item["contentComplete"] = contentComplete
	item["redacted"] = containsRuntimeRedactionMarker(content)
	item["safeForFileWrite"] = contentComplete && !containsRuntimeRedactionMarker(content)
	return item
}

func patchRepairRequestsFromStep(step *runModel.Step) []runtimePatchRequest {
	if step == nil {
		return nil
	}
	var ret []runtimePatchRequest
	seen := map[string]bool{}
	add := func(value map[string]any) {
		if len(value) == 0 {
			return
		}
		if strings.TrimSpace(stringFromMap(value, "patchType")) == "" &&
			strings.TrimSpace(stringFromMap(value, "operation")) == "" &&
			strings.TrimSpace(stringFromMap(value, "diff")) != "" {
			value = cloneStepPayload(value)
			value["patchType"] = "unified_diff"
		}
		request, err := runtimePatchRequestFromMap(value)
		if err != nil {
			return
		}
		key := patchRepairRequestKey(request)
		if seen[key] {
			return
		}
		seen[key] = true
		ret = append(ret, request)
	}
	add(step.Input)
	add(step.Output)
	for _, patch := range mapSliceFromValue(step.Output["patches"]) {
		add(patch)
	}
	if failedInput := mapFromValue(step.Output["failedStepInput"]); len(failedInput) > 0 {
		add(failedInput)
	}
	if failedOutput := mapFromValue(step.Output["failedStepOutput"]); len(failedOutput) > 0 {
		add(failedOutput)
		for _, patch := range mapSliceFromValue(failedOutput["patches"]) {
			add(patch)
		}
	}
	return ret
}

func patchRepairRequestKey(request runtimePatchRequest) string {
	return strings.Join([]string{
		normalizeRepairPatchTargetRef(request.TargetRef),
		strings.TrimSpace(request.PatchType),
		request.Diff,
		request.OldText,
		request.NewText,
		request.Content,
	}, "\x00")
}

func patchRepairSuggestionsForDiff(deliverableDirectory string, targetRef string, diff string) []map[string]any {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil || patchRepairLooksBinary(data) {
		return nil
	}
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil || !diffTargetMatches(targetRef, filePatch) {
		return nil
	}
	content := string(data)
	contentHash := sha256HexBytes(data)
	var ret []map[string]any
	seen := map[string]bool{}
	for _, hunk := range filePatch.Hunks {
		for _, replacement := range patchRepairReplacementCandidates(hunk, content) {
			oldText := replacement["oldText"]
			newText := replacement["newText"]
			if oldText == "" || oldText == newText {
				continue
			}
			if containsRuntimeRedactionMarker(oldText) || containsRuntimeRedactionMarker(newText) {
				continue
			}
			if strings.Count(content, oldText) != 1 {
				continue
			}
			key := oldText + "\x00" + newText
			if seen[key] {
				continue
			}
			seen[key] = true
			ret = append(ret, map[string]any{
				"targetRef":      targetRef,
				"patchType":      "unified_diff",
				"oldText":        oldText,
				"newText":        newText,
				"contentHash":    contentHash,
				"matchCount":     1,
				"source":         replacement["source"],
				"guidance":       "Use this exact current oldText/newText replacement to construct the smallest unified_diff or file/edit repair; do not copy redacted or truncated snapshot content.",
				"safeForMinimal": true,
			})
		}
	}
	return ret
}

func patchRepairSuggestionForTextReplace(deliverableDirectory string, request runtimePatchRequest) map[string]any {
	if strings.TrimSpace(request.TargetRef) == "" || request.PatchType != "text_replace" || request.OldText == "" {
		return nil
	}
	if containsRuntimeRedactionMarker(request.OldText) || containsRuntimeRedactionMarker(request.NewText) {
		return nil
	}
	target, err := workspaceTargetPath(deliverableDirectory, request.TargetRef)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil || patchRepairLooksBinary(data) {
		return nil
	}
	content := string(data)
	matchCount := strings.Count(content, request.OldText)
	if matchCount != 1 {
		return nil
	}
	return map[string]any{
		"targetRef":      request.TargetRef,
		"patchType":      "text_replace",
		"oldText":        request.OldText,
		"newText":        request.NewText,
		"contentHash":    sha256HexBytes(data),
		"matchCount":     matchCount,
		"source":         "current_text_replace_match",
		"guidance":       "Use this exact current oldText/newText replacement with the current contentHash; do not retry stale baseHash or altered oldText.",
		"safeForMinimal": true,
	}
}

func patchRepairAlreadySatisfiedSignalForDiff(deliverableDirectory string, targetRef string, diff string) map[string]any {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil || patchRepairLooksBinary(data) {
		return nil
	}
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil || !diffTargetMatches(targetRef, filePatch) {
		return nil
	}
	content := string(data)
	addedLines, deletedLines := patchRepairChangedLines(filePatch)
	if len(addedLines) == 0 && len(deletedLines) == 0 {
		return nil
	}
	if patchRepairLinesAlreadySatisfied(content, addedLines, deletedLines) {
		return map[string]any{
			"targetRef":          targetRef,
			"patchType":          "unified_diff",
			"contentHash":        sha256HexBytes(data),
			"addedLineCount":     len(addedLines),
			"deletedLineCount":   len(deletedLines),
			"safeForValidation":  true,
			"reason":             "current_target_already_matches_failed_patch_intent",
			"guidance":           "The current target snapshot already reflects the failed patch's intended line-level state. Do not repeat the stale patch; run bounded concrete validation or return a real governance blocker.",
			"mechanicalSignal":   true,
			"businessJudgment":   false,
			"matchedCurrentFile": true,
		}
	}
	return nil
}

func patchRepairAlreadySatisfiedSignalForTextReplace(deliverableDirectory string, request runtimePatchRequest) map[string]any {
	if strings.TrimSpace(request.TargetRef) == "" || request.PatchType != "text_replace" || request.OldText == "" {
		return nil
	}
	if containsRuntimeRedactionMarker(request.OldText) || containsRuntimeRedactionMarker(request.NewText) {
		return nil
	}
	target, err := workspaceTargetPath(deliverableDirectory, request.TargetRef)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil || patchRepairLooksBinary(data) {
		return nil
	}
	content := string(data)
	if strings.Contains(content, request.OldText) {
		return nil
	}
	newText := request.NewText
	newTextSatisfied := strings.TrimSpace(newText) == "" || strings.Contains(content, newText)
	if !newTextSatisfied {
		return nil
	}
	return map[string]any{
		"targetRef":          request.TargetRef,
		"patchType":          "text_replace",
		"contentHash":        sha256HexBytes(data),
		"oldTextHash":        sha256HexBytes([]byte(request.OldText)),
		"newTextHash":        sha256HexBytes([]byte(request.NewText)),
		"safeForValidation":  true,
		"reason":             "current_target_already_matches_failed_patch_intent",
		"guidance":           "The current target snapshot already reflects the failed text replacement intent. Do not repeat the stale patch; run bounded concrete validation or return a real governance blocker.",
		"mechanicalSignal":   true,
		"businessJudgment":   false,
		"matchedCurrentFile": true,
	}
}

func patchRepairChangedLines(filePatch unifiedDiffFilePatch) (added []string, deleted []string) {
	seenAdded := map[string]bool{}
	seenDeleted := map[string]bool{}
	add := func(dst *[]string, seen map[string]bool, value string) {
		if strings.TrimSpace(value) == "" || containsRuntimeRedactionMarker(value) {
			return
		}
		if seen[value] {
			return
		}
		seen[value] = true
		*dst = append(*dst, value)
	}
	for _, hunk := range filePatch.Hunks {
		for _, line := range hunk.Lines {
			if len(line) == 0 {
				continue
			}
			switch line[0] {
			case '+':
				add(&added, seenAdded, line[1:])
			case '-':
				add(&deleted, seenDeleted, line[1:])
			}
		}
	}
	return added, deleted
}

func patchRepairLinesAlreadySatisfied(content string, added []string, deleted []string) bool {
	for _, line := range added {
		if !patchRepairContentContainsLine(content, line) {
			return false
		}
	}
	for _, line := range deleted {
		if patchRepairContentContainsLine(content, line) {
			return false
		}
	}
	return len(added) > 0 || len(deleted) > 0
}

func patchRepairContentContainsLine(content string, expected string) bool {
	expected = strings.TrimRight(expected, "\r")
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimRight(line, "\r") == expected {
			return true
		}
	}
	return false
}

func patchRepairReplacementText(hunk unifiedDiffHunk) (string, string) {
	var oldLines []string
	var newLines []string
	for _, line := range hunk.Lines {
		if line == "" {
			continue
		}
		switch line[0] {
		case '-':
			oldLines = append(oldLines, line[1:])
		case '+':
			newLines = append(newLines, line[1:])
		}
	}
	return strings.Join(oldLines, "\n"), strings.Join(newLines, "\n")
}

func patchRepairReplacementCandidates(hunk unifiedDiffHunk, content string) []map[string]string {
	var ret []map[string]string
	hasAddition := false
	hasDeletion := false
	for _, line := range hunk.Lines {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			hasAddition = true
		case '-':
			hasDeletion = true
		}
	}
	add := func(oldText string, newText string, source string) {
		oldText = strings.TrimSuffix(oldText, "\n")
		newText = strings.TrimSuffix(newText, "\n")
		if oldText == "" || oldText == newText {
			return
		}
		ret = append(ret, map[string]string{
			"oldText": oldText,
			"newText": newText,
			"source":  source,
		})
	}
	oldText, newText := patchRepairReplacementText(hunk)
	add(oldText, newText, "failed_patch_current_snapshot_unique_replacement")

	if strings.Count(content, oldText) != 1 {
		sourceLines, sourceHadFinalNewline := splitTextLines(content)
		start, err := unifiedDiffHunkStart(sourceLines, 0, hunk)
		if err == nil {
			oldLines := unifiedDiffHunkOldLines(hunk)
			if start >= 0 && start+len(oldLines) <= len(sourceLines) {
				nextLines := append([]string(nil), sourceLines[:start]...)
				for _, line := range hunk.Lines {
					if line == "" {
						continue
					}
					switch line[0] {
					case ' ', '+':
						nextLines = append(nextLines, line[1:])
					}
				}
				nextLines = append(nextLines, sourceLines[start+len(oldLines):]...)
				nextContent := joinTextLines(nextLines, sourceHadFinalNewline)
				add(content, nextContent, "failed_patch_current_snapshot_full_hunk_replay")
			}
		}
	}

	if hasAddition || !hasDeletion {
		return ret
	}
	for _, line := range hunk.Lines {
		if len(line) < 2 || line[0] != '-' {
			continue
		}
		oldLine := line[1:]
		if strings.Count(content, oldLine) == 1 {
			add(oldLine, "", "failed_patch_current_snapshot_unique_line_removal")
		}
	}
	return ret
}

func patchRepairLooksBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func convertedDecisionReadableResourceContext(ctx context.Context, workspaces repository.WorkspaceRepository, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	decision := mapFromValue(failedStep.Output["convertedDecision"])
	context := mapFromValue(decision["context"])
	workspace, err := workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	context = enrichDecisionContextWithReadableWorkspaceResources(ctx, workspaces, run, workspace, context, stringFromMap(decision, "question"), stringFromMap(decision, "reason"))
	if !knownReadableWorkspaceResourceDecisionContext(context) {
		return nil
	}
	resources := referencedReadableWorkspaceResources(context)
	if len(resources) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(resources), 5))
	for idx, targetRef := range resources {
		if idx >= 5 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, targetRef)
		if len(item) == 0 {
			continue
		}
		item["source"] = "converted_decision_known_workspace_resource"
		item["guidance"] = "This resource was already known to the runtime; use it for automated repair instead of requesting manual input."
		ret = append(ret, item)
	}
	return ret
}

func enrichDecisionContextWithReadableWorkspaceResources(
	ctx context.Context,
	workspaces repository.WorkspaceRepository,
	run *runModel.Run,
	workspace *workspaceModel.Workspace,
	context map[string]any,
	decisionQuestion string,
	decisionReason string,
) map[string]any {
	if run == nil {
		return context
	}
	if workspace == nil && workspaces != nil {
		if loaded, err := workspaces.Get(ctx, run.WorkspaceID); err == nil {
			workspace = loaded
		}
	}
	deliverableDirectory := ""
	if workspace != nil {
		deliverableDirectory = strings.TrimSpace(workspace.DeliverableDirectory)
	}
	if deliverableDirectory == "" {
		return context
	}
	fields := readableResourceReferenceTexts(context)
	if decisionQuestion = strings.TrimSpace(decisionQuestion); decisionQuestion != "" {
		fields = append(fields, decisionQuestion)
	}
	if decisionReason = strings.TrimSpace(decisionReason); decisionReason != "" {
		fields = append(fields, decisionReason)
	}
	if len(fields) == 0 {
		return context
	}
	manifest := workspaceFileManifestFromDirectory(deliverableDirectory)
	if len(manifest) == 0 {
		return context
	}
	var resources []string
	seen := map[string]bool{}
	for _, item := range manifest {
		path := stringFromMap(item, "path")
		if path == "" {
			continue
		}
		for _, field := range fields {
			if !decisionFieldReferencesKnownResource(field, []string{path}) {
				continue
			}
			if !seen[path] {
				seen[path] = true
				resources = append(resources, path)
			}
			break
		}
	}
	if len(resources) == 0 {
		return context
	}
	enriched := cloneStepPayload(context)
	if enriched == nil {
		enriched = map[string]any{}
	}
	if decisionQuestion != "" {
		enriched[decisionContextKeyQuestion] = decisionQuestion
	}
	if decisionReason != "" {
		enriched[decisionContextKeyReason] = decisionReason
	}
	existing := stringListFromValue(enriched["readableResources"])
	for _, resource := range resources {
		if !stringSliceContains(existing, resource) {
			existing = append(existing, resource)
		}
	}
	enriched["readableResources"] = existing
	return enriched
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func referencedReadableWorkspaceResources(context map[string]any) []string {
	resources := knownReadableWorkspaceResourceNames(context)
	if len(resources) == 0 {
		return nil
	}
	fields := readableResourceReferenceTexts(context)
	seen := map[string]bool{}
	var ret []string
	for _, resource := range resources {
		for _, field := range fields {
			if !decisionFieldReferencesKnownResource(field, []string{resource}) {
				continue
			}
			if !seen[resource] {
				seen[resource] = true
				ret = append(ret, resource)
			}
			break
		}
	}
	return ret
}

func appendRepairGuidance(existing any, items ...string) []string {
	ret := stringListFromValue(existing)
	seen := map[string]bool{}
	compact := make([]string, 0, len(ret)+len(items))
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		compact = append(compact, value)
	}
	for _, item := range ret {
		add(item)
	}
	for _, item := range items {
		add(item)
	}
	return compact
}

func shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep *runModel.Step) bool {
	if failedStep == nil || failedStep.Output == nil {
		return false
	}
	if len(failureTargetRepairRefs(failedStep)) > 0 {
		return true
	}
	if stringFromMap(failedStep.Output, failureOutputKeyClass) == failureClassWorkspacePath {
		return true
	}
	diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"])
	if stringFromMap(diagnostics, "kind") == "workspace_path_conflict" {
		return true
	}
	failedOutput := mapFromValue(failedStep.Output["failedStepOutput"])
	if stringFromMap(failedOutput, failureOutputKeyClass) == failureClassWorkspacePath {
		return true
	}
	if diagnostics := mapFromValue(failedOutput["runtimeDiagnostics"]); stringFromMap(diagnostics, "kind") == "workspace_path_conflict" {
		return true
	}
	if boolValue(failedStep.Input["stateChangingOutputRequired"]) &&
		(failedStepInputHasMissingDoDItems(failedStep) ||
			len(missingDoDItemsFromFailureOutput(failedStep.Output)) > 0 ||
			runtimeRepairInputHasResourceEvidence(failedStep)) {
		return true
	}
	return false
}

func runtimeRepairInputHasResourceEvidence(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	for _, evidence := range mapSliceFromValue(failedStep.Input[stepInputKeyGraphEvidence]) {
		if strings.TrimSpace(stringFromMap(evidence, "kind")) == "file/list" {
			return true
		}
		output := mapFromValue(evidence["output"])
		if strings.TrimSpace(stringFromMap(output, "stage")) == "file_list" || len(mapSliceFromValue(output["entries"])) > 0 {
			return true
		}
	}
	return false
}

func runtimeRepairInputHasEmptyWorkspaceSurfaceEvidence(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	if repairInputHasEmptyWorkspaceSurfaceEvidence(failedStep.Input) {
		return true
	}
	return repairInputHasEmptyWorkspaceSurfaceEvidence(failedStep.Output)
}

func repairInputHasEmptyWorkspaceSurfaceEvidence(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if graphEvidenceHasEmptyWorkspaceSurface(input) {
		return true
	}
	for _, evidence := range mapSliceFromValue(input[stepInputKeyGraphEvidence]) {
		if graphEvidenceHasEmptyWorkspaceSurface(evidence) {
			return true
		}
		if graphEvidenceHasEmptyWorkspaceSurface(evidence["output"]) {
			return true
		}
	}
	if nested := mapFromValue(input["failedStepInput"]); len(nested) > 0 && repairInputHasEmptyWorkspaceSurfaceEvidence(nested) {
		return true
	}
	if nested := mapFromValue(input["failedStepOutput"]); len(nested) > 0 && repairInputHasEmptyWorkspaceSurfaceEvidence(nested) {
		return true
	}
	if processTimelineHasEmptyWorkspaceSurface(input["processEvidenceTimeline"]) {
		return true
	}
	if coverageEvidenceCandidatesHaveEmptyWorkspaceSurface(input) {
		return true
	}
	return false
}

func coverageEvidenceCandidatesHaveEmptyWorkspaceSurface(input map[string]any) bool {
	for _, key := range []string{"missingDoDItems", "missingGoalCoverage", "coverageTargetSet"} {
		for _, item := range mapSliceFromValue(input[key]) {
			for _, candidate := range mapSliceFromValue(item["evidenceCandidates"]) {
				if graphEvidenceHasEmptyWorkspaceSurface(candidate) || graphEvidenceHasEmptyWorkspaceSurface(candidate["output"]) {
					return true
				}
			}
		}
	}
	return false
}

func graphEvidenceHasEmptyWorkspaceSurface(value any) bool {
	item := mapFromValue(value)
	if len(item) == 0 {
		return false
	}
	kind := strings.TrimSpace(stringFromMap(item, "kind"))
	output := mapFromValue(item["output"])
	stage := strings.TrimSpace(stringFromMap(item, "stage"))
	if stage == "" {
		stage = strings.TrimSpace(stringFromMap(output, "stage"))
	}
	if kind != "file/list" && stage != "file_list" {
		return false
	}
	path := strings.TrimSpace(stringFromMap(output, "path"))
	if path == "" {
		path = strings.TrimSpace(stringFromMap(item, "path"))
	}
	if path != "" && path != "." {
		return false
	}
	if boolValue(item["truncated"]) || boolValue(output["truncated"]) {
		return false
	}
	for _, entriesValue := range []any{
		item["entries"],
		item["items"],
		output["entries"],
		output["items"],
	} {
		if count, ok := graphEvidenceEntryCount(entriesValue); ok && count == 0 {
			return true
		}
	}
	return false
}

func processTimelineHasEmptyWorkspaceSurface(value any) bool {
	for _, item := range mapSliceFromValue(value) {
		if graphEvidenceHasEmptyWorkspaceSurface(item) {
			return true
		}
	}
	return false
}

func graphEvidenceEntryCount(value any) (int, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return len(typed), true
	case []any:
		return len(typed), true
	case map[string]any:
		for _, key := range []string{"items", "entries", "count", "total"} {
			if raw, ok := typed[key]; ok {
				switch nested := raw.(type) {
				case []map[string]any:
					return len(nested), true
				case []any:
					return len(nested), true
				case map[string]any:
					if count, ok := graphEvidenceEntryCount(nested); ok {
						return count, true
					}
				default:
					if count, ok := graphEvidenceNumericCount(raw); ok {
						return count, true
					}
				}
			}
		}
	default:
		if count, ok := graphEvidenceNumericCount(value); ok {
			return count, true
		}
	}
	return 0, false
}

func graphEvidenceNumericCount(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		count, err := strconv.Atoi(strings.TrimSpace(typed))
		return count, err == nil
	case fmt.Stringer:
		count, err := strconv.Atoi(strings.TrimSpace(typed.String()))
		return count, err == nil
	default:
		return 0, false
	}
}

func missingDoDItemsFromFailureOutput(output map[string]any) []map[string]any {
	if len(output) == 0 {
		return nil
	}
	if items := mapSliceFromValue(output["missingDoDItems"]); len(items) > 0 {
		return items
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		if items := mapSliceFromValue(diagnostics["missingDoDItems"]); len(items) > 0 {
			return items
		}
	}
	if failedOutput := mapFromValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		if items := missingDoDItemsFromFailureOutput(failedOutput); len(items) > 0 {
			return items
		}
	}
	if assessment := doDAssessmentFromOutput(output); len(assessment) > 0 {
		if items := missingDoDItemsFromDoDAssessment(assessment); len(items) > 0 {
			return items
		}
	}
	return nil
}

func (r *repairSubsystem) recentRuntimeRepairAttemptSummaries(ctx context.Context, runID common.RunID, failedStep *runModel.Step, limit int) []map[string]any {
	if r == nil || r.steps == nil || runID == "" || failedStep == nil || limit <= 0 {
		return nil
	}
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	steps = orderedRuntimeEvidenceSteps(steps)
	scope := runtimeRepairScopeFromFailedStep(failedStep)
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	windowRoot := runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep)
	failureClass := strings.TrimSpace(scope.failureClass)
	ret := make([]map[string]any, 0, limit)
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if targetScope != "" {
			if runtimeFailureTargetScopeFromStep(step) != targetScope {
				continue
			}
			if windowRoot != "" && runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
				continue
			}
			if failureClass != "" {
				stepClass := strings.TrimSpace(firstNonEmptyString(
					stringFromMap(step.Input, stepInputKeyRepairOfFailureClass),
					stringFromMap(step.Input, failureOutputKeyClass),
					stringFromMap(step.Output, failureOutputKeyClass),
				))
				if stepClass != "" && stepClass != failureClass {
					continue
				}
			}
		} else if !runtimeRepairStepMatchesScope(step, scope) {
			continue
		}
		item := map[string]any{
			"stepId":       string(step.ID),
			"state":        string(step.State),
			"attempt":      runtimeRepairAttemptValue(step),
			"failureClass": stringFromMap(step.Output, failureOutputKeyClass),
			"failureStage": stringFromMap(step.Output, "stage"),
		}
		if summary := runtimeRepairAttemptFailureSummary(step); summary != "" {
			item["failureSummary"] = summary
		}
		if rejectedLLMDecisionOutputForProgress(step) {
			item["rejectedOutputSummaryOmitted"] = true
		}
		if missing := missingDoDItemsFromFailureOutput(step.Output); len(missing) > 0 {
			item["missingDoDItems"] = missing
		}
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	if len(ret) > limit {
		ret = ret[len(ret)-limit:]
	}
	return ret
}

func runtimeRepairAttemptFailureSummary(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	if rejectedLLMDecisionOutputForProgress(step) {
		return firstNonEmptyString(stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason"), stringFromMap(step.Output, failureOutputKeySignature))
	}
	return firstNonEmptyString(stringFromMap(step.Output, "summary"), stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason"))
}

func runtimeRepairAttemptValue(step *runModel.Step) int {
	if step == nil {
		return 0
	}
	switch typed := step.Input[stepInputKeyRuntimeRepairAttempt].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (r *repairSubsystem) workspaceFileManifest(ctx context.Context, run *runModel.Run) []map[string]any {
	if r == nil || r.workspaces == nil || run == nil {
		return nil
	}
	workspace, err := r.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil {
		return nil
	}
	return workspaceFileManifestFromDirectory(workspace.DeliverableDirectory)
}

func workspaceFileManifestFromDirectory(deliverableDirectory string) []map[string]any {
	files, err := workspaceManifestFiles(deliverableDirectory)
	if err != nil || len(files) == 0 {
		return nil
	}
	manifest := make([]map[string]any, 0, len(files))
	for _, file := range files {
		item := map[string]any{
			"path": file.Path,
			"kind": "file",
			"size": file.Size,
		}
		if file.Executable {
			item["executable"] = true
		}
		manifest = append(manifest, item)
	}
	return manifest
}
