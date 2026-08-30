# workorchd Application Module

Status: active

Type: application-module-contract

Last Verified: 2026-08-30

## 功能范围与边界

`application/workorchd` 组合 HTTP/Web、CLI/TUI facade、typed bootstrap、application use case、read model、projection、
SSE 和 runtime session 生命周期。它负责把用户入口转换成接收 owner 定义的 concrete command，并把 owner fact 转换成
detached public DTO；不拥有 Workspace、Run、Result、Decision、Approval、Memory、Skill、Soul、MCP Registry、
Eventstream 或 Runtime SM-1～SM-4 的正式状态。

本模块拥有三类进程内状态：统一 ingress barrier、`runtimeComponent` 持有的长期 session，以及
`projection.Component` 持有的派生 Store/后台任务。三者只服务应用生命周期，不能成为业务 authority、恢复队列或跨 owner
状态机。

Workspace 的启动输入是 Goal、DoD、Template/Profile snapshot 与 resolved project directory。Application 只做 typed
解析、规范化、snapshot assembly 和提交；Workspace owner 独占 `DefinitionAdmissionCandidate`、promotion、
`StartupInputHash` 与 activation eligibility。Definition 只接受完整 `all_of`；unsupported relation 必须形成 blocking
clarification，不能创建 Run。启动输入封存后，执行期 LLM 处理不得修改它。

`resume` 是既有 Run 的 continuation，不携带 fresh bootstrap；`restart` 使用当前 eligible Workspace definition 编译 fresh
bootstrap。两者都进入 Workspace owner 的 workspace-scoped execution admission，Application 不直达 Run mutation，也不创建
replacement successor。

## 组件交互及合同

| 交互方 | 方向 | concrete contract | Application 责任 |
| --- | --- | --- | --- |
| `blocks/workspace` | 双向 | Workspace/Goal/DoD/Template CRUD、definition admission/promotion、execution intent、single deletion intent、typed read | 规范化输入、校验 exact result scope、形成 DTO；不选择 owner outcome |
| `blocks/run` | 双向只读 | Application → Run 的 Run/Step/Checkpoint/status read；Run → Application 的 immutable `RunStateChangedData` | `RunPorts` 不含 lifecycle、Runtime CAS 或 deletion mutation；通知只更新派生 read model |
| `kernel/orchestrator` | 双向只读 | immutable drive observation、ToolCall/runtime audit query | 不调用 reducer、不持有 aggregate、不生成 dispatch |
| `blocks/llm` | Application → LLM | active configuration、catalog/refresh、selection validation、generic completion/embedding | 只调用 LLM owner；generic result 必须与 exact command/target 绑定并 detached，不直连 provider |
| Decision / Approval / Governance | 双向应用用例 | scoped query、generic resolution、trace query | generic resolve 不发布 Resume；Definition clarification 不进入 generic resolve |
| Result / Memory / Skill / Soul / MCP Registry | Application → owner | concrete CRUD/query、bounded content read、consistent selection snapshot | 校验 identity/revision、deep clone；不读取 owner repository 或私有路径 |
| Eventstream | 双向只读 | bounded event/high-watermark query、`EventRecordAppendedData` | high-watermark result 必须精确匹配 normalized workspace scope 并 detached；projection/SSE 不从审计重建 authority |
| File / Git / Shell / SSH / Web / MCP executor | 无直接执行 I/O | 无 | operation 只由 Orchestrator sealed dispatch 调用 |

所有跨 owner I/O 都从接收 owner 的 `pkg/events` 导入具体 Command/Data/Result。HTTP DTO、CLI DTO、projection DTO 与
EventHub DTO 互不复用；response 在跨层前校验 workspace/run/item identity 并 deep clone。

### Application lifecycle

- `biz.Biz` 是 EventHub 订阅生命周期根；transport 不保存 Hub 或 Runtime Session。
- `runtimeComponent` 是唯一长期 runtime session owner；借用端口不复制 Session 指针。
- `projection.Component` 是唯一 projection Manager/Store lifecycle owner；server 只持有无生命周期 reader/write port。
- Stop 顺序固定为关闭 admission、drain CLI/HTTP、等待 in-flight、解除应用订阅、关闭 projection/background tasks，最后
  关闭 runtime session。重复 Stop 幂等。

### Admission and deletion ingress

- HTTP、CLI 与 TUI execution 都进入同一个 workspace-scoped owner lane；重复 RequestID 每次交给 Workspace/Run owner 的
  durable idempotency contract，Application 不保存 queue、pending map 或 retry successor。
- owner 返回具体 execution admission 后，Application 才可写 immutable accepted/rejected projection；transport/bootstrap
  error 不能伪装成 owner rejection。
- DELETE 只发送一个 `DeleteWorkspaceCommand`。Application 不预读 Run、不枚举 owner、不清理目录、不组合 purge saga；
  Workspace owner 的 durable deletion journal 负责 fence、quiescence、purge 与恢复。
- `WorkspaceDeletedData` 只进入 process-local SSE replay/fan-out，不追加回 Eventstream，也不触发任何 owner transition。

### Bootstrap and projection

- bootstrap compiler 无损封存 definition/profile、provider、Soul、Skill、MCP、Memory、evidence policy、budget 与 directory
  snapshot；所有 current-shape admission binding 由 Workspace owner pure compiler 重验。
