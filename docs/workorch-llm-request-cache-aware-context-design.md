# WorkOrch LLM Request Cache-Aware Context Design

## 背景

WorkOrch 已经通过 `workorch-runtime-stage-contract-neutrality-design.md` 约束了 LLM 上下文完整性，通过 `codex-like-llm-prompt-context-design.md` 约束了多消息组织方向，通过 `model-aware-llm-context-budget-design.md` 约束了模型感知预算。

当前仍然需要补齐一类独立问题：如何组织提交给 LLM 的对话信息，使 provider 侧 prompt / context cache 更容易复用。

这个问题不是单纯减少上下文大小。若每轮请求都把稳定合同、Goal / DoD、运行进展、失败证据、memory recall、临时 ID、时间戳和动态 provider 信息混成一个大 JSON，即使实际变化只发生在尾部，provider 也很难复用前缀缓存。最终表现为：

- 同一 workspace 多轮 LLM 调用成本偏高。
- 长流程 repair / validation 每轮重新处理大量稳定合同。
- 高频变化字段靠前出现，导致稳定内容后移并破坏前缀一致性。
- 大 JSON key 顺序、空字段、source refs 排序不稳定，造成语义相同但文本不同。
- memory / progress 噪点每轮变化，放大缓存失效范围。

本设计目标是在不牺牲上下文完整性、不引入业务绑定、不绑定具体 provider cache API 的前提下，形成一套 provider-neutral 的 LLM request 组织合同。

## 实测样本对比

本设计已结合以下运行态样本校准：

- WorkOrch workspace：`.workorch-data/internal/workspaces/ws-51c639c68a58324a`
- Codex 请求日志：`/home/rangh/aispace/ai-proxy/logs/codex`

### WorkOrch 当前请求形态

`ws-51c639c68a58324a` 的 `run-003` 已经记录了真实 LLM request：

- `runs/run-003/llm-calls/run-003/001-step-1/request.json`
- `runs/run-003/llm-calls/run-003/002-repair-001-of-planning-001/request.json`

实际结构特征：

- 顶层是 WorkOrch 自定义对象：`kind`、`model`、`workspaceId`、`runId`、`stepId`、`input`、`promptContext`、`runtimeCapabilities`、`contextBudget`、`traceRefs`。
- 没有标准 `messages` 数组；真正的多轮材料位于 `promptContext.turns` 和 `promptContext.sections`。
- `promptContext.turns` 已经具备 role 顺序，但 section 仍以 `turn-01`、`turn-02` 这类顺序名表达，缺少 C0-C5 语义分区、cacheability、section hash 和 cache breaker。
- `contextBudget` 已包含预算、redaction、protectedFields、contextGateAction 等完整性信息，但缺少 `cachePlan`、`stablePrefixHash`、`workspaceDefinitionHash`、`capabilityContractHash`、`stageBundleHash`、`volatileTailHash`。
- `definitionChangeImpact` 已存在于 `input.executionContextPackage`，说明运行期 Goal / DoD 变化已经进入上下文，但该信息尚未成为 C1 definition revision/hash 的稳定边界。
- repair 请求中存在非常大的易变 user turn，例如 `Execution Context and Evidence Signals` 超过 140KB，说明当前仍有大量执行上下文被混合打包进单个尾部 turn，难以判断哪些内容真正是当前决策必须信息。

当前 `run-003` 两个请求的典型体量：

| Request | actualBytes | turns | 大体量 turn |
| --- | ---: | ---: | --- |
| `001-step-1` | 117552 | 11 | `Execution Context and Evidence Signals` 约 90KB |
| `002-repair-001-of-planning-001` | 180318 | 12 | `Execution Context and Evidence Signals` 约 142KB |

结论：

- WorkOrch 已经有多 turn 组织雏形，不需要回退为单 message。
- 当前缺口是 turn 只有顺序，没有稳定语义分区；request audit 只能说明预算和完整性，不能说明缓存复用边界。
- 后续实现应优先把 `promptContext.sections` 从 `turn-xx` 升级为 C0-C5 section，并保留当前 human-readable turn 顺序作为渲染结果。

### Codex 当前请求形态

Codex 日志样本显示其请求使用 `/v1/responses` 风格结构，典型字段包括：

- `instructions`
- `tools`
- `input`
- `prompt_cache_key`
- `reasoning`
- `text`
- `tool_choice`
- `parallel_tool_calls`
- `store`

实际结构特征：

- `instructions` 约 21KB，作为稳定开发者/系统约束长期放在前部。
- `tools` 作为稳定工具能力定义单独提交。
- `prompt_cache_key` 显式存在，用于提示 provider/cache 层复用语义。
- `input` 是事件数组，包含 message、function_call、function_call_output、custom_tool_call、custom_tool_call_output、reasoning 等历史事件。
- 事件序列保留顺序，最新用户请求位于尾部，不会把所有历史重新压平成一个无差别大 JSON。
- 工具调用和工具输出以 typed event 表达，天然比自由文本堆叠更容易做截断、摘要和引用。

WorkOrch 可借鉴的不是 Codex 私有 schema，而是以下中立机制：

- 稳定 instructions / runtime contract 与 volatile input event tape 分离。
- 工具能力定义与当前请求状态分离。
- 使用 cache key / cache plan 描述缓存复用边界。
- 历史过程按 typed event tape 保序，而不是合并成一个巨大状态快照。
- 最新用户意图、最新失败、最新工具输出位于尾部。
- 工具输出可按事件级别独立摘要、裁剪和引用。

WorkOrch 不应借鉴的内容：

- 不依赖 Codex 私有 `reasoning` 作为可恢复状态。
- 不复制 Codex provider 私有 request schema 到 core runtime。
- 不把软件开发专用提示词放入 WorkOrch core。
- 不把 provider 私有 `prompt_cache_key` 作为业务状态；core 只能生成中立 `cachePlan`，由 adapter 选择是否映射。

