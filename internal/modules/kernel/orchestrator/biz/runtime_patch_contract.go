package orchestrator

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateLLMExecutionGraphOutputContract(step *runModel.Step, output map[string]any) error {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return nil
	}
	if err := validateExecutionGraphPatchApplyTargetUniqueness(graph); err != nil {
		return err
	}
	for _, node := range graph.Nodes {
		kind := strings.TrimSpace(node.Kind)
		switch kind {
		case "patch/preview", "patch/apply":
			request, err := runtimePatchRequestFromMap(node.Input)
			if err != nil {
				return fmt.Errorf("%w: executionGraph %s input invalid: %v", common.ErrInvalidArgument, kind, err)
			}
			if err := validateRepairPatchRequestGranularity(step, request); err != nil {
				return err
			}
			if kind == "patch/apply" {
				if err := validateRepairPatchTargetRelevance(step, request); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateExecutionGraphPatchApplyTargetUniqueness(graph runtimemodel.BlockExecutionGraph) error {
	seen := map[string]string{}
	for _, node := range graph.Nodes {
		if canonicalRuntimeGraphNodeKind(node.Kind) != "patch/apply" {
			continue
		}
		request, err := runtimePatchRequestFromMap(node.Input)
		if err != nil {
			continue
		}
		baseHash := strings.TrimSpace(request.BaseHash)
		if baseHash == "" {
			continue
		}
		target := filepath.Clean(strings.TrimSpace(request.TargetRef))
		if target == "" || target == "." {
			continue
		}
		key := target + "\x00" + strings.ToLower(baseHash)
		if previous := seen[key]; previous != "" {
			return fmt.Errorf("%w: executionGraph patch/apply contains multiple patches for target %q with the same baseHash in one graph (%s and %s); merge them into one cohesive patch or retry after a fresh target snapshot", common.ErrInvalidArgument, request.TargetRef, previous, strings.TrimSpace(node.ID))
		}
		seen[key] = strings.TrimSpace(node.ID)
	}
	return nil
}

func validateLLMNextStepPatchTargetRelevance(step *runModel.Step, output map[string]any) error {
	if !repairPatchTargetRelevanceRequired(step) {
		return nil
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) != "patch/apply" {
		return nil
	}
	nextInput := mapFromValue(output[stepOutputKeyNextInput])
	request, err := runtimePatchRequestFromMap(nextInput)
	if err != nil {
		return fmt.Errorf("%w: nextStep patch/apply input invalid: %v", common.ErrInvalidArgument, err)
	}
	if err := validateRepairPatchTargetRelevance(step, request); err != nil {
		return err
	}
	return nil
}

func validateRepairPatchRequestGranularity(step *runModel.Step, request runtimePatchRequest) error {
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if err := validateRepairPatchRequestSubstance(step, request); err != nil {
		return err
	}
	if strings.TrimSpace(request.PatchType) == "unified_diff" && len(repairPatchContextItems(step)) > 0 {
		if ok, reason := unifiedDiffHasSubstantiveNonBlankChange(request.Diff); !ok {
			if reason == "" {
				reason = "non_substantive_unified_diff"
			}
			return fmt.Errorf("%w: runtime repair patch/apply unified_diff for target %q is non-substantive (%s); rebuild an exact patch from current target snapshot or return a real governance blocker", common.ErrInvalidArgument, request.TargetRef, reason)
		}
	}
	if strings.TrimSpace(request.PatchType) != "file_write" {
		return nil
	}
	if len(repairPatchContextItems(step)) == 0 {
		return nil
	}
	target := filepath.Clean(strings.TrimSpace(request.TargetRef))
	if target == "." || target == "" {
		return nil
	}
	for _, item := range repairPatchContextItems(step) {
		if item == nil || !boolValue(item["exists"]) {
			continue
		}
		itemTarget := filepath.Clean(strings.TrimSpace(stringFromMap(item, "targetRef")))
		if itemTarget != target {
			continue
		}
		if repairPatchContextAllowsSafeFileWrite(item, request) {
			return nil
		}
		return fmt.Errorf("%w: runtime repair patch/apply file_write cannot replace existing target %q when current repair context is available; use a minimal unified_diff or exact replacement against the current snapshot", common.ErrInvalidArgument, request.TargetRef)
	}
	return nil
}

func validateRepairPatchRequestSubstance(step *runModel.Step, request runtimePatchRequest) error {
	switch strings.TrimSpace(request.PatchType) {
	case "text_replace":
		if strings.TrimSpace(request.OldText) == strings.TrimSpace(request.NewText) {
			return fmt.Errorf("%w: runtime repair patch/apply text_replace for target %q is non-substantive; return a substantive state-changing artifact, bounded validation evidence when already satisfied, or a real governance blocker", common.ErrInvalidArgument, request.TargetRef)
		}
	case "file_write":
		target := filepath.Clean(strings.TrimSpace(request.TargetRef))
		if target == "" || target == "." {
			return nil
		}
		for _, item := range repairPatchContextItems(step) {
			if item == nil || !boolValue(item["exists"]) {
				continue
			}
			itemTarget := filepath.Clean(strings.TrimSpace(stringFromMap(item, "targetRef")))
			if itemTarget != target {
				continue
			}
			content := stringFromMap(item, "content")
			if content == "" || containsRuntimeRedactionMarker(content) || boolValue(item["truncated"]) || boolValue(item["isBinary"]) {
				continue
			}
			if strings.TrimSpace(content) == strings.TrimSpace(request.Content) {
				return fmt.Errorf("%w: runtime repair patch/apply file_write for target %q is non-substantive; current target content differs only by whitespace", common.ErrInvalidArgument, request.TargetRef)
			}
		}
	}
	return nil
}

func repairPatchContextAllowsSafeFileWrite(item map[string]any, request runtimePatchRequest) bool {
	if len(item) == 0 || strings.TrimSpace(request.PatchType) != "file_write" {
		return false
	}
	if !boolValue(item["contentComplete"]) || !boolValue(item["safeForFileWrite"]) {
		return false
	}
	if boolValue(item["redacted"]) || boolValue(item["truncated"]) || boolValue(item["isBinary"]) {
		return false
	}
	contentHash := strings.TrimSpace(stringFromMap(item, "contentHash"))
	if contentHash == "" || strings.TrimSpace(request.BaseHash) == "" || strings.TrimSpace(request.BaseHash) != contentHash {
		return false
	}
	if containsRuntimeRedactionMarker(request.Content) || containsUnresolvedRuntimeTemplate(request.Content) {
		return false
	}
	return true
}

func validateRepairPatchOutputTargetRelevance(step *runModel.Step, output map[string]any) error {
	if !repairPatchTargetRelevanceRequired(step) {
		return nil
	}
	requests, err := runtimePatchRequests(output)
	if err != nil {
		return err
	}
	for _, request := range requests {
		if err := validateRepairPatchTargetRelevance(step, request); err != nil {
			return err
		}
	}
	return validatePrimaryDiagnosticPatchOutputContract(step, output)
}

func validateRepairPatchTargetRelevance(step *runModel.Step, request runtimePatchRequest) error {
	if !repairPatchTargetRelevanceRequired(step) {
		return nil
	}
	targetRefs := repairPatchRelevantTargetRefs(step)
	if len(targetRefs) == 0 {
		return nil
	}
	if repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if repairPatchRequestDeclaresRelatedFailureTargets(request, targetRefs) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if primaryDiagnosticRequestHasInferredRelatedShapeRepair(step, request, primaryDiagnosticRepairContractForStep(step)) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if repairPatchRequestDeclaresCurrentCoverageTarget(step, request) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if repairPatchTargetRefIsGroundedRootCauseCandidate(step.Input, request.TargetRef, targetRefs) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if repairPatchTargetRefIsRelatedPrerequisite(step.Input, request.TargetRef, targetRefs) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	return fmt.Errorf("%w: runtime repair patch/apply target %q is not related to failure target scope %v; patch a current failure target or return a real governance blocker", common.ErrInvalidArgument, request.TargetRef, limitedStrings(targetRefs, 5))
}

func repairPatchRequestDeclaresCurrentCoverageTarget(step *runModel.Step, request runtimePatchRequest) bool {
	if step == nil || step.Input == nil || !coverageBoundStateChangingOutputRequired(nil, step) {
		return false
	}
	allowedTargets := coverageBoundTargetTokens(step)
	if len(allowedTargets) == 0 {
		return false
	}
	if patchNodeDeclaresCoverageTargets(request.Extra, allowedTargets) {
		return true
	}
	return patchNodeDeclaresCoverageTargets(map[string]any{"coverageTargets": request.CoverageTargets}, allowedTargets)
}

func validatePrimaryDiagnosticPatchOutputContract(step *runModel.Step, output map[string]any) error {
	contract := primaryDiagnosticRepairContractForStep(step)
	if len(contract) == 0 || !boolValue(contract["required"]) {
		return nil
	}
	requests, err := primaryDiagnosticPatchRequestsFromOutput(output)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	for _, request := range requests {
		if primaryDiagnosticPatchRequestAddressesContract(step, request, contract) {
			return nil
		}
	}
	return fmt.Errorf("%w: primary diagnostic repair requires patch/apply to address line-referenced resource shape blockers before secondary changes", common.ErrInvalidArgument)
}

func primaryDiagnosticPatchRequestsFromOutput(output map[string]any) ([]runtimePatchRequest, error) {
	if output == nil {
		return nil, nil
	}
	var ret []runtimePatchRequest
	if requests, err := runtimePatchRequests(output); err != nil {
		return nil, err
	} else {
		ret = append(ret, requests...)
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		for _, node := range graph.Nodes {
			if strings.TrimSpace(node.Kind) != "patch/apply" {
				continue
			}
			request, err := runtimePatchRequestFromMap(node.Input)
			if err != nil {
				return nil, fmt.Errorf("%w: executionGraph patch/apply input invalid: %v", common.ErrInvalidArgument, err)
			}
			ret = append(ret, request)
		}
	}
	if _, nextKind := nextStepSpec(output); strings.TrimSpace(nextKind) == "patch/apply" {
		request, err := runtimePatchRequestFromMap(mapFromValue(output[stepOutputKeyNextInput]))
		if err != nil {
			return nil, fmt.Errorf("%w: nextStep patch/apply input invalid: %v", common.ErrInvalidArgument, err)
		}
		ret = append(ret, request)
	}
	return ret, nil
}

func primaryDiagnosticRepairContractForStep(step *runModel.Step) map[string]any {
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	inputSignals := mapFromValue(step.Input["structuredFailureSignals"])
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	actionSignals := mapFromValue(actionPackage["structuredFailureSignals"])
	if structuredFailureSignalsAreRuntimeContractOnly(inputSignals) ||
		structuredFailureSignalsAreRuntimeContractOnly(actionSignals) {
		return nil
	}
	if contract := mapFromValue(step.Input["primaryDiagnosticRepair"]); len(contract) > 0 {
		return contract
	}
	if contract := mapFromValue(actionPackage["primaryDiagnosticRepair"]); len(contract) > 0 {
		return contract
	}
	if len(inputSignals) > 0 {
		return primaryDiagnosticRepairContractFromSignals(inputSignals)
	}
	if len(actionSignals) > 0 {
		return primaryDiagnosticRepairContractFromSignals(actionSignals)
	}
	return nil
}

func primaryDiagnosticPatchRequestAddressesContract(step *runModel.Step, request runtimePatchRequest, contract map[string]any) bool {
	if primaryDiagnosticPatchRequestDirectlyAddressesContract(step, request, contract) {
		return true
	}
	targetRefs := primaryDiagnosticTargetRefsFromContract(contract)
	return primaryDiagnosticGraphEvidenceAlreadyAddressesContract(step, contract) &&
		primaryDiagnosticRequestTargetsDiagnosticScope(request, targetRefs)
}

func primaryDiagnosticPatchRequestDirectlyAddressesContract(step *runModel.Step, request runtimePatchRequest, contract map[string]any) bool {
	if len(contract) == 0 {
		return true
	}
	lineRefs := mapSliceFromValue(contract["lineRefs"])
	targetRefs := primaryDiagnosticTargetRefsFromContract(contract)
	if !primaryDiagnosticRequestTargetsDiagnosticScope(request, targetRefs) {
		return primaryDiagnosticRequestHasInferredRelatedShapeRepair(step, request, contract)
	}
	if primaryDiagnosticRequestHasMatchingDeclaration(request, contract) {
		return true
	}
	if primaryDiagnosticRequestDiffOverlapsLineRefs(request, lineRefs) {
		return true
	}
	if primaryDiagnosticRequestTextReplaceOverlapsLineRefs(step, request, lineRefs) {
		return true
	}
	if primaryDiagnosticRequestFileWriteChangesLineRefs(step, request, lineRefs) {
		return true
	}
	if primaryDiagnosticRequestMentionsIdentifiers(request, contract) &&
		(repairPatchTargetRefMatches(request.TargetRef, targetRefs) || repairPatchRequestDeclaresRelatedFailureTargets(request, targetRefs)) {
		return true
	}
	return false
}

func primaryDiagnosticGraphEvidenceAlreadyAddressesContract(step *runModel.Step, contract map[string]any) bool {
	if step == nil || step.Input == nil || len(contract) == 0 {
		return false
	}
	for _, output := range primaryDiagnosticPriorGraphEvidenceOutputs(step.Input) {
		requests, err := runtimePatchRequests(output)
		if err != nil {
			continue
		}
		for _, request := range requests {
			if primaryDiagnosticPatchRequestDirectlyAddressesContract(step, request, contract) {
				return true
			}
		}
	}
	return false
}

func primaryDiagnosticPriorGraphEvidenceOutputs(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	var ret []map[string]any
	if output := mapFromValue(input["previousStepOutput"]); len(output) > 0 {
		ret = append(ret, output)
	}
	for _, item := range mapSliceFromValue(input[stepInputKeyGraphEvidence]) {
		output := mapFromValue(item["output"])
		if len(output) == 0 {
			continue
		}
		ret = append(ret, output)
	}
	return ret
}

func primaryDiagnosticTargetRefsFromContract(contract map[string]any) []string {
	if len(contract) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, value := range stringSliceFromValue(contract["targetRefs"]) {
		add(value)
	}
	for _, ref := range mapSliceFromValue(contract["lineRefs"]) {
		add(stringFromMap(ref, "targetRef"))
	}
	return ret
}

func primaryDiagnosticRequestHasInferredRelatedShapeRepair(step *runModel.Step, request runtimePatchRequest, contract map[string]any) bool {
	if len(contract) == 0 || !boolValue(contract["required"]) {
		return false
	}
	targetRefs := primaryDiagnosticTargetRefsFromContract(contract)
	if len(targetRefs) == 0 || repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		return false
	}
	if !primaryDiagnosticRequestTargetsRelatedResourceNeighborhood(step, request, targetRefs) {
		return false
	}
	identifiers := primaryDiagnosticMeaningfulIdentifiers(contract)
	if len(identifiers) == 0 {
		return false
	}
	changedText := primaryDiagnosticPatchChangedText(request)
	if changedText == "" {
		return false
	}
	return primaryDiagnosticTextMentionsAny(changedText, identifiers)
}

func primaryDiagnosticRequestTargetsRelatedResourceNeighborhood(step *runModel.Step, request runtimePatchRequest, targetRefs []string) bool {
	if len(targetRefs) == 0 {
		return false
	}
	for _, targetRef := range targetRefs {
		if repairPatchTargetsShareDirectory(request.TargetRef, targetRef) {
			return true
		}
	}
	if step == nil || step.Input == nil {
		return false
	}
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	for _, item := range mapSliceFromValue(actionPackage["workspaceFileManifest"]) {
		if !repairPatchTargetRefMatches(stringFromMap(item, "path"), []string{request.TargetRef}) {
			continue
		}
		for _, targetRef := range targetRefs {
			if repairPatchTargetsShareDirectory(request.TargetRef, targetRef) {
				return true
			}
		}
	}
	return false
}

func primaryDiagnosticMeaningfulIdentifiers(contract map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	for _, value := range stringSliceFromValue(contract["identifiers"]) {
		trimmed := strings.TrimSpace(value)
		normalized := strings.ToLower(trimmed)
		if trimmed == "" || len([]rune(trimmed)) < 3 || seen[normalized] {
			continue
		}
		if primaryDiagnosticMetadataIdentifier(normalized) {
			continue
		}
		seen[normalized] = true
		ret = append(ret, trimmed)
	}
	return ret
}

func primaryDiagnosticMetadataIdentifier(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	switch value {
	case "fail", "failed", "failure", "failureclass", "failuresignature", "failuretargets", "exitcode", "status", "error", "reason", "stdout", "stderr":
		return true
	}
	return strings.Contains(value, "failure") ||
		strings.Contains(value, "failed") ||
		strings.HasPrefix(value, "exit") ||
		strings.HasSuffix(value, ".test")
}

func primaryDiagnosticPatchChangedText(request runtimePatchRequest) string {
	switch strings.TrimSpace(request.PatchType) {
	case "text_replace":
		return primaryDiagnosticNonCommentText(strings.Join([]string{
			request.OldText,
			request.NewText,
		}, "\n"))
	case "file_write":
		return primaryDiagnosticNonCommentText(request.Content)
	case "unified_diff":
		filePatch, err := parseSingleFileUnifiedDiff(request.Diff)
		if err != nil {
			return primaryDiagnosticNonCommentText(request.Diff)
		}
		var lines []string
		for _, hunk := range filePatch.Hunks {
			for _, line := range hunk.Lines {
				if line == "" {
					continue
				}
				switch line[0] {
				case '+', '-':
					lines = append(lines, strings.TrimSpace(line[1:]))
				}
			}
		}
		return primaryDiagnosticNonCommentText(strings.Join(lines, "\n"))
	default:
		return ""
	}
}

func primaryDiagnosticNonCommentText(value string) string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || primaryDiagnosticLooksCommentOnly(line) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.ToLower(strings.Join(lines, "\n"))
}

func primaryDiagnosticLooksCommentOnly(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "--") ||
		strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "*") ||
		strings.HasPrefix(line, "<!--")
}

