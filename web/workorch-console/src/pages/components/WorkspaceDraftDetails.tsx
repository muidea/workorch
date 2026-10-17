import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Descriptions, Modal, Space, Typography } from 'antd';
import { workorchApi } from '../../api';
import { AuthoringSessionFence } from '../../workspaceAuthoring';
import { errorMessage } from '../../utils/errorMessage';
import type { DefinitionAuthoringView } from '../../types';

// Read-only STEP01 discovery. Recovery is an explicit replay, never editing or compilation.
export function WorkspaceDraftDetails({ workspaceId, onClose }: { workspaceId?: string; onClose: () => void }) {
  const [view, setView] = useState<DefinitionAuthoringView>();
  const [failure, setFailure] = useState('');
  const [busy, setBusy] = useState(false);
  const fence = useRef(new AuthoringSessionFence());
  const inFlight = useRef(false);
  useEffect(() => {
    const token = fence.current.begin();
    setView(undefined); setFailure(''); setBusy(false); inFlight.current = false;
    if (workspaceId) {
      setBusy(true); inFlight.current = true;
      workorchApi.readDefinitionAuthoring(workspaceId).then(next => {
        if (fence.current.matches(token)) {
          if (next.workspaceId !== workspaceId) throw new Error('草稿读取身份不匹配，保持待核对');
          setView(next);
        }
      }).catch(error => { if (fence.current.matches(token)) setFailure(errorMessage(error)); })
        .finally(() => { if (fence.current.matches(token)) { inFlight.current = false; setBusy(false); } });
    }
    return () => { fence.current.begin(); };
  }, [workspaceId]);
  const readOrRecover = async (recover: boolean) => {
    if (inFlight.current || !workspaceId) return;
    inFlight.current = true;
    const token = fence.current.current();
    setBusy(true); setFailure('');
    try {
      const request = view?.creation?.replay;
      if (recover && (!request || request.requestId !== view?.creation?.requestId)) throw new Error('原创建 payload 无法可靠重建，请保持待核对，不能用新身份重建');
      const next = recover ? await workorchApi.createDefinitionAuthoring(request!) : await workorchApi.readDefinitionAuthoring(workspaceId);
      if (!fence.current.matches(token)) return;
      if (next.workspaceId !== workspaceId || recover && (!next.creation?.directoryPrepared || next.creation.requestId !== request!.requestId)) throw new Error('原创建尚未完成，请保留原身份继续核对');
      setView(next);
    } catch (error) { if (fence.current.matches(token)) setFailure(errorMessage(error)); }
    finally { if (fence.current.matches(token)) { inFlight.current = false; setBusy(false); } }
  };
  return <Modal open={Boolean(workspaceId)} title="服务端草稿详情" onCancel={onClose} width={760} footer={<Space>
    <Button onClick={onClose}>关闭</Button>
    <Button disabled={busy} onClick={() => void readOrRecover(false)}>重新核对服务端</Button>
    {view?.creation && !view.creation.directoryPrepared && <Button disabled={busy || !view.creation.replay} onClick={() => void readOrRecover(true)}>按原身份恢复目录准备</Button>}
  </Space>}>
    {failure && <Alert type="warning" message="结果待核对" description={failure} />}
    {view && <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type={view.creation?.directoryPrepared ? 'info' : 'warning'} message={view.state === 'draft' ? '草稿（M1），不是执行就绪' : `创作状态：${view.state}`} description={view.creation?.directoryPrepared ? '原创建的目录准备已完成；不代表当前资源就绪。本页不会生成合同或启动执行。' : '草稿已保存，但原创建的目录准备未确认完成。GET 不会补目录；仅在可核对原身份与 payload 时显式恢复，否则保持待核对。'} />
      <Descriptions column={1} items={[
        { key: 'id', label: 'Workspace ID', children: view.workspaceId },
        { key: 'title', label: '名称', children: view.title || '-' },
        { key: 'template', label: '模板', children: view.templateId || '-' },
        { key: 'directory', label: '工作目录', children: view.deliverableDirectory || '-' },
        { key: 'request', label: '创建 requestId', children: view.creation?.requestId || '不可确定' },
        { key: 'hash', label: '输入身份', children: `${view.inputRevision || '-'} · ${view.inputHash || '-'}` },
        { key: 'state', label: '创作状态', children: view.state },
      ]} />
      <Typography.Title level={5}>已保存目标</Typography.Title>
      <pre style={{ whiteSpace: 'pre-wrap' }}>{view.input?.goalText}</pre>
      <Typography.Title level={5}>已保存资源引用（未声明可访问）</Typography.Title>
      {(view.input?.resourceRefs || []).map((ref, i) => <pre key={i} style={{ whiteSpace: 'pre-wrap' }}>{ref}</pre>)}
      <Typography.Title level={5}>已保存额外验收要求</Typography.Title>
      {(view.input?.acceptanceRequirements || []).map((text, i) => <pre key={i} style={{ whiteSpace: 'pre-wrap' }}>{text}</pre>)}
    </Space>}
  </Modal>;
}