## 设计目标

- 提升 LLM 请求稳定前缀的复用率。
- 将稳定合同、半稳定定义、易变执行状态明确分区。
- 保持 Goal / DoD 完整进入上下文，不为缓存复用做语义压缩。
- 减少随机 ID、时间戳、动态 provider 信息、噪点 memory 对稳定前缀的污染。
- 通过 canonical section ordering 和 canonical JSON / text rendering 降低无意义差异。
- 在 request audit 中记录 cacheability 信息，使缓存失效原因可观测。
- 保持 WorkOrch core runtime 功能中立，只定义抽象上下文组织规则。

## 非目标

- 不在 core runtime 中绑定某个 provider 的 cache-control 私有字段。
- 不为了缓存复用裁剪 P0-P4 必要上下文。
- 不把 provider / model / baseURL 等运行期动态绑定作为可恢复持久状态。
- 不把软件开发、测试、代码 patch 等具体业务规则写入 core prompt builder。
- 不要求所有 provider 都真实支持 prompt cache；本设计先保证请求结构具备缓存友好性。

## 核心原则

### 完整性优先于缓存

缓存优化不能覆盖 `LLM 上下文完整性合同`。

若 P0-P4 必要字段缺失、Goal / DoD 被压缩、current failure 缺失、target snapshot 缺失或 coverage target 缺失，runtime 必须按 context gate 处理，不得因为稳定前缀复用而提交不完整上下文。

### 稳定信息靠前，易变信息靠后

Provider 侧缓存通常更依赖请求前缀稳定性。因此 request message 必须按 cacheability 从高到低组织：

1. Stable runtime prefix。
2. Semi-stable workspace definition。
3. Stable capability contract。
4. Current run volatile tail。
5. Large refs / trace refs。

越靠前的 section 越不能包含时间戳、随机 ID、临时路径、provider 动态绑定或最新执行噪点。

### Core 只定义分区，不定义业务

Core runtime 可以定义：

- section 名称。
- section 顺序。
- source ref 规则。
- canonical rendering 规则。
- request audit 字段。
- cacheability 等级。

Core runtime 不可以定义：

- 软件开发专用 prompt。
- 测试优先级。
- 代码文件分类。
- 具体工具使用策略。
- 具体 provider cache API 绑定。

业务或 profile 相关内容只能以 profile namespace、workspaceTemplate、soul、skill、tool capability summary 的形式进入对应 section。

## Request 分区模型

### C0 Stable Runtime Prefix

C0 是最希望被 provider 复用的稳定前缀。

内容：

- WorkOrch runtime 中立合同。
- LLM 输出 JSON schema。
- stage transition / blocked / repair / validation 通用规则。
- Goal / DoD 保护规则。
- context gate 规则。
- response language / format rule。
- capability usage 的中立规则。

约束：

- 不得包含 workspaceId / runId / stepId。
- 不得包含 providerId / model / baseURL。
- 不得包含当前时间。
- 不得包含当前 workspace 的状态。
- 不得包含 memory recall。
- 不得包含当前错误。
- 只能随 runtime contract version / schema version 变化。

推荐 message：

```text
system:
  WorkOrch stable runtime contract.

developer:
  Stable context usage rules, neutrality constraints, output schema.
```

### C1 Semi-Stable Workspace Definition

C1 是 workspace 级定义块。它不像 C0 那样全局稳定，但在同一 workspace 的多轮执行中应尽量稳定。

这里的“稳定”只能理解为“同一 definition revision 内稳定”。Workspace 的 Goal 和 DoD 允许在运行期通过 Web、TUI、ticket、approval 或其他治理入口继续补充、澄清和修订。任何 Goal / DoD 变更都必须形成新的 definition revision，并使后续 LLM request 显式使用最新 revision；runtime 不得继续复用旧 definition 作为当前验收基准。

内容：

- workspace identity。
- workspace type / profile identity。
- Goal entries 原文。
- DoD entries 原文。
- Goal / DoD change events。
- effective Goal View。
- effective DoD View。
- definition revision。
- definition hash。
- definition changed reason。
- previous definition hash。

约束：

- Goal / DoD 原文必须完整保留。
- Goal / DoD 可生成 normalized view，但不能替代 original text。
- Goal / DoD 新增或修改时，只应导致 C1 hash 变化，不应导致 C0 变化。
- Goal / DoD 运行期补充后，C1 必须按最新 revision 重建；C3 必须引用最新 definition revision；C4 必须说明本轮是否因 definition change 触发重新规划、重新执行或重新验收。
- 旧 revision 只能作为 lineage / historical evidence 进入 C5，不得作为当前目标或验收基准进入 C3 / C4。
- createdAt、source channel、ticket refs 可以保留，但排序必须稳定。
- 若时间戳不是 LLM 当前决策必须信息，应进入 metadata 子字段，不影响正文顺序。

推荐 message：

```text
user:
  Workspace definition block.
```

### C2 Stable Capability Contract

C2 描述当前 workspace 可用能力和边界，通常随配置、profile、workspaceTemplate 或 capability registry 变化。

内容：

- skill / tool / mcp / soul / prompt 的能力摘要。
- capability input contract。
- unsupported operations。
- required fields。
- allowed artifact kinds。
- profile-specific constraints under profile namespace。

约束：

- 只描述能力边界，不注入领域硬编码。
- 能力列表按 stable id 排序。
- capability summary 使用稳定版本或 hash。
- endpoint / command / args 等连接信息属于运行配置，可进入能力 metadata，但不得进入 C0。
- 若能力不可用，应明确 unavailable reason，避免 LLM 假设能力存在。

推荐 message：

