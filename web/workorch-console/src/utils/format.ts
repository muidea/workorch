import {
  stateLabels,
  progressKindLabels,
  recoveryModeLabels,
  repairLoopStateLabels,
  soulBindingStageLabels
} from './constants';

// 状态标签格式化
export function stateLabel(state?: string): string {
  if (!state) {
    return '未知';
  }
  return stateLabels[state] ? `${stateLabels[state]} (${state})` : state;
}

// Soul 绑定阶段标签
export function soulBindingStageLabel(stage: string): string {
  return soulBindingStageLabels[stage] ? `${soulBindingStageLabels[stage]} (${stage})` : stage;
}

// 进度类型标签
export function progressKindLabel(kind?: string): string {
  if (!kind) {
    return '未知';
  }
  return progressKindLabels[kind] ?? kind;
}

// 恢复模式标签
export function recoveryModeLabel(mode?: string): string {
  if (!mode) {
    return '未知';
  }
  return recoveryModeLabels[mode] ? `${recoveryModeLabels[mode]} (${mode})` : mode;
}

// 修复循环状态标签
export function repairLoopStateLabel(state?: string): string {
  if (!state) {
    return '未知';
  }
  return repairLoopStateLabels[state] ? `${repairLoopStateLabels[state]} (${state})` : state;
}

// 状态颜色映射
export function getStateColor(state?: string): string {
  const colorMap: Record<string, string> = {
    running: 'blue',
    completed: 'green',
    accepted: 'green',
    failed: 'red',
    cancelled: 'default',
    pending: 'orange',
    approved: 'green',
    rejected: 'red',
    open: 'orange',
    resolved: 'green',
    active: 'blue',
    ready: 'cyan',
    satisfied: 'green',
    validating: 'processing',
    blocked_on_decision: 'warning',
    blocked_on_approval: 'warning',
    waiting_decision: 'warning',
    waiting_approval: 'warning',
    stalled: 'red'
  };
  return colorMap[state || ''] || 'default';
}

// ID 截断显示
export function shortId(value?: string, keep = 10): string {
  if (!value) {
    return '-';
  }
  if (value.length <= keep) {
    return value;
  }
  return `${value.slice(0, keep)}...`;
}

// 错误消息提取
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

// 判断是否为 404 错误
export function isNotFoundError(error: unknown): boolean {
  return error instanceof Error && error.message.includes('404');
}

// JSON 格式化
export function compactJSON(value: unknown): string {
  if (value === undefined || value === null) {
    return '';
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

// 数字格式化
export function numberText(value?: number): string {
  if (value === undefined || value === null) {
    return '-';
  }
  return value.toLocaleString();
}

export function percentText(value?: number): string {
  if (value === undefined || value === null) {
    return '-';
  }
  return `${(value * 100).toFixed(1)}%`;
}

// 按 ID 合并数组（用于分页追加）
export function mergeById<T extends { id: string }>(current: T[], items: T[], append: boolean): T[] {
  if (!append) {
    return items;
  }
  const seen = new Set(current.map((item) => item.id));
  return [
    ...current,
    ...items.filter((item) => {
      if (seen.has(item.id)) {
        return false;
      }
      seen.add(item.id);
      return true;
    })
  ];
}

// 将未知值转换为字符串数组
export function asStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.map(String).filter(Boolean) : [];
}

// 任务进度步骤状态映射
export function taskProgressStepStatus(state?: string): 'wait' | 'process' | 'finish' | 'error' {
  if (state === 'failed') {
    return 'error';
  }
  if (state === 'completed') {
    return 'finish';
  }
  if (state === 'running' || state === 'blocked') {
    return 'process';
  }
  return 'wait';
}
