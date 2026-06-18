# WorkOrchD State Loop And Projection Closure Plan

Status: closure-verified

Type: closure-plan

Last Updated: 2026-06-12

## Purpose

本文档汇总当前针对 `workorchd`、Web/TUI 展示、workspace projection、EventHub 通讯、LLM 上下文一致性和多 workspace 并行执行的核对结论，用于后续逐项收口和验收。

当前阶段不考虑兼容性。实现应优先满足以下设计预期：

- `workorchd` 是服务端 ViewModel/projection owner，负责全量加载 workspace、实时维护 workspace 状态投影，并对 Web/TUI 输出整理后的状态。
- Web/TUI 只是 View/Presenter 和输入交互入口，不拥有正式状态，不自行 fan-out 聚合 owner 状态。
- 正式状态仍由 kernel owner module 管理，跨 module/block 的正式状态读写和状态机转换只能通过 EventHub command/response 完成。
- LLM 请求上下文来自 run 执行快照和运行期 evidence，不来自 TUI/Web 显示态。
- 多 workspace 并行执行时，不同 workspace 的状态投影、刷新请求和事件处理不能被不必要的全局串行化阻塞。

## Closure Progress

### 2026-06-10 收口进展

已完成并通过受影响范围测试：

- `workspaceviewcore.Manager` 已将 workspace scoped event 从 full rebuild pending 中拆出，full rebuild in-flight 期间只记录 pending workspace rebuild；global workspace/list 事件才触发 follow-up full rebuild。
- `workspaceviewcore.Manager` 的 workspace rebuild 去重状态已按 workspace entry 聚合为 `workspaceRebuildState`，不再用 `rebuilding`、`rebuildPending`、`pendingWorkspaceRebuilds` 多张 map 表达同一 workspace 的 rebuild 状态。
- `workspaceviewcore.Store` 已按 workspace projection entry 聚合管理 overview、snapshot、revision、source revision、projected source revision、stale、error、rebuiltAt，避免同一 workspace 状态分散在多张 map。
- full rebuild 已拆成 list baseline 与 per-workspace detail refresh；单 workspace detail 失败落到对应 workspace degraded/stale，不导致 workspace list 不可用。
- projection changed payload 已包含 `sourceEventRevision`、`projectedSourceRevision`、`staleReason`、`rebuiltAt` 等状态字段。
- TUI workspace context request tracking 已从全局 seq 改为 per-workspace seq，避免 A workspace 请求使 B workspace 响应被误判过期。
- TUI projection event refresh 已按 scope 分流：`workspaceId==""` 刷新 workspace list，active workspace detail event 只刷新当前 context，非 active detail event 不刷新主视图。
- TUI workspace 切换已增加 workspace display cache：切出 workspace 时保存展示态，切回时恢复目标 workspace 的 task/result/progress/blocker/runtime event 等显示信息；无缓存目标仍清空旧视图并请求最新 projection。
- TUI workspace runtime state 已按 workspace entry 聚合管理 display cache、last event revision 和 context request seq，避免 workspace 切换、事件 gap 检测和过期响应判断分散在多张 map 中。
- HTTP server run snapshot cache 已按 `workspace/run` cache key 聚合 cached snapshot 与 in-flight warmup，cache invalidation 只清理 cached snapshot，不破坏正在进行的 snapshot 读取等待。
- 已同步核对 appservice/server/TUI 长期运行态 map：当前仍保留的 `workspaceSetupDraftSessions` 是单一 draft session map，`workspaceOrder` 是排序辅助结构，配置 map、DTO payload、allowed key 集合、函数局部索引不属于同一业务对象运行态拆分；本轮无需继续拆改。
- read model snapshot 已增加 `runHeartbeat` 派生投影，从 TaskProgress、EventRecords、LLM usage、Result、Artifact、Patch 等 evidence 中输出 latest progress、activity state、no activity / ineffective retry 信号。
- run heartbeat 已补基于投影生成时间的 10 分钟 stalled 阈值判定：live run 无有效进展或最近有效进展超过阈值未更新时标记 `stalled`；等待决策、等待审批、终态和无效重试不被时钟阈值覆盖。
- TUI `/status` 已展示 run heartbeat 的 activity、summary 和 latest effective progress，remote backend 与 HTTP client 已补齐 heartbeat 解码契约。
- TUI progress text 已识别 `stalled` heartbeat，避免服务端判定停滞后端侧仍显示普通 running。
- Web console 已补齐 `ProjectionStatus.projectedSourceRevision/staleReason` 与 `runHeartbeat` 类型契约；Workspace Board 使用统一 projection summary，Workspace Detail 展示 projection status 与 run heartbeat。
- LLM prompt cache 稳定性已补测试：相同 Goal/DoD/Capability 下，volatile progress/current action 变化不会改变 C0/C1/C2 cacheable prefix hash / cache key，只改变 volatile tail hash。
- `workorchclient.ProjectionStatus` 已补齐 `projectedSourceRevision` 与 `staleReason` 解码契约。
- 新增 deployed-instance 验证脚本 `scripts/verify-workorch-projection-workspace-switch.sh` 与 Make 目标 `verify-workspace-projection-switch`，覆盖两个 workspace 的 scoped projection changed SSE、workspace list/detail projection metadata、projection rebuild metrics、以及无 workspace scope run action 不成功。
- `verify-workspace-projection-switch` 已强化 scoped projection changed SSE contract：workspace detail event 必须携带 `globalRevision`、`workspaceRevision`、`sourceEventRevision`、`projectedSourceRevision`、`updatedAt`、`rebuiltAt`，并要求 `projectedSourceRevision >= sourceEventRevision`；workspace snapshot/list projection 也要求具备 revision/timing metadata，避免 Web/TUI 只能靠弱刷新信号判断状态。
- 已通过 `verify-workorch-managed-full.sh` 自动管理临时 `./bin/workorch` 实例执行 `verify-workspace-projection-switch`，验证服务端 projection refresh contract 可在 file-backed 部署实例上闭环。
- governed concurrency loadtest 已对齐当前 completion gate contract：Goal/DoD 只描述 LLM assessment 接受前可由 evidenceRefs 证明的事项；Result/Artifact 物化由 runtime 接受后最终断言覆盖。
- Decision/Approval resolve 生产链路和 e2e 均已携带 workspace scope，避免多 workspace 下相同 `run-001-*` ticket/approval ID 串扰。
- Decision/Approval governance resolve 的 owner read 命令已统一绑定显式 workspace scope：即使调用方原始 `context.Context` 未携带 runtime workspace，single/batch resolve 也会先把 query/path scope 绑定到 command context，再发布 `decision get ticket` / `approval get` 读取命令；batch resolve 复用预校验读取到的实体发起后续状态转换，避免先全局读取再后置过滤导致重复 ID 串扰。
- Decision/Approval 详情读和 workspace execution context input 读取也已绑定显式 workspace scope：`DecisionView` / `ApprovalView` 在读取单个治理对象前绑定 scope；activation 读取 Goal/DoD 时使用目标 workspace context，避免同名 Goal/DoD 在不同 workspace 下被无 scope 读取误命中。
- Activation 治理历史和 start conflict 恢复读取已绑定显式 workspace scope：`WorkspaceGovernanceHistory` 在读取 workspace ticket 后，继续以同一 workspace context 读取 Run 与 DecisionResolution；`RunObservableAfterStartConflict` 的调用方在 StartRun conflict 后使用 startCtx 读取 run，避免同名 `run-001` 在多 workspace 下被误判为当前 workspace 的可观测 run。
- Runtime raw content 与 artifact fallback 读取已按 URI workspace scope 前置绑定：`runtime-checkpoints` / `results` 虚拟 raw content 读取使用 ref 中的 workspace scope 发布 owner read command；artifact content fallback 在按 run 列出 artifact 前使用 URI workspace scope，避免先无 scope owner 读取再后置过滤。
- Governance list query、run definition validation、run recovery candidate 和 memory registry/selection 读取已进一步按已知 scope 前置绑定：Decision/Approval 同时带 `workspaceId/runId` 查询时先绑定 workspace scope 再 list-by-run；`ValidateRunDefinitions` 在并发读取 Goal/DoD 前绑定 workspace scope；run recovery helper 根据 `run.WorkspaceID` 自行绑定 checkpoint/step/continuation checkpoint 读取；activation explicit memory selection 与 context registry memory read/list/search 使用 owner read port 的 scoped command，memory create/update/delete 写命令也携带 workspace/run envelope 和 payload scope；workspace 存在性校验改用 workspace owner read port，不再绕过统一 command envelope。
- Memory 单对象 REST/client/port 已改为显式 workspace scope：HTTP 支持 `/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId` GET/PUT/DELETE，旧 ID-only path 至少要求 `workspaceId` query 后绑定 runtime scope；`workorchclient` 与 `workorchport.MemoryBackend` 的 Get/Update/Delete 均携带 workspace ID；CLI memory get/update/delete 已要求 `--workspace`，embedded EventHub backend 会把 workspace ID 写入 CLI command envelope。
- Decision/Approval 单对象与 resolve REST/client/port 已改为显式 workspace scope：`workorchclient`、`workorchport`、remote/eventhub backend、CLI、TUI、agent 和 e2e 脚本在 Get/Resolve/Approve/Reject/BatchResolve 路径均传递 workspace ID；server CLI governance command 会先把命令 payload 中的 workspace scope 绑定到 runtime context，再发布 owner command，避免多 workspace 复用 ticket/approval ID 时误读或误恢复其他 workspace。
- 部署实例只读观测发现部分历史/quiet workspace 的 snapshot projection 在 `state=ready` 时缺少 `sourceEventRevision/projectedSourceRevision` 字段：根因是 workspace 自身没有 event revision 时 status 返回 0，JSON `omitempty` 把字段省略。已调整 `workspaceviewcore.Status` 的 revision 字段为稳定输出，并在 workspace status 中对 quiet workspace 使用全局 source/projected revision fallback，避免 Web/TUI display contract 因字段缺失失败；需重新部署后对 `/api/v1/workorch/workspaces/:workspaceId/snapshot` 复验。
- Goal/DoD quality check 的显式 definition ID 读取已绑定 workspace scope：`CheckGoalDoDQuality` 在解析目标 workspace 后使用 scoped context 读取 Goal/DoD，避免质量预检在多 workspace 同名 definition ID 下读取错误对象。
- Goal/DoD update/delete 写命令已从 runtime context 继承 workspace scope：`UpdateGoalView` / `UpdateDoDView` 虽然保持 ID-only facade，但内部 `UpdateGoal` / `UpdateDoD` / `DeleteGoal` / `DeleteDoD` 会在显式参数为空时读取 `RuntimeWorkspaceID`，并把 workspace 写入 owner command envelope 与 payload。
- Result view projection 已保留结构化 `doDAssessment` 与 `summaryDetail`，Web/TUI/HTTP snapshot 可以直接消费服务端 ViewModel 输出，不再因 detail view 投影裁剪导致前端/验证侧重新推导 Result 详情。
- 已通过自动管理临时 `./bin/workorch` 的 governed concurrency e2e：2 个 workspace 并发执行均完成 2 个 Decision、1 个 Approval、MCP tool call、ValidationResult、RuntimeCheckpoint、accepted Result 和 2 个 Artifact，且无 LLM protocol error、无 timeline runState 缺失。
- HTTP workspace snapshot cache 只在同一 workspace 恰好存在一个新鲜 terminal run snapshot 时复用 cached run snapshot；同一 workspace 多 terminal run cache 时强制回退权威读取，避免 map 迭代随机选择旧 run 污染 workspace detail。
- workspace-scoped file repository 的单对象读取已统一为 scope-aware unique lookup：Goal、DoD、MemoryItem、MemoryRecallTrace、Step、ToolCall、Patch、ValidationResult、RuntimeCheckpoint、ArtifactContinuation、Result、Artifact、EventRecord、DecisionTicket、DecisionResolution、Approval 等对象无 scope 多命中时返回 conflict，带 workspace scope 时只在对应 workspace 内解析。
- 已补仓储级回归测试，覆盖 workspace-local Definition/Governance 对象重复 ID 的无 scope conflict 与带 scope 精确读取。
- workspace-scoped file repository 已清理失效的 `runRootsByID` 运行期索引，run root 不再保留裸 `runID` cache；重复 `runID` 的解析完全依赖 workspace scope 或唯一性扫描。
- memory repository 已同步 workspace-local ID 语义：Goal、DoD、MemoryItem、MemoryRecallTrace、Run、Step、ToolCall、Patch、ValidationResult、RuntimeCheckpoint、Result、Artifact、EventRecord、DecisionTicket、DecisionResolution、Approval 均改为 workspace scoped key；无 scope 多命中返回 conflict，带 workspace scope 精确解析。系统级 Soul、Skill、MCP Server、MCP Capability、Workspace 等仍保持全局 ID。
- 已补内存仓储回归测试，覆盖 Definition、Memory、Run/Step、Governance、Result/Artifact/EventRecord 的重复 workspace-local ID 作用域行为。
- 通用工具/能力采用 system default + workspace overlay 规则：MCP Server/Capability 等通用目录由系统级 owner 统一维护；workspace 仅在显式设置 default/requested/required capability 或需要 env placeholder 投影时形成 workspace 运行期选择，不为每个 workspace 复制系统能力实体。若 workspace 未设置覆盖项，运行期直接复用系统公共定义；若后续引入 workspace-specific tool override，必须只保存覆盖字段，未覆盖字段回退系统公共配置。
- 创建 workspace 时不再把 template 中的通用工具默认选择（`defaultMCPServerIds` / `defaultCapabilityIds`）和通用工具 managed config 定义（`managedWorkspaceConfigs`）批量复制到 workspace context hints；activation 读取时重新合成 template/system 公共默认与 workspace overlay，workspace 显式设置优先，未设置字段回退公共配置。
- 通用工具 managed config 采用按需 materialize 与按需更新：workspace 创建阶段不再为每个 workspace 生成一份配置文件；启动执行时把 template/system 公共 managed config 写入 `dataRoot/system/managed-configs` 并允许公共默认更新覆盖旧系统文件。workspace 只有显式提供 `managedWorkspaceConfigs` overlay 时才写入 workspace 内部配置；MCP env 通过 `{workspaceManagedConfig:<path>}` 解析为 workspace 内部覆盖优先，否则复用系统公共配置。系统公共配置是通用工具的默认事实来源，不随 workspace 数量膨胀；workspace 私有配置仅表达差异。
- governed concurrency loadtest 已补 active run 并行证据：结果明细输出关键 run 推进阶段的起止时间，summary/report 输出 `maxObservedActiveStageOverlap`、`maxObservedRunRequestsInFlight`、`expectedActiveRunOverlap` 与 `activeRunOverlapPass`；设置 `WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP=1` 时可作为 `maxConcurrentRuns>1` 的并行推进 gate。已通过自动管理临时 `./bin/workorch` 的 2 workspace 回归，`serverMaxConcurrentRuns=5`、`expectedActiveRunOverlap=2`、`maxObservedActiveStageOverlap=2`、`maxObservedRunRequestsInFlight=2`、`activeRunOverlapPass=true`。
- Web Workspace Detail 已移除端侧 `projectedRunSnapshot` 拼装逻辑：workspace projection 只用于确定当前展示 run，正式 run 详情、outcome、task progress、evidence、Result/Artifact 等均通过服务端 run snapshot / observability / paged runtime evidence API 获取；run 详情请求绑定 workspace ID 与 workspace context request seq，避免 workspace 切换后旧请求回写当前主视图。TUI 侧仍只消费服务端 workspace snapshot/projection DTO 并映射为显示字段，不构造新的正式 RunSnapshot。
- LLM provider prompt 已补 projection-only 输入防回归：即使 step input 误带 `workspaceProjection`、`workspaceSnapshot`、`runSnapshot`、`viewModel` 等显示态字段，provider-facing prompt 也不会包含这些字段；允许进入 prompt 的仍是 `executionContextPackage` 中冻结的 run execution context、`currentProgressDigest`、`processEvidenceTimeline` 等运行期 evidence / step input 白名单字段。
- TUI workspace 切换后主视图和状态栏刷新链路已补模型级自动化验证：确认切换 A/B 后，active workspace projection detail event 会触发 per-workspace context refresh；B 的 context response 会更新 `currentTask`、`activeRunID`、progress/status bar 和主视图；A 的旧 context response 与非 active projection event 不会污染 B 的当前视图。
- TUI stream revision gap 已按 workspace runtime entry 计数：active workspace gap 会触发 context refresh，inactive workspace gap 只记录计数不污染当前主视图；`/status` 会展示当前 workspace 的 last revision、gap count 和 latest gap。
- workspace path scoped Goal/DoD GET/PUT 已绑定 `RuntimeWorkspaceID`，重复 workspace-local definition ID 场景下按 path workspace 精确读取和更新，不再依赖无 scope 裸 ID 解析。
- appservice owner read ports 已为 workspace-local/run-local 读命令携带 `RuntimeWorkspaceID` scope：Step、RuntimeCheckpoint、ToolCall、Patch、ValidationResult、ArtifactContinuation、Result、Artifact、DecisionResolution、MemoryItem、Goal、DoD 等按 workspace scope 发布 EventHub command；MCP/Soul/Skill/Template 等系统级目录仍保持全局读取。
- kernel owner command handlers 已在进入 repository/service 前从 command envelope/payload 绑定 runtime workspace scope：Run、Workspace Definition、Orchestrator Evidence、Result/Artifact、Memory/EventStream 等 owner handler 与内部 state/result/governance adapter 均按 scope 调用 owner repository。
- memory repository 的 Step 与 DecisionResolution 已补齐 scoped lookup fallback：带 workspace scope 时优先读取 workspace scoped key，未命中时只回退公共 key；无 scope 多 workspace 命中仍返回 conflict，避免测试 fixture 或未 scoped legacy 写入导致读模型误判 not found。
- workspace projection 已补基础观测指标：`workspaceviewcore.Manager` 记录 full rebuild / per-workspace rebuild 次数、失败次数、累计耗时、pending workspace rebuild 数、debounced rebuild 数和 scheduler drop 数；server `/metrics` 输出 `workorch_workspace_projection_*` 指标，支持运行期观察 projection storm、pending 堆积和 rebuild 失败。
- workspace projection 已补 run heartbeat 派生观测指标：`workorch_workspace_projection_active_run_heartbeats`、`workorch_workspace_projection_active_run_heartbeat_age_seconds_max`、`workorch_workspace_projection_stalled_runs`、`workorch_workspace_projection_ineffective_retry_runs`，用于运行期定位 active run 停滞、无效重试和最新有效进展老化。
- `verify-workspace-projection-switch` 已把 projection metrics 纳入 deployed-instance gate：要求 `/metrics` 暴露 full rebuild、per-workspace rebuild、pending、debounced、schedule drop、active run heartbeat、stalled run 和 ineffective retry 指标，并确认新建的两个 workspace 均有 workspace rebuild 计数；同一 gate 还校验 projection SSE revision/timing payload，确保端侧按 projection metadata 刷新的契约可在部署实例上闭环。
- `verify-workspace-projection-switch` 已覆盖 Web Console 部署包基础 contract：通过部署实例 `/console` 递归读取入口和动态 JS chunk，确认 bundle 包含 workspace projection SSE topic、projection changed payload marker、projection status 展示入口和 run heartbeat/stalled/ineffective retry 展示入口。
- `observe-runtime-closure` 已作为运行期观测入口接入 Makefile：在已部署或 managed 临时服务上采集 `/readyz`、workspace projection list、`/metrics` 前后快照、SSE 事件、可选 workspace snapshot 和可选 ai-proxy cache hit rate，并输出 `runtime-observation-summary.json` 作为真实运行期核对证据；默认只读，也支持 `WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT` 在 SSE 监听窗口内创建临时 workspace 以刺激 projection/SSE 样本。
- `verify-tui-workspace-switch-visual` 已补文本录制式 TUI workspace 切换视图验收：通过 Bubble Tea model 的 `View()` 输出生成切换前、切换后、projection refresh 后和 inactive projection event 后的文本快照，验证主视图/status bar 绑定目标 workspace，旧 workspace 内容不泄漏。
- `verify-tui-terminal-render` 已补真实 PTY 渲染 smoke：通过 Python 标准库 `pty` 启动 `./bin/workorch --remote <deployed> repl`，创建唯一 workspace，绑定 CLI last active workspace，捕获 Bubble Tea 终端渲染输出，验证远端 workspace projection 可在真实终端 renderer 中显示，并执行 `/status` 后干净退出。
- TUI transient projection retry 已改为一次性 command 延迟返回，不再使用 `tea.Tick` / `time.Tick` / `Ticker`，`verify-tui-agent` 的 no fixed polling guard 已恢复通过。
- `verify-web-console-projection-visual` 已补 Web Console projection/heartbeat 展示契约：本地构建后同时核对源码和部署 dist asset，确保 workspace detail 渲染投影状态、运行活动、最近有效进展、`stalled`、`ineffective retry` 和 projection SSE refresh marker；同时校验 Workspace Detail 读取服务端 workspace projection、workspace-scoped run snapshot 和 run observability，并禁止恢复 `projectedRunSnapshot` / `buildProjectedRunSnapshot` 这类端侧正式状态拼装路径。脚本已进一步支持部署实例 runtime artifact：提供 `WORKORCH_BASE_URL` 时会抓取 `/console`、入口 JS、动态 chunks、`/readyz` 和 workspace projection list，确认当前服务 bundle 含 projection/heartbeat 展示 marker 并输出 `web-console-runtime-visual-summary.json`。
- `observe-runtime-closure` 已支持从 ai-proxy interactions metadata 读取真实 provider usage/cache 样本；本地 `/home/rangh/aispace/ai-proxy/interactions` 观测到 334 个有效 cache 样本，平均 cache hit rate 约 21.53%，最新有效样本约 0.415%（provider=`aiapi`，model=`MiniMax-M3`，stream=true）。
- `observe-runtime-closure` 已补真实 provider cache 样本过滤和基线对比：支持 `WORKORCH_OBSERVE_CACHE_PROVIDER`、`WORKORCH_OBSERVE_CACHE_MODEL`、`WORKORCH_OBSERVE_CACHE_SINCE`、`WORKORCH_OBSERVE_CACHE_UNTIL`、`WORKORCH_OBSERVE_MIN_CACHE_SAMPLE_COUNT`、`WORKORCH_OBSERVE_CACHE_BASELINE_FILE`、`WORKORCH_OBSERVE_MIN_CACHE_AVERAGE_DELTA` 和 `WORKORCH_OBSERVE_MIN_CACHE_LATEST_DELTA`，可把 real-provider 场景的观测窗口限定到指定 provider/model/时间范围，并与历史 summary 做 average/latest 改善阈值校验。
- `verify-real-runtime-closure` 已补真实运行期闭环入口：先执行 `loadtest-governed` 的 real provider 多 workspace 并发场景，再用本次 load 的 workspace IDs 和 load 起始时间调用 `observe-runtime-closure`，统一产出 `real-runtime-closure-summary.json`，用于把真实 LLM/provider 并发推进、projection/SSE/heartbeat 观测和 cache provider/model/时间窗口/baseline gate 串成一个验收动作。该目标不纳入 CI/mock 全量链，需在已部署且真实 provider 可用的服务上显式执行；2026-06-11 已在当前 8080 部署实例上完成一次真实闭环，artifact `/tmp/tmp.95xCC5ldj1`。
- `verify-tui-long-output-e2e` 已明确输出自动化覆盖边界：deployed service 会生成 LONG-IMPLEMENTATION 长结果；`verify-tui-agent` 覆盖 PgDown/PgUp 主视图滚动、workspace monitor 固定和 mouse reporting 禁用；`verify-tui-workspace-switch-visual` 覆盖 workspace switch refresh 文本快照；`verify-tui-terminal-render` 覆盖真实 PTY renderer 启动、远端 workspace projection 渲染、`/status` 输入和退出；long-output e2e 会打开长结果 workspace，发送 PgDown/PgUp，验证真实 PTY 输出包含 LONG-IMPLEMENTATION，通过 PTY screen artifact 断言滚动后 Workspaces panel、底部 prompt/help rows 仍可见，并从 raw PTY 输出负向断言 TUI 未启用 xterm mouse reporting 序列。真实终端鼠标拖选复制体验的可自动化边界已收敛为 mouse selection proxy contract：TUI 不启用 xterm mouse reporting，不接管普通终端拖选事件。
- 真实运行期核对暴露 governance coverage 漏洞：当 LLM 直接返回 `mcp/tool_call` graph 时，MCP validation evidence 曾可误满足 Decision/Approval DoD 条目，且 operator policy MCP tool call 可能绕过硬审批门禁。已收口为：Decision/Approval 文本类 coverage item 默认要求 owner evidence kind；assessment evidence refs 必须分别呈现 decision/ticket/resolution 或 approval 语义；MCP operator policy tool call 在 runtime 执行前由 orchestrator preflight 强制创建并等待 Approval。
- 真实运行期复核进一步确认：`two Decision tickets are resolved` 属于数量语义，不能用一条 Decision resolution 或 LLM 文本评估替代第二张 Decision ticket；coverage ledger 已增加 `requiredEvidenceCounts`，并按 owner ticket/approval ref 计数，resolution ref 只作为附属 evidence 保留，不抵扣 ticket 数量。
- coverage ledger 已在 orchestrator 调度前同步 governance owner evidence：resolved Decision ticket、Decision resolution、approved Approval 会自动写入对应 Decision/Approval coverage item，避免已批准 Approval 因 LLM assessment 未显式引用 owner ref 而长期停留在 open 状态；正式数量不足时仍保持 unresolved，不凭空补齐治理对象。
- `loadtest-governed` real 模式已与 mock 协议解耦：mock 继续验证固定两张 Decision ticket + 一条 Approval 的协议闭环；real provider 场景改为验证 Decision governance 已解决、Approval 已批准和 MCP evidence 已收集，避免把 mock LLM 的固定轮次强加给真实模型。
- `verify-real-runtime-closure` / `loadtest-governed` real 模式已支持临时收敛真实 provider timeout（默认 `WORKORCH_LOAD_REAL_PROVIDER_TIMEOUT=180s`，可由 `WORKORCH_REAL_RUNTIME_PROVIDER_TIMEOUT` 透传），执行后恢复原 LLM 配置，避免部署环境中 10m provider timeout 使单个异常 stream 长尾拖垮 900s 验证窗口。
- `verify-workorch-managed-full.sh` 已支持 `WORKORCH_MANAGED_LLM_CONFIG_JSON=<private-json>`：自动管理临时 `./bin/workorch` 实例 ready 后，先把该私有 JSON 作为 `/api/v1/workorch/configuration/llm` payload 注入，再执行指定验证目标。该入口用于在不污染仓库默认配置、不依赖 8080 部署实例的情况下，用最新代码跑 `WORKORCH_MANAGED_VERIFY_TARGET=verify-real-runtime-closure`；JSON 文件应由操作者本地提供并包含真实 provider/API key，脚本只在 artifact 中保留服务端返回的 redacted configuration response。managed 模式下 `verify-real-runtime-closure` 的 artifacts 默认落到 managed artifact 目录的 `verify-real-runtime-closure/` 子目录，便于一次性归档。
- `verify-real-runtime-closure` 已补真实闭环 preflight：loadtest 前先读取 `/readyz` 与 `/metrics`，并把 `workorch_stale_dispatched_step_min_age_seconds` 与 `WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS` 比较；若 watchdog 阈值不短于 load 等待窗口，则 fail-fast 并在 `preflight/preflight-summary.json` 中保留证据，避免真实 provider 长跑后才发现部署配置无法在窗口内触发 stale-dispatched 恢复。可用 `WORKORCH_REAL_RUNTIME_PREFLIGHT_ONLY=1` 单独执行 preflight，不触发真实 provider loadtest。
- MCP operator approval preflight 已支持 graph step resume：`mcp/tool_call` dispatch 后先记录 dispatch evidence，再检查 `run.ExecutionContext.mcpServers/mcpCapabilities` 的 operator policy；缺少已批准 approval 时创建 `<stepID>-mcp-operator-approval` 并 block run，批准后复用同一 dispatched step 继续执行 runtime。
- completion gate evidence normalization 已补 governance owner refs 规则：无 `/` 的 Decision/Approval owner evidence ID（如 `run-001-step-1-decision`、`run-001-llm-after-decision-2-approval`）属于合法 traceable governance evidence，不再被 completion evidence catalog 丢弃或 fallback 成 MCP step evidence；未知普通 loose note 仍会被过滤。
- governed concurrency loadtest 已跟随硬审批门禁更新：脚本在业务 Approval 后继续循环处理当前 `blockingApprovalId`，直到 run completed；mock LLM 最终 DoD assessment 对 Decision/Approval 条目引用 owner evidence refs，对 MCP validation 条目引用 tool step evidence，验证覆盖治理 evidence owner 与 runtime validation evidence 的边界。
- 已通过自动管理临时 `./bin/workorch` 的 governed concurrency e2e 回归：2 个 workspace 并发均完成，`workspaceSuccessTotal=2`、`workspaceFailedTotal=0`、`successRate=1.0`、`expectedActiveRunOverlap=2`、`maxObservedActiveStageOverlap=2`、`maxObservedRunRequestsInFlight=2`、`activeRunOverlapPass=true`，artifact 目录 `/tmp/workorch-loadtest-load-20260610110141-d8z0a3yx`。
- governance resume scheduler 已移除裸 `runID` 调度入口，内部幂等键统一使用 `workspaceID/runID`；同一 workspace 的重复 resume 会合并，不同 workspace 的相同 `run-001` 会分别绑定 workspace context 并独立恢复。
- 已通过自动管理临时 `./bin/workorch` 的 scoped governance resume 并发回归：`WORKORCH_MANAGED_VERIFY_TARGET=loadtest-governed` 下 2 个 workspace 均使用 `run-001` 并发完成，`workspaceSuccessTotal=2`、`successRate=1.0`、`expectedActiveRunOverlap=2`、`maxObservedActiveStageOverlap=2`、`maxObservedRunRequestsInFlight=2`、`activeRunOverlapPass=true`，artifact 目录 `/tmp/workorch-loadtest-load-20260610111247-eb6lsn1q`。
- 真实 provider 闭环核对暴露 stale dispatched watchdog 阈值问题：`verify-real-runtime-closure` 在 `WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS=420` 下失败，run 停留在 `running` / `currentStepState=dispatched`，同时 `/metrics` 显示 `workorch_run_requests_in_flight=0`；根因是服务端 stale dispatched 默认阈值为 10 分钟，大于 420 秒验证窗口，watchdog 不会在窗口内触发恢复。已新增 `server.staleDispatchedStepMinAge` 配置项，HTTP server watchdog 使用该配置，`/metrics` 暴露 `workorch_stale_dispatched_step_min_age_seconds`；managed verifier 可通过 `WORKORCH_MANAGED_STALE_DISPATCHED_STEP_MIN_AGE` 写入临时服务配置，真实验证脚本通过 `WORKORCH_REAL_RUNTIME_LOAD_RUN_TIMEOUT_SECONDS` / `WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS` 明确 load 等待窗口。
- 真实 provider 失败 artifact 同时暴露 validation evidence 质量漏洞：LLM 生成的 shell 验证命令以 `exitCode=0` 返回 `stdout=0` 时，coverage ledger 曾可能把 completed shell step 绑定为 validation evidence，导致 coverage item 进入 `evidence_ready`。已在 evidence quality 层将 shell validation 的负向标量输出（`0`、`false`、`null`、`[]`、`{}`）判为 inconclusive，并在 DoD evidence kind requirement 与 coverage work item step update 中复用该判断，避免 `stdout=0` 这类“查询为空”结果被误当作通过验证。
- 真实 provider 闭环在已部署服务上复跑暴露配置写入脚本问题：`loadtest-governed` 曾把服务端只读派生字段 `contextProfile.capabilitySource` 原样回写到 `/configuration/llm`，导致严格 JSON decode 返回 400。脚本已把 provider 清洗对齐写入契约，仅保留 `inferenceProfile` 与 `contextProfile` 的可写字段，并继续保留 failover/inference/context 显式配置。
- 真实 provider 闭环在已部署服务上继续复跑得到 artifact `/tmp/tmp.TzcfUkcwSn`：workspace `ws-f562b23cf172a6d6` 已完成第一张 Decision 的 resolve 并持续推进 repair，最终因 `repair-010-of-llm-after-decision` 修复预算耗尽进入 `blocked_on_decision`。根因不是 projection/TUI hang，而是真实 LLM 多轮返回的 concrete validation repair graph 未声明 `input.validationEvidence.mode` 与 `input.validationEvidence.observationType`，被 runtime contract 正确拒绝。已在 `repairActionPackage.requiredOutput` 中加入 `validationEvidenceNodeExample`，并在 provider prompt 最小 executionGraph 示例中补齐 `validationEvidence` 字段；`loadtest-governed` real 模式遇到 `repair-exhausted` ticket 时改为 fail-fast 记录真实修复耗尽，不再用普通 `provide_missing_info` 误 resolve。
- 真实 provider 闭环在已部署服务上继续复跑得到 artifact `/tmp/workorch-real-runtime-closure-20260610232854`：2 workspace real load 中 1 个成功、1 个失败，失败 workspace `ws-2cc1d3005d0cf880` 在 `POST /api/v1/workorch/decisions/<long-ticket>/resolve?workspaceId=...` 返回 HTTP 500，错误为 `file name too long`。根因是 appservice 按 `resolution-<workspaceID>-<ticketID>-<resolutionType>` 派生 Decision resolution ID，真实 repair ticket ID 可被多轮 retry 前缀拉长，file repository 又把完整 ID 作为单个文件名段写入，超过文件系统单段长度。已在 file repository item path 层对每个安全路径段设置 160 字符上限，超过时保留可读前缀并追加 SHA256 hash 后缀；短 ID 文件名保持原样。已补 repository 与 HTTP resolve 回归，确认长 resolution ID 仍可按原 ID 读取，REST resolve 不再因持久化文件名过长返回 500。该修复已通过 2026-06-11 当前 8080 真实 provider 闭环复验覆盖，artifact `/tmp/tmp.95xCC5ldj1`。

