package orchestrator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func structuredFailureSignalsFromFailureOutput(output map[string]any, execErr error, failureClass string, failureSignature string) map[string]any {
	texts := repairFailureSignalTexts(output, execErr, failureClass, failureSignature)
	sources := make([]string, 0, len(texts))
	targets := explicitFailureTargetsForSignature(output)
	hasCapabilityInputTargets := len(capabilityInputFailureTargets(output)) > 0
	hasExternalCapabilityCallIdentity := externalCapabilityFailureHasCallIdentity(output)
	externalRefs := []string{}
	identifiers := []string{}
	messages := []string{}
	lineRefs := []map[string]any{}
	seenLineRefs := map[string]bool{}
	for source, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		sources = append(sources, source)
		if shouldExtractFailureTextTargets(source, hasCapabilityInputTargets, hasExternalCapabilityCallIdentity) {
			for _, ref := range extractStructuredFailureLineRefs(text) {
				key := fmt.Sprintf("%s:%v:%v", ref["targetRef"], ref["line"], ref["column"])
				if seenLineRefs[key] {
					continue
				}
				seenLineRefs[key] = true
				lineRefs = append(lineRefs, ref)
				if targetRef := stringFromMap(ref, "targetRef"); targetRef != "" {
					targets = append(targets, targetRef)
				}
				if len(lineRefs) >= maxLLMContextSliceItems {
					break
				}
			}
			for _, targetRef := range extractStructuredFailureTargetRefs(text) {
				if structuredFailureRefLooksExternalReference(targetRef) {
					externalRefs = append(externalRefs, targetRef)
					continue
				}
				if structuredFailureTargetRefGroundedBySourceOrText(source, text, targetRef) {
					targets = append(targets, targetRef)
				}
			}
		}
		identifiers = append(identifiers, extractStructuredFailureIdentifiers(text)...)
		messages = append(messages, extractStructuredFailureMessages(text)...)
	}
	ret := map[string]any{}
	if capabilityInputLocatorInvalidOutput(output) {
		ret["capabilityInputLocatorInvalid"] = true
	}
	if sources = limitedStrings(compactUniqueStrings(sources), 8); len(sources) > 0 {
		ret["sources"] = sources
	}
	if targets = limitedStrings(compactUniqueStrings(targets), maxLLMContextSliceItems); len(targets) > 0 {
		ret["targetRefs"] = targets
	}
	if externalRefs = limitedStrings(compactUniqueStrings(externalRefs), maxLLMContextSliceItems); len(externalRefs) > 0 {
		ret["externalReferences"] = externalRefs
	}
	if len(lineRefs) > 0 {
		ret["lineRefs"] = lineRefs
	}
	if identifiers = limitedStrings(compactUniqueStrings(identifiers), maxLLMContextSliceItems); len(identifiers) > 0 {
		ret["identifiers"] = identifiers
	}
	if messages = limitedStrings(compactUniqueStrings(messages), 10); len(messages) > 0 {
		ret["messages"] = messages
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func shouldExtractFailureTextTargets(source string, hasCapabilityInputTargets bool, hasExternalCapabilityCallIdentity bool) bool {
	if !hasCapabilityInputTargets {
		return !hasExternalCapabilityCallIdentity
	}
	source = strings.ToLower(strings.TrimSpace(source))
	return strings.Contains(source, "signature") ||
		strings.HasSuffix(source, "targetref") ||
		strings.Contains(source, "runtimediagnostics")
}

func repairFailureSignalTexts(output map[string]any, execErr error, failureClass string, failureSignature string) map[string]string {
	ret := map[string]string{}
	add := func(source string, value string) {
		value = strings.TrimSpace(value)
		if source == "" || value == "" || containsRuntimeRedactionMarker(value) {
			return
		}
		ret[source] = value
	}
	for _, key := range []string{"stderr", "stdout", "stderrPreview", "stdoutPreview", "error", "reason", failureOutputKeySignature, "failureCategory"} {
		add(key, stringFromMap(output, key))
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		for _, key := range []string{"kind", "reason", "error", "targetRef"} {
			add("runtimeDiagnostics."+key, stringFromMap(diagnostics, key))
		}
	}
	if diagnostics := mapFromValue(output["streamDiagnostics"]); len(diagnostics) > 0 {
		for _, key := range []string{"kind", "reason", "error"} {
			add("streamDiagnostics."+key, stringFromMap(diagnostics, key))
		}
	}
	if diagnostics := mapFromValue(output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		for _, key := range []string{"targetRef", "stdoutPreview", "stderrPreview", "error", "reason", failureOutputKeySignature} {
			add("lastFailureDiagnostics."+key, stringFromMap(diagnostics, key))
		}
	}
	if failedOutput := mapFromValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		for _, key := range []string{"error", "reason", failureOutputKeySignature} {
			add("failedStepOutput."+key, stringFromMap(failedOutput, key))
		}
		if diagnostics := mapFromValue(failedOutput["runtimeDiagnostics"]); len(diagnostics) > 0 {
			for _, key := range []string{"targetRef", "conflictRef"} {
				add("failedStepOutput.runtimeDiagnostics."+key, stringFromMap(diagnostics, key))
			}
		}
	}
	add("execErr", errorString(execErr))
	add("failureClass", failureClass)
	add(failureOutputKeySignature, failureSignature)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func extractStructuredFailureLineRefs(text string) []map[string]any {
	matches := structuredFailureLineRefPattern.FindAllStringSubmatch(text, maxLLMContextSliceItems)
	if len(matches) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		targetRef := normalizeStructuredFailureTargetRef(match[1])
		if targetRef == "" || structuredFailureRefLooksExternalReference(targetRef) {
			continue
		}
		line, _ := strconv.Atoi(match[2])
		if line <= 0 {
			continue
		}
		item := map[string]any{
			"targetRef": targetRef,
			"line":      line,
		}
		if len(match) > 3 && strings.TrimSpace(match[3]) != "" {
			if column, _ := strconv.Atoi(match[3]); column > 0 {
				item["column"] = column
			}
		}
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func extractStructuredFailureTargetRefs(text string) []string {
	matches := structuredFailureTargetRefPattern.FindAllStringSubmatch(text, maxLLMContextSliceItems*2)
	if len(matches) == 0 {
		return nil
	}
	ret := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if targetRef := normalizeStructuredFailureTargetRef(match[1]); targetRef != "" && structuredFailureTextTargetRefAllowed(targetRef) {
			ret = append(ret, targetRef)
		}
	}
	return ret
}

func structuredFailureTargetRefGroundedBySourceOrText(source string, text string, targetRef string) bool {
	targetRef = normalizeStructuredFailureTargetRef(targetRef)
	if targetRef == "" {
		return false
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if strings.Contains(source, "signature") || strings.HasSuffix(source, "targetref") {
		return true
	}
	text = strings.ToLower(strings.TrimSpace(text))
	return structuredFailureTextGroundsPathDiagnostic(text) ||
		strings.Contains(text, "targetref=") ||
		strings.Contains(text, "failuretargets=") ||
		strings.Contains(text, "conflictref=")
}

func structuredFailureTextGroundsPathDiagnostic(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"no such file",
		"file does not exist",
		"cannot find",
		"not found",
		"lstat ",
		"stat ",
		"open ",
		"read ",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func normalizeStructuredFailureTargetRef(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "targetRef=")
	value = strings.TrimPrefix(value, "failureTargets=")
	value = strings.TrimPrefix(value, "conflictRef=")
	value = strings.Trim(value, "`'\"()[]{}<>;,")
	value = strings.ReplaceAll(value, "\\", "/")
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	value = strings.TrimPrefix(value, "a/")
	value = strings.TrimPrefix(value, "b/")
	value = strings.Trim(value, "/")
	if value == "" || strings.Contains(value, "://") || strings.Contains(value, " ") || !runtimeRepairTargetRefAdmissible(value) {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		return ""
	}
	if !strings.Contains(value, ".") {
		return ""
	}
	return value
}

func structuredFailureTextTargetRefAllowed(value string) bool {
	value = normalizeStructuredFailureTargetRef(value)
	if value == "" || !runtimeRepairTargetRefAdmissible(value) {
		return false
	}
	if strings.Contains(value, "/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(value))
	if ext == "" {
		return false
	}
	if len(ext) <= 1 || len(ext) > 7 {
		return false
	}
	for _, r := range ext[1:] {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return false
		}
	}
	base := strings.TrimSuffix(value, ext)
	if base == "" || strings.HasSuffix(base, ".") {
		return false
	}
	if structuredFailureBareTargetLooksLikeNumericMeasurement(base, ext) {
		return false
	}
	return true
}

func runtimeRepairTargetRefAdmissible(value string) bool {
	original := strings.TrimSpace(strings.Trim(value, `"'()[]{}<>;,`))
	if original == "" {
		return false
	}
	original = strings.ReplaceAll(original, "\\", "/")
	if filepath.IsAbs(original) || strings.HasPrefix(original, "/") || strings.HasPrefix(original, "~") {
		return false
	}
	clean := normalizeRepairPatchTargetRef(original)
	if clean == "" || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "../") {
		return false
	}
	lower := strings.ToLower(filepath.ToSlash(clean))
	if strings.Contains(lower, "://") || strings.Contains(lower, " ") || strings.Contains(lower, ":") {
		return false
	}
	for _, marker := range []string{
		".workorch-data/",
		".workorch-workspaces/",
		"/.workorch-data/",
		"/.workorch-workspaces/",
	} {
		if strings.Contains(lower, marker) || strings.HasPrefix(lower, strings.TrimSuffix(marker, "/")) {
			return false
		}
	}
	if runtimeRepairTargetLooksLikeLocalAbsolutePath(lower) {
		return false
	}
	if !strings.Contains(clean, "/") {
		ext := strings.ToLower(filepath.Ext(clean))
		base := strings.TrimSuffix(clean, ext)
		if structuredFailureBareTargetLooksLikeNumericMeasurement(base, ext) {
			return false
		}
	}
	if isRuntimeContractFieldTargetRef(lower) {
		return false
	}
	if isReservedWorkspacePatchTarget(clean) || isRuntimeDecisionArtifactTarget(clean) {
		return false
	}
	return true
}

func isRuntimeContractFieldTargetRef(cleanLower string) bool {
	cleanLower = strings.TrimSpace(filepath.ToSlash(cleanLower))
	if cleanLower == "" || strings.Contains(cleanLower, "/") {
		return false
	}
	for _, prefix := range []string{
		"dodcoverage.",
		"dodassessment.",
		"validationevidence.",
		"governance.",
		"executiongraph.",
		"llmexecutiondecision.",
		"convergencecontract.",
		"repairactionpackage.",
		"strategyreassessment.",
		"runtimecontext.",
		"runtimediagnostics.",
		"currentexecutionprogress.",
		"contextbudget.",
		"llmusage.",
	} {
		if strings.HasPrefix(cleanLower, prefix) {
			return true
		}
	}
	return false
}

func runtimeRepairTargetLooksLikeLocalAbsolutePath(cleanLower string) bool {
	cleanLower = strings.Trim(cleanLower, "/")
	if cleanLower == "" {
		return false
	}
	parts := strings.Split(cleanLower, "/")
	if len(parts) < 3 {
		return false
	}
	switch parts[0] {
	case "home", "users", "var", "tmp", "private", "opt", "usr":
		return true
	default:
		return false
	}
}

func isRuntimeDecisionArtifactTarget(targetRef string) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(strings.TrimSpace(targetRef))))
	clean = strings.TrimPrefix(clean, "./")
	base := filepath.Base(clean)
	if base == "llmexecutiondecision.plan" || strings.HasPrefix(base, "llmexecutiondecision.") {
		return true
	}
	switch base {
	case "request.json", "response.json", "repair-response.json", "usage.json", "metadata.json", "output.json", "input.json", "run.json":
		return true
	default:
		return false
	}
}

