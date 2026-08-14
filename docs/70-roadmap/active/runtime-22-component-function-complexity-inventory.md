# WorkOrch 22 个组件功能与复杂度盘点

Status: active architecture inventory

Type: component-function-complexity-inventory

Last Updated: 2026-08-14

本文是当前 22 个运行单元的结构盘点，不新增 Module、Block、Initiator 或状态机，也不改变现有
Owner 合约。后续简化应以本文的证据和优先级为入口，遵守仓库的 `go-multi-module-dev` 运行单元约束以及
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md) 的边界。

## 1. 盘点范围与方法

### 1.1 22 个单元的组成

当前基线是 **1 Initiator + 4 Modules + 17 Blocks = 22 个运行单元**：

- Initiator：`workorchruntime`。
- Modules：`application/workorchd`、`kernel/governance`、`kernel/orchestrator`、`kernel/system`。
- Blocks：`approval`、`decision`、`eventstream`、`file`、`git`、`llm`、`mcp`、`mcpregistry`、`memory`、
  `result`、`run`、`shell`、`skill`、`soul`、`ssh`、`web`、`workspace`。

`internal/modules/base/biz` 是共享基座，不是运行单元；`internal/services/workorch`、
`application/workorch`、`internal/pkg` 和 Web 前端也不计入 22 个单元。

### 1.2 定量口径

快照基于提交 `7bbcdd83`，使用 Git 已跟踪的 `*.go` 文件和 `wc -l` 统计：

- 生产代码：122,123 行、1,241 个文件。
- 测试代码：108,090 行、327 个文件。
- 合计：230,213 行、1,568 个文件。
- 运行单元内部 Go package 目录：143 个。

行数只用于发现集中点，不能单独作为拆分依据。特别是 application 的 projection 纯映射代码可以很多，
而 Orchestrator 的少量文件可能承载更高的状态迁移风险。

### 1.3 复杂度评分

每个单元按 1–5 分进行工程评估，分数是结构风险指标，不是运行时性能分数：

| 维度 | 含义 |
|---|---|
| F | 功能面：资源/协议/用例种类及领域规则数量 |
| A | 权威状态：正式聚合、状态转换、CAS、恢复或证据权威数量 |
| X | 交互面：跨单元 typed contract、EventHub fan-in/fan-out 和 scope 数量 |
| O | 运行时风险：外部 I/O、并发、超时、幂等、恢复和安全边界 |
| V | 维护体量：生产 LOC、文件/包碎片化、测试矩阵和重复适配层 |

总分为 `F+A+X+O+V`：

- 5–9：低（L）
- 10–14：中（M）
- 15–19：高（H）
- 20–25：极高（VH）

“简化”默认指同一 Owner 内的 focused package、纯 helper、合同收敛和删除重复 facade；不表示把状态机
拆成多个 Owner，也不表示把多个资源合并成一个 Block。

## 2. 定量基线

| 单元 | 类型 | 生产文件 | 生产 LOC | 测试文件 | 测试 LOC | Go 包目录 |
|---|---:|---:|---:|---:|---:|---:|
| `initiators/workorchruntime` | Initiator | 2 | 329 | 1 | 123 | 2 |
| `modules/application/workorchd` | Module | 888 | 55,149 | 132 | 38,919 | 39 |
| `modules/kernel/governance` | Module | 11 | 2,506 | 5 | 2,135 | 5 |
| `modules/kernel/orchestrator` | Module | 77 | 25,412 | 78 | 43,710 | 6 |
| `modules/kernel/system` | Module | 4 | 212 | 1 | 120 | 3 |
| `modules/blocks/approval` | Block | 13 | 985 | 3 | 1,165 | 6 |
| `modules/blocks/decision` | Block | 17 | 2,102 | 4 | 1,904 | 6 |
| `modules/blocks/eventstream` | Block | 11 | 1,855 | 5 | 1,002 | 6 |
| `modules/blocks/file` | Block | 18 | 4,048 | 7 | 2,375 | 4 |
| `modules/blocks/git` | Block | 16 | 2,450 | 8 | 1,345 | 4 |
| `modules/blocks/llm` | Block | 24 | 7,046 | 15 | 3,556 | 7 |
| `modules/blocks/mcp` | Block | 16 | 2,698 | 8 | 2,136 | 4 |
| `modules/blocks/mcpregistry` | Block | 11 | 1,118 | 3 | 511 | 5 |
| `modules/blocks/memory` | Block | 11 | 1,200 | 4 | 588 | 5 |
| `modules/blocks/result` | Block | 13 | 1,163 | 4 | 524 | 5 |
| `modules/blocks/run` | Block | 21 | 3,765 | 15 | 2,508 | 5 |
| `modules/blocks/shell` | Block | 18 | 3,503 | 11 | 2,611 | 4 |
| `modules/blocks/skill` | Block | 9 | 447 | 3 | 222 | 5 |
| `modules/blocks/soul` | Block | 10 | 656 | 3 | 343 | 5 |
| `modules/blocks/ssh` | Block | 14 | 1,030 | 5 | 414 | 4 |
| `modules/blocks/web` | Block | 16 | 1,928 | 5 | 739 | 6 |
| `modules/blocks/workspace` | Block | 20 | 2,408 | 6 | 1,013 | 7 |

