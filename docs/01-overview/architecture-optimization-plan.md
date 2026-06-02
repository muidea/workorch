# 项目架构与代码组织优化方案

> 编制日期：2026-06-01
> 范围：WorkOrch 项目整体架构评估与改进建议
> 策略：分层治理、渐进推进，不做大规模重构

---

## 一、架构现状评估

### 1.1 整体架构

WorkOrch 采用 4 层架构，依赖方向清晰：

```
application (workorch CLI + workorchd HTTP daemon)
     ↓
kernel (orchestrator, workspace, run, decision, approval, governance, memory, soul, skill, mcp, eventstream, result)
     ↓
blocks (llm, file, shell, git, ssh, web, mcp)
     ↓
internal/pkg (repository, events, common, config)
```

每个模块遵循统一的 `biz/pkg/service/module.go` 结构，通过 magicCommon 框架的 plugin 机制注册和组装。

### 1.2 代码规模分布

| 层 | 行数（源码+测试） | 占比 |
|---|------|------|
| kernel/orchestrator | 83,344 (源码 38,896 + 测试 43,945) | 39% |
| application/workorchd | 54,104 | 25% |
| application/workorch (CLI/TUI) | 19,136 | 9% |
| blocks (全部 7 个) | 24,857 | 12% |
| kernel (非 orchestrator，12 个模块) | 12,147 | 6% |
| internal/pkg | 17,768 | 8% |
| **总计（Go 源码，不含 vendor）** | **~213,000** | |

### 1.3 架构优势

1. **层次分明** — 无循环包导入，kernel 不依赖 application，blocks 不依赖 application
2. **模块结构一致** — 所有 13 个 kernel 模块 + 7 个 blocks 模块遵循相同 biz/pkg/service pattern
3. **Repository 抽象干净** — memory/file 两种实现，接口在 `internal/pkg/repository`
4. **验证基础设施完善** — 35 个验证脚本，502 个测试函数，多种 e2e 场景
5. **文档体系完整** — 100+ 设计文档，分层编号组织

### 1.4 识别的问题

#### 问题 1：orchestrator God Module

`RuntimeOrchestrator` 结构体有 **229 个方法**，分布在 20 个源文件中，源码 38,896 行。承担职责包括：

- Run 生命周期管理
- Step dispatch + follow-up
- Graph 执行
- Repair/replan 决策
- Convergence 评估
- Coverage work item 管理
- Patch 管理
- Artifact 组装
- Evidence 质量评估
- LLM context 组装
- Memory 管理
- Result 录制
- Checkpoint 管理
- Event 发布

虽然文件已按职责分组，但所有方法共享同一个 receiver 和同一组状态，意味着：
- 任何子系统都能直接访问所有内部状态
- 测试一个子系统需要构造完整 orchestrator
- 新增职责没有天然边界约束

**最大文件：**

| 文件 | 行数 |
|------|------|
| `runtime_repair.go` | 8,249 |
| `runtime_orchestrator.go` | 6,268 |
| `runtime_graph.go` | 2,808 |
| `execution_context_package.go` | 2,003 |
| `runtime_convergence.go` | 1,873 |
| `runtime_dod.go` | 1,739 |
| `runtime_patches.go` | 1,560 |
| `runtime_coverage_work_items.go` | 1,539 |

#### 问题 2：application 层大文件

| 文件 | 行数 | 问题 |
|------|------|------|
| `read_model_service.go` | 7,131 | 所有投影逻辑集中 |
| `http.go` | 5,929 | 所有 HTTP handler 集中 |
| `runtime_adapter.go` (LLM block) | 3,907 | 所有 capability 类型集中 |

#### 问题 3：kernel ↔ blocks 双向耦合

- orchestrator 导入 `blocks/{llm,git,web}/pkg/common`（step kind 常量）
- blocks 导入 `kernel/{orchestrator,mcp,approval}/pkg/models`（model struct）

虽然 Go 编译器允许（限于 `pkg/` 子包），但概念上形成循环依赖。

#### 问题 4：测试文件过大

`runtime_orchestrator_test.go` — 31,201 行，502 个测试函数，覆盖所有子系统。

---

## 二、优化方案

### 核心判断

当前架构的层次设计合理。问题不在于架构方向错误，而在于 orchestrator 模块内部的**职责密度过高**，以及 application 层的**文件粒度过粗**。这两个问题可以在不改变包结构的前提下逐步改善。

kernel ↔ blocks 双向耦合限于类型常量层面，实际风险可控，暂不投入。

---

### P1：orchestrator 内部组合化

#### 方案描述

将逻辑子系统提取为独立 struct，通过组合嵌入 orchestrator。不改变包结构，不改变对外接口。

#### 目标结构

```go
// Before: 229 methods all on RuntimeOrchestrator
func (o *RuntimeOrchestrator) stateChangingRepairRequired(...) bool { ... }
func (o *RuntimeOrchestrator) dispatchRuntimeRepairFollowUp(...) error { ... }
func (o *RuntimeOrchestrator) coverageSourceCanCarryStateChange(...) bool { ... }

// After: subsystem struct with focused responsibility
type repairSubsystem struct {
    repo       repository.RunRepository
    adapters   *RuntimeAdapterRegistry
    events     events.Hub
    // only the fields repair actually needs
}

func (r *repairSubsystem) stateChangingRepairRequired(...) bool { ... }
func (r *repairSubsystem) dispatchFollowUp(...) error { ... }

// RuntimeOrchestrator composes subsystems
type RuntimeOrchestrator struct {
    repair       *repairSubsystem
    graph        *graphSubsystem
    convergence  *convergenceSubsystem
    coverage     *coverageSubsystem
    patches      *patchSubsystem
    context      *contextSubsystem
    result       *resultSubsystem
    // core dispatch + lifecycle stays on orchestrator itself
}
```

#### 子系统拆分边界

| 子系统 | 当前文件 | 行数 | 职责 |
|--------|----------|------|------|
| repair | `runtime_repair.go`, `failure_classification.go` | 8,974 | failure classification, repair dispatch, follow-up, budget |
| graph | `runtime_graph.go` | 2,808 | graph execution, sibling evidence, follow-up |
| convergence | `runtime_convergence.go`, `runtime_dod.go` | 3,612 | convergence mapping, DoD gap analysis |
| coverage | `runtime_coverage_work_items.go`, `runtime_dod_evidence_refs.go` | 2,458 | coverage ledger, evidence role, evidence refs |
| patches | `runtime_patches.go`, `runtime_patch_approval.go` | ~2,100 | patch lifecycle, approval gate |
| context | `runtime_llm_context.go`, `execution_context_package.go` | 3,256 | LLM context assembly, budget, section ordering |
| result | `runtime_result_*.go` (6 files) | ~4,500 | result recording, artifacts, memory, manifest, summary |

核心 dispatch + lifecycle（step dispatch、state machine、event routing、adapter registry）保留在 `RuntimeOrchestrator` 本体上，预计约 6,000 行。

#### 子系统间交互

子系统之间通过 orchestrator 协调，不直接互相调用：

```go
// orchestrator 协调 repair 和 convergence
func (o *RuntimeOrchestrator) handleStepFailure(ctx context.Context, step *Step) error {
    class := o.repair.classifyFailure(step)
    action := o.convergence.decideRecoveryAction(class, step)
    return o.repair.dispatch(ctx, action, step)
}
```

如果子系统确实需要互相访问（如 repair 需要读 coverage 状态），通过构造时注入接口：

```go
type repairSubsystem struct {
    coverageReader interface {
        CurrentFocus(runID common.RunID) *CoverageWorkItem
    }
    // ...
}
```

#### 收益

- 每个子系统只能访问自己声明的依赖，天然约束职责蔓延
- 子系统可独立单元测试（不需要构造完整 orchestrator）
- 新增职责时有明确的归属判断
- 不改变包结构，不影响外部调用方（`orchestrator/service` 和 `orchestrator/pkg` 不变）

#### 风险控制

- 逐个子系统提取，每次一个，确保 `make test-race` 通过
- 先从最独立的开始（coverage、patches），再做耦合较深的（repair、graph）
- 提取过程中如果发现子系统间耦合过深，说明当前拆分边界不对，调整边界而非强行拆分

---

### P2：application 层大文件拆分

纯文件级重构，不改变包结构，不改变任何公开接口。

#### http.go（5,929 行）→ 按领域拆分

| 新文件 | 内容 |
|--------|------|
| `http_system.go` | health/readiness/metrics/pprof routes |
| `http_console.go` | console static asset + favicon routes |
| `http_configuration.go` | configuration read/update routes |
| `http_catalog.go` | soul, workspace template, and skill catalog routes |
| `http_read.go` | read-only API routes |
| `http_write.go` | write/action API routes |
| `http_events.go` | event list + SSE stream routes |
| `http_governance.go` | decision + approval read/write routes |
| `http_memory.go` | memory read/write routes |
| `http_mcp.go` | MCP server/capability/tool/resource/prompt routes |
| `http_workspace.go` | workspace CRUD + snapshot + activation + setup draft handlers |
| `http_run.go` | run CRUD + status + observability + detail page handlers |
| `http.go` | router setup, middleware, shared helpers, service struct |

#### read_model_service.go（7,131 行）→ 按投影面拆分

