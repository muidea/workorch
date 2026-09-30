# workorchd Application Module

Status: active

Type: application-module-contract

Last Verified: 2026-09-10

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

Goal-only 创作用例先保存 Workspace owner 的版本化草稿，以持久 reserve permit 调用派生和六类独立审查，确认 exact proposal/specification hash 后才物化正式定义并准入。阻塞后继沿同一候选 lineage 重新审查；Application 只协调 Decision ticket 的替换或 resolution，Workspace 验证其持久来源并独占 pending → eligible 转换。刷新投影以 owner 查询为准；pending startup 显示 `admitting`，不能显示已准入或启动 Run。当前正式创作已接稳定 management ports 和 HTTP 创建／查询／修订／编译／确认 route，创建 RequestID 由 Workspace owner 持久验证；Web 创建向导已改为单一创作／审查／确认入口，发送前保存创建身份，重新打开按 owner 查询恢复。生产 HTTP 编译由 Application 的 BackgroundRoutine 托管，接受回执返回 202，任务使用独立于 HTTP 请求的生命周期 context，Stop 取消并排空已接受任务；任务数有界，结果与预算仍只在 Workspace ledger。已 reserve 但结果未决不重发 provider。当前尚未完成旧 REST 创建旁路收口、完整端到端恢复及显式启动验收，不能以 Web helper 测试或构建通过替代这些合同。

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

- 可选 `ImplementationWorkPolicy` 只从匹配 Profile 声明实例化到 exact CWI 的不可变 Definition，wildcard/exact
  必须一致；不修改原 CWI/check/最终 validation proof，不把工作 operation/selector 写入 mutable focus。
  合并后的非 validation-only CWI 拒绝，Runtime 独立反证 Profile source。当前协议仍被执行支持门禁拒绝；
  编译成功不是可执行工作授权。`runtime_implementation_work_test.go` 覆盖精确匹配、冲突、隔离、身份与门禁。
  已匹配 CWI 的工作 operation kinds 同时编入 Definition capability contract、对应 owner 的 state-change
  inventory prerequisite 和 operation-admissibility prerequisite；不从未匹配策略或最终验证权限推导。
  Runtime 独立校验该投影的完整性；能力清单不是 CWI 工作 binding，也不扩大最终 proof。

- builtin-defaults query 只汇总 Soul/Workspace 的 detached concrete result；单资源 upgrade 只向对应 owner 发出带
  expected/target content hash 的 typed command。Application 不维护升级状态、不得用 query 结果代替 owner 写入时校验，
  也不隐式刷新 Workspace binding/admission 或重启 Run。
- bootstrap compiler 无损封存 definition/profile、provider、Soul、Skill、MCP、Memory、evidence policy、budget 与 directory
  snapshot；新 Start/Restart 自动读取当前 Template/Soul 并提交 Workspace owner-private admission，不要求手动刷新或重建
  Workspace。旧已准入 candidate 只作 lineage，不用当前 compiler 重验；新 candidate 严格按当前合同准入。Resume/recovery
  不重新读取当前资源。capability requirement 必须是终态证明
  kinds 与 Profile validation-repair route kinds 的并集，因此 repair-only owner operation 可被启动时采集，但不得被加入终态
  `AcceptedStepKinds`。authority-bearing route 数组按原 cardinality 严格解析；`null`/scalar/empty/unknown、重复 identity 或
  duplicate exact trigger 不能被过滤、折叠或默认补全，且 repair kinds 未共同声明 concrete owner confirmation protocol 时
  必须阻塞 pre-seal。
- Application 可以定义 profile 到 typed focus/proof declaration 的映射，但不得把 profile-specific action 逻辑下沉到
  kernel reducer。required source 缺 exact executable proof 时必须在 pre-seal admission 阻塞。
- Profile 集合在 Template/Workspace merge 前检查可无损转换的对象/数组形态；同一输入层的空/重复成员必须保留给 typed
  owner admission，不能用后写覆盖隐藏 required 义务。只有两层各自唯一的同 identity 项可按显式配置覆盖规则合并；
  该配置规则不是语义一致性证明。final bootstrap 独立验证同一完整 Profile，不能依赖前一 adapter 已检查。
- Profile 的原始配置还必须符合当前对象层级的封闭字段与精确值类型；未知/错层字段、布尔字符串、混合/空白成员列表、
  非整数或越界数值均在两侧 merge 前返回输入错误。final activation 独立检查；不把非法配置转为默认值，也不伪造
  Workspace admission 或 Runtime 状态。既有 repair lossless decoder 保留其 owner 裁决路径。六个内置模板、两侧
  覆盖、直接 activation 和正常 payload 重载由 `activationports/runtime_profile_input_test.go` 验证；SM-1～SM-4 不变。
  `activationports/runtime_profile_decoder_contract_test.go` 在测试中逐对象核对实际 decoder 的字段与转换类型，防止
  校验器和 decoder 单边漂移；该 AST 索引不是生产 registry 或运行期权威重建。
