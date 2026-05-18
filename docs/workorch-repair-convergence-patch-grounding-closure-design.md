# WorkOrch Repair Convergence and Patch Grounding Closure Design

本文定义 rejected result continuation 生效之后，runtime repair 如何继续保持中立并稳定收敛。它补充 `workorch-runtime-stage-contract-neutrality-design.md` 与 `workorch-runtime-neutral-repair-grounding-design.md`，作为本轮 `ws-86534a43f17f1d23` 核对暴露问题的最终收口清单。

## 背景

`ws-86534a43f17f1d23` 最新运行证明：

- `rejectedResultContinuation` 已正确注入 successor run。
- LLM request 中可以看到 predecessor result、required evidence、satisfied DoD items 与 refs。
- LLM 已开始围绕 `requiredEvidence` 生成 state-changing patch 与验证命令。

但该 run 仍长期 running，出现 90+ steps、30+ repair attempts、0 result / 0 artifact 的情况。主要失败形态包括：

- patch hunk header 缺少行号。
- patch hunk context mismatch。
- state-changing repair 返回 read-only / shell-only graph。
- repair target 被判定不属于当前 failure target scope。
- 当前失败是 patch / test failure，但 `structuredFailureSignals` 仍残留历史 graph contract violation。
- provider 504 与 patch repair 混入同一个 repair loop 观测。

这说明问题已从 “rejected result 无法延续” 转移到 “repair 阶段缺少当前失败优先、当前快照 grounding 和预算终态收敛”。

## 中立约束

Core runtime 只能处理以下中立概念：

- failure class / failure signature / failure target scope。
- current failure / previous failures。
- target refs / current target snapshots / content hash。
- patch capability contract。
- state-changing output contract。
- repair budget / provider retry budget。
- failed assessment / blocker / rejected result。

Core runtime 不得内置：

- 软件开发、Go、测试文件名、API_BASE_URL、多供应商等业务语义。
- 某类业务应该修改哪个文件。
- 某个测试、配置、协议或命令的领域解释。

软件开发相关提示只能放在 workspaceTemplate / soul / profile 中。

## 目标

1. 每次 repair 的 LLM 输入必须以当前失败为主，而不是历史失败。
2. patch apply / edit 失败后，下一轮 repair 必须基于当前 target snapshot 生成可应用修复。
3. 同一 failure scope 多次失败后，runtime 必须切换 repair 模式或终态 rejected / blocker，禁止无限 running。
4. provider retry、LLM schema repair、patch apply repair、verification failure repair 的预算要能区分，不互相污染。
5. read model 与 observability 必须能显示当前失败证据、repair target set、budget 状态和收敛路径。

## Repair Action Package 合同

`repairActionPackage` 是 state-changing repair 的主输入。其结构必须保持 summary/ref-only，但对当前失败足够可操作：

```json
{
  "mode": "state_changing_repair",
  "currentFailure": {
    "failureClass": "patch_apply_failed",
    "failureSignature": "class=patch_apply_failed|...",
    "failedStepId": "run/graph/step",
    "failedStepKind": "patch/apply",
    "targetRef": "target.txt",
    "error": "unified diff hunk context mismatch"
  },
  "currentTargetSnapshots": [
    {
      "targetRef": "target.txt",
      "contentHash": "sha256...",
      "contentComplete": true,
      "safeForFileWrite": true,
      "redacted": false,
      "truncated": false
    }
  ],
  "failedPatchDigest": {
    "patchId": "run/step/patch/1",
    "targetRef": "target.txt",
    "failureReason": "unified_diff_hunk_context_mismatch",
    "stalePatchRejected": true
  },
  "repairTargetSet": ["target.txt"],
  "requiredRepairMode": "patch_current_snapshot_only",
  "forbiddenActions": [
    "read_only_graph_for_state_changing_repair",
    "modify_target_outside_repair_target_set",
    "reuse_stale_patch_hunk"
  ],
  "exitCriteria": [
    "state_changing_artifact_applied_then_verify",
    "failed_assessment_or_blocker_when_not_repairable"
  ]
}
```

字段规则：

- `currentFailure` 必须来自当前 `failedStepId` 的 output / execErr。
- `previousFailures` 只能作为历史摘要，不能覆盖 `currentFailure`。
- `failedPatchDigest` 只记录失败 patch 的摘要和失败原因，不携带完整旧 diff。
- `currentTargetSnapshots` 只允许携带当前目标快照的安全元数据；完整 content 仅在已有 runtime 安全标记允许时进入 LLM 输入。
- `repairTargetSet` 是中立 target ref 集合，不表达业务语义。
- `requiredRepairMode` 与 `forbiddenActions` 是 capability contract，不是业务规则。

## Patch Grounding 策略

当 patch / edit 失败原因属于 context mismatch、base mismatch、oldText not found、invalid hunk header 或 stale artifact：

1. 下一轮 repair 必须携带当前 target snapshot 元数据。
2. `requiredRepairMode` 设置为 `patch_current_snapshot_only`。
3. `forbiddenActions` 必须包含 `reuse_stale_patch_hunk`。
4. 连续 context mismatch 达到阈值后，升级为 `exact_replacement_against_current_snapshot` 或 failed assessment / blocker。
5. 如果当前 snapshot content 不完整、不安全、被截断或被脱敏，必须要求先重新读取目标或返回 blocker，不能伪装成可完整 file_write。

## Failure Signals 当前失败优先

Runtime / read model / repairLoop 必须遵守：

- `lastFailureSignature`、`lastFailureClass`、`structuredFailureSignals` 必须来自同一个 current failed step。
- 历史 `repairOfFailureSignature`、历史 graph contract violation、provider retry 失败只能进入 `previousFailures`。
- 如果当前失败是 `patch_apply_failed`，则 signals 必须表达 patch apply failure，而不能残留旧的 `llm_execution_graph_plan` contract violation。