| 新文件 | 内容 |
|--------|------|
| `read_model_entrypoints.go` | public read model entrypoints and session/backend/env wrappers |
| `read_model_workspace.go` | workspace snapshot, workspace outcome, workspace task progress |
| `read_model_run.go` | run snapshot, run observability, run context bundle |
| `read_model_timeline.go` | 已存在（442 行），保持不变 |
| `read_model_event.go` | event record canonicalization, payload projection, event page projection helpers |
| `read_model_governance.go` | decision/approval pagination, ordering, governance view projection helpers |
| `read_model_memory.go` | memory recall trace, selection score, memory hit projection helpers |
| `read_model_coverage.go` | coverage projection, evidence mapping, ledger summary |
| `read_model_llm_usage.go` | LLM context budget traces, usage traces, usage summary |
| `read_model_outcome.go` | mapRunOutcome, mapWorkspaceOutcome, terminal state logic |
| `read_model_runtime_tools.go` | tool call, web source, git operation, runtime capability projection helpers |
| `read_model_result_artifact.go` | result summary, DoD assessment, artifact, artifact continuation, result/artifact page projection helpers |
| `read_model_progress_context.go` | task progress, task decomposition, execution section, context bundle, compressed context, repair loop projection helpers |
| `read_model_service.go` | 公共 helpers, 类型转换, service struct |

#### 收益

- 代码导航效率提升
- git blame 粒度更细
- merge conflict 减少
- 新开发者更容易定位相关代码

---

### P3：测试文件拆分

`runtime_orchestrator_test.go`（31,201 行，502 个测试函数）按被测子系统拆分。

| 新文件 | 覆盖范围 |
|--------|----------|
| `runtime_repair_test.go` | repair dispatch, follow-up, budget, failure classification |
| `runtime_graph_test.go` | graph execution, sibling evidence, follow-up |
| `runtime_dispatch_test.go` | step dispatch, state machine transitions |
| `runtime_result_test.go` | result recording, acceptance, rejection |
| `runtime_governance_test.go` | decision/approval blocking, resume |
| `runtime_coverage_test.go` | coverage work items, evidence role |
| `runtime_orchestrator_test.go` | 集成场景、端到端流程（保留） |

此拆分与 P1 子系统提取对齐——子系统提取后，测试自然跟随迁移。

---

### 暂不处理

#### kernel ↔ blocks 双向耦合

当前耦合限于类型常量层面（`pkg/common` 中的 step kind 字符串、model struct），不涉及行为调用。引入中间接口层的成本（大量 interface 定义 + adapter 胶水代码）高于收益。

**触发条件：** 当 blocks 数量显著增长，或出现实际编译/测试隔离问题时再考虑。

#### LLM block runtime_adapter.go

3,907 行，是 LLM 调用的统一入口，知道所有 capability 类型是其职责所在。可以按 capability 类型拆文件（纯文件级），但不需要架构层面改动。

---

## 三、实施顺序

| 阶段 | 内容 | 依赖 | 预估工作量 | 风险 |
|------|------|------|-----------|------|
| 1 | `http.go` 拆分 | 无 | 半天 | 极低 |
| 2 | `read_model_service.go` 拆分 | 无 | 半天 | 极低 |
| 3 | 提取 coverage subsystem | 无 | 1 天 | 低 |
| 4 | 提取 patches subsystem | 无 | 1 天 | 低 |
| 5 | 提取 repair subsystem | 3（repair 可能读 coverage） | 1-2 天 | 中 |
| 6 | 提取 graph subsystem | 无 | 1 天 | 低 |
| 7 | 提取 convergence subsystem | 3, 4（读 coverage 状态） | 1 天 | 中 |
| 8 | 提取 context subsystem | 无 | 1 天 | 低 |
| 9 | 提取 result subsystem | 无 | 1 天 | 低 |
| 10 | 测试文件跟随拆分 | 随各步骤同步 | 随步骤 | 低 |

**建议分两批：**
- **第一批（1-2）：** 纯文件移动，立即改善可读性，半天完成
- **第二批（3-9）：** 子系统提取，每次一个，逐步推进

每一步都是独立可交付的，`make test-race` 通过即可合入。

---

## 四、当前收口状态

### 4.1 已完成

