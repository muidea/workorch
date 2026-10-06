export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '操作失败';
}
