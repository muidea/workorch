import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Descriptions, Modal, Space, Typography } from 'antd';
import { workorchApi } from '../../api';
import { WorkorchRequestError } from '../../creationRecovery';
import type { DefinitionAuthoringView } from '../../types';
import { errorMessage } from '../../utils/errorMessage';
import { AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS, AuthoringAcceptedPreparationObservation, AuthoringQueryFence, AuthoringSessionFence, authoringNeedsPolling, mergeAuthoringObservation } from '../../workspaceAuthoring';
import { AuthoringCandidateProposal } from './AuthoringCandidateProposal';
import { AuthoringDiagnosticDetails } from './AuthoringDiagnosticDetails';
import { AuthoringPreparationFailure } from './AuthoringPreparationFailure';

// STEP02: only persisted-input queries and explicit, identity-bound compilation.
// No local input editor, create/PUT/confirm/admission/execution path lives here.
export function WorkspaceAuthoringCandidate({ workspaceId, onClose }: { workspaceId?: string; onClose: () => void }) {
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState('');
  const [accepted, setAccepted] = useState(false);
  const [observingPreparation, setObservingPreparation] = useState(false);
  const [observationExpired, setObservationExpired] = useState(false);
  const session = useRef(new AuthoringSessionFence());
  const queries = useRef(new AuthoringQueryFence());
  const inFlight = useRef(false);
  const preparation = useRef(new AuthoringAcceptedPreparationObservation());
  const acceptedInput = useRef('');
  const acceptedFromPreparationFailure = useRef(false);
  const observe = (next: DefinitionAuthoringView, id: string) => {
    if (next.workspaceId !== id) throw new Error('候选查询身份不匹配，保持待核对');
    const status = preparation.current.observe(next);
    // 首次生成的新准备失败已是 owner 结果；只有从旧失败显式恢复时才等待新诊断。
    const firstPreparationFailure = next.state === 'preparation_failed' && !acceptedFromPreparationFailure.current;
    if (status.freshLedger || firstPreparationFailure || acceptedInput.current && acceptedInput.current !== next.inputHash || !['draft', 'preparation_failed'].includes(next.state)) setAccepted(false);
    setObservingPreparation(status.active);
    setObservationExpired(status.exhausted);
    setView(current => mergeAuthoringObservation(current, next));
  };
  useEffect(() => {
    const token = session.current.begin();
    queries.current.invalidate();
    preparation.current = new AuthoringAcceptedPreparationObservation();
    acceptedInput.current = '';
    acceptedFromPreparationFailure.current = false;
    inFlight.current = false;
    setView(undefined); setFailure(''); setAccepted(false); setBusy(false);
    setObservingPreparation(false); setObservationExpired(false);
    if (workspaceId) {
      const query = queries.current.begin();
      queries.current.read(() => workorchApi.readDefinitionAuthoring(workspaceId), () => session.current.matches(token) && queries.current.matches(query))
        .then(next => { if (session.current.matches(token) && queries.current.matches(query)) observe(next, workspaceId); })
        .catch(error => { if (session.current.matches(token) && queries.current.matches(query)) setFailure(errorMessage(error)); });
    }
    return () => { session.current.begin(); queries.current.invalidate(); };
  }, [workspaceId]);

  useEffect(() => {
    if (!observingPreparation) return;
    const token = session.current.current();
    const timer = window.setTimeout(() => {
      if (!session.current.matches(token) || !preparation.current.expire()) return;
      queries.current.invalidate(); setObservingPreparation(false); setObservationExpired(true);
    }, AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS);
    return () => window.clearTimeout(timer);
  }, [observingPreparation]);

  useEffect(() => {
    if (!workspaceId || !view || busy || !authoringNeedsPolling(view, observingPreparation)) return;
    const token = session.current.current();
    let disposed = false;
    let timer: number;
    const poll = async () => {
      const query = queries.current.begin();
      try {
        const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(workspaceId), () => !disposed && session.current.matches(token) && queries.current.matches(query));
        if (!disposed && session.current.matches(token) && queries.current.matches(query)) observe(next, workspaceId);
      } catch (error) { if (!disposed && session.current.matches(token) && queries.current.matches(query)) setFailure(errorMessage(error)); }
      finally { if (!disposed && session.current.matches(token) && queries.current.matches(query)) timer = window.setTimeout(poll, 1500); }
    };
    timer = window.setTimeout(poll, 1500);
    return () => { disposed = true; window.clearTimeout(timer); };
  }, [workspaceId, busy, view?.state, view?.inputHash, view?.compilationAccepted, observingPreparation]);

  const refresh = async () => {
    if (!workspaceId || inFlight.current) return;
    const token = session.current.current();
    const query = queries.current.begin();
    inFlight.current = true; setBusy(true); setFailure('');
    try {
      const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(workspaceId), () => session.current.matches(token) && queries.current.matches(query));
      if (session.current.matches(token) && queries.current.matches(query)) observe(next, workspaceId);
    } catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) { inFlight.current = false; setBusy(false); } }
  };

  const canGenerate = Boolean(view?.input && view.inputHash && view.templateSnapshotHash && view.templateCurrent && ['draft', 'derived', 'reviewing', 'preparation_failed'].includes(view.state) && !accepted && !busy);
  const generate = async () => {
    if (!workspaceId || !view || !canGenerate || inFlight.current) return;
    const token = session.current.current();
    const baseline = view;
    inFlight.current = true; setBusy(true); setFailure(''); setObservationExpired(false);
    queries.current.invalidate();
    try {
      const next = await workorchApi.compileDefinitionAuthoring(workspaceId, { expectedInputHash: baseline.inputHash!, expectedTemplateSnapshotHash: baseline.templateSnapshotHash! });
      if (!session.current.matches(token)) return;
      if (next.inputHash !== baseline.inputHash || next.templateSnapshotHash !== baseline.templateSnapshotHash) throw new Error('生成回执与已查看输入不匹配，请只读核对服务端；不会自动生成新版本');
      setAccepted(Boolean(next.compilationAccepted));
      acceptedInput.current = baseline.inputHash || '';
      acceptedFromPreparationFailure.current = baseline.state === 'preparation_failed';
      if (next.compilationAccepted) preparation.current.begin(baseline);
      observe(next, workspaceId);
    } catch (error) {
      if (!session.current.matches(token)) return;
      setFailure(errorMessage(error));
      // Conflict is a read-back, never an implicit recompile of a newer draft.
      if (error instanceof WorkorchRequestError && error.status === 409) {
        try {
          const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(workspaceId), () => session.current.matches(token));
          if (session.current.matches(token)) observe(next, workspaceId);
        } catch (readError) { if (session.current.matches(token)) setFailure(`${errorMessage(error)}；回读失败：${errorMessage(readError)}`); }
      }
    } finally { if (session.current.matches(token)) { inFlight.current = false; setBusy(false); } }
  };
  const close = () => { session.current.begin(); queries.current.invalidate(); onClose(); };
  return <Modal open={Boolean(workspaceId)} title="合同候选生成与审查（STEP02）" width={960} onCancel={close} footer={<Space>
    <Button onClick={close}>关闭</Button>
    <Button disabled={busy} onClick={() => void refresh()}>只读刷新</Button>
    <Button type="primary" disabled={!canGenerate} loading={busy} onClick={() => void generate()}>{view?.state === 'preparation_failed' ? '显式恢复前置准备' : '生成已保存输入的合同候选'}</Button>
  </Space>}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="info" message="本页只读已保存输入，生成是独立显式动作；不会创建或保存修订、确认合同、准入或启动 Run。关闭／刷新丢弃页面内状态，不取消服务端事务；结果由服务端 owner 保存。" />
      {failure && <Alert type="error" message={failure} description="结果以 owner 回读为准；不会自动重发或换身份。输入冲突时请核对新版本后再显式操作。" />}
      {accepted && <Alert type="info" message="任务已受理，不等于生成成功；下方为 owner 实际持久状态。" />}
      {observingPreparation && <Alert type="info" message="正在观察前置准备恢复；15 秒仅为旧诊断观察窗，不是编译超时。" />}
      {observationExpired && <Alert type="warning" message="前置准备观察窗结束；后台未被取消，未获重发授权。请只读刷新核对。" />}
      {view && <>
        <Descriptions column={1} items={[
          { key: 'workspace', label: 'Workspace', children: view.workspaceId },
          { key: 'input', label: '已保存输入身份', children: `revision ${view.inputRevision || '-'} · ${view.inputHash || '-'}` },
          { key: 'template', label: '模板快照', children: `${view.templateId || '-'} · ${view.templateSnapshotHash || '-'}` },
          { key: 'state', label: 'owner 持久状态', children: view.state },
          { key: 'derivation', label: '派生请求身份', children: `${view.derivationRequestId || '-'} · ${view.derivationPermitId || '-'}` },
          { key: 'binding', label: '配置与资源绑定', children: `${view.providerBindingHash || '-'} · ${view.resourceSnapshotHash || '-'}` },
        ]} />
        {!view.templateCurrent && <Alert type="warning" message="模板已变化或不可用；本步不会隐式保存新 revision，生成已阻塞。" />}
        <Typography.Title level={5}>已保存输入原文</Typography.Title>
        <pre style={{ whiteSpace: 'pre-wrap' }}>{view.input?.goalText}</pre>
        {(view.input?.acceptanceRequirements || []).map((text, i) => <pre key={`requirement-${i}`} style={{ whiteSpace: 'pre-wrap' }}>{text}</pre>)}
        {(view.input?.resourceRefs || []).map((text, i) => <pre key={`resource-${i}`} style={{ whiteSpace: 'pre-wrap' }}>{text}</pre>)}
        {view.state === 'ready_to_confirm' && <Alert type="success" message="候选已达到 ready_to_confirm，仅供审阅；本步不会确认或形成正式合同。" />}
        {view.state === 'review_blocked' && <Alert type="warning" message="候选审查或验证声明存在阻塞，请逐项核对理由与绑定；结构完整不等于通过。" />}
        {view.state === 'failed' && <Alert type="error" message="请求已明确失败，同输入不会自动重试；保留原失败与诊断。" />}
        {view.state === 'outcome_unresolved' && <Alert type="warning" message="已 reserve 请求结果未决，只能查询原事实；不会再次物理调用 provider。" />}
        {view.preparationDiagnostics?.map(item => <AuthoringPreparationFailure key={item.diagnosticId} diagnostic={item} />)}
        {view.requests?.map(item => <div key={item.permitId}>{item.phase} · {item.reviewKind} · {item.state} · request {item.requestId} · permit {item.permitId}<div>配置绑定：{item.providerBindingHash}；proposal：{item.proposalHash || '非审查阶段'}</div>
          {item.state === 'failed' && <AuthoringDiagnosticDetails workspaceId={view.workspaceId} permitId={item.permitId || ''} phase={item.phase} failureCode={item.failureCode} diagnostic={item.diagnostic} />}
        </div>)}
        {view.proposal && <AuthoringCandidateProposal proposal={view.proposal} />}
      </>}
    </Space>
  </Modal>;
}