| 项 | 状态 | 证据 |
|----|------|------|
| P2.1 HTTP console 路由拆分 | 已完成 | `internal/modules/application/workorchd/service/server/http_console.go` 承载 `/console/**` 与 `/favicon.ico` |
| P2.1 HTTP system 路由拆分 | 已完成 | `internal/modules/application/workorchd/service/server/http_system.go` 承载 `/healthz`、`/readyz`、`/metrics`、`/debug/pprof/**` |
| P2.1 HTTP configuration 路由拆分 | 已完成 | `internal/modules/application/workorchd/service/server/http_configuration.go` 承载 `/api/v1/workorch/configuration*` routes |
| P2.2 HTTP read/write 大路由拆分 | 已完成 | `http_read.go` 承载 read-only API routes；`http_write.go` 承载 write/action API routes；`http.go` 降为 router setup/state/shared helpers |
| P2.2 HTTP events 路由拆分 | 已完成 | `http_events.go` 承载 `/api/v1/workorch/events` 与 `/api/v1/workorch/events/stream` |
| P2.2 HTTP governance 路由拆分 | 已完成 | `http_governance.go` 承载 decisions/approvals 的 read 与 resolve/approve/reject routes |
| P2.2 HTTP memory 路由拆分 | 已完成 | `http_memory.go` 承载 memory list/read/update/delete 与 workspace memory create routes |
| P2.2 HTTP MCP 路由拆分 | 已完成 | `http_mcp.go` 承载 MCP server/capability read/write 与 tools/resources/prompts list routes |
| P2.2 HTTP catalog 路由拆分 | 已完成 | `http_catalog.go` 承载 souls、workspace templates、skills 的 read/write routes |
| P2.2 HTTP workspace 路由拆分 | 已完成 | `http_workspace.go` 承载 workspace CRUD、snapshot/progress/context/decomposition、memory recall、soul binding、Goal/DoD 与 quality-check routes |
| P2.2 HTTP run 路由拆分 | 已完成 | `http_run.go` 承载 run list/read/create/start/resume、workspace-scoped run detail、status、observability、steps、tool calls、results、patches、artifacts、context bundle 与 runtime payload routes；`http_read.go`/`http_write.go` 降为领域路由聚合入口 |
| P2.1/P2.2 行为验证 | 已完成 | `go test ./internal/modules/application/workorchd/service/server -count=1` 通过；`http_test.go` 的 native route registration 覆盖已同步 |
| P2.4 HTTP shared helper 拆分 | 已完成 | `http.go` 只保留 config/server constructor/startup/router setup；响应/request DTO 与 function aliases 移至 `http_types.go`，默认 appservice 代理移至 `http_defaults.go`，run cache/queue/status wait 移至 `http_run_cache.go`，query helper 移至 `http_query_helpers.go`，metrics/path/decode/write helper 移至 `http_response_helpers.go` |
| P2.4 HTTP shared helper 拆分验证 | 已完成 | `go test ./internal/modules/application/workorchd/service/server -count=1` 通过；只移动 service 层 helper 落点，不改变 route registration、handler、请求响应结构、cache key、queue 优先级或 error mapping |
| P2.3 read model 公开入口拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_entrypoints.go` 承载 `Read*WithSession/WithBackend/FromEnv` 入口 |
| P2.3 read model outcome 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_outcome.go` 承载 `mapRunOutcome`、`mapWorkspaceOutcome` 与 terminal result selection helpers |
| P2.3 read model LLM usage 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_llm_usage.go` 承载 LLM context budget、usage trace、usage summary 映射 |
| P2.3 read model coverage 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_coverage.go` 承载 coverage summary、DoD leaf coverage、evidence ref index 与 weak validation 判定 |
| P2.3 read model workspace 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_workspace.go` 承载 workspace snapshot、workspace projection run selection、workspace task progress/context/decomposition 入口 |
| P2.3 read model run 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_run.go` 承载 run snapshot、recovery snapshot、run status、run task progress 与 run step selection helpers |
| P2.3 read model runtime tools 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_runtime_tools.go` 承载 tool call、web source、git operation、runtime capability catalog/selection/usage projection helpers |
| P2.3 read model result/artifact 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_result_artifact.go` 承载 result summary、DoD assessment、artifact、artifact continuation、result/artifact page projection helpers |
| P2.3 read model event 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_event.go` 承载 event record canonicalization、payload projection 与 event page projection helpers |
| P2.3 read model governance 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_governance.go` 承载 decision/approval pagination、ordering 与 governance view projection helpers |
| P2.3 read model memory 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_memory.go` 承载 memory recall trace、selection score、memory hit 与 memory ID projection helpers |
| P2.3 read model progress/context 投影拆分 | 已完成 | `internal/modules/application/workorchd/biz/appservice/read_model_progress_context.go` 承载 task progress、task decomposition、execution section、context bundle、compressed context 与 repair loop projection helpers |
| P2.3 read model 验证 | 已完成 | `go test ./internal/modules/application/workorchd/biz/appservice -count=1` 通过 |
| P1.1 coverage subsystem 入口收口 | 已完成 | `RuntimeOrchestrator` 通过 `coverage *coverageSubsystem` 与 `coverageLedger()` 统一访问 coverage ledger；旧 free function 兼容入口已移除 |
| P1.1 coverage subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；coverage 相关测试已改为直接验证 subsystem 方法 |
| P1.2 patches subsystem 入口收口 | 已完成 | `RuntimeOrchestrator` 通过 `patches *patchSubsystem` 与 `patchManager()` 统一访问 patch apply/revert、patch approval gate 与 patch preview |
| P1.2 patches subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；patch 相关测试已改为直接验证 subsystem 方法 |
| P1.3 coverage source extraction 拆分 | 已完成 | `runtime_coverage_sources.go` 承载 Goal/DoD/profile/failed coverage source extraction、source key、markdown leaf 解析 helper |
| P1.3 coverage source extraction 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只移动 source extraction helper，不改变 work item 生成、证据应用、acceptance gate 或 LLM 上下文字段 |
| P1.4 patches request/filesystem/diff 拆分 | 已完成 | `runtime_patch_requests.go` 承载 patch request 解析与校验；`runtime_patch_filesystem.go` 承载 workspace path/hash/apply/preview；`runtime_patch_unified_diff.go` 承载 unified diff parser/apply helper |
| P1.4 patches request/filesystem/diff 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只移动 patch parsing、filesystem apply、unified diff helper，不改变 approval gate、patch state、文件写入、revert 或事件 payload |
| P1.5 patches approval risk 拆分 | 已完成 | `runtime_patch_approval_risk.go` 承载 patch/revert approval risk reasons、line stats、approval context request helper |
| P1.5 patches approval risk 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只移动 risk analysis 与 approval context helper，不改变 approval gate、risk reason、审批输出或恢复行为 |
| P1.6 coverage context/evidence 拆分 | 已完成 | `runtime_coverage_context.go` 承载 focus、ledger summary、target set、failure diagnostics 等 context projection；`runtime_coverage_evidence.go` 承载 required/missing evidence、step/assessment binding 与 work item evidence apply |
| P1.6 coverage context/evidence 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只移动 context projection 与 evidence binding helper，不改变 coverage ledger、work item 状态推进、focus 排序或 LLM 上下文字段 |
| P1.7 result subsystem 入口收口 | 已完成 | `RuntimeOrchestrator` 通过 `results *resultSubsystem` 与 `resultManager()` 统一访问 result artifact recording、runtime detail collection 与 evidence detail projection |
| P1.7 result subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只移动 result artifact/detail helper 的依赖边界，不改变 result state、artifact content、evidence refs、event payload 或 memory 写入内容 |
| P1.8 graph subsystem 入口收口 | 已完成 | `RuntimeOrchestrator` 通过 `graph *graphSubsystem` 与 `graphManager()` 统一访问 graph ready spec、follow-up input、graph evidence、run evidence summary 与 process evidence context；状态变更型 graph dispatch/repair/complete 仍保留在 orchestrator 协调层 |
| P1.8 graph subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 graph 读取与上下文组装边界，不改变 graph node 调度顺序、follow-up 条件、evidence refs、coverage/convergence contract 或 run 状态转换 |
| P1.9 convergence subsystem 入口收口 | 已完成 | `RuntimeOrchestrator` 通过 `convergence *convergenceSubsystem` 与 `convergenceManager()` 统一访问 convergence contract build/apply、decision ticket gate 与 compact projection；无 receiver 的纯 helper 保留 free function，避免强行引入 orchestrator 依赖 |
| P1.9 convergence subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 RuntimeOrchestrator 内部调用入口，不改变 convergence state 判定、mechanical continuation gate、decision ticket 创建条件或 LLM 上下文字段 |
| P1.10 repair budget subsystem 入口收口 | 已完成 | `RuntimeOrchestrator` 通过 `repair *repairSubsystem` 与 `repairManager()` 统一访问 provider retry、runtime repair attempt、failure scope 与 target scope 计数；repair dispatch、blocker、context package 构造继续保留在 orchestrator 协调层 |
| P1.10 repair budget subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 repair budget/attempt 读取边界，不改变 retry/repair 预算上限、scope/window 匹配、blocker 判断或 run 状态转换 |
| P1.11 repair context subsystem 入口收口 | 已完成 | `repairSubsystem` 统一访问 patch repair context/suggestions/already-satisfied signals、failure target context、resource shape alignment、root-cause candidates、recent repair attempt summaries 与 workspace file manifest；repair follow-up 主流程仍由 `RuntimeOrchestrator` 协调 |
| P1.11 repair context subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 repair context 读取与输入素材装配边界，不改变 repair objective、blocker ticket、provider retry、patch apply、graph dispatch 或 run 状态转换 |
| P1.12 LLM context subsystem 入口收口 | 已完成 | `RuntimeOrchestrator` 通过 `llmContext *llmContextSubsystem` 与 `llmContextManager()` 统一访问 step input/output、executionGraph 与 map compaction；已接入 step event、tool-call evidence、repair/blocker 输入投影路径 |
| P1.12 LLM context subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 LLM context compact/projection 调用入口，不改变字段裁剪规则、section 顺序、raw payload ref、event payload 或 tool-call evidence 内容 |
| P1.13 LLM context event/evidence 投影收口 | 已完成 | step event payload 与 runtime tool-call evidence 的 input/output summary 统一通过 `llmContextSubsystem` 生成，raw payload ref 仍由原事件/evidence helper 负责 |
| P1.13 LLM context event/evidence 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 event/evidence summary 的压缩入口，不改变事件 topic、tool-call state、payload ref 或 evidence repository 写入 |
| P1.14 repair dispatch action selection 收口 | 已完成 | `repairSubsystem` 承载 failure 后 `runtimeRecoveryAction` 选择入口，并通过 `repairRecoveryPolicy` 接口复用 orchestrator 现有 provider retry、context gate、duplicate failure、repair exhaustion 与 runtime repair gate 判断 |
| P1.14 repair dispatch action selection 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 recovery action selection 的调用边界，不改变 provider retry、runtime repair、blocker、FailRun 或 dispatchStep 的实际执行分支 |
| P1.15 repair dispatch executable spec 收口 | 已完成 | `repairSubsystem` 承载 provider retry 与 runtime repair 两类可继续执行分支的 `stepDispatchSpec` 规划；通过 `repairDispatchSpecBuilder` 回调复用 orchestrator 的 unique step ID 与 input 构造 |
| P1.15 repair dispatch executable spec 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整可执行 repair/retry 分支 spec 生成边界，不改变 block/fail 分支、dispatchStep 调用、attempt 计数规则或 step kind/input 内容 |
| P1.16 repair dispatch blocker delegation 收口 | 已完成 | `repairSubsystem` 承载 provider retry exhausted、runtime repair exhausted、duplicate failure、context integrity blocked 四类 blocked recovery action 的委派分支；通过 `repairBlockedRecoveryExecutor` 回调复用 orchestrator 原有 blocker ticket/run state 处理 |
| P1.16 repair dispatch blocker delegation 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 blocked recovery action 委派边界，不改变 ticket 创建、BlockOnDecision、rejected result recording 或错误映射 |
| P1.17 repair dispatch fail-run reason 收口 | 已完成 | `repairSubsystem` 承载 fail-run reason 归一化，固定 execErr、step output error、step output reason、默认 runtime failure 的优先级；FailRun 与 rejected result recording 继续由 `RuntimeOrchestrator` 协调 |
| P1.17 repair dispatch fail-run reason 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整失败摘要选择边界，不改变 fail-run 分支、result 记录、event payload 或状态转换 |
| P1.18 runtime execution coordination 边界确认 | 已完成 | `dispatchStep`、blocker ticket 创建、`BlockOnDecision`、`FailRun`、rejected result recording 等状态变更型执行协调继续保留在 `RuntimeOrchestrator`；子系统只负责分类、规划、投影或通过窄接口委派，避免把核心状态机拆散 |
| P1.19 repair 大文件职责拆分 | 已完成 | `runtime_repair.go` 按 failure dispatch、provider/budget、coverage gap、follow-up input、schema contract、action package、failure signals、policy、context material 拆分为多个 `runtime_repair_*.go` 文件；拆分后最大 repair 源文件为 `runtime_repair_context_material.go`，约 1,754 行 |
| P1.19 repair 大文件职责拆分验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；本轮只做函数块机械移动和 import 清理，不改变函数体、调用关系、状态转换、LLM 上下文字段或业务语义 |
| P1.20 runtime orchestrator 协调层文件拆分 | 已完成 | `runtime_orchestrator.go` 只保留核心类型、构造、subsystem accessor 与 policy normalization；run lifecycle、resume、completion、run state/workspace sync、dispatch execution、dispatch spec、runtime output contract 分别拆至 `runtime_lifecycle.go`、`runtime_resume.go`、`runtime_completion.go`、`runtime_run_state.go`、`runtime_dispatch_execution.go`、`runtime_dispatch_specs.go` 与 `runtime_*_contract.go` |
| P1.20 runtime orchestrator 协调层文件拆分验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；本轮只移动协调层函数落点，不改变状态机写入边界、dispatchStep、BlockOnDecision、FailRun、workspace sync、contract gate 或 recovery 行为 |
| P1.21 event/checkpoint subsystem 收口 | 已完成 | `eventSubsystem` 承载 run/workspace/step/result/artifact/patch/memory/LLM usage 事件发布；`checkpointSubsystem` 承载 runtime checkpoint 写入与 latest step 查询；`RuntimeOrchestrator` 通过 `eventPublisher()` 与 `checkpointManager()` 访问 |
| P1.21 event/checkpoint subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只移动事件发布与 checkpoint 写入边界，不改变 EventHub topic、payload、checkpoint ID、run/workspace/step 状态转换或 artifact/result/patch 行为 |
| P1.22 result recording subsystem 收口 | 已完成 | `resultSubsystem` 承载 accepted/rejected result 记录、completion evidence 并行记录、run summary/skill feedback/validation command memory 写入与 deliverable manifest 生成；`RuntimeOrchestrator` 仍只在 completion gate 通过后调用 result manager，不下沉 run/workspace 状态转换 |
| P1.22 result recording subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只调整 result/memory/manifest 记录接收者与调用入口，不改变 result ID、state、artifact refs、memory content、event topic 或 completion gate 行为；`resultManager()` 每次同步当前 evidence/event 依赖，避免子系统依赖快照滞后 |
| P1.23 run state read helper subsystem 收口 | 已完成 | `runStateSubsystem` 承载 replan run ID 生成、replan 继承上下文查找与 terminal run workspace release 判定；`RuntimeOrchestrator` 仍负责 run/workspace repository 写入、事件发布、replan successor 创建与 StartRun 调度 |
| P1.23 run state read helper subsystem 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只移动读取/计算 helper 的接收者，不改变 run ID 格式、replan context 字段、workspace release 条件、run/workspace 状态转换或 event payload |
| P1.24 run/workspace active state sync 收口 | 已完成 | `runStateSubsystem` 承载 active run/workspace 冲突判定、workspace active run 绑定、run/workspace 并行持久化同步；`RuntimeOrchestrator` 仍负责 lifecycle/resume/block/validate 的流程分支、事件发布与后续 dispatch |
| P1.24 run/workspace active state sync 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；只移动中立状态同步边界，不改变 active run 冲突规则、workspace state 映射、run state 写入内容、事件发布顺序或 dispatch 行为 |
| P1.25 validation run/workspace sync 收口 | 已完成 | `runStateSubsystem` 承载 validation 阶段 run/workspace 并行持久化同步，复用 active run 冲突判定；`RuntimeOrchestrator` 仍负责 validation completion 两阶段事件发布与最终 completed 状态收口 |
| P1.25 validation run/workspace sync 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；只移动 validation 同步边界，不改变 validating/completed 状态写入、workspace state、active run 绑定、事件发布顺序或 completion gate 行为 |
| P1.26 terminal workspace release/completion sync 收口 | 已完成 | `runStateSubsystem` 承载 terminal run 释放 workspace 与 completed run/workspace 并行持久化同步；`RuntimeOrchestrator` 仍负责 CompleteRun、validated completion 的事件发布顺序与状态机流程入口 |
| P1.26 terminal workspace release/completion sync 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；只移动终态 workspace release/completed sync 边界，不改变 terminal release 条件、completed state 写入、activeRun 清理或事件 payload |
| P1.27 decision resume selection 收口 | 已完成 | `decisionResumeSubsystem` 承载 resolved decision ticket/resolution 读取、failed step replan ticket 筛选与 resolved ticket 新旧排序；`RuntimeOrchestrator` 仍负责 resume 分支、FailRun、replanLoadedRun、ticket dismiss/update、run/workspace 状态同步与事件发布 |
| P1.27 decision resume selection 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；只移动 decision resume 的只读选择边界，不改变 resolution type 语义、replan 优先级、running recovery fallback、状态转换或 event payload |
| P1.28 ValidateRun validation sync 收口 | 已完成 | `ValidateRun` 复用 `runStateSubsystem.syncRunAndWorkspaceForValidation` 完成 run/workspace 并行持久化，移除旧 `syncWorkspaceForActiveRun` wrapper；`RuntimeOrchestrator` 仍负责 API 入口校验与事件发布顺序 |
| P1.28 ValidateRun validation sync 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；只统一 validation 状态同步路径，不改变 run validating 条件、workspace state、activeRun 绑定或 event payload |
| P1.29 StartRun active workspace guard 收口 | 已完成 | `runStateSubsystem.workspaceForCreatedRunStart` 承载 created run 启动前的 workspace activeRun 冲突读取与 stale activeRunID 判定；`RuntimeOrchestrator` 仍负责 StartRun 状态转换、事件发布、next dispatch 与失败后 reconcile |
| P1.29 StartRun active workspace guard 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；只移动启动前 guard 的读取边界，不改变 created-only 启动条件、stale activeRunID conflict 语义、active run 冲突规则或 dispatch 行为 |
| P1.30 workspace active run reconcile 收口 | 已完成 | `runStateSubsystem.reconcileWorkspaceActiveRun` 承载 workspace activeRun 读取、missing active run 清理、terminal active run 释放与 workspace 状态写入；`RuntimeOrchestrator` 仍负责 reconcile 调用点和 workspace state event 发布 |
| P1.30 workspace active run reconcile 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRunStateSubsystemReconcileWorkspaceActiveRunClearsMissingActiveRun|TestRuntimeOrchestrator_ReconcileWorkspaceActiveRun|TestRuntimeOrchestrator_StartRun|TestRuntimeOrchestrator_Resume' -count=1 -timeout=120s` 通过；只移动 activeRun reconcile 持久化边界，不改变 missing activeRunID、terminal active run、non-terminal active run 或 event payload 语义 |
| P1.31 rejected DoD result recording 收口 | 已完成 | `resultSubsystem.recordRejectedCompletionDoDAssessment` 承载 rejected completion DoD assessment 的合成、降级、runtime diagnostics 保留与 rejected result 写入；`RuntimeOrchestrator` 只在 repair/block gate 中调用 result manager |
| P1.31 rejected DoD result recording 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_RecordsRejected|TestRuntimeOrchestrator_Rejected|TestRuntimeOrchestrator_.*Rejected' -count=1 -timeout=120s` 通过；只移动 rejected result 录制边界，不改变 completion gate、recovery 分支、result state、DoD assessment 字段或 event payload |
| P1.32 unused dispatch single-spec helper 清理 | 已完成 | 删除无调用点的 `RuntimeOrchestrator.nextDispatchSpec`，统一使用 `nextDispatchSpecs` 多 spec 入口；不改变任何 dispatch 行为 |
| P1.32 unused dispatch single-spec helper 验证 | 已完成 | `rg -n "nextDispatchSpec\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 148 个 |
| P1.33 dispatch step ID helper 收口 | 已完成 | `uniqueDispatchStepID` 改为包级 helper，所有 dispatch/graph/artifact/repair 路径显式传入 `StepRepository`；`repairDispatchSpecBuilder` 不再暴露 step ID 去重能力 |
| P1.33 dispatch step ID helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*RepairDispatch|Test.*Dispatch|Test.*ArtifactContinuation|TestRuntimeOrchestrator_.*Graph' -count=1 -timeout=120s` 通过；只移动 step ID 去重 helper，不改变 ID 生成规则、冲突判定或 dispatch 行为 |
| P1.34 lifecycle block wrapper 清理 | 已完成 | 删除只做转发的 `blockOnDecision` / `blockOnApproval` wrapper，生产入口和测试统一调用 `blockOnDecisionAtStep` / `blockOnApprovalAtStep` |
| P1.34 lifecycle block wrapper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*BlockOn|Test.*StateTransition|TestRuntimeOrchestrator_.*Decision|TestRuntimeOrchestrator_.*Approval' -count=1 -timeout=120s` 通过；只删除无行为 wrapper，不改变 decision/approval block 状态转换、workspace state 或 event payload |
| P1.35 unused lifecycle/completion wrapper 清理 | 已完成 | 删除无调用点的 `supersedeLoadedRun` 与 `validateAndCompleteRun`，保留实际使用的 `closeLoadedRun` 与 `validateAndCompleteRunWithWorkspace` |
| P1.35 unused lifecycle/completion wrapper 验证 | 已完成 | `rg -n "supersedeLoadedRun\\(|validateAndCompleteRun\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 143 个 |
| P1.36 unused private receiver helper 清理 | 已完成 | 删除无调用点的 `dispatchGraphStepBatch`、`shouldBlockOnProviderRetryExhaustion` 与 `evidenceRefStepKind`，保留实际使用的 with-workspace/预算/step lookup 路径 |
| P1.36 unused private receiver helper 验证 | 已完成 | `rg -n "dispatchGraphStepBatch\\(|shouldBlockOnProviderRetryExhaustion\\(|evidenceRefStepKind\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 140 个 |
| P1.37 unused dispatch specs wrapper 清理 | 已完成 | 删除只做转发的 `dispatchStepSpecs`，`DispatchNextStep` 直接调用 `dispatchStepSpecsWithWorkspace(ctx, run, specs, nil)`；不改变 dispatch spec 选择、workspace 注入或 graph batch dispatch 行为 |
| P1.37 unused dispatch specs wrapper 验证 | 已完成 | `rg -n "dispatchStepSpecs\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 139 个 |
| P1.38 runtime policy accessor 收口 | 已完成 | 将 `llmOutputPolicyValues`、`maxRuntimeRepairAttempts`、`maxProviderRetryAttempts` 从 `RuntimeOrchestrator` receiver 改为包级 policy helper；调用点显式传入 `RuntimePolicy`，不改变默认值、配置覆盖或 retry/repair 预算语义 |
| P1.38 runtime policy accessor 验证 | 已完成 | `rg -n "func \\(o \\*RuntimeOrchestrator\\) (llmOutputPolicyValues|maxRuntimeRepairAttempts|maxProviderRetryAttempts)\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 136 个 |
| P1.39 unused dispatch step wrapper 清理 | 已完成 | 删除只做转发的 `dispatchStep`，runtime repair 与相关集成测试直接调用 `dispatchStepWithWorkspace(..., nil)`；不改变 step create/update、event publish、tool call dispatch 或 workspace 注入行为 |
| P1.39 unused dispatch step wrapper 验证 | 已完成 | `rg -n "dispatchStep\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 135 个 |
| P1.40 provider retry input builder 收口 | 已完成 | 从 `repairDispatchSpecBuilder` 移除 `providerRetryStepInput`，`repairSubsystem.recoveryDispatchSpec` 显式接收 max provider retry attempts 并调用包级 input helper；不改变 provider retry attempt、budget scope、step ID 或 input 字段 |
| P1.40 provider retry input builder 验证 | 已完成 | `rg -n "func \\(o \\*RuntimeOrchestrator\\) providerRetryStepInput\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 134 个 |
| P1.41 decision block rejected result recording 收口 | 已完成 | `recordRejectedResultForDecisionBlock` 从 `RuntimeOrchestrator` receiver 移至 `resultSubsystem`，orchestrator 仍负责 decision/block 分支与状态机协调；不改变 rejected result policy、assessment 记录或 block 行为 |
| P1.41 decision block rejected result recording 验证 | 已完成 | `rg -n "func \\(o \\*RuntimeOrchestrator\\) recordRejectedResultForDecisionBlock\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 133 个 |
| P1.42 current-run evidence ref wrapper 清理 | 已完成 | 删除单调用的 `hasCurrentRunUsableEvidenceRef`，definition change evidence 校验直接遍历 refs 并复用 `currentRunEvidenceRefUsable`；不改变 current-run evidence 判定、错误结构或 suggested actions |
| P1.42 current-run evidence ref wrapper 验证 | 已完成 | `rg -n "hasCurrentRunUsableEvidenceRef\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 132 个 |
| P1.43 failed loaded run wrapper 清理 | 已完成 | 删除只做转发的 `failLoadedRun`，rejected result、decision reject、approval reject 路径直接调用 `closeLoadedRun(..., RunStateFailed, reason)`；不改变 terminal close 条件、failure reason 或 event/workspace sync 行为 |
| P1.43 failed loaded run wrapper 验证 | 已完成 | `rg -n "failLoadedRun\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 131 个 |
| P1.44 governance history LLM input wrapper 清理 | 已完成 | 删除单调用的 `currentGovernanceHistoryForLLMInput`，repair input 装配处直接合并 current run governance history 与 executionContext history 后按原规则压缩；不改变 history 顺序、压缩上限或字段内容 |
| P1.44 governance history LLM input wrapper 验证 | 已完成 | `rg -n "currentGovernanceHistoryForLLMInput\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 130 个 |
| P1.45 result context projection 收口 | 已完成 | `latestAcceptedResultContext` 与 `currentDeliverableContexts` 从 `RuntimeOrchestrator` receiver 移至 `resultSubsystem`，execution context 装配通过 `resultManager()` 读取结果/交付物投影；不改变排序、数量上限、字段映射或 stable context 复用规则 |
| P1.45 result context projection 验证 | 已完成 | `rg -n "func \\(o \\*RuntimeOrchestrator\\) (latestAcceptedResultContext|currentDeliverableContexts)\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 128 个 |
| P1.46 recent failure context helper 收口 | 已完成 | `recentFailureContexts` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository` 包级 helper，execution context 装配直接传入 `o.steps`；不改变失败 step 过滤、倒序扫描、数量上限或字段映射 |
| P1.46 recent failure context helper 验证 | 已完成 | `rg -n "func \\(o \\*RuntimeOrchestrator\\) recentFailureContexts\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 127 个 |
| P1.47 step input memory run state helper 收口 | 已完成 | `stepInputMemoryRunStates` 从 `RuntimeOrchestrator` receiver 改为显式 `RunRepository` 包级 helper，memory context 刷新直接传入 `o.runs`；不改变 workspace run state 读取、selectionScores 或 memory selection 降噪规则 |
| P1.47 step input memory run state helper 验证 | 已完成 | `rg -n "func \\(o \\*RuntimeOrchestrator\\) stepInputMemoryRunStates\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 126 个 |
| P1.48 completion evidence ref catalog helper 收口 | 已完成 | `completionEvidenceRefCatalog` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`RuntimeEvidenceRepositories` 包级 helper；不改变 step/patch alias、scope、superseded failure ref 或 canonical ref 判定 |
| P1.48 completion evidence ref catalog helper 验证 | 已完成 | `rg -n "completionEvidenceRefCatalog\\(" internal/modules/kernel/orchestrator/biz -g'*.go'` 无结果；`RuntimeOrchestrator` receiver 方法数降至约 125 个 |
| P1.49 evidence ref repository helper 收口 | 已完成 | completion evidence ref 规范化、patch usable 判定、step/validation evidence ref 查询和 validation quality verdict 从 `RuntimeOrchestrator` receiver 改为显式 repository 包级 helper；不改变 DoD evidence refs 归一化、fallback 选择、current-run evidence 或 evidence quality 判定 |
| P1.49 evidence ref repository helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 115 个 |
| P1.50 output mode selector helper 收口 | 已完成 | `selectOutputMode` 与 `openArtifactContinuationContext` 从 `RuntimeOrchestrator` receiver 改为显式 policy/repository helper；`applyLLMOutputModeSelection` 仍作为 orchestrator 入口负责注入 policy 与 artifact continuation repository |
| P1.50 output mode selector helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*OutputMode\|Test.*ArtifactContinuation' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 113 个 |
| P1.51 repair coverage evidence candidate helper 收口 | 已完成 | missing Goal/DoD coverage evidence candidate、goal coverage text match candidate、latest action evidence candidate 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository` helper；不改变候选证据排序、fallback、coverage target 匹配或 LLM repair input 字段 |
| P1.51 repair coverage evidence candidate helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*CompletionGate\|Test.*ProviderRetry\|Test.*RepairInput' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 108 个 |
| P1.52 decision readable resource context helper 收口 | 已完成 | converted decision readable resource context 与 decision context readable resource enrichment 从 `RuntimeOrchestrator` receiver 改为显式 `WorkspaceRepository` helper；不改变 workspace manifest 扫描、readableResources 补全或自动 repair 判定 |
| P1.52 decision readable resource context helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*Decision\|Test.*RepairInput\|Test.*Dispatch\|Test.*KnownReadable' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 106 个 |
| P1.53 completion closure evidence helper 收口 | 已完成 | latest completion closure evidence 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`RuntimeEvidenceRepositories` helper，graph subsystem 直接持有 evidence 依赖；不改变 latest/superseded/patch/validation evidence 投影或 graph expansion limit follow-up 输入 |
| P1.53 completion closure evidence helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*Graph\|Test.*CompletionGate\|Test.*RepairInput\|Test.*Event' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 105 个 |
| P1.54 DoD evidence quality sub-check helper 收口 | 已完成 | artifact continuation evidence ref、definition change current-run evidence、profile evidence kind requirement 与 evidence ref step kind 子校验从 `RuntimeOrchestrator` receiver 改为显式 repository helper；保留 `validateDoDAssessmentEvidenceQuality` 作为 orchestrator 入口 |
| P1.54 DoD evidence quality sub-check helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*EvidenceQuality\|Test.*DoDAssessment\|Test.*ArtifactContinuation\|Test.*DefinitionChange' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 101 个 |
| P1.55 completion gate evidence ref normalization helper 收口 | 已完成 | completion gate DoD assessment evidence ref normalization 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`RuntimeEvidenceRepositories` helper；不改变 catalog 构建、DoD assessment 挂载位置或 evidenceRef 归一化规则 |
| P1.55 completion gate evidence ref normalization helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*CompletionGate\|Test.*EvidenceQuality\|Test.*DoDAssessment\|Test.*Graph' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 100 个 |
| P1.56 runtime repair current progress helper 收口 | 已完成 | runtime repair current execution progress 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`repairSubsystem` helper；不改变 currentExecutionProgress 字段、latest action evidence 或 recent repair attempt 摘要 |
| P1.56 runtime repair current progress helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*RepairInput\|Test.*RepairCoverage\|Test.*ProviderRetry\|Test.*CurrentExecutionProgress' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 99 个 |
| P1.57 coverage validation repair escalation helper 收口 | 已完成 | coverage validation repair escalation 判定从 `RuntimeOrchestrator` receiver 改为显式 `repairSubsystem` helper；不改变 validation repair 升级为 state-changing repair 的 attempt、target evidence 或 empty workspace surface 判定 |
| P1.57 coverage validation repair escalation helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*RepairInput\|Test.*RepairPolicy\|Test.*CoverageValidation\|Test.*RuntimeRepair' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 98 个 |
| P1.58 LLM output mode selection input helper 收口 | 已完成 | `applyLLMOutputModeSelection` 从 `RuntimeOrchestrator` receiver 改为显式 `LLMOutputRuntimePolicy`/`ArtifactContinuationRepository` helper；不改变 outputMode、outputModeSelector、artifactOutputContract 或 requiredOutput 注入规则 |
| P1.58 LLM output mode selection input helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*OutputMode\|Test.*ArtifactContinuation\|Test.*Convergence\|Test.*RepairInput' -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 97 个 |
| P1.59 initial repair target snapshot hydration helper 收口 | 已完成 | `hydrateInitialRepairTargetSnapshots` 从 `RuntimeOrchestrator` receiver 改为显式 `WorkspaceRepository`/`convergenceSubsystem` helper；不改变 initial step input、repairTargetSet 快照读取、patchRepairContext 合并或 convergenceContract 注入规则 |
| P1.59 initial repair target snapshot hydration helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 96 个 |
| P1.60 execution context workspace manifest helper 收口 | 已完成 | `executionContextWorkspaceFileManifest` 从 `RuntimeOrchestrator` receiver 改为显式 `WorkspaceRepository` helper；不改变 step input manifest 优先级、workspace deliverable manifest 读取、include 条件或 compact 规则 |
| P1.60 execution context workspace manifest helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 95 个 |
| P1.61 current-run governance history helper 收口 | 已完成 | `currentRunGovernanceHistory` 从 `RuntimeOrchestrator` receiver 改为显式 `DecisionTicketRepository`/`DecisionResolutionRepository`/`ApprovalRepository` helper；不改变 ticket、resolution、approval 历史投影字段、错误忽略策略或 repair/execution context 注入顺序 |
| P1.61 current-run governance history helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 94 个 |
| P1.62 step input memory context helper 收口 | 已完成 | `refreshedStepInputMemoryContext` 从 `RuntimeOrchestrator` receiver 改为显式 `MemoryItemRepository`/`RunRepository` helper；不改变 memory item 读取、run state 降噪、selectionScores、retrieval 元数据或 degraded fallback |
| P1.62 step input memory context helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 93 个 |
| P1.63 DoD assessment evidence quality helper 收口 | 已完成 | `validateDoDAssessmentEvidenceQuality` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`RuntimeEvidenceRepositories` helper；不改变 definition change current-run evidence、artifact continuation evidence ref、validation evidence quality 或 profile evidence kind requirement 校验规则 |
| P1.63 DoD assessment evidence quality helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 92 个 |
| P1.64 runtime repair exhaustion success evidence helper 收口 | 已完成 | `runtimeRepairExhaustionBlockerHasSuccessEvidence` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository` helper；不改变 repair-plan ticket context scope 解析、success evidence step 匹配或 blocked decision auto-release 判定 |
| P1.64 runtime repair exhaustion success evidence helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 91 个 |
| P1.65 duplicate runtime failure auto-resume helper 收口 | 已完成 | `duplicateRuntimeFailureBlockerCanAutoResume` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`repairSubsystem`/`RuntimePolicy` helper；不改变 duplicate failure ticket 识别、failed step 读取、attempt budget 或 auto-resume 判定 |
| P1.65 duplicate runtime failure auto-resume helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 90 个 |
| P1.66 runtime repair exhaustion auto-resume helper 收口 | 已完成 | `runtimeRepairExhaustionBlockerCanAutoResume` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`repairSubsystem`/`RuntimePolicy` helper；不改变 repair-exhausted ticket 识别、coverage evidence shortcut、target scope/window 判定或 attempt budget |
| P1.66 runtime repair exhaustion auto-resume helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 89 个 |
| P1.67 known readable decision auto-repair helper 收口 | 已完成 | `knownReadableDecisionBlockerCanAutoRepair` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`WorkspaceRepository` helper；不改变 completion gate、decision kind、completed step 或 readable workspace resource 判定 |
| P1.67 known readable decision auto-repair helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 88 个 |
| P1.68 provider retry resolved-decision specs helper 收口 | 已完成 | `providerRetrySpecsAfterResolvedDecision` 从 `RuntimeOrchestrator` receiver 改为显式 `StepRepository`/`repairSubsystem`/`RuntimePolicy` helper；不改变 failed provider step 判定、attempt 计数、唯一 step ID 或 decisionAuthorizedRetry 输入 |
| P1.68 provider retry resolved-decision specs helper 验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=120s` 通过；`RuntimeOrchestrator` receiver 方法数降至约 87 个 |
| P3.1 LLM context 测试拆分 | 已完成 | `runtime_llm_context_test.go` 承载 LLM context compaction、artifact chunk omission、lineage compaction 与 redacted patch fragment omission 测试；`runtime_orchestrator_test.go` 不再承载这组纯 context helper 测试 |
| P3.1 LLM context 测试拆分验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只迁移测试落点，不改变被测行为 |
| P3.2 graph 测试拆分 | 已完成 | `runtime_graph_test.go` 承载 run evidence summary 与 ready graph spec 复用已加载 steps 的测试；`runtime_orchestrator_test.go` 继续保留依赖完整状态流转的 graph e2e/dispatch 测试 |
| P3.2 graph 测试拆分验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只迁移测试落点，不改变测试断言或 runtime graph 行为 |
| P3.3 convergence 测试落点核对 | 已完成 | convergence 合同测试已集中在 `runtime_convergence_test.go`；本轮入口收口无需迁移测试文件 |
| P3.4 repair budget 测试拆分 | 已完成 | `runtime_repair_attempts_test.go` 承载 runtime repair attempt count 与 failure signature scope 计数测试；`runtime_orchestrator_test.go` 继续保留依赖完整 repair/blocker 状态流转的测试 |
| P3.5 repair context 测试拆分 | 已完成 | `runtime_repair_context_test.go` 承载 repair context target snapshot 与 workspace manifest 的 subsystem 级测试；完整 repair input 组装测试继续保留在 `runtime_orchestrator_test.go` |
| P3.6 LLM context subsystem 测试补齐 | 已完成 | `runtime_llm_context_test.go` 增加 `llmContextSubsystem` 入口测试，验证 subsystem 复用稳定 compaction 规则；既有 compact input/output、artifact chunk、lineage 与 patch redaction 测试保持不变 |
| P3.7 LLM context event/evidence 测试适配 | 已完成 | `runtime_orchestrator_test.go` 的 raw payload ref 测试显式通过 `llmContextSubsystem` 入口生成 event/tool-call summary，断言保持原有字段不变 |
| P3.8 repair dispatch spec 测试拆分 | 已完成 | `runtime_repair_dispatch_test.go` 承载 provider retry 与 runtime repair executable spec 的 subsystem 级测试；完整 dispatch/block/fail 状态流转测试继续保留在 `runtime_orchestrator_test.go` |
| P3.9 repair dispatch blocker 测试拆分 | 已完成 | `runtime_repair_dispatch_test.go` 补充 blocked recovery action delegation 的 subsystem 级测试；完整 blocker ticket 持久化与 run 状态测试继续保留在现有 orchestrator 测试中 |
| P3.10 repair dispatch fail-run reason 测试补齐 | 已完成 | `runtime_repair_dispatch_test.go` 补充 fail-run reason 优先级测试，覆盖 execErr、output.error、output.reason 与默认文案 |
| P3.11 runtime orchestrator 集成测试拆分 | 已完成 | `runtime_orchestrator_test.go` 保留公共 stub/helper，按 mainline、patch、repair input、context、repair flow、provider retry、memory、event、decision、graph follow-up、DoD validation、execution contract、strategy contract、evidence quality、rejected result、completion gate、result completion、resume gates、lifecycle、resume contract、misc contract 拆分为多个 `runtime_*_integration_test.go` 文件 |
| P3.11 runtime orchestrator 集成测试拆分验证 | 已完成 | `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过；只迁移测试函数落点，不改变测试断言、fixture、helper 或被测行为 |

