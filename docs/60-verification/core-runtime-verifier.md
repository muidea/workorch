# WorkOrch Core Runtime Verifier

Status: active

Type: verification

Last Updated: 2026-07-29

## Purpose

本文定义 Runtime V2 当前唯一的 core 验证合同。验证对象是四状态机、单 owner、typed EventHub 边界、
canonical decision、无损反馈、sealed dispatch、receipt 与 terminal delivery；不再以 V1 graph/step/checkpoint、
历史测试名或 dated workspace fixture 作为通过标准。

已完成的 fresh-live/restart 覆盖见：
[Runtime V2 单 Owner 与合同归一收口记录](../90-archive/completed/runtime-v2-single-owner-state-machine-closure-plan.md)。
当前变更的实施顺序只由 active 未关项定义。

## Authority And Scope

core verifier 只验证：

- SM-1～SM-4 的 owner、合法/禁止迁移与跨状态机交接；
- `ConvergenceState` 是唯一 canonical decision，action/output kind 仅可派生；
- request、candidate、feasibility、operation、drive、delivery 的 stable factual category 与 safe original
  diagnostic 全链路保真；
- candidate request session、attempt、sealed dispatch、operation execution、assessment、governance binding 与
  delivery acknowledgement 保持正交；
- capability receipt outcome、terminal outcome 与 owner lifecycle 各自只有一个权威字段；
- C0～C5、provider ceiling、stable prefix、audit-safe/provider-safe 投影和敏感信息边界；
- workspace/run scope、restart、duplicate/stale/CAS、exactly-once effect 与 projection 单向性；
- core 的领域中立性和 22 运行单元的 pkg / concrete EventHub 边界。

core verifier 不验证 software-dev 特定文件、语言、测试命令、目录或业务 DoD；这些属于 profile verifier。

## Four-State-Machine Mandatory Matrix

| 状态机 | 唯一 owner | 必须验证的合法迁移 | 必须拒绝的迁移 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | activation/continuation/typed terminal fact 先成为 owner-local event，再执行 pending→active、blocked continuation 或 terminal | 外部指定目标 state、读取 Orchestrator action/focus 选择 lifecycle、terminal→active |
| SM-2 authority/focus | `kernel/orchestrator` | prerequisite/feasibility/receipt/assessment/governance fact→internal event→canonical decision；focus/evidence/repair 原子推进 | capability/adapter 选 decision、错误文本选 target、直接写 Run/Workspace/Result |
| SM-3 candidate/dispatch | `kernel/orchestrator` | request session→complete document→single evaluator→persist diagnostic→final/唯一 correction；accepted→seal→execute | incomplete/transport 创建 attempt、attempt 2、adapter 选 correction/terminal、未 seal 执行 |
| SM-4 drive/recovery/delivery | `kernel/orchestrator` | Run fact→run-scoped singleflight→load/re-drive；typed conflict/recovery；Result→Workspace→Run acknowledgement | 重建 SM-2 authority、projection/application 反向驱动、重复 durable effect、写其他 owner state |

每次 Runtime V2 变更必须标出受影响 SM，并补：transition、invariant、restart/idempotency、stale/duplicate、
budget/guard 以及适用的跨状态机合同测试。

## Contract-Normalization Assertions

### Canonical Decision And Terminal

- aggregate、authority snapshot、transaction、request identity 不持久化独立 `RuntimeAction` 或
  `RequiredOutput.Kind`；
- provider-facing operation mode 只能从 sealed canonical state/output constraints 派生；
- Runtime aggregate 只保存 canonical state + terminal code/summary，不保存第二个 terminal kind；
- SM-4 sealed delivery 使用 `TerminalOutcome.Kind = completed | failed | cancelled | superseded`；
- completed/failed 只来自 sealed SM-2 terminal，cancelled/superseded 只来自 SM-1 typed administrative fact；
- terminal delivery identity/acknowledgement 不保留 action 或第二份 administrative kind；
- Run/Workspace/Result 不解释 Orchestrator decision，只执行各自 owner-local typed outcome transition。

### Feedback Preservation

- concrete Result 同时携带本 domain stable category 与 producer 的 audit-safe original diagnostic；
- original class/code/message/signature、field violations、target/revision、scope 与 details ref/hash 在
  normalize/clone/hash/persistence/EventHub/projection 中不被覆盖；
- producer 没有 diagnostic 的普通 accepted fact 才允许 original 为空；
- unknown code 保留 original 并 typed fail-fast，不能默认 retry、CAS、repair、governance 或 terminal；
- provider-safe correction projection 与 audit-safe original diagnostic 分开生成，互不覆盖；
- credential、endpoint、raw prompt/provider body/stream、未脱敏文件内容只允许记录 hash/bytes 与
  redacted/omitted metadata，不得进入 aggregate、eventstream、projection、memory 或下一次 candidate context。

### Candidate And Request

- incomplete document、fragment/EOF 与 transient transport 只使用 request-session recovery，不创建 attempt；
- authentication/configuration/protocol/submission-unknown/continuation-exhausted 形成 factual result，不创建
  candidate attempt；
- complete document 只进入一个 evaluator，first-failure 顺序稳定；
- evaluator 只返回 accepted 或完整 typed rejection，不返回 correction/retry/terminal disposition；
- SM-3 持久化 rejection 后才可形成 typed terminal 或唯一 `correctable_once` internal result；
- attempt 1 复用 transaction、manifest、C0～C3 与 stable prefix；第二次 complete rejection terminal，零第三次请求；
- owner feasibility 不进入 candidate correction 或 attempt ledger。

