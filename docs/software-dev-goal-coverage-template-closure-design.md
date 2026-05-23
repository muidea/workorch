# Software Dev Goal Coverage and Template Closure Design

本文定义基于 `ws-2778196baf7315f3` 核对结果形成的 WorkOrch core runtime 与 `software-dev` workspaceTemplate 收口方案。本文不是替代 `workorch-runtime-stage-contract-neutrality-design.md`，而是其在软件开发 profile 上的实施设计和 checklist。

## 权威边界

- `docs/workorch-runtime-stage-contract-neutrality-design.md` 仍是 WorkOrch runtime 后续开发的最高设计合约。
- Core runtime 必须保持功能中立，只治理 stage、contract、artifact、evidence、result、repair、replan、memory 和 capability 边界。
- `software-dev` 只能作为 profile / template / Soul / evidence policy 表达软件开发任务的通用约束，不得把某个具体业务应用的协议、配置 schema、默认路由或技术方案写成模板默认行为。
- 具体 workspace 的业务目标，例如 LLM gateway、多供应商协议、Token 审计等，只能来自该 workspace 的 Goal / DoD / Memory / 用户输入，不得写入 core runtime 或通用 `software-dev` template。

## 背景问题

`ws-2778196baf7315f3` 已能完成 run，并通过自身验证命令，但生成代码与 Goal 的关键能力仍存在偏差：

- 标准 `cmd/gateway` 入口损坏，只有 `package main`，无法独立构建。
- `Provider.Type` 未真正驱动多协议转换，实际只按 OpenAI-compatible 转发。
- `base_url` 拼接存在重复 `/v1` 风险。
- 流式响应无 usage 时 fallback 仍记录 `0,0`，与 DoD 的估算要求不匹配。
- 测试通过但没有覆盖入口完整性、协议差异、base URL 归一化、流式无 usage fallback、错误路径和并发请求路径。

这些问题说明：当前 runtime 与模板已经能推动“产生代码并跑通命令”，但还不能稳定保证“Goal 叶子能力均被实现并由有效 evidence 覆盖”。

## 根因判断

### 不是 core 应理解具体业务

Core runtime 不应判断：

- 什么是 LLM gateway。
- 哪些 provider 协议需要转换。
- `config.yaml` 应该有什么字段。
- Go 项目的入口应该放在哪里。
- 某个测试是否覆盖 OpenAI / Anthropic / DeepSeek 语义。

这些都属于 workspace Goal / DoD 或 profile / Soul 的输入范围。

### Core 需要补强的中立能力

Core 可以并且需要治理：

- Goal leaf requirement 是否进入 acceptance coverage。
- 每个 satisfied coverage item 是否有 evidenceRefs。
- evidence 是否来自当前 run 的有效 step / artifact / result。
- accepted result 是否缺少必要 coverage map。
- profile 声明的 acceptance dimensions 是否都有 evidence 或明确 failed / skipped reason。
- deliverable contract 是否与实际 artifact / validation evidence 一致。
- weak / partial evidence 是否被错误提升为 satisfied。

### Software-dev template 需要补强的 profile 约束

`software-dev` template 应提供通用软件开发验收框架：

- 从 Goal / DoD / 需求文档提取功能目标、运行入口、外部接口、配置、错误路径、持久化、副作用、并发和交付形态。
- 要求每类目标映射到实现文件、测试或替代验证、验证命令和 evidenceRefs。
- 对“命令通过但目标能力未覆盖”的情况要求 validation 返回 failed / skipped assessment，并给出修复项。
- 移除任何具体应用的默认 schema、默认路由、默认协议实现建议。

## Core Runtime 改进设计

### 1. Goal Leaf Coverage Contract

Core 应把 Goal coverage 视为 acceptance 的一等合同，而不是 LLM 自述字段。

要求：

- Goal 文本解析为中立 leaf requirement 列表。
- Markdown 标题、分组项、仅以冒号结尾的父级描述只作为组织上下文，不作为必须单独满足的 leaf。
- Acceptance result 必须包含每个 leaf 的 coverage state。
- `satisfied` leaf 必须引用 evidenceRefs。
- `failed` / `skipped` leaf 必须包含 reason、impact 和 nextActions。
- 缺 coverage、缺 evidenceRefs 或引用无效 evidence 时，不得 accepted。

Core 不做业务语义判断，只校验结构完整性和引用有效性。

### 2. Acceptance Dimension Contract

Profile 可以声明 acceptance dimensions，Core 只校验 profile 声明是否被覆盖。

中立字段建议：