本轮 HTTP 拆分只移动 route registration 和静态资源处理函数，不改变 API 路径、请求/响应结构、repository 调用、事件发布或 runtime 状态流转。
events/governance HTTP 细分只移动 route registration 和 handler 函数归属，不改变 SSE replay/filter 规则、decision/approval 查询分页、resolve/approve/reject 行为或 run resume 调度。
memory HTTP 细分只移动 memory route registration 和 handler 函数归属，不改变 workspace memory create、memory search/read/update/delete 的请求校验、响应结构或 metrics 计数。
MCP HTTP 细分只移动 MCP route registration 和 handler 函数归属，不改变 MCP server/capability CRUD、tool/resource/prompt listing、查询过滤或 metrics 计数。
catalog HTTP 细分只移动 souls、workspace templates、skills route registration 和 handler 函数归属，不改变 catalog CRUD、workspaceId query 过滤、响应结构或 metrics 计数。
workspace HTTP 细分只移动 workspace/Goal/DoD route registration 和 handler 函数归属，不改变 workspace CRUD、execution activation、Goal/DoD 定义更新、quality check、snapshot/context/progress 读取或 run continuation scheduling。
run HTTP 细分只移动 run route registration、workspace-scoped run helper 和 runtime observability handler 函数归属，不改变 run create/start/resume、status wait、snapshot cache、artifact content/runtime payload 读取、metrics 计数或 run continuation scheduling。
HTTP shared helper 拆分只移动 DTO、operation function aliases、default appservice proxies、cache/queue、query/path、metrics/response helper 的文件归属，不改变 server state 字段、startup wiring、route 行为或 API contract。
read model 入口拆分只移动公开入口和薄包装函数，不改变投影核心逻辑、分页策略、事件映射或 snapshot 结构。
outcome 投影拆分只移动 outcome state/result selection 逻辑，不改变 run/workspace terminal 判断、result 选择优先级或 evidence/artifact refs 映射。
LLM usage 投影拆分只移动 usage/context budget trace 映射与 summary 聚合逻辑，不改变事件过滤、排序、cache token 统计或 ratio 计算。
coverage 投影拆分只移动 coverage summary、DoD leaf coverage 和 evidence ref 判定 helper，不改变 DoD 文本规范化、missing coverage 判定或 weak validation 规则。
workspace 投影拆分只移动 workspace snapshot 聚合和 workspace projection run selection 逻辑，不改变 workspace state 映射、terminal result 优先级或 cached run snapshot 复用策略。
run 投影拆分只移动 run snapshot/status 聚合、governance ticket/approval selection 和 current step selection helper，不改变 repository 查询范围、并发加载策略、snapshot 字段映射或 runtime capability 映射。
runtime tools 投影拆分只移动 tool call、web source、git operation 和 runtime capability helper，不改变 tool 输出字段映射、capability ID 生成、catalog 查询触发条件或 evidence refs 组装。
result/artifact 投影拆分只移动 result summary、DoD assessment、artifact、artifact continuation 和 result/artifact page helper，不改变 result/artifact 字段映射、DoD coverage 映射、artifact continuation progress/failureReason 计算或分页 cursor 规则。
event 投影拆分只移动 event record canonicalization 和 payload projection helper，不改变 step kind canonicalization、payload key 白名单、字符串/列表裁剪上限或 event record 字段映射。
governance 投影拆分只移动 decision/approval pagination、ordering 和 view projection helper，不改变倒序排序规则、cursor 分页规则、decision/approval 字段映射或 service 层调用方式。
memory 投影拆分只移动 memory recall trace、selection score、memory hit 和 memory ID projection helper，不改变 recall trace 字段映射、quota/score/hit 映射或 memory ID 字符串化规则。
progress/context 投影拆分只移动 task progress、task decomposition、execution section、context bundle、compressed context 和 repair loop helper，不改变进度状态判定、恢复建议、上下文 section 排序、压缩候选规则、repair loop 观测字段或 evidence refs 聚合规则。
coverage subsystem 入口收口只调整 coverage ledger 的内部调用边界，不改变 work item 生成规则、证据状态转换、acceptance gate 校验或 LLM 上下文投影字段。
patches subsystem 入口收口只调整 patch apply/revert、approval gate 与 preview event 的内部调用边界，不改变 patch request 解析、敏感路径审批规则、文件写入/revert 规则或事件 payload。
coverage source extraction 拆分只移动 Goal/DoD/profile/failed coverage source extraction、source key 和 markdown leaf 解析 helper，不改变 coverage work item ID 生成、证据状态转换、acceptance gate 校验或 LLM 上下文投影字段。
patches request/filesystem/diff 拆分只移动 patch request 解析校验、workspace path/hash/apply/preview、unified diff parser/apply helper，不改变 approval gate、patch state 转换、文件写入/revert 规则、preview 字段或事件 payload。
patches approval risk 拆分只移动 patch/revert approval risk analysis、line stats 和 approval context request helper，不改变 approval gate 触发条件、risk reason 字段、审批 block 输出或后续恢复行为。
coverage context/evidence 拆分只移动 coverage context projection、focus/ledger summary、target set、failure diagnostics、required/missing evidence 和 evidence binding helper，不改变 coverage ledger 数据结构、work item 状态推进、focus 排序、缺口计算或 LLM 上下文字段。
result subsystem 入口收口只调整 result artifact recording、runtime detail collection 和 evidence detail projection 的内部调用边界，不改变 accepted/rejected result 状态、artifact 内容、evidence refs、event payload、memory 写入内容或 completion gate 行为。
graph subsystem 入口收口只调整 graph ready spec、follow-up context、graph evidence 与 run process evidence 的读取/组装边界，不改变 graph dispatch、runtime repair、completion gate 或 run state transition。
convergence subsystem 入口收口只调整 RuntimeOrchestrator 对 convergence contract 的调用入口，不改变合同状态、输出字段、mechanical repair/replan 判定或 governance gate 规则。
repair budget subsystem 入口收口只调整 retry/repair attempt 计数的读取边界，不改变 repair action、blocker ticket、runtime failure classification 或 LLM repair input 内容。
repair context subsystem 入口收口只调整 repair 输入素材的读取与投影边界，不改变 repair follow-up 主流程、状态转换、目标判定或具体输出合同。
LLM context subsystem 入口收口只调整 RuntimeOrchestrator 内部对 compact/projection helper 的访问边界，不改变上下文字段裁剪、摘要上限、引用 URI、事件 payload 或 tool-call evidence 字段。
LLM context event/evidence 投影收口只统一 event/tool-call summary 的压缩入口，不改变 raw payload 引用策略、事件发布顺序或 evidence 记录状态。
repair dispatch action selection 收口只移动失败恢复动作选择入口，实际 dispatch、blocker ticket 创建、run fail/complete 与 step 创建仍由 `RuntimeOrchestrator` 协调。
repair dispatch executable spec 收口只移动 provider retry/runtime repair 的 spec 规划，实际 step dispatch、blocker ticket 创建和 run 状态变更仍由 `RuntimeOrchestrator` 协调。
repair dispatch blocker delegation 收口只移动 blocked recovery action 到具体 blocker executor 的分发边界，实际治理 ticket、decision block 与 result 记录仍由 `RuntimeOrchestrator` 协调。
repair dispatch fail-run reason 收口只移动失败摘要优先级归一化，实际 FailRun、rejected result recording 与事件发布仍由 `RuntimeOrchestrator` 协调。
runtime execution coordination 边界确认明确状态变更型执行协调不继续下沉，避免子系统直接持有 run/step/ticket/result 状态机写入权。
repair 大文件职责拆分只按现有函数职责重新落文件，未新增业务判断，未改变 runtime repair 合同、recovery action、provider retry、patch apply、coverage gap、context material 或 governance blocker 行为。
runtime orchestrator 协调层文件拆分只移动 lifecycle/resume/completion/state sync/dispatch/contract/spec helper 的文件归属，`RuntimeOrchestrator` 仍作为状态机写入协调边界。
event/checkpoint subsystem 收口只移动事件发布和 checkpoint 记录的依赖边界，事件 topic/payload 与 checkpoint 数据结构保持不变。
result recording subsystem 收口只移动 result、completion evidence、memory 与 deliverable manifest 记录职责，completion gate、run/workspace 状态写入和 dispatch 协调仍由 `RuntimeOrchestrator` 负责。
run state read helper subsystem 收口只移动 run 状态相关读取/计算 helper，run close、workspace sync、replan successor 创建与事件发布继续由 `RuntimeOrchestrator` 协调。
run/workspace active state sync 收口只移动 active run 绑定和 run/workspace 持久化同步边界，lifecycle/resume/block/validate 的流程判断、事件发布与 dispatch 继续由 `RuntimeOrchestrator` 协调。
validation run/workspace sync 收口只移动 validation 阶段的 run/workspace 持久化同步，completion gate、validating/completed 两阶段事件发布与最终完成流程继续由 `RuntimeOrchestrator` 协调。
terminal workspace release/completion sync 收口只移动终态 workspace 释放和 completed run/workspace 持久化同步，CompleteRun、validated completion 的流程入口与事件发布继续由 `RuntimeOrchestrator` 协调。
decision resume selection 收口只移动 resolved ticket/resolution 读取与选择逻辑，实际 resume/replan/fail 状态机分支继续由 `RuntimeOrchestrator` 协调。
ValidateRun validation sync 收口只统一手动 validation 与自动 validation completion 的 run/workspace 持久化路径，入口校验和事件发布继续由 `RuntimeOrchestrator` 协调。
StartRun active workspace guard 收口只移动启动前 workspace/active run 冲突读取逻辑，run state 写入、workspace active 绑定、事件发布与 dispatch 继续由 `RuntimeOrchestrator` 协调。
workspace active run reconcile 收口只移动 workspace activeRun 读取与修正写入逻辑，reconcile 调用时机和事件发布继续由 `RuntimeOrchestrator` 协调。
rejected DoD result recording 收口只移动 rejected result 的 assessment 合成与记录逻辑，触发时机、repair/block 分支和状态机流程继续由 `RuntimeOrchestrator` 协调。
unused dispatch single-spec helper 清理只删除无调用点的旧入口，运行期 dispatch 继续使用 `nextDispatchSpecs`。
dispatch step ID helper 收口只将 step ID 去重逻辑改为显式 repository helper，dispatch 时机、spec 内容和状态机流程不变。
lifecycle block wrapper 清理只删除无行为转发层，decision/approval block 仍由 `RuntimeOrchestrator` 的 AtStep 协调入口处理。
unused lifecycle/completion wrapper 清理只删除无调用点旧入口，实际终态关闭和 validation completion 流程不变。
unused private receiver helper 清理只删除无调用点旧入口，实际 graph dispatch、provider retry budget 与 evidence step lookup 路径不变。
unused dispatch specs wrapper 清理只删除单调用转发层，实际 dispatch spec 选择、step dispatch 和 graph batch dispatch 路径不变。
runtime policy accessor 收口只将 policy 默认值读取改为显式包级 helper，实际 repair/retry 预算、LLM output policy 投影和上下文字段不变。
unused dispatch step wrapper 清理只删除单调用转发层，实际 step create/update、event publish、tool call dispatch 和 workspace 注入路径不变。
provider retry input builder 收口只将 provider retry input 构造从 orchestrator builder 接口改为显式 policy 参数，实际 retry attempt、budget scope、step ID 与 input 字段不变。
decision block rejected result recording 收口只移动 rejected result 记录 helper 的接收者，实际 decision/block 分支、result policy 和 rejected assessment 写入不变。
current-run evidence ref wrapper 清理只删除单调用循环 wrapper，实际 current-run evidence 可用性判定与错误输出不变。
failed loaded run wrapper 清理只删除失败关闭转发层，实际 `closeLoadedRun` 状态转换、事件发布和 workspace sync 路径不变。
governance history LLM input wrapper 清理只删除单调用投影包装，实际 current governance 前置、history 压缩上限和 LLM 输入字段不变。
result context projection 收口只移动 accepted result 与 deliverable context 投影接收者，实际排序、数量上限、字段映射和 stable context 复用规则不变。
recent failure context helper 收口只将失败历史投影改为显式 repository helper，实际失败 step 过滤、倒序扫描、数量上限和字段映射不变。
step input memory run state helper 收口只将 workspace run state 读取改为显式 repository helper，实际 selection score、降噪标签和 memory selection 规则不变。
completion evidence ref catalog helper 收口只将 completion evidence catalog 构建改为显式 repository helper，实际 step/patch alias、scope 和 canonical ref 判定不变。
evidence ref repository helper 收口只将 evidence ref 查询与质量判定依赖显式化，实际 DoD evidence refs 归一化、fallback 选择、current-run evidence 与 validation quality 规则不变。
output mode selector helper 收口只将输出模式优先级计算与 open artifact continuation 读取依赖显式化，实际 mode priority、requiredOutput 注入和 artifact continuation context 字段不变。
repair coverage evidence candidate helper 收口只将 coverage candidate 读取改为显式 step repository helper，实际候选排序、coverage target 匹配、fallback 数量和 repair input 字段不变。
decision readable resource context helper 收口只将 readable workspace resource context 的 workspace 读取依赖显式化，实际 manifest 扫描、readableResources 字段和自动 repair 判定不变。
completion closure evidence helper 收口只将 completion closure evidence 投影依赖显式化，实际 latest/superseded/patch/validation evidence 字段和 graph follow-up 输入不变。
DoD evidence quality sub-check helper 收口只将 evidence quality 子校验依赖显式化，实际 artifact continuation evidence、definition change current-run evidence、profile evidence kind 与 claim alignment 规则不变。
completion gate evidence ref normalization helper 收口只将 completion gate DoD assessment evidence ref normalization 依赖显式化，实际 catalog、output 挂载位置和 evidenceRef 规则不变。
runtime repair current progress helper 收口只将 currentExecutionProgress 投影依赖显式化，实际字段、latest action evidence 和 recent repair attempt 摘要不变。
coverage validation repair escalation helper 收口只将 validation repair 升级判定依赖显式化，实际 attempt、target evidence 和 empty workspace surface 规则不变。
LLM output mode selection input helper 收口只将 output mode input 投影依赖显式化，实际 outputMode、artifactOutputContract 和 requiredOutput 规则不变。
initial repair target snapshot hydration helper 收口只将初始 repair 目标快照读取与 convergence contract 注入依赖显式化，实际 initial step input、target snapshot 合并和 repair guidance 规则不变。
execution context workspace manifest helper 收口只将 workspace manifest 投影依赖显式化，实际 step input manifest 优先、workspace deliverable 扫描和 compact 规则不变。
current-run governance history helper 收口只将当前 run 的 decision/approval 历史投影依赖显式化，实际字段、resolved decision enrichment 和 repair/execution context 注入顺序不变。
step input memory context helper 收口只将 memory recall 投影依赖显式化，实际 memory 选择、run state 降噪和 degraded fallback 规则不变。
DoD assessment evidence quality helper 收口只将 assessment evidence quality 校验依赖显式化，实际 evidence ref、profile requirement 和 definition change 校验规则不变。
runtime repair exhaustion success evidence helper 收口只将 repair-plan blocker 成功证据判定依赖显式化，实际 scope 匹配和 auto-release 判定不变。
duplicate runtime failure auto-resume helper 收口只将 duplicate failure blocker auto-resume 判定依赖显式化，实际 ticket 识别、attempt budget 和 resume 条件不变。
runtime repair exhaustion auto-resume helper 收口只将 repair-exhausted blocker auto-resume 判定依赖显式化，实际 coverage evidence shortcut、target scope/window 和 budget 规则不变。
known readable decision auto-repair helper 收口只将已知可读资源决策 blocker 的判定依赖显式化，实际 completion gate、decision kind、completed step 与 readable resource 匹配规则不变。
provider retry resolved-decision specs helper 收口只将 provider retry 决策恢复后的 spec 生成依赖显式化，实际 failed provider step 判定、attempt 计数、唯一 step ID 与 retry 输入规则不变。
LLM context 测试拆分只迁移 compact input/output、artifact chunk omission、lineage compaction、redacted patch omission 测试落点，不改变测试断言或 runtime context 行为。
runtime orchestrator 集成测试拆分只移动测试函数文件归属，公共 stub/helper 保留在 `runtime_orchestrator_test.go`，不改变测试输入、断言或覆盖意图。

