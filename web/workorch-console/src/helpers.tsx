import React, { useState } from 'react';
import { Alert, App as AntApp, Button, Descriptions, Empty, Form, Input, List, Select, Space, Steps, Tabs, Tag, Typography } from 'antd';
import { CardGrid, ProCard, ProTable } from './components/proCompat';
import type { AcceptanceProjection, AcceptanceSlice, ContextBundle, ContextCompressionCandidate, ContextEvidence, ContextSection, ConvergenceStatus, Decision, DoD, EventPayload, EventPayloadValue, EventRecord, ExecutionSection, GapBaseline, GapTodoItem, Goal, LLMContextBudgetSummary, LLMContextBudgetTrace, LLMUsageProviderModelBreakdown, LLMUsageSummary, LLMUsageTrace, MemoryRecallTrace, ProgressBlocker, ProgressItem, ProjectionStatus, RepairLoop, Result, Run, RunHeartbeat, RunObservability, RunSnapshot, RuntimeCapability, RuntimeTimelineItem, RuntimeAuditAssessment, RuntimeAuditCoverageBinding, RuntimeAuditDispatch, RuntimeAuditEvidence, RuntimeAuditOperationReceipt, RuntimeSemanticPlanningItem, ServiceConfiguration, Skill, Soul, TaskDecomposition, TaskDecompositionNode, TaskPlan, TaskProgress, UpdateLLMConfigurationRequest, UpdateWebSearchConfigurationRequest, UpdateWebSearchProviderConfiguration, WebSearchProviderTemplate, WebSourceTrace, WorkspaceExecutionIntent, WorkspaceExecutionIntentRequest, WorkspaceSetupDraft, WorkspaceSnapshot, WorkspaceSoulBindings, WorkspaceTemplate } from './types';
import { errorMessage } from './utils/errorMessage';
import { workorchApi } from './api';
import { modelLabels, progressKindLabels, recoveryModeLabels, repairLoopStateLabels, soulBindingStageLabels } from './utils/constants';
import { stateLabel as canonicalStateLabel } from './utils/format';
import { stateTag as canonicalStateTag } from './utils/ui';
import { presentWorkspaceRunCommand } from './workspaceRunCommand';
import { validationRepairPolicyLabel } from './runtimeProgressPresentation';
import { originalDiagnosticLabel } from './runtimeProgressPresentation';
import { RuntimeInputReviewDetails } from './components/RuntimeInputReviewDetails';
import { RuntimeValidationOutputLinks } from './components/RuntimeValidationOutputLinks';
import type { RuntimeAuditCandidateAttempt } from './types';
import { canonicalActiveRunIsGovernanceBlocked, canonicalDefinitionClarification, workspaceBoardExecutionIntentAllowed, workspaceDefinitionClarification } from './governanceMutationTarget';

export { progressKindLabels, recoveryModeLabels, repairLoopStateLabels, soulBindingStageLabels, stateLabels } from './utils/constants';

export type ViewKey = 'board' | 'workspaceDetail' | 'governance' | 'runtimeControl' | 'configuration' | 'service';

export const runtimeEventPageLimit = 100;
export const runtimeStepPageLimit = 100;
export const runtimeToolCallPageLimit = 100;
export const runtimeCheckpointPageLimit = 100;
export const runtimeResultPageLimit = 100;
export const runtimePatchPageLimit = 100;
export const runtimeValidationPageLimit = 100;
export const runtimeArtifactPageLimit = 100;

export function mergeById<T extends { id: string }>(current: T[], items: T[], append: boolean) {
  if (!append) {
    return items;
  }
  const seen = new Set(current.map((item) => item.id));
  return [
    ...current,
    ...items.filter((item) => {
      if (seen.has(item.id)) {
        return false;
      }
      seen.add(item.id);
      return true;
    })
  ];
}

export const defaultSoulBindingStages = ['planning', 'execution', 'decision', 'approval', 'validation', 'artifact'];

export function stateLabel(state?: string) {
  return canonicalStateLabel(state);
}

export function soulBindingStageLabel(stage: string) {
  return soulBindingStageLabels[stage] ? `${soulBindingStageLabels[stage]} (${stage})` : stage;
}

export function progressKindLabel(kind?: string) {
  if (!kind) {
    return '未知';
  }
  return progressKindLabels[kind] ?? kind;
}

export function recoveryModeLabel(mode?: string) {
  if (!mode) {
    return '未知';
  }
  return recoveryModeLabels[mode] ? `${recoveryModeLabels[mode]} (${mode})` : mode;
}

export function repairLoopStateLabel(state?: string) {
  if (!state) {
    return '未知';
  }
  return repairLoopStateLabels[state] ? `${repairLoopStateLabels[state]} (${state})` : state;
}

export function stateTag(state?: string) {
  return canonicalStateTag(state);
}

export function shortId(value?: string, keep = 10) {
  if (!value) {
    return '-';
  }
  return value.length > keep ? `${value.slice(0, keep)}…` : value;
}

export { errorMessage } from './utils/errorMessage';

export function isNotFoundError(error: unknown) {
  return /(^|\s)404(\s|$)|not found|不存在/i.test(errorMessage(error));
}

export function pendingCount(items?: Array<{ state: string }>) {
  return (items ?? []).filter((item) => item.state === 'open' || item.state === 'pending').length;
}

export function runStateSummary(runs?: Run[]) {
  const items = runs ?? [];
  return {
    total: items.length,
    created: items.filter((item) => item.state === 'created').length,
    active: items.filter((item) => item.state === 'running').length,
    blocked: items.filter((item) => item.state === 'blocked_on_decision' || item.state === 'blocked_on_approval').length,
    validating: items.filter((item) => item.state === 'validating').length,
    completed: items.filter((item) => item.state === 'completed').length,
    failed: items.filter((item) => item.state === 'failed' || item.state === 'cancelled' || item.state === 'superseded').length,
    latest: items[items.length - 1]
  };
}

export function workspaceProjectionRun(snapshot?: WorkspaceSnapshot) {
  if (!snapshot) {
    return undefined;
  }
  const runs = snapshot.runs?.filter(item => item.workspaceId === snapshot.workspace.id);
  const activeRun = runs?.find((item) => item.id === snapshot.workspace.activeRunId);
  if (activeRun) {
    return activeRun;
  }
  const outcomeRun = runs?.find((item) => item.id === snapshot.displayOutcome?.runId);
  if (outcomeRun) {
    return outcomeRun;
  }
  const progressRun = runs?.find((item) => item.id === snapshot.taskProgress?.runId);
  if (progressRun) {
    return progressRun;
  }
  return runStateSummary(runs).latest;
}

export function workspaceProjectionRunLabel(snapshot?: WorkspaceSnapshot) {
  const run = workspaceProjectionRun(snapshot);
  return run ? `${run.id} / ${stateLabel(run.state)}` : '尚未启动执行';
}

export function projectionStatusSummary(status?: ProjectionStatus) {
  if (!status) {
    return '投影状态未知';
  }
  const revisions = [
    `global ${status.globalRevision ?? 0}`,
    status.workspaceRevision ? `workspace ${status.workspaceRevision}` : '',
    status.projectedSourceRevision ? `projected ${status.projectedSourceRevision}` : ''
  ].filter(Boolean);
  const suffix = revisions.length ? ` / ${revisions.join(' / ')}` : '';
  return `${stateLabel(status.state)}${suffix}`;
}

export function projectionStatusDescriptions(status?: ProjectionStatus) {
  if (!status) {
    return [{ key: 'projection', label: '投影状态', children: '-' }];
  }
  return [
    { key: 'state', label: '投影状态', children: stateTag(status.state) },
    { key: 'globalRevision', label: '全局 Revision', children: status.globalRevision ?? '-' },
    { key: 'workspaceRevision', label: 'Workspace Revision', children: status.workspaceRevision ?? '-' },
    { key: 'sourceRevision', label: 'Source Revision', children: status.sourceEventRevision ?? '-' },
    { key: 'projectedRevision', label: '已投影 Revision', children: status.projectedSourceRevision ?? '-' },
    { key: 'updatedAt', label: '更新时间', children: status.updatedAt || '-' },
    { key: 'rebuiltAt', label: '重建时间', children: status.rebuiltAt || '-' },
    { key: 'staleReason', label: 'Stale 原因', children: status.staleReason || '-' },
    { key: 'error', label: '投影错误', children: status.error ? boundedParagraph(status.error, { rows: 2 }) : '-' }
  ];
}

export function runHeartbeatDescriptions(heartbeat?: RunHeartbeat) {
  if (!heartbeat || (!heartbeat.runId && !heartbeat.activityState && !heartbeat.summary)) {
    return [{ key: 'heartbeat', label: '运行活动', children: '-' }];
  }
  return [
    { key: 'activity', label: '运行活动', children: <Space wrap>{heartbeat.activityState ? stateTag(heartbeat.activityState) : '-'}{heartbeat.stalled ? <Tag color="red">stalled</Tag> : null}{heartbeat.ineffectiveRetry ? <Tag color="gold">ineffective retry</Tag> : null}</Space> },
    { key: 'summary', label: '活动摘要', children: boundedParagraph(heartbeat.summary, { rows: 2 }) },
    { key: 'latestStep', label: '最近步骤', children: [heartbeat.latestStepId, heartbeat.latestStepKind, heartbeat.latestStepState].filter(Boolean).join(' / ') || '-' },
    { key: 'latestEvent', label: '最近事件', children: [heartbeat.latestEventTopic, heartbeat.latestEventRevision ? `rev ${heartbeat.latestEventRevision}` : '', heartbeat.latestEventAt].filter(Boolean).join(' / ') || '-' },
    { key: 'latestActivity', label: '最近活动', children: heartbeat.latestActivityAt || '-' },
    { key: 'latestProgress', label: '最近有效进展', children: heartbeat.latestEffectiveProgressAt || '-' },
    { key: 'latestLLM', label: '最近 LLM', children: heartbeat.latestLLMUsageAt || '-' },
    { key: 'latestResult', label: '最近结果', children: heartbeat.latestResultAt || '-' },
    { key: 'latestArtifact', label: '最近交付物', children: heartbeat.latestArtifactAt || '-' },
    { key: 'latestPatch', label: '最近补丁', children: heartbeat.latestPatchAt || '-' },
    { key: 'evidence', label: '最近证据', children: heartbeat.latestEvidenceRef || '-' }
  ];
}

export function convergenceStatusDescriptions(status?: ConvergenceStatus) {
  if (!status || (!status.runId && !status.currentFocusId && !status.convergenceState && !status.currentGap && !status.gapSummary && !status.latestContractViolationRule && !status.assessmentDisposition && !status.currentSliceId)) {
    return [{ key: 'convergenceStatus', label: '收敛状态', children: '-' }];
  }
  const repairLabel = status.maxRepairAttempts
    ? `${status.repairAttempts ?? 0}/${status.maxRepairAttempts}`
    : (status.repairAttempts ?? status.retryAttempt)
      ? String(status.repairAttempts ?? status.retryAttempt)
      : null;
  return [
    { key: 'run', label: '收敛运行', children: [status.runId, status.runState].filter(Boolean).join(' / ') || '-' },
    { key: 'slice', label: '当前 Slice', children: status.currentSliceId || '-' },
    { key: 'focus', label: '当前 Focus', children: status.currentFocusId || '-' },
    { key: 'convergenceState', label: '收敛状态', children: status.convergenceState ? <Tag color="blue">{status.convergenceState}</Tag> : '-' },
    { key: 'coverage', label: 'Coverage', children: <Space wrap><Tag color="green">closed {status.closedCoverageItemCount ?? 0}</Tag><Tag color="gold">remaining {status.remainingCoverageItemCount ?? 0}</Tag></Space> },
    { key: 'disposition', label: 'Assessment Disposition', children: status.assessmentDisposition ? <Tag color="purple">{status.assessmentDisposition}</Tag> : '-' },
    { key: 'profileRepair', label: 'Profile 修复上限', children: validationRepairPolicyLabel(status) },
    { key: 'repair', label: 'Repair / Budget', children: <Space wrap>{repairLabel ? <Tag>repair {repairLabel}</Tag> : null}{status.repeatedFailureSignatureCount || status.retrySignatureRepeatCount ? <Tag color="gold">repeat {status.repeatedFailureSignatureCount ?? status.retrySignatureRepeatCount}</Tag> : null}{!repairLabel && !(status.repeatedFailureSignatureCount || status.retrySignatureRepeatCount) ? '-' : null}</Space> },
    { key: 'gap', label: '当前 Gap', children: boundedParagraph(status.gapSummary || status.currentGap, { rows: 2 }) },
    { key: 'missingEvidence', label: '缺失证据', children: compactTags(status.missingEvidenceKinds ?? [], 4) },
    { key: 'evidenceCount', label: '证据计数', children: <Space wrap><Tag color="green">accepted {status.acceptedEvidenceCount ?? 0}</Tag><Tag color="red">rejected {status.rejectedEvidenceCount ?? 0}</Tag>{status.latestAcceptedEvidenceRef ? <Tag>latest {status.latestAcceptedEvidenceRef}</Tag> : null}</Space> },
    { key: 'latestProgress', label: '最近有效进展', children: status.lastProgressAt || status.latestEffectiveProgressAt || '-' },
    { key: 'nextActivation', label: '需要下一 Activation', children: status.nextActivationRequired ? <Tag color="red">required</Tag> : '-' },
    { key: 'contractViolation', label: '最近合同违例', children: status.latestContractViolationRule ? <Space wrap><Tag color="red">{status.latestContractViolationRule}</Tag>{status.latestRecovery ? <Tag color="gold">{status.latestRecovery}</Tag> : null}</Space> : '-' },
    { key: 'decisionCause', label: '决策原因', children: status.decisionCause ? <Tag color="gold">{status.decisionCause}</Tag> : '-' },
    { key: 'retryState', label: 'Retry 状态', children: <Space wrap>{status.retryAttempt ? <Tag>retry {status.retryMaxAttempts ? `${status.retryAttempt}/${status.retryMaxAttempts}` : status.retryAttempt}</Tag> : null}{status.narrowedRetryAttempted ? <Tag color="blue">narrowed</Tag> : null}{status.managedValidationAvailable ? <Tag color="cyan">managed validation</Tag> : null}{status.retrySignatureRepeatCount ? <Tag color="gold">repeat {status.retrySignatureRepeatCount}</Tag> : null}{!status.retryAttempt && !status.narrowedRetryAttempted && !status.managedValidationAvailable && !status.retrySignatureRepeatCount ? '-' : null}</Space> },
    { key: 'rejectedDiagnostic', label: 'Rejected Diagnostic', children: status.latestRejectedDiagnosticRef || '-' },
    { key: 'problemSignals', label: '问题信号', children: <Space wrap>{status.stalled ? <Tag color="red">stalled</Tag> : null}{status.ineffectiveRetry ? <Tag color="gold">ineffective retry</Tag> : null}{!status.stalled && !status.ineffectiveRetry ? '-' : null}</Space> }
  ];
}

export function runCommandDescriptions(snapshot?: WorkspaceSnapshot) {
  const command = presentWorkspaceRunCommand(snapshot?.runCommand);
  if (!command) {
    return [{ key: 'runCommand', label: '执行准入', children: '-' }];
  }
  const stateColor = command.state === 'accepted' ? 'green' : 'red';
  return [
    { key: 'intent', label: '执行准入', children: <Space wrap><Tag color={stateColor}>{command.state}</Tag>{command.intent ? <Tag>{command.intent}</Tag> : null}{command.source ? <Tag>{command.source}</Tag> : null}</Space> },
    { key: 'requestId', label: '请求 ID', children: command.requestId || '-' },
    { key: 'runId', label: '目标运行', children: command.runId || '-' },
    { key: 'outcome', label: 'Owner outcome', children: <Tag color={stateColor}>{command.outcome}</Tag> },
    { key: 'admissionReason', label: 'Admission reason', children: command.admissionReason },
    { key: 'diagnostic', label: 'Diagnostic', children: boundedParagraph(command.diagnostic, { rows: 2 }) },
    { key: 'requestReason', label: '请求原因', children: command.requestReason || '-' },
    { key: 'observedAt', label: command.state === 'accepted' ? '接收时间' : '拒绝时间', children: command.state === 'accepted' ? command.acceptedAt : command.rejectedAt },
    { key: 'revision', label: '投影 Revision', children: command.revision }
  ];
}

export function isBlockedRunState(state?: string) {
  return state === 'blocked_on_decision' || state === 'blocked_on_approval';
}

export function isCompletedRunState(state?: string) {
  return state === 'completed';
}

export function isFailedRunState(state?: string) {
  return state === 'failed' || state === 'cancelled' || state === 'superseded';
}

export function isTerminalRunState(state?: string) {
  return isCompletedRunState(state) || isFailedRunState(state);
}

// The Web adapter chooses only the user's lifecycle intent. Run owner remains
// authoritative for admission. A missing or terminal current Run requires a
// fresh activation instead of a Resume patch or implicit retry.
export function workspaceExecutionPrimaryIntent(snapshot?: WorkspaceSnapshot): WorkspaceExecutionIntent {
  const activeRunID = snapshot?.workspace.activeRunId;
  if (!activeRunID) {
    return 'restart';
  }
  const activeRun = snapshot?.runs?.find((item) => item.id === activeRunID);
  if (canonicalDefinitionClarification(snapshot)) {
    return 'restart';
  }
  if (isTerminalRunState(activeRun?.state)) {
    return 'restart';
  }
  return 'resume';
}

export function activeOrLatestRun(snapshot?: WorkspaceSnapshot) {
  return workspaceProjectionRun(snapshot);
}

