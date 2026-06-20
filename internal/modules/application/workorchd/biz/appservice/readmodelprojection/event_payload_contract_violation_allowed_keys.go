package readmodelprojection

var eventPayloadContractViolationAllowedKeys = map[string]struct{}{
	"attempt":              {},
	"failureSignature":     {},
	"maxAttempts":          {},
	"recoverability":       {},
	"rejectedOutputRef":    {},
	"retryHint":            {},
	"retryStepId":          {},
	"signatureRepeatCount": {},
	"sourceStepId":         {},
	"ticketId":             {},
	"violatedRule":         {},
}
