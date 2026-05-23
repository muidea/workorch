import { useCallback, useEffect, useState } from 'react';
import { App as AntApp, Badge, Button, Descriptions, Empty, Space, Steps, Typography } from 'antd';
import { PageContainer, ProCard } from '@ant-design/pro-components';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { useSSE } from '../hooks';
import { errorMessage, shortId, stateTag, stateLabel, boundedParagraph, metricGrid } from '../utils';
import { workspaceBoardActivationRequest, workspaceBoardActivationHint, workspaceProjectionRun, runStateSummary, pendingCount, isTerminalRunState, workspaceProcessItems, runtimeSummaryTags } from '../helpers';
import type { ControlPlaneProps, WorkspaceBoardItem, WorkspaceExecutionActivationRequest } from '../types';

export function WorkspaceBoardPage({ onOpenWorkspace }: ControlPlaneProps) {
  const { message } = AntApp.useApp();
  const [boardItems, setBoardItems] = useState<WorkspaceBoardItem[]>([]);
  const [loading, setLoading] = useState(false);

  const loadBoard = useCallback(async (showLoading = true) => {
    if (showLoading) {
      setLoading(true);
    }
    try {
      const workspaces = await workorchApi.listWorkspaces();
      const nextBoardItems = await Promise.all(workspaces.map(async (item) => {
        const [snapshot, goals, dods] = await Promise.all([
          workorchApi.readWorkspaceSnapshot(item.id),
          workorchApi.listGoals(item.id),
          workorchApi.listDoDs(item.id)
        ]);
        return { snapshot, goals, dods };
      }));
      setBoardItems(nextBoardItems);
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

  useSSE(() => {
    void loadBoard(false);
  }, {
    debounceMs: 250,
    onReconnect: () => void loadBoard(false)
  });

  const trackWorkspaceActivationInBackground = (workspaceId: string, request: WorkspaceExecutionActivationRequest) => {
    void workorchApi.activateWorkspaceExecution(workspaceId, request)
      .then(() => {
        message.success('工作空间执行已在后台启动');
        void loadBoard();
      })
      .catch((error: unknown) => {
        message.error(`工作空间执行后台启动失败：${errorMessage(error)}`);
        void loadBoard();
      });
  };

  const activateWorkspaceExecution = async (item: WorkspaceBoardItem) => {
    const request = workspaceBoardActivationRequest(item);
    if (!request) {
      message.warning(workspaceBoardActivationHint(item));
      return;
    }
    message.info('工作空间执行已提交，正在后台启动');
    trackWorkspaceActivationInBackground(item.snapshot.workspace.id, request);
  };

  return (
    <PageContainer
      title="业务总览"
      subTitle="从工作空间视角查看目标、运行、治理阻塞和交付状态"
      extra={<Button onClick={() => void loadBoard()} loading={loading}>刷新</Button>}
    >
      {boardItems.length === 0 && !loading ? (
        <Empty description="暂无工作空间" />
      ) : (
        <ProCard className="workspace-board-grid" gutter={16} wrap loading={loading}>
          {boardItems.map((item) => {
            const { snapshot, goals, dods } = item;
            const activeRun = snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId);
            const currentRun = workspaceProjectionRun(snapshot);
            const runSummary = runStateSummary(snapshot.runs);
            const pendingDecisions = pendingCount(snapshot.decisions);
            const pendingApprovals = pendingCount(snapshot.approvals);
            const canStartRun = !currentRun || isTerminalRunState(currentRun.state);
            return (
              <ProCard
                key={snapshot.workspace.id}
                title={snapshot.workspace.title || snapshot.workspace.id}
                subTitle={snapshot.workspace.id}
                colSpan={{ xs: 24, md: 12, xl: 8 }}
                bordered
                extra={stateTag(snapshot.outcome?.state ?? snapshot.workspace.state)}
              >
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                  <div className="workorch-card-summary">
                    <div>
                      <Typography.Text type="secondary">当前运行（Run）</Typography.Text>
                      <div>{currentRun ? <Space>{shortId(currentRun.id)}{stateTag(currentRun.state)}</Space> : '-'}</div>
                    </div>
                    <div>
                      <Typography.Text type="secondary">治理阻塞</Typography.Text>
                      <div><Badge count={pendingDecisions + pendingApprovals} showZero color={pendingDecisions + pendingApprovals > 0 ? '#faad14' : '#52c41a'} /></div>
                    </div>
                  </div>
                  {metricGrid([
                    { key: 'goal', label: modelLabels.goal, value: goals.length },
                    { key: 'dod', label: modelLabels.dod, value: dods.length },
                    { key: 'run', label: modelLabels.run, value: runSummary.total },
                    { key: 'completed', label: '完成', value: runSummary.completed, color: '#389e0d' },
                    { key: 'failed', label: '失败', value: runSummary.failed, color: '#cf1322' }
                  ])}
                  <Descriptions
                    column={1}
                    size="small"
                    items={[
                      { key: 'deliverableDirectory', label: '工作目录', children: boundedParagraph(snapshot.workspace.deliverableDirectory, { rows: 1, copyText: snapshot.workspace.deliverableDirectory }) },
                      { key: 'activeRun', label: '活跃运行', children: activeRun ? `${activeRun.id} / ${stateLabel(activeRun.state)}` : '-' },
                      { key: 'outcome', label: '业务结果', children: snapshot.outcome?.runId ? `${snapshot.outcome.runId} / ${stateLabel(snapshot.outcome.state)}` : stateLabel(snapshot.outcome?.state ?? snapshot.workspace.state) },
                      { key: 'runtimeSummary', label: '运行摘要', children: runtimeSummaryTags(snapshot.runs) }
                    ]}
                  />
                </Space>
                <Steps
                  size="small"
                  className="workorch-section"
                  items={workspaceProcessItems(snapshot, goals, dods)}
                />
                <Space className="workorch-section" wrap>
                  <Button type="primary" onClick={() => onOpenWorkspace(snapshot.workspace.id)}>进入工作空间</Button>
                  {canStartRun ? <Button onClick={() => void activateWorkspaceExecution(item)}>{currentRun ? '重新启动执行' : '启动执行'}</Button> : null}
                  {currentRun && !isTerminalRunState(currentRun.state) ? <Button onClick={() => void workorchApi.resumeRun(currentRun.id).then(() => loadBoard()).catch((error: unknown) => message.error(errorMessage(error)))}>继续运行</Button> : null}
                </Space>
              </ProCard>
            );
          })}
        </ProCard>
      )}
    </PageContainer>
  );
}
