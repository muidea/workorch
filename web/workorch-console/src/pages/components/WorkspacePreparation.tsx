import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Descriptions, List, Modal, Space, Tag, Typography } from 'antd';
import { workorchApi } from '../../api';
import { mcpPreparationReasonLabel, providerPreparationReasonLabel, validatePreparationView, validatePreparationCompilation } from '../../startupPreparation';
import type { PreparationBootstrapCompilation, PreparationCheckRequest, StartupPreparationView } from '../../startupPreparation';

export function WorkspacePreparation({ workspaceId, onClose }: { workspaceId?: string; onClose: () => void }) {
  return <Modal title="激活前准备（STEP04）" open={Boolean(workspaceId)} onCancel={onClose} footer={null} width={900} destroyOnHidden>
    {workspaceId ? <PreparationPanel key={workspaceId} workspaceId={workspaceId} /> : null}
  </Modal>;
}

function PreparationPanel({ workspaceId }: { workspaceId: string }) {
  const [view, setView] = useState<StartupPreparationView>();
  const [pending, setPending] = useState<PreparationCheckRequest>();
  const [pendingInitialization, setPendingInitialization] = useState<PreparationCheckRequest>();
  const initializationIntent = useRef<PreparationCheckRequest | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [compilation, setCompilation] = useState<{ requestId: string; value: PreparationBootstrapCompilation }>();
  const alive = useRef(true);
  const inflight = useRef(false);
  const serial = useRef(0);
  const intent = useRef<PreparationCheckRequest | undefined>(undefined);

  const accept = (next: StartupPreparationView) => {
    validatePreparationView(next, workspaceId);
    const original = intent.current;
    const a = original && next.record?.attempts.find(row => row.requestId === original.requestId);
    if (a && a.expectedWorkspaceRevision !== original?.expectedWorkspaceRevision) throw new Error('原准备请求绑定不匹配');
    const initOriginal = initializationIntent.current;
    const init = initOriginal && next.resourceInitializations?.find(r => r.requestId === initOriginal.requestId);
    if (init && init.expectedWorkspaceRevision !== initOriginal?.expectedWorkspaceRevision) throw new Error('原初始化请求绑定不匹配');
    if (init?.state === 'completed') { initializationIntent.current = undefined; setPendingInitialization(undefined); }
    if (!initializationIntent.current) {
      const lastInit = next.resourceInitializations?.at(-1);
      if (lastInit && lastInit.state !== 'completed') { const restored = { requestId: lastInit.requestId, expectedWorkspaceRevision: lastInit.expectedWorkspaceRevision }; initializationIntent.current = restored; setPendingInitialization(restored); }
    }
    setView(next);
    setCompilation(previous => previous && next.record?.attempts.at(-1)?.requestId === previous.requestId ? previous : undefined);
    if (a && (a.state === 'blocked' || a.state === 'failed')) {
      intent.current = undefined;
      setPending(undefined);
    }
    if (!intent.current) {
      const last = next.record?.attempts.at(-1);
      if (last && (last.state === 'accepted' || last.state === 'unknown')) {
        const recovered = { requestId: last.requestId, expectedWorkspaceRevision: last.expectedWorkspaceRevision };
        intent.current = recovered;
        setPending(recovered);
      }
    }
  };

  const refresh = async () => {
    if (inflight.current) return;
    inflight.current = true;
    setBusy(true);
    const token = ++serial.current;
    try {
      const next = await workorchApi.readStartupPreparation(workspaceId);
      if (alive.current && token === serial.current) { accept(next); setError(''); }
    } catch (reason) {
      if (alive.current && token === serial.current) setError(String(reason));
    } finally {
      if (alive.current && token === serial.current) { inflight.current = false; setBusy(false); }
    }
  };

  useEffect(() => {
    alive.current = true;
    void refresh();
    return () => { alive.current = false; serial.current++; inflight.current = false; };
    // The keyed panel isolates Workspace changes and all late responses.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const submit = async (recoverOriginal: boolean) => {
    if (inflight.current || !view || (!recoverOriginal && intent.current)) return;
    const original = intent.current;
    if (recoverOriginal && !original) return;
    const request = original ?? { requestId: crypto.randomUUID(), expectedWorkspaceRevision: view.workspaceRevision };
    intent.current = request;
    setPending(request);
    inflight.current = true;
    setBusy(true);
    setError('');
    setCompilation(undefined);
    const token = ++serial.current;
    try {
      const result = await (recoverOriginal ? workorchApi.recoverStartupPreparation(workspaceId, request) : workorchApi.checkStartupPreparation(workspaceId, request));
      if (!alive.current || token !== serial.current) return;
      if (result.requestId !== request.requestId || !result.preparation.record?.attempts.some(a => a.requestId === request.requestId && a.expectedWorkspaceRevision === request.expectedWorkspaceRevision)) throw new Error('准备响应未绑定原操作');
      const compiled = validatePreparationCompilation(result, request);
      accept(result.preparation);
      if (compiled) setCompilation({ requestId: request.requestId, value: compiled });
    } catch (reason) {
      if (alive.current && token === serial.current) setError(`${String(reason)}；保留原请求，请只读对账或显式恢复，不要换请求号重试。`);
    } finally {
      // Always reconcile after POST, including partial persistence + 400/409.
      // GET never retries a check or clears a missing/unknown original intent.
      if (alive.current && token === serial.current) {
        try {
          const next = await workorchApi.readStartupPreparation(workspaceId);
          if (alive.current && token === serial.current) accept(next);
        } catch (reason) {
          if (alive.current && token === serial.current) setError(`原操作读回失败：${String(reason)}；请保留原请求身份。`);
        }
        if (alive.current && token === serial.current) { inflight.current = false; setBusy(false); }
      }
    }
  };

  const initialize = async (recoverOriginal: boolean) => {
    if (inflight.current || !view || pending || (!recoverOriginal && initializationIntent.current)) return;
    const original = initializationIntent.current;
    if (recoverOriginal && !original) return;
    const request = original ?? { requestId: crypto.randomUUID(), expectedWorkspaceRevision: view.workspaceRevision };
    initializationIntent.current = request;
    setPendingInitialization(request);
    inflight.current = true; setBusy(true); setError(''); setCompilation(undefined);
    const token = ++serial.current;
    try {
      const result = await (recoverOriginal ? workorchApi.recoverStartupPreparationResources(workspaceId, request) : workorchApi.initializeStartupPreparationResources(workspaceId, request));
      if (!alive.current || token !== serial.current) return;
      if (result.requestId !== request.requestId || !result.preparation.resourceInitializations?.some(r => r.requestId === request.requestId && r.expectedWorkspaceRevision === request.expectedWorkspaceRevision)) throw new Error('初始化响应未绑定原操作');
      accept(result.preparation);
    } catch (reason) {
      if (alive.current && token === serial.current) setError(`${String(reason)}；保留原初始化请求，请只读对账或显式恢复。`);
    } finally {
      if (alive.current && token === serial.current) {
        try { const next = await workorchApi.readStartupPreparation(workspaceId); if (alive.current && token === serial.current) accept(next); }
        catch (reason) { if (alive.current && token === serial.current) setError(`初始化读回失败：${String(reason)}；保留原请求身份。`); }
        if (alive.current && token === serial.current) { inflight.current = false; setBusy(false); }
      }
    }
  };
  const last = view?.record?.attempts.at(-1);
  const lastInitialization = view?.resourceInitializations?.at(-1);
  const registrationStage = Boolean(lastInitialization?.registrationState && lastInitialization.registrationState !== 'waiting_for_files');
  const initializationBudget = registrationStage ? (lastInitialization?.registrationAttempts ?? 0) : (lastInitialization?.attempts ?? 0);
  return <Space direction="vertical" style={{ width: '100%' }}>
    <Alert type="info" showIcon message="准备检查仍保持阻塞，不授予 ready、M3 或执行权限。" description="开页和刷新仅读已有记录；显式新检查会在服务端先封存 SkillHub status 目标，再按 global／project 执行，可能保存 SkillHub 状态或访问其已配置服务，不执行 install、sync、use、apply 或 repair。不调用模型、不创建 Run。status 历史记录不等于 Skill 就绪；效果未决时禁止新请求绕过，恢复不重跑观察。原始材料由服务端受限保存，仅公开随机历史引用。未保存的页面交互在关闭或刷新后丢失，已受理操作由服务端保存。" />
    {error ? <Alert type="error" message={error} /> : null}
    {view?.skillHubProjectIssue === 'project_registration_required' ? <Alert type="warning" showIcon message="SkillHub 项目尚未登记" description="原检查收到 PROJECT_NOT_FOUND，不能将其视为空技能库。仅创建 .agents/skills 目录不足以完成 SkillHub 登记；需通过受支持的显式项目登记流程处理。原效果未决记录仍保留，刷新或升级不会解除，也不会自动重跑 status。" /> : null}
    <Space wrap>
      <Button disabled={busy || !view || Boolean(pending) || Boolean(pendingInitialization) || view.record?.attempts.some(a => a.skillHubEffectState === 'reserved' || a.skillHubEffectState === 'unresolved')} onClick={() => void initialize(false)}>显式初始化准备资源</Button>
      <Button disabled={busy || Boolean(pending) || !pendingInitialization || initializationBudget >= 4 || lastInitialization?.registrationState === 'failed'} onClick={() => void initialize(true)}>显式恢复资源初始化</Button>
      <Button disabled={busy} onClick={() => void refresh()}>只读刷新准备结果</Button>
      <Button disabled={busy || !view || Boolean(pending) || Boolean(pendingInitialization) || view.record?.attempts.some(a => a.skillHubEffectState === 'reserved' || a.skillHubEffectState === 'unresolved')} onClick={() => void submit(false)}>显式检查准备</Button>
      <Button disabled={busy || !pending} onClick={() => void submit(true)}>显式恢复原准备操作</Button>
    </Space>
    <Alert type="info" message="资源初始化创建缺失的托管配置和项目 .agents/skills 目录，并显式登记 SkillHub 空项目，保留已有内容及技能。登记回复丢失后，恢复仅查询原请求回执，不重新登记；不安装或同步 Skill。完成后请显式检查准备；初始化不授予 ready、M3 或执行权限。" />
    {lastInitialization ? <Typography.Paragraph>最近资源初始化：{lastInitialization.requestId}；{lastInitialization.state}；File 已消耗 {lastInitialization.attempts} 次派发预算。{lastInitialization.registrationState ? `SkillHub 登记：${lastInitialization.registrationState}；已消耗 ${lastInitialization.registrationAttempts ?? 0} 次登记／查询预算。` : ''}{lastInitialization.state === 'completed' ? '基础设施创建完成，资源就绪仍须检查。' : lastInitialization.registrationState === 'failed' ? '登记已确定失败，请核对服务端原始材料；不会重发登记。' : initializationBudget >= 4 ? '已达派发次数上限，请核对服务端原始材料。' : '原操作未完成，请保留原身份并显式恢复；已有冲突不会被覆盖。'}</Typography.Paragraph> : null}
    {pendingInitialization ? <Typography.Paragraph>待对账初始化请求：{pendingInitialization.requestId}；原 revision：{pendingInitialization.expectedWorkspaceRevision}</Typography.Paragraph> : null}
    {pending ? <Typography.Paragraph>待对账原请求：{pending.requestId}；原 revision：{pending.expectedWorkspaceRevision}</Typography.Paragraph> : null}
    {compilation ? <Alert type="info" message={compilation.value.failureCode ? `完整结构编译受阻：${compilation.value.failureCode}` : '完整 PreparedBootstrap 结构编译已复现；不是 ready、M3 或执行授权'} description={`原请求 ${compilation.requestId}；${compilation.value.bootstrapHash || '未产生 bootstrap'}。仅本次显式检查／恢复从服务端原归档编译；刷新页面不补造该结果，当前资源有效性仍须独立核验。`} /> : null}
    {view ? <Descriptions column={1} size="small" items={[
      { key: 'revision', label: 'Workspace revision', children: view.workspaceRevision },
      { key: 'state', label: '操作状态', children: view.operationState },
      { key: 'validity', label: '当前有效性', children: view.currentValidity },
      { key: 'run', label: '独立 Run 投影', children: view.activeRunId || '未投影 ActiveRunID（不证明无历史或在途 Run）' },
      { key: 'm3', label: 'M3', children: '本流程不签发 M3 完成凭据' },
      { key: 'request', label: '最近检查请求', children: last?.requestId || '未检查' },
      { key: 'all-of', label: '整体 required 检查', children: last?.assessment ? `原检查通过 ${last.assessment.passedRequiredCount}/${last.assessment.requiredCount}；${last.assessment.outcome === 'blocked' ? `仍有 ${last.assessment.outstanding.length} 项未通过` : '原 required 检查均通过；仍不等于当前 ready 或 M3'}` : '未保存整体裁决（不从旧单项记录补造）' },
      { key: 'definition-compile', label: '封存来源纯编译', children: last?.definitionCompilation ? `已编译 ${last.definitionCompilation.sourceCount} 个来源、${last.definitionCompilation.obligationCount} 个义务；非完整 bootstrap，不代表 ready 或 M3` : '未保存编译摘要（不回填旧记录）' },
      { key: 'execution-inputs', label: '原执行策略输入', children: last?.executionInputVersion ? '已归档原模板、Workspace 策略、对话语言与修复上限；非完整 bootstrap，不代表 ready 或 M3' : '未保存原输入（不借用当前配置补造）' },
      { key: 'evaluation', label: '逐项裁决', children: last?.evaluationVersion ? '原检查时点的单项结论；passed 不代表当前仍有效、整体 ready 或 M3' : '原版阻塞记录（不回填单项通过）' },
      { key: 'directory-evidence', label: '目录历史证据引用', children: last?.directoryEvidenceHash || '未保存（不可视为目录通过）' },
      { key: 'resource-evidence', label: '规格资源历史证据引用', children: last?.resourceEvidenceHash || '未保存（不可视为资源通过）' },
      { key: 'managed-config-evidence', label: '托管配置历史证据引用', children: last?.managedConfigEvidenceRef || '未保存（不可视为配置通过）' },
      { key: 'soul-evidence', label: 'Soul 历史证据引用', children: last?.soulEvidenceRef || '未保存完整观察（不可视为 Soul 通过）' },
      { key: 'seal-evidence', label: '精确封存历史证据引用', children: last?.sealEvidenceRef || '未保存完整封存观察；不是整体就绪证明' },
      { key: 'provider-cache-evidence', label: 'Provider 缓存历史证据引用', children: last?.providerCacheEvidenceRef || '未保存完整观察（不可视为 provider 就绪）' },
      { key: 'run-history-evidence', label: 'Run 历史观察引用', children: last?.runHistoryEvidenceRef || '未保存完整历史观察（不可视为无 Run）' },
      { key: 'no-run-final', label: 'no-Run 原结算栅栏', children: last?.noRunEvaluationVersion ? '原 Run 完整历史与当次 Workspace CAS 栅栏联合核验通过；不表示当前无 Run，不授予 M3 或执行权' : '尚未取得原结算栅栏证明；仅空历史不能通过' },
      { key: 'memory-inventory-evidence', label: 'Memory 库存历史证据引用', children: last?.memoryInventoryEvidenceRef || '未保存完整库存（不代表无记忆或已完成选择）' },
      { key: 'mcp-registry-evidence', label: 'MCP Registry 历史证据引用', children: last?.mcpRegistryEvidenceRef || '未保存完整 registry（不代表无需 MCP 或连接就绪）' },
 { key: 'skill-registry-evidence', label: 'Skill Registry 历史证据引用', children: last?.skillRegistryEvidenceRef || '未保存完整目录观察（不可视为 Skill 选择通过）' },
      { key: 'skillhub-effect', label: 'SkillHub status 效果记录', children: last?.skillHubEffectRef ? `${last.skillHubEffectRef} / ${last.skillHubEffectState}（历史操作，不代表就绪）` : '未封存' },
      { key: 'skillhub-installed', label: 'SkillHub 已安装材料', children: last?.skillHubInstalledState === 'observed_selection_pending' ? '原 status 对应的指令双读与元数据已封存；不代表选择通过或就绪' : last?.skillHubInstalledState === 'incomplete' ? '已保存部分观察或失败；材料不完整，不代表就绪' : '未封存安装材料' },
      { key: 'skill-selection', label: 'Skill 组合选择', children: last?.skillSelectionState === 'compiled_checks_pending' ? '完整候选身份与精确选择已封存；freshness 与辅助资源等检查仍待完成，不代表就绪' : last?.skillSelectionState === 'sources_incomplete' ? '组合来源不完整，未产生选择证明' : last?.skillSelectionState === 'conflict' ? '组合来源或候选冲突，未产生选择证明' : '未封存组合选择材料' },
      { key: 'skill-context', label: 'Skill context 预算', children: last?.skillContextState === 'compiled_checks_pending' ? '既有预算合同编译完成；其他资源检查仍待完成，不代表就绪' : last?.skillContextState === 'bounded_checks_pending' ? '已按既有预算生成有界投影；完整原文仍保留，其他检查待完成，不代表就绪' : last?.skillContextState === 'rejected' ? '选中材料未通过既有 context 合同；原材料保留，准备仍受阻' : '未封存 context 预算材料' },
      { key: 'skill-references', label: 'Skill 辅助文件', children: last?.skillReferenceState === 'observed_checks_pending' ? '声明的辅助文件已按安装来源双读（可为空声明）；状态及持续有效性仍待核验，不代表就绪' : last?.skillReferenceState === 'source_unresolved' ? '选中 Registry 技能的辅助文件缺少唯一根目录来源；未猜测路径' : last?.skillReferenceState === 'unavailable' ? '辅助文件缺失、越界、漂移或读取不完整；原观察已保留，准备仍受阻' : last?.skillReferenceState === 'sources_incomplete' ? '辅助文件观察来源不完整；未补造证明' : '未封存辅助文件观察' },
      { key: 'skill-status', label: 'Skill 来源状态', children: last?.skillStatusState === 'observed_freshness_pending' ? '原选中来源已核对；synced 及 Registry 观察不代表持续有效或就绪' : last?.skillStatusState === 'status_blocked' ? '选中 SkillHub 技能未报告精确 synced 状态；原状态保留，未自动修复' : last?.skillStatusState === 'version_conflict' ? '真实安装指令版本与 status 本地版本冲突；未采用仓库版本替代' : last?.skillStatusState === 'sources_incomplete' ? '选中 Skill 的原状态或安装来源不完整；未补造证明' : '未封存来源状态诊断' },
    ]} /> : null}
    <List dataSource={last?.checks ?? []} locale={{ emptyText: '尚无逐项检查结果' }} renderItem={check => <List.Item><Space wrap><Typography.Text>{check.id}</Typography.Text><Tag>{check.owner}</Tag><Tag>{check.required ? 'required' : 'optional'}</Tag><Tag color={check.state === 'failed' ? 'error' : check.state === 'passed' ? 'blue' : 'warning'}>{check.state}</Tag><Typography.Text>{check.reason}</Typography.Text>{check.id === 'mcp_selection' && mcpPreparationReasonLabel(check.reason) && <Typography.Text type="secondary">{mcpPreparationReasonLabel(check.reason)}</Typography.Text>}{check.id === 'provider_cache' && providerPreparationReasonLabel(check.reason) && <Typography.Text type="secondary">{providerPreparationReasonLabel(check.reason)}</Typography.Text>}</Space></List.Item>} />
  </Space>;
}
