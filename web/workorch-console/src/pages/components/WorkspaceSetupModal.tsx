import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, App as AntApp, Button, Form, Input, List, Modal, Select, Space, Tag, Typography } from 'antd';
import type { FormInstance } from 'antd';
import { workorchApi } from '../../api';
import { WorkorchRequestError } from '../../creationRecovery';
import type { CreationRecovery } from '../../creationRecovery';
import { errorMessage } from '../../utils/errorMessage';
import { AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS, authoringSourceLabel, AuthoringAcceptedPreparationObservation, AuthoringQueryFence, mergeAuthoringObservation, AuthoringSessionFence, authoringCanEnterAdmitted, authoringCanConfirm, authoringCanReplayConfirmation, authoringRequireNoPendingConfirmation, authoringNeedsSave, authoringNeedsPolling } from '../../workspaceAuthoring';
import type { AuthoringRecovery } from '../../workspaceAuthoring';
import type { DefinitionAuthoringView, WorkspaceTemplate } from '../../types';
import { AuthoringDiagnosticDetails } from './AuthoringDiagnosticDetails';
import { AuthoringPreparationFailure } from './AuthoringPreparationFailure';
import { AuthoringFailedRecoveryNotice } from './AuthoringFailedRecoveryNotice';

type Props = { open: boolean; onClose: () => void; onCreated: (workspaceId: string, isCurrent: () => boolean) => void | Promise<void> };
type Values = { title: string; templateId: string; deliverableDirectory?: string; goalText: string; requirements?: string; resources?: string };
const lines = (value?: string) => (value || '').split('\n').map(line => line.trim()).filter(Boolean);

const SetupInputForm = memo(function SetupInputForm({ form, templateOptions, disabled, created, onValuesChange }: {
  form: FormInstance<Values>;
  templateOptions: { value: string; label: string }[];
  disabled: boolean;
  created: boolean;
  onValuesChange: () => void;
}) {
  return <Form form={form} layout="vertical" onValuesChange={onValuesChange} disabled={disabled}>
    <Form.Item name="title" label="名称" rules={[{ required: true }]}><Input disabled={created} /></Form.Item>
    <Form.Item name="templateId" label="模板" rules={[{ required: true }]}><Select options={templateOptions} /></Form.Item>
    <Form.Item name="deliverableDirectory" label="工作目录（可留空，自动解析）"><Input disabled={created} /></Form.Item>
    <Form.Item name="goalText" label="目标" rules={[{ required: true }]}><Input.TextArea rows={4} /></Form.Item>
    <Form.Item name="resources" label="本地资源引用（每行一个，最多 8 个，相对于工作目录）"><Input.TextArea rows={2} /></Form.Item>
    <Form.Item name="requirements" label="额外验收要求（可选，每行一条）"><Input.TextArea rows={3} /></Form.Item>
  </Form>;
});

const SetupProposal = memo(function SetupProposal({ proposal }: { proposal: NonNullable<DefinitionAuthoringView['proposal']> }) {
  return <>
    <Typography.Title level={5}>待确认验收清单</Typography.Title>
    <List dataSource={proposal.items} renderItem={item => <List.Item><Space align="start"><Tag>{authoringSourceLabel(item.sourceKind)}</Tag><Typography.Text>{item.text}</Typography.Text></Space></List.Item>} />
    <Typography.Title level={5}>六类独立审查</Typography.Title>
    <List dataSource={proposal.reviews || []} renderItem={review => <List.Item><Space><Tag color={review.status === 'PASS' ? 'green' : 'orange'}>{review.status}</Tag><Typography.Text>{review.kind}：{review.reason || '无附加说明'}</Typography.Text></Space></List.Item>} />
    <Typography.Title level={5}>验证声明</Typography.Title>
    <List dataSource={proposal.verifications || []} renderItem={item => <List.Item><Typography.Text>{item.id} · {item.status} · {item.declaration}</Typography.Text></List.Item>} />
  </>;
});