```json
{
  "acceptanceDimensions": [
    {
      "id": "external_contract",
      "required": true,
      "description": "External behavior promised by Goal or DoD"
    },
    {
      "id": "runtime_entrypoint",
      "required": true,
      "description": "How the deliverable is started or invoked"
    }
  ]
}
```

Core 规则：

- Profile 声明的 required dimension 必须在 `dodAssessment` 或 `goalCoverage` 中有 evidence / failed / skipped 结论。
- Core 不理解 dimension 的业务含义。
- Dimension 的具体解释由 profile / Soul / template prompt 提供。

### 3. Deliverable Consistency Contract

Acceptance 需要补一个中立的一致性检查：

- declared deliverables：LLM 声明的交付物、入口、配置、运行方式。
- produced artifacts：实际 state-changing artifacts、files、patch、build output。
- validation evidence：验证命令和结果。

Core 校验：

- final result 中声明的交付物必须能引用到 artifact 或 evidence。
- 如果存在多个候选入口或交付路径，profile 可以要求全部覆盖或说明 skipped reason。
- 如果验证命令只覆盖部分交付路径，不得把未覆盖路径标记为 satisfied。

Core 不判断入口命名是否合理，只校验声明、产物和证据之间的引用关系。

### 4. Evidence Quality Contract

现有 zero-work evidence policy 需要扩展为通用 evidence quality contract。

Profile 可以声明：

- weak evidence marker。
- work evidence marker，用于聚合命令中抵消局部 weak / zero-work marker。
- minimum assertion expectation。
- required evidence kinds。
- required branch categories。

Core 只执行：

- 标记 weak / inconclusive evidence。
- 当同一输出同时命中 work evidence marker 和 zero-work marker 时，优先保留有效工作证据，不把整条聚合命令降级为 zero-work。
- 阻止 weak evidence 自动支撑 satisfied。
- 把 quality gap 转成结构化 repair diagnostic。

### 5. Profile Namespaced Contract

所有 profile-specific 字段必须放在 profile namespace：

```json
{
  "profileContracts": {
    "software-dev": {
      "acceptanceDimensions": [],
      "evidenceQualityRules": [],
      "deliverableExpectations": [],
      "evidenceRequirements": []
    }
  }
}
```

禁止：

- 在 core runtime 顶层新增 `softwareDev`、`testFiles`、`goBuild`、`patchRequired` 等领域字段。
- 在 core runtime 中通过文件名、命令名或业务关键词推断软件开发语义。

`evidenceRequirements` 是 profile 命名空间内的中立证据约束。Core 只读取 `scope`、`id`、`acceptedStepKinds`、`rejectedStepKinds` 并按 step kind 校验，不理解该 `id` 的业务含义。`software-dev` 可以声明：当 `persistence_or_side_effect`、`streaming_or_long_running`、`error_behavior`、`concurrency` 等验收维度被标记 satisfied 时，不能只引用 `file/read` / `file/list` / `mcp/resource_read` / `mcp/prompt_get` 这类只读证据，必须同时具备 `shell/command`、`patch/apply` 或 `mcp/tool_call` 等动作证据。只读证据可以作为辅助上下文引用，但不能单独支撑动作类声明。

## Software-dev Template 改进设计

### 1. 移除具体业务默认实现

当前 `soul-software-dev-execution` 中存在具体 LLM gateway 配置建议，例如多 provider 默认使用某组字段。这类内容应移除。

保留原则：

- 可以要求 LLM 根据 Goal / DoD 自主选择合理配置 schema。
- 可以要求配置字段、默认策略和错误行为必须有测试或说明。
- 不可以指定某个具体业务应用的默认字段集合。

### 2. Goal Decomposition Checklist

Planning Soul 应要求把软件开发 Goal 拆成以下通用维度：

| Dimension | 说明 |
| --- | --- |
| `functional_behavior` | 目标功能行为和主路径 |
| `external_contract` | API、CLI、文件格式、协议、输入输出兼容性 |
| `runtime_entrypoint` | 启动方式、命令入口、服务入口或调用入口 |
| `configuration` | 配置来源、默认值、环境变量、错误处理 |
| `persistence_or_side_effect` | 文件写入、状态变更、审计记录或外部副作用 |
| `error_behavior` | 错误透传、错误码、异常输入、依赖失败 |
| `streaming_or_long_running` | 流式、长耗时、增量输出或异步行为 |
| `concurrency` | 并发调用、共享状态、资源锁或竞态风险 |
| `deliverable` | 构建产物、发布文件、单二进制或文档交付 |

这些维度不是每个 Goal 都必须满足；但如果 Goal / DoD 涉及，就必须进入 plan 和 validation coverage。

