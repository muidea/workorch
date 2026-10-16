import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { RuntimeValidationOutputLinks } from '../../src/components/RuntimeValidationOutputLinks';
import type { RuntimeAuditOperationReceipt } from '../../src/types';

it('按服务端原回执显示下载链接、保存范围和两种未保存原因，不推定完成', () => {
  const outputs: RuntimeAuditOperationReceipt['validationOutputs'] = [
    { stageId: 'test', stream: 'stdout', bytes: 100, savedBytes: 64, sha256: 'original-hash', complete: false, omittedReason: 'capacity_exceeded', ref: 'workorch://exact-output' },
    { stageId: 'test', stream: 'stderr', bytes: 5, savedBytes: 0, sha256: 'stderr-hash', complete: true, storageOmittedReason: 'storage_unavailable' }
  ];
  const before = JSON.stringify(outputs);
  render(<RuntimeValidationOutputLinks outputs={outputs} />);
  const link = screen.getByRole('link', { name: 'test / stdout' });
  expect(link.getAttribute('href')).toBe('/api/v1/workorch/runtime/payloads?ref=workorch%3A%2F%2Fexact-output');
  expect(screen.getByText('已保存 64 / 100 字节（不完整） · capacity_exceeded')).toBeTruthy();
  expect(screen.getByText('test / stderr：未保存')).toBeTruthy();
  expect(screen.getByText('已保存 0 / 5 字节（不完整） · storage_unavailable')).toBeTruthy();
  expect(screen.getAllByRole('link')).toHaveLength(1);
  expect(JSON.stringify(outputs)).toBe(before);
});
