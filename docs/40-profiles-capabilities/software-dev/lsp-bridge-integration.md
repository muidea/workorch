# Software-Dev LSP Bridge Integration

Status: normative

Type: profile-capability

Last Verified: 2026-08-16

## Current Default

The embedded MCP Registry defaults contain one enabled long-lived stdio server, `mcp-lsp-bridge`, with command
`lsp-bridge`. Its environment resolves workspace-managed config and the Workspace deliverable root at activation/session time.

It declares nine tools:

`lsp_initialize`, `lsp_sync`, `lsp_definition`, `lsp_hover`, `lsp_references`, `lsp_diagnostics`, `lsp_status`,
`lsp_shutdown`, and `lsp_repair`.

This is an optional MCP capability set, not a special Orchestrator or software-dev Runtime adapter.

## Owner Flow

1. MCP Registry owner supplies selected server/capability metadata.
2. fresh activation seals allowed MCP refs and tool kinds.
3. Orchestrator may seal an `mcp/tool_call` only when the manifest and binding allow the exact server/tool.
4. `blocks/mcp` owns stdio session, execution and factual receipt.
5. Orchestrator decides whether the receipt is accepted evidence.

## Use In Software Development

- initialize before semantic queries for a Workspace session;
- after unsaved/in-memory content changes, `lsp_sync` must carry the workspace-relative file and complete content expected by
  the tool contract;
- definition/hover/references/diagnostics/status can narrow a symbol or contract-shape investigation;
- shutdown/repair only manage the workspace-local language-service session and must not modify deliverables;
- empty, truncated, timeout or unavailable results should fall back to file/search/build/test evidence unless Goal/DoD
  explicitly requires LSP.

## Evidence Boundary

LSP output is supporting semantic evidence. Clean diagnostics alone do not prove build, test, external behavior or complete
DoD coverage. A truncated reference result cannot prove full impact analysis. File modifications still require File owner
operations and matching revision fences.

## Deployment Boundary

WorkOrch embeds registry metadata, not the `lsp-bridge` executable or language servers. The deployment environment must provide
the command and its dependencies. Workspace-managed configuration remains internal state and is not a deliverable patch target.

## PRD 交付证据报告

目标项目的 PRD 完整交付验收与上述可选 MCP 工具默认集是两个边界。`scripts/verify_lsp_bridge_prd_evidence.py` 按 `LSP-01`～`LSP-20` 检查固定的逐项事实、artifact 原文件 hash 及 Workspace／Run／startup／最终交付 commit／服务 source identity；不读取本地 Git HEAD 代替部署或交付身份，也不执行报告中的命令。

十工具、五语言真实 definition、未保存编辑、错误定位、三个受限输出的超限探针、实例关闭／修复／重启、TTL／LRU、路径、十并发、构建／测试／race、配置与完整文档各有独立检查。Definition 和 Hover 时延分别要求预热后 `<50ms` 且排除 LSP 本身计算；RSS 要求同时三种受支持语言、`<64MB` 且排除子进程。缺项、skip、fixture 冒充真实执行、引用／hash／scope 漂移及仅源码／文档存在均不能满足行为证据。

- `make verify-lsp-bridge-prd-evidence-tests` 仅校准 synthetic 正反例，不代表项目通过 PRD。
- `make verify-lsp-bridge-prd-evidence LSP_BRIDGE_PRD_EVIDENCE=<报告路径> LSP_BRIDGE_EXPECTED_IDENTITIES=<外部身份路径>` 只读核对报告；缺失或非法时退出 `1`。尚无独立可信 producer 证明时，即使结构与文件校验通过也返回 `PENDING_UNVERIFIED`、`acceptanceEstablished=false` 并退出 `2`，不能登记为发布或 live 验收通过。
- JSON Schema 描述输入结构；逐项语义由本地校验器执行。schema-valid、provider PASS 或手工填报事实都不等于 owner receipt 与真实测量证明。

`lsp_bridge_blackbox_producer.py` 的 stdio 观察边界只允许 exact argv／cwd／显式 environment keys／有限期限的子进程，不读取现有实现，也不从 PRD 猜测响应字段。MCP envelope 由传输层原样传给观察层，业务响应由显式 adapter 解释。请求、实际写入片段、响应行和 stdout／stderr 原 bytes 分别保存并记录 hash；失败、EOF、无换行片段、超时和部分 stdin 写入均不重编码或自动重发。初始化 notification 也纳入原始 exchange。只有 decoded 值的 transport 明示未观察 wire，不能以重新序列化值补造原文。

最终 stdio 观察必须先关闭本对象启动的子进程并收集流；未观察完整 EOF 或读取失败时明确 `streamsComplete=false` 及原因。产物在 delivery 下新建的私有目录中完整写入后不覆盖地发布。观察报告不生成 managed owner receipt，不产生正式 acceptance，所有尚未完整证明的 PRD 项保持 `NOT_VERIFIED`；部分事实及完整 wire 不能替代 Result 的 DoD 判定和交付身份关联。

## Verification

- default metadata: `go test ./internal/pkg/workorchdefaults -count=1`
- MCP owner/session: `go test ./internal/modules/blocks/mcp/... -count=1`
- profile integration: `make verify-runtime-profile-software-dev`
