# WorkOrch Runtime Readable ID and Recovery Contract

本文定义 WorkOrch runtime 后续收口的可读 ID、provider retry 边界、contract violation 治理、验证脚本口径和 memory 噪音治理合同。目标是让执行状态可以从 ID 中直观看出阶段和顺序，同时继续用结构化字段表达因果关系，避免重新依赖 ID 字符串推断 lineage。

## 背景

`ws-ecda4b033112135b` 的最新运行暴露出几个前期修复后仍未闭合的问题：

- runtime 已能识别 read-only graph、弱测试证据、缺失 `targetRef` 等 LLM contract violation，但 violation 之后的恢复路径仍不够统一。
- provider retry step 出现 `step-d5427442128e-provider-retry-1` 这类缺少 run 前缀的 ID，虽然避免了长 lineage，但不满足 run-scoped 一致性，也不利于排查。
- follow-up / graph / repair 使用 hash token 后不再嵌入完整祖先 ID，但 `step-2042911477da` 这类 token 仍不够直观，无法通过 ID 快速判断 stage、loop 和业务节点。
- 验证脚本把历史 contract violation 统一判为失败，未区分“未治理违规”和“已由 runtime 正确治理的违规”。
- memory recall 中仍可能携带旧长 ID 和 provider retry 流水，干扰 LLM 对当前 Goal / DoD / patch / evidence 缺口的判断。

## 设计目标

- 所有 runtime 生成的 step ID 必须 run-scoped。
- 尽量减少 hash/random token，ID 应表达 workspace/run/stage/loop/node 等可读信息。
- ID 只用于定位和人工排查，完整因果关系仍由结构化 lineage 字段表达。
- provider retry 只处理 provider transient failure，不承担业务 repair 语义。
- LLM contract violation 走统一恢复状态机：repair、rejected result、decision/blocker。
- 验证脚本按“违规是否被治理”判定，而不是只要历史出现违规就失败。
- memory recall 优先保留高价值决策信息，压缩或剔除旧 ID 噪音。

## 非目标

- 不通过 ID 字符串反推 recovery root、budget scope 或业务因果关系。
- 不为了可读 ID 牺牲 repository 主键唯一性、并发安全和 resume 幂等。
- 不在 core runtime 中引入具体业务领域或技术栈规则。
- 不兼容旧 ID 生成策略；当前阶段以彻底收口为目标。

## ID 合同

### 总原则

运行时可读 ID 分两层：

- 主键 ID：用于 repository、幂等、resume 和事件追踪，必须稳定、唯一、run-scoped。
- 结构化 lineage：用于表达因果关系、root、attempt、budget scope 和恢复动作。

所有 step ID 必须满足：

- 以 `<run-id>/` 开头。
- 包含可读 stage。
- 使用递增 loop / attempt 表达顺序。
- 对 graph node 保留可读 node ID。
- 不嵌入完整 ancestor step ID。
- 不出现裸 `step-<hash>` 作为顶层 step ID。

### 推荐格式

| 类型 | 格式 | 示例 |
| --- | --- | --- |
| 初始 LLM | `<run-id>/planning-001` 或保留 `<run-id>/step-1` | `run-xxx/planning-001` |
| graph node | `<run-id>/graph-<graph-seq>/<node-seq>-<node-id>` | `run-xxx/graph-001/004-write-main` |
| graph follow-up | `<run-id>/follow-up-<seq>/<purpose>` | `run-xxx/follow-up-002/dod-gate` |
| completion follow-up | `<run-id>/follow-up-<seq>/completion` | `run-xxx/follow-up-003/completion` |
| runtime repair | `<run-id>/repair-<attempt>` | `run-xxx/repair-002` |
| provider retry | `<run-id>/provider-retry-<attempt>` 或 `<run-id>/<stage>/provider-retry-<attempt>` | `run-xxx/provider-retry-001` |
| assessment correction | `<run-id>/assessment-correction-<attempt>` | `run-xxx/assessment-correction-001` |
| decision ticket | `<run-id>/decision-<seq>/<reason>` | `run-xxx/decision-001/provider-config` |
| result | `<run-id>/result/<kind>` | `run-xxx/result/dod-assessment` |

### Graph Scope

同一个 run 内可能多次出现相同 graph node ID，例如多个 graph 都有 `write-readme`。不能使用 hash token 解决可读性问题，必须使用 graph scope：

```text
run-xxx/graph-001/004-write-readme
run-xxx/graph-002/004-write-readme
```

graph scope 来自结构化顺序，不来自 root step ID 字符串。root 关系记录在：

```json
{
  "graphRootStepId": "run-xxx/repair-001",
  "graphSeq": 2,
  "graphNodeSeq": 4,
  "graphNodeId": "write-readme"
}
```

### Provider Retry ID

provider retry ID 必须 run-scoped，不能再出现：

```text
step-d5427442128e-provider-retry-1
```

预期形态：

```text
run-xxx/provider-retry-001
```

