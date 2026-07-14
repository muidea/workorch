# Run Resume And Gap Confirmation Design

Status: active

Type: module-architecture

Last Updated: 2026-06-21

## Purpose

本文固化 WorkOrch 在启动 workspace 执行前的 run command 语义、gap confirmation、todo baseline 和后续调度入口设计，作为后续收口 TUI/Web/workorchd/orchestrator 代码的依据。

当前阶段不考虑兼容性。若现有 旧推进命令、`/run restart`、REST activation、decision replan 或 projection 行为与本文冲突，应以本文作为后续代码收口依据。

近期核对暴露的问题是：workspace 多次执行后虽然状态能流转，但首轮目标理解、后续 gap 识别、违例恢复、TUI 命令语义和投影刷新之间没有形成同一条闭环，导致 LLM 可能先产出偏离 Goal 的文件，后续又在无效 repair/replan 中反复消耗上下文。本文的收口目标不是放宽质量门禁，而是把“启动前确认 gap -> 带着 gap 执行 -> 违例反馈重试 -> 投影展示同一事实”的链路固化为通用能力。

本文补充以下设计文档：

- [eventhub-driven-workorchd-state-loop-design.md](eventhub-driven-workorchd-state-loop-design.md)
- [runtime-execution-event-queue-state-loop-design.md](runtime-execution-event-queue-state-loop-design.md)
- [mvvp-workspace-viewmodel-architecture.md](mvvp-workspace-viewmodel-architecture.md)
- [contract-violation-recovery-loop-design.md](contract-violation-recovery-loop-design.md)

本文的验收方式是“按编号条目逐一收口”。后续任何代码修改都应引用本文的 `D-xx`、`Pxx` 或 `RG-xx` 条目，说明命中主路径、边界归属和验证证据，避免继续围绕单个 live workspace 现象做发散修补。

## Design Baseline Freeze

本轮核对后的设计基线固定为：

- 默认执行入口统一为 `/run resume`，用于首次启动、服务重启后恢复、idle/recoverable run 继续推进，以及 decision/approval resolution 后继续收敛。
- `/run restart` 只表达显式 replacement run，不再承担“继续执行”语义；它必须关闭或 supersede 旧 active run，并在新 run 上重新进入 gap confirmation。
- 首次执行、resume、replan、restart 和 recover 在调度任何 state-changing step 前，都必须先生成或刷新服务端 `GapBaseline`。
- `GapBaseline` 是 scheduler、orchestrator、LLM step input 和 projection 的同一份收敛合同，不是 TUI/Web 本地 UI 摘要。
- LLM 违例输出可以作为 rejected diagnostic evidence 进入 bounded retry，但不得执行、不得成为 accepted evidence/result/artifact，也不得写入长期高价值 memory。
- TUI/Web/CLI/agent 只提交 command 并消费 workorchd ViewModel projection，不拥有 run、decision、approval、gap 或 todo 状态机。
- WorkOrch core 只实现业务中立的 command、gap、todo、evidence、projection、retry 和调度机制；业务推进优先级、验证偏好和产物策略由 workspaceTemplate、Soul、profile policy 或 skill 承载。

后续收口优先验证主路径：

```text
client command
  -> WorkspaceRunRequested EventHub command
  -> workorchd scheduler accepted command projection
  -> GapBaseline checkpoint/projection
  -> LLM/runtime step input
  -> runtime result or rejected diagnostic
  -> workorchd ViewModel projection
```

如果 live workspace 仍然不能正向推进，应先按这条主路径定位断点，再决定修改 `P1-P6`、`RG-01-RG-19` 或对应 `Phase` 条目；不得先加入模板、语言、provider、文件名或具体 workspace 的 core 特判。

## This Review Output Index

本次设计文档输出用于承接以下核对结论，并作为后续逐项代码收口入口：

### Current Design Deliverable

本轮输出的对应设计文档即本文。后续围绕“workspace 不能持续正向产出、LLM 违例频繁导致停滞、首次执行缺少 gap 确认、旧推进命令 / `/run restart` 语义混淆、TUI/Web 状态显示与 workorchd 投影不一致”的代码收口，统一以本文作为主设计依据。

本轮设计固化以下最终口径：

- 常规推进命令统一为 `/run resume`；它覆盖首次启动、恢复 idle/recoverable run、服务重启后重新激活、decision/approval resolution 后继续推进。
- `/run restart` 只表示显式 replacement run；它必须关闭或 supersede 旧 active run，并基于最新 workspace definition 重新进入 gap confirmation。
- 旧推进命令 不再作为 canonical UX 或内部设计语义；若入口临时接受该文本，只能立即归一化为 `resume`，不得在 port、REST、EventHub、scheduler 或 projection 中形成第二套状态。
- 首次执行、resume、replan、restart、recover 都必须先生成或刷新服务端 `GapBaseline`，再调度任何 state-changing LLM/runtime step。
- `GapBaseline` 是 workorchd/orchestrator/LLM input/projection 共享的收敛合同，不是 TUI/Web 本地摘要。
- LLM 可以返回违例内容，但违例只能成为 rejected diagnostic evidence，并进入 bounded retry；不得执行、不得成为 accepted result/artifact/coverage evidence，也不得写入长期高价值 memory。
- TUI/Web/CLI/agent 只提交 command 并消费 workorchd ViewModel projection，不拥有 run、decision、approval、gap、todo 或 prompt 的业务状态机。
- WorkOrch core 必须保持业务中立；具体业务推进策略、产物优先级、验证偏好和工具顺序只能由 workspaceTemplate、Soul、profile policy 或 skill 以结构化数据提供。

后续代码收口应按本文的 `D-01` 到 `D-07`、`P1` 到 `P6`、`RG-01` 到 `RG-19` 和 `Phase A` 到 `Phase F` 逐项推进。每项完成时必须补充测试证据、边界归属说明和必要的 live/e2e 核对结果。

| Review conclusion | Design anchor | Closure entry |
| --- | --- | --- |
| workspace 启动后长期没有有效文件产出，不能继续优先叠加业务化 prompt | `Current Review Finalization`、`D-03`、`D-04` | `P2`、`P4`、`RG-05`、`RG-15` |
| 首次执行前需要确认 Goal/DoD 与已完成内容 gap | `First-Run Gap Baseline` | `Phase C` |
| 旧推进命令 / `/run restart` 语义容易混淆，需要统一常规推进动作 | `D-01`、`D-02`、`Command Semantics` | `Phase A`、`RG-01`、`RG-02`、`RG-10` |
| 常规推进命令应统一为 `/run resume`，restart 只表达 replacement run | `REST / Port / TUI Mapping` | `Phase A`、`Phase B` |
| replan 必须基于同一 active run 继续推进，不等同于 restart | `Target State Flow`、`D-04` | `RG-12` |
| LLM 违例不能执行，但应反馈违例原因并 bounded retry | `Contract Violation Feedback Retry`、`D-05` | `Phase D`、`RG-16`、`RG-17` |
| TUI/Web 只负责交互提交和投影展示，不拥有状态机 | `Projection And MVVP`、`D-06` | `Phase E`、`RG-09`、`RG-18` |
| WorkOrch core 必须保持业务中立，业务推进策略进入 template/Soul/profile/skill | `EventHub And Module Boundary`、`D-07` | 全阶段边界审核 |

后续收口优先级固定为：

1. 先统一 command 语义和 EventHub 主路径，确保 `/run resume`、replan、restart 都能进入同一 workorchd 调度入口。
2. 再落实首次执行和每次恢复执行前的 `GapBaseline` gate，禁止绕过 gap/todo/requiredOutput 直接发起状态变更。
3. 再补齐 LLM step input、违例 rejected diagnostic retry 和 MVVP projection。
4. 最后补齐 restart/service recovery、watchdog 边界和 deployed e2e。

这意味着后续不再把“没有有效文件产出”优先归因为某个 live workspace、某个 provider 或某段提示词，而是先验证服务端是否已经建立统一的 command -> gap -> scheduler -> LLM/runtime -> projection 闭环。

## Review Conclusions

本次核对后的设计结论如下：

1. 正向推进慢、首轮没有有效产出、后续反复 repair/replan 的问题，不能只归因于 prompt 文案，也不能通过放宽核心合同解决。
2. 更直接的问题是启动类命令、run 状态恢复、Goal/DoD coverage、当前 gap、requiredOutput、违例诊断和 ViewModel 投影没有形成同一条权威链路。
3. 旧推进命令 与 `/run restart` 对用户语义不清。默认交互应统一为 `/run resume`，表示“推进当前 workspace 的当前有效 run”；`restart` 只表达显式 replacement run。
4. 首次执行也必须做 gap confirmation。没有 run 或没有历史 evidence 时，gap baseline 仍然可以确定：最新 Goal/DoD/template/profile 的可验收项均为 open，已有文件/evidence 只作为 deterministic 输入被确认。
5. LLM 输出违例应被允许接收和诊断，但不能被执行、不能成为 accepted evidence，也不能进入长期高价值 memory。可恢复违例应反馈给下一轮 bounded retry。
6. Web/TUI 只负责提交 command 和展示 workorchd ViewModel，不拥有 run 状态机、decision 状态机、gap/todo 计算或恢复逻辑。
7. WorkOrch core 必须保持业务中立。具体软件开发策略、产物优先级、工具偏好和验证路径只能由 workspaceTemplate、Soul、profile policy 或 skill 提供。

因此，本设计的核心不是新增一个端侧命令，也不是新增一段提示词，而是把 `run command -> gap confirmation -> scheduler -> LLM/runtime step -> violation recovery -> projection` 固化为可测试的服务端闭环。

## Latest Review Addendum

本轮进一步核对后，需要把以下结论作为后续逐项收口的硬约束：

1. workspace 无法持续产生符合 Goal 的有效文件产出时，不能继续优先通过叠加业务化提示词或扩大禁止项修补。优先确认服务端是否在每次启动、resume、replan 和 restart 前建立了最新 `GapBaseline`，并把 open todo、current focus、requiredOutput、accepted/rejected evidence 输入到下一步执行。
2. 首次执行前必须先做 gap confirmation。即使没有历史 run，也应基于最新 Goal/DoD/template/profile 得到一个“全部待完成或可由现有 evidence 证明”的 baseline，然后再让 LLM 执行状态变更。
3. 旧推进命令 与 `/run restart` 的用户语义容易混淆。TUI/Web 的默认推进动作统一为 `/run resume`；旧推进命令 不再进入设计语义；`/run restart` 只表示显式 replacement run，并且必须有清晰的 destructive 语义。
4. 大 DoD 不应直接导致执行发散。core 可以把 DoD/coverage work item 切成可追踪 todo 和 current focus，但不能根据业务文本判断优先级；具体优先级、验证偏好和交付节奏由 workspaceTemplate、Soul、profile policy 或 skill 通过结构化字段提供。
5. 为避免首轮产出偏差和后续无效 repair，LLM 第一次可见的执行合同必须同时包含“当前要做什么”和“哪些输出是允许的”。禁止项只能防止越界，不能替代正向执行路径。
6. 若 LLM 返回违例，runtime 应把违例输出记录为 rejected diagnostic evidence，并用 bounded retry 反馈修正要求；不得接受该输出作为 artifact/result，也不得让违例内容写入长期高价值 memory。
7. TUI/Web 只能展示 workorchd 投影并提交命令。prompt、workspace 列表、主视图、状态栏必须来自同一 ViewModel revision，不能在端侧临时推断 waiting/running/decision 状态。
8. 当前阶段不考虑兼容性。若历史 API、脚本、测试或 UI 文案与上述设计冲突，应按本文统一收口。

## Current Review Finalization

本节固化本轮围绕“为什么 workspace 仍未持续输出有效文件、是否需要首次 gap 确认、旧推进命令 / `/run restart` 是否应统一”的最终设计口径。后续代码收口应优先命中本节，再落到 `D-xx`、`Pxx`、`RG-xx` 和 `Phase` 条目。

### 1. 正向推进不足的直接收口方向

当前现象是：run 能进入 active / blocked_on_decision / repair 等状态，但没有稳定产生符合 Goal 的有效产物，或者 replan 后仍停留在 decision / repair 循环。Workspace owner state 不承载 decision/running 语义，只通过 projection 展示 active run、ticket 和 prompt mode。

设计判断：

- 这不是单纯的 LLM provider 问题，也不能只通过添加更严厉的禁止提示词解决。
- 更关键的问题是 LLM 在状态变更前没有被稳定地喂入同一份服务端权威收敛合同：最新 Goal/DoD、已接受 evidence、已拒绝 evidence、当前 gap、当前 focus、requiredOutput 和违例诊断。
- 因此主路径必须调整为“先确认 gap，再执行状态变更”。首次执行、resume、replan、restart、recover 都不能绕过该 gate。

### 2. 首次执行前必须做 Gap Confirmation

首次执行也需要 gap confirmation。没有历史 run 并不表示可以直接让 LLM 根据 Goal/DoD 原文生成文件；它只表示 deterministic baseline 的默认结论是：未被已有 evidence 证明的目标均为 open。

首次执行主路径固定为：

```text
WorkspaceRunRequested(intent=resume)
  -> resolve latest workspace definition
  -> create first run if missing
  -> build deterministic GapBaseline
  -> persist/checkpoint GapBaseline
  -> derive first currentFocus and requiredOutput
  -> build first LLM/runtime step input
  -> dispatch state-changing step
```

如果 deterministic 数据不足以判断某个已有 artifact 是否满足目标，只能进入 assessment-only gap confirmation。该阶段禁止 patch、file write、shell/MCP/file side effect 和 execution graph。

### 3. `/run resume` 是唯一常规推进语义

TUI/Web/CLI 面向用户的常规推进动作统一为 `/run resume`。它表达“继续推进当前 workspace 的当前有效 run”，不要求用户判断是首次启动、服务重启恢复、decision 释放后继续，还是 failed recoverable 后继续。

命令差异固定如下：

| Command / Intent | Semantics | Run handling | User visibility |
| --- | --- | --- | --- |
| `/run resume` / `resume` | 推进当前有效 run | 不 supersede，不重建；无 run 时创建首个 run | 默认推荐 |
| decision `replan` / `replan` | 处理 decision resolution 后重新规划当前 run | 继承当前 run、Goal/DoD、GapBaseline 和 blocker resolution | 仅 decision 场景 |
| `/run restart` / `restart` | 显式 replacement run | 关闭或 supersede 旧 active run，创建 replacement run | destructive，需明确提示 |
| 旧推进命令 | 不再作为设计语义 | 若临时保留只能在入口归一化为 resume | 非 canonical |

`/run restart` 不能作为“继续执行”的替代命令。后续若 TUI 为避免误解，可以把它展示为更明确的 destructive 操作，例如 reset / restart new run；但后端 intent 仍保持 `restart`。

### 4. Replan 不等于 Restart

decision replan 的设计语义是“基于同一个 active run 的最新 blocker resolution 重新规划”，不是新建 run，也不是清空历史。

