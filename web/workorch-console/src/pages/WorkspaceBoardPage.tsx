import { useCallback, useEffect, useState } from 'react';
import { App as AntApp, Badge, Button, Descriptions, Empty, Space, Tag, Typography } from 'antd';
import { CardGrid, PageContainer, ProCard } from '../components/proCompat';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { useSSE } from '../hooks';
import { errorMessage, shortId, stateTag, stateLabel, boundedParagraph, metricGrid } from '../utils';
import { projectionStatusSummary, workspaceBoardActivationRequest, workspaceBoardActivationHint } from '../helpers';
import type { ControlPlaneProps, ProjectionStatus, Workspace, WorkspaceExecutionActivationRequest } from '../types';

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

  const loadBoard = useCallback(async (showLoading = true) => {
    if (showLoading) {
      setLoading(true);
    }
    try {
      const nextProjection = await workorchApi.listWorkspaceProjection();
      setProjection(nextProjection.projection);
      setBoardItems(nextProjection.workspaces.map((workspace) => ({ workspace })));
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      if (showLoading) {
        setLoading(false);
      }
    }
  }, [message]);

  useEffect(() => {
    void loadBoard();
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

  const trackWorkspaceResumeInBackground = (workspaceId: string, request: WorkspaceExecutionActivationRequest) => {
    void workorchApi.resumeWorkspaceExecution(workspaceId, request)
      .then(() => {
        message.success('工作空间执行已提交');
        void loadBoard();
      })
      .catch((error: unknown) => {
        message.error(`工作空间执行提交失败：${errorMessage(error)}`);
        void loadBoard();
      });
  };

  const resumeWorkspaceExecution = async (workspace: Workspace) => {
    const detail = await workorchApi.readWorkspaceSnapshotProjection(workspace.id);
    const item = { snapshot: detail.snapshot };
    const request = workspaceBoardActivationRequest(item);
    if (!request) {
      message.warning(workspaceBoardActivationHint(item));
      return;
    }
    message.info('工作空间执行已提交，正在后台处理');
    trackWorkspaceResumeInBackground(workspace.id, request);
  };

  return (
    <PageContainer
      title="业务总览"
      subTitle={projection ? projectionStatusSummary(projection) : '从工作空间视角查看目标、运行、治理阻塞和交付状态'}
      extra={<Button onClick={() => void loadBoard()} loading={loading}>刷新</Button>}
    >
      {boardItems.length === 0 && !loading ? (
        <Empty description="暂无工作空间" />
      ) : (
        <CardGrid className="workspace-board-grid" loading={loading}>
          {boardItems.map((item) => {
            const { workspace } = item;
            const hasActiveRun = Boolean(workspace.activeRunId);
            const canStartRun = !hasActiveRun;
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
                      <div>{workspace.activeRunId ? <Space>{shortId(workspace.activeRunId)}<Tag>{stateLabel(workspace.state)}</Tag></Space> : '-'}</div>
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
                  <Button type="primary" onClick={() => onOpenWorkspace(workspace.id)}>进入工作空间</Button>
                  {canStartRun ? <Button onClick={() => void resumeWorkspaceExecution(workspace).catch((error: unknown) => message.error(errorMessage(error)))}>提交执行</Button> : null}
                  {workspace.activeRunId ? <Button onClick={() => void workorchApi.resumeRun(workspace.activeRunId || '', undefined, workspace.id).then(() => loadBoard()).catch((error: unknown) => message.error(errorMessage(error)))}>恢复运行</Button> : null}
                </Space>
              </ProCard>
            );
          })}
        </CardGrid>
      )}
    </PageContainer>
  );
}