### 4.2 待继续收口

| 项 | 状态 | 下一步 |
|----|------|--------|
| P1 orchestrator 子系统组合化后续项 | 进行中 | coverage、patches、result、graph、convergence、repair budget/context、LLM context、event/checkpoint、repair dispatch action selection/spec/blocker/fail-run reason、result recording、run state read helper、run/workspace active state sync、validation run/workspace sync、terminal workspace release/completion sync、decision resume selection、ValidateRun validation sync、StartRun active workspace guard、workspace active run reconcile、rejected DoD result recording、unused dispatch single-spec helper 清理、dispatch step ID helper、lifecycle block wrapper 清理、unused lifecycle/completion wrapper 清理、unused private receiver helper 清理、unused dispatch specs wrapper 清理、runtime policy accessor 收口、unused dispatch step wrapper 清理、provider retry input builder 收口、decision block rejected result recording 收口、current-run evidence ref wrapper 清理、failed loaded run wrapper 清理、governance history LLM input wrapper 清理、result context projection 收口、recent failure context helper 收口、step input memory run state helper 收口、completion evidence ref catalog helper 收口、evidence ref repository helper 收口、output mode selector helper 收口、repair coverage evidence candidate helper 收口、decision readable resource context helper 收口、completion closure evidence helper 收口、DoD evidence quality sub-check helper 收口、completion gate evidence ref normalization helper 收口、runtime repair current progress helper 收口、coverage validation repair escalation helper 收口、LLM output mode selection input helper 收口、initial repair target snapshot hydration helper 收口、execution context workspace manifest helper 收口、current-run governance history helper 收口、step input memory context helper 收口、DoD assessment evidence quality helper 收口、runtime repair exhaustion success evidence helper 收口、duplicate runtime failure auto-resume helper 收口、runtime repair exhaustion auto-resume helper 收口、known readable decision auto-repair helper 收口、provider retry resolved-decision specs helper 收口、repair 文件职责拆分与 runtime orchestrator 协调层文件拆分已完成；orchestrator 源码最大文件已低于 3,000 行，`RuntimeOrchestrator` receiver 方法已降至约 87 个，需继续核对 dispatch/lifecycle 是否还可在保持状态机流程协调边界的前提下通过窄接口收口 |
| P2 application 大文件拆分后续项 | 已完成 | read model 已降至约 1,293 行；`http.go` 已降至约 45 行，server shared helpers 已按类型、默认代理、run cache/queue、query helper、response helper 拆分；application 层剩余大文件不再作为本阶段阻塞项 |
| P3 orchestrator 测试拆分 | 已完成 | LLM context、graph 局部、repair budget/context、repair dispatch spec/blocker/fail-run reason 测试已迁移或补齐，convergence 测试已独立；`runtime_orchestrator_test.go` 已降至约 937 行，orchestrator 测试最大文件约 2,895 行 |

