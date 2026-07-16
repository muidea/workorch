# Operation Map / JSON 交互合同残留审计

Status: completed

Type: audit / closure backlog

Last Updated: 2026-07-14

## Purpose

记录 2026-07-14 对当前代码库的核对与收口结果：哪些路径曾使用 **operation map** 或 **json marshal/unmarshal** 作为交互合同，以及最终如何收敛为 concrete typed command/result。

本文档用于后续收口，不充当 Runtime 设计 SoT。权威约束仍以：

- `AGENTS.md`
- `docs/00-core/runtime-stage-contract.md`
- `docs/30-module-architecture/blocks-runtime-adapters.md`
- `internal/modules/kernel/orchestrator/readme.md`
- `internal/pkg/workorchruntimecontract`

为准。

## Authority Constraints（核对基线）

| 约束 | 来源 |
| --- | --- |
| 跨 owner I/O 必须使用 EventHub + concrete typed command/result | `AGENTS.md` |
| 禁止通用 RuntimeAdapter registry / map bridge 回 EventHub | `docs/00-core/runtime-stage-contract.md` |
| `map[string]any` 不得作为组件交互或 owner 状态合同；仅允许在持久化、HTTP/SSE、provider/MCP/Web 外部协议和最终产品 DTO 边界物化 | `docs/30-module-architecture/blocks-runtime-adapters.md`、`orchestrator/readme.md` |
| Production path 仅 Runtime V2；不得重引入 map-based authority rebuild | `AGENTS.md` |

## Executive Summary

| 类别 | 现状 | 是否仍是“交互合同” | 收口优先级 |
| --- | --- | --- | --- |
| EventHub 跨模块 command/result | 主路径已 typed | 否（目标形态） | — |
| Runtime V2 operation 执行边界 | capability 独立 concrete command/data/result；共享 `StepExecutionRequest/Result` 已删除 | 否 | 已完成 |
| file edit / patch apply | 已独立 typed command + concrete request/result | 否 | 对照面 / 完成 |
| decision / approval 域模型 | EventHub 与 owner 使用 concrete `workorcheventpayload.Payload` 开放扩展字段 | 否（typed extension bag） | 已完成 |
| 观测事件 payload | EventRecord 使用 typed payload；产品/API 末端才物化 map | 否（边界 DTO） | 已完成 |
| `json.Marshal/Unmarshal` | 主要用于外部协议 / 持久化 / HTTP API | **跨模块主链基本没有**；外边界合理存在 | P2（外边界保留） |
| 通用 adapter registry | 未再发现 `map[kind]→handler` 回灌 EventHub | 已废止 | — |

**一句话结论：**

- EventHub 跨 owner 主合同大体已是 concrete typed command/result。
- 原核心 map 交互合同 `StepExecutionRequest/Result` 已删除；orchestrator 直接投递 capability 对应的 concrete operation data/result。
- `json.Marshal/Unmarshal` 不是 kernel↔blocks 主交互合同；主要出现在外部协议、存储与产品 API。
- file list/read/edit/patch、shell、git、mcp、ssh、web 均已切换到独立 typed operation 合同。

---

## 2. 已合规 / 对照完成面

## 1. 术语说明

本文中的 operation map 指历史上把 typed `RuntimeOperation.Parameters` 压成 `map[string]any`，再由多个 block 共用同一 StepExecution adapter 的做法。该合同已经删除。

当前边界定义：

- EventHub command/data/result：必须是 capability 所属组件定义的 concrete Go 类型。
- Owner 状态：必须是 concrete struct 或 `workorcheventpayload.Payload`；不得保存裸 `map[string]any` / `any`。
- JSON：仅允许在 LLM、MCP、Web 外部协议，HTTP/SSE 产品接口及持久化边界使用。
- 产品 DTO：CLI/TUI/Web 最终展示层可以按既有 JSON 合同物化 map，但不得回流到 owner/EventHub/runtime authority。

### 2.1 EventHub 主命令已 typed

Orchestrator 侧 Runtime V2 相关 command/result 已为 concrete struct，例如：

- `ExecuteRuntimeOperationCommand` / `ExecuteRuntimeOperationResult`
- `ExecuteFileEditOperationCommand` / `ExecuteFileEditOperationResult`
- `ExecutePatchApplyOperationCommand` / `ExecutePatchApplyOperationResult`
- `RequestRuntimeCandidateCommand` / `RequestRuntimeCandidateResult`
- `AcquireTargetSnapshotCommand` / `AcquireRuntimeCapabilitiesCommand` 等

