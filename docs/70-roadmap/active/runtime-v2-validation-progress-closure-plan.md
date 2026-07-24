# Runtime V2 Live 合同正向收敛收口方案

Status: active

Type: implementation closure plan

Last Updated: 2026-07-24

## 1. 目的与适用范围

本文是当前 Runtime V2 live 正向收敛问题的唯一可执行代码收口清单。此前在同一路径下持续
累积的历史 live 日记、阶段性实现记录和旧状态表已经清理，不再作为实现依据。

本文解决的问题不是“让某一条候选通过”，而是保证以下闭环在不同 workspace 内容、provider
输出和进程重启条件下都能够确定性前进：

```text
Goal / DoD / workspace observation
  -> typed Focus + coverage dependency + target role
  -> owner-grounded target / diagnostic / snapshot prerequisite
  -> Runtime V2 authority transaction
  -> provider candidate
  -> sealed dispatch
  -> capability owner receipt
  -> evidence admission
  -> effective progress / repair / retarget
  -> assessment / focus advance / typed terminal
  -> idempotent Result + Run + Workspace projection
```

本文从以下权威合同派生，不建立第二套 Runtime 规则：

1. [Runtime 核心合同](../../00-core/runtime-stage-contract.md)；
2. [Runtime 请求与缓存规则](../../00-core/runtime-request-cache-note.md)；
3. `internal/modules/kernel/orchestrator/readme.md`；
4. `internal/pkg/workorchruntimecontract`；
5. [22 组件合同矩阵](component-contract-and-regression-closure-matrix.md)。

## 2. 当前基线与结论

当前代码基线为 `5a6adcf3`。截至 2026-07-23，本地以下验证通过：

```text
go test ./internal/modules/blocks/shell/... -count=1
go test ./internal/modules/blocks/file/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
./scripts/verify-boundary-contract.sh
```

这只能证明已覆盖的局部合同和状态序列成立，不能证明 fresh deployed Run 已完成端到端正向
收敛。最新两项代码增量仍需 fresh live 证据：

- `f75ba969`：validation diagnostic 从 Shell owner receipt 进入 Failure、RepairDirective 和 LLM C3；
- `5a6adcf3`：File feasibility 在缺 target snapshot 时返回 typed prerequisite request，Orchestrator
  将未封存 candidate 转换为 snapshot acquisition，并保留 repair/recovery budget。

### 2.1 总体判断

| 判断项 | 结论 |
| --- | --- |
| Runtime V2 四状态机 | 总体设计成立，不推翻 SM-1～SM-4 |
| live 无法持续推进的首要原因 | owner-grounded 事实供给与严格 admission 之间存在合同缺口 |
| 次要原因 | SM-2/SM-3 曾把 receipt accepted、evidence admitted、effective progress 混为一谈 |
| 复杂度问题 | 合同数量本身有必要，但同一语义分散、fallback 隐式、状态序列验证不足 |
| provider/LLM | 是缺口触发器，不是 authority 事实源，也不是主要根因 |
| 当前状态 | 局部代码基线通过；P0 合同项和 fresh live gate 未关闭 |

## 3. 缺陷分类与修复性质

修复性质定义：

- **单点修复**：仅修正一个 owner 内部的事实生成或协议适配，不改变跨组件状态迁移；
- **全面修复**：必须同时覆盖 typed DTO、owner observation、Orchestrator transition、restart 与
  live state sequence；
- **已撤回方案**：曾落地但与当前设计目标冲突，不得作为兼容路径保留。

| ID | 缺陷类别 | 典型表现 | 主要组件 | 影响 SM | 修复性质 | 当前状态 | 主要遗留 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| D01 | validation evidence 失真 | all-skipped、zero-work 被当成 valid | Shell、contract、Orchestrator admission | SM-3 | 全面修复 | landed local | fresh live 复核 |
| D02 | accepted 与 progress 混淆 | accepted validation 清 repair window，同 gap 循环 | Orchestrator、contract 纯 helper | SM-2/3 | 全面修复 | landed local | coverage dependency reopening |
| D03 | anti-replay 缺少合法纠正路径 | exact rejected shape 被拒绝，但 authority 没有新事实 | Orchestrator、LLM | SM-2/3 | 全面修复 | owner retarget + same-gap/file-backed restart landed local | fresh live |
| D04 | provider 输出被固定上限截断 | JSON 在 4096/8192 附近 EOF | LLM catalog/adapter、activation、Orchestrator continuation | SM-3/4 | 全面修复 | landed local | provider fallback 与 fresh live |
| D05 | target grounding 不完整 | 空 workspace、scope 当 concrete target、目标不存在 | defaults/profile、activation、File、Orchestrator、LLM | SM-2/3 | 全面修复 | landed local | fresh deployed profile-selector sequence pending |
| D06 | failure target 来源不可信 | command target、验证入口、自由文本被提升为 repair target | Shell/MCP/File、Orchestrator | SM-2/3 | 全面修复 | Shell protocol landed local | clean-data tool/profile live pending |
| D07 | repair directive 迁移错误 | state change 已完成但仍缺 validation，directive 继续要求 state change | Orchestrator | SM-2 | 全面修复 | landed local | focus/coverage 依赖 open |
| D08 | candidate schema/operation 不一致 | targetRef/targetRefs、空 target、非法参数、无 durable effect | LLM、contract、File/Shell、Orchestrator | SM-3 | 全面防线；prompt 为单点增强 | landed baseline | owner variant 扩展时需持续回归 |
| D09 | File governance 误分类 | 普通 source patch 被要求 approval | File、Orchestrator executor | SM-3 | 全面修复 | landed local | clean-data live |
| D10 | metrics/projection 事实源错误 | activation/EventStream 推断 terminal 或 repair | Run notification、workorchd、EventStream | SM-4 | 全面修复 | landed local | duplicate/restart 观察 |
| D11 | capability preflight 适配错误 | `./path/...` 被当成真实 `...` 目录 | Shell | SM-3 input | 单点修复 | landed local | 其他 command grammar 变体 |
| D12 | validation diagnostic 丢失 | owner 有 diagnostic，LLM C3 只有自由文本 | Shell、contract、Orchestrator、LLM、defaults | SM-2/3 | 全面修复 | landed local | fresh live |
| D13 | feasibility 信息死锁 | patch 需要重读，但 same authority 禁止 read-only candidate | File、Orchestrator、contract、EventHub | SM-2/3 | 全面修复 | landed local | crash/live 组合 |
| D14 | TUI/Web activation 生命周期 | resume、模型继承、退出、事件刷新表现不一致 | workorchd、Workspace/Run、TUI/Web、LLM target resolution | SM-1/4 边界 | 单点与生命周期修复 | baseline landed | 不得成为 Runtime 补偿入口 |
| D15 | 显式交付目标方案不符合设计 | 空 workspace 要求用户手工声明文件 | workorchd、Workspace、File | bootstrap input | 已撤回方案 | replaced | 不恢复兼容字段 |