export function workspaceProcessItems(snapshot?: WorkspaceSnapshot, goals?: Goal[], dods?: DoD[]) {
  const currentRun = activeOrLatestRun(snapshot);
  const hasDefinition = Boolean((goals?.length ?? 0) > 0 && (dods?.length ?? 0) > 0);
  const missingDefinition = !goals?.length ? `需要定义${modelLabels.goal}` : !dods?.length ? `需要定义${modelLabels.dod}` : `需要补齐${modelLabels.goal} / ${modelLabels.dod}`;
  const pendingDecisions = pendingCount(snapshot?.decisions);
  const pendingApprovals = pendingCount(snapshot?.approvals);
  const blocked = isBlockedRunState(currentRun?.state) || pendingDecisions > 0 || pendingApprovals > 0;
  const terminal = isTerminalRunState(currentRun?.state);
  const failed = isFailedRunState(currentRun?.state);
  const completed = isCompletedRunState(currentRun?.state);
  const governanceDescription = pendingDecisions > 0 || pendingApprovals > 0
    ? `待处理：决策 ${pendingDecisions} / 审批 ${pendingApprovals}`
    : blocked ? stateLabel(currentRun?.state) : '无待处理阻塞';
  const evidenceDescription = completed
    ? stateLabel(currentRun?.state)
    : failed ? stateLabel(currentRun?.state)
      : currentRun?.state === 'validating' ? `正在执行${modelLabels.doDAssessment}`
        : currentRun ? '等待运行证据闭环' : `尚未提交${modelLabels.run}`;
  return [
    { title: '工作空间', description: snapshot?.workspace ? stateLabel(snapshot.workspace.state) : '未选择', status: snapshot?.workspace ? 'finish' as const : 'wait' as const },
    { title: '目标与验收', description: hasDefinition ? `${modelLabels.goal} / ${modelLabels.dod} 已定义` : missingDefinition, status: hasDefinition ? 'finish' as const : 'process' as const },
    { title: '提交执行', description: currentRun ? `${currentRun.id} / ${stateLabel(currentRun.state)}` : `尚未提交${modelLabels.run}`, status: currentRun ? 'finish' as const : 'wait' as const },
    { title: '治理处理', description: governanceDescription, status: blocked ? 'process' as const : currentRun ? 'finish' as const : 'wait' as const },
    { title: '证据验收', description: evidenceDescription, status: failed ? 'error' as const : completed ? 'finish' as const : terminal ? 'finish' as const : currentRun ? 'process' as const : 'wait' as const }
  ];
}

export function runtimeSummaryTags(runs?: Run[]) {
  const summary = runStateSummary(runs);
  return (
    <Space wrap>
      <Tag>总数 {summary.total}</Tag>
      <Tag color="blue">运行中 {summary.active}</Tag>
      <Tag color="gold">阻塞 {summary.blocked}</Tag>
      <Tag color="cyan">验收中 {summary.validating}</Tag>
      <Tag color="green">完成 {summary.completed}</Tag>
      <Tag color="red">失败 {summary.failed}</Tag>
    </Space>
  );
}

export function taskProgressStepStatus(state?: string) {
  if (state === 'failed') {
    return 'error' as const;
  }
  if (state === 'completed') {
    return 'finish' as const;
  }
  if (state === 'running' || state === 'blocked') {
    return 'process' as const;
  }
  return 'wait' as const;
}

export function acceptanceCard(acceptance?: AcceptanceProjection) {
  if (!acceptance || acceptanceProjectionEmpty(acceptance)) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无收敛投影" />;
  }
  const summary = acceptance.summary ?? { acceptedSlices: 0, activeSlices: 0, openSlices: 0, failedSlices: 0 };
  const focus = acceptance.currentFocus;
  const budget = acceptance.budget;
  const directive = acceptance.repairDirective;
  const semantic = acceptance.semanticPlanning;
  const audit = acceptance.audit;
  const observations = audit?.assessments ?? [];
  const coverageBindings = audit?.coverageBindings ?? [];
  const dispatches = audit?.dispatches ?? [];
  const acceptedEvidence = audit?.acceptedEvidence ?? [];
  const rejectedEvidence = audit?.rejectedEvidence ?? [];
  const operationReceipts = audit?.operationReceipts ?? [];
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Space wrap>
        <Tag color="green">accepted {summary.acceptedSlices}</Tag>
        <Tag color="blue">active {summary.activeSlices}</Tag>
        <Tag color="gold">open {summary.openSlices}</Tag>
        <Tag color="red">failed {summary.failedSlices}</Tag>
      </Space>
      <Descriptions
        column={{ xs: 1, md: 2 }}
        size="small"
        items={[
          { key: 'activeSlice', label: 'Active Slice', children: acceptance.activeSliceId || '-' },
          { key: 'sliceTotal', label: 'Slice Total', children: acceptance.slices?.length ?? 0 },
          { key: 'focus', label: 'Current Focus', children: focus?.id || '-' },
          { key: 'focusState', label: 'Focus State', children: focus?.state ? stateTag(focus.state) : '-' },
          { key: 'focusFailure', label: 'Focus Failure', children: [focus?.failureClass, focus?.failureSignature].filter(Boolean).join(' / ') || '-' },
          { key: 'focusAttempt', label: 'Focus Attempt', children: focus?.attempt || '-' },
          { key: 'requiredEvidence', label: 'Required Evidence', children: compactTags(focus?.requiredEvidenceKinds ?? [], 4) },
          { key: 'targetRefs', label: 'Target Refs', children: compactTags(focus?.targetRefs ?? [], 4) },
          { key: 'budgetDecision', label: 'Budget Decision', children: budget?.decision || budget?.state || '-' },
          { key: 'budgetReason', label: 'Budget Reason', children: budget?.lastDecisionReason || '-' },
          { key: 'repairAttempts', label: 'Repair Attempts', children: budget?.repairAttempts || '-' },
          { key: 'profileRepairLimit', label: 'Profile Repair Limit', children: validationRepairPolicyLabel(budget) },
          { key: 'globalRepairLimit', label: 'Global Repair Limit', children: budget?.maxRepairAttemptsPerFocus ?? '-' },
          { key: 'noDeltaRounds', label: 'No Delta Rounds', children: budget?.noCoverageDeltaRounds || '-' }
        ]}
      />
      {semantic ? (
        <>
          <Descriptions
            title="Semantic Planning (run-scoped observation)"
            column={{ xs: 1, md: 2 }}
            size="small"
            items={[
              { key: 'semanticState', label: 'State', children: stateTag(semantic.state) },
              { key: 'semanticPhase', label: 'Persisted Phase', children: semantic.phase || '-' },
              { key: 'semanticScope', label: 'Workspace / Run', children: `${shortId(semantic.workspaceId)} / ${shortId(semantic.runId)}` },
              { key: 'semanticRevision', label: 'State Revision', children: semantic.stateRevision },
              { key: 'semanticRequest', label: 'Request', children: shortId(semantic.logicalRequestId) },
              { key: 'semanticAttempt', label: 'Plan / Catalog Attempts', children: `${semantic.attempt} / ${semantic.catalogRefreshAttempts}` },
              { key: 'semanticManifest', label: 'Manifest', children: shortId(semantic.manifestHash) },
              { key: 'semanticBinding', label: 'Sealed Binding Set', children: shortId(semantic.sealedBindingSetHash) }
            ]}
          />
          {semantic.failure ? (
            <Alert
              type="error"
              showIcon
              message={`${semantic.failure.originalCode || semantic.failure.originalClass || 'semantic planning rejected'}: ${semantic.failure.originalMessage}`}
              description={<pre className="json-panel">{compactJSON(semantic.failure)}</pre>}
            />
          ) : null}
          {(semantic.rejections?.length ?? 0) > 0 ? (
            <Alert
              type="warning"
              showIcon
              message={`${semantic.rejections?.length ?? 0} semantic proposal rejection(s)`}
              description={<pre className="json-panel">{compactJSON(semantic.rejections)}</pre>}
            />
          ) : null}
          {(semantic.items?.length ?? 0) > 0 ? (
            <ProTable<RuntimeSemanticPlanningItem>
              rowKey="coverageWorkItemId"
              title={() => 'Per-CWI Semantic Plan / Owner Feasibility'}
              search={false}
              options={false}
              pagination={{ pageSize: 8 }}
              size="small"
              columns={[
                { title: 'CWI', dataIndex: 'coverageWorkItemId', ellipsis: true },
                { title: 'State', dataIndex: 'status', width: 150, render: (_, item) => stateTag(item.status) },
                { title: 'Proofs', dataIndex: 'proofs', ellipsis: true, render: (_, item) => compactTags((item.proofs ?? []).map((proof) => [proof.evidenceKind, proof.ownerModuleId, proof.operationKind].filter(Boolean).join(' / ')).filter(Boolean), 3) },
                { title: 'Targets', dataIndex: 'proofs', ellipsis: true, render: (_, item) => compactTags((item.proofs ?? []).flatMap((proof) => proof.targetRefs ?? []).filter(Boolean), 3) },
                { title: 'Recipe / Checks', dataIndex: 'proofs', ellipsis: true, render: (_, item) => compactTags((item.proofs ?? []).flatMap((proof) => [proof.validationRecipeId, ...(proof.requiredCheckRefs ?? [])]).filter(Boolean) as string[], 3) },
                { title: 'Observed Revision', dataIndex: 'proofs', ellipsis: true, render: (_, item) => compactTags((item.proofs ?? []).map((proof) => proof.observedRevision).filter(Boolean) as string[], 3) },
                { title: 'Diagnostic', dataIndex: 'proofs', ellipsis: true, render: (_, item) => (item.proofs ?? []).map((proof) => proof.diagnostic?.originalMessage || proof.diagnostic?.omittedReason).filter(Boolean).join(' | ') || '-' }
              ]}
              expandable={{
                rowExpandable: (item) => Boolean(item.proofs?.some((proof) => proof.diagnostic)),
                expandedRowRender: (item) => <pre className="json-panel">{compactJSON(item.proofs)}</pre>
              }}
              dataSource={semantic.items}
            />
          ) : null}
          {(semantic.feasibilityGaps?.length ?? 0) > 0 ? (
            <Alert
              type="info"
              showIcon
              message={`${semantic.feasibilityGaps?.length ?? 0} persisted feasibility refresh generation(s)`}
              description={<pre className="json-panel">{compactJSON(semantic.feasibilityGaps)}</pre>}
            />
          ) : null}
        </>
      ) : null}
      {directive ? (
        <Descriptions
          title="Canonical Repair Directive"
          column={{ xs: 1, md: 2 }}
          size="small"
          items={[
            { key: 'directiveAction', label: 'Required Action', children: stateTag(directive.requiredAction) },
            { key: 'directiveFocus', label: 'Focus', children: directive.focusId || '-' },
            { key: 'directiveTrigger', label: 'Trigger', children: directive.trigger || '-' },
            { key: 'directiveCoverage', label: 'Coverage', children: compactTags(directive.coverageWorkItemIds ?? [], 4) },
            { key: 'directiveEvidence', label: 'Required Evidence', children: compactTags(directive.requiredEvidenceKinds ?? [], 4) },
            { key: 'directiveTargets', label: 'Grounded Targets', children: compactTags(directive.targetRefs ?? directive.closure?.groundedTargetRefs ?? [], 4) },
            { key: 'directiveClosure', label: 'Closure', children: shortId(directive.closure?.id) },
            { key: 'directiveBaseline', label: 'Baseline Evidence', children: directive.closure?.baselineEvidenceRevision || '-' }
          ]}
        />
      ) : null}
      {audit ? (
        <Descriptions
          title="Run 封存配置（只读，资源升级不会改写）"
          column={1}
          size="small"
          items={[
            { key: 'sealedInput', label: 'Startup Input Hash', children: audit.startupInputHash || '-' },
            { key: 'sealedProfiles', label: 'Profile Contracts Hash', children: audit.sealedProfileContractsHash || '-' },
            { key: 'sealedSouls', label: '封存 Soul 版本', children: (
              <Space direction="vertical" size="small">
                {(audit.sealedSoulBindings ?? []).map((binding) => (
                  <Typography.Text key={binding.stage} copyable>
                    {binding.stage}: {binding.soulId} · r{binding.revision} · {binding.contentHash}
                  </Typography.Text>
                ))}
              </Space>
            ) }
          ]}
        />
      ) : null}
      {audit?.failure ? (
        <Descriptions
          title="Runtime Failure (diagnostic only)"
          column={{ xs: 1, md: 2 }}
          size="small"
          items={[
            { key: 'failureClass', label: 'Canonical Class', children: audit.failure.class || '-' },
            { key: 'failureCode', label: 'Canonical Code', children: audit.failure.code || '-' },
            { key: 'failureSignature', label: 'Signature', children: audit.failure.signature || '-' },
            { key: 'failureTargets', label: 'Targets', children: compactTags(audit.failure.targetRefs ?? [], 4) },
            { key: 'failureMessage', label: 'Message', children: audit.failure.message || '-' },
            { key: 'failureOwner', label: 'Producer', children: audit.failure.originalDiagnostic?.ownerModuleId || '-' },
            { key: 'failureOriginalCode', label: 'Original Code', children: audit.failure.originalDiagnostic?.originalCode || '-' },
            { key: 'failureOperation', label: 'Operation', children: audit.failure.originalDiagnostic?.operationId || '-' },
            { key: 'failureOriginal', label: 'Original Diagnostic', children: <Space direction="vertical" style={{ width: '100%' }}>{audit.failure.originalDiagnostic?.originalMessage || audit.failure.originalDiagnostic?.omittedReason || '-'}<RuntimeInputReviewDetails diagnostic={audit.failure.originalDiagnostic} /></Space> },
            { key: 'failureRevision', label: 'Expected / Observed Revision', children: [audit.failure.originalDiagnostic?.expectedRevision, audit.failure.originalDiagnostic?.observedRevision].filter(Boolean).join(' / ') || '-' }
          ]}
        />
      ) : null}
      {audit?.candidateAttempts?.length ? (
        <ProTable<RuntimeAuditCandidateAttempt>
          rowKey={(item) => `${item.authorityContractId}-${item.attempt ?? 0}`}
          title={() => 'Candidate Attempts (diagnostic only)'}
          search={false}
          options={false}
          pagination={{ pageSize: 5 }}
          size="small"
          columns={[
            { title: 'Attempt', dataIndex: 'attempt', render: (_, item) => item.attempt ?? 0 },
            { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
            { title: 'Owner Outcome', dataIndex: 'ownerFeasibilityOutcome', ellipsis: true },
            { title: 'Failure Code', dataIndex: 'failureCode', ellipsis: true },
            { title: 'Owner / Original Code', render: (_, item) => originalDiagnosticLabel(item.originalDiagnostic) },
            { title: 'Target', render: (_, item) => compactTags(item.originalDiagnostic?.targetRefs ?? [], 4) },
            { title: 'Diagnostic', ellipsis: true, render: (_, item) => item.originalDiagnostic?.originalMessage || '-' }
          ]}
          dataSource={audit.candidateAttempts}
        />
      ) : null}
      {observations.length ? (
        <ProTable<RuntimeAuditAssessment>
          rowKey={(item) => item.id ?? `${item.focusId}-${item.summary}`}
          title={() => 'Assessment Observations (diagnostic only)'}
          search={false}
          options={false}
          pagination={{ pageSize: 5 }}
          size="small"
          columns={[
            { title: 'Focus', dataIndex: 'focusId', ellipsis: true },
            { title: 'Claimed', dataIndex: 'claimedDisposition', width: 150, render: (_, item) => stateTag(item.claimedDisposition) },
            { title: 'Canonical', dataIndex: 'canonicalDisposition', width: 150, render: (_, item) => stateTag(item.canonicalDisposition) },
            { title: 'Summary', dataIndex: 'summary', ellipsis: true },
            { title: 'Claimed Missing', dataIndex: ['gap', 'missingEvidenceKinds'], ellipsis: true, render: (_, item) => compactTags(item.gap?.missingEvidenceKinds ?? [], 3) },
            { title: 'Diagnostic', dataIndex: ['gap', 'reason'], ellipsis: true, render: (_, item) => item.gap?.reason || [item.gap?.failureClass, item.gap?.failureCode].filter(Boolean).join(' / ') || '-' }
          ]}
          dataSource={observations}
        />
      ) : null}
      {coverageBindings.length ? (
        <ProTable<RuntimeAuditCoverageBinding>
          rowKey={(item) => item.id ?? `${item.focusId}-${item.coverageWorkItemId}`}
          title={() => 'Coverage Evidence Bindings (audit only)'}
          search={false}
          options={false}
          pagination={{ pageSize: 8 }}
          size="small"
          columns={[
            { title: 'CWI', dataIndex: 'coverageWorkItemId', ellipsis: true },
            { title: 'Binding', dataIndex: 'id', ellipsis: true, render: (_, item) => shortId(item.id) },
            { title: 'Evidence', dataIndex: 'proofs', ellipsis: true, render: (_, item) => compactTags((item.proofs ?? []).map((proof) => proof.evidenceKind).filter(Boolean) as string[], 3) },
            { title: 'Target / Selector', dataIndex: 'targetRefs', ellipsis: true, render: (_, item) => compactTags(item.targetRefs?.length ? item.targetRefs : [item.targetSelectorRef, item.prospectiveTargetRef].filter(Boolean) as string[], 3) },
            { title: 'Recipe / Checks', dataIndex: 'proofs', ellipsis: true, render: (_, item) => compactTags((item.proofs ?? []).flatMap((proof) => [proof.validationRecipeId, ...(proof.requiredCheckRefs ?? [])]).filter(Boolean) as string[], 3) },
            { title: 'Declaration', dataIndex: 'declarationRef', ellipsis: true }
          ]}
          dataSource={coverageBindings}
        />
      ) : null}
      {dispatches.length ? (
        <ProTable<RuntimeAuditDispatch>
          rowKey={(item) => item.id ?? item.authoritySnapshotId ?? 'dispatch'}
          title={() => 'Sealed Dispatch Operations (audit only)'}
          search={false}
          options={false}
          pagination={{ pageSize: 8 }}
          size="small"
          columns={[
            { title: 'Dispatch', dataIndex: 'id', ellipsis: true, render: (_, item) => shortId(item.id) },
            { title: 'State', dataIndex: 'state', width: 130, render: (_, item) => stateTag(item.state) },
            { title: 'Operations', dataIndex: 'operations', ellipsis: true, render: (_, item) => compactTags((item.operations ?? []).map((operation) => `${operation.id || '-'}:${operation.kind || '-'}`), 4) },
            { title: 'Binding Refs', dataIndex: 'operations', ellipsis: true, render: (_, item) => compactTags((item.operations ?? []).flatMap((operation) => operation.bindingRefs ?? []), 4) },
            { title: 'Targets', dataIndex: 'operations', ellipsis: true, render: (_, item) => compactTags((item.operations ?? []).flatMap((operation) => operation.targetRefs ?? []), 4) }
          ]}
          expandable={{
            expandedRowRender: (item) => <pre className="json-panel">{compactJSON(item.operations ?? [])}</pre>
          }}
          dataSource={dispatches}
        />
      ) : null}
      {acceptedEvidence.length || rejectedEvidence.length ? (
        <ProTable<RuntimeAuditEvidence>
          rowKey={(item) => item.id ?? `${item.dispatchId}-${item.operationId}-${item.revision}`}
          title={() => 'Evidence and Derived CWI Credit (audit only)'}
          search={false}
          options={false}
          pagination={{ pageSize: 8 }}
          size="small"
          columns={[
            { title: 'Evidence', dataIndex: 'id', ellipsis: true, render: (_, item) => shortId(item.id) },
            { title: 'Kind', dataIndex: 'kind', width: 130, render: (_, item) => stateTag(item.kind) },
            { title: 'Dispatch / Operation', dataIndex: 'dispatchId', ellipsis: true, render: (_, item) => [shortId(item.dispatchId), item.operationId].filter(Boolean).join(' / ') || '-' },
            { title: 'Binding Refs', dataIndex: 'bindingRefs', ellipsis: true, render: (_, item) => compactTags(item.bindingRefs ?? [], 3) },
            { title: 'Derived CWI Credit', dataIndex: 'coverageWorkItemIds', ellipsis: true, render: (_, item) => compactTags(item.coverageWorkItemIds ?? [], 3) },
            { title: 'Checks', dataIndex: 'passedChecks', ellipsis: true, render: (_, item) => compactTags(item.passedChecks ?? [], 3) },
            { title: 'Quality', dataIndex: 'quality', ellipsis: true, render: (_, item) => [item.quality, item.zeroWork ? 'zero-work' : undefined, item.failureCode].filter(Boolean).join(' / ') || '-' }
          ]}
          dataSource={[...acceptedEvidence, ...rejectedEvidence]}
        />
      ) : null}
      {operationReceipts.length ? (
        <ProTable<RuntimeAuditOperationReceipt>
          rowKey={(item) => item.executionKey ?? `${item.operationId}-${item.fence}`}
          title={() => 'Owner Operation Receipts (audit only)'}
          search={false}
          options={false}
          pagination={{ pageSize: 8 }}
          size="small"
          columns={[
            { title: 'Operation', dataIndex: 'operationId', ellipsis: true },
            { title: 'Outcome', dataIndex: 'outcome', width: 130, render: (_, item) => stateTag(item.outcome) },
            { title: 'Effect', dataIndex: 'observedEffectScope', ellipsis: true, render: (_, item) => [item.observedEffectScope, item.changed ? 'changed' : 'unchanged'].filter(Boolean).join(' / ') },
            { title: 'Targets', dataIndex: 'targetRefs', ellipsis: true, render: (_, item) => compactTags(item.targetRefs ?? [], 3) },
            { title: 'Recipe / Checks', dataIndex: 'validationRecipeId', ellipsis: true, render: (_, item) => compactTags([item.validationRecipeId, ...(item.passedChecks ?? [])].filter(Boolean) as string[], 3) },
            { title: 'Owner Feedback', dataIndex: 'stableCategory', ellipsis: true, render: (_, item) => [item.originalDiagnostic?.ownerModuleId, item.stableCategory, item.originalDiagnostic?.originalCode, item.originalDiagnostic?.originalMessage].filter(Boolean).join(' / ') || '-' },
            { title: 'Receipt', dataIndex: 'executionKey', ellipsis: true, render: (_, item) => shortId(item.executionKey) },
            { title: '验证原始输出', dataIndex: 'validationOutputs', render: (_, item) => <RuntimeValidationOutputLinks outputs={item.validationOutputs} /> }
          ]}
          dataSource={operationReceipts}
        />
      ) : null}
      {acceptance.slices?.length ? (
        <ProTable<AcceptanceSlice>
          rowKey="id"
          search={false}
          options={false}
          pagination={{ pageSize: 5 }}
          size="small"
          columns={[
            { title: 'Slice', dataIndex: 'id', ellipsis: true },
            { title: 'Title', dataIndex: 'title', ellipsis: true },
            { title: 'State', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
            { title: 'Priority', dataIndex: 'priority', width: 90 },
            { title: 'Scope', dataIndex: 'scope', ellipsis: true },
            { title: 'Evidence', dataIndex: 'requiredEvidenceKinds', ellipsis: true, render: (_, item) => compactTags(item.requiredEvidenceKinds ?? [], 3) }
          ]}
          dataSource={acceptance.slices}
        />
      ) : null}
    </Space>
  );
}