```text
user:
  Capability contract block.
```

### C3 Current Stage Bundle

C3 是当前 stage 的输入合同，变化频率中等。它连接 C1 / C2 与当前执行动作。

内容：

- runtimeStage。
- runtimeStageMode。
- current unit / step target。
- requiredOutput。
- allowedOutput / forbiddenOutput。
- stateChangePolicy。
- current coverage baseline。
- current execution contract。

约束：

- 必须引用 C1 的 definition hash。
- 必须引用 C2 的 capability hash。
- 不重复展开 C0 的稳定合同。
- 当前 stage 变化时 C3 可以变化，但 section 顺序和字段顺序必须稳定。

推荐 message：

```text
user:
  Current stage bundle.
```

### C4 Volatile Execution Tail

C4 是最易变区域，只应放当前轮真正需要 LLM 消化的最新运行事实。

内容：

- run id / step id / loop / attempt。
- currentProgressDigest。
- processEvidenceTimeline tail。
- latest tool result summary。
- latest validation result。
- current failure。
- repair objective。
- current target snapshots / refs。
- unresolved ticket / approval。
- latest memory recall。
- current requested action。

约束：

- C4 必须在所有稳定 section 之后。
- processEvidenceTimeline 按 monotonic sequence 排序。
- memory recall 只进入当前决策有用的信息。
- 大 stdout / stderr / artifact 使用摘要 + source refs。
- 不把完整旧 provider request、raw stream delta、provider retry 流水放入 C4。
- 当 C4 体量持续膨胀时，必须进一步拆成 typed event tape：current failure、latest validation、latest tool outputs、process timeline tail、current target snapshots、memory recall、requested action。不得继续把所有执行证据混入一个无语义标签的大 user turn。

推荐 message：

```text
user:
  Current volatile execution tail.

user:
  Current requested action.
```

### C5 Large Refs And Trace

C5 不直接展开大文本，只提供引用和摘要，用于保证可追溯。

内容：

- artifact refs。
- payload refs。
- omitted section refs。
- compacted history refs。
- contentHash。
- redaction reason。
- omitted reason。
- trace policy。

约束：

- C5 不应出现在 C0-C2 之前。
- C5 不能替代 P0-P4 必要上下文。
- C5 中的 refs 必须稳定排序。

## 推荐 Message Layout

第一阶段仍可继续使用 chat-completions 风格 messages，但必须拆分 message，而不是把所有内容塞入一个大 user message。

推荐顺序：

```text
system:
  C0 stable runtime contract.

developer:
  C0 stable context usage and output schema.

user:
  C1 workspace definition block.

user:
  C2 capability contract block.

user:
  C3 current stage bundle.

user:
  C4 current volatile execution tail.

user:
  C5 large refs and trace.

user:
  Current requested action.
```

允许 provider adapter 在不改变语义的前提下把上述 messages 映射到 provider 支持的最佳形式。若某个 provider 支持显式 cache-control，必须由 adapter 层基于 `cachePlan` 做转换，core runtime 不直接写 provider 私有字段。

### 与现有 promptContext 的落地关系

WorkOrch 当前真实 request 已经包含 `promptContext.sections` 与 `promptContext.turns`，后续不需要推翻该结构，而是补齐其语义：

```json
{
  "promptContext": {
    "sections": [
      {
        "id": "C0",
        "name": "stable-runtime-prefix",
        "role": "system",
        "cacheability": "global_stable",
        "hash": "sha256:...",
        "sourceRefs": []
      },
      {
        "id": "C1",
        "name": "workspace-definition",
        "role": "user",
        "cacheability": "revision_stable",
        "definitionRevision": "definition-rev-003",
        "hash": "sha256:...",
        "sourceRefs": ["goal://...", "dod://..."]
      }
    ],
    "turns": [
      {
        "sectionId": "C0",
        "role": "system",
        "content": "..."
      }
    ]
  }
}
```

要求：

- `sections[]` 是语义分区和 hash 的权威记录。
- `turns[]` 是 provider-facing 渲染结果，必须引用 `sectionId`。
- 不能只保留 `turn-01` 这类顺序 ID；顺序 ID 可作为 display order，但不能替代 section identity。
- `contextBudget.includedSections` 应从 `turn-01` 升级为 C0-C5 + typed sub-section，例如 `C4.currentFailure`、`C4.validationTail`。
- 大体量 section 必须记录 `subsections`、`sourceRefs`、`omittedRefs`、`redactedRefs`，避免尾部继续膨胀成不可解释的大 JSON。

### Typed Event Tape

对标 Codex 的事件数组，WorkOrch 应在 C4 内形成中立 typed event tape。该 tape 不复制 Codex schema，只表达 WorkOrch 可恢复运行事实。

推荐事件类型：

| Event type | 含义 | 推荐区域 |
| --- | --- | --- |
| `runtime_message` | 当前 stage / requested action / governance instruction | C3 / C4 |
| `llm_decision` | 已接受或待修复的 LLM 输出摘要 | C4 |
| `tool_call` | 工具调用摘要、输入 ref、能力 ref | C4 |
| `tool_result` | 工具输出摘要、状态、payload ref | C4 |
| `validation_result` | 验证结果、失败签名、evidence refs | C4 |
| `artifact_event` | 产物变化、contentHash、artifactRef | C4 / C5 |
| `definition_change` | Goal / DoD revision 变化 | C1 / C4 |
| `context_gate` | context gate pass/fail/read_required | C4 |
| `memory_recall` | 当前决策可用 memory 摘要 | C4 |

每个 event 至少包含：

```json
{
  "sequence": 12,
  "type": "tool_result",
  "stage": "repair",
  "state": "completed|failed|skipped",
  "summary": "...",
  "sourceRef": "step://...",
  "payloadRef": "artifact://...",
  "contentHash": "sha256:..."
}
```