通用 capability block（shell/git/mcp/ssh/web/file）通过订阅：

- `TopicOrchestratorExecuteRuntimeOperation`
- `TopicOrchestratorAcquireRuntimeCapabilities`

以 **Go 类型断言** 接收 command，不以 JSON 字节跨模块解码。

### 2.2 file edit / patch apply 已脱离 map 主合同

| 层 | 类型 |
| --- | --- |
| EventHub | `ExecuteFileEditOperationCommand` + `FileEditOperationData` |
| EventHub | `ExecutePatchApplyOperationCommand` + `PatchApplyOperationData` |
| block 内部 | `filebiz.FileEditRequest` / `PatchApplyRequest` / `RuntimeFileOperationResult` |

约束：通用 `ExecuteRuntimeOperation` 对 `file/edit` 与 patch apply kind 会拒绝，要求走 concrete contract。

这是后续 shell/git/mcp/ssh/web 收口的参考形态。

### 2.3 已废止且代码中未见回潮

未再发现生产路径中的：

- 通用 `RuntimeAdapter registry` 用 `map[kind]handler` 回灌 EventHub
- orchestrator 与 block 之间用 `json.Marshal/Unmarshal` 传 command/result
- 用 free-form map 重建 Runtime V2 authority / candidate / dispatch / evidence 主链

主链 authority 对象已是 typed `RuntimeOperation` / `RuntimeOperationEvidence` / sealed dispatch。

---

## 4. Wave B/C 边界结论

## 3. Wave A 完成证据

Runtime V2 已删除共享 `StepExecutionRequest/Result` 及其 map bridge。Orchestrator 现在按 capability 直接发送独立 concrete 合同：

- File：`ExecuteFileListOperationCommand/Result`、`ExecuteFileReadOperationCommand/Result`、`ExecuteFileEditOperationCommand/Result`。
- Patch：`ExecutePatchPreviewOperationCommand/Result`、`ExecutePatchApplyOperationCommand/Result`。
- Shell：`ExecuteShellOperationCommand/Result`。
- Git：`ExecuteGitOperationCommand/Result`。
- MCP：`ExecuteMCPOperationCommand/Result`。
- SSH：`ExecuteSSHOperationCommand/Result`。
- Web：`ExecuteWebOperationCommand/Result`。

`runtime_operation.go` 不再把 `RuntimeOperation.Parameters` 压成 map，也不再从 map 输出回填 evidence/failure。`workorchruntimecontract` 中不再保留 StepExecution 合同；外部 provider 的 JSON 仍只存在于 provider/MCP/Web 边界。

Run owner 的 `Step`、`ToolCall`、`RuntimeCheckpoint`、`Patch` 状态字段也已改为具体 payload/struct；持久化和产品 DTO 是唯一 map 物化边界。

### 4.1 Decision / Approval

| 位置 | 形态 |
| --- | --- |
| EventHub messages | concrete `workorcheventpayload.Payload`（typed recursive extension bag） |
| module 转换 | 直接传递 payload；不以 map 作为 owner/module 合同 |
| domain contracts/models | context/resolution/approval 使用 concrete payload；开放字段只在该边界内解释 |

相关文件：

- `internal/modules/blocks/decision/module.go`
- `internal/modules/blocks/decision/pkg/events/messages.go`
- `internal/modules/blocks/decision/pkg/contracts/commands.go`
- `internal/modules/blocks/decision/pkg/models/ticket.go` / `resolution.go`
- `internal/modules/blocks/approval/module.go`
- `internal/modules/blocks/approval/pkg/events/messages.go`
- `internal/modules/blocks/approval/pkg/contracts/commands.go`

**判定：** 不是裸 JSON 或 map 跨模块；是有明确边界的 concrete typed extension payload。

**收口结果：** governance context / resolution payload 已统一为 `workorcheventpayload.Payload`；该 payload 是开放扩展点，不得退化为 `map[string]any` 或 JSON bridge。

### 4.2 观测 / 事件投影 payload

文件：`internal/pkg/events/workorch_payload.go`

