import type { DefinitionAuthoringCreate, DefinitionAuthoringInput, DefinitionAuthoringView } from './types.ts';

export async function enterAuthoringWorkspace(workspaceId: string, refresh: () => Promise<void>, isCurrent: () => boolean, navigate: (workspaceId: string) => void): Promise<void> {
  if (!isCurrent()) return;
  await refresh();
  if (isCurrent()) navigate(workspaceId);
}

export function authoringSourceLabel(kind: string): string {
  return ({ USER_EXPLICIT: '用户要求', TEMPLATE_REQUIRED: '模板必需', DERIVED: '派生' } as Record<string, string>)[kind] || kind;
}

export const authoringRecoveryKey = 'workorch.workspace-authoring.v1';
export const authoringLocalDraftKey = 'workorch.workspace-authoring.local-draft.v1';
export type AuthoringLocalDraft = { title: string; templateId: string; deliverableDirectory?: string; goalText: string; requirements?: string; resources?: string };
export type AuthoringSaveRequest = { requestId: string; input: DefinitionAuthoringInput; templateId: string; expectedWorkspaceRevision: number; expectedInputHash: string };
export type AuthoringConfirmation = { inputHash: string; derivationPermitId: string; proposalHash: string; specificationHash: string; expectedWorkspaceRevision: number };
export type AuthoringTransactionRecovery = { workspaceId?: string; pendingSave?: AuthoringSaveRequest; pendingConfirmation?: AuthoringConfirmation };
export type AuthoringRecovery = AuthoringTransactionRecovery & { request: DefinitionAuthoringCreate };
export type AuthoringEditorValues = { templateId: string; goalText: string; requirements?: string; resources?: string };
export type AuthoringEditorRecovery = AuthoringTransactionRecovery & { values?: AuthoringEditorValues };

export function parseAuthoringLocalDraft(text: string | null): AuthoringLocalDraft | undefined {
  if (!text) return undefined;
  try {
    const value = JSON.parse(text) as AuthoringLocalDraft;
    if (!value || typeof value !== 'object' || ['title', 'templateId', 'goalText'].some(key => typeof value[key as keyof AuthoringLocalDraft] !== 'string') || ['deliverableDirectory', 'requirements', 'resources'].some(key => value[key as keyof AuthoringLocalDraft] !== undefined && typeof value[key as keyof AuthoringLocalDraft] !== 'string')) return undefined;
    return value;
  } catch { return undefined; }
}

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

function validAuthoringTransaction(value: AuthoringTransactionRecovery): boolean {
  if ((value.workspaceId !== undefined && (typeof value.workspaceId !== 'string' || !value.workspaceId)) || (value.pendingSave !== undefined && (!value.pendingSave || typeof value.pendingSave !== 'object' || Array.isArray(value.pendingSave))) || (value.pendingConfirmation !== undefined && (!value.pendingConfirmation || typeof value.pendingConfirmation !== 'object' || Array.isArray(value.pendingConfirmation)))) return false;
  if (value.pendingSave && (typeof value.pendingSave.requestId !== 'string' || !value.pendingSave.requestId || !value.workspaceId || value.pendingConfirmation || typeof value.pendingSave.templateId !== 'string' || !value.pendingSave.templateId || typeof value.pendingSave.input?.goalText !== 'string' || !value.pendingSave.input.goalText || typeof value.pendingSave.expectedInputHash !== 'string' || !value.pendingSave.expectedInputHash || !Number.isSafeInteger(value.pendingSave.expectedWorkspaceRevision) || value.pendingSave.expectedWorkspaceRevision < 1 || [value.pendingSave.input.acceptanceRequirements, value.pendingSave.input.resourceRefs].some(items => items !== undefined && (!Array.isArray(items) || items.some(item => typeof item !== 'string'))))) return false;
  if (value.pendingConfirmation && (!value.workspaceId || ['inputHash', 'derivationPermitId', 'proposalHash', 'specificationHash'].some(key => typeof value.pendingConfirmation![key as keyof AuthoringConfirmation] !== 'string' || !value.pendingConfirmation![key as keyof AuthoringConfirmation]) || !Number.isSafeInteger(value.pendingConfirmation.expectedWorkspaceRevision) || value.pendingConfirmation.expectedWorkspaceRevision < 1)) return false;
  return true;
}

export function parseAuthoringEditorRecovery(text: string | null, workspaceId: string): AuthoringEditorRecovery | undefined {
  if (!text) return undefined;
  try {
    const value = JSON.parse(text) as AuthoringEditorRecovery;
    if (!value || typeof value !== 'object' || Array.isArray(value) || value.workspaceId !== workspaceId || !workspaceId || !validAuthoringTransaction(value)) return undefined;
    if (value.values !== undefined && (!value.values || typeof value.values !== 'object' || Array.isArray(value.values))) return undefined;
    if (value.values && (typeof value.values.templateId !== 'string' || typeof value.values.goalText !== 'string' || [value.values.requirements, value.values.resources].some(item => item !== undefined && typeof item !== 'string'))) return undefined;
    return value;
  } catch { return undefined; }
}

export function restoreAuthoringEditorRecovery(text: string | null, workspaceId: string): AuthoringEditorRecovery | undefined {
  const value = parseAuthoringEditorRecovery(text, workspaceId);
  if (text !== null && !value) throw new Error('本地编辑恢复记录损坏；保留原记录，禁止提交替代事务');
  return value;
}

export function authoringEditorValues(recovery: AuthoringEditorRecovery): AuthoringEditorValues | undefined {
  if (recovery.values) return recovery.values;
  const pending = recovery.pendingSave;
  if (!pending) return undefined;
  return { templateId: pending.templateId, goalText: pending.input.goalText, requirements: pending.input.acceptanceRequirements?.join('\n'), resources: pending.input.resourceRefs?.join('\n') };
}

export function restoreAuthoringRecovery(text: string | null): AuthoringRecovery | undefined {
  const value = parseAuthoringRecovery(text);
  if (text !== null && !value) throw new Error('本地创建恢复记录损坏；保留原记录，禁止创建替代事务');
  return value;
}

export function parseAuthoringRecovery(text: string | null): AuthoringRecovery | undefined {
  if (!text) return undefined;
  try {
    const value = JSON.parse(text) as AuthoringRecovery;
    if (!value || typeof value !== 'object' || Array.isArray(value) || !value.request || typeof value.request !== 'object' || Array.isArray(value.request) || typeof value.request.requestId !== 'string' || !value.request.requestId || typeof value.request.title !== 'string' || !value.request.title || typeof value.request.templateId !== 'string' || !value.request.templateId || typeof value.request.input?.goalText !== 'string' || !value.request.input.goalText || (value.request.deliverableDirectory !== undefined && typeof value.request.deliverableDirectory !== 'string') || [value.request.input.acceptanceRequirements, value.request.input.resourceRefs].some(items => items !== undefined && (!Array.isArray(items) || items.some(item => typeof item !== 'string')))) return undefined;
    if (!validAuthoringTransaction(value)) return undefined;
    return value;
  } catch {
    return undefined;
  }
}