func filterAdmissibleRuntimeRepairTargetRefs(targetRefs []string) []string {
	if len(targetRefs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		targetRef = normalizeRepairPatchTargetRef(targetRef)
		if targetRef == "" || !runtimeRepairTargetRefAdmissible(targetRef) || seen[targetRef] {
			continue
		}
		seen[targetRef] = true
		ret = append(ret, targetRef)
	}
	return ret
}

func structuredFailureRefLooksExternalReference(value string) bool {
	value = normalizeStructuredFailureTargetRef(value)
	if value == "" || !strings.Contains(value, "/") {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) < 2 || !strings.Contains(parts[0], ".") {
		return false
	}
	return true
}

func structuredFailureBareTargetLooksLikeNumericMeasurement(base string, ext string) bool {
	base = strings.TrimSpace(base)
	ext = strings.TrimPrefix(strings.TrimSpace(ext), ".")
	if base == "" || ext == "" {
		return false
	}
	baseNumeric := true
	for _, r := range base {
		if r != '.' && (r < '0' || r > '9') {
			baseNumeric = false
			break
		}
	}
	if !baseNumeric {
		return false
	}
	hasDigit := false
	hasLetter := false
	for _, r := range ext {
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		if r >= 'a' && r <= 'z' {
			hasLetter = true
			continue
		}
		return false
	}
	return hasDigit || !hasLetter
}

