import { useCallback, useEffect, useRef, useState } from 'react';
import { App as AntApp, Badge, Button, Descriptions, Empty, Space, Tag, Typography } from 'antd';
import { CardGrid, PageContainer, ProCard } from '../components/proCompat';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { useSSE } from '../hooks';
import { errorMessage, shortId, stateTag, stateLabel, boundedParagraph, metricGrid } from '../utils';
import { projectionStatusSummary, workspaceBoardActivationRequest, workspaceBoardActivationHint } from '../helpers';
import { workspaceDefinitionClarification } from '../governanceMutationTarget';
import type { ControlPlaneProps, ProjectionStatus, Workspace, WorkspaceExecutionIntentRequest } from '../types';
import { enterAuthoringWorkspace } from '../workspaceAuthoring';
import { WorkspaceSetupModal } from './components/WorkspaceSetupModal';
import { AuthoringSummary } from './components/AuthoringSummary';
import { WorkspaceDraftDetails } from './components/WorkspaceDraftDetails';
import { WorkspaceAuthoringCandidate } from './components/WorkspaceAuthoringCandidate';
import { WorkspaceAuthoringConfirmation } from './components/WorkspaceAuthoringConfirmation';

type WorkspaceBoardCardItem = {
  workspace: Workspace;
};

function isWorkspaceProjectionChangedEvent(event: { topic?: string; payload?: Record<string, unknown> }) {
  return event.topic === '/workorchd/workspace/projection/changed' || event.payload?.type === 'workspace_projection_changed';
}