func primaryDiagnosticRequestTargetsDiagnosticScope(request runtimePatchRequest, targetRefs []string) bool {
	if len(targetRefs) == 0 {
		return true
	}
	if repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		return true
	}
	return repairPatchRequestDeclaresRelatedFailureTargets(request, targetRefs)
}

func primaryDiagnosticRequestDiffOverlapsLineRefs(request runtimePatchRequest, lineRefs []map[string]any) bool {
	if strings.TrimSpace(request.PatchType) != "unified_diff" || len(lineRefs) == 0 {
		return false
	}
	filePatch, err := parseSingleFileUnifiedDiff(request.Diff)
	if err != nil {
		return false
	}
	for _, ref := range lineRefs {
		if !repairPatchTargetRefMatches(request.TargetRef, []string{stringFromMap(ref, "targetRef")}) {
			continue
		}
		line := intFromAny(ref["line"])
		if line <= 0 {
			continue
		}
		for _, hunk := range filePatch.Hunks {
			if primaryDiagnosticHunkOverlapsLine(hunk, line) {
				return true
			}
		}
	}
	return false
}

func primaryDiagnosticHunkOverlapsLine(hunk unifiedDiffHunk, line int) bool {
	if line <= 0 || !hunk.HasOldStart {
		return false
	}
	oldLineCount := 0
	for _, item := range hunk.Lines {
		if item == "" {
			continue
		}
		switch item[0] {
		case ' ', '-':
			oldLineCount++
		}
	}
	if oldLineCount <= 0 {
		oldLineCount = 1
	}
	const margin = 2
	start := hunk.OldStart - margin
	end := hunk.OldStart + oldLineCount + margin
	return line >= start && line <= end
}

