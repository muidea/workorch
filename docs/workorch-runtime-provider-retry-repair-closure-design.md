# WorkOrch Runtime Provider Retry / Repair Closure Design

本文记录 `ws-c51fa60493105285` 最新真实运行暴露的 provider retry、runtime repair、provider/config terminal policy 与 memory recall 缺口，并定义后续落代码前的收口设计。当前阶段不考虑兼容旧行为。

## 触发背景

最新核对的运行包括：

- `run-e2eae0f67cd60dbc`：failed，出现 `provider-retry-001-of-repair-001` 的 root 指向 `repair-001-of-provider-retry-001`。
- `run-a496dae327bb3201`：failed，脚本通过，但仅覆盖 provider `rate_limited -> provider_unavailable` 的简单终止路径。
- `run-71acc18bb58ff359`：running，但已经复现同类 provider retry / repair lineage 污染，并出现未治理 state-changing violation。

核心脚本失败示例：

```text
provider retry root points at retry step:
run-71acc18bb58ff359/provider-retry-001-of-repair-001
root=run-71acc18bb58ff359/repair-001-of-provider-retry-001

unhandled violation:
state-changing step returned non-patch executionGraph
```

这说明前期“禁止 retry-of-retry”的修复只覆盖了直接 provider retry step 再 retry，没有覆盖 “provider retry 输出 contract violation -> runtime repair -> provider transient failure -> provider retry” 的交织路径。

## 当前偏差

| ID | 偏差 | 运行表现 | 风险 |
| --- | --- | --- | --- |
| PRR-01 | runtime repair step ID 使用 provider retry 外壳 label | `repair-001-of-provider-retry-001` | 人工排查仍看到 retry lineage 嵌套，后续 retry root 容易被污染 |
| PRR-02 | provider retry root 可指向 repair-of-provider-retry 外壳 | `providerRetryRootStepId=run.../repair-001-of-provider-retry-001` | provider retry budget 被外壳切分，无法按业务 root 聚合 |
| PRR-03 | provider/config terminal policy 未统一 | `auth_unavailable` 可写业务 rejected result 并 failed | WorkOrch runtime provider 配置问题被误当作业务 DoD 失败 |
| PRR-04 | running run 可停在 unhandled violation | `provider-retry-001-of-repair-001` failed 后无 result / ticket | run 既未继续修复，也未进入治理终态 |
| PRR-05 | memory recall trace 字段不完整 | step-input trace 缺 `RefreshSource` / `SelectionPolicyVersion` / candidates | 部署核对无法解释 memory 选择，LLM 仍可能接收低价值流水 |
| PRR-06 | provider retry 流水 memory 仍进入 recall | workspace 仅有 `Provider retry outcome captured` tool_feedback | LLM 上下文被 runtime 机制噪音占用，缺少 Goal/DoD 决策信息 |
| PRR-07 | state-changing repair 再次返回只读 graph 后闭环不足 | `run-506f7364eb5ecb36/provider-retry-002-of-repair-001-of-planning-001` failed 后 run 仍为 `running`，无 ticket/result/next step | provider retry / runtime repair 交织后，最后失败 step 无法持续推进或进入治理终态 |
| PRR-08 | current step 读模型按 ID 字典序取最后 step | status 显示 `step-1`，但真实最新失败是 `provider-retry-002-of-repair-001-of-planning-001` | 排障和 resume 判断被旧 step 误导 |
| PRR-09 | activation memory 仍注入低价值 provider retry 流水 | step input 的 `executionContext.memories` 含历史 `provider_retry` tool_feedback，且 `memoryRecallTrace` 缺失 | LLM 可见输入被流程噪音污染，难以围绕 Goal/DoD 产出补丁决策 |

## 设计目标

