package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeValidationInputReviewFeedback is a detached, audit-safe projection of
// an admitted report. No input bytes, execution grants or replacement criteria
// are included. ReviewRef addresses the original SM-3 document; ReportHash
// commits its exact report even when sensitive diagnostic text is redacted.
type RuntimeValidationInputReviewFeedback struct {
	ReviewRef          string                       `json:"reviewRef"`
	ProtocolRef        string                       `json:"protocolRef"`
	CoverageWorkItemID common.CoverageWorkItemID    `json:"coverageWorkItemId"`
	RequirementHash    string                       `json:"requirementHash"`
	ReportHash         string                       `json:"reportHash"`
	Report             RuntimeValidationInputReview `json:"report"`
	Redacted           bool                         `json:"redacted,omitempty"`
}

func StableRuntimeValidationInputReviewHash(report RuntimeValidationInputReview) string {
	h := sha256.New()
	writeRuntimeValidationInputReviewHash(h, &report)
	return "runtime-input-review-report-" + hex.EncodeToString(h.Sum(nil))
}

func (f RuntimeValidationInputReviewFeedback) Clone() RuntimeValidationInputReviewFeedback {
	f.Report = f.Report.Clone()
	return f
}

// ProjectRuntimeValidationInputReviewFeedback performs no I/O and makes no
// consistency judgment. The owner must first admit the exact provider report.
func ProjectRuntimeValidationInputReviewFeedback(ref string, binding RuntimeValidationProofRequirementBinding, report RuntimeValidationInputReview) (RuntimeValidationInputReviewFeedback, error) {
	if err := report.ValidateAgainstBinding(binding); err != nil {
		return RuntimeValidationInputReviewFeedback{}, err
	}
	f := RuntimeValidationInputReviewFeedback{ReviewRef: ref, ProtocolRef: binding.InputReview.ProtocolRef,
		CoverageWorkItemID: binding.CoverageWorkItemID, RequirementHash: binding.RequirementHash,
		ReportHash: StableRuntimeValidationInputReviewHash(report), Report: report.Clone()}
	safe := func(value string) string {
		if validateRuntimeInputReviewFeedbackText(value) == nil {
			return value
		}
		f.Redacted = true
		return "[redacted diagnostic text; sha256:" + RuntimeValidationInputContentHash(value) + "]"
	}
	for i := range f.Report.Checks {
		check := &f.Report.Checks[i]
		check.Reason = safe(check.Reason)
		check.RequiredCheckRef = safe(check.RequiredCheckRef)
		for j := range check.InputTargetRefs {
			check.InputTargetRefs[j] = safe(check.InputTargetRefs[j])
		}
		for j := range check.Obligations {
			obligation := &check.Obligations[j]
			obligation.CriterionText = safe(obligation.CriterionText)
			obligation.Reason = safe(obligation.Reason)
			for k := range obligation.InputTargetRefs {
				obligation.InputTargetRefs[k] = safe(obligation.InputTargetRefs[k])
			}
		}
	}
	return f, f.Validate()
}

func (f RuntimeValidationInputReviewFeedback) Validate() error {
	if !strings.HasPrefix(f.ReviewRef, "runtime-input-review-") || f.CoverageWorkItemID == "" || f.RequirementHash == "" ||
		!strings.HasPrefix(f.ReportHash, "runtime-input-review-report-") || !RuntimeValidationInputReviewProtocolSupported(f.ProtocolRef) ||
		f.Report.SubjectHash == "" || len(f.Report.Checks) == 0 {
		return fmt.Errorf("input review feedback requires exact admitted report and requirement identity")
	}
	if !f.Redacted && f.ReportHash != StableRuntimeValidationInputReviewHash(f.Report) {
		return fmt.Errorf("input review feedback lost exact original report content")
	}
	for _, check := range f.Report.Checks {
		if f.ProtocolRef == RuntimeValidationInputReviewProtocol && check.Obligations != nil {
			return fmt.Errorf("legacy input review feedback cannot introduce v2 observations")
		}
		if check.RequiredCheckRef == "" || check.Reason == "" || len(check.InputTargetRefs) == 0 {
			return fmt.Errorf("input review feedback lost a per-check observation")
		}
		switch check.Conclusion {
		case RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive:
		default:
			return fmt.Errorf("input review feedback has an unsupported conclusion")
		}
		texts := append([]string{check.RequiredCheckRef, check.Reason}, check.InputTargetRefs...)
		for _, obligation := range check.Obligations {
			texts = append(texts, obligation.CriterionText, obligation.Reason)
			texts = append(texts, obligation.InputTargetRefs...)
		}
		for _, value := range texts {
			if value == "" || !utf8.ValidString(value) || len(value) > RuntimeValidationInputReviewMaxScopeBytes {
				return fmt.Errorf("input review feedback requires bounded complete text")
			}
			if err := validateRuntimeInputReviewFeedbackText(value); err != nil {
				return err
			}
		}
		if f.ProtocolRef == RuntimeValidationInputReviewProtocolV2 {
			if err := validateRuntimeValidationInputObligationShape(check, !f.Redacted); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRuntimeInputReviewFeedbackText(value string) error {
	if strings.IndexFunc(value, func(r rune) bool { return (r < 32 && r != '\n' && r != '\t') || r == 127 }) >= 0 {
		return fmt.Errorf("input review feedback contains unsafe control characters")
	}
	return validateRuntimeFeedbackSafeText("input review", value)
}

func writeRuntimeValidationInputReviewFeedbackHash(h interface{ Write([]byte) (int, error) }, values []RuntimeValidationInputReviewFeedback) {
	if len(values) == 0 {
		return
	}
	writeHashString(h, "runtime-input-review-feedback/v1")
	for _, value := range values {
		writeHashString(h, value.ReviewRef)
		writeHashString(h, value.ProtocolRef)
		writeHashString(h, string(value.CoverageWorkItemID))
		writeHashString(h, value.RequirementHash)
		writeHashString(h, value.ReportHash)
		writeRuntimeValidationInputReviewHash(h, &value.Report)
		writeHashString(h, fmt.Sprint(value.Redacted))
	}
}