## 4. 目标状态与核心不变量

### 4.1 四状态机停止线

| 状态机 | 唯一职责 | 本轮允许修改 | 明确禁止 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | pending/active/blocked/terminal 与 lineage | 仅补 bootstrap/fresh activation 和 projection 回归 | 选择 next action、从 UI 重建 authority |
| SM-2 authority/focus convergence | focus、coverage、prerequisite、repair、assessment、terminal decision | focus 拆分、dependency reopening、grounding、progress、retarget | 执行 operation、访问 capability repository |
| SM-3 candidate/sealed dispatch | candidate attempt、admission、seal、execution receipt、evidence commit | typed prerequisite feedback、target-bound admission、output continuation | 选择 next action、从 prompt 扩展 authority |
| SM-4 drive/recovery/projection | bounded drive、CAS reload、singleflight、terminal delivery | restart matrix、projection exactly-once | 从 projection/EventStream 恢复 target、repair 或 dispatch |

### 4.2 必须始终成立的不变量

1. `owner receipt accepted != evidence admitted != effective convergence progress`。
2. 只有 Orchestrator reducer/commit 路径可以修改 `Facts.Repair`、coverage、focus 和 decision。
3. `pkg` 只保存 DTO、合同及纯无状态 helper，不执行 I/O、不选择 action、不修改 aggregate。
4. repair target 必须来自已提交的 owner observation，并绑定 owner、subject、role、revision 和 refs。
5. File manifest 只说明文件存在性；不得从“第一项”“后缀”或 LLM 建议猜业务 target。
6. 缺少 provider 所需事实时进入 typed prerequisite；不得让 attempt 1 在同一信息集合上猜测。
7. exact replay 必须拒绝；合法 retarget 必须由新的 persisted observation 或 dependency delta 证明。
8. candidate 的 kind、variant、parameters、target refs 和 effect scope 必须同时满足 sealed
   `RequiredOutputContract` 与 owner admissibility。
9. C0～C2 stable prefix 不得因 snapshot、diagnostic、repair、rejected shape 或 retry 漂移；动态事实
   只能进入 C3～C5。
10. CAS 前状态对重启完全不可见；CAS 后只能以同一 identity 幂等消费。

## 5. 组件职责与 typed 交互合同

### 5.1 直接修改组件

| 组件 | 唯一职责 | 本轮输出 | 不得承担 |
| --- | --- | --- | --- |
| `workorchdefaults` | 声明 profile 的 focus 粒度、target role、selector、validation/coverage policy | declarative profile contract | 状态迁移、workspace 探测 |
| `application/workorchd/.../activationcore` | 将 Goal/DoD/profile/workspace observation 编译为 typed bootstrap | Focus、coverage item、dependency、selector binding | 猜 repair target、选择 action |
| `blocks/file/biz` | manifest、selection、snapshot、admissibility、patch feasibility、receipt | owner observation/result | 业务语义 target、progress 裁决 |
| `blocks/shell/biz` | 执行 validation，形成质量、diagnostic 与可验证 related target observation | typed owner receipt | coverage、repair、next action |
| `blocks/mcp/biz` | 仅采纳失败协议显式返回的 structured failure target | typed observation | 从自由文本或 request target 补靶 |
| `blocks/llm/biz` | 渲染 sealed authority，调用 provider，返回 candidate/fragment | typed candidate、usage、provider failure | 发现 target、接受 evidence、修改 budget/ledger |
| `kernel/orchestrator/biz/runtimev2` | 唯一 reducer、prerequisite、admission、repair、retarget、dispatch authority | CAS transition、sealed dispatch、typed terminal | 领域硬编码、跨 owner repository |
| `kernel/run` | Runtime V2 aggregate CAS 与 recovery inventory | committed state/revision | action、evidence 质量和 progress 判定 |
| `workorchruntimecontract` | DTO、Validate/Normalize/Clone/Hash/Equal/纯集合 delta | immutable values | EventHub/repository、state transition |

### 5.2 相邻回归组件

`blocks/workspace`、`blocks/result`、`blocks/git`、`kernel/eventstream`、`kernel/governance`、
Decision/Approval、Web/TUI 只验证已裁决事实的保存、治理或展示，不因本轮新增 target、repair、
Resume 或 completion 判断。

### 5.3 跨 owner I/O

所有强一致获取与执行使用 concrete typed EventHub `Send`：