- projection 只保存 bounded immutable public DTO。canonical Workspace/Run state、active Run 与 exact blocker ref 原样投影；
  display summary、consistency diagnostic、recovery hint 与 metrics 都不能生成 mutation intent。
- `contextHints.executionLimits` 的 `runtime-execution-limits/v1` 收紧 Run 级 provider call 与 capability operation 上限，
  并可选 `maxProofBatchesPerCWI`（1～17）；该字段只封存进 typed activation，由 Orchestrator 原 semantic ledger 消费，
  不编译为物理调用 dimension，也不是一般工作迭代授权。省略不附加逻辑上限，显式零/错类型/超界拒绝。
  两侧原始配置先独立检查，再按 Template 优先合并；effective typed identity 进入 Workspace admission policy binding，
  final bootstrap 反查该封存。`activationcore` 将声明编译到既有预算 policy，不增加计数 owner、调用入口或执行权限。
  `activationports/runtime_execution_limits_test.go` 与 `activationcore/runtime_execution_limits_test.go` 覆盖无损输入、
  合并、完整准入身份、当前协议重载与 policy 重封存漂移；未知协议、缺字段和扩大默认硬上限拒绝。
- blocking input-review 详情从 Orchestrator 的 sealed original diagnostic 深拷贝到 audit/outcome DTO；Web 的只读展开项
  保留全部 check/obligation、输入身份与原因，不受摘要标签的条数截断。脱敏文本及原报告 hash 原样显示，不能据此
  生成 approval/retry/验收权限，也不能把 review 当作实际 validation 或有效进展。
- Run heartbeat 区分 `latestActivityAt` 与 `latestEffectiveProgressAt`。事件、LLM 调用及普通 artifact/patch/result 时间只形成
  activity；effective progress 仅使用当前 Run 已被 Orchestrator 接受的 exact committed receipt（CWI proof 或 durable changed
  repair）及当前 Run accepted Result。activity age 用于心跳指标；局部有效进展不能覆盖 failed terminal。重复失败只来自
  当前 failure signature 的 owner count > 1 或正式 exhausted fact，不继承旧 projection 的 retry 标记。
- 版本展示分为 builtin-defaults catalog current/target、Workspace admission 的 template revision/snapshot/input hash，以及
  Run audit 的 sealed startup/profile hash 与 Soul stage/id/revision/content hash/provenance。展示不实时读取 catalog 替换
  Run snapshot，不泄漏 sealed guidance；这些 detached metadata 不能反向构造 Workspace admission 或 Runtime authority。
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
- `WorkspaceProjectionChangedData` 是 Application self-notification；consumer 只接受 exact `application/workorchd` source 与
  concrete data，forged/noncanonical source 不得进入 SSE transport。

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
| activity/progress 与封存版本 | `biz/appservice/readmodelprojection/progress_owner_facts_test.go`、`runtime_audit_test.go`、`biz/appservice/workspacecore/workspace_test.go`、`biz/appservice/workspaceviewcore/store_metrics_activity_test.go` | exact current-run receipt、activity 不冒充 proof、当前 signature 重复计数、旧 projection 零继承、catalog/admission/Run 分离、metadata clone 与禁止反向重建 |
| Application self-source authorization | `TestWorkspaceProjectionChangedConsumerRequiresExactApplicationOwnerSource` | 覆盖 exact owner、forged/noncanonical source 与 wrong concrete payload |
| Workspace deletion ingress | `biz/appservice/appmodelcore/workspace_deletion_test.go`、`biz/appservice/workspaceports/workspace_command_payload_test.go`、`service/server/event_subscriptions_test.go` | stable RequestID、single owner command、duplicate disposition、SSE-only receipt；payload-conflict/response-loss 的 durable replay 属于 Workspace owner 验收 |
| persistence and owner boundaries | `biz/testkit/runtime_integration_test.go`、`biz/production_setup_test.go`、`service/server/production_transport_boundary_test.go`、`service/server/application_port_validation_test.go` | memory/file assembly、typed production wiring、repository/service/EventHub AST fences |

真实 HTTP/file-backed/managed-service 场景由
[`docs/50-product-interface/service-acceptance.md`](../../../../docs/50-product-interface/service-acceptance.md) 与
[`deployed-convergence-scenarios.md`](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) 验收；它们不替代
上表的组件级合同测试。