### 2.1 体量集中点

1. `application/workorchd` 占运行单元生产代码约 45%，且有 888 个生产文件；其中
   `biz/appservice/readmodelprojection` 单包有 431 个 Go 文件、约 17.3k 生产 LOC。它是“过度碎片化 +
   应用职责过宽”并存的主要位置。
2. `kernel/orchestrator` 只有 6 个包目录，但集中 25.4k 生产 LOC和 43.7k 测试 LOC；核心大文件包括
   `semantic_planning.go`（约 95.8 KB）、`controller.go`（82.1 KB）、`runtime_mainline.go`（74.0 KB）、
   `dispatch_commit.go`（67.3 KB）、`prerequisite_freshness.go`（64.4 KB）和 `reducer.go`（56.2 KB）。
   这是“状态迁移集中”而不是简单的目录碎片问题。
3. `blocks/llm` 的 `runtime_candidate_provider.go` 约 96.6 KB；`blocks/shell` 的
   `shell_runtime_adapter.go` 约 72.1 KB；`blocks/file` 的 `file_runtime_adapter.go` 和 `owner.go`
   各约 45 KB。三个能力 Block 的复杂度主要来自 provider/安全/恢复合同叠加。
4. `kernel/governance/biz/service.go` 约 58.5 KB；`eventstream/pkg/recordpayload/payload.go` 约 35.3 KB。
   两者功能边界仍清晰，但单文件可读性和变更冲突风险较高。

## 3. 逐组件功能与复杂度评估

评分列为 `F/A/X/O/V = 总分`。

