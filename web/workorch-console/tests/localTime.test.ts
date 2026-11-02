import assert from 'node:assert/strict';
import test from 'node:test';
import { formatLocalDateTime } from '../src/utils/time.ts';

test('时间按当前时区转换，支持跨日、源偏移及夏令时', () => {
  const original = process.env.TZ;
  try {
    for (const [zone, input, expected] of [
      ['Asia/Shanghai', '2026-11-01T18:30:45.123456789Z', '2026-11-02 02:30:45 +08:00'],
      ['Asia/Shanghai', '2026-11-02T08:00:00+02:00', '2026-11-02 14:00:00 +08:00'],
      ['Asia/Kathmandu', '2026-11-02T00:00:00Z', '2026-11-02 05:45:00 +05:45'],
      ['America/St_Johns', '2026-11-02T00:00:00Z', '2026-11-01 20:30:00 -03:30'],
      ['America/New_York', '2026-07-01T02:15:00Z', '2026-06-30 22:15:00 -04:00'],
      ['America/New_York', '2026-11-02T02:15:00Z', '2026-11-01 21:15:00 -05:00'],
      ['UTC', '2026-11-02T00:00:00Z', '2026-11-02 00:00:00 +00:00'],
    ]) {
      process.env.TZ = zone;
      assert.equal(formatLocalDateTime(input), expected);
    }
  } finally {
    if (original === undefined) delete process.env.TZ;
    else process.env.TZ = original;
  }
});

test('缺少时间使用占位，无时区或不可解析的原文不猜测、不截断', () => {
  assert.equal(formatLocalDateTime(), '-');
  assert.equal(formatLocalDateTime(''), '-');
  for (const value of ['2026-11-02', '2026-11-02T09:00:00', '  unknown  ', '2026-99-02T09:00:00Z']) {
    assert.equal(formatLocalDateTime(value), value);
  }
});
