package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeValidationInputReviewFeedback is a detached, content-preserving projection of
// an admitted report. No input bytes, execution grants or replacement criteria
// are included. ReviewRef addresses the original SM-3 document; ReportHash
// commits its exact original report. Text is not rewritten for display.
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
	if len(f.Report.DiagnosticSources) > RuntimeValidationDiagnosticSourceLimit || f.ProtocolRef != RuntimeValidationInputReviewProtocolV3 && f.Report.DiagnosticSources != nil {
		return fmt.Errorf("invalid diagnostic source feedback extension")
	}
	seenSources := map[string]bool{}
	for _, source := range f.Report.DiagnosticSources {
		if source.SourceRef == "" || seenSources[source.SourceRef] || source.Reason == "" || len(source.Reason) > RuntimeValidationInputReviewMaxReasonBytes || len(source.InputTargetRefs) == 0 {
			return fmt.Errorf("diagnostic source feedback lost independent finding")
		}
		seenSources[source.SourceRef] = true
		switch source.Conclusion {
		case RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive:
		default:
			return fmt.Errorf("invalid diagnostic source feedback conclusion")
		}
		for _, value := range append([]string{source.SourceRef, source.Reason}, source.InputTargetRefs...) {
			if value == "" || !utf8.ValidString(value) || len(value) > RuntimeValidationInputReviewMaxScopeBytes {
				return fmt.Errorf("diagnostic source feedback text invalid")
			}
		}
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
			texts = append(texts, obligation.ProofConsumerRefs...)
			if f.ProtocolRef != RuntimeValidationInputReviewProtocolV3 && obligation.ProofConsumerRefs != nil {
				return fmt.Errorf("legacy feedback cannot introduce composite proof mappings")
			}
			if f.ProtocolRef == RuntimeValidationInputReviewProtocolV3 && len(obligation.ProofConsumerRefs) == 0 {
				return fmt.Errorf("composite feedback lost proof consumer mapping")
			}
		}
		for _, value := range texts {
			if value == "" || !utf8.ValidString(value) || len(value) > RuntimeValidationInputReviewMaxScopeBytes {
				return fmt.Errorf("input review feedback requires bounded complete text")
			}
		}
		if f.ProtocolRef != RuntimeValidationInputReviewProtocol {
			if err := validateRuntimeValidationInputObligationShape(check, !f.Redacted); err != nil {
				return err
			}
		}
	}
	return nil
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
