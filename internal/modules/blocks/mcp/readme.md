# MCP 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-26

## 功能范围与边界

`blocks/mcp` 是 MCP tool、resource 与 prompt 的唯一 execution owner。它读取 Registry owner 发布的一致 catalog，
执行 owner-private transport/session，并把有界结果规范化为 `RuntimeOperationReceipt`。它不拥有 Runtime aggregate，
不选择 focus/action/credit/terminal，也不修改 MCP Registry。

## Runtime 合同

- SM-2 的 capability、operation-admissibility、operation-feasibility、semantic-proof feasibility 与 CWI reconciliation
  均使用本组件 `pkg/events` 定义的 concrete command/result。catalog/feasibility 只读取一致 Registry snapshot，校验
  schema/input binding 与本地 transport prerequisite；不得调用 MCP endpoint、启动 stdio process、创建 dispatch 或 receipt。
- callable capability 与 validation surface 分离。只有 `ready + direct`、已有 owner-sealed input binding、non-durable
  且 replay policy 合法的 surface 才进入 `ValidationRecipeRefs`；required-input、auxiliary、not-declared 与 unavailable
  surface 只保留 typed audit fact，不能关闭 CWI。
- SM-3 的 `ExecuteOperationCommand` 只携带完整 `SealedRuntimeOperation`，不存在可独立漂移的第二份 `Data`。payload hash
  同时绑定 operation、参数、target/CWI scope、owner catalog snapshot、Registry revision、exact Capability、effect/
  idempotency policy 与适用 validation surface。execution envelope 的 `AdmissibilitySnapshotID + AcquisitionBinding` 必须
  与 seal 内 catalog identity 精确相等；缺少 SM-3 封存 identity 或用同 execution key/fence 替换 catalog 均 fail-closed。
- 外部调用前依次校验同步 result、Orchestrator producer、workspace/run/dispatch/execution key/fence、sealed payload、
  Orchestrator receipt authorization、当前 Registry revision、enabled Server/Capability、allowlist、exact kind/name/URI、
  bounded input schema、effect/idempotency 与 exact approval grant。任一漂移均在 transport 前 fail-closed。
- receipt query 的 `Found=false` 只有 Orchestrator 已证明当前 execution 仍为 active+claimed 且 grant parity 精确匹配时
  才是执行许可；MCP owner 不把普通“未找到 receipt”解释为授权。已提交的 exact receipt 重放只返回既有事实；query 的
  Found/Receipt 一致性与 identity、commit 的完整 receipt equality 由 capability owners 共享的 typed validator fail-closed 校验。
- approval policy 来自 exact Capability（缺失时使用同一 Server snapshot 的 default）；typed requirement 绑定 capability、
  operation payload hash、catalog/Registry/capability revision、effect 与 idempotency。MCP owner 不创建或解决 Approval。

## 输出与 effect 合同

- HTTP/stdio response 总量上限为 1 MiB，stream/notification 上限为 128；超限或非法 JSON/protocol 直接 rejected，
  不做无标记截断。HTTP 仅允许同 origin 的有界 redirect，跨 origin payload 转发在 follow 前拒绝。动态 operation
  参数最多 64 项，单项/名称/总字符串预算均由 Runtime contract 强制。
- provider response 与 stream event 只写入 workspace internal 下 owner-private、`0600`、content-addressed evidence file；
  EventHub/Runtime receipt 仅携带安全 summary、SHA-256、`workorch://mcp/evidence/...` raw ref 与 exact capability source ref。
  endpoint、credential、stderr、raw provider error 和实际 tool 内容不得进入 aggregate、Eventstream 或 projection。
- non-durable capability 只能返回无 changed fact。durable capability 必须在 response 的 `workorchEffect` 中提供与 sealed
  effect 一致的 `changed`、before/after revision 及 bounded remote resource changes；真实 no-op 返回相同 revision。
  缺失、冲突或伪造 observation 产生 rejected receipt，不能按声明盲报 `Changed=true`。
- provider side effect 已发生但 receipt 尚未持久化时，恢复严格服从 sealed replay class：`at_most_once | unknown`
  进入 `outcome_unknown`，不得重发；已有 exact receipt 的响应丢失只重放该 receipt/evidence transaction。

## Registry 与 secret 边界

- 本 block 只导入 `blocks/mcpregistry/pkg/events`，通过 `ReadRegistrySnapshotCommand/Result` 一次取得同一
  `RegistryRevision` 的 Server/Capability aggregate；静态 transport probe 可使用具体 `GetServerCommand/Result`，
  不注入 Registry service/repository，也不拼接两次 list。
- EventHub 中的 environment metadata 仅允许 `env://ENV_NAME` 或受支持的 workspace placeholder。stdio execution owner
  在启动进程前最后解析引用；缺失引用、明文值、非法路径或非法 placeholder 均 fail-closed。secret value 不进入
  Registry DTO、Runtime authority、日志或 projection。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/reconciliation/execute；receipt query/commit | 完整；SM-2 零副作用，SM-3 exact sealed execution |
| `blocks/mcpregistry` | MCP → Registry | `ReadRegistrySnapshotCommand/Result`、静态 probe 的 `GetServerCommand/Result` | 完整；单 aggregate revision、reference-only environment |
| 外部 MCP server | MCP-private adapter | bounded tool/resource/prompt invocation、owner-private evidence/effect observation | 完整；只在 authorized SM-3 execute 中触达 |

## 回归覆盖

- [x] direct/auxiliary/not-declared 与 ready/prerequisite-required/unavailable surface、required input、empty binding、非法 schema。
- [x] catalog acquisition 与 semantic feasibility 零 endpoint/process side effect；Registry repository 不跨 owner 注入。
- [x] Post、错误 producer、scope/fence、sealed payload、execution-record catalog identity、Capability/allowlist/schema/policy/
  revision 与 approval grant 漂移，且前述 ingress 漂移均为零 provider transport。
- [x] tool/resource/prompt exact Capability 选择及 non-durable、durable changed、durable no-op、缺失 effect observation。
- [x] HTTP/stdio byte/event guard、安全 diagnostic、private raw ref/hash、credential/environment reference 隔离。
- [x] receipt commit 前后崩溃、response loss、duplicate/restart 与 stale catalog 的 at-most-once/replay 收敛。

真实 provider/credential 的 deployed verification 继续由 `RUNTIME-LIVE` 与显式 MCP live gate 负责；它不是第二套
组件合同或 Runtime authority。
