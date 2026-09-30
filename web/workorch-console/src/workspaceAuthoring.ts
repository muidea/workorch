import type { DefinitionAuthoringCreate, DefinitionAuthoringView } from './types.ts';

export const authoringRecoveryKey = 'workorch.workspace-authoring.v1';
export type AuthoringRecovery = { request: DefinitionAuthoringCreate; workspaceId?: string; pendingConfirmation?: { inputHash: string; derivationPermitId: string; proposalHash: string; specificationHash: string; expectedWorkspaceRevision: number } };

const reviewKinds = new Set(['dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility']);

export function authoringCanConfirm(view?: DefinitionAuthoringView): boolean {
  return Boolean(view && view.state === 'ready_to_confirm' && view.templateCurrent && view.inputHash && view.derivationPermitId && view.proposal?.proposalHash && view.proposal.specificationHash && view.proposal.derivationPermitId === view.derivationPermitId && view.proposal.items.length > 0 && view.proposal.reviews?.length === 6 && view.proposal.reviews.every(review => review.status === 'PASS' && reviewKinds.has(review.kind) && review.specificationHash === view.proposal!.proposalHash) && new Set(view.proposal.reviews.map(review => review.kind)).size === 6);
}

export function authoringCanReplayConfirmation(view?: DefinitionAuthoringView, recovery?: AuthoringRecovery): boolean {
  const exact = recovery?.pendingConfirmation;
  return Boolean(view && exact && recovery?.workspaceId === view.workspaceId && ['ready_to_confirm', 'confirming', 'admitting'].includes(view.state) && view.templateCurrent && exact.inputHash === view.inputHash && exact.derivationPermitId === view.derivationPermitId && exact.proposalHash === view.proposal?.proposalHash && exact.specificationHash === view.proposal?.specificationHash && (view.state !== 'ready_to_confirm' || authoringCanConfirm(view)));
}

export function authoringNeedsPolling(view?: DefinitionAuthoringView): boolean {
  return Boolean(view && (view.compilationAccepted || ['deriving', 'derived', 'reviewing', 'outcome_unresolved'].includes(view.state)));
}

export function parseAuthoringRecovery(text: string | null): AuthoringRecovery | undefined {
  if (!text) return undefined;
  try {
    const value = JSON.parse(text) as AuthoringRecovery;
    if (!value.request || typeof value.request.requestId !== 'string' || typeof value.request.title !== 'string' || typeof value.request.templateId !== 'string' || typeof value.request.input?.goalText !== 'string' || (value.workspaceId !== undefined && typeof value.workspaceId !== 'string')) return undefined;
    if (value.pendingConfirmation && (!value.workspaceId || !value.pendingConfirmation.inputHash || !value.pendingConfirmation.derivationPermitId || !value.pendingConfirmation.proposalHash || !value.pendingConfirmation.specificationHash || typeof value.pendingConfirmation.inputHash !== 'string' || typeof value.pendingConfirmation.derivationPermitId !== 'string' || typeof value.pendingConfirmation.proposalHash !== 'string' || typeof value.pendingConfirmation.specificationHash !== 'string' || !Number.isSafeInteger(value.pendingConfirmation.expectedWorkspaceRevision) || value.pendingConfirmation.expectedWorkspaceRevision < 1)) return undefined;
    return value;
  } catch {
    return undefined;
  }
}