replan 必须：

- 通过 EventHub command 进入与 resume 相同的 scheduler 主路径。
- 携带 workspace scope、active run id、decision id、resolution revision、request id。
- 先刷新或复用最新 `GapBaseline`，再调度 replan step。
- 继承 accepted/rejected evidence、current focus、requiredOutput 和原失败阶段语义。

replan 不得：

- 在 TUI/Web 本地直接把 prompt 改成 running。
- 绕过 workorchd command projection。
- 清空旧 run 后创建 replacement run。
- 把 decision resolution 文本当作新的 Goal/DoD 权威定义。

### 5. Strict Contract 与 Flexible Guidance 分层

为了减少 LLM 违例导致的停滞，设计上允许 LLM 返回违例内容并记录诊断，但不放宽状态机合同。

后续 prompt/context 必须分层：

| Layer | Ownership | Examples | Handling |
| --- | --- | --- | --- |
| strict contract | core | JSON schema、workspace scope、requiredOutput、assessment-only 禁止副作用、approval 边界 | 违例即 rejected diagnostic，bounded retry |
| flexible guidance | template/Soul/profile/skill | 产物粒度、验证偏好、工具顺序、交付节奏、业务优先级 | 偏离后进入质量评估、repair 或 decision |

core 可以提供“避免过大单文件、按小粒度产物推进、保持 evidence 可追溯”等中立 guidance；具体语言、文件组织、验证命令和业务交付优先级必须由 workspaceTemplate、Soul、profile policy 或 skill 提供。

### 6. 设计收口判定

如果后续 live workspace 仍未正向推进，先按以下顺序核对，不再直接追加业务提示词：

1. command 是否进入 `WorkspaceRunRequested` EventHub 主路径，并投影 accepted command revision。
2. 当前 workspace/run 是否已经生成最新 `GapBaseline`，且 baseline revision 出现在 projection 和 LLM step input。
3. LLM step input 是否包含 currentFocus、open todo、requiredOutput、accepted/rejected evidence 和 latest definition revision。
4. 如果 LLM 输出违例，违例是否被记录为 rejected diagnostic，并通过 bounded retry 反馈，而不是直接 accepted 或无限 decision。
5. TUI/Web prompt 是否只来自同一 workorchd ViewModel revision，且没有端侧本地状态覆盖。
6. core 是否仍保持业务中立，没有新增模板、语言、命令、文件名、provider 或 live workspace 特判。

## Finalized Decisions

本节作为后续代码收口的最终决策基线。若其他文档或现有实现与本节冲突，应先修正为本节定义的语义，再继续实现细节。

### D-01: 常规执行入口统一为 `/run resume`

`/run resume` 是用户、TUI、Web 和 CLI 面向“继续推进当前 workspace”的唯一常规语义。它覆盖以下用户意图：

- 首次启动 workspace。
- 服务重启后重新激活可恢复 workspace。
- 当前 run 已暂停、idle、waiting 后被 resolution 释放后继续推进。
- 用户希望继续当前目标收敛，而不是丢弃当前 run。

旧推进命令 不再作为设计语义存在。当前阶段不要求兼容历史命令；如果实现里临时保留 alias，只能作为迁移期输入归一化，不能在 port、REST、EventHub、scheduler 或 projection 中形成第二套 legacy resume alias 状态或分支。

### D-02: `/run restart` 只表示 replacement run

`/run restart` 不能与 resume 混用。它必须表达“终止或 supersede 当前 active run，并基于最新 workspace 定义创建 replacement run”。

restart 的硬规则：

- 必须显式关闭旧 active run，旧 run 不能继续接受后续 step。
- 必须创建或复用同一 dedupe key 对应的 replacement run。
- replacement run 必须先执行 gap confirmation，再调度任何状态变更 step。
- TUI/Web 不能把 restart 当作普通“继续执行”按钮。

### D-03: 首次执行前必须先生成 GapBaseline

workspace 没有历史 run 或没有 accepted evidence 时，也必须生成 `GapBaseline`。此时 baseline 的含义不是“已有进度”，而是“基于最新 Goal/DoD/template/profile 可确认的 open gap”。

首次执行的调度顺序必须是：

```text
resume command accepted
  -> create first run if missing
  -> build deterministic GapBaseline
  -> persist/checkpoint GapBaseline
  -> build first LLM/runtime step input from GapBaseline
  -> dispatch state-changing step
```

禁止直接把自然语言 Goal/DoD 原文交给 LLM 后立即请求状态变更输出。首个 state-changing step 必须看到 open todo、current focus、requiredOutput、accepted/rejected evidence 和当前 evidence requirement。

### D-04: GapBaseline 是执行收敛合同，不是 UI 摘要

`GapBaseline` 必须同时服务于 scheduler、orchestrator、LLM step input 和 projection。TUI/Web 可以展示其摘要，但不能在端侧重新计算或改写。

GapBaseline 至少承担以下职责：

- 明确当前最新 Goal/DoD definition revision。
- 保留 accepted evidence 与 rejected evidence。
- 标记 open / satisfied / blocked / failed todo。
- 给出 current focus 和下一步 requiredOutput。
- 让 repair、replan、resume 和 first-run 使用同一收敛基线。

### D-05: 违例输出允许被诊断，但绝不能被接受

近期反复出现 LLM 返回内容不稳定或违反合同的现象。设计上不应通过放宽核心合同解决，而应通过 rejected diagnostic retry 提升容错性。

违例处理硬规则：

- 违例输出不得执行。
- 违例输出不得写成 accepted result、artifact 或 coverage evidence。
- 违例输出不得进入长期高价值 memory。
- 可恢复违例必须形成结构化 diagnostic，反馈给 bounded retry。
- 重复同类违例超过预算后进入 decision/failed，而不是无限 repair。

### D-06: TUI/Web 只展示 workorchd ViewModel

workorchd 是 workspace 状态投影、run command 状态、blocker、gap/todo 和 prompt 状态的唯一 ViewModel owner。

TUI/Web 的职责边界：

- 只提交 `WorkspaceRunCommand`、decision resolution、approval resolution 等 command。
- 只展示 workorchd projection。
- 不持有第二套 run 状态机。
- 不根据本地输入成功推断 workspace 已进入 running。
- prompt 必须绑定 projection 中的当前 workspace、active run、blocker id 和 accepted command revision。

### D-07: Core 保持业务中立，业务推进策略进入 template/Soul/profile

为促进更快 goal 收敛，core 可以提供通用机制：

- gap/todo 建模
- requiredOutput 建模
- evidence quality gate
- contract violation retry
- context budget 与历史摘要
- small artifact guidance

core 不能包含以下内容：

- 模板 ID、Soul ID、具体编程语言或文件名特判。
- 根据 Goal 文本、DoD 文本或命令输出 marker 推断业务优先级。
- 针对某个 live workspace 的目标、路径、provider 或工具写分支。

具体业务优先级、产物形态、验证偏好、工具选择、交付节奏应由 workspaceTemplate、Soul、profile policy 或 skill 以结构化数据提供。

## Scope Lock

后续代码收口必须命中本文的范围，避免继续在 prompt、TUI、watchdog 或单个 live workspace 现象之间发散。

In scope:

- 统一端侧 run command intent，默认 `resume`，不再在内部模型保留 `continue` 语义，明确 `restart` destructive/replacement 语义。
- 所有 TUI/Web/REST 启动类动作通过 EventHub command 进入 workorchd scheduler。
- 在首次执行、resume、replan、restart、服务重启后的恢复执行前，建立或刷新 deterministic `GapBaseline`。
- 将 `GapBaseline.todoItems`、current focus、accepted/rejected evidence、required evidence kinds 输入到后续 LLM/runtime step。
- 对合同违例建立 rejected diagnostic evidence 和 bounded retry。
- workorchd projection 暴露 gap/todo/command/blocker/violation 的权威 ViewModel，TUI/Web 只消费投影。
- 补充单测/e2e 验证多 workspace、重复 resume、decision replan、service restart、违例 retry 和 projection prompt 同步。

Out of scope:

- 在 core 中根据模板 ID、业务文本、具体语言、文件名、命令、provider 或 live workspace ID 分支。
- 让 TUI/Web 自行判断是否可继续、是否需要 replan、是否应该恢复。
- 用扩大禁止项、降低合同准入、无限 retry 或 watchdog 补偿替代主路径修复。
- 把不通过合同的 LLM 输出作为可执行 graph、accepted result、artifact、coverage evidence 或长期 memory。

## Design Intent

目标设计是：

- TUI 默认只暴露 `/run resume`，降低用户对 continue/restart 的理解成本。
- `/run resume` 表示恢复或推进当前 workspace 的当前有效 run；它不自动重建 run。
- 显式 restart/reset 仍保留，但必须是用户明确选择的 destructive / new-run 操作。
- 所有启动类命令进入后端后统一走 EventHub command -> intent resolution -> gap confirmation -> scheduler。
- 首次执行、resume、replan、restart 都必须先确认当前 Goal、DoD、已完成 evidence 和未关闭 gap，再生成下一步 todo baseline。
- 首次 LLM 状态变更前必须完成 deterministic gap baseline；LLM 不应在缺少当前 gap/todo 的情况下直接生成文件或计划。
- LLM 违例输出允许被记录为 rejected diagnostic evidence，并将结构化违例原因反馈给下一轮重试；违例输出不得产生 state-changing side effect。
- workorchd 是服务端 ViewModel owner；Web/TUI 只展示投影和提交 command，不自行推导 gap、todo 或状态机。
- WorkOrch core 只承载通用 run intent、gap/coverage/todo contract、状态机和事件流；具体业务推进策略由 workspaceTemplate、Soul、profile policy 和 skill 承载。

## Command Semantics

### `/run resume`

`/run resume` 是 TUI 侧默认推荐命令。语义是“恢复或继续当前 workspace”。

行为规则：

| Current state | Resume behavior |
| --- | --- |
| no run exists | 基于最新 Goal/DoD/template 创建首个 run，并执行 gap confirmation |
| idle with recoverable run | 重新激活当前有效 run，并执行 gap confirmation |
| active / executing | 幂等返回当前运行状态，不重复调度 |
| blocked_on_decision | 若 decision 已 resolution 且可继续，则进入 gap confirmation；否则提示先处理 decision |
| blocked_on_approval | 若 approval 已批准且可继续，则进入 gap confirmation；否则提示先处理 approval |
| failed but recoverable | 基于当前 run 的失败 surface 和 coverage ledger 执行 gap confirmation，再决定 repair/replan/decision |
| terminal completed | 返回已完成状态；除非用户显式 restart/reset，否则不创建新 run |

幂等要求：

- 同一 `workspaceId + activeRunId + intent=resume + requestId/dedupeKey` 只能产生一个 accepted work item。
- 已有同 scope work item 正在 pending/running 时，重复 resume 返回 accepted/current state，不创建重复 step。
- resume 不得自动 supersede 当前 run。

### `/run restart`

`/run restart` 是显式重建命令，不作为 TUI 常用提示展示。

行为规则：

- 关闭或 supersede 当前 active run。
- 基于 workspace 最新 Goal/DoD/template 和当前文件状态创建 replacement run。
- 对 replacement run 执行 gap confirmation。
- restart 必须携带 `requestId` 或等价 dedupe key。
- TUI 侧应要求用户确认，或调整为更明确的 `/run reset` / `/run restart --new-run`。

幂等要求：

- 同一 `workspaceId + intent=restart + requestId` 只允许创建一个 replacement run。
- 如果 restart work item 已 accepted，重复命令返回该 work item / replacement run 状态。
- restart 不能复用旧 run 继续执行；旧 run 必须进入明确终态，例如 `superseded`、`cancelled` 或 `restarted`。

### 旧推进命令

旧推进命令 不再作为用户命令和内部语义。

收口规则：

- TUI/Web/CLI 文档、help、prompt 自动提示统一展示 `/run resume`。
- REST/API、port DTO、EventHub contract、scheduler、projection 不得出现 continue intent 或 legacy resume alias 状态。
- 如果实现为了迁移期临时接受 旧推进命令 输入，只能在命令解析入口立即归一化为 `/run resume`，并且不得新增 legacy alias 测试作为长期验收目标。
- 旧脚本或测试若仍使用 continue，应在收口阶段改为 resume。

## Problem Findings

本轮核对结果需要被设计直接吸收：

1. `continue` 与 `restart` 在 TUI 中容易被误解。用户只是希望“继续推进当前 workspace”时，不应被迫判断是否需要新 run、是否会丢弃旧 run、是否会重复提交。
2. 多个 workspace 同时执行时，端侧命令必须是幂等 command，不应由 TUI/Web 直接推断是否可继续执行。
3. 当前执行慢、长期无有效文件产出的核心症状，不应通过在 core 中加入业务场景特判解决；应通过启动前 gap baseline、当前 focus、requiredOutput、rejected diagnostic feedback 和模板策略共同约束。
4. 当 LLM 输出违反合同，直接进入 decision 或失败会造成停滞；但直接接受违例又会破坏状态机。正确行为是将违例作为 rejected diagnostic evidence，反馈给 LLM 进行 bounded retry。
5. 首轮执行前如果没有明确“当前 Goal/DoD 已完成什么、缺什么、下一步必须产出什么”，LLM 容易先生成偏差产物，后续 repair 会围绕偏差产物反复收敛失败。
6. TUI/Web 只能显示 workorchd 投影，不能持有自己的 run 状态机、decision 状态机或 gap/todo 计算逻辑。

## Unified Command Event

所有端侧命令最终应归一为一个中立 command：

```json
{
  "workspaceId": "ws-...",
  "intent": "resume",
  "requestId": "client-or-server-generated-id",
  "source": "tui|web|cli|agent|watchdog",
  "reason": "user_requested|decision_resolved|approval_resolved|service_recovery|definition_changed",
  "expectedRunId": "optional-active-run-id",
  "definitionRevision": "optional-known-definition-revision"
}
```

`intent` 取值：

| Intent | Meaning |
| --- | --- |
| `resume` | 恢复或推进当前有效 run |
| `restart` | 显式关闭旧 run 并创建 replacement run |
| `replan` | 基于 decision resolution 或用户明确 replan 重新规划当前 run |
| `recover` | watchdog 或服务恢复产生的结构化恢复请求 |

命名建议：

- Event topic owner: `application/workorchd/pkg/events`
- Command name: `WorkspaceRunRequested` 或 `WorkspaceExecutionRequested`
- Payload 字段必须携带 workspace scope。
- 正式 run/step 状态变更仍由 `kernel/run`、`kernel/orchestrator` owner event 写入。

## Target State Flow

### Resume Flow

```text
TUI/Web
  -> /run resume
  -> backend port command
  -> REST/EventHub command adapter
  -> workorchd WorkspaceRunRequested(intent=resume)
  -> intent resolution
  -> gap confirmation
  -> scheduler enqueue next step / no-op / decision / approval
  -> projection changed
  -> TUI/Web refresh from ViewModel
```

