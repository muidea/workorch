package readmodelprojection

func readModelFallbackStructuredFailureSignals(output map[string]any, failureClass string, failureSignature string) map[string]any {
	if output == nil {
		return nil
	}
	sources, messages := readModelFallbackFailureSourceMessages(output, failureClass, failureSignature)
	targetRefs := readModelFallbackFailureTargetRefs(output, failureSignature)
	return readModelStructuredFailureSignalMap(sources, targetRefs, messages)
}
