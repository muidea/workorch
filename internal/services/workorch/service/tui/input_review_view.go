package tui

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The inspect view is deliberately complete; compact progress remains a
// summary. Neither renderer selects retry, publication, governance or credit.
func writeInputReviewFeedback(b *strings.Builder, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) {
	if diagnostic == nil {
		return
	}
	for _, review := range diagnostic.ValidationInputReviews {
		fmt.Fprintf(b, "- input review (diagnostic only, not validation evidence): CWI=%s protocol=%s\n  report=%s requirement=%s input=%s reportHash=%s\n", review.CoverageWorkItemID, review.ProtocolRef, review.ReviewRef, review.RequirementHash, review.Report.SubjectHash, review.ReportHash)
		if review.Redacted {
			b.WriteString("  sensitive diagnostic text redacted; original report reference/hash retained\n")
		}
		for _, check := range review.Report.Checks {
			fmt.Fprintf(b, "  original check=%s conclusion=%s reason=%s inputs=%s\n", check.RequiredCheckRef, check.Conclusion, check.Reason, strings.Join(check.InputTargetRefs, ", "))
			for _, obligation := range check.Obligations {
				fmt.Fprintf(b, "    original obligation=%q input coverage=%s reason=%s inputs=%s\n", obligation.CriterionText, obligation.Finding, obligation.Reason, strings.Join(obligation.InputTargetRefs, ", "))
			}
		}
	}
}
