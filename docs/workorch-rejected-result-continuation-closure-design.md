# WorkOrch Rejected Result Continuation Closure Design

本文定义 rejected result 之后如何继续补齐 Goal / DoD / coverage 缺口。它是 `workorch-runtime-stage-contract-neutrality-design.md` 的补充设计，所有实现必须保持 WorkOrch core 功能中立。

## 背景

`ws-86534a43f17f1d23` 的最新执行已经证明 read-model 投影可以正确显示终态 rejected result：

- `snapshot.result` 指向 `run-a9bc75fdf723808c/result/dod-assessment`。
- `taskProgress.currentStepKind` 指向 `result/dod_assessment`。
- `compressedContext.resultRefs` 包含 rejected result 与关键 evidence refs。

但运行本身仍失败，因为 DoD 项虽然 satisfied，Goal 级 coverage 与 required deliverable coverage 仍不完整。当前问题已经不是“终态看不清”，而是“rejected result 没有稳定变成下一轮补齐输入”。

## 中立约束

Core runtime 只能处理以下中立概念：

- rejected / accepted result。
- DoD assessment。
- goalCoverage / acceptanceDimensions / deliverableCoverage。
- missing coverage items。
- evidenceRefs / artifactRefs。
- nextActions / residualRisks / knownLimitations。
- predecessor / successor run lineage。

Core runtime 不得内置以下领域内容：

- 软件开发、代码、测试、构建、具体文件、具体配置名。
- 多供应商、API_BASE_URL、二进制构建等业务语义。
- 某类任务应该如何实现或验证。

这些领域约束只能由 workspaceTemplate、profile、soul、skill 或用户 Goal / DoD 提供。

## 目标

1. 新 run 启动时，若同一 workspace / Goal / DoD 存在最新 rejected result，应自动携带一个中立 `rejectedResultContinuation`。
2. continuation 必须足够结构化，让 LLM 明确知道上一轮哪些项已满足、哪些 coverage 仍缺、哪些 evidence 可以复用。
3. continuation 只能携带 summary/ref-only 信息，不得复制 raw LLM stream、完整 execution graph、provider 动态绑定或大块中间 JSON。
4. read-model 和 context bundle 必须能稳定显示 terminal result 与 continuation lineage。

## Continuation 输入合同

新 run 的 `executionContext.rejectedResultContinuation` 结构：

```json
{
  "mode": "rejected_result_continuation",
  "predecessorRunId": "run-prev",
  "resultId": "run-prev/result/dod-assessment",
  "resultState": "rejected",
  "sourceType": "dod_assessment",
  "sourceRef": "run-prev/assessment-step",
  "summary": "rejected result summary",
  "goalOutcome": "result summary detail goalOutcome",
  "satisfiedDoDItems": [
    {
      "item": "DoD item text",
      "evidenceRefs": ["run-prev/validation-step"]
    }
  ],
  "unsatisfiedDoDItems": [
    {
      "item": "DoD item text",
      "state": "failed",
      "reason": "neutral failure reason",
      "evidenceRefs": ["run-prev/evidence"]
    }
  ],
  "coverageGaps": {
    "missingGoalCoverage": [],
    "missingAcceptanceDimensions": [],
    "missingDeliverableCoverage": []
  },
  "requiredEvidence": [
    "neutral next action or residual risk text"
  ],
  "resultEvidenceRefs": ["run-prev/evidence"],
  "artifactRefs": ["artifact-ref"]
}
```

字段规则：

- `mode` 固定为 `rejected_result_continuation`。
- `predecessorRunId`、`resultId`、`sourceRef` 必须是可追踪引用。
- `satisfiedDoDItems` 只保留 item / evidenceRefs，不复制冗长 explanation。
- `unsatisfiedDoDItems` 可保留 state / reason / evidenceRefs。
- `coverageGaps` 从 `DoDAssessment` 和 `SummaryDetail` 中抽取结构化缺口；没有结构化项时保留空数组，不从自然语言臆造业务项。
- `requiredEvidence` 只能来自 nextActions / residualRisks / knownLimitations 等已有 result 字段，作为提示，不作为 core 判定规则。
- 所有 refs 必须压缩去重。

