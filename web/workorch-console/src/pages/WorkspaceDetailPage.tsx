import { useEffect, useRef, useState } from 'react';
import { Alert, App as AntApp, Button, Checkbox, Descriptions, Dropdown, Empty, FloatButton, Form, Input, List, Modal, Select, Space, Steps, Tabs, Tag, Typography } from 'antd';
import { CardGrid, PageContainer, ProCard, ProTable } from '../components/proCompat';
import type { ProColumns } from '../components/proCompat';
import { DownOutlined } from '@ant-design/icons';
import type { Approval, Artifact, ArtifactContinuation, ArtifactPage, ContextBundle, Decision, DoD, EventRecord, Goal, GoalDoDQualityCheck, MCPServer, MemoryItem, Patch, PatchPage, ProjectionStatus, Result, ResultPage, Run, RunObservability, RunSnapshot, RuntimeCheckpoint, RuntimeCheckpointPage, RuntimeTimelineItem, ServiceConfiguration, Skill, Soul, StepPage, StepView, ToolCall, ToolCallPage, ValidationResult, ValidationResultPage, Workspace, WorkspaceExecutionActivationRequest, WorkspaceSnapshot, WorkspaceSoulBindings } from '../types';
import { workorchApi } from '../api';
import { useSSE } from '../hooks';
import { modelLabels } from '../utils/constants';
import * as helpers from '../helpers';
import { canonicalActiveRunIsGovernanceBlocked, canonicalDefinitionClarification, workspaceDefinitionClarification, workspaceExecutionIntentAllowed } from '../governanceMutationTarget';

type ControlPlaneProps = helpers.ControlPlaneProps;
type WorkspaceDeleteFormValues = helpers.WorkspaceDeleteFormValues;

const {
  acceptanceCard,
  boundedParagraph, checkpointTypeLabel, compactJSON, compactTags, contextBundleCard,
  dodAssessmentItems, errorMessage, eventPayloadDisplay, eventRecordState, eventRecordSubject, eventRecordSummary,
  evidenceCount, executionSectionsCard, gapBaselineCard, goalOutcomeState, goalOutcomeSummary,
  isNotFoundError,
  llmContextBudgetCard, llmUsageCard, memoryRecallCard,
  memoryRefIDs, mergeById, metricGrid, numberText, percentText,
  outputText, pendingCount, primaryAcceptedResult, primaryResult,
	runOutcomeState, runRawState, runStateSummary, runtimeArtifactPageLimit,
  runtimeCapabilityTable, runtimeCheckpointPageLimit, runtimeDetailActions,
  runtimeEventPageLimit, runtimePatchPageLimit, runtimeRawRefActions, runtimeResultPageLimit,
  runtimeStepPageLimit, runtimeTarget, runtimeToolCallPageLimit, runtimeValidationPageLimit,
  runCommandDescriptions, soulBindingStageLabel, splitLines, stageBindings,
  projectionStatusDescriptions, projectionStatusSummary, runHeartbeatDescriptions,
  convergenceStatusDescriptions,
  stateLabel, stateTag, taskPlanCard, taskProgressCard, timelineLabel,
  timelineOccurredAtContent, timelineStreamContent, webSourceCard, webSourceRows,
  workspaceProcessItems, workspaceProjectionRunLabel, defaultSoulBindingStages
} = helpers;

function providerModelTargetLabel(selection?: { target?: { providerId?: string; modelId?: string }; reasoningEffort?: string }): string {
  const target = selection?.target;
  const modelID = target?.modelId?.trim();
  if (!modelID) {
    return '-';
  }
  return selection?.reasoningEffort ? `${modelID} · reasoning ${selection.reasoningEffort}` : modelID;
}

