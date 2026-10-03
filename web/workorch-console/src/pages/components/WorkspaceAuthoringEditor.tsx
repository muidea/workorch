import { useEffect, useRef, useState } from 'react';
import { Alert, App as AntApp, Button, Form, Input, List, Modal, Select, Space, Tag, Typography } from 'antd';
import { workorchApi } from '../../api';
import { errorMessage } from '../../helpers';
import { AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS, authoringSourceLabel, AuthoringAcceptedPreparationObservation, AuthoringQueryFence, mergeAuthoringObservation, AuthoringSessionFence, authoringCanEnterAdmitted, authoringCanConfirm, authoringCanReplayConfirmation, authoringConfirmationRecovery, authoringRequireNoPendingConfirmation, authoringNeedsSave, authoringNeedsPolling } from '../../workspaceAuthoring';
import type { AuthoringEditorRecovery, AuthoringEditorValues } from '../../workspaceAuthoring';
import type { DefinitionAuthoringView, WorkspaceTemplate } from '../../types';
import { AuthoringDiagnosticDetails } from './AuthoringDiagnosticDetails';
import { AuthoringPreparationDiagnosticDetails } from './AuthoringPreparationDiagnosticDetails';
import { AuthoringFailedRecoveryNotice } from './AuthoringFailedRecoveryNotice';

type Props = { open: boolean; workspaceId?: string; onClose: () => void; onAdmitted: () => Promise<void> };
type Values = AuthoringEditorValues;
type Recovery = AuthoringEditorRecovery;
const lines = (value?: string) => (value || '').split('\n').map(item => item.trim()).filter(Boolean);