### Restart Flow

```text
TUI/Web
  -> explicit restart/reset with confirmation
  -> WorkspaceRunRequested(intent=restart)
  -> workorchd validates scope and dedupe
  -> supersede/cancel current active run through owner command
  -> create replacement run
  -> gap confirmation
  -> scheduler enqueue first step
  -> projection changed
```

### Decision Replan Flow

```text
Decision resolved as replan
  -> Decision owner writes resolution
  -> workorchd receives resolved event
  -> WorkspaceRunRequested(intent=replan, reason=decision_resolved)
  -> gap confirmation
  -> scheduler enqueue replan step
```

The important rule is that decision resolution, resume, restart, and recovery all enter the same gap-confirmation gate before scheduling LLM/runtime work.

## Gap Confirmation

Gap confirmation is a service-side baseline step that computes the current closure state before scheduling the next LLM/runtime action.

### Inputs

Gap confirmation must read or reconstruct:

- latest Goal and DoD definition revision
- workspace template/profile/Soul binding
- profile deliverable expectations
- acceptance slice policy
- existing run executionContext
- coverage ledger / coverage work items
- accepted evidence refs
- rejected evidence refs
- latest runtime failure surface
- latest decision/approval resolution
- workspace file manifest or current output resource summary
- current run terminal/pending/dispatched step state

### Outputs

Gap confirmation produces a neutral `GapBaseline`:

```json
{
  "workspaceId": "ws-...",
  "runId": "run-...",
  "definitionRevision": "rev-...",
  "baselineRevision": "gap-...",
  "currentFocus": {},
  "openCoverageWorkItems": [],
  "acceptedEvidenceRefs": [],
  "rejectedEvidenceRefs": [],
  "todoItems": [],
  "nextAction": {
    "kind": "schedule_step|wait_decision|wait_approval|no_action|fail_fast",
    "reason": "..."
  }
}
```

`todoItems` 是调度侧和投影侧共享的中立结构，不是端侧 UI 文本：

```json
{
  "id": "todo-...",
  "source": "goal|dod|deliverable|runtime_failure|governance",
  "sourceRef": "goal.leaves[0]|dod.items[2]|deliverable:...",
  "coverageWorkItemIds": ["cwi-..."],
  "requiredEvidenceKinds": ["state_change", "validation", "assessment"],
  "status": "open|in_progress|blocked|satisfied|failed",
  "priority": 10,
  "reason": "structured neutral reason"
}
```

### Deterministic First

Gap confirmation must prefer deterministic reconstruction:

- If workspace is empty, mark all Goal/DoD/deliverable expectations open.
- If evidence ledger already proves an item, mark it satisfied or evidence_ready.
- If rejected evidence exists, preserve it as rejected evidence and do not erase it.
- If run is blocked_on_decision/approval, nextAction must be wait unless resolution is already present.
- If a dispatched runtime step has durable terminal result, nextAction should finalize/recover before creating new LLM work.

### LLM Assessment Only When Needed

LLM may be used only when deterministic data cannot decide whether current artifacts satisfy a Goal/DoD item.

LLM gap assessment constraints:

- `assessmentOnly=true`
- no `executionGraph`
- no `patch/apply`
- no shell/MCP/file/git executable output
- must output structured gap assessment only
- output must cite existing evidenceRefs/artifactRefs or mark item failed/skipped/open

This assessment step may not be used as a substitute for state-changing execution.

## First-Run Gap Baseline

首次执行不是特殊旁路，必须走与 resume/replan 相同的 gap confirmation，只是输入状态为空。

### First-Run Rules

- workspace 没有 run 时，`intent=resume` 创建首个 run，但不得立即调度 LLM 状态变更 step。
- 创建 run 后先建立 `GapBaseline`，其中所有 Goal/DoD/deliverable expectation 默认为 open，已有 workspace 文件或导入 evidence 可被 deterministic 评估。
- `GapBaseline.todoItems` 必须成为首个 LLM step 的 primary contract 输入。
- 首个 LLM step 的 `requiredOutput` 必须基于 open todo 和 profile/template 能力合同生成，不得只依赖 Goal/DoD 自然语言全文。
- 若 Goal/DoD 过大，core 只负责分块、优先级和引用追踪；具体业务优先级由 template/Soul/profile policy 通过结构化字段提供。

### Avoiding Early Output Drift

首轮偏差产出通常来自三类问题：

- LLM 看到完整 Goal 但没有看到明确 gap/todo，导致自行选择低价值输出。
- LLM 看到历史 memory 或旧 repair 诊断，但缺少当前 accepted/rejected evidence 和当前 requiredOutput。
- LLM 被过多禁止项压制，但没有足够明确的可选动作和最小可交付目标。

因此首轮 prompt/context 必须满足：

- 必须包含最新 Goal/DoD definition revision。
- 必须包含 `GapBaseline.baselineRevision`、open todo、currentFocus 和 required evidence kinds。
- 必须包含允许的 output kinds 和当前 requiredOutput。
- 必须明确“尽量输出小粒度文件/补丁，避免单文件过大”；该约束是通用产物粒度约束，不绑定具体语言或模板。
- 禁止输出合同只列不可做的事而不提供可做路径；每个禁止项应对应 allowed alternative 或 retry guidance。

## Contract Violation Feedback Retry

合同门禁继续严格执行，但违例处理从“直接阻塞”调整为“可诊断、可重试、无副作用”。

### Violation Classification

LLM 输出门禁失败时，runtime 必须生成结构化违例诊断：

```json
{
  "violationId": "violation-...",
  "workspaceId": "ws-...",
  "runId": "run-...",
  "stepId": "step-...",
  "baselineRevision": "gap-...",
  "ruleId": "output_contract.invalid_execution_graph",
  "severity": "retryable|terminal",
  "message": "neutral diagnostic message",
  "rejectedOutputRef": "content-ref-or-empty",
  "requiredCorrection": {
    "preserveIntent": true,
    "allowedOutputKinds": [],
    "requiredOutput": {}
  }
}
```

分类规则：

| Class | Behavior |
| --- | --- |
| retryable schema/contract violation | 记录 rejected diagnostic evidence，构造 bounded retry |
| executable output in assessment-only stage | 拒绝副作用，反馈 assessment-only 修正合同 |
| missing required state-changing output | 反馈 requiredOutput 和 current gap，要求重试 |
| repeated same violation beyond budget | 转入 decision 或 failed，并带完整 violation history |
| unsafe or destructive output | fail fast 或进入 approval/decision，不自动执行 |

### Retry Contract

违例重试必须继承原始 step 的阶段语义和 gap baseline：

- 不创建新的业务目标。
- 不丢失 `Goal/DoD definitionRevision`。
- 不覆盖 `GapBaseline.baselineRevision`，除非执行期间发生 definition change 或 accepted evidence 变化。
- 不接受违例输出中的 result/artifact/evidence。
- 只把违例摘要作为 C4 current-action diagnostic 注入下一轮，不写入长期高价值 memory。
- retry budget 必须按 `workspaceId + runId + originalStepId + ruleId + failureSignature` 计数，避免无限循环。

### Prompt Guidance Balance

为了提升容错性，prompt 合同应区分 strict contract 与 flexible guidance：

- strict contract：JSON schema、state-changing side effect、requiredOutput、assessment-only 禁止执行、approval 边界、workspace scope。
- flexible guidance：实现策略、任务分解顺序、工具选择偏好、产物粒度建议、验证路径建议。
- strict contract 失败只能走 rejected diagnostic retry；flexible guidance 偏离可以进入质量评估、repair 或 decision。
- core 只能定义中立 strict contract；template/Soul/profile policy 定义业务化 flexible guidance。

## Scheduler Decision Rules

After gap confirmation:

| GapBaseline nextAction | Scheduler behavior |
| --- | --- |
| `schedule_step` | Build convergence contract, create next step, publish runtime execute request |
| `wait_decision` | Set run `blocked_on_decision` and decision projection, keep workspace owner `active`, do not schedule LLM |
| `wait_approval` | Set run `blocked_on_approval` and approval projection; workspace owner may become `blocked_on_approval`, do not schedule LLM |
| `no_action` | Return idempotent accepted/current state |
| `fail_fast` | Mark run failed or create structured decision according to blocker class |

Scheduler must not create LLM/runtime work before GapBaseline is persisted or checkpointed.

## Run Command State Machine

统一后的命令状态机只在 workorchd/scheduler 侧存在，TUI/Web 不复制该逻辑。

```text
command accepted
  -> resolve workspace scope
  -> resolve active run
  -> resolve intent
  -> dedupe by scope/request
  -> create or reuse scheduler work item
  -> build/refresh GapBaseline
  -> decide nextAction
  -> publish owner command or no-op
  -> update projection
```

### Idempotency Keys

| Intent | Key |
| --- | --- |
| resume | `workspaceId + activeRunId + definitionRevision + requestId` |
| replan | `workspaceId + activeRunId + decisionId + resolutionRevision + requestId` |
| restart | `workspaceId + previousRunId + requestId` |
| recover | `workspaceId + runId + recoveryReason + failureSignature` |

如果端侧没有提供 `requestId`，workorchd 可以生成服务端 request id，但必须把它写入 accepted response 和 projection，便于端侧展示与追踪。

## REST / Port / TUI Mapping

### Port Contract

`internal/pkg/workorchport` 应提供一个端侧中立入口：

```go
type RunCommandIntent string

const (
    RunCommandResume  RunCommandIntent = "resume"
    RunCommandRestart RunCommandIntent = "restart"
    RunCommandReplan  RunCommandIntent = "replan"
    RunCommandRecover RunCommandIntent = "recover"
)

type WorkspaceRunCommand struct {
    WorkspaceID        string
    ExpectedRunID      string
    Intent             RunCommandIntent
    Reason             string
    Source             string
    RequestID          string
    DefinitionRevision string
    DecisionID         string
}
```

TUI/CLI 只构造该 port DTO，不直接调用 run owner 或 orchestrator owner。

### REST Contract

REST 可以保留资源化路径，但语义必须映射为统一 command：

| UI Action | REST behavior | Intent |
| --- | --- | --- |
| workspace start / no run | submit workspace run command | resume |
| run/resume | submit workspace run command | resume |
| run/restart | submit workspace run command | restart |
| decision replan | resolve decision then submit continuation command | replan |

REST handler 的同步返回只表示 command accepted/no-op/conflict，不表示 run 已完成。

Accepted response 必须暴露端侧可追踪的 command metadata，而不是伪装成 run 已经完成：

```json
{
  "accepted": true,
  "workspaceId": "ws-...",
  "runId": "run-...",
  "action": "resume",
  "intent": "resume",
  "requestId": "client-or-server-generated-id",
  "source": "web|tui|cli|agent|http",
  "reason": "user_requested|decision_resolved|approval_resolved|service_recovery",
  "definitionRevision": "optional-known-definition-revision",
  "decisionId": "optional-decision-id"
}
```

`action` 表达 HTTP endpoint 或 UI action，`intent` 表达进入 scheduler 的语义；后续调度、去重和投影必须以 `intent/requestId/workspaceId/runId` 为准。

### TUI Contract

TUI 侧命令展示：

- 默认提示：`/run resume`
- destructive：`/run restart`，需要明确说明会创建 replacement run
- decision 状态：prompt 来源必须是 projection 中的 blocker 状态；提交 replan 后等待 projection 更新，不在本地改状态

TUI 主视图、workspace 列表、prompt、状态栏必须消费同一份 ViewModel snapshot 或 stream event revision。

## Projection And MVVP

workorchd ViewModel must expose:

- current run intent status
- latest gap baseline revision
- current focus
- todo list summary
- open/failed/satisfied coverage counts
- blocked_on_decision/approval blockers
- runtime pending/finalize gap
- last accepted command and dedupe status
- latest violation diagnostics summary
- latest retry budget status

TUI/Web rules:

- TUI prompt must use ViewModel state.
- TUI help should show `/run resume` as default command.
- TUI may hide `/run restart` from default command suggestions or mark it destructive.
- Web buttons should map to resume/restart intent explicitly.
- Neither Web nor TUI recomputes Goal/DoD gap locally.

Projection 必须满足：

- workspace list 与 workspace detail 使用同一投影源。
- detail 投影未 ready 时，端侧显示 `projection refreshing`，但不得把旧 prompt 状态继续当成事实。
- command accepted 后，投影至少要出现 accepted command revision；后续 scheduler/step 进展再通过 event stream 逐步刷新。
- blocked_on_decision/approval 的 prompt 必须绑定具体 blocker id；blocker 关闭后 prompt 必须随投影清空或切回 resume。

## EventHub And Module Boundary

### Core Owns

- generic run intent command schema
- gap baseline model
- coverage/todo neutral fields
- scheduler state transition
- idempotency/dedupe
- projection fields
- LLM assessment-only contract

### Template / Soul / Policy Owns

- concrete todo prioritization rules for a workspace type
- concrete evidence requirements
- domain-specific verification guidance
- profile deliverable expectations
- capability preference and fallback policy
- user-facing wording for specialized workflows

Core must not hardcode template id, language, command, file name, provider, protocol, or a live workspace's Goal text.

## Implementation Scope

### Workorchd / Server

- Add unified run command endpoint/handler path for resume/restart/replan/recover intent.
- Remove continue intent from internal command, scheduler and projection contracts.
- Ensure REST/Web/TUI all produce the same command event.
- Ensure accepted response means command was validated and a BackgroundRoutine work item was accepted or no-op was determined.
- Ensure command handlers never perform long-running execution inline.
- Ensure service restart loads workspace projections and normalizes stale active/executing states to idle/recoverable unless a durable in-flight work item exists in the current process.

### Appservice / ViewModel

- Add GapBaseline projection fields.
- Add todo list projection fields.
- Add current command intent / dedupe status projection.
- Ensure workspace list/detail derive from the same projection source.
- Add latest contract violation diagnostic projection.
- Add latest accepted command revision and source.

### Orchestrator

- Add deterministic gap confirmation before scheduling next LLM/runtime step.
- Persist/checkpoint GapBaseline before scheduling.
- Build convergence contract from GapBaseline, not from stale run context alone.
- Preserve accepted/rejected evidence and latest definition revision.
- Add contract violation rejected diagnostic evidence and bounded retry input.
- Ensure violation retry cannot execute rejected state-changing output.

### TUI

- Update command parser and help:
  - default: `/run resume`
  - explicit destructive: `/run restart`
- Prompt and status bar must consume backend projection.
- Do not keep a second local run state machine.
- Ensure command submission waits for accepted/no-op/conflict response, then returns to projection-driven rendering.

### Web

- Map default start/resume button to `intent=resume`.
- Map restart/reset button to explicit `intent=restart` with confirmation.
- Display gap/todo projection from workorchd.
- Event stream disconnect must be retried without resetting local business state; the next snapshot must come from workorchd projection.

## Affected Code Inventory