export function gapBaselineCard(gap?: GapBaseline) {
  if (!gap || gapBaselineEmpty(gap)) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无 GapBaseline 投影" />;
  }
  const focusId = gapFocusValue(gap.currentFocus, 'id');
  const focusState = gapFocusValue(gap.currentFocus, 'state');
  const openCount = gap.openCoverageWorkItems?.length ?? 0;
  const todoCount = gap.todoItems?.length ?? 0;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Space wrap>
        <Tag>todo {todoCount}</Tag>
        <Tag>open {openCount}</Tag>
        <Tag color="green">accepted evidence {gap.acceptedEvidenceRefs?.length ?? 0}</Tag>
        <Tag color="red">rejected evidence {gap.rejectedEvidenceRefs?.length ?? 0}</Tag>
      </Space>
      <Descriptions
        column={{ xs: 1, md: 2 }}
        size="small"
        items={[
          { key: 'baselineRevision', label: 'Baseline Revision', children: gap.baselineRevision || '-' },
          { key: 'definitionRevision', label: 'Definition Revision', children: gap.definitionRevision || '-' },
          { key: 'run', label: 'Run', children: gap.runId || '-' },
          { key: 'currentFocus', label: 'Current Focus', children: focusId || '-' },
          { key: 'focusState', label: 'Focus State', children: focusState ? stateTag(focusState) : '-' },
          { key: 'acceptedEvidence', label: 'Accepted Evidence', children: compactTags(gap.acceptedEvidenceRefs ?? [], 4) },
          { key: 'rejectedEvidence', label: 'Rejected Evidence', children: compactTags(gap.rejectedEvidenceRefs ?? [], 4) }
        ]}
      />
      {gap.todoItems?.length ? (
        <ProTable<GapTodoItem>
          rowKey={(item, index) => item.id || `todo-${index}`}
          search={false}
          options={false}
          pagination={{ pageSize: 5 }}
          size="small"
          columns={[
            { title: 'Todo', dataIndex: 'id', ellipsis: true },
            { title: 'Source', dataIndex: 'source', width: 120 },
            { title: 'Source Ref', dataIndex: 'sourceRef', ellipsis: true },
            { title: 'Status', dataIndex: 'status', width: 120, render: (_, item) => item.status ? stateTag(item.status) : '-' },
            { title: 'Priority', dataIndex: 'priority', width: 90 },
            { title: 'Required Evidence', dataIndex: 'requiredEvidenceKinds', ellipsis: true, render: (_, item) => compactTags(item.requiredEvidenceKinds ?? [], 3) },
            { title: 'Reason', dataIndex: 'reason', ellipsis: true }
          ]}
          dataSource={gap.todoItems}
        />
      ) : null}
    </Space>
  );
}

function gapBaselineEmpty(gap: GapBaseline) {
  return !gap.baselineRevision &&
    !gap.definitionRevision &&
    !gap.currentFocus &&
    !(gap.openCoverageWorkItems?.length) &&
    !(gap.todoItems?.length) &&
    !(gap.acceptedEvidenceRefs?.length) &&
    !(gap.rejectedEvidenceRefs?.length);
}

function gapFocusValue(focus: Record<string, unknown> | undefined, key: string) {
  const value = focus?.[key];
  return typeof value === 'string' ? value : undefined;
}

function acceptanceProjectionEmpty(acceptance: AcceptanceProjection) {
  const summary = acceptance.summary ?? { acceptedSlices: 0, activeSlices: 0, openSlices: 0, failedSlices: 0 };
  return !acceptance.activeSliceId &&
    !acceptance.currentFocus &&
    !acceptance.budget &&
    !acceptance.repairDirective &&
    !acceptance.semanticPlanning &&
    !acceptance.audit &&
    !(acceptance.slices?.length) &&
    summary.acceptedSlices === 0 &&
    summary.activeSlices === 0 &&
    summary.openSlices === 0 &&
    summary.failedSlices === 0;
}

export function taskProgressCard(progress?: TaskProgress, repairLoopOverride?: RepairLoop) {
  if (!progress) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无任务进度投影" />;
  }
  const blockers = progress.blockers ?? [];
  const recovery = progress.recovery ?? { mode: 'unknown' };
  const repairLoop = repairLoopOverride ?? progress.repairLoop;
  const repairAttempt = repairLoop?.attemptCount ?? 0;
  const repairFailure = [repairLoop?.lastFailureClass, repairLoop?.lastFailureSignature].filter(Boolean).join(' / ');
  const repairEscalation = repairLoop?.escalated
    ? [repairLoop.escalationTicketId, repairLoop.escalationReason].filter(Boolean).join(' / ')
    : '-';
  const repairSignals = repairLoop?.structuredFailureSignals;
  const repairSignalTargets = asStringArray(repairSignals?.targetRefs);
  const repairSignalIdentifiers = asStringArray(repairSignals?.identifiers);
  const repairCompactionFields = asStringArray(repairLoop?.repairInputCompaction?.omittedFields);
  const prerequisiteRefresh = progress.runtime;
  const prerequisiteSources = (prerequisiteRefresh?.prerequisiteInvalidations ?? []).map((item) => {
    const resource = [item.resourceDomain, item.subjectRef].filter(Boolean).join(':');
    return [item.source, item.ownerModuleId, resource || undefined].filter(Boolean).join(' / ');
  });
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Steps
        size="small"
        items={(progress.milestones ?? []).map((item) => ({
          title: item.title,
          description: stateLabel(item.state),
          status: taskProgressStepStatus(item.state)
        }))}
      />
      <Descriptions
        column={{ xs: 1, md: 2 }}
        size="small"
        items={[
          { key: 'state', label: '任务状态', children: stateTag(progress.state) },
          { key: 'currentMilestone', label: '当前里程碑', children: progress.currentMilestone || '-' },
          { key: 'summary', label: '进度摘要', children: boundedParagraph(progress.summary, { rows: 2 }) },
          { key: 'recoveryMode', label: '恢复模式', children: recoveryModeLabel(recovery.mode) },
          { key: 'checkpoint', label: '恢复检查点', children: recovery.checkpointId ? boundedParagraph(recovery.checkpointId, { rows: 1, copyText: recovery.checkpointId }) : '-' },
          { key: 'blockingTicket', label: '阻塞决策', children: recovery.blockingTicketId || '-' },
          { key: 'blockingApproval', label: '阻塞审批', children: recovery.blockingApprovalId || '-' },
          { key: 'blockingOwner', label: '阻塞资源', children: recovery.blockingResource ? `${recovery.blockingResource.ownerKind}:${recovery.blockingResource.resourceId}` : '-' },
          { key: 'requiredAction', label: 'Owner 要求', children: recovery.blockingResource?.requiredAction || '-' },
          { key: 'allowedActions', label: '允许的用户动作', children: compactTags(recovery.blockingResource?.allowedUserActions ?? [], 4) },
          { key: 'navigation', label: '建议导航', children: recovery.suggestedNavigation || '-' },
          { key: 'repairState', label: '修复状态', children: repairLoop ? repairLoopStateLabel(repairLoop.state) : '-' },
          { key: 'repairAttempt', label: '修复轮次', children: repairAttempt || '-' },
          { key: 'repairFailure', label: '最近失败', children: repairFailure || '-' },
          { key: 'repairTargetScope', label: '失败目标', children: repairLoop?.lastFailureTargetScope || '-' },
          { key: 'repairInputBytes', label: '修复输入字节', children: repairLoop?.lastRepairInputBytes ? numberText(repairLoop.lastRepairInputBytes) : '-' },
          { key: 'repairRepeatedReason', label: '重复失败原因', children: repairLoop?.repeatedFailureReason || '-' },
          { key: 'repairEscalation', label: '升级信息', children: repairEscalation },
          { key: 'repairSignalTargets', label: '失败信号目标', children: compactTags(repairSignalTargets, 4) },
          { key: 'repairSignalIdentifiers', label: '失败信号标识符', children: compactTags(repairSignalIdentifiers, 4) },
          { key: 'repairCompaction', label: '修复输入裁剪', children: repairCompactionFields.length ? compactTags(repairCompactionFields, 4) : '-' },
          { key: 'repairEvidence', label: '修复证据', children: compactTags(repairLoop?.evidenceRefs ?? [], 4) },
          { key: 'prerequisiteRefresh', label: '前置条件刷新', children: prerequisiteRefresh?.prerequisiteRefreshState ? `${prerequisiteRefresh.prerequisiteRefreshState}（stale ${prerequisiteRefresh.prerequisiteRefreshUsed ?? 0}/${prerequisiteRefresh.prerequisiteRefreshLimit ?? 0}；resource ${prerequisiteRefresh.prerequisiteResourceChangeRefreshUsed ?? 0}/${prerequisiteRefresh.prerequisiteResourceChangeRefreshLimit ?? 0}）` : '-' },
          { key: 'prerequisiteSources', label: '刷新来源', children: prerequisiteSources.length ? compactTags(prerequisiteSources, 4) : '-' }
        ]}
      />
      {repairSignals || repairLoop?.repairInputCompaction ? (
        <Descriptions
          column={{ xs: 1, md: 2 }}
          size="small"
          items={[
            {
              key: 'signals',
              label: 'Structured Failure Signals',
              children: repairSignals ? <pre className="json-panel">{compactJSON(repairSignals)}</pre> : '-'
            },
            {
              key: 'compaction',
              label: 'Repair Input Compaction',
              children: repairLoop?.repairInputCompaction ? <pre className="json-panel">{compactJSON(repairLoop.repairInputCompaction)}</pre> : '-'
            }
          ]}
        />
      ) : null}
      {blockers.length ? (
        <ProTable<ProgressBlocker>
          rowKey="id"
          search={false}
          options={false}
          pagination={false}
          size="small"
          columns={[
            { title: '阻塞项', dataIndex: 'id', copyable: true, ellipsis: true },
            { title: '类型', dataIndex: 'kind', width: 120, render: (_, item) => progressKindLabel(String(item.kind)) },
            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(String(item.state)) },
            { title: '原因', dataIndex: 'reason', ellipsis: true },
            { title: '证据', dataIndex: 'evidenceRef', ellipsis: true }
          ]}
          dataSource={blockers}
        />
      ) : null}
      {(progress.items?.length ?? 0) > 0 ? (
        <ProTable<ProgressItem>
          rowKey="id"
          search={false}
          options={false}
          pagination={{ pageSize: 6 }}
          size="small"
          columns={[
            { title: '项目', dataIndex: 'id', copyable: true, ellipsis: true },
            { title: '类型', dataIndex: 'kind', width: 140, render: (_, item) => progressKindLabel(String(item.kind)) },
            { title: '名称', dataIndex: 'title', ellipsis: true },
            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(String(item.state)) }
          ]}
          dataSource={progress.items ?? []}
        />
      ) : null}
    </Space>
  );
}

export function taskDecompositionCard(items?: TaskDecomposition[]) {
  const decompositions = items ?? [];
  if (decompositions.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无任务拆分图" />;
  }
  const latest = decompositions.at(-1);
  const total = decompositions.reduce((sum, item) => sum + item.nodeTotal, 0);
  const completed = decompositions.reduce((sum, item) => sum + item.nodeCompleted, 0);
  const failed = decompositions.reduce((sum, item) => sum + item.nodeFailed, 0);
  const running = decompositions.reduce((sum, item) => sum + item.nodeRunning, 0);
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Descriptions
        column={{ xs: 1, md: 4 }}
        size="small"
        items={[
          { key: 'graph', label: '拆分图', children: decompositions.length },
          { key: 'node', label: '节点', children: `${completed}/${total} 已完成` },
          { key: 'running', label: '执行中', children: running },
          { key: 'failed', label: '失败', children: failed },
          { key: 'latest', label: '最新根步骤', children: latest?.rootStepId ? boundedParagraph(latest.rootStepId, { rows: 1, copyText: latest.rootStepId }) : '-' },
          { key: 'state', label: '最新状态', children: latest ? stateTag(latest.state) : '-' },
          { key: 'branch', label: '分支', children: latest?.graphBranch || '-' },
          { key: 'summary', label: '摘要', children: boundedParagraph(latest?.summary, { rows: 2 }) }
        ]}
      />
      <Tabs
        items={decompositions.map((item, index) => ({
          key: item.id || `${item.rootStepId}-${index}`,
          label: `拆分图 ${index + 1} / ${stateLabel(item.state)}`,
          children: (
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              {metricGrid([
                { key: 'nodes', label: '节点总数', value: item.nodeTotal },
                { key: 'completed', label: '已完成', value: item.nodeCompleted, color: '#389e0d' },
                { key: 'running', label: '执行中', value: item.nodeRunning, color: '#0958d9' },
                { key: 'failed', label: '失败', value: item.nodeFailed, color: '#cf1322' }
              ])}
              <ProTable<TaskDecompositionNode>
                rowKey={(node, nodeIndex) => `${item.id}-${node.id}-${nodeIndex}`}
                search={false}
                options={false}
                pagination={{ pageSize: 8 }}
                size="small"
                columns={[
                  { title: '节点', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 150 },
                  { title: '标题', dataIndex: 'title', ellipsis: true, render: (_, node) => boundedParagraph(node.title, { rows: 2 }) },
                  { title: '状态', dataIndex: 'state', width: 110, render: (_, node) => stateTag(node.state) },
                  { title: '依赖', dataIndex: 'dependsOn', width: 180, render: (_, node) => compactTags(node.dependsOn ?? []) },
                  { title: '执行步骤', dataIndex: 'stepId', copyable: true, ellipsis: true }
                ]}
                expandable={{
                  expandedRowRender: (node) => (
                    <Descriptions
                      column={1}
                      size="small"
                      items={[
                        { key: 'stepState', label: '步骤状态', children: node.stepState ? stateLabel(node.stepState) : '-' },
                        { key: 'branch', label: '分支', children: node.branch || '-' },
                        { key: 'evidence', label: '证据', children: compactTags(node.evidenceRefs ?? [], 6) },
                        { key: 'input', label: '输入', children: <pre className="json-panel">{compactJSON(node.input)}</pre> }
                      ]}
                    />
                  )
                }}
                dataSource={item.nodes ?? []}
              />
            </Space>
          )
        }))}
      />
    </Space>
  );
}