约束：

- `sequence` 必须单调递增并可复现。
- event 不得包含 provider/model/baseURL 动态绑定。
- 大输出只放摘要、head/tail 和 ref。
- C4 超预算时按 event 粒度摘要或引用，不按整段 user turn 截断。
- 最新失败、最新 validation、最新 state-changing artifact event 必须优先保留。

### 顺序保持合同

Cache-aware request 组织不得改变运行事实的发生顺序。Canonical rendering 的目标是让同一语义稳定渲染，不是把所有数据按字典序重新排序。

必须区分两类顺序：

| 数据类型 | 排序规则 | 说明 |
| --- | --- | --- |
| section order | 固定 C0 -> C1 -> C2 -> C3 -> C4 -> C5 -> requestedAction | 为缓存前缀稳定服务 |
| map object keys | schema order 或字典序 | 只影响同一对象字段展示，不表达发生顺序 |
| unordered refs / capability ids | stable id 排序 | 本身无发生顺序 |
| Goal / DoD entries | definition event order / revision order | 不能按文本或 ID 重排 |
| processEvidenceTimeline | `order` / `sequence` 升序 | 表达真实过程顺序 |
| typed event tape | `sequence` 升序 | 表达真实事件顺序 |
| tool call / validation / step history | run-scoped sequence 或 step order | 不能按 kind/name 字典序重排 |
| memory candidates | memory selector score order | 表达召回优先级，不等同时间顺序 |

实现约束：

- C4 typed event tape 必须从现有 `processEvidenceTimeline.order`、step `sequence`、run-scoped path sequence 或等价 monotonic sequence 派生。
- 若原始输入已有顺序字段，禁止为了 canonical JSON 对数组按 ID、kind、summary 或 sourceRef 重新排序。
- 若原始输入没有顺序字段，runtime 必须先补充稳定 sequence，再进入 event tape；不能依赖 Go map iteration 顺序。
- 超预算裁剪时，必须按事件粒度裁剪，并保留 remaining event 的原相对顺序。
- 对“最近 N 条”上下文的裁剪可以选择尾部窗口，但窗口内顺序必须仍然从旧到新。
- Hash 计算必须基于最终 provider-facing 顺序；若顺序变化，必须导致相应 section hash 变化并在 `cacheBreakers` 中可见。

当前代码已有可复用的顺序基础：

- `runtime_context_order.go` 使用显式 sequence / eventSequence / stepSequence / graphSequence / evidenceOrdinal 优先，再按 step id 规则稳定排序。
- `processEvidenceTimeline` 已包含 `order`，并通过 `compareProcessEvidenceOrder` 保序。
- `runtimepath.SequenceScopedPath` 已让 `llm-calls`、`steps`、`tool-calls`、`validations` 等运行态目录使用 run 内自增前缀，并支持服务重启后继续复用最大值。

因此后续改造应复用这些顺序来源，不重新发明一套与运行态 evidence path 不一致的排序规则。

## Canonical Rendering 合同

为减少语义相同但文本不同造成的缓存失效，所有 LLM request section 必须遵守 canonical rendering。

### Section 顺序

固定顺序：

```text
C0 -> C1 -> C2 -> C3 -> C4 -> C5 -> requestedAction
```

同级 section 内固定顺序：

```text
identity -> contract -> definitions -> capabilities -> stage -> progress -> failure -> evidence -> memory -> budget -> refs
```

### JSON 规则

- object keys 按字典序或显式 schema order 输出，二者只能选一个并保持稳定。
- array 若无业务顺序，按 stable id 排序。
- array 若表达发生顺序、定义修订顺序、事件顺序、工具调用顺序、验证顺序或记忆召回优先级，必须保持原语义顺序，禁止按 stable id 重排。
- 空字符串、空 map、空 array 使用统一省略规则。
- `null` 与字段缺失的语义必须明确，不能混用。
- 数字、布尔值按 JSON 标准输出，不用字符串伪装。
- contentHash 使用稳定算法和稳定输入。

### Text 规则

- Markdown 标题层级固定。
- 不使用随机分隔线。
- 不输出当前时间，除非该时间是决策必要证据。
- 不输出运行环境噪点。
- 不在稳定 section 中输出“本轮”“当前最新”等易变措辞。
- 保留 Goal / DoD 原文，不做语义改写。

### ID 与路径规则

- 稳定 section 中避免 run id / step id / attempt id。
- 易变 section 中 ID 使用 deterministic run sequence / step sequence。
- source refs 按 workspaceId、runId、sequence、artifactRef 排序。
- 临时文件名、hash 目录名、provider request 文件路径默认放 C5 refs，不放 C0-C3。

## Goal / DoD Cache 规则

Goal / DoD 是执行合同，不是普通 prompt 材料。

要求：

- 以 append-only entries 记录。
- 每个 entry 有 stable id。
- effective view 由 entries 派生。
- 运行期每次新增、补充、澄清、修订或启停 Goal / DoD 都必须生成 definition change event。
- definition change event 必须包含 source、previousRevision、nextRevision、changedEntries、changeKind、reason、sourceRef。
- `definitionHash` 只由当前有效 Goal / DoD 合同和必要 metadata 计算。
- Goal / DoD 未变化时，C1 文本必须保持字节级稳定。
- Goal / DoD 变化时，应生成新的 definition revision，并使 C3 / C4 显式引用新 revision。
- Goal / DoD 变化后，后续 planning / execution / repair / validation / acceptance 必须以最新 effective view 为准；旧 evidence 只能作为候选 evidence 或 lineage，不能自动关闭新 revision 的 coverage。
- 若当前 run 已经产生过 plan、patch、validation 或 result，definition change 必须触发中立 impact assessment：判断已有产物和证据是否仍覆盖最新 Goal / DoD，以及是否需要 replan、follow-up、repair 或重新 acceptance。