// 编辑已有 owner 草稿，不生成新创建身份，也不提供正式 Goal/DoD CRUD。
export function WorkspaceAuthoringEditor({ open, workspaceId, onClose, onAdmitted }: Props) {
  const { modal } = AntApp.useApp();
  const [form] = Form.useForm<Values>();
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [templates, setTemplates] = useState<WorkspaceTemplate[]>([]);
  const [busy, setBusy] = useState(false);
  const [edited, setEdited] = useState(false);
  const [failure, setFailure] = useState('');
  const recovery = useRef<Recovery>({});
  const editVersion = useRef(0);
  const session = useRef(new AuthoringSessionFence());
  const queries = useRef(new AuthoringQueryFence());
  const acceptedPreparationObservation = useRef(new AuthoringAcceptedPreparationObservation());
  const [acceptedPreparationObservationTimedOut, setAcceptedPreparationObservationTimedOut] = useState(false);
  const [acceptedPreparationObservationActive, setAcceptedPreparationObservationActive] = useState(false);
  const [acceptedPreparationObservationGeneration, setAcceptedPreparationObservationGeneration] = useState(0);
  const observe = (next: DefinitionAuthoringView) => setView(current => current ? mergeAuthoringObservation(current, next) : next);
  const close = (force = false) => {
    if (!force && (edited || busy || acceptedPreparationObservationActive || recovery.current.pendingSave || recovery.current.pendingConfirmation || ['confirming', 'admitting'].includes(view?.state || ''))) {
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
    recovery.current = {};
    setView(undefined);
    setEdited(false);
    setAcceptedPreparationObservationActive(false);
    setAcceptedPreparationObservationTimedOut(false);
    acceptedPreparationObservation.current = new AuthoringAcceptedPreparationObservation();
    setBusy(false);
    setFailure('');
    form.resetFields();
    onClose();
  };
  const persist = (next: Recovery) => { recovery.current = { ...next, workspaceId }; };
  const apply = (next: DefinitionAuthoringView) => {
    observe(next);
    if (next.input) form.setFieldsValue({ templateId: next.templateId, goalText: next.input.goalText, requirements: next.input.acceptanceRequirements?.join('\n'), resources: next.input.resourceRefs?.join('\n') });
    setEdited(false);
  };

  useEffect(() => {
    if (!open || !workspaceId) return;
    const token = session.current.begin();
    queries.current.invalidate();
    recovery.current = { workspaceId };
    setView(undefined);
    setEdited(false);
    setBusy(false);
    setFailure('');
    acceptedPreparationObservation.current = new AuthoringAcceptedPreparationObservation();
    setAcceptedPreparationObservationTimedOut(false);
    setAcceptedPreparationObservationActive(false);
    form.resetFields();
    const openingEditVersion = editVersion.current;
    let disposed = false;
    workorchApi.listWorkspaceTemplates().then(items => { if (!disposed && session.current.matches(token)) setTemplates(items); }).catch(error => { if (!disposed && session.current.matches(token)) setFailure(errorMessage(error)); });
    const query = queries.current.begin();
    queries.current.read(() => workorchApi.readDefinitionAuthoring(workspaceId), () => !disposed && session.current.matches(token) && queries.current.matches(query)).then(next => {
      if (disposed || !session.current.matches(token) || !queries.current.matches(query)) return;
      if (!next.input) { setFailure('该工作空间没有持久创作草稿；不能通过旧 CRUD 补写正式定义'); return; }
      if (recovery.current.pendingSave || recovery.current.pendingConfirmation) {
        if (recovery.current.workspaceId === workspaceId) observe(next);
      }
      else if (editVersion.current === openingEditVersion) {
        const serverConfirmation = authoringConfirmationRecovery(next, workspaceId);
        if (serverConfirmation) recovery.current = { workspaceId, pendingConfirmation: serverConfirmation };
        apply(next);
      }
    }).catch(error => { if (!disposed && session.current.matches(token) && queries.current.matches(query)) setFailure(errorMessage(error)); });
    return () => { disposed = true; session.current.begin(); };
  }, [open, workspaceId]);

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

  const generate = async () => {
    const token = session.current.current();
    if (!view) return;
    try { authoringRequireNoPendingConfirmation(recovery.current); }
    catch (error) { setFailure(errorMessage(error)); return; }
    if (serverRecoveryInvalid) { setFailure('服务端确认恢复参数不合法或未匹配，sealed 状态禁止编辑或确认'); return; }
    queries.current.invalidate();
    acceptedPreparationObservation.current = new AuthoringAcceptedPreparationObservation();
    setAcceptedPreparationObservationActive(false);
    setAcceptedPreparationObservationTimedOut(false);
    setBusy(true); setFailure('');
    try {
      const values = await form.validateFields();
      if (!session.current.matches(token)) return;
      let current = view;
      const pending = recovery.current.pendingSave;
      if (pending) {
        current = await workorchApi.saveDefinitionAuthoring(current.workspaceId, pending.input, pending.templateId, pending.expectedWorkspaceRevision, pending.expectedInputHash, pending.requestId);
        if (!session.current.matches(token)) return;
        persist({ workspaceId: view.workspaceId });
        observe(current);
      }
      const input = { goalText: values.goalText, acceptanceRequirements: lines(values.requirements), resourceRefs: lines(values.resources) };
      if (authoringNeedsSave(current, input, values.templateId)) {
        const pendingSave = { requestId: crypto.randomUUID(), input, templateId: values.templateId, expectedWorkspaceRevision: current.workspaceRevision, expectedInputHash: current.inputHash || '' };
        persist({ ...recovery.current, pendingSave });
        current = await workorchApi.saveDefinitionAuthoring(current.workspaceId, input, values.templateId, pendingSave.expectedWorkspaceRevision, pendingSave.expectedInputHash, pendingSave.requestId);
        if (!session.current.matches(token)) return;
      }
      persist({}); apply(current);
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
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };
  const refreshSaveBaseline = () => {
    if (!view || !recovery.current.pendingSave) return;
    const token = session.current.current();
    Modal.confirm({
      title: '读取当前草稿并放弃原保存请求的重放身份？',
      content: '本地输入保留。此操作不会保存或覆盖 owner；请核对当前目标后，再显式提交修订。若原请求已成功，当前草稿会体现其结果。',
      okText: '读取当前草稿',
      cancelText: '保留原请求',
      onOk: async () => {
        if (!session.current.matches(token)) return;
        queries.current.invalidate();
        setBusy(true); setFailure('');
        try {
          const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(view.workspaceId), () => session.current.matches(token));
          if (!session.current.matches(token)) return;
          persist({ workspaceId: view.workspaceId });
          observe(next);
          setEdited(true);
        } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
        finally { if (session.current.matches(token)) setBusy(false); }
      }
    });
  };
  const exact = recovery.current.pendingConfirmation;
  const serverRecoveryInvalid = Boolean(view && ['confirming', 'admitting'].includes(view.state) && !recovery.current.pendingConfirmation && !authoringConfirmationRecovery(view, workspaceId));
  const replayAllowed = authoringCanReplayConfirmation(view, recovery.current);
  const canConfirm = !serverRecoveryInvalid && !recovery.current.pendingSave && Boolean(exact ? replayAllowed : !edited && authoringCanConfirm(view));
  const confirm = async () => {
    const token = session.current.current();
    if (!view || !canConfirm) return;
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
        recovery.current = {}; close(true);
      }
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };
  const enterAdmitted = async () => {
    if (!authoringCanEnterAdmitted(view, workspaceId)) return;
    const token = session.current.current();
    queries.current.invalidate();
    setBusy(true); setFailure('');
    try {
      await onAdmitted();
      if (!session.current.matches(token)) return;
      recovery.current = {}; close(true);
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) setBusy(false); }
  };
  const sealed = ['confirming', 'admitting', 'admitted'].includes(view?.state || '');
  return <Modal open={open} title="持久规格创作／澄清" width={850} onCancel={() => close()} footer={<Space>
    <Button onClick={() => close()}>关闭</Button>
    {authoringCanEnterAdmitted(view, workspaceId) && <Button type="primary" disabled={busy} onClick={enterAdmitted}>恢复已准入工作空间</Button>}
    <Button loading={busy} disabled={serverRecoveryInvalid || busy || !view || sealed || Boolean(exact)} onClick={generate}>保存修订并审查</Button>
    <Button type="primary" loading={busy} disabled={serverRecoveryInvalid || busy || !canConfirm} onClick={confirm}>{exact ? '恢复原确认并准入' : '确认并准入'}</Button>
  </Space>}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="info" message="修改创建期目标、模板、资源或用户验收要求后，必须重新完成六审查；此操作不会启动 Run。同 Run 规格修订不受支持。" />
      {serverRecoveryInvalid && !recovery.current.pendingConfirmation && <Alert type="error" message="服务端确认恢复参数缺失或不匹配；该 sealed 状态禁止编辑和确认，请核对工作空间列表中的服务端状态。" />}
      {failure && <Alert type="error" showIcon message={failure} />}
      {recovery.current.pendingSave && <Alert type="warning" message="原保存请求尚未确认，重试先按原 CAS 身份恢复；冲突不能自动覆盖。" action={<Button disabled={busy} onClick={refreshSaveBaseline}>冲突后读取当前草稿</Button>} />}
      {edited && view?.input && <Alert type="info" message="owner 当前目标（本地编辑尚未覆盖）" description={view.input.goalText} />}
      <Form form={form} layout="vertical" disabled={serverRecoveryInvalid || busy || !view || sealed || Boolean(exact)} onValuesChange={(_, values: Values) => { if (recovery.current.pendingConfirmation || serverRecoveryInvalid) return; editVersion.current++; setEdited(true); persist({ ...recovery.current, values }); }}>
        <Form.Item name="templateId" label="模板" rules={[{ required: true }]}><Select options={templates.map(item => ({ value: item.id, label: item.name || item.id }))} /></Form.Item>
        <Form.Item name="goalText" label="目标" rules={[{ required: true }]}><Input.TextArea rows={4} /></Form.Item>
        <Form.Item name="resources" label="本地资源引用（每行一个）"><Input.TextArea rows={2} /></Form.Item>
        <Form.Item name="requirements" label="额外用户验收要求（每行一条）"><Input.TextArea rows={3} /></Form.Item>
      </Form>
      {view && <Typography.Text>{view.workspaceId} · revision {view.inputRevision} · {view.state}</Typography.Text>}
      {view && !view.templateCurrent && <Alert type="warning" message="模板已变化，请保存修订并重新审查。" />}
      {edited && <Alert type="warning" message="本地输入已修改，旧规格不可确认。" />}
      {view?.admissionViolationCode && <Alert type="warning" message={view.admissionViolationCode} description={view.admissionOffendingRefs?.join('、')} />}
      {view?.preparationDiagnostics?.map(diagnostic => <Alert key={diagnostic.diagnosticId} type="error" showIcon message={`${diagnostic.current ? '当前' : '历史'}前置准备失败 · ${diagnostic.stage}${diagnostic.reviewKind ? ` · ${diagnostic.reviewKind}` : ''} · ${diagnostic.phaseCode}`} description={<Space direction="vertical" style={{ width: '100%' }}><Typography.Text>前置准备失败，尚未为该阶段取得 provider permit；修复后显式重试。此提示不表示其他阶段未调用 provider。</Typography.Text><AuthoringPreparationDiagnosticDetails diagnostic={diagnostic} /></Space>} />)}
      {acceptedPreparationObservationActive && <Alert type="info" showIcon message="本次任务已受理，正在观察 owner 状态；页面观察是临时的，不会恢复或取消服务端事务。" />}
      {acceptedPreparationObservationTimedOut && <Alert type="warning" showIcon message="观察期限已到，最近一次可用 owner 状态为原前置准备失败；无法确认后台任务是否仍在执行。请手动刷新核对或修复后显式重试。" />}
      {view?.requests?.filter(request => request.state === 'failed').map((request, index) => <Alert key={`${request.phase}-${request.reviewKind || index}`} type="error" showIcon message={`规格请求失败：${request.phase}`} description={<AuthoringDiagnosticDetails workspaceId={view.workspaceId} permitId={request.permitId} phase={request.phase} failureCode={request.failureCode} diagnostic={request.diagnostic} />} />)}
      {view?.state === 'failed' && <AuthoringFailedRecoveryNotice />}
      {view?.state === 'outcome_unresolved' && <Alert type="warning" message="持久请求结果未决，不会自动重发 provider；可查询恢复或修订输入。" />}
      <List dataSource={view?.proposal?.verifications || []} renderItem={item => <List.Item>{item.id} · {item.status} · {item.declaration}</List.Item>} />
      <List dataSource={view?.proposal?.items || []} renderItem={item => <List.Item><Space><Tag>{authoringSourceLabel(item.sourceKind)}</Tag>{item.text}</Space></List.Item>} />
      <List dataSource={view?.proposal?.reviews || []} renderItem={item => <List.Item><Space><Tag color={item.status === 'PASS' ? 'green' : 'orange'}>{item.status}</Tag>{item.kind}：{item.reason}</Space></List.Item>} />
    </Space>
  </Modal>;
}
