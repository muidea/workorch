import type { RuntimeOriginalDiagnostic } from './types.ts';

// Read-only labels. Limits and owner causes are projected facts, never client policy.
export function originalDiagnosticLabel(value?: RuntimeOriginalDiagnostic): string {
  return [value?.ownerModuleId, value?.originalCode].filter(Boolean).join(' / ') || '-';
}

// Complete diagnostic text, not an acceptance/retry decision. Preserve source
// order and every check; compact summary tags are not this detailed report.
export function inputReviewFeedbackLines(value?: RuntimeOriginalDiagnostic): string[] {
  return (value?.validationInputReviews ?? []).flatMap(review => [
    `输入评审（不是验证执行或验收证据）: ${review.coverageWorkItemId} / ${review.protocolRef}`,
    `报告: ${review.reviewRef}`,
    `原准则: ${review.requirementHash}`,
    `输入版本: ${review.report.subjectHash}`,
    `报告哈希: ${review.reportHash}`,
    ...(review.redacted ? ['敏感诊断已脱敏；保留原报告引用和哈希。'] : []),
    ...review.report.checks.flatMap(check => [
      `原检查 ${check.requiredCheckRef}: ${check.conclusion} — ${check.reason}`,
      `输入: ${check.inputTargetRefs.join(', ')}`,
      ...(check.obligations ?? []).flatMap(obligation => [
        `原义务: ${obligation.criterionText}`,
        `输入覆盖 ${obligation.finding}: ${obligation.reason}`,
        `对应输入: ${obligation.inputTargetRefs.join(', ')}`,
      ]),
    ]),
  ]);
}

export function validationRepairPolicyLabel(value?: {
  validationRepairPolicyId?: string;
  validationRepairMaxAttempts?: number;
}): string {
  const id = value?.validationRepairPolicyId?.trim();
  const limit = value?.validationRepairMaxAttempts;
  return id && Number.isInteger(limit) && Number(limit) > 0 ? `${limit} (${id})` : '-';
}