```text
activationcore -> Run owner: Create/Start Runtime V2 bootstrap
Orchestrator -> capability owner: Acquire/Probe/Execute command
capability owner -> Orchestrator: concrete Result / persisted receipt
Orchestrator -> Run owner: CAS RuntimeStateRecord
Orchestrator -> Result/Run/Workspace: terminal delivery
Run owner -> workorchd: typed state notification
```

禁止跨 owner service/repository、`map[string]any` authority、JSON/reflect bridge、通用 Envelope，
以及由 EventStream audit record触发 Resume 或状态补写。

## 6. P0 代码收口包

### WP-1：Focus 拆分与 coverage dependency reopening

**状态：landed local / live pending；优先级：P0；影响：SM-2 输入与迁移。**

目标：一个 focus 只包含能够共享 action、target role、evidence policy 和 assessment gap 的
coverage item。跨 focus 依赖必须显式声明，不能依赖文件名、语言或测试框架推断。

实现要求：

1. profile contract 增加或完善纯声明式 coverage dependency：source coverage、validation
   coverage、assessment coverage 的关系使用 opaque IDs 表达；
2. activation compiler 保持 Goal/DoD identity，按声明式边界拆分 focus，稳定排序并生成确定 hash；
3. source-changing evidence 命中 dependency 时，Orchestrator 原子重新打开依赖 validation/
   assessment coverage；已关闭且无依赖 delta 的 item 不得回退；
4. reopening 与 receipt/evidence commit 在同一次 aggregate CAS 中完成；duplicate receipt 不得
   二次 reopening；
5. core 不识别 `.go`、`_test.go`、LSP、MCP 或具体命令名称。

主要落点：

- `internal/pkg/workorchruntimecontract/runtime_profile_contract.go`；
- `internal/pkg/workorchruntimecontract/runtime_bootstrap.go` / coverage DTO；
- `internal/pkg/workorchdefaults/defaults/templates.json`；
- `internal/modules/application/workorchd/biz/appservice/activationcore/runtime_v2_focus_compiler.go`；
- `internal/modules/kernel/orchestrator/biz/runtimev2/dispatch_commit.go`、assessment/reducer focused files。

退出测试：exact/neutral profile 编译、stable hash、dependency cycle/unknown ID rejection、source
change reopening、duplicate/stale receipt、CAS 前后 restart、focus advance 后 ledger 隔离。

2026-07-23 本地已落地：profile 依赖 DTO/解析/hash、按完整 evidence/target policy 的 focus 拆分、
依赖图 cycle/unknown rejection，以及在 evidence commit 同一 CAS 内的 coverage reopening 与 completed
focus requeue；software-dev default 声明 `source_change -> validation_report`。已覆盖 duplicate、
repository reload（restart shape）和 queue reactivation。仍需 fresh deployed workspace 证明真实 provider
候选能依序完成 source、validation、reopen 后 validation 的正向收敛。

### WP-2：Profile/capability semantic target selector

**状态：landed local / live pending；优先级：P0；影响：SM-2 prerequisite、SM-3 admission。**

目标：空工作区允许通过 Goal/DoD 与 profile 声明产生“可创建目标”选择合同；非空工作区由
有权拥有领域语义的 profile/capability owner提供 selector，File 只执行有界文件事实查询。

实现要求：

1. selector 必须包含 selector ID、target role、owner、subject、revision dependency 与 bounded scope；
2. File owner只能返回 `selected`、`create`、`scope`、`absent`、`ambiguous` 或 `unsupported`；
3. `absent` 在 profile 明确允许 durable create 且提供 canonical prospective target 时，形成可创建
   target observation；不得要求用户重复输入已经包含在 Goal/DoD/profile 中的目标；
4. `ambiguous` 不请求 provider，进入新的 owner observation acquisition、governance 或 typed
   `target_grounding_unresolved`；
5. workspace scope 只能限制 File operation surface，不能自动扩权 Shell/Git/MCP；
6. target observation revision 变化必须失效相交 snapshot/admissibility/candidate authority。

主要落点：defaults/profile、activationcore、File owner、Orchestrator prerequisite/controller、LLM
C3 rendering。禁止在 Orchestrator 添加“选第一个文件”或业务路径模板。

退出测试：empty/one/multiple/large workspace、prospective create、unsupported selector、revision
drift、zero-provider-call ambiguous、snapshot reacquire、restart only sees committed selection。

2026-07-23 本地已补齐 File-backed prospective-create 合同：profile 声明经 activation focus、
prerequisite ID 与 typed EventHub command 封存到 File owner；File 仅验证 workspace-relative
缺失路径并在创建后重选时保留同一 declaration；Orchestrator 只接受与 prerequisite 及 sealed Focus
的 role、selector、prospective path 完全一致的 observation，并只允许该路径上的 durable File operation。
未知 File selector 现返回 typed
`unsupported` observation，并由 SM-2 以 `target_grounding_unresolved` 终止、零 provider call，
不再退化为 adapter/drive error。已覆盖 canonical/escape、snapshot hash、candidate target mismatch
和 receipt-CAS/restart 夹具。未声明 selector 的 profile/capability 仍必须保持 typed unresolved，不能在
Orchestrator 或 File 中猜测路径。

2026-07-24 本地已补齐 `file/profile_declared_candidates`：profile 通过 `selector owner`、opaque
`selector subject` 和有界 canonical candidate refs 显式拥有语义声明；activation 将 profile ID（未显式
给出时）封存为 owner identity，且把完整 binding 写入 Focus、prerequisite、typed EventHub command 与
File observation/hash。File 仅对声明 candidates 与 manifest 做存在性交集：恰好一个为 `selected`，多个为
`ambiguous`，没有为 `absent`，只有被声明为 candidate 的 prospective target 可返回 `create`。因此无关
workspace 文件不能影响选择，ambiguous 不会请求 provider，workspace-tree revision 改变仍按既有 freshness
机制失效 selection/snapshot/admissibility。已覆盖 File one/multiple/create 与 activation sealing；仍需 fresh
deployed workspace 完成 provider → receipt → restart 的 live sequence。

