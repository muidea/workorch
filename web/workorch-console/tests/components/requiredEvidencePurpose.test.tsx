import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { AuthoringRequiredEvidencePurposes } from '../../src/pages/components/AuthoringRequiredEvidencePurposes';
import type { DefinitionAuthoringView } from '../../src/types';

const proposal = (acceptanceItemRef: string): NonNullable<DefinitionAuthoringView['proposal']> => ({
  goalText: '原目标', version: 2, goalItemRef: 'goal', goalTextHash: 'hash', dimensions: [], requirements: [], obligations: [], proposalHash: 'proposal', derivationPermitId: 'permit',
  items: [{ ref: 'original-dod', text: '原 DoD 全文\n不弱化', sourceKind: 'USER_EXPLICIT' }],
  requiredEvidencePurposes: [{ contractRef: 'runtime/profile-deliverable-evidence-purpose/v2', historicalItemRef: 'profile/neutral/deliverable/initial', acceptanceItemRef, historicalProfileId: 'neutral', historicalDeliverableId: 'initial', historicalProfileHash: 'exact-profile-hash', historicalText: ' 完整交付原文\n含全部要求 ' }],
});
it('显示必需关系缺项，不凭空选择原 DoD', () => {
  const value = proposal(''); const before = JSON.stringify(value);
  render(<AuthoringRequiredEvidencePurposes proposal={value} />);
  expect(screen.getByText(/缺少原 DoD 验收来源/)).toBeTruthy();
  expect(screen.getByText(/缺项阻止确认/)).toBeTruthy();
  expect(JSON.stringify(value)).toBe(before);
});
it('展示完整 Profile 快照、原文和独立验收原文，声明不冒充执行 credit', () => {
  const value = proposal('original-dod'); const { container } = render(<AuthoringRequiredEvidencePurposes proposal={value} />);
  expect(screen.getByText(/exact-profile-hash/)).toBeTruthy();
  expect(Array.from(container.querySelectorAll('pre')).map(node => node.textContent)).toEqual([' 完整交付原文\n含全部要求 ', '原 DoD 全文\n不弱化']);
  expect(screen.getByText(/不代表工作或终验完成/)).toBeTruthy();
  expect(screen.queryByText(/缺少原 DoD 验收来源/)).toBeNull();
});
it('其它 Profile 未声明该关系时不输出无关要求', () => {
  const { container } = render(<AuthoringRequiredEvidencePurposes proposal={{ ...proposal(''), requiredEvidencePurposes: undefined }} />);
  expect(container.textContent).toBe('');
});
