import { useEffect, useRef, useState } from 'react';
import { Alert, App as AntApp, Button, Form, Input, List, Modal, Select, Space, Tag, Typography } from 'antd';
import { workorchApi } from '../../api';
import { errorMessage } from '../../helpers';
import { authoringSourceLabel, AuthoringQueryFence, mergeAuthoringObservation, AuthoringSessionFence, authoringCanEnterAdmitted, authoringCanConfirm, authoringCanReplayConfirmation, authoringRequireNoPendingConfirmation, authoringNeedsSave, authoringNeedsPolling, authoringLocalDraftKey, authoringRecoveryKey, parseAuthoringLocalDraft, restoreAuthoringRecovery } from '../../workspaceAuthoring';
import type { AuthoringRecovery } from '../../workspaceAuthoring';
import type { DefinitionAuthoringView, WorkspaceTemplate } from '../../types';

type Props = { open: boolean; onClose: () => void; onCreated: (workspaceId: string, isCurrent: () => boolean) => void | Promise<void> };
type Values = { title: string; templateId: string; deliverableDirectory?: string; goalText: string; requirements?: string; resources?: string };
const lines = (value?: string) => (value || '').split('\n').map(line => line.trim()).filter(Boolean);

export function WorkspaceSetupModal({ open, onClose, onCreated }: Props) {
  const { message } = AntApp.useApp();
  const [form] = Form.useForm<Values>();
  const [templates, setTemplates] = useState<WorkspaceTemplate[]>([]);
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [recovery, setRecovery] = useState<AuthoringRecovery>();
  const [busy, setBusy] = useState(false);
  const [edited, setEdited] = useState(false);
  const [failure, setFailure] = useState('');
  const recoveryBlocked = useRef(false);
  const editVersion = useRef(0);
  const session = useRef(new AuthoringSessionFence());
  const queries = useRef(new AuthoringQueryFence());
  const observe = (next: DefinitionAuthoringView) => setView(current => mergeAuthoringObservation(current, next));
  const close = () => { session.current.begin(); queries.current.invalidate(); onClose(); };

  const saveLocalDraft = (values: Values) => {
    if (recoveryBlocked.current) return;
    localStorage.setItem(authoringLocalDraftKey, JSON.stringify({ title: values.title || '', templateId: values.templateId || '', deliverableDirectory: values.deliverableDirectory, goalText: values.goalText || '', requirements: values.requirements, resources: values.resources }));
  };
  const clearRecovery = () => {
    localStorage.removeItem(authoringRecoveryKey);
    localStorage.removeItem(authoringLocalDraftKey);
    setRecovery(undefined);
  };

  const persist = (value: AuthoringRecovery) => {
    localStorage.setItem(authoringRecoveryKey, JSON.stringify(value));
    setRecovery(value);
  };
  const applyView = (next: DefinitionAuthoringView) => {
    observe(next);
    setEdited(false);
    localStorage.removeItem(authoringLocalDraftKey);
    if (next.input) form.setFieldsValue({ templateId: next.templateId, goalText: next.input.goalText, requirements: next.input.acceptanceRequirements?.join('\n'), resources: next.input.resourceRefs?.join('\n') });
  };

  useEffect(() => {
    if (!open) return;
    const token = session.current.begin();
    queries.current.invalidate();
    setBusy(false);
    let disposed = false;
    setFailure('');
    workorchApi.listWorkspaceTemplates().then(items => { if (!disposed && session.current.matches(token)) setTemplates(items); }).catch(error => { if (!disposed && session.current.matches(token)) setFailure(errorMessage(error)); });
    let saved: AuthoringRecovery | undefined;
    let localDraft: ReturnType<typeof parseAuthoringLocalDraft>;
    recoveryBlocked.current = false;
    try {
      saved = restoreAuthoringRecovery(localStorage.getItem(authoringRecoveryKey));
      const draftText = localStorage.getItem(authoringLocalDraftKey);
      localDraft = parseAuthoringLocalDraft(draftText);
      if (draftText !== null && !localDraft) throw new Error('本地编辑记录损坏；未覆盖原记录');
    } catch (error) {
      recoveryBlocked.current = true;
      setFailure(`本地恢复不可用：${errorMessage(error)}；禁止创建替代事务。`);
    }
    const openingEditVersion = editVersion.current;
    setRecovery(saved);
    setView(undefined);
    setEdited(Boolean(localDraft || saved?.pendingSave));
    form.resetFields();
    if (saved) {
      form.setFieldsValue({ title: saved.request.title, templateId: saved.request.templateId, deliverableDirectory: saved.request.deliverableDirectory, goalText: saved.request.input.goalText, requirements: saved.request.input.acceptanceRequirements?.join('\n'), resources: saved.request.input.resourceRefs?.join('\n') });
      if (saved.pendingSave) form.setFieldsValue({ templateId: saved.pendingSave.templateId, goalText: saved.pendingSave.input.goalText, requirements: saved.pendingSave.input.acceptanceRequirements?.join('\n'), resources: saved.pendingSave.input.resourceRefs?.join('\n') });
      const query = queries.current.begin();
      if (saved.workspaceId) queries.current.read(() => workorchApi.readDefinitionAuthoring(saved!.workspaceId!), () => !disposed && session.current.matches(token) && queries.current.matches(query)).then(next => {
        if (disposed || !session.current.matches(token) || !queries.current.matches(query)) return;
        if (localDraft || saved.pendingSave || editVersion.current !== openingEditVersion) observe(next);
        else applyView(next);
      }).catch(error => { if (!disposed && session.current.matches(token) && queries.current.matches(query)) setFailure(errorMessage(error)); });
    }
    if (localDraft) form.setFieldsValue(localDraft);
    return () => { disposed = true; session.current.begin(); };
  }, [open]);

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
    if (recoveryBlocked.current) return;
    const token = session.current.current();
    queries.current.invalidate();
    setBusy(true);
    setFailure('');
    try {
      authoringRequireNoPendingConfirmation(recovery);
      const values = await form.validateFields();
      if (!session.current.matches(token)) return;
      const input = { goalText: values.goalText, acceptanceRequirements: lines(values.requirements), resourceRefs: lines(values.resources) };
      let current = view;
      let saved = recovery;
      if (!current) {
        // 发送前保存原创建 payload；响应丢失后必须以原身份重放。
        saved = saved || { request: { requestId: crypto.randomUUID(), title: values.title, templateId: values.templateId, deliverableDirectory: values.deliverableDirectory, input } };
        persist(saved);
        current = saved.workspaceId ? await queries.current.read(() => workorchApi.readDefinitionAuthoring(saved!.workspaceId!), () => session.current.matches(token)) : await workorchApi.createDefinitionAuthoring(saved.request);
        if (!session.current.matches(token)) return;
        saved = { ...saved, workspaceId: current.workspaceId };
        persist(saved);
        observe(current);
      }
      if (saved?.pendingSave) {
        const pending = saved.pendingSave;
        current = await workorchApi.saveDefinitionAuthoring(current.workspaceId, pending.input, pending.templateId, pending.expectedWorkspaceRevision, pending.expectedInputHash, pending.requestId);
        if (!session.current.matches(token)) return;
        saved = { request: saved.request, workspaceId: current.workspaceId };
        persist(saved);
        observe(current);
      }
      if (authoringNeedsSave(current, input, values.templateId)) {
        if (!saved) throw new Error('创建恢复身份缺失，不能发送修订');
        const pendingSave = { requestId: crypto.randomUUID(), input, templateId: values.templateId, expectedWorkspaceRevision: current.workspaceRevision, expectedInputHash: current.inputHash || '' };
        saved = { request: saved.request, workspaceId: current.workspaceId, pendingSave };
        persist(saved);
        current = await workorchApi.saveDefinitionAuthoring(current.workspaceId, pendingSave.input, pendingSave.templateId, pendingSave.expectedWorkspaceRevision, pendingSave.expectedInputHash, pendingSave.requestId);
        if (!session.current.matches(token)) return;
        saved = { request: saved.request, workspaceId: current.workspaceId };
        persist(saved);
      }
      applyView(current);
      const next = await workorchApi.compileDefinitionAuthoring(current.workspaceId);
      if (!session.current.matches(token)) return;
      observe(next);
    } catch (error) {
      if (session.current.matches(token)) setFailure(errorMessage(error));
    } finally {
      if (session.current.matches(token)) setBusy(false);
    }
  };

  const refreshSaveBaseline = () => {
    if (recoveryBlocked.current || !view || !recovery?.pendingSave) return;
    const token = session.current.current();
    Modal.confirm({
      title: '读取当前草稿并放弃原保存请求的重放身份？',
      content: '本地输入保留，不会保存或覆盖 owner。核对当前目标后，可另行显式提交修订。',
      okText: '读取当前草稿',
      cancelText: '保留原请求',
      onOk: async () => {
        if (!session.current.matches(token)) return;
        queries.current.invalidate();
        setBusy(true); setFailure('');
        try {
          const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(view.workspaceId), () => session.current.matches(token));
          if (!session.current.matches(token)) return;
          saveLocalDraft(form.getFieldsValue());
          persist({ request: recovery.request, workspaceId: next.workspaceId });
          observe(next);
          setEdited(true);
        } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
        finally { if (session.current.matches(token)) setBusy(false); }
      }
    });
  };

  const canConfirm = !recovery?.pendingSave && (recovery?.pendingConfirmation ? authoringCanReplayConfirmation(view, recovery) : !edited && authoringCanConfirm(view));

  const confirm = async () => {
    const token = session.current.current();
    if (recoveryBlocked.current || !view || !canConfirm) return;
    queries.current.invalidate();
    setBusy(true);
    setFailure('');
    try {
      const pendingConfirmation = recovery?.pendingConfirmation || { inputHash: view.inputHash || '', derivationPermitId: view.derivationPermitId || '', proposalHash: view.proposal?.proposalHash || '', specificationHash: view.proposal?.specificationHash || '', expectedWorkspaceRevision: view.workspaceRevision };
      if (!recovery) throw new Error('缺少原创建身份，不能提交确认');
      if (!recovery.pendingConfirmation) persist({ ...recovery, pendingConfirmation });
      const next = await workorchApi.confirmDefinitionAuthoring(view, pendingConfirmation);
      if (!session.current.matches(token)) return;
      observe(next);
      if (authoringCanEnterAdmitted(next, view.workspaceId)) {
        await onCreated(next.workspaceId, () => session.current.matches(token));
        if (!session.current.matches(token)) return;
        clearRecovery();
        message.success('规格已确认并准入；运行需另行显式启动');
        close();
      }
    } catch (error) {
      if (session.current.matches(token)) setFailure(errorMessage(error));
    } finally { if (session.current.matches(token)) setBusy(false); }
  };

  const enterAdmitted = async () => {
    if (recoveryBlocked.current || !view || !authoringCanEnterAdmitted(view, recovery?.workspaceId)) return;
    const token = session.current.current();
    queries.current.invalidate();
    setBusy(true); setFailure('');
    try {
      await onCreated(view.workspaceId, () => session.current.matches(token));
      if (!session.current.matches(token)) return;
      clearRecovery(); close();
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };

  return <Modal open={open} title="新建工作空间 · 规格创作" width={850} onCancel={close} footer={<Space>
    <Button onClick={close}>关闭（本地编辑与已保存草稿可恢复）</Button>
    {authoringCanEnterAdmitted(view, recovery?.workspaceId) && <Button type="primary" disabled={recoveryBlocked.current || busy} onClick={enterAdmitted}>进入已准入工作空间</Button>}
    <Button loading={busy} onClick={generate} disabled={recoveryBlocked.current || busy || Boolean(recovery?.pendingConfirmation) || ['confirming', 'admitting', 'admitted'].includes(view?.state || '')}>保存并生成／恢复审查</Button>
    <Button type="primary" loading={busy} onClick={confirm} disabled={recoveryBlocked.current || busy || !canConfirm}>{recovery?.pendingConfirmation ? '恢复原确认并准入' : '确认规格并准入'}</Button>
  </Space>}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="info" showIcon message="输入目标即可，验收清单由模板与目标派生。六类审查通过后才可确认；此操作不会启动 Run。" />
      {recoveryBlocked.current && <Alert type="error" showIcon message="本地恢复记录不可用；原记录已保留，禁止创建替代事务或清理。请显式处理该记录后重新打开。" />}
      {failure && <Alert type="error" showIcon message={failure} description={recovery ? '创建身份已保留。重试将恢复原草稿，不会重新创建工作空间。' : undefined} />}
      {view?.requests?.filter(request => request.state === 'failed').map((request, index) => <Alert key={`${request.phase}-${request.reviewKind || index}`} type="error" showIcon message={`规格请求失败：${request.phase}${request.reviewKind ? ` · ${request.reviewKind}` : ''}`} description={`${request.failureCode || '请求失败'}。当前输入不会自动重发 provider；请修订输入后重新审查。`} />)}
      {recovery?.pendingSave && <Alert type="warning" message="保存响应尚未确认。重试将先恢复原保存请求，再处理当前编辑；不会用新身份覆盖未决修订。" action={<Button disabled={recoveryBlocked.current || busy} onClick={refreshSaveBaseline}>冲突后读取当前草稿</Button>} />}
      {edited && view?.input && <Alert type="info" message="owner 当前目标（本地编辑尚未覆盖）" description={view.input.goalText} />}
      {edited && <Alert type="info" message="本地编辑尚未保存到服务端。关闭／刷新可在此浏览器恢复，不能作为准入输入。" />}
      {recovery && !view && <Alert type="warning" message="存在未完成的创建请求。将先以原 payload 恢复，再保存当前修改。" />}
      <Form form={form} layout="vertical" onValuesChange={(_, values) => { editVersion.current++; setEdited(true); try { saveLocalDraft(values); } catch (error) { setFailure(`本地编辑保存失败：${errorMessage(error)}`); } }} disabled={recoveryBlocked.current || busy || Boolean(recovery?.pendingConfirmation) || ['confirming', 'admitting', 'admitted'].includes(view?.state || '')}>
        <Form.Item name="title" label="名称" rules={[{ required: true }]}><Input disabled={Boolean(recovery)} /></Form.Item>
        <Form.Item name="templateId" label="模板" rules={[{ required: true }]}><Select options={templates.map(item => ({ value: item.id, label: item.name || item.id }))} /></Form.Item>
        <Form.Item name="deliverableDirectory" label="工作目录（可留空，自动解析）"><Input disabled={Boolean(recovery)} /></Form.Item>
        <Form.Item name="goalText" label="目标" rules={[{ required: true }]}><Input.TextArea rows={4} /></Form.Item>
        <Form.Item name="resources" label="本地资源引用（每行一个，最多 8 个，相对于工作目录）"><Input.TextArea rows={2} /></Form.Item>
        <Form.Item name="requirements" label="额外验收要求（可选，每行一条）"><Input.TextArea rows={3} /></Form.Item>
      </Form>
      {view && <>
        <Typography.Text>草稿：{view.workspaceId} · revision {view.inputRevision} · {view.state}</Typography.Text>
        {recovery?.pendingConfirmation && !authoringCanReplayConfirmation(view, recovery) && view.state !== 'admitted' && <Alert type="warning" message="原确认请求与当前草稿不一致，不能用旧凭据确认新规格。保留原请求，先核对并恢复确认；不能自动生成替代确认。" />}
        {edited && <Alert type="warning" message="输入已修改，旧规格不可确认。请保存并重新审查。" />}
        {!view.templateCurrent && <Alert type="warning" message="模板已变化或不可用，请保存新的创作 revision。" />}
        {view.state === 'outcome_unresolved' && <Alert type="warning" message="已有持久请求结果未决，不会自动重发 provider。可查询恢复结果或修改输入形成新 revision。" />}
        {view.admissionViolationCode && <Alert type="warning" message={`准入需澄清：${view.admissionViolationCode}`} description={view.admissionOffendingRefs?.join('、')} />}
        {view.proposal && <>
          <Typography.Title level={5}>待确认验收清单</Typography.Title>
          <List dataSource={view.proposal.items} renderItem={item => <List.Item><Space align="start"><Tag>{authoringSourceLabel(item.sourceKind)}</Tag><Typography.Text>{item.text}</Typography.Text></Space></List.Item>} />
          <Typography.Title level={5}>六类独立审查</Typography.Title>
          <List dataSource={view.proposal.reviews || []} renderItem={review => <List.Item><Space><Tag color={review.status === 'PASS' ? 'green' : 'orange'}>{review.status}</Tag><Typography.Text>{review.kind}：{review.reason || '无附加说明'}</Typography.Text></Space></List.Item>} />
          <Typography.Title level={5}>验证声明</Typography.Title>
          <List dataSource={view.proposal.verifications || []} renderItem={item => <List.Item><Typography.Text>{item.id} · {item.status} · {item.declaration}</Typography.Text></List.Item>} />
        </>}
      </>}
    </Space>
  </Modal>;
}