禁止：

- 为了复用缓存裁剪 Goal / DoD。
- 把 Goal / DoD 合并改写后只传改写结果。
- 把旧 Goal / DoD 当 memory recall 噪点注入 C4。
- 让 C4 中的历史失败描述覆盖最新 Goal / DoD。
- Goal / DoD 运行期变化后继续使用旧 definitionHash 生成 LLM request。
- 把 definition change 当作普通 memory recall，导致 LLM 无法识别当前目标已经变化。

### Definition Change 对缓存的影响

Goal / DoD 运行期变更会使 C1 缓存失效，这是正确行为。设计目标不是让 C1 永远命中缓存，而是把失效范围限制在 C1 及依赖它的 C3 / C4，避免 C0 stable runtime prefix 被无关变更破坏。

变更后的 cache behavior：

| Section | Goal / DoD 未变 | Goal / DoD 变化 |
| --- | --- | --- |
| C0 stable runtime prefix | hash 不变 | hash 不变 |
| C1 workspace definition | hash 不变 | hash 变化，revision 增加 |
| C2 capability contract | hash 不变 | 通常不变 |
| C3 current stage bundle | 引用原 revision | 引用新 revision，并声明 impact |
| C4 volatile execution tail | 只包含当前进展 | 包含 definitionChangeImpact 与下一步动作 |
| C5 refs / trace | 可不变 | 追加 previous revision / superseded evidence refs |

若 Goal / DoD 变化后 runtime 无法判断当前执行应继续、replan、repair 还是重新验收，必须进入中立 decision / assessment，而不是静默沿用旧计划继续推进。

## Memory 与缓存边界

Memory recall 应作为参考信息，而不是运行流水垃圾桶。

进入 LLM request 的 memory 必须先分类：

| 类型 | 推荐区域 | 规则 |
| --- | --- | --- |
| stable accepted instruction | C2 或 C3 | 只有来源可靠且长期有效时进入 |
| current decision relevant memory | C4 | 必须解释为什么与当前动作相关 |
| historical evidence memory | C5 | 摘要 + refs |
| rejected / stale / provider retry memory | 不进入默认 request | 只能作为追溯 ref |

Memory recall 必须避免以下情况：

- 把已被新 Goal / DoD 覆盖的旧目标继续作为建议。
- 把失败的 LLM 自述当作事实。
- 把 provider timeout / retry 流水当作业务证据。
- 把低价值日志每轮重新注入，造成 C4 高噪点变化。

## Cache Plan 与 Request Audit

每次 LLM request 应输出 cache-aware audit。该信息用于观测和排障，不作为业务状态恢复依据。

建议字段：

```json
{
  "cachePlan": {
    "schemaVersion": "workorch.llm.cache-aware-context.v1",
    "cacheIntent": "stable_prefix_reuse",
    "cacheKey": "workspace://ws-.../definition-rev-003/runtime-contract-v1/capability-rev-...",
    "stablePrefixHash": "sha256:...",
    "workspaceDefinitionHash": "sha256:...",
    "capabilityContractHash": "sha256:...",
    "stageBundleHash": "sha256:...",
    "volatileTailHash": "sha256:...",
    "definitionRevision": "definition-rev-003",
    "previousDefinitionHash": "sha256:...",
    "definitionChangeImpact": "none|requires_replan|requires_revalidation|requires_repair|requires_decision",
    "messageLayout": [
      "system:C0",
      "developer:C0",
      "user:C1",
      "user:C2",
      "user:C3",
      "user:C4",
      "user:C5",
      "user:requestedAction"
    ],
    "cacheBreakers": [
      {
        "section": "C4",
        "reason": "latest_progress_changed"
      }
    ],
    "canonicalization": {
      "sectionOrder": "v1",
      "jsonOrder": "schema_order",
      "emptyFieldPolicy": "omit_empty",
      "sourceRefOrder": "stable_ref_order"
    },
    "observedRequestShape": {
      "providerFacingMode": "workorch_prompt_context|chat_messages|responses_input",
      "turnsHaveSectionIds": true,
      "typedEventTape": true
    }
  }
}
```

`contextBudget` 可引用 `cachePlan`，但二者职责不同：

- `contextBudget` 表示上下文大小、裁剪、完整性与预算。
- `cachePlan` 表示上下文分区、稳定性、hash 与缓存失效原因。

若实现阶段为了减少字段数量，也可以把 `cachePlan` 作为 `contextBudget.cachePlan` 子对象，但语义必须保持独立。

## Context Gate 增量约束

Cache-aware 组织不能绕过 context gate。新增检查：

| 检查项 | 触发条件 | 处置 |
| --- | --- | --- |
| `stable_prefix_contains_volatile_field` | C0 出现 runId、stepId、timestamp、provider dynamic binding | fail-fast 或降级为实现缺陷 |
| `goal_dod_not_in_definition_block` | Goal / DoD 被移动到 C4 或只以 memory 形式出现 | fail-fast |
| `definition_hash_missing` | C1 缺少 revision/hash | request audit warning；若 Goal/DoD 完整性不可验证则 fail-fast |
| `stale_definition_revision` | C3 / C4 引用的 definition revision 不是 workspace 最新有效 revision | fail-fast 或 replan_required |
| `definition_change_impact_missing` | Goal / DoD 变化后继续执行，但 request 未说明对 plan / evidence / acceptance 的影响 | fail-fast 或 assessment_required |
| `volatile_tail_before_stable_sections` | C4 被放到 C0-C2 之前 | request audit warning；严重时 fail-fast |
| `non_canonical_order_detected` | 同一输入多次 rendering 不稳定 | request audit warning，并记录 section |
| `cache_optimization_dropped_p0_p4` | 为缓存复用省略 P0-P4 | fail-fast |

