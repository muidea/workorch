import { useEffect, useState } from 'react';
import { Alert, App as AntApp, Button, Form, Input, List, Modal, Select, Space, Tag, Typography } from 'antd';
import { workorchApi } from '../../api';
import { errorMessage } from '../../helpers';
import { authoringCanConfirm, authoringNeedsPolling, authoringRecoveryKey, parseAuthoringRecovery } from '../../workspaceAuthoring';
import type { AuthoringRecovery } from '../../workspaceAuthoring';
import type { DefinitionAuthoringView, WorkspaceTemplate } from '../../types';

type Props = { open: boolean; onClose: () => void; onCreated: (workspaceId: string) => void | Promise<void> };
type Values = { title: string; templateId: string; deliverableDirectory?: string; goalText: string; requirements?: string; resources?: string };
const lines = (value?: string) => (value || '').split('\n').map(line => line.trim()).filter(Boolean);
const sourceLabels: Record<string, string> = { USER_EXPLICIT: '用户要求', TEMPLATE_REQUIRED: '模板必需', DERIVED: '派生' };

export function WorkspaceSetupModal({ open, onClose, onCreated }: Props) {
  const { message } = AntApp.useApp();
  const [form] = Form.useForm<Values>();
  const [templates, setTemplates] = useState<WorkspaceTemplate[]>([]);
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [recovery, setRecovery] = useState<AuthoringRecovery>();
  const [busy, setBusy] = useState(false);
  const [edited, setEdited] = useState(false);
  const [failure, setFailure] = useState('');

  const persist = (value: AuthoringRecovery) => {
    localStorage.setItem(authoringRecoveryKey, JSON.stringify(value));
    setRecovery(value);
  };
  const applyView = (next: DefinitionAuthoringView) => {
    setView(next);
    setEdited(false);
    if (next.input) form.setFieldsValue({ templateId: next.templateId, goalText: next.input.goalText, requirements: next.input.acceptanceRequirements?.join('\n'), resources: next.input.resourceRefs?.join('\n') });
  };

  useEffect(() => {
    if (!open) return;
    let disposed = false;
    setFailure('');
    workorchApi.listWorkspaceTemplates().then(items => { if (!disposed) setTemplates(items); }).catch(error => { if (!disposed) setFailure(errorMessage(error)); });
    const saved = parseAuthoringRecovery(localStorage.getItem(authoringRecoveryKey));
    setRecovery(saved);
    setView(undefined);
    setEdited(false);
    if (saved) {
      form.setFieldsValue({ title: saved.request.title, templateId: saved.request.templateId, deliverableDirectory: saved.request.deliverableDirectory, goalText: saved.request.input.goalText, requirements: saved.request.input.acceptanceRequirements?.join('\n'), resources: saved.request.input.resourceRefs?.join('\n') });
      if (saved.workspaceId) workorchApi.readDefinitionAuthoring(saved.workspaceId).then(next => { if (!disposed) applyView(next); }).catch(error => { if (!disposed) setFailure(errorMessage(error)); });
    } else form.resetFields();
    return () => { disposed = true; };
  }, [open]);

  useEffect(() => {
    if (!open || !view || !authoringNeedsPolling(view)) return;
    let disposed = false;
    const timer = window.setInterval(() => {
      workorchApi.readDefinitionAuthoring(view.workspaceId).then(next => {
        if (disposed) return;
        // 任务接受回执不是持久状态；首个 reserve 出现前继续查询。
        if (view.compilationAccepted && next.state === 'draft') next.compilationAccepted = true;
        setView(next);
      }).catch(error => { if (!disposed) setFailure(errorMessage(error)); });
    }, 1500);
    return () => { disposed = true; window.clearInterval(timer); };
  }, [open, view?.workspaceId, view?.state, view?.compilationAccepted]);

  const generate = async () => {
    setBusy(true);
    setFailure('');
    try {
      const values = await form.validateFields();
      const input = { goalText: values.goalText, acceptanceRequirements: lines(values.requirements), resourceRefs: lines(values.resources) };
      let current = view;
      if (!current) {
        // 发送前保存原创建 payload；响应丢失后必须以原身份重放。
        const saved = recovery || { request: { requestId: crypto.randomUUID(), title: values.title, templateId: values.templateId, deliverableDirectory: values.deliverableDirectory, input } };
        persist(saved);
        current = saved.workspaceId ? await workorchApi.readDefinitionAuthoring(saved.workspaceId) : await workorchApi.createDefinitionAuthoring(saved.request);
        persist({ ...saved, workspaceId: current.workspaceId });
        applyView(current);
      }
      if (edited || current.input?.goalText !== input.goalText || current.templateId !== values.templateId || JSON.stringify(current.input?.acceptanceRequirements || []) !== JSON.stringify(input.acceptanceRequirements) || JSON.stringify(current.input?.resourceRefs || []) !== JSON.stringify(input.resourceRefs)) {
        current = await workorchApi.saveDefinitionAuthoring(current.workspaceId, input, values.templateId, current.workspaceRevision, current.inputHash || '');
        applyView(current);
        if (recovery) persist({ request: recovery.request, workspaceId: current.workspaceId });
      }
      const next = await workorchApi.compileDefinitionAuthoring(current.workspaceId);
      setView(next);
    } catch (error) {
      setFailure(errorMessage(error));
    } finally {
      setBusy(false);
    }
  };

  const confirm = async () => {
    if (!view || edited || (!authoringCanConfirm(view) && !recovery?.pendingConfirmation && !['confirming', 'admitting'].includes(view.state))) return;
    setBusy(true);
    setFailure('');
    try {
      if (recovery && !recovery.pendingConfirmation) persist({ ...recovery, pendingConfirmation: { inputHash: view.inputHash || '', derivationPermitId: view.derivationPermitId || '', proposalHash: view.proposal?.proposalHash || '', specificationHash: view.proposal?.specificationHash || '', expectedWorkspaceRevision: view.workspaceRevision } });
      const next = await workorchApi.confirmDefinitionAuthoring(view, recovery?.pendingConfirmation);
      setView(next);
      if (next.state === 'admitted') {
        localStorage.removeItem(authoringRecoveryKey);
        setRecovery(undefined);
        message.success('规格已确认并准入；运行需另行显式启动');
        await onCreated(next.workspaceId);
        onClose();
      }
    } catch (error) {
      setFailure(errorMessage(error));
    } finally { setBusy(false); }
  };

  return <Modal open={open} title="新建工作空间 · 规格创作" width={850} onCancel={onClose} footer={<Space>
    <Button onClick={onClose}>关闭（已保存草稿可恢复）</Button>
    {view?.state === 'admitted' && <Button type="primary" onClick={async () => { await onCreated(view.workspaceId); localStorage.removeItem(authoringRecoveryKey); onClose(); }}>进入已准入工作空间</Button>}
    <Button loading={busy} onClick={generate} disabled={busy || ['confirming', 'admitting', 'admitted'].includes(view?.state || '')}>保存并生成／恢复审查</Button>
    <Button type="primary" loading={busy} onClick={confirm} disabled={busy || edited || (!authoringCanConfirm(view) && !recovery?.pendingConfirmation && !['confirming', 'admitting'].includes(view?.state || ''))}>确认规格并准入</Button>
  </Space>}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="info" showIcon message="输入目标即可，验收清单由模板与目标派生。六类审查通过后才可确认；此操作不会启动 Run。" />
      {failure && <Alert type="error" showIcon message={failure} description={recovery ? '创建身份已保留。重试将恢复原草稿，不会重新创建工作空间。' : undefined} />}
      {recovery && !view && <Alert type="warning" message="存在未完成的创建请求。将先以原 payload 恢复，再保存当前修改。" />}
      <Form form={form} layout="vertical" onValuesChange={() => setEdited(true)} disabled={busy || ['confirming', 'admitting', 'admitted'].includes(view?.state || '')}>
        <Form.Item name="title" label="名称" rules={[{ required: true }]}><Input disabled={Boolean(recovery)} /></Form.Item>
        <Form.Item name="templateId" label="模板" rules={[{ required: true }]}><Select options={templates.map(item => ({ value: item.id, label: item.name || item.id }))} /></Form.Item>
        <Form.Item name="deliverableDirectory" label="工作目录（可留空，自动解析）"><Input disabled={Boolean(recovery)} /></Form.Item>
        <Form.Item name="goalText" label="目标" rules={[{ required: true }]}><Input.TextArea rows={4} /></Form.Item>
        <Form.Item name="resources" label="本地资源引用（每行一个，最多 8 个，相对于工作目录）"><Input.TextArea rows={2} /></Form.Item>
        <Form.Item name="requirements" label="额外验收要求（可选，每行一条）"><Input.TextArea rows={3} /></Form.Item>
      </Form>
      {view && <>
        <Typography.Text>草稿：{view.workspaceId} · revision {view.inputRevision} · {view.state}</Typography.Text>
        {edited && <Alert type="warning" message="输入已修改，旧规格不可确认。请保存并重新审查。" />}
        {!view.templateCurrent && <Alert type="warning" message="模板已变化或不可用，请保存新的创作 revision。" />}
        {view.state === 'outcome_unresolved' && <Alert type="warning" message="已有持久请求结果未决，不会自动重发 provider。可查询恢复结果或修改输入形成新 revision。" />}
        {view.admissionViolationCode && <Alert type="warning" message={`准入需澄清：${view.admissionViolationCode}`} description={view.admissionOffendingRefs?.join('、')} />}
        {view.proposal && <>
          <Typography.Title level={5}>待确认验收清单</Typography.Title>
          <List dataSource={view.proposal.items} renderItem={item => <List.Item><Space align="start"><Tag>{sourceLabels[item.sourceKind] || item.sourceKind}</Tag><Typography.Text>{item.text}</Typography.Text></Space></List.Item>} />
          <Typography.Title level={5}>六类独立审查</Typography.Title>
          <List dataSource={view.proposal.reviews || []} renderItem={review => <List.Item><Space><Tag color={review.status === 'PASS' ? 'green' : 'orange'}>{review.status}</Tag><Typography.Text>{review.kind}：{review.reason || '无附加说明'}</Typography.Text></Space></List.Item>} />
          <Typography.Title level={5}>验证声明</Typography.Title>
          <List dataSource={view.proposal.verifications || []} renderItem={item => <List.Item><Typography.Text>{item.id} · {item.status} · {item.declaration}</Typography.Text></List.Item>} />
        </>}
      </>}
    </Space>
  </Modal>;
}