export function taskPlanCard(plans?: TaskPlan[], decompositions?: TaskDecomposition[]) {
  const taskPlans = plans ?? [];
  if (taskPlans.length === 0) {
    return taskDecompositionCard(decompositions);
  }
  const latest = taskPlans.at(-1);
  const total = taskPlans.reduce((sum, item) => sum + item.nodeTotal, 0);
  const completed = taskPlans.reduce((sum, item) => sum + item.nodeCompleted, 0);
  const failed = taskPlans.reduce((sum, item) => sum + item.nodeFailed, 0);
  const running = taskPlans.reduce((sum, item) => sum + item.nodeRunning, 0);
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Descriptions
        column={{ xs: 1, md: 4 }}
        size="small"
        items={[
          { key: 'plans', label: '计划修订', children: taskPlans.length },
          { key: 'node', label: '节点', children: `${completed}/${total} 已完成` },
          { key: 'running', label: '执行中', children: running },
          { key: 'failed', label: '失败', children: failed },
          { key: 'latest', label: '最新计划', children: latest ? boundedParagraph(latest.id, { rows: 1, copyText: latest.id }) : '-' },
          { key: 'state', label: '最新状态', children: latest ? stateTag(latest.state) : '-' },
          { key: 'source', label: '输出状态', children: latest?.sourceOutputState ? stateTag(latest.sourceOutputState) : '-' },
          { key: 'summary', label: '摘要', children: boundedParagraph(latest?.summary, { rows: 2 }) }
        ]}
      />
      <Tabs
        items={taskPlans.map((item, index) => ({
          key: item.id || `${item.anchorStepId}-${item.revision}-${index}`,
          label: `计划 ${index + 1} / ${stateLabel(item.state)}`,
          children: (
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              {metricGrid([
                { key: 'nodes', label: '节点总数', value: item.nodeTotal },
                { key: 'completed', label: '已完成', value: item.nodeCompleted, color: '#389e0d' },
                { key: 'running', label: '执行中', value: item.nodeRunning, color: '#0958d9' },
                { key: 'failed', label: '失败', value: item.nodeFailed, color: '#cf1322' }
              ])}
              <Descriptions
                column={{ xs: 1, md: 2 }}
                size="small"
                items={[
                  { key: 'anchor', label: '锚定步骤', children: item.anchorStepId ? boundedParagraph(item.anchorStepId, { rows: 1, copyText: item.anchorStepId }) : '-' },
                  { key: 'revisionStep', label: '修订步骤', children: item.revisionStepId ? boundedParagraph(item.revisionStepId, { rows: 1, copyText: item.revisionStepId }) : '-' },
                  { key: 'revision', label: '修订号', children: item.revision || '-' },
                  { key: 'stage', label: '阶段', children: item.stage || '-' },
                  { key: 'sourceProjection', label: '投影来源', children: item.sourceProjection || '-' },
                  { key: 'branch', label: '分支', children: item.graphBranch || '-' },
                  { key: 'retryOf', label: '重试来源', children: item.retryOf ? boundedParagraph(item.retryOf, { rows: 1, copyText: item.retryOf }) : '-' },
                  { key: 'retryAttempt', label: '重试轮次', children: item.retryAttempt || '-' },
                  { key: 'violation', label: '违例', children: [item.violationType, item.violatedRule].filter(Boolean).join(' / ') || '-' },
                  { key: 'failure', label: '失败分类', children: [item.failureClass, item.failureSignature].filter(Boolean).join(' / ') || '-' },
                  { key: 'targetScope', label: '目标范围', children: item.targetScope || '-' },
                  { key: 'blocking', label: '阻塞决策', children: item.blockingTicketId || '-' },
                  { key: 'evidence', label: '证据', children: compactTags(item.evidenceRefs ?? [], 6) }
                ]}
              />
              <ProTable<TaskDecompositionNode>
                rowKey={(node, nodeIndex) => `${item.id}-${node.id}-${nodeIndex}`}
                search={false}
                options={false}
                pagination={{ pageSize: 8 }}
                size="small"
                columns={[
                  { title: '节点', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 150 },
                  { title: '标题', dataIndex: 'title', ellipsis: true, render: (_, node) => boundedParagraph(node.title, { rows: 2 }) },
                  { title: '状态', dataIndex: 'state', width: 130, render: (_, node) => stateTag(node.state) },
                  { title: '重试', dataIndex: 'retryAttempt', width: 90, render: (_, node) => node.retryAttempt || '-' },
                  { title: '违例', width: 190, ellipsis: true, render: (_, node) => [node.violationType, node.violatedRule].filter(Boolean).join(' / ') || '-' },
                  { title: '执行步骤', dataIndex: 'stepId', copyable: true, ellipsis: true }
                ]}
                expandable={{
                  expandedRowRender: (node) => (
                    <Descriptions
                      column={1}
                      size="small"
                      items={[
                        { key: 'stepState', label: '步骤状态', children: node.stepState ? stateLabel(node.stepState) : '-' },
                        { key: 'branch', label: '分支', children: node.branch || '-' },
                        { key: 'retryOf', label: '重试来源', children: node.retryOf ? boundedParagraph(node.retryOf, { rows: 1, copyText: node.retryOf }) : '-' },
                        { key: 'failure', label: '失败分类', children: [node.failureClass, node.failureSignature].filter(Boolean).join(' / ') || '-' },
                        { key: 'targetScope', label: '目标范围', children: node.targetScope || '-' },
                        { key: 'blocking', label: '阻塞决策', children: node.blockingTicketId || '-' },
                        { key: 'coverageTargets', label: '覆盖目标', children: compactTags(node.coverageTargets ?? [], 6) },
                        { key: 'evidence', label: '证据', children: compactTags(node.evidenceRefs ?? [], 6) },
                        { key: 'input', label: '输入', children: <pre className="json-panel">{compactJSON(node.input)}</pre> }
                      ]}
                    />
                  )
                }}
                dataSource={item.nodes ?? []}
              />
            </Space>
          )
        }))}
      />
    </Space>
  );
}

export function executionSectionsCard(items?: ExecutionSection[]) {
  const sections = items ?? [];
  if (sections.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`暂无${modelLabels.executionSection}`} />;
  }
  const completed = sections.filter((item) => item.state === 'completed').length;
  const failed = sections.filter((item) => item.state === 'failed').length;
  const running = sections.filter((item) => item.state === 'running').length;
  const memoryRecallRefs = new Set(sections.flatMap((item) => item.memoryRecallRefs ?? [])).size;
  const memoryIds = new Set(sections.flatMap((item) => item.memoryIds ?? [])).size;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'total', label: '分区总数', value: sections.length, color: '#1668dc' },
        { key: 'completed', label: '已完成', value: completed, color: '#389e0d' },
        { key: 'running', label: '执行中', value: running, color: '#0958d9' },
        { key: 'failed', label: '失败', value: failed, color: '#cf1322' },
        { key: 'memoryRecallRefs', label: modelLabels.memoryRecall, value: memoryRecallRefs, color: '#08979c' },
        { key: 'memoryIds', label: `${modelLabels.memory}引用`, value: memoryIds, color: '#531dab' }
      ])}
      <ProTable<ExecutionSection>
        rowKey="id"
        search={false}
        options={false}
        pagination={{ pageSize: 8 }}
        size="small"
        columns={[
          { title: '分区', dataIndex: 'title', ellipsis: true, render: (_, item) => boundedParagraph(item.title || item.id, { rows: 2 }) },
          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
          { title: '图节点', dataIndex: 'graphNodeId', copyable: true, ellipsis: true },
          { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
          { title: modelLabels.memoryRecall, dataIndex: 'memoryRecallRefs', width: 180, render: (_, item) => compactTags(item.memoryRecallRefs ?? [], 3) },
          { title: modelLabels.memory, dataIndex: 'memoryIds', width: 160, render: (_, item) => compactTags(item.memoryIds ?? [], 3) },
          { title: '证据', dataIndex: 'actualEvidenceRefs', width: 220, render: (_, item) => compactTags(item.actualEvidenceRefs ?? [], 4) }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <Descriptions
              column={1}
              size="small"
              items={[
                { key: 'summary', label: '摘要', children: boundedParagraph(item.summary, { rows: 2 }) },
                { key: 'dod', label: '关联验收项', children: compactTags(item.relatedDoDItems ?? [], 6) },
                { key: 'planned', label: '计划引用', children: compactTags(item.plannedStepRefs ?? [], 6) },
                { key: 'governance', label: '治理工单', children: compactTags([...(item.decisionRefs ?? []), ...(item.approvalRefs ?? [])], 6) },
                { key: 'runtime', label: '运行证据', children: compactTags([...(item.toolCallRefs ?? []), ...(item.patchRefs ?? []), ...(item.validationRefs ?? [])], 6) },
                { key: 'outputs', label: `${modelLabels.result}/${modelLabels.artifact}`, children: compactTags([...(item.resultRefs ?? []), ...(item.artifactRefs ?? [])], 6) },
                { key: 'memoryRecall', label: `${modelLabels.memoryRecall}证据`, children: compactTags(item.memoryRecallRefs ?? [], 8) },
                { key: 'memoryIds', label: `${modelLabels.memory}引用`, children: compactTags(item.memoryIds ?? [], 8) }
              ]}
            />
          )
        }}
        dataSource={sections}
      />
    </Space>
  );
}

export type MemoryRecallGroup = {
  key: string;
  source: string;
  stage: string;
  status: string;
  traces: MemoryRecallTrace[];
  memoryIds: string[];
};

export function memoryRecallGroups(items: MemoryRecallTrace[]) {
  const grouped = new Map<string, MemoryRecallTrace[]>();
  for (const item of items) {
    const source = item.source || 'unknown';
    const stage = item.stage || 'unknown';
    const key = `${source}/${stage}`;
    grouped.set(key, [...(grouped.get(key) ?? []), item]);
  }
  return Array.from(grouped.entries())
    .map(([key, traces]) => {
      const [source, stage] = key.split('/');
      const status = traces.some((item) => item.status === 'degraded') ? 'degraded' : traces.some((item) => item.status === 'selected') ? 'selected' : traces[0]?.status || 'unknown';
      const memoryIds = Array.from(new Set(traces.flatMap((item) => item.retrievedMemoryIds ?? [])));
      return { key, source, stage, status, traces, memoryIds };
    })
    .sort((a, b) => a.key.localeCompare(b.key));
}

export function memoryRecallQueryPanel(item: MemoryRecallTrace) {
  return compactJSON({
    policy: item.policy,
    providerId: item.providerId,
    operation: item.operation,
    queryTextPreview: item.queryTextPreview,
    queryTextSha256: item.queryTextSha256,
    minScore: item.minScore,
    limit: item.limit,
    embeddingModel: item.embeddingModel,
    vectorDimension: item.vectorDimension,
    occurredAt: item.occurredAt,
    evidenceRef: item.evidenceRef
  });
}

