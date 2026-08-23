import type { Approval, Decision, Run, WorkspaceExecutionIntent, WorkspaceSnapshot } from './types';

type GovernanceResource = { id: string; workspaceId: string; runId: string; state: string };
type SnapshotReader = (workspaceId: string) => Promise<WorkspaceSnapshot>;

export const governanceContinuationGuidance = '治理 owner 受理后，Run continuation 会自动收敛；请等待投影刷新。若持续阻塞，请检查 resolution delivery diagnostic，不要额外提交执行恢复意图。';

export function governanceResourceKey(item: Pick<GovernanceResource, 'workspaceId' | 'id'>) {
  return `${item.workspaceId}\u0000${item.id}`;
}

export function mergeGovernanceResources<T extends GovernanceResource>(current: T[], incoming: T[], append: boolean) {
  if (!append) {
    return incoming;
  }
  const merged = new Map(current.map((item) => [governanceResourceKey(item), item]));
  for (const item of incoming) {
    merged.set(governanceResourceKey(item), item);
  }
  return [...merged.values()];
}

export function isDedicatedDefinitionDecision(item?: Pick<Decision, 'decisionKind' | 'failureClass' | 'definitionClarification'>) {
  if (item?.definitionClarification) {
    return true;
  }
  const kind = item?.decisionKind?.trim().toLowerCase() ?? '';
  const failureClass = item?.failureClass?.trim().toLowerCase() ?? '';
  return kind.includes('definition_clarification')
    || kind === 'definition_admission'
    || failureClass === 'definition_admission';
}

export function isTypedDefinitionDecision(item?: Pick<Decision, 'definitionClarification'>) {
  return Boolean(item?.definitionClarification);
}

// Definition clarification is a dedicated lifecycle: the user revises the
// immutable activation inputs and explicitly restarts. GovernanceDisplay is
// deliberately absent from this selector; only the canonical Workspace/Run
// references intersected with the concrete owner resource can suppress Resume.
export function canonicalDefinitionClarification(snapshot?: WorkspaceSnapshot): Decision | undefined {
  const workspaceId = snapshot?.workspace?.id;
  const runId = snapshot?.workspace?.activeRunId;
  if (!snapshot || !workspaceId || !runId) {
    return undefined;
  }
  const run = snapshot.runs?.find((candidate) => candidate.id === runId && candidate.workspaceId === workspaceId);
  if (run?.state !== 'blocked_on_decision' || !run.blockingTicketId || run.blockingApprovalId) {
    return undefined;
  }
  return snapshot.decisions?.find((decision) => decision.id === run.blockingTicketId
    && decision.workspaceId === workspaceId
    && decision.runId === runId
    && decision.state === 'open'
    && decision.definitionClarification?.class === 'runtime_definition');
}

export function workspaceDefinitionClarification(snapshot?: WorkspaceSnapshot): Decision | undefined {
  const canonicalRuntime = canonicalDefinitionClarification(snapshot);
  if (canonicalRuntime) {
    return canonicalRuntime;
  }
  const workspaceId = snapshot?.workspace?.id;
  if (!workspaceId) {
    return undefined;
  }
  return snapshot.decisions?.find((decision) => decision.workspaceId === workspaceId
    && !decision.runId
    && decision.state === 'open'
    && decision.definitionClarification?.class === 'workspace_definition_pre_seal');
}

function activeRun(snapshot: WorkspaceSnapshot, workspaceId: string, runId: string): Run | undefined {
  if (snapshot.workspace?.id !== workspaceId || snapshot.workspace.activeRunId !== runId) {
    return undefined;
  }
  return snapshot.runs?.find((run) => run.id === runId && run.workspaceId === workspaceId);
}

export function canonicalActiveRun(snapshot?: WorkspaceSnapshot): Run | undefined {
  const workspaceId = snapshot?.workspace?.id;
  const runId = snapshot?.workspace?.activeRunId;
  if (!snapshot || !workspaceId || !runId) {
    return undefined;
  }
  return activeRun(snapshot, workspaceId, runId);
}

export function canonicalActiveRunIsGovernanceBlocked(snapshot?: WorkspaceSnapshot) {
  const run = canonicalActiveRun(snapshot);
  return run?.state === 'blocked_on_decision' || run?.state === 'blocked_on_approval';
}

