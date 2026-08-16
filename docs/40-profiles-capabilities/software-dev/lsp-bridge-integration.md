# Software-Dev LSP Bridge Integration

Status: active

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

## Verification

- default metadata: `go test ./internal/pkg/workorchdefaults -count=1`
- MCP owner/session: `go test ./internal/modules/blocks/mcp/... -count=1`
- profile integration: `make verify-runtime-profile-software-dev`
