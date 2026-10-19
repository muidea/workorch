import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Checkbox, Descriptions, Modal, Space } from 'antd';
import { workorchApi } from '../../api';
import type { DefinitionAuthoringView } from '../../types';
import { errorMessage } from '../../utils/errorMessage';
import { AuthoringQueryFence, AuthoringSessionFence, authoringCanConfirm, authoringCanEnterAdmitted, authoringCanRecoverExact, authoringConfirmationRecovery, authoringTicketDeliveryRecovery, mergeAuthoringObservation } from '../../workspaceAuthoring';
import type { AuthoringConfirmation } from '../../workspaceAuthoring';
import { AuthoringCandidateProposal } from './AuthoringCandidateProposal';

// STEP03 never creates/edits/compiles a draft or activates execution. Only an
// explicit click may send the exact five-field confirmation intent.
export function WorkspaceAuthoringConfirmation({ workspaceId, onClose }: { workspaceId?: string; onClose: () => void }) {
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [pending, setPending] = useState<AuthoringConfirmation>();
  const [reviewed, setReviewed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState('');
  const session = useRef(new AuthoringSessionFence());
  const queries = useRef(new AuthoringQueryFence());
  const observed = useRef<DefinitionAuthoringView | undefined>(undefined);
  const intent = useRef<AuthoringConfirmation | undefined>(undefined);
  const inFlight = useRef(false);
  const observe = (next: DefinitionAuthoringView, id: string) => {
    if (next.workspaceId !== id) throw new Error('确认回读 Workspace 身份不匹配，保持只读');
    const merged = mergeAuthoringObservation(observed.current, next);
    if (merged !== next) return; // no recovery/checkbox side effects from stale GET
    const previous = observed.current;
    if (!previous || previous.workspaceRevision !== next.workspaceRevision || previous.inputHash !== next.inputHash || previous.proposal?.proposalHash !== next.proposal?.proposalHash || previous.proposal?.specificationHash !== next.proposal?.specificationHash) setReviewed(false);
    observed.current = next;
    setView(next);
    // An in-memory uncertain intent is never replaced with a newer tuple.
    if (!intent.current && next.state !== 'ready_to_confirm') {
      const recovery = authoringConfirmationRecovery(next, id) || authoringTicketDeliveryRecovery(next, id);
      if (recovery && recovery.expectedWorkspaceRevision === next.workspaceRevision) { intent.current = recovery; setPending(recovery); }
    }
  };
  const read = async (id: string, token: number) => {
    const query = queries.current.begin();
    const next = await queries.current.read(() => workorchApi.readDefinitionAuthoring(id), () => session.current.matches(token) && queries.current.matches(query));
    if (session.current.matches(token) && queries.current.matches(query)) observe(next, id);
  };
  useEffect(() => {
    const token = session.current.begin();
    queries.current.invalidate(); observed.current = undefined; intent.current = undefined; inFlight.current = false;
    setView(undefined); setPending(undefined); setReviewed(false); setBusy(false); setFailure('');
    if (workspaceId) void read(workspaceId, token).catch(error => { if (session.current.matches(token)) setFailure(errorMessage(error)); });
    return () => { session.current.begin(); queries.current.invalidate(); };
  }, [workspaceId]);
  const refresh = async () => {
    if (!workspaceId || inFlight.current) return;
    const token = session.current.current();
    inFlight.current = true; setBusy(true); setFailure('');
    try { await read(workspaceId, token); }
    catch (error) { if (session.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (session.current.matches(token)) { inFlight.current = false; setBusy(false); } }
  };
  const intentMatches = !pending || Boolean(view && pending.inputHash === view.inputHash && pending.derivationPermitId === view.derivationPermitId && pending.proposalHash === view.proposal?.proposalHash && pending.specificationHash === view.proposal?.specificationHash);
  const admitted = intentMatches && authoringCanEnterAdmitted(view, workspaceId);
  const canRecover = Boolean(view && workspaceId && pending && authoringCanRecoverExact(view, workspaceId, pending));
  const canConfirm = Boolean(!pending && workspaceId === view?.workspaceId && Number.isSafeInteger(view?.workspaceRevision) && view!.workspaceRevision > 0 && authoringCanConfirm(view));
  const confirm = async () => {
    if (!view || !workspaceId || !reviewed || inFlight.current || !(pending ? canRecover : canConfirm)) return;
    const token = session.current.current();
    const exact = intent.current || { inputHash: view.inputHash!, derivationPermitId: view.derivationPermitId!, proposalHash: view.proposal!.proposalHash!, specificationHash: view.proposal!.specificationHash!, expectedWorkspaceRevision: view.workspaceRevision };
    intent.current = { ...exact }; setPending({ ...exact });
    inFlight.current = true; setBusy(true); setFailure(''); queries.current.invalidate();
    try {
      const next = await workorchApi.confirmDefinitionAuthoring(view, exact);
      if (session.current.matches(token)) observe(next, workspaceId);
    } catch (error) {
      if (session.current.matches(token)) setFailure(`${errorMessage(error)}；结果尚待 owner 回读核对，不能推断无写入。`);
    } finally {
      // Read back after every outcome, including 409 and lost responses. GET
      // cannot advance confirmation and never changes the frozen POST tuple.
      if (session.current.matches(token)) {
        try { await read(workspaceId, token); }
        catch (error) { if (session.current.matches(token)) setFailure(current => `${current} 回读失败：${errorMessage(error)}`); }
        if (session.current.matches(token)) { inFlight.current = false; setBusy(false); }
      }
    }
  };
  const close = () => { session.current.begin(); queries.current.invalidate(); onClose(); };
  return <Modal open={Boolean(workspaceId)} title="精确确认与合同封存（STEP03）" width={960} onCancel={close} footer={<Space>
    <Button onClick={close}>关闭</Button><Button disabled={busy} onClick={() => void refresh()}>只读刷新确认结果</Button>
    <Button type="primary" loading={busy} disabled={busy || !reviewed || admitted || !(pending ? canRecover : canConfirm)} onClick={() => void confirm()}>{pending ? '显式恢复原确认／票据交付' : '确认当前精确合同并准入'}</Button>
  </Space>}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="info" message="开页、刷新和关闭只读；确认不启动 Run。页面内存不持久保存事务身份，重开仅从 owner 恢复。关闭只停止观察，不取消后台事务。" />
      {failure && <Alert type="warning" message={failure} />}
      {view && <>
        <Descriptions column={1} items={[
          { key: 'workspace', label: 'Workspace', children: view.workspaceId },
          { key: 'state', label: 'owner 持久状态', children: view.state },
          { key: 'revision', label: 'Workspace revision', children: view.workspaceRevision },
          { key: 'input', label: 'inputHash', children: view.inputHash || '-' },
          { key: 'permit', label: 'derivationPermitId', children: view.derivationPermitId || '-' },
          { key: 'proposal', label: 'proposalHash（六审绑定）', children: view.proposal?.proposalHash || '-' },
          { key: 'specification', label: 'specificationHash（规范绑定）', children: view.proposal?.specificationHash || '-' },
          { key: 'phase', label: '确认物化阶段', children: view.confirmationPhase || '尚未物化' },
          { key: 'candidate', label: '准入 candidateHash', children: view.candidateHash || '-' },
          { key: 'activation', label: '准入状态', children: view.activationState || view.admissionDisposition || '-' },
          { key: 'startup', label: '封存 startupInputHash', children: view.startupInputHash || '-' },
        ]} />
        <pre style={{ whiteSpace: 'pre-wrap' }} aria-label="已保存 Goal 原文">{view.input?.goalText}</pre>
        {view.proposal && <AuthoringCandidateProposal proposal={view.proposal} />}
        {admitted && <Alert type="success" message="合同已确认、封存且 eligible；尚无 Run。本阶段不表示当前运行资源检查或完整 M3 已完成。" />}
        {['confirming', 'admitting'].includes(view.state) && <Alert type="info" message="确认／准入尚在进行或等待恢复；可只读刷新，恢复必须显式使用原意图。" />}
        {view.activationState === 'pending_clarification_resolution' && <Alert type="warning" message="已有封存事实，但仍等待精确澄清 resolution；不是 eligible。" />}
        {view.state === 'clarification_required' && <Alert type="warning" message={`准入阻塞：${view.admissionViolationCode || '未提供原因'}`} description={<><div>offending refs：{view.admissionOffendingRefs?.join('、') || '-'}</div><div>{view.blockingTicketId ? `已验证 ticket：${view.blockingTicketId}` : '票据交付未完成／尚未验证；只读查询不会补交。'}</div><div>已完成物化不会回滚；恢复票据不改约束、不解决票据，也不授予 eligible。</div></>} />}
        {!admitted && !(pending ? canRecover : canConfirm) && <Alert type="warning" message="当前事实或精确恢复身份不满足操作条件，保持只读阻塞；不会自动确认最新版本。" />}
        {pending && <pre aria-label="冻结的原确认意图">{JSON.stringify(pending, null, 2)}</pre>}
        <Checkbox checked={reviewed} disabled={busy || admitted || !(pending ? canRecover : canConfirm)} onChange={event => setReviewed(event.target.checked)}>我已审阅以上原文、required／all_of、来源、义务、六审及 DECLARED（仅能力声明，非运行证据），确认此精确版本</Checkbox>
      </>}
    </Space>
  </Modal>;
}
