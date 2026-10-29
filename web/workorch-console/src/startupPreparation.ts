export type PreparationCheckRequest = { requestId: string; expectedWorkspaceRevision: number };
export type PreparationCheck = { id: string; owner: string; required: boolean; state: 'pending' | 'blocked' | 'passed' | 'failed'; reason: string };
export type PreparationAssessment = {
  version: 'workorch-preparation-all-of-assessment/v1';
  manifestHash: string;
  outcome: 'blocked' | 'checks_passed';
  requiredCount: number;
  passedRequiredCount: number;
  outstanding: { id: string; owner: string; state: string; reason: string }[];
};
export type PreparationAttempt = {
  definitionCompilation?: {
    compilerVersion: string; snapshotHash: string; manifestHash: string;
    sourceInventoryHash: string; profileContractsHash: string;
    sourceCount: number; requirementCount: number; obligationCount: number; verificationCount: number;
  };
  assessment?: PreparationAssessment;
  mcpRegistryEvidenceRef?: string;
  requestId: string;
  payloadHash: string;
  expectedWorkspaceRevision: number;
  committedWorkspaceRevision: number;
  state: 'accepted' | 'unknown' | 'blocked' | 'failed';
  startedAt: string;
  completedAt?: string;
  creationEpoch?: string;
  preparationEpoch?: number;
  binding: { startupInputHash: string; snapshotHash: string; manifestHash: string };
  checks?: PreparationCheck[];
  evaluationVersion?: string;
  noRunEvaluationVersion?: string;
  emptySelectionEvaluationVersion?: string;
  executionInputVersion?: string;
  directoryEvidenceHash?: string;
  resourceEvidenceHash?: string;
  managedConfigEvidenceRef?: string;
  soulEvidenceRef?: string;
  sealEvidenceRef?: string;
  providerCacheEvidenceRef?: string;
  runHistoryEvidenceRef?: string;
  memoryInventoryEvidenceRef?: string;
 skillRegistryEvidenceRef?: string;
 skillHubEffectRef?: string;
 skillHubEffectState?: 'reserved' | 'recorded' | 'unresolved';
 skillHubInstalledState?: 'incomplete' | 'observed_selection_pending';
 skillSelectionState?: 'sources_incomplete' | 'conflict' | 'compiled_checks_pending';
 skillContextState?: 'compiled_checks_pending' | 'bounded_checks_pending' | 'rejected';
 skillReferenceState?: 'observed_checks_pending' | 'sources_incomplete' | 'source_unresolved' | 'unavailable';
 skillStatusState?: 'observed_freshness_pending' | 'sources_incomplete' | 'status_blocked' | 'version_conflict';
};
export type PreparationResourceInitialization = { requestId: string; expectedWorkspaceRevision: number; committedWorkspaceRevision: number; state: 'pending' | 'blocked' | 'completed'; attempts: number; registrationState?: 'waiting_for_files' | 'not_started' | 'pending' | 'unresolved' | 'running' | 'unknown' | 'failed' | 'succeeded'; registrationAttempts?: number };
export type PreparationResourceInitializationOperation = { requestId: string; preparation: StartupPreparationView };
export type StartupPreparationView = {
  skillHubProjectIssue?: 'project_registration_required';
 resourceInitializations?: PreparationResourceInitialization[];
  workspaceId: string;
  workspaceRevision: number;
  activeRunId?: string;
  operationState: 'not_checked' | PreparationAttempt['state'];
  currentValidity: 'not_checked' | 'not_evaluated' | 'stale';
  record?: { version: string; attempts: PreparationAttempt[] };
};
export type PreparationBootstrapCompilation = {
  version: 'workorch-original-prepared-bootstrap/v1'; failureCode?: string;
  snapshotHash: string; manifestHash: string; bootstrapHash?: string;
};
export type PreparationOperation = { requestId: string; preparation: StartupPreparationView; compilation?: PreparationBootstrapCompilation };