// UI adapters may offer an intent only when canonical owner projections permit
// it. A governance-blocked Run converges through owner delivery, never client
// Resume. A Definition replacement remains an explicit user Restart and the
// server performs the authoritative admission; read/display freshness never
// enables or disables that command.
export function workspaceExecutionIntentAllowed(snapshot: WorkspaceSnapshot | undefined, intent: WorkspaceExecutionIntent) {
  if (canonicalDefinitionClarification(snapshot)) {
    return intent === 'restart';
  }
  if (workspaceDefinitionClarification(snapshot)) {
    return false;
  }
  return intent !== 'resume' || !canonicalActiveRunIsGovernanceBlocked(snapshot);
}

// Board is a summary adapter and never starts a Definition replacement. It
// navigates to Workspace Detail, where Restart is an explicit user action.
export function workspaceBoardExecutionIntentAllowed(snapshot: WorkspaceSnapshot | undefined, intent: WorkspaceExecutionIntent) {
  return !workspaceDefinitionClarification(snapshot) && workspaceExecutionIntentAllowed(snapshot, intent);
}

export function isCanonicalDecisionMutationTarget(snapshot: WorkspaceSnapshot | undefined, item: Decision) {
  if (!snapshot || item.state !== 'open' || isDedicatedDefinitionDecision(item)) {
    return false;
  }
  const concrete = snapshot.decisions?.find((decision) => governanceResourceKey(decision) === governanceResourceKey(item));
  if (!concrete || concrete.runId !== item.runId || concrete.state !== 'open' || isDedicatedDefinitionDecision(concrete)) {
    return false;
  }
  const run = activeRun(snapshot, item.workspaceId, item.runId);
  return run?.state === 'blocked_on_decision'
    && run.blockingTicketId === item.id
    && !run.blockingApprovalId;
}

export function isCanonicalApprovalMutationTarget(snapshot: WorkspaceSnapshot | undefined, item: Approval) {
  if (!snapshot || item.state !== 'pending') {
    return false;
  }
  const concrete = snapshot.approvals?.find((approval) => governanceResourceKey(approval) === governanceResourceKey(item));
  if (!concrete || concrete.runId !== item.runId || concrete.state !== 'pending') {
    return false;
  }
  const run = activeRun(snapshot, item.workspaceId, item.runId);
  return run?.state === 'blocked_on_approval'
    && run.blockingApprovalId === item.id
    && !run.blockingTicketId;
}

export async function readGovernanceSnapshots(items: GovernanceResource[], readSnapshot: SnapshotReader) {
  const workspaceIds = [...new Set(items.map((item) => item.workspaceId).filter(Boolean))];
  const snapshots = await Promise.all(workspaceIds.map(async (workspaceId) => [workspaceId, await readSnapshot(workspaceId)] as const));
  return new Map(snapshots);
}

function requireUniqueBatchResourceIDs(items: GovernanceResource[]) {
  const ids = new Set<string>();
  for (const item of items) {
    if (ids.has(item.id)) {
      throw new Error(`治理批量接口无法区分重复资源 ID ${item.id}，请按工作空间分别处理`);
    }
    ids.add(item.id);
  }
}

export async function requireCanonicalDecisionMutationTargets(items: Decision[], readSnapshot: SnapshotReader) {
  requireUniqueBatchResourceIDs(items);
  const snapshots = await readGovernanceSnapshots(items, readSnapshot);
  const rejected = items.find((item) => !isCanonicalDecisionMutationTarget(snapshots.get(item.workspaceId), item));
  if (rejected) {
    throw new Error(`决策 ${rejected.id} 已不是活动运行的真实阻塞工单，或必须通过专用定义澄清流程处理；未提交任何变更`);
  }
  return snapshots;
}

export async function requireCanonicalApprovalMutationTargets(items: Approval[], readSnapshot: SnapshotReader) {
  requireUniqueBatchResourceIDs(items);
  const snapshots = await readGovernanceSnapshots(items, readSnapshot);
  const rejected = items.find((item) => !isCanonicalApprovalMutationTarget(snapshots.get(item.workspaceId), item));
  if (rejected) {
    throw new Error(`审批 ${rejected.id} 已不是活动运行的真实待处理阻塞项；未提交任何变更`);
  }
  return snapshots;
}