### 3. DoD-to-Test Mapping 强化

Planning / execution / validation Soul 都应要求：

- 每个关键 DoD 项映射到至少一个 verification command 或替代 evidence。
- 对每个 Goal leaf requirement 标注覆盖方式。
- 明确哪些测试覆盖主路径、错误路径、配置路径、持久化路径、并发路径。
- 如果只能 smoke test，不能用它支撑复杂能力 satisfied。

### 4. Runtime Entrypoint Integrity

Software-dev profile 应新增通用约束：

- 如果项目存在显式入口目录、脚本、命令或构建目标，必须验证目标入口可用。
- 如果 Goal 声明单二进制、CLI、服务或 API 交付，必须验证对应入口构建或启动路径。
- 如果存在多个入口但只有一个是目标入口，必须说明选择依据。

这不是 Go 特判；Go 的 `cmd/gateway` 只是本次 workspace 暴露出的样例。

### 5. External Contract and Branch Coverage

Validation Soul 应补充：

- 外部接口必须覆盖成功路径和失败路径。
- 若 Goal 涉及协议兼容或格式转换，必须验证输入输出格式、关键字段和错误透传。
- 若 Goal 涉及流式/增量输出，必须验证正常结束、异常结束和统计/收尾逻辑。
- 若 Goal 涉及持久化或审计，必须验证记录字段、追加行为和并发安全。

这些要求仍是 profile 层，不进入 core runtime。

### 6. Artifact Policy

Software-dev artifact Soul 应明确：

- 不交付构建产物二进制，除非 Goal 明确要求并且 runtime artifact policy 允许。
- 最终结果必须引用 source diff、validation evidence、coverage map。
- 若 workspace 中生成了二进制、日志或临时文件，应在 artifact 中说明是否属于交付范围。

## Implementation Checklist

### Phase 1: 文档与模板边界

| ID | 内容 | 状态 |
| --- | --- | --- |
| SDC-01 | 新增本文作为 software-dev goal coverage 收口设计 | Done |
| SDC-02 | 从 `software-dev` Soul 移除具体 LLM gateway 默认配置建议 | Done |
| SDC-03 | 补充 `software-dev` planning / validation / artifact Soul 的通用验收维度 | Done |
| SDC-04 | 更新 built-in template / soul 测试，防止具体业务建议再次进入通用模板 | Done |

### Phase 2: Core 中立合同

| ID | 内容 | 状态 |
| --- | --- | --- |
| SDC-05 | 增加 Goal leaf coverage 结构校验 | Done |
| SDC-06 | 增加 profile acceptance dimensions 的声明与校验入口 | Done |
| SDC-07 | 增加 deliverable consistency 的中立引用校验 | Done |
| SDC-08 | 扩展 evidence quality diagnostic，支持 weak / partial / missing branch coverage | Done |
| SDC-09 | 确保所有 profile-specific contract 位于 profile namespace | Done |

### Phase 3: Runtime / Template 验证

| ID | 内容 | 状态 |
| --- | --- | --- |
| SDC-10 | 增加模板单元测试，校验 software-dev Soul 不包含具体业务默认实现 | Done |
| SDC-11 | 增加 acceptance contract fixture，覆盖缺 goalCoverage、无 evidenceRefs、weak evidence satisfied | Done |
| SDC-12 | 增加 deployed verification 脚本检查 accepted result 的 Goal leaf coverage 与 evidenceRefs | Done |
| SDC-13 | 使用新的 workspace run 重新验证软件开发任务是否能发现入口/协议/分支覆盖缺口 | Pending |
| SDC-14 | 增加 profile evidenceRequirements，阻止只读证据单独支撑副作用、流式、错误路径和并发声明 | Done |
| SDC-15 | 强化 software-dev Soul 的 claim-to-evidence consistency、多目标矩阵、字段级副作用断言要求 | Done |

## Done Definition

本轮收口完成标准：

- `software-dev` built-in Soul 不再包含具体应用业务默认实现。
- 软件开发模板能要求 LLM 从 Goal / DoD 提取关键验收维度，并映射到 plan、execution、validation、artifact。
- Core acceptance 不允许在 Goal coverage 缺失、evidenceRefs 缺失或 weak evidence 未解释时 accepted。
- Profile evidenceRequirements 能拦截只读证据支撑动作类声明，且规则位于 profile namespace。
- Runtime 改动不包含软件开发、Go、测试框架、LLM gateway、provider 协议等硬编码。
- `go test ./... -count=1` 通过。
- 至少一个 fixture 或部署核对能复现并拦截“命令通过但 Goal 关键能力未覆盖”的情况。
