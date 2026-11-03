import { render, cleanup } from '@testing-library/react';
import { describe, expect, it, afterEach } from 'vitest';
import { TaskProgressCard } from '../../src/components/cards/TaskProgressCard';
import { taskProgressCard } from '../../src/helpers';
import type { TaskProgress } from '../../src/types';

afterEach(cleanup);
const fixture = (enabled: boolean, count: number): TaskProgress => ({
  workspaceId: 'ws', state: 'active', recovery: { mode: 'auto' }, repairLoop: { state: 'inactive', attemptCount: 0, escalated: false },
  runtime: { implementationWork: {
    workRoundsUsed: 12, inputRevisionsUsed: count, inputRevisionEnabled: enabled,
    repeatedWorkStates: 0, repeatedStateLimit: 2, reservedRounds: 12, completedRounds: 12,
    committedReceipts: 12, unknownExecutions: 0, semanticGenerations: 24, archivedSegments: 1,
  } },
});

describe('输入修订累计展示', () => {
  for (const variant of ['card', 'helper']) {
    for (const count of [0, 12]) {
      it(`${variant} 显示累计 ${count} 次，零次不表示未启用`, () => {
        const progress = fixture(true, count);
        const { container } = render(variant === 'card' ? <TaskProgressCard progress={progress} /> : taskProgressCard(progress));
        expect(container.textContent).toContain(`输入修订 累计 ${count} 次`);
        expect(container.textContent).not.toContain('输入修订 未启用');
        expect(container.textContent).not.toContain(`${count}/4`);
        expect(container.textContent).toContain('工作回执不代表验收通过');
      });
    }
    it(`${variant} 由协议事实表示未启用`, () => {
      const progress = fixture(false, 0);
      const { container } = render(variant === 'card' ? <TaskProgressCard progress={progress} /> : taskProgressCard(progress));
      expect(container.textContent).toContain('输入修订 未启用');
    });
  }
});