如果需要表达 retry 的业务上下文，放入结构化字段：

```json
{
  "providerRetryRootStepId": "run-xxx/follow-up-003/dod-gate",
  "recoveryAction": "provider_retry",
  "recoveryRootStepId": "run-xxx/follow-up-003/dod-gate",
  "recoveryLineageType": "provider_retry",
  "recoveryBudgetScope": {
    "lineageType": "provider_retry",
    "rootStepId": "run-xxx/follow-up-003/dod-gate",
    "failureClass": "provider_timeout",
    "failureSignature": "..."
  }
}
```

### 唯一性规则

ID 顺序必须由当前 run 已存在 step 推导，不能依赖随机数。推荐规则：

- stage seq：统计当前 run 下同 stage 已存在数量后加一。
- graph seq：每个新的 `executionGraph` root 分配一个递增 `graphSeq`。
- node seq：使用 graph nodes 中的顺序，格式化为三位数。
- attempt：使用结构化 root + failure class + failure signature 的 budget count 后加一。
- 并发图节点使用 graph node order 保证稳定，不使用创建时间生成 ID。

## Lineage 字段合同

ID 可读不代表可以移除 lineage。以下字段仍是恢复与预算计算的唯一依据：

| 字段 | 目的 |
| --- | --- |
| `parentStepId` | 直接派生来源 |
| `rootStepId` | 当前阶段根步骤 |
| `graphRootStepId` | graph node 所属 graph root |
| `graphSeq` | graph scope 顺序 |
| `graphNodeId` | LLM 输出的原始 node ID |
| `graphNodeSeq` | graph 内节点顺序 |
| `recoveryAction` | provider_retry / runtime_repair / decision / fail |
| `recoveryRootStepId` | recovery budget root |
| `recoveryLineageType` | provider_retry / runtime_repair / assessment_correction |
| `recoveryBudgetScope` | budget 聚合 key |
| `providerRetryRootStepId` | provider retry 的原始失败 root |
| `repairOfStepId` | repair 直接修复对象 |
| `repairOfRootStepId` | repair 业务根因 |

## Provider Retry 合同

provider retry 只处理 provider transient failure：

- `provider_timeout`
- `rate_limited`
- `context_too_large`

provider retry 不处理：

- `llm_schema_invalid`
- `llm_decision_invalid`
- read-only graph
- `patch/apply` 缺 `targetRef`
- assessment-only 返回 executable fields
- weak evidence / DoD coverage missing

provider retry 返回后必须重新分类：

| retry 输出 | 后续动作 |
| --- | --- |
| provider transient failure | 若未超 budget，继续 provider retry；超 budget 后 block provider/config decision |
| contract/schema invalid | 退出 provider retry，进入 runtime repair 或 terminal rejected |
| failed DoD assessment | 进入 DoD/evidence repair 或 rejected result |
| valid patch graph / patch output | 正常派发 graph / patch |
| valid accepted DoD assessment | 写 accepted result 并完成 |

provider retry step 不得成为新的 provider retry root。retry root 只能来自 `providerRetryRootStepId`。

## Contract Violation 治理合同

以下都归为 LLM contract violation：

- state-changing 阶段返回 read-only / inspection-only / shell-only graph。
- `executionGraph` 中 `patch/apply` 缺 `targetRef`、`patchType` 或具体 patch 内容。
- assessment-only / assessment correction 返回 `executionGraph`、patch、shell、mcp、memoryWrites。
- validation follow-up 在需要 DoD assessment 时返回 read-only graph。
- implementation planning 返回不含 patch 的探索 graph。

统一恢复策略：

1. 分类为 `llm_decision_invalid` 或 `runtime_failed` contract violation。
2. 若当前阶段仍有可修复空间，派发 runtime repair。
3. repair 必须继承必要约束：
   - 实现缺口：`stateChangingOutputRequired=true`。
   - 证据缺口：要求补有效测试/验证证据。
   - assessment-only 违规：派发只读 correction 或 rejected result。
4. 若 repair budget exhausted，写 rejected result，并根据 policy failed 或 blocked_on_decision。
5. 不允许 contract violation 重新进入 provider retry，除非后续实际 provider 调用再次发生 transient failure。

## Weak Evidence 合同

弱证据不等于 runtime failure，但不能支撑 accepted result。

典型弱证据：

- `go test ./...` 返回 `[no test files]`
- `0 tests`
- no assertions
- 只 build 不覆盖新增 Goal

处理策略：

- DoD gate 必须标记 failed/skipped/inconclusive。
- 若可自动修复，进入 evidence repair / test repair。
- 若 repair exhausted，写 rejected result。
- 不允许仅凭 build pass 写 accepted result。

## Result 归因合同

failed run 必须尽力写 rejected result。rejected result 的归因应指向业务根因，而不是 provider retry 外壳。

当前偏差示例：

```text
SourceRef = step-d5427442128e-provider-retry-1
```

目标：