const bootstrapFailures = ['original_inputs_unavailable', 'original_policy_unavailable', 'soul_source_unavailable', 'skill_source_unavailable',
  'memory_source_unavailable', 'memory_semantic_evidence_required', 'mcp_source_unavailable', 'provider_declaration_unavailable',
  'activation_structure_rejected', 'bootstrap_structure_rejected'];

// Only explicit check/recover returns a reproducible compilation. It cannot
// update per-check verdicts, currentValidity, M3 or execution permissions.
export function validatePreparationCompilation(result: PreparationOperation, request: PreparationCheckRequest): PreparationBootstrapCompilation | undefined {
  const c = result.compilation;
  if (!c) return undefined; // historical server responses have no such product
  const a = result.preparation.record?.attempts.find(row => row.requestId === request.requestId && row.expectedWorkspaceRevision === request.expectedWorkspaceRevision);
  if (result.requestId !== request.requestId || !a || c.version !== 'workorch-original-prepared-bootstrap/v1' || c.snapshotHash !== a.binding.snapshotHash || c.manifestHash !== a.binding.manifestHash ||
      Object.keys(c).some(key => !['version', 'failureCode', 'snapshotHash', 'manifestHash', 'bootstrapHash'].includes(key)) ||
      (c.failureCode ? !bootstrapFailures.includes(c.failureCode) || Boolean(c.bootstrapHash) : a.executionInputVersion !== 'workorch-preparation-execution-inputs/v1' || !a.definitionCompilation || !/^prepared-bootstrap-sha256:[0-9a-f]{64}$/.test(c.bootstrapHash ?? ''))) {
    throw new Error('结构编译响应未绑定原操作或包含无效结论');
  }
  return c;
}

const mcpSelectionLabels = {
  mcp_selection_empty_policy_pending: '选择为空；尚未证明无需 MCP，不授予 N/A 或就绪',
  mcp_selection_compiled_connection_pending: '选择已编译；尚缺连接及持续有效性证明',
  mcp_selection_source_binding_conflict: '选择来源绑定冲突；未取得连接证明',
  mcp_selection_configuration_source_unavailable: '选择配置来源不完整；未取得连接证明',
  mcp_selection_configuration_declaration_invalid: '选择配置声明无效或有歧义；未取得连接证明',
  mcp_selection_selection_limit: '选择超出数量边界；未取得连接证明',
  mcp_selection_skill_source_incomplete: '完整 Skill 选择来源缺失或受阻；未取得连接证明',
  mcp_selection_skill_requirement_invalid: 'Skill 的 MCP 必需引用无效；未取得连接证明',
  mcp_selection_server_disabled: '所需 MCP server 已禁用；未取得连接证明',
  mcp_selection_server_missing: '所需 MCP server 不存在；未取得连接证明',
  mcp_selection_capability_disabled: '所需 MCP capability 已禁用；未取得连接证明',
  mcp_selection_capability_server_not_selected: '所需 capability 的 server 未被选中；不会自动补选',
  mcp_selection_capability_not_allowed: '所需 capability 不在 server allowlist 内；未取得连接证明',
  mcp_selection_capability_missing: '所需 MCP capability 不存在；未取得连接证明',
} as const;

const providerSelectionLabels = {
  provider_selection_authorization_pending: '精确模型与能力声明已绑定；尚缺授权、预算及持续有效性证明，不代表就绪',
  provider_selection_budget_observed_authorization_pending: '精确模型容量预算已由 LLM owner 观察；尚缺授权及持续有效性证明，不代表就绪或执行额度',
  provider_selection_budget_unavailable: '精确模型容量预算不可用；未使用默认值或相邻模型',
  provider_selection_budget_source_conflict: '容量预算与原精确模型来源冲突',
  provider_selection_source_binding_conflict: '模型选择来源绑定冲突',
  provider_selection_configuration_source_unavailable: '缺少冻结的模型选择配置来源',
  provider_selection_invalid: '原模型选择无效；不会自动改选模型',
  provider_selection_model_missing: '精确模型不在原缓存中；不会使用相邻模型',
  provider_selection_capability_incomplete: '精确模型能力声明不完整',
  provider_selection_operation_unsupported: '精确模型或 override 不支持 Responses',
  provider_selection_reasoning_unsupported: '原 reasoning 设置没有精确能力声明支持',
} as const;

