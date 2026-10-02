import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Space, Typography } from 'antd';
import { workorchApi } from '../../api';
import type { AuthoringDiagnostic, AuthoringDiagnosticMetadata } from '../../types';

type Props = {
  workspaceId: string;
  permitId?: string;
  phase: string;
  failureCode?: string;
  diagnostic?: AuthoringDiagnosticMetadata;
};

type VerifiedDiagnostic = AuthoringDiagnostic & { bytes: Uint8Array };

function decodeAndVerify(diagnostic: AuthoringDiagnostic): VerifiedDiagnostic {
  if (!diagnostic.payloadSaved) throw new Error(diagnostic.notSavedReason || '原始响应未保存，无法加载。');
  if (!/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(diagnostic.payloadBase64)) {
    throw new Error('Base64 格式无效，原始响应完整性校验失败。');
  }
  let binary: string;
  try { binary = atob(diagnostic.payloadBase64); }
  catch { throw new Error('Base64 解码失败，原始响应完整性校验失败。'); }
  const bytes = new Uint8Array(new ArrayBuffer(binary.length));
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  if (bytes.byteLength !== diagnostic.payloadBytes) {
    throw new Error(`原始响应字节数不匹配（收到 ${bytes.byteLength}，记录 ${diagnostic.payloadBytes}），完整性校验失败。`);
  }
  return { ...diagnostic, bytes } as VerifiedDiagnostic;
}

function verifyParseError(diagnostic: AuthoringDiagnosticMetadata): void {
  if (diagnostic.parseError === undefined) throw new Error('已保存的解析错误缺少原文，完整性校验失败。');
  const bytes = new TextEncoder().encode(diagnostic.parseError);
  if (bytes.byteLength !== diagnostic.parseErrorBytes) {
    throw new Error(`解析错误字节数不匹配（收到 ${bytes.byteLength}，记录 ${diagnostic.parseErrorBytes}），完整性校验失败。`);
  }
}

async function verifyHash(bytes: Uint8Array, expected: string, label: string): Promise<void> {
  if (!globalThis.crypto?.subtle) throw new Error('浏览器不支持 SHA-256 校验，不能确认诊断完整性。');
  const digest = await globalThis.crypto.subtle.digest('SHA-256', bytes.buffer as ArrayBuffer);
  const actual = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('');
  if (actual.toLowerCase() !== expected.toLowerCase()) {
    throw new Error(`${label} SHA-256 不匹配（收到 ${actual}，记录 ${expected}），完整性校验失败。`);
  }
}

async function verifyDiagnosticIntegrity(diagnostic: AuthoringDiagnosticMetadata): Promise<void> {
  if (!diagnostic.parseErrorSaved) return;
  verifyParseError(diagnostic);
  await verifyHash(new TextEncoder().encode(diagnostic.parseError!), diagnostic.parseErrorSHA256, '解析错误');
}

function matchesMetadata(actual: AuthoringDiagnosticMetadata, expected: AuthoringDiagnosticMetadata): boolean {
  return actual.parseError === expected.parseError
    && actual.parseErrorSHA256 === expected.parseErrorSHA256
    && actual.parseErrorBytes === expected.parseErrorBytes
    && actual.parseErrorSaved === expected.parseErrorSaved
    && actual.payloadSHA256 === expected.payloadSHA256
    && actual.payloadBytes === expected.payloadBytes
    && actual.payloadSaved === expected.payloadSaved
    && actual.notSavedReason === expected.notSavedReason;
}

async function verifyPayloadHash(record: VerifiedDiagnostic): Promise<void> {
  await verifyHash(record.bytes, record.payloadSHA256, '原始响应');
}