- provider retry、runtime repair、result attribution 全部按结构化业务 root 聚合，不按 step ID 字符串外壳推断。
- provider retry 只处理 provider transient failure，不吸收 contract violation 或业务 repair 语义。
- provider/config 类 terminal failure 进入 WorkOrch runtime provider decision，默认 `resultPolicy=none`，不写业务 rejected result。
- state-changing contract violation 必须被 runtime repair、rejected result 或 decision 覆盖，不能让 running run 停在 unhandled violation。
- memory recall 对 LLM 可见内容只保留高价值 decision input/output；provider retry 流水只保留为审计 trace。

## Root 归一合同

### Root 类型

后续恢复链路统一使用三类 root：

| Root | 含义 | 示例 |
| --- | --- | --- |
| `businessRootStepId` | 当前业务目标或阶段的根因 step | `run-xxx/step-1` / `run-xxx/planning-001` |
| `repairRootStepId` | runtime repair budget root，必须归一到业务 root | `run-xxx/step-1` |
| `providerRetryRootStepId` | provider retry budget root，必须是实际发生 provider failure 的业务/repair request root，但不得是 provider retry 派生外壳 | `run-xxx/repair-001-of-planning-001` 或归一后的业务 root |

### Provider Retry Root 归一

`providerRetryRootStepID(step)` 不再直接返回以下外壳：

- `provider-retry-*`
- `repair-*-of-provider-retry-*`
- `assessment-correction-*-of-provider-retry-*`
- 任何输入中 `repairOfStepId` / `failedStepId` 指向 provider retry step 的派生 recovery step

归一顺序：

1. 如果 `step.Input.providerRetryRootStepId` 存在，先解析该 root。
2. 如果 root 是 provider retry step，读取该 root step 的 `providerRetryRootStepId` 继续归一。
3. 如果 root 是 repair-of-provider-retry step，读取其 `repairOfRootStepId` / `recoveryRootStepId` / `repairOfStepId`，继续归一到业务 root。
4. 如果当前 failed step 是 runtime repair step，且其 `repairOfRootStepId` 存在，provider retry root 应使用该 repair step 自身还是业务 root 由 failure 类型决定：
   - provider failure 发生在 repair LLM 调用本身：root 可以是该 repair step，但该 repair step 的 label 必须使用业务 root，不得包含 provider retry 外壳。
   - provider failure 发生在 repair-of-provider-retry 外壳：root 归一到 `repairOfRootStepId` 或原始 repair root，不能使用外壳 ID。
5. 最终 root 必须满足 `normalizedRecoveryRoot(root) == root`，且不包含 provider retry 派生外壳。

### Runtime Repair Root 归一

`runtimeRepairRootStepID(step)` 不再把 provider retry 外壳作为 repair root。

归一顺序：

1. 如果 failed step 是 provider retry step，优先使用 `providerRetryRootStepId` 的归一结果。
2. 如果 failed step 是 provider retry 的 contract violation 输出，repair root 指向该 provider retry 所属业务 root，而 `repairOfStepId` 保留 provider retry step 作为直接修复对象。
3. 如果 failed step 是 repair-of-provider-retry 派生 step，repair root 指向 `repairOfRootStepId` 或 `recoveryRootStepId` 的归一结果。
4. `repairOfStepId` 只表达直接失败对象；`repairOfRootStepId` / `recoveryRootStepId` 才参与 budget 与 readable ID label。

### Readable ID Label

`runtimeRepairFollowUpStepID` 和 `providerRetryStepID` 的 label 使用归一 root label，不使用直接 failed step label。

预期变化：

```text
错误：
run-xxx/repair-001-of-provider-retry-001
run-xxx/provider-retry-001-of-repair-001

目标：
run-xxx/repair-001-of-planning-001
run-xxx/provider-retry-001-of-repair-001-of-planning-001
```

若同一 run 内出现同 label 冲突，使用结构化 attempt 递增或追加 recovery seq，不重新引入 hash。

## Recovery 决策合同

### Provider Failure 发生位置

