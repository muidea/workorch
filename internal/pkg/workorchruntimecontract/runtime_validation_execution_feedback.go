package workorchruntimecontract

// ProjectRuntimeValidationExecutionFeedback drops raw text and projects only
// the bounded diagnostic protocol shared by semantic and operation requests.
func ProjectRuntimeValidationExecutionFeedback(value *RuntimeValidationObservation) *RuntimeValidationExecutionFeedback {
	if value == nil {
		return nil
	}
	o := value.Normalized()
	ret := &RuntimeValidationExecutionFeedback{RecipeID: o.RecipeID, ObservationProtocolRef: o.ObservationProtocolRef,
		FailureDetailsIncomplete: o.FailureDetailsTruncated || o.OutputTruncated, ChecksTruncated: o.OutputTruncated}
	bounded := func(values []string) []string {
		if len(values) > RuntimeValidationFailureDetailLimit {
			ret.ChecksTruncated = true
			values = values[:RuntimeValidationFailureDetailLimit]
		}
		return append([]string(nil), values...)
	}
	ret.ExecutedChecks, ret.PassedChecks = bounded(o.ExecutedChecks), bounded(o.PassedChecks)
	ret.FailedChecks, ret.SkippedChecks = bounded(o.FailedChecks), bounded(o.SkippedChecks)
	ret.IncompleteChecks = bounded(o.IncompleteChecks)
	if o.ValidateFailureDetails() == nil {
		ret.FailureDetailProtocolRef = o.FailureDetailProtocolRef
		for _, detail := range o.CheckFailures {
			if !containsString(ret.FailedChecks, detail.CheckRef) {
				// A bounded inventory cannot retain an orphan detail whose
				// failed check was omitted. Record that loss explicitly.
				ret.FailureDetailsIncomplete = true
				continue
			}
			ret.CheckFailures = append(ret.CheckFailures, detail)
		}
	} else {
		ret.FailureDetailsIncomplete = true
	}
	// The explicit diagnostic extension distinguishes traceable locations from
	// actionable reported shapes; neither a line nor a hash supplies an assertion.
	if o.FailureDetailProtocolRef == RuntimeValidationFailureDetailProtocol {
		for _, detail := range ret.CheckFailures {
			if detail.ReportedAssertion == (RuntimeValidationReportedAssertion{}) {
				ret.FailureDetailsIncomplete = true
			}
		}
	}
	for _, check := range o.FailedChecks {
		found := false
		for _, detail := range ret.CheckFailures {
			if detail.CheckRef == check {
				found = true
				break
			}
		}
		if !found {
			ret.FailureDetailsIncomplete = true
		}
	}
	return ret
}
