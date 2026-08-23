import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Form, Input, Modal, Space, Typography } from 'antd';
import { CardGrid, PageContainer, ProCard, ProTable } from '../components/proCompat';
import type { ProColumns } from '../components/proCompat';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { useSSE } from '../hooks';
import { errorMessage, compactJSON, boundedParagraph, stateTag } from '../utils';
import { decisionInitialPayload, normalizeDecisionResolutionPayload, decisionPayloadFields, parseDecisionExtraPayload } from '../helpers';
import {
  governanceResourceKey,
  canonicalDefinitionClarification,
  isCanonicalDecisionMutationTarget,
  isDedicatedDefinitionDecision,
  mergeGovernanceResources,
  readGovernanceSnapshots,
  requireCanonicalDecisionMutationTargets
} from '../governanceMutationTarget';
import type { ControlPlaneProps, Decision, DecisionPage, WorkspaceSnapshot } from '../types';

const governancePageLimit = 20;

function isGovernanceRefreshEvent(event: { topic: string; payload?: Record<string, unknown> }) {
  return event.topic === '/workorchd/workspace/projection/changed'
    || event.payload?.type === 'workspace_projection_changed'
    || event.topic.includes('decision')
    || event.topic.includes('governance')
    || event.topic.includes('run.');
}

