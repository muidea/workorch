import { Collapse } from 'antd';
import { inputReviewFeedbackLines } from '../runtimeProgressPresentation';
import type { RuntimeOriginalDiagnostic } from '../types';

export function RuntimeInputReviewDetails({ diagnostic }: { diagnostic?: RuntimeOriginalDiagnostic }) {
  const lines = inputReviewFeedbackLines(diagnostic);
  if (!lines.length) return null;
  return <Collapse size="small" items={[{
    key: 'input-review',
    label: '输入评审详细报告（非验收结论）',
    children: <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', margin: 0 }}>{lines.join('\n')}</pre>,
  }]} />;
}