- Application 可以定义 profile 到 typed focus/proof declaration 的映射，但不得把 profile-specific action 逻辑下沉到
  kernel reducer。required source 缺 exact executable proof 时必须在 pre-seal admission 阻塞。
- projection 只保存 bounded immutable public DTO。canonical Workspace/Run state、active Run 与 exact blocker ref 原样投影；
  display summary、consistency diagnostic、recovery hint 与 metrics 都不能生成 mutation intent。
- Artifact、Step、ToolCall 和 RuntimeCheckpoint 内容只经其 owner 的 bounded typed read；禁止以 `dataRoot/backend`、
  `os.ReadFile` 或 raw-file fallback 绕过 owner。

## 不变量与非职责

- Application 不写任何 sibling owner repository/service，不拥有 SM-1～SM-4 reducer、delivery phase 或 deletion phase。
- 生产代码不得引入 generic envelope、`map[string]any` authority、JSON/reflect bridge、callback dispatcher 或双 Hub 兼容层。
- 不存在 application Start/Resume queue、inactive-run recovery、stale-step watchdog、owner readback retry 或 caller-controlled
  Run priority；恢复只来自 owner aggregate 的幂等 re-drive。
- projection failure 只能改变派生 Store 健康度；不得回写或修复 owner state。opaque Runtime aggregate 不进入 Store/HTTP cache。
- Decision/Approval resolve、display blocker、SSE、metrics 和 context bundle 都不能选择 governance continuation、Restart 或
  terminal。
- Runtime raw provider body、reasoning 与 credential 不进入 public content API、projection、SSE 或 audit DTO。
- 当前配置严格拒绝 unknown/retired field；Application 不提供 data migration、legacy decoder 或 compatibility outcome。
- repository builder、owner fixture 与 direct repository write 只允许在 `biz/testkit` 或 `_test.go`。
- `WorkspaceProjectionChangedData` 是 Application self-notification；当前 consumer 尚无 exact `application/workorchd` source gate，
  不能把该路径计作已完成的 producer-authorization 边界。

## 能力验收

| 验收面 | 自动化证据 | 覆盖维度 |
| --- | --- | --- |
| Module/port/session lifecycle | `module_test.go`、`biz/application_ports_test.go`、`biz/runtime_component_test.go`、`biz/owner_stop_contract_test.go` | setup failure、port completeness、single owner、ordered/idempotent shutdown |
| workspace-scoped execution admission | `biz/execution/*_test.go`、`biz/execution_admission_gateway_test.go`、`service/server/workspace_execution_admission_typed_test.go` | normal/rejected、scope mismatch、duplicate、concurrency、transport failure、zero queue side effect |
| Definition/bootstrap/restart | `biz/appservice/activationports/definition_admission_test.go`、`biz/appservice/activationports/sealed_restart_snapshot_test.go`、`biz/appservice/activationcore/*_test.go`、`biz/appservice/workspacecore/directory_layout_test.go`、`service/server/http_test.go` | all-of admission、profile proof closure、bounds、directory safety、sealed snapshot、fresh restart ingress |
| owner reads and content isolation | `biz/appservice/*_service_test.go`、`biz/appservice/contextregistryports/*_test.go`、`service/server/production_transport_boundary_test.go` | nil/wrong result、identity/revision drift、pagination cycle、deep clone、private-path rejection |
| Eventstream high-watermark result | `TestReadWorkspaceProjectionHighWatermarksRejectsResultContractDrift`、`TestReadWorkspaceProjectionHighWatermarksReturnsDetachedCanonicalResult` | 覆盖 exact requested scope/order/revision 校验与 detached clone |
| LLM embedding result | `TestGenerateEmbeddingRejectsCompletionResultOnEmbeddingTopic`、`TestGenerateEmbeddingRejectsInvalidConcreteResult`、`TestGenerateEmbeddingReturnsDetachedVectorAndUsage` | 覆盖 topic/result type、exact target/vector fact 与 vector/usage detachment |
| projection/read model/SSE | `biz/projection/*_test.go`、`biz/appservice/readmodelprojection/*_test.go`、`service/server/projection_sse_test.go` | legal/illegal lifecycle、duplicate/stale event、restart generation、failed/degraded recovery、transport detachment |
| Application self-source authorization | 无直接自动化证据 | 未覆盖：`WorkspaceProjectionChangedData` consumer 尚未拒绝 forged/noncanonical source |
| Workspace deletion ingress | `biz/appservice/appmodelcore/workspace_deletion_test.go`、`biz/appservice/workspaceports/workspace_command_payload_test.go`、`service/server/event_subscriptions_test.go` | stable RequestID、single owner command、duplicate disposition、SSE-only receipt；payload-conflict/response-loss 的 durable replay 属于 Workspace owner 验收 |
| persistence and owner boundaries | `biz/testkit/runtime_integration_test.go`、`biz/production_setup_test.go`、`service/server/production_transport_boundary_test.go`、`service/server/application_port_validation_test.go` | memory/file assembly、typed production wiring、repository/service/EventHub AST fences |

真实 HTTP/file-backed/managed-service 场景由
[`docs/50-product-interface/service-acceptance.md`](../../../../docs/50-product-interface/service-acceptance.md) 与
[`deployed-convergence-scenarios.md`](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) 验收；它们不替代
上表的组件级合同测试。