- 已删除旧的 `*Payload(...) map[string]any` constructor
- 已删除旧的 `Decode*Payload(payload map[string]any)` decoder
- 保留 `EventPayload()` 与 typed decoder，EventRecord 内部使用 `workorcheventpayload.Payload`

消费方包括 eventstream、application observed 投影、TUI/CLI 展示等。

**判定：** observability / read-model 的内部 EventRecord 合同是 typed payload，不是 Runtime V2 authority 主链；map 只在最终 API/UI/持久化边界物化。

**收口结果：** 高频 `workorch_payload` 的 map constructor/decoder 已删除，保留 `EventPayload()` 与 typed decoder；投影 DTO 的 map 字段属于产品边界。

### 4.3 CLI / TUI / backends 展示与命令 payload

`internal/services/workorch/biz/backends` 与 TUI/CLI 仍大量使用 `payload map[string]any` 做资源投影与决策/审批输入。

**判定：** 产品交互边界的软合同；不阻塞 Runtime V2 authority 收口，但影响类型安全与前后端一致性。

### 4.4 Run owner 状态模型与 managed validation

| 位置 | 收口结果 |
| --- | --- |
| `internal/modules/kernel/run/pkg/models` | Step、ToolCall、RuntimeCheckpoint、Patch 已使用 concrete payload/struct |
| managed validation | 使用 Shell typed operation result 与 typed runtime contract |
| V1 retirement | 不再作为 Runtime V2 输入，也不保留 Step output map bridge |

**判定：** 偏 V1 retirement / managed validation 资源收口，不是 V2 sealed dispatch 主路径。

**收口结果：** Run owner 状态已完成 typed 化；map 只在 repository/API/read-model 边界物化。

---

## 5. JSON marshal/unmarshal 盘点

### 5.1 不是 EventHub 跨模块主合同

在 `kernel/orchestrator` 与 blocks 的 EventHub command/result 路径上，**没有**“先 marshal 成 JSON bytes，再跨模块 unmarshal”作为主交互方式。跨模块主合同是 concrete Go 类型；开放扩展字段使用 `workorcheventpayload.Payload`。

### 5.2 仍存在的 JSON 使用（按边界）

| 边界 | 代表位置 | 角色 | 是否算模块交互合同 | 建议 |
| --- | --- | --- | --- | --- |
| LLM Provider HTTP | `blocks/llm/biz/http_provider.go` | 外部协议编解码 | 否 | 保留 |
| Runtime candidate 请求投影 | `blocks/llm/runtime_candidate_provider.go` | typed 投影 marshal 给 provider | 外边界 | 保留；内部 candidate 继续 typed |
| MCP HTTP/stdio | `blocks/mcp/biz/mcp_runtime_adapter.go` | JSON-RPC / HTTP body | 否（外部协议） | 保留外边界；内边界改 typed |
| Web provider | `blocks/web/biz/runtime_adapter.go` | 外部搜索/抓取 API | 否 | 保留 |
| 文件仓库持久化 | `internal/pkg/repository/file` 等 | 存储序列化 | 否 | 保留 |
| HTTP/SSE API | `workorchd` http_types / client | 产品接口 | 是（产品 API 合同） | 产品层维护；不混入 kernel 内合同 |
| setup draft 解析 | `setupdraftcore/completion_*.go` | 解析 LLM 返回 | 半外边界 | 保留 decode 边界 |
| skillhub runner | `contextregistryports/skillhub_runner.go` | 解析外部进程输出 | 外边界 | 保留 |

### 5.3 结论

“json marshal/unmarshal 当交互合同”在 **kernel↔blocks Runtime V2 主链**上基本已不成立。  
残留主问题是 **map bag**，不是 JSON 字节合同。JSON 在外部协议 / 持久化 / 产品 API 上通常合理，**不作为本轮 P0 删除目标**。

---

## 7. 收口 Backlog

## 6. 关键代码索引（完成态）

| 路径 | 完成态职责 |
| --- | --- |
| `internal/modules/kernel/orchestrator/pkg/events/messages.go` | File/Git/Shell/MCP/SSH/Web concrete operation command/data/result |
| `internal/modules/kernel/orchestrator/runtime_v2_operation_executor.go` | typed operation 路由与结果解包 |
| `internal/modules/blocks/*/module.go` | 直接订阅本 capability 的 concrete topic |
| `internal/modules/blocks/*/biz/*_runtime_typed.go` | block 内部 concrete request/result |
| `internal/modules/kernel/run/pkg/models` | Step/ToolCall/RuntimeCheckpoint/Patch typed owner 状态 |
| `internal/pkg/workorcheventpayload` | typed recursive payload 与持久化/API 边界转换 |
| `internal/pkg/events/workorch_payload.go` | typed EventPayload constructor/decoder；无旧 map constructor/decoder |