function isProviderSelectionReason(reason: string): reason is keyof typeof providerSelectionLabels {
  return Object.hasOwn(providerSelectionLabels, reason);
}
export function providerPreparationReasonLabel(reason: string): string | undefined {
  if (reason === 'provider_configuration_observed_remaining_checks') return '已读取 Workorch 的 LLM 配置；启动执行时封存模型选择，后续 loop 使用运行上下文';
  return isProviderSelectionReason(reason) ? providerSelectionLabels[reason] : undefined;
}

function isMCPSelectionReason(reason: string): reason is keyof typeof mcpSelectionLabels {
  return Object.hasOwn(mcpSelectionLabels, reason);
}
function mcpReasonRequiresEvidence(reason: string): boolean {
  return reason === 'mcp_registry_observed_connection_pending' || isMCPSelectionReason(reason);
}
export function mcpPreparationReasonLabel(reason: string): string | undefined {
  if (isMCPSelectionReason(reason)) return mcpSelectionLabels[reason];
  if (reason === 'mcp_registry_observed_connection_pending') return '仅保留完整 registry 历史；未保存选择裁决或连接证明';
  return undefined;
}

function validBlockedReason(check: PreparationCheck): boolean {
  if (check.id === 'mcp_selection' && (check.reason === 'mcp_registry_observation_unavailable_not_saved' || mcpReasonRequiresEvidence(check.reason))) return true;
  if (check.id === 'memory_selection' && ['memory_inventory_unavailable_not_saved', 'memory_inventory_observed_selection_pending', 'memory_semantic_evidence_required', 'memory_selection_candidate_limit'].includes(check.reason)) return true;
  if (check.id === 'skill_selection' && ['skill_selection_sources_incomplete', 'skill_selection_conflict', 'skill_selection_compiled_checks_pending'].includes(check.reason)) return true;
 if (check.id === 'skill_selection' && ['skill_registry_observation_unavailable_not_saved','skill_registry_observed_selection_pending'].includes(check.reason)) return true;
  if (check.id === 'soul_selection') return ['check_pipeline_incomplete', 'original_check_interrupted', 'binding_changed', 'soul_observation_unavailable_not_saved', 'souls_observed_remaining_checks'].includes(check.reason);
  if (check.id === 'managed_configuration' && ['managed_config_observation_unavailable', 'managed_config_observation_incomplete', 'managed_config_declaration_invalid', 'managed_config_content_invalid', 'managed_config_content_conflict', 'managed_configs_observed_remaining_checks'].includes(check.reason)) return true;
  if (['check_pipeline_incomplete', 'original_check_interrupted', 'binding_changed'].includes(check.reason)) return true;
  if (check.id === 'provider_cache') return ['provider_cache_unavailable', 'provider_cache_miss', 'provider_cache_expired', 'provider_declaration_only'].includes(check.reason) || isProviderSelectionReason(check.reason);
  if (check.id === 'first_activation_no_run') return ['run_history_unavailable', 'run_history_retired', 'run_history_present', 'run_history_empty_fence_not_final'].includes(check.reason);
  if (check.id === 'directory_boundary') return ['directory_observation_unavailable', 'directory_observation_incomplete', 'directory_permissions_denied', 'directory_observed_remaining_checks'].includes(check.reason);
  if (check.id.startsWith('resource/')) return ['resource_observation_unavailable', 'resource_observation_incomplete', 'resource_content_changed', 'resource_expected_hash_unavailable', 'resource_observed_remaining_checks'].includes(check.reason);
  return false;
}