func primaryDiagnosticRequestTextReplaceOverlapsLineRefs(step *runModel.Step, request runtimePatchRequest, lineRefs []map[string]any) bool {
	if strings.TrimSpace(request.PatchType) != "text_replace" || len(lineRefs) == 0 || request.OldText == "" {
		return false
	}
	for _, ref := range lineRefs {
		targetRef := stringFromMap(ref, "targetRef")
		if !repairPatchTargetRefMatches(request.TargetRef, []string{targetRef}) {
			continue
		}
		line := intFromAny(ref["line"])
		if line <= 0 {
			continue
		}
		content, ok := currentRepairSnapshotContentForTarget(step, targetRef)
		if !ok {
			continue
		}
		if primaryDiagnosticTextReplaceTouchesLine(content, request.OldText, line) {
			return true
		}
	}
	return false
}

func primaryDiagnosticTextReplaceTouchesLine(content string, oldText string, oneBasedLine int) bool {
	if content == "" || oldText == "" || oneBasedLine <= 0 {
		return false
	}
	start := 0
	for {
		idx := strings.Index(content[start:], oldText)
		if idx < 0 {
			return false
		}
		idx += start
		lineStart := 1 + strings.Count(content[:idx], "\n")
		lineCount := strings.Count(oldText, "\n") + 1
		if strings.HasSuffix(oldText, "\n") && lineCount > 1 {
			lineCount--
		}
		const margin = 1
		lineEnd := lineStart + lineCount - 1
		if oneBasedLine >= lineStart-margin && oneBasedLine <= lineEnd+margin {
			return true
		}
		start = idx + 1
		if start >= len(content) {
			return false
		}
	}
}