## Provider Adapter 边界

Core runtime 只输出 provider-neutral messages 与 `cachePlan`。

Provider adapter 可以做：

- 将 C0 标记为 provider 支持的 cache prefix。
- 将 C1 / C2 标记为短期 cache segment。
- 将 C4 保持非缓存尾部。
- 根据 provider 能力选择 message role / cache-control 字段。

Provider adapter 不可以做：

- 删除 Goal / DoD。
- 改写 runtime contract。
- 把 provider 私有 cache 命中信息写入可恢复业务状态。
- 因 provider cache 限制改变 context gate 结果。

## 代码影响面

本设计的第一阶段实现应集中在 LLM block 的 request / prompt 构造层，少量复用 orchestrator 已有上下文顺序与过程证据，不应修改 workspace 输出物，也不应把 provider 私有 cache API 写入 core runtime。

### 必改文件

| 文件 | 当前职责 | 调整内容 |
| --- | --- | --- |
| `internal/modules/blocks/llm/pkg/models/decision.go` | `ProviderRequest`、`PromptContext`、`PromptSection`、`PromptTurn`、`ProviderContextBudget` 数据模型 | 扩展 section metadata、cachePlan / contextBudget.cachePlan、section hash、cacheability、sourceRefs、definitionRevision、subsections |
| `internal/modules/blocks/llm/biz/prompt_messages.go` | 构造 promptContext 与 provider-facing messages | 将 `turn-xx` builder 改为 C0-C5 section builder；让 `turns[]` 引用 `sectionId`；把 `executionSignalsPromptSection` 拆成 C4 typed event tape |
| `internal/modules/blocks/llm/biz/provider_context_budget.go` | provider request 预算、裁剪、context gate audit | 增加 cachePlan audit；在裁剪后维护 section hash/cacheBreakers；确认只对无顺序语义 map/ref 做排序 |
| `internal/modules/blocks/llm/biz/provider_request_redaction.go` | provider request 脱敏与 redactedPaths | 适配 C0-C5 语义路径；redactedPaths 从 `promptContext.sections.turn-xx` 升级为 `promptContext.sections.C4.<subsection>` |
| `internal/modules/blocks/llm/biz/runtime_adapter.go` | StepExecutionRequest -> ProviderRequest、LLM call 落盘、contextBudget 输出 | 调整 providerReq 构造顺序，确保 cachePlan hash 基于最终 provider-facing promptContext；输出与落盘 request.json 保留 cachePlan |
| `internal/modules/blocks/llm/biz/http_provider.go` | HTTP chat-completions provider message 映射 | 继续消费 `buildProviderPromptMessages`，必要时透传 section-aware includedSections；不在第一阶段直接绑定 provider 私有 cache-control |
| `internal/modules/blocks/llm/biz/sdk_provider.go` | SDK provider message 映射 | 同 HTTP provider；后续 provider cache hint 只在 adapter 层消费 `cachePlan` |

### 复用文件

| 文件 | 可复用能力 | 约束 |
| --- | --- | --- |
| `internal/modules/kernel/orchestrator/biz/runtime_context_order.go` | step 稳定排序：显式 sequence 优先，fallback 到 step id 规则 | C4 event tape 的 step/event 顺序应复用该排序原则 |
| `internal/modules/kernel/orchestrator/biz/execution_context_package.go` | `executionContextPackage`、`processEvidenceTimeline`、`currentProgressDigest`、`definitionChangeImpact` | 不重新发明过程证据来源；typed event tape 从这里派生 |
| `internal/pkg/runtimepath/path.go` | run-scoped sequence path，服务重启后继续最大序号 | `llm-calls`、`steps`、`tool-calls`、`validations` 的关联顺序继续复用该能力 |
| `internal/modules/application/workorchd/biz/appservice/read_model_service.go` | contextBudget / llmUsage read model 投影 | 后续展示 cachePlan，可在 read model 只读投影，不改变运行语义 |
| `internal/modules/application/workorch/service/tui/view.go`、`web/workorch-console/src/components/cards/LLMContextBudgetCard.tsx` | TUI / Web 展示 includedSections、budget | 可选展示 C0-C5 / cacheBreakers，不作为第一阶段 core blocker |

### 不应改动

- 不修改 workspace 交付物目录或已有运行输出物。
- 不在 orchestrator core 中写入软件开发、代码、测试框架或具体业务领域规则。
- 不改变 Goal / DoD 持久化语义；只在 promptContext C1 中引用 revision/hash。
- 不把 provider/model/baseURL 写入可恢复业务状态。
- 不复制 Codex 私有 `/v1/responses` request schema 到 WorkOrch core。

## 代码收口顺序

后续按以下顺序逐项实施，避免一次性大改导致顺序和上下文完整性同时失控：

### LCAC-P1: 数据模型与只读 audit

状态：已完成首批代码收口。

目标：先让 request.json 能表达 C0-C5 和 cachePlan，但不改变 provider 实际收到的 message 内容。

涉及：

- 扩展 `PromptSection` / `PromptTurn` / `ProviderContextBudget`。
- 新增 cachePlan 构造与 hash helper。
- `buildPromptContextFromRequest` 继续输出原内容，但 section id 从 `turn-xx` 增加语义别名或额外 metadata。

验收：

- 已保持 provider-facing prompt content 由 `promptContext.turns` 渲染，首批实现只补 section metadata / hash / cachePlan audit。
- request.json 可出现 C0-C5 metadata、section hash、cacheability、sourceRefs、definitionRevision。
- `turns[]` 已引用语义 `sectionId`。
- 已补顺序测试，证明 turns 顺序与 section 顺序一一对应。