export function AuthoringDiagnosticDetails({ workspaceId, permitId, phase, failureCode, diagnostic }: Props) {
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [record, setRecord] = useState<VerifiedDiagnostic>();
  const [rawText, setRawText] = useState<string>();
  const [loadError, setLoadError] = useState('');
  const [displayError, setDisplayError] = useState('');
  const generation = useRef(0);
  const displayCheck = useRef(0);

  useEffect(() => {
    generation.current++;
    setExpanded(false);
    setLoading(false);
    setRecord(undefined);
    setRawText(undefined);
    setLoadError('');
    setDisplayError('');
    const check = ++displayCheck.current;
    if (diagnostic?.parseErrorSaved) {
      void verifyDiagnosticIntegrity(diagnostic).catch(error => {
        if (displayCheck.current === check) setDisplayError(error instanceof Error ? error.message : String(error));
      });
    }
    return () => { generation.current++; displayCheck.current++; };
  }, [workspaceId, permitId, phase, failureCode, diagnostic?.parseError, diagnostic?.parseErrorSHA256, diagnostic?.parseErrorBytes, diagnostic?.parseErrorSaved, diagnostic?.payloadSHA256, diagnostic?.payloadBytes, diagnostic?.payloadSaved, diagnostic?.notSavedReason]);

  const load = async () => {
    if (!permitId || loading) return;
    const token = ++generation.current;
    setLoading(true);
    setLoadError('');
    try {
      const result = await workorchApi.readAuthoringDiagnostic(workspaceId, permitId);
      if (generation.current !== token || result.workspaceId !== workspaceId || result.permitId !== permitId) {
        if (generation.current === token) setLoadError('诊断响应的 workspace 或 permit 身份不匹配，已拒绝显示。');
        return;
      }
      await verifyDiagnosticIntegrity(result);
      if (result.phase !== phase || result.failureCode !== (failureCode || '')) throw new Error('诊断的阶段或失败码与失败记录不匹配，已拒绝显示。');
      if (diagnostic && !matchesMetadata(result, diagnostic)) throw new Error('诊断元数据与失败记录不匹配，已拒绝显示。');
      const verified = decodeAndVerify(result);
      await verifyPayloadHash(verified);
      let decoded: string | undefined;
      try { decoded = new TextDecoder('utf-8', { fatal: true }).decode(verified.bytes); }
      catch { /* 保留已校验原字节供下载，不以替换字符冒充原文。 */ }
      if (generation.current !== token) return;
      setRecord(verified);
      setRawText(decoded);
      setExpanded(true);
    } catch (error) {
      if (generation.current === token) setLoadError(error instanceof Error ? error.message : String(error));
    } finally {
      if (generation.current === token) setLoading(false);
    }
  };

  const download = () => {
    if (!record) return;
    const blob = new Blob([record.bytes.buffer as ArrayBuffer], { type: 'application/octet-stream' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `authoring-${permitId}-response.bin`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  return <Space direction="vertical" style={{ width: '100%' }} size="small">
    <Typography.Text strong>{failureCode || '请求失败'} · {phase}</Typography.Text>
    {diagnostic ? <>
      {displayError && <Alert type="error" showIcon message="解析错误诊断完整性校验失败；下方保留服务端返回的原文，但不能视为已校验记录。" description={displayError} />}
      {diagnostic.parseErrorSaved
        ? <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', margin: 0 }}>{diagnostic.parseError ?? ''}</pre>
        : <Typography.Text>原始解析错误未保存（{diagnostic.parseErrorBytes} 字节）；{diagnostic.notSavedReason || '原因未提供'}。</Typography.Text>}
      <Typography.Text type="secondary">
        解析错误 SHA-256 {diagnostic.parseErrorSHA256} · {diagnostic.parseErrorBytes} 字节；响应 SHA-256 {diagnostic.payloadSHA256} · {diagnostic.payloadBytes} 字节。
        {diagnostic.payloadSaved ? '原始响应已保存。' : `原始响应未保存：${diagnostic.notSavedReason || '原因未提供'}。`}
      </Typography.Text>
      {permitId && diagnostic.payloadSaved && <Button size="small" loading={loading} onClick={() => { if (expanded) { setExpanded(false); return; } void load(); }}>
        {expanded ? '收起原始响应' : '按需加载并校验原始响应'}
      </Button>}
      {loadError && <Alert type="error" showIcon message="原始响应不可用或完整性校验失败" description={loadError} />}
      {expanded && record && rawText === undefined && <Alert type="warning" showIcon message="原始响应不是有效 UTF-8 文本；已验证原字节可下载，界面不进行字符替换。" />}
      {expanded && record && rawText !== undefined && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 320, overflow: 'auto', margin: 0 }}>{rawText}</pre>}
      {expanded && record && <Button size="small" onClick={download}>下载已校验原始字节</Button>}
    </> : <Typography.Text>此旧失败记录没有保存诊断；原始原因未记录，且不会重新调用 provider。</Typography.Text>}
  </Space>;
}
