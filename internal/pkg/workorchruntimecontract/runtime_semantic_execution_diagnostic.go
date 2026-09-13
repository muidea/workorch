package workorchruntimecontract

import "fmt"

// RuntimeSemanticExecutionDiagnostic is a provider-safe observation of a
// committed owner's diagnostic. It never supplies a repair target or policy.
// The exact feedback hash binds omitted material without exposing its contents
// or suggesting that the diagnostic is complete or independently verified.
type RuntimeSemanticExecutionDiagnostic struct {
	FeedbackHash         string                               `json:"feedbackHash"`
	OwnerModuleID        string                               `json:"ownerModuleId"`
	OriginalClass        string                               `json:"originalClass"`
	OriginalCode         string                               `json:"originalCode"`
	Signature            string                               `json:"signature"`
	ExpectedRevision     string                               `json:"expectedRevision,omitempty"`
	ObservedRevision     string                               `json:"observedRevision,omitempty"`
	SummaryOnly          bool                                 `json:"summaryOnly"`
	Truncated            bool                                 `json:"truncated"`
	Redacted             bool                                 `json:"redacted"`
	ValidationDiagnostic *RuntimeValidationDiagnosticFeedback `json:"validationDiagnostic"`
}

// RuntimeValidationDiagnosticFeedback contains typed owner-reported diagnostic
// fields. Neither a default exit code nor command kind proves a process ran;
// actual check execution remains a separate validation observation.
type RuntimeValidationDiagnosticFeedback struct {
	Class            string `json:"class"`
	OwnerCode        string `json:"ownerCode"`
	CommandKind      string `json:"commandKind"`
	ReportedExitCode int    `json:"reportedExitCode"`
	TimedOut         bool   `json:"timedOut"`
}

func (d *RuntimeSemanticExecutionDiagnostic) Clone() *RuntimeSemanticExecutionDiagnostic {
	if d == nil {
		return nil
	}
	ret := *d
	if d.ValidationDiagnostic != nil {
		value := *d.ValidationDiagnostic
		ret.ValidationDiagnostic = &value
	}
	return &ret
}

func (d RuntimeSemanticExecutionDiagnostic) Validate(owner string) error {
	if d.FeedbackHash == "" || d.OwnerModuleID != owner || owner == "" || d.OriginalClass == "" || d.OriginalCode == "" || d.Signature == "" || !d.SummaryOnly {
		return fmt.Errorf("semantic execution diagnostic requires exact owner, feedback identity and summary-only marker")
	}
	values := []string{d.FeedbackHash, d.OwnerModuleID, d.OriginalClass, d.OriginalCode, d.Signature, d.ExpectedRevision, d.ObservedRevision}
	if d.ValidationDiagnostic != nil {
		values = append(values, d.ValidationDiagnostic.Class, d.ValidationDiagnostic.OwnerCode, d.ValidationDiagnostic.CommandKind)
	}
	for _, value := range values {
		if err := validateRuntimeFeedbackSafeText("semantic execution diagnostic", value); err != nil {
			return err
		}
	}
	return nil
}

// ProjectRuntimeSemanticExecutionDiagnostic selects facts, not a next action.
// Raw message, reason, previews, field values, input-maintenance authorization,
// governance and owner-private detail references deliberately do not cross.
func ProjectRuntimeSemanticExecutionDiagnostic(feedback *RuntimeFeedback, owner string) (*RuntimeSemanticExecutionDiagnostic, error) {
	if feedback == nil {
		return nil, nil
	}
	if err := feedback.Validate(); err != nil {
		return nil, err
	}
	if feedback.OriginalDiagnostic == nil {
		return nil, nil
	}
	d := feedback.OriginalDiagnostic
	ret := &RuntimeSemanticExecutionDiagnostic{
		FeedbackHash: StableRuntimeFeedbackHash(*feedback), OwnerModuleID: d.OwnerModuleID,
		OriginalClass: d.OriginalClass, OriginalCode: d.OriginalCode, Signature: d.Signature,
		ExpectedRevision: d.ExpectedRevision, ObservedRevision: d.ObservedRevision,
		SummaryOnly: true,
		Truncated:   d.Truncated, Redacted: d.Redacted,
	}
	if d.ValidationDiagnostic != nil {
		v := d.ValidationDiagnostic
		ret.ValidationDiagnostic = &RuntimeValidationDiagnosticFeedback{Class: v.Class, OwnerCode: v.OwnerCode, CommandKind: v.CommandKind, ReportedExitCode: v.ExitCode, TimedOut: v.TimedOut}
	}
	return ret, ret.Validate(owner)
}