func extractStructuredFailureIdentifiers(text string) []string {
	matches := structuredFailureIdentifierPattern.FindAllString(text, maxLLMContextSliceItems*4)
	if len(matches) == 0 {
		return nil
	}
	ret := make([]string, 0, len(matches))
	for _, value := range matches {
		value = strings.TrimSpace(value)
		if !structuredFailureIdentifierUseful(value) {
			continue
		}
		ret = append(ret, value)
	}
	return ret
}

func structuredFailureIdentifierUseful(value string) bool {
	if value == "" || len(value) < 3 || len(value) > 96 {
		return false
	}
	lower := strings.ToLower(value)
	if structuredFailureIdentifierStopWords[lower] {
		return false
	}
	hasShape := false
	for _, r := range value {
		if unicode.IsUpper(r) || unicode.IsDigit(r) || r == '_' {
			hasShape = true
			break
		}
	}
	return hasShape
}

func extractStructuredFailureMessages(text string) []string {
	lines := strings.Split(text, "\n")
	ret := make([]string, 0, min(len(lines), 10))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || containsRuntimeRedactionMarker(line) {
			continue
		}
		ret = append(ret, truncateLLMContextString(line))
		if len(ret) >= 10 {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func forbiddenRepairContentReuseFromFailure(input map[string]any, rawFailedOutput ...map[string]any) []string {
	output := map[string]any(nil)
	if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		output = rawFailedOutput[0]
	} else if failedOutput := mapFromValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		output = failedOutput
	}
	text := strings.ToLower(strings.Join(repairForbiddenReuseTextValues(input, output), "\n"))
	ret := []string{}
	if llmDecisionInvalidOutput(output) ||
		strings.Contains(text, "invalid json") ||
		strings.Contains(text, "invalid character") ||
		strings.Contains(text, "schema invalid") ||
		strings.Contains(text, "llm decision invalid") {
		ret = append(ret, "raw_invalid_llm_decision_content")
	}
	if runtimeValueContainsRedactionMarker(output, 0) ||
		runtimeValueContainsRedactionMarker(input["failedStepOutput"], 0) ||
		strings.Contains(text, "redacted runtime marker") ||
		strings.Contains(text, "runtime redaction marker") {
		ret = append(ret, "redacted_runtime_marker_text")
	}
	if strings.Contains(text, "malformed unified") ||
		strings.Contains(text, "invalid unified") ||
		strings.Contains(text, "hunk header") ||
		strings.Contains(text, "bare @@") {
		ret = append(ret, "malformed_diff_text")
	}
	if strings.Contains(text, "oldtext not found") ||
		strings.Contains(text, "old text not found") ||
		strings.Contains(text, "file/edit oldtext") {
		ret = append(ret, "missing_old_text")
	}
	if strings.Contains(text, "context mismatch") ||
		strings.Contains(text, "patch does not apply") ||
		strings.Contains(text, "stale") ||
		strings.Contains(text, "conflict") {
		ret = append(ret, "stale_patch_text")
	}
	return limitedStrings(compactUniqueStrings(ret), 8)
}