2026-07-23 另补齐默认 `software-dev` profile 的空工作区首个交付物声明：
`source_change` 及先于它编译的 `dodItem`/`goalCoverage` direct-evidence policy 都以
`file/workspace_deliverable` 选择器封存 `workorch-deliverable.md`。DoD work unit 在 profile
deliverable 之前进入 focus queue；只给后者声明 prospective target 会使空工作区仍在第一个 DoD
focus 停止。路径由 profile 数据声明、activation 封存、File owner 验证，Runtime V2 和 File 均不从
Goal/DoD、文件后缀或 provider 文本猜测。该基线由 defaults、activation mapping、leading-DoD focus
sealing 和 File create-selection 的独立回归共同覆盖。上述三个 policy 同时显式接受 `file/write`：File
owner 对 sealed `create` observation 只允许该 prospective path 的首个 durable File 操作，`patch/apply`
在目标尚不存在时不可行；只声明后者会令 provider 的合法首次创建在 sealed contract 阶段被拒绝。任意领域
profile 如需不同目标，仍必须提供自身
semantic selector 或明确的 prospective declaration。

同日补齐 create observation 的状态机闭环：若当前 focus 仅要求 validation，但已提交的 File selection
为 `create` 且不存在 current-focus target-bound durable evidence，SM-2 必须先封存
`produce_state_change`，而不能对尚不存在的目标发起 validation。该 RequiredOutput 仅保留同时满足
File owner selector inventory 与 operation descriptor `AllowsNewTarget` 的 durable kind（当前为
`file/write`）；`patch/apply` 不会再作为空目标的候选。创建 receipt 使 workspace revision 变化后，正常的
prerequisite refresh 重取 selected target/snapshot，后续 validation 与 assessment 的 `TargetRefs` 一律从
同一 `GroundedRepairTargets` 投影。`AllowsNewTarget` 属于 executable descriptor，已纳入
`OperationContractRevision`，因此旧 Run 不会在重启后静默采用新的 create 语义。

### WP-3：Shell structured failure-target observation

**状态：landed local / live pending；优先级：P0；影响：SM-3 receipt input、SM-2 repair grounding。**

目标：Shell 只在能够验证来源时报告 implementation-related target；无法验证时明确返回 nil，
由 reducer 选��其他 prerequisite、governance 或 terminal，不使用自由文本补靶。

最小 DTO 语义：

```text
FailureTargetObservation
  sourceOwnerModuleID
  subjectRef
  targetRole
  targetRefs[]
  observedRevision
  diagnosticClass
  evidenceRefs[]
  contentHash
```

实现要求：

1. Shell parser/adapter 只能从结构化工具输出或已声明 mapping 产生 observation；
2. command target、validation entry、operation `targetRefs`、stderr 文件名和 LLM summary 默认都
   不是 related implementation target；
3. observation 必须匹配当前 operation、owner catalog、focus role 和 revision；
4. multiple targets 保留为 bounded observation，不在 Shell 内选择；
5. duplicate execution key/fence 返回相同 receipt/observation；stale revision 由 Orchestrator拒绝；
6. MCP 保持“仅失败 response 顶层显式 `failureTargets`”的受限基线。

主要落点：Shell owner/validation observation、owner events、runtime contract DTO、Orchestrator
receipt admission。`pkg` 只做字段校验、canonical sort/hash/clone。

退出测试：structured/missing/multiple/mismatched role、success response、duplicate receipt、stale
owner revision、receipt CAS 前后 restart、nil observation 不触发 provider 猜测。

2026-07-23 本地已落地固定 `WORKORCH_FAILURE_TARGET_V1=` stdout JSONL record：仅单条、闭合
schema、与 sealed `TargetRole` 和 Shell owner diagnostic 严格一致的 record 才生成 observation。
Shell owner 以 `shell_runtime_policy` revision、canonical record content hash 与 `workorch://` evidence
ref 写入 receipt；命令 target、validation entry、stderr 和 LLM summary 仍只保留为执行/诊断
lineage。Orchestrator 仅对 MCP 既有显式 response 或此 Shell content-bound protocol 开放 repair
authority，并继续要求 focus role、目标集合和 current owner-catalog freshness 完全一致。已覆盖
structured/missing/multiple/unknown-field/role/diagnostic mismatch、receipt replay、DTO hash/clone 和
stale/missing owner-freshness admission。尚需在 clean-data live 验证实际 profile/tool 是否输出该
protocol；未输出时保持 nil observation，不得 fallback 猜测。

2026-07-24 已补齐 Shell 的跨 owner 回归：真实 Shell 执行结构化失败记录，经 typed EventHub
查询/提交 receipt 后，只有已 CAS 的 observation 会形成 `FailureTarget`，随后由 File owner 为
`implementation.go` 获取 target snapshot；重启只读取该已提交 target/snapshot，重复同
`executionKey + fence` 返回原 receipt，不执行替换 command。另覆盖 `shell_runtime_policy` owner
catalog stale：失效的 admissibility observation 会在同一 SM-2 transition 清除 FailureTarget、
Failure 的 target refs 和 RepairDirective，candidate attempt 标为 invalidated；重启后仅重新获取
该 prerequisite，不会复活旧 target。实现仍不从 validation entry、stderr 或自由文本派生 repair
target。

### WP-4：统一 grounded authority、progress 与 same-signature retarget

**状态：landed local / live pending；优先级：P0；影响：SM-2/SM-3，是主收口面。**

目标：selection、FailureTarget、snapshot、RequiredOutput、RepairDirective、candidate admission、
evidence target 和 assessment delta 始终使用同一组已提交 grounded refs。

