package workorchruntimecontract

const RuntimeValidationAssertionMaxBytes = 16 * 1024
const RuntimeValidationAssertionsMaxBytes = 128 * 1024

// ProjectRuntimeValidationExecutionFeedback selects the observed failing
// assertions, not the unrelated transcript. Native bodies remain byte-exact.
func ProjectRuntimeValidationExecutionFeedback(value *RuntimeValidationObservation) *RuntimeValidationExecutionFeedback {
	if value == nil {
		return nil
	}
	o := value.Normalized()
	if len(o.FailedChecks) != 0 {
		// Failure feedback needs every failed case, not the unrelated passing
		// suite. Preserve conflicting dispositions for those same checks.
		relevant := func(values []string) []string {
			var ret []string
			for _, c := range values {
				if containsString(o.FailedChecks, c) {
					ret = append(ret, c)
				}
			}
			return ret
		}
		o.ExecutedChecks, o.PassedChecks = relevant(o.ExecutedChecks), relevant(o.PassedChecks)
	}
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
	// A location/hash or a closed shape cannot replace the native body.
	for _, detail := range ret.CheckFailures {
		if detail.AssertionText == "" {
			ret.FailureDetailsIncomplete = true
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