function validEvaluatedCheck(a: PreparationAttempt, c: PreparationCheck): boolean {
  if (c.state === 'blocked') return validBlockedReason(c);
  if (a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || a.state !== 'blocked') return false;
  if (c.id === 'first_activation_no_run') return c.owner === 'run' && c.required && a.noRunEvaluationVersion === 'workorch-preparation-no-run-final-fence/v1' && (a.preparationEpoch ?? 0) === 0 && !!a.runHistoryEvidenceRef && !!a.sealEvidenceRef && c.state === 'passed' && c.reason === 'run_history_empty_final_fence_observed';
  if (c.id === 'exact_seal' || c.id === 'persistence_integrity') return c.owner === 'workspace' && c.required && !!a.sealEvidenceRef && c.state === 'passed' && c.reason === (c.id === 'exact_seal' ? 'seal_observed_remaining_checks' : 'persistence_binding_observed_remaining_checks');
  if (c.id === 'soul_selection') return c.owner === 'soul' && c.required && !!a.soulEvidenceRef && c.state === 'passed' && c.reason === 'souls_observed_remaining_checks';
  if (c.id === 'provider_cache') return c.owner === 'llm' && c.required && !!a.providerCacheEvidenceRef && c.state === 'passed' && c.reason === 'provider_configuration_observed_remaining_checks';
  if (c.id === 'skill_selection') return c.owner === 'skill' && c.required && c.state === 'passed' && c.reason === 'skill_empty_selection_observed_remaining_checks' && a.emptySelectionEvaluationVersion === 'workorch-preparation-empty-selection-evaluation/v1' && !!a.skillRegistryEvidenceRef && !!a.soulEvidenceRef && a.skillHubEffectState === 'recorded' && a.skillSelectionState === 'compiled_checks_pending' && a.skillContextState === 'compiled_checks_pending' && a.skillReferenceState === 'observed_checks_pending' && a.skillStatusState === 'observed_freshness_pending';
  if (c.id === 'mcp_selection') return c.owner === 'mcpregistry' && c.required && c.state === 'passed' && c.reason === 'mcp_empty_selection_observed_remaining_checks' && a.emptySelectionEvaluationVersion === 'workorch-preparation-empty-selection-evaluation/v1' && !!a.mcpRegistryEvidenceRef && !!a.skillRegistryEvidenceRef;
  if (c.id === 'memory_selection') return c.owner === 'memory' && c.required && !!a.memoryInventoryEvidenceRef && c.state === 'passed' && ['memory_list_selection_observed_remaining_checks', 'memory_empty_selection_observed_remaining_checks'].includes(c.reason);
  if (c.owner !== 'file') return false;
  if (c.id === 'directory_boundary') return c.required && !!a.directoryEvidenceHash && (c.state === 'passed' && c.reason === 'directory_observed_remaining_checks' || c.state === 'failed' && c.reason === 'directory_permissions_denied');
  if (c.id === 'managed_configuration') return c.required && !!a.managedConfigEvidenceRef && (c.state === 'passed' && c.reason === 'managed_configs_observed_remaining_checks' || c.state === 'failed' && ['managed_config_content_invalid', 'managed_config_content_conflict'].includes(c.reason));
  return c.id.startsWith('resource/') && !!a.resourceEvidenceHash && (c.state === 'passed' && c.reason === 'resource_observed_remaining_checks' || c.state === 'failed' && c.reason === 'resource_content_changed');
}

function validNoRunFinalShape(a: PreparationAttempt): boolean {
  if (a.noRunEvaluationVersion !== 'workorch-preparation-no-run-final-fence/v1' || a.state !== 'blocked' || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || (a.preparationEpoch ?? 0) !== 0 || !a.runHistoryEvidenceRef || !a.sealEvidenceRef || a.committedWorkspaceRevision !== a.expectedWorkspaceRevision + 2) return false;
  const checks = a.checks?.filter(c => ['first_activation_no_run', 'exact_seal', 'persistence_integrity'].includes(c.id));
  return checks?.length === 3 && checks.every(c => c.state === 'passed' && validEvaluatedCheck(a, c));
}

// This view has no ready/M3/activation permission representation. Reject wrong
// scope or invented success, rather than letting a late/malformed response
// unlock a new operation. All identities remain in page memory or server state.
function memoryReasonRequiresEvidence(reason: string): boolean {
  return ['memory_inventory_observed_selection_pending', 'memory_semantic_evidence_required', 'memory_selection_candidate_limit', 'memory_list_selection_observed_remaining_checks', 'memory_empty_selection_observed_remaining_checks'].includes(reason);
}