export function memoryRecallCard(items?: MemoryRecallTrace[]) {
  const traces = items ?? [];
  if (traces.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未记录${modelLabels.memoryRecall}证据`} />;
  }
  const groups = memoryRecallGroups(traces);
  const memoryIds = new Set(traces.flatMap((item) => item.retrievedMemoryIds ?? [])).size;
  const degraded = traces.filter((item) => item.status === 'degraded').length;
  const selected = traces.filter((item) => item.status === 'selected').length;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'traces', label: modelLabels.memoryRecallTrace, value: traces.length, color: '#1668dc' },
        { key: 'groups', label: '阶段分组', value: groups.length, color: '#08979c' },
        { key: 'selected', label: '已选中', value: selected, color: '#389e0d' },
        { key: 'degraded', label: '降级', value: degraded, color: degraded ? '#cf1322' : undefined },
        { key: 'memoryIds', label: modelLabels.memory, value: memoryIds, color: '#531dab' }
      ])}
      <ProTable<MemoryRecallGroup>
        rowKey="key"
        search={false}
        options={false}
        pagination={false}
        size="small"
        columns={[
          { title: '分组', dataIndex: 'key', render: (_, item) => <Space><Tag color="blue">{item.source}</Tag><Tag>{item.stage}</Tag></Space> },
          { title: '状态', dataIndex: 'status', width: 110, render: (_, item) => stateTag(item.status) },
          { title: modelLabels.memoryRecallTrace, width: 140, render: (_, item) => item.traces.length },
          { title: `命中${modelLabels.memory}`, dataIndex: 'memoryIds', ellipsis: true, render: (_, item) => compactTags(item.memoryIds, 5) }
        ]}
        expandable={{
          expandedRowRender: (group) => (
            <ProTable<MemoryRecallTrace>
              rowKey="id"
              search={false}
              options={false}
              pagination={false}
              size="small"
              columns={[
                { title: modelLabels.memoryRecallTrace, dataIndex: 'id', copyable: true, ellipsis: true },
                { title: '策略', dataIndex: 'policy', ellipsis: true },
                { title: '状态', dataIndex: 'status', width: 100, render: (_, item) => stateTag(item.status) },
                { title: `命中${modelLabels.memory}`, dataIndex: 'retrievedMemoryIds', ellipsis: true, render: (_, item) => compactTags(item.retrievedMemoryIds ?? [], 5) },
                { title: '原因', dataIndex: 'reason', ellipsis: true, render: (_, item) => boundedParagraph(item.reason || item.queryTextPreview || '-', { rows: 2 }) }
              ]}
              dataSource={group.traces}
              expandable={{
                expandedRowRender: (item) => (
                  <CardGrid>
                    <ProCard title="召回查询" colSpan={{ xs: 24, lg: 12 }}>
                      <pre className="json-panel json-panel-large">{memoryRecallQueryPanel(item)}</pre>
                    </ProCard>
                    <ProCard title={`命中${modelLabels.memory}详情`} colSpan={{ xs: 24, lg: 12 }}>
                      <pre className="json-panel json-panel-large">{compactJSON(item.retrievedMemoryHits ?? [])}</pre>
                    </ProCard>
                  </CardGrid>
                )
              }}
            />
          )
        }}
        dataSource={groups}
      />
    </Space>
  );
}

export type WebSourceRow = {
  key: string;
  kind: 'search' | 'fetch';
  providerId?: string;
  query?: string;
  title?: string;
  url: string;
  finalUrl?: string;
  source?: string;
  occurredAt?: string;
  contentHash?: string;
  truncated?: boolean;
  evidenceRef?: string;
  approvalId?: string;
  policyDecision?: string;
};

export function webSourceRows(items?: WebSourceTrace[]) {
  const rows: WebSourceRow[] = [];
  for (const trace of items ?? []) {
    for (const source of trace.sources ?? []) {
      rows.push({
        key: `search-${source.evidenceRef || source.url}-${rows.length}`,
        kind: 'search',
        providerId: trace.providerId,
        query: trace.query,
        title: source.title,
        url: source.url,
        source: source.source,
        occurredAt: trace.searchedAt || source.publishedAt,
        evidenceRef: source.evidenceRef || trace.evidenceRefs?.[0],
        approvalId: trace.approvalId,
        policyDecision: trace.policyDecision
      });
    }
    for (const source of trace.fetchedSources ?? []) {
      rows.push({
        key: `fetch-${source.evidenceRef || source.finalUrl || source.url}-${rows.length}`,
        kind: 'fetch',
        providerId: trace.providerId,
        title: source.title,
        url: source.url,
        finalUrl: source.finalUrl,
        source: source.source,
        occurredAt: source.retrievedAt,
        contentHash: source.contentHash,
        truncated: source.truncated,
        evidenceRef: source.evidenceRef || trace.evidenceRefs?.[0],
        approvalId: trace.approvalId,
        policyDecision: trace.policyDecision
      });
    }
  }
  return rows;
}

export function webSourceCard(items?: WebSourceTrace[]) {
  const rows = webSourceRows(items);
  if (rows.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前运行未记录 Web 来源证据" />;
  }
  const fetched = rows.filter((item) => item.kind === 'fetch').length;
  const approvals = new Set(rows.map((item) => item.approvalId).filter(Boolean)).size;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'sources', label: '来源总数', value: rows.length, color: '#1668dc' },
        { key: 'fetched', label: '已读取来源', value: fetched, color: fetched ? '#389e0d' : undefined },
        { key: 'approvals', label: modelLabels.approval, value: approvals, color: approvals ? '#d48806' : undefined }
      ])}
      <ProTable<WebSourceRow>
        rowKey="key"
        search={false}
        options={false}
        pagination={{ pageSize: 6 }}
        size="small"
        columns={[
          { title: '类型', dataIndex: 'kind', width: 90, render: (_, item) => <Tag color={item.kind === 'fetch' ? 'green' : 'blue'}>{item.kind === 'fetch' ? 'Fetch' : 'Search'}</Tag> },
          { title: '来源', render: (_, item) => boundedParagraph(item.title || item.source || item.url, { rows: 2, copyText: item.url }) },
          { title: 'URL', render: (_, item) => <Typography.Link href={item.finalUrl || item.url} target="_blank" rel="noreferrer">{item.finalUrl || item.url}</Typography.Link>, ellipsis: true },
          { title: '证据', dataIndex: 'evidenceRef', ellipsis: true, render: (_, item) => item.evidenceRef ? compactTags([item.evidenceRef], 1) : '-' },
          { title: '策略', dataIndex: 'policyDecision', width: 110, render: (_, item) => item.policyDecision || '-' },
          { title: '时间', dataIndex: 'occurredAt', width: 180, ellipsis: true },
          { title: 'Hash', dataIndex: 'contentHash', ellipsis: true, render: (_, item) => item.contentHash || '-' }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <Descriptions
              size="small"
              column={1}
              items={[
                { key: 'query', label: '查询', children: item.query || '-' },
                { key: 'provider', label: 'Provider', children: item.providerId || '-' },
                { key: 'approval', label: modelLabels.approval, children: item.approvalId || '-' },
                { key: 'truncated', label: '是否截断', children: item.truncated ? '是' : '否' },
                { key: 'url', label: '原始 URL', children: boundedParagraph(item.url, { rows: 1, copyText: item.url }) }
              ]}
            />
          )
        }}
        dataSource={rows}
      />
    </Space>
  );
}

export function numberText(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

export function percentText(value?: number) {
  if (value === undefined || value === null) {
    return '-';
  }
  return `${(value * 100).toFixed(1)}%`;
}

export function llmUsageProviderModelKey(item: LLMUsageProviderModelBreakdown, index?: number) {
  return [item.providerId, item.provider, item.model, index].filter(Boolean).join('|');
}

export function llmUsageCard(summary?: LLMUsageSummary, traces?: LLMUsageTrace[]) {
  if (!summary || summary.availability === 'not_loaded') {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="LLM 用量未加载；未加载不代表零调用" />;
  }
  const usageSummary = summary;
  const usageTraces = traces ?? [];
  const scopeLabel = summary.scope === 'workspace' ? 'Workspace' : 'Run';
  const incomplete = summary.availability !== 'complete';
  const queryDescription = `仅显示已查询的 ${scopeLabel} 用量窗口（上限 ${summary.recordLimit} 条，返回 ${summary.observedRecords} 条）；不代表全部历史调用。`;
  if (usageSummary.entryTotal === 0 && usageTraces.length === 0) {
    if (incomplete) {
      return <Alert type="warning" showIcon message="LLM 用量数据不完整" description={`${queryDescription} 返回记录尚不能形成完整用量汇总，不能视为零调用。`} />;
    }
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`已查询的 ${scopeLabel} 用量窗口内没有记录；不代表全部历史调用`} />;
  }
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert type={incomplete ? 'warning' : 'info'} showIcon message={incomplete ? 'LLM 用量数据不完整' : 'LLM 用量查询范围'} description={queryDescription} />
      {metricGrid([
        { key: 'calls', label: 'LLM 调用', value: numberText(usageSummary.entryTotal), color: '#1668dc' },
        { key: 'tokens', label: 'Token 总量', value: numberText(usageSummary.totalTokens), color: '#0958d9' },
        { key: 'input', label: '输入 Token', value: numberText(usageSummary.inputTokens) },
        { key: 'output', label: '输出 Token', value: numberText(usageSummary.outputTokens) },
        { key: 'cacheRead', label: 'Cache Read Token', value: numberText(usageSummary.cacheReadTokens) },
        { key: 'cacheWrite', label: 'Cache Write Token', value: numberText(usageSummary.cacheWriteTokens) },
        { key: 'cacheReadRatio', label: 'Cache Read/Input', value: percentText(usageSummary.cacheReadInputRatio), color: usageSummary.cacheReadInputRatio ? '#389e0d' : undefined },
        { key: 'failed', label: '失败调用', value: numberText(usageSummary.failedEntries), color: usageSummary.failedEntries ? '#cf1322' : undefined }
      ])}
      <Descriptions
        column={{ xs: 1, md: 3 }}
        size="small"
        items={[
          { key: 'native', label: 'Provider 原生 usage', children: numberText(usageSummary.providerNativeEntries) },
          { key: 'estimated', label: '估算 usage', children: numberText(usageSummary.estimatedEntries) },
          { key: 'cacheable', label: 'Cacheable/Non-cacheable 输入', children: `${numberText(usageSummary.cacheableInputTokens)} / ${numberText(usageSummary.nonCacheableInputTokens)}` },
          { key: 'cacheRatio', label: 'Cacheable/Non-cacheable 比例', children: `${percentText(usageSummary.cacheableInputRatio)} / ${percentText(usageSummary.nonCacheableInputRatio)}` },
          { key: 'cacheEntries', label: 'Cache 口径', children: `${numberText(usageSummary.cacheProviderEntries)} provider / ${numberText(usageSummary.cacheEstimatedEntries)} estimated` },
          { key: 'providerModels', label: 'Provider/Model', children: numberText(usageSummary.providerModelBreakdown?.length ?? 0) }
        ]}
      />
      <Tabs
        items={[
          {
            key: 'providerModel',
            label: `Provider/Model ${usageSummary.providerModelBreakdown?.length ?? 0}`,
            children: (
              <ProTable<LLMUsageProviderModelBreakdown>
                rowKey={llmUsageProviderModelKey}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[
                  { title: 'Provider', render: (_, item) => `${item.providerId || '-'} / ${item.provider || '-'}`, ellipsis: true },
                  { title: 'Model', dataIndex: 'model', ellipsis: true },
                  { title: '调用', dataIndex: 'entryTotal', width: 90, render: (_, item) => numberText(item.entryTotal) },
                  { title: '输入/输出', width: 150, render: (_, item) => `${numberText(item.inputTokens)} / ${numberText(item.outputTokens)}` },
                  { title: 'Token', dataIndex: 'totalTokens', width: 120, render: (_, item) => numberText(item.totalTokens) },
                  { title: 'Cache R/W', width: 130, render: (_, item) => `${numberText(item.cacheReadTokens)} / ${numberText(item.cacheWriteTokens)}` },
                  { title: 'Cache Read/Input', width: 140, render: (_, item) => percentText(item.cacheReadInputRatio) },
                  { title: '估算/失败', width: 120, render: (_, item) => `${numberText(item.estimatedEntries)} / ${numberText(item.failedEntries)}` }
                ]}
                dataSource={usageSummary.providerModelBreakdown ?? []}
              />
            )
          },
          {
            key: 'traces',
            label: `调用明细 ${usageTraces.length}`,
            children: (
              <ProTable<LLMUsageTrace>
                rowKey="id"
                search={false}
                options={false}
                pagination={{ pageSize: 8 }}
                size="small"
                columns={[
                  { title: '调用', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: 'Provider', render: (_, item) => `${item.providerId || '-'} / ${item.model || '-'}`, ellipsis: true },
                  { title: '操作', dataIndex: 'operation', ellipsis: true },
                  { title: '状态', dataIndex: 'status', width: 110, render: (_, item) => stateTag(item.status || 'unknown') },
                  { title: 'usage 来源', dataIndex: 'usageSource', width: 140, render: (_, item) => item.usageSource || (item.estimated ? 'estimated' : 'provider_native') },
                  { title: 'Token', width: 150, render: (_, item) => `${numberText(item.inputTokens)} / ${numberText(item.outputTokens)} / ${numberText(item.totalTokens)}` },
                  { title: '耗时', dataIndex: 'durationMs', width: 100, render: (_, item) => item.durationMs ? `${numberText(item.durationMs)} ms` : '-' },
                  { title: '物理响应', dataIndex: 'physicalResponseMode', width: 110, render: (_, item) => <Tag color={item.physicalResponseMode === 'streaming' ? 'blue' : 'default'}>{item.physicalResponseMode}</Tag> },
                  { title: 'Provider 提交', dataIndex: 'providerSubmitted', width: 110, render: (_, item) => item.providerSubmitted ? <Tag color="green">submitted</Tag> : <Tag>not submitted</Tag> }
                ]}
                expandable={{
                  expandedRowRender: (item) => (
                    <Descriptions
                      size="small"
                      column={1}
                      items={[
                        { key: 'logicalRequestId', label: '逻辑请求 ID', children: item.logicalRequestId ? boundedParagraph(item.logicalRequestId, { rows: 1, copyText: item.logicalRequestId }) : '-' },
                        { key: 'physicalCallId', label: '物理调用 ID', children: item.physicalCallId ? boundedParagraph(item.physicalCallId, { rows: 1, copyText: item.physicalCallId }) : '-' },
                        { key: 'step', label: '步骤', children: item.stepId ? boundedParagraph(item.stepId, { rows: 1, copyText: item.stepId }) : '-' },
                        { key: 'bytes', label: '请求/响应字节', children: `${numberText(item.requestBytes)} / ${numberText(item.responseBytes)}` },
                        { key: 'failure', label: '失败分类', children: item.failureClass || '-' },
                        { key: 'time', label: '时间', children: [item.startedAt, item.completedAt || item.occurredAt].filter(Boolean).join(' → ') || '-' },
                        { key: 'cacheUsage', label: 'Cache Usage', children: <pre className="json-panel">{compactJSON(item.cacheUsage)}</pre> },
                        { key: 'budget', label: 'Context Budget', children: <pre className="json-panel">{compactJSON(item.contextBudget)}</pre> }
                      ]}
                    />
                  )
                }}
                dataSource={usageTraces}
              />
            )
          }
        ]}
      />
    </Space>
  );
}

export function llmContextBudgetCard(traces?: LLMContextBudgetTrace[], summary?: LLMContextBudgetSummary) {
	const items = traces ?? [];
	if (items.length === 0) {
		return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前运行未记录 LLM Context Budget" />;
	}
	const truncatedCount = summary?.truncatedCount ?? items.filter((item) => item.truncated).length;
	const overBudgetCount = summary?.overBudgetCount ?? items.filter((item) => item.overBudget).length;
	const p0OverflowCount = summary?.p0OverflowCount ?? items.filter((item) => item.p0Overflow).length;
	const goalDoDCompressedCount = summary?.goalDoDCompressedCount ?? items.filter((item) => item.goalDoDCompressed).length;
	const traceTotal = summary?.traceTotal || items.length;
	return (
		<Space direction="vertical" size="middle" style={{ width: '100%' }}>
			{metricGrid([
				{ key: 'traceCount', label: '预算轨迹', value: numberText(traceTotal), color: '#1668dc' },
				{ key: 'truncatedCount', label: '触发压缩', value: numberText(truncatedCount), color: truncatedCount ? '#d48806' : undefined },
				{ key: 'overBudgetCount', label: '超预算', value: numberText(overBudgetCount), color: overBudgetCount ? '#cf1322' : undefined },
				{ key: 'p0OverflowCount', label: 'P0 Overflow', value: numberText(p0OverflowCount), color: p0OverflowCount ? '#cf1322' : undefined },
				{ key: 'goalDoDCompressedCount', label: 'Goal/DoD 被压缩', value: numberText(goalDoDCompressedCount), color: goalDoDCompressedCount ? '#cf1322' : '#389e0d' },
				{ key: 'maxShrinkLevel', label: '最大压缩级别', value: numberText(summary?.maxShrinkLevel) },
				{ key: 'maxContextUtilization', label: '最大利用率', value: percentText(summary?.maxContextUtilizationRatio), color: summary?.maxContextUtilizationRatio && summary.maxContextUtilizationRatio > 0.85 ? '#d48806' : undefined },
				{ key: 'avgContextUtilization', label: '平均利用率', value: percentText(summary?.avgContextUtilizationRatio) },
				{ key: 'maxInputTokens', label: '最大输入 Token', value: numberText(summary?.maxInputTokens) },
				{ key: 'recentWindow', label: '最近窗口', value: `${numberText(summary?.recentTraceTotal)} / ${numberText(summary?.recentWindowSize)}` },
				{ key: 'recentUtilization', label: '最近平均利用率', value: percentText(summary?.recentAvgContextUtilizationRatio), color: summary?.recentAvgContextUtilizationRatio && summary.recentAvgContextUtilizationRatio > 0.85 ? '#d48806' : undefined },
				{ key: 'recentMaxInputTokens', label: '最近最大输入', value: numberText(summary?.recentMaxInputTokens) },
				{ key: 'contextUtilizationTrend', label: '利用率趋势', value: summary?.contextUtilizationTrend || '-' }
			])}
      <ProTable<LLMContextBudgetTrace>
        rowKey={(item, index) => `${item.stepId}-${index}`}
        search={false}
        options={false}
        pagination={{ pageSize: 8 }}
        size="small"
        columns={[
          { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
          { title: 'Provider/Model', render: (_, item) => `${item.providerId || '-'} / ${item.model || '-'}`, ellipsis: true },
          { title: '字节预算', width: 150, render: (_, item) => `${numberText(item.actualBytes)} / ${numberText(item.maxBytes)}` },
          { title: 'Token 预算', width: 150, render: (_, item) => `${numberText(item.estimatedInputTokens)} / ${numberText(item.maxTokens)}` },
				{ title: '压缩级别', dataIndex: 'shrinkLevel', width: 90, render: (_, item) => numberText(item.shrinkLevel) },
				{ title: '利用率', width: 100, render: (_, item) => percentText(item.contextUtilizationRatio) },
				{ title: '状态', width: 220, render: (_, item) => compactTags([item.truncated ? 'truncated' : 'full', item.overBudget ? 'over_budget' : '', item.p0Overflow ? 'p0_overflow' : '', item.goalDoDCompressed ? 'goal_dod_compressed' : 'goal_dod_protected'].filter(Boolean), 5) }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <Descriptions
              size="small"
              column={1}
              items={[
                { key: 'rules', label: '预算规则', children: compactTags(item.rules ?? [], 8) },
                { key: 'sections', label: '分区处理', children: compactTags([`included:${(item.includedSections ?? []).join('|') || '-'}`, `normalized:${(item.normalizedSections ?? []).join('|') || '-'}`, `compressed:${(item.compressedSections ?? []).join('|') || '-'}`, `omitted:${(item.omittedSections ?? []).join('|') || '-'}`], 8) },
                { key: 'refs', label: '来源引用', children: compactTags(item.sourceRefs ?? [], 8) },
				{ key: 'profileMode', label: 'Profile', children: `${item.profileMode || '-'} / source: ${item.capabilitySource || '-'}` },
				{ key: 'stageBudget', label: 'Stage Budget', children: `${percentText(item.stageBudgetRatio)} / source: ${item.stageBudgetSource || '-'}` },
				{ key: 'requestBytes', label: '请求字节上限', children: numberText(item.maxRequestBytes) },
				{ key: 'policy', label: 'trace 策略', children: `${item.tracePolicy || '-'} / estimator: ${item.estimator || '-'}` },
                { key: 'profile', label: 'Profile', children: <pre className="json-panel">{compactJSON(item.profile)}</pre> }
              ]}
            />
          )
        }}
        dataSource={items}
      />
    </Space>
  );
}

export function contextBundleCard(bundle?: ContextBundle) {
  if (!bundle) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无上下文包" />;
  }
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Descriptions
        column={{ xs: 1, md: 3 }}
        size="small"
        items={[
          { key: 'mode', label: '上下文模式', children: bundle.mode },
          { key: 'compressedMode', label: '压缩模式', children: bundle.compressedContext?.mode || '-' },
          { key: 'run', label: `关联${modelLabels.run}`, children: bundle.runId || '-' },
          { key: 'summary', label: '摘要', children: boundedParagraph(bundle.summary, { rows: 2 }) },
		  { key: 'recovery', label: '恢复状态', children: bundle.recovery?.mode || '-' },
          { key: 'checkpoint', label: '恢复检查点', children: bundle.latestCheckpoint?.id ? boundedParagraph(bundle.latestCheckpoint.id, { rows: 1, copyText: bundle.latestCheckpoint.id }) : '-' },
          { key: 'compression', label: '压缩候选', children: `${bundle.compressionCandidates?.length ?? 0} 项` }
        ]}
      />
      <Tabs
        items={[
          {
            key: 'sections',
            label: `上下文分区 ${bundle.sections?.length ?? 0}`,
            children: (
              <ProTable<ContextSection>
                rowKey="id"
                search={false}
                options={false}
                pagination={{ pageSize: 6 }}
                size="small"
                columns={[
                  { title: '分区', dataIndex: 'title', ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 160 },
                  { title: '优先级', dataIndex: 'priority', width: 90 },
                  { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => item.state ? stateTag(item.state) : '-' },
                  { title: '摘要', dataIndex: 'summary', ellipsis: true }
                ]}
                expandable={{
                  expandedRowRender: (item) => <pre className="json-panel json-panel-large">{compactJSON(item.data)}</pre>
                }}
                dataSource={bundle.sections ?? []}
              />
            )
          },
          {
            key: 'compressed',
            label: '压缩上下文',
            children: (
              <Descriptions
                column={1}
                size="small"
                items={[
                  { key: 'version', label: '版本', children: bundle.compressedContext?.version || '-' },
                  { key: 'summary', label: '压缩摘要', children: boundedParagraph(bundle.compressedContext?.summary, { rows: 3 }) },
                  { key: 'retention', label: '保留策略', children: boundedParagraph(bundle.compressedContext?.retentionPolicy, { rows: 2 }) },
                  { key: 'sourceSections', label: '来源分区', children: compactTags(bundle.compressedContext?.sourceSectionRefs ?? []) },
                  { key: 'sourceEvidence', label: '来源证据', children: compactTags(bundle.compressedContext?.sourceEvidenceRefs ?? []) }
                ]}
              />
            )
          },
          {
            key: 'evidence',
            label: `最近证据 ${bundle.recentEvidence?.length ?? 0}`,
            children: (
              <ProTable<ContextEvidence>
                rowKey={(item, index) => `${item.kind}-${item.id}-${index}`}
                search={false}
                options={false}
                pagination={{ pageSize: 6 }}
                size="small"
                columns={[
                  { title: '证据', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 180 },
                  { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => item.state ? stateTag(item.state) : '-' },
                  { title: '摘要', dataIndex: 'summary', ellipsis: true },
                  { title: '时间', dataIndex: 'occurredAt', width: 190 }
                ]}
                expandable={{
                  expandedRowRender: (item) => <pre className="json-panel json-panel-large">{compactJSON(item.data)}</pre>
                }}
                dataSource={bundle.recentEvidence ?? []}
              />
            )
          },
          {
            key: 'compression',
            label: `压缩候选 ${bundle.compressionCandidates?.length ?? 0}`,
            children: (
              <ProTable<ContextCompressionCandidate>
                rowKey={(item, index) => `${item.sectionId}-${index}`}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[
                  { title: '分区', dataIndex: 'sectionId', width: 180 },
                  { title: '建议模式', dataIndex: 'suggestedMode', width: 150 },
                  { title: '原因', dataIndex: 'reason', ellipsis: true },
                  { title: '保留策略', dataIndex: 'retention', ellipsis: true }
                ]}
                dataSource={bundle.compressionCandidates ?? []}
              />
            )
          }
        ]}
      />
    </Space>
  );
}

export function skillScopeLabel(skill: Skill) {
  if (skill.source === 'skill-hub') {
    return skill.scope === 'workspace' ? '工作空间 SkillHub 技能' : '全局 SkillHub 技能';
  }
  return '内置技能';
}

export function skillDisplayLabel(skill: Skill) {
  const name = skill.name || skill.sourceSkillId || skill.id;
  const version = skill.version ? `@${skill.version}` : '';
  const status = skill.status ? ` / ${skill.status}` : '';
  return `${name}${version} / ${skillScopeLabel(skill)}${status}`;
}

export type MetricItem = {
  key: string;
  label: string;
  value: React.ReactNode;
  color?: string;
};

export function metricGrid(items: MetricItem[]) {
  return (
    <div className="workorch-metric-grid">
      {items.map((item) => (
        <div key={item.key} className="workorch-metric-item">
          <Typography.Text type="secondary">{item.label}</Typography.Text>
          <Typography.Title level={4} style={{ margin: 0, color: item.color }}>{item.value}</Typography.Title>
        </div>
      ))}
    </div>
  );
}

export function runtimeCapabilityTable(items: RuntimeCapability[] | undefined, emptyText: string) {
  if (!items?.length) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />;
  }
  return (
    <ProTable<RuntimeCapability>
      rowKey="id"
      search={false}
      options={false}
      pagination={false}
      size="small"
      columns={[
        { title: '业务能力', dataIndex: 'displayName', ellipsis: true },
        { title: '类型', dataIndex: 'kind', width: 100, render: (_, item) => capabilityKindTag(item.kind) },
        { title: '技术标识', dataIndex: 'name', ellipsis: true },
        { title: '服务', dataIndex: 'serverId', ellipsis: true },
        { title: '分类', dataIndex: 'category', width: 120, render: (_, item) => item.category || '-' },
        { title: '风险', dataIndex: 'riskLevel', width: 110, render: (_, item) => item.riskLevel || '-' },
        { title: '使用次数', dataIndex: 'useCount', width: 100, render: (_, item) => item.useCount ?? '-' },
        { title: '证据', dataIndex: 'evidenceRefs', ellipsis: true, render: (_, item) => (item.evidenceRefs ?? []).join(', ') || '-' }
      ]}
      dataSource={items}
    />
  );
}

export function capabilityKindTag(kind?: string) {
  if (kind === 'tool') return <Tag color="blue">{modelLabels.mcpTool}</Tag>;
  if (kind === 'resource') return <Tag color="cyan">{modelLabels.mcpResource}</Tag>;
  if (kind === 'prompt') return <Tag color="geekblue">{modelLabels.mcpPrompt}</Tag>;
  return kind ? <Tag>{kind}</Tag> : '-';
}

export function evidenceCount(snapshot?: RunSnapshot) {
  if (!snapshot) {
    return 0;
  }
  return (snapshot.toolCalls?.length ?? 0) +
    (snapshot.patches?.length ?? 0) +
    (snapshot.validationResults?.length ?? 0) +
    (snapshot.runtimeCheckpoints?.length ?? 0) +
    (snapshot.results?.length ?? 0) +
    (snapshot.artifacts?.length ?? 0);
}

export function timelineLabel(item: RuntimeTimelineItem) {
  return item.topic || item.type || item.source;
}

export function compactJSON(value: unknown) {
  if (value === undefined || value === null) {
    return '-';
  }
  return JSON.stringify(value, null, 2);
}

export function secondaryCellText(value: string) {
  return <Typography.Text type="secondary">{value}</Typography.Text>;
}

export function timelineStreamContent(item: RuntimeTimelineItem) {
  if (item.source === 'step_stream') {
    if (item.type === 'omitted') {
      const omitted = numberValue(item.data?.omitted);
      const total = numberValue(item.data?.total);
      return <Space size={4} wrap><Tag color="orange">omitted {numberText(omitted)}</Tag><Typography.Text type="secondary">total {numberText(total)}</Typography.Text></Space>;
    }
    const delta = outputText(item.data, 'delta');
    const truncated = item.data?.deltaTruncated === true;
    if (delta !== '-') {
      return <Space size={4} wrap><Typography.Text>{delta}</Typography.Text>{truncated ? <Tag color="orange">truncated</Tag> : null}</Space>;
    }
    return item.type ? `事件类型：${item.type}` : '已记录流式片段';
  }
  if (item.source === 'step_stream_summary') {
    const bytes = numberValue(item.data?.receivedBytes);
    const chunks = numberValue(item.data?.receivedEventCount ?? item.data?.receivedDeltaCount);
    const incomplete = item.data?.streamIncomplete === true;
    return <Space size={4} wrap><Tag color={incomplete ? 'red' : 'cyan'}>{incomplete ? 'stream incomplete' : 'stream summary'}</Tag>{chunks ? <Typography.Text>{numberText(chunks)} chunks</Typography.Text> : null}{bytes ? <Typography.Text>{numberText(bytes)} bytes</Typography.Text> : null}</Space>;
  }
  const raw = (item.output ?? item.data)?.streamEvents;
  if (Array.isArray(raw) && raw.length > 0) {
    return <Tag color="cyan">已记录 {raw.length} 条</Tag>;
  }
  const streamEventCount = numberValue((item.output ?? item.data)?.streamEventCount);
  if ((item.output ?? item.data)?.streamEventsOmitted === true || streamEventCount > 0) {
    return <Space size={4} wrap><Tag color="cyan">stream events omitted</Tag>{streamEventCount ? <Typography.Text>{numberText(streamEventCount)} chunks</Typography.Text> : null}</Space>;
  }
  return secondaryCellText('不适用');
}

function numberValue(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

export function timelineOccurredAtContent(item: RuntimeTimelineItem) {
  return item.occurredAt || secondaryCellText('未记录');
}

export function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

export type DecisionPayloadField = {
  name: string;
  label: string;
  required: boolean;
  initialValue?: unknown;
};

export function stringListFromValue(value: unknown) {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map((item) => {
    if (typeof item === 'string') {
      return item;
    }
    const record = recordValue(item);
    return String(record?.name ?? record?.key ?? record?.field ?? '');
  }).map((item) => item.trim()).filter(Boolean);
}

export function primitiveSuggestedPayload(context: Record<string, unknown> | undefined) {
  const payload: Record<string, unknown> = {};
  if (!context) {
    return payload;
  }
  for (const [key, value] of Object.entries(context)) {
    const lowerKey = key.toLowerCase();
    const shouldUse = lowerKey.startsWith('suggested') ||
      lowerKey.startsWith('default') ||
      lowerKey === 'candidate' ||
      lowerKey === 'payload';
    const record = shouldUse ? recordValue(value) : undefined;
    if (!record) {
      continue;
    }
    for (const [field, fieldValue] of Object.entries(record)) {
      if (fieldValue === null || fieldValue === undefined) {
        continue;
      }
      if (typeof fieldValue === 'string' || typeof fieldValue === 'number' || typeof fieldValue === 'boolean') {
        payload[field] = fieldValue;
      }
    }
  }
  return payload;
}

export function decisionRequiredFields(item: Decision) {
  const context = recordValue(item.context);
  const requiredFields = [
    ...stringListFromValue(context?.requiredFields),
    ...stringListFromValue(context?.fields)
  ];
  const suggestedFields = Object.keys(primitiveSuggestedPayload(context));
  return Array.from(new Set([...requiredFields, ...suggestedFields]));
}

export function decisionPayloadFields(item: Decision): DecisionPayloadField[] {
  const context = recordValue(item.context);
  const suggested = primitiveSuggestedPayload(context);
  const required = new Set([
    ...stringListFromValue(context?.requiredFields),
    ...stringListFromValue(context?.fields)
  ]);
  return decisionRequiredFields(item).map((field) => ({
    name: field,
    label: decisionFieldLabel(field),
    required: required.has(field),
    initialValue: suggested[field]
  }));
}

export function decisionInitialPayload(item: Decision) {
  const structuredPayload: Record<string, unknown> = {};
  for (const field of decisionPayloadFields(item)) {
    structuredPayload[field.name] = field.initialValue ?? '';
  }
  return {
    operator: 'console',
    answer: '',
    note: '',
    extraPayloadText: '',
    structuredPayload
  };
}

export function decisionFieldLabel(field: string) {
  return field;
}

export function parseDecisionExtraPayload(text: unknown) {
  const content = typeof text === 'string' ? text.trim() : '';
  if (!content) {
    return {};
  }
  const parsed = JSON.parse(content) as unknown;
  const record = recordValue(parsed);
  if (!record) {
    throw new Error('JSON 高级载荷必须是对象');
  }
  return record;
}

export function normalizeDecisionResolutionPayload(values: Record<string, unknown>) {
  const structuredPayload = recordValue(values.structuredPayload) ?? {};
  const payload: Record<string, unknown> = {
    operator: typeof values.operator === 'string' && values.operator.trim() ? values.operator.trim() : 'console'
  };
  for (const [key, value] of Object.entries(structuredPayload)) {
    if (typeof value === 'string') {
      payload[key] = value.trim();
      continue;
    }
    payload[key] = value;
  }
  if (typeof values.answer === 'string' && values.answer.trim()) {
    payload.answer = values.answer.trim();
  }
  if (typeof values.note === 'string' && values.note.trim()) {
    payload.note = values.note.trim();
  }
  return {
    ...payload,
    ...parseDecisionExtraPayload(values.extraPayloadText)
  };
}

export function outputText(output: Record<string, unknown> | undefined, key: string) {
  const value = output?.[key];
  if (value === undefined || value === null || value === '') {
    return '-';
  }
  return typeof value === 'string' ? value : compactJSON(value);
}

export function eventRecordSummary(item: EventRecord) {
  const payload = item.payload ?? item.payloadSummary;
  for (const key of ['summary', 'reason', 'failureReason', 'error', 'action', 'status', 'state']) {
    const value = eventPayloadText(payload, key);
    if (value !== '-') {
      return value;
    }
  }
  if (item.payloadRef) {
    return item.payloadRef;
  }
  return item.subjectId || item.subjectType || '-';
}

export function eventRecordState(item: EventRecord) {
  const payload = item.payload ?? item.payloadSummary;
  const state = eventPayloadText(payload, 'state');
  if (state !== '-') {
    return stateLabel(state);
  }
  const status = eventPayloadText(payload, 'status');
  return status !== '-' ? status : '-';
}

export function eventPayloadDisplay(payload: EventPayload | undefined): Record<string, unknown> | undefined {
  if (!payload?.fields?.length) {
    return undefined;
  }
  return Object.fromEntries(payload.fields.map((field) => [field.name, eventPayloadValue(field.value)]));
}

function eventPayloadText(payload: EventPayload | undefined, key: string) {
  const field = payload?.fields?.find((item) => item.name === key);
  if (!field) {
    return '-';
  }
  const value = eventPayloadValue(field.value);
  if (value === undefined || value === null || value === '') {
    return '-';
  }
  return typeof value === 'string' ? value : compactJSON(value);
}

function eventPayloadValue(value: EventPayloadValue): unknown {
  switch (value.kind) {
    case 'null': return null;
    case 'string': return value.string ?? '';
    case 'bool': return value.bool ?? false;
    case 'int': return value.int ?? 0;
    case 'uint': return value.uint ?? 0;
    case 'float': return value.float ?? 0;
    case 'object': return Object.fromEntries((value.object ?? []).map((field) => [field.name, eventPayloadValue(field.value)]));
    case 'array': return (value.array ?? []).map(eventPayloadValue);
    default: return undefined;
  }
}

export function eventRecordSubject(item: EventRecord) {
  return [item.subjectType, item.subjectId].filter(Boolean).join(' / ') || '-';
}

export function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

export const runtimeRawRefFields = [
  { key: 'requestRef', label: 'Request' },
  { key: 'responseRef', label: 'Response' },
  { key: 'streamRef', label: 'Stream' },
  { key: 'usageRef', label: 'Usage' },
  { key: 'repairResponseRef', label: 'Repair' }
];

export function runtimeRawRefActions(value: unknown) {
  const root = asRecord(value);
  const refs = asRecord(root?.llmCallRefs) ?? (runtimeRawRefFields.some((field) => typeof root?.[field.key] === 'string') ? root : undefined);
  if (!refs) {
    return null;
  }
  const actions = runtimeRawRefFields.flatMap((field) => {
    const ref = refs[field.key];
    if (typeof ref !== 'string' || !ref) {
      return [];
    }
    return [{ ...field, ref }];
  });
  if (actions.length === 0) {
    return null;
  }
  return (
    <Space size={4} wrap className="runtime-raw-actions">
      <Typography.Text type="secondary">LLM Raw</Typography.Text>
      {actions.map((action) => (
        <Button
          key={`${action.key}:${action.ref}`}
          size="small"
          type="link"
          href={workorchApi.runtimePayloadURL(action.ref)}
          target="_blank"
        >
          {action.label}
        </Button>
      ))}
    </Space>
  );
}

export function runtimeRawItemSafe(value: string) {
  return btoa(value).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

export function runtimePayloadRef(workspaceId: string, runId: string, collection: string, itemId: string, fileName: string) {
  return `workorch://workspaces/${workspaceId}/runs/${runId}/${collection}/${runtimeRawItemSafe(itemId)}/${fileName}`;
}

export function runtimeDetailActions(workspaceId: string, runId: string, collection: string, itemId: string, actions: Array<{ fileName: string; label: string }>) {
  if (!workspaceId || !runId || !collection || !itemId || actions.length === 0) {
    return null;
  }
  return (
    <Space size={4} wrap className="runtime-raw-actions">
      <Typography.Text type="secondary">Raw</Typography.Text>
      {actions.map((action) => {
        const ref = runtimePayloadRef(workspaceId, runId, collection, itemId, action.fileName);
        return (
          <Button
            key={`${collection}:${itemId}:${action.fileName}`}
            size="small"
            type="link"
            href={workorchApi.runtimePayloadURL(ref)}
            target="_blank"
          >
            {action.label}
          </Button>
        );
      })}
    </Space>
  );
}

export function asStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.map(String).filter(Boolean) : [];
}

