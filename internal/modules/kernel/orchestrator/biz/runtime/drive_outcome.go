package runtime

import (
	"fmt"
	"strings"
)

// OperationOutcomeUnknownError is an owner-local SM-4 outcome. It is returned
// only after the execution fence has been persisted as outcome_unknown, so the
// drive classifier never needs to infer recovery authority from error text.
type OperationOutcomeUnknownError struct {
	OperationID string
	Reason      string
}

func (e *OperationOutcomeUnknownError) Error() string {
	if e == nil {
		return "runtime operation outcome is unknown"
	}
	operationID := strings.TrimSpace(e.OperationID)
	reason := strings.TrimSpace(e.Reason)
	if operationID == "" {
		return "runtime operation outcome is unknown: " + reason
	}
	if reason == "" {
		return fmt.Sprintf("operation %q outcome is unknown", operationID)
	}
	return fmt.Sprintf("operation %q outcome is unknown: %s", operationID, reason)
}

func operationOutcomeUnknown(operationID, reason string) error {
	return &OperationOutcomeUnknownError{OperationID: strings.TrimSpace(operationID), Reason: strings.TrimSpace(reason)}
}