已落地文件：

- `internal/modules/blocks/llm/pkg/models/decision.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/provider_cache_plan.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`
- `internal/modules/blocks/llm/biz/prompt_messages_test.go`
- `internal/modules/blocks/llm/biz/runtime_adapter_test.go`

验证：

- `GOCACHE=/tmp/workorch-go-build go test ./internal/modules/blocks/llm/biz -count=1`

### LCAC-P2: C0-C5 Section Builder

状态：已完成基础 section 映射；C4 细粒度 event tape 拆分仍归属 LCAC-P3。

目标：把原有 prompt sections 映射到 C0-C5。

映射建议：

| 现有 section | 目标 section |
| --- | --- |
| `systemPromptForRequest`、`languageAndContractPrompt` 中稳定合同 | C0 |
| `goalPromptSection`、`dodPromptSection`、`effectiveGoalDoDPromptSection` | C1 |
| `capabilityContractPrompt`、runtime capability summary、soul/policy capability summary | C2 |
| `workspace-run`、`stageContractPromptSection`、`repairActionPackage.requiredOutput` | C3 |
| `previousDecisionPromptSection`、`executionSignalsPromptSection` 的 current failure/progress/evidence 部分 | C4 |
| `traceRefs`、large payload refs、omitted/redacted refs | C5 |
| `currentActionPromptSection` | requestedAction，紧随 C4/C5 后 |

验收：

- C0 已不包含 workspaceId/runId/stepId/current failure，能力合同已从 C0 拆出到 C2。
- C1 已包含完整 Goal / DoD 原文和 definitionChangeImpact，并保留 raw_required protectedSections。
- C2 已独立表达 runtime capability contract / governance policy。
- C3 已表达 workspace-run、stage-contract、repair-action-package。
- C4 已承载 previous-decision、execution-signals、current-action；后续 LCAC-P3 继续把混合 execution-signals 拆成 typed event tape。

### LCAC-P3: Typed Event Tape

状态：已完成首批 C4 typed section 拆分；后续继续细化 event 粒度裁剪和 sourceRef 强校验。

目标：把大体量 `Execution Context and Evidence Signals` 拆成 C4 typed events。

事件来源优先级：

1. `input.executionContextPackage.processEvidenceTimeline`
2. `input.processEvidenceTimeline`
3. `input.currentProgressDigest`
4. `input.repairActionPackage`
5. `previousStepOutput`
6. `runtimeCapabilities` 中与当前 requested action 相关的能力摘要

验收：

- C4 已不再只依赖一个混合大 JSON，首批拆为 runtime-stage、current-failure、current-progress、process-evidence-timeline、validation-signal、governance-resolution、memory-recall、repair-json-output、current-action。
- processEvidenceTimeline 继续使用运行期已提供的顺序数据，不在 LLM block 中重新排序。
- 首批实现保留 sourceRef / payloadRef / contentHash 等原始字段；后续需要在 LCAC-P3.1 增加强校验与 event 粒度裁剪。
- repair 请求中 latest failure、validation、target snapshot 由 current-failure / validation-signal / repair-action-package 共同表达。

### LCAC-P4: CachePlan 与 ContextBudget 联动

目标：让 request audit 可判断缓存复用边界。

新增或投影：

- `stablePrefixHash`
- `workspaceDefinitionHash`
- `capabilityContractHash`
- `stageBundleHash`
- `volatileTailHash`
- `cacheBreakers`
- `observedRequestShape`

验收：

- 连续相同 Goal / DoD 的 repair 请求，C0/C1/C2 hash 稳定，C4 hash 变化。
- Goal / DoD 变化后，C0 hash 不变，C1 hash 变化，C3 引用新 revision。
- provider/config 变化不进入业务恢复状态。

### LCAC-P5: Provider Adapter 预留

目标：adapter 可以消费 cachePlan，但 core 不绑定 provider 私有字段。

第一阶段：

- HTTP / SDK provider 继续走 chat messages。
- 不强制新增 provider 私有 cache-control。

后续可选：

- OpenAI Responses adapter 可将 C0/C2 映射到 instructions/tools。
- 支持 provider cache-control 的 adapter 可将 C0 标记为 cache prefix。

验收：

- 不支持 prompt cache 的 provider 行为不变。
- 支持 prompt cache 的 provider 只在 adapter 层消费 cachePlan。

### LCAC-P6: 展示与核对

目标：让 Web/TUI 能看懂缓存分区和失效原因。

涉及：

- `read_model_types.go` / `read_model_service.go`：投影 cachePlan。
- `LLMContextBudgetCard.tsx`：展示 C0-C5、hash 简写、cacheBreakers。
- TUI context budget view：展示 included sections 从 `turn-xx` 变为 C0-C5。

验收：

- Web/TUI 能看到本轮 request 是 C4 变化、C1 变化还是 C0 变化。
- 不展示完整 prompt 明文，只展示 summary / refs / hash。

## 测试收口清单

| 测试文件 | 必补场景 |
| --- | --- |
| `internal/modules/blocks/llm/biz/prompt_messages_test.go` | C0-C5 section identity、turns 引用 sectionId、Goal / DoD 完整保护、C4 event tape 顺序 |
| `internal/modules/blocks/llm/biz/provider_context_budget_test.go` | cachePlan audit、hash 稳定性、cacheBreakers、缓存优化不得裁剪 P0-P4 |
| `internal/modules/blocks/llm/biz/runtime_adapter_test.go` | request.json 落盘包含 C0-C5/cachePlan；redaction 后仍保留 hash/sourceRefs |
| `internal/modules/kernel/orchestrator/biz/execution_context_package_test.go` | processEvidenceTimeline 到 typed event tape 的 order 不变 |
| `internal/modules/application/workorchd/biz/appservice/read_model_service_test.go` | cachePlan 只读投影，不泄露 prompt 明文 |
| `internal/modules/application/workorch/service/tui/view_context_test.go`、Web component tests | C0-C5 展示兼容 includedSections |