| 失败位置 | failure class | 恢复动作 |
| --- | --- | --- |
| 普通 LLM step | `provider_timeout` / `rate_limited` / `context_too_large` | provider retry，超预算后 provider/config decision |
| runtime repair LLM 调用 | provider transient | provider retry 当前 repair request，root label 归一到 repair business root |
| provider retry LLM 调用 | provider transient | 同 root provider retry attempt +1，超预算后 provider/config decision |
| provider retry 返回 contract violation | `runtime_failed` / `llm_decision_invalid` | 退出 provider retry，进入 runtime repair；repair root 归一到业务 root |
| repair-of-provider-retry 再遇 provider transient | provider transient | 不生成 repair-of-provider-retry root；按归一 root 计入 provider retry budget |
| provider unavailable / auth unavailable | `provider_unavailable` / provider config | provider/config decision，`resultPolicy=none` |

### Terminal Policy

- provider/config blocker：`blocked_on_decision`，`resultPolicy=none`，不写业务 rejected result。
- LLM contract / DoD / patch 不可修复：`blocked_on_decision` 或 `failed`，`resultPolicy=rejected`。
- running run 不能停在 failed step 后无后续动作；`DispatchNextStep` / resume 必须补齐 recovery action 或关闭为终态。

## Result 归因合同

rejected result 的 `SourceRef` 必须指向业务 root，provider retry / repair 外壳只能作为 evidence。

对于本次类似链路，若最终是业务 contract 不可修复：

```json
{
  "SourceRef": "run-xxx/step-1",
  "EvidenceRefs": [
    "run-xxx/step-1",
    "run-xxx/repair-001-of-planning-001",
    "run-xxx/provider-retry-001-of-repair-001-of-planning-001"
  ]
}
```

若最终是 provider/config blocker：

```json
{
  "runState": "blocked_on_decision",
  "decisionContext": {
    "scope": "workorch_runtime",
    "dependency": "llm_provider",
    "resultPolicy": "none",
    "providerFailureClass": "provider_unavailable"
  },
  "result": null
}
```

## Memory Recall 合同补充

### LLM 可见 memory

不得进入 LLM `selectedContext.memories`：

- `memoryType=tool_feedback` 且 tags 包含 `provider_retry`。
- summary/content 只包含 provider retry attempt、timeout、rate limit、provider unavailable 等 runtime 流水。
- 没有 Goal / DoD / patch / evidence / skill / tool decision 价值的 process memory。

可进入 LLM context：

- `accepted_input` 的 skill/tool/MCP/soul/prompt 约束。
- `dod_feedback` 中的 unmet DoD 与 evidence gap。
- `execution_lesson` 中的 repair root cause、patch plan、verification command。
- `project_context` 中被 LLM 接纳的项目结构、配置、入口、约束。

### Trace 要求

即使没有选中 memory，step-input recall trace 也必须写入：

- `RefreshSource`
- `SelectionPolicyVersion`
- `CandidateMemoryIDs`
- `SelectedMemoryIDs`
- `ExcludedMemoryIDs`
- `TypeQuota`
- `SelectionScores`
- `Reason`

当只有 provider retry 流水候选时，预期：

```json
{
  "CandidateMemoryIDs": ["run-xxx/provider-retry-001/memory/1"],
  "SelectedMemoryIDs": [],
  "ExcludedMemoryIDs": ["run-xxx/provider-retry-001/memory/1"],
  "SelectionScores": [
    {
      "id": "run-xxx/provider-retry-001/memory/1",
      "reason": "tool_feedback:current_run:excluded_low_value_process_memory"
    }
  ]
}
```

## 验证矩阵

