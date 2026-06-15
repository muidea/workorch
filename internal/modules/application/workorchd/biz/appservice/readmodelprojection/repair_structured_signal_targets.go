package readmodelprojection

import "strings"

func readModelFallbackFailureTargetRefs(output map[string]any, failureSignature string) []string {
	targetRefs := []string{}
	addTarget := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			targetRefs = append(targetRefs, value)
		}
	}
	addTarget(readModelStringValue(output["targetRef"]))
	addTarget(readModelStringValue(output["path"]))
	for _, scope := range []string{
		readModelStringValue(output["failureTargetScope"]),
		readModelStringValue(output["targetScope"]),
	} {
		for _, target := range readModelTargetsFromFailureSignature(scope) {
			addTarget(target)
		}
	}
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	for _, key := range []string{"targetRef", "conflictRef"} {
		addTarget(readModelStringValue(diagnostics[key]))
	}
	progress := readModelMapValue(output["currentExecutionProgress"])
	lastFailure := readModelMapValue(progress["lastObservedFailure"])
	for _, source := range []map[string]any{progress, lastFailure} {
		addTarget(readModelStringValue(source["targetRef"]))
		for _, scope := range []string{
			readModelStringValue(source["failureTargetScope"]),
			readModelStringValue(source["targetScope"]),
		} {
			for _, target := range readModelTargetsFromFailureSignature(scope) {
				addTarget(target)
			}
		}
	}
	for _, signature := range []string{failureSignature, readModelStringValue(output["failureSignature"])} {
		for _, target := range readModelTargetsFromFailureSignature(signature) {
			addTarget(target)
		}
	}
	return targetRefs
}
