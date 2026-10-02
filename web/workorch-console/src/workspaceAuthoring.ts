import type { DefinitionAuthoringCreate, DefinitionAuthoringInput, DefinitionAuthoringView } from './types.ts';

export async function enterAuthoringWorkspace(workspaceId: string, refresh: () => Promise<void>, isCurrent: () => boolean, navigate: (workspaceId: string) => void): Promise<void> {
  if (!isCurrent()) return;
  await refresh();
  if (isCurrent()) navigate(workspaceId);
}

export function authoringSourceLabel(kind: string): string {
  return ({ USER_EXPLICIT: '用户要求', TEMPLATE_REQUIRED: '模板必需', DERIVED: '派生' } as Record<string, string>)[kind] || kind;
}

export type AuthoringSaveRequest = { requestId: string; input: DefinitionAuthoringInput; templateId: string; expectedWorkspaceRevision: number; expectedInputHash: string };
export type AuthoringConfirmation = { inputHash: string; derivationPermitId: string; proposalHash: string; specificationHash: string; expectedWorkspaceRevision: number };
export type AuthoringTransactionRecovery = { workspaceId?: string; pendingSave?: AuthoringSaveRequest; pendingConfirmation?: AuthoringConfirmation };
export type AuthoringRecovery = AuthoringTransactionRecovery & { request: DefinitionAuthoringCreate };
export type AuthoringEditorValues = { templateId: string; goalText: string; requirements?: string; resources?: string };
export type AuthoringEditorRecovery = AuthoringTransactionRecovery & { values?: AuthoringEditorValues };

// 每次打开与关闭均更换会话，旧响应不能修改新表单或恢复记录。
export class AuthoringSessionFence {
  private generation = 0;
  begin(): number { return ++this.generation; }
  current(): number { return this.generation; }
  matches(token: number): boolean { return token === this.generation; }
}

export function mergeAuthoringObservation(current: DefinitionAuthoringView | undefined, next: DefinitionAuthoringView): DefinitionAuthoringView {
  if (!current) return next;
  if (current.workspaceId !== next.workspaceId || next.workspaceRevision < current.workspaceRevision) return current;
  if (current.compilationAccepted && next.state === 'draft' && current.inputHash === next.inputHash) return { ...next, compilationAccepted: true };
  return next;
}

export class AuthoringQueryFence {
  private sequence = 0;
  private active: Promise<unknown> | undefined;
  async read<T>(load: () => Promise<T>, isCurrent: () => boolean = () => true): Promise<T> {
    // effect 重建也必须等待原物理查询结束，失效排队不得发送新 GET。
    while (this.active) {
      try { await this.active; } catch { /* 失败由发起查询的会话处理。 */ }
    }
    if (!isCurrent()) throw new Error('创作查询已失效，未发送请求');
    const active = load();
    this.active = active;
    try { return await active; }
    finally { if (this.active === active) this.active = undefined; }
  }
  invalidate(): void { this.sequence++; }
  begin(): number { return ++this.sequence; }
  matches(token: number): boolean { return token === this.sequence; }
}

export function authoringCanEnterAdmitted(view?: DefinitionAuthoringView, workspaceId?: string): boolean {
  return Boolean(view && workspaceId && view.workspaceId === workspaceId && view.state === 'admitted' && view.activationState === 'eligible' && view.startupInputHash);
}

const reviewKinds = new Set(['dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility']);

export function authoringCanConfirm(view?: DefinitionAuthoringView): boolean {
  return Boolean(view && view.state === 'ready_to_confirm' && view.templateCurrent && view.inputHash && view.derivationPermitId && view.proposal?.proposalHash && view.proposal.specificationHash && view.proposal.derivationPermitId === view.derivationPermitId && view.proposal.items.length > 0 && view.proposal.reviews?.length === 6 && view.proposal.reviews.every(review => review.status === 'PASS' && reviewKinds.has(review.kind) && review.specificationHash === view.proposal!.proposalHash) && new Set(view.proposal.reviews.map(review => review.kind)).size === 6);
}

export function authoringConfirmationRecovery(view?: DefinitionAuthoringView, workspaceId?: string): AuthoringConfirmation | undefined {
  const exact = view?.confirmationRecovery;
  if (!view || !workspaceId || view.workspaceId !== workspaceId || !exact || !['ready_to_confirm', 'confirming', 'admitting'].includes(view.state) || !view.templateCurrent || !exact.inputHash || !exact.derivationPermitId || !exact.proposalHash || !exact.specificationHash || !Number.isSafeInteger(exact.expectedWorkspaceRevision) || exact.expectedWorkspaceRevision <= 0 || (view.state === 'ready_to_confirm' && exact.expectedWorkspaceRevision !== view.workspaceRevision)) return undefined;
  if (exact.inputHash !== view.inputHash || exact.derivationPermitId !== view.derivationPermitId || exact.proposalHash !== view.proposal?.proposalHash || exact.specificationHash !== view.proposal?.specificationHash) return undefined;
  if (view.state === 'ready_to_confirm' && !authoringCanConfirm(view)) return undefined;
  return { ...exact };
}

export function authoringCanReplayConfirmation(view?: DefinitionAuthoringView, recovery?: AuthoringTransactionRecovery): boolean {
  const exact = recovery?.pendingConfirmation;
  return Boolean(view && exact && recovery?.workspaceId === view.workspaceId && ['ready_to_confirm', 'confirming', 'admitting'].includes(view.state) && view.templateCurrent && exact.inputHash === view.inputHash && exact.derivationPermitId === view.derivationPermitId && exact.proposalHash === view.proposal?.proposalHash && exact.specificationHash === view.proposal?.specificationHash && (view.state !== 'ready_to_confirm' || authoringCanConfirm(view)));
}

export function authoringRequireNoPendingConfirmation(recovery?: AuthoringTransactionRecovery): void {
  if (recovery?.pendingConfirmation) throw new Error('原确认请求尚未恢复；保留原身份，请先恢复确认，不能提交替代修订或重新审查');
}

export function authoringNeedsSave(view: DefinitionAuthoringView, input: DefinitionAuthoringInput, templateId: string): boolean {
  const normalized = (values?: string[]) => (values || []).map(value => value.trim());
  return !view.input || !view.templateCurrent || view.templateId !== templateId.trim() || view.input.goalText.trim() !== input.goalText.trim() || JSON.stringify(normalized(view.input.acceptanceRequirements)) !== JSON.stringify(normalized(input.acceptanceRequirements)) || JSON.stringify(normalized(view.input.resourceRefs)) !== JSON.stringify(normalized(input.resourceRefs));
}

export function authoringNeedsPolling(view?: DefinitionAuthoringView): boolean {
  return Boolean(view && (view.compilationAccepted || ['deriving', 'derived', 'reviewing', 'outcome_unresolved', 'confirming', 'admitting'].includes(view.state)));
}
