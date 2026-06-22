export type ViewKey = 'board' | 'workspaceDetail' | 'governance' | 'runtimeControl' | 'configuration' | 'service';

// 分页限制
export const runtimeEventPageLimit = 100;
export const runtimeStepPageLimit = 100;
export const runtimeToolCallPageLimit = 100;
export const runtimeCheckpointPageLimit = 100;
export const runtimeResultPageLimit = 100;
export const runtimePatchPageLimit = 100;
export const runtimeValidationPageLimit = 100;
export const runtimeArtifactPageLimit = 100;
export const governancePageLimit = 100;

// 状态标签
export const stateLabels: Record<string, string> = {
  unknown: '未知',
  active: '活跃',
  created: '已创建',
  running: '运行中',
  validating: '验收中',
  completed: '已完成',
  accepted: '已接受',
  failed: '失败',
  cancelled: '已取消',
  superseded: '已替代',
  blocked_on_decision: '等待决策',
  blocked_on_approval: '等待审批',
  waiting_decision: '等待决策',
  waiting_approval: '等待审批',
  stalled: '停滞',
  open: '待处理',
  resolved: '已解决',
  pending: '待审批',
  approved: '已批准',
  rejected: '已拒绝',
  ready: '就绪',
  satisfied: '已满足'
};

// 进度类型标签
export const progressKindLabels: Record<string, string> = {
  step: '步骤',
  tool_call: '工具调用',
  patch: '补丁（Patch）',
  validation_result: '校验结果',
  result: '结果（Result）',
  artifact: '交付物（Artifact）',
  decision: '决策工单',
  approval: '审批工单'
};

// 恢复模式标签
export const recoveryModeLabels: Record<string, string> = {
  idle: '待启动',
  resume: '可恢复',
  wait_decision: '等待决策',
  wait_approval: '等待审批',
  inspect_failure: '排查失败',
  restart: '可重新启动',
  none: '无需恢复'
};

// 恢复操作标签
export const recoveryActionLabels: Record<string, string> = {
  start_workspace_execution: '提交工作空间执行',
  inspect_active_run: '查看活跃运行',
  resume_run: '恢复运行',
  resume_validation: '恢复验收',
  resolve_decision_ticket: '补充或处理决策工单',
  resolve_approval: '处理审批工单',
  inspect_result: '查看结果（Result）/交付物（Artifact）',
  inspect_failure_reason_and_evidence: '查看失败原因和运行证据',
  start_new_run: '启动新的运行'
};

// 修复循环状态标签
export const repairLoopStateLabels: Record<string, string> = {
  inactive: '未进入修复',
  repairing: '自动修复中',
  awaiting_decision: '等待决策',
  awaiting_approval: '等待审批',
  completed_after_repair: '修复后完成',
  escalated_decision: '已升级决策',
  terminated: '修复终止'
};

// Soul 绑定阶段
export const defaultSoulBindingStages = ['planning', 'execution', 'decision', 'approval', 'validation', 'artifact'];

export const soulBindingStageLabels: Record<string, string> = {
  planning: '规划',
  execution: '执行',
  decision: '决策',
  approval: '审批',
  validation: '验收',
  artifact: '交付物'
};

// 模型标签
export const modelLabels = {
  workspace: '工作空间（Workspace）',
  goal: '目标（Goal）',
  dod: '验收标准（DoD）',
  doDAssessment: 'DoD 验收评估',
  doDItem: 'DoD 验收项',
  run: '运行（Run）',
  decision: '决策工单（Decision）',
  approval: '审批工单（Approval）',
  result: '结果（Result）',
  artifact: '交付物（Artifact）',
  memory: '记忆（Memory）',
  memoryRecall: '记忆召回（Memory Recall）',
  memoryRecallTrace: '记忆召回轨迹',
  executionSection: '执行分区（Execution Section）',
  soul: '执行人格（Soul）',
  skill: '技能（Skill）',
  mcpServer: 'MCP 服务（MCP Server）',
  mcpCapability: 'MCP 能力（Tool / Resource / Prompt）',
  mcpTool: '工具（Tool）',
  mcpResource: '资源（Resource）',
  mcpPrompt: '提示词（Prompt）',
  step: '步骤（Step）',
  patch: '补丁（Patch）',
  toolCall: '工具调用（Tool Call）',
  validationResult: '校验结果（Validation Result）',
  runtimeCheckpoint: '运行检查点（Runtime Checkpoint）',
  llmProvider: 'LLM 服务商（Provider）',
  llmUsage: 'LLM 使用量',
  metrics: '运行指标（Metrics）'
};