实现要求：

1. Orchestrator 校验并 CAS 接受 owner observation；未提交 observation 对 candidate 不可见；
2. `RequiredOutput.TargetRefs`、snapshot refs、admissibility refs 与 state-changing operation refs
   必须一致；scope selector 不可替代 concrete target；
3. accepted receipt 只有形成 coverage/focus/grounded durable change/assessment gap strict delta 时才
   构成 effective progress；
4. require-assessment focus 中，durable change 只使旧 assessment stale；必须等待新 assessment 才
   决定 gap 是否缩小；
5. same gap 保存 accepted-no-progress canonical shapes，并拒绝 cosmetic 变化产生的 exact replay；
6. same-signature retarget 仅在新的 owner observation、grounded target revision 或 coverage dependency
   delta 已 CAS 提交后发生；retarget 必须生成新 authority，不复用旧 dispatch；
7. `RepairAttempts` 是同 focus 总预算，只有 focus advance 创建新 ledger；candidate/provider/
   prerequisite/recovery 各自使用独立预算；
8. state change 已完成但 validation/assessment 缺失时，RepairDirective 必须允许 reducer 选择
   `produce_validation`，不得被旧 `produce_state_change` 固化。

主要落点：`runtimev2/controller.go`、`reducer.go`、`dispatch_commit.go`、
`candidate_admission.go`、`repair_directive.go`、`prerequisite*.go`。

2026-07-23 本地已统一 `FailureTarget > Focus.TargetRefs` 的 repair precedence：CAS 提交的 owner
observation 会令 snapshot acquisition、`RequiredOutput`、repair directive 与 candidate admission 同时
切换到新 target；重启后仅在新 snapshot 已提交时生成 attempt `0` 的新 authority，旧 dispatch/shape
不得复活。owner catalog stale 会原子清除该 observation、failure target 与派生 directive，并退回
sealed Focus target。空 workspace 的 profile-declared prospective create path 是同样封存的 candidate
target，但因其经 File owner 确认不存在，明确不请求 target snapshot；它只能允许该路径上的 durable
File operation，不能扩大为其他 owner 或其他路径的写权限。已覆盖 retarget、snapshot restart、stale
restart、creation receipt crash、owner-failure receipt crash，以及 same-gap rejected shape → owner retarget
→ file-backed process restart；仍需 fresh live 证据。

退出测试：non-target/no-op/weak validation、same-gap/partial-gap/satisfied、exact replay、合法 retarget、
focus advance、duplicate/stale、governance、terminal，以及每个 transition 的 CAS/restart。

## 7. P1 完整性收口包

### WP-5：Candidate sealed context 与 closed schema

**状态：landed local / live pending；优先级：P1；影响：SM-3。**

1. LLM 只渲染 committed Focus、grounded target、snapshot、diagnostic、RepairDirective、
   admissibility 和 no-progress shapes；
2. operation schema 对每个 kind/variant封闭 allowed/required/forbidden parameters；
3. `targetRef` 与 `targetRefs` 采用单一 canonical 映射；进入 contract 后只保留一种权威表示；
4. `produce_state_change` 至少包含一个具有 durable effect scope 的 admissible operation；
5. retry context 必须包含具体 sealed mismatch，不得只返回自然语言“换一种操作”；
6. snapshot/prerequisite 更新后创建新 authority attempt 0，不将它记为 same-contract attempt 1；
7. C3～C5 增量不得改变 C0～C2 `stablePrefixHash`。

退出测试：所有 operation variant schema、unknown parameter、target mismatch、durable effect、retry
contract、stable prefix、uncommitted observation invisible、provider candidate boundary。

2026-07-23 本地已补齐 provider-facing parameter union：每个固定参数都以 `name + 唯一已声明 typed
value field` 的 `additionalProperties:false` object 分支渲染；required/variant operation 也必须显式提供
`parameters`。`file/write` 与 `file/edit` 现在和既有 admission 合同一致，要求 canonical top-level
`targetRefs`。MCP 仅允许 `mcp/tool_call` 的 `input.<name>` 与 `mcp/prompt_get` 的
`arguments.<name>` 动态标量（string/integer/boolean）分支；resource read 仍无动态字段。纯合同层同步
拒绝 string-list 或多 typed-field 的动态 MCP 参数，并把该规则纳入 executable operation catalog revision，
因此已封存旧 Run 不会静默切换到新合同。普通与 repair candidate context 均从 sealed
`RuntimeDecision.RequiredOutput` 填充并校验 `requiredAction`、effect/evidence requirements 与 admissible
operation kinds，消除无 RepairDirective 时 C3 action 为空的双权威表述。Schema 渲染仅位于 LLM block；
状态迁移和 owner I/O 均未迁移。

2026-07-24 补齐 operation dependency 的单一 wire 合同：`dependsOn` 是 operation 内字段，不是候选
顶层字段。LLM boundary 已删除重复 strict decoder，统一委托
`workorchruntimecontract.DecodeRuntimeCandidatePayload`，使普通提交与 output-continuation/restart 使用
同一 DTO、unknown-field gate 与映射。C3 schema 仅在 sealed required-operation sequence 实际声明依赖时
才向模型暴露该字段；普通候选明确要求省略它，仍严格拒绝错误层级。该修复只影响 SM-3 的 provider
payload rendering/decode，不改变 SM-2 reducer、SM-1 Run lifecycle 或 SM-4 recovery/projection。

