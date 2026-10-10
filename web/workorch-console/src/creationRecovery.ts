export type CreationRecovery = {
  workspaceId: string;
  requestId: string;
  persistence: 'unknown' | 'persisted';
  phase: 'owner_persistence' | 'owner_receipt' | 'directory_preparation' | 'authoring_read';
  retrySameRequest: true;
};

export class WorkorchRequestError extends Error {
  readonly creationRecovery?: CreationRecovery;

  constructor(message: string, creationRecovery?: CreationRecovery) {
    super(message);
    this.creationRecovery = creationRecovery;
  }
}

// 恢复提示不能替代创建成功响应；页面仍须重放原请求，补齐目录准备。
export function creationRecoveryFromResponse(body: unknown): CreationRecovery | undefined {
  if (!body || typeof body !== 'object') return undefined;
  const response = body as { code?: unknown; creationRecovery?: unknown };
  if (response.code !== 'workspace_creation_incomplete' || !response.creationRecovery || typeof response.creationRecovery !== 'object') return undefined;
  const value = response.creationRecovery as Partial<CreationRecovery>;
  if (typeof value.workspaceId !== 'string' || !value.workspaceId || typeof value.requestId !== 'string' || !value.requestId || !['unknown', 'persisted'].includes(value.persistence || '') || !['owner_persistence', 'owner_receipt', 'directory_preparation', 'authoring_read'].includes(value.phase || '') || value.retrySameRequest !== true) return undefined;
  return value as CreationRecovery;
}