export function DecisionQueuePage({ onOpenWorkspace }: Pick<ControlPlaneProps, 'onOpenWorkspace'>) {
  const { message } = AntApp.useApp();
  const [selectedDecisionKeys, setSelectedDecisionKeys] = useState<React.Key[]>([]);
  const [decisionTarget, setDecisionTarget] = useState<{ item: Decision; resolutionType: string } | undefined>(undefined);
  const [decisionSubmitting, setDecisionSubmitting] = useState(false);
  const [decisionRecords, setDecisionRecords] = useState<Decision[]>([]);
  const [decisionCursorAfterId, setDecisionCursorAfterId] = useState<string | undefined>(undefined);
  const [decisionHasMore, setDecisionHasMore] = useState(false);
  const [decisionLoading, setDecisionLoading] = useState(false);
  const [decisionSnapshots, setDecisionSnapshots] = useState<Map<string, WorkspaceSnapshot>>(new Map());
  const [decisionForm] = Form.useForm();

  function applyDecisionPage(page: DecisionPage, append: boolean) {
    const items = (page.decisions ?? []).sort((left, right) => Number(left.state !== 'open') - Number(right.state !== 'open'));
    setDecisionRecords((current) => mergeGovernanceResources(current, items, append));
    setDecisionCursorAfterId(page.nextCursorAfterId || items.at(-1)?.id);
    setDecisionHasMore(Boolean(page.hasMore));
  }

  async function loadDecisionPage(cursorAfterId?: string) {
    setDecisionLoading(true);
    try {
      const page = await workorchApi.listDecisionPage({ cursorAfterId, limit: governancePageLimit });
      applyDecisionPage(page, Boolean(cursorAfterId));
      const snapshots = await readGovernanceSnapshots(page.decisions ?? [], workorchApi.readWorkspaceSnapshot);
      setDecisionSnapshots((current) => Boolean(cursorAfterId) ? new Map([...current, ...snapshots]) : snapshots);
    } finally {
      setDecisionLoading(false);
    }
  }

  useEffect(() => {
    void loadDecisionPage().catch((error: unknown) => message.error(errorMessage(error)));
  }, []);

  useSSE((event) => {
    if (isGovernanceRefreshEvent(event)) {
      void loadDecisionPage().catch((error: unknown) => message.error(errorMessage(error)));
    }
  }, {
    debounceMs: 300,
    onExhausted: () => {
      message.warning('治理事件流已断开，请检查服务状态或手动刷新。');
      void loadDecisionPage().catch((error: unknown) => message.error(errorMessage(error)));
    }
  });

  const resolve = async (item: Decision, resolutionType: string, payload?: Record<string, unknown>) => {
    try {
      await requireCanonicalDecisionMutationTargets([item], workorchApi.readWorkspaceSnapshot);
      const decision = await workorchApi.resolveDecision(item.workspaceId, item.id, resolutionType, payload);
      const recoveryAction = typeof decision.resolutionPayload?.recoveryAction === 'string'
        ? decision.resolutionPayload.recoveryAction
        : undefined;
      const normalizedReplan = resolutionType === 'replan'
        && decision.resolutionType === 'provide_missing_info'
        && recoveryAction === 'retry_same_step';
      message.success(normalizedReplan
        ? `当前工单不适用重新规划，已转为重试当前步骤；${modelLabels.run}已由服务端继续处理`
        : `决策已按 ${decision.resolutionType || resolutionType} 处理，${modelLabels.run}已由服务端转入后台继续`);
      void loadDecisionPage().catch((error: unknown) => message.error(errorMessage(error)));
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const openDecisionResolution = (item: Decision, resolutionType: string) => {
    decisionForm.resetFields();
    void requireCanonicalDecisionMutationTargets([item], workorchApi.readWorkspaceSnapshot)
      .then(() => workorchApi.readDecision(item.workspaceId, item.id))
      .then((detail) => {
        if (resolutionType === 'provide_missing_info') {
          decisionForm.setFieldsValue(decisionInitialPayload(detail));
        } else {
          decisionForm.setFieldsValue({ operator: 'console' });
        }
        setDecisionTarget({ item: detail, resolutionType });
      })
      .catch((error: unknown) => message.error(errorMessage(error)));
  };

  const submitDecisionResolution = async () => {
    if (!decisionTarget) {
      return;
    }
    setDecisionSubmitting(true);
    try {
      const values = await decisionForm.validateFields();
      const payload = normalizeDecisionResolutionPayload(values);
      const target = decisionTarget;
      await requireCanonicalDecisionMutationTargets([target.item], workorchApi.readWorkspaceSnapshot);
      await workorchApi.resolveDecision(target.item.workspaceId, target.item.id, target.resolutionType, payload);
      setDecisionTarget(undefined);
      decisionForm.resetFields();
      message.success(`决策已按 ${target.resolutionType} 处理，${modelLabels.run} 已由服务端转入后台继续`);
      void loadDecisionPage().catch((error: unknown) => message.error(errorMessage(error)));
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setDecisionSubmitting(false);
    }
  };

  const batchResolve = async (resolutionType: string) => {
    if (selectedDecisionKeys.length === 0) {
      message.warning('请先选择待处理决策工单');
      return;
    }
    try {
      const selectedKeys = new Set(selectedDecisionKeys.map(String));
      const selected = decisionRecords.filter((item) => selectedKeys.has(governanceResourceKey(item)));
      if (selected.length !== selectedKeys.size) {
        throw new Error('所选决策已不在当前列表中；未提交任何变更，请刷新后重试');
      }
      await requireCanonicalDecisionMutationTargets(selected, workorchApi.readWorkspaceSnapshot);
      const workspaceIds = [...new Set(selected.map((item) => item.workspaceId))];
      const ticketIds = selected.map((item) => item.id);
      const resolved = await workorchApi.batchResolveDecisions(workspaceIds.length === 1 ? workspaceIds[0] : undefined, ticketIds, resolutionType, { operator: 'console', mode: 'batch' });
      const retried = resolved.filter((item) => resolutionType === 'replan'
        && item.resolutionType === 'provide_missing_info'
        && item.resolutionPayload?.recoveryAction === 'retry_same_step').length;
      const replanned = resolved.length - retried;
      const summary = retried > 0
        ? `${replanned > 0 ? `${replanned} 个已重新规划，` : ''}${retried} 个 provider 恢复工单已转为重试当前步骤`
        : `${ticketIds.length} 个决策工单已按 ${resolutionType} 处理`;
      message.success(`${summary}，相关${modelLabels.run}已由服务端转入后台继续`);
      setSelectedDecisionKeys([]);
      void loadDecisionPage().catch((error: unknown) => message.error(errorMessage(error)));
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const decisionIsMutable = (item: Decision) => isCanonicalDecisionMutationTarget(decisionSnapshots.get(item.workspaceId), item);
  const decisionIsCanonicalDefinitionBlocker = (item: Decision) => canonicalDefinitionClarification(decisionSnapshots.get(item.workspaceId))?.id === item.id;

  const columns: ProColumns<Decision>[] = [
    { title: '工单 ID', dataIndex: 'id', copyable: true },
    { title: '工作空间', dataIndex: 'workspaceId', copyable: true },
    { title: '运行', dataIndex: 'runId', copyable: true },
    { title: '类型', dataIndex: 'decisionKind' },
    { title: '问题', dataIndex: 'question', ellipsis: true, render: (_, item) => boundedParagraph(item.question, { rows: 2 }) },
    { title: '原因', dataIndex: 'reason', ellipsis: true },
    {
      title: 'Definition blocker',
      render: (_, item) => item.definitionClarification
        ? <Space direction="vertical" size={0}>
            <Space>{stateTag(item.definitionClarification.class)}{stateTag(item.definitionClarification.bindingState)}</Space>
            <Typography.Text type="secondary">{item.definitionClarification.violationOrGapCode}</Typography.Text>
            {decisionIsCanonicalDefinitionBlocker(item) ? <Typography.Text type="warning">活动 Run 的 canonical blocker</Typography.Text> : null}
          </Space>
        : '-'
    },
    { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' },
    { title: '解决方式', dataIndex: 'resolutionType', width: 140, render: (_, item) => item.resolutionType ? stateTag(item.resolutionType) : '-' },
    { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => decisionIsMutable(item) ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => openDecisionResolution(item, 'provide_missing_info')}>补充信息</Button>
          <Button size="small" onClick={() => void resolve(item, 'replan', { operator: 'console' })}>
            {item.allowedRecoveryActions?.includes('replan') === false && item.allowedRecoveryActions.includes('provide_missing_info')
              ? '重新规划（将重试）'
              : '重新规划并继续'}
          </Button>
          <Button size="small" danger onClick={() => void resolve(item, 'reject_current_path', { operator: 'console' })}>拒绝路径并继续</Button>
        </Space>
      ) : item.state === 'open' && item.definitionClarification
        ? <Button size="small" type="primary" onClick={() => onOpenWorkspace(item.workspaceId)}>修订 Definition</Button>
        : item.state === 'open' && isDedicatedDefinitionDecision(item)
          ? <Typography.Text type="secondary">专用定义澄清流程（typed binding 未就绪）</Typography.Text>
        : item.state === 'open'
          ? <Typography.Text type="secondary">非活动运行阻塞项（只读）</Typography.Text>
          : stateTag(item.resolutionType)
    }
  ];

  return (
    <PageContainer
      title="决策工单"
      subTitle={`处理阻塞${modelLabels.run}的${modelLabels.decision}，支持补充信息、重新规划和拒绝当前路径`}
      extra={(
        <Space wrap>
          <Button disabled={selectedDecisionKeys.length === 0} onClick={() => void batchResolve('replan')}>批量重新规划</Button>
          <Button danger disabled={selectedDecisionKeys.length === 0} onClick={() => void batchResolve('reject_current_path')}>批量拒绝路径</Button>
        </Space>
      )}
    >
      <ProTable<Decision>
        rowKey={governanceResourceKey}
        search={false}
        options={false}
        rowSelection={{
          selectedRowKeys: selectedDecisionKeys,
          onChange: setSelectedDecisionKeys,
          getCheckboxProps: (record) => ({ disabled: !decisionIsMutable(record) })
        }}
        columns={columns}
        loading={decisionLoading}
        dataSource={decisionRecords}
        expandable={{
          expandedRowRender: (item) => (
            <CardGrid>
              {item.definitionClarification ? (
                <ProCard title="Definition clarification（owner typed projection）" colSpan={{ xs: 24, lg: 24 }}>
                  <Descriptions
                    size="small"
                    column={{ xs: 1, md: 2 }}
                    items={[
                      { key: 'class', label: 'Class', children: stateTag(item.definitionClarification.class) },
                      { key: 'bindingState', label: 'Binding state', children: stateTag(item.definitionClarification.bindingState) },
                      { key: 'requiredAction', label: 'Required action', children: item.definitionClarification.requiredAction },
                      { key: 'gap', label: 'Gap / violation', children: item.definitionClarification.violationOrGapCode },
                      { key: 'offendingRefs', label: 'Offending refs', children: item.definitionClarification.offendingRefs?.join(', ') || '-' },
                      { key: 'definitionRevision', label: 'Definition revision', children: item.definitionClarification.definitionRevision || '-' },
                      { key: 'authoritySnapshotId', label: 'Authority snapshot', children: item.definitionClarification.authoritySnapshotId || '-' },
                      { key: 'clarificationDispatchId', label: 'Clarification dispatch', children: item.definitionClarification.clarificationDispatchId || '-' },
                      { key: 'focusRef', label: 'Focus', children: item.definitionClarification.focusRef || '-' }
                    ]}
                  />
                </ProCard>
              ) : null}
              <ProCard title="决策上下文" colSpan={{ xs: 24, lg: 12 }}>
                <pre className="json-panel json-panel-large">{item.context ? compactJSON(item.context) : '列表投影未加载上下文，请进入处理弹窗查看完整详情'}</pre>
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
      <Space className="workorch-section">
        <Button
          size="small"
          onClick={() => void loadDecisionPage().catch((error: unknown) => message.error(errorMessage(error)))}
          loading={decisionLoading}
        >
          刷新决策
        </Button>
        <Button
          size="small"
          disabled={!decisionHasMore || !decisionCursorAfterId}
          onClick={() => void loadDecisionPage(decisionCursorAfterId).catch((error: unknown) => message.error(errorMessage(error)))}
          loading={decisionLoading}
        >
          加载更多
        </Button>
      </Space>
      <Modal
        title="补充决策信息"
        open={Boolean(decisionTarget)}
        confirmLoading={decisionSubmitting}
        onCancel={() => setDecisionTarget(undefined)}
        onOk={() => void submitDecisionResolution()}
        okText="提交并转入后台"
        destroyOnHidden
      >
        {decisionTarget ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions
              column={1}
              size="small"
              items={[
                { key: 'ticket', label: '决策工单', children: decisionTarget.item.id },
                { key: 'run', label: `关联${modelLabels.run}`, children: decisionTarget.item.runId },
                { key: 'question', label: '问题', children: decisionTarget.item.question || '-' },
                { key: 'reason', label: '原因', children: decisionTarget.item.reason || '-' }
              ]}
            />
            <Form form={decisionForm} layout="vertical">
              {decisionPayloadFields(decisionTarget.item).map((field) => (
                <Form.Item
                  key={field.name}
                  name={['structuredPayload', field.name]}
                  label={field.label}
                  rules={field.required ? [{ required: true, message: `请补充${field.label}` }] : undefined}
                >
                  <Input />
                </Form.Item>
              ))}
              {decisionPayloadFields(decisionTarget.item).length === 0 ? (
                <Form.Item name="answer" label="补充信息" rules={[{ required: true, message: '请填写补充信息' }]}>
                  <Input.TextArea rows={4} placeholder="按决策问题补充业务信息，例如目标对象、执行范围、路径选择或缺失参数。" />
                </Form.Item>
              ) : (
                <Form.Item name="answer" label="补充说明">
                  <Input.TextArea rows={3} placeholder="可选。用于补充字段之外的业务判断、执行约束或上下文说明。" />
                </Form.Item>
              )}
              <Form.Item name="note" label="处理备注">
                <Input.TextArea rows={2} placeholder="可选。记录本次处理依据，供后续审计和审批查看。" />
              </Form.Item>
              <Form.Item
                name="extraPayloadText"
                label="高级 JSON 载荷"
                tooltip="可选。填写 JSON 对象，会合并到 resolutionPayload，用于复杂结构化信息。"
                rules={[
                  {
                    validator: (_, value) => {
                      try {
                        parseDecisionExtraPayload(value);
                        return Promise.resolve();
                      } catch (error) {
                        return Promise.reject(error instanceof Error ? error : new Error('JSON 格式不正确'));
                      }
                    }
                  }
                ]}
              >
                <Input.TextArea rows={4} placeholder='例如：{"target":"业务对象","method":"执行方式","constraints":["约束条件"]}' />
              </Form.Item>
              <Form.Item name="operator" label="处理人" rules={[{ required: true, message: '请填写处理人' }]}>
                <Input />
              </Form.Item>
            </Form>
            <Typography.Text type="secondary">上下文</Typography.Text>
            <pre className="json-panel">{compactJSON(decisionTarget.item.context)}</pre>
          </Space>
        ) : null}
      </Modal>
    </PageContainer>
  );
}
