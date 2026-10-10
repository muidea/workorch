import { render, screen } from '@testing-library/react';
import { ConfigProvider } from 'antd';
import { expect, it } from 'vitest';
import { RuntimeSourceObligationDetails } from '../../src/components/RuntimeSourceObligationDetails';
import type { RuntimeResultCloseout } from '../../src/types';

it('共享 CWI 分开显示 A/B，保持原判据 bytes，不在页面补发完成结论', () => {
  const closeout: RuntimeResultCloseout = {
    CloseoutID: 'closeout', StartupInputHash: 'startup', DefinitionRevision: 'definition', CloseoutHash: 'hash',
    RequirementOutcomes: [{ CoverageWorkItemID: 'shared', SourceObligations: ['A', 'B'].map((id) => ({
      Check: { InventoryHash: 'inventory', ObligationID: id, RequirementID: 'requirement', VerificationID: `verify-${id}`, Criterion: ` ${id} 原判据\n `, Declaration: ` 验证 ${id}\t `, CheckRef: `check-${id}` },
      CoverageState: id === 'A' ? 'covered' : 'uncovered', AssessmentState: id === 'A' ? 'satisfied' : 'unassessed', AssessmentReasonCode: '', AcceptedEvidenceRefs: id === 'A' ? ['evidence-A'] : [],
      CurrentBindingRef: { BindingID: 'binding', BindingHash: 'binding-hash', FocusID: 'focus', SemanticManifestHash: 'manifest' }
    })) }]
  };
  const before = JSON.stringify(closeout);
  const { container } = render(<ConfigProvider theme={{ token: { motion: false } }}><RuntimeSourceObligationDetails closeout={closeout} /></ConfigProvider>);
  expect(screen.getByText('A / verify-A')).toBeTruthy();
  expect(screen.getByText('B / verify-B')).toBeTruthy();
  expect(screen.getByText('covered / satisfied')).toBeTruthy();
  expect(screen.getByText('uncovered / unassessed')).toBeTruthy();
  const spans = Array.from(container.querySelectorAll('span'));
  expect(spans.some((span) => span.textContent === ' A 原判据\n ')).toBe(true);
  expect(spans.some((span) => span.textContent === ' 验证 B\t ')).toBe(true);
  expect(JSON.stringify(closeout)).toBe(before);
});

it('缺少服务端逐源结论时不从共同 DoD 或证据猜测源义务', () => {
  const { container } = render(<RuntimeSourceObligationDetails closeout={{ CloseoutID: 'closeout', StartupInputHash: 'startup', DefinitionRevision: 'definition', CloseoutHash: 'hash', RequirementOutcomes: [{ CoverageWorkItemID: 'shared' }] }} />);
  expect(container.textContent).toBe('');
});
