import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Space, Typography } from 'antd';
import { workorchApi } from '../../api';
import type { AuthoringDiagnostic, AuthoringDiagnosticMetadata, ProviderObservedContent, ProviderObservedContentMetadata } from '../../types';

type Props = {
  workspaceId: string;
  permitId?: string;
  phase: string;
  failureCode?: string;
  diagnostic?: AuthoringDiagnosticMetadata;
};

type VerifiedDiagnostic = AuthoringDiagnostic & { bytes?: Uint8Array; text?: string };
type VerifiedProviderContent = ProviderObservedContent & { bytes: Uint8Array; text?: string };

function decodeAndVerify(diagnostic: AuthoringDiagnostic): VerifiedDiagnostic {
  if (!diagnostic.payloadSaved) throw new Error(diagnostic.notSavedReason || '解析器输入 payload未保存，无法加载。');
  if (!/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(diagnostic.payloadBase64)) {
    throw new Error('Base64 格式无效，解析器输入 payload完整性校验失败。');
  }
  let binary: string;
  try { binary = atob(diagnostic.payloadBase64); }
  catch { throw new Error('Base64 解码失败，解析器输入 payload完整性校验失败。'); }
  const bytes = new Uint8Array(new ArrayBuffer(binary.length));
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  if (bytes.byteLength !== diagnostic.payloadBytes) {
    throw new Error(`解析器输入 payload字节数不匹配（收到 ${bytes.byteLength}，记录 ${diagnostic.payloadBytes}），完整性校验失败。`);
  }
  return { ...diagnostic, bytes } as VerifiedDiagnostic;
}

function decodeObservedContent(content: ProviderObservedContentMetadata, contentBase64?: string, label = 'provider 原始内容'): VerifiedProviderContent {
  if (!content.observed) throw new Error(`${label}未观察，不可按完整内容展示或下载。`);
  if (!content.saved) throw new Error(content.reason || `${label}已观察但未保存，无法下载。`);
  if (contentBase64 === undefined || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(contentBase64)) {
    throw new Error(`${label} Base64 格式无效，完整性校验失败。`);
  }
  let binary: string;
  try { binary = atob(contentBase64); }
  catch { throw new Error(`${label} Base64 解码失败，完整性校验失败。`); }
  const bytes = new Uint8Array(new ArrayBuffer(binary.length));
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  if (bytes.byteLength !== content.observedBytes) {
    throw new Error(`${label} 字节数不匹配（收到 ${bytes.byteLength}，记录 ${content.observedBytes}），完整性校验失败。`);
  }
  let text: string | undefined;
  try { text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes); }
  catch { /* 原始 bytes 保持可下载；无效 UTF-8 不做替换解码。 */ }
  return { ...content, contentBase64, bytes, text };
}

async function verifyObservedContent(content: ProviderObservedContentMetadata, contentBase64: string | undefined, label: string): Promise<VerifiedProviderContent> {
  const verified = decodeObservedContent(content, contentBase64, label);
  await verifyHash(verified.bytes, content.sha256, label);
  return verified;
}