| ID | 场景 | 预期 |
| --- | --- | --- |
| PRR-T01 | provider retry step 输出 state-changing read-only graph | 退出 provider retry，派发 runtime repair；repair root 归一到业务 root |
| PRR-T02 | repair-of-provider-retry LLM 调用 provider timeout | provider retry root 不得指向 `repair-*-of-provider-retry-*` |
| PRR-T03 | provider unavailable / auth unavailable | blocked_on_decision + `resultPolicy=none`，无 rejected result |
| PRR-T04 | repeated provider timeout under same repair root | provider retry attempt 递增，不生成新 root |
| PRR-T05 | running run 最后 step failed | `DispatchNextStep` 后必须进入 repair / decision / failed result 之一 |
| PRR-T06 | rejected result from provider retry / repair chain | `SourceRef` 指向业务 root，外壳 step 在 `EvidenceRefs` |
| PRR-T07 | only provider retry tool_feedback memory exists | LLM selected memories 为空或不含该 memory，trace 记录 excluded reason |
| PRR-T08 | accepted Skill / Tool / MCP / Soul memory 与 provider retry memory 同时存在 | 选中 accepted input memory，排除 provider retry 流水 |
| PRR-T09 | verify script fixture for repair-of-provider-retry root | 脚本 fail，提示 provider retry root points at retry-derived repair |
| PRR-T10 | verify script fixture for provider/config decision | blocked_on_decision + resultPolicy none 通过 |
| PRR-T11 | state-changing repair 的 provider retry 返回 read-only graph | latest failed step 后必须继续 runtime repair，或达到预算后写 rejected result / block decision，不停留 running |
| PRR-T12 | run status currentStep | 使用最新 checkpoint / 真实最新 step，不受 step ID 字典序影响 |
| PRR-T13 | DoD run 反复读取缺失 workspace 文件 | runtime repair 必须继承 state-changing 约束，并引导创建缺失文件；若仍无法推进，必须写 result 或开 decision |
| PRR-T14 | provider retry / runtime repair 携带失败 graph node context | 允许保留 `graphNodeId` 作为失败上下文，但 recovery step ID 不按 graph dispatch ID 校验；普通 graph node 仍强制 `graph-<seq>/<node-seq>` |
| PRR-T13 | activation memory contains only provider retry process memory | 不进入 LLM 可见 `executionContext.memories`，trace 记录 candidate/excluded |

## 落地步骤

1. 增加 recovery root normalizer。
   - 统一处理 provider retry step、repair-of-provider-retry step、assessment correction 外壳。
   - 供 `providerRetryRootStepID`、`runtimeRepairRootStepID`、result attribution、verify script 共用。
2. 调整 provider retry / runtime repair ID 生成。
   - ID label 使用 normalized root label。
   - 保留 `repairOfStepId` 指向直接失败 step。
3. 调整 recovery action terminal policy。
   - `provider_unavailable` / auth unavailable 进入 provider/config decision。
   - provider/config decision 使用 `resultPolicy=none`。
4. 修复 running run failed-step 补派闭环。
   - `DispatchNextStep` 对最后 failed step 必须执行 recovery action。
   - 无可恢复动作时明确 terminal failed/rejected。
5. 修复 current step 读模型。
   - status/snapshot 优先使用最新 checkpoint 指向的 step。
   - 禁止按 step ID 字典序把早期 step 误判为最新 step。
6. 完善 memory selector 与 trace。
   - 即使 selected 为空也记录结构化 candidate/excluded trace。
   - provider retry process memory 不进入 LLM context。
   - activation 阶段与 step-input refresh 使用同一可见性过滤口径。
7. 扩展脚本与 Go 测试。
   - 新增 PRR-T01 至 PRR-T13 覆盖。
8. 更新进度文档并部署核对 `ws-c51fa60493105285` 的新 run。

## 收口判定

完成后，最新真实运行必须满足：

- `scripts/verify-workorch-runtime-core-flow.sh <workspace> <run>` 通过，或只输出 handled warning。
- 不再出现 `repair-*-of-provider-retry-*` 作为 provider retry root。
- 不再出现 provider/config failure 写业务 rejected result 后 failed。
- running run 不停留在最后 failed step 无后续动作。
- LLM memory context 不包含 provider retry 流水；trace 能解释排除原因。