export function WorkspaceDetailPage({ selectedWorkspaceId }: Pick<ControlPlaneProps, 'selectedWorkspaceId'>) {
  const { message } = AntApp.useApp();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceId, setWorkspaceId] = useState<string | undefined>(undefined);
  const [workspaceProjection, setWorkspaceProjection] = useState<ProjectionStatus | undefined>(undefined);
  const [snapshot, setSnapshot] = useState<WorkspaceSnapshot | undefined>(undefined);
  const [goals, setGoals] = useState<Goal[]>([]);
  const [dods, setDoDs] = useState<DoD[]>([]);
  const [souls, setSouls] = useState<Soul[]>([]);
  const [soulBindings, setSoulBindings] = useState<WorkspaceSoulBindings | undefined>(undefined);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [mcpServers, setMCPServers] = useState<MCPServer[]>([]);
  const [memoryItems, setMemoryItems] = useState<MemoryItem[]>([]);
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [runSnapshot, setRunSnapshot] = useState<RunSnapshot | undefined>(undefined);
  const [runObservability, setRunObservability] = useState<RunObservability | undefined>(undefined);
  const [contextBundle, setContextBundle] = useState<ContextBundle | undefined>(undefined);
  const [runEventRecords, setRunEventRecords] = useState<EventRecord[]>([]);
  const [runEventCursorAfterId, setRunEventCursorAfterId] = useState<string | undefined>(undefined);
  const [runEventHasMore, setRunEventHasMore] = useState(false);
  const [runEventLoading, setRunEventLoading] = useState(false);
  const [runStepRecords, setRunStepRecords] = useState<StepView[]>([]);
  const [runStepCursorAfterId, setRunStepCursorAfterId] = useState<string | undefined>(undefined);
  const [runStepHasMore, setRunStepHasMore] = useState(false);
  const [runStepLoading, setRunStepLoading] = useState(false);
  const [runToolCallRecords, setRunToolCallRecords] = useState<ToolCall[]>([]);
  const [runToolCallCursorAfterId, setRunToolCallCursorAfterId] = useState<string | undefined>(undefined);
  const [runToolCallHasMore, setRunToolCallHasMore] = useState(false);
  const [runToolCallLoading, setRunToolCallLoading] = useState(false);
  const [runCheckpointRecords, setRunCheckpointRecords] = useState<RuntimeCheckpoint[]>([]);
  const [runCheckpointCursorAfterId, setRunCheckpointCursorAfterId] = useState<string | undefined>(undefined);
  const [runCheckpointHasMore, setRunCheckpointHasMore] = useState(false);
  const [runCheckpointLoading, setRunCheckpointLoading] = useState(false);
  const [runResultRecords, setRunResultRecords] = useState<Result[]>([]);
  const [runResultCursorAfterId, setRunResultCursorAfterId] = useState<string | undefined>(undefined);
  const [runResultHasMore, setRunResultHasMore] = useState(false);
  const [runResultLoading, setRunResultLoading] = useState(false);
  const [runPatchRecords, setRunPatchRecords] = useState<Patch[]>([]);
  const [runPatchCursorAfterId, setRunPatchCursorAfterId] = useState<string | undefined>(undefined);
  const [runPatchHasMore, setRunPatchHasMore] = useState(false);
  const [runPatchLoading, setRunPatchLoading] = useState(false);
  const [runValidationRecords, setRunValidationRecords] = useState<ValidationResult[]>([]);
  const [runValidationCursorAfterId, setRunValidationCursorAfterId] = useState<string | undefined>(undefined);
  const [runValidationHasMore, setRunValidationHasMore] = useState(false);
  const [runValidationLoading, setRunValidationLoading] = useState(false);
  const [runArtifactRecords, setRunArtifactRecords] = useState<Artifact[]>([]);
  const [runArtifactCursorAfterId, setRunArtifactCursorAfterId] = useState<string | undefined>(undefined);
  const [runArtifactHasMore, setRunArtifactHasMore] = useState(false);
  const [runArtifactLoading, setRunArtifactLoading] = useState(false);
  const [loading, setLoading] = useState(false);
  const [activationOpen, setActivationOpen] = useState(false);
  const [activationLoading, setActivationLoading] = useState(false);
  const [activationIntent, setActivationIntent] = useState<'resume' | 'restart'>('resume');
  const [goalDefinitionOpen, setGoalDefinitionOpen] = useState(false);
  const [dodDefinitionOpen, setDoDDefinitionOpen] = useState(false);
  const [soulBindingEditorOpen, setSoulBindingEditorOpen] = useState(false);
  const [workspaceDeleteOpen, setWorkspaceDeleteOpen] = useState(false);
  const [businessSubmitting, setBusinessSubmitting] = useState(false);
  const [workspaceDeleting, setWorkspaceDeleting] = useState(false);
  const [soulBindingSaving, setSoulBindingSaving] = useState(false);
  const [editingGoal, setEditingGoal] = useState<Goal | undefined>(undefined);
  const [editingDoD, setEditingDoD] = useState<DoD | undefined>(undefined);
  const [qualityCheck, setQualityCheck] = useState<GoalDoDQualityCheck | undefined>(undefined);
  const [qualityCheckOpen, setQualityCheckOpen] = useState(false);
  const [qualityCheckLoading, setQualityCheckLoading] = useState(false);
  const [activationTracking, setActivationTracking] = useState(false);
  const [workspaceListReady, setWorkspaceListReady] = useState(false);
  const workspaceListRequestSeq = useRef(0);
  const workspaceContextRequestSeq = useRef(0);
  const currentWorkspaceIdRef = useRef<string | undefined>(undefined);
  const lastEventRevisionRef = useRef(0);
  const workspaceEventRefreshTimerRef = useRef<number | undefined>(undefined);
  const [activationForm] = Form.useForm();
	const activationSelectedTargetKey = Form.useWatch('llmTargetKey', activationForm);
  const [soulBindingsForm] = Form.useForm();
  const [workspaceDeleteForm] = Form.useForm<WorkspaceDeleteFormValues>();
  const [goalForm] = Form.useForm();
  const [dodForm] = Form.useForm();

  useEffect(() => {
    currentWorkspaceIdRef.current = workspaceId;
    lastEventRevisionRef.current = 0;
  }, [workspaceId]);

  useEffect(() => {
    setWorkspaceListReady(false);
    setLoading(true);
    void loadWorkspaceList(selectedWorkspaceId)
      .catch((error: unknown) => {
        message.error(errorMessage(error));
      })
      .finally(() => setLoading(false));
  }, [selectedWorkspaceId]);

  useEffect(() => {
    if (!workspaceListReady) {
      return;
    }
    if (!workspaceId) {
      clearWorkspaceContext();
      return;
    }
    setLoading(true);
    void loadWorkspaceContext(workspaceId).catch((error: unknown) => {
      if (isNotFoundError(error)) {
        message.warning('当前工作空间不存在，已刷新可用工作空间列表');
        void loadWorkspaceList(undefined, workspaceId).catch((refreshError: unknown) => {
          message.error(errorMessage(refreshError));
        });
      } else {
        message.error(errorMessage(error));
      }
    }).finally(() => setLoading(false));
  }, [workspaceListReady, workspaceId]);

  useSSE((event) => {
    if (!workspaceListReady) {
      return;
    }
    const selected = currentWorkspaceIdRef.current;
    if (!isWorkspaceProjectionChangedEvent(event)) {
      return;
    }
    const eventWorkspaceId = projectionEventWorkspaceID(event);
    const revision = projectionEventRevision(event);
    if (revision && lastEventRevisionRef.current && revision > lastEventRevisionRef.current + 1) {
      lastEventRevisionRef.current = revision;
      scheduleWorkspaceEventRefresh(true, selected);
      return;
    }
    if (revision && revision > lastEventRevisionRef.current) {
      lastEventRevisionRef.current = revision;
    }
    if (eventWorkspaceId && selected && eventWorkspaceId !== selected) {
      scheduleWorkspaceEventRefresh(true, selected, false);
      return;
    }
    scheduleWorkspaceEventRefresh(true, eventWorkspaceId || selected);
  }, {
    enabled: workspaceListReady && Boolean(workspaceId),
    workspaceId,
    debounceMs: 0,
    onReconnect: () => scheduleWorkspaceEventRefresh(true, currentWorkspaceIdRef.current),
    onExhausted: () => {
      message.warning('当前工作空间事件流已断开，请检查服务状态或手动刷新。');
      scheduleWorkspaceEventRefresh(true, currentWorkspaceIdRef.current);
    }
  });

  const readyDoDs = dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
  const activationLLMTargetOptions = helpers.chatTargetOptions(configuration);
  const selectedOption = activationLLMTargetOptions.find((item) => item.value === activationSelectedTargetKey);
  const activationReadyTarget = helpers.activationReadyChatTarget(configuration);
  const defaultChatModelId = configuration?.llm.defaultChat?.modelId;
  const defaultChatProviderId = configuration?.llm.defaultChat?.providerId || 'ai-proxy';
  // Readiness for the system default requires a ready Responses model.
  const defaultChatReady = Boolean(
    defaultChatModelId &&
      (
        helpers.chatTargetOptions(configuration).some(
          (item) => item.providerId === defaultChatProviderId && item.modelId === defaultChatModelId
        )
      )
  );
  // Disable submit when neither a selectable ready model nor a ready defaultChat is available.
  const activationLLMReady = Boolean(activationReadyTarget);
  const soulBindingStages = soulBindings?.availableStages?.length ? soulBindings.availableStages : defaultSoulBindingStages;
  const soulOptions = souls.map((item) => ({
    value: item.id,
    label: `${item.name || item.id} (${item.id})`
  }));
  const soulNameById = new Map(souls.map((item) => [item.id, item.name ? `${item.name} (${item.id})` : item.id]));
  const workspaceDefaultSoulLabel = soulBindings?.defaultSoulId
    ? soulNameById.get(soulBindings.defaultSoulId) || soulBindings.defaultSoulId
    : '未配置';
  const stageBindingTags = soulBindingStages
    .filter((stage: string) => soulBindings?.bindings?.[stage])
    .map((stage: string) => `${soulBindingStageLabel(stage)}: ${soulNameById.get(soulBindings?.bindings?.[stage] || '') || soulBindings?.bindings?.[stage]}`);

  function clearWorkspaceContext() {
    setWorkspaceProjection(undefined);
    setSnapshot(undefined);
    setGoals([]);
    setDoDs([]);
    setSoulBindings(undefined);
    setSkills([]);
    setMCPServers([]);
    setMemoryItems([]);
    setRunSnapshot(undefined);
    setRunObservability(undefined);
    setContextBundle(undefined);
    setRunEventRecords([]);
    setRunEventCursorAfterId(undefined);
    setRunEventHasMore(false);
    setRunEventLoading(false);
    setRunStepRecords([]);
    setRunStepCursorAfterId(undefined);
    setRunStepHasMore(false);
    setRunStepLoading(false);
    setRunToolCallRecords([]);
    setRunToolCallCursorAfterId(undefined);
    setRunToolCallHasMore(false);
    setRunToolCallLoading(false);
    setRunCheckpointRecords([]);
    setRunCheckpointCursorAfterId(undefined);
    setRunCheckpointHasMore(false);
    setRunCheckpointLoading(false);
    setRunResultRecords([]);
    setRunResultCursorAfterId(undefined);
    setRunResultHasMore(false);
    setRunResultLoading(false);
    setRunPatchRecords([]);
    setRunPatchCursorAfterId(undefined);
    setRunPatchHasMore(false);
    setRunPatchLoading(false);
    setRunValidationRecords([]);
    setRunValidationCursorAfterId(undefined);
    setRunValidationHasMore(false);
    setRunValidationLoading(false);
    setRunArtifactRecords([]);
    setRunArtifactCursorAfterId(undefined);
    setRunArtifactHasMore(false);
    setRunArtifactLoading(false);
  }

  function clearRunProcessView(nextContextBundle?: ContextBundle) {
    setRunSnapshot(undefined);
    setRunObservability(undefined);
    setContextBundle(nextContextBundle);
    setRunEventRecords([]);
    setRunEventCursorAfterId(undefined);
    setRunEventHasMore(false);
    setRunEventLoading(false);
    setRunStepRecords([]);
    setRunStepCursorAfterId(undefined);
    setRunStepHasMore(false);
    setRunStepLoading(false);
    setRunToolCallRecords([]);
    setRunToolCallCursorAfterId(undefined);
    setRunToolCallHasMore(false);
    setRunToolCallLoading(false);
    setRunCheckpointRecords([]);
    setRunCheckpointCursorAfterId(undefined);
    setRunCheckpointHasMore(false);
    setRunCheckpointLoading(false);
    setRunResultRecords([]);
    setRunResultCursorAfterId(undefined);
    setRunResultHasMore(false);
    setRunResultLoading(false);
    setRunPatchRecords([]);
    setRunPatchCursorAfterId(undefined);
    setRunPatchHasMore(false);
    setRunPatchLoading(false);
    setRunValidationRecords([]);
    setRunValidationCursorAfterId(undefined);
    setRunValidationHasMore(false);
    setRunValidationLoading(false);
    setRunArtifactRecords([]);
    setRunArtifactCursorAfterId(undefined);
    setRunArtifactHasMore(false);
    setRunArtifactLoading(false);
  }

  function applyRunStepPage(page: StepPage, append: boolean) {
    const items = page.steps ?? [];
    setRunStepRecords((current) => {
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
    });
    setRunStepCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setRunStepHasMore(Boolean(page.hasMore));
  }

  async function loadRunStepPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunStepLoading(true);
    try {
      const page = await workorchApi.listRunSteps(runId, {
        workspaceId: targetWorkspaceId,
        cursorAfterId,
        limit: runtimeStepPageLimit
      });
      applyRunStepPage(page, Boolean(cursorAfterId));
    } finally {
      setRunStepLoading(false);
    }
  }

  function applyRunToolCallPage(page: ToolCallPage, append: boolean) {
    const items = page.toolCalls ?? [];
    setRunToolCallRecords((current) => {
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
    });
    setRunToolCallCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setRunToolCallHasMore(Boolean(page.hasMore));
  }

  async function loadRunToolCallPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunToolCallLoading(true);
    try {
      const page = await workorchApi.listRunToolCalls(runId, {
        workspaceId: targetWorkspaceId,
        cursorAfterId,
        limit: runtimeToolCallPageLimit
      });
      applyRunToolCallPage(page, Boolean(cursorAfterId));
    } finally {
      setRunToolCallLoading(false);
    }
  }

  function applyRunCheckpointPage(page: RuntimeCheckpointPage, append: boolean) {
    const items = page.runtimeCheckpoints ?? [];
    setRunCheckpointRecords((current) => {
      if (!append) {
        return items;
      }
      const seen = new Set(current.map((item) => item.id));
      return [...current, ...items.filter((item) => {
        if (seen.has(item.id)) {
          return false;
        }
        seen.add(item.id);
        return true;
      })];
    });
    setRunCheckpointCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setRunCheckpointHasMore(Boolean(page.hasMore));
  }

  async function loadRunCheckpointPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunCheckpointLoading(true);
    try {
      const page = await workorchApi.listRunRuntimeCheckpoints(runId, {
        workspaceId: targetWorkspaceId,
        cursorAfterId,
        limit: runtimeCheckpointPageLimit
      });
      applyRunCheckpointPage(page, Boolean(cursorAfterId));
    } finally {
      setRunCheckpointLoading(false);
    }
  }

  function applyRunResultPage(page: ResultPage, append: boolean) {
    const items = page.results ?? [];
    setRunResultRecords((current) => {
      if (!append) {
        return items;
      }
      const seen = new Set(current.map((item) => item.id));
      return [...current, ...items.filter((item) => {
        if (seen.has(item.id)) {
          return false;
        }
        seen.add(item.id);
        return true;
      })];
    });
    setRunResultCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setRunResultHasMore(Boolean(page.hasMore));
  }

  async function loadRunResultPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunResultLoading(true);
    try {
      const page = await workorchApi.listRunResults(runId, {
        workspaceId: targetWorkspaceId,
        cursorAfterId,
        limit: runtimeResultPageLimit
      });
      applyRunResultPage(page, Boolean(cursorAfterId));
    } finally {
      setRunResultLoading(false);
    }
  }

  function applyRunPatchPage(page: PatchPage, append: boolean) {
    const items = page.patches ?? [];
    setRunPatchRecords((current) => mergeById(current, items, append));
    setRunPatchCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setRunPatchHasMore(Boolean(page.hasMore));
  }

  async function loadRunPatchPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunPatchLoading(true);
    try {
      const page = await workorchApi.listRunPatches(runId, {
        workspaceId: targetWorkspaceId,
        cursorAfterId,
        limit: runtimePatchPageLimit
      });
      applyRunPatchPage(page, Boolean(cursorAfterId));
    } finally {
      setRunPatchLoading(false);
    }
  }

  function applyRunValidationPage(page: ValidationResultPage, append: boolean) {
    const items = page.validationResults ?? [];
    setRunValidationRecords((current) => mergeById(current, items, append));
    setRunValidationCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setRunValidationHasMore(Boolean(page.hasMore));
  }

  async function loadRunValidationPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunValidationLoading(true);
    try {
      const page = await workorchApi.listRunValidationResults(runId, {
        workspaceId: targetWorkspaceId,
        cursorAfterId,
        limit: runtimeValidationPageLimit
      });
      applyRunValidationPage(page, Boolean(cursorAfterId));
    } finally {
      setRunValidationLoading(false);
    }
  }

  function applyRunArtifactPage(page: ArtifactPage, append: boolean) {
    const items = page.artifacts ?? [];
    setRunArtifactRecords((current) => mergeById(current, items, append));
    setRunArtifactCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setRunArtifactHasMore(Boolean(page.hasMore));
  }

  async function loadRunArtifactPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunArtifactLoading(true);
    try {
      const page = await workorchApi.listRunArtifacts(runId, {
        workspaceId: targetWorkspaceId,
        cursorAfterId,
        limit: runtimeArtifactPageLimit
      });
      applyRunArtifactPage(page, Boolean(cursorAfterId));
    } finally {
      setRunArtifactLoading(false);
    }
  }

  function applyRunEventPage(items: EventRecord[], append: boolean) {
    setRunEventRecords((current) => {
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
    });
    setRunEventCursorAfterId(items.at(-1)?.id);
    setRunEventHasMore(items.length >= runtimeEventPageLimit);
  }

  function scheduleWorkspaceEventRefresh(refreshList: boolean, targetWorkspaceId?: string, refreshDetail = true) {
    if (workspaceEventRefreshTimerRef.current !== undefined) {
      window.clearTimeout(workspaceEventRefreshTimerRef.current);
    }
    workspaceEventRefreshTimerRef.current = window.setTimeout(() => {
      workspaceEventRefreshTimerRef.current = undefined;
      const selected = currentWorkspaceIdRef.current;
      const nextWorkspaceId = targetWorkspaceId || selected;
      if (refreshList) {
        void loadWorkspaceList(nextWorkspaceId).catch((error: unknown) => {
          message.error(errorMessage(error));
        });
      }
      if (refreshDetail && nextWorkspaceId && nextWorkspaceId === selected) {
        void loadWorkspaceContext(nextWorkspaceId).catch((error: unknown) => {
          if (isNotFoundError(error)) {
            void loadWorkspaceList(undefined, nextWorkspaceId).catch((refreshError: unknown) => {
              message.error(errorMessage(refreshError));
            });
            return;
          }
          message.error(errorMessage(error));
        });
      }
    }, 500);
  }

  function isWorkspaceProjectionChangedEvent(event: { topic?: string; payload?: Record<string, unknown> }) {
    return event.topic === '/workorchd/workspace/projection/changed' || event.payload?.type === 'workspace_projection_changed';
  }

  function projectionEventWorkspaceID(event: { workspaceId?: string; payload?: Record<string, unknown> }) {
    const payloadWorkspaceId = event.payload?.workspaceId;
    return typeof payloadWorkspaceId === 'string' && payloadWorkspaceId.trim() !== '' ? payloadWorkspaceId.trim() : event.workspaceId;
  }

  function projectionEventRevision(event: { revision?: number; payload?: Record<string, unknown> }) {
    const workspaceRevision = event.payload?.workspaceRevision;
    if (typeof workspaceRevision === 'number' && workspaceRevision > 0) {
      return workspaceRevision;
    }
    const globalRevision = event.payload?.globalRevision;
    if (typeof globalRevision === 'number' && globalRevision > 0) {
      return globalRevision;
    }
    return event.revision ?? 0;
  }

  async function loadRunEventPage(runId: string, cursorAfterId?: string) {
    const targetWorkspaceId = workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunEventLoading(true);
    try {
      const items = await workorchApi.listEventRecords({
        workspaceId: targetWorkspaceId,
        runId,
        cursorAfterId,
        limit: runtimeEventPageLimit
      });
      applyRunEventPage(items, Boolean(cursorAfterId));
    } finally {
      setRunEventLoading(false);
    }
  }

  async function loadRunProcess(runId: string, includeContext = false, requestedWorkspaceId?: string, expectedWorkspaceContextSeq?: number) {
    const targetWorkspaceId = requestedWorkspaceId || workspaceId;
    if (!targetWorkspaceId) {
      return;
    }
    setRunEventLoading(true);
    setRunStepLoading(true);
    setRunToolCallLoading(true);
    setRunCheckpointLoading(true);
    setRunResultLoading(true);
    setRunPatchLoading(true);
    setRunValidationLoading(true);
    setRunArtifactLoading(true);
    try {
      const [nextRunSnapshot, nextRunObservability, nextRunContextBundle, stepPage, toolCallPage, checkpointPage, resultPage, patchPage, validationPage, artifactPage, eventRecords] = await Promise.all([
        workorchApi.readRunSnapshot(runId, targetWorkspaceId),
        workorchApi.readRunObservability(runId, targetWorkspaceId),
        includeContext ? workorchApi.readRunContextBundle(runId, targetWorkspaceId) : Promise.resolve(undefined),
        workorchApi.listRunSteps(runId, { workspaceId: targetWorkspaceId, limit: runtimeStepPageLimit }),
        workorchApi.listRunToolCalls(runId, { workspaceId: targetWorkspaceId, limit: runtimeToolCallPageLimit }),
        workorchApi.listRunRuntimeCheckpoints(runId, { workspaceId: targetWorkspaceId, limit: runtimeCheckpointPageLimit }),
        workorchApi.listRunResults(runId, { workspaceId: targetWorkspaceId, limit: runtimeResultPageLimit }),
        workorchApi.listRunPatches(runId, { workspaceId: targetWorkspaceId, limit: runtimePatchPageLimit }),
        workorchApi.listRunValidationResults(runId, { workspaceId: targetWorkspaceId, limit: runtimeValidationPageLimit }),
        workorchApi.listRunArtifacts(runId, { workspaceId: targetWorkspaceId, limit: runtimeArtifactPageLimit }),
        workorchApi.listEventRecords({ workspaceId: targetWorkspaceId, runId, limit: runtimeEventPageLimit })
      ]);
      if (expectedWorkspaceContextSeq !== undefined && expectedWorkspaceContextSeq !== workspaceContextRequestSeq.current) {
        return;
      }
      if (targetWorkspaceId !== currentWorkspaceIdRef.current) {
        return;
      }
      setRunSnapshot(nextRunSnapshot);
      setRunObservability(nextRunObservability);
      if (nextRunContextBundle) {
        setContextBundle(nextRunContextBundle);
      }
      applyRunStepPage(stepPage, false);
      applyRunToolCallPage(toolCallPage, false);
      applyRunCheckpointPage(checkpointPage, false);
      applyRunResultPage(resultPage, false);
      applyRunPatchPage(patchPage, false);
      applyRunValidationPage(validationPage, false);
      applyRunArtifactPage(artifactPage, false);
      applyRunEventPage(eventRecords, false);
    } finally {
      setRunEventLoading(false);
      setRunStepLoading(false);
      setRunToolCallLoading(false);
      setRunCheckpointLoading(false);
      setRunResultLoading(false);
      setRunPatchLoading(false);
      setRunValidationLoading(false);
      setRunArtifactLoading(false);
    }
  }

  async function loadWorkspaceList(preferredWorkspaceId?: string, ignoredWorkspaceId?: string) {
    const requestSeq = ++workspaceListRequestSeq.current;
    const projection = await workorchApi.listWorkspaceProjection();
    const items = projection.workspaces;
    if (requestSeq !== workspaceListRequestSeq.current) {
      return workspaceId;
    }
    setWorkspaces(items);
    const selectableItems = ignoredWorkspaceId ? items.filter((item) => item.id !== ignoredWorkspaceId) : items;
    const selectableIDs = new Set(selectableItems.map((item) => item.id));
    const nextWorkspaceId = preferredWorkspaceId && selectableIDs.has(preferredWorkspaceId)
      ? preferredWorkspaceId
      : workspaceId && selectableIDs.has(workspaceId)
        ? workspaceId
        : selectableItems[0]?.id;
    setWorkspaceListReady(true);
    currentWorkspaceIdRef.current = nextWorkspaceId;
    setWorkspaceId(nextWorkspaceId);
    if (!nextWorkspaceId) {
      clearWorkspaceContext();
    }
    return nextWorkspaceId;
  }

  async function loadWorkspaceContext(targetWorkspaceId: string) {
    const requestSeq = ++workspaceContextRequestSeq.current;
    const [nextSnapshotProjection, nextSouls, nextSoulBindings, nextSkills, nextMCPServers, nextMemoryItems, nextConfiguration] = await Promise.all([
      workorchApi.readWorkspaceSnapshotProjection(targetWorkspaceId),
      workorchApi.listSouls(),
      workorchApi.readWorkspaceSoulBindings(targetWorkspaceId),
      workorchApi.listSkills(targetWorkspaceId),
      workorchApi.listMCPServers(),
      workorchApi.listMemory(targetWorkspaceId),
      workorchApi.readConfiguration()
    ]);
    if (requestSeq !== workspaceContextRequestSeq.current) {
      return;
    }
    const nextSnapshot = nextSnapshotProjection.snapshot;
    setWorkspaceProjection(nextSnapshotProjection.projection);
    setSnapshot(nextSnapshot);
    setContextBundle(nextSnapshot.contextBundle);
    setGoals(nextSnapshot.goals ?? []);
    setDoDs(nextSnapshot.dods ?? []);
    setSouls(nextSouls);
    setSoulBindings(nextSoulBindings);
    soulBindingsForm.setFieldsValue({
      defaultSoulId: nextSoulBindings.defaultSoulId,
      bindings: nextSoulBindings.bindings ?? {}
    });
    setSkills(nextSkills);
    setMCPServers(nextMCPServers);
    setMemoryItems(nextMemoryItems);
    setConfiguration(nextConfiguration);
    const keepRunProcessView = runSnapshot
      && runSnapshot.workspace.id === targetWorkspaceId
      && (nextSnapshot.runs ?? []).some((item) => item.id === runSnapshot.run.id);
    if (!keepRunProcessView) {
      clearRunProcessView(nextSnapshot.contextBundle);
    }
  }

  const saveWorkspaceSoulBindings = async (values: { defaultSoulId?: string; bindings?: Record<string, string> }) => {
    if (!workspaceId) {
      message.error('请先选择工作空间');
      return;
    }
    setSoulBindingSaving(true);
    try {
      const nextBindings = await workorchApi.updateWorkspaceSoulBindings(workspaceId, {
        defaultSoulId: values.defaultSoulId,
        bindings: values.bindings
      });
      setSoulBindings(nextBindings);
      soulBindingsForm.setFieldsValue({
        defaultSoulId: nextBindings.defaultSoulId,
        bindings: nextBindings.bindings ?? {}
      });
      await loadWorkspaceContext(workspaceId);
      setSoulBindingEditorOpen(false);
      message.success('执行人格配置已保存');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSoulBindingSaving(false);
    }
  };

  const openSoulBindingEditor = () => {
    soulBindingsForm.setFieldsValue({
      defaultSoulId: soulBindings?.defaultSoulId,
      bindings: soulBindings?.bindings ?? {}
    });
    setSoulBindingEditorOpen(true);
  };

  const openWorkspaceDelete = () => {
    workspaceDeleteForm.setFieldsValue({ purge: false });
    setWorkspaceDeleteOpen(true);
  };

  const submitWorkspaceDelete = async () => {
    if (!workspaceId) {
      return;
    }
    const targetWorkspaceId = workspaceId;
    const values = workspaceDeleteForm.getFieldsValue();
    setWorkspaceDeleting(true);
    try {
      await workorchApi.deleteWorkspace(targetWorkspaceId, values.purge);
      setWorkspaceDeleteOpen(false);
      workspaceDeleteForm.resetFields();
      clearWorkspaceContext();
      await loadWorkspaceList(undefined, targetWorkspaceId);
      message.success(values.purge ? '工作空间及关联数据已删除' : '工作空间已删除');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setWorkspaceDeleting(false);
    }
  };

  const submitWorkspaceExecution = (request: WorkspaceExecutionActivationRequest & { llmTargetKey?: string }) => {
    if (!workspaceId) {
      return;
    }
    const commandIntent = request.intent || activationIntent;
    const targetWorkspaceId = workspaceId;
    if (!workspaceExecutionIntentAllowed(snapshot, commandIntent)) {
      message.warning(workspaceDefinitionClarification(snapshot)
        ? commandIntent === 'resume'
          ? 'Definition clarification 不能通过 Resume 绕过；请修订 Goal/DoD，然后由用户显式 Restart 交给 owner 判定'
          : 'pre-seal Definition clarification 尚未形成可替换的 Run；请先修订 Goal/DoD，Restart 未提交'
        : '当前活动 Run 正在等待 Decision/Approval owner 收口：请处理治理阻塞并等待 continuation 自动收敛；Resume 未提交');
      return;
    }
    let prepareOwnerState: Promise<unknown> = Promise.resolve();
    if (commandIntent === 'restart') {
      const selectedModelId = request.llmTarget?.modelId?.trim() || activationReadyTarget?.modelId;
      const normalizedLLMTarget = selectedModelId
        ? {
            providerId: request.llmTarget?.providerId?.trim() || activationReadyTarget?.providerId || 'ai-proxy',
            modelId: selectedModelId
          }
        : undefined;
      if (!normalizedLLMTarget) {
        message.error('当前没有 ready 的对话模型，请先完成模型配置');
        return;
      }
      const reasoningEffort = request.reasoningEffort?.trim() || undefined;
      const expectedRevision = snapshot?.workspace.revision ?? 0;
      // Fresh activation configuration is persisted through Workspace-owner
      // APIs before the immutable restart intent. Resume deliberately skips
      // this branch and can never patch model or Soul state.
      prepareOwnerState = workorchApi.updateWorkspaceLLMSelection(
        targetWorkspaceId,
        { target: normalizedLLMTarget, reasoningEffort },
        expectedRevision
      ).then(async () => {
        if (request.soulId || request.soulBindings) {
          const nextBindings: WorkspaceSoulBindings = {
            defaultSoulId: request.soulId || soulBindings?.defaultSoulId,
            bindings: {
              ...(soulBindings?.bindings ?? {}),
              ...(request.soulBindings ?? {})
            }
          };
          const updated = await workorchApi.updateWorkspaceSoulBindings(targetWorkspaceId, nextBindings);
          setSoulBindings(updated);
        }
      });
    }
    setActivationLoading(true);
    setActivationOpen(false);
    setActivationTracking(true);
    activationForm.resetFields();
    message.info(commandIntent === 'restart' ? '正在保存 fresh activation 配置并提交新执行' : '正在提交当前 Run 的恢复意图');
    void prepareOwnerState
		.then(() => workorchApi.submitWorkspaceExecution(targetWorkspaceId, { intent: commandIntent }))
      .then(async (admission) => {
        if (!admission.accepted) {
          await loadWorkspaceContext(targetWorkspaceId);
          message.error(`Workspace owner 拒绝执行意图：${admission.diagnostic || admission.reason}`);
          return;
        }
        setWorkspaceProjection(undefined);
        setRunSnapshot(undefined);
        setRunObservability(undefined);
        setRunEventRecords([]);
        setRunEventCursorAfterId(undefined);
        setRunEventHasMore(false);
        setRunStepRecords([]);
        setRunStepCursorAfterId(undefined);
        setRunStepHasMore(false);
        setRunToolCallRecords([]);
        setRunToolCallCursorAfterId(undefined);
        setRunToolCallHasMore(false);
        setRunCheckpointRecords([]);
        setRunCheckpointCursorAfterId(undefined);
        setRunCheckpointHasMore(false);
        setRunResultRecords([]);
        setRunResultCursorAfterId(undefined);
        setRunResultHasMore(false);
        setRunPatchRecords([]);
        setRunPatchCursorAfterId(undefined);
        setRunPatchHasMore(false);
        setRunValidationRecords([]);
        setRunValidationCursorAfterId(undefined);
        setRunValidationHasMore(false);
        setRunArtifactRecords([]);
        setRunArtifactCursorAfterId(undefined);
        setRunArtifactHasMore(false);
        setContextBundle(undefined);
        await loadWorkspaceContext(targetWorkspaceId);
        message.success(commandIntent === 'restart' ? 'Workspace owner 已受理新执行意图' : 'Workspace owner 已受理恢复意图');
      })
      .catch((error: unknown) => {
        message.error(`工作空间执行提交失败：${errorMessage(error)}`);
      })
      .finally(() => {
        setActivationLoading(false);
        setActivationTracking(false);
      });
  };

  const requestWorkspaceActivation = (intent: 'resume' | 'restart' = 'resume') => {
    if (!workspaceExecutionIntentAllowed(snapshot, intent)) {
      message.warning(workspaceDefinitionClarification(snapshot)
        ? intent === 'resume'
          ? 'Definition clarification 不能通过 Resume 绕过；请修订 Goal/DoD，然后由用户显式 Restart 交给 owner 判定'
          : 'pre-seal Definition clarification 尚未形成可替换的 Run；请先修订 Goal/DoD，Restart 未提交'
        : '当前活动 Run 正在等待 Decision/Approval owner 收口：请处理治理阻塞并等待 continuation 自动收敛；Resume 未提交');
      return;
    }
    setActivationIntent(intent);
    activationForm.resetFields();
    if (intent === 'restart') {
      const defaultSelection = snapshot?.workspace.defaultLLMSelection;
      const readyDefault = activationLLMTargetOptions.find(
        (item) => item.providerId === defaultSelection?.target.providerId && item.modelId === defaultSelection?.target.modelId
      );
      const target = readyDefault ? defaultSelection?.target : activationReadyTarget;
      const reasoningEffort = readyDefault && defaultSelection?.reasoningEffort && (readyDefault.reasoningEfforts ?? []).includes(defaultSelection.reasoningEffort)
        ? defaultSelection.reasoningEffort
        : undefined;
      activationForm.setFieldsValue({
        llmTarget: target,
        reasoningEffort,
        llmTargetKey: helpers.providerModelTargetKey(target?.providerId, target?.modelId),
        soulId: soulBindings?.defaultSoulId,
		soulBindings: { ...(soulBindings?.bindings ?? {}) },
		intent
	  });
	} else {
	  activationForm.setFieldsValue({ intent });
    }
    setActivationOpen(true);
  };

  const openGoalDefinition = (goal?: Goal) => {
    setEditingGoal(goal);
    goalForm.setFieldsValue({ text: goal?.text || '' });
    setGoalDefinitionOpen(true);
  };

  const openDoDDefinition = (dod?: DoD) => {
    setEditingDoD(dod);
    dodForm.setFieldsValue({ items: dod?.items?.join('\n') || '' });
    setDoDDefinitionOpen(true);
  };

  const openDefinitionRevision = () => {
    const goal = goals[0];
    if (goal) {
      openGoalDefinition(goal);
      return;
    }
    const dod = readyDoDs[0] ?? dods[0];
    if (dod) {
      openDoDDefinition(dod);
      return;
    }
    openGoalDefinition();
  };

  const openGoalDoDQualityCheck = async (goal?: Goal, dod?: DoD) => {
    if (!workspaceId) {
      message.error('请先选择工作空间');
      return;
    }
    const targetGoal = goal ?? goals[0];
    const targetDoD = dod ?? readyDoDs[0] ?? dods[0];
    if (!targetGoal || !targetDoD) {
      message.warning('请先定义目标和验收标准');
      return;
    }
    setQualityCheckLoading(true);
    try {
      const next = await workorchApi.checkGoalDoDQuality(workspaceId, {
        goalId: targetGoal.id,
        doDId: targetDoD.id,
        goalText: targetGoal.text,
        doDItems: targetDoD.items
      });
      setQualityCheck(next);
      setQualityCheckOpen(true);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setQualityCheckLoading(false);
    }
  };

  const runColumns: ProColumns<Run>[] = [
    { title: '运行 ID（Run ID）', dataIndex: 'id', copyable: true },
    { title: '目标', dataIndex: 'goalId', copyable: true },
    { title: '验收标准', dataIndex: 'doDId', copyable: true },
    { title: '封存模型目标', render: (_, item) => providerModelTargetLabel(item.sealedLLMSelection), ellipsis: true },
	{ title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: '阻塞工单', dataIndex: 'blockingTicketId', copyable: true },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => (
        <Space>
          <Button
            size="small"
            onClick={() => void loadRunProcess(item.id, true).catch((error: unknown) => message.error(errorMessage(error)))}
          >
            查看成果与过程
          </Button>
        </Space>
      )
    }
  ];
  const activeRun = snapshot?.workspace.activeRunId
    ? snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId)
    : undefined;
  const definitionClarificationDecision = workspaceDefinitionClarification(snapshot);
  const definitionClarification = definitionClarificationDecision?.definitionClarification;
  const runtimeDefinitionReplacementAvailable = Boolean(canonicalDefinitionClarification(snapshot));
  const activeRunGovernanceBlocked = canonicalActiveRunIsGovernanceBlocked(snapshot);
  const primaryExecutionIntent = helpers.workspaceExecutionPrimaryIntent(snapshot);
  const activeRunRequiresFreshActivation = activeRun?.runtimeActivation?.disposition === 'fresh_activation_required';
  const activeRunSchemaTransition = activeRunRequiresFreshActivation && activeRun.runtimeActivation?.reason === 'schema_unsupported'
    ? `Runtime schema v${activeRun.runtimeActivation?.actualSchemaVersion} → v${activeRun.runtimeActivation?.supportedSchemaVersion}`
    : undefined;
  const activeRunFreshActivationDetail = activeRunSchemaTransition
    ? `当前二进制不支持已持久化的 ${activeRunSchemaTransition}；该运行不能恢复，请显式创建新执行。`
    : activeRun?.runtimeActivation?.reason === 'cwi_ledger_invalid'
      ? '当前运行的已持久化 CWI 台账存在歧义，不能安全恢复；请显式创建新执行。'
      : '当前运行不能安全恢复；请显式创建新执行。';
  const runSummary = runStateSummary(snapshot?.runs);
  const outcomeRun = runSnapshot?.run;
  const outcomeGoal = goals.find((item) => item.id === outcomeRun?.goalId);
  const outcomeDoD = dods.find((item) => item.id === outcomeRun?.doDId);
  const outcomeResult = primaryAcceptedResult(runSnapshot) ?? primaryResult(runSnapshot);
  const outcomeSummary = outcomeResult?.summaryDetail;
  const currentArtifactRecords = runArtifactRecords.length ? runArtifactRecords : (runSnapshot?.artifacts ?? []);
  const outcomeArtifacts = outcomeResult
    ? currentArtifactRecords.filter((item) => item.resultId === outcomeResult.id || !item.resultId)
    : currentArtifactRecords;
  const outcomeMemoryRefs = memoryRefIDs(runSnapshot);
  const outcomeMemories = memoryItems.filter((item) => item.runId === outcomeRun?.id || outcomeMemoryRefs.has(item.id));
  const outcomeDoDItems = dodAssessmentItems(runSnapshot);
  const runWebSources = runObservability?.webSources ?? runSnapshot?.webSources ?? snapshot?.webSources;
  const webSourceCount = webSourceRows(runWebSources).length;
  const runLLMContextBudgets = runObservability?.llmContextBudgets ?? runSnapshot?.llmContextBudgets ?? snapshot?.llmContextBudgets;
  const runLLMContextBudgetSummary = runObservability?.llmContextBudgetSummary ?? runSnapshot?.llmContextBudgetSummary ?? snapshot?.llmContextBudgetSummary;
  const workspaceLLMUsageSummary = snapshot?.llmUsageSummary;
  const runLLMUsageSummary = runObservability?.llmUsageSummary ?? runSnapshot?.llmUsageSummary ?? snapshot?.llmUsageSummary;
  const runLLMUsageTraces = runObservability?.llmUsageTraces ?? runSnapshot?.llmUsageTraces ?? snapshot?.llmUsageTraces;
  const summaryLLMUsage = workspaceLLMUsageSummary ?? runLLMUsageSummary;
  const currentArtifactContinuations = runSnapshot?.artifactContinuations ?? snapshot?.artifactContinuations ?? [];
  const currentRunHeartbeat = runSnapshot?.runHeartbeat ?? snapshot?.runHeartbeat;
  const currentConvergenceStatus = runSnapshot?.convergenceStatus ?? snapshot?.convergenceStatus;
  const dodDefinitionRows = dods.flatMap((dod) => dod.items.map((text, index) => ({
    key: `${dod.id}-${index}`,
    id: dod.id,
    state: dod.state,
    index: index + 1,
    text
  })));

  return (
    <PageContainer
      title="工作空间"
      subTitle="围绕工作空间（Workspace）管理目标、验收标准、执行过程、治理阻塞和运行证据"
      extra={
        <div className="workspace-page-header-actions">
          <div className="workspace-header-context">
            <Typography.Text type="secondary" className="workspace-header-context-label">当前工作空间</Typography.Text>
            <Select
              className="workspace-header-select"
              placeholder="选择工作空间"
              value={workspaceId}
              onChange={setWorkspaceId}
              options={workspaces.map((item) => ({ value: item.id, label: `${item.title || item.id} (${item.id})` }))}
            />
            {activationTracking ? <Tag color="processing">后台跟踪中</Tag> : null}
          </div>
          <div className="workspace-header-actions-main">
            <Button
              type="primary"
              disabled={!workspaceId || (activeRunGovernanceBlocked && !definitionClarification)}
              onClick={() => definitionClarification && !runtimeDefinitionReplacementAvailable
                ? openDefinitionRevision()
                : requestWorkspaceActivation(definitionClarification ? 'restart' : primaryExecutionIntent)}
            >
              {definitionClarification
                ? runtimeDefinitionReplacementAvailable ? '显式 Restart（由 owner 校验）' : '修订 Definition'
                : activeRunGovernanceBlocked
                  ? '等待治理 Owner 收口'
                  : primaryExecutionIntent === 'restart'
                    ? `创建新执行${activeRunSchemaTransition ? `（${activeRunSchemaTransition}）` : ''}`
                    : '恢复当前执行'}
            </Button>
            {primaryExecutionIntent === 'resume' && !definitionClarification ? (
              <Button disabled={!workspaceId} onClick={() => requestWorkspaceActivation('restart')}>创建新执行</Button>
            ) : null}
            <Dropdown
              menu={{
                items: [
                  { key: 'goal', label: '定义目标', disabled: !workspaceId },
                  { key: 'dod', label: '定义验收标准', disabled: !workspaceId },
					{ key: 'soul', label: '调整执行人格', disabled: !workspaceId }
                ],
                onClick: ({ key }) => {
                  if (key === 'goal') {
                    openGoalDefinition();
                  } else if (key === 'dod') {
                    openDoDDefinition();
                  } else if (key === 'soul') {
                    openSoulBindingEditor();
                  }
                }
              }}
            >
              <Button disabled={!workspaceId}>完善定义 <DownOutlined /></Button>
            </Dropdown>
            <Dropdown
              menu={{
                items: [
                  { key: 'delete', label: '删除工作空间', danger: true, disabled: !workspaceId }
                ],
                onClick: ({ key }) => {
                  if (key === 'delete') {
                    openWorkspaceDelete();
                  }
                }
              }}
            >
              <Button disabled={!workspaceId}>更多操作 <DownOutlined /></Button>
            </Dropdown>
          </div>
        </div>
      }
    >
      {snapshot ? (
        <>
          {definitionClarification ? (
            <Alert
              className="workorch-section"
              type="warning"
              showIcon
              message={`Definition clarification · ${definitionClarification.violationOrGapCode}`}
              description={(
                <Space direction="vertical" size="small" style={{ width: '100%' }}>
                  <Descriptions
                    size="small"
                    column={{ xs: 1, md: 2 }}
                    items={[
                      { key: 'class', label: 'Class', children: stateTag(definitionClarification.class) },
                      { key: 'bindingState', label: 'Binding state', children: stateTag(definitionClarification.bindingState) },
                      { key: 'requiredAction', label: 'Required action', children: stateTag(definitionClarification.requiredAction) },
                      { key: 'gap', label: 'Gap / violation', children: definitionClarification.violationOrGapCode },
                      { key: 'offendingRefs', label: 'Offending refs', children: compactTags(definitionClarification.offendingRefs ?? [], 8) },
                      { key: 'definitionRevision', label: 'Blocked definition revision', children: definitionClarification.definitionRevision || '-' },
                      { key: 'authoritySnapshotId', label: 'Authority snapshot', children: definitionClarification.authoritySnapshotId || '-' },
                      { key: 'clarificationDispatchId', label: 'Clarification dispatch', children: definitionClarification.clarificationDispatchId || '-' },
                      { key: 'focusRef', label: 'Focus', children: definitionClarification.focusRef || '-' }
                    ]}
                  />
                  <Typography.Text type="secondary">
                    {runtimeDefinitionReplacementAvailable
                      ? '可先修订 Goal/DoD，再由用户显式 Restart；最终 admission 只由 Workspace owner 判定，读模型 freshness 不参与命令许可。此工单不能被解释为通用 answer 或 Resume。'
                      : '这是 pre-seal Definition clarification；请修订 Goal/DoD。没有 Run 时不能解释为通用 answer、Resume 或 Restart。'}
                  </Typography.Text>
                  <Space wrap>
                    <Button type="primary" onClick={() => openGoalDefinition(goals[0])}>修订 Goal</Button>
                    <Button onClick={() => openDoDDefinition(readyDoDs[0] ?? dods[0])}>修订 DoD</Button>
                    {runtimeDefinitionReplacementAvailable ? <Button onClick={() => requestWorkspaceActivation('restart')}>显式 Restart（由 owner 校验）</Button> : null}
                  </Space>
                </Space>
              )}
            />
          ) : null}
          <ProCard loading={loading} title={snapshot.workspace.title || snapshot.workspace.id} extra={stateTag(snapshot.workspace.state)}>
            <Descriptions
              column={2}
              items={[
                { key: 'id', label: '工作空间 ID', children: snapshot.workspace.id },
                { key: 'activeRun', label: '活跃运行', children: snapshot.workspace.activeRunId || '-' },
                { key: 'nextRunLLMTarget', label: '下次运行默认模型', children: providerModelTargetLabel(snapshot.workspace.defaultLLMSelection) },
                { key: 'activeRunSealedLLMSelection', label: '当前运行封存模型', children: providerModelTargetLabel(activeRun?.sealedLLMSelection) },
                ...(activeRunRequiresFreshActivation ? [{
                  key: 'activeRunRuntimeActivation',
                  label: '运行恢复状态',
                  children: activeRunFreshActivationDetail
                }] : []),
                { key: 'displayOutcome', label: '展示结果（非 Owner 状态）', children: snapshot.displayOutcome?.runId ? `${snapshot.displayOutcome.runId} / ${stateLabel(snapshot.displayOutcome.state)}` : '-' },
                { key: 'canonicalRunState', label: 'Run Owner 状态', children: activeRun ? `${activeRun.id} / ${stateLabel(activeRun.state)}` : '-' },
                { key: 'governanceDisplay', label: '治理等待（只读）', children: snapshot.governanceDisplay?.decisionId || snapshot.governanceDisplay?.approvalId || '-' },
                ...(snapshot.consistencyDiagnostic?.code ? [{ key: 'consistencyDiagnostic', label: 'Owner 一致性诊断', children: `${snapshot.consistencyDiagnostic.code}: ${snapshot.consistencyDiagnostic.summary || '-'}` }] : []),
                { key: 'projection', label: '投影摘要', children: projectionStatusSummary(workspaceProjection) },
                { key: 'description', label: '描述', children: boundedParagraph(snapshot.workspace.description, { rows: 2 }) },
                { key: 'deliverableDirectory', label: '工作目录', children: boundedParagraph(snapshot.workspace.deliverableDirectory, { rows: 1, copyText: snapshot.workspace.deliverableDirectory }) },
                { key: 'defaultSoul', label: '默认执行人格', children: workspaceDefaultSoulLabel },
                { key: 'stageSouls', label: '阶段执行人格', children: compactTags(stageBindingTags, 4) }
              ]}
            />
          </ProCard>
          <ProCard title="投影状态" className="workorch-section" extra={workspaceProjection ? stateTag(workspaceProjection.state) : null}>
            <Descriptions column={{ xs: 1, md: 2 }} size="small" items={projectionStatusDescriptions(workspaceProjection)} />
          </ProCard>
          <ProCard title="执行过程" className="workorch-section">
            <Steps items={workspaceProcessItems(snapshot, goals, dods)} />
          </ProCard>
          <ProCard title="主要业务模型" className="workorch-section">
            {metricGrid([
              { key: 'workspace', label: modelLabels.workspace, value: 1, color: '#1668dc' },
              { key: 'goal', label: modelLabels.goal, value: goals.length },
              { key: 'dod', label: modelLabels.dod, value: dods.length },
              { key: 'run', label: modelLabels.run, value: snapshot.runs?.length ?? 0, color: '#0958d9' },
              { key: 'decision', label: modelLabels.decision, value: snapshot.decisions?.length ?? 0 },
              { key: 'approval', label: modelLabels.approval, value: snapshot.approvals?.length ?? 0 },
              { key: 'soul', label: modelLabels.soul, value: souls.length },
              { key: 'skill', label: modelLabels.skill, value: skills.length },
              { key: 'mcp', label: modelLabels.mcpServer, value: mcpServers.length },
              { key: 'memory', label: modelLabels.memory, value: memoryItems.length },
              { key: 'step', label: modelLabels.step, value: runSnapshot?.steps?.length ?? 0 },
              { key: 'result', label: modelLabels.result, value: snapshot.results?.length ?? (runResultRecords.length || runSnapshot?.results?.length || 0), color: '#389e0d' },
              { key: 'artifact', label: modelLabels.artifact, value: snapshot.artifacts?.length ?? (runArtifactRecords.length || runSnapshot?.artifacts?.length || 0) },
              { key: 'artifactContinuation', label: 'Artifact Continuation', value: currentArtifactContinuations.length, color: currentArtifactContinuations.length ? '#08979c' : undefined },
              { key: 'webSources', label: 'Web 来源', value: webSourceCount, color: webSourceCount ? '#08979c' : undefined },
              { key: 'llmContext', label: 'LLM Context', value: runLLMContextBudgets?.length ?? 0, color: (runLLMContextBudgets?.length ?? 0) > 0 ? '#1677ff' : undefined },
              { key: 'llmUsage', label: modelLabels.llmUsage, value: runLLMUsageSummary?.entryTotal ?? 0, color: runLLMUsageSummary?.entryTotal ? '#d48806' : undefined },
              { key: 'tool', label: modelLabels.toolCall, value: runSnapshot?.toolCalls?.length ?? 0 },
              { key: 'patch', label: modelLabels.patch, value: runPatchRecords.length || runSnapshot?.patches?.length || 0 },
              { key: 'validation', label: modelLabels.validationResult, value: runValidationRecords.length || runSnapshot?.validationResults?.length || 0 },
              { key: 'checkpoint', label: modelLabels.runtimeCheckpoint, value: runCheckpointRecords.length || runSnapshot?.runtimeCheckpoints?.length || 0 }
            ])}
          </ProCard>
          <ProCard title="运行摘要" className="workorch-section">
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              {metricGrid([
                { key: 'runs', label: '运行（Run）总数', value: runSummary.total },
                { key: 'created', label: '已创建', value: runSummary.created },
                { key: 'active', label: '运行中', value: runSummary.active, color: '#0958d9' },
                { key: 'blocked', label: '阻塞中', value: runSummary.blocked, color: '#d48806' },
                { key: 'validating', label: '验收中', value: runSummary.validating, color: '#08979c' },
                { key: 'completed', label: '已完成', value: runSummary.completed, color: '#389e0d' },
                { key: 'failed', label: '失败/终止', value: runSummary.failed, color: '#cf1322' },
                { key: 'llmCalls', label: 'LLM 调用总数', value: summaryLLMUsage?.entryTotal ?? 0, color: summaryLLMUsage?.entryTotal ? '#d48806' : undefined },
                { key: 'llmInputTokens', label: 'LLM 输入 Token', value: numberText(summaryLLMUsage?.inputTokens) },
                { key: 'llmOutputTokens', label: 'LLM 输出 Token', value: numberText(summaryLLMUsage?.outputTokens) },
                { key: 'llmTotalTokens', label: 'LLM Token 总量', value: numberText(summaryLLMUsage?.totalTokens), color: summaryLLMUsage?.totalTokens ? '#0958d9' : undefined },
                { key: 'llmCacheReadTokens', label: 'LLM Cache Read Token', value: numberText(summaryLLMUsage?.cacheReadTokens) },
                { key: 'llmCacheWriteTokens', label: 'LLM Cache Write Token', value: numberText(summaryLLMUsage?.cacheWriteTokens) },
                { key: 'llmCacheReadRatio', label: 'LLM Cache Read/Input', value: percentText(summaryLLMUsage?.cacheReadInputRatio), color: summaryLLMUsage?.cacheReadInputRatio ? '#389e0d' : undefined },
                { key: 'decisions', label: '待处理决策', value: pendingCount(snapshot.decisions), color: pendingCount(snapshot.decisions) ? '#d48806' : undefined },
                { key: 'approvals', label: '待处理审批', value: pendingCount(snapshot.approvals), color: pendingCount(snapshot.approvals) ? '#d48806' : undefined }
              ])}
              <Descriptions
                column={1}
                size="small"
                items={[
                  { key: 'latestRun', label: '当前运行', children: workspaceProjectionRunLabel(snapshot) },
                  { key: 'displayOutcome', label: '展示结果（非 Owner 状态）', children: snapshot.displayOutcome?.runId ? `${snapshot.displayOutcome.runId} / ${stateLabel(snapshot.displayOutcome.state)}` : '-' }
                ]}
              />
              <Descriptions
                column={{ xs: 1, md: 2 }}
                size="small"
                items={runCommandDescriptions(snapshot)}
              />
              <Descriptions
                column={{ xs: 1, md: 2 }}
                size="small"
                items={runHeartbeatDescriptions(currentRunHeartbeat)}
              />
              <Descriptions
                column={{ xs: 1, md: 2 }}
                size="small"
                items={convergenceStatusDescriptions(currentConvergenceStatus)}
              />
            </Space>
          </ProCard>
          <CardGrid className="workorch-section">
            <ProCard title="目标定义（Goal）" colSpan={{ xs: 24, lg: 9 }} extra={<Tag>{goals.length} 项</Tag>}>
              {goals.length ? (
                <List
                  className="workorch-definition-list"
                  pagination={goals.length > 4 ? { pageSize: 4, size: 'small' } : false}
                  dataSource={goals}
                  renderItem={(item) => {
                    const targetDoD = readyDoDs[0] ?? dods[0];
                    return (
                      <List.Item
                        actions={[
                          <Button key="edit" type="link" size="small" onClick={() => openGoalDefinition(item)}>修改</Button>,
                          <Button key="quality" type="link" size="small" loading={qualityCheckLoading} disabled={!targetDoD} onClick={() => void openGoalDoDQualityCheck(item, targetDoD)}>质量评分</Button>
                        ]}
                      >
                        <List.Item.Meta
                          title={<Space><Typography.Text copyable>{item.id}</Typography.Text></Space>}
                          description={boundedParagraph(item.text, { rows: 3 })}
                        />
                      </List.Item>
                    );
                  }}
                />
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无目标" />}
            </ProCard>
            <ProCard title="验收标准（DoD）" colSpan={{ xs: 24, lg: 9 }} extra={<Tag>{dodDefinitionRows.length} 项</Tag>}>
              {dodDefinitionRows.length ? (
                <List
                  className="workorch-definition-list"
                  pagination={dodDefinitionRows.length > 5 ? { pageSize: 5, size: 'small' } : false}
                  dataSource={dodDefinitionRows}
                  renderItem={(item) => {
                    const dod = dods.find((candidate) => candidate.id === item.id);
                    const targetGoal = goals[0];
                    return (
                      <List.Item
                        actions={[
                          <Button key="edit" type="link" size="small" disabled={!dod} onClick={() => dod && openDoDDefinition(dod)}>修改</Button>,
                          <Button key="quality" type="link" size="small" loading={qualityCheckLoading} disabled={!dod || !targetGoal} onClick={() => dod && void openGoalDoDQualityCheck(targetGoal, dod)}>质量评分</Button>
                        ]}
                      >
                        <List.Item.Meta
                          title={<Space><Tag>{item.index}</Tag>{stateTag(item.state)}<Typography.Text copyable>{item.id}</Typography.Text></Space>}
                          description={boundedParagraph(item.text, { rows: 2 })}
                        />
                      </List.Item>
                    );
                  }}
                />
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无验收标准" />}
            </ProCard>
            <ProCard title="执行配置（Soul）" colSpan={{ xs: 24, lg: 6 }} extra={<Button size="small" onClick={openSoulBindingEditor}>调整</Button>}>
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Typography.Text type="secondary">
                  执行人格（Soul）作为工作空间执行配置参与规划、执行、决策、审批、验收和交付物阶段，不改变目标（Goal）/验收标准（DoD）主体定义。
                </Typography.Text>
                <Descriptions
                  size="small"
                  column={1}
                  items={[
                    { key: 'defaultSoul', label: '默认执行人格', children: workspaceDefaultSoulLabel },
                    { key: 'stageSoul', label: '阶段覆盖', children: compactTags(stageBindingTags, 3) },
                    { key: 'priority', label: '启用优先级', children: compactTags(soulBindings?.priorityPolicy ?? []) }
                  ]}
                />
              </Space>
            </ProCard>
            <ProCard title="工作空间记忆（Memory）" colSpan={{ xs: 24, lg: 12 }}>
              {memoryItems.length ? (
                <List
                  className="workorch-definition-list"
                  pagination={memoryItems.length > 5 ? { pageSize: 5, size: 'small' } : false}
                  dataSource={memoryItems}
                  renderItem={(item) => (
                    <List.Item>
                      <List.Item.Meta
                        title={<Space><Tag>{item.memoryType}</Tag><Typography.Text copyable>{item.id}</Typography.Text></Space>}
                        description={<Space direction="vertical" size={2}>{boundedParagraph(item.summary, { rows: 2 })}<Typography.Text type="secondary">{item.sourceType}{item.sourceRef ? ` / ${item.sourceRef}` : ''}</Typography.Text>{item.tags?.length ? compactTags(item.tags, 5) : null}</Space>}
                      />
                    </List.Item>
                  )}
                />
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无工作空间记忆" />}
            </ProCard>
          </CardGrid>
          <ProCard
            title="目标成果"
            className="workorch-section"
            extra={goalOutcomeState(runSnapshot)}
          >
            {runSnapshot ? (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Descriptions
                  column={2}
                  items={[
                    { key: 'run', label: `关联${modelLabels.run}`, children: `${runSnapshot.run.id} / ${stateLabel(runOutcomeState(runSnapshot))}` },
                    { key: 'rawRunState', label: '原始运行状态', children: stateLabel(runRawState(runSnapshot)) },
                    { key: 'result', label: modelLabels.result, children: outcomeResult ? boundedParagraph(outcomeResult.id, { rows: 1, copyText: outcomeResult.id }) : '-' },
                    { key: 'goal', label: '目标', children: boundedParagraph(outcomeGoal?.text || runSnapshot.run.goalId, { rows: 2 }) },
                    { key: 'dod', label: '验收标准', children: outcomeDoD?.items?.length ? compactTags(outcomeDoD.items) : runSnapshot.run.doDId || '-' },
                    { key: 'summary', label: '成果结论', children: boundedParagraph(goalOutcomeSummary(runSnapshot), { rows: 3, className: 'goal-outcome-summary' }) },
                    { key: 'implementation', label: '实现摘要', children: outcomeSummary?.implementationSummary ? boundedParagraph(outcomeSummary.implementationSummary, { rows: 3 }) : '-' },
                    { key: 'verification', label: '验证摘要', children: outcomeSummary?.verificationSummary ? boundedParagraph(outcomeSummary.verificationSummary, { rows: 3 }) : '-' },
                    { key: 'risks', label: '剩余风险', children: outcomeSummary?.residualRisks?.length ? compactTags(outcomeSummary.residualRisks, 4) : '-' },
                    { key: 'nextActions', label: '后续动作', children: outcomeSummary?.nextActions?.length ? compactTags(outcomeSummary.nextActions, 4) : '-' },
                    { key: 'source', label: '结果来源', children: outcomeResult ? `${outcomeResult.sourceType || '-'} / ${outcomeResult.sourceRef || '-'}` : '-' },
                    { key: 'evidence', label: '结果证据', children: outcomeResult?.evidenceRefs?.length ? compactTags(outcomeResult.evidenceRefs) : '-' }
                  ]}
                />
                <CardGrid>
                  <ProCard title={modelLabels.doDAssessment} colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeDoDItems.length ? (
                      <ProTable
                        rowKey="key"
                        search={false}
                        options={false}
                        pagination={false}
                        size="small"
                        columns={[
                          { title: modelLabels.doDItem, dataIndex: 'item', ellipsis: true },
                          { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(String(item.state)) },
                          { title: '证据', dataIndex: 'evidenceRefs', ellipsis: true, render: (_, item) => compactTags((item.evidenceRefs as string[]) ?? [], 4) },
                          { title: '未满足原因', dataIndex: 'failureReason', ellipsis: true, render: (_, item) => item.failureReason || '-' }
                        ]}
                        dataSource={outcomeDoDItems}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}尚未输出${modelLabels.doDAssessment}`} />
                    )}
                  </ProCard>
                  <ProCard title={modelLabels.artifact} colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeArtifacts.length > 0 ? (
                      <ProTable<Artifact>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={false}
                        size="small"
                        columns={[
                          { title: '名称/URI', ellipsis: true, render: (_, item) => item.name || item.uri || item.id },
                          { title: '类型', dataIndex: 'artifactType', width: 120 },
                          { title: '分类', dataIndex: 'category', width: 130, render: (_, item) => item.category || '-' },
                          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
                          { title: '摘要', dataIndex: 'summary', ellipsis: true },
                          {
                            title: '操作',
                            valueType: 'option',
                            render: (_, item) => item.uri ? (
                              <Button size="small" type="link" href={workorchApi.artifactContentURL(item.uri)} target="_blank">
                                下载
                              </Button>
                            ) : null
                          }
                        ]}
                        dataSource={outcomeArtifacts}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}尚未形成${modelLabels.artifact}`} />
                    )}
                  </ProCard>
                  <ProCard title="Artifact Continuation" colSpan={{ xs: 24, lg: 12 }}>
                    {currentArtifactContinuations.length > 0 ? (
                      <ProTable<ArtifactContinuation>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={{ pageSize: 5 }}
                        size="small"
                        columns={[
                          { title: '目标', dataIndex: 'targetRef', ellipsis: true },
                          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
                          { title: '序列', width: 120, render: (_, item) => `${item.receivedSequences?.length ?? 0}/${item.expectedSequence || '-'}` },
                          { title: '字节', dataIndex: 'bytesWritten', width: 110, render: (_, item) => numberText(item.bytesWritten) },
                          { title: '覆盖目标', dataIndex: 'coverageTargets', ellipsis: true, render: (_, item) => compactTags(item.coverageTargets ?? [], 3) },
                          { title: '最近失败', dataIndex: 'failureReason', ellipsis: true, render: (_, item) => item.failureReason || '-' }
                        ]}
                        dataSource={currentArtifactContinuations}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前运行未进入分片产物续写" />
                    )}
                  </ProCard>
                </CardGrid>
                <CardGrid>
                  <ProCard title={modelLabels.toolCall} colSpan={{ xs: 24, lg: 12 }}>
                    {runToolCallRecords.length > 0 ? (
                      <ProTable<ToolCall>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={{ pageSize: 5 }}
                        loading={runToolCallLoading}
                        size="small"
                        columns={[
                          { title: '工具', dataIndex: 'toolName', ellipsis: true },
                          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
                          { title: '结果摘要', dataIndex: 'exitSummary', ellipsis: true },
                          { title: '错误', dataIndex: 'error', ellipsis: true }
                        ]}
                        dataSource={runToolCallRecords}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未产生${modelLabels.toolCall}`} />
                    )}
                  </ProCard>
                  <ProCard title={modelLabels.memory} colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeMemories.length ? (
                      <ProTable<MemoryItem>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={{ pageSize: 5 }}
                        size="small"
                        columns={[
                          { title: '记忆摘要', dataIndex: 'summary', ellipsis: true },
                          { title: '记忆类型', dataIndex: 'memoryType', width: 130 },
                          { title: '来源类型', dataIndex: 'sourceType', width: 110 },
                          { title: '向量状态', width: 100, render: (_, item) => item.vectorReady ? <Tag color="cyan">{item.vectorDimension}</Tag> : '-' }
                        ]}
                        dataSource={outcomeMemories}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未产生或引用${modelLabels.memory}`} />
                    )}
                  </ProCard>
                  <ProCard title={modelLabels.memoryRecall} colSpan={{ xs: 24, lg: 12 }}>
                    {memoryRecallCard(runSnapshot.memoryRecallTraces)}
                  </ProCard>
                  <ProCard title="Web 来源" colSpan={{ xs: 24, lg: 12 }}>
                    {webSourceCard(runWebSources)}
                  </ProCard>
                  <ProCard title="LLM Context Budget" colSpan={{ xs: 24, lg: 12 }}>
                    {llmContextBudgetCard(runLLMContextBudgets, runLLMContextBudgetSummary)}
                  </ProCard>
                  <ProCard title={modelLabels.llmUsage} colSpan={{ xs: 24, lg: 12 }}>
                    {llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)}
                  </ProCard>
                </CardGrid>
              </Space>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请在运行记录中点击“查看成果与过程”加载运行明细" />
            )}
          </ProCard>
          <ProCard title="运行记录（Run）" className="workorch-section">
            <ProTable<Run> rowKey="id" search={false} options={false} pagination={false} columns={runColumns} dataSource={snapshot.runs ?? []} />
          </ProCard>
          <ProCard
            title="任务进度与恢复"
            className="workorch-section"
            extra={stateTag((runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress)?.state)}
          >
            {taskProgressCard(
              runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress,
              runObservability?.repairLoop ?? runSnapshot?.repairLoop
            )}
          </ProCard>
          <ProCard
            title="Goal 收敛投影"
            className="workorch-section"
            extra={<Tag>{runObservability?.acceptance?.activeSliceId ?? runSnapshot?.acceptance?.activeSliceId ?? snapshot.acceptance?.activeSliceId ?? '未激活'}</Tag>}
          >
            {acceptanceCard(runObservability?.acceptance ?? runSnapshot?.acceptance ?? snapshot.acceptance)}
          </ProCard>
          <ProCard
            title="GapBaseline"
            className="workorch-section"
			extra={<Tag>open {(runSnapshot?.gapBaseline?.todoItems ?? snapshot.gapBaseline?.todoItems)?.length ?? 0}</Tag>}
          >
            {gapBaselineCard(runSnapshot?.gapBaseline ?? snapshot.gapBaseline)}
          </ProCard>
          <ProCard
            title="任务计划"
            className="workorch-section"
            extra={<Tag>{(runObservability?.taskPlans ?? runSnapshot?.taskPlans ?? snapshot.taskPlans ?? []).length || (runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions ?? []).length} 个投影</Tag>}
          >
            {taskPlanCard(
              runObservability?.taskPlans ?? runSnapshot?.taskPlans ?? snapshot.taskPlans,
              runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions
            )}
          </ProCard>
          <ProCard
            title="执行分区"
            className="workorch-section"
            extra={<Tag>{(runObservability?.executionSections ?? runSnapshot?.executionSections ?? snapshot.executionSections ?? []).length} 个分区</Tag>}
          >
            {executionSectionsCard(runObservability?.executionSections ?? runSnapshot?.executionSections ?? snapshot.executionSections)}
          </ProCard>
          <ProCard
            title="上下文恢复包"
            className="workorch-section"
            extra={<Tag>{contextBundle?.mode || '未生成'}</Tag>}
          >
            {contextBundleCard(contextBundle)}
          </ProCard>
          <CardGrid className="workorch-section">
            <ProCard title="决策工单（Decision）" colSpan="50%">
              <ProTable<Decision>
                rowKey="id"
                search={false}
                options={false}
                pagination={false}
                columns={[
                  { title: '工单', dataIndex: 'id', copyable: true },
                  { title: '类型', dataIndex: 'decisionKind', width: 150 },
                  { title: '问题', dataIndex: 'question', ellipsis: true, render: (_, item) => boundedParagraph(item.question, { rows: 2 }) },
                  { title: '原因', dataIndex: 'reason', ellipsis: true },
                  { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' },
                  { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                  { title: '解决方式', dataIndex: 'resolutionType' }
                ]}
                dataSource={snapshot.decisions ?? []}
                expandable={{
                  expandedRowRender: (item) => (
                    <CardGrid>
                      <ProCard title="上下文" colSpan={{ xs: 24, lg: 12 }}>
                        <pre className="json-panel json-panel-large">{compactJSON(item.context)}</pre>
                      </ProCard>
                      <ProCard title="解决内容" colSpan={{ xs: 24, lg: 12 }}>
                        <pre className="json-panel json-panel-large">{compactJSON(item.resolutionPayload)}</pre>
                      </ProCard>
                      <ProCard title="自动治理摘要" colSpan={{ xs: 24, lg: 12 }}>
                        <pre className="json-panel json-panel-large">{compactJSON(item.autoGovernance)}</pre>
                      </ProCard>
                    </CardGrid>
                  )
                }}
              />
            </ProCard>
            <ProCard title="审批工单（Approval）" colSpan="50%">
              <ProTable<Approval>
                rowKey="id"
                search={false}
                options={false}
                pagination={false}
                columns={[
                  { title: '审批', dataIndex: 'id', copyable: true },
                  { title: '运行', dataIndex: 'runId', copyable: true, ellipsis: true },
                  { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
                  { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                  { title: '原因', dataIndex: 'reason' },
                  { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' }
                ]}
                dataSource={snapshot.approvals ?? []}
              />
            </ProCard>
          </CardGrid>
          <ProCard title="运行过程与证据" className="workorch-section" extra={runSnapshot ? <Tag>{evidenceCount(runSnapshot)} 条证据</Tag> : null}>
            {runSnapshot ? (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Steps
                  size="small"
                  items={[
                    { title: '运行状态', description: stateLabel(runOutcomeState(runSnapshot, runObservability)), status: 'finish' },
                    { title: '步骤', description: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0}`, status: runObservability?.progress.stepFailed ? 'error' : 'finish' },
                    { title: '工具调用', description: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runToolCallRecords.length}`, status: 'finish' },
                    { title: '校验', description: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? (runValidationRecords.length || runSnapshot.validationResults?.length || 0)}`, status: runOutcomeState(runSnapshot, runObservability) === 'failed' ? 'error' : 'finish' },
                    { title: '终态', description: runObservability?.progress.terminal ? '已进入终态' : '尚未终态', status: runObservability?.progress.terminal ? 'finish' : 'process' }
                  ]}
                />
                {metricGrid([
                  { key: 'steps', label: '步骤完成', value: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0}`, color: runObservability?.progress.stepFailed ? '#cf1322' : '#0958d9' },
                  { key: 'tools', label: '工具完成', value: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runToolCallRecords.length}` },
                  { key: 'validations', label: '校验通过', value: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? (runValidationRecords.length || runSnapshot.validationResults?.length || 0)}`, color: runObservability?.evidence.validationFailed ? '#cf1322' : '#389e0d' },
                  { key: 'patches', label: '补丁（Patch）应用', value: `${runObservability?.evidence.patchApplied ?? 0}/${runObservability?.evidence.patchTotal ?? (runPatchRecords.length || runSnapshot.patches?.length || 0)}` },
                  { key: 'results', label: modelLabels.result, value: runObservability?.evidence.resultTotal ?? runResultRecords.length, color: '#389e0d' },
                  { key: 'artifacts', label: modelLabels.artifact, value: runObservability?.evidence.artifactTotal ?? (runArtifactRecords.length || runSnapshot.artifacts?.length || 0) },
                  { key: 'webSources', label: 'Web 来源', value: webSourceCount, color: webSourceCount ? '#08979c' : undefined },
                  { key: 'llmContextBudgets', label: 'LLM Context', value: runLLMContextBudgets?.length ?? 0, color: (runLLMContextBudgets?.length ?? 0) > 0 ? '#1677ff' : undefined },
                  { key: 'llmUsage', label: modelLabels.llmUsage, value: runLLMUsageSummary?.entryTotal ?? 0, color: runLLMUsageSummary?.entryTotal ? '#d48806' : undefined }
                ])}
                <Descriptions
                  column={3}
                  items={[
                    { key: 'run', label: '运行', children: `${runSnapshot.run.id} / ${stateLabel(runOutcomeState(runSnapshot, runObservability))}` },
                    { key: 'rawRunState', label: '原始运行状态', children: stateLabel(runRawState(runSnapshot, runObservability)) },
                    { key: 'currentStep', label: '当前步骤', children: runObservability?.progress.currentStepId || runSnapshot.steps?.at(-1)?.id || '-' },
                    { key: 'terminal', label: '是否终态', children: runObservability ? (runObservability.progress.terminal ? '是' : '否') : '-' },
                    { key: 'steps', label: '步骤', children: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0} 已完成` },
                    { key: 'toolCalls', label: '工具调用', children: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runToolCallRecords.length} 已完成` },
                    { key: 'patches', label: modelLabels.patch, children: `${runObservability?.evidence.patchApplied ?? 0}/${runObservability?.evidence.patchTotal ?? (runPatchRecords.length || runSnapshot.patches?.length || 0)} 已应用` },
                    { key: 'validations', label: modelLabels.validationResult, children: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? (runValidationRecords.length || runSnapshot.validationResults?.length || 0)} 通过` },
                    { key: 'checkpoints', label: modelLabels.runtimeCheckpoint, children: runObservability?.evidence.runtimeCheckpointTotal ?? runCheckpointRecords.length },
                    { key: 'results', label: modelLabels.result, children: runObservability?.evidence.resultTotal ?? runResultRecords.length },
                    { key: 'artifacts', label: modelLabels.artifact, children: runObservability?.evidence.artifactTotal ?? (runArtifactRecords.length || runSnapshot.artifacts?.length || 0) },
                    { key: 'webSources', label: 'Web 来源', children: webSourceCount },
                    { key: 'llmContextBudgets', label: 'LLM Context', children: runLLMContextBudgets?.length ?? 0 },
                    { key: 'llmUsage', label: modelLabels.llmUsage, children: `${numberText(runLLMUsageSummary?.entryTotal)} 次 / ${numberText(runLLMUsageSummary?.inputTokens)} 输入 / ${numberText(runLLMUsageSummary?.outputTokens)} 输出 / ${numberText(runLLMUsageSummary?.totalTokens)} 总量` },
                    { key: 'selectedCapabilities', label: '选择能力', children: runObservability?.selectedCapabilities?.length ?? runSnapshot.selectedCapabilities?.length ?? 0 },
                    { key: 'usedCapabilities', label: '使用能力', children: runObservability?.usedCapabilities?.length ?? runSnapshot.usedCapabilities?.length ?? 0 },
                    { key: 'events', label: '事件', children: runObservability?.evidence.eventTotal ?? 0 },
                  ]}
                />
                <CardGrid>
                  <ProCard title="已选择 MCP 能力" colSpan={{ xs: 24, lg: 12 }}>
                    {runtimeCapabilityTable(runObservability?.selectedCapabilities ?? runSnapshot.selectedCapabilities, `当前${modelLabels.run}未显式选择${modelLabels.mcpCapability}`)}
                  </ProCard>
                  <ProCard title="实际使用 MCP 能力" colSpan={{ xs: 24, lg: 12 }}>
                    {runtimeCapabilityTable(runObservability?.usedCapabilities ?? runSnapshot.usedCapabilities, `当前${modelLabels.run}未产生${modelLabels.mcpCapability}使用证据`)}
                  </ProCard>
                </CardGrid>
                <ProCard
                  size="small"
                  title="最新检查点"
                  className="workorch-checkpoint-card"
                  extra={runObservability?.latestCheckpoint ? stateTag(runObservability.latestCheckpoint.runStateSnapshot) : <Tag>暂无</Tag>}
                >
                  {runObservability?.latestCheckpoint ? (
                    <Descriptions
                      size="small"
                      column={{ xs: 1, sm: 2, md: 4 }}
                      items={[
                        { key: 'type', label: '检查点类型', children: checkpointTypeLabel(runObservability.latestCheckpoint.checkpointType) },
                        { key: 'state', label: '运行状态', children: stateLabel(runObservability.latestCheckpoint.runStateSnapshot) },
                        { key: 'step', label: '当前步骤', children: boundedParagraph(runObservability.latestCheckpoint.currentStepId, { rows: 1, copyText: runObservability.latestCheckpoint.currentStepId }) },
                        { key: 'blocked', label: '阻塞信息', children: runObservability.latestCheckpoint.blockingTicketId || runObservability.latestCheckpoint.blockingApprovalId || '无阻塞' }
                      ]}
                    />
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}尚未记录${modelLabels.runtimeCheckpoint}`} />
                  )}
                </ProCard>
                <Tabs
                  items={[
                    {
                      key: 'timeline',
                      label: `时间线 ${runObservability?.timeline?.length ?? 0}`,
                      children: (
                        <ProTable<RuntimeTimelineItem>
                          rowKey={(item, index) => `${item.source}-${item.id || item.topic || index}-${index}`}
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: '来源', dataIndex: 'source', width: 110 },
                            { title: '事件', render: (_, item) => timelineLabel(item), ellipsis: true },
                            { title: '运行状态', dataIndex: 'runState', width: 130, render: (_, item) => stateLabel(item.runState) },
                            { title: '步骤', dataIndex: 'stepId', ellipsis: true },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true },
                            { title: '流式事件', render: (_, item) => timelineStreamContent(item), ellipsis: true },
                            { title: '发生时间', dataIndex: 'occurredAt', width: 210, render: (_, item) => timelineOccurredAtContent(item) }
                          ]}
                          expandable={{
                            expandedRowRender: (item) => (
                              <Space direction="vertical" size={8} className="runtime-expanded-detail">
                                {runtimeRawRefActions(item.output ?? item.data)}
                                <pre className="json-panel json-panel-large">{compactJSON(item.output ?? item.data)}</pre>
                              </Space>
                            )
                          }}
                          dataSource={runObservability?.timeline ?? []}
                        />
                      )
                    },
                    {
                      key: 'events',
                      label: `事件 ${runObservability?.evidence.eventTotal ?? runEventRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<EventRecord>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runEventLoading}
                            size="small"
                            columns={[
                              { title: '事件', dataIndex: 'topic', ellipsis: true },
                              { title: '状态', width: 120, render: (_, item) => eventRecordState(item) },
                              { title: '主体', render: (_, item) => eventRecordSubject(item), ellipsis: true },
                              { title: '步骤', dataIndex: 'stepId', ellipsis: true },
                              { title: '摘要', render: (_, item) => eventRecordSummary(item), ellipsis: true },
                              { title: '发生时间', dataIndex: 'occurredAt', width: 210, render: (_, item) => item.occurredAt || '-' }
                            ]}
                            expandable={{
                              expandedRowRender: (item) => (
                                <pre className="json-panel json-panel-large">{compactJSON(eventPayloadDisplay(item.payload ?? item.payloadSummary) ?? { payloadRef: item.payloadRef })}</pre>
                              )
                            }}
                            dataSource={runEventRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunEventPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runEventLoading}
                            >
                              刷新事件
                            </Button>
                            <Button
                              size="small"
                              disabled={!runEventHasMore || !runEventCursorAfterId}
                              onClick={() => void loadRunEventPage(runSnapshot.run.id, runEventCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runEventLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    },
                    {
                      key: 'webSources',
                      label: `Web 来源 ${webSourceCount}`,
                      children: webSourceCard(runWebSources)
                    },
                    {
                      key: 'llmContextBudgets',
                      label: `LLM Context ${runLLMContextBudgets?.length ?? 0}`,
                      children: llmContextBudgetCard(runLLMContextBudgets, runLLMContextBudgetSummary)
                    },
                    {
                      key: 'llmUsage',
                      label: `LLM 使用量 ${runLLMUsageSummary?.entryTotal ?? 0}`,
                      children: llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)
                    },
                    {
                      key: 'steps',
                      label: `步骤 ${runObservability?.progress.stepTotal ?? runStepRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<StepView>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runStepLoading}
                            size="small"
                            columns={[
                              { title: '步骤', dataIndex: 'id', copyable: true, ellipsis: true },
                              { title: '类型', dataIndex: 'kind', width: 180 },
                              { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                              { title: '操作', render: (_, item) => outputText(item.output, 'operation'), ellipsis: true },
                              { title: '服务', render: (_, item) => outputText(item.output, 'serverId'), ellipsis: true },
                              { title: '目标', render: (_, item) => runtimeTarget(item.output), ellipsis: true },
                              { title: '阶段', render: (_, item) => outputText(item.output, 'stage'), ellipsis: true },
                              { title: '下一类型', render: (_, item) => outputText(item.output, 'nextKind'), ellipsis: true }
                            ]}
                            expandable={{
                              expandedRowRender: (item) => (
                                <Space direction="vertical" size={8} className="runtime-expanded-detail">
                                  {runtimeDetailActions(runSnapshot.workspace.id, runSnapshot.run.id, 'steps', item.id, [
                                    { fileName: 'input.json', label: 'Input' },
                                    { fileName: 'output.json', label: 'Output' }
                                  ])}
                                  {runtimeRawRefActions(item.output)}
                                  <pre className="json-panel json-panel-large">{compactJSON(item.output)}</pre>
                                </Space>
                              )
                            }}
                            dataSource={runStepRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunStepPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runStepLoading}
                            >
                              刷新步骤
                            </Button>
                            <Button
                              size="small"
                              disabled={!runStepHasMore || !runStepCursorAfterId}
                              onClick={() => void loadRunStepPage(runSnapshot.run.id, runStepCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runStepLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    },
                    {
                      key: 'toolCalls',
                      label: `工具调用 ${runObservability?.evidence.toolCallTotal ?? runToolCallRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<ToolCall>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runToolCallLoading}
                            size="small"
                            columns={[
                              { title: modelLabels.toolCall, dataIndex: 'id', copyable: true, ellipsis: true },
                              { title: '工具', dataIndex: 'toolName', ellipsis: true },
                              { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                              { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
                              { title: '摘要', dataIndex: 'exitSummary', ellipsis: true },
                              { title: '错误', dataIndex: 'error', ellipsis: true }
                            ]}
                            expandable={{
                              expandedRowRender: (item) => (
                                <Space direction="vertical" size={8} className="runtime-expanded-detail">
                                  {runtimeDetailActions(runSnapshot.workspace.id, runSnapshot.run.id, 'tool-calls', item.id, [
                                    { fileName: 'arguments.json', label: 'Arguments' },
                                    { fileName: 'output.json', label: 'Output' }
                                  ])}
                                  <pre className="json-panel json-panel-large">{compactJSON(item)}</pre>
                                </Space>
                              )
                            }}
                            dataSource={runToolCallRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunToolCallPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runToolCallLoading}
                            >
                              刷新工具调用
                            </Button>
                            <Button
                              size="small"
                              disabled={!runToolCallHasMore || !runToolCallCursorAfterId}
                              onClick={() => void loadRunToolCallPage(runSnapshot.run.id, runToolCallCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runToolCallLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    },
                    {
                      key: 'validations',
                      label: `校验结果 ${runObservability?.evidence.validationTotal ?? runValidationRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<ValidationResult>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runValidationLoading}
                            size="small"
                            columns={[
                              { title: modelLabels.validationResult, dataIndex: 'id', copyable: true, ellipsis: true },
                              { title: '类型', dataIndex: 'validationKind', width: 160 },
                              { title: '目标', dataIndex: 'targetRef', ellipsis: true },
                              { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                              { title: '摘要', dataIndex: 'summary', ellipsis: true }
                            ]}
                            dataSource={runValidationRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunValidationPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runValidationLoading}
                            >
                              刷新校验
                            </Button>
                            <Button
                              size="small"
                              disabled={!runValidationHasMore || !runValidationCursorAfterId}
                              onClick={() => void loadRunValidationPage(runSnapshot.run.id, runValidationCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runValidationLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    },
                    {
                      key: 'patches',
                      label: `${modelLabels.patch} ${runObservability?.evidence.patchTotal ?? runPatchRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<Patch>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runPatchLoading}
                            size="small"
                            columns={[
                              { title: modelLabels.patch, dataIndex: 'id', copyable: true, ellipsis: true },
                              { title: '类型', dataIndex: 'patchType', width: 140 },
                              { title: '目标', dataIndex: 'targetRef', ellipsis: true },
                              { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                              { title: '增删行', width: 100, render: (_, item) => `+${item.addedLines ?? 0}/-${item.deletedLines ?? 0}` },
                              { title: '审批', dataIndex: 'approvalId', copyable: true, ellipsis: true, render: (_, item) => item.approvalId || item.revertApprovalId || '-' },
                              { title: '摘要', dataIndex: 'summary', ellipsis: true }
                            ]}
                            dataSource={runPatchRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunPatchPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runPatchLoading}
                            >
                              刷新补丁
                            </Button>
                            <Button
                              size="small"
                              disabled={!runPatchHasMore || !runPatchCursorAfterId}
                              onClick={() => void loadRunPatchPage(runSnapshot.run.id, runPatchCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runPatchLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    },
                    {
                      key: 'checkpoints',
                      label: `${modelLabels.runtimeCheckpoint} ${runObservability?.evidence.runtimeCheckpointTotal ?? runCheckpointRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<RuntimeCheckpoint>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runCheckpointLoading}
                            size="small"
                            columns={[
                              { title: modelLabels.runtimeCheckpoint, dataIndex: 'id', copyable: true, ellipsis: true },
                              { title: '类型', dataIndex: 'checkpointType', width: 180 },
                              { title: '运行状态', dataIndex: 'runStateSnapshot', width: 140, render: (_, item) => stateTag(item.runStateSnapshot) },
                              { title: '当前步骤', dataIndex: 'currentStepId', ellipsis: true },
                              { title: '决策', dataIndex: 'blockingTicketId', ellipsis: true },
                              { title: '审批', dataIndex: 'blockingApprovalId', ellipsis: true }
                            ]}
                            expandable={{
                              expandedRowRender: (item) => (
                                <Space direction="vertical" size={8} className="runtime-expanded-detail">
                                  {runtimeDetailActions(runSnapshot.workspace.id, runSnapshot.run.id, 'runtime-checkpoints', item.id, [
                                    { fileName: 'data.json', label: 'Data' }
                                  ])}
                                  <pre className="json-panel json-panel-large">{compactJSON(item)}</pre>
                                </Space>
                              )
                            }}
                            dataSource={runCheckpointRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunCheckpointPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runCheckpointLoading}
                            >
                              刷新检查点
                            </Button>
                            <Button
                              size="small"
                              disabled={!runCheckpointHasMore || !runCheckpointCursorAfterId}
                              onClick={() => void loadRunCheckpointPage(runSnapshot.run.id, runCheckpointCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runCheckpointLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    },
                    {
                      key: 'results',
                      label: `结果 ${runObservability?.evidence.resultTotal ?? runResultRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<Result>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runResultLoading}
                            size="small"
                            columns={[
                              { title: modelLabels.result, dataIndex: 'id', copyable: true, ellipsis: true },
                              { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                              { title: '标题', dataIndex: 'title', ellipsis: true },
                              { title: '摘要', dataIndex: 'summary', ellipsis: true },
                              { title: '来源', render: (_, item) => `${item.sourceType || '-'} / ${item.sourceRef || '-'}`, ellipsis: true }
                            ]}
                            expandable={{
                              expandedRowRender: (item) => (
                                <Space direction="vertical" size={8} className="runtime-expanded-detail">
                                  {runtimeDetailActions(runSnapshot.workspace.id, runSnapshot.run.id, 'results', item.id, [
                                    { fileName: 'dod-assessment.json', label: 'DoD' },
                                    { fileName: 'summary-detail.json', label: 'Detail' }
                                  ])}
                                  <pre className="json-panel json-panel-large">{compactJSON(item)}</pre>
                                </Space>
                              )
                            }}
                            dataSource={runResultRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunResultPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runResultLoading}
                            >
                              刷新结果
                            </Button>
                            <Button
                              size="small"
                              disabled={!runResultHasMore || !runResultCursorAfterId}
                              onClick={() => void loadRunResultPage(runSnapshot.run.id, runResultCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runResultLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    },
                    {
                      key: 'artifacts',
                      label: `交付物 ${runObservability?.evidence.artifactTotal ?? runArtifactRecords.length}`,
                      children: (
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                          <ProTable<Artifact>
                            rowKey="id"
                            search={false}
                            options={false}
                            pagination={{ pageSize: 8 }}
                            loading={runArtifactLoading}
                            size="small"
                            columns={[
                              { title: modelLabels.artifact, dataIndex: 'id', copyable: true, ellipsis: true },
                              { title: '类型', dataIndex: 'artifactType', width: 140 },
                              { title: '分类', dataIndex: 'category', width: 140, render: (_, item) => item.category || '-' },
                              { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                              { title: '名称/URI', render: (_, item) => item.name || item.uri || '-', ellipsis: true },
                              { title: '摘要', dataIndex: 'summary', ellipsis: true },
                              {
                                title: '操作',
                                valueType: 'option',
                                render: (_, item) => item.uri ? (
                                  <Button size="small" type="link" href={workorchApi.artifactContentURL(item.uri)} target="_blank">
                                    下载
                                  </Button>
                                ) : null
                              }
                            ]}
                            dataSource={runArtifactRecords}
                          />
                          <Space>
                            <Button
                              size="small"
                              onClick={() => void loadRunArtifactPage(runSnapshot.run.id).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runArtifactLoading}
                            >
                              刷新交付物
                            </Button>
                            <Button
                              size="small"
                              disabled={!runArtifactHasMore || !runArtifactCursorAfterId}
                              onClick={() => void loadRunArtifactPage(runSnapshot.run.id, runArtifactCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
                              loading={runArtifactLoading}
                            >
                              加载更多
                            </Button>
                          </Space>
                        </Space>
                      )
                    }
                  ]}
                />
              </Space>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择一个运行记录查看过程和证据" />
            )}
          </ProCard>
        </>
      ) : (
        <Empty description={loading ? '正在加载工作空间详情' : '未选择工作空间'} />
      )}
      <Modal
        title="调整工作空间执行人格（Soul）"
        open={soulBindingEditorOpen}
        confirmLoading={soulBindingSaving}
        onCancel={() => setSoulBindingEditorOpen(false)}
        onOk={() => soulBindingsForm.submit()}
        okText="保存执行配置"
        cancelText="取消"
        width={760}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Typography.Paragraph type="secondary">
            执行人格（Soul）是工作空间级执行配置。先维护目标（Goal）和验收标准（DoD），再按需设置默认执行人格与阶段绑定；修改后由下一次新执行从 owner 状态编译，不提供单次运行覆盖。
          </Typography.Paragraph>
          <Descriptions
            size="small"
            column={1}
            items={[
              { key: 'workspace', label: '工作空间', children: snapshot?.workspace ? `${snapshot.workspace.title || snapshot.workspace.id} (${snapshot.workspace.id})` : '-' },
              { key: 'priority', label: '启用优先级', children: compactTags(soulBindings?.priorityPolicy ?? []) }
            ]}
          />
          {souls.length ? (
            <Form
              form={soulBindingsForm}
              layout="vertical"
              onFinish={(values) => void saveWorkspaceSoulBindings(values as { defaultSoulId?: string; bindings?: Record<string, string> })}
            >
              <Form.Item name="defaultSoulId" label="工作空间默认执行人格（Soul）">
                <Select allowClear placeholder="未绑定时不注入默认执行人格" options={soulOptions} />
              </Form.Item>
              <Space className="workorch-section">
                <Typography.Text type="secondary">阶段执行人格绑定</Typography.Text>
                <Button
                  size="small"
                  onClick={() => soulBindingsForm.setFieldsValue({ bindings: stageBindings(soulBindingStages, soulBindingsForm.getFieldValue('defaultSoulId')) })}
                >
                  默认执行人格应用到全部阶段
                </Button>
              </Space>
              <div className="soul-binding-grid">
                {soulBindingStages.map((stage) => (
                  <Form.Item key={stage} name={['bindings', stage]} label={soulBindingStageLabel(stage)}>
                    <Select allowClear placeholder="继承默认执行人格" options={soulOptions} />
                  </Form.Item>
                ))}
              </div>
            </Form>
          ) : (
            <Empty description="暂无可用执行人格，可先到系统配置创建执行人格" />
          )}
        </Space>
      </Modal>
      <Modal
        title="删除工作空间"
        open={workspaceDeleteOpen}
        okText="确认删除"
        okButtonProps={{ danger: true }}
        confirmLoading={workspaceDeleting}
        onCancel={() => setWorkspaceDeleteOpen(false)}
        onOk={() => void submitWorkspaceDelete()}
      >
        <Form form={workspaceDeleteForm} layout="vertical" initialValues={{ purge: false }}>
          <Typography.Paragraph>
            确认删除「{snapshot?.workspace.title || workspaceId}」？默认仅删除工作空间实体记录，不会清理运行数据或工作目录。
          </Typography.Paragraph>
          <Form.Item name="purge" valuePropName="checked">
            <Checkbox>
              同时删除该工作空间的目标、验收标准、运行数据、治理记录、交付物索引、记忆（Memory）和工作目录
            </Checkbox>
          </Form.Item>
          <Typography.Paragraph type="secondary">
            勾选后会清理 file backend 下该工作空间的数据目录，并删除工作空间（Workspace）的工作目录。该操作不可恢复。
          </Typography.Paragraph>
        </Form>
      </Modal>
      <Modal
        title={editingGoal ? '修改目标' : '定义目标'}
        open={goalDefinitionOpen}
        confirmLoading={businessSubmitting}
        onCancel={() => {
          setGoalDefinitionOpen(false);
          setEditingGoal(undefined);
          goalForm.resetFields();
        }}
        onOk={() => goalForm.submit()}
      >
        <Form
          form={goalForm}
          layout="vertical"
          onFinish={async (values: { text: string }) => {
            if (!workspaceId) {
              message.error('请先选择工作空间');
              return;
            }
            setBusinessSubmitting(true);
            try {
              if (editingGoal) {
                await workorchApi.updateGoal(workspaceId, editingGoal.id, values.text);
              } else {
                await workorchApi.defineGoal(workspaceId, values.text);
              }
              await loadWorkspaceContext(workspaceId);
              setGoalDefinitionOpen(false);
              setEditingGoal(undefined);
              goalForm.resetFields();
              message.success(editingGoal ? '目标已修改' : '目标已定义');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="text" label="目标" rules={[{ required: true, whitespace: true, min: 1 }]}>
            <Input.TextArea rows={4} placeholder="描述该工作空间要达成的业务结果" />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title={editingDoD ? '修改验收标准' : '定义验收标准'}
        open={dodDefinitionOpen}
        confirmLoading={businessSubmitting}
        onCancel={() => {
          setDoDDefinitionOpen(false);
          setEditingDoD(undefined);
          dodForm.resetFields();
        }}
        onOk={() => dodForm.submit()}
      >
        <Form
          form={dodForm}
          layout="vertical"
          onFinish={async (values: { items: string }) => {
            if (!workspaceId) {
              message.error('请先选择工作空间');
              return;
            }
            setBusinessSubmitting(true);
            try {
              const items = values.items.split('\n').map((item) => item.trim()).filter(Boolean);
              if (editingDoD) {
                await workorchApi.updateDoD(workspaceId, editingDoD.id, items);
              } else {
                await workorchApi.defineDoD(workspaceId, items);
              }
              await loadWorkspaceContext(workspaceId);
              setDoDDefinitionOpen(false);
              setEditingDoD(undefined);
              dodForm.resetFields();
              message.success(editingDoD ? '验收标准已修改' : '验收标准已定义');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="items" label="验收项" rules={[{ required: true }]}>
            <Input.TextArea rows={5} placeholder={'每行一个验收标准\n示例：服务已完成本地部署并可访问'} />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title="Goal / DoD 质量评分"
        open={qualityCheckOpen}
        onCancel={() => setQualityCheckOpen(false)}
        footer={<Button onClick={() => setQualityCheckOpen(false)}>关闭</Button>}
        width={760}
      >
        {qualityCheck ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions
              bordered
              size="small"
              column={{ xs: 1, md: 2 }}
              items={[
                { key: 'status', label: '状态', children: stateTag(qualityCheck.status) },
                { key: 'executionScore', label: '执行评分', children: numberText(qualityCheck.executionScore) },
                { key: 'verificationScore', label: '验证评分', children: numberText(qualityCheck.verificationScore) },
                { key: 'executable', label: '可执行', children: qualityCheck.executable ? <Tag color="green">是</Tag> : <Tag color="red">否</Tag> },
                { key: 'verifiable', label: '可验证', children: qualityCheck.verifiable ? <Tag color="green">是</Tag> : <Tag color="red">否</Tag> },
                { key: 'goal', label: 'Goal', children: qualityCheck.goalId || '-' },
                { key: 'dod', label: 'DoD', children: qualityCheck.doDId || '-' }
              ]}
            />
            {qualityCheck.missingConstraints?.length ? (
              <div>
                <Typography.Text strong>缺失约束</Typography.Text>
                <div>{compactTags(qualityCheck.missingConstraints, 8)}</div>
              </div>
            ) : null}
            {qualityCheck.suggestions?.length ? (
              <div>
                <Typography.Text strong>改进建议</Typography.Text>
                <List
                  size="small"
                  dataSource={qualityCheck.suggestions}
                  renderItem={(item) => <List.Item>{item}</List.Item>}
                />
              </div>
            ) : null}
            {qualityCheck.issues?.length ? (
              <div>
                <Typography.Text strong>问题明细</Typography.Text>
                <List
                  size="small"
                  dataSource={qualityCheck.issues}
                  renderItem={(item) => (
                    <List.Item>
                      <List.Item.Meta
                        title={<Space><Tag>{item.severity}</Tag><Typography.Text>{item.code}</Typography.Text><Typography.Text type="secondary">{item.target}</Typography.Text></Space>}
                        description={item.suggestion ? `${item.message} ${item.suggestion}` : item.message}
                      />
                    </List.Item>
                  )}
                />
              </div>
            ) : null}
          </Space>
        ) : (
          <Empty description="暂无质量评分结果" />
        )}
      </Modal>
      <Modal
        title={activationIntent === 'restart' ? '创建新的工作空间执行' : '恢复当前工作空间执行'}
        open={activationOpen}
        confirmLoading={activationLoading}
        onCancel={() => setActivationOpen(false)}
        onOk={() => activationForm.submit()}
        okText={activationIntent === 'restart' ? '创建新的后台执行' : '恢复当前执行'}
        okButtonProps={{ disabled: goals.length === 0 || readyDoDs.length === 0 || (activationIntent === 'restart' && !activationLLMReady) }}
      >
        <Form
          form={activationForm}
          layout="vertical"
          onFinish={(values: WorkspaceExecutionActivationRequest & { llmTargetKey?: string }) => void submitWorkspaceExecution(values)}
        >
          <Form.Item name="intent" hidden>
            <Input />
          </Form.Item>
          {activationIntent === 'resume' ? (
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <Typography.Text type="secondary">
                Resume 只继续当前 Run 已封存的定义、模型和执行上下文；不会读取或改写 Workspace 的下次运行偏好。
              </Typography.Text>
              <Descriptions
                size="small"
                column={1}
                items={[
                  { key: 'run', label: '当前 Run', children: activeRun?.id || '-' },
                  { key: 'sealedSelection', label: '当前 Run 封存模型', children: providerModelTargetLabel(activeRun?.sealedLLMSelection) },
                  { key: 'nextSelection', label: '下次 fresh activation 偏好', children: providerModelTargetLabel(snapshot?.workspace.defaultLLMSelection) }
                ]}
              />
            </Space>
          ) : (
            <>
              <Typography.Text type="warning">
                该操作会从 Workspace 当前定义与配置创建 fresh activation；不会继续或迁移旧 Run 的持久化状态。
              </Typography.Text>
              <Form.Item
                name="llmTargetKey"
                label="下次 fresh activation 模型"
                rules={[{ required: true, message: '请选择可用的对话模型' }]}
                extra={
                  !activationLLMReady
                    ? '当前没有可用的对话模型，请先在系统配置中完成模型设置'
                    : defaultChatModelId
                      ? `系统默认：${defaultChatModelId}${defaultChatReady ? '' : `（当前不可用，已自动选择 ${activationReadyTarget?.modelId}）`}`
                      : `已自动选择模型：${activationReadyTarget?.modelId}`
                }
              >
                <Select
                  showSearch
                  placeholder="选择可用对话模型"
                  options={activationLLMTargetOptions.map((item) => ({ value: item.value, label: item.label }))}
                  onChange={(value?: string) => {
                    const selected = activationLLMTargetOptions.find((item) => item.value === value);
                    activationForm.setFieldsValue({
                      llmTarget: {
                        providerId: selected?.providerId || 'ai-proxy',
                        modelId: selected?.modelId
                      },
                      reasoningEffort: undefined
                    });
                  }}
                />
              </Form.Item>
              <Form.Item name={['llmTarget', 'providerId']} hidden initialValue="ai-proxy">
                <Input />
              </Form.Item>
              <Form.Item name={['llmTarget', 'modelId']} hidden>
                <Input />
              </Form.Item>
              <Form.Item name="reasoningEffort" label="推理强度" extra={selectedOption?.defaultReasoningEffort ? `模型默认：${selectedOption.defaultReasoningEffort}` : '留空时使用模型默认策略'}>
                <Select
                  allowClear
                  placeholder="模型默认"
                  disabled={!selectedOption?.reasoningSupported || !selectedOption?.reasoningValid}
                  options={(selectedOption?.reasoningEfforts ?? []).map((effort) => ({ value: effort, label: effort }))}
                />
              </Form.Item>
              <Form.Item name="soulId" label="下次 fresh activation 默认执行人格（Soul）">
                <Select allowClear placeholder="继承工作空间默认执行人格" options={soulOptions} />
              </Form.Item>
              <Typography.Text type="secondary">下次 fresh activation 阶段执行人格</Typography.Text>
              <div className="soul-binding-grid soul-binding-grid-compact">
                {soulBindingStages.map((stage) => (
                  <Form.Item key={stage} name={['soulBindings', stage]} label={soulBindingStageLabel(stage)}>
                    <Select allowClear placeholder="继承工作空间配置" options={soulOptions} />
                  </Form.Item>
                ))}
              </div>
            </>
          )}
        </Form>
      </Modal>
      <FloatButton.BackTop className="workspace-back-top" visibilityHeight={240} tooltip="返回顶部" />
    </PageContainer>
  );
}

type MCPServerFormValues = {
  id: string;
  name: string;
  transport: string;
  endpoint?: string;
  enabled: boolean;
  allowedTools?: string;
  allowedResources?: string;
  allowedPrompts?: string;
  defaultApprovalPolicy?: string;
  sessionPolicy?: string;
  timeoutMillis?: number;
};


export function mcpServerPayload(values: MCPServerFormValues): MCPServer {
  return {
    id: values.id.trim(),
    name: values.name.trim(),
    transport: values.transport,
    endpoint: values.endpoint?.trim(),
    enabled: values.enabled !== false,
    allowedTools: splitLines(values.allowedTools),
    allowedResources: splitLines(values.allowedResources),
    allowedPrompts: splitLines(values.allowedPrompts),
    defaultApprovalPolicy: values.defaultApprovalPolicy?.trim(),
    sessionPolicy: values.sessionPolicy?.trim(),
    timeoutMillis: values.timeoutMillis
  };
}

export function displayMCPEndpoint(endpoint?: string) {
  const value = endpoint?.trim() ?? '';
  const marker = '.workorch-data/system/mcp-endpoints/';
  const index = value.indexOf(marker);
  return index >= 0 ? value.slice(index) : value;
}