function downloadBytes(bytes: Uint8Array, filename: string): void {
  const blob = new Blob([bytes.buffer as ArrayBuffer], { type: 'application/octet-stream' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

function contentStatus(content: ProviderObservedContentMetadata, label: string): string {
  if (!content.observed) return `${label}未观察。`;
  const completeness = content.complete ? '已完整观察' : '仅部分观察';
  const observed = `${label}${completeness}（${content.observedBytes} 字节${content.sha256 ? `，SHA-256 ${content.sha256}` : ''}）`;
  if (!content.saved) return `${observed}，但未保存：${content.reason || '原因未提供'}。`;
  if (!content.complete) return `${observed}，不视为完整 provider 输出。`;
  return `${observed}并已保存。`;
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

function providerMetadataMatches(actual?: ProviderObservedContentMetadata, expected?: ProviderObservedContentMetadata): boolean {
  return actual?.observed === expected?.observed
    && actual?.complete === expected?.complete
    && actual?.saved === expected?.saved
    && actual?.observedBytes === expected?.observedBytes
    && actual?.sha256 === expected?.sha256
    && actual?.reason === expected?.reason;
}

function matchesMetadata(actual: AuthoringDiagnosticMetadata, expected: AuthoringDiagnosticMetadata): boolean {
  return actual.parseError === expected.parseError
    && actual.parseErrorSHA256 === expected.parseErrorSHA256
    && actual.parseErrorBytes === expected.parseErrorBytes
    && actual.parseErrorSaved === expected.parseErrorSaved
    && actual.payloadSHA256 === expected.payloadSHA256
    && actual.payloadBytes === expected.payloadBytes
    && actual.payloadSaved === expected.payloadSaved
    && actual.notSavedReason === expected.notSavedReason
    && actual.providerCode === expected.providerCode
    && providerMetadataMatches(actual.providerDocument, expected.providerDocument)
    && providerMetadataMatches(actual.providerWire, expected.providerWire)
    && providerMetadataMatches(actual.providerCause, expected.providerCause);
}

async function verifyPayloadHash(record: VerifiedDiagnostic): Promise<void> {
  if (!record.bytes) throw new Error('解析器输入 payload字节不可用，无法校验。');
  await verifyHash(record.bytes, record.payloadSHA256, '解析器输入 payload');
}

export function AuthoringDiagnosticDetails({ workspaceId, permitId, phase, failureCode, diagnostic }: Props) {
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [record, setRecord] = useState<VerifiedDiagnostic>();
  const [recordScope, setRecordScope] = useState('');
  const [providerDocument, setProviderDocument] = useState<VerifiedProviderContent>();
  const [providerWire, setProviderWire] = useState<VerifiedProviderContent>();
  const [providerCause, setProviderCause] = useState<VerifiedProviderContent>();
  const [rawText, setRawText] = useState<string>();
  const [loadError, setLoadError] = useState('');
  const [displayError, setDisplayError] = useState('');
  const generation = useRef(0);
  const displayCheck = useRef(0);
  const observationIdentity = (content?: ProviderObservedContentMetadata) => content
    ? [content.observed, content.complete, content.saved, content.observedBytes, content.sha256, content.reason]
    : null;
  const scopeKey = JSON.stringify([workspaceId, permitId, phase, failureCode,
    diagnostic?.parseError, diagnostic?.parseErrorSHA256, diagnostic?.parseErrorBytes, diagnostic?.parseErrorSaved,
    diagnostic?.payloadSHA256, diagnostic?.payloadBytes, diagnostic?.payloadSaved, diagnostic?.notSavedReason, diagnostic?.providerCode,
    observationIdentity(diagnostic?.providerDocument), observationIdentity(diagnostic?.providerWire), observationIdentity(diagnostic?.providerCause)]);
  const currentScope = useRef(scopeKey);
  currentScope.current = scopeKey;

  useEffect(() => {
    generation.current++;
    setExpanded(false);
    setLoading(false);
    setRecord(undefined);
    setRecordScope('');
    setProviderDocument(undefined);
    setProviderWire(undefined);
    setProviderCause(undefined);
    setRawText(undefined);
    setLoadError('');
    setDisplayError('');
    const check = ++displayCheck.current;
    if (diagnostic?.parseErrorSaved) {
      void verifyDiagnosticIntegrity(diagnostic).catch(error => {
        if (displayCheck.current === check && currentScope.current === scopeKey) setDisplayError(error instanceof Error ? error.message : String(error));
      });
    }
    return () => { generation.current++; displayCheck.current++; };
  }, [workspaceId, permitId, phase, failureCode, diagnostic?.parseError, diagnostic?.parseErrorSHA256, diagnostic?.parseErrorBytes, diagnostic?.parseErrorSaved, diagnostic?.payloadSHA256, diagnostic?.payloadBytes, diagnostic?.payloadSaved, diagnostic?.notSavedReason, diagnostic?.providerCode, diagnostic?.providerDocument?.observed, diagnostic?.providerDocument?.complete, diagnostic?.providerDocument?.saved, diagnostic?.providerDocument?.observedBytes, diagnostic?.providerDocument?.sha256, diagnostic?.providerDocument?.reason, diagnostic?.providerWire?.observed, diagnostic?.providerWire?.complete, diagnostic?.providerWire?.saved, diagnostic?.providerWire?.observedBytes, diagnostic?.providerWire?.sha256, diagnostic?.providerWire?.reason, diagnostic?.providerCause?.observed, diagnostic?.providerCause?.complete, diagnostic?.providerCause?.saved, diagnostic?.providerCause?.observedBytes, diagnostic?.providerCause?.sha256, diagnostic?.providerCause?.reason]);

  const load = async () => {
    if (!permitId || loading) return;
    const token = ++generation.current;
    setLoading(true);
    setLoadError('');
    try {
      const result = await workorchApi.readAuthoringDiagnostic(workspaceId, permitId);
      if (generation.current !== token || currentScope.current !== scopeKey || result.workspaceId !== workspaceId || result.permitId !== permitId) {
        if (generation.current === token && currentScope.current === scopeKey) setLoadError('诊断响应的 workspace 或 permit 身份不匹配，已拒绝显示。');
        return;
      }
      await verifyDiagnosticIntegrity(result);
      if (result.phase !== phase || result.failureCode !== (failureCode || '')) throw new Error('诊断的阶段或失败码与失败记录不匹配，已拒绝显示。');
      if (diagnostic && !matchesMetadata(result, diagnostic)) throw new Error('诊断元数据与失败记录不匹配，已拒绝显示。');
      const verified = result.payloadSaved
        ? decodeAndVerify(result)
        : ({ ...result, bytes: undefined } as VerifiedDiagnostic);
      if (verified.bytes) await verifyPayloadHash(verified as VerifiedDiagnostic);
      const verifiedDocument = result.providerDocument?.observed && result.providerDocument.saved
        ? await verifyObservedContent(result.providerDocument, result.providerDocument.contentBase64, 'provider 文档')
        : undefined;
      const verifiedWire = result.providerWire?.observed && result.providerWire.saved
        ? await verifyObservedContent(result.providerWire, result.providerWire.contentBase64, 'provider wire')
        : undefined;
      const verifiedCause = result.providerCause?.observed && result.providerCause.saved
        ? await verifyObservedContent(result.providerCause, result.providerCause.contentBase64, 'provider 传输错误原文')
        : undefined;
      let decoded: string | undefined;
      if (verified.bytes) {
        try { decoded = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(verified.bytes); }
        catch { /* 保留已校验原字节供下载，不以替换字符冒充原文。 */ }
      }
      if (!result.providerCode && verifiedDocument && (!verifiedDocument.complete || !verified.bytes
        || verified.bytes.length !== verifiedDocument.bytes.length
        || !verified.bytes.every((byte, index) => byte === verifiedDocument.bytes[index]))) {
        throw new Error('解析器输入 payload 与 Provider 文档的原始 bytes 不一致，已拒绝显示。');
      }
      if (generation.current !== token || currentScope.current !== scopeKey) return;
      setRecord(verified);
      setRecordScope(scopeKey);
      setProviderDocument(verifiedDocument);
      setProviderWire(verifiedWire);
      setProviderCause(verifiedCause);
      setRawText(decoded);
      setExpanded(true);
    } catch (error) {
      if (generation.current === token && currentScope.current === scopeKey) setLoadError(error instanceof Error ? error.message : String(error));
    } finally {
      if (generation.current === token && currentScope.current === scopeKey) setLoading(false);
    }
  };

  const showRecord = expanded && recordScope === scopeKey;
  const download = () => {
    if (!showRecord || !record?.bytes) return;
    downloadBytes(record.bytes, `authoring-${permitId}-parser-payload.bin`);
  };

  const canLoad = Boolean(diagnostic?.payloadSaved
    || (diagnostic?.providerDocument?.observed && diagnostic.providerDocument.saved)
    || (diagnostic?.providerWire?.observed && diagnostic.providerWire.saved)
    || (diagnostic?.providerCause?.observed && diagnostic.providerCause.saved));
  const downloadProvider = (content: VerifiedProviderContent | undefined, name: string) => {
    if (!showRecord || !content) return;
    downloadBytes(content.bytes, `authoring-${permitId}-${name}.bin`);
  };

  return <Space direction="vertical" style={{ width: '100%' }} size="small">
    <Typography.Text strong>{failureCode || '请求失败'} · {phase}</Typography.Text>
    {diagnostic ? <>
      {displayError && <Alert type="error" showIcon message="解析错误诊断完整性校验失败；下方保留服务端返回的原文，但不能视为已校验记录。" description={displayError} />}
      {diagnostic.parseErrorSaved
        ? <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', margin: 0 }}>{diagnostic.parseError ?? ''}</pre>
        : <Typography.Text>原始解析错误未保存（{diagnostic.parseErrorBytes} 字节）；{diagnostic.notSavedReason || '原因未提供'}。</Typography.Text>}
      <Typography.Text type="secondary">
        解析错误 SHA-256 {diagnostic.parseErrorSHA256} · {diagnostic.parseErrorBytes} 字节；解析器输入 payload SHA-256 {diagnostic.payloadSHA256} · {diagnostic.payloadBytes} 字节。
        {diagnostic.payloadSaved ? '解析器输入 payload 已保存。' : `解析器输入 payload 未保存：${diagnostic.notSavedReason || '原因未提供'}。`}
      </Typography.Text>
      {(['providerDocument', 'providerWire', 'providerCause'] as const).filter(key => !diagnostic[key]).map(key => <Typography.Text type="secondary" key={key}>{key} 未提供观测 metadata；不能据此判断是否观察、完整或保存。解析器输入 payload 不替代 Provider 观测。</Typography.Text>)}
      {diagnostic.providerCode && <Typography.Text type="secondary">Provider code {diagnostic.providerCode}</Typography.Text>}
      {diagnostic.providerDocument && <Typography.Text type="secondary">{contentStatus(diagnostic.providerDocument, 'Provider 文档')}</Typography.Text>}
      {diagnostic.providerWire && <Typography.Text type="secondary">{contentStatus(diagnostic.providerWire, 'Provider wire')}</Typography.Text>}
      {diagnostic.providerCause && <Typography.Text type="secondary">安全失败分类与传输错误原文分离。{contentStatus(diagnostic.providerCause, '传输错误原文')}</Typography.Text>}
      {permitId && canLoad && <Button size="small" loading={loading} onClick={() => { if (showRecord) { setExpanded(false); return; } void load(); }}>
        {showRecord ? '收起诊断原文' : '按需加载并校验诊断原文'}
      </Button>}
      {loadError && <Alert type="error" showIcon message="诊断原文不可用或完整性校验失败" description={loadError} />}
      {showRecord && record && rawText === undefined && record.bytes && <Alert type="warning" showIcon message="解析器输入 payload不是有效 UTF-8 文本；已验证原字节可下载，界面不进行字符替换。" />}
      {showRecord && record && !record.bytes && <Alert type="warning" showIcon message="解析器输入 payload 未保存；Provider 内容仅依据各自观测记录展示。" description={diagnostic.notSavedReason} />}
      {showRecord && record && rawText !== undefined && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 320, overflow: 'auto', margin: 0 }}>{rawText}</pre>}
      {showRecord && record?.bytes && <Button size="small" onClick={download}>下载已校验原始字节</Button>}
      {showRecord && providerDocument?.text === undefined && providerDocument && <Alert type="warning" showIcon message="Provider 文档不是有效 UTF-8 文本；已验证观测字节仍可下载。" />}
      {showRecord && providerDocument?.text !== undefined && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 320, overflow: 'auto', margin: 0 }}>{providerDocument.text}</pre>}
      {showRecord && providerDocument && <Button size="small" onClick={() => downloadProvider(providerDocument, 'provider-document')}>下载已校验 Provider 文档 bytes</Button>}
      {showRecord && providerWire?.text === undefined && providerWire && <Alert type="warning" showIcon message="Provider wire 不是有效 UTF-8 文本；已验证观测字节仍可下载。" />}
      {showRecord && providerWire?.text !== undefined && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 320, overflow: 'auto', margin: 0 }}>{providerWire.text}</pre>}
      {showRecord && providerWire && <Button size="small" onClick={() => downloadProvider(providerWire, 'provider-wire')}>下载已校验 Provider wire bytes</Button>}
      {showRecord && providerCause?.text === undefined && providerCause && <Alert type="warning" showIcon message="传输错误原文不是有效 UTF-8 文本；已验证观测字节仍可下载。" />}
      {showRecord && providerCause?.text !== undefined && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 320, overflow: 'auto', margin: 0 }}>{providerCause.text}</pre>}
      {showRecord && providerCause && <Button size="small" onClick={() => downloadProvider(providerCause, 'provider-cause')}>下载已校验传输错误原文 bytes</Button>}
    </> : <Typography.Text>此失败记录没有可用诊断；原始原因未记录，且不会重新调用 provider。</Typography.Text>}
  </Space>;
}
