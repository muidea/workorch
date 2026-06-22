import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Form, Input, Modal, Space, Typography } from 'antd';
import { PageContainer, ProTable } from '../components/proCompat';
import type { ProColumns } from '../components/proCompat';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { useSSE } from '../hooks';
import { errorMessage, mergeById, compactJSON, stateTag } from '../utils';
import type { Approval, ApprovalPage, Decision } from '../types';

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
  const [approvalForm] = Form.useForm<{ operator?: string; comment?: string }>();
  const actionLabel = (action: 'approve' | 'reject') => action === 'approve' ? '已批准' : '已拒绝';

  function applyApprovalPage(page: ApprovalPage, append: boolean) {
    const items = (page.approvals ?? []).sort((left, right) => Number(left.state !== 'pending') - Number(right.state !== 'pending'));
    setApprovalRecords((current) => mergeById(current, items, append));
    setApprovalCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setApprovalHasMore(Boolean(page.hasMore));
  }

  async function loadApprovalPage(cursorAfterId?: string) {
    setApprovalLoading(true);
    try {
      const page = await workorchApi.listApprovalPage({ cursorAfterId, limit: governancePageLimit });
      applyApprovalPage(page, Boolean(cursorAfterId));
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
      if (approvalTarget.action === 'approve') {
        await workorchApi.approveApproval(approvalTarget.item.workspaceId, approvalTarget.item.id);
      } else {
        await workorchApi.rejectApproval(approvalTarget.item.workspaceId, approvalTarget.item.id);
      }
      const target = approvalTarget;
      setApprovalTarget(undefined);
      approvalForm.resetFields();
      message.success(`审批${actionLabel(target.action)}，${modelLabels.run} 已由服务端转入后台继续`);
      void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)));
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setApprovalSubmitting(false);
    }
  };

  const batchResolve = async (action: 'approve' | 'reject') => {
    const approvalIds = selectedApprovalKeys.map(String);
    if (approvalIds.length === 0) {
      message.warning('请先选择待审批项');
      return;
    }
    try {
      const selected = approvalRecords.filter((item) => approvalIds.includes(item.id));
      const byWorkspace = new Map<string, string[]>();
      for (const item of selected) {
        const workspaceId = item.workspaceId || '';
        const items = byWorkspace.get(workspaceId) ?? [];
        items.push(item.id);
        byWorkspace.set(workspaceId, items);
      }
      for (const [workspaceId, ids] of byWorkspace) {
        await workorchApi.batchResolveApprovals(workspaceId, ids, action);
      }
      message.success(`${approvalIds.length} 个审批项${actionLabel(action)}，相关${modelLabels.run}已由服务端转入后台继续`);
      setSelectedApprovalKeys([]);
      void loadApprovalPage().catch((error: unknown) => message.error(errorMessage(error)));
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

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
      render: (_, item) => item.state === 'pending' ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => void openApprovalResolution(item, 'approve')}>审批确认</Button>
          <Button size="small" danger onClick={() => void openApprovalResolution(item, 'reject')}>拒绝确认</Button>
        </Space>
      ) : stateTag(item.state)
    }
  ];

  return (
    <PageContainer
      title="审批队列"
      subTitle={`处理阻塞${modelLabels.run}的${modelLabels.approval}，批准后运行可继续推进，拒绝后运行失败收口`}
      extra={(
        <Space wrap>
          <Button disabled={selectedApprovalKeys.length === 0} onClick={() => void batchResolve('approve')}>批量批准</Button>
          <Button danger disabled={selectedApprovalKeys.length === 0} onClick={() => void batchResolve('reject')}>批量拒绝</Button>
        </Space>
      )}
    >
      <ProTable<Approval>
        rowKey="id"
        search={false}
        options={false}
        rowSelection={{
          selectedRowKeys: selectedApprovalKeys,
          onChange: setSelectedApprovalKeys,
          getCheckboxProps: (record) => ({ disabled: record.state !== 'pending' })
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
        okText={approvalTarget?.action === 'reject' ? '拒绝并转入后台' : '批准并转入后台'}
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