本节列出后续逐一收口时必须核对的主要落点。清单用于定位责任边界，不表示每个文件都必须修改；若某项已经满足本文设计，只需补充测试或验收证据。

### Application Entry

| Path | Responsibility | Closure focus |
| --- | --- | --- |
| `application/workorch/main.go` | magicCommon framework application entry | 只按需注册 initiator/module/block/service，不承载 run 状态机、gap 计算或 TUI/Web 业务逻辑 |
| `application/workorch/*_test.go` | entry wiring tests | 验证 framework 注册路径，不验证具体 workspace 业务策略 |

### Port And Client Contracts

| Path | Responsibility | Closure focus |
| --- | --- | --- |
| `internal/pkg/workorchport/run.go` | CLI/TUI/agent 侧中立 run command DTO | 只保留 `resume/restart/replan/recover` 等明确 intent，避免端侧状态机和 legacy `continue` 语义 |
| `internal/pkg/workorchport/workspace.go` | workspace projection DTO | 确保端侧只消费 workorchd projection，不派生业务状态 |
| `internal/pkg/workorchclient/run.go` | REST client run command request/response | requestId、intent、expectedRunId、workspace scope 必须完整透传 |
| `internal/pkg/workorchclient/workspace.go` | REST client workspace request/response | restart/reset 必须显式映射为 replacement run，不伪装成 resume |

### Workorchd Event And Scheduler Path

| Path | Responsibility | Closure focus |
| --- | --- | --- |
| `internal/modules/application/workorchd/pkg/events/contracts.go` | workorchd command/event payload contract | 定义 workspace-scoped command event，避免无 scope run/ticket/approval 操作 |
| `internal/modules/application/workorchd/pkg/events/topics.go` | workorchd event topics | 统一 run requested topic，避免 TUI/Web/REST/decision 各自旁路调度 |
| `internal/modules/application/workorchd/service/server/http_run.go` | HTTP run command handler | 同步返回 accepted/no-op/conflict，不执行长任务；resume/replan/recover 进入统一 command path |
| `internal/modules/application/workorchd/service/server/http_workspace.go` | workspace activation/restart HTTP handler | restart 必须是显式 replacement，不能复用 resume 语义 |
| `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go` | run continuation scheduling bridge | 收口为 EventHub command 后的 scheduler bridge，不承担端侧语义推断 |
| `internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go` | scheduler command consumer | 统一处理 resume/replan/restart/recover，落实 workspace scope、dedupe 和 accepted projection |
| `internal/modules/application/workorchd/service/server/event_subscriptions.go` | service event subscriptions | 只订阅 owner event 并推动 projection/scheduler，不做业务特判 |
| `internal/modules/application/workorchd/service/server/service.go` | workorchd service lifecycle | 启动期恢复 stale active/executing projection，等待显式 resume |

### Appservice And Projection

