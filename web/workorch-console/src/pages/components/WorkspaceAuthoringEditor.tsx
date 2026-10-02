import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Form, Input, List, Modal, Select, Space, Tag, Typography } from 'antd';
import { workorchApi } from '../../api';
import { errorMessage } from '../../helpers';
import { authoringSourceLabel, AuthoringQueryFence, mergeAuthoringObservation, AuthoringSessionFence, authoringCanEnterAdmitted, authoringCanConfirm, authoringCanReplayConfirmation, authoringEditorValues, authoringRequireNoPendingConfirmation, authoringNeedsSave, authoringNeedsPolling, restoreAuthoringEditorRecovery } from '../../workspaceAuthoring';
import type { AuthoringEditorRecovery, AuthoringEditorValues } from '../../workspaceAuthoring';
import type { DefinitionAuthoringView, WorkspaceTemplate } from '../../types';

type Props = { open: boolean; workspaceId?: string; onClose: () => void; onAdmitted: () => Promise<void> };
type Values = AuthoringEditorValues;
type Recovery = AuthoringEditorRecovery;
const lines = (value?: string) => (value || '').split('\n').map(item => item.trim()).filter(Boolean);

// 编辑已有 owner 草稿，不生成新创建身份，也不提供正式 Goal/DoD CRUD。
export function WorkspaceAuthoringEditor({ open, workspaceId, onClose, onAdmitted }: Props) {
  const [form] = Form.useForm<Values>();
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [templates, setTemplates] = useState<WorkspaceTemplate[]>([]);
  const [busy, setBusy] = useState(false);
  const [edited, setEdited] = useState(false);
  const [failure, setFailure] = useState('');
  const recovery = useRef<Recovery>({});
  const recoveryBlocked = useRef(false);
  const editVersion = useRef(0);
  const session = useRef(new AuthoringSessionFence());
  const queries = useRef(new AuthoringQueryFence());
  const observe = (next: DefinitionAuthoringView) => setView(current => mergeAuthoringObservation(current, next));
  const close = () => { session.current.begin(); queries.current.invalidate(); onClose(); };
  const key = `workorch.workspace-authoring.editor.v1/${workspaceId}`;
  const persist = (next: Recovery) => {
    if (recoveryBlocked.current) throw new Error('本地恢复记录不可用，禁止覆盖原记录');
    const scoped = { ...next, workspaceId };
    localStorage.setItem(key, JSON.stringify(scoped));
    recovery.current = scoped;
  };
  const apply = (next: DefinitionAuthoringView) => {
    observe(next);
    if (next.input) form.setFieldsValue({ templateId: next.templateId, goalText: next.input.goalText, requirements: next.input.acceptanceRequirements?.join('\n'), resources: next.input.resourceRefs?.join('\n') });
    setEdited(false);
  };

  useEffect(() => {
    if (!open || !workspaceId) return;
    const token = session.current.begin();
    queries.current.invalidate();
    setBusy(false);
    let disposed = false;
    const version = editVersion.current;
    setView(undefined);
    setFailure('');
    setEdited(false);
    form.resetFields();
    recoveryBlocked.current = false;
    try {
      const text = localStorage.getItem(key);
      const validated = restoreAuthoringEditorRecovery(text, workspaceId);
      recovery.current = validated || { workspaceId };
      const values = authoringEditorValues(recovery.current);
      if (values) { form.setFieldsValue(values); setEdited(true); }
    } catch (error) { recoveryBlocked.current = true; recovery.current = {}; setFailure(errorMessage(error)); }
    workorchApi.listWorkspaceTemplates().then(next => { if (!disposed && session.current.matches(token)) setTemplates(next); }).catch(error => { if (!disposed && session.current.matches(token)) setFailure(errorMessage(error)); });
    const query = queries.current.begin();
    queries.current.read(() => workorchApi.readDefinitionAuthoring(workspaceId), () => !disposed && session.current.matches(token) && queries.current.matches(query)).then(next => {
      if (disposed || !session.current.matches(token) || !queries.current.matches(query)) return;
      if (!next.input) { setFailure('该工作空间没有持久创作草稿；不能通过旧 CRUD 补写正式定义'); return; }
      if (authoringEditorValues(recovery.current) || version !== editVersion.current) observe(next);
      else apply(next);
    }).catch(error => { if (!disposed && session.current.matches(token) && queries.current.matches(query)) setFailure(errorMessage(error)); });
    return () => { disposed = true; session.current.begin(); };
  }, [open, workspaceId]);

  useEffect(() => {
    if (!open || busy || !view || !authoringNeedsPolling(view)) return;
    const token = session.current.current();
    let disposed = false;
    let timer: number;
    const poll = async () => {
      const query = queries.current.begin();
      try {
        const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(view.workspaceId), () => !disposed && session.current.matches(token) && queries.current.matches(query));
        if (!disposed && session.current.matches(token) && queries.current.matches(query)) observe(next);
      } catch (error) {
        if (!disposed && session.current.matches(token) && queries.current.matches(query)) setFailure(errorMessage(error));
      } finally {
        if (!disposed && session.current.matches(token)) timer = window.setTimeout(poll, 1500);
      }
    };
    timer = window.setTimeout(poll, 1500);
    return () => { disposed = true; window.clearTimeout(timer); };
  }, [open, busy, view?.workspaceId, view?.state, view?.compilationAccepted]);

  const generate = async () => {
    const token = session.current.current();
    if (recoveryBlocked.current || !view) return;
    queries.current.invalidate();
    setBusy(true); setFailure('');
    try {
      authoringRequireNoPendingConfirmation(recovery.current);
      const values = await form.validateFields();
      if (!session.current.matches(token)) return;
      let current = view;
      const pending = recovery.current.pendingSave;
      if (pending) {
        current = await workorchApi.saveDefinitionAuthoring(current.workspaceId, pending.input, pending.templateId, pending.expectedWorkspaceRevision, pending.expectedInputHash, pending.requestId);
        if (!session.current.matches(token)) return;
        persist({ values });
        observe(current);
      }
      const input = { goalText: values.goalText, acceptanceRequirements: lines(values.requirements), resourceRefs: lines(values.resources) };
      if (authoringNeedsSave(current, input, values.templateId)) {
        const pendingSave = { requestId: crypto.randomUUID(), input, templateId: values.templateId, expectedWorkspaceRevision: current.workspaceRevision, expectedInputHash: current.inputHash || '' };
        persist({ values, pendingSave });
        current = await workorchApi.saveDefinitionAuthoring(current.workspaceId, input, values.templateId, pendingSave.expectedWorkspaceRevision, pendingSave.expectedInputHash, pendingSave.requestId);
        if (!session.current.matches(token)) return;
      }
      persist({}); apply(current);
      const next = await workorchApi.compileDefinitionAuthoring(current.workspaceId);
      if (!session.current.matches(token)) return;
      observe(next);
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };
  const refreshSaveBaseline = () => {
    if (recoveryBlocked.current || !view || !recovery.current.pendingSave) return;
    const token = session.current.current();
    Modal.confirm({
      title: '读取当前草稿并放弃原保存请求的重放身份？',
      content: '本地输入保留。此操作不会保存或覆盖 owner；请核对当前目标后，再显式提交修订。若原请求已成功，当前草稿会体现其结果。',
      okText: '读取当前草稿',
      cancelText: '保留原请求',
      onOk: async () => {
        if (recoveryBlocked.current || !session.current.matches(token)) return;
        queries.current.invalidate();
        setBusy(true); setFailure('');
        try {
          const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(view.workspaceId), () => session.current.matches(token));
          if (!session.current.matches(token)) return;
          const values = form.getFieldsValue();
          persist({ values });
          observe(next);
          setEdited(true);
        } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
        finally { if (session.current.matches(token)) setBusy(false); }
      }
    });
  };
  const exact = recovery.current.pendingConfirmation;
  const replayAllowed = authoringCanReplayConfirmation(view, recovery.current);
  const canConfirm = !recovery.current.pendingSave && Boolean(exact ? replayAllowed : !edited && authoringCanConfirm(view));
  const confirm = async () => {
    const token = session.current.current();
    if (recoveryBlocked.current || !view || !canConfirm) return;
    queries.current.invalidate();
    setBusy(true); setFailure('');
    try {
      const pendingConfirmation = exact || { inputHash: view.inputHash || '', derivationPermitId: view.derivationPermitId || '', proposalHash: view.proposal?.proposalHash || '', specificationHash: view.proposal?.specificationHash || '', expectedWorkspaceRevision: view.workspaceRevision };
      persist({ pendingConfirmation });
      const next = await workorchApi.confirmDefinitionAuthoring(view, pendingConfirmation);
      if (!session.current.matches(token)) return;
      observe(next);
      if (authoringCanEnterAdmitted(next, view.workspaceId)) {
        await onAdmitted();
        if (!session.current.matches(token)) return;
        localStorage.removeItem(key); recovery.current = {}; close();
      }
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };
  const enterAdmitted = async () => {
    if (recoveryBlocked.current || !authoringCanEnterAdmitted(view, workspaceId)) return;
    const token = session.current.current();
    queries.current.invalidate();
    setBusy(true); setFailure('');
    try {
      await onAdmitted();
      if (!session.current.matches(token)) return;
      localStorage.removeItem(key); recovery.current = {}; close();
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };
  const sealed = ['confirming', 'admitting', 'admitted'].includes(view?.state || '');
  return <Modal open={open} title="持久规格创作／澄清" width={850} onCancel={close} footer={<Space>
    <Button onClick={close}>关闭（本地编辑可恢复）</Button>
    {authoringCanEnterAdmitted(view, workspaceId) && <Button type="primary" disabled={recoveryBlocked.current || busy} onClick={enterAdmitted}>恢复已准入工作空间</Button>}
    <Button loading={busy} disabled={recoveryBlocked.current || busy || !view || sealed || Boolean(exact)} onClick={generate}>保存修订并审查</Button>
    <Button type="primary" loading={busy} disabled={recoveryBlocked.current || busy || !canConfirm} onClick={confirm}>{exact ? '恢复原确认并准入' : '确认并准入'}</Button>
  </Space>}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="info" message="修改创建期目标、模板、资源或用户验收要求后，必须重新完成六审查；此操作不会启动 Run。同 Run 规格修订不受支持。" />
      {recoveryBlocked.current && <Alert type="error" showIcon message="本地恢复记录不可用；原记录已保留，禁止保存、确认或清理。请显式处理该记录后重新打开。" />}
      {exact && <Alert type="warning" message="原确认请求尚未恢复；保存修订与重新审查已暂停，原 hashes 和 expected revision 保持不变。" />}
      {failure && <Alert type="error" showIcon message={failure} />}
      {recovery.current.pendingSave && <Alert type="warning" message="原保存请求尚未确认，重试先按原 CAS 身份恢复；冲突不能自动覆盖。" action={<Button disabled={recoveryBlocked.current || busy} onClick={refreshSaveBaseline}>冲突后读取当前草稿</Button>} />}
      {edited && view?.input && <Alert type="info" message="owner 当前目标（本地编辑尚未覆盖）" description={view.input.goalText} />}
      <Form form={form} layout="vertical" disabled={recoveryBlocked.current || busy || !view || sealed || Boolean(exact)} onValuesChange={(_, values: Values) => { if (recoveryBlocked.current || recovery.current.pendingConfirmation) return; editVersion.current++; setEdited(true); try { persist({ ...recovery.current, values }); } catch (error) { setFailure(errorMessage(error)); } }}>
        <Form.Item name="templateId" label="模板" rules={[{ required: true }]}><Select options={templates.map(item => ({ value: item.id, label: item.name || item.id }))} /></Form.Item>
        <Form.Item name="goalText" label="目标" rules={[{ required: true }]}><Input.TextArea rows={4} /></Form.Item>
        <Form.Item name="resources" label="本地资源引用（每行一个）"><Input.TextArea rows={2} /></Form.Item>
        <Form.Item name="requirements" label="额外用户验收要求（每行一条）"><Input.TextArea rows={3} /></Form.Item>
      </Form>
      {view && <Typography.Text>{view.workspaceId} · revision {view.inputRevision} · {view.state}</Typography.Text>}
      {view && !view.templateCurrent && <Alert type="warning" message="模板已变化，请保存修订并重新审查。" />}
      {edited && <Alert type="warning" message="本地输入已修改，旧规格不可确认。" />}
      {view?.admissionViolationCode && <Alert type="warning" message={view.admissionViolationCode} description={view.admissionOffendingRefs?.join('、')} />}
      {view?.requests?.filter(request => request.state === 'failed').map((request, index) => <Alert key={`${request.phase}-${request.reviewKind || index}`} type="error" message={`规格请求失败：${request.phase}`} description={request.failureCode} />)}
      {view?.state === 'outcome_unresolved' && <Alert type="warning" message="持久请求结果未决，不会自动重发 provider；可查询恢复或修订输入。" />}
      <List dataSource={view?.proposal?.verifications || []} renderItem={item => <List.Item>{item.id} · {item.status} · {item.declaration}</List.Item>} />
      <List dataSource={view?.proposal?.items || []} renderItem={item => <List.Item><Space><Tag>{authoringSourceLabel(item.sourceKind)}</Tag>{item.text}</Space></List.Item>} />
      <List dataSource={view?.proposal?.reviews || []} renderItem={item => <List.Item><Space><Tag color={item.status === 'PASS' ? 'green' : 'orange'}>{item.status}</Tag>{item.kind}：{item.reason}</Space></List.Item>} />
    </Space>
  </Modal>;
}