## Coverage 缺口抽取规则

Core 可以按中立结构抽取：

- `dodAssessment.items[]` 中非 satisfied 项 -> `unsatisfiedDoDItems`。
- `dodAssessment.goalCoverage.items[]` 中非 satisfied 或缺 evidenceRefs 项 -> `missingGoalCoverage`。
- `dodAssessment.acceptanceDimensions.items[]` 中非 satisfied 或缺 evidenceRefs 项 -> `missingAcceptanceDimensions`。
- `dodAssessment.deliverableCoverage.items[]` 中非 satisfied 且缺 evidenceRefs / artifactRefs 项 -> `missingDeliverableCoverage`。
- `summaryDetail.nextActions` / `residualRisks` / `knownLimitations` -> `requiredEvidence`。

Core 不得根据文本关键词推断缺口类型。若 result 只有自然语言 summary，没有结构化 coverage，core 只能传递 summary / nextActions / evidenceRefs。

## 下一轮运行策略

当 workspace 无 active run 且存在 latest rejected result：

1. 用户或服务触发 workspace execution activation。
2. activation 选择当前有效 Goal / DoD。
3. 若 latest rejected result 属于相同 workspace / effective Goal / DoD，build executionContext 时注入 `rejectedResultContinuation`。
4. 新 run 仍按常规 runtime stage 运行；core 只提供 continuation 输入，不强制具体业务实现方案。

## Software-dev Profile 补强边界

软件开发相关补齐仍由 template / soul 表达：

- 要求输出完整 `coverage_map`。
- 要求 Goal leaf / DoD / acceptanceDimensions / deliverableExpectations 到 evidenceRefs 的映射。
- 要求把用户 Goal 中的具体接口、配置、构建、验证要求映射到 evidence。

这些要求不得下沉到 core。

## 收口任务

| ID | 内容 | 范围 | 状态 |
| --- | --- | --- | --- |
| RRC-01 | 输出 rejected result continuation 设计文档 | docs | completed |
| RRC-02 | activation 自动注入 latest rejected result continuation | appservice | completed |
| RRC-03 | 结构化抽取 satisfied/unsatisfied DoD 与 coverage gaps | appservice | completed |
| RRC-04 | continuation 输入去重、脱敏、限制字段大小 | appservice | completed |
| RRC-05 | 软件开发 template / soul 强化 coverage_map 约束 | template/soul | completed |
| RRC-06 | 增加单测覆盖终态 rejected 后 successor activation | tests | completed |

## 实现收口记录

- `Result.DoDAssessment` 已补齐中立 coverage 字段：`goalCoverage`、`acceptanceDimensions`、`deliverableCoverage`。
- activation 构建执行上下文时，会选择同一 workspace / effective Goal / effective DoD 下最新终态 result；只有最新终态 result 为 `rejected` 时才注入 `rejectedResultContinuation`，避免历史 rejected 覆盖后续 accepted。
- continuation 会压缩去重 refs，并经过敏感信息脱敏和 provider/model 动态绑定剔除。
- execution context package 与 runtime step input 已携带压缩后的 `rejectedResultContinuation`，确保 LLM 能在下一轮看到结构化缺口。
- software-dev planning / validation soul 已补充提示：存在 `rejectedResultContinuation` 时，优先围绕 `missingGoalCoverage`、`missingAcceptanceDimensions`、`missingDeliverableCoverage` 与 `requiredEvidence` 做补齐，而不是重新泛化为“已完成”。

## 验收标准

- 对终态 failed workspace 再次 activation 时，新 run 的 `executionContext.rejectedResultContinuation` 存在。
- continuation 指向 latest rejected result，而不是历史 failed run。
- continuation 不包含 provider/model 动态绑定、raw stream、完整 graph 或大块中间输入。
- DoD satisfied 项和 failed/skipped 项被结构化传递。
- coverage gaps 只来自结构化 coverage 字段，不从业务文本硬编码推断。
- `go test ./internal/modules/application/workorchd/biz/appservice -count=1` 通过。
