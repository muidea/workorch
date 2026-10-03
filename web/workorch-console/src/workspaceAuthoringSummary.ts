import type { WorkspaceAuthoringSummary } from './types.ts';

export type AuthoringSummaryPresentation = {
  label: string;
  color?: string;
  type: 'info' | 'warning' | 'error';
  failurePhase?: string;
  failureCode?: string;
  reviewCount?: number;
};

const phaseLabels: Record<string, string> = {
  derivation: '派生',
  proposal: '方案生成',
  review: '审查',
};

const failureCodeLabels: Record<string, string> = {
  invalid_proposal: '方案无效',
};

const knownStates: Record<string, Pick<AuthoringSummaryPresentation, 'label' | 'color' | 'type'>> = {
  draft: { label: '草稿', color: 'default', type: 'info' },
  deriving: { label: '正在派生', color: 'processing', type: 'info' },
  derived: { label: '已派生', color: 'blue', type: 'info' },
  reviewing: { label: '审查中', color: 'processing', type: 'info' },
  reviewed: { label: '审查已完成，确认资格待核对', color: 'blue', type: 'info' },
  ready_to_confirm: { label: '待确认', color: 'blue', type: 'info' },
  confirming: { label: '确认处理中', color: 'processing', type: 'info' },
  admitting: { label: '准入处理中', color: 'processing', type: 'info' },
  admitted: { label: '已准入', color: 'green', type: 'info' },
  outcome_unresolved: { label: '请求结果未决', color: 'orange', type: 'warning' },
  preparation_failed: { label: '创作准备失败', color: 'red', type: 'error' },
  failed: { label: '创作失败', color: 'red', type: 'error' },
  review_blocked: { label: '审查受阻', color: 'orange', type: 'warning' },
  clarification_required: { label: '需要澄清', color: 'orange', type: 'warning' },
};

export function authoringSummaryPresentation(summary?: WorkspaceAuthoringSummary): AuthoringSummaryPresentation | undefined {
  if (!summary || typeof summary.state !== 'string' || !summary.state.trim()) return undefined;
  const state = knownStates[summary.state];
  const reviewCount = Number.isSafeInteger(summary.reviewCount) && summary.reviewCount >= 0
    ? summary.reviewCount
    : undefined;
  const phase = summary.failurePhase?.trim();
  return {
    ...(state || { label: `未知状态（${summary.state}）`, color: 'default', type: 'warning' as const }),
    ...(phase ? { failurePhase: phaseLabels[phase] || phase } : {}),
    ...(summary.failureCode?.trim() ? { failureCode: failureCodeLabels[summary.failureCode.trim()] || summary.failureCode.trim() } : {}),
    ...(reviewCount !== undefined ? { reviewCount } : {}),
  };
}