### Capability And Receipt

- File/Git/Shell/MCP/SSH/Web 各自拥有 emitted-code total mapping，共享 pkg 不维护全 owner code taxonomy；
- feasibility 只返回 feasible/stale/prerequisite_required/infeasible factual result；
- operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，不存在跨 EventHub `Status + Outcome` 双权威；
- durable no-op 使用 `Outcome=no_op`，删除重复 `NoOp bool`；read-only/validation 的正常零 durable effect 仍为
  accepted，`Changed` 只作 observed effect metadata；
- accepted receipt 不携带 failure，rejected/unknown outcome 保留完整 safe original diagnostic；
- durable effect 使用稳定 ExecutionKey/Fence/owner receipt，restart/duplicate 不重复副作用或改变 diagnostic hash；
- stale/resource revision 独立于 failure/candidate budget；accepted resource change 不消耗 recovery budget。

### Failure, Recovery And Governance

- 只有 repository expected/observed revision 不一致产生 typed concurrent-state conflict 并消费 CAS reload；
- semantic reject、stale identity、candidate session conflict、duplicate/no-op 不得借 `common.ErrConflict` 进入 CAS；
- provider transport、operation outcome recovery、delivery retry 使用独立 persisted ledger；
- repair 只读取 typed gap/category，不解析 message、stderr/stdout 或 summary；
- retry/repair exhaustion 与 repeated failure 形成 typed terminal/no-progress fact，本身不能创建 ticket；
- governance 只来自独立封存的 `ExternalDependency/GovernanceRequirement`，不存在无信息 Decision/Approval；
- projection、TUI、Web、eventstream 不根据 error/message 自动 Resume、ticket、action 或 owner state change。

## Workspace, Restart And Concurrency

- workspace-scoped command/result/feedback 携带 workspace/run，适用时携带 authority/dispatch/operation identity；
- component instance、Service、global/cache/background task 不保存未经 workspace scope 约束的“current workspace”；
- 至少两个 workspace 并发 candidate、receipt、recovery、delivery 时不串 identity、diagnostic 或 singleflight；
- CAS 前/后 crash、owner commit 前/后 crash、response 丢失、duplicate/stale/乱序均零重复副作用；
- Run inventory 只返回 SM-1 state + opaque record presence/revision/ref；Orchestrator load 后 owner-local 分类
  `active_no_dispatch`、`active_sealed_dispatch`、`blocked_governance`、`terminal_delivery_pending` 并自动恢复；
- 不出现 `active + nonterminal + no scheduled drive`，也不使用人工 Resume 作为恢复机制。

## Verification Entry

按变更范围先执行 targeted tests，整轮至少执行：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/{file,git,shell,mcp,ssh,web,llm}/... -count=1
go test ./internal/modules/blocks/{run,result,workspace}/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
make verify-boundary-contract
go test -race ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/{llm,file,shell,run,workspace,result}/... -count=1
go test -p 1 ./... -count=1
go vet ./...
make verify-ci-mvp-file
```

`verify-boundary-contract` 最终还必须扫描：共享 pkg transition policy、全 owner code 表、error-text routing、
Action/Output.Kind 与 Status/Outcome 双权威、terminal action、通用 Envelope/map/any/JSON/reflect bridge、scope
缺失及禁止 raw provider 内容。

历史 `verify-workorch-runtime-core-flow.sh`、graph step fixture 或 incremental-artifact fixture 若仍存在，只能证明
对应非权威存储/投影或兼容清理状态，不能替代上述 Runtime V2 typed contract 测试，也不能重新定义通过标准。

## Deployed Verification

使用最新 build、fresh data 和至少两个并发 workspace；不得人工修改 runtime data/schema、Decision、Approval，
不得重复 Resume、提高 budget、放宽 decoder 或启用兼容 fallback。

deployed 验证必须证明：

1. activation 自动进入 active 并封存 ready provider/model ceiling；
2. request incomplete/transient/configuration 与 complete candidate 分流正确，stable prefix 稳定；
3. complete contract rejection 最多一次 correction，original diagnostic 可由 UI/audit 追溯；
4. owner feasibility/operation outcome 不进入错误 correction/CAS/governance；
5. durable operation exactly once，restart 不重复 effect；
6. typed validation/assessment/repair 持续推进，不解析错误文本；
7. governance 只在真实 dependency 上等待；
8. terminal typed outcome 完成 Result→Workspace→Run acknowledgement；
9. TUI/Web/eventstream 只展示 safe projection，不反向驱动；
10. 两个 workspace 完全隔离，并分别收敛到 completed、typed terminal 或真实 governed wait。

## Boundary

core verifier 不应验证：

- software-dev 特定测试命令、文件名、语言栈或项目结构；
- profile/Soul/Skill/MCP 的具体策略文本；
- V1 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、graph checkpoint 或自动 successor；
- archived test name inventory、dated workspace/run 记录或 raw runtime/provider payload。

## Source Documents

- [runtime-stage-contract.md](../00-core/runtime-stage-contract.md)
- [runtime-request-cache-note.md](../00-core/runtime-request-cache-note.md)
- [runtime-v2-single-owner-state-machine-closure-plan.md](../90-archive/completed/runtime-v2-single-owner-state-machine-closure-plan.md)
- `internal/modules/kernel/orchestrator/readme.md`
- [service-acceptance.md](service-acceptance.md)
- [profile-software-dev-verifier.md](profile-software-dev-verifier.md)
