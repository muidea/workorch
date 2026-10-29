# MCP Capability Management

Status: normative

Type: profile-capability

Last Verified: 2026-08-21

## Purpose

MCP capability management separates declarative catalog/selection metadata from concrete Runtime execution. The split is a
single-owner boundary, not two representations of the same mutable state.

## Owner Split

| Concern | Owner |
| --- | --- |
| MCP server definition, discovered catalog metadata and `MCPCapability` availability/provenance | `blocks/mcpregistry` |
| Workspace/profile/Soul/Skill references used to select allowed capabilities | their respective state owners; compiled by workorchd |
| Frozen per-Run MCP metadata | `RuntimeBootstrap.Activation`, sealed by Run owner |
| Concrete `mcp/tool_call`, `mcp/resource_read`, `mcp/prompt_get` operation | `blocks/mcp` |
| Candidate/sealed dispatch/evidence acceptance | `kernel/orchestrator` |
| UI list/detail/usage view | workorchd projection only |

MCP Registry does not execute tools. MCP execution does not mutate catalog definitions. Orchestrator does not inspect MCP
implementation or call a registry/service facade.

## Catalog Contract

An `MCPCapability` provides bounded metadata such as:

- identity, server identity, `kind`, technical name and display name;
- description/category/tags/stage hints;
- input/output summary and optional URI/schema metadata;
- risk level、approval policy与唯一authoritative availability字段`enabled`；
- Runtime operation kind, effect scope, idempotency class and validation proof applicability.

Effect scope, idempotency and `direct | auxiliary | not_declared` proof applicability are explicit Registry-owner metadata.
Runtime must not infer them from tool name, description, category, risk label or DoD text. MCP execution independently compiles
technical `ready | prerequisite_required | unavailable` from the detached registry snapshot and sealed input facts; only
`ready + direct` may enter provider-visible validation recipes. A ready auxiliary tool such as the default `lsp_status` remains
callable/auditable but cannot directly close CWI coverage.

Server、Capability与binding属于一个revisioned Registry aggregate；所有写入使用whole-aggregate CAS，bounded list携带稳定
`RegistryRevision`，activation/Runtime消费owner-issued一致snapshot而不是拼接两次list。Capability provenance为owner-issued
`source`、`sourceRef`和monotonic item `revision`；不存在另一套draft/active/deprecated lifecycle。

Server environment configuration只保存`environmentRefs`。`env://ENV_NAME`由MCP execution在last responsible moment解析；
plaintext secret在Registry persistence或EventHub publication前拒绝。

The management API exposes server/capability list/create/read/update plus server tool/resource/prompt discovery. Mutation routes
are listed in [Service Command API](../50-product-interface/service-api.md)，query/discovery routes in
[Service Read API](../50-product-interface/service-read-api.md)。

## Activation And Selection

For a fresh activation, workorchd resolves Workspace/profile defaults, Skill requirements and effective Soul allowlists,
then freezes allowed server IDs and `RuntimeMCPCapabilityContext` values in `RuntimeActivationContext`. The public execution
request cannot inject MCP server/capability IDs.

Selection and use are different facts:

- sealed activation metadata defines what may be considered;
- a sealed dispatch names one admitted concrete operation;
- capability owner result proves what was actually attempted/effected;
- accepted operation evidence proves what Orchestrator admitted into convergence.

Resume/recovery reuses the sealed activation/catalog observation. Workspace or registry changes affect a later fresh
activation and cannot rewrite an active Run.

SM-2 catalog acquisition/feasibility只读取一个Registry aggregate、校验bounded input schema与local HTTP/stdio launch
prerequisite；不得联系endpoint、启动process、采样tool、创建receipt或执行operation。`safe_to_replay` declaration也不授予
SM-3 execution authority。

## Runtime Execution

