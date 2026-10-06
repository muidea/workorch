import { memo } from 'react';
import { Alert, Space, Typography } from 'antd';
import type { DefinitionAuthoringPreparationDiagnosticMetadata } from '../../types';
import { AuthoringPreparationDiagnosticDetails } from './AuthoringPreparationDiagnosticDetails';

export const AuthoringPreparationFailure = memo(function AuthoringPreparationFailure({ diagnostic }: {
  diagnostic: DefinitionAuthoringPreparationDiagnosticMetadata;
}) {
  return <Alert type="error" showIcon message={`${diagnostic.current ? '当前' : '历史'}前置准备失败 · ${diagnostic.stage}${diagnostic.reviewKind ? ` · ${diagnostic.reviewKind}` : ''} · ${diagnostic.phaseCode}`} description={<Space direction="vertical" style={{ width: '100%' }}>
    <Typography.Text>前置准备失败，尚未为该阶段取得 provider permit；修复后显式重试。此提示不表示其他阶段未调用 provider。</Typography.Text>
    <AuthoringPreparationDiagnosticDetails diagnostic={diagnostic} />
  </Space>} />;
}, (previous, next) => {
  const before = previous.diagnostic;
  const after = next.diagnostic;
  const keys = Object.keys(before) as (keyof DefinitionAuthoringPreparationDiagnosticMetadata)[];
  return keys.length === Object.keys(after).length && keys.every(key => before[key] === after[key]);
});