## Repair Target Set

`repairTargetSet` 来源：

- 当前失败 targetRef / failure target scope。
- 当前 failed patch targetRef。
- 当前 repair target snapshots。
- 当前 run 已成功 applied patch targets。
- LLM 在 repair plan 中声明且 runtime 记录的 related target refs。

约束：

- state-changing repair 默认只能修改 `repairTargetSet`。
- 新增 target 必须通过中立 relation 进入 set，例如 `current_failure_target`、`failed_patch_target`、`related_validation_target`、`supporting_target`。
- Core 只校验 target ref 与 lineage，不判断领域语义。

## Budget 与终态收敛

Runtime repair 需要至少区分以下预算：

- `providerRetryBudget`：provider timeout / 504 / transient transport。
- `llmSchemaRepairBudget`：JSON schema invalid / parse failure。
- `stateChangingContractBudget`：read-only graph、缺 patch/apply 等 contract violation。
- `patchApplyRepairBudget`：patch dispatch/apply/context mismatch。
- `verificationRepairBudget`：shell/test/tool validation failure。

收敛规则：

- provider retry 不得消耗 patch apply repair budget。
- 同一 `failureClass + targetRef + repairMode` 达到预算后不得继续普通 repair。
- 已有 applied state-changing artifact 但验证失败且预算耗尽时，写 rejected result，保留 applied patch refs、failed validation refs、remaining gaps / nextActions。
- 无有效 state-changing artifact 且预算耗尽时，写 rejected result 或 blocker。
- active run 不得在预算耗尽后继续无限 running。

## Software-dev Profile 补充边界

以下仅属于 software-dev template / soul：

- patch context mismatch 后先读取当前文件。
- 编译失败修复必须最小化，不扩大测试文件。
- 同一文件连续 patch 失败时，应输出 failed assessment / blocker。
- 不允许用 read-only graph 响应 state-changing repair。
- 多轮 repair 必须引用当前失败 step，而不是历史 failure summary。

## 收口任务

| ID | 内容 | 范围 | 状态 |
| --- | --- | --- | --- |
| RPC-01 | 输出 repair convergence / patch grounding closure 文档 | docs | completed |
| RPC-02 | 修复 structuredFailureSignals 当前失败优先，避免历史失败串线 | orchestrator/read-model | completed |
| RPC-03 | 扩展 repairActionPackage.currentFailure / failedPatchDigest / requiredRepairMode / forbiddenActions | orchestrator | completed |
| RPC-04 | patch context mismatch 后强制 current snapshot grounding 与 stale hunk 禁止 | orchestrator | completed |
| RPC-05 | 引入 repairTargetSet 并用于 state-changing target relevance | orchestrator | completed |
| RPC-06 | 拆分 provider/schema/contract/patch/verification repair budget 观测与收敛 | orchestrator/read-model | completed |
| RPC-07 | 预算耗尽后写 rejected result / blocker，禁止无限 running | orchestrator | completed |
| RPC-08 | software-dev soul 补充 patch repair 收敛提示 | template/soul | completed |
| RPC-09 | 增加单测覆盖 failure signals、patch grounding、budget terminal closure | tests | completed |

## 实现收口记录

- RPC-02：`repairLoop.structuredFailureSignals` 先从 `lastFailureStepId` 对应 step output 投影；只有当前失败无 signals 时才回退到 repair input / repairActionPackage，避免历史 graph contract violation 覆盖当前 patch failure。
- RPC-03：`repairActionPackage` 增加 `currentFailure`、`failedPatchDigest`、`requiredRepairMode`、`forbiddenActions`、`repairBudget` 与 `exitCriteria`，且仅保存当前失败摘要和中立 target refs。
- RPC-04：patch context mismatch、oldText not found、malformed hunk、stale patch 等失败会设置 `patch_current_snapshot_only`；同 target 多次失败升级为 `exact_replacement_against_current_snapshot`，并禁止复用旧 hunk。
- RPC-05：`repairTargetSet` 从当前 failure target scope、failed patch target、current target snapshots、patch suggestions 与 structured failure targetRefs 中抽取，core 只处理 target ref，不解释业务语义。
- RPC-06：`repairActionPackage.repairBudget.budgetKind` 区分 provider retry、LLM schema repair、state-changing contract repair、patch apply repair 与 verification repair，便于 read-model 与后续治理观测。
- RPC-07：`patch_apply_failed + targetRef` 使用 run 级 target budget，不因 graph root / loop / node 变化重置预算；达到 `maxRuntimeRepairAttempts` 后走既有 repair exhaustion rejected result / decision blocker 收敛。
- RPC-08：software-dev execution soul 补充 profile 侧 patch repair 规则，要求以 currentFailure 为准，基于 currentTargetSnapshots/file/read 重建补丁，并在快照不安全时输出 failed assessment / blocker。
- RPC-09：新增/扩展单测覆盖当前失败 signals 优先、patch failure action package、patch target budget 跨 graph 收敛、software-dev soul 规则。

## 验收标准

- `structuredFailureSignals` 与 `lastFailureSignature` 始终来自当前失败 step。
- patch context mismatch 之后的 repair input 包含 current target snapshot / failedPatchDigest / requiredRepairMode。
- 同一 target 的 stale hunk 不会无限重复。
- state-changing repair 返回 read-only graph 时能被预算化收敛，而不是无限重试。
- provider 504 不污染 patch apply repair budget。
- 达到预算后 run 进入 rejected / blocker / failed，而不是继续 running。
- Core 代码不出现软件开发、Go、API_BASE_URL、具体测试文件名等业务硬编码。