func primaryDiagnosticRequestFileWriteChangesLineRefs(step *runModel.Step, request runtimePatchRequest, lineRefs []map[string]any) bool {
	if strings.TrimSpace(request.PatchType) != "file_write" || len(lineRefs) == 0 || strings.TrimSpace(request.Content) == "" {
		return false
	}
	for _, ref := range lineRefs {
		targetRef := stringFromMap(ref, "targetRef")
		if !repairPatchTargetRefMatches(request.TargetRef, []string{targetRef}) {
			continue
		}
		line := intFromAny(ref["line"])
		if line <= 0 {
			continue
		}
		content, ok := currentRepairSnapshotContentForTarget(step, targetRef)
		if !ok {
			continue
		}
		if primaryDiagnosticFileWriteChangesLine(content, request.Content, line) {
			return true
		}
	}
	return false
}

func currentRepairSnapshotContentForTarget(step *runModel.Step, targetRef string) (string, bool) {
	if step == nil || step.Input == nil {
		return "", false
	}
	for _, item := range repairPatchContextItems(step) {
		if item == nil || !repairPatchTargetRefMatches(stringFromMap(item, "targetRef"), []string{targetRef}) {
			continue
		}
		content := stringFromMap(item, "content")
		if content != "" && !containsRuntimeRedactionMarker(content) {
			return content, true
		}
	}
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	for _, item := range mapSliceFromValue(actionPackage["targetSnapshots"]) {
		if item == nil || !repairPatchTargetRefMatches(stringFromMap(item, "targetRef"), []string{targetRef}) {
			continue
		}
		content := stringFromMap(item, "content")
		if content != "" && !containsRuntimeRedactionMarker(content) {
			return content, true
		}
	}
	return "", false
}

