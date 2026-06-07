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
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	for _, key := range []string{"targetRef", "conflictRef"} {
		addTarget(readModelStringValue(diagnostics[key]))
	}
	for _, signature := range []string{failureSignature, readModelStringValue(output["failureSignature"])} {
		for _, target := range readModelTargetsFromFailureSignature(signature) {
			addTarget(target)
		}
	}
	return targetRefs
}