func repairForbiddenReuseTextValues(input map[string]any, output map[string]any) []string {
	values := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	for _, source := range []map[string]any{input, output} {
		for _, key := range []string{
			"stage",
			"status",
			"error",
			"reason",
			"failureReason",
			failureOutputKeyClass,
			failureOutputKeySignature,
			"failureCategory",
		} {
			add(stringFromMap(source, key))
		}
	}
	for _, key := range []string{"runtimeDiagnostics", "streamDiagnostics"} {
		diagnostics := mapFromValue(output[key])
		for _, diagnosticKey := range []string{"kind", "reason", "error"} {
			add(stringFromMap(diagnostics, diagnosticKey))
		}
	}
	return values
}

func runtimeValueContainsRedactionMarker(value any, depth int) bool {
	if depth > 4 || value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return containsRuntimeRedactionMarker(typed)
	case map[string]any:
		for _, nested := range typed {
			if runtimeValueContainsRedactionMarker(nested, depth+1) {
				return true
			}
		}
	case []map[string]any:
		for _, nested := range typed {
			if runtimeValueContainsRedactionMarker(nested, depth+1) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if runtimeValueContainsRedactionMarker(nested, depth+1) {
				return true
			}
		}
	case []string:
		for _, nested := range typed {
			if containsRuntimeRedactionMarker(nested) {
				return true
			}
		}
	}
	return false
}