func primaryDiagnosticFileWriteChangesLine(oldContent string, newContent string, oneBasedLine int) bool {
	if oneBasedLine <= 0 {
		return false
	}
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	idx := oneBasedLine - 1
	if idx >= len(oldLines) {
		return false
	}
	oldLine := oldLines[idx]
	if idx >= len(newLines) {
		return true
	}
	if newLines[idx] != oldLine {
		return true
	}
	return strings.TrimSpace(oldLine) != "" && !strings.Contains(newContent, oldLine)
}

func primaryDiagnosticRequestHasMatchingDeclaration(request runtimePatchRequest, contract map[string]any) bool {
	targetRefs := stringSliceFromValue(contract["targetRefs"])
	identifiers := primaryDiagnosticContractIdentifiers(contract)
	lineRefs := mapSliceFromValue(contract["lineRefs"])
	for _, key := range []string{"primaryDiagnosticResolution", "primaryDiagnosticResolutions", "addressesFailureSignals"} {
		if primaryDiagnosticDeclarationMatches(request.Extra[key], request, targetRefs, identifiers, lineRefs) {
			return true
		}
	}
	return false
}

func primaryDiagnosticDeclarationMatches(value any, request runtimePatchRequest, targetRefs []string, identifiers []string, lineRefs []map[string]any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return primaryDiagnosticDeclarationMapMatches(typed, request, targetRefs, identifiers, lineRefs)
	case []map[string]any:
		for _, item := range typed {
			if primaryDiagnosticDeclarationMapMatches(item, request, targetRefs, identifiers, lineRefs) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if primaryDiagnosticDeclarationMatches(item, request, targetRefs, identifiers, lineRefs) {
				return true
			}
		}
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		if text == "" {
			return false
		}
		return primaryDiagnosticTextMentionsAny(text, targetRefs) &&
			(primaryDiagnosticTextMentionsAny(text, identifiers) || primaryDiagnosticTextMentionsLineRef(text, lineRefs))
	}
	return false
}