## 实施计划

| ID | 优先级 | 状态 | 内容 | 验收标准 |
| --- | --- | --- | --- | --- |
| LCAC-01 | P0 | done | 定义 cache-aware section builder | LLM request 能按 C0-C5 输出多 message |
| LCAC-02 | P0 | done | 实现 canonical rendering | 相同输入重复构造得到相同 section hash |
| LCAC-03 | P0 | done | Goal / DoD definition hash | Goal / DoD 未变时 C1 hash 不变，变化时 revision/hash 更新 |
| LCAC-04 | P0 | done | contextBudget / cachePlan audit | 每次 request 可看到 stablePrefixHash、definitionHash、volatileTailHash |
| LCAC-05 | P0 | done | definition change impact 注入 | Goal / DoD 运行期变化后，C3/C4 明确引用最新 revision 并说明 replan / revalidation / repair / decision 影响 |
| LCAC-06 | P0 | done | promptContext section identity | `promptContext.sections` 从 `turn-xx` 升级为 C0-C5 语义分区，`turns[]` 引用 sectionId |
| LCAC-07 | P0 | done | C4 typed event tape | 大体量 execution signals 拆为 current failure / validation / tool / artifact / memory / requested action 等 typed events |
| LCAC-08 | P1 | done | memory cacheability 分类 | 无关、过期、低价值 memory 不进入默认 C4 |
| LCAC-09 | P1 | done | provider adapter cache hint 预留 | core 不绑定 provider，adapter 可选消费 cachePlan |
| LCAC-10 | P1 | done | request log 核对脚本 | 可对比连续请求的 section hash 和 cacheBreakers |
| LCAC-11 | P1 | done | 文档摘要回填核心合同 | 将强制性规则摘要合入 runtime stage neutrality design |

LCAC-08 至 LCAC-11 已落地：

- C4 `memory-recall` 只展开当前 retrieval 选中、未被排除、非 stale / superseded / diagnostic-only、非低置信或低重要度的 memory；retrieval 审计摘要仍保留。
- `cachePlan.adapterHint` 提供 provider-neutral prefix hint，core 不写任何 provider 私有 cache-control 字段。
- 新增 `scripts/verify-llm-cache-aware-request-log.sh`，可离线核对 workspace request.json 的 C0/C1/C4 section、section hash、cachePlan 和 cacheBreakers。
- read model 与 Web `LLMContextBudgetCard` 已投影并展示 cachePlan hash 简写和 cacheBreakers，不展示 prompt 明文。

验证：

- `GOCACHE=/tmp/workorch-go-build go test ./internal/modules/blocks/llm/biz -count=1`
- `GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorchd/biz/appservice -count=1`
- `npm run build` in `web/workorch-console`

## 验收用例

### 连续 repair 重试

输入：

- Goal / DoD 未变化。
- runtime contract 未变化。
- capability 未变化。
- latest failure / progress 变化。

预期：

- C0 hash 不变。
- C1 hash 不变。
- C2 hash 不变。
- C3 可按 stage 是否变化决定。
- C4 hash 变化。
- cacheBreakers 只指向 C4 或 C3。

### Goal 补充后重新执行

输入：

- 新增 Goal entry。
- 其他 runtime contract 不变。

预期：

- C0 hash 不变。
- C1 hash 变化。
- C3 引用新的 definition revision。
- C4 不携带旧 Goal memory 噪点。
- request 明确要求按新 Goal / DoD 重新评估。

### Provider 配置变更

输入：

- provider / model / baseURL 改变。
- Goal / DoD / runtime contract 不变。

预期：

- C0-C3 语义 hash 不因 provider 动态绑定变化而变化。
- contextBudget 的 provider profile 可以变化。
- provider 动态绑定不进入可恢复业务状态。

### Memory 噪点清理

输入：

- memory recall 返回旧失败自述、provider timeout 流水、已被覆盖目标。

预期：

- 旧失败自述不作为事实进入 C4。
- provider timeout 流水不作为业务证据进入 C4。
- 已被覆盖目标不进入 C1 / C4。
- C5 可以保留追溯 refs。

### Canonical 稳定性

输入：

- 同一 workspace、同一 run、同一 stage 输入重复构造 request。

预期：

- section 顺序一致。
- JSON key 顺序一致。
- refs 顺序一致。
- hash 一致。
- 无随机目录名、当前时间、map iteration 顺序导致的 diff。

## 与现有文档关系

- `workorch-runtime-stage-contract-neutrality-design.md` 是长期核心合约，本设计中的 P0 强制规则最终应摘要回填到该文档。
- `codex-like-llm-prompt-context-design.md` 定义多消息和完整上下文组织，本设计进一步定义哪些 message 应保持稳定以提升缓存复用。
- `model-aware-llm-context-budget-design.md` 定义预算和裁剪，本设计不改变预算优先级，只要求裁剪后仍保持 section hash / omitted reason 可观测。
- `workorch-runtime-storage-json-control-design.md` 定义存储膨胀控制，本设计只约束 provider request 组织和 audit，不要求持久化完整 prompt 明文。

## 结论

WorkOrch 后续 LLM request 不能再被视为单个大快照。它应被视为一个带 cacheability 语义的分层上下文包：

```text
Stable runtime prefix
  -> workspace definition
  -> capability contract
  -> current stage bundle
  -> volatile execution tail
  -> large refs and trace
```

该方案可以在保持 core runtime 功能中立和上下文完整性的前提下，提高 LLM provider 对稳定前缀的缓存复用机会，并让缓存失效原因可审计、可回放、可验证。