var structuredFailureLineRefPattern = regexp.MustCompile(`(?:^|[\s"'(\[])([A-Za-z0-9_./\\-]+\.[A-Za-z0-9_./\\-]+):([0-9]+)(?::([0-9]+))?`)
var structuredFailureTargetRefPattern = regexp.MustCompile(`(?:^|[\s"'(\[=])([A-Za-z0-9_./\\-]+\.[A-Za-z0-9_./\\-]+)(?::[0-9]+(?::[0-9]+)?)?`)
var structuredFailureIdentifierPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)
var providerFailureHTTPStatusPattern = regexp.MustCompile(`\bhttp status\s+([0-9]{3})\b`)

var structuredFailureIdentifierStopWords = map[string]bool{
	"error": true, "failed": true, "failure": true, "status": true, "stage": true,
	"stderr": true, "stdout": true, "reason": true, "class": true, "targetref": true,
	"undefined": true, "unknown": true, "field": true, "literal": true, "type": true,
	"struct": true, "exit": true, "runtime": true, "patch": true, "diff": true,
	"content": true, "context": true, "mismatch": true, "invalid": true,
}

func repairActionTargetSnapshots(input map[string]any) []map[string]any {
	var ret []map[string]any
	seen := map[string]bool{}
	manifest := repairPatchRelevantWorkspaceManifest(input)
	add := func(items []map[string]any, source string) {
		for _, item := range items {
			targetRef := strings.TrimSpace(stringFromMap(item, "targetRef"))
			if canonical, ok := canonicalRepairTargetRefFromManifest(manifest, targetRef); ok {
				if canonical != normalizeRepairPatchTargetRef(targetRef) && !boolValue(item["exists"]) && strings.TrimSpace(stringFromMap(item, "content")) == "" {
					continue
				}
				targetRef = canonical
			}
			if targetRef == "" || !runtimeRepairTargetRefAdmissible(targetRef) || boolValue(item["isBinary"]) || seen[targetRef] {
				continue
			}
			seen[targetRef] = true
			snapshot := repairActionTargetSnapshot(item)
			if len(snapshot) == 0 || boolValue(snapshot["isBinary"]) {
				continue
			}
			if original := strings.TrimSpace(stringFromMap(snapshot, "targetRef")); original != "" && original != targetRef {
				snapshot["diagnosticTargetRef"] = original
			}
			snapshot["targetRef"] = targetRef
			if source != "" {
				snapshot["contextSource"] = source
			}
			ret = append(ret, snapshot)
			if len(ret) >= maxLLMContextSliceItems {
				return
			}
		}
	}
	add(mapSliceFromValue(input["failureTargetRepairContext"]), "failure_target")
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["readableResourceRepairContext"]), "readable_resource")
	}
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["patchRepairContext"]), "patch_target")
	}
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["targetSnapshots"]), "lineage_target")
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionTargetSnapshot(item map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range []string{
		"targetRef",
		"diagnosticTargetRef",
		"exists",
		"contentHash",
		"contentComplete",
		"safeForFileWrite",
		"redacted",
		"truncated",
		"isBinary",
		"size",
		"source",
		"error",
	} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	content := strings.TrimSpace(stringFromMap(item, "content"))
	if content != "" && !containsRuntimeRedactionMarker(content) {
		if boolValue(item["contentComplete"]) && boolValue(item["safeForFileWrite"]) {
			ret["content"] = content
		} else {
			ret["contentPreview"] = truncateLLMContextString(content)
		}
	}
	return ret
}

