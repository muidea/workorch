package readmodelprojection

import "strings"

func readModelFallbackFailureSourceMessages(output map[string]any, failureClass string, failureSignature string) ([]string, []string) {
	sources := []string{}
	messages := []string{}
	addSourceMessage := func(source string, value string) {
		value = strings.TrimSpace(value)
		if source == "" || value == "" {
			return
		}
		sources = append(sources, source)
		messages = append(messages, readModelTruncateString(value, 512))
	}
	addSourceMessage("failureClass", readModelFirstNonEmptyString(failureClass, readModelStringValue(output["failureClass"])))
	addSourceMessage("failureSignature", readModelFirstNonEmptyString(failureSignature, readModelStringValue(output["failureSignature"])))
	for _, key := range []string{"error", "reason", "stderr", "stdout", "stage"} {
		addSourceMessage(key, readModelStringValue(output[key]))
	}
	return sources, messages
}