function validateAssessment(a: PreparationAttempt): void {
  const s = a.assessment;
  if (s === undefined) return;
  if (!s || a.state !== 'blocked' || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || s.version !== 'workorch-preparation-all-of-assessment/v1' || s.manifestHash !== a.binding.manifestHash || !Array.isArray(s.outstanding) || !Array.isArray(a.checks)) throw new Error('整体检查裁决身份无效');
  const required = a.checks.filter(c => c.required);
  const outstanding = required.filter(c => c.state !== 'passed');
  if (required.length === 0 || s.requiredCount !== required.length || s.passedRequiredCount !== required.length - outstanding.length || s.outcome !== (outstanding.length ? 'blocked' : 'checks_passed') || s.outstanding.length !== outstanding.length) throw new Error('整体检查裁决与逐项结果不一致');
  for (const [i, c] of outstanding.entries()) {
    const actual = s.outstanding[i];
    if (!actual || actual.id !== c.id || actual.owner !== c.owner || actual.state !== c.state || actual.reason !== c.reason) throw new Error('整体检查裁决遗漏或替换阻塞项');
  }
}

export function validatePreparationView(view: StartupPreparationView, workspaceId: string): void {
  if (!view || view.workspaceId !== workspaceId || !Number.isSafeInteger(view.workspaceRevision) || view.workspaceRevision <= 0) throw new Error('准备结果作用域或版本不匹配');
  if (view.skillHubProjectIssue !== undefined) {
    const last = view.record?.attempts.at(-1);
    if (view.skillHubProjectIssue !== 'project_registration_required' || view.record?.version !== 'workorch-preparation-record/v2' || last?.state !== 'blocked' || last.skillHubEffectState !== 'unresolved') throw new Error('SkillHub 项目诊断未绑定原阻塞操作');
  }
  if (view.resourceInitializations !== undefined) {
    if (!Array.isArray(view.resourceInitializations) || view.resourceInitializations.length > 16) throw new Error('资源初始化记录无效');
    const seen = new Set<string>();
    for (const r of view.resourceInitializations) {
      if (!r || typeof r.requestId !== 'string' || !r.requestId.trim() || seen.has(r.requestId) || !Number.isSafeInteger(r.expectedWorkspaceRevision) || r.expectedWorkspaceRevision <= 0 || !Number.isSafeInteger(r.committedWorkspaceRevision) || r.committedWorkspaceRevision <= r.expectedWorkspaceRevision || r.committedWorkspaceRevision > view.workspaceRevision || !['pending', 'blocked', 'completed'].includes(r.state) || !Number.isSafeInteger(r.attempts) || r.attempts < 1 || r.attempts > 4) throw new Error('资源初始化身份或状态无效');
      const registrationStates = ['waiting_for_files', 'not_started', 'pending', 'unresolved', 'running', 'unknown', 'failed', 'succeeded'];
      const n = r.registrationAttempts ?? 0;
      if (!Number.isSafeInteger(n) || n < 0 || n > 4 || (r.registrationState === undefined ? n !== 0 : !registrationStates.includes(r.registrationState) || (['waiting_for_files', 'not_started'].includes(r.registrationState) ? n !== 0 : n < 1)) || r.state === 'completed' && r.registrationState !== undefined && r.registrationState !== 'succeeded' || r.registrationState === 'succeeded' && r.state !== 'completed' || ['not_started', 'pending'].includes(r.registrationState ?? '') && r.state !== 'pending' || ['failed', 'unresolved', 'unknown', 'running'].includes(r.registrationState ?? '') && r.state !== 'blocked') throw new Error('SkillHub 登记回执状态无效');
      seen.add(r.requestId);
    }
  }
  if (!view.record) {
    if (view.operationState !== 'not_checked' || view.currentValidity !== 'not_checked') throw new Error('准备结果状态不完整');
    return;
  }
  const attempts = view.record.attempts;
  if (!['workorch-preparation-record/v1', 'workorch-preparation-record/v2'].includes(view.record.version) || !Array.isArray(attempts) || attempts.length === 0 || attempts.length > 64 || !['not_evaluated', 'stale'].includes(view.currentValidity)) throw new Error('准备记录无效');
  const seen = new Set<string>();
  let revision = 0;
  for (const a of attempts) {
    if (a.emptySelectionEvaluationVersion !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || a.emptySelectionEvaluationVersion !== 'workorch-preparation-empty-selection-evaluation/v1')) throw new Error('空选择裁决版本无效');
    if (a.executionInputVersion !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || a.executionInputVersion !== 'workorch-preparation-execution-inputs/v1' || !a.sealEvidenceRef)) throw new Error('原执行输入归档版本无效');
    if (a.noRunEvaluationVersion !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || !validNoRunFinalShape(a))) throw new Error('no-Run 原结算栅栏版本或绑定无效');
    if (a.runHistoryEvidenceRef !== undefined && (
      typeof a.runHistoryEvidenceRef !== 'string' || view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked'
      || !/^run-history-observation:[0-9a-f]{32}$/.test(a.runHistoryEvidenceRef)
      || !a.checks?.some(c => c.id === 'first_activation_no_run' && c.owner === 'run' && c.required && (c.state === 'passed' && validNoRunFinalShape(a) || c.state === 'blocked' && ['run_history_retired', 'run_history_present', 'run_history_empty_fence_not_final', 'binding_changed'].includes(c.reason)))
    )) throw new Error('Run 历史观察引用无效');
    if (a.mcpRegistryEvidenceRef !== undefined && (
      typeof a.mcpRegistryEvidenceRef !== 'string' || view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked'
      || !/^mcp-registry-observation:[0-9a-f]{32}$/.test(a.mcpRegistryEvidenceRef)
      || !a.checks?.some(c => c.id === 'mcp_selection' && c.owner === 'mcpregistry' && c.required && (mcpReasonRequiresEvidence(c.reason) || c.reason === 'binding_changed' || c.state === 'passed' && validEvaluatedCheck(a, c)))
    )) throw new Error('MCP Registry 历史引用无效');
    if (a.evaluationVersion !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1')) throw new Error('逐项裁决版本无效');
    if (a.providerCacheEvidenceRef !== undefined && (typeof a.providerCacheEvidenceRef !== 'string' || view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || !/^provider-cache-observation:[0-9a-f]{32}$/.test(a.providerCacheEvidenceRef))) throw new Error('Provider 缓存历史证据引用无效');
    if (a.providerCacheEvidenceRef && !a.checks?.some(c => c.id === 'provider_cache' && c.owner === 'llm' && c.required && (['provider_cache_miss', 'provider_cache_expired', 'provider_declaration_only', 'binding_changed', 'provider_configuration_observed_remaining_checks'].includes(c.reason) || isProviderSelectionReason(c.reason)))) throw new Error('Provider 缓存历史检查缺失');
    if (a.memoryInventoryEvidenceRef !== undefined && (typeof a.memoryInventoryEvidenceRef !== 'string' || view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || !/^memory-inventory-observation:[0-9a-f]{32}$/.test(a.memoryInventoryEvidenceRef) || !a.checks?.some(c => c.id === 'memory_selection' && c.owner === 'memory' && c.required && (memoryReasonRequiresEvidence(c.reason) || c.reason === 'binding_changed')))) throw new Error('Memory 库存历史引用无效');
    if (a.skillContextState !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || a.skillSelectionState !== 'compiled_checks_pending' || !['compiled_checks_pending', 'bounded_checks_pending', 'rejected'].includes(a.skillContextState))) throw new Error('Skill context 预算状态无效');
    if (a.skillReferenceState !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || a.skillSelectionState !== 'compiled_checks_pending' || !['observed_checks_pending', 'sources_incomplete', 'source_unresolved', 'unavailable'].includes(a.skillReferenceState))) throw new Error('Skill 辅助文件观察状态无效');
    if (a.skillStatusState !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || a.skillSelectionState !== 'compiled_checks_pending' || !['observed_freshness_pending', 'sources_incomplete', 'status_blocked', 'version_conflict'].includes(a.skillStatusState))) throw new Error('Skill 来源状态诊断无效');
    if (a.skillSelectionState !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || !a.skillRegistryEvidenceRef || !['sources_incomplete', 'conflict', 'compiled_checks_pending'].includes(a.skillSelectionState) || a.skillSelectionState === 'compiled_checks_pending' && (!a.soulEvidenceRef || a.skillHubEffectState !== 'recorded' || a.skillHubInstalledState !== 'observed_selection_pending'))) throw new Error('Skill 组合选择材料状态无效');
    if (a.skillHubInstalledState !== undefined && (view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || !a.skillHubEffectRef || !['incomplete', 'observed_selection_pending'].includes(a.skillHubInstalledState))) throw new Error('SkillHub 已安装材料状态无效');
    if (a.skillHubEffectRef !== undefined) {
      if (view.record.version !== 'workorch-preparation-record/v2' || !/^skillhub-status:[0-9a-f]{32}$/.test(a.skillHubEffectRef) || (a.state === 'accepted' || a.state === 'unknown' ? a.skillHubEffectState !== 'reserved' : a.state !== 'blocked' || !['recorded', 'unresolved'].includes(a.skillHubEffectState ?? ''))) throw new Error('SkillHub 操作效果状态无效');
    } else if (a.skillHubEffectState !== undefined) throw new Error('SkillHub 操作引用缺失');
 if (a.skillRegistryEvidenceRef !== undefined && (typeof a.skillRegistryEvidenceRef !== 'string' || a.state !== 'blocked' || view.record.version !== 'workorch-preparation-record/v2' || !/^skill-registry-observation:[0-9a-f]{32}$/.test(a.skillRegistryEvidenceRef))) throw new Error('Skill Registry 历史证据引用无效');
    if (a.sealEvidenceRef !== undefined && (typeof a.sealEvidenceRef !== 'string' || a.state !== 'blocked' || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || view.record.version !== 'workorch-preparation-record/v2' || !/^seal-observation:[0-9a-f]{32}$/.test(a.sealEvidenceRef))) throw new Error('封存历史证据引用无效');
    if (a.soulEvidenceRef !== undefined && (typeof a.soulEvidenceRef !== 'string' || a.state !== 'blocked' || view.record.version !== 'workorch-preparation-record/v2' || !/^soul-observation:[0-9a-f]{32}$/.test(a.soulEvidenceRef))) throw new Error('Soul 历史证据引用无效');
    if (a.managedConfigEvidenceRef !== undefined && (typeof a.managedConfigEvidenceRef !== 'string' || a.state !== 'blocked' || view.record.version !== 'workorch-preparation-record/v2' || !/^managed-config-observation:[0-9a-f]{32}$/.test(a.managedConfigEvidenceRef))) throw new Error('托管配置历史证据引用无效');
    if (a.resourceEvidenceHash !== undefined && (typeof a.resourceEvidenceHash !== 'string' || a.state !== 'blocked' || view.record.version !== 'workorch-preparation-record/v2' || !/^[0-9a-f]{64}$/.test(a.resourceEvidenceHash))) throw new Error('资源历史证据引用无效');
    if (a.directoryEvidenceHash !== undefined && (typeof a.directoryEvidenceHash !== 'string' || a.state !== 'blocked' || view.record.version !== 'workorch-preparation-record/v2' || !/^[0-9a-f]{64}$/.test(a.directoryEvidenceHash))) throw new Error('目录历史证据引用无效');
    if (!a.requestId || !a.payloadHash || seen.has(a.requestId) || !['accepted', 'unknown', 'blocked', 'failed'].includes(a.state) || !Number.isSafeInteger(a.expectedWorkspaceRevision) || a.expectedWorkspaceRevision <= 0 || !Number.isSafeInteger(a.committedWorkspaceRevision) || a.committedWorkspaceRevision <= a.expectedWorkspaceRevision || a.committedWorkspaceRevision <= revision || a.committedWorkspaceRevision > view.workspaceRevision) throw new Error('原准备身份无效');
    if (a.assessment !== undefined && view.record.version !== 'workorch-preparation-record/v2') throw new Error('整体检查裁决版本无效');
    validateAssessment(a);
    if (a.definitionCompilation !== undefined) {
      const c = a.definitionCompilation;
      if (!c || view.record.version !== 'workorch-preparation-record/v2' || a.state !== 'blocked' || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || !a.assessment || !a.sealEvidenceRef || c.compilerVersion !== 'workorch-preparation-definition-compiler/v1' || c.snapshotHash !== a.binding.snapshotHash || c.manifestHash !== a.binding.manifestHash || typeof c.sourceInventoryHash !== 'string' || !c.sourceInventoryHash || c.sourceInventoryHash.length > 512 || typeof c.profileContractsHash !== 'string' || !c.profileContractsHash || c.profileContractsHash.length > 512 || !Number.isSafeInteger(c.sourceCount) || c.sourceCount < 2 || c.sourceCount > 4096 || [c.requirementCount, c.obligationCount, c.verificationCount].some(n => !Number.isSafeInteger(n) || n < 1 || n > 4096) || a.checks?.some(check => ['binding_changed', 'original_check_interrupted'].includes(check.reason))) throw new Error('封存来源编译身份或摘要无效');
    }
    seen.add(a.requestId);
    revision = a.committedWorkspaceRevision;
    if (view.record.version === 'workorch-preparation-record/v2') {
      if (!a.creationEpoch || !/^[0-9a-f]{32}$/.test(a.creationEpoch) || !Array.isArray(a.checks) || a.checks.length === 0 || a.checks.length > 128) throw new Error('准备检查清单缺失');
      let id = '';
      for (const c of a.checks) {
        if (c.id === 'provider_cache' && isProviderSelectionReason(c.reason) && (!a.providerCacheEvidenceRef || c.owner !== 'llm' || !c.required || a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || a.state !== 'blocked' || c.state !== 'blocked')) throw new Error('Provider 选择分类缺少原版本或证据，或伪造成功');
        if (c.id === 'mcp_selection' && mcpReasonRequiresEvidence(c.reason) && (!a.mcpRegistryEvidenceRef || c.owner !== 'mcpregistry' || !c.required)) throw new Error('MCP Registry 历史引用缺失或检查身份不匹配');
        if (c.id === 'mcp_selection' && isMCPSelectionReason(c.reason) && (a.evaluationVersion !== 'workorch-preparation-check-evaluation/v1' || a.state !== 'blocked' || c.state !== 'blocked')) throw new Error('MCP 选择裁决缺少版本或伪造成功');
        if (c.id === 'memory_selection' && memoryReasonRequiresEvidence(c.reason) && !a.memoryInventoryEvidenceRef) throw new Error('Memory 库存历史引用缺失');
        if (c.id === 'skill_selection' && c.reason.startsWith('skill_selection_') && (!a.skillSelectionState || c.reason !== `skill_selection_${a.skillSelectionState}`)) throw new Error('Skill 组合选择材料引用缺失或不一致');
 if (c.id === 'skill_selection' && c.reason === 'skill_registry_observed_selection_pending' && !a.skillRegistryEvidenceRef) throw new Error('Skill Registry 历史证据引用缺失');
        if (c.id === 'soul_selection' && c.reason === 'souls_observed_remaining_checks' && !a.soulEvidenceRef) throw new Error('Soul 历史证据引用缺失');
        if (!c.id || c.id <= id || !c.owner || typeof c.required !== 'boolean' || (a.state === 'accepted' || a.state === 'unknown' ? c.state !== 'pending' || c.reason !== 'not_observed' : !validEvaluatedCheck(a, c))) throw new Error('准备检查结果无效');
        id = c.id;
      }
    }
  }
  if (view.operationState !== attempts[attempts.length - 1].state) throw new Error('准备操作投影不一致');
}
