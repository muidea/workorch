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
| `http_workspace.go` | workspace CRUD + snapshot + activation + setup draft handlers |
| `http_run.go` | run CRUD + status + observability + detail page handlers |
| `http_governance.go` | decision + approval handlers |
| `http_configuration.go` | config + soul + skill + MCP + template handlers |
| `http_events.go` | SSE stream + event list handlers |
| `http.go` | router setup, middleware, shared helpers, service struct |

#### read_model_service.go（7,131 行）→ 按投影面拆分

| 新文件 | 内容 |
|--------|------|
| `read_model_workspace.go` | workspace snapshot, workspace outcome, workspace task progress |
| `read_model_run.go` | run snapshot, run observability, run context bundle |
| `read_model_timeline.go` | 已存在（442 行），保持不变 |
| `read_model_coverage.go` | coverage projection, evidence mapping, ledger summary |
| `read_model_outcome.go` | mapRunOutcome, mapWorkspaceOutcome, terminal state logic |
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

## 四、验证方式

| 验证项 | 方法 |
|--------|------|
| 文件拆分正确性 | `make ci` 通过（fmt-check + vet + test + build） |
| 子系统提取正确性 | `make test-race` 通过，无 data race |
| 接口不变性 | 外部调用方（application 层）无需修改 |
| 行为不变性 | 502 个现有测试全部通过 |
| 架构约束 | 子系统 struct 不导入 orchestrator 本体的非公开字段 |

---

## 五、预期效果

| 指标 | 当前 | 目标 |
|------|------|------|
| RuntimeOrchestrator 方法数 | 229 | ~60（核心 dispatch + lifecycle） |
| 最大源文件行数 | 8,249 (runtime_repair.go) | <3,000 |
| 最大测试文件行数 | 31,201 | <8,000 |
| 子系统独立测试覆盖 | 不可能（需完整 orchestrator） | 每个子系统可独立构造和测试 |
| 新增职责归属判断 | 模糊（任何文件都可以加方法） | 明确（必须选择子系统） |