export function dodAssessment(snapshot?: RunSnapshot) {
  const resultAssessment = (primaryAcceptedResult(snapshot) ?? primaryResult(snapshot))?.doDAssessment;
  if (resultAssessment) {
    return resultAssessment as Record<string, unknown>;
  }
  const steps = snapshot?.steps ?? [];
  for (let index = steps.length - 1; index >= 0; index -= 1) {
    const assessment = asRecord(steps[index].output?.dodAssessment);
    if (assessment) {
      return assessment;
    }
  }
  return undefined;
}

export function dodAssessmentItems(snapshot?: RunSnapshot) {
  const assessmentItems = dodAssessment(snapshot)?.items;
  return Array.isArray(assessmentItems) ? assessmentItems.map((item, index) => ({
    key: `dod-${index}`,
    item: asRecord(item)?.item ? String(asRecord(item)?.item) : compactJSON(item),
    state: asRecord(item)?.state ? String(asRecord(item)?.state) : '-',
    evidenceRefs: asStringArray(asRecord(item)?.evidenceRefs),
    failureReason: asRecord(item)?.failureReason ? String(asRecord(item)?.failureReason) : ''
  })) : [];
}

export function memoryRefIDs(snapshot?: RunSnapshot) {
	const refs = new Set<string>();
	for (const step of snapshot?.steps ?? []) {
		for (const ref of asStringArray(step.output?.memoryRefs)) {
			refs.add(ref);
    }
	}
	return refs;
}

export function resultTimeValue(result: Result) {
	return result.updatedAt || result.createdAt || result.id;
}

export function primaryResult(snapshot?: RunSnapshot) {
	const results = snapshot?.results ?? [];
	if (!results.length) {
		return undefined;
	}
	return [...results].sort((left, right) => resultTimeValue(right).localeCompare(resultTimeValue(left)))[0];
}

export function primaryAcceptedResult(snapshot?: RunSnapshot) {
	const results = (snapshot?.results ?? []).filter((item) => item.state === 'accepted');
	if (!results.length) {
		return undefined;
	}
	return [...results].sort((left, right) => resultTimeValue(right).localeCompare(resultTimeValue(left)))[0];
}

export function goalOutcomeSummary(snapshot?: RunSnapshot) {
	const result = primaryAcceptedResult(snapshot) ?? primaryResult(snapshot);
	const summary = snapshot?.displayOutcome?.summary || result?.summaryDetail?.goalOutcome || result?.doDAssessment?.summary || result?.summary || snapshot?.run.failureReason;
	return summary ? String(summary) : '尚未形成可确认的目标成果';
}

export function goalOutcomeState(snapshot?: RunSnapshot) {
	if (snapshot?.displayOutcome?.accepted || snapshot?.displayOutcome?.state === 'accepted') {
		return <Tag color="green">结果已接受</Tag>;
	}
	if (snapshot?.displayOutcome?.state === 'failed') {
		return <Tag color="red">验收未通过</Tag>;
	}
	const result = primaryResult(snapshot);
	if (result?.state === 'accepted') {
		return <Tag color="green">结果已接受</Tag>;
	}
	if (result) {
		return stateTag(result.state);
	}
	const assessment = dodAssessment(snapshot);
	if (assessment?.satisfied === true) {
		return <Tag color="green">验收标准（DoD）已满足</Tag>;
  }
  if (assessment?.satisfied === false) {
    return <Tag color="red">验收标准（DoD）未满足</Tag>;
  }
  return snapshot ? stateTag(snapshot.run.state) : <Tag>未选择运行（Run）</Tag>;
}

export function runOutcomeState(snapshot?: RunSnapshot, observability?: RunObservability) {
  return observability?.displayOutcome?.state || snapshot?.displayOutcome?.state || snapshot?.run.state || 'unknown';
}

export function runRawState(snapshot?: RunSnapshot, observability?: RunObservability) {
  return observability?.displayOutcome?.rawState || snapshot?.displayOutcome?.rawState || snapshot?.run.state || 'unknown';
}

export function boundedParagraph(value?: React.ReactNode, options?: { rows?: number; copyText?: string; className?: string }) {
  return (
    <Typography.Paragraph
      className={['workorch-bounded-text', options?.className].filter(Boolean).join(' ')}
      copyable={options?.copyText ? { text: options.copyText } : false}
      ellipsis={{ rows: options?.rows ?? 2, expandable: true, symbol: '展开' }}
    >
      {value || '-'}
    </Typography.Paragraph>
  );
}

export function compactTags(items: string[], limit = 3) {
  if (items.length === 0) {
    return '-';
  }
  return (
    <Space wrap size={[4, 4]} className="workorch-compact-tags">
      {items.slice(0, limit).map((item) => <Tag key={item}>{item}</Tag>)}
      {items.length > limit ? <Tag color="blue">+{items.length - limit}</Tag> : null}
    </Space>
  );
}

export function workspaceSetupDraftStateLabel(state?: string) {
  switch ((state || '').trim()) {
    case 'draft_ready':
      return '草案可用';
    case 'clarifying':
      return '需要澄清';
    case 'confirmed':
      return '已确认';
    case 'expired':
      return '已过期';
    default:
      return '未开始';
  }
}