```text
SourceRef = run-xxx/follow-up-003/dod-gate
RootCauseRef = run-xxx/follow-up-003/dod-gate
EvidenceRefs = [
  "run-xxx/provider-retry-001",
  "run-xxx/follow-up-003/dod-gate"
]
```

provider retry 只能作为 evidence，不应覆盖业务 root cause。

## 验证脚本合同

验证脚本应区分三类 violation：

| 类型 | 判定 |
| --- | --- |
| unhandled violation | fail |
| handled violation | pass with warning |
| terminal violation with rejected result | pass |

handled violation 必须满足至少一个条件：

- 后续存在 runtime repair step，且 `repairOfStepId` 指向违规 step。
- 后续存在 rejected result，且 evidence 或 root cause 覆盖违规 step。
- 后续存在 decision ticket，且 `resultPolicy` 与 blocker scope 匹配。
- 后续存在 assessment correction，并且 correction 输入输出满足 assessment-only 合同。

脚本仍必须 fail：

- provider retry ID 不 run-scoped。
- provider retry root 指向 retry step。
- recovery step 缺少 lineage 字段。
- completed run 无 accepted result。
- failed DoD run 无 rejected result。
- rejected result source/evidence 无法追溯到根因。

## Memory Recall 合同

memory 应服务当前 LLM 决策，而不是复述 runtime 流水。

优先保留：

- 当前 Goal / DoD。
- LLM 已接纳的关键输入。
- 已应用 patch 摘要。
- 当前未满足 DoD 与证据缺口。
- 有效测试命令、弱证据结论。
- skill / tool / MCP / soul / prompt 约束。
- 历史修复 lesson 的归一化摘要。

应压缩或剔除：

- 旧长 lineage step ID。
- provider retry 流水。
- 重复 timeout 详情。
- 与当前 Goal / DoD 无关的旧 run 细节。
- 只包含 runtime 机制而没有业务决策价值的 memory。

推荐 memory 摘要格式：

```text
type: execution_lesson
stage: evidence_repair
rootCause: tests were inconclusive because go test reported no test files
affectedDoD: API proxy, streaming, token accounting, error passthrough, persistence
nextAction: add effective tests for non-stream, stream, upstream error, CSV logging, config loading
sourceRefs: [run-xxx/follow-up-003/dod-gate]
```

## 收口计划

| ID | 优先级 | 内容 | 验收 |
| --- | --- | --- | --- |
| RID-01 | P0 | 所有 provider retry ID 改为 run-scoped | 不再出现裸 `step-...provider-retry` |
| RID-02 | P0 | 用 stage/loop/node 替代 hash token | follow-up、repair、graph ID 可读 |
| RID-03 | P0 | graph scope 使用 `graph-<seq>/<node-seq>-<node>` | repeated node 不冲突且不含 ancestor ID |
| RID-04 | P0 | provider retry schema/contract invalid 退出 retry 分支 | retry 后 contract invalid 进入 repair 或 rejected |
| RID-05 | P0 | contract violation 统一恢复状态机 | read-only graph、缺 targetRef、assessment graph 都按同一规则治理 |
| RID-06 | P1 | rejected result root cause 归因调整 | source/root 指向业务失败 root，provider retry 作为 evidence |
| RID-07 | P1 | 验证脚本支持 handled violation | 已治理违规不导致脚本失败 |
| RID-08 | P1 | memory recall 去除旧 ID 噪音 | recall 以 Goal/DoD/patch/evidence gap 为主 |
| RID-09 | P1 | 补充测试矩阵 | 覆盖 ID、retry、contract violation、result、script |

## 测试矩阵

- provider retry ID 必须以 run ID 开头。
- provider retry ID 不包含完整 ancestor step ID。
- provider retry root 不指向 retry step。
- repeated graph node 在不同 graph scope 下生成可读唯一 ID。
- graph follow-up ID 使用 purpose + seq，不使用 hash。
- state-changing read-only graph 触发 runtime repair。
- `patch/apply` 缺 `targetRef` 触发 contract repair 或 rejected result。
- provider retry 返回 `llm_schema_invalid` 不继续 provider retry。
- weak evidence 不能 accepted，必须进入 evidence repair 或 rejected。
- failed DoD run 必须写 rejected result。
- 验证脚本对 handled violation pass with warning。
- memory recall 不包含旧长 lineage ID 作为主要内容。

## 部署核对清单

每次部署后核对：

1. 最新 run ID 和 workspace 状态。
2. 所有 step ID 是否 run-scoped。
3. 是否存在裸 `step-` 前缀 step ID。
4. provider retry 是否只处理 provider transient failure。
5. contract violation 是否进入 repair / rejected / decision。
6. patch graph 是否至少含合法 `patch/apply`。
7. weak evidence 是否被拒绝并触发修复。
8. terminal run 是否有匹配 result。
9. result root cause 是否指向业务根因。
10. 验证脚本是否通过或仅输出 handled warning。
11. memory recall 是否以高价值决策信息为主。