export function WorkspaceBoardPage({ onOpenWorkspace }: ControlPlaneProps) {
  const { message } = AntApp.useApp();
  const [boardItems, setBoardItems] = useState<WorkspaceBoardCardItem[]>([]);
  const [projection, setProjection] = useState<ProjectionStatus | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [workspaceSetupOpen, setWorkspaceSetupOpen] = useState(false);
  const [draftWorkspaceId, setDraftWorkspaceId] = useState<string>();
  const [candidateWorkspaceId, setCandidateWorkspaceId] = useState<string>();
  const [confirmationWorkspaceId, setConfirmationWorkspaceId] = useState<string>();
  const boardRequest = useRef(0);
  const mounted = useRef(true);

  const loadBoard = useCallback(async (showLoading = true) => {
    const token = ++boardRequest.current;
    const isCurrent = () => mounted.current && token === boardRequest.current;
    if (showLoading) {
      setLoading(true);
    }
    try {
      const nextProjection = await workorchApi.listWorkspaceProjection();
      if (!isCurrent()) return;
      setProjection(nextProjection.projection);
      setBoardItems(nextProjection.workspaces.map((workspace) => ({ workspace })));
    } catch (error) {
      if (isCurrent()) message.error(errorMessage(error));
    } finally {
      if (isCurrent()) {
        setLoading(false);
      }
    }
  }, [message]);

  useEffect(() => {
    mounted.current = true;
    void loadBoard();
    return () => { mounted.current = false; boardRequest.current++; };
  }, [loadBoard]);

  useSSE((event) => {
    if (!isWorkspaceProjectionChangedEvent(event)) {
      return;
    }
    void loadBoard(false);
  }, {
    debounceMs: 250,
    onReconnect: () => void loadBoard(false),
    onExhausted: () => {
      message.warning('事件流连接已断开，请检查服务状态或手动刷新。');
      void loadBoard(false);
    }
  });

  const trackWorkspaceExecutionInBackground = (workspace: Workspace, request: WorkspaceExecutionIntentRequest) => {
    const workspaceId = workspace.id;
    void workorchApi.submitWorkspaceExecution(workspaceId, request)
      .then((admission) => {
        if (admission.accepted) {
          message.success(request.intent === 'restart' ? 'Workspace owner 已受理新执行请求' : 'Workspace owner 已受理恢复请求');
        } else if (admission.state === 'pending') {
          message.warning(`原执行请求 ${admission.pendingRequestId} 结果未决；请保留原请求身份，不要换请求号重试。`);
        } else {
          message.error(`Workspace owner 拒绝执行请求：${admission.diagnostic || admission.reason}`);
        }
        void loadBoard();
      })
      .catch((error: unknown) => {
        message.error(`工作空间执行提交失败：${errorMessage(error)}`);
        void loadBoard();
      });
  };

  const submitWorkspaceExecution = async (workspace: Workspace) => {
    const detail = await workorchApi.readWorkspaceSnapshotProjection(workspace.id);
    const item = { snapshot: detail.snapshot };
    const request = workspaceBoardActivationRequest(item);
    if (!request) {
      message.warning(workspaceBoardActivationHint(item));
      if (workspaceDefinitionClarification(detail.snapshot)) {
        onOpenWorkspace(workspace.id);
      }
      return;
    }
    message.info('正在提交工作空间执行意图');
    trackWorkspaceExecutionInBackground(workspace, request);
  };

  const handleWorkspaceCreated = (workspaceId: string, isCurrent: () => boolean) =>
    enterAuthoringWorkspace(workspaceId, loadBoard, isCurrent, onOpenWorkspace);

  return (
    <PageContainer
      title="业务总览"
      subTitle={boardItems.length > 0 && projection
        ? projectionStatusSummary(projection)
        : '创建工作空间后，可在这里查看目标、运行、治理阻塞和交付状态'}
      extra={<Space><Button type="primary" onClick={() => setWorkspaceSetupOpen(true)}>创建工作空间</Button><Button onClick={() => void loadBoard()} loading={loading}>刷新</Button></Space>}
    >
      {boardItems.length === 0 && !loading ? (
        <ProCard className="workspace-board-empty" bordered>
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={(
              <Space direction="vertical" size="small">
                <Typography.Text strong>还没有工作空间</Typography.Text>
                <Typography.Text type="secondary">从这里创建一个工作空间，定义目标、验收标准和执行人格后，即可提交执行并在此跟踪全局进展。</Typography.Text>
              </Space>
            )}
          >
            <Button type="primary" onClick={() => setWorkspaceSetupOpen(true)}>创建第一个工作空间</Button>
          </Empty>
        </ProCard>
      ) : (
        <CardGrid className="workspace-board-grid" loading={loading}>
          {boardItems.map((item) => {
            const { workspace } = item;
            const hasActiveRun = Boolean(workspace.activeRunId);
            return (
              <ProCard
                key={workspace.id}
                title={workspace.title || workspace.id}
                subTitle={workspace.id}
                colSpan={{ xs: 24, md: 12, xl: 8 }}
                bordered
                extra={stateTag(workspace.state)}
              >
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                  <div className="workorch-card-summary">
                    <div>
                      <Typography.Text type="secondary">当前运行（Run）</Typography.Text>
                      <div>{workspace.activeRunId ? <Space>{shortId(workspace.activeRunId)}<Tag>{stateLabel(workspace.state)}</Tag></Space> : <Typography.Text type="secondary">无当前运行（Run）</Typography.Text>}</div>
                    </div>
                    <div>
                      <Typography.Text type="secondary">治理阻塞</Typography.Text>
                      <div><Badge status={workspace.state === 'blocked' || workspace.state === 'waiting_decision' ? 'warning' : 'success'} text={stateLabel(workspace.state)} /></div>
                    </div>
                  </div>
                  {metricGrid([
                    { key: 'workspace', label: modelLabels.workspace, value: 1 },
                    { key: 'activeRun', label: modelLabels.run, value: hasActiveRun ? 1 : 0 },
                    { key: 'state', label: '状态', value: stateLabel(workspace.state) }
                  ])}
                  {workspace.authoringSummary ? <AuthoringSummary compact summary={workspace.authoringSummary} onOpenSpecification={() => onOpenWorkspace(workspace.id)} /> : null}
                  <Descriptions
                    column={1}
                    size="small"
                    items={[
                      { key: 'deliverableDirectory', label: '工作目录', children: boundedParagraph(workspace.deliverableDirectory, { rows: 1, copyText: workspace.deliverableDirectory }) },
                      { key: 'activeRun', label: '活跃运行', children: workspace.activeRunId || '-' },
                      { key: 'defaultSoul', label: '默认 Soul', children: workspace.defaultSoulId || '-' }
                    ]}
                  />
                </Space>
                <Space className="workorch-section" wrap>
                  {workspace.authoringSummary && <Button onClick={() => setDraftWorkspaceId(workspace.id)}>查看草稿与创建状态</Button>}
                  <Button type="primary" onClick={() => onOpenWorkspace(workspace.id)}>进入工作空间</Button>
                  <Button onClick={() => void submitWorkspaceExecution(workspace).catch((error: unknown) => message.error(errorMessage(error)))}>提交执行</Button>
                </Space>
              </ProCard>
            );
          })}
        </CardGrid>
      )}
      <WorkspaceSetupModal
        open={workspaceSetupOpen}
        onClose={() => setWorkspaceSetupOpen(false)}
        onCreated={handleWorkspaceCreated}
      />
      <WorkspaceDraftDetails workspaceId={draftWorkspaceId} onClose={() => setDraftWorkspaceId(undefined)} onOpenCandidate={setCandidateWorkspaceId} />
      <WorkspaceAuthoringCandidate workspaceId={candidateWorkspaceId} onClose={() => setCandidateWorkspaceId(undefined)} onOpenConfirmation={setConfirmationWorkspaceId} />
      <WorkspaceAuthoringConfirmation workspaceId={confirmationWorkspaceId} onClose={() => setConfirmationWorkspaceId(undefined)} />
    </PageContainer>
  );
}
