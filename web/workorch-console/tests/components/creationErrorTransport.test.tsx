import { afterEach, describe, expect, it, vi } from 'vitest';
import { workorchApi } from '../../src/api';
import { WorkorchRequestError } from '../../src/creationRecovery';

describe('创建错误 transport 保留明确阶段而非按 status 猜测', () => {
  afterEach(() => vi.unstubAllGlobals());
  it.each([undefined, 'workspace_creation_rejected_before_dispatch'])('400 code=%s 精确传递', async code => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: '拒绝', code }), { status: 400 })));
    try {
      await workorchApi.createDefinitionAuthoring({ requestId: 'request', title: '任务', templateId: 't', input: { goalText: '目标' } });
      throw new Error('must reject');
    } catch (error) {
      expect(error).toBeInstanceOf(WorkorchRequestError);
      expect((error as WorkorchRequestError).status).toBe(400);
      expect((error as WorkorchRequestError).creationRejectedBeforeDispatch).toBe(code !== undefined);
    }
  });
});
