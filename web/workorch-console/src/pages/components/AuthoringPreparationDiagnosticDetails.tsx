import { memo, useEffect, useRef, useState } from 'react';
import { Alert, Button, Space, Typography } from 'antd';
import { workorchApi } from '../../api';
import type { DefinitionAuthoringPreparationDiagnostic, DefinitionAuthoringPreparationDiagnosticMetadata } from '../../types';

type Props = { diagnostic: DefinitionAuthoringPreparationDiagnosticMetadata };
type VerifiedCause = DefinitionAuthoringPreparationDiagnostic & { bytes: Uint8Array; text?: string };

const metadataKeys: (keyof DefinitionAuthoringPreparationDiagnosticMetadata)[] = [
  'diagnosticId', 'workspaceId', 'inputHash', 'templateSnapshotHash', 'observedWorkspaceRevision',
  'stage', 'targetRequestId', 'derivationPermitId', 'reviewKind', 'proposalHash', 'phaseCode',
  'resourceRef', 'providerBindingHash', 'causeSHA256', 'causeBytes', 'causeSaved', 'notSavedReason',
];

function metadataMatches(actual: DefinitionAuthoringPreparationDiagnosticMetadata, expected: DefinitionAuthoringPreparationDiagnosticMetadata): boolean {
  return metadataKeys.every(key => actual[key] === expected[key]);
}

function decodeBase64(base64: string): Uint8Array {
  if (!/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(base64)) throw new Error('Base64 格式无效，前置原因完整性校验失败。');
  let binary: string;
  try { binary = atob(base64); }
  catch { throw new Error('Base64 解码失败，前置原因完整性校验失败。'); }
  const bytes = new Uint8Array(new ArrayBuffer(binary.length));
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  return bytes;
}

async function verifyCause(diagnostic: DefinitionAuthoringPreparationDiagnostic, expected: DefinitionAuthoringPreparationDiagnosticMetadata): Promise<VerifiedCause> {
  if (!metadataMatches(diagnostic, expected)) throw new Error('前置诊断的 workspace、diagnostic、input、template、target、stage 或 phase 身份不匹配，已拒绝显示。');
  if (!diagnostic.causeSaved) throw new Error(diagnostic.notSavedReason || '原始前置原因未保存，无法加载。');
  const bytes = decodeBase64(diagnostic.causeBase64 ?? '');
  if (bytes.byteLength !== diagnostic.causeBytes) throw new Error(`前置原因字节数不匹配（收到 ${bytes.byteLength}，记录 ${diagnostic.causeBytes}），完整性校验失败。`);
  if (!globalThis.crypto?.subtle) throw new Error('浏览器不支持 SHA-256 校验，不能确认前置原因完整性。');
  const digest = await globalThis.crypto.subtle.digest('SHA-256', bytes.buffer as ArrayBuffer);
  const actualHash = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('');
  if (actualHash.toLowerCase() !== diagnostic.causeSHA256.toLowerCase()) throw new Error(`前置原因 SHA-256 不匹配（收到 ${actualHash}，记录 ${diagnostic.causeSHA256}），完整性校验失败。`);
  let text: string | undefined;
  try { text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes); }
  catch { /* 保留经校验的原字节供下载；不以替换字符伪造原文。 */ }
  return { ...diagnostic, bytes, text };
}