| Path | Responsibility | Closure focus |
| --- | --- | --- |
| `internal/modules/application/workorchd/biz/appservice/activationports/*.go` | workspace/run activation command adapter | 创建或恢复 run 前必须保留 workspace scope、Goal/DoD revision 和 template/profile binding |
| `internal/modules/application/workorchd/biz/appservice/runports/*.go` | run owner command/read adapter | stale state recovery、failure/update command 必须幂等且不绕过 EventHub 主路径 |
| `internal/modules/application/workorchd/biz/appservice/runtimecore/*.go` | runtime continuation context | resume/replan/recover 必须携带 gap baseline、accepted/rejected evidence 和 current blocker |
| `internal/modules/application/workorchd/biz/appservice/readmodelcore/*gap*` | projection DTO | `GapBaseline`、todo、current focus、evidence refs 作为服务端 ViewModel 字段暴露 |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection/*.go` | read model projection builder | workspace list/detail/run snapshot 必须来自同一 projection source/revision |
| `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/*.go` | in-memory workspace ViewModel store | command accepted、blocker、gap baseline 和 workspace state merge 不能互相擦除 |

### Orchestrator And Runtime Contract

| Path | Responsibility | Closure focus |
| --- | --- | --- |
| `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go` | deterministic gap baseline model/building | 首次执行、resume、replan、restart、recover 都先生成或刷新 baseline |
| `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go` | next dispatch selection | 调度任何 state-changing step 前必须检查 baseline nextAction |
| `internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go` | event-driven dispatch entry | queued dispatch 不能绕过 gap gate 或 unresolved blocker |
| `internal/modules/kernel/orchestrator/biz/runtime_step_input.go` | LLM/runtime step input assembly | 从 baseline 构造 current focus、todo、requiredOutput、accepted/rejected evidence |
| `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go` | repair/retry input assembly | contract violation retry 必须继承原始 stage 语义和 baseline |
| `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go` | contract violation gate/recovery | 违例输出 rejected diagnostic 化，bounded retry，无副作用 |
| `internal/modules/kernel/orchestrator/biz/runtime_convergence*.go` | convergence budget and completion gate | 使用 coverage/todo/evidence contract 收敛，不加入模板/语言/文件名特判 |

### TUI / CLI / Web

| Path | Responsibility | Closure focus |
| --- | --- | --- |
| `internal/services/workorch/service/tui/commands.go` | TUI command parser/help | canonical UX 使用 `/run resume`，restart 标记 destructive |
| `internal/services/workorch/service/tui/input.go` | TUI input submission | 提交 command 后等待 accepted/no-op/conflict，再由 projection 驱动刷新 |
| `internal/services/workorch/service/tui/model.go` | TUI MVVP model | 不维护第二套 run/decision/gap 状态机 |
| `internal/services/workorch/service/tui/workspace_state.go` | TUI display state mapping | prompt 绑定 projection workspace/run/blocker/command revision |
| `internal/services/workorch/service/tui/view.go` | TUI rendering | workspace list/detail/status/prompt 使用同一 ViewModel snapshot |
| `internal/services/workorch/service/cli/run.go` | CLI run command | 复用 port DTO，不直接访问 workorchd internals |
| `internal/services/workorch/biz/backends/eventhub.go` | embedded backend | 通过 EventHub command 访问 owner state，不直接调 orchestrator/repository |
| `internal/services/workorch/biz/backends/remote.go` | remote backend | 通过 REST client 提交 workspace-scoped command |
| `web/workorch-console/src/api.ts` | Web REST API client | 默认执行按钮映射 resume intent，restart/reset 显式 destructive |
| `web/workorch-console/src/pages/*.tsx` | Web projection display pages | 只展示服务端 projection，不在前端重算 gap/todo/run 状态 |
| `web/workorch-console/src/hooks/useSSE.ts` | Web event stream | SSE 断线只触发重连和 snapshot reload，不重置业务状态 |

### Verification Assets

| Path | Responsibility | Closure focus |
| --- | --- | --- |
| `scripts/verify-run-resume-gap-confirmation-e2e.sh` | deployed-instance e2e | 覆盖 resume、repeat resume、restart recovery、decision replan 和 no-continue projection |
| `scripts/verify-boundary-contract.sh` | core/template boundary guard | 验证 core 没有新增业务/模板/语言特判 |
| `docs/50-product-interface/tui/interaction-contract.md` | TUI user contract | 与 `/run resume` canonical UX、MVVP projection 保持一致 |
| `docs/50-product-interface/service-api.md` | REST API contract | 与 workspace-scoped command、projection response、SSE retry 语义保持一致 |
| `docs/20-runtime-flow/repair-replan-flow.md` | repair/replan runtime flow | 与 decision replan -> command event -> gap baseline -> scheduler 主路径保持一致 |

## Todo Matrix

| ID | Area | Task | Verification |
| --- | --- | --- | --- |
| RG-01 | command contract | Define unified workspace run command intent and payload | unit test command decode/scope/dedupe |
| RG-02 | TUI | Rename default command/help from continue to resume; remove continue from canonical UX | TUI command parser/presenter tests |
| RG-03 | Web/REST | Map default execution action to resume intent | handler/client tests |
| RG-04 | workorchd scheduler | Route resume/restart/replan/recover through one EventHub command path | scheduler command accepted/no-op tests |
| RG-05 | orchestrator | Add deterministic GapBaseline before first LLM/runtime scheduling | orchestrator integration test for empty workspace |
| RG-06 | orchestrator | Add GapBaseline for resume after restart/service recovery | test with existing run/evidence ledger |
| RG-07 | orchestrator | Add assessment-only gap confirmation when deterministic data is insufficient | contract test rejects executable output |
| RG-08 | projection | Expose current focus, todo summary, gap baseline revision | read model snapshot tests |
| RG-09 | MVVP | Ensure TUI/Web only consume projection gap/todo | code review / import boundary search |
| RG-10 | restart | Make restart explicitly supersede old run and create replacement run | multi-run state transition test |
| RG-11 | resume | Make resume idempotent for active/executing/pending work | duplicate command test |
| RG-12 | decision replan | Route resolved replan through same gap confirmation path | decision resolution integration test |
| RG-13 | e2e | Add deployed-instance test for `/run resume` from idle/blocked_on_decision/service-restart | e2e script |
| RG-14 | docs | Update CLI/TUI user docs and architecture docs | docs review |
| RG-15 | first-run | Ensure first run creates GapBaseline before first state-changing LLM step | orchestrator/workorchd integration test |
| RG-16 | violation retry | Record contract violations as rejected diagnostics and retry boundedly | contract violation recovery tests |
| RG-17 | prompt balance | Split strict contract from flexible guidance in LLM step input | prompt rendering/cache tests |
| RG-18 | projection prompt | Bind TUI prompt to projection blocker id and accepted command revision | TUI presenter tests |
| RG-19 | restart recovery | Normalize stale active projection after service restart | server/appservice recovery tests |

## Acceptance Criteria

The design is implemented when:

- TUI default prompt/help shows `/run resume`.
- 旧推进命令 is removed from canonical UX and never appears as an internal intent.
- `/run restart` is explicit destructive restart and does not silently behave like resume.
- Every resume/restart/replan/recover command enters a single EventHub command path.
- GapBaseline is created or refreshed before the first LLM/runtime step after command acceptance.
- LLM receives current Goal/DoD, accepted/rejected evidence, open gap, current focus and requiredOutput in a consistent context.
- Web/TUI display the same server-side todo/gap projection.
- Duplicate resume does not create duplicate run/step.
- Restart creates a replacement run and marks previous active run terminal/superseded.
- First run cannot schedule a state-changing LLM step until GapBaseline exists.
- Contract violation output is rejected, visible as diagnostic, and can drive bounded retry without side effects.
- Repeated same violation stops with structured decision/failed state instead of infinite repair.
- TUI prompt changes after blocker resolution because projection changed, not because TUI guessed a state transition.
- No core production code adds template-specific or language-specific special cases.

## Verification Commands

Minimum verification after code收口:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/services/workorch/service/tui -count=1
./scripts/verify-boundary-contract.sh
go test ./... -count=1
```

If LLM request rendering changes:

```bash
go test ./internal/modules/blocks/llm/biz -count=1
./scripts/verify-llm-cache-aware-request-log.sh <workspaceId>
```

## Open Questions

- Whether `GapBaseline` should be a formal kernel owner model or a workorchd/orchestrator checkpoint first. The initial implementation can store it as run executionContext/checkpoint plus projection, then promote to owner model if multiple modules need direct command access.
- Whether restart command should be named `/run restart`, `/run reset`, or `/run restart --new-run` in TUI. The backend intent should remain `restart` regardless of UI wording.
- Whether assessment-only gap confirmation should use the normal LLM block or a dedicated orchestrator validation stage. The output contract must remain executable-output forbidden in either case.

## Migration Order

建议按以下顺序收口，避免再次出现端侧状态、后端状态和 LLM 合同不同步：

1. 收口 TUI/Port/REST 命令语义：默认 `/run resume`，移除内部 `continue` 语义，restart 显式 destructive。
2. 收口 workorchd command event：所有 resume/replan/restart 进入统一 EventHub command 和 scheduler work item。
3. 收口 projection：workspace list/detail/prompt 使用同一 ViewModel，新增 accepted command、blocker id、gap baseline revision。
4. 收口 deterministic GapBaseline：首次执行、resume、replan 都先生成 baseline，再调度 LLM/runtime。
5. 收口 LLM step input：从 GapBaseline 构造 current focus、todo、requiredOutput、accepted/rejected evidence。
6. 收口 violation feedback retry：违例输出 rejected diagnostic 化，bounded retry，不产生副作用。
7. 收口 restart/service recovery：重启后 stale active 状态归一为 idle/recoverable，等待显式 resume。
8. 补充 e2e：覆盖 idle resume、blocked_on_decision replan、service restart 后 resume、重复 resume 幂等、违例 retry 后成功推进。

## Boundary Audit Notes

本文涉及的 core 调整必须保持业务中立：

- 不在 core 中根据模板 ID、Goal 文本、文件名、语言、命令、provider 或具体 workspace ID 分支。
- GapBaseline 只表达通用 coverage/todo/evidence/blocker；模板业务优先级通过 workspaceTemplate/Soul/profile policy 的结构化字段输入。
- “避免生成过大文件”是通用产物粒度和上下文可维护性约束，可以写入通用 guidance；具体单文件大小阈值或语言拆分规则应由 profile/template 提供。
- “允许违例后反馈重试”是通用合同恢复机制；具体哪些业务输出更优、哪些验证命令合适，仍由 template/Soul/policy 决定。
- TUI/Web 不拥有正式状态，不根据文本推断 waiting/running/decision，只显示投影。

## Review-To-Closure Mapping

本节把近期核对结果映射成后续代码收口项。后续每次修改都应说明命中哪个条目，避免继续围绕单个 live workspace 现象反复试探。

### P1: 命令语义不统一

现象：

- 用户希望继续推进当前 workspace，但 旧推进命令、`/run restart`、decision replan 和 Web start 的语义不一致。
- 部分路径只改变端侧提示或 projection，没有真正进入服务端调度队列。
- 多 workspace 时，某个 workspace command accepted 后，另一个 workspace 的 prompt 或状态可能沿用旧本地状态。

设计收口：

- 端侧常规推进命令统一为 `/run resume`。
- 旧推进命令 不再作为 canonical UX 或内部 intent。
- `/run restart` 只表示 replacement run，必须关闭或 supersede 旧 active run。
- TUI/Web/CLI/agent 只提交 `WorkspaceRunCommand`，由 workorchd 通过 EventHub command 统一处理。
- command accepted 必须投影 `intent/requestId/source/revision`，端侧不得自行推断 running/waiting 状态。

验收：

- 搜索生产代码不得出现内部 legacy resume alias、`legacy_resume_alias`、`Mode=continue`。
- TUI help、prompt、suggestion 默认只出现 `/run resume`。
- 重复 resume 对同一 running/dispatched workspace 幂等，不创建重复 step。
- decision replan 后必须通过同一 command path 重新进入 gap confirmation。

### P2: 首次执行缺少 GapBaseline

现象：

- workspace 首轮 LLM 调用前，模型看到 Goal/DoD，但缺少当前 gap、current focus、requiredOutput 和 accepted/rejected evidence。
- 首轮输出容易偏离目标，后续 repair 围绕偏差产物反复修复，难以回到真实 goal。

设计收口：

- 没有 run 或没有 accepted evidence 也必须先生成 `GapBaseline`。
- `GapBaseline` 必须在第一个 state-changing step 前持久化或 checkpoint。
- 首个 LLM step input 必须从 `GapBaseline.todoItems/currentFocus/requiredEvidenceKinds` 构建，而不是只拼接 Goal/DoD 原文。
- 若确定性数据不足，只允许 assessment-only gap confirmation，不允许产生 state-changing output。

验收：

- empty workspace 首次 resume 的集成测试证明 first state-changing step 前已有 baseline revision。
- LLM step input 包含 baseline revision、open todo、requiredOutput、accepted/rejected evidence。
- assessment-only 阶段返回 executable graph、patch、file write、shell/MCP/file side effect 时被拒绝并进入 violation retry。

### P3: LLM 违例导致停滞或错误阻塞

现象：

- LLM 返回字段不稳定、缺少 required output、返回 forbidden output 或把 assessment 与 execution 混合时，run 容易直接进入 decision/failed。
- 如果放宽合同接受违例，又会破坏状态机和 evidence ledger。

设计收口：

- 合同门禁保持严格，但违例输出允许作为 rejected diagnostic evidence 记录。
- 可恢复违例进入 bounded retry，retry 继承原始 step 语义、GapBaseline、Goal/DoD revision 和 requiredOutput。
- 违例输出不得执行，不得成为 accepted result/artifact/evidence，不得写入长期高价值 memory。
- 重复同类违例超过预算后进入 structured decision 或 failed，禁止无限循环。

验收：

- 合同违例单测覆盖 schema violation、assessment-only side effect、missing required state-changing output。
- rejected diagnostic 可在 projection/request context 中追溯，但不会进入 accepted evidence/result/artifact。
- retry budget 按 `workspaceId + runId + originalStepId + ruleId + failureSignature` 收敛。

### P4: DoD 大导致收敛慢

现象：

- DoD 条目较多时，runtime 容易把任务视为 large DoD，产生过宽上下文和发散计划。
- 仅靠禁止项或更严格 prompt 不能促进更快文件产出。

设计收口：

- core 只负责把 Goal/DoD/deliverable expectations 切成中立 coverage work items 和 todo。
- current focus 必须来自可追踪 todo baseline，并在每轮 step input 中明确当前必须关闭的 gap。
- 业务优先级、验证偏好、交付节奏由 workspaceTemplate/Soul/profile policy 通过结构化字段提供。
- core 可以提供通用小粒度产物 guidance，避免单个输出过大；具体阈值和语言拆分规则不进入 core。

验收：

- 多条 DoD 的 run input 能展示稳定 work item id、status、required evidence kinds。
- core 中没有根据模板名、语言、文件名、命令、Goal 文本或 DoD 文本进行优先级分支。
- profile/template policy 可以影响 priority/focus，但 runtime 只消费结构化字段。

### P5: MVVP 投影与 TUI prompt 不一致

现象：

- 切换 workspace 后，主视图、workspace 列表、状态栏和 prompt 可能来自不同 revision。
- decision/replan 后端状态已变化，但 TUI prompt 仍显示旧 blocker 或旧 command 状态。

设计收口：

- workorchd 是 ViewModel owner，workspace list/detail/prompt/blocker/command/gap/todo 必须来自同一投影源。
- TUI/Web 不保留第二套 run/decision/approval 状态机。
- prompt 必须绑定 projection 中的 workspace id、active run id、blocker id、accepted command revision。
- detail projection 未 ready 时只能显示 refreshing，不得继续把旧 prompt 当成事实。

验收：

- TUI presenter/model 测试覆盖 workspace switch、decision replan accepted、blocker closed、projection refreshing。
- projection store 测试证明 snapshot upsert 不会擦除刚接受的 command revision。
- Web/TUI 端没有根据本地 command submit 成功直接切换业务状态。

### P6: 服务重启和恢复语义

现象：

- workorchd 重启后，历史 projection 可能仍显示 running，但当前进程没有对应执行事件。
- 依赖 watchdog 补偿会掩盖主路径 event 丢失问题。

设计收口：

- 服务启动加载 workspace projection 后，若不存在当前进程内 durable in-flight work item，stale active/executing 必须归一为 idle/recoverable，等待显式 `/run resume`。
- watchdog 只处理外部调用长期无返回、可幂等补偿的健康检查场景；不得替代正常 EventHub command 主路径。
- 运行中 event 丢失必须定位丢失原因并修复，不用 watchdog 作为常态补偿。

验收：

- service restart 测试证明 stale active 归一为 idle/recoverable。
- `/run resume` 可以重新激活 recoverable workspace，并先进入 gap confirmation。
- watchdog 相关测试只覆盖外部调用 timeout / stale executing 的幂等恢复，不覆盖普通 command 投递失败。

## Code Closure Guardrails

后续代码收口必须遵守以下 guardrails：

- 先修主路径，再考虑补偿路径。replan/resume/restart 必须能通过 EventHub command 正常入队和调度。
- 每个 command 必须具备 workspace scope、intent、requestId、source、reason；无 requestId 时由 workorchd 生成并投影。
- 所有长期运行动作不得在 HTTP handler、TUI submit 或 REST client 同步链路内执行；同步返回只表达 accepted/no-op/conflict。
- `GapBaseline` 是服务端执行合同，不是 UI 文案；TUI/Web 只能展示摘要。
- prompt/context/cache 改动不得把运行期动态字段放入 C0 stable prefix。
- contract violation recovery 只能把违例诊断作为 C4 current-action 输入，不写入长期高价值 memory。
- 任何 core 逻辑都不得引入具体模板、语言、文件名、命令、provider 或 live workspace id 特判。

## Required Evidence For Each Closure PR

每个后续收口提交至少需要提供以下证据：

1. 命中条目：说明修改对应 P1-P6 或 RG-xx 的哪一项。
2. 主路径证明：说明旧失败路径进入哪个函数，现在会得到什么 command/projection/baseline/step 输出。
3. 边界证明：说明改动属于 core 通用能力还是 template/Soul/profile policy，未把业务策略写入 core。
4. 测试证明：列出受影响包测试、边界搜索和必要 e2e。
5. Live 验证证明：若用于修复 live workspace，说明 workspace 当前状态、最后事件、修复后下一步应出现的事件或投影变化。

## Closure Phase Plan

后续代码收口按以下阶段推进。每个阶段完成后需要更新本文或对应 follow-up 文档中的验收状态，不能只凭 live workspace 表象判断完成。

### Phase A: Command Contract

目标：

- `/run resume` 成为 TUI/Web/CLI 默认推进命令。
- 旧推进命令 从 canonical UX、port、REST、EventHub、scheduler 和 projection 中退出。
- `/run restart` 仅表示 replacement run，不再承担普通继续执行语义。

必须完成：

- `internal/pkg/workorchport` 定义唯一端侧 run command DTO。
- `internal/pkg/workorchclient` 映射 REST request/response，不引入端侧状态机。
- TUI command parser、help、prompt 和 suggestion 统一展示 `/run resume`。
- Web 默认启动/继续按钮映射 `intent=resume`，restart/reset 显式确认。
- 所有命令携带 `workspaceId`、`intent`、`requestId`、`source`、`reason`。

验收证据：

- production search 无内部 legacy resume alias、`legacy_resume_alias`、`Mode=continue`。
- TUI tests 覆盖 `/run resume` 成功、`/run restart` destructive、旧推进命令 非 canonical。
- REST/client tests 覆盖 requestId 透传和 accepted command projection。

### Phase B: EventHub Scheduler Path

目标：

- TUI/Web/REST/decision resolution 只产生 command event。
- workorchd 基于 EventHub command 驱动 scheduler work item。
- HTTP/TUI 同步链路只返回 accepted/no-op/conflict，不执行长任务。

必须完成：

- resume/replan/restart/recover 进入统一 `WorkspaceRunRequested` 或等价 command topic。
- scheduler 按 workspace scope 和 idempotency key 去重。
- command accepted 后立即投影 accepted command revision。
- running/dispatched workspace 重复 resume 返回当前状态，不重复创建 step。

验收证据：

- scheduler tests 覆盖多 workspace 并发 command，不串错 workspace。
- decision replan resolved event 能投递 `intent=replan` 并进入同一 scheduler path。
- HTTP handler tests 证明长任务不在 handler 中同步执行。

### Phase C: GapBaseline Gate

目标：

- 首次执行、resume、replan、restart、recover 都先生成或刷新 `GapBaseline`。
- `GapBaseline` 成为 scheduler、orchestrator、LLM input、projection 的同一收敛合同。

必须完成：

- empty workspace 首次 resume 创建 run 后先 checkpoint baseline，再调度 state-changing step。
- existing run resume 复用最新 definition revision、accepted/rejected evidence 和 coverage ledger。
- blocked_on_decision/approval 只有 resolution 已满足时才进入 baseline/schedule，否则 nextAction 为 wait。
- deterministic 不足时仅允许 assessment-only gap confirmation。

验收证据：

- orchestrator/workorchd integration tests 证明 first state-changing step 前已有 baseline revision。
- run snapshot/workspace snapshot 都能投影 baseline revision、todo summary、current focus。
- assessment-only 阶段的 executable output 被合同门禁拒绝并进入 rejected diagnostic。

### Phase D: LLM Contract And Violation Retry

目标：

- LLM prompt/context 在每轮执行中携带当前 gap/todo/requiredOutput。
- 合同违例可诊断、可 bounded retry，但不产生副作用。

必须完成：

- LLM step input 从 `GapBaseline` 构建 current focus、allowed output kinds、requiredOutput 和 evidence refs。
- rejected diagnostic 只进入当前恢复上下文，不进入 accepted evidence/result/artifact 或长期高价值 memory。
- retry budget 按 workspace/run/original step/rule/failure signature 收敛。
- repeated violation 超预算进入 structured decision 或 failed。

验收证据：

- contract violation recovery tests 覆盖 schema violation、assessment-only side effect、missing required state-changing output。
- prompt/cache tests 证明动态 violation/current action 不污染 C0 stable prefix。
- projection 能显示 latest violation diagnostic summary。

### Phase E: MVVP Projection

目标：

- workorchd 是 workspace list/detail/status/prompt/gap/todo 的唯一 ViewModel owner。
- TUI/Web 只消费 projection，不维护第二套业务状态。

必须完成：

- workspace list 和 detail 来自同一 projection store/revision。
- TUI prompt 绑定 workspace id、run id、blocker id、accepted command revision。
- blocker 关闭、decision replan、approval approve 后，prompt 只能随 projection 更新。
- projection detail 未 ready 时端侧显示 refreshing，不复用旧业务 prompt。

验收证据：

- TUI model tests 覆盖 workspace switch、blocked_on_decision、replan accepted、blocker closed、projection refreshing。
- projection store tests 覆盖 snapshot merge 不擦除 accepted command/gap baseline。
- Web/TUI 代码搜索无端侧 gap/todo/run 状态推断。

### Phase F: Recovery And E2E

目标：

- 服务重启后 stale active/executing 归一为 idle/recoverable，等待显式 resume。
- watchdog 只处理外部调用超时等幂等补偿，不替代主路径。
- deployed e2e 覆盖关键路径。

必须完成：

- startup projection recovery 不保留无当前执行事件的 active 假象。
- recoverable workspace resume 后先进入 GapBaseline gate。
- watchdog 只覆盖外部调用长期无返回、worker lost、stale executing 等可幂等场景。
- e2e 覆盖 idle resume、blocked_on_decision replan、service restart 后 resume、重复 resume 幂等。

验收证据：

- service restart tests 证明 stale active -> idle/recoverable。
- deployed-instance e2e 证明 `/run resume` 和 decision replan 都能触发 command projection 与后续调度。
- CPU/IO 观测证明不再因端侧轮询或反复全量扫描导致持续高负载。

## Document Usage

后续代码收口以本文为主设计文档：

- 需求和设计决策引用 `D-01` 到 `D-07`。
- 现象到收口项引用 `P1` 到 `P6`。
- 具体任务拆分引用 `RG-01` 到 `RG-19`。
- 实施顺序引用 `Phase A` 到 `Phase F`。

每一轮代码收口完成后，应在提交说明或核对记录中明确：

- 命中哪个 `P` / `RG` / `Phase` 条目。
- 修改落点属于 core 通用能力还是 template/Soul/profile policy。
- 是否影响 EventHub command 主路径、GapBaseline gate、LLM step input、contract violation retry 或 MVVP projection。
- 已执行哪些单测、边界搜索和必要 e2e。

本文不替代以下设计文档：

- EventHub 主路径和状态循环以 [eventhub-driven-workorchd-state-loop-design.md](eventhub-driven-workorchd-state-loop-design.md) 为准。
- runtime 执行队列和异步发布以 [runtime-execution-event-queue-state-loop-design.md](runtime-execution-event-queue-state-loop-design.md) 为准。
- TUI/Web 投影边界以 [mvvp-workspace-viewmodel-architecture.md](mvvp-workspace-viewmodel-architecture.md) 为准。
- LLM 违例恢复以 [contract-violation-recovery-loop-design.md](contract-violation-recovery-loop-design.md) 为准。

若这些文档与本文在 `/run resume`、GapBaseline gate、违例 rejected diagnostic retry 或 TUI/Web 只消费 projection 的原则上出现冲突，应先同步刷新对应文档，再进行代码收口。

## Closure Notes

### 2026-06-21 Phase A/B Unified Command Contract Closure

命中范围：

- `Phase A: Command Contract`
- `Phase B: EventHub Scheduler Path`
- `RG-01`: unified workspace run command intent and payload
- `RG-02`: TUI default command is `/run resume`
- `RG-03`: REST/client default execution action maps to `resume`
- `RG-04`: resume/restart/replan/recover route through one scheduler command path
- `RG-10`: restart is explicit replacement execution
- `RG-11`: command accepted projection and duplicate-safe command metadata
- `RG-12`: decision/replan continuation uses the same command projection

收口结论：

- `internal/pkg/workorchport.WorkspaceRunCommand` is the shared port DTO for `resume`, `restart`, `replan` and `recover`.
- Embedded EventHub backend publishes all supported intents to `TopicWorkorchDCommandWorkspaceRunRequested`.
- Remote backend now also preserves the unified command intent for `replan` and `recover` by sending it through the workspace-scoped run command HTTP path; it no longer rejects non-destructive non-resume intents while embedded mode accepts them.
- HTTP run command handling accepts `resume`, `replan` and `recover` intents on the run command path and rejects unsupported/destructive intents there; `restart` stays on the explicit workspace restart path.
- HTTP accepted responses now return command metadata (`intent`, `requestId`, `source`, `reason`, `definitionRevision`, `decisionId`) so TUI/Web/CLI can trace the accepted command without treating the response as completed run state.
- CLI and agent resume paths now route through the same `WorkspaceRunCommand` port DTO and require workspace scope.
- Web default execution requests now include `intent=resume`, generated `requestId`, `source=web` and `reason=user_requested`; recovery labels use `resume_run` / `resume_validation` instead of `continue_*`.
- HTTP workspace restart handling projects `intent=restart` with command metadata and queues workspace restart through the same scheduler topic; the appservice restart path supersedes the old active run and starts a replacement run.
- Accepted command projection can now create a detail placeholder for a known workspace when the detail projection has not been built yet. This ensures command accepted status is visible to TUI/Web as soon as the scheduler accepts the command, instead of depending on a completed detail rebuild.
- Unknown workspace command projection is still rejected, preserving workspace scope safety.

验证证据：

```bash
go test ./internal/services/workorch/biz/backends -run 'TestRemoteRunCommand(ResumeSendsRequestID|ReplanSendsUnifiedIntent)|TestEventHubRunRestartPublishesUnifiedWorkspaceRunCommand' -count=1
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkorchD(WorkspaceRestartHTTPProjectsRestartIntent|RunResumeHTTPProjectsReplanIntent|RunResumeHTTPReturnsGeneratedRequestID|RunResumeHTTPRejectsRestartIntent|ActivationHTTPProjectsRequestID|WorkspaceRunCommandResumeEnqueuesRunContinuation|WorkspaceRunCommandRestartEnqueuesWorkspaceRestart)|TestWorkorchDSchedulerWorkspaceRunCommandEnqueuesWorkspaceRestartByWorkspaceScope|TestDecisionResolveEndpointProjectsReplanRunCommand' -count=1
go test ./internal/services/workorch/biz/agent ./internal/services/workorch/service/cli -count=1
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -run 'TestStoreRunCommandAccepted(CreatesDetailPlaceholderForKnownWorkspace|ProjectsCommandRevision|RejectsUnknownWorkspace|PreservesAcceptedRunCommand)|TestStoreUpsertSnapshotPreservesAcceptedRunCommand' -count=1
go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivateWorkspaceExecutionWithBackendRestartReplacesCreatedActiveRun' -count=1
./scripts/verify-web-console-projection-visual.sh
./scripts/verify-boundary-contract.sh
git diff --check
```

### 2026-06-21 Phase C Gate Closure

命中范围：

- `Phase C: GapBaseline Gate`
- `RG-05`: deterministic `GapBaseline` before scheduling
- `RG-08`: run/workspace projections expose baseline revision, current focus and todo summary
- `RG-07`: scheduler must respect assessment/wait/no-action gate before executable work
- `RG-15`: first run persists `GapBaseline` before first state-changing runtime step

收口结论：

- Orchestrator dispatch now consumes `GapBaseline.nextAction` before creating or dispatching runtime steps.
- `wait_decision` and `wait_approval` return blocked without scheduling new LLM/runtime work.
- `no_action` returns no dispatch specs.
- `fail_fast` and unknown next actions are treated as conflicts.
- Event-driven `DispatchRequestedStep` also honors the same gate, so a queued dispatch command cannot bypass an unresolved blocker.
- Read-model projection exposes `GapBaseline` on run and merged workspace snapshots, including baseline revision, current focus, todo items, accepted evidence refs and rejected evidence refs.
- Client/port snapshot decoding preserves the same gap baseline evidence refs for TUI/Web consumers.

验证证据：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_(NextDispatchSpecsHonorsGapBaselineWaitAction|DispatchRequestedStepHonorsGapBaselineWaitAction|NextDispatchSpecsHonorsGapBaselineTerminalActions|DispatchRequestedStepHydratesGapBaseline|NextDispatchReusesActiveLLMWorkspaceExecution)|TestNextDispatchSpecsPersistsGapBaselineBeforeInitialStep|TestGapBaselineFromRunPreservesAcceptedAndRejectedEvidenceRefs|TestRuntimeOrchestrator_ResumeRunResolvedDecisionReplanSuccessorCarriesGapBaseline' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestRunSnapshotFromModels(ProjectsBlockingDecisionStepAndCoverage|PrefersPersistedGapBaseline)|TestWorkspaceSnapshotWithRunSnapshotMergesRunProjection' -count=1
go test ./internal/services/workorch/biz/backends -run 'TestDecodeSnapshotInfoPreservesGapBaselineEvidenceRefs' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

### 2026-06-21 Phase D Contract Violation Baseline Closure

命中范围：

- `Phase D: LLM Contract And Violation Retry`
- `RG-16`: contract violation rejected diagnostic and bounded retry
- `RG-17`: retry input preserves current gap/todo/required output context

收口结论：

- Contract violation retry continues through the normal runtime repair input path.
- Runtime repair failure projection now preserves `GapBaseline` from the failed step when the run has not already supplied a newer baseline.
- If the run execution context already carries a `GapBaseline`, it remains the authoritative latest baseline and is not overwritten by the failed step.
- This keeps retry input aligned with the same coverage/todo baseline that produced the rejected output, without accepting or executing that rejected output.

验证证据：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolation|TestRuntimeRepairInputIncludesContractViolationRecovery' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

### 2026-06-21 Phase D/E Contract Violation Projection Closure

命中范围：

- `Phase D: LLM Contract And Violation Retry`
- `Phase E: MVVP Projection`
- `RG-16`: contract violation rejected diagnostic and bounded retry
- `RG-18`: prompt/status use projection blocker and diagnostic state

收口结论：

- `RuntimeExecutionState` now remains visible when the only available runtime signal is a `contractViolation` diagnostic from the failed step output or retry step input.
- A terminal failed LLM step that was rejected by the runtime contract no longer disappears from the run/workspace projection just because it has no runtime execute checkpoint.
- Workspace detail merging now preserves `runtimeExecution.contractViolation` from the run snapshot, so workspace detail, workspace list-derived state and TUI/Web status rendering continue to share the same server-side diagnostic.
- Remote/TUI backend snapshot decoding preserves `runtimeExecution.contractViolation`, so TUI/Web can display the same server-side diagnostic instead of inferring failure state locally.
- This keeps rejected diagnostic evidence visible while still preventing the rejected output from becoming accepted result, artifact, coverage evidence or long-term high-value memory.

验证证据：

```bash
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui -run 'TestRunSnapshotFromModelsProjectsContractViolationWithoutRuntimeProgress|TestWorkspaceSnapshotWithRunSnapshotMergesRunProjection|TestDecodeSnapshotInfoPreservesRuntimeExecutionContractViolation|TestRuntimeExecutionStatusTextShowsContractViolationProjection' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui -count=1
```

### 2026-06-21 Phase E Workspace Projection State Sync Closure

命中范围：

- `Phase E: MVVP Projection`
- `RG-09`: TUI/Web consume projection instead of local state machines
- `RG-18`: prompt/status use projection blocker and command revision state

收口结论：

- Workspace list updates now synchronize the existing detail snapshot's `Workspace` field instead of leaving stale detail state behind.
- `UpsertWorkspace` and `ReplaceWorkspaceList` preserve detail-only projections such as `RunCommand` and `GapBaseline` while updating workspace state, active run and blocker fields.
- This keeps workspace list, detail snapshot and TUI prompt inputs aligned to the same workorchd ViewModel state after blocker resolution, replan acceptance or workspace state events.

验证证据：

```bash
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -run 'TestStore(UpsertWorkspaceSynchronizesDetailWorkspaceState|ReplaceWorkspaceListSynchronizesDetailWorkspaceState|UpsertSnapshotPreservesAcceptedRunCommand|RunCommandAcceptedProjectsCommandRevision)' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1
go test ./internal/services/workorch/service/tui -count=1
```

### 2026-06-21 Phase F Startup Recovery Resume Path Closure

命中范围：

- `Phase F: Recovery And E2E`
- `RG-06`: GapBaseline path for resume after service recovery
- `RG-13`: deployed/service restart resume coverage
- `RG-19`: normalize stale active projection after service restart

收口结论：

- Service startup recovery keeps `Validate` as a non-mutating readiness check, while `Service.Run` performs lifecycle recovery before workspace projection starts.
- A workspace whose persisted state was `active` with an active `active` run from a previous process is normalized to `active` with no `activeRunId`; the old inactive run is cancelled.
- After this normalization, an explicit `intent=resume` command enters the unified `WorkspaceRunRequested` EventHub path and queues workspace activation, not a direct run continuation and not a destructive restart.
- The queued activation preserves workspace scope and selected Goal/DoD definitions. The subsequent orchestrator dispatch continues to rely on the Phase C `GapBaseline` gate before any state-changing runtime step.
- The deployed run-resume e2e script now also probes open decision tickets for the verified workspace. If one exists, it resolves the ticket with `resolutionType=replan` and verifies the projected command is `intent=replan`; `WORKORCH_RUN_RESUME_VERIFY_REQUIRE_REPLAN=1` can make this branch mandatory in a seeded environment.
- The deterministic HTTP regression test covers the same replan projection path without depending on provider behavior or a live decision appearing during execution.

验证证据：

```bash
go test ./internal/modules/application/workorchd/service/server -run 'TestServiceRunRecoveredWorkspaceResumeEnqueuesWorkspaceActivation|TestServiceRunRecoversStartupInactiveRunningWorkspaceBeforeProjection|TestHTTPServerStartupLeavesFileBackedRunningRunForManualRecovery' -count=1
go test ./internal/modules/application/workorchd/service/server -run 'TestDecisionResolveEndpointProjectsReplanRunCommand|TestDecisionResolveEndpointResolvesTicketAndAllowsRunResume|TestServerStateDecisionReplanResolvedEventProjectsReplanCommand' -count=1
bash -n scripts/verify-run-resume-gap-confirmation-e2e.sh
WORKORCH_RUN_RESUME_VERIFY_MANAGE_INSTANCE=1 WORKORCH_RUN_RESUME_VERIFY_BUILD=1 WORKORCH_RUN_RESUME_VERIFY_TIMEOUT_SECONDS=180 ./scripts/verify-run-resume-gap-confirmation-e2e.sh
```

补充诊断收口：

- The deployed e2e script now preserves HTTP response status and body for failed POST commands, so activation/replan/resume failures can be diagnosed from artifacts instead of being reduced to an empty curl failure.
- Managed-instance mode can build the current source binary with `WORKORCH_RUN_RESUME_VERIFY_BUILD=1`, avoiding false failures caused by a stale `./bin/workorch` during local closure verification.

### 2026-06-21 Phase A Canonical Resume Input Closure

命中范围：

- `Phase A: Command Contract`
- `P1`: 命令语义不统一
- `RG-02`: TUI default command is `/run resume`

收口结论：

- TUI slash command path exposes `/run resume` as the only resume-style run command.
- English run-control intent recognition now uses `resume` as the canonical verb and avoids legacy English aliases.
- Chinese natural-language input such as `继续执行` remains accepted as user text intent, but it is not projected or persisted as a slash command or internal legacy resume alias.
- CLI agent empty decision answer now defaults to `resume`, keeping command-adjacent user input aligned with the unified resume terminology.
- Affected TUI command tests no longer carry legacy slash-command examples as long-term fixtures.

验证证据：

```bash
go test ./internal/services/workorch/service/tui -run 'TestRunControlIntentUsesResumeAsCanonicalEnglishVerb|TestModelNonCanonicalRunCommandsAreNotSupported|TestModelHelpPrefersRunResume|TestModelRunResumeCommandIsSupported|TestModelBlockedWorkspaceInputRefreshesBlockerInsteadOfContinuing|TestResumeRunReturnsBlockedRunWithoutResume' -count=1
go test ./internal/services/workorch/service/cli ./internal/services/workorch/biz/backends -run 'TestEventHubRunResourceMapCompactsExecutionContext|Test.*Run|Test.*Agent' -count=1
rg -n 'legacy_resume_alias|continuerun|continueexecution' internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient web/workorch-console/src docs/50-product-interface docs/20-runtime-flow -g '*.go' -g '*.ts' -g '*.tsx' -g '*.md'
```

### 2026-06-21 Phase B/F Accepted Response E2E Closure

命中范围：

- `Phase B: EventHub Scheduler Path`
- `Phase F: Recovery And E2E`
- `RG-03`: REST/client default execution action maps to `resume`
- `RG-04`: resume/replan/restart/recover route through one scheduler command path
- `RG-13`: deployed-instance e2e for `/run resume`

收口结论：

- The deployed run-resume e2e no longer treats `accepted=true` as sufficient proof for submit commands.
- The script now validates the synchronous accepted response metadata for resume activation, repeat resume and service-restart resume:
  - `intent=resume`
  - expected `requestId`
  - expected `reason`
  - non-empty `source`
- This keeps the e2e aligned with the design rule that HTTP/TUI synchronous paths return command accepted/no-op/conflict metadata, not completed run state.
- Projection validation remains unchanged: the same e2e still waits for workorchd projection to expose `runCommand.intent=resume`, matching `requestId`, positive command revision and `GapBaseline`.

验证证据：

```bash
bash -n scripts/verify-run-resume-gap-confirmation-e2e.sh
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkorchD(ActivationHTTPProjectsRequestID|RunResumeHTTPReturnsGeneratedRequestID|RunResumeHTTPProjectsReplanIntent|WorkspaceRunCommandResumeEnqueuesRunContinuation)' -count=1
```

### 2026-06-21 Phase E Web GapBaseline Projection Closure

命中范围：

- `Phase E: MVVP Projection`
- `RG-08`: projection exposes current focus, todo summary and gap baseline revision
- `RG-09`: TUI/Web consume projection instead of local state machines

收口结论：

- Web console type definitions now include `GapBaseline`, `GapTodoItem` and `GapNextAction` matching the workorchd snapshot projection contract.
- Workspace detail renders a dedicated `GapBaseline` section from `runSnapshot.gapBaseline` or `workspaceSnapshot.gapBaseline`.
- The section displays only server-projected fields: baseline revision, definition revision, next action, current focus, todo count, open coverage count and accepted/rejected evidence refs.
- Web does not recompute gap/todo/current focus locally; it only renders the workorchd ViewModel fields.
- Console assets were rebuilt so the deployed web bundle includes the new projection display.

验证证据：

```bash
npm --prefix web/workorch-console run build
./scripts/verify-web-console-projection-visual.sh
```

### 2026-06-21 RG-14 Resume Terminology Alignment Closure

命中范围：

- `Phase A: Command Contract`
- `Phase E: MVVP Projection`
- `RG-02`: TUI/Web canonical UX uses `/run resume`
- `RG-14`: CLI/TUI/Web/API/user-facing docs align with run command design

收口结论：

- 旧架构文档中的 旧推进命令 主路径已经改为 `/run resume`，并明确进入 `WorkspaceRunRequested(intent=resume)` command path。
- `eventhub-driven-workorchd-state-loop-design.md`、`contract-violation-recovery-loop-design.md`、`runtime-execution-event-queue-state-loop-design.md`、`mvvp-workspace-viewmodel-architecture.md` 和 `repair-replan-flow.md` 不再描述旧英文推进别名作为常规用户 run command。
- Web Console workspace board 和 workspace detail 的 active run 按钮文案改为“恢复运行”，行为仍调用 `resumeRun`，不在前端推导业务状态。
- `docs/50-product-interface/web-console/overview.md` 同步把交互动作命名为“恢复运行”。
- 旧推进命令只保留为本文的历史设计说明；中文自然语言“继续运行”仍只作为 TUI 用户意图词表，不形成内部 legacy resume alias。

验证证据：

```bash
npm --prefix web/workorch-console run build
./scripts/verify-web-console-projection-visual.sh
./scripts/verify-boundary-contract.sh
git diff --check -- docs/30-module-architecture/eventhub-driven-workorchd-state-loop-design.md docs/30-module-architecture/contract-violation-recovery-loop-design.md docs/30-module-architecture/runtime-execution-event-queue-state-loop-design.md docs/30-module-architecture/mvvp-workspace-viewmodel-architecture.md docs/20-runtime-flow/repair-replan-flow.md docs/50-product-interface/web-console/overview.md web/workorch-console/src/pages/WorkspaceBoardPage.tsx web/workorch-console/src/pages/WorkspaceDetailPage.tsx
rg -n "旧推进命令|legacy_resume_alias|Mode=continue|继续运行" docs/30-module-architecture docs/50-product-interface docs/20-runtime-flow internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient web/workorch-console/src scripts/verify-run-resume-gap-confirmation-e2e.sh -g "*.md" -g "*.go" -g "*.ts" -g "*.tsx" -g "*.sh"
```

### 2026-06-21 Phase C/D First-Run Gap Contract Closure

命中范围：

- `Phase C: GapBaseline Gate`
- `Phase D: LLM Contract And Violation Retry`
- `RG-15`: first run creates `GapBaseline` before first state-changing LLM step
- `RG-17`: LLM step input carries current gap/todo/requiredOutput from structured contracts

收口结论：

- `initialStepInput` now promotes a first-run `GapBaseline` with `nextAction=schedule_step` into a state-changing convergence contract when the structured coverage context explicitly has missing `state_change` evidence.
- The promotion is driven only by neutral structured fields: `GapBaseline`, `coverageWorkItemContext`, `stateChangePolicy=must_produce_state_change`, missing evidence kinds and coverage target set.
- The first LLM/runtime step input now carries `repairActionPackage.requiredOutput`, `coverageTargetSet`, `coverageWorkItemContext`, `stateChangingOutputRequired` and `repairObjective=produce_state_changing_output` when the workspace definition requires state-changing evidence.
- Pure Goal/DoD work that only requires validation is not forced into state-changing output. Business-specific delivery priority remains in workspaceTemplate/Soul/profile policy through structured execution contracts.
- Added a regression test proving first-run dispatch does not stop at a bare `GapBaseline`; it builds a state-changing requiredOutput contract from the current gap when the definition requires state change.

验证证据：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestInitialDispatchBuildsStateChangingContractFromGapBaseline|TestNextDispatchSpecsPersistsGapBaselineBeforeInitialStep|TestGapBaselineFromRunPreservesAcceptedAndRejectedEvidenceRefs|TestBuildConvergenceContractUsesProfileStateChangingEvidenceRequirement' -count=1 -v
go test ./internal/modules/kernel/orchestrator/biz -count=1
./scripts/verify-boundary-contract.sh
git diff --check -- internal/modules/kernel/orchestrator/biz/runtime_step_input.go internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items_test.go docs/30-module-architecture/run-resume-gap-confirmation-design.md
```

### 2026-06-21 RG-07 Assessment-Only Side-Effect Gate Closure

命中范围：

- `Phase C: GapBaseline Gate`
- `Phase D: LLM Contract And Violation Retry`
- `RG-07`: assessment-only gap confirmation rejects executable output
- `RG-16`: contract violations become rejected diagnostics and bounded retry inputs

收口结论：

- Assessment-only LLM output is now checked before artifact, patch, memory, graph or follow-up side effects can run.
- The gate rejects neutral executable/state-changing output fields including `executionGraph`, `patchRequests`, `patches`, `shellCommands`, `mcpToolCalls`, `memoryWrites`, `memoryRefs`, `nextStep` / `nextKind` / `nextInput`, `artifactPlan`, `artifactChunk` and `artifactChunkRepair`.
- Nested executable output under `llmDecision` is rejected by the same gate.
- Pure `dodAssessment` output remains valid for assessment-only stages.
- Rejected assessment-only executable output is classified as `assessment_only_executable_output`, mapped to the generic `wrong_output_shape` contract violation rule, and enters the existing contract violation retry / escalation path without executing the rejected output.
- Existing assessment-only graph tests were realigned from the old "ignore graph when assessment exists" behavior to the new rejected diagnostic retry behavior. Repeated same-shape violations escalate to a structured decision instead of executing forbidden graph nodes.

验证证据：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestValidateLLMExecutionOutputContractRejectsAssessmentOnlyExecutableOutputs|TestRuntimeOrchestrator_AssessmentOnlyExecutableOutputRetriesWithoutSideEffects|TestRuntimeOrchestrator_AssessmentOnlyContractViolationEscalatesRepeatedGraph|TestRuntimeRepairInputIncludesContractViolationRecovery|TestContractViolationRecoveryActionRetriesWithinBudget' -count=1 -v
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

### 2026-06-21 RG-16 Contract Violation Side-Effect Isolation Closure

命中范围：

- `Phase D: LLM Contract And Violation Retry`
- `RG-16`: rejected contract violation output has no accepted side effects

收口结论：

- The assessment-only executable-output regression now runs through the full orchestrator path with a `MemoryItems` repository attached.
- The rejected assessment-only output intentionally contains both an executable `executionGraph` and explicit `memoryWrites`.
- The runtime rejects that output before side effects, dispatches a contract violation retry, and later completes from a valid assessment-only retry output.
- The rejected step has no `memoryRefs`, the forbidden graph node is never executed, and the memory repository does not contain the rejected output summary/content.
- This proves the rejected diagnostic can be used for retry context without becoming accepted memory or executable runtime work.

验证证据：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_AssessmentOnlyExecutableOutputRetriesWithoutSideEffects|TestValidateLLMExecutionOutputContractRejectsAssessmentOnlyExecutableOutputs|TestRuntimeOrchestrator_AssessmentOnlyContractViolationEscalatesRepeatedGraph' -count=1 -v
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

### 2026-06-21 Phase A Search Evidence Cleanup Closure

命中范围：

- `Phase A: Command Contract`
- `RG-02`: TUI/Web canonical UX uses `/run resume`
- `RG-14`: CLI/TUI/Web/API/user-facing docs align with run command design

收口结论：

- `runports` tests no longer use `mode=continue` as arbitrary execution-context patch data for resume command publication. The test now uses `mode=resume`, so the sample data no longer looks like a retained internal continue intent.
- Recovery checkpoint tests no longer use `nextAction=continue` for a non-continuation progress checkpoint sample. The test now uses neutral `nextAction=observe`, preserving the negative assertion that only recovery checkpoints with explicit `resumeAction=resume_run` count as continuation checkpoints.
- Governance runtime-repair action normalization no longer treats the English literal `continue` as a recovery-action alias. Canonical retry/resume inputs remain `retry`, `rerun`, `resume`, `proceed`, `goahead` or equivalent localized user text.
- Candidate-selection metadata now uses `candidateSelectionMode=semantic_resume` instead of `semantic_continue`, aligning decision-resolution projection terminology with the `/run resume` command contract.
- Decision/approval/read-model tests no longer use `"continue"` as generic resolution/action fixture data; they now use `resume` where the fixture describes continuing the same run.
- TUI tests no longer use `run-continue` as generic active-run fixture IDs; they use `run-resume`, leaving `continue` matches only in negative command-recognition tests.
- The historical framework migration note now describes this path as semantic resume/continuation mode instead of semantic-continue mode.
- Canonical resume/retry behavior is unchanged, while the English literal `continue` is intentionally no longer accepted as a governance recovery-action alias. The remaining fixture cleanup closes search-evidence noise so Phase A verification can distinguish actual legacy continue semantics from negative tests and protocol fields.

验证证据：

```bash
go test ./internal/modules/application/workorchd/biz/appservice/runports -count=1
go test ./internal/modules/application/workorchd/biz/appservice/governancecore ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/biz/bootstrap -run 'Test.*(Decision|Governance|Candidate|Resolution|Approval|RunSnapshot|Runtime|WorkspaceScope)' -count=1
go test ./internal/services/workorch/service/tui -count=1
go test ./internal/modules/blocks/llm/biz ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1
WORKORCH_RUN_RESUME_VERIFY_MANAGE_INSTANCE=1 WORKORCH_RUN_RESUME_VERIFY_BUILD=1 WORKORCH_RUN_RESUME_VERIFY_TIMEOUT_SECONDS=180 ./scripts/verify-run-resume-gap-confirmation-e2e.sh
rg -n "旧推进命令|legacy_resume_alias|ModeContinue" internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient internal/modules/application/workorchd web/workorch-console/src docs/50-product-interface docs/20-runtime-flow docs/30-module-architecture -g '*.go' -g '*.ts' -g '*.tsx' -g '*.md'
```

### 2026-06-21 RG-12/RG-13 Decision Replan Scheduler Evidence Closure

命中范围：

- `Phase B: EventHub Scheduler Path`
- `Phase F: Recovery And E2E`
- `RG-12`: resolved decision replan routes through the same scheduler continuation path
- `RG-13`: deployed/service verification has deterministic server-side evidence for waiting-decision replan

收口结论：

- HTTP decision resolve with `resolutionType=replan` now has a deterministic server test proving the handler resolves the ticket, publishes the governance continuation request, projects `runCommand.intent=replan`, and queues governance background work.
- The queued governance continuation assertion uses the production `governanceRunResumeKey` contract instead of hard-coded key text, including the decision-scoped replan key shape.
- Owner `decision/ticket_resolved` events still project a governance replan command with workspace, run, source, reason, request id and decision id.
- No public test-only endpoint was added to create decisions. Decision creation remains runtime/kernel-owned; the deterministic evidence is provided by server integration tests and the deployed e2e can continue to validate live decision replan when the runtime naturally produces one.
- Existing governance resume pending/started tests were aligned with the same key contract, preserving duplicate coalescing and background-submit recovery evidence.

验证证据：

```bash
go test ./internal/modules/application/workorchd/service/server -run 'TestDecisionResolveEndpointProjectsReplanRunCommand|TestDecisionResolveEndpointResolvesTicketAndAllowsRunResume|TestServerStateDecisionReplanResolvedEventProjectsReplanCommand|TestServerStateGovernanceRunResumeCoalescesDuplicateRunID|TestServerStateGovernanceResumeKeepsPendingAfterBackgroundSubmitFailure' -count=1 -v
```

### 2026-06-21 Phase A/E TUI Resume Recovery Prompt Closure

命中范围：

- `Phase A: Command Contract`
- `Phase E: MVVP Projection`
- `RG-02`: TUI default command is `/run resume`
- `RG-09`: TUI consumes backend projection/command result instead of local run-state ownership
- `RG-18`: prompt/status uses projection-backed run state and command result

收口结论：

- TUI `/run resume` no longer locally blocks terminal or recovered runs by telling the operator to use `/run restart`.
- If the workspace projection exposes a same-workspace run while `ActiveRunID` is empty, TUI submits a scoped `WorkspaceRunCommand(intent=resume, expectedRunId=<projected-run>)` and lets workorchd decide recover/no-op/fail through the normal command path.
- If no run exists, `/run resume` still submits `intent=resume` without `expectedRunId`, preserving the first-run creation path defined by this document.
- `/run restart` remains available, but slash suggestions and help text describe it as a replacement run rather than a normal continuation path.
- TUI tests now prove failed/recovered terminal runs are resumed through the backend command path and do not create implicit activation or replacement runs.

验证证据：

```bash
go test ./internal/services/workorch/service/tui -count=1
rg -n "restart workspace execution|restart current workspace" internal/services/workorch/service/tui docs/50-product-interface/tui/interaction-contract.md -g '*.go' -g '*.md'
```

### 2026-06-21 Phase A/B Client Activation Command Path Closure

命中范围：

- `Phase A: Command Contract`
- `Phase B: EventHub Scheduler Path`
- `RG-01`: unified workspace run command intent and payload
- `RG-03`: client default execution action maps to `resume`
- `RG-04`: resume/restart/replan/recover route through one scheduler command path

收口结论：

- CLI `run activate`, Agent first-run execution and TUI goal submission no longer call the legacy `RunBackend.Activate` path directly.
- These client-side entrypoints now submit `WorkspaceRunCommand(intent=resume, source=cli|agent|tui, reason=user_requested)` with an activation payload.
- Backend-specific `Activate` remains as a lower-level compatibility facade, but client process services route activation intent through the same command DTO used by resume/restart/replan/recover.
- Focused tests assert that CLI and Agent first-run flows record a `WorkspaceRunCommand` and do not use the legacy `Activate` method directly.
- A reverse search confirms no production CLI/TUI/agent command path calls `Run().Activate` directly.

验证证据：

```bash
go test ./internal/services/workorch/service/cli ./internal/services/workorch/biz/agent ./internal/services/workorch/service/tui -count=1
rg -n "Run\\(\\)\\.Activate\\(|be\\.Run\\(\\)\\.Activate\\(|a\\.be\\.Run\\(\\)\\.Activate\\(|mustBackend\\(\\)\\.Run\\(\\)\\.Activate" internal/services/workorch/service/cli internal/services/workorch/biz/agent internal/services/workorch/service/tui -g '*.go'
```

### 2026-06-21 Phase A/B RunBackend Interface Narrowing Closure

命中范围：

- `Phase A: Command Contract`
- `Phase B: EventHub Scheduler Path`
- `RG-01`: unified workspace run command intent and payload
- `RG-04`: run lifecycle client actions enter the scheduler through one command path
- `RG-09`: process services do not own a second run lifecycle API

收口结论：

- `internal/pkg/workorchport.RunBackend` now exposes `Command` plus read-only `Get/List/Status` methods. Legacy `Activate`, `Resume` and `Restart` are no longer part of the client-facing run backend contract.
- Remote and EventHub backend production implementations no longer expose public `Activate`, `Resume` or `Restart` methods on their run backend concrete types. Remote backend keeps private helpers only for translating `WorkspaceRunCommand` to existing REST endpoints.
- EventHub restart tests now call `Run().Command(intent=restart)` and assert the unified `WorkspaceRunRequested` event, rather than calling a restart facade.
- Production reverse search confirms no CLI/TUI/agent/backend code calls `Run().Activate`, `Run().Resume` or `Run().Restart`.
- Remaining legacy-shaped methods are limited to test fakes where they model older behavior or assert it is not used.

验证证据：

```bash
go test ./internal/pkg/workorchport ./internal/services/workorch/biz/backends ./internal/services/workorch/service/cli ./internal/services/workorch/biz/agent ./internal/services/workorch/service/tui -count=1
rg -n "func \\(.*remoteRunBackend\\) (Activate|Resume|Restart)|func \\(.*eventHubRunBackend\\) (Activate|Resume|Restart)|Run\\(\\)\\.(Activate|Resume|Restart)\\(" internal/pkg/workorchport internal/services/workorch -g '*.go' -g '!**/*_test.go'
```

### 2026-06-21 Phase E Web Resume Command Wording Closure

命中范围：

- `Phase A: Command Contract`
- `Phase E: MVVP Projection`
- `RG-03`: Web default execution action maps to `resume`
- `RG-09`: Web consumes workorchd projection and command responses instead of owning run lifecycle semantics
- `RG-14`: Web user-facing wording aligns with resume command design

收口结论：

- Web API client method `activateWorkspaceExecution` was renamed to `resumeWorkspaceExecution`, while still using the existing REST resource that workorchd maps to `WorkspaceRunRequested(intent=resume)`.
- Workspace board/detail page handler names and user messages now describe submitting execution rather than locally "activating" or "starting" a second lifecycle path.
- User-visible Web labels for `start_workspace_execution`, board process steps, modal title and validation hints now use "提交执行/提交工作空间执行".
- Source search confirms the Web console source no longer contains `activateWorkspaceExecution` or the old startup wording.
- The Web console build refreshed embedded console assets after the source change.

验证证据：

```bash
npm run build
rg -n "activateWorkspaceExecution|启动工作空间执行|启动执行|后台启动|再启动执行|启动上下文" web/workorch-console/src -g '*.ts' -g '*.tsx'
```

### 2026-06-21 Phase A/B/E Workspace Resume Command Metadata Closure

命中范围：

- `Phase A: Command Contract`
- `Phase B: EventHub Scheduler Path`
- `Phase E: MVVP Projection`
- `RG-03`: Web/REST default execution action maps to `resume`
- `RG-04`: resume/restart/replan/recover route through the scheduler command path
- `RG-09`: Web consumes workorchd projection and command responses instead of owning lifecycle state

收口结论：

- `internal/pkg/workorchclient` no longer exposes `ActivateWorkspaceExecution` as the remote client method used by process services. The remote backend now calls `ResumeWorkspaceExecution`, while the underlying REST resource path remains `/execution-activations` because it returns or accepts the execution activation contract.
- Remote first-run resume now sends explicit command metadata on the workspace-scoped request body: `intent=resume`, `requestId`, `reason`, `source` and `definitionRevision`.
- Web `resumeWorkspaceExecution` now injects the same command metadata by default instead of submitting a bare activation request.
- The Web detail page no longer treats the synchronous workspace execution response as a completed activation snapshot. After command acceptance it clears local detail state and reloads workorchd projection, preserving the MVVP rule that Web displays server ViewModel state.
- The workspace execution activation HTTP handler now accepts an explicit `intent` field and rejects run-scoped intents such as `replan` on the workspace-scoped endpoint. Replan/recover must continue to use the run-scoped command path with an expected run id.
- Embedded console route tests and HTTP verification script now assert the canonical Web wording `提交执行` instead of the retired `启动执行` wording.

验证证据：

```bash
go test ./internal/pkg/workorchclient ./internal/services/workorch/biz/backends -run 'TestRemoteRunCommand(FirstResumeSendsWorkspaceCommandMetadata|ResumeSendsRequestID|ReplanSendsUnifiedIntent)' -count=1
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkorchD(ActivationHTTPProjectsRequestID|ActivationHTTPRejectsRunScopedIntent|RunResumeHTTPRejectsRestartIntent)' -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
npm --prefix web/workorch-console run build
./scripts/verify-boundary-contract.sh
```

### 2026-06-21 RG-14 Service API Resume Contract Documentation Closure

命中范围：

- `Phase A: Command Contract`
- `Phase B: EventHub Scheduler Path`
- `Phase E: MVVP Projection`
- `RG-03`: Web/REST default execution action maps to `resume`
- `RG-14`: CLI/TUI/Web/API/user-facing docs align with run command design

收口结论：

- `docs/50-product-interface/service-api.md` no longer documents the default workspace execution request as `activateWorkspaceExecutionRequest`; it now uses `resumeWorkspaceExecutionRequest`.
- The service API write-contract section now states that `/workspaces/:workspaceId/execution-activations` and run-scoped start/resume endpoints return `202` command accepted metadata, not a completed run or client-owned activation state.
- The same document now records that workspace-scoped execution commands carry `intent=resume`, `requestId`, `source` and `reason`, and that run-scoped intents such as `replan` / `recover` must use the run command path.
- `docs/50-product-interface/web-console/overview.md` now uses the canonical Web wording `提交执行` / `恢复运行` for interactive actions.
- Client comments and HTTP error wording were aligned to "workspace execution command" semantics while keeping the internal execution activation contract name for run-context assembly.
- The HTTP request DTO/helper used by `/execution-activations` was renamed to `workspaceExecutionCommandRequest` / `workspaceExecutionCommandSchedulerPayload`, so the REST edge no longer names the default command request as activation.
- The serverState injection point used by the scheduler bridge was renamed from `activateWorkspace` to `resumeWorkspaceExecution`, keeping the service/server boundary aligned with the command path while the appservice activation contract remains the run-context assembly implementation detail.

验证证据：

```bash
rg -n 'activateWorkspaceExecutionRequest|启动执行|Activation / Setup|activation / governance|返回 `200`.*execution-activations' docs/50-product-interface/service-api.md docs/50-product-interface/web-console/overview.md
rg -n 'activateWorkspaceExecutionRequest|activationRequestSchedulerPayload|ActivateWorkspaceExecution\\(|activateWorkspaceExecution|启动执行|workspace execution activation supports' internal/pkg/workorchclient internal/services/workorch/biz/backends internal/modules/application/workorchd/service/server/http_workspace.go internal/modules/application/workorchd/service/server/http_types.go web/workorch-console/src scripts/verify-workorch-http.sh -g '*.go' -g '*.ts' -g '*.tsx' -g '*.sh'
go test ./internal/pkg/workorchclient ./internal/modules/application/workorchd/service/server -run 'TestWorkorchD(ActivationHTTPRejectsRunScopedIntent|ActivationHTTPProjectsRequestID)' -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
```

### 2026-06-21 Phase B/F E2E Accepted Activation Run Discovery Closure

命中范围：

- `Phase B: EventHub Scheduler Path`
- `Phase F: Recovery And E2E`
- `RG-03`: Web/REST default execution action maps to `resume`
- `RG-04`: resume/restart/replan/recover route through the scheduler command path
- `RG-13`: deployed-instance e2e validates `/run resume` command accepted semantics

收口结论：

- `scripts/verify-skillhub-workspace-execution.sh` no longer treats `/execution-activations` as a synchronous activation response with `activation.run.id`. It now captures the pre-command run list, submits `intent=resume` with `requestId/reason/source`, validates accepted command metadata, then waits for the created run through the workspace runs projection before checking completion and snapshot evidence.
- `scripts/verify-incremental-artifact-e2e.sh` now builds its activation request with explicit resume command metadata and discovers the resulting run from workspace runs instead of reading `activation.run.id`.
- `scripts/verify-workspace-web-search-e2e.sh` now validates the accepted resume command response and waits for the new run before checking governed Web Search traceability.
- `scripts/verify-tui-long-output-e2e.sh` now validates accepted command metadata and waits for run materialization before checking completed long-output projection and terminal rendering.
- `scripts/verify-weather-ticket-approval-query.sh` now works from accepted resume command metadata, discovers the run through workspace runs, then waits for `blocked_on_decision` / `blocked_on_approval` / `completed` state before continuing the governance verification path. This applies to both mock and real modes because the same script is shared.
- `scripts/verify-real-llm-workspace-execution.sh` now validates accepted resume command metadata, discovers the run from workspace runs, waits for `completed`, then loads snapshot/observability/task read models.
- `scripts/verify-real-vector-memory-recall.sh` now validates accepted resume command metadata, discovers the run from workspace runs, waits for `completed`, then validates the vector memory recall snapshot.
- `scripts/verify-real-mcp-stdio-workspace-execution.sh` now validates accepted resume command metadata and discovers the run from workspace runs before using its existing completed-state wait.
- `scripts/verify-real-weather-query.sh` now validates accepted resume command metadata, discovers the run from workspace runs, waits for `completed`, then validates weather snapshot/observability evidence.
- `scripts/verify-software-dev-goal-dod-e2e.sh` now normalizes each scenario activation payload in the shared helper by adding `intent=resume`, `requestId`, `reason=e2e_resume` and `source=e2e`, validates accepted command metadata, and recovers the materialized run from workspace runs instead of reading `activation.run.id`.
- `scripts/verify-llm-workspace-execution.sh`, `scripts/verify-mcp-stdio-workspace-execution.sh`, `scripts/verify-contract-violation-recovery-e2e.sh` and `scripts/verify-acceptance-slice-result-deployed-e2e.sh` no longer keep legacy `activation.run.id` fallback in their run discovery helpers. Their helpers now require accepted resume command metadata and discover materialized runs from the workspace runs projection.
- These changes keep the REST edge aligned with the design rule that `/execution-activations` returns command accepted/no-op/conflict metadata, not a completed run.

验证证据：

```bash
bash -n scripts/verify-llm-workspace-execution.sh scripts/verify-mcp-stdio-workspace-execution.sh scripts/verify-contract-violation-recovery-e2e.sh scripts/verify-acceptance-slice-result-deployed-e2e.sh scripts/verify-software-dev-goal-dod-e2e.sh scripts/verify-real-llm-workspace-execution.sh scripts/verify-real-vector-memory-recall.sh scripts/verify-real-mcp-stdio-workspace-execution.sh scripts/verify-real-weather-query.sh scripts/verify-skillhub-workspace-execution.sh scripts/verify-incremental-artifact-e2e.sh scripts/verify-workspace-web-search-e2e.sh scripts/verify-tui-long-output-e2e.sh scripts/verify-weather-ticket-approval-query.sh
rg -n 'activation\.run\.id|\("activation", "run", "id"\)|\("activation","run","id"\)|json_path .*activation.*run.*id|run_id="\$\(json_path .*activation' scripts -g '*.sh'
```

### 2026-06-21 Phase A TUI Resume Wording Residual Closure

命中范围：

- `Phase A: Command Contract`
- `RG-02`: TUI default command is `/run resume`
- `RG-14`: CLI/TUI/Web/API/user-facing docs align with run command design

收口结论：

- TUI terminal-run guidance now uses "resume" consistently for explicit run progression.
- Negative TUI tests no longer carry legacy slash-command examples as long-term fixtures.

验证证据：

```bash
rg -n "resume this workspace|resume current workspace|resume current run|resume execution|resume if recoverable" internal/services/workorch/service/tui internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient web/workorch-console/src docs/50-product-interface docs/20-runtime-flow -g '*.go' -g '*.ts' -g '*.tsx' -g '*.md'
go test ./internal/services/workorch/service/tui -run 'TestModel(HelpPrefersRunResume|NonCanonicalRunCommandsAreNotSupported|RunResumeCommandIsSupported)|TestRunControlIntentUsesResumeAsCanonicalEnglishVerb' -count=1
git diff --check -- internal/services/workorch/service/tui/commands.go
```

### 2026-06-21 Current Goal Verification Audit

命中范围：

- `Phase A: Command Contract`
- `Phase B: EventHub Scheduler Path`
- `Phase C: GapBaseline Gate`
- `Phase D: LLM Contract And Violation Retry`
- `Phase E: MVVP Projection`
- `Phase F: Recovery And E2E`
- `RG-01` through `RG-19`

收口结论：

- Focused restart verification confirms `intent=restart` remains an explicit replacement path: the appservice replacement run test, HTTP workspace restart scheduler tests and embedded EventHub restart publication test all pass on the current worktree.
- The core orchestrator, workorchd server, appservice and TUI packages all pass package-level verification after the run-resume/gap-baseline changes.
- The full Go test suite passes after regenerating embedded Web console assets. The first full run failed only because `web/consoleassets/dist/assets/*` had not yet been rebuilt; `npm --prefix web/workorch-console run build` regenerated the tracked bundle inputs, and the subsequent `go test ./... -count=1` passed.
- The deployed-instance run-resume gap confirmation e2e passes in managed-instance mode. The script built the current source binary, started a temporary workorch service, verified resume command metadata, run materialization, projection command state and `GapBaseline`, then stopped the managed instance.
- Boundary verification passes, and reverse searches show no remaining production `activation.run.id` script contract, no client-facing `Run().Activate/Resume/Restart` production calls, and no production internal `continue` run intent outside this design document and TUI negative tests.

验证证据：

```bash
go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivateWorkspaceExecutionWithBackendRestartReplacesCreatedActiveRun|TestActivateWorkspaceExecutionWithBackendRestart.*|TestActivateWorkspaceExecutionWithBackend.*Replacement' -count=1
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkorchD(WorkspaceRestartHTTPProjectsRestartIntent|SchedulerWorkspaceRunCommandEnqueuesWorkspaceRestartByWorkspaceScope|WorkspaceRunCommandRestartEnqueuesWorkspaceRestart|RunResumeHTTPRejectsRestartIntent)' -count=1
go test ./internal/services/workorch/biz/backends -run 'TestEventHubRunRestartPublishesUnifiedWorkspaceRunCommand' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/services/workorch/service/tui -count=1
npm --prefix web/workorch-console run build
go test ./... -count=1
./scripts/verify-boundary-contract.sh
WORKORCH_RUN_RESUME_VERIFY_MANAGE_INSTANCE=1 WORKORCH_RUN_RESUME_VERIFY_BUILD=1 WORKORCH_RUN_RESUME_VERIFY_TIMEOUT_SECONDS=180 ./scripts/verify-run-resume-gap-confirmation-e2e.sh
rg -n 'activation\.run\.id|\("activation", "run", "id"\)|\("activation","run","id"\)|json_path .*activation.*run.*id|run_id="\$\(json_path .*activation' scripts -g '*.sh'
rg -n 'func \(.*remoteRunBackend\) (Activate|Resume|Restart)|func \(.*eventHubRunBackend\) (Activate|Resume|Restart)|Run\(\)\.(Activate|Resume|Restart)\(' internal/pkg/workorchport internal/services/workorch -g '*.go' -g '!**/*_test.go'
rg -n "旧推进命令|legacy_resume_alias|ModeContinue" internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient internal/modules/application/workorchd web/workorch-console/src docs/50-product-interface docs/20-runtime-flow docs/30-module-architecture -g '*.go' -g '*.ts' -g '*.tsx' -g '*.md'
git diff --check
```