func primaryDiagnosticDeclarationMapMatches(item map[string]any, request runtimePatchRequest, targetRefs []string, identifiers []string, lineRefs []map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	targetMatched := false
	for _, key := range []string{"targetRef", "failureTarget", "relatedFailureTarget"} {
		if repairPatchTargetRefMatches(stringFromMap(item, key), targetRefs) {
			targetMatched = true
			break
		}
	}
	if !targetMatched {
		for _, value := range stringSliceFromValue(item["targetRefs"]) {
			if repairPatchTargetRefMatches(value, targetRefs) {
				targetMatched = true
				break
			}
		}
	}
	if !targetMatched && repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		targetMatched = true
	}
	if !targetMatched {
		return false
	}
	lineMatched := false
	if line := intFromAny(item["line"]); line > 0 {
		for _, ref := range lineRefs {
			if line == intFromAny(ref["line"]) {
				lineMatched = true
				break
			}
		}
	}
	identifierMatched := primaryDiagnosticTextMentionsAny(strings.ToLower(strings.TrimSpace(strings.Join([]string{
		stringFromAny(item["identifier"]),
		stringFromAny(item["identifiers"]),
		stringFromAny(item["summary"]),
		stringFromAny(item["reason"]),
	}, "\n"))), identifiers)
	return lineMatched || identifierMatched
}

