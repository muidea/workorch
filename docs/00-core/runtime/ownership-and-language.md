# Runtime Ownership And Contract Language

Status: normative

Type: core-contract-part

Last Verified: 2026-08-26

## Core Neutrality And Owner Layers

Runtime core 只理解 workspace、run、definition、focus、requirement、prerequisite、candidate、sealed dispatch、
operation、effect、evidence、assessment、governance、recoverable wait、budget、result、recovery 和 projection。它不推断领域交付物、
实现正确性或工具偏好。

| Layer | Owns | Must not |
| --- | --- | --- |
| Runtime core | facts、reducer、authority、sealed dispatch、evidence、governance/recoverable wait、budget sub-ledger、terminal | 领域策略、owner 资源猜测、provider 私有逻辑 |
| profile/capability | 交付类型、能力、owner admission、validation/evidence policy | 修改 Runtime authority 或越权关闭 coverage |
| LLM | 在 sealed required output 内提出 bounded document/candidate | 选择 Runtime action、扩权、授予 credit 或裁决 terminal |
| application/interface/projection | use-case composition、intent、协议、projection | drive Runtime、重建 authority 或隐式续跑 |

`internal/pkg/workorchruntimecontract` 只保存 owner-neutral DTO、合同与无状态 normalize/validate/clone/hash helper；
不得拥有状态迁移、CAS、EventHub、filesystem/network I/O、retry 或 owner topic。

## Cross-Owner Contract Boundary

- 正式状态（包括对象创建时的 initial state）只由唯一 owner 的 private event/reducer 选择并持久化；外部 command/service 只表达 immutable intent/fact，不携带或直写 target/initial state。同一 state type 可以由该唯一 owner 的多个内部 transition 协调，但任何一个 transition 都不能属于第二个 owner。
- 跨 owner runtime I/O 只使用 EventHub 与接收 owner `pkg/events` 定义的 concrete typed Command/Data/Result。
- 需要同步结果、强一致写入或 admission 的交互使用 `Send`；审计通知使用 `Post`，Post handler 不依赖 `event.Result`。
- 禁止跨 owner service/repository/adapter callback、Envelope、`map[string]any`、无约束 `any`、JSON/reflect bridge 或通用 registry。
- EventHub payload 只传 bounded immutable data，不传 repository、registry、recorder、channel、writer 或其它可变资源。
- 无持久化过程状态的 Application use case 可以组合 owner query/command；一旦流程需要 phase、
  retry journal、partial-ack 或 restart recovery，必须由一个明确的业务 owner 持久化唯一 coordinator
  aggregate，并且只由该 owner 的 private internal event/reducer 选择 transition。Application 只提交
  单一 intent，不得保存影子 saga 或用部分 owner result 直接推进状态。
- read model/UI/session 中的 display state 只能从 owner fact 或自身源 fact 纯派生；不得另存一份可独立迁移的 mirror lifecycle，也不得使用 queued/active 等展示值触发 owner transition。
- Runtime EventHub metadata 中的 `StateMachine` 只表示参与哪一条协调链，不授予 reducer authority；`ReducerOwner`
  固定为 SM-1=`run`、SM-2～SM-4=`orchestrator`，`MachineRole` 区分 participant、owner ingress 与 owner observation。
  EventHub envelope 即使是 owner ingress，也必须先翻译为 owner-private internal event；只有该私有事实可标记
  `owner_internal_reducer` 并选择 transition。Run 对 Orchestrator aggregate 的物理 CAS 只能标记为
  `opaque_persistence`，不能据此解释 SM-2～SM-4。

Workspace scope 是 command execution context 的一部分，不是组件实例的“当前 workspace”。带 `WorkspaceID` 的入站
命令必须先绑定 matching EventHub context；已有 scope 不能静默覆盖。Biz、Service、Module、共享 cache、全局变量和
后台任务不得保存未受 scope 约束的 current WorkspaceID。跨 owner/local-only identity 必须携带足够 scope；无法唯一解析
时 fail-closed。

## Canonical Decision And Factual Feedback

SM-2 的唯一 canonical decision 是 `ConvergenceState`。action 文案、candidate mode 与 required-output kind 只能由该
state 和 sealed output constraints 纯派生，不得成为独立 persisted authority、CAS 或跨 owner lifecycle 字段。
`RuntimeDecision` 只保存 canonical state 与 terminal code/summary；SM-4 seal delivery 时才生成
`TerminalOutcome{Kind,Code,Summary}`。`completed|failed` 来自 sealed SM-2 terminal；`cancelled|superseded` 只能源自
Run SM-1 typed administrative intent。后一种 intent 由 SM-4 以 `RuntimeTerminationRecord` 保存为 exact delivery handoff，
但 record 不迁移 Run；只有最终 Run-private SM-1 terminal-delivery event 可以改变 Run lifecycle。

`recoverable_runtime_wait`是canonical nonterminal state/fact：只能从typed condition、trigger/deadline、budget和guard派生，
并由SM-4持有persisted re-drive successor。reason/message、projection或客户端不能创建/解除它。execution-budget view
同样只从各sub-ledger纯派生，不是第二份decision或consumption authority。

request、candidate、feasibility、operation、drive 和 delivery feedback 使用双层值合同：

1. reducer 只读取当前结果域的 closed stable factual category；
2. 同时保存 producer 提供的 bounded、audit-safe original diagnostic，包括 owner、class/code/message、signature、
   field violations、target/revision 与允许的 details ref/hash。

stable category 不能覆盖 original；message/code/summary 不得选择 action、retry、target、ticket 或 terminal。
credential、endpoint、raw prompt/provider body、未脱敏文件内容不属于“反馈保真”，只能按 request contract 保存允许的
hash、bytes 与 redacted/omitted metadata。双层 feedback 是嵌入 concrete Result 的纯值，不是通用 Envelope。

## Coverage Language

Goal/DoD、Template/Profile 和 Definition seed 是 declaration，不是 credit authority。只有 SM-2 根据 sealed semantic
document 与 capability-owner feasibility 封存的 `CoverageEvidenceBinding` 才是 canonical binding；capability receipt
只是 owner fact。CWI credit、coverage ledger 与 terminal gate只能由 SM-2 从 declaration、binding 和 matching owner
fact 派生。candidate 自报 CWI、assessment claim、PassedChecks、exit code、summary 与 projection 均不能写入 credit。

Provider ceiling、ExpectedDocument 与 cache section 规则见
[Runtime Request Provider Contract](../runtime-request-provider-contract.md) 和
[Runtime Cache Partitioning](../runtime-cache-partitioning.md)。DefinitionFocus generation 与 owner-policy contradiction
规则见 [Semantic Convergence](semantic-convergence.md)。