| 单元 | Owner 功能闭环 | 评分/级别 | 主要复杂度来源 | 简化方向（保持同一 Owner） |
|---|---|---:|---|---|
| `workorchruntime` | 创建并按配置暴露 memory/file repository provider，负责基础设施生命周期清理 | `1/1/2/2/2 = 8 L` | repository group 多、配置 fail-fast 与 backend 分支 | 保持一个 Initiator；将 backend builder 和 repository group mapping 仅作为 focused helper，禁止引入综合 runtime container |
| `application/workorchd` | HTTP/Web、CLI facade、scheduler、activation、context registry、read model、projection、SSE 生命周期接线 | `5/3/5/4/5 = 22 VH` | 入口适配与用例编排过宽；888 文件；projection 431 文件；多 owner 读写合同 | 保持一个 Application Module；先把 server lifecycle、activation use-case、read-model query、projection mapping 分成 4 个稳定内聚层，再将 431 个 projection 文件按事件族合并为少量纯 mapper 包 |
| `kernel/governance` | 自动治理评估、Decision/Approval resolve、升级/阻塞、AutoGovernanceTrace | `3/3/5/3/3 = 17 H` | 跨 6 个 owner 的策略组合；trace 与治理决策易混 | 保持一个 Module/Owner；将 policy evaluation、trace persistence、EventHub adapter 拆为 focused package，禁止 Governance 写 Run/Orchestrator 状态 |
| `kernel/orchestrator` | Runtime SM-2 authority、SM-3 candidate/dispatch、SM-4 drive/recovery/delivery；不拥有 Run lifecycle | `5/5/5/5/5 = 25 VH` | 四状态机共享一个 aggregate/CAS；semantic、prerequisite、dispatch、receipt、evidence、governance、delivery 全链路 | 不拆成多个 Module/Owner；在同一 `biz/runtime` 内按 SM-2/SM-3/SM-4 和纯合同适配分包，统一 private event dispatcher、transition coordinator、operation matrix，削减重复 adapter |
| `kernel/system` | 启动阶段幂等初始化 Soul、WorkspaceTemplate、MCP defaults | `2/1/4/2/1 = 10 M` | 跨 owner 初始化顺序和 fail-fast | 保持单一启动 Module；把 defaults 清单作为 typed data，避免再增加 service/facade；若生命周期不再独立再评估并回 application bootstrap |
| `blocks/approval` | Approval 请求、查询、更新、批准/拒绝、supersede 与通知 | `2/3/3/2/2 = 12 M` | 生命周期状态与授权 fence；测试/合同重复 | 保持 Block；合并过薄 contracts/helper，集中 lifecycle reducer 和 evidence mapping，不接收外部 target state |
| `blocks/decision` | DecisionTicket/Resolution 生命周期、治理提案与 resolution | `3/4/4/2/3 = 16 H` | ticket 与 resolution 双对象；Orchestrator governance proposal reconciliation | 保持 Block；将 ticket lifecycle 与 runtime proposal reconciliation 分成两个 focused package，仍由 Decision owner 单一 CAS 推进 |
| `blocks/eventstream` | 多 owner 通知归一化、按 workspace 排序的审计 EventRecord、列表/high-watermark/purge | `3/3/5/3/3 = 17 H` | 高 fan-in；recordpayload 类型映射多；审计与 projection 边界 | 保持 Block；把 payload normalization 按事件族聚合，统一 record append/revision 计算；禁止增加业务修复入口 |
| `blocks/file` | workspace-local list/read/write/edit/patch/preview/revert、target/admissibility/feasibility/receipt | `4/3/4/5/4 = 20 VH` | 路径/symlink/regular-file 安全；create-only 与 revision-bound replacement；运行时 receipt/replay | 保持单一 File owner；拆 `pathguard`、`patchengine`、`runtime-admission`、`receipt` 纯包，所有状态和副作用仍在 File reducer |
| `blocks/git` | 受控 Git read/write、changed refs、capability-state prerequisite receipt | `3/3/4/5/3 = 18 H` | subprocess/仓库边界；durable capability-state receipt；写操作授权 | 保持 Block；将 repository inspection、capability-state store、operation receipt 分层，禁止把 Git policy 下沉到 Shell |
| `blocks/llm` | ai-proxy catalog/cache、model/ceiling、Responses/embedding、semantic/candidate request、provider factual outcome | `5/3/5/5/4 = 22 VH` | 外部 provider、cache/concurrency、预算 ceiling、strict document boundary、诊断分层 | 保持一个 LLM Block；拆 catalog/cache、provider transport、request renderer、candidate boundary；统一 provider snapshot，删除重复 request assembly |
| `blocks/mcp` | 已注册 MCP tool/resource/prompt 调用及结果规范化 | `4/3/4/5/3 = 19 H` | 外部协议 transport、result normalization、runtime receipt、超时/重试边界 | 保持与 Registry 分离的执行 Block；拆 transport、argument validation、result normalization，禁止 MCP owner 解释业务 coverage |
| `blocks/mcpregistry` | MCPServer/MCPCapability registry CRUD、结构化 metadata、defaults | `3/3/3/2/2 = 13 M` | server/capability 关联、provenance、effect/idempotency metadata | 保持 Block；收敛 server/capability mapping 与 defaults helper，避免与 `blocks/mcp` 合并 |
| `blocks/memory` | MemoryItem CRUD/search、MemoryRecallTrace 记录与查询 | `3/3/3/2/2 = 13 M` | item 与 recall trace 两类资源；搜索结果边界 | 保持 Block；将 recall trace 作为内部 focused package，统一 clone/query mapping，不引入 runtime authority |
| `blocks/result` | Result/Artifact formal outcome、证据引用、artifact metadata、terminal delivery ack | `3/3/3/2/2 = 13 M` | Result 与 Artifact 生命周期、不可变 terminal delivery 顺序 | 保持 Block；分离 result lifecycle 与 artifact metadata helper，Result 只存已判断 outcome，不做 evidence quality 判定 |
| `blocks/run` | Run/Step/Checkpoint 正式状态、SM-1 lifecycle、opaque RuntimeStateRecord CAS、recovery inventory | `4/5/5/4/4 = 22 VH` | SM-1 与 opaque Runtime persistence 同时存在；scope/CAS/restart/replacement lineage | 保持单一 Run owner；内部拆 lifecycle reducer、runtime envelope/CAS、command mapping；Run 不解析 Runtime aggregate |
| `blocks/shell` | 受控本地命令、输出摘要、validation recipe、workspace effect snapshot、receipt | `4/3/4/5/4 = 20 VH` | 命令安全/资源限制、真实 changed refs、recipe/check marker、evidence quality | 保持 Block；拆 command policy/runner、effect snapshot、validation parser；禁止 Shell 选择 retry/terminal/CWI |
| `blocks/skill` | Skill registry CRUD、指令/metadata/binding/provenance | `2/2/2/1/1 = 8 L` | 主要是 registry schema 与 context budget | 保持 Block；清理空薄层和重复 clone helper，不与 Soul 合并 |
| `blocks/soul` | Soul catalog、stage/capability/risk policy、provenance、默认初始化 | `3/2/3/2/2 = 12 M` | 结构化 policy metadata、defaults 与 activation 绑定 | 保持 Block；集中 policy validation/defaults，禁止把 Soul guidance 变成 runtime authority |
| `blocks/ssh` | 结构化远程 target/session/credential refs 与受控命令 receipt | `3/2/4/5/2 = 16 H` | credential 引用、远端失败/超时、输出安全与 evidence binding | 保持 Block；拆 target/session validation 与 transport adapter，禁止复制 Shell 的本地策略 |
| `blocks/web` | 受控搜索/fetch、bounded summary/source/raw ref、Approval 查询 | `4/2/4/5/3 = 18 H` | 网络/provider 波动、内容截断、外部授权、source/evidence 引用 | 保持 Block；拆 provider transport、content bounder、approval query adapter；Web 只返回事实 |
| `blocks/workspace` | Workspace/Goal/DoD/Template definition 聚合、revision、active Run slot release | `4/4/4/2/3 = 17 H` | definition 版本与 activation binding；active slot/fresh-activation release；不应调度 Run | 保持 Block；内部拆 definition lifecycle 与 active-slot reducer，统一 definition revision 校验 |

