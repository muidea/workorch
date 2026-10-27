import { ConfigProvider } from 'antd';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { workorchApi } from '../../src/api';
import { WorkspacePreparation } from '../../src/pages/components/WorkspacePreparation';
import type { PreparationCheckRequest, StartupPreparationView } from '../../src/startupPreparation';
import { validatePreparationCompilation, validatePreparationView } from '../../src/startupPreparation';

vi.mock('../../src/api', () => ({ workorchApi: { readStartupPreparation: vi.fn(), checkStartupPreparation: vi.fn(), recoverStartupPreparation: vi.fn(), submitWorkspaceExecution: vi.fn() } }));
const empty = (id = 'ws'): StartupPreparationView => ({ workspaceId: id, workspaceRevision: 10, operationState: 'not_checked', currentValidity: 'not_checked' });
const observed = (request: PreparationCheckRequest, state: 'accepted' | 'blocked' | 'unknown' = 'blocked', id = 'ws'): StartupPreparationView => ({
  workspaceId: id, workspaceRevision: request.expectedWorkspaceRevision + (state === 'blocked' ? 2 : 1), operationState: state, currentValidity: 'not_evaluated',
  record: { version: 'workorch-preparation-record/v2', attempts: [{ requestId: request.requestId, payloadHash: 'a'.repeat(64), expectedWorkspaceRevision: request.expectedWorkspaceRevision, committedWorkspaceRevision: request.expectedWorkspaceRevision + (state === 'blocked' ? 2 : 1), state, startedAt: '2026-10-22T00:00:00Z', creationEpoch: 'a'.repeat(32), preparationEpoch: 0,
    binding: { startupInputHash: 'startup', snapshotHash: 'snapshot', manifestHash: 'manifest' }, checks: [{ id: 'exact_seal', owner: 'workspace', required: true, state: state === 'blocked' ? 'blocked' : 'pending', reason: state === 'blocked' ? 'check_pipeline_incomplete' : 'not_observed' }] }] },
});
const wrap = (id = 'ws') => <ConfigProvider theme={{ token: { motion: false } }}><WorkspacePreparation workspaceId={id} onClose={vi.fn()} /></ConfigProvider>;
const checkButton = () => screen.getByRole('button', { name: '显式检查准备' }) as HTMLButtonElement;
const recoverButton = () => screen.getByRole('button', { name: '显式恢复原准备操作' }) as HTMLButtonElement;
function deferred<T>() { let resolve!: (v: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return { promise, resolve }; }

describe('STEP04 准备主体流程', () => {
  it('完整结构编译仅展示原显式操作产物，不修改持久检查或授权', async () => {
    const request = { requestId: 'compile-original', expectedWorkspaceRevision: 10 };
    const view = observed(request);
    const a = view.record!.attempts[0];
    a.executionInputVersion = 'workorch-preparation-execution-inputs/v1';
    a.definitionCompilation = { compilerVersion: 'workorch-preparation-definition-compiler/v1', snapshotHash: 'snapshot', manifestHash: 'manifest', sourceInventoryHash: 'inventory', profileContractsHash: 'profile', sourceCount: 2, requirementCount: 2, obligationCount: 1, verificationCount: 1 };
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.sealEvidenceRef = `seal-observation:${'a'.repeat(32)}`;
    a.assessment = { version: 'workorch-preparation-all-of-assessment/v1', manifestHash: 'manifest', outcome: 'blocked', requiredCount: 1, passedRequiredCount: 0, outstanding: [{ ...a.checks![0] }] };
    const result = { requestId: request.requestId, preparation: view, compilation: { version: 'workorch-original-prepared-bootstrap/v1' as const, snapshotHash: 'snapshot', manifestHash: 'manifest', bootstrapHash: `prepared-bootstrap-sha256:${'a'.repeat(64)}` } };
    expect(validatePreparationCompilation(result, request)).toEqual(result.compilation);
    for (const mutate of [
      (v: typeof result) => { v.compilation.snapshotHash = 'other'; },
      (v: typeof result) => { v.compilation.manifestHash = 'other'; },
      (v: typeof result) => { v.requestId = 'other'; },
      (v: typeof result) => { v.compilation.bootstrapHash = 'claimed'; },
      (v: typeof result) => { Object.assign(v.compilation, { ready: true }); },
      (v: typeof result) => { Object.assign(v.compilation, { failureCode: 'provider_declaration_unavailable' }); },
    ]) { const bad = structuredClone(result); mutate(bad); expect(() => validatePreparationCompilation(bad, request)).toThrow(); }
    let ownerView = empty();
    vi.mocked(workorchApi.readStartupPreparation).mockImplementation(async () => ownerView);
    vi.mocked(workorchApi.checkStartupPreparation).mockImplementation(async (_id, original) => {
      ownerView = { ...view, record: { ...view.record!, attempts: [{ ...a, requestId: original.requestId }] } };
      return { ...result, requestId: original.requestId, preparation: ownerView };
    });
    render(wrap()); await act(async () => {});
    await act(async () => { fireEvent.click(checkButton()); });
    expect(screen.getByText('完整 PreparedBootstrap 结构编译已复现；不是 ready、M3 或执行授权')).toBeTruthy();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    cleanup();
    render(wrap()); await act(async () => {});
    expect(screen.queryByText('完整 PreparedBootstrap 结构编译已复现；不是 ready、M3 或执行授权')).toBeNull();
    expect(workorchApi.checkStartupPreparation).toHaveBeenCalledTimes(1);
  });
  it('原执行输入归档独立展示，缺失不补造，版本不能作为 ready 或执行授权', async () => {
    const view = observed({ requestId: 'inputs-original', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.sealEvidenceRef = `seal-observation:${'a'.repeat(32)}`;
    a.executionInputVersion = 'workorch-preparation-execution-inputs/v1';
    expect(() => validatePreparationView(view, 'ws')).not.toThrow();
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('已归档原模板、Workspace 策略、对话语言与修复上限；非完整 bootstrap，不代表 ready 或 M3')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].executionInputVersion = 'other'; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].sealEvidenceRef; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
    const old = structuredClone(view); delete old.record!.attempts[0].executionInputVersion;
    expect(() => validatePreparationView(old, 'ws')).not.toThrow();
  });
  it('来源纯编译与 Provider 阻塞独立展示，不把编译摘要当 bootstrap 或 M3', async () => {
    const view = observed({ requestId: 'compile-original', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.sealEvidenceRef = `seal-observation:${'a'.repeat(32)}`;
    a.checks = [{ id: 'provider_cache', owner: 'llm', required: true, state: 'blocked', reason: 'provider_cache_unavailable' }];
    a.assessment = { version: 'workorch-preparation-all-of-assessment/v1', manifestHash: a.binding.manifestHash, outcome: 'blocked', requiredCount: 1, passedRequiredCount: 0, outstanding: [{ ...a.checks[0] }] };
    a.definitionCompilation = { compilerVersion: 'workorch-preparation-definition-compiler/v1', snapshotHash: a.binding.snapshotHash, manifestHash: a.binding.manifestHash, sourceInventoryHash: 'inventory', profileContractsHash: 'profile', sourceCount: 2, requirementCount: 2, obligationCount: 1, verificationCount: 1 };
    expect(() => validatePreparationView(view, 'ws')).not.toThrow();
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('已编译 2 个来源、1 个义务；非完整 bootstrap，不代表 ready 或 M3')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].definitionCompilation!.manifestHash = 'other'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].definitionCompilation!.sourceCount = 1; },
      (v: StartupPreparationView) => { v.record!.attempts[0].definitionCompilation!.obligationCount = NaN; },
      (v: StartupPreparationView) => { v.record!.attempts[0].definitionCompilation!.compilerVersion = 'other'; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].assessment; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].sealEvidenceRef; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it('展示完整 required 汇总，拒绝遗漏阻塞项或伪造整体成功，不提交执行', async () => {
    const view = observed({ requestId: 'all-of-original', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.assessment = { version: 'workorch-preparation-all-of-assessment/v1', manifestHash: a.binding.manifestHash, outcome: 'blocked', requiredCount: 1, passedRequiredCount: 0, outstanding: [{ id: 'exact_seal', owner: 'workspace', state: 'blocked', reason: 'check_pipeline_incomplete' }] };
    expect(() => validatePreparationView(view, 'ws')).not.toThrow();
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('原检查通过 0/1；仍有 1 项未通过')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].assessment!.outcome = 'checks_passed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].assessment!.outstanding = []; },
      (v: StartupPreparationView) => { v.record!.attempts[0].assessment!.manifestHash = 'other'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].assessment!.passedRequiredCount = 1; },
      (v: StartupPreparationView) => { v.record!.attempts[0].assessment!.outstanding[0].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; v.operationState = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it('空选择通过与整体 blocked、当前 stale、实际 Run 分轴展示，不伪造 M3', async () => {
    const view = observed({ requestId: 'empty-original', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.emptySelectionEvaluationVersion = 'workorch-preparation-empty-selection-evaluation/v1';
    a.skillRegistryEvidenceRef = `skill-registry-observation:${'a'.repeat(32)}`;
    a.soulEvidenceRef = `soul-observation:${'b'.repeat(32)}`;
    a.mcpRegistryEvidenceRef = `mcp-registry-observation:${'c'.repeat(32)}`;
    a.skillHubEffectRef = `skillhub-status:${'d'.repeat(32)}`;
    a.skillHubEffectState = 'recorded'; a.skillHubInstalledState = 'observed_selection_pending';
    a.skillSelectionState = 'compiled_checks_pending'; a.skillContextState = 'compiled_checks_pending';
    a.skillReferenceState = 'observed_checks_pending'; a.skillStatusState = 'observed_freshness_pending';
    a.checks = [
      { id: 'mcp_selection', owner: 'mcpregistry', required: true, state: 'passed', reason: 'mcp_empty_selection_observed_remaining_checks' },
      { id: 'provider_cache', owner: 'llm', required: true, state: 'blocked', reason: 'provider_cache_unavailable' },
      { id: 'skill_selection', owner: 'skill', required: true, state: 'passed', reason: 'skill_empty_selection_observed_remaining_checks' },
    ];
    a.assessment = { version: 'workorch-preparation-all-of-assessment/v1', manifestHash: a.binding.manifestHash, outcome: 'blocked', requiredCount: 3, passedRequiredCount: 2, outstanding: [{ id: 'provider_cache', owner: 'llm', state: 'blocked', reason: 'provider_cache_unavailable' }] };
    view.currentValidity = 'stale'; view.activeRunId = 'actual-run'; view.workspaceRevision++;
    expect(() => validatePreparationView(view, 'ws')).not.toThrow();
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('原检查通过 2/3；仍有 1 项未通过')).toBeTruthy();
    expect(screen.getByText('actual-run')).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    const bad = structuredClone(view); delete bad.record!.attempts[0].emptySelectionEvaluationVersion;
    expect(() => validatePreparationView(bad, 'ws')).toThrow();
  });
  it('原 no-Run 结算栅栏通过仍独立展示历史、stale 和实际 ActiveRun，不自动运行', async () => {
    const view = observed({ requestId: 'final-original', expectedWorkspaceRevision: 10 });
    view.currentValidity = 'stale'; view.activeRunId = 'later-run'; view.workspaceRevision++;
    const a = view.record!.attempts[0];
    a.noRunEvaluationVersion = 'workorch-preparation-no-run-final-fence/v1';
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.runHistoryEvidenceRef = `run-history-observation:${'b'.repeat(32)}`;
    a.sealEvidenceRef = `seal-observation:${'c'.repeat(32)}`;
    a.checks = [
      { id: 'exact_seal', owner: 'workspace', required: true, state: 'passed', reason: 'seal_observed_remaining_checks' },
      { id: 'first_activation_no_run', owner: 'run', required: true, state: 'passed', reason: 'run_history_empty_final_fence_observed' },
      { id: 'persistence_integrity', owner: 'workspace', required: true, state: 'passed', reason: 'persistence_binding_observed_remaining_checks' },
    ];
    expect(() => validatePreparationView(view, 'ws')).not.toThrow();
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('原 Run 完整历史与当次 Workspace CAS 栅栏联合核验通过；不表示当前无 Run，不授予 M3 或执行权')).toBeTruthy();
    expect(screen.getByText('later-run')).toBeTruthy();
    expect(screen.getByText('run_history_empty_final_fence_observed')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].noRunEvaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].noRunEvaluationVersion = 'future'; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].runHistoryEvidenceRef; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].sealEvidenceRef; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].preparationEpoch = 1; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![1].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![1].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![1].reason = 'run_history_empty_fence_not_final'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![1].state = 'blocked'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'blocked'; v.record!.attempts[0].checks![0].reason = 'binding_changed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].committedWorkspaceRevision++; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; v.operationState = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks = []; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it.each([
    ['observed_freshness_pending', '原选中来源已核对；synced 及 Registry 观察不代表持续有效或就绪'],
    ['status_blocked', '选中 SkillHub 技能未报告精确 synced 状态；原状态保留，未自动修复'],
    ['version_conflict', '真实安装指令版本与 status 本地版本冲突；未采用仓库版本替代'],
    ['sources_incomplete', '选中 Skill 的原状态或安装来源不完整；未补造证明'],
  ] as const)('来源状态 %s 独立展示且不授权', async (state, label) => {
    const view = observed({ requestId: 'status-original', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.skillRegistryEvidenceRef = `skill-registry-observation:${'c'.repeat(32)}`;
    a.soulEvidenceRef = `soul-observation:${'d'.repeat(32)}`;
    a.skillHubEffectRef = `skillhub-status:${'a'.repeat(32)}`;
    a.skillHubEffectState = 'recorded'; a.skillHubInstalledState = 'observed_selection_pending';
    a.skillSelectionState = 'compiled_checks_pending'; a.skillStatusState = state;
    a.checks = [{ id: 'skill_selection', owner: 'skill', required: true, state: 'blocked', reason: 'skill_selection_compiled_checks_pending' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(label)).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].skillStatusState = 'ready' as typeof state; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].skillSelectionState; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; v.operationState = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it.each([
    ['observed_checks_pending', '声明的辅助文件已按安装来源双读（可为空声明）；状态及持续有效性仍待核验，不代表就绪'],
    ['source_unresolved', '选中 Registry 技能的辅助文件缺少唯一根目录来源；未猜测路径'],
    ['unavailable', '辅助文件缺失、越界、漂移或读取不完整；原观察已保留，准备仍受阻'],
    ['sources_incomplete', '辅助文件观察来源不完整；未补造证明'],
  ] as const)('辅助文件 %s 独立展示且不授就绪', async (state, label) => {
    const view = observed({ requestId: 'references', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.skillRegistryEvidenceRef = `skill-registry-observation:${'c'.repeat(32)}`;
    a.soulEvidenceRef = `soul-observation:${'d'.repeat(32)}`;
    a.skillHubEffectRef = `skillhub-status:${'a'.repeat(32)}`;
    a.skillHubEffectState = 'recorded'; a.skillHubInstalledState = 'observed_selection_pending';
    a.skillSelectionState = 'compiled_checks_pending'; a.skillReferenceState = state;
    a.checks = [{ id: 'skill_selection', owner: 'skill', required: true, state: 'blocked', reason: 'skill_selection_compiled_checks_pending' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(label)).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].skillReferenceState = 'ready' as typeof state; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].skillSelectionState; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].skillRegistryEvidenceRef; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it.each(['run_history_empty_fence_not_final', 'run_history_present', 'run_history_retired', 'binding_changed'])('展示 Run 历史 %s 原引用但不证明无 Run 或触发新操作', async reason => {
    const view = observed({ requestId: 'history-original', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.runHistoryEvidenceRef = `run-history-observation:${'b'.repeat(32)}`;
    a.checks = [{ id: 'first_activation_no_run', owner: 'run', required: true, state: 'blocked', reason }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(a.runHistoryEvidenceRef)).toBeTruthy();
    expect(screen.getByText(reason)).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].runHistoryEvidenceRef = 'run-history-observation:ready'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks = []; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'passed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'run_history_unavailable'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'unknown'; v.operationState = 'unknown'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it.each(['exact_seal', 'persistence_integrity'])('展示 %s 原时点结论和引用，不自动重取或启动', async id => {
    const view = observed({ requestId: 'seal-original', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.sealEvidenceRef = `seal-observation:${'b'.repeat(32)}`;
    a.checks = [{ id, owner: 'workspace', required: true, state: 'passed', reason: id === 'exact_seal' ? 'seal_observed_remaining_checks' : 'persistence_binding_observed_remaining_checks' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(a.sealEvidenceRef)).toBeTruthy();
    expect(screen.getByText(a.checks[0].reason)).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].sealEvidenceRef; },
      (v: StartupPreparationView) => { v.record!.attempts[0].sealEvidenceRef = 'seal-observation:raw'; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'ready'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it.each([
    ['provider_selection_authorization_pending', '精确模型与能力声明已绑定；尚缺授权、预算及持续有效性证明，不代表就绪'],
    ['provider_selection_budget_observed_authorization_pending', '精确模型容量预算已由 LLM owner 观察；尚缺授权及持续有效性证明，不代表就绪或执行额度'],
    ['provider_selection_budget_unavailable', '精确模型容量预算不可用；未使用默认值或相邻模型'],
    ['provider_selection_budget_source_conflict', '容量预算与原精确模型来源冲突'],
    ['provider_selection_source_binding_conflict', '模型选择来源绑定冲突'],
    ['provider_selection_configuration_source_unavailable', '缺少冻结的模型选择配置来源'],
    ['provider_selection_invalid', '原模型选择无效；不会自动改选模型'],
    ['provider_selection_model_missing', '精确模型不在原缓存中；不会使用相邻模型'],
    ['provider_selection_capability_incomplete', '精确模型能力声明不完整'],
    ['provider_selection_operation_unsupported', '精确模型或 override 不支持 Responses'],
    ['provider_selection_reasoning_unsupported', '原 reasoning 设置没有精确能力声明支持'],
  ])('Provider %s 保留原历史且不授予 ready', async (reason, label) => {
    const view = observed({ requestId: 'provider-selection', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.providerCacheEvidenceRef = `provider-cache-observation:${'b'.repeat(32)}`;
    a.checks = [{ id: 'provider_cache', owner: 'llm', required: true, state: 'blocked', reason }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(reason)).toBeTruthy();
    expect(screen.getByText(label)).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].providerCacheEvidenceRef; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'passed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'provider_selection_ready'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'provider_selection_model_missing:raw-config'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'toString'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it.each([
    ['mcp_selection_empty_policy_pending', '选择为空；尚未证明无需 MCP，不授予 N/A 或就绪'],
    ['mcp_selection_compiled_connection_pending', '选择已编译；尚缺连接及持续有效性证明'],
    ['mcp_selection_source_binding_conflict', '选择来源绑定冲突；未取得连接证明'],
    ['mcp_selection_configuration_source_unavailable', '选择配置来源不完整；未取得连接证明'],
    ['mcp_selection_configuration_declaration_invalid', '选择配置声明无效或有歧义；未取得连接证明'],
    ['mcp_selection_selection_limit', '选择超出数量边界；未取得连接证明'],
    ['mcp_selection_skill_source_incomplete', '完整 Skill 选择来源缺失或受阻；未取得连接证明'],
    ['mcp_selection_skill_requirement_invalid', 'Skill 的 MCP 必需引用无效；未取得连接证明'],
    ['mcp_selection_server_disabled', '所需 MCP server 已禁用；未取得连接证明'],
    ['mcp_selection_server_missing', '所需 MCP server 不存在；未取得连接证明'],
    ['mcp_selection_capability_disabled', '所需 MCP capability 已禁用；未取得连接证明'],
    ['mcp_selection_capability_server_not_selected', '所需 capability 的 server 未被选中；不会自动补选'],
    ['mcp_selection_capability_not_allowed', '所需 capability 不在 server allowlist 内；未取得连接证明'],
    ['mcp_selection_capability_missing', '所需 MCP capability 不存在；未取得连接证明'],
  ])('MCP 精确选择分类 %s 只展示历史阻塞，不自动检查、恢复或执行', async (reason, label) => {
    const view = observed({ requestId: 'mcp-selection', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.mcpRegistryEvidenceRef = `mcp-registry-observation:${'b'.repeat(32)}`;
    a.checks = [{ id: 'mcp_selection', owner: 'mcpregistry', required: true, state: 'blocked', reason }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(reason)).toBeTruthy();
    expect(screen.getByText(label)).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].mcpRegistryEvidenceRef; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'passed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'failed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'mcp_selection_ready'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'mcp_selection_server_missing:raw-config'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'toString'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].id = 'exact_seal'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
    const stale = structuredClone(view); stale.currentValidity = 'stale';
    expect(() => validatePreparationView(stale, 'ws')).not.toThrow();
  });
  it('MCP 完整 registry 引用不替代连接事实，读取不自动检查或执行', async () => {
    const view = observed({ requestId: 'mcp-registry', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.mcpRegistryEvidenceRef = `mcp-registry-observation:${'b'.repeat(32)}`;
    a.checks = [{ id: 'mcp_selection', owner: 'mcpregistry', required: true, state: 'blocked', reason: 'mcp_registry_observed_connection_pending' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(a.mcpRegistryEvidenceRef)).toBeTruthy();
    expect(screen.getByText('mcp_registry_observed_connection_pending')).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].mcpRegistryEvidenceRef; },
      (v: StartupPreparationView) => { v.record!.attempts[0].mcpRegistryEvidenceRef = 'mcp-registry-observation:ready'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'passed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'failed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'mcp_registry_observation_unavailable_not_saved'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
    const stale = structuredClone(view); stale.currentValidity = 'stale'; stale.record!.attempts[0].checks![0].reason = 'binding_changed';
    expect(() => validatePreparationView(stale, 'ws')).not.toThrow();
  });
  it('Memory 原模式单项裁决仅展示历史结果，semantic 或预算受阻不能伪装通过', async () => {
    const view = observed({ requestId: 'memory-evaluation', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.memoryInventoryEvidenceRef = `memory-inventory-observation:${'b'.repeat(32)}`;
    a.checks = [{ id: 'memory_selection', owner: 'memory', required: true, state: 'passed', reason: 'memory_list_selection_observed_remaining_checks' }];
    for (const [state, reason] of [
      ['passed', 'memory_list_selection_observed_remaining_checks'],
      ['passed', 'memory_empty_selection_observed_remaining_checks'],
      ['blocked', 'memory_semantic_evidence_required'],
      ['blocked', 'memory_selection_candidate_limit'],
    ] as const) {
      const current = structuredClone(view);
      Object.assign(current.record!.attempts[0].checks![0], { state, reason });
      vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(current);
      render(wrap()); await act(async () => {});
      expect(screen.getByText(reason)).toBeTruthy();
      expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
      cleanup();
      delete current.record!.attempts[0].memoryInventoryEvidenceRef;
      expect(() => validatePreparationView(current, 'ws')).toThrow();
    }
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'file'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'binding_changed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'original_check_interrupted'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'memory_semantic_evidence_required'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'memory_selection_candidate_limit'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'ready' as typeof a.state; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it('Soul 单项通过仅展示原事实裁决，缺版本或来源、漂移与恢复中断不能通过', async () => {
    const view = observed({ requestId: 'soul-evaluation', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.soulEvidenceRef = `soul-observation:${'d'.repeat(32)}`;
    a.checks = [{ id: 'soul_selection', owner: 'soul', required: true, state: 'passed', reason: 'souls_observed_remaining_checks' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('passed')).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].soulEvidenceRef; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'file'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].required = false; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'binding_changed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'original_check_interrupted'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].state = 'failed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'ready' as typeof a.state; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it('版本化单项通过与整体受阻分轴展示，不提升为 ready 或 M3', async () => {
    const view = observed({ requestId: 'per-check', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.evaluationVersion = 'workorch-preparation-check-evaluation/v1';
    a.directoryEvidenceHash = 'b'.repeat(64);
    a.checks!.unshift({ id: 'directory_boundary', owner: 'file', required: true, state: 'passed', reason: 'directory_observed_remaining_checks' });
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('passed')).toBeTruthy();
    expect(screen.getByText('原检查时点的单项结论；passed 不代表当前仍有效、整体 ready 或 M3')).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { delete v.record!.attempts[0].evaluationVersion; },
      (v: StartupPreparationView) => { v.record!.attempts[0].evaluationVersion = 'future'; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].directoryEvidenceHash; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'binding_changed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![1].state = 'passed'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
    const failed = structuredClone(view);
    failed.record!.attempts[0].checks![0].state = 'failed';
    failed.record!.attempts[0].checks![0].reason = 'directory_permissions_denied';
    expect(() => validatePreparationView(failed, 'ws')).not.toThrow();
  });
	it('Memory 完整库存引用只读展示，不替代选择模式或召回证明', async () => {
	  const view = observed({requestId: 'memory-inventory', expectedWorkspaceRevision: 10});
	  const a = view.record!.attempts[0];
	  a.memoryInventoryEvidenceRef = `memory-inventory-observation:${'a'.repeat(32)}`;
	  a.checks = [{id: 'memory_selection', owner: 'memory', required: true, state: 'blocked', reason: 'memory_inventory_observed_selection_pending'}];
	  vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
	  render(wrap()); await act(async () => {});
	  expect(screen.getByText(a.memoryInventoryEvidenceRef)).toBeTruthy();
	  expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
	  expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
	  for (const mutate of [
	    (v: StartupPreparationView) => { delete v.record!.attempts[0].memoryInventoryEvidenceRef; },
	    (v: StartupPreparationView) => { v.record!.attempts[0].memoryInventoryEvidenceRef = 'ready'; },
	    (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
	    (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'memory_inventory_unavailable_not_saved'; },
	    (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
	    (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
	  ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
	});
  beforeEach(() => { vi.resetAllMocks(); vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(empty()); });
  afterEach(cleanup);
  it('Provider 完整历史引用只读展示，不变为连接就绪或执行许可', async () => {
    const view = observed({ requestId: 'cache', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.providerCacheEvidenceRef = `provider-cache-observation:${'b'.repeat(32)}`;
    a.checks = [{ id: 'provider_cache', owner: 'llm', required: true, state: 'blocked', reason: 'provider_declaration_only' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(a.providerCacheEvidenceRef)).toBeTruthy();
    expect(screen.getByText('provider_declaration_only')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].providerCacheEvidenceRef = 'provider-cache-observation:ready'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks = []; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].owner = 'caller'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it('context 预算独立展示有界投影或拒绝，不提升为资源就绪', async () => {
    const view = observed({ requestId: 'context', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.skillRegistryEvidenceRef = `skill-registry-observation:${'c'.repeat(32)}`;
    a.soulEvidenceRef = `soul-observation:${'d'.repeat(32)}`;
    a.skillHubEffectRef = `skillhub-status:${'a'.repeat(32)}`;
    a.skillHubEffectState = 'recorded';
    a.skillHubInstalledState = 'observed_selection_pending';
    a.skillSelectionState = 'compiled_checks_pending';
    a.checks = [{ id: 'skill_selection', owner: 'skill', required: true, state: 'blocked', reason: 'skill_selection_compiled_checks_pending' }];
    for (const [state, label] of [
      ['compiled_checks_pending', '既有预算合同编译完成；其他资源检查仍待完成，不代表就绪'],
      ['bounded_checks_pending', '已按既有预算生成有界投影；完整原文仍保留，其他检查待完成，不代表就绪'],
      ['rejected', '选中材料未通过既有 context 合同；原材料保留，准备仍受阻'],
    ] as const) {
      a.skillContextState = state;
      vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(structuredClone(view));
      render(wrap()); await act(async () => {});
      expect(screen.getByText(label)).toBeTruthy();
      cleanup();
    }
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].skillContextState = 'ready' as typeof a.skillContextState; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].skillSelectionState; },
      (v: StartupPreparationView) => { v.record!.attempts[0].skillSelectionState = 'sources_incomplete'; v.record!.attempts[0].checks![0].reason = 'skill_selection_sources_incomplete'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it('组合选择已编译仍不代表就绪；拒绝缺来源或伪造成功', async () => {
    const view = observed({ requestId: 'selection', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.skillRegistryEvidenceRef = `skill-registry-observation:${'c'.repeat(32)}`;
    a.soulEvidenceRef = `soul-observation:${'d'.repeat(32)}`;
    a.skillHubEffectRef = `skillhub-status:${'a'.repeat(32)}`;
    a.skillHubEffectState = 'recorded';
    a.skillHubInstalledState = 'observed_selection_pending';
    a.skillSelectionState = 'compiled_checks_pending';
    a.checks = [{ id: 'skill_selection', owner: 'skill', required: true, state: 'blocked', reason: 'skill_selection_compiled_checks_pending' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('完整候选身份与精确选择已封存；freshness 与辅助资源等检查仍待完成，不代表就绪')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].skillSelectionState = 'ready' as typeof a.skillSelectionState; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].skillRegistryEvidenceRef; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].soulEvidenceRef; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].skillHubInstalledState; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.attempts[0].checks![0].reason = 'skill_selection_conflict'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
    for (const state of ['sources_incomplete', 'conflict'] as const) {
      const partial = structuredClone(view); partial.record!.attempts[0].skillSelectionState = state;
      partial.record!.attempts[0].checks![0].reason = `skill_selection_${state}`;
      expect(() => validatePreparationView(partial, 'ws')).not.toThrow();
    }
  });
  it('SkillHub 双读材料封存只表示选择待裁决；拒绝伪造 ready', async () => {
    const view = observed({ requestId: 'installed', expectedWorkspaceRevision: 10 });
    const a = view.record!.attempts[0];
    a.skillHubEffectRef = `skillhub-status:${'a'.repeat(32)}`;
    a.skillHubEffectState = 'recorded';
    a.skillHubInstalledState = 'observed_selection_pending';
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('原 status 对应的指令双读与元数据已封存；不代表选择通过或就绪')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    const partial = structuredClone(view); partial.record!.attempts[0].skillHubInstalledState = 'incomplete';
    expect(() => validatePreparationView(partial, 'ws')).not.toThrow();
    for (const mutate of [
      (v: StartupPreparationView) => { v.record!.attempts[0].skillHubInstalledState = 'ready' as typeof a.skillHubInstalledState; },
      (v: StartupPreparationView) => { delete v.record!.attempts[0].skillHubEffectRef; },
      (v: StartupPreparationView) => { v.record!.attempts[0].state = 'accepted'; },
      (v: StartupPreparationView) => { v.record!.version = 'workorch-preparation-record/v1'; },
    ]) { const bad = structuredClone(view); mutate(bad); expect(() => validatePreparationView(bad, 'ws')).toThrow(); }
  });
  it('SkillHub 未决效果独立展示并禁止换请求；GET 不执行 status', async () => {
    const view = observed({ requestId: 'status', expectedWorkspaceRevision: 10 });
    const attempt = view.record!.attempts[0];
    attempt.skillHubEffectRef = `skillhub-status:${'a'.repeat(32)}`;
    attempt.skillHubEffectState = 'unresolved';
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(/skillhub-status:.*unresolved/)).toBeTruthy();
    expect(checkButton().disabled).toBe(true);
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    for (const state of ['ready', 'reserved', undefined]) {
      const bad = structuredClone(view); bad.record!.attempts[0].skillHubEffectState = state as typeof attempt.skillHubEffectState;
      expect(() => validatePreparationView(bad, 'ws')).toThrow();
    }
    const missing = structuredClone(view); delete missing.record!.attempts[0].skillHubEffectRef;
    expect(() => validatePreparationView(missing, 'ws')).toThrow();
    const recorded = structuredClone(view); recorded.record!.attempts[0].skillHubEffectState = 'recorded';
    expect(() => validatePreparationView(recorded, 'ws')).not.toThrow();
  });
  it('Soul 只展示完整历史观察的随机引用；不授M3，不保存原文', async () => {
    const view = observed({ requestId: 'soul', expectedWorkspaceRevision: 10 });
    const ref = `soul-observation:${'f'.repeat(32)}`;
    view.record!.attempts[0].soulEvidenceRef = ref;
    view.record!.attempts[0].checks = [{ id: 'soul_selection', owner: 'soul', required: true, state: 'blocked', reason: 'souls_observed_remaining_checks' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(ref)).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    const negative = structuredClone(view); delete negative.record!.attempts[0].soulEvidenceRef;
    negative.record!.attempts[0].checks![0].reason = 'soul_observation_unavailable_not_saved';
    expect(() => validatePreparationView(negative, 'ws')).not.toThrow();
    for (const ref of ['f'.repeat(64), 'soul-observation:bad']) {
      const bad = structuredClone(view); bad.record!.attempts[0].soulEvidenceRef = ref;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('Soul 历史证据引用无效');
    }
    for (const state of ['accepted', 'unknown']) {
      const bad = observed({ requestId: 'bad', expectedWorkspaceRevision: 10 }, state as 'accepted' | 'unknown'); bad.record!.attempts[0].soulEvidenceRef = ref;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('Soul 历史证据引用无效');
    }
    const omitted = structuredClone(view); delete omitted.record!.attempts[0].soulEvidenceRef;
    expect(() => validatePreparationView(omitted, 'ws')).toThrow('Soul 历史证据引用缺失');
    for (const reason of ['pass', 'ready', 'managed_configs_observed_remaining_checks']) {
      const bad = structuredClone(view); bad.record!.attempts[0].checks![0].reason = reason;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('准备检查结果无效');
    }
  });
  it('Skill Registry 只展示完整历史观察的随机引用；不授M3，不保存原文', async () => {
    const view = observed({ requestId: 'soul', expectedWorkspaceRevision: 10 });
    const ref = `skill-registry-observation:${'f'.repeat(32)}`;
    view.record!.attempts[0].skillRegistryEvidenceRef = ref;
    view.record!.attempts[0].checks = [{ id: 'skill_selection', owner: 'skill', required: true, state: 'blocked', reason: 'skill_registry_observed_selection_pending' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(ref)).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    const negative = structuredClone(view); delete negative.record!.attempts[0].skillRegistryEvidenceRef;
    negative.record!.attempts[0].checks![0].reason = 'skill_registry_observation_unavailable_not_saved';
    expect(() => validatePreparationView(negative, 'ws')).not.toThrow();
    for (const ref of ['f'.repeat(64), 'skill-registry-observation:bad']) {
      const bad = structuredClone(view); bad.record!.attempts[0].skillRegistryEvidenceRef = ref;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('Skill Registry 历史证据引用无效');
    }
    for (const state of ['accepted', 'unknown']) {
      const bad = observed({ requestId: 'bad', expectedWorkspaceRevision: 10 }, state as 'accepted' | 'unknown'); bad.record!.attempts[0].skillRegistryEvidenceRef = ref;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('Skill Registry 历史证据引用无效');
    }
    const omitted = structuredClone(view); delete omitted.record!.attempts[0].skillRegistryEvidenceRef;
    expect(() => validatePreparationView(omitted, 'ws')).toThrow('Skill Registry 历史证据引用缺失');
    for (const reason of ['pass', 'ready', 'managed_configs_observed_remaining_checks']) {
      const bad = structuredClone(view); bad.record!.attempts[0].checks![0].reason = reason;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('准备检查结果无效');
    }
  });
  it('托管配置只显示随机历史引用；拒绝内容hash、错状态与伪PASS', async () => {
    const view = observed({ requestId: 'managed-config', expectedWorkspaceRevision: 10 });
    const ref = `managed-config-observation:${'e'.repeat(32)}`;
    view.record!.attempts[0].managedConfigEvidenceRef = ref;
    view.record!.attempts[0].checks = [{ id: 'managed_configuration', owner: 'file', required: true, state: 'blocked', reason: 'managed_configs_observed_remaining_checks' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText(ref)).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const reason of ['managed_config_observation_unavailable', 'managed_config_observation_incomplete', 'managed_config_declaration_invalid', 'managed_config_content_invalid', 'managed_config_content_conflict']) {
      const negative = structuredClone(view); negative.record!.attempts[0].checks![0].reason = reason;
      expect(() => validatePreparationView(negative, 'ws')).not.toThrow();
    }
    for (const value of ['e'.repeat(64), 'managed-config-observation:bad']) {
      const bad = structuredClone(view); bad.record!.attempts[0].managedConfigEvidenceRef = value;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('托管配置历史证据引用无效');
    }
    for (const state of ['accepted', 'unknown']) {
      const bad = observed({ requestId: 'bad', expectedWorkspaceRevision: 10 }, state as 'accepted' | 'unknown'); bad.record!.attempts[0].managedConfigEvidenceRef = ref;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('托管配置历史证据引用无效');
    }
    for (const reason of ['pass', 'ready', 'resource_observed_remaining_checks']) {
      const bad = structuredClone(view); bad.record!.attempts[0].checks![0].reason = reason;
      expect(() => validatePreparationView(bad, 'ws')).toThrow('准备检查结果无效');
    }
  });
  it('只读展示规格资源历史引用；原字节材料不进入页面，匹配也不授M3', async () => {
    const view = observed({ requestId: 'resource-evidence', expectedWorkspaceRevision: 10 });
    view.record!.attempts[0].resourceEvidenceHash = 'd'.repeat(64);
    view.record!.attempts[0].checks = [{ id: 'resource/input', owner: 'file', required: true, state: 'blocked', reason: 'resource_observed_remaining_checks' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('d'.repeat(64))).toBeTruthy();
    expect(screen.getByText('resource_observed_remaining_checks')).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const state of ['accepted', 'unknown']) {
      const bad = observed({ requestId: 'bad', expectedWorkspaceRevision: 10 }, state as 'accepted' | 'unknown');
      bad.record!.attempts[0].resourceEvidenceHash = 'd'.repeat(64);
      expect(() => validatePreparationView(bad, 'ws')).toThrow('资源历史证据引用无效');
    }
    const bad = structuredClone(view); bad.record!.attempts[0].resourceEvidenceHash = 'not-a-hash';
    expect(() => validatePreparationView(bad, 'ws')).toThrow('资源历史证据引用无效');
    for (const reason of ['resource_observation_unavailable', 'resource_observation_incomplete', 'resource_content_changed', 'resource_expected_hash_unavailable']) {
      const negative = structuredClone(view); negative.record!.attempts[0].checks![0].reason = reason;
      expect(() => validatePreparationView(negative, 'ws')).not.toThrow();
    }
    for (const reason of ['pass', 'ready', 'directory_observed_remaining_checks']) {
      const invalid = structuredClone(view); invalid.record!.attempts[0].checks![0].reason = reason;
      expect(() => validatePreparationView(invalid, 'ws')).toThrow('准备检查结果无效');
    }
  });
  it('只读展示目录历史引用与具体阻塞；引用不授予ready或执行', async () => {
    const view = observed({ requestId: 'file-evidence', expectedWorkspaceRevision: 10 });
    view.record!.attempts[0].directoryEvidenceHash = 'c'.repeat(64);
    view.record!.attempts[0].checks = [{ id: 'directory_boundary', owner: 'file', required: true, state: 'blocked', reason: 'directory_observed_remaining_checks' }];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
    render(wrap()); await act(async () => {});
    expect(screen.getByText('c'.repeat(64))).toBeTruthy();
    expect(screen.getByText('directory_observed_remaining_checks')).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    for (const state of ['accepted', 'unknown']) {
      const bad = observed({ requestId: 'bad', expectedWorkspaceRevision: 10 }, state as 'accepted' | 'unknown');
      bad.record!.attempts[0].directoryEvidenceHash = 'c'.repeat(64);
      expect(() => validatePreparationView(bad, 'ws')).toThrow('目录历史证据引用无效');
    }
    const bad = structuredClone(view); bad.record!.attempts[0].directoryEvidenceHash = 'not-a-hash';
    expect(() => validatePreparationView(bad, 'ws')).toThrow('目录历史证据引用无效');
  });
	 it('展示缓存和历史的具体阻塞原因；新鲜声明和空历史仍不算ready', async () => {
	   const view = observed({ requestId: 'negative-observations', expectedWorkspaceRevision: 10 });
	   view.record!.attempts[0].checks = [
	     { id: 'first_activation_no_run', owner: 'run', required: true, state: 'blocked', reason: 'run_history_empty_fence_not_final' },
	     { id: 'provider_cache', owner: 'llm', required: true, state: 'blocked', reason: 'provider_declaration_only' },
	   ];
	   vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
	   render(wrap()); await act(async () => {});
	   expect(screen.getByText('provider_declaration_only')).toBeTruthy();
	   expect(screen.getByText('run_history_empty_fence_not_final')).toBeTruthy();
	   expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
	   expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
	   expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
	   for (const reason of ['ready', 'pass', 'provider_cache_miss']) {
	     const malformed = structuredClone(view);
	     malformed.record!.attempts[0].checks![0].reason = reason;
	     expect(() => validatePreparationView(malformed, 'ws')).toThrow('准备检查结果无效');
	   }
	 });
  it('开页只GET；显式检查单飞、读回阻塞，不启动Run', async () => {
    const post = deferred<Awaited<ReturnType<typeof workorchApi.checkStartupPreparation>>>();
    vi.mocked(workorchApi.checkStartupPreparation).mockReturnValue(post.promise);
    render(wrap()); await act(async () => {});
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(recoverButton().disabled).toBe(true);
    await act(async () => { fireEvent.click(checkButton()); fireEvent.click(checkButton()); });
    expect(workorchApi.checkStartupPreparation).toHaveBeenCalledTimes(1);
    const request = vi.mocked(workorchApi.checkStartupPreparation).mock.calls[0][1];
    expect(request.expectedWorkspaceRevision).toBe(10);
    expect(request.requestId).toBeTruthy();
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(observed(request));
    await act(async () => post.resolve({ requestId: request.requestId, preparation: observed(request) }));
    expect(screen.getByText('check_pipeline_incomplete')).toBeTruthy();
    expect(screen.getByText('本流程不签发 M3 完成凭据')).toBeTruthy();
    expect(workorchApi.readStartupPreparation).toHaveBeenCalledTimes(2);
    expect(recoverButton().disabled).toBe(true);
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
  });
  it('部分持久后丢回复，GET保留原revision；仅显式恢复原身份', async () => {
    vi.mocked(workorchApi.checkStartupPreparation).mockImplementation(async (_id, request) => {
      vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(observed(request, 'accepted'));
      throw new Error('409 after accepted write');
    });
    vi.mocked(workorchApi.recoverStartupPreparation).mockImplementation(async (_id, request) => {
      const view = observed(request);
      vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(view);
      return { requestId: request.requestId, preparation: view };
    });
    render(wrap()); await act(async () => {});
    await act(async () => fireEvent.click(checkButton()));
    const request = vi.mocked(workorchApi.checkStartupPreparation).mock.calls[0][1];
    expect(checkButton().disabled).toBe(true);
    expect(recoverButton().disabled).toBe(false);
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '只读刷新准备结果' })));
    expect(checkButton().disabled).toBe(true);
    await act(async () => fireEvent.click(recoverButton()));
    expect(workorchApi.recoverStartupPreparation).toHaveBeenCalledExactlyOnceWith('ws', request);
    expect(request.expectedWorkspaceRevision).toBe(10);
    expect(checkButton().disabled).toBe(false);
    expect(workorchApi.checkStartupPreparation).toHaveBeenCalledTimes(1);
  });
  it('错误后GET未见原请求不证明拒绝，不解锁新身份或自动重发', async () => {
    vi.mocked(workorchApi.checkStartupPreparation).mockRejectedValue(new Error('response lost'));
    render(wrap()); await act(async () => {});
    await act(async () => fireEvent.click(checkButton()));
    expect(checkButton().disabled).toBe(true);
    expect(recoverButton().disabled).toBe(false);
    expect(screen.getByText(/待对账原请求/)).toBeTruthy();
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '只读刷新准备结果' })));
    expect(checkButton().disabled).toBe(true);
    expect(workorchApi.checkStartupPreparation).toHaveBeenCalledTimes(1);
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
  });
  it('重开仅使用服务端原身份，unknown不被刷新结算', async () => {
    const original = { requestId: 'server-original', expectedWorkspaceRevision: 7 };
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(observed(original, 'unknown'));
    vi.mocked(workorchApi.recoverStartupPreparation).mockResolvedValue({ requestId: original.requestId, preparation: observed(original) });
    render(wrap()); await act(async () => {});
    expect(checkButton().disabled).toBe(true);
    expect(recoverButton().disabled).toBe(false);
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    await act(async () => fireEvent.click(recoverButton()));
    expect(workorchApi.recoverStartupPreparation).toHaveBeenCalledExactlyOnceWith('ws', original);
  });
  it('切换Workspace隔离迟到GET；错误scope和ready响应不成为可检查视图', async () => {
    const old = deferred<StartupPreparationView>();
    vi.mocked(workorchApi.readStartupPreparation).mockReturnValueOnce(old.promise).mockResolvedValue(empty('other'));
    const mounted = render(wrap()); await act(async () => {});
    mounted.rerender(wrap('other')); await act(async () => {});
    await act(async () => old.resolve(empty()));
    expect(workorchApi.readStartupPreparation).toHaveBeenCalledTimes(2);
    expect(checkButton().disabled).toBe(false);
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    cleanup();
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue({ ...empty(), operationState: 'ready' } as unknown as StartupPreparationView);
    render(wrap()); await act(async () => {});
    expect(checkButton().disabled).toBe(true);
    expect(screen.getByText(/准备结果状态不完整/)).toBeTruthy();
  });
  it('原操作完成而其他页面已有新未决操作时，继续锁定并恢复服务端最新身份', async () => {
    const older = { requestId: 'older', expectedWorkspaceRevision: 7 };
    const newer = { requestId: 'newer', expectedWorkspaceRevision: 9 };
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(observed(older, 'accepted'));
    vi.mocked(workorchApi.recoverStartupPreparation).mockImplementation(async () => {
      const oldView = observed(older);
      const next = observed(newer, 'accepted');
      next.record!.attempts.unshift(oldView.record!.attempts[0]);
      vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(next);
      return { requestId: older.requestId, preparation: next };
    });
    render(wrap()); await act(async () => {});
    await act(async () => fireEvent.click(recoverButton()));
    expect(checkButton().disabled).toBe(true);
    expect(screen.getByText(/待对账原请求：newer/)).toBeTruthy();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
  });
  it('切换Workspace后迟到POST不触发旧页对账或覆盖新页', async () => {
    const post = deferred<Awaited<ReturnType<typeof workorchApi.checkStartupPreparation>>>();
    vi.mocked(workorchApi.checkStartupPreparation).mockReturnValue(post.promise);
    const mounted = render(wrap()); await act(async () => {});
    await act(async () => fireEvent.click(checkButton()));
    const request = vi.mocked(workorchApi.checkStartupPreparation).mock.calls[0][1];
    vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue(empty('other'));
    mounted.rerender(wrap('other')); await act(async () => {});
    await act(async () => post.resolve({ requestId: request.requestId, preparation: observed(request) }));
    expect(workorchApi.readStartupPreparation).toHaveBeenCalledTimes(2);
    expect(screen.queryByText('check_pipeline_incomplete')).toBeNull();
    expect(checkButton().disabled).toBe(false);
  });
});