export function workspaceSetupDraftStateColor(state?: string) {
  switch ((state || '').trim()) {
    case 'draft_ready':
      return 'green';
    case 'clarifying':
      return 'orange';
    case 'confirmed':
      return 'blue';
    case 'expired':
      return 'red';
    default:
      return undefined;
  }
}

export function workspaceSetupDraftConfidenceLabel(value?: string) {
  switch ((value || '').trim()) {
    case 'high':
      return '高';
    case 'medium':
      return '中';
    case 'low':
      return '低';
    default:
      return '-';
  }
}

export function workspaceSetupDraftTextBlock(value?: string) {
  const text = (value || '').trim();
  return (
    <Typography.Paragraph className="workspace-draft-text-block">
      {text || '-'}
    </Typography.Paragraph>
  );
}

export function workspaceSetupDraftDoDList(items?: string[]) {
  const normalized = (items ?? []).map((item) => item.trim()).filter(Boolean);
  if (normalized.length === 0) {
    return <Typography.Text type="secondary">-</Typography.Text>;
  }
  return (
    <ol className="workspace-draft-dod-list">
      {normalized.map((item, index) => (
        <li key={`${index}-${item}`}>
          <Typography.Text>{item}</Typography.Text>
        </li>
      ))}
    </ol>
  );
}

export function workspaceSetupDraftInfoList(items?: string[]) {
  const normalized = (items ?? []).map((item) => item.trim()).filter(Boolean);
  if (normalized.length === 0) {
    return null;
  }
  return (
    <ul className="workspace-draft-info-list">
      {normalized.map((item, index) => (
        <li key={`${index}-${item}`}>
          <Typography.Text>{item}</Typography.Text>
        </li>
      ))}
    </ul>
  );
}

export function hasWorkspaceSetupDraftInfoItems(items?: string[]) {
  return (items ?? []).some((item) => item.trim());
}

export function workspaceSetupDraftReadyForApply(draft?: WorkspaceSetupDraft) {
  return Boolean(draft?.goalText?.trim() && (draft.doDItems?.some((item) => item.trim()) ?? false));
}

export function splitLines(value?: string) {
  return (value || '').split('\n').map((item) => item.trim()).filter(Boolean);
}

export function joinLines(values?: string[]) {
  return (values ?? []).join('\n');
}

export function splitHeaderLines(value?: string) {
  const headers: Record<string, string> = {};
  for (const line of splitLines(value)) {
    const separatorIndex = line.includes(':') ? line.indexOf(':') : line.indexOf('=');
    if (separatorIndex <= 0) {
      continue;
    }
    const key = line.slice(0, separatorIndex).trim();
    const headerValue = line.slice(separatorIndex + 1).trim();
    if (key) {
      headers[key] = headerValue;
    }
  }
  return headers;
}

export function validateHeaderLines(value?: string) {
  for (const line of splitLines(value)) {
    const separatorIndex = line.includes(':') ? line.indexOf(':') : line.indexOf('=');
    const key = separatorIndex >= 0 ? line.slice(0, separatorIndex).trim() : '';
    if (separatorIndex <= 0 || !key) {
      throw new Error(`Invalid header line: ${line}`);
    }
  }
}

export function joinHeaderLines(headers?: Record<string, string>) {
  return Object.entries(headers ?? {}).map(([key, value]) => `${key}: ${value}`).join('\n');
}

export function splitDefinitionList(value?: string) {
  return (value || '').split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
}

export type DoDListContext = {
  indent: number;
  text: string;
};

export function splitDoDRawLines(value?: string) {
  return (value || '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .flatMap((line) => {
      if (!shouldSplitDoDInline(line)) {
        return [line];
      }
      return line.split(/[;；]/);
    });
}

export function shouldSplitDoDInline(line: string) {
  if (!/[;；]/.test(line)) {
    return false;
  }
  const trimmed = line.trim();
  if (!trimmed || /^#{1,6}\s+/.test(trimmed)) {
    return false;
  }
  return !stripDoDListMarker(trimmed).isListItem;
}

export function stripDoDListMarker(line: string): { text: string; isListItem: boolean } {
  const trimmed = line.trim();
  if (/^[-*+•]\s+/.test(trimmed)) {
    return { text: stripDoDCheckbox(trimmed.slice(2).trim()), isListItem: true };
  }
  const ordered = trimmed.match(/^\d+[\.)、]\s+/);
  if (ordered) {
    return { text: stripDoDCheckbox(trimmed.slice(ordered[0].length).trim()), isListItem: true };
  }
  if (/^\[\s\]/.test(trimmed) || /^\[[xX]\]/.test(trimmed)) {
    return { text: stripDoDCheckbox(trimmed), isListItem: true };
  }
  return { text: trimmed, isListItem: false };
}

export function stripDoDCheckbox(line: string) {
  return line.replace(/^\[\s\]\s*/, '').replace(/^\[[xX]\]\s*/, '').trim();
}

export function cleanDoDInlineMarkdown(value: string) {
  return value.replace(/\*\*|__|`/g, '').trim().split(/\s+/).filter(Boolean).join(' ');
}

export function stripDoDOrdinal(value: string) {
  return value.replace(/^\d+(?:\.\d+)*(?:[\.)、])?\s+/, '').trim();
}

export function trimDoDTrailingColon(value: string) {
  return value.replace(/[\s:：]+$/, '').trim();
}

export function hasDoDInlineLabel(value: string) {
  const index = value.search(/[:：]/);
  if (index <= 0 || index >= value.length - 1) {
    return false;
  }
  return value.slice(0, index).trim().length > 0 && Array.from(value.slice(0, index).trim()).length <= 40;
}

export function isDoDSameLevelContinuation(isListItem: boolean, value: string) {
  return isListItem && !/[:：]$/.test(value) && !hasDoDInlineLabel(value);
}

export function doDLeadingIndent(line: string) {
  let indent = 0;
  for (const char of line) {
    if (char === ' ') {
      indent += 1;
    } else if (char === '\t') {
      indent += 4;
    } else {
      break;
    }
  }
  return indent;
}

export function doDWithContext(context: string[], item: string) {
  const cleanItem = item.trim();
  if (context.length === 0) {
    return cleanItem;
  }
  const prefix = context.join(' / ');
  if (cleanItem.startsWith(prefix)) {
    return cleanItem;
  }
  return `${prefix}： ${cleanItem}`;
}

export function setupDoDItems(value?: string) {
  const headings: string[] = [];
  const listContexts: DoDListContext[] = [];
  const seen = new Set<string>();
  const items: string[] = [];

  for (const line of splitDoDRawLines(value)) {
    const trimmed = line.trim();
    if (!trimmed || /^[-_*]{3,}$/.test(trimmed)) {
      continue;
    }

    const heading = trimmed.match(/^(#{1,6})\s+(.+)$/);
    if (heading) {
      const level = heading[1].length;
      const text = stripDoDOrdinal(cleanDoDInlineMarkdown(heading[2]));
      if (text) {
        headings[level - 1] = text;
        headings.length = level;
        listContexts.length = 0;
      }
      continue;
    }

    const indent = doDLeadingIndent(line);
    const stripped = stripDoDListMarker(trimmed);
    const content = cleanDoDInlineMarkdown(stripped.text);
    if (!content) {
      continue;
    }
    const keepSameLevel = isDoDSameLevelContinuation(stripped.isListItem, content);
    while (
      listContexts.length > 0 &&
      (listContexts[listContexts.length - 1].indent > indent ||
        (listContexts[listContexts.length - 1].indent === indent && !keepSameLevel))
    ) {
      listContexts.pop();
    }
    if (stripped.isListItem && /[:：]$/.test(content)) {
      const parent = trimDoDTrailingColon(content);
      if (parent) {
        listContexts.push({ indent, text: parent });
      }
      continue;
    }
    const context = [
      ...headings.filter(Boolean),
      ...listContexts.map((item) => item.text).filter(Boolean)
    ];
    const normalized = doDWithContext(context, content);
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    items.push(normalized);
  }

  return items;
}

export function workspaceSetupDraftFromForms(goalText?: string, doDText?: string): WorkspaceSetupDraft {
  return {
    goalText: (goalText || '').trim(),
    doDItems: setupDoDItems(doDText)
  };
}

export function hasWorkspaceSetupDraftContent(draft?: WorkspaceSetupDraft) {
  return Boolean(
    draft &&
    (
      draft.goalText?.trim() ||
      (draft.doDItems?.length ?? 0) > 0 ||
      (draft.assumptions?.length ?? 0) > 0 ||
      (draft.clarifyingQuestions?.length ?? 0) > 0 ||
      (draft.warnings?.length ?? 0) > 0
    )
  );
}

export function workspaceSetupDraftMessageLabel(role?: string) {
  switch ((role || '').trim()) {
    case 'assistant':
      return '草案助手';
    case 'system':
      return '系统';
    default:
      return '用户';
  }
}

export const workspaceSetupSmartDraftStep = 5;

export type SoulBindingFormValues = {
  defaultSoulId?: string;
  bindings?: Record<string, string | undefined>;
};

export type WorkspaceSetupMode = 'manual' | 'smart';

export type WorkspaceSetupFormValues = {
  setupMode?: WorkspaceSetupMode;
  templateId?: string;
  title: string;
  description?: string;
  deliverableDirectory?: string;
}

export type GoalSetupFormValues = {
  text: string;
};

export type DoDSetupFormValues = {
  items: string;
};

export type WorkspaceDeleteFormValues = {
  purge?: boolean;
};

export function normalizeSoulBindingFormValues(values: SoulBindingFormValues): WorkspaceSoulBindings {
  const defaultSoulId = values.defaultSoulId?.trim() || undefined;
  const bindings = Object.fromEntries(
    Object.entries(values.bindings ?? {})
      .map(([stage, soulId]) => [stage, soulId?.trim() || ''])
      .filter(([, soulId]) => Boolean(soulId))
  );
  return {
    defaultSoulId,
    bindings
  };
}

export function stageBindings(stages: string[], soulId?: string) {
  const value = soulId?.trim();
  if (!value) {
    return {};
  }
  return Object.fromEntries(stages.map((stage) => [stage, value]));
}

export type SoulDefinitionFormValues = {
  id?: string;
  name?: string;
  description?: string;
  businessDomains?: string;
  soulMd?: string;
  allowedSkillRefs?: string;
  allowedMCPServerRefs?: string;
  allowedToolKinds?: string;
};

export function soulDefinitionPayload(values: SoulDefinitionFormValues, soulID: string): Soul {
  return {
    id: soulID,
    name: (values.name || '').trim(),
    description: (values.description || '').trim(),
    businessDomains: splitDefinitionList(values.businessDomains),
    soulMd: (values.soulMd || '').trim(),
    allowedSkillRefs: splitDefinitionList(values.allowedSkillRefs),
    allowedMCPServerRefs: splitDefinitionList(values.allowedMCPServerRefs),
    allowedToolKinds: splitDefinitionList(values.allowedToolKinds)
  };
}

export type WorkspaceTemplateFormValues = {
  id?: string;
  name?: string;
  description?: string;
  goalText?: string;
  doDItems?: string;
  defaultSoulId?: string;
  soulBindings?: Record<string, string | undefined>;
  contextHintsJSON?: string;
  evidencePoliciesJSON?: string;
};

export function workspaceTemplatePayload(values: WorkspaceTemplateFormValues, templateID: string): WorkspaceTemplate {
  const bindings = normalizeSoulBindingFormValues({
    defaultSoulId: values.defaultSoulId,
    bindings: values.soulBindings
  });
  return {
    id: templateID,
    name: (values.name || '').trim(),
    description: (values.description || '').trim(),
    goalText: (values.goalText || '').trim(),
    doDItems: setupDoDItems(values.doDItems),
    defaultSoulId: bindings.defaultSoulId,
    soulBindings: bindings.bindings,
		contextHints: workspaceContractJSONObject(values.contextHintsJSON, 'Context Hints'),
		evidencePolicies: workspaceContractJSONArray(values.evidencePoliciesJSON, 'Evidence Policies')
  };
}

function workspaceContractJSON(value: string | undefined, label: string): unknown {
  const source = (value || '').trim();
  if (!source) {
    return undefined;
  }
  try {
    return JSON.parse(source);
  } catch {
    throw new Error(`${label} 必须是有效 JSON`);
  }
}

export function workspaceContractJSONObject(value: string | undefined, label: string): Record<string, unknown> | undefined {
  const parsed = workspaceContractJSON(value, label);
  if (parsed === undefined) {
    return undefined;
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${label} 必须是 JSON 对象`);
  }
  return parsed as Record<string, unknown>;
}

export function workspaceContractJSONArray(value: string | undefined, label: string): Record<string, unknown>[] | undefined {
  const parsed = workspaceContractJSON(value, label);
  if (parsed === undefined) {
    return undefined;
  }
  if (!Array.isArray(parsed) || parsed.some((item) => !item || Array.isArray(item) || typeof item !== 'object')) {
    throw new Error(`${label} 必须是 JSON 对象数组`);
  }
  return parsed as Record<string, unknown>[];
}

function workspaceTemplateJSONText(value: unknown): string {
  if (value === undefined || value === null) {
    return '';
  }
  return JSON.stringify(value, null, 2);
}

export function metricValue(metrics: string, name: string) {
  const line = metrics.split('\n').find((item) => item.startsWith(`${name} `));
  if (!line) {
    return '-';
  }
  const value = line.trim().split(/\s+/).at(-1);
  return value || '-';
}

export function runtimeTarget(output?: Record<string, unknown>) {
  if (!output) {
    return '-';
  }
  return outputText(output, 'tool') !== '-' ? outputText(output, 'tool') :
    outputText(output, 'uri') !== '-' ? outputText(output, 'uri') :
      outputText(output, 'prompt');
}

export function checkpointTypeLabel(type?: string) {
  const labels: Record<string, string> = {
    run_state: '运行状态',
    runtime_step: '运行步骤',
    graph_state: '执行图',
    validation: '验收校验'
  };
  return type ? labels[type] ?? type : '-';
}

export type LLMConfigurationFormValues = UpdateLLMConfigurationRequest;

export type ProviderModelTargetOption = {
  value: string;
  label: string;
  providerId: string;
  modelId: string;
  source: 'catalog' | 'configured';
  ready: boolean;
  disabled?: boolean;
  reasoningSupported?: boolean;
  reasoningValid?: boolean;
  defaultReasoningEffort?: string;
  reasoningEfforts?: string[];
};

export function providerModelTargetKey(providerId?: string, modelId?: string) {
  const normalizedProviderId = providerId?.trim();
  const normalizedModelId = modelId?.trim();
  if (!normalizedProviderId || !normalizedModelId) {
    return undefined;
  }
  return `${encodeURIComponent(normalizedProviderId)}/${encodeURIComponent(normalizedModelId)}`;
}

export function llmConfigurationFormValues(configuration: ServiceConfiguration): LLMConfigurationFormValues {
  return {
    apiKey: undefined,
    defaultChatModelId: configuration.llm.defaultChat?.modelId,
    defaultEmbeddingModelId: configuration.llm.defaultEmbedding?.modelId,
    budget: {
      contextUtilizationRatio: configuration.llm.budget?.contextUtilizationRatio,
      estimatedTokensPerByte: configuration.llm.budget?.estimatedTokensPerByte
    }
  };
}

export function readyCatalogModels(configuration?: ServiceConfiguration, operation?: string) {
  const models = (configuration?.llm.catalog?.models ?? []).filter((item) => {
    const providerId = item.providerId?.trim() || configuration?.llm.catalog?.providerId?.trim() || 'ai-proxy';
    if (providerId !== 'ai-proxy') {
      return false;
    }
    if (!item.capabilityReady) {
      return false;
    }
    const ops = item.operations ?? [];
    if (ops.length === 0) {
      return false;
    }
    if (!operation) {
      return true;
    }
    return ops.includes(operation);
  });
  return models;
}

export function readyCatalogTargetOptions(configuration?: ServiceConfiguration, operation?: string): ProviderModelTargetOption[] {
  return readyCatalogModels(configuration, operation).flatMap((item) => {
    const providerId = 'ai-proxy';
    const modelId = item.modelId?.trim();
    const value = providerModelTargetKey(providerId, modelId);
    if (!value || !modelId) {
      return [];
    }
    return [{
      value,
      label: modelId,
      providerId,
      modelId,
      source: 'catalog' as const,
      ready: true,
      reasoningSupported: item.reasoning?.supported,
      reasoningValid: item.reasoning?.valid,
      defaultReasoningEffort: item.reasoning?.defaultEffort,
      reasoningEfforts: item.reasoning?.efforts
    }];
  });
}

export function llmModelOptions(
  configuration: ServiceConfiguration | undefined,
  operation: string,
  configuredModelId: string | undefined
): ProviderModelTargetOption[] {
  const options = readyCatalogTargetOptions(configuration, operation).map((item) => ({
    ...item,
    value: item.modelId
  }));
  const unique = new Map(options.map((item) => [item.value, item]));
  const modelId = configuredModelId?.trim();
  if (modelId && !unique.has(modelId)) {
    unique.set(modelId, {
      value: modelId,
      label: `${modelId}（已配置但未 ready）`,
      providerId: 'ai-proxy',
      modelId,
      source: 'configured',
      ready: false,
      disabled: true
    });
  }
  return Array.from(unique.values());
}
export function chatTargetOptions(configuration?: ServiceConfiguration) {
  return readyCatalogTargetOptions(configuration, 'responses').map((item) => ({
    value: item.value,
    label: item.label,
    providerId: item.providerId,
    modelId: item.modelId,
    source: 'catalog' as const,
    reasoningSupported: item.reasoningSupported,
    reasoningValid: item.reasoningValid,
    defaultReasoningEffort: item.defaultReasoningEffort,
    reasoningEfforts: item.reasoningEfforts
  }));
}

// Resolve the concrete chat target carried by every Web activation request.
// A configured default is usable only when it is present in the ready catalog
// in the configured Responses catalog. If it is stale, choose
// the first ready target instead of sending the stale default through hidden
// form fields or relying on backend fallback.
export function activationReadyChatTarget(configuration?: ServiceConfiguration) {
  const options = chatTargetOptions(configuration);
  const defaultProviderId = configuration?.llm.defaultChat?.providerId?.trim() || 'ai-proxy';
  const defaultModelId = configuration?.llm.defaultChat?.modelId?.trim();
  const selected = defaultModelId
    ? options.find((item) => item.providerId === defaultProviderId && item.modelId === defaultModelId)
    : undefined;
  const target = selected ?? options[0];
  if (!target) {
    return undefined;
  }
  return { providerId: target.providerId, modelId: target.modelId };
}