### Wave A — Runtime adapter map 桥（P0，已完成）

- [x] A1. shell/git/mcp/ssh/web 字段与失败集合已落实为各自 typed request/result
- [x] A2. shell concrete operation command/result
- [x] A3. git concrete operation command/result
- [x] A4. mcp concrete operation command/result（外边界 JSON 保留）
- [x] A5. ssh concrete operation command/result
- [x] A6. web concrete operation command/result（外边界 JSON 保留）
- [x] A7. file list/read/edit/patch 均使用独立 typed operation 合同
- [x] A8. 删除 `runtimeOperationInput` / 通用 `runtimeOperationEvidence(map)` 桥
- [x] A9. 从共享 `workorchruntimecontract` 移除 `StepExecutionRequest/Result`
- [x] A10. capability routing、typed evidence、managed validation 与 file edit/patch 回归通过

### Wave B — Domain soft map（P1，已完成）

- [x] B1. decision context / resolution payload 使用 concrete `workorcheventpayload.Payload`
- [x] B2. approval context / resolve context 使用 concrete `workorcheventpayload.Payload`
- [x] B3. module 边界不再以 `Payload.Map()` 传递域模型；map 仅在持久化、HTTP/SSE 或产品 DTO 边界转换
- [x] B4. Step、ToolCall、RuntimeCheckpoint、Patch owner model 已 typed 化；managed validation 不再依赖 Step Input map

### Wave C — Observability / product bag（P1/P2，已完成本轮范围）

- [x] C1. 高频 `workorch_payload` 已收敛为 typed `EventPayload()` / typed decoder；旧 map constructor/decoder 已删除
- [x] C2. CLI/TUI/backends 的 map 只保留最终产品/API/UI DTO 边界，内部 EventRecord 使用 typed payload
- [x] C3. Runtime adapter 架构与部署收口文档已删除通用 StepExecution 跨模块合同表述

### 非目标（本轮明确不做）

- 删除 LLM/MCP/Web 外部协议 JSON
- 删除文件仓库 / 配置持久化 JSON
- 删除 HTTP/SSE 产品 API JSON
- 重新引入通用 adapter registry

---

## 8. 验收标准（收口完成定义）

以下验收项已满足；本文标记为 `completed`，待统一文档归档流程迁出 active：

1. Runtime V2 operation 执行路径中，orchestrator 与 capability blocks 之间 **不再** 共享 `StepExecutionRequest.Input/Output map[string]any` 作为交互合同。
2. shell/git/mcp/ssh/web 的生产执行路径均有 concrete typed command/data/result（或等价 typed payload），字段可静态审查。
3. `runtime_operation.go` 不再从 `map[string]any` 回填 evidence/failure 主字段。
4. EventHub 跨 owner 路径仍无 JSON marshal/unmarshal 作为 command/result 合同。
5. decision/approval 的 context/payload 使用 concrete `workorcheventpayload.Payload` 作为明确开放扩展点，禁止 map/JSON EventHub 合同。
6. 相关 `make test` / 目标包测试与既有 runtime contract 回归通过。
7. `docs/30-module-architecture/blocks-runtime-adapters.md` 与相关 active 计划中的“map 仅内部”表述与代码一致，不再依赖共享 StepExecution map。

---

## 9. 审计方法与边界

- 核对方式：静态搜索、关键路径精读、代码迁移与回归验证。
- 范围：`internal/modules`、`internal/pkg`、相关 docs；不以 vendor / 生成物为准。
- 时间：2026-07-14。
- 非结论：本文件不重新定义 Runtime V2 authority 语义；不替代 `runtime-stage-contract.md`。

## 10. 变更记录

| 日期 | 说明 |
| --- | --- |
| 2026-07-14 | 初版：完成 operation map 与 json marshal/unmarshal 交互合同残留核对，并形成收口 backlog |
| 2026-07-14 | 完成 Wave A/B/C：删除共享 StepExecution map 桥、typed 化 Run owner 状态与 payload API，并同步架构文档 |
