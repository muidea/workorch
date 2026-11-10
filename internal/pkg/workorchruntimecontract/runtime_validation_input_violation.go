package workorchruntimecontract

import (
	"errors"
	"fmt"
)

// A pure review constraint carries provider-safe rules and exact relative wire
// paths. The original error remains diagnostic data, never correction authority.
type runtimeValidationInputViolation struct {
	Path     string
	Actual   string
	Required []string
	Allowed  []string
	Cause    error
}

func (e *runtimeValidationInputViolation) Error() string { return e.Cause.Error() }
func (e *runtimeValidationInputViolation) Unwrap() error { return e.Cause }

func runtimeInputViolation(path, actual, required, message string) error {
	return &runtimeValidationInputViolation{Path: path, Actual: actual, Required: []string{required}, Cause: errors.New(message)}
}

func runtimeInputViolationAt(path string, err error) error {
	if joined, ok := err.(*runtimeInputViolations); ok {
		items := make([]error, 0, len(joined.Items))
		for _, item := range joined.Items {
			items = append(items, runtimeInputViolationAt(path, item))
		}
		return joinRuntimeInputViolations(items)
	}
	var violation *runtimeValidationInputViolation
	if !errors.As(err, &violation) {
		return err // Owner-subject defects are not provider-editable rules.
	}
	copy := *violation
	copy.Path = path + copy.Path
	copy.Cause = err
	return &copy
}

func runtimeSemanticInputReviewViolation(proposal RuntimeSemanticProofProposal, index int, err error) error {
	if joined, ok := err.(*runtimeInputViolations); ok {
		items := make([]error, 0, len(joined.Items))
		for _, item := range joined.Items {
			items = append(items, runtimeSemanticInputReviewViolation(proposal, index, item))
		}
		return errors.Join(items...)
	}
	var violation *runtimeValidationInputViolation
	if errors.As(err, &violation) {
		return runtimeSemanticProposalViolation(proposal, index, "validationInputReview"+violation.Path,
			RuntimeSemanticConstraintRelational, violation.Actual, violation.Required, violation.Allowed, err)
	}
	// Owner-authored subject validation is not repaired by changing a report.
	return fmt.Errorf("runtime input review owner binding: %w", err)
}

// Complete decodable independent wire constraints are reported together within
// the existing correction bound. Oversized feedback fails explicitly rather
// than silently claiming to have returned the complete set.
type runtimeInputViolations struct{ Items []error }

func (e *runtimeInputViolations) Error() string   { return errors.Join(e.Items...).Error() }
func (e *runtimeInputViolations) Unwrap() []error { return e.Items }
func joinRuntimeInputViolations(items []error) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return items[0]
	}
	return &runtimeInputViolations{Items: items}
}
