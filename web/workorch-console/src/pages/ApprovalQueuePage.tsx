import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Form, Input, Modal, Space, Typography } from 'antd';
import { PageContainer, ProTable } from '../components/proCompat';
import type { ProColumns } from '../components/proCompat';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { useSSE } from '../hooks';
import { errorMessage, compactJSON, stateTag } from '../utils';
import {
  governanceResourceKey,
  governanceContinuationGuidance,
  isCanonicalApprovalMutationTarget,
  mergeGovernanceResources,
  readGovernanceSnapshots,
  requireCanonicalApprovalMutationTargets
} from '../governanceMutationTarget';
import type { Approval, ApprovalPage, Decision, WorkspaceSnapshot } from '../types';

const governancePageLimit = 20;

function isGovernanceRefreshEvent(event: { topic: string; payload?: Record<string, unknown> }) {
  return event.topic === '/workorchd/workspace/projection/changed'
    || event.payload?.type === 'workspace_projection_changed'
    || event.topic.includes('approval')
    || event.topic.includes('governance')
    || event.topic.includes('run.');
}

export function ApprovalQueuePage() {
  const { message } = AntApp.useApp();
  const [selectedApprovalKeys, setSelectedApprovalKeys] = useState<React.Key[]>([]);
  const [approvalTarget, setApprovalTarget] = useState<{ item: Approval; action: 'approve' | 'reject'; relatedDecision?: Decision } | undefined>(undefined);
  const [approvalSubmitting, setApprovalSubmitting] = useState(false);
  const [approvalRecords, setApprovalRecords] = useState<Approval[]>([]);
  const [approvalCursorAfterId, setApprovalCursorAfterId] = useState<string | undefined>(undefined);
  const [approvalHasMore, setApprovalHasMore] = useState(false);
  const [approvalLoading, setApprovalLoading] = useState(false);
  const [approvalSnapshots, setApprovalSnapshots] = useState<Map<string, WorkspaceSnapshot>>(new Map());
  const [approvalForm] = Form.useForm<{ operator?: string; comment?: string }>();
  const actionLabel = (action: 'approve' | 'reject') => action === 'approve' ? '已批准' : '已拒绝';

  function applyApprovalPage(page: ApprovalPage, append: boolean) {
    const items = (page.approvals ?? []).sort((left, right) => Number(left.state !== 'pending') - Number(right.state !== 'pending'));
    setApprovalRecords((current) => mergeGovernanceResources(current, items, append));
    setApprovalCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setApprovalHasMore(Boolean(page.hasMore));
  }

  async function loadApprovalPage(cursorAfterId?: string) {
    setApprovalLoading(true);
    try {
      const page = await workorchApi.listApprovalPage({ cursorAfterId, limit: governancePageLimit });
      applyApprovalPage(page, Boolean(cursorAfterId));
      const snapshots = await readGovernanceSnapshots(page.approvals ?? [], workorchApi.readWorkspaceSnapshot);
      setApprovalSnapshots((current) => Boolean(cursorAfterId) ? new Map([...current, ...snapshots]) : snapshots);
    } finally {
      setApprovalLoading(false);
    }
  }

  useEffect(() => {
    void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)));
  }, []);

  useSSE((event) => {
    if (isGovernanceRefreshEvent(event)) {
      void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)));
    }
  }, {
    debounceMs: 300,
    onExhausted: () => {
      message.warning('治理事件流已断开，请检查服务状态或手动刷新。');
      void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)));
    }
  });

  const openApprovalResolution = async (item: Approval, action: 'approve' | 'reject') => {
    approvalForm.resetFields();
    approvalForm.setFieldsValue({ operator: 'console' });
    try {
      await requireCanonicalApprovalMutationTargets([item], workorchApi.readWorkspaceSnapshot);
      const [approvalDetail, decisions] = await Promise.all([
        workorchApi.readApproval(item.workspaceId, item.id),
        workorchApi.listDecisionPage({ workspaceId: item.workspaceId, runId: item.runId, limit: governancePageLimit })
      ]);
      const relatedDecision = decisions
        .decisions
        ?.filter((decision) => decision.runId === item.runId && decision.state === 'resolved')
        .at(-1);
      setApprovalTarget({ item: approvalDetail, action, relatedDecision });
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const submitApprovalResolution = async () => {
    if (!approvalTarget) {
      return;
    }
    setApprovalSubmitting(true);
    try {
      await approvalForm.validateFields();
      await requireCanonicalApprovalMutationTargets([approvalTarget.item], workorchApi.readWorkspaceSnapshot);
      if (approvalTarget.action === 'approve') {
        await workorchApi.approveApproval(approvalTarget.item.workspaceId, approvalTarget.item.id);
      } else {
        await workorchApi.rejectApproval(approvalTarget.item.workspaceId, approvalTarget.item.id);
      }
      const target = approvalTarget;
      setApprovalTarget(undefined);
      approvalForm.resetFields();
      message.success(`审批${actionLabel(target.action)}。${governanceContinuationGuidance}`);
      void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)));
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setApprovalSubmitting(false);
    }
  };

  const batchResolve = async (action: 'approve' | 'reject') => {
    if (selectedApprovalKeys.length === 0) {
      message.warning('请先选择待审批项');
      return;
    }
    try {
      const selectedKeys = new Set(selectedApprovalKeys.map(String));
      const selected = approvalRecords.filter((item) => selectedKeys.has(governanceResourceKey(item)));
      if (selected.length !== selectedKeys.size) {
        throw new Error('所选审批已不在当前列表中；未提交任何变更，请刷新后重试');
      }
      await requireCanonicalApprovalMutationTargets(selected, workorchApi.readWorkspaceSnapshot);
      const workspaceIds = [...new Set(selected.map((item) => item.workspaceId))];
      const approvalIds = selected.map((item) => item.id);
      await workorchApi.batchResolveApprovals(workspaceIds.length === 1 ? workspaceIds[0] : undefined, approvalIds, action);
      message.success(`${approvalIds.length} 个审批项${actionLabel(action)}。${governanceContinuationGuidance}`);
      setSelectedApprovalKeys([]);
      void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)));
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const approvalIsMutable = (item: Approval) => isCanonicalApprovalMutationTarget(approvalSnapshots.get(item.workspaceId), item);

  const columns: ProColumns<Approval>[] = [
    { title: '审批 ID', dataIndex: 'id', copyable: true },
    { title: '工作空间', dataIndex: 'workspaceId', copyable: true },
    { title: '运行', dataIndex: 'runId', copyable: true },
    { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
    { title: '原因', dataIndex: 'reason', ellipsis: true },
    { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' },
    { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => approvalIsMutable(item) ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => void openApprovalResolution(item, 'approve')}>审批确认</Button>
          <Button size="small" danger onClick={() => void openApprovalResolution(item, 'reject')}>拒绝确认</Button>
        </Space>
      ) : item.state === 'pending'
        ? <Typography.Text type="secondary">非活动运行阻塞项（只读）</Typography.Text>
        : stateTag(item.state)
    }
  ];

  return (
    <PageContainer
      title="审批队列"
      subTitle={`处理${modelLabels.approval}事实；${governanceContinuationGuidance}`}
      extra={(
        <Space wrap>
          <Button disabled={selectedApprovalKeys.length === 0} onClick={() => void batchResolve('approve')}>批量批准</Button>
          <Button danger disabled={selectedApprovalKeys.length === 0} onClick={() => void batchResolve('reject')}>批量拒绝</Button>
        </Space>
      )}
    >
      <ProTable<Approval>
        rowKey={governanceResourceKey}
        search={false}
        options={false}
        rowSelection={{
          selectedRowKeys: selectedApprovalKeys,
          onChange: setSelectedApprovalKeys,
          getCheckboxProps: (record) => ({ disabled: !approvalIsMutable(record) })
        }}
        columns={columns}
        loading={approvalLoading}
        dataSource={approvalRecords}
      />
      <Space className="workorch-section">
        <Button
          size="small"
          onClick={() => void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)))}
          loading={approvalLoading}
        >
          刷新审批
        </Button>
        <Button
          size="small"
          disabled={!approvalHasMore || !approvalCursorAfterId}
          onClick={() => void loadApprovalPage(approvalCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
          loading={approvalLoading}
        >
          加载更多
        </Button>
      </Space>
      <Modal
        title={approvalTarget?.action === 'reject' ? '拒绝审批确认' : '审批确认'}
        open={Boolean(approvalTarget)}
        confirmLoading={approvalSubmitting}
        onCancel={() => setApprovalTarget(undefined)}
        onOk={() => void submitApprovalResolution()}
        okText={approvalTarget?.action === 'reject' ? '拒绝审批' : '批准审批'}
        okButtonProps={{ danger: approvalTarget?.action === 'reject' }}
        destroyOnHidden
      >
        {approvalTarget ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions
              column={1}
              size="small"
              items={[
                { key: 'approval', label: '审批 ID', children: approvalTarget.item.id },
                { key: 'run', label: `关联${modelLabels.run}`, children: approvalTarget.item.runId },
                { key: 'step', label: '步骤', children: approvalTarget.item.stepId || '-' },
                { key: 'reason', label: '审批原因', children: approvalTarget.item.reason || '-' }
              ]}
            />
            <Typography.Text type="secondary">已补充的决策信息</Typography.Text>
            <pre className="json-panel">{compactJSON(approvalTarget.relatedDecision?.resolutionPayload)}</pre>
            <Typography.Text type="secondary">自动治理摘要</Typography.Text>
            <pre className="json-panel">{compactJSON(approvalTarget.item.autoGovernance)}</pre>
            <Form form={approvalForm} layout="vertical">
              <Form.Item name="operator" label="审批人" rules={[{ required: true, message: '请填写审批人' }]}>
                <Input />
              </Form.Item>
              <Form.Item name="comment" label="审批意见">
                <Input.TextArea rows={3} placeholder="记录本次审批判断依据，当前版本用于界面确认，不写入审批模型" />
              </Form.Item>
            </Form>
          </Space>
        ) : null}
      </Modal>
    </PageContainer>
  );
}
