# WorkOrch Unified Runtime Design Contract

Status: normative

Type: core-contract-index

Last Verified: 2026-08-21

## Purpose

本文是 WorkOrch Runtime 语义的唯一总入口。详细规则按单一专题拆分；下列子合同共同构成规范，不是可任选的参考资料。

## Contract Set

| 合同域 | 规范入口 | 回答的问题 |
| --- | --- | --- |
| Runtime semantics | [Runtime Topics](runtime/overview.md) | Owner、收敛、candidate transaction、四状态机与全局门禁 |
| Definition convergence | [Definition Convergence](definition-convergence/overview.md) | Goal/DoD all-of admission、语义图、调度、clarification 与上下文 |
| Request/cache | [Runtime Request Cache](runtime-request-cache-note.md) | C0～C5、provider ceiling、rendering 与 audit |

Runtime 行为变更必须先读取总入口，再从 Runtime Topics 进入受影响专题；不得只引用本页摘要替代子合同。

## Authority And Ownership

- SM-1 只属于 Run owner：Run lifecycle、continuation、administrative termination 和 replacement intent。
- SM-2、SM-3、SM-4 只属于 Orchestrator：authority/focus convergence、candidate/sealed dispatch、drive/recovery/delivery。
- Workspace、Result、Decision、Approval、Memory 与 capability owners 只修改自己的正式状态。
- Application 构造 typed bootstrap、提交 Start/Resume/Restart intent、提供协议与 projection；它不拥有 Runtime drive。
- 跨 owner I/O 只使用 EventHub 与接收 owner 定义的 concrete typed command/data/result；禁止 service/repository 直连。

## Production Mainline

```text
Workspace owner seals admitted startup input
  -> Application builds typed RuntimeBootstrap
  -> Run owner seals SM-1 activation
  -> Orchestrator SM-4 drives persisted aggregate
  -> SM-2 seals authority, semantic contract, ready focus and evidence binding
  -> SM-3 requests/evaluates candidate and seals dispatch
  -> capability owner returns factual result/receipt
  -> SM-2 credits evidence, settles/reopens CWI or seals terminal
  -> SM-4 delivers Result -> Workspace -> Run
```

Recovery is idempotent re-drive from the Run-owned opaque aggregate. Projection、Eventstream、provider text、process-local cache
和历史 event sequence 都不能重建或替代 authority。

Runtime 必须区分 genuine governance wait、具备 typed recovery condition/trigger/deadline/budget 的 bounded recoverable
runtime wait 和 terminal failure。recoverable wait 是 SM-2/3/4 aggregate fact 与 SM-4 re-drive responsibility，不新增
SM-1 lifecycle state，也不能留下无后继的 `active + nonterminal` Run。完整记录、owner probe 映射与迁移见
[Recoverable Wait](runtime/recoverable-wait.md)。

provider、candidate、semantic proof、capability operation、repair 与 recovery 的消费按各自 persisted sub-ledger 管理；
产品级 execution-budget view 只确定性汇总，不成为第二份消费 authority。完整合同见
[Execution Budget](runtime/execution-budget.md)。

## Definition And LLM Boundary

- 正式 startup input 是 Goal、DoD、Workspace Template snapshot 与 resolved project workspace directory。
- canonical Goal/DoD aggregation、dependency、gate 与 terminal 只支持 `all_of`；不提供 `one_of`/`any_of` terminal escape。
- 显式 unsupported relation 只能存在于 Workspace owner 的 pre-seal admission candidate，并必须在 Run 前形成 blocking ticket。
- startup input 封存后允许执行期 LLM 基于 exact revision/hash 处理 semantic proof、candidate 或 bounded gap diagnostic；LLM
  不得改写输入、选择 Runtime action、授予 credit 或裁决 terminal。
- Definition 修订不能被 Resume 吸收；需要新输入时必须显式 Restart，并遵守 replacement/delivery transaction。

## Four Coordinated Machines

| ID | Sole owner | Owns | Must not |
| --- | --- | --- | --- |
| SM-1 | Run | activation、continuation、replacement、administrative lifecycle | 选择 focus/action 或解释 aggregate |
| SM-2 | Orchestrator | authority、CWI、binding、credit、assessment、terminal | 调 provider 或执行 capability effect |
| SM-3 | Orchestrator | request session、attempt、candidate、sealed dispatch、receipt transaction | 选择下一 action 或绕过 seal |
| SM-4 | Orchestrator | bounded drive、singleflight、recovery、delivery ledger | 从 projection/history 重建 authority |

## Forbidden Production Surfaces

`StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、application
Start/Resume queue、自动 replan successor、checkpoint replay，以及 map/JSON/reflect authority bridge 均不得回到生产合同。

## Change Gate

每次 Runtime 变更必须：

1. 标明受影响的 SM、owner、persisted fact、合法及禁止迁移；
2. 为 owner-defined concrete command/result 覆盖 producer/consumer；
3. 覆盖 normal、duplicate、stale、CAS 前中断、CAS 后响应丢失与 fresh-process reload；
4. 覆盖 budget/guard 和零副作用断言；
5. 同步对应 core 子合同、组件 README、flow 与 verification；
6. 通过 `make verify-boundary-contract` 及受影响测试。

实施进度、候选需求、dated run 和 closure diary 不属于本合同，也不进入 `docs/` 的规范树。