function downloadCause(record: VerifiedCause): void {
  const blob = new Blob([record.bytes.buffer as ArrayBuffer], { type: 'application/octet-stream' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `authoring-preparation-${record.diagnosticId}.bin`;
  anchor.click();
  URL.revokeObjectURL(url);
}

export const AuthoringPreparationDiagnosticDetails = memo(function AuthoringPreparationDiagnosticDetails({ diagnostic }: Props) {
  const scopeKey = JSON.stringify(metadataKeys.map(key => diagnostic[key]));
  const scope = useRef(scopeKey);
  scope.current = scopeKey;
  const generation = useRef(0);
  const [loading, setLoading] = useState(false);
  const [record, setRecord] = useState<VerifiedCause>();
  const [recordScope, setRecordScope] = useState('');
  const [error, setError] = useState('');
  const [errorScope, setErrorScope] = useState('');
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    generation.current++;
    setLoading(false);
    setRecord(undefined);
    setRecordScope('');
    setError('');
    setErrorScope('');
    setExpanded(false);
    return () => { generation.current++; };
  }, [scopeKey]);

  const load = async () => {
    const token = ++generation.current;
    const requestedScope = scopeKey;
    setLoading(true);
    setError('');
    try {
      const raw = await workorchApi.readAuthoringPreparationDiagnostic(diagnostic.workspaceId, diagnostic.diagnosticId);
      if (generation.current !== token || scope.current !== requestedScope) return;
      const verified = await verifyCause(raw, diagnostic);
      if (generation.current !== token || scope.current !== requestedScope) return;
      setRecord(verified);
      setRecordScope(requestedScope);
      setExpanded(true);
    } catch (cause) {
      if (generation.current === token && scope.current === requestedScope) {
        setError(cause instanceof Error ? cause.message : String(cause));
        setErrorScope(requestedScope);
      }
    } finally {
      if (generation.current === token && scope.current === requestedScope) setLoading(false);
    }
  };

  const showRecord = expanded && recordScope === scopeKey;
  return <Space direction="vertical" style={{ width: '100%' }} size="small">
    <Typography.Text strong>{diagnostic.stage}{diagnostic.reviewKind ? ` · ${diagnostic.reviewKind}` : ''} · {diagnostic.phaseCode}</Typography.Text>
    <Typography.Text type="secondary">
      diagnostic {diagnostic.diagnosticId} · target {diagnostic.targetRequestId} · revision {diagnostic.observedWorkspaceRevision} · input {diagnostic.inputHash} · template {diagnostic.templateSnapshotHash}
      {diagnostic.derivationPermitId ? ` · derivation permit ${diagnostic.derivationPermitId}` : ''}
      {diagnostic.proposalHash ? ` · proposal ${diagnostic.proposalHash}` : ''}
      {diagnostic.resourceRef ? ` · resource ${diagnostic.resourceRef}` : ''}
      {diagnostic.providerBindingHash ? ` · provider binding ${diagnostic.providerBindingHash}` : ''}
    </Typography.Text>
    <Typography.Text type="secondary">前置原因 SHA-256 {diagnostic.causeSHA256} · {diagnostic.causeBytes} 字节。{diagnostic.causeSaved ? '原始原因已保存。' : `原始原因未保存：${diagnostic.notSavedReason || '原因未提供'}。`}</Typography.Text>
    {diagnostic.causeSaved && <Button size="small" loading={loading} onClick={() => { if (showRecord) { setExpanded(false); return; } void load(); }}>
      {showRecord ? '收起前置原因' : '按需加载并校验前置原因'}
    </Button>}
    {error && errorScope === scopeKey && <Alert type="error" showIcon message="前置原因不可用或完整性校验失败" description={error} />}
    {showRecord && record && record.text === undefined && <Alert type="warning" showIcon message="前置原因不是有效 UTF-8 文本；已验证原字节可下载，界面不进行字符替换。" />}
    {showRecord && record && record.bytes.byteLength === 0 && <Typography.Text type="secondary">原始原因为空（0 字节）；下载内容保留为空原始字节。</Typography.Text>}
    {showRecord && record && record.text !== undefined && record.text.length > 0 && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 320, overflow: 'auto', margin: 0 }}>{record.text}</pre>}
    {showRecord && record && <Button size="small" onClick={() => downloadCause(record)}>下载已校验原始字节</Button>}
  </Space>;
}, (previous, next) => metadataMatches(previous.diagnostic, next.diagnostic));
