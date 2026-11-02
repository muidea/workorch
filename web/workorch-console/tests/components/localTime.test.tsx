import { fireEvent, render, screen } from '@testing-library/react';
import { Descriptions } from 'antd';
import { describe, expect, it } from 'vitest';
import {
  convergenceStatusDescriptions, llmUsageCard, projectionStatusDescriptions,
  runHeartbeatDescriptions, timelineOccurredAtContent, webSourceCard,
} from '../../src/helpers';
import { LLMUsageCard } from '../../src/components/cards/LLMUsageCard';
import { WebSourceCard } from '../../src/components/cards/WebSourceCard';
import { formatLocalDateTime } from '../../src/utils/time';
import type { LLMUsageSummary, LLMUsageTrace, WebSourceTrace } from '../../src/types';

const timestamp = '2026-11-01T18:30:45Z';
const local = formatLocalDateTime(timestamp);

describe('本地时区时间展示', () => {
  it('投影、心跳、收敛和时间线使用格式化值，原时间不变', () => {
    const heartbeat = { runId: 'run-1', latestActivityAt: timestamp, latestEffectiveProgressAt: timestamp, latestEventAt: timestamp };
    render(<>
      <Descriptions items={projectionStatusDescriptions({ state: 'current', globalRevision: 1, updatedAt: timestamp })} />
      <Descriptions items={runHeartbeatDescriptions(heartbeat)} />
      <Descriptions items={convergenceStatusDescriptions({ runId: 'run-1', lastProgressAt: timestamp })} />
      <span>{timelineOccurredAtContent({ source: 'runtime', type: 'evidence', occurredAt: timestamp })}</span>
    </>);
    expect(screen.getAllByText(local).length).toBe(6);
    expect(screen.queryByText(timestamp)).toBeNull();
    expect(heartbeat.latestActivityAt).toBe(timestamp);
  });

  const sources: WebSourceTrace[] = [{ searchedAt: timestamp, sources: [{ url: 'https://example.invalid/source' }] }];
  for (const surface of ['card', 'detail'] as const) {
    it(`${surface} 来源时间按本地时区展示`, () => {
      render(surface === 'card' ? <WebSourceCard items={sources} /> : webSourceCard(sources));
      expect(screen.getByText(local)).toBeTruthy();
      expect(screen.queryByText(timestamp)).toBeNull();
      expect(sources[0].searchedAt).toBe(timestamp);
    });
  }

  const summary: LLMUsageSummary = {
    availability: 'complete', scope: 'run', recordLimit: 200, observedRecords: 1,
    entryTotal: 1, inputTokens: 1, outputTokens: 1, totalTokens: 2,
    estimatedEntries: 0, providerNativeEntries: 1, failedEntries: 0,
  };
  const traces: LLMUsageTrace[] = [{
    id: 'usage-1', workspaceId: 'ws-1', runId: 'run-1', estimated: false,
    physicalResponseMode: 'non_streaming', providerSubmitted: true,
    startedAt: timestamp, completedAt: timestamp,
  }];
  for (const surface of ['card', 'detail'] as const) {
    it(`${surface} LLM详情格式化开始和结束时间`, () => {
      render(surface === 'card' ? <LLMUsageCard summary={summary} traces={traces} /> : llmUsageCard(summary, traces));
      fireEvent.click(screen.getByRole('tab', { name: '调用明细 1' }));
      fireEvent.click(screen.getByRole('button', { name: 'Expand row' }));
      expect(screen.getByText(surface === 'card' ? `${local} / ${local}` : `${local} → ${local}`)).toBeTruthy();
      expect(traces[0].startedAt).toBe(timestamp);
    });
  }
});