`blocks/mcp` owns the concrete `ExecuteOperationCommand/Result` topic and performs transport/session/tool/resource/prompt
work。command绑定完整persisted operation、selected catalog snapshot、exact Capability/policy和operation-scoped grant，不存在
独立mutable `Data` copy。owner在transport前重验Orchestrator producer、current claimed ExecutionKey/Fence、payload hash、
Registry revision、enabled Server/Capability/allowlist/schema/effect/idempotency与exact grant；missing receipt只有在同一execution
已经被证明active+claimed时才可执行。

跨EventHub结果只有一个`RuntimeOperationReceipt`。provider output/stream由owner做byte/event guard并存为private
content-addressed evidence；receipt只携带safe summary、SHA-256和opaque refs。durable capability必须返回matching scope、
changed/no-op revision与bounded resource changes的typed effect observation，不能从Registry metadata推导`Changed=true`。

It must not:

- choose Runtime action, focus or evidence acceptance;
- create/resolve Approval or Decision state;
- update MCP Registry metadata as a side effect of execution;
- receive a generic Orchestrator operation envelope or `map[string]any` authority payload.

## Governance

Risk and approval policy are declarative inputs. Whether a specific dispatch needs authorization is decided by the sealed
Runtime/governance contract. Approval owner records the authorization result; MCP owner consumes only the typed immutable
fact needed for the operation. The UI may correlate approval and capability projections but cannot decide authorization.

grant必须绑定exact Capability、complete operation payload hash、Registry/capability revision、effect和idempotency；stale、
reused或scope drift在network/process action前失败。MCP owner可以返回`governance_required`，但不能创建或解决Approval。

## Projection Rules

Catalog pages may show display name, kind, server, risk, approval policy, stage hints, enabled and provenance. Run pages should
distinguish:

- allowed/selected capability snapshot;
- actually used capability and operation receipt;
- accepted evidence/result/artifact;
- related governance fact.

HTTP/stdio output上限为1 MiB和128个stream/notification events；invalid/oversized protocol data直接拒绝。HTTP redirect有界且
只允许same-origin。Large payload只使用validated owner-private raw refs；projection/Eventstream不得包含credential、endpoint、
raw provider error、unbounded output或live client/session。

## Verification

- registry CRUD/discovery and scope tests live with `blocks/mcpregistry` and workorchd server;
- typed operation/admissibility tests live with `blocks/mcp`;
- `make verify-mcp-stdio` is the deterministic deployed baseline;
- `make verify-real-mcp-stdio`, `make verify-real-weather` and `make verify-weather-ticket-approval` are explicit live gates;
- Runtime candidate/dispatch/evidence assertions follow
  [Core Runtime Verification](../00-core/runtime/verification.md).

## Related Documents

- [Block Boundaries](../30-module-architecture/block-boundaries.md)
- [Capability Catalog](../10-domain-model/capability-catalog.md)
- [Service Command API](../50-product-interface/service-api.md)
- [Service Read API](../50-product-interface/service-read-api.md)
- [Service Acceptance](../50-product-interface/service-acceptance.md)

## Preparation observation

Workspace 的显式准备可以使用 MCP owner 独立的版本化 stdio initialize/list/close 观察；
它只证明原检查时点的协议及选中能力，不执行业务工具、授予 credit 或持有运行会话。
参数、预算、原操作重放/unknown 与非 stdio fail-closed 边界见
[MCP owner contract](../../internal/modules/blocks/mcp/readme.md#激活前-preparation-协议观察)。
Runtime 六入口及 execute 的完整授权链保持不变。

stdio 客户端首选协议 `2025-11-25`，接受服务端协商选择 `2024-11-05` 或 `2025-11-25`，
不以版本日期大小或请求/响应字符串相等推断兼容。Runtime 与 preparation v2 共用 MCP owner 的明确支持集合；
不支持或无效初始化在 initialized/list/业务调用前拒绝并关闭。Runtime session 保存真实协商版本及原 result，
preparation 使用原始 frame 独立反证。旧 preparation v1 的既有判定保持不变，不能升级历史验收；新检查产生 v2。