func repairActionPromotableRootCauseCandidateTargetSnapshots(candidates []map[string]any) []map[string]any {
	if len(candidates) == 0 {
		return nil
	}
	var ret []map[string]any
	seen := map[string]bool{}
	for _, candidate := range candidates {
		targetRef := normalizeRepairPatchTargetRef(stringFromMap(candidate, "targetRef"))
		if targetRef == "" || seen[targetRef] || !runtimeRepairTargetRefAdmissible(targetRef) || boolValue(candidate["isBinary"]) {
			continue
		}
		if !boolValue(candidate["exists"]) || !boolValue(candidate["contentComplete"]) || !boolValue(candidate["safeForFileWrite"]) ||
			strings.TrimSpace(stringFromMap(candidate, "contentHash")) == "" ||
			strings.TrimSpace(stringFromMap(candidate, "content")) == "" {
			continue
		}
		snapshot := repairActionTargetSnapshot(candidate)
		if len(snapshot) == 0 || boolValue(snapshot["isBinary"]) {
			continue
		}
		snapshot["targetRef"] = targetRef
		snapshot["contextSource"] = firstNonEmptyString(stringFromMap(candidate, "source"), "root_cause_candidate")
		seen[targetRef] = true
		ret = append(ret, snapshot)
		if len(ret) >= maxLLMContextSliceItems {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionTargetSnapshotMetadata(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		next := map[string]any{}
		for _, key := range []string{
			"targetRef",
			"diagnosticTargetRef",
			"exists",
			"contentHash",
			"contentComplete",
			"safeForFileWrite",
			"redacted",
			"truncated",
			"isBinary",
			"size",
			"source",
			"contextSource",
			"error",
		} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionWorkspaceManifest(input map[string]any) []map[string]any {
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) == 0 {
		return nil
	}
	limit := min(len(manifest), maxLLMContextSliceItems)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		item := manifest[idx]
		entry := map[string]any{}
		for _, key := range []string{"path", "kind", "size", "contentHash"} {
			if value, ok := item[key]; ok && value != nil {
				entry[key] = compactLLMContextValue(value, 1)
			}
		}
		if len(entry) > 0 {
			ret = append(ret, entry)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func promoteRepairActionPackagePrerequisiteRootCauseCandidates(pkg map[string]any) bool {
	if len(pkg) == 0 {
		return false
	}
	var prerequisiteCandidates []map[string]any
	for _, candidate := range mapSliceFromValue(pkg["rootCauseCandidateContext"]) {
		source := strings.ToLower(strings.TrimSpace(stringFromMap(candidate, "source")))
		if !strings.Contains(source, "prerequisite_manifest") {
			continue
		}
		prerequisiteCandidates = append(prerequisiteCandidates, candidate)
	}
	promoted := repairActionPromotableRootCauseCandidateTargetSnapshots(prerequisiteCandidates)
	if len(promoted) == 0 {
		return false
	}
	pkg["targetSnapshots"] = promoted
	pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(promoted)
	pkg["rootCauseCandidateTargetsPromoted"] = true
	requiredOutput := mapFromValue(pkg["requiredOutput"])
	if len(requiredOutput) == 0 {
		requiredOutput = stateChangingRepairRequiredOutput()
	}
	requiredOutput["mustTargetFailureScope"] = true
	requiredOutput["targetRelation"] = "grounded_prerequisite_root_cause_candidate_or_declared_related_failure_target"
	applyCurrentSnapshotDirectPatchContract(requiredOutput)
	requiredOutput["fileWriteExistingTargetRule"] = "only allowed when the target snapshot is complete/safe and patch/apply file_write includes baseHash equal to that snapshot contentHash; otherwise use patch/apply text_replace with exact oldText/newText/baseHash, bounded unified_diff, file/edit, or a declared auditable shell/tool state-changing operation"
	pkg["requiredOutput"] = requiredOutput
	targets := stringSliceFromValue(pkg["repairTargetSet"])
	for _, item := range promoted {
		if targetRef := normalizeRepairPatchTargetRef(stringFromMap(item, "targetRef")); targetRef != "" {
			targets = append(targets, targetRef)
		}
	}
	if targets = filterAdmissibleRuntimeRepairTargetRefs(compactUniqueStrings(targets)); len(targets) > 0 {
		pkg["repairTargetSet"] = targets
	}
	return true
}

func stateChangingReadOnlyGraphFailureText(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "state-changing") &&
		(strings.Contains(value, "read-only executiongraph") ||
			strings.Contains(value, "without patch/apply") ||
			strings.Contains(value, "inspection-only") ||
			strings.Contains(value, "llm-gated"))
}

func compactStateChangingRecoveryFailureOutput(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"stage",
		"status",
		"error",
		"reason",
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureCategory",
	} {
		if value, ok := output[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if diagnostics := mapFromValue(output["streamDiagnostics"]); len(diagnostics) > 0 {
		ret["streamDiagnostics"] = compactLLMContextMap(diagnostics, 0)
	}
	if llmDecisionInvalidOutput(output) {
		if _, ok := output["rawContentPreview"]; ok {
			ret["rawContentPreviewOmitted"] = true
		}
		if _, ok := output["repairRawContentPreview"]; ok {
			ret["repairRawContentPreviewOmitted"] = true
		}
		if len(mapSliceFromValue(output["streamEvents"])) > 0 {
			ret["streamEventsOmitted"] = true
		}
		if ret["rawContentPreviewOmitted"] == true ||
			ret["repairRawContentPreviewOmitted"] == true ||
			ret["streamEventsOmitted"] == true {
			ret["omittedContentReason"] = "invalid or rejected LLM decision content is not reusable repair input; rebuild from current context, target snapshots, or fresh reads"
		}
	} else if preview := streamEventsPreview(mapSliceFromValue(output["streamEvents"]), 1200); preview != "" {
		if containsRuntimeRedactionMarker(preview) {
			ret["partialOutputPreviewOmitted"] = true
			ret["omittedContentReason"] = "rejected or redacted runtime output is not reusable repair input; rebuild from current target snapshots or fresh reads"
		} else {
			ret["partialOutputPreview"] = compactLLMContextString(preview)
		}
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		ret["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	} else if graph, ok := runtimeGraphSpecFromValue(mapFromValue(output["llmDecision"])[stepOutputKeyExecutionGraph]); ok {
		ret["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	}
	return ret
}

func streamEventsPreview(events []map[string]any, limit int) string {
	if limit <= 0 || len(events) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, event := range events {
		delta := stringFromMap(event, "delta")
		if delta == "" {
			continue
		}
		remaining := limit - builder.Len()
		if remaining <= 0 {
			break
		}
		if len(delta) > remaining {
			builder.WriteString(delta[:remaining])
			break
		}
		builder.WriteString(delta)
	}
	return strings.TrimSpace(builder.String())
}

func rejectedGraphSummary(graph runtimemodel.BlockExecutionGraph) map[string]any {
	if len(graph.Nodes) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(graph.Nodes))
	nodeIDs := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if node.Kind != "" {
			kinds = append(kinds, node.Kind)
		}
		if node.ID != "" {
			nodeIDs = append(nodeIDs, node.ID)
		}
	}
	return map[string]any{
		"nodeCount": len(graph.Nodes),
		"nodeIds":   limitedStrings(compactUniqueStrings(nodeIDs), 12),
		"kinds":     limitedStrings(compactUniqueStrings(kinds), 12),
	}
}

func limitedStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

func repairObjectiveRequiresStateChange(value string) bool {
	switch strings.TrimSpace(value) {
	case repairObjectiveStateChangingOutput:
		return true
	default:
		return false
	}
}

func recoveryBudgetScope(lineageType string, rootStepID common.StepID, failureClass string, failureSignature string) map[string]any {
	ret := map[string]any{
		"lineageType": lineageType,
		"rootStepId":  string(rootStepID),
	}
	if failureClass = strings.TrimSpace(failureClass); failureClass != "" {
		ret["failureClass"] = failureClass
	}
	if failureSignature = strings.TrimSpace(failureSignature); failureSignature != "" {
		ret["failureSignature"] = failureSignature
	}
	return ret
}