export type WebSearchProviderFormValues = Omit<Partial<UpdateWebSearchProviderConfiguration>, 'allowDomains' | 'denyDomains' | 'headers'> & {
  allowDomains?: string;
  denyDomains?: string;
  headers?: string;
};

export function webSearchConfigurationFormValues(configuration: ServiceConfiguration): UpdateWebSearchConfigurationRequest {
  return {
    defaultProviderId: configuration.webSearch.defaultProviderId,
    providers: (configuration.webSearch.providers ?? []).map((item) => ({
      id: item.id,
      provider: item.provider,
      searchEndpoint: item.searchEndpoint,
      headers: item.headers ?? {},
      apiKey: '',
      timeout: item.timeout,
      maxResults: item.maxResults,
      maxFetchBytes: item.maxFetchBytes,
      maxRedirects: item.maxRedirects,
      allowDomains: item.allowDomains ?? [],
      denyDomains: item.denyDomains ?? [],
      disabled: item.disabled
    }))
  };
}

export function webSearchProviderFormValues(provider?: UpdateWebSearchProviderConfiguration): WebSearchProviderFormValues {
  return {
    ...provider,
    allowDomains: joinLines(provider?.allowDomains),
    denyDomains: joinLines(provider?.denyDomains),
    headers: joinHeaderLines(provider?.headers)
  };
}

export function webSearchProviderDraftFromTemplate(template: WebSearchProviderTemplate, existing: UpdateWebSearchProviderConfiguration[], current?: UpdateWebSearchProviderConfiguration): UpdateWebSearchProviderConfiguration {
  let id = current?.id?.trim() || template.id;
  const currentID = current?.id?.trim();
  const existingIDs = new Set(existing.map((item) => item.id?.trim()).filter((item) => item && item !== currentID));
  if (existingIDs.has(id)) {
    let index = existing.length + 1;
    id = `${template.provider}-${index}`;
    while (existingIDs.has(id)) {
      index += 1;
      id = `${template.provider}-${index}`;
    }
  }
  return {
    ...current,
    id,
    provider: template.provider,
    searchEndpoint: template.searchEndpoint ?? current?.searchEndpoint,
    timeout: template.timeout || current?.timeout || '20s',
    maxResults: template.maxResults ?? current?.maxResults ?? 8,
    maxFetchBytes: template.maxFetchBytes ?? current?.maxFetchBytes ?? 262144,
    maxRedirects: template.maxRedirects ?? current?.maxRedirects ?? 3,
    disabled: current?.disabled === true,
    apiKey: current?.apiKey ?? ''
  };
}

export function nextWebSearchProviderDraft(existing: UpdateWebSearchProviderConfiguration[], template?: WebSearchProviderTemplate): UpdateWebSearchProviderConfiguration {
  if (template) {
    return webSearchProviderDraftFromTemplate(template, existing);
  }
  let index = existing.length + 1;
  let id = `web-provider-${index}`;
  const existingIDs = new Set(existing.map((item) => item.id));
  while (existingIDs.has(id)) {
    index += 1;
    id = `web-provider-${index}`;
  }
  return {
    id,
    provider: 'http',
    timeout: '20s',
    maxResults: 8,
    maxFetchBytes: 262144,
    maxRedirects: 3,
    disabled: false
  };
}

export function SoulDefinitionManager({ souls, onSoulsChange }: { souls: Soul[]; onSoulsChange: (souls: Soul[]) => void }) {
  const { message, modal } = AntApp.useApp();
  const [form] = Form.useForm<SoulDefinitionFormValues>();
  const [editingSoul, setEditingSoul] = useState<Soul | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [deletingSoulId, setDeletingSoulId] = useState<string | undefined>(undefined);

  const editSoul = (soul?: Soul) => {
    setEditingSoul(soul);
    form.setFieldsValue({
      id: soul?.id || '',
      name: soul?.name || '',
      description: soul?.description || '',
      businessDomains: joinLines(soul?.businessDomains),
      soulMd: soul?.soulMd || '',
      allowedSkillRefs: joinLines(soul?.allowedSkillRefs),
      allowedMCPServerRefs: joinLines(soul?.allowedMCPServerRefs),
      allowedToolKinds: joinLines(soul?.allowedToolKinds)
    });
  };

  const saveSoul = async (values: SoulDefinitionFormValues) => {
    const soulID = (editingSoul?.id || values.id || '').trim();
    if (!soulID) {
      message.error('请填写执行人格 ID');
      return;
    }
    setSaving(true);
    try {
      const payload = soulDefinitionPayload(values, soulID);
      const saved = editingSoul
        ? await workorchApi.updateSoul(soulID, payload)
        : await workorchApi.createSoul(payload);
      const nextSouls = await workorchApi.listSouls();
      onSoulsChange(nextSouls);
      editSoul(saved);
      message.success(editingSoul ? '执行人格定义已更新' : '执行人格定义已创建');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const deleteSoul = (soul: Soul) => {
    modal.confirm({
      title: `删除${modelLabels.soul}`,
      content: `确认删除「${soul.name || soul.id}」？删除后，仍引用该执行人格的工作空间或模板需要重新调整执行配置。`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setDeletingSoulId(soul.id);
        try {
          await workorchApi.deleteSoul(soul.id);
          const nextSouls = await workorchApi.listSouls();
          onSoulsChange(nextSouls);
          if (editingSoul?.id === soul.id) {
            editSoul(undefined);
          }
          message.success('执行人格定义已删除');
        } catch (error) {
          message.error(errorMessage(error));
          throw error;
        } finally {
          setDeletingSoulId(undefined);
        }
      }
    });
  };

  return (
    <ProCard title={modelLabels.soul} className="workorch-section">
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          执行人格（Soul）是系统级执行配置，定义人格、阶段策略和输出契约；技能（Skill）、MCP 服务和工具（Tool）权限必须通过结构化字段显式维护。
        </Typography.Text>
        <CardGrid>
          <ProCard colSpan={{ xs: 24, lg: 8 }} ghost>
            <List
              size="small"
              dataSource={souls}
              locale={{ emptyText: <Empty description="暂无执行人格定义" /> }}
              renderItem={(item) => (
                <List.Item
                  actions={[
                    <Button key="edit" type="link" size="small" onClick={() => editSoul(item)}>编辑</Button>,
                    <Button key="delete" type="link" danger size="small" loading={deletingSoulId === item.id} onClick={() => deleteSoul(item)}>删除</Button>
                  ]}
                >
                  <List.Item.Meta
                    title={<Typography.Text strong>{item.name || item.id}</Typography.Text>}
                    description={
                      <Space direction="vertical" size={2}>
                        <Typography.Text type="secondary">{item.id}</Typography.Text>
                        {compactTags(item.businessDomains ?? [], 2)}
                      </Space>
                    }
                  />
                </List.Item>
              )}
            />
            <Button style={{ marginTop: 12 }} onClick={() => editSoul(undefined)}>新增执行人格</Button>
          </ProCard>
          <ProCard colSpan={{ xs: 24, lg: 16 }} ghost>
            <Form form={form} layout="vertical" onFinish={(values) => void saveSoul(values)}>
              <Form.Item name="id" label="执行人格 ID" rules={[{ required: !editingSoul, message: '请输入执行人格 ID' }]}>
                <Input disabled={Boolean(editingSoul)} placeholder="例如 soul-engineering-delivery" />
              </Form.Item>
              <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
                <Input placeholder="例如 Engineering Delivery" />
              </Form.Item>
              <Form.Item name="description" label="描述">
                <Input.TextArea rows={2} placeholder="说明该执行人格的适用场景" />
              </Form.Item>
              <Form.Item name="businessDomains" label="业务域">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔，例如 engineering, validation" />
              </Form.Item>
              <Form.Item name="soulMd" label="soul.md" rules={[{ required: true, message: '请输入执行人格（Soul）定义内容' }]}>
                <Input.TextArea rows={8} placeholder="# Soul\n\n定义人格、阶段策略和输出契约" />
              </Form.Item>
              <Form.Item name="allowedSkillRefs" label="允许技能（Skill）">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔" />
              </Form.Item>
              <Form.Item name="allowedMCPServerRefs" label="允许 MCP 服务（MCP Server）">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔" />
              </Form.Item>
              <Form.Item name="allowedToolKinds" label="允许工具类型（Tool Kind）">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔，例如 llm/chat" />
              </Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" loading={saving}>
                  {editingSoul ? '保存执行人格' : '创建执行人格'}
                </Button>
                <Button onClick={() => editSoul(undefined)}>清空</Button>
              </Space>
            </Form>
          </ProCard>
        </CardGrid>
      </Space>
    </ProCard>
  );
}

export function WorkspaceTemplateManager({ souls, templates, onTemplatesChange }: { souls: Soul[]; templates: WorkspaceTemplate[]; onTemplatesChange: (templates: WorkspaceTemplate[]) => void }) {
  const { message, modal } = AntApp.useApp();
  const [form] = Form.useForm<WorkspaceTemplateFormValues>();
  const [editingTemplate, setEditingTemplate] = useState<WorkspaceTemplate | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [deletingTemplateId, setDeletingTemplateId] = useState<string | undefined>(undefined);
  const stages = defaultSoulBindingStages;
  const soulOptions = souls.map((item) => ({ value: item.id, label: `${item.name || item.id} (${item.id})` }));

  const editTemplate = (template?: WorkspaceTemplate) => {
    setEditingTemplate(template);
    form.setFieldsValue({
      id: template?.id || '',
      name: template?.name || '',
      description: template?.description || '',
      goalText: template?.goalText || '',
      doDItems: joinLines(template?.doDItems),
      defaultSoulId: template?.defaultSoulId,
      soulBindings: template?.soulBindings ?? {},
      contextHintsJSON: workspaceTemplateJSONText(template?.contextHints),
      evidencePoliciesJSON: workspaceTemplateJSONText(template?.evidencePolicies)
    });
  };

  const applyDefaultSoulToStages = () => {
    const defaultSoulId = form.getFieldValue('defaultSoulId');
    form.setFieldsValue({ soulBindings: stageBindings(stages, defaultSoulId) });
  };

  const saveTemplate = async (values: WorkspaceTemplateFormValues) => {
    const templateID = (editingTemplate?.id || values.id || '').trim();
    if (!templateID) {
      message.error('请填写模板 ID');
      return;
    }
    setSaving(true);
    try {
      const payload = workspaceTemplatePayload(values, templateID);
      const saved = editingTemplate
        ? await workorchApi.updateWorkspaceTemplate(templateID, payload)
        : await workorchApi.createWorkspaceTemplate(payload);
      const nextTemplates = await workorchApi.listWorkspaceTemplates();
      onTemplatesChange(nextTemplates);
      editTemplate(saved);
      message.success(editingTemplate ? '工作空间模板已更新' : '工作空间模板已创建');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const deleteTemplate = (template: WorkspaceTemplate) => {
    modal.confirm({
      title: '删除工作空间模板',
      content: `确认删除「${template.name || template.id}」？删除模板不会删除已创建的工作空间。`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setDeletingTemplateId(template.id);
        try {
          await workorchApi.deleteWorkspaceTemplate(template.id);
          const nextTemplates = await workorchApi.listWorkspaceTemplates();
          onTemplatesChange(nextTemplates);
          if (editingTemplate?.id === template.id) {
            editTemplate(undefined);
          }
          message.success('工作空间模板已删除');
        } catch (error) {
          message.error(errorMessage(error));
        } finally {
          setDeletingTemplateId(undefined);
        }
      }
    });
  };

  return (
    <ProCard title="工作空间模板（Workspace Template）" className="workorch-section">
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          模板用于复用新建工作空间时的目标（Goal）、验收标准（DoD）、默认执行人格（Soul）与阶段执行人格绑定。高级合同字段以 JSON 原样保存；它们只在下一次显式 restart 的 activation 编译时生效，不会修改已有 Run。
        </Typography.Text>
        <CardGrid>
          <ProCard colSpan={{ xs: 24, lg: 8 }} ghost>
            <List
              size="small"
              dataSource={templates}
              locale={{ emptyText: <Empty description="暂无工作空间模板" /> }}
              renderItem={(item) => (
                <List.Item actions={[
                  <Button key="edit" type="link" size="small" onClick={() => editTemplate(item)}>编辑</Button>,
                  <Button key="delete" type="link" danger size="small" loading={deletingTemplateId === item.id} onClick={() => deleteTemplate(item)}>删除</Button>
                ]}>
                  <List.Item.Meta
                    title={<Typography.Text strong>{item.name || item.id}</Typography.Text>}
                    description={
                      <Space direction="vertical" size={2}>
                        <Typography.Text type="secondary">{item.id}</Typography.Text>
                        <Space wrap size={[4, 4]}>
                          {item.goalText ? <Tag color="blue">{modelLabels.goal}</Tag> : null}
                          {(item.doDItems?.length ?? 0) > 0 ? <Tag color="green">{modelLabels.dod} {item.doDItems?.length}</Tag> : null}
                          {item.defaultSoulId ? <Tag color="cyan">默认执行人格</Tag> : null}
                        </Space>
                      </Space>
                    }
                  />
                </List.Item>
              )}
            />
            <Button style={{ marginTop: 12 }} onClick={() => editTemplate(undefined)}>新增模板</Button>
          </ProCard>
          <ProCard colSpan={{ xs: 24, lg: 16 }} ghost>
            <Form form={form} layout="vertical" onFinish={(values) => void saveTemplate(values)}>
              <Form.Item name="id" label="模板 ID" rules={[{ required: !editingTemplate, message: '请输入模板 ID' }]}>
                <Input disabled={Boolean(editingTemplate)} placeholder="例如 workspace-template-delivery" />
              </Form.Item>
              <Form.Item name="name" label="模板名称" rules={[{ required: true, message: '请输入模板名称' }]}>
                <Input placeholder="例如 软件交付验证模板" />
              </Form.Item>
              <Form.Item name="description" label="描述">
                <Input.TextArea rows={2} placeholder="说明模板适用场景" />
              </Form.Item>
              <Form.Item name="goalText" label="默认目标（Goal）">
                <Input.TextArea rows={3} placeholder="创建工作空间时自动带入，可在向导中修改" />
              </Form.Item>
              <Form.Item name="doDItems" label="默认验收标准（DoD）">
                <Input.TextArea rows={4} placeholder={'每行一个验收标准\n创建工作空间时自动带入，可在向导中修改'} />
              </Form.Item>
              <Form.Item name="defaultSoulId" label="默认执行人格（Soul）">
                <Select allowClear placeholder="不设置默认执行人格" options={soulOptions} />
              </Form.Item>
              <Space className="workorch-section">
                <Typography.Text type="secondary">阶段执行人格绑定</Typography.Text>
                <Button size="small" onClick={applyDefaultSoulToStages}>默认执行人格应用到全部阶段</Button>
              </Space>
              <div className="soul-binding-grid">
                {stages.map((stage) => (
                  <Form.Item key={stage} name={['soulBindings', stage]} label={soulBindingStageLabel(stage)}>
                    <Select allowClear placeholder="继承默认执行人格" options={soulOptions} />
                  </Form.Item>
                ))}
              </div>
              <Form.Item name="contextHintsJSON" label="执行合同 Context Hints（JSON）" extra="在 profileContracts 中为每个需要语义证明的 CWI 声明精确 target/selector、允许操作、validationRecipeId 与 requiredCheckRefs。通配符规则只作为质量门禁，不能关闭 CWI。">
                <Input.TextArea rows={10} spellCheck={false} placeholder={'{\n  "profileContracts": {\n    "your-profile": {\n      "evidenceRequirements": []\n    }\n  }\n}'} />
              </Form.Item>
              <Form.Item name="evidencePoliciesJSON" label="Validation Recipes / Evidence Policies（JSON）" extra="为 managed Shell 验证逐项声明 recipeDeclared、recipeArgs 与对应 CWI 的 validationRecipeId；不要以共享 recipe 关闭多个 CWI。">
                <Input.TextArea rows={8} spellCheck={false} placeholder={'[\n  {\n    "id": "cwi-validation",\n    "recipeDeclared": true\n  }\n]'} />
              </Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" loading={saving}>
                  {editingTemplate ? '保存模板' : '创建模板'}
                </Button>
                <Button onClick={() => editTemplate(undefined)}>清空</Button>
              </Space>
            </Form>
          </ProCard>
        </CardGrid>
      </Space>
    </ProCard>
  );
}

export type ControlPlaneProps = {
  selectedWorkspaceId?: string;
  onOpenWorkspace: (workspaceId: string) => void;
};

export type WorkspaceBoardItem = {
  snapshot: WorkspaceSnapshot;
};

export function readyDoDs(dods: DoD[]) {
  return dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
}

export function workspaceBoardActivationRequest(item: WorkspaceBoardItem): WorkspaceExecutionIntentRequest | undefined {
  const goals = item.snapshot.goals ?? [];
  const availableDoDs = readyDoDs(item.snapshot.dods ?? []);
  if (goals.length > 0 && availableDoDs.length > 0) {
    const intent = workspaceExecutionPrimaryIntent(item.snapshot);
    return workspaceBoardExecutionIntentAllowed(item.snapshot, intent) ? { intent } : undefined;
  }

  return undefined;
}

export function workspaceBoardActivationHint(item: WorkspaceBoardItem) {
  const goals = item.snapshot.goals ?? [];
  const availableDoDs = readyDoDs(item.snapshot.dods ?? []);
  if (goals.length === 0) {
    return `当前工作空间尚未定义目标，请进入工作空间补齐${modelLabels.goal}后再提交执行`;
  }
  if (availableDoDs.length === 0) {
    return `当前工作空间尚无可用验收标准，请进入工作空间补齐${modelLabels.dod}后再提交执行`;
  }
  if (workspaceDefinitionClarification(item.snapshot)) {
    return '当前正在等待 Definition 澄清；Board 不提交 Resume/Restart，请进入工作空间修订 Goal/DoD，并从详情页显式 Restart 交由 Workspace owner 判定';
  }
  if (canonicalActiveRunIsGovernanceBlocked(item.snapshot)) {
    return '当前运行正在等待 Decision/Approval owner 收口；请处理治理阻塞并等待 continuation 自动收敛，不要提交 Resume';
  }
  return '当前工作空间无法形成有效执行定义，请进入工作空间核对后再提交执行';
}
