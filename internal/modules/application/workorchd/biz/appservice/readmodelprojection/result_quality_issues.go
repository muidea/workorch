package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func appendResultQualityIssue(quality *workorchprojectioncontract.ResultQualityTrace, code string, severity string, message string, suggestion string, refs ...string) {
	if quality == nil {
		return
	}
	issue := workorchprojectioncontract.ResultQualityIssue{
		Code:         code,
		Severity:     severity,
		Message:      message,
		Suggestion:   suggestion,
		EvidenceRefs: compactUniqueStrings(refs),
	}
	quality.Issues = append(quality.Issues, issue)
	if severity == "error" {
		quality.State = "failed"
	} else if quality.State != "failed" {
		quality.State = "warning"
	}
}