---

## 五、验证方式

| 验证项 | 方法 |
|--------|------|
| 文件拆分正确性 | `make ci` 通过（fmt-check + vet + test + build）；阶段性拆分可先运行受影响包测试与 `git diff --check` |
| 子系统提取正确性 | `make test-race` 通过，无 data race；阶段性收口先以受影响包 `go test -count=1` 验证行为不变 |
| 接口不变性 | 外部调用方（application 层）无需修改 |
| 行为不变性 | 502 个现有测试全部通过 |
| 架构约束 | 子系统 struct 不导入 orchestrator 本体的非公开字段 |

---

## 六、预期效果

| 指标 | 当前 | 目标 |
|------|------|------|
| RuntimeOrchestrator 方法数 | ~87 | ~60（核心 dispatch + lifecycle） |
| 最大源文件行数 | ~2,830 (`runtime_graph.go`)；repair 最大文件约 1,754，HTTP 最大拆分文件约 2,074 | <3,000 |
| 最大测试文件行数 | ~2,895 (`runtime_convergence_test.go`)；`runtime_orchestrator_test.go` 已降至约 937 | <8,000 |
| 子系统独立测试覆盖 | 部分完成（coverage/patches/result/graph/convergence/repair/LLM context 已有局部入口或独立测试） | 每个子系统可独立构造和测试 |
| 新增职责归属判断 | 已改善但未完全闭环（状态变更型协调仍集中在 `RuntimeOrchestrator`） | 明确（必须选择子系统；状态机写入保留协调层） |
