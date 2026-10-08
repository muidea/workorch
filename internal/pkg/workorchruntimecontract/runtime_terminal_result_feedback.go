package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

// runtimeCompleteResultFeedback 仅描述已裁决终态事实，不重新验收、不选择后继或重试。
// 未评估项不能描述为验证失败；owner 原始诊断及完整输入评审保持原文。
func runtimeCompleteResultFeedback(item *RuntimeTerminalResultMaterial, source RuntimeStateRecord) {
	closeout := item.RuntimeCloseout
	if closeout == nil {
		return
	}
	var assessmentItems []workorchresultcontract.DoDAssessmentItem
	for _, outcome := range closeout.RequirementOutcomes {
		for _, coverage := range source.Facts.Coverage {
			if coverage.ID != outcome.CoverageWorkItemID || (coverage.SourceScope != "" && coverage.SourceScope != "dodItem") {
				continue
			}
			text := coverage.TargetText
			if text == "" {
				text = outcome.RequirementRef
			}
			assessment := workorchresultcontract.DoDAssessmentItem{
				Item: text, State: string(outcome.AssessmentState), SourceNote: outcome.RequirementRef,
				EvidenceRefs: append([]string(nil), outcome.AcceptedEvidenceRefs...), FailureReason: outcome.AssessmentReasonCode,
			}
			if outcome.AssessmentState == RuntimeRequirementUnassessed {
				assessment.FailureReason = "此要求尚未记录验收评估。"
			}
			assessmentItems = append(assessmentItems, assessment)
		}
	}
	if len(assessmentItems) != 0 {
		item.DoDAssessment.Items = assessmentItems
	}
	item.SummaryDetail.DoDItems = item.DoDAssessment.Clone().Items
	acceptedChanges, acceptedValidation, rejectedValidation := 0, 0, 0
	for _, fact := range source.Facts.Evidence.Accepted {
		switch fact.Kind {
		case EvidenceKindStateChange:
			acceptedChanges++
		case EvidenceKindValidation:
			acceptedValidation++
		}
	}
	for _, fact := range source.Facts.Evidence.Rejected {
		if fact.Kind == EvidenceKindValidation {
			rejectedValidation++
		}
	}
	item.SummaryDetail.ImplementationSummary = fmt.Sprintf("已接受状态变更证据：%d；owner 操作回执：%d。", acceptedChanges, len(source.OperationReceipts))
	item.SummaryDetail.VerificationSummary = fmt.Sprintf("已接受验证证据：%d；已拒绝验证证据：%d。", acceptedValidation, rejectedValidation)
	if acceptedValidation+rejectedValidation == 0 {
		item.SummaryDetail.VerificationSummary = "尚未记录验证证据；无法确认验证是否执行或成功。"
	}
	if !item.DoDAssessment.Satisfied {
		item.SummaryDetail.KnownLimitations = []string{"Run 已结束，但未完成全部 Goal/DoD 验收。"}
		item.SummaryDetail.ResidualRisks = []string{fmt.Sprintf("仍有 %d 项覆盖工作未满足；部分输出不得视为已验收交付。", len(closeout.UnsatisfiedCoverageWorkItemIDs))}
		item.SummaryDetail.NextActions = []string{"检查终态及 owner 原始诊断，解决报告中的阻塞；如需继续，请显式请求新的执行。"}
		if closeout.OriginalDiagnostic != nil {
			for _, review := range closeout.OriginalDiagnostic.ValidationInputReviews {
				for _, check := range review.Report.Checks {
					item.SummaryDetail.KnownLimitations = append(item.SummaryDetail.KnownLimitations,
						fmt.Sprintf("仅为输入评审；CWI %s；原检查 %s；%s：%s 输入：%s。报告：%s。", review.CoverageWorkItemID, check.RequiredCheckRef, check.Conclusion, check.Reason, strings.Join(check.InputTargetRefs, ", "), review.ReviewRef))
					for _, obligation := range check.Obligations {
						item.SummaryDetail.KnownLimitations = append(item.SummaryDetail.KnownLimitations,
							fmt.Sprintf("原验收义务 %q；输入覆盖 %s：%s", obligation.CriterionText, obligation.Finding, obligation.Reason))
					}
				}
			}
		}
	}
}