## 4. 复杂度分布与关键结论

### 4.1 不是“22 个组件都应该再拆”

当前真正的问题有两类：

1. **过宽的应用 Module**：`workorchd` 同时承载入口适配、用例编排、读模型、projection 和 SSE；这会让任何
   一个 Web/CLI 变更都触及大量包，但这些包不应升级为新的状态 Owner。
2. **过密的核心状态 Owner**：`orchestrator`、`run` 和若干 capability Block 将恢复、幂等、安全和合同逻辑集中在
   大文件中；这里的正确做法是同一 Owner 内的纯包和规则族拆分，而不是迁移状态机。

`skill`、`workorchruntime`、`system` 等低体量单元没有明显的拆分收益。为了目录对称而再建 Block 会增加
   生命周期、EventHub topic 和测试成本。

### 4.2 交互热点

| 热点 | 当前角色 | 主要风险 | 盘点结论 |
|---|---|---|---|
| Orchestrator ↔ capability blocks | SM-2/SM-3 通过 typed prerequisite/operation receipt 调用 File/Git/Shell/LLM/MCP/Web/SSH | contract、revision、receipt、evidence 任一重复就可能形成第二权威 | 保留集中编排；统一 operation/admissibility matrix 和 owner observation adapter |
| Application ↔ 全部 owner | HTTP/CLI/scheduler/read model 入口 | application facade 越界读取 repository 或解释 Runtime 状态 | 只保留 typed EventHub/窄 read port；projection 与 action 彻底分离 |
| Governance ↔ Decision/Approval/Run/Workspace/Memory/Orchestrator | 自动治理与升级 | Governance 误写 Run 或重复决定 terminal | Governance 只持有 AutoGovernanceTrace；所有 continuation/terminal 回到对应 Owner |
| Eventstream ↔ 多通知发布者 | 审计与展示 | payload mapping 膨胀、投影反向修复状态 | 只做 append/order/high-watermark/purge，按事件族聚合 mapper |
| Workspace ↔ Run | active slot 与 Start/Resume/Restart intent | stale release 清空新 Run 或 Workspace 选择生命周期 | 保持两个 Owner；只通过 scoped intent/fact，补充 slot/restart 矩阵测试 |
| MCP Registry ↔ MCP execution | metadata registry 与实际调用 | registry 元数据被执行 Block 当作业务 authority | 保持两个 Block，Registry 只提供 capability fact |
| File ↔ Git/Shell | workspace effects 与 target/revision | 共享文件策略导致 owner 混淆 | 共享只能是 owner-neutral path/hash helper；禁止共享 service/repository |