已验证：

```bash
go test ./internal/pkg/workorchrepo ./internal/pkg/repository -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1 -timeout=30s
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1 -timeout=60s
go test ./internal/services/workorch/service/tui -count=1 -timeout=60s
go test ./internal/pkg/workorchclient -count=1 -timeout=60s
go test ./internal/modules/application/workorchd/service/server -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/biz/appservice -count=1 -timeout=180s
go test ./internal/pkg/repository/... ./internal/pkg/workorchrepo -count=1 -timeout=180s
go test ./internal/pkg/repository/... ./internal/pkg/workorchrepo ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/biz/appservice/activationports -run 'Test.*(MCP|Capability|Selection|Default|Workspace)' -count=1 -timeout=180s
go test ./internal/pkg/workorchmanagedconfig ./internal/modules/application/workorchd/biz/appservice/workspacecore ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/biz/appservice/activationports ./internal/pkg/workorchdefaults -count=1 -timeout=180s
go test ./internal/modules/blocks/mcp/biz -run 'TestRuntimeAdapterExpandsManagedConfigPlaceholderWithWorkspaceOverride|TestRuntimeAdapterExpandsManagedConfigPlaceholderWithSystemDefault|TestRuntimeAdapterExecutesStdioCommandArgsEnvWhenEndpointMissing' -count=1 -timeout=120s
go test ./internal/modules/kernel/run ./internal/modules/kernel/orchestrator ./internal/modules/kernel/result ./internal/modules/kernel/eventstream ./internal/modules/kernel/memory ./internal/modules/kernel/decision ./internal/modules/kernel/approval ./internal/modules/kernel/workspace -count=1 -timeout=240s
go test ./internal/modules/application/workorchd/biz/appservice -run 'TestReadRunSnapshotReadsDecisionResolutionsConcurrently|TestReadRunStatusUsesLatestCheckpointStep|TestReadRunObservabilityWithSessionReturnsDerivedRuntimeProgress|TestReadWorkspaceSnapshotProjectsActiveRunAndCurrentStep|TestReadWorkspaceSnapshotCanonicalizesWorkspacePlanCurrentStepKind|TestCompressedContextPatchForRunRefreshesCumulativeGoalAndDoD' -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/... ./internal/services/workorch/... ./internal/pkg/workorchclient/... -count=1 -timeout=240s
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore ./internal/modules/application/workorchd/service/server -run 'TestManager.*Projection|TestManager.*Rebuild|TestHTTPMetrics|TestWorkspaceExecutionActivationEndpointCreatesAndStartsRun' -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/... ./internal/services/workorch/... ./internal/pkg/workorchclient/... ./internal/pkg/workorchport/... -count=1 -timeout=300s
go test ./internal/modules/blocks/... ./internal/modules/kernel/... -count=1 -timeout=300s
go test ./internal/pkg/repository/... ./internal/pkg/workorchrepo ./internal/pkg/workorchmanagedconfig ./internal/pkg/workorchdefaults -count=1 -timeout=180s
go test ./internal/pkg/events ./internal/pkg/workorchruntimecontract ./internal/pkg/workorchconfig -count=1 -timeout=120s
go test ./internal/services/workorch/service/tui -run TestModelWorkspaceSwitchThenProjectionRefreshUpdatesMainViewAndStatusBar -count=1 -timeout=60s
go test ./internal/modules/blocks/llm/biz -run 'TestProviderPromptDoesNotIncludeProjectionOnlyInputState|TestBuildProviderFacingPromptGroupsStablePrefixAndDynamicTail|TestBuildPromptContextUsesCacheAwareSectionMetadata|TestProviderCachePlanKeepsCacheablePrefixStableAcrossVolatileProgress' -count=1 -timeout=180s
go test ./internal/modules/kernel/orchestrator/biz -run 'TestCompletionGatePreservesGovernanceOwnerEvidenceRefs|TestNormalizeCompletionGateDoDAssessmentOutputPreservesGovernanceOwnerEvidenceRefs|TestRuntimeOrchestratorBlocksOperatorMCPToolCallBeforeRuntimeExecution|TestCoverageWorkItems.*Governance' -count=1 -timeout=120s
go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s
go test ./internal/modules/kernel/orchestrator/... ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient ./internal/services/workorch/service/tui -count=1 -timeout=300s
go test ./internal/modules/application/workorchd/service/server -run 'TestServerStateGovernanceRunResume(CoalescesDuplicateRunID|AllowsSameRunIDAcrossWorkspaces|UsesWorkspaceContext)' -count=1 -timeout=120s
go test ./internal/modules/application/workorchd/service/server -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient ./internal/services/workorch/service/tui ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s
go test ./internal/modules/application/workorchd/biz/appservice/ownerreadports -count=1 -timeout=120s
go test ./internal/modules/application/workorchd/biz/appservice/ownerreadports ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/result -count=1 -timeout=300s
go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/blocks/llm/biz -count=1 -timeout=300s
npm --prefix web/workorch-console run build
python3 -m py_compile scripts/loadtest-governed-workspace-concurrency.py
git diff --check
bash -n scripts/observe-workorch-runtime-closure.sh
bash -n scripts/verify-workorch-managed-full.sh
bash -n scripts/verify-tui-workspace-switch-visual.sh
bash -n scripts/verify-web-console-projection-visual.sh
make verify-tui-workspace-switch-visual
make verify-tui-terminal-render
make verify-tui-agent
make verify-web-console-projection-visual
make verify-e2e-template-contract
WORKORCH_MANAGED_VERIFY_TARGET=verify-software-dev-goal-dod-e2e WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/modules/application/workorchd/biz/appservice ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=verify-e2e-full WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/pkg/workorchmanagedconfig' ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=verify-workspace-projection-switch WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=verify-tui-terminal-render WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/services/workorch/service/tui ./internal/pkg/workorchclient' ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=verify-tui-long-output WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/services/workorch ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' WORKORCH_OBSERVE_DURATION_SECONDS=3 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' WORKORCH_OBSERVE_DURATION_SECONDS=6 WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT=2 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' WORKORCH_OBSERVE_DURATION_SECONDS=6 WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT=2 WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR=/home/rangh/aispace/ai-proxy/interactions WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=1 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' WORKORCH_OBSERVE_DURATION_SECONDS=4 WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT=1 WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR=/home/rangh/aispace/ai-proxy/interactions WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=1 WORKORCH_OBSERVE_CACHE_PROVIDER=api_test_abc WORKORCH_OBSERVE_CACHE_MODEL=gpt-5.4 WORKORCH_OBSERVE_MIN_CACHE_SAMPLE_COUNT=10 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' WORKORCH_OBSERVE_DURATION_SECONDS=2 WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR=/home/rangh/aispace/ai-proxy/interactions WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=1 WORKORCH_OBSERVE_CACHE_PROVIDER=api_test_abc WORKORCH_OBSERVE_CACHE_MODEL=gpt-5.4 WORKORCH_OBSERVE_MIN_CACHE_SAMPLE_COUNT=10 WORKORCH_OBSERVE_CACHE_BASELINE_FILE=<previous-runtime-observation-summary.json> WORKORCH_OBSERVE_MIN_CACHE_AVERAGE_DELTA=0 WORKORCH_OBSERVE_MIN_CACHE_LATEST_DELTA=0 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=loadtest-governed WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' WORKORCH_LOAD_TOTAL=2 WORKORCH_LOAD_CONCURRENCY=2 WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=5 WORKORCH_LOAD_KEEP_ARTIFACTS=1 WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=loadtest-governed WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui ./internal/pkg/workorchclient' WORKORCH_LOAD_TOTAL=2 WORKORCH_LOAD_CONCURRENCY=2 WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=5 WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP=1 WORKORCH_LOAD_KEEP_ARTIFACTS=1 WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=loadtest-governed WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/kernel/orchestrator/... ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient' WORKORCH_LOAD_TOTAL=2 WORKORCH_LOAD_CONCURRENCY=2 WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=5 WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP=1 WORKORCH_LOAD_KEEP_ARTIFACTS=1 WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=loadtest-governed WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient' WORKORCH_LOAD_TOTAL=2 WORKORCH_LOAD_CONCURRENCY=2 WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=5 WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP=1 WORKORCH_LOAD_KEEP_ARTIFACTS=1 WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 ./scripts/verify-workorch-managed-full.sh
WORKORCH_MANAGED_VERIFY_TARGET=loadtest-governed WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=5 WORKORCH_LOAD_TOTAL=2 WORKORCH_LOAD_CONCURRENCY=2 WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=5 WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP=1 WORKORCH_LOAD_KEEP_ARTIFACTS=1 WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 WORKORCH_MANAGED_GO_TEST_PACKAGES='./internal/modules/kernel/orchestrator/biz ./internal/modules/blocks/llm/biz ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient' ./scripts/verify-workorch-managed-full.sh
```