func primaryDiagnosticRequestMentionsIdentifiers(request runtimePatchRequest, contract map[string]any) bool {
	text := strings.ToLower(strings.Join([]string{
		request.Summary,
		request.Content,
		request.Diff,
		request.OldText,
		request.NewText,
		stringFromAny(request.Extra["summary"]),
		stringFromAny(request.Extra["reason"]),
	}, "\n"))
	return primaryDiagnosticTextMentionsAny(text, primaryDiagnosticContractIdentifiers(contract))
}

func primaryDiagnosticTextMentionsAny(text string, values []string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if primaryDiagnosticLooksSimpleIdentifier(value) {
			if primaryDiagnosticTextContainsIdentifier(text, value) {
				return true
			}
			continue
		}
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func primaryDiagnosticContractIdentifiers(contract map[string]any) []string {
	if len(contract) == 0 {
		return nil
	}
	ret := stringSliceFromValue(contract["identifiers"])
	for _, message := range stringSliceFromValue(contract["messages"]) {
		ret = append(ret, extractStructuredFailureLinePayloadIdentifiers(message)...)
	}
	return limitedStrings(compactUniqueStrings(ret), maxLLMContextSliceItems)
}

func primaryDiagnosticLooksSimpleIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return false
		}
	}
	return true
}