## 5. 简化优先级

### P0：先降低状态和入口风险

1. **Orchestrator**：建立内部 `SM-2 / SM-3 / SM-4 / contracts / adapters` 包边界；所有 mutation 仍经同一
   `RuntimeInternalEvent` dispatcher 和 `RuntimeTransitionCoordinator.Commit`。优先处理 6 个超大文件和重复
   prerequisite/operation adapter。
2. **Application/workorchd**：将 `service/server` 限定为 HTTP/SSE 生命周期壳；将 appservice 归并为 activation、
   owner command/read ports、runtime session、projection 四组；`readmodelprojection` 的 431 个文件按事件族
   合并为少量纯 mapper 文件，不能创建新的 Module。
3. **Run**：把 SM-1 lifecycle reducer、opaque RuntimeStateRecord envelope、recovery inventory、command mapping
   分开；Run 只做 scope/schema/revision/CAS，不解析 Runtime 内容。
4. **LLM**：拆 provider transport、catalog/cache、ceiling、request renderer、candidate decoder；保持 provider
   snapshot 与 request budget 单一来源。
5. **File/Shell**：分别抽出 path/effect/patch 和 command/effect/recipe 纯算法包，先减少副作用边界里的分支。

### P1：再降低跨 owner 协调成本

1. Governance：拆策略评估与 trace persistence。
2. Eventstream：按事件族整理 payload mapper，保持审计只读。
3. Git/MCP/Web/SSH：统一各自 Block 内 transport、admission、receipt 三层，不共享可变执行器。
4. Workspace/Decision：分别隔离 definition/slot 和 ticket/resolution 两组 reducer。

### P2：只做清理，不做结构扩张

`approval`、`mcpregistry`、`memory`、`result`、`soul`、`skill`、`system`、`workorchruntime`：删除空包、重复
  DTO/clone helper 和薄 forwarding facade；没有独立生命周期或复用价值的包直接并回同一 `biz`。

## 6. 禁止的“简化”方式

- 不把 SM-2、SM-3、SM-4 分成三个 Module，也不把 Focus/CWI/evidence 迁移到 capability Block。
- 不建立跨 owner `RuntimeService`、综合 registry、全局 adapter map、callback bus 或 `map[string]any` 兼容层。
- 不把 `readmodelprojection` 变成新的状态 owner；它只能是纯投影/查询映射。
- 不为了降低单文件行数而暴露 reducer、repository、EventHub 或可变 registry。
- 不把 File、Git、Shell、SSH、Web、MCP 合并成“通用执行 Block”；安全边界、凭据边界和 receipt owner 必须独立。
- 不把 Skill 与 Soul、MCP Registry 与 MCP execution、Workspace 与 Run、Result 与 Eventstream 合并；这些组合会
  模糊正式状态和通知/投影边界。

## 7. 下一轮逐项验收清单

每完成一个 P0/P1 工作包，必须同时通过：

1. `go test`：受影响 Owner 包、跨 owner contract/restart/stale/duplicate/budget 测试。
2. 边界扫描：无跨 owner repository/service/biz 生产依赖，无状态 setter、StagePacket、NextAction、map/any。
3. 生命周期扫描：Hub/observer 只在 Biz，Module 只接线，Teardown 幂等，长期任务挂在 BackgroundRoutine。
4. 复杂度回归：包数量、公共类型、EventHub topic、Owner mutation entry 不得无理由增加；超大文件减少后不能
   产生第二个状态权威。
5. 文档同步：对应单元 `readme.md`、`docs/01-overview/current-state.md` 和 active roadmap 记录最终边界。

建议每个工作包只处理一个 Owner，并在提交说明中标注受影响的 SM ID；完成顺序为：

```text
Orchestrator / Run
  -> Application workorchd
  -> LLM / File / Shell
  -> Governance / Eventstream / Workspace / Decision
  -> Git / MCP / Web / SSH
  -> 其余低复杂度 Block 与 Initiator 清理
```

该顺序是复杂度降低顺序，不是新的运行时执行顺序。