自动化收口状态：

- `verify-e2e-full` 已通过自动管理临时 `./bin/workorch` 的全量 e2e 回归，覆盖 HTTP、workspace template/soul、LLM workspace execution、incremental artifact、MCP stdio、SkillHub、auto-governance、software-dev code agent、software-dev Goal/DoD、SRD orchestration、file/diff/patch、web search、projection workspace switch、Web Console projection visual、weather/mock、TUI agent、TUI workspace switch visual、TUI terminal render、TUI long output。
- `verify-software-dev-goal-dod-e2e` 已对齐重复运行时失败提前 replan 与修复耗尽 replan 两种合法治理路径，确认 continuation run、accepted result 和后续场景均通过。
- `loadtest-governed` 已在本轮 prompt/requiredOutput/verifier/managed LLM config 注入入口修正后通过自动管理临时 `./bin/workorch` 的并发回归：`workspaceSuccessTotal=2`、`workspaceFailedTotal=0`、`successRate=1.0`、`expectedActiveRunOverlap=2`、`maxObservedActiveStageOverlap=2`、`maxObservedRunRequestsInFlight=2`、`activeRunOverlapPass=true`，最新 artifact 目录 `/tmp/workorch-loadtest-load-20260610142004-zac1bhxi`。
- `verify-tui-long-output` 已在本轮通过自动管理临时 `./bin/workorch` 回归：真实 PTY renderer 打开长输出 workspace `ws-907241144fe63899`，发送 PgDown/PgUp，验证 `LONG-IMPLEMENTATION` marker，通过 screen artifact 断言滚动后 Workspaces panel、底部 prompt/help rows 仍可见，并通过 raw PTY 负向 gate 确认 `enabledMouseReportingSequences={}`；managed artifact 目录 `/tmp/tmp.b9vwiAauF2`，PTY summary 位于 `/tmp/tmp.b9vwiAauF2/verify-tui-long-output/tui-terminal-render/tui-terminal-render-summary.json`。
- `observe-runtime-closure` 已在本轮通过自动管理临时 `./bin/workorch` 回归，并将 synthetic workspace 观测从弱采样升级为强 gate：2 个 synthetic workspace 均出现在 workspace projection list 与 snapshot projection 中，均捕获 `/workspace/created` 和 scoped `/workorchd/workspace/projection/changed` SSE，summary 中 `errors=[]`；runtime observation artifact 目录 `/tmp/tmp.HuNzVXaJ35`，managed artifact 目录 `/tmp/tmp.q1f61B26N5`。
- `verify-web-console-projection-visual` 已在本轮通过自动管理临时 `./bin/workorch` 回归：源码/dist 均覆盖 projection/heartbeat 展示入口，同时新增 MVVP 负向 gate，确认 Web Detail 不恢复端侧 `projectedRunSnapshot` 拼装、run detail 通过 workspace-scoped server run snapshot / observability 读取，并用 context seq 与 active workspace ref 防止 workspace 切换后旧响应污染当前视图；managed artifact 目录 `/tmp/tmp.RGkQkOIGue`。
- `verify-real-runtime-closure` preflight-only 已在当前已部署 8080 服务上通过：`serviceReady=true`、`workorch_stale_dispatched_step_min_age_seconds=600`、`WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS=900`，满足 watchdog 阈值短于 load 等待窗口；artifact 目录 `/tmp/workorch-real-preflight-20260610231350`，preflight summary 位于 `/tmp/workorch-real-preflight-20260610231350/preflight/preflight-summary.json`。同一服务下用 `WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS=420` 已验证 fail-fast 生效：artifact 目录 `/tmp/workorch-real-preflight-failfast-20260610231446`，错误明确指出 600s watchdog 不短于 420s load 等待窗口。该证据只证明真实闭环前置配置检查可用，不替代真实 provider loadtest。
- `verify-workorch-managed-full` 已通过 `WORKORCH_REAL_RUNTIME_PREFLIGHT_ONLY=1` 管理临时 `./bin/workorch` 执行 `verify-real-runtime-closure` preflight-only：`serviceReady=true`、`staleDispatchedStepMinAgeSeconds=600`、`runTimeoutSeconds=900`，并确认 real-runtime artifact 归档在 managed artifact 子目录；managed artifact 目录 `/tmp/tmp.EkVQOtzfat`，preflight summary 位于 `/tmp/tmp.EkVQOtzfat/verify-real-runtime-closure/preflight/preflight-summary.json`。
- `observe-runtime-closure` 已补运行期 projection DTO display contract gate：对 workspace list projection、workspace snapshot projection、workspace 展示字段和 run heartbeat 展示字段执行强校验，并输出 `projection-display-contract.json`；本轮 managed 回归 `displayContract.pass=true`、`snapshotCount=2`、`issues=[]`，managed artifact 目录 `/tmp/tmp.zyypxTsJYc`，display contract 位于 `/tmp/tmp.zyypxTsJYc/observe-runtime-closure/projection-display-contract.json`。
- 长 Decision resolution ID 文件名过长问题已完成本地代码级回归：`go test ./internal/pkg/repository/file -run 'TestDecisionResolutionRepositoryStoresLongReadableIDsWithSafeFileName|TestGovernanceRepositoriesStoreItemsWithoutMapRewrite' -count=1`、`go test ./internal/modules/application/workorchd/service/server -run 'DecisionResolveEndpoint|DecisionBatchResolveEndpoint|GovernanceRunResume' -count=1 -timeout=180s`、`go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/decision -run 'Decision|Governance|Resolve|Resolution' -count=1 -timeout=180s`、`go test ./internal/pkg/repository/... ./internal/pkg/workorchrepo -run 'DecisionResolution|WorkspaceScoped|Contract|File|Memory|Governance' -count=1 -timeout=180s` 均通过。
- Approval resolve workspace scope 已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/governanceports -run 'Approval|ResolveApproval' -count=1 -timeout=120s`、`go test ./internal/modules/application/workorchd/service/server -run 'ApprovalBatchResolveEndpointScopesDuplicateApprovalIDsByWorkspace|ApprovalBatchResolveEndpointApprovesPendingApprovals' -count=1 -timeout=120s`、`go test ./internal/modules/kernel/approval ./internal/modules/kernel/approval/biz ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -run 'Approval|Governance|Resolve|Batch|EventHubActionPort' -count=1 -timeout=240s`、`go test ./internal/modules/kernel/approval ./internal/modules/kernel/approval/biz ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count=1 -timeout=300s` 均通过。
- Governance resolve 显式 scope 绑定已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/governanceports -run 'BatchResolve|ResolveApproval|ResolveDecision|Approval|Decision' -count=1 -timeout=120s`、`go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/approval ./internal/modules/kernel/decision -run 'Approval|Decision|Governance|Resolve|Batch' -count=1 -timeout=240s` 均通过。
- Governance detail 与 activation input 显式 scope 绑定已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice/activationports -run 'ExplicitScope|Reads.*Scope|DecisionView|ApprovalView|ReadWorkspaceExecutionContextInputs' -count=1 -timeout=120s` 已通过。
- Activation governance history、start conflict 恢复读取、runtime raw content 和 artifact fallback scope 绑定已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/activationports ./internal/modules/application/workorchd/biz/appservice/contentports ./internal/modules/application/workorchd/biz/appservice -run 'RunObservableAfterStartConflict|CreateAndStartWorkspaceExecutionRun|WorkspaceGovernanceHistory|ReadArtifactFallbackListsArtifactsWithURIWorkspaceScope|ReadRuntimeRawContentWithBackendScopesVirtualReadsByURIWorkspace' -count=1 -timeout=180s` 已通过。
- Governance query、run validation/recovery 和 memory registry/selection scope 绑定已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/activationports ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice/runports ./internal/modules/application/workorchd/biz/appservice -count=1 -timeout=300s` 已通过。
- Memory 单对象 HTTP/client/port workspace scope 与 run continuation scoped helper 已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/... ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/biz/backends ./internal/services/workorch/service/cli ./internal/services/workorch/service/tui -count=1 -timeout=300s` 已通过。
- Governance client/port/backend/CLI/TUI/agent workspace scope 已完成本地代码级回归：`go test ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/biz/backends ./internal/services/workorch/service/cli ./internal/services/workorch/service/tui ./internal/services/workorch/biz/agent -count=1 -timeout=240s` 已通过；`go test ./internal/pkg/workorchclient -run 'TestClient(DecodesWorkspaceProjectionMetadata|GovernanceRequestsCarryWorkspaceScope)' -count=1` 已通过；`bash -n scripts/verify-workorch-container.sh scripts/verify-weather-ticket-approval-query.sh scripts/verify-software-dev-goal-dod-e2e.sh` 已通过。新增 `TestClientGovernanceRequestsCarryWorkspaceScope` 固化 Decision/Approval detail、single resolve 和 batch resolve 请求必须携带 `workspaceId` query 且 path/query 分隔正确。
- Workspace snapshot projection revision 字段稳定输出已完成本地代码级与自动管理实例回归：`go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1`、`go test ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient -run 'Workspace|Projection|Snapshot|Client' -count=1 -timeout=180s`、`go test ./internal/modules/application/workorchd/biz/appservice/... ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/biz/backends ./internal/services/workorch/service/cli ./internal/services/workorch/service/tui ./internal/services/workorch/biz/agent -count=1 -timeout=300s` 均通过。部署实例复验前，`WORKORCH_OBSERVE_WORKSPACE_IDS=ws-1271429d840c2fc3,ws-1d4f3a05b4151b64,ws-2cc1d3005d0cf880,ws-a16bbf1bd7b06fca,ws-a8b37b24183901a7,ws-f562b23cf172a6d6 ./scripts/observe-workorch-runtime-closure.sh` 在 artifact `/tmp/tmp.MMmA1Nol09` 暴露上述字段缺失问题；修复后 `WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=6 WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT=2 ./scripts/verify-workorch-managed-full.sh` 已通过，managed artifact `/tmp/tmp.euPmUc8jUD`，`displayContract.pass=true`、`snapshotCount=2`、`errors=[]`。`observe-runtime-closure` 已进一步增强为未显式传入 `WORKORCH_OBSERVE_WORKSPACE_IDS` 且未创建 synthetic workspace 时，默认从 workspace list 自动抽取最多 20 个 workspace snapshot 进行 display contract 验证，避免历史/quiet workspace snapshot projection 缺字段被 list-only 观测漏过；可用 `WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=0` 关闭。自动抽样分支已通过临时 HTTP mock 回归，artifact `/tmp/workorch-observe-auto-o4cxla0a`；增强后 managed 复验再次通过，artifact `/tmp/tmp.ZS4k5veR0O`，`displayContract.pass=true`、`snapshotCount=2`、`errors=[]`。普通无 active run workspace 不再触发 `runHeartbeat.activityState missing` 噪声告警；该告警只在 heartbeat 对象存在或 workspace 存在 `activeRunId` 时生效，临时 HTTP mock 回归 artifact `/tmp/workorch-observe-heartbeat-s1m22mbm`。最新一次 managed observe 复验 artifact `/tmp/tmp.kDsD3N8khc`，`runtime-observation-summary.json` 显示 `projectionState=ready`、`workspaceTotal=2`、2 个 synthetic workspace 均捕获 `/workspace/created` 与 scoped `/workorchd/workspace/projection/changed`，`displayContract.pass=true`、`snapshotCount=2`、`warnings=[]`、`errors=[]`。增强后的只读观测用于 8080 部署服务时，先在 artifact `/tmp/tmp.gpW5Dv21YV` 发现旧进程仍运行 `/home/rangh/aispace/workorch/bin/workorch (deleted)`，9 个 snapshot 中 5 个历史/quiet workspace 缺少 `sourceEventRevision/projectedSourceRevision`；停止旧 PID 后用当前 `./bin/workorch --server` 通过 `setsid` 重启，进程核对显示 `/proc/624369/exe=/home/rangh/aispace/workorch/bin/workorch` 且 cmdline 为 `./bin/workorch --server`。重启后同一只读观测通过，artifact `/tmp/tmp.ZBlGoHdEZa`，`projectionState=ready`、`workspaceTotal=9`、自动抽样 9 个 workspace snapshot、`displayContract.pass=true`、`snapshotCount=9`、`issues=[]`、`errors=[]`。
- 最终只读运行期观测再次覆盖当前 8080 部署实例：`WORKORCH_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_DURATION_SECONDS=4 WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT=0 ./scripts/observe-workorch-runtime-closure.sh` 通过，artifact `/tmp/tmp.wMgVLrn1ey`，`projectionState=ready`、`workspaceTotal=14`、自动抽样 14 个 workspace snapshot、`displayContract.pass=true`、`snapshotCount=14`、`warnings=[]`、`errors=[]`。
- 真实 provider 闭环已在当前 8080 部署实例上完成复验：`WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS=1 WORKORCH_REAL_RUNTIME_AI_PROXY_INTERACTIONS_DIR=/home/rangh/aispace/ai-proxy/interactions WORKORCH_REAL_RUNTIME_REQUIRE_CACHE_SAMPLES=1 WORKORCH_REAL_RUNTIME_MIN_CACHE_SAMPLE_COUNT=1 ./scripts/verify-real-runtime-closure.sh` 通过，artifact `/tmp/tmp.95xCC5ldj1`。load 阶段 `batchId=load-20260610190617`、`workspaceSuccessTotal=2`、`workspaceFailedTotal=0`、`successRate=1.0`、`activeRunOverlapPass=true`、`maxObservedActiveStageOverlap=2`、`maxObservedRunRequestsInFlight=2`；observe 阶段 `projectionState=ready`、`observedWorkspaceIds=[ws-92b5eb7fe8c6f136,ws-fe0d056bfb9d98e5]`、`displayContract.pass=true`、`snapshotCount=2`、`errors=[]`；cache 观测从 ai-proxy interactions 采集到 4 个样本，`average=41.922793587834356`、`latest=36.010403949254204`，latest sample provider/model 为 `deepseek/deepseek-v4-flash`。此前单独 preflight artifact `/tmp/tmp.3XJG51YWNp` 也确认 `serviceReady=true`、`runTimeoutSeconds=900`、`staleDispatchedStepMinAgeSeconds=600`、`pass=true`。
- Web/TUI 视觉体验已补当前 8080 部署实例自动化证据：`WORKORCH_BASE_URL=http://127.0.0.1:8080 WORKORCH_WEB_CONSOLE_VISUAL_KEEP_ARTIFACTS=1 ./scripts/verify-web-console-projection-visual.sh` 通过，最新 artifact `/tmp/tmp.WTIG56pERC`，summary 显示 `serviceReady=true`、`projectionState=ready`、`workspaceTotal=13`、`chunkCount=13`，部署 bundle 含 projection/heartbeat 展示 marker；`WORKORCH_TUI_WORKSPACE_SWITCH_KEEP_ARTIFACTS=1 ./scripts/verify-tui-workspace-switch-visual.sh` 通过，artifact `/tmp/tmp.EAl2sHbsTA`，覆盖切换前/切换后/refresh 后/inactive event 后文本快照；`WORKORCH_BASE_URL=http://127.0.0.1:8080 WORKORCH_TUI_TERMINAL_KEEP_ARTIFACTS=1 python3 scripts/verify-tui-terminal-render-smoke.py` 通过，最新 artifact `/tmp/workorch-tui-terminal-zgdhi45u`，真实 PTY summary 中 `sawWorkspace=true`、`sentStatusCommand=true`、`sentExitCommand=true`、`enabledMouseReportingSequences={}`；鼠标选择代理复验 artifact `/tmp/workorch-tui-terminal-gadk02cl` 进一步显示 `mouseSelectionProxyPass=true`。
- Goal/DoD quality check 显式 scope 绑定已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/qualityports -count=1 -timeout=120s` 已通过。
- Goal/DoD definition update/delete scope 绑定已完成本地代码级回归：`go test ./internal/modules/application/workorchd/biz/appservice/definitionports -count=1 -timeout=120s`、`go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/workspace -run 'Goal|DoD|Definition|Workspace|Quality' -count=1 -timeout=240s` 均通过。

真实环境/人工验收收口状态：

- TUI workspace 切换后的主视图与 status bar 刷新已具备模型级、文本录制式视图快照、服务端 projection/SSE contract、真实 PTY renderer smoke、长结果 PTY marker 验证、滚动后固定区 PTY screen assertion 和只读运行期观测脚本验证入口。鼠标拖选复制体验的应用侧风险已由自动化代理覆盖：`TestTUIE2EViewportKeyboardScrollAndMouseSelectionContract` 固化 viewport 不启用 mouse wheel reporting；真实 PTY smoke artifact `/tmp/workorch-tui-terminal-gadk02cl` 显示 `mouseSelectionProxyPass=true` 且 `enabledMouseReportingSequences={}`，证明 TUI 不接管终端原生鼠标选择事件。

### 业务对象状态聚合规则

后续继续收口时，凡是同一业务对象的状态、错误、revision、stale、pending、in-flight、rebuiltAt 等运行期元数据被拆散在多张 map 或多处结构中维护，应优先改为按业务对象聚合 entry 管理。

适用示例：

- workspace projection：按 workspace entry 管理。
- workspace rebuild scheduler：按 workspace rebuild entry 管理。
- TUI workspace runtime state：按 workspace entry 管理 display cache、event revision、context request seq。
- HTTP run snapshot cache：按 workspace/run cache key entry 管理 cached snapshot 和 in-flight snapshot warmup。
- run / step / ticket / approval 等 owner 状态：如果存在类似拆分，也应按对应业务对象 entry 管理。
- workspace-local repository 对象：持久化和内存实现都必须支持 workspace scoped key 或等价 scope resolution；裸 ID 查找只允许唯一命中，否则返回 conflict。
- 通用工具目录：MCP Server、MCP Capability 等系统级工具资源不按 workspace 复制；workspace 只记录 overlay/selection，读取时按 workspace overlay 优先、系统默认兜底。
- 通用工具默认选择和 managed config：template/system 可维护公共默认；workspace 只保存显式覆盖字段。activation 应按 `workspace overlay > template/system default` 合成有效 hints，并把合成后的选择冻结到 run execution context；公共 managed config 写入系统公共目录，workspace 内部配置只作为显式覆盖存在，MCP runtime 解析 `{workspaceManagedConfig:<path>}` 时按 `workspace override > system default` 选择实际文件。新增通用工具配置时必须先判断是否属于系统公共能力；只有明确存在 workspace 差异时才创建 workspace overlay，禁止为所有 workspace 批量生成同构副本。

不适用示例：

- 函数内部临时 `seen` 集合。
- allowed key 常量集合。
- repository/cache 中单一用途的对象索引。
- 明确只作为排序索引的 `workspaceOrder` 这类辅助结构。

## Current Confirmed Baseline

### LLM 上下文一致性

当前 LLM 交互主链路已经具备基础一致性：

1. `ActivateWorkspaceExecutionView` 先通过 workspace owner 读取正式 workspace，并绑定 runtime workspace scope。
   - 参考：`internal/modules/application/workorchd/biz/appservice/activationports/workspace_activation.go`
2. `BuildWorkspaceExecutionContext` 会校验 `goal.WorkspaceID == workspace.ID` 和 `dod.WorkspaceID == workspace.ID`。
   - 参考：`internal/modules/application/workorchd/biz/appservice/activationports/execution_context.go`
3. run 创建时将 execution context clone 到 `run.ExecutionContext`，形成 run 级冻结快照。
   - 参考：`internal/modules/application/workorchd/biz/appservice/activationports/run.go`
   - 参考：`internal/modules/application/workorchd/biz/appservice/activationcore/created_run_context.go`
4. `MergeExecutionContextPatch` 阻止请求 patch 覆盖 `workspaceId`、`goalId`、`doDId`、`goal`、`dod`、definition policy、continuation 等权威字段。
   - 参考：`internal/modules/application/workorchd/biz/appservice/activationcore/execution_context_patch.go`
5. orchestrator 在 LLM step 前从 `run.ExecutionContext`、step input、runtime evidence 构造 `executionContextPackage`。
   - 参考：`internal/modules/kernel/orchestrator/biz/execution_context_package.go`
6. LLM prompt 已将原始 Goal/DoD 放入 `raw_required` 的 C1 section，并通过 `definitionRevision` 标识 workspace definition。
   - 参考：`internal/modules/blocks/llm/biz/prompt_messages.go`

当前一致性语义是：run 创建时冻结权威定义，后续 LLM 调用围绕该 run 的 execution context 和 evidence 递进。它不是每次 LLM 调用都读取 workspace 最新定义。

### Projection 基础实现

当前 `workspaceviewcore.Manager` 已经具备服务端 projection 基础：

- 启动时通过 `ScheduleRebuildAll` 建立 baseline。
- 订阅 WorkOrch owner events。
- 有 per-workspace `rebuilding` / `rebuildPending` 去重。
- projection changed 事件通过 `TopicWorkspaceProjectionChanged` 发布。

参考：

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/manager.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store.go`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`

## P0 Must Close

### P0.1 Projection rebuild 必须从全局优先改为 workspace 优先

当前缺陷：

- `Manager.handleEvent` 在收到带 `WorkspaceID` 的事件时，如果 `rebuildAllInFlight` 为 true，会转为 `ScheduleRebuildAll()`。
- 多 workspace 并行执行时，一个 workspace 的高频事件可能触发全局 pending rebuild，拖慢其它 workspace 投影。

收口要求：

- workspace scoped event 永远记录到该 workspace 的 pending rebuild 集合。
- full rebuild in-flight 期间收到 workspace event，不再重新调度 full rebuild；应在 full rebuild 结束后补跑这些 workspace 的 detail rebuild。
- 只有 workspace list 结构变化、startup baseline、显式恢复校验这类全局事件才允许调度 full rebuild。

建议实现：

- 在 `workspaceviewcore.Manager` 增加 `pendingWorkspaceRebuilds map[string]struct{}` 或等价结构。
- `handleEvent` 对 `envelope.WorkspaceID != ""` 只做：
  - `store.MarkStale(workspaceID, envelope.Revision)`
  - 如果 full rebuild in-flight，记录 pending workspace。
  - 否则 `ScheduleRebuildWorkspace(workspaceID)`。
- `finishRebuildAll` 后调度 pending workspace rebuild，而不是再次 `ScheduleRebuildAll()`。

验收：

- full rebuild 期间连续收到 `ws-a`、`ws-b` 事件，full rebuild 完成后只补跑 `ws-a`、`ws-b` detail rebuild。
- `ws-a` 的高频 LLM stream 不触发全局 workspace projection 重扫。

### P0.2 Full rebuild 必须拆成 list baseline 和 workspace detail refresh

当前缺陷：

- `rebuildAll` 先 `ListWorkspaces`，再串行逐个 `ReadWorkspaceSnapshot`。
- 任意 workspace snapshot 慢或失败，会拖慢整体 baseline。

收口要求：

- full rebuild 的第一阶段只刷新 workspace overview/list。
- workspace detail snapshot 按 workspace scope 调度独立 rebuild。
- 单 workspace detail 失败只标记该 workspace degraded/stale，不导致全局 projection 不可用。

建议实现：

- 引入 list baseline 方法，只更新 workspace overview 和 workspace order。
- 对每个 workspace 调用 `ScheduleRebuildWorkspace(workspaceID)`。
- 保留全局 projection status，但增加 per-workspace projection status。

验收：

- 某个 workspace detail 读取失败时，`GET /api/v1/workorch/workspaces` 仍能返回其它 workspace。
- 失败 workspace 的 snapshot 返回明确 degraded/stale 状态。

### P0.3 Store 需要增加 source revision 对齐语义

当前缺陷：

- `Store.ReplaceAll` 会清空所有 stale 标记。
- full rebuild 期间的新事件可能被全局 pending 掩盖，单 workspace 是否已投影到最新 source revision 不够明确。

收口要求：

- 每个 workspace 维护：
  - `lastSourceRevision`
  - `projectedSourceRevision`
  - `workspaceRevision`
  - `staleReason`
  - `updatedAt`
  - `rebuiltAt`
- rebuild 完成只清理对应 workspace stale。
- full rebuild 不应盲目清理全部 stale，除非确认每个 workspace 已投影到对应 source revision。

验收：

- projection status 能判断单 workspace 是 ready、stale、degraded 还是 failed。
- Web/TUI 可以根据 projection metadata 判断是否需要刷新当前 workspace。

### P0.4 Projection changed event 必须精确表达 scope

当前缺陷：

- workspace detail 更新和 full rebuild 更新都通过 `TopicWorkspaceProjectionChanged` 表达，但端侧需要更明确的 scope 语义。

收口要求：

- `workspaceId == ""` 只表示 workspace list/baseline 结构变化。
- `workspaceId != ""` 表示指定 workspace detail/overview 变化。
- payload 必须包含 projection state、global revision、workspace revision、source event revision、change kind。

建议 payload 字段：

- `workspaceId`
- `changeKind`
- `projectionState`
- `globalRevision`
- `workspaceRevision`
- `sourceEventRevision`
- `projectedSourceRevision`
- `updatedAt`
- `rebuiltAt`
- `error`

验收：

- TUI/Web 收到 `workspaceId != activeWorkspaceID` 的事件时，只更新列表缓存或非 active 缓存，不刷新当前主视图。
- 收到 `workspaceId == activeWorkspaceID` 时刷新当前 workspace context。

### P0.5 TUI context refresh 必须改成 per-workspace request tracking

当前缺陷：

- TUI 使用单一 `workspaceContextRequestSeq`。
- 多 workspace 快速切换、projection retry、事件触发刷新、手工 run command 同时发生时，请求容易互相失效。

参考：

- `internal/services/workorch/service/tui/model.go`
- `internal/services/workorch/service/tui/commands.go`

收口要求：

- `workspaceContextRequestSeq` 改为 `map[string]int64` 或等价 per-workspace tracker。
- `WorkspaceContextMsg` 使用 `WorkspaceID + RequestSeq` 判断是否过期。
- 非 active workspace 的返回可以进入 workspace context cache，但不更新主视图。
- 切换到某 workspace 时，优先显示该 workspace 缓存，再请求最新 projection。

验收：

- A/B 两个 workspace 同时运行，TUI 从 A 切到 B 后，B 的后续 projection event 能刷新主视图。
- A 的旧 context response 不会覆盖 B 的主视图。
- B 的旧 response 不会被 A 的新请求 seq 误判为过期。

### P0.6 TUI list refresh 需要支持事件密集场景

当前缺陷：

- `workspaceListRefreshInFlight` 是全局标记，密集 projection event 会被压成 pending list refresh。
- 这能降低请求量，但会导致状态栏和 workspace 列表在并行执行下滞后。

收口要求：

- list refresh 只处理 list/baseline 变化，不应该由所有 workspace detail 事件触发。
- workspace detail event 只刷新对应 workspace cache。
- 状态栏显示应基于服务端 projection list response 和 per-workspace event patch 协同更新。

验收：

- 高频 LLM stream 不导致 TUI 持续全量 FetchAll。
- workspace list 能在 workspace create/delete/state overview 变化时刷新。

### P0.7 TUI workspace 切换必须重新绑定视图状态

当前缺陷：

- 之前实际观测中，切换 workspace 后主视图和状态栏停止刷新。
- 当前实现仍然以单 active workspace 和若干全局 transient 字段为主，切换时容易保留旧 workspace 的 `progress`、`activeRunID`、`blockingDecision`、`blockingApproval`、runtime events。

收口要求：

- 切换 workspace 时必须：
  - 设置 `activeWorkspaceID`
  - 清理旧 workspace transient display state
  - 保存旧 workspace display cache
  - 加载目标 workspace cache
  - 请求目标 workspace context
  - 重置或重建 main viewport
- 旧 workspace 的后续 events 不应修改当前主视图。

验收：

- 运行 `ws-a` 后切换到 `ws-b`，`ws-b` 可再次启动或继续执行。
- 切回 `ws-a` 时能显示 `ws-a` 最新状态，不被 `ws-b` 状态污染。

### P0.8 EventHub lane 使用必须避免无状态任务全局串行

当前设计规则：

- 同一业务对象内需要顺序处理时使用稳定 lane。
- 无状态 block/module 不应绑定全局 lane。
- observer 内不能执行重活。

收口要求：

- workspace/run/step/ticket/approval/result/artifact 等状态 owner 的写路径使用 per-object lane。
- projection rebuild 使用 per-workspace 去重或 per-workspace lane。
- LLM、MCP、file、shell 等 runtime block 若无共享状态，不应因全局 lane 被串行化。
- 禁止在同一 lane 的 event handler 内同步等待同 lane 的 command，避免自锁。

验收：

- `maxConcurrentRuns=5` 时，不同 workspace 的 active run 可以并行推进。
- 一个 workspace 的 projection rebuild 不阻塞另一个 workspace 的 run state change 或 LLM stream event。

## P1 Should Close

### P1.1 Active run 定义漂移处理需要统一

当前已有 `ActiveRunDefinitionStale` 判断，但需要确保所有激活入口行为一致。

收口要求：

- Goal/DoD/Soul/Skill/MCP 等关键定义变更后，必须明确：
  - 继续旧 run
  - supersede 旧 run 并创建 replacement run
  - 拒绝启动并提示定义冲突
- 决策结果应记录到 run evidence 或 projection 状态，供 TUI/Web 展示。

验收：

- active run 定义 stale 时，重新激活能稳定进入 replacement 或明确拒绝。

### P1.2 LLM prompt 缓存稳定性继续优化

当前已有 `PromptContext`、section id、cacheability 和 C1 raw_required 保护，但缓存命中率仍有提升空间。

收口要求：

- C0/C1/C2 稳定前缀保持顺序、内容和 hash 稳定。
- volatile evidence 和 current action 放在尾部。
- 不稳定字段不得混入 workspace definition section。

验收：

- 连续 LLM 调用中，stable prefix hash 和 workspace definition hash 稳定。
- context budget 压缩不丢 Goal/DoD 原始契约。

当前进展：

- 已有 `PromptContext`、section id、cacheability、provider cache plan 和 Goal/DoD raw_required 保护。
- 已补测试验证 volatile progress/current action 不影响 C0/C1/C2 cacheable prefix hash 和 cache key。
- `observe-runtime-closure` 已能读取 ai-proxy log 和 interactions metadata 中的真实 provider usage/cache 样本，支持 provider/model/时间窗口过滤、最小样本数和 baseline average/latest delta 校验；本机 interactions 样本按 `api_test_abc/gpt-5.4` 过滤可采集到 177 个有效样本，平均 cache hit rate 约 22.65%。2026-06-11 当前 8080 真实闭环 artifact `/tmp/tmp.95xCC5ldj1` 已生成明确 workorch real-provider 窗口样本，cache 样本数 4，average hit rate 41.92%，latest hit rate 36.01%。

### P1.3 执行推进有效性需要投影化

当前观测过 workspace 长时间无有效代码产出、连续 Decision、无效重试等现象。

收口要求：

- projection 中增加 run progress heartbeat：
  - latest step
  - latest evidence
  - latest LLM usage
  - latest state transition
  - latest artifact/result change
  - last effective progress time
- 识别 stalled/ineffective retry：
  - 连续相同 Decision
  - 连续同 prompt retry
  - 无新 step/evidence
  - 长时间 active 但 no activity

验收：

- TUI/Web 能区分 running、waiting、stalled、ineffective retry、blocked。

当前进展：

- `WorkspaceSnapshot.runHeartbeat` 已输出 activity state、latest effective progress、ineffective retry、no activity/stalled 等确定性派生字段。
- TUI `/status` 已展示 heartbeat。
- Web Workspace Detail 已展示 heartbeat；基于投影生成时间的 10 分钟 stalled 阈值判定已补齐。
- 真实多 workspace active run 运行期观测已由 artifact `/tmp/tmp.95xCC5ldj1` 覆盖：2 个真实 provider workspace 并发完成，observe 阶段采集 heartbeat metrics，`stalledRuns=0`、历史 `ineffectiveRetryRuns=3` 作为既有问题信号保留在指标中。

### P1.4 HTTP/shared session 多 workspace 并行安全需继续审计

当前 server 使用 `withSharedSession` 借出 shared runtime session。

参考：

- `internal/modules/application/workorchd/service/server/http_types.go`

收口要求：

- 确认 `RuntimeSession.Borrow()` 后不存在跨 request/workspace 共享可变状态污染。
- HTTP handler/appservice 所有正式状态读写仍通过 owner EventHub command/response 或 projection reader。
- workspace scope 必须在 command context 和 payload 中同时体现。

验收：

- 并行 HTTP/TUI 请求操作不同 workspace 时不会互相污染 activeRun、blocking decision、snapshot。

当前进展：

- Decision/Approval resolve 生产链路已通过 `governanceRunResumeRef{WorkspaceID, RunID}` 调度 run resume。
- governance resume scheduler 已删除只接受裸 `runID` 的内部 helper；幂等合并和 in-flight 标记统一使用 `workspaceID/runID`。
- 已补 `TestServerStateGovernanceRunResumeAllowsSameRunIDAcrossWorkspaces`，覆盖两个 workspace 同时拥有 `run-001` 时都会独立恢复并绑定各自 workspace context。
- owner read port 的 run-local command scope 已补强回归：Run、Step、RuntimeCheckpoint、ToolCall、ArtifactContinuation、Result、Artifact、DecisionTicket、Approval 等读命令在 workspace scoped context 下必须同时携带 envelope `WorkspaceID/RunID` 与 payload `workspaceId/runId`。Result/Artifact by-run read command 已补显式 `PayloadKeyRunID`，避免只依赖 command struct 或 context 解析。
- Approval resolve 写命令已补齐 workspace/run scope contract：`ResolveApprovalCommand` 携带 `WorkspaceID/RunID`，appservice 在 single/batch resolve 中先读取并校验 approval item，再用 item scope 发布 ensure-approved/rejected；runtime action port 从 context 继承 workspace scope；kernel approval handler 在 ensure-approved/rejected 时先绑定 scoped repository context，再执行状态转换。已补 `TestResolveApprovalItemPublishesScopedContractCommand`、`TestBatchResolveApprovalViewsApprovesPendingItems` scope 断言、`TestEventHubActionPortPublishesOwnerCommands` scope 断言，以及 HTTP 端到端 `TestApprovalBatchResolveEndpointScopesDuplicateApprovalIDsByWorkspace`，覆盖两个 workspace 复用同一 `approvalID` 时 batch resolve 只处理 query workspace，另一个 workspace 保持 pending。
- Governance resolve 显式 scope 已绑定到 owner read command context：`ResolveApprovalView`、`BatchResolveApprovalViews` 和 `BatchResolveDecisionViews` 在读取 approval/ticket 前使用显式 workspace scope 生成 runtime context，owner read command 同时携带 envelope `WorkspaceID` 与 payload `workspaceId`；batch decision resolve 复用预校验 ticket 实体完成状态转换，避免同名 ticket/approval 在多 workspace 下被无 scope 读取误命中。已补 `TestResolveApprovalViewReadsApprovalWithExplicitScope`、`TestBatchResolveApprovalViewsApprovesPendingItems` 和 `TestBatchResolveDecisionViewsReadsTicketsWithExplicitScope`。
- Governance detail 与 activation input 读取已纳入同一 scope 规则：`DecisionView`、`ApprovalView`、`ReadWorkspaceExecutionContextInputs` 在存在显式 workspace 参数时先绑定 runtime workspace context，再发布 owner read command。已补 `TestDecisionViewReadsTicketWithExplicitScope`、`TestApprovalViewReadsApprovalWithExplicitScope`、`TestReadWorkspaceExecutionContextInputsReadsDefinitionsWithExplicitScope`。
- Activation governance history、start conflict 恢复和 content artifact/raw 读取已纳入同一 scope 规则：从 workspace 参数或 URI 解析出的 workspace ID 必须在 owner read/list command 发布前绑定到 runtime context，不能先无 scope 读取再通过返回对象过滤。已补 `TestWorkspaceGovernanceHistoryReadsRunAndResolutionWithWorkspaceScope`、`TestRunObservableAfterStartConflictReadsRunWithContextWorkspaceScope`、`TestReadRuntimeRawContentWithBackendScopesVirtualReadsByURIWorkspace`、`TestReadArtifactFallbackListsArtifactsWithURIWorkspaceScope`。
- Governance query、run validation/recovery 和 memory registry/selection 已纳入同一 scope 规则：`ApprovalsForQuery` / `DecisionTicketsForQuery` 在 workspace+run 查询时发布 scoped list-by-run command；`ValidateRunDefinitions` 并发读取 workspace/Goal/DoD 时带 workspace scope；run recovery helper 对 checkpoint/step/continuation checkpoint 读取使用 `run.WorkspaceID`；context registry memory read/list/search 改用 owner read port，memory create/update/delete 写命令携带 workspace/run envelope 和 payload scope；activation explicit memory selection 在 owner get 前绑定 workspace scope；`EnsureWorkspaceExists` 统一通过 workspace owner read port 发布 scoped command；Memory HTTP/client/port 单对象 Get/Update/Delete 均携带 workspace ID。已补 `TestApprovalsForQueryFiltersOwnerCommandResults`、`TestDecisionTicketsForQueryFiltersOwnerCommandResults`、`TestValidateRunDefinitionsReadsDefinitionsWithWorkspaceScope`、`TestRecoveryCandidateReadsRunScopedItemsWithWorkspaceScope`、`TestMemoryQueriesPublishScopedOwnerReadCommands`、`TestPublishMemoryItemPublishesScopedCommandEnvelope`、`TestSelectedWorkspaceMemoryItemsReadsExplicitIDsWithWorkspaceScope`、`TestEnsureWorkspaceExistsPublishesScopedOwnerReadCommand`、`TestWorkspaceScopedMemoryItemRoutesBindWorkspaceScope`。
- Goal/DoD quality check 已纳入 definition scope 规则：`CheckGoalDoDQuality` 在显式 GoalID/DoDID 分支使用 workspace scoped context 读取 owner definition。已补 `TestCheckGoalDoDQualityReadsExplicitDefinitionsWithWorkspaceScope`。
- Goal/DoD update/delete 写命令已纳入同一 scope 规则：`UpdateGoal` / `UpdateDoD` / `DeleteGoal` / `DeleteDoD` 使用显式参数优先、runtime context 兜底的 workspace scope，并在 command envelope/payload 中同时携带 `workspaceId`。已补 `TestUpdateGoalViewPublishesWorkspaceScopeFromContext`、`TestUpdateDoDViewPublishesWorkspaceScopeFromContext`、`TestDeleteGoalPublishesWorkspaceScopeFromContext`、`TestDeleteDoDPublishesWorkspaceScopeFromContext`。

### P1.5 workorchd / web / tui MVVP 边界固化

收口要求：

- `workorchd`: projection owner、REST/SSE/stream 输出、输入编排。
- Web: REST/SSE 展示和用户输入。
- TUI: stream 展示和用户输入。
- Kernel owner modules: 正式状态和状态机。
- Blocks: runtime 技术能力，不拥有正式状态。

验收：

- Web/TUI 不直接访问 owner repository/service。
- Web/TUI 不自行拼接跨 owner 状态。
- Web/TUI 只消费 projection DTO 和 stream event。

## P2 Verification

### P2.1 单元测试

需要补充或强化：

- `workspaceviewcore.Manager`
  - full rebuild in-flight 时 workspace event 被记录为 pending workspace rebuild。
  - full rebuild 结束后只补跑 pending workspace，不触发 full rebuild storm。
  - per-workspace rebuild 不互相阻塞。
- `workspaceviewcore.Store`
  - source revision / projected revision 对齐。
  - 单 workspace stale 清理不影响其它 workspace。
- TUI model
  - per-workspace context request seq。
  - workspace 切换后旧 response 不覆盖新 active workspace。
  - 非 active workspace event 不污染主视图。
- LLM context
  - execution context package 不来自 projection-only state。
  - prompt budget 不压缩 Goal/DoD raw contract。

### P2.2 E2E 回归

需要覆盖：

- 自动管理 `./bin/workorch` 实例。
- 同时运行两个 workspace。
- 一个 workspace 高频 LLM stream，另一个 workspace step completed/result accepted。
- TUI 从 A 切到 B 后主视图、状态栏持续刷新。
- `maxConcurrentRuns=5` 下多个 workspace 不被全局 lane 串行化。
- workspace projection full rebuild 期间不丢 workspace scoped event。
- 无 workspace 的 run 启动直接失败并提示 workspace 缺失。

当前进展：

- `verify-workspace-projection-switch` 已覆盖自动管理部署实例上的双 workspace projection list/detail/SSE contract、projection event revision/timing metadata、projection rebuild metrics、Web Console projection/heartbeat bundle contract，以及 unscoped run action 不成功。
- `loadtest-governed` 已具备 active overlap gate：`WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP=1` 会要求成功 workspace 的 active stage overlap 或服务端 `workorch_run_requests_in_flight` 采样达到 `min(concurrency, serverMaxConcurrentRuns, successTotal)`。
- `verify-real-runtime-closure` 已把 `loadtest-governed-real` 与 `observe-runtime-closure` 串联：load 阶段验证真实 provider 多 workspace 并发成功率和 active overlap；observe 阶段按本次 load workspace IDs 采集 projection/SSE/heartbeat，并可用 ai-proxy interactions provider/model/时间窗口和 baseline delta 校验 cache 改善。
- mock LLM active run 并行推进 e2e 已通过，证明 `maxConcurrentRuns=5` 下 2 个 workspace 的 run 不被全局 lane 串行化；真实 LLM/provider 运行期观测也已通过 artifact `/tmp/tmp.95xCC5ldj1` 覆盖，2 个真实 provider workspace 均成功且 `maxObservedActiveStageOverlap=2`、`maxObservedRunRequestsInFlight=2`。

### P2.3 观测指标

已补基础指标：

- projection rebuild count by workspace
- projection rebuild duration by workspace
- pending workspace rebuild count
- full rebuild count
- dropped/debounced projection event count
- active run heartbeat count and max age
- stalled run count
- ineffective retry run count
- TUI stream revision gap count

仍可继续增强：

- 真实终端 TUI renderer smoke 已由 `verify-tui-terminal-render` 覆盖；长 Result 的真实 PTY 渲染 marker、PgDown/PgUp 输入、滚动后右侧 workspace panel 与底部 prompt/help 可见性已由 `verify-tui-long-output-e2e` 覆盖；文本录制式视图快照由 `verify-tui-workspace-switch-visual` 承担，只读运行期观测由 `observe-runtime-closure` 承担。鼠标拖选复制的应用侧风险通过禁用 mouse reporting 和真实 PTY raw 负向断言覆盖。

### P2.4 Run Watchdog / Runtime Gap Recovery

workorchd 的 run watchdog 属于服务端状态循环和 runtime gap recovery，不属于 Web/TUI 端侧补偿逻辑。它由 `HTTPServer.Start()` 在 workspace projection 启动后调用 `state.scheduleWatchdog()` 接线，并通过 framework 注入的 `BackgroundRoutine` 管理后台 timer 生命周期。

watchdog 不是正常调度链路，也不是 event 丢失的兜底替代。TUI/Web/REST 提交的 command、orchestrator 派发的 step、runtime block 的 result/progress 都必须通过 EventHub 主路径可靠投递和处理；如果正常运行中发现 event 异常丢失，应核对 EventHub publish/send、订阅、owner command/result、runtime checkpoint 写入和投影消费链路并修复丢失原因，不能通过 watchdog 静默补偿来掩盖主路径缺陷。

触发时机：

- Startup scheduling: 服务启动后只注册 periodic watchdog timer，不自动恢复、重派或失败历史运行态 run。服务重启后需要恢复历史运行时，由用户通过 TUI/Web/REST 显式触发 `/run continue`、replan、restart 或后续专用诊断/恢复入口。
- Periodic watchdog: 服务运行期间每 `30s` 执行一次 timer tick。
- Stale dispatched 判定不是每 `30s` 直接修改状态；timer 只是检查窗口。真正恢复还需要 dispatched step 年龄超过 `server.staleDispatchedStepMinAge`，默认 `10m`。

当前覆盖范围：

- `recoverStuckRunningRuns`
  - 不由服务启动或 periodic watchdog 自动执行。
  - 仅作为显式诊断/手动恢复能力保留，用于 operator 确认历史 running run 已无活动且需要收口的场景。
  - 可恢复 terminal dispatched step 时交回 orchestrator resume。
  - 只有在没有 blocker、没有 continuation checkpoint、也没有可恢复 step 时，才把 stuck running run 收口为 failure。
- `recoverStaleDispatchedRuns`
  - 在 periodic watchdog 中执行。
  - 受 `MaxConcurrentRuns` run slot 和 `server.staleDispatchedStepMinAge` 控制。
  - 用于发现长时间停留在 dispatched 且已有持久 runtime checkpoint 证明存在 runtime gap 的 step，并通过 appservice / EventHub / orchestrator 恢复。
  - 覆盖范围仅限 `runtime_terminal_not_finalized` 且存在 `runtime_step_result_available`、`worker_lost` 且存在 `runtime_worker_lost`、以及有 worker heartbeat 且超过 stale 阈值的 accepted/in-progress gap；不处理无证据的 event 丢失。
- `recoverResolvedGovernanceBlockedRuns`
  - 不由 periodic watchdog 自动执行。
  - 按 workspace 列表逐一扫描 workspace-scoped runs。
  - 只处理 `blocked_on_decision` 且 blocking decision 已 `resolved`，以及 `blocked_on_approval` 且 blocking approval 已 `approved` / `rejected` / `resolved` 的不一致状态。
  - 仅作为显式诊断/手动恢复能力保留；正常治理恢复必须由 decision/approval resolve 主路径发布 scoped continuation command。
- `recoverReplanContinuations`
  - 不由服务启动自动执行。
  - 用于重启后发现旧 run 已 superseded 且存在 resolved replan decision，但 successor 未创建或未继续推进的场景。

影响范围与边界：

- Watchdog 只负责通用运行时形态恢复，不根据 workspace template、业务目标、文件名、测试框架或具体 capability 做业务分支。
- Watchdog 不负责补偿正常运行中的 event 丢失。event 丢失必须作为 EventHub / owner command / runtime checkpoint / projection 链路缺陷处理，并通过代码修复保证主路径可靠。
- 正式状态转换仍由 owner module / orchestrator 完成；watchdog 不绕过 EventHub command/response，不直接写 kernel owner repository。
- Governance resume 去重键为 `workspaceID/runID`：同一 workspace 的重复 resume 会合并，不同 workspace 即使 run ID 相同也必须独立恢复。
- Periodic stale dispatched recovery 会进入 run slot 队列，受 `MaxConcurrentRuns` 约束，避免 watchdog 抢占正常执行容量。
- Web/TUI 只消费 projection / SSE / command result；如果端侧已成功提交 decision/approval resolve，但 run 仍处于 waiting decision/approval，应核对 resolve handler、scheduler command 发布、EventHub 订阅和 run continuation 主路径，不应等待 watchdog 扫描补偿。

后续特性跟踪项：

- [ ] `/metrics` 增加 watchdog 维度指标：startup watchdog timer registration count、periodic tick count、stale dispatched recovery count、last watchdog error。
- [ ] `/readyz` 或 runtime diagnostics 增加最近一次 watchdog 执行时间、执行耗时和恢复摘要，便于区分 provider 卡住、projection stale 和 continuation 未调度。
- [ ] `observe-runtime-closure` 增加 resolved-governance-blocked 场景观测：模拟 ticket 已 resolved 后确认 resolve handler / scheduler command / EventHub 主路径提交 scoped resume；若 run 仍 blocked，应暴露为主链路缺陷。
- [ ] `verify-real-runtime-closure` 的 preflight 除 stale dispatched 阈值外，补充 watchdog interval、run slot 上限和 governance continuation 主路径指标采样。
- [ ] TUI `/status` 可展示服务端 projection 中的 latest watchdog summary，但不得在 TUI 端主动重放 resume。
- [ ] 根据真实 provider 长跑结果评估是否把 replan continuation recovery 纳入 periodic watchdog，前提是仍保持 workspace-scoped、幂等和 run-slot 受控。

当前进展：

- `/metrics` 已输出 active run heartbeat 总数、最大 heartbeat age、stalled run 数和 ineffective retry run 数，并纳入 `verify-workspace-projection-switch` deployed-instance gate。
- TUI `/status` 已输出当前 workspace stream revision gap count；模型测试覆盖 active/inactive workspace gap 计数和 active gap refresh 行为。
- `observe-runtime-closure` 会采集 `/readyz` projection 状态、workspace list、metrics 前后快照、SSE topic/workspace 事件计数、指定或自动抽样 workspace snapshot、projection DTO display contract，以及可选 ai-proxy cache hit rate；managed 验证可通过 `WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure` 自动启动临时服务执行。未显式传入 `WORKORCH_OBSERVE_WORKSPACE_IDS` 且未创建 synthetic workspace 时，默认 `WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=20`，从 workspace list 自动选择最多 20 个 workspace snapshot 做 display contract 验证；设置为 `0` 可关闭。启用 `WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT=2` 时，会在 SSE 监听窗口内创建 2 个临时 workspace，并强校验 `/workspace/created` 与 scoped projection changed 事件、workspace projection list metadata、snapshot projection revision/timing metadata、workspaceTotal、Web/TUI 展示所需 projection/workspace/runHeartbeat DTO 字段对齐。启用 `WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR` 与 `WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=1` 时，会从真实 ai-proxy interactions metadata 中读取 provider/model/token/cache hit rate 样本并写入 `runtime-observation-summary.json`；可进一步用 `WORKORCH_OBSERVE_CACHE_PROVIDER`、`WORKORCH_OBSERVE_CACHE_MODEL`、`WORKORCH_OBSERVE_CACHE_SINCE`、`WORKORCH_OBSERVE_CACHE_UNTIL` 限定 real-provider 场景窗口，并通过 `WORKORCH_OBSERVE_CACHE_BASELINE_FILE` 与 delta 阈值做改善对比 gate。
- `verify-real-runtime-closure` 会保留 loadtest、observation 和 merged summary artifacts；推荐真实环境执行时设置 `WORKORCH_REAL_RUNTIME_AI_PROXY_INTERACTIONS_DIR`、`WORKORCH_REAL_RUNTIME_CACHE_PROVIDER`、`WORKORCH_REAL_RUNTIME_CACHE_MODEL`、`WORKORCH_REAL_RUNTIME_CACHE_BASELINE_FILE` 与 delta 阈值，形成可审计的真实 provider cache 改善证据。
- `verify-workorch-managed-full` 会在 managed artifact 目录下保留 `verify-real-runtime-closure` 的内层 artifact，并让 `WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS` 默认继承 `WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS`；真实 provider managed 复验通过后，`real-runtime-closure-summary.json`、loadtest summary 和 runtime observation summary 会归档在同一个 managed artifact 树中。
- 真实 provider 闭环如果需要在较短窗口内验证 stale dispatched 恢复，`verify-real-runtime-closure` 会前置确认 `/metrics` 中 `workorch_stale_dispatched_step_min_age_seconds` 小于 `WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS`。自动管理临时服务时推荐显式设置 `WORKORCH_MANAGED_STALE_DISPATCHED_STEP_MIN_AGE=120s`；已部署服务需要在 `application.toml` 的 `[server]` 中设置 `staleDispatchedStepMinAge = "120s"` 并重启后再执行真实闭环。部署后可先执行 `WORKORCH_REAL_RUNTIME_PREFLIGHT_ONLY=1 make verify-real-runtime-closure` 验证 readiness 和 watchdog 窗口。
- `verify-tui-workspace-switch-visual` 会保留 TUI 文本视图快照 artifact，用于复核 workspace 切换、refresh 中间态、目标 workspace 主视图/status bar 和 inactive projection event 隔离效果。
- `verify-tui-terminal-render` 会保留真实 PTY raw/text/summary artifact，用于复核 Bubble Tea renderer 在远端服务、真实终端输入输出和 workspace projection 加载下的基础表现。
- `verify-web-console-projection-visual` 会先构建 Web Console，再核对源码与 dist asset 中 projection/heartbeat 展示入口，覆盖投影状态、运行活动、最近有效进展、stalled/ineffective retry 和 projection SSE refresh marker；同时核对 Web Detail 读取 server workspace projection、workspace-scoped run snapshot / observability，并禁止 `projectedRunSnapshot` / `buildProjectedRunSnapshot` 端侧拼装正式状态路径回归。提供 `WORKORCH_BASE_URL` 时还会保留部署实例 runtime artifact，包括 `/console` HTML、合并后的 console JS、chunk 列表、`/readyz`、workspace projection list 和 `web-console-runtime-visual-summary.json`。
- `verify-tui-long-output-e2e` 的输出已区分 automated contract coverage 与 terminal-native selection proxy contract，并会通过真实 PTY renderer 打开长结果 workspace、发送 PgDown/PgUp、验证 LONG-IMPLEMENTATION marker，断言滚动后固定 workspace panel 与底部 prompt/help rows 仍在屏幕中，并从 raw PTY 输出确认未启用 xterm mouse reporting 序列，避免 TUI 拦截终端原生拖选复制。
- `verify-workorch-managed-full` 会在 managed artifact 目录下保留 `verify-tui-long-output` 的内层 artifact，并让 `WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS` 默认继承 `WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS`，避免 managed 验证通过后丢失 PTY raw/text/screen/summary 证据。

## Suggested Closure Order

1. 收口 `workspaceviewcore.Manager` 的 full rebuild / per-workspace rebuild 调度。
2. 收口 `workspaceviewcore.Store` 的 per-workspace revision 和 stale 状态。
3. 调整 projection changed event payload 和 Web/TUI 刷新契约。
4. 调整 TUI per-workspace context request tracking 和切换状态隔离。
5. 审计 shared session 与 HTTP/appservice workspace scope。
6. 补 LLM context consistency 和 prompt cache 稳定性测试。
7. 补多 workspace 并行 e2e。
8. 根据实现结果刷新相关 skill 和验证 checklist。

## Completion Audit 2026-06-10

当前代码与自动化收口证据：

- `WORKORCH_MANAGED_VERIFY_TARGET=verify-e2e-full ... ./scripts/verify-workorch-managed-full.sh` 已用最新目标链通过，覆盖 projection、Web Console、TUI workspace switch、TUI terminal render、TUI long output、software-dev Goal/DoD、MCP stdio、SkillHub、auto-governance、SRD、Web Search、weather/mock 等当前自动化 gate。
- `verify-tui-long-output-e2e` 已通过真实 PTY renderer 打开长结果 workspace，发送 PgDown/PgUp，验证终端输出包含 `LONG-IMPLEMENTATION`，通过 screen artifact 断言固定 workspace panel 与底部 prompt/help rows 在滚动后仍可见，并通过 raw PTY 负向 gate 确认 TUI 未启用 xterm mouse reporting；因此“终端渲染能看到长结果 marker”“固定区滚动后仍可见”和“应用未主动接管鼠标事件”已不再是人工缺口。
- `observe-runtime-closure` 已能在 managed 临时服务上采集 projection/SSE/metrics，并能从 ai-proxy interactions metadata 按 provider/model 过滤 cache 样本、校验最小样本数和 baseline average/latest delta；启用 synthetic workspace 时会强制验证 `/workspace/created`、scoped projection changed、workspace list projection metadata 和 snapshot projection revision/timing metadata。
- `verify-real-runtime-closure` 已在当前 8080 部署实例完成真实 provider 闭环复验。历史 artifact `/tmp/tmp.hAJwQzCena` 暴露 stale dispatched 阈值问题；artifact `/tmp/tmp.TzcfUkcwSn` 暴露 concrete validation repair graph 缺少 `validationEvidence` contract 导致修复预算耗尽；artifact `/tmp/workorch-real-runtime-closure-20260610232854` 暴露长 Decision resolution ID 持久化文件名过长。当前已补 prompt/requiredOutput 字段示例、verifier fail-fast 分类和 file repository 安全文件名分段截断，并在重启最新服务后执行 `./scripts/verify-real-runtime-closure.sh` 通过，artifact `/tmp/tmp.95xCC5ldj1`，2 workspace real load 全部成功、active overlap 通过、projection display contract 通过、cache interactions 样本通过。
- 真实 provider 闭环可选用 `WORKORCH_MANAGED_VERIFY_TARGET=verify-real-runtime-closure WORKORCH_MANAGED_LLM_CONFIG_JSON=<private-json> ./scripts/verify-workorch-managed-full.sh` 在临时端口复验最新代码，避免依赖外部已部署实例是否已重启。

非代码证据边界说明：

- 真实终端鼠标拖选复制体验的应用侧边界已自动化覆盖：TUI 不启用 mouse reporting，不接管终端拖选事件；具体终端模拟器剪贴板行为属于宿主环境能力，不作为 workorch 代码收口缺口。

## Acceptance Checklist

- [x] workspace scoped event 不触发不必要的 full rebuild。
- [x] full rebuild 不会因为单 workspace 慢或失败阻塞所有 workspace 可见状态。
- [x] projection status 能表达 per-workspace ready/stale/degraded/failed。
- [x] projection changed event scope 明确。
- [x] TUI context request 使用 per-workspace seq。
- [x] TUI detail projection event 不触发不必要的 workspace list refresh。
- [x] TUI 切换 workspace 时保存/恢复 workspace display cache，且无缓存目标不会继承旧 workspace 主视图。
- [x] TUI 切换 workspace 后主视图和状态栏持续刷新。
- [x] Web/TUI 不从端侧自行推导正式状态。
- [x] LLM request context 只来自 run execution context、step input 和 runtime evidence。
- [x] LLM prompt cacheable prefix 对 volatile progress/current action 保持稳定。
- [x] Workspace snapshot 包含 run heartbeat 派生投影，TUI 可展示 no activity / ineffective retry 等状态。
- [x] 不同 workspace active run 可按 `maxConcurrentRuns` 并行推进。
- [x] 全量测试和 e2e 回归通过。