同日补齐 provider adapter 的 schema 交付链：AI Proxy resolved runtime client 明确声明
`jsonSchemaResponse` capability；HTTP 与 OpenAI SDK adapter 在该 capability 和 non-empty sealed schema
同时成立时，以 `response_format.type=json_schema` 发送完整 schema（普通兼容 provider 不假定支持）。
schema 仍保留在 C3 prompt 供不强制 schema 的模型理解，contract decoder 继续是最终严格 gate。由于当前
candidate schema 使用跨 provider 的条件/union 规则，adapter 不把 `strict:true` 伪装成普适保证；只有在
provider 明确接受该 schema profile 后才可升级 strict mode。已覆盖 HTTP、SDK 实际 wire payload 与
AI Proxy resolved capability；影响仅为 SM-3 provider request rendering。

同日修复 File owner 的 no-op 语义：同内容的 `file_write` 不再调用 `os.WriteFile` 触碰 mtime。工作区
manifest revision 包含 owner 观察到的 mtime；此前这种无内容变化的触碰会伪造 workspace-tree revision，
在下一次 admissibility probe 触发无效的 freshness invalidation，并额外请求一次相同的 state-change
candidate。现以“同内容写入保持 manifest revision”回归测试封存，保证 no-op receipt 不制造 SM-2/SM-3
伪进展或消耗 prerequisite refresh。该项不改变 reducer 的 action 选择、Run lifecycle 或 recovery 预算。

### WP-6：Provider ceiling 与 output continuation 审计

**状态：landed local / live pending；优先级：P1；影响：SM-3/SM-4。**

1. selected model catalog 的 context window 和 max output 是 hard capability；不再叠加通用固定
   4096/8192 output cap 或固定 `MaxRequestBytes`；
2. input bytes 由模型能力、reserved output、利用率和 token/byte 估算派生；
3. catalog 不可用时不得静默冒充完整模型能力；adapter 必须返回明确 unavailable/degraded
   capability source；
4. provider hard limit 内仍被截断的合法 structured output 使用已封存 bounded continuation；
5. fragment/chunk CAS 后重启只继续同一 transaction、stable prefix、sequence 和 tail hash；
6. adapter fallback 常量只能是 provider API 缺省兼容值，不得再次成为 Runtime 通用 ceiling。

退出测试：catalog 65536、small model、catalog unavailable、fragment+suffix、duplicate/hash conflict、
restart、budget exhausted、stable prefix 不变和 fresh provider request audit。

2026-07-24 补齐 fresh-live 的 capability-source 审计闭环：metadata-only `AcceptanceProjection.Audit`
投影 Run 已封存的 provider/model、context window、provider/effective output、input/byte ceiling 与 capability/
configuration revision；live verifier 必须从同一 Run 的 LLM usage event `contextBudget` 找到完全匹配的
provider/model/revision/input/output/byte 记录，且拒绝 `overBudget`。因此 adapter 私有 fallback 或旧固定
4096/8192 output trace 不能再伪装为本次 sealed model ceiling。该投影不携带 endpoint、credential、raw prompt
或 provider payload，不改变 SM-2/SM-3 authority；本地 projection/self-test 已通过，尚待 fresh deployed
provider request 验收。

### WP-7：SM-4 crash/restart 组合矩阵

**状态：landed local / live pending；优先级：P1；影响：SM-3/SM-4。**

逐项覆盖下列 commit boundary：

| 阶段 | CAS 前 | CAS 后响应丢失 | 重启期望 |
| --- | --- | --- | --- |
| owner selection/failure observation | 不可见 | committed observation 可见 | 不重建/不猜测 |
| manifest/admissibility/snapshot | 不可见 | 复用 acquisition identity | 不重复扣 freshness |
| candidate proposal | 不可见 | 复用 proposal | 不重放 provider |
| dispatch seal | 无 dispatch | 复用 ID/key/fence | 不生成第二 dispatch |
| owner receipt | 查询/重放同 key/fence | 返回同 receipt | side effect exactly-once |
| evidence/progress/assessment | 不可见 | 只消费 committed delta | 不重复关 coverage/清 ledger |
| retarget | 旧 authority 保持 | 新 authority 可见 | 不复活旧 target/shape |
| terminal delivery | terminal authority 已定 | Result/Run/Workspace 幂等补齐 | 不改写 terminal |

矩阵至少组合 duplicate result、stale dependency、CAS reload、process restart 和 terminal delivery
failure。只覆盖单个 happy-path restart 不得关闭 WP-7。

2026-07-23 本地审计已将已有回归按 CAS 边界重新索引；它们不是过程日志，而是本 work package
的可执行证据：

| 边界 | CAS 前 / CAS 后 / restart 回归 |
| --- | --- |
| owner selection、failure observation | `TestRuntimeV2StateRepositoryTargetSelectionCommitCrashRestartIsAtomic`、`TestRuntimeV2StateRepositoryOwnerFailureObservationReceiptCrashRestartIsAtomic`（含 failure evidence 已提交后的 response-loss、restart 与 duplicate receipt） |
| manifest、admissibility、snapshot | `TestRuntimeV2StateRepositoryPrerequisiteCommitCrashRestartIsAtomic`、`TestRuntimeV2StateRepositoryTargetSnapshotCommitCrashRestartIsAtomic`、`TestRuntimeV2FileBackedGitQueryReceiptCrashBeforePrerequisiteCommitReusesSnapshot`、`TestRuntimeV2FileBackedGitSnapshotCommitCrashBeforeCandidateReusesCommittedSnapshot` |
| candidate proposal、dispatch seal | `TestRuntimeV2StateRepositoryCandidateProposalAndDispatchSealCrashRestartAreAtomic`、`TestRuntimeV2MainlineRestartsFromPersistedCandidateProposalWithoutProviderReplay` |
| owner receipt、evidence / progress / assessment | `TestDriveRuntimeV2FileBackedRestartRecoversCrashAfterOwnerReceipt`、`TestRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure`、`TestRuntimeV2FileBackedClaimEvidenceAndDispatchCloseCrashMatrix`、`TestRuntimeV2StateRepositoryAssessmentCommitCrashRestartIsAtomic`、`TestRuntimeV2StateRepositorySameGapAssessmentCrashRestartRetainsRepairWindow` |
| stale dependency、retarget | `TestRuntimeV2FileBackedGitStaleRefreshCrashRestartDoesNotReplayOldDispatch`、`TestOwnerFailureObservationRetargetsExplicitFocusAfterSnapshotRestart`、`TestRuntimeV2FileBackedRestartKeepsRejectedShapeAcrossOwnerRetarget`、`TestMCPOwnerCatalogStaleRestartDoesNotReuseFailureTarget` |
| terminal delivery | `TestDeliverRuntimeV2TerminalRestartsAfterOwnerProjectionAcknowledgementLoss`、`TestDeliverRuntimeV2TerminalRestartsAfterResultPersistenceBeforeOwnerProjection`、`TestRuntimeV2TerminalDeliveryBudgetExhaustionRetainsStartupRecoveryInventory` |