func primaryDiagnosticTextContainsIdentifier(text string, identifier string) bool {
	text = strings.ToLower(text)
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if text == "" || identifier == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(text[start:], identifier)
		if idx < 0 {
			return false
		}
		absolute := start + idx
		beforeOK := absolute == 0 || !primaryDiagnosticIdentifierRune(rune(text[absolute-1]))
		after := absolute + len(identifier)
		afterOK := after >= len(text) || !primaryDiagnosticIdentifierRune(rune(text[after]))
		if beforeOK && afterOK {
			return true
		}
		start = absolute + len(identifier)
		if start >= len(text) {
			return false
		}
	}
}

func primaryDiagnosticIdentifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func primaryDiagnosticTextMentionsLineRef(text string, lineRefs []map[string]any) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, ref := range lineRefs {
		target := strings.ToLower(strings.TrimSpace(stringFromMap(ref, "targetRef")))
		line := intFromAny(ref["line"])
		if target == "" || line <= 0 {
			continue
		}
		if strings.Contains(text, target) && strings.Contains(text, strconv.Itoa(line)) {
			return true
		}
	}
	return false
}

func validateRepeatedRepairRootCauseTargetConsistency(step *runModel.Step, request runtimePatchRequest) error {
	if step == nil || step.Input == nil {
		return nil
	}
	reassessment := mapFromValue(step.Input["strategyReassessment"])
	if !boolValue(reassessment["required"]) {
		return nil
	}
	if !strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		return nil
	}
	if strategyRepairModeFromCandidate(request.Extra) != strategyRepairModeShiftRootCauseTarget {
		return nil
	}
	declaredRootCauseTargets := rootCauseTargetRefsFromStrategyValue(request.Extra["rootCauseTarget"])
	if len(declaredRootCauseTargets) == 0 {
		return nil
	}
	targetRef := normalizeRepairPatchTargetRef(request.TargetRef)
	if targetRef == "" {
		return nil
	}
	if repairPatchTargetRefMatches(targetRef, declaredRootCauseTargets) {
		return nil
	}
	return fmt.Errorf("%w: strategyReassessment.required=true requires patch target %q to match declared rootCauseTarget %q when repeated failed-surface repair must shift targets", common.ErrInvalidArgument, request.TargetRef, strings.TrimSpace(stringFromAny(request.Extra["rootCauseTarget"])))
}

func rootCauseTargetRefsFromStrategyValue(value any) []string {
	text := strings.TrimSpace(stringFromAny(value))
	if text == "" {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, target := range repairPatchTargetRefsFromScopeText(text) {
		add(target)
	}
	for _, target := range extractStructuredFailureTargetRefs(text) {
		add(target)
	}
	for _, ref := range strings.Fields(text) {
		ref = strings.Trim(ref, "`'\"()[]{}<>;,")
		if strings.Contains(ref, ".") {
			add(ref)
			for _, separator := range []string{":", "#"} {
				if prefix, _, ok := strings.Cut(ref, separator); ok && strings.Contains(prefix, ".") {
					add(prefix)
				}
			}
		}
	}
	return ret
}