export function WorkspaceSetupModal({ open, onClose, onCreated }: Props) {
  const { message, modal } = AntApp.useApp();
  const [form] = Form.useForm<Values>();
  const formOpened = useRef(false);
  const [templates, setTemplates] = useState<WorkspaceTemplate[]>([]);
  const templateOptions = useMemo(() => templates.map(item => ({ value: item.id, label: item.name || item.id })), [templates]);
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [recovery, setRecovery] = useState<AuthoringRecovery>();
  const recoveryRef = useRef<AuthoringRecovery | undefined>(undefined);
  const setRecoveryMemory = (next: AuthoringRecovery | undefined) => { recoveryRef.current = next; setRecovery(next); };
  const [busy, setBusy] = useState(false);
  const submitting = useRef(false);
  const [edited, setEdited] = useState(false);
  const onValuesChange = useCallback(() => { setEdited(true); }, []);
  const [failure, setFailure] = useState('');
  const [creationFailure, setCreationFailure] = useState<CreationRecovery>();
  const [creationRejected, setCreationRejected] = useState(false);
  const session = useRef(new AuthoringSessionFence());
  const queries = useRef(new AuthoringQueryFence());
  const acceptedPreparationObservation = useRef(new AuthoringAcceptedPreparationObservation());
  const [acceptedPreparationObservationTimedOut, setAcceptedPreparationObservationTimedOut] = useState(false);
  const [acceptedPreparationObservationActive, setAcceptedPreparationObservationActive] = useState(false);
  const [acceptedPreparationObservationGeneration, setAcceptedPreparationObservationGeneration] = useState(0);
  const observe = (next: DefinitionAuthoringView) => setView(current => mergeAuthoringObservation(current, next));
  const close = (force = false) => {
    if (!force && (edited || busy || acceptedPreparationObservationActive || (recoveryRef.current && (!view || recoveryRef.current.pendingSave || recoveryRef.current.pendingConfirmation)) || ['confirming', 'admitting'].includes(view?.state || ''))) {
      const closeToken = session.current.current();
      modal.confirm({
        title: '关闭并丢弃页面内输入与重放身份？',
        content: '关闭将丢弃页面内未提交输入和请求重放身份；已发送的服务端事务不会取消。若事务结果未决，请关闭后从工作空间列表核对服务端状态，再决定如何继续。取消可保留当前页面继续处理。',
        okText: '关闭并丢弃',
        cancelText: '继续编辑',
        onOk: () => { if (session.current.matches(closeToken)) close(true); },
      });
      return;
    }
    session.current.begin(); queries.current.invalidate();
    recoveryRef.current = undefined;
    setRecovery(undefined);
    setView(undefined);
    setCreationFailure(undefined);
    setCreationRejected(false);
    setEdited(false);
    setAcceptedPreparationObservationActive(false);
    setAcceptedPreparationObservationTimedOut(false);
    acceptedPreparationObservation.current = new AuthoringAcceptedPreparationObservation();
    setBusy(false);
    submitting.current = false;
    form.resetFields();
    formOpened.current = false;
    onClose();
  };

  const persist = (value: AuthoringRecovery) => setRecoveryMemory(value);
  const applyView = (next: DefinitionAuthoringView) => {
    observe(next);
    setEdited(false);
    if (next.input) {
      const values = { templateId: next.templateId, goalText: next.input.goalText, requirements: next.input.acceptanceRequirements?.join('\n'), resources: next.input.resourceRefs?.join('\n') };
      // 原草稿重试不重复发布相同字段；owner 规范化后的变化仍逐项更新。
      const changed = Object.fromEntries(Object.entries(values).filter(([name, value]) => !Object.is(form.getFieldValue(name as keyof Values), value)));
      if (Object.keys(changed).length) form.setFieldsValue(changed);
    }
  };

  useEffect(() => {
    if (!open) return;
    const token = session.current.begin();
    queries.current.invalidate();
    setBusy(false);
    submitting.current = false;
    let disposed = false;
    setFailure('');
    setAcceptedPreparationObservationTimedOut(false);
    setAcceptedPreparationObservationActive(false);
    acceptedPreparationObservation.current = new AuthoringAcceptedPreparationObservation();
    setRecoveryMemory(undefined);
    setView(undefined);
    setCreationFailure(undefined);
    setCreationRejected(false);
    setEdited(false);
    // 首次或已在关闭时清空的表单无需重挂；外部重开仍清空字段与校验状态。
    if (formOpened.current) form.resetFields();
    formOpened.current = true;
    workorchApi.listWorkspaceTemplates().then(items => { if (!disposed && session.current.matches(token)) setTemplates(items); }).catch(error => { if (!disposed && session.current.matches(token)) setFailure(errorMessage(error)); });
    return () => { disposed = true; session.current.begin(); };
  }, [open]);

  useEffect(() => {
    if (!open || !acceptedPreparationObservationActive) return;
    const token = session.current.current();
    let disposed = false;
    const timeout = window.setTimeout(() => {
      if (disposed || !session.current.matches(token) || !acceptedPreparationObservation.current.expire()) return;
      queries.current.invalidate();
      setAcceptedPreparationObservationActive(false);
      setAcceptedPreparationObservationTimedOut(true);
    }, AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS);
    return () => { disposed = true; window.clearTimeout(timeout); };
  }, [open, acceptedPreparationObservationActive, acceptedPreparationObservationGeneration]);

  useEffect(() => {
    if (!open || busy || !view || !authoringNeedsPolling(view, acceptedPreparationObservationActive)) return;
    const token = session.current.current();
    let disposed = false;
    let timer: number;
    const poll = async () => {
      if (acceptedPreparationObservationActive && !acceptedPreparationObservation.current.isActive()) return;
      const query = queries.current.begin();
      try {
        const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(view.workspaceId), () => !disposed && session.current.matches(token) && queries.current.matches(query));
        if (!disposed && session.current.matches(token) && queries.current.matches(query)) {
          if (acceptedPreparationObservationActive) {
            const acceptedObservation = acceptedPreparationObservation.current.observe(next);
            setAcceptedPreparationObservationActive(acceptedObservation.active);
            setAcceptedPreparationObservationTimedOut(acceptedObservation.exhausted);
          }
          observe(next);
        }
      } catch (error) {
        if (!disposed && session.current.matches(token) && queries.current.matches(query)) setFailure(errorMessage(error));
      } finally {
        if (!disposed && session.current.matches(token) && queries.current.matches(query) && (!acceptedPreparationObservationActive || acceptedPreparationObservation.current.isActive())) timer = window.setTimeout(poll, 1500);
      }
    };
    timer = window.setTimeout(poll, 1500);
    return () => { disposed = true; window.clearTimeout(timer); };
  }, [open, busy, view?.workspaceId, view?.state, view?.compilationAccepted, acceptedPreparationObservationActive, acceptedPreparationObservationGeneration]);

  // STEP01 only: create/replay the original payload, never PUT or compile.
  const saveDraft = async () => {
    if (submitting.current || view || recoveryRef.current?.pendingSave || recoveryRef.current?.pendingConfirmation) return;
    submitting.current = true;
    const token = session.current.current();
    const hadPriorRequest = Boolean(recoveryRef.current);
    queries.current.invalidate();
    setBusy(true); setFailure(''); setCreationRejected(false);
    try {
      let saved = recoveryRef.current;
      if (!saved) {
        const values = await form.validateFields();
        if (!session.current.matches(token)) return;
        saved = { request: { requestId: crypto.randomUUID(), title: values.title, templateId: values.templateId, deliverableDirectory: values.deliverableDirectory, input: { goalText: values.goalText, acceptanceRequirements: lines(values.requirements), resourceRefs: lines(values.resources) } } };
        persist(saved);
      }
      const next = await workorchApi.createDefinitionAuthoring(saved.request);
      if (!session.current.matches(token)) return;
      if (next.state !== 'draft' || next.inputRevision !== 1 || !next.inputHash || next.creation?.requestId !== saved.request.requestId || !next.creation.directoryPrepared) throw new Error('创建返回尚不能证明本步草稿完整保存，请保留原身份核对服务端');
      persist({ request: saved.request, workspaceId: next.workspaceId });
      setCreationFailure(undefined);
      observe(next);
      const values = form.getFieldsValue();
      setEdited(authoringNeedsSave(next, { goalText: values.goalText, acceptanceRequirements: lines(values.requirements), resourceRefs: lines(values.resources) }, values.templateId));
      message.success('草稿已保存；未生成合同，未启动执行');
    } catch (error) {
      if (session.current.matches(token)) {
        setFailure(errorMessage(error));
        if (error instanceof WorkorchRequestError && error.creationRecovery?.requestId === recoveryRef.current?.request.requestId) setCreationFailure(error.creationRecovery);
        // Only a first attempt's explicit pre-write rejection releases its identity.
        // An earlier unknown is never erased by a later error response.
        if (!hadPriorRequest && error instanceof WorkorchRequestError && error.creationRejectedBeforeDispatch && !error.creationRecovery && error.status === 400) {
          setRecoveryMemory(undefined); setCreationRejected(true);
        }
      }
    } finally { if (session.current.matches(token)) { submitting.current = false; setBusy(false); } }
  };

  const generate = async () => {
    if (submitting.current) return;
    submitting.current = true;
    const token = session.current.current();
    queries.current.invalidate();
    acceptedPreparationObservation.current = new AuthoringAcceptedPreparationObservation();
    setAcceptedPreparationObservationActive(false);
    setAcceptedPreparationObservationTimedOut(false);
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
        current = await workorchApi.createDefinitionAuthoring(saved.request);
        if (!session.current.matches(token)) return;
        saved = { ...saved, workspaceId: current.workspaceId };
        setCreationFailure(undefined);
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
      if (next.compilationAccepted) {
        acceptedPreparationObservation.current.begin(current);
        const acceptedObservation = acceptedPreparationObservation.current.observe(next);
        setAcceptedPreparationObservationActive(acceptedObservation.active);
        setAcceptedPreparationObservationTimedOut(false);
        setAcceptedPreparationObservationGeneration(generation => generation + 1);
      }
      observe(next);
    } catch (error) {
      if (session.current.matches(token)) {
        setFailure(errorMessage(error));
        if (error instanceof WorkorchRequestError && error.creationRecovery?.requestId === recoveryRef.current?.request.requestId) setCreationFailure(error.creationRecovery);
      }
    } finally {
      if (session.current.matches(token)) { submitting.current = false; setBusy(false); }
    }
  };

  const refreshSaveBaseline = () => {
    if (!view || !recovery?.pendingSave) return;
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
    if (!view || !canConfirm) return;
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
        message.success('规格已确认并准入；运行需另行显式启动');
        close(true);
      }
    } catch (error) {
      if (session.current.matches(token)) setFailure(errorMessage(error));
    } finally { if (session.current.matches(token)) setBusy(false); }
  };

  const enterAdmitted = async () => {
    if (!view || !authoringCanEnterAdmitted(view, recovery?.workspaceId)) return;
    const token = session.current.current();
    queries.current.invalidate();
    setBusy(true); setFailure('');
    try {
      await onCreated(view.workspaceId, () => session.current.matches(token));
      if (!session.current.matches(token)) return;
      close(true);
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };

  const enterDraft = async () => {
    if (busy || edited || !view) return;
    const token = session.current.current();
    setBusy(true); setFailure('');
    try {
      await onCreated(view.workspaceId, () => session.current.matches(token));
      if (session.current.matches(token)) close(true);
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };

  return <Modal open={open} title="新建工作空间 · 规格创作" width={850} onCancel={() => close()} footer={<Space>
    <Button onClick={() => close()}>关闭</Button>
    <Button loading={busy} disabled={busy || Boolean(view) || Boolean(recovery?.pendingSave) || Boolean(recovery?.pendingConfirmation)} onClick={saveDraft}>{recovery && !view ? '按原请求恢复草稿保存' : '保存草稿'}</Button>
    {view?.state === 'draft' && <Button disabled={busy || edited} onClick={enterDraft}>查看已保存草稿</Button>}
    {authoringCanEnterAdmitted(view, recovery?.workspaceId) && <Button type="primary" disabled={busy} onClick={enterAdmitted}>进入已准入工作空间</Button>}
    <Button loading={busy} onClick={generate} disabled={busy || Boolean(recovery?.pendingConfirmation) || ['confirming', 'admitting', 'admitted'].includes(view?.state || '')}>保存并生成／恢复审查</Button>
    <Button type="primary" loading={busy} onClick={confirm} disabled={busy || !canConfirm}>{recovery?.pendingConfirmation ? '恢复原确认并准入' : '确认规格并准入'}</Button>
  </Space>}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="info" showIcon message="保存草稿仅持久化输入，不生成或确认合同，不启动 Run。保存并生成会另行发起规格审查。未保存输入与重放身份仅在当前页面内存中，刷新或关闭后丢失；已保存草稿可从服务端列表找回。" />
      {creationFailure && <Alert type="warning" showIcon message={creationFailure.persistence === 'persisted' ? 'Workspace 身份已持久，创建准备待恢复' : '创建结果未知，请先核对原请求'} description={`Workspace：${creationFailure.workspaceId}；requestId：${creationFailure.requestId}；阶段：${creationFailure.phase}。重试沿用原 payload，补齐准备后才继续；请勿更换请求重复创建。`} />}
      {creationRejected && <Alert type="warning" message="输入在保存前被拒绝，草稿未保存。请更正输入后重新提交。" />}
      {failure && <Alert type="error" showIcon message={failure} description={recovery ? '创建身份已保留。重试将恢复原草稿，不会重新创建工作空间。' : undefined} />}
      {view?.preparationDiagnostics?.map(diagnostic => <AuthoringPreparationFailure key={diagnostic.diagnosticId} diagnostic={diagnostic} />)}
      {acceptedPreparationObservationActive && <Alert type="info" showIcon message="本次任务已受理，正在观察 owner 状态；页面观察是临时的，不会恢复或取消服务端事务。" />}
      {acceptedPreparationObservationTimedOut && <Alert type="warning" showIcon message="观察期限已到，最近一次可用 owner 状态为原前置准备失败；无法确认后台任务是否仍在执行。请手动刷新核对或修复后显式重试。" />}
      {view?.requests?.filter(request => request.state === 'failed').map((request, index) => <Alert key={`${request.phase}-${request.reviewKind || index}`} type="error" showIcon message={`规格请求失败：${request.phase}${request.reviewKind ? ` · ${request.reviewKind}` : ''}`} description={<AuthoringDiagnosticDetails workspaceId={view.workspaceId} permitId={request.permitId} phase={request.phase} failureCode={request.failureCode} diagnostic={request.diagnostic} />} />)}
      {recovery?.pendingSave && <Alert type="warning" message="保存响应尚未确认。重试将先恢复原保存请求，再处理当前编辑；不会用新身份覆盖未决修订。" action={<Button disabled={busy} onClick={refreshSaveBaseline}>冲突后读取当前草稿</Button>} />}
      {edited && view?.input && <Alert type="info" message="owner 当前目标（本地编辑尚未覆盖）" description={view.input.goalText} />}
      {edited && <Alert type="info" message="本地编辑尚未保存到服务端，关闭将丢弃。" />}
      {recovery && !view && <Alert type="warning" message="原创建结果待核对。重试只使用原身份与原 payload；请勿盲目新建。当前修改不会覆盖原请求，关闭后先从服务端列表核对。" />}
      <SetupInputForm form={form} templateOptions={templateOptions} onValuesChange={onValuesChange} created={Boolean(recovery)} disabled={busy || Boolean(recovery?.pendingConfirmation) || ['confirming', 'admitting', 'admitted'].includes(view?.state || '')} />
      {view && <>
        <Typography.Text>草稿：{view.workspaceId} · revision {view.inputRevision} · {view.state}</Typography.Text>
        {view.state === 'draft' && <Alert type="success" message="草稿已持久保存（M1），尚未形成正式合同或执行准入。" description={`创建 requestId：${view.creation?.requestId || '未公开'}；目录准备：${view.creation?.directoryPrepared ? '原创建阶段已完成，不代表当前资源就绪' : '待核对'}。`} />}
        {recovery?.pendingConfirmation && !authoringCanReplayConfirmation(view, recovery) && view.state !== 'admitted' && <Alert type="warning" message="原确认请求与当前草稿不一致，不能用旧凭据确认新规格。保留原请求，先核对并恢复确认；不能自动生成替代确认。" />}
        {edited && <Alert type="warning" message="输入已修改，旧规格不可确认。请保存并重新审查。" />}
        {!view.templateCurrent && <Alert type="warning" message="模板已变化或不可用，请保存新的创作 revision。" />}
        {view.state === 'failed' && <AuthoringFailedRecoveryNotice />}
        {view.state === 'outcome_unresolved' && <Alert type="warning" message="已有持久请求结果未决，不会自动重发 provider。可查询恢复结果或修改输入形成新 revision。" />}
        {view.admissionViolationCode && <Alert type="warning" message={`准入需澄清：${view.admissionViolationCode}`} description={view.admissionOffendingRefs?.join('、')} />}
        {view.proposal && <SetupProposal proposal={view.proposal} />}
      </>}
    </Space>
  </Modal>;
}