本轮新增的 failure-observation response-loss 补齐“owner observation + rejected evidence”的 CAS-after
组合；`TestRuntimeV2FileBackedRestartKeepsRejectedShapeAcrossOwnerRetarget` 补齐 **same-gap rejected
shape → 合法 owner retarget → 完整 file-backed process restart**：旧 shape 在重启后仍被拒绝，而新
owner target 的 distinct shape 以新 authority attempt 0 通过 admission。WP-7 的本地矩阵已落地；
只能由 fresh deployed workspace 验证关闭 live gate。

## 8. P2 边界与文档收口

### WP-8：Application、TUI/Web 与 projection 非权威回归

**状态：baseline landed；优先级：P2；影响：SM-1/SM-4 边界。**

- TUI/Web 首次启动使用 explicit workspace target → workspace default → system default 的统一解析；
- `/run resume` 只发送 scope/intent，不提交 execution-context patch，不区分 restart；
- `/exit` 只管理本地进程/TUI lifecycle，不修改 Run authority；
- activation `202 Accepted` 只代表请求已接受，不代表 terminal/progress；
- EventStream/SSE/Web/TUI 只展示 typed facts，不触发补偿 Resume；
- decision ticket 只在 reducer 产生真实 governance dependency 时创建，普通 contract defect 不提交
  无信息 ticket；
- deployed 验证脚本必须使用隔离 data/config/workspace 目录，不写默认配置或真实 workspace model。

### WP-9：文档状态同步

每个代码切片提交时同步更新：

- 本文 work package 状态与对应 focused test；
- [current-state.md](../../01-overview/current-state.md)；
- [current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)；
- [active overview](overview.md)；
- [unfinished todo index](../../01-overview/unfinished-todo-index.md)。

历史 Run 只能作为反例；不得用旧 terminal Run 证明新二进制已通过。完成后将本文迁入
`docs/90-archive/completed/`，active 只保留真实 open 项。

## 9. 实施顺序与提交切片

必须按下列依赖顺序实施，避免先增强 prompt、后补 authority：

1. `define runtime focus and coverage dependency contracts`（WP-1 DTO/default/compiler）；
2. `ground runtime semantic target selection`（WP-2 owner selection/prerequisite）；
3. `add shell structured failure target observations`（WP-3 owner receipt）；
4. `close runtime grounded repair and retarget transitions`（WP-4 reducer/admission/ledger）；
5. `seal runtime candidate target and schema context`（WP-5 LLM/admission）；
6. `complete runtime v2 restart convergence matrix`（WP-7）；
7. `verify runtime v2 fresh live convergence`（WP-6/WP-8/live/docs）。

每个提交说明必须写明：

- 影响的 SM ID；
- 唯一状态 owner；
- concrete typed I/O；
- CAS commit point；
- duplicate/stale/restart 行为；
- 不构成 progress 的反例；
- focused regression 命令；
- 尚未满足的 fresh live 证据。

## 10. 组件独立回归入口

| 组件 | 最低独立回归 |
| --- | --- |
| runtime contract | `go test ./internal/pkg/workorchruntimecontract -count=1` |
| defaults/profile | `go test ./internal/pkg/workorchdefaults -count=1` |
| activationcore | `go test ./internal/modules/application/workorchd/biz/appservice/activationcore -count=1` |
| File owner | `go test ./internal/modules/blocks/file/... -count=1` |
| Shell owner | `go test ./internal/modules/blocks/shell/... -count=1` |
| MCP owner | `go test ./internal/modules/blocks/mcp/... -count=1` |
| LLM owner | `go test ./internal/modules/blocks/llm/... -count=1` |
| Orchestrator | `go test ./internal/modules/kernel/orchestrator/... -count=1` |
| Run owner | `go test ./internal/modules/kernel/run/... -count=1` |
| workorchd | `go test ./internal/modules/application/workorchd/... -count=1` |
| TUI/process service | `go test ./internal/services/workorch/... -count=1` |
| boundary | `make verify-boundary-contract` |

切片完成后再运行：

```text
make fmt-check
go vet ./...
go test ./... -count=1
make verify-boundary-contract
make verify-runtime-v2-positive-convergence-file
```

## 11. Fresh live 验收序列

必须使用新建 workspace、新建 Run、清洁 data root 和本次构建二进制。不得复用旧 aggregate 或
手工修改 runtime data。

`make verify-runtime-v2-positive-convergence-live` 的 `positive` 与 `bounded_terminal` 模式必须设置
两个 fresh fixture workspace ID 和 `WORKORCH_E2E_ACTIVATE=1`。脚本会把 activation 前的 Run ID 与
本次 activation 后的 Run ID 写入 artifact，并拒绝复用旧 Run；只读的 `liveness` 模式不构成此 DoD
证据。

### 11.1 必须观察到的正向序列

```text
Start
  -> typed bootstrap contains bounded focuses/dependencies/selector
  -> missing owner fact enters AcquirePrerequisite with zero provider call
  -> committed target/diagnostic/snapshot creates authority attempt 0
  -> state-changing candidate matches sealed concrete targets
  -> durable owner receipt is admitted exactly once
  -> validation executes real work and returns truthful quality/diagnostic
  -> assessment strictly shrinks gap or closes coverage
  -> dependency coverage is reopened/closed exactly once when required
  -> focus advances and receives a fresh repair ledger
  -> closeout operation/assessment completes
  -> Result, Run terminal and Workspace release converge
```

### 11.2 失败也必须符合预期

遇到真实不可行情况时，只允许：

- 缺 owner fact：进入 typed prerequisite；
- target ambiguous：governance 或 `target_grounding_unresolved`；
- owner明确不可执行：typed infeasible terminal或有界 retarget；
- provider transport：独立 recovery ledger；
- provider输出截断：bounded output continuation；
- repeated same gap：单调 repair/no-progress budget后 typed terminal。

不得出现：无信息 decision ticket、同 authority 无限 attempt、exact shape replay、projection 触发
Resume、accepted receipt 伪 progress、旧 target 在重启后复活。

### 11.3 Live 审计字段

至少保存并核对：workspace/run、definition/evidence/state revision、focus/coverage、grounded refs、
selector/observation/snapshot revision、authority transaction、candidate attempt、stable prefix、dispatch
ID、execution key/fence、owner receipt hash、repair counters、assessment gap delta、terminal delivery 和
projection watermark。

2026-07-24 已补充只读 `AcceptanceProjection.Audit`：它从 Run owner 已持久化的 Runtime V2 aggregate
投影 scope/revision、focus/coverage、selector/freshness/snapshot metadata、authority transaction、candidate
attempt/stable prefix、dispatch、repair/assessment 与 terminal-delivery counter；不序列化 snapshot content、
candidate parameters 或 owner-private payload，也不成为新的 authority。fresh-live verifier 现强制核对该
审计链与 `/readyz` projection watermark；本地 projection 与 verifier self-test 已通过，仍待重部署后的
fresh fixture 运行完成外部验收。

同日继续补齐 completion lineage：audit 现在保留 accepted/rejected evidence 的 identity/kind/revision/
dispatch/target/coverage metadata，以及 remaining/completed focus queue identity。live verifier 因而不再把
terminal UI 状态当作正向收敛证明：必须观察到 accepted state-change、validation、satisfied assessment，且
bounded dual-focus fixture 还必须有持久化 focus advance 和清空的 remaining queue。该投影不含 evidence
summary/content 或 candidate payload；最终 sequence 仍仅由 fresh deployed run 关闭。

## 12. 可能遗留问题与风险

| 风险 | 处理原则 |
| --- | --- |
| profile 仍未提供某业务域 semantic selector | 保持 typed unresolved；不得把领域规则写进 core |
| Shell 工具没有 structured related target | 返回 nil observation；走 prerequisite/governance/terminal |
| focus 拆分过细导致请求膨胀 | 通过 profile 声明和 bounded context优化，不合并不相容 coverage |
| coverage dependency 图循环或漂移 | activation fail-fast；definition revision 冲突，不运行时猜测 |
| provider catalog 401/不可用 | 明确 target not-ready/capability unavailable，不采用 UI 缓存冒充 ready |
| adapter 私有 4096 fallback 再次变成 ceiling | 增加 capability-source 审计与 provider adapter test |
| exact replay 拒绝后无合法 retarget | 只允许新的 owner observation；预算耗尽进入 typed terminal |
| deployed crash/restart 仍未观察 | WP-7 本地矩阵已落地；fresh deployment 只核对实际 binary 与 owner store 的恢复，不以旧 Run 代替 |
| 文档与部署二进制漂移 | live 记录 commit/build identity；每轮先确认 fresh Run |

## 13. 非目标

- 不恢复 V1 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch 或 auto replan；
- 不在 Orchestrator hard-code 软件开发、Go、LSP、MCP、文件后缀或测试框架规则；
- 不让 LLM、File、Shell、MCP、Run、Result、EventStream、Web/TUI 选择 next action；
- 不通过提高 repair/candidate/prerequisite budget隐藏无进展；
- 不通过 prompt 文案替代 owner observation、candidate admission 或 CAS transition；
- 不为历史 aggregate 增加兼容迁移；验证使用 fresh Run。

## 14. Definition of Done

只有以下条件全部满足，`RUNTIME-VPC` 才能关闭：

1. WP-1～WP-9 的 open/partial 项全部 landed，并链接 focused tests；
2. `pkg` 边界扫描确认没有状态迁移、EventHub/repository I/O 或 action 选择；
3. direct owner、Orchestrator、Run、workorchd、TUI 均可独立回归；
4. transition、invariant、duplicate/stale、budget、CAS/restart 测试完整；
5. C0～C2 stable prefix 在 snapshot/diagnostic/repair/retarget 中保持不变；
6. fresh live 完成“diagnostic → snapshot → corrected durable change → validation → assessment delta
   → focus advance → terminal delivery”的完整序列；
7. fresh live 不再出现 rejected shape 无合法纠正路径、无信息 governance ticket或 accepted receipt
   伪 progress；
8. File governance、provider continuation、projection exactly-once 均有 clean-data deployed 证据；
9. full test、vet、boundary 和 positive convergence verifier 通过；
10. current-state、active overview、unclosed matrix 和 todo index同步，本文归档为 completed。

仅让 Run 更早失败、仅修改 prompt、仅提高预算、仅通过单组件测试或单次偶然 live 成功，均不
构成完成。
