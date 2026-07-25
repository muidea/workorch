# Runtime V2 持续正向收敛：执行收口规格

Status: active

Type: implementation closure plan

Last Updated: 2026-07-25

Execution Revision: R4 — single-source contract, reachable-owner closure, then convergence gates

## 1. 目的、范围与唯一性

本文件是 `RUNTIME-VPC` 唯一可产生 Runtime V2 代码任务的活动执行规格。它替代此前在同一文件中
并存的 C4/C5/C6、A-3M、P4-A 子表和 live 修复清单：那些内容中已关闭的部分只作为回归事实保留，
不再定义“下一步”。

R4 同时完成文档权威对齐：`00-core/runtime-stage-contract.md` 现只保留 Runtime V2 四状态机、
权威与禁止迁移；已删除与 V2-only 冲突的 V1 graph/step、`ExecutionContext`、自动 successor 和
旧 repair-flow 叙述。后续不得从 archive 或其他 flow 文档恢复这些术语作为实现依据。

本轮目标不是降低合同强度以迁就不稳定的 LLM 输出，而是证明完整合同链只有一个解释，并在每个
持久化边界中恢复：

```text
SM-2 authority / prerequisite / required output
  -> CandidateContractManifest
  -> LLM request renderer and strict decoder
  -> SM-3 candidate admission and sealed dispatch
  -> owner feasibility / operation receipt
  -> SM-2 evidence / coverage / focus
  -> SM-1 terminal projection, driven by SM-4 recovery
```

近期 live 中的 `unknown field`、参数白名单、目标或 effect 不匹配、replay、无意义 governance、
prerequisite refresh 耗尽以及重启后不收敛，均按这条链定位第一个失真点；不得按错误文本逐点热修。

本计划只覆盖 Runtime V2 生产路径。禁止恢复 V1 `StagePacket`、`NextAction`、
`DispatchNextStep`、`ExecutionContextPatch`、auto-replan successor、map/JSON authority bridge 或任何
兼容 fallback。当前阶段不做历史数据兼容；验证一律使用 fresh Run。

约束优先级为：

1. [Runtime V2 核心合同](../../00-core/runtime-stage-contract.md)；
2. [request/cache 从属合同](../../00-core/runtime-request-cache-note.md)；
3. `internal/modules/kernel/orchestrator/readme.md` 与
   `internal/pkg/workorchruntimecontract`；
4. 本文件；
5. [组件合同与回归矩阵](component-contract-and-regression-closure-matrix.md)。

## 2. 当前判定

### 2.1 根因与处理原则

当前阻塞的主要风险不是“校验过严”，也不是仅靠提高 token、bytes、retry 就能解决，而是同一事实曾在
manifest compiler、prompt renderer、wire DTO、decoder、admission、owner receipt 与 fixture 中拥有多份
解释。模型即使遵循可见提示，也可能因另一层的字段名、默认值、allow-list、target 或 effect 规则漂移
而被拒绝。若拒绝后的 correction 只得到自然语言摘要，又没有确定的合法候选空间，就会表现为 shape
replay、错误 ticket 或 refresh 空转。

严格的 target、effect、coverage、owner admissibility、参数白名单和 evidence 质量仍然是安全边界，
不可放宽。收口的判断标准是：

- 每个可提交字段只有一个 typed 编译来源；
- renderer、schema、decoder、admission、owner feasibility 与 fixture 对同一 manifest 闭包；
- 不完整 document、transport、configuration、stale prerequisite 与 complete-invalid candidate 走不同且
  有界的状态机路径；
- 任一 CAS 前/后中断后，旧 document、旧 resource、旧 receipt、旧 dispatch 都不会复活或重放。

### 2.2 适应不稳定 LLM 的执行模型

LLM 输出不稳定不是降低合同的理由，而是必须缩小每次 LLM 可选择空间的原因。每张代码卡必须先
固定 deterministic owner facts，再让 LLM 只填写同一 `CandidateContractManifest` 允许的候选字段。

| 层 | 先后关系 | 失败归属 | 不允许的替代方案 |
| --- | --- | --- | --- |
| authority / prerequisite | owner receipt、local mapping、freshness commit 先完成 | SM-2 | 从 Goal/DoD、目录、错误字符串或旧 prompt 猜资源 |
| candidate protocol | manifest renderer → strict decoder → admission → owner feasibility | SM-3 | unknown-field 忽略、双 schema、allow-list 旁路 |
| durable execution | sealed dispatch → exact fence receipt → evidence | SM-3/SM-2 | UI Resume、重放 operation、把 action 当 evidence |
| recovery/display | aggregate reload → bounded drive → derived projection | SM-4/SM-1 | Web/TUI watcher 重建 authority 或补偿状态 |

因此 live 症状只用于取得脱敏 trace 并生成最小 deterministic fixture；它不能直接授权修改 prompt、
token/byte 上限、预算、TUI/Web 或合同宽严度。一次卡内代码改动只能消除一个首失真点，并须保留其
负向断言（零 query/attempt/dispatch/ticket/effect 或精确一次 effect）。

### 2.3 已关闭的回归基线

以下内容已通过定向验证，必须保留回归；它们不是新的代码工作卡。

| 基线 | 已证明的合同 | 不得回退 |
| --- | --- | --- |
| C4 | request session first、fragment/continuation、transport/configuration/stale 分流 | 不完整 provider document 不进入 candidate attempt |
| C5 | complete document strict admission、一次 correction、proposal/dispatch restart | 不以 EventHub 内存 body 或 compatibility DTO seal candidate |
| C6 | semantic replay 判等、attempt 1 原子 exhaustion、CAS reload | 不用 operation ID、JSON 顺序或错误字符串绕过 replay fence |
| P0–P2 | 单一 `CandidateContractManifest`，canonical renderer/wire/strict decoder round-trip | 不恢复 legacy decision/schema fallback |
| P3 | admission、diagnostic、correction、governance route 与 file-backed restart matrix | 不创建无信息 ticket，不把 terminal fence 变为 retry |
| P4-A0–A3M | File/Git/Shell 单 owner receipt，以及三 owner merge、精确 stale intersection、fresh-process stale/reload、两条 ledger source split | 不扩大 stale 影响集，不交叉扣减两条 refresh ledger |

已验证的三 owner 组合回归包括：

- `TestRuntimeV2FileBackedAdmissibilityThreeOwnerMergeRestart`；
- `TestThreeOwnerAdmissibilityStaleIntersection`；
- `TestRuntimeV2FileBackedAdmissibilityThreeOwnerStaleRestart`；
- Git read-only stale 和 receipt-resource-change 的 CAS 前/后 restart 用例。

它们要求 File、Git、Shell 的真实 owner 分别通过 typed EventHub result 提供 receipt；Run owner 只 CAS，
SM-2 只按 `(domain, ownerModuleID, subjectRef, expectedRevision)` 的精确相交退休 observation。所有这些
用例均断言零 candidate session/attempt、provider call、sealed dispatch、governance ticket、operation 与
durable effect（receipt-resource-change 用例只读取已持久化 receipt，且不得重放它的 effect）。

## 3. 不可违反的状态机和组件边界

| 状态机 | 唯一职责 | 禁止事项 |
| --- | --- | --- |
| SM-1 Run lifecycle | 持久化 Run 生命周期并消费 Orchestrator 的 blocked/terminal 投影 | 选择 action、修改 candidate、补偿 Resume |
| SM-2 authority/focus | reducer、prerequisite、freshness、focus/repair、required output、terminal decision | 调 provider、执行 capability operation、直读其他 owner repository |
| SM-3 candidate/dispatch | request session、complete document、attempt、admission、sealed dispatch/evidence transaction | 从错误文本选择下一 action、修补 authority |
| SM-4 drive/recovery/projection | 有界 drive、CAS reload、singleflight、派生投影重试 | 由 TUI/Web/projection 重建 authority 或重放副作用 |

| 组件 | 可承担的职责 | 独立回归边界 | 明确禁止 |
| --- | --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | DTO、合同描述、Normalize/Validate/Clone/Hash 等纯无状态 helper | pure contract/property test | 状态迁移、CAS、EventHub、I/O、retry 决策 |
| `orchestrator/biz/runtimev2` | SM-2 reducer、freshness、admission 的纯/聚合逻辑 | reducer/invariant/stale/duplicate/budget test | provider-specific 兼容、跨 owner repository |
| `orchestrator/biz` | session/content CAS、drive、typed EventHub prerequisite/operation 编排 | file-backed fresh-process restart test | 从提示词或错误字符串推断领域修复 |
| `kernel/run` | aggregate CAS、reload、startup inventory | CAS/reload/idempotency test | action、candidate、evidence quality 判定 |
| LLM block | sealed manifest rendering、provider request、strict decode、provider ceiling receipt/audit | renderer/decode/provider-ceiling producer-consumer test | 修改 Runtime aggregate、消耗 repair budget |
| File/Git/Shell 及其他 capability owner | 本 owner 的 prerequisite/feasibility/operation 与 typed receipt | owner producer-consumer and receipt test | 扩展授权目标、关闭 coverage、写 Runtime aggregate |
| Decision/Approval owner | 只处理真实 governance dependency 与 bound resolution | ticket/binding/reload test | 把 candidate rejection 变成泛化 decision |
| `workorchd`、TUI、Web、eventstream | activation command、审计、只读投影 | projection/high-watermark/UI command test | authority rebuild、隐式 Resume、状态补偿 |

跨 owner I/O 只能是具体 typed EventHub command/result；禁止跨 owner service/repository、`map[string]any`、
`any`、JSON/reflect bridge 和生产 compatibility adapter。

## 4. 工作方法：一张卡、一个首失真点、一个退出条件

每张卡严格按以下顺序执行，任一项失败就停在本卡：

1. 用最小 typed input 写失败的组件级测试，同时断言正向结果和不得变化的 ledger/side effect。
2. 存在持久化边界时，用同一输入升级为 file-backed fresh-process 测试：销毁 repository adapter、Run
   owner、Orchestrator、EventHub、真实 responder 和临时配置对象后重建，不能复用进程内对象。
3. 仅修改本卡列出的 owner。发现缺字段时，先补 owner-defined DTO 和 producer/consumer 回归；不引入
   test-only runtime owner、通用 receipt 或兼容路径。
4. 先跑本卡的选择器，再跑相邻 owner、Run CAS 和边界门禁。全部通过后才更新状态并解锁下一卡。
5. P6 以前不以 live 成功、TUI/Web `/run resume`、人工 decision、修改 prompt 或提高预算作为完成证据。

每个持久化用例必须覆盖：normal、duplicate、CAS 前崩溃、CAS 后响应丢失和 fresh reload。若某格在语义
上不可达，测试必须显式证明不可达，而不是省略。

### 4.1 live 症状的归类规则

| 分类 | 第一个判定边界 | 回流工作卡 | 禁止的伪修复 |
| --- | --- | --- | --- |
| F0 request/context | manifest、C0–C5 rendering、strict decode | P0–P2 回归 corpus | 忽略未知字段、截断 P0–P4、提高 retry/budget |
| F1 candidate route | admission、correction、semantic replay、governance binding | P3 corpus/restart | UI Resume、泛化 decision、按 message 猜 operation |
| F2 prerequisite | receipt acquisition、observation、stale/invalidation、refresh ledger | P4-A | provider call、candidate attempt、live 热修 |
| F3 authority resource | selection、snapshot、failure target | P4-B | 从 Goal/DoD 文本猜文件、扩大 owner authority |
| F4 convergence fact | evidence、coverage、focus advance、assessment/failure | P4-C/D | 历史 summary/evidence 关闭当前 coverage、清零总 repair budget |
| F5 side effect/delivery | seal/claim、receipt、evidence、governance fence、terminal projection | P5 | 重放 provider/durable effect、projection 重建 authority |
| F6 display/activation | committed revision、activation/projection | P5/P6 的观察用例 | 修改 reducer、watcher 自动 Resume |

一个 live 样本只能落在**第一个**失真边界。先记录脱敏 `ContractTrace`（manifest/diagnostic/shape/request
hash、ledger delta、revision），再形成 deterministic fixture；不得直接改模型、提示词、预算或运行时分支。

## 5. 唯一串行执行队列

下表是唯一“下一步”定义。`done` 只表示必须保留回归；只有第一张 `in_progress` 卡允许产生代码改动。

| 顺序 | 工作卡 / 状态 | 单一目标 | 涉及状态机和 owner | 解锁条件 |
| --- | --- | --- | --- | --- |
| Q0 | P0–P3、A-0–A-3M `done` | 维持既有合同与三 owner freshness 基线 | 已关闭基线 | 所有后续卡均以此为回归前置 |
| Q1 | **A-4 reachable prerequisite owner inventory `done`** | resolver 的九种 prerequisite 已完成 owner、receipt 类型、CAS/restart 或 local mapping 证据 | SM-2 + typed owner；Run 仅 CAS；SM-4 仅重启 drive | A-4-0 至 A-4-4 已全部完成 |
| Q2 | **A-5 mixed-owner budget/exhaustion `done`** | 两条 prerequisite refresh ledger 的混合 owner 上限、typed terminal 与无 query loop | SM-2 freshness/reducer + Run CAS | 已完成；保持回归 |
| Q3 | **P4-B resource/focus `done`** | selection、snapshot、failure target 取得/退休的原子性 | SM-2 + File owner typed receipt | 已完成；保持回归 |
| Q4 | **P4-C receipt/evidence/coverage/focus `done`** | accepted receipt 到 evidence、coverage、focus advance 的恰一次推进 | SM-2；SM-3 仅消费 sealed dispatch | 已完成；保持回归 |
| Q5 | **P4-D assessment/failure/repair `done`** | assessment、failure、repair budget 的恰一次推进 | SM-2；SM-3 仅消费 sealed dispatch | 已完成；保持回归 |
| Q6 | **P5 side effect/delivery `done`** | seal/claim、owner receipt、governance fence、terminal delivery 的 exactly-once | SM-3、SM-2、SM-1、SM-4 | 已完成；保持 P5-A～P5-D fresh-process 回归 |
| Q7 | **P6 isolated scripted/live `in_progress`** | 已证明脚本的部署、投影和真实 provider 可达性 | SM-1/SM-4 observation only | P0–P5 全部关闭；继续保留隔离 artifact 与全仓门禁 |

## 6. Q1 / A-4：可达 prerequisite owner inventory

### 6.1 问题定义

`resolveRuntimeV2Prerequisite` 是 production reachability 的唯一入口。此前已充分验证 Manifest、Target
Selection、Target Snapshot、Operation Admissibility 和 Git Capability State 的代表路径，却尚未对其余可达
kind 做逐项 file-backed CAS 前/后证明。若忽略它们，新的 prerequisite 即可在 restart 后重新查询、泄漏
未提交 resource，或把 local filesystem/LLM owner 的结果错误带入 candidate 事务。

本卡不重新讨论目标选择语义、candidate JSON 或 operation side effect；它只证明 prerequisite acquisition、
commit、reload 和 retirement 的权威边界。

### 6.2 可达性清单与证据要求

| prerequisite kind | 当前来源 / owner | 结果归属 | A-4 必须证明 |
| --- | --- | --- | --- |
| Manifest | File typed EventHub | File manifest + observation | 已完成；保持回归 |
| TargetSelection | File typed EventHub | selection + observation | 已完成；保持回归 |
| TargetSnapshot | File typed EventHub | snapshot + observation | 已完成；保持回归 |
| OperationAdmissibility | File/Git/Shell typed EventHub | owner inventory + observation | 已完成三 owner merge/stale；保持回归 |
| CapabilityState | Git typed EventHub | Git state + observation/binding | 已完成；保持回归 |
| ProviderCeiling | LLM typed EventHub | provider ceiling + observation | **已由 `TestRuntimeV2FileBackedProviderCeilingCommitCrashRestart` 覆盖 before/after-CAS fresh-process matrix** |
| Capability | capability owner typed EventHub，owner 由 definition 指定 | capability set + owner-catalog observation | File、Git、Shell receiver 已分别由 `TestRuntimeV2FileBackedFileCapabilityCommitCrashRestart`、`TestRuntimeV2FileBackedGitCapabilityCommitCrashRestart`、`TestRuntimeV2FileBackedShellCapabilityCommitCrashRestart` 覆盖；**仍须按真实可达 owner 分类，并为其余 receipt 实现补证明或不可达证明** |
| FailureTarget | Orchestrator-local typed failure 映射 | failure target | **已完成** exact typed identity、invalid input、signature/target 变化 retirement 与 CAS 前/后全 actor restart：`TestRuntimeV2FileBackedFailureTargetCommitCrashRestart`、`TestRuntimeV2StateRepositoryFailureTargetCommitCrashRestartIsAtomic`、`TestRuntimeControllerReacquiresFailureTargetWhenRecordedTypedFailureChanges` |
| ValidationScratch | Orchestrator-local filesystem prerequisite | scratch reference/revision | **已完成** scoped root、resource incarnation revision、CAS 两侧、全 actor restart、duplicate/no-op 与无关 stale 隔离：`TestRuntimeV2FileBackedValidationScratchCommitCrashRestart`、`TestRuntimeV2FileBackedValidationScratchStaleInvalidationCrashRestart`、`TestRuntimeV2StateRepositoryValidationScratchStaleInvalidationIsAtomic` |

任何新增 `PrerequisiteKind`、或 switch 中新增 case，必须在同一提交更新这张清单并增加对应
before-CAS/after-CAS 或 reachability-unreachable test；不能先上线后补覆盖。

### 6.3 A-4-0：冻结可达性和 owner 分类（`done`）

**影响：SM-2；无外部副作用。**

先新增 resolver reachability test，枚举全部 `PrerequisiteKind` 并对每种结果断言恰好一个分类：

1. 外部 owner typed EventHub receipt；
2. Orchestrator-local deterministic mapping；
3. definition/profile 在当前 production path 不可达，且 reducer 无法生成它。

测试还必须断言 external case 的 command destination 等于 `OwnerModuleID`（或该 kind 固定的 LLM/File/Git
owner），不得落到通用 responder。local case 不得伪造 EventHub owner receipt。若 `Capability` 使用动态
owner，测试需从实际 activation/definition 逐项生成 File、Git、Shell、Decision/Approval 等真实可达样本；
未被 production profile 声明的 owner 必须返回 typed unsupported/infeasible，而不是由默认能力集合兜底。

**退出条件：** 本节 6.2 每行均有 producer、consumer、persistent result、dependency/observation（若有）、
测试定位和分类结论；unknown kind 在无 provider/session/attempt/dispatch/ticket/effect 的前提下 fail-fast。

已完成：`TestRuntimeV2PrerequisiteResolverInventoryUsesProductionRequirementSources` 直接调用 production
`activationcore.RuntimeV2Bootstrap`，再驱动 SM-2 reducer 的 target-selection/target-snapshot 路径和 typed
failure-target 路径，覆盖 contract 的全部九种 `PrerequisiteKind`；它不维护第二份手写 kind 清单。
每个 activation requirement 断言 canonical owner 与 external/local 分类，动态 File requirement 同样断言
owner。`TestRuntimeV2PrerequisiteResolverClassificationCoversContractKinds` 还调用实际 resolver 验证 unknown
kind 在任何 Hub/owner interaction 之前 fail-fast。各 external owner 的 producer/consumer 与 CAS matrix 继续
由 A-4-1/A-4-2 的对应 receiver 用例承担。

#### A-4-0 完成证据

已以仅读取 contract/production bootstrap 的 reachability inventory test 取代手写清单。其输出覆盖：

| 字段 | 要求 |
| --- | --- |
| `PrerequisiteKind` / requirement ID | 来自 reducer 实际可生成的 typed requirement |
| classification | `external_owner`、`orchestrator_local` 或 `unreachable` 三选一 |
| owner / command / result | external 必须是 canonical `OwnerModuleID` 与具体 EventHub command/result；local 不得伪造 owner |
| persisted fact | `Facts.Resources` 中唯一写入位置，以及是否写入 observation |
| freshness dependency | observation 的 domain/owner/subject/revision，或明确 `none` |
| recovery fixture | owner producer-consumer test、before-CAS、after-CAS、fresh-process reload 的测试名 |
| unsupported path | 无 responder/不支持三元组的 typed failure code，及零副作用断言 |

执行顺序固定为：先让 inventory 覆盖所有枚举/实际 bootstrap，再补一个缺项的 owner 合同；只有 inventory
报告完整后才补该 kind 的 crash/restart matrix。不得先以默认 capability、generic governance 或空结果
隐藏缺项。

### 6.4 A-4-1：ProviderCeiling 的完整 CAS/restart 闭环（`done`）

**影响：SM-2 acquisition/receipt；SM-4 restart；LLM owner 只返回 typed ceiling。**

在 `runtime_v2_drive_test.go` 增加独立 file-backed 场景，参照 manifest restart harness，不复用旧 Hub、
LLM module、Run repository、workspace responder 或配置对象：

1. 用 `t.TempDir`、`t.Setenv` 和 cleanup 设置 test-only LLM config；不得修改运行环境的 default chat、
   workspace `DefaultLLMTarget`、`config/`、`.workorch-data` 或 `.workorch-workspaces`。
2. bootstrap 只要求 `ProviderCeiling`，activation provider target 固定为测试专用
   `custom-test/model-1`；启动 File-backed Run repository、Run module、LLM module 和只计数的 query spy。
   spy 不能作为 responder。
3. fault repository 在 `Facts.Resources.ProviderCeiling != nil` 的 save 点注入一次中断。
4. **before-CAS：** 已持久化 aggregate 不含 ceiling/observation；销毁全部进程对象后重建，LLM acquire
   query 恰增加一次；仅新查询结果可提交。
5. **after-CAS：** reload 的 ceiling、configuration revision、snapshot/observation ID、content hash 和
   acquisition binding 完全相同；LLM acquire query 增量为零。
6. 两侧均断言零 candidate request/session/attempt、provider completion、sealed dispatch、governance
   ticket、operation receipt/effect；duplicate response 不能增加 revision 或 observation。

**完成证据：** `TestRuntimeV2FileBackedProviderCeilingCommitCrashRestart` 已以真实 LLM module、query
spy、before/after-CAS fault repository 和新建的 Run/EventHub/LLM/config source 通过上述矩阵。它同时断言
candidate/session/attempt、provider completion、dispatch、ticket、operation receipt/effect 均为零。

provider configuration 不可用时仍须走已定义的 typed configuration route，不得被错误归为 candidate
rejection 或 prerequisite refresh；该行为继续由 LLM producer-consumer 和 request-session 回归守护。

### 6.5 A-4-2：Capability 的真实 owner 覆盖（`done`）

**影响：SM-2 acquisition/receipt；各 capability owner 的单 owner 状态；Run 仅 CAS。**

`Capability` 不是一个通用的“已具备能力”布尔值。它由 definition 的 `OwnerModuleID`、capability class 和
required kinds 共同限定，必须通过具体 owner typed command/result 获得。执行顺序如下：

1. **已完成的 receiver：** File state-change capability 已由
   `TestRuntimeV2FileBackedFileCapabilityCommitCrashRestart` 在真实 File module 上覆盖 CAS 前/后、
   duplicate/reload 和零 candidate/dispatch/effect。Git state-change capability 已由
   `TestRuntimeV2FileBackedGitCapabilityCommitCrashRestart` 在真实 Git module、独立 Git adapter/receipt
   store、全 actor restart 上证明相同边界。Shell validation capability 已由
   `TestRuntimeV2FileBackedShellCapabilityCommitCrashRestart` 在真实 Shell module、全 actor restart 上覆盖
   CAS 两侧、catalog observation 与零 candidate/dispatch/effect。三者均通过同一 typed
   `AcquireRuntimeCapabilitiesCommand/Result` 及 `ResolveRuntimeCapabilities` 的 canonical receipt constructor；
   但它们仍分别验证 owner subscription、允许 kind 和错误隔离，不能替代 Decision、LLM、MCP、SSH 或 Web。
2. 从当前 production profiles/definitions 枚举实际会产生 capability prerequisite 的 owner/class/kind
   三元组；将其分为 File、Git、Shell、Decision/Approval 与其他已注册 capability owner。
3. 每个三元组先有 owner producer-consumer test，验证结果不能夹带其他 owner capability；缺 owner、空
   `OwnerModuleID`、required kinds 不匹配均需 typed failure，不能静默返回空 capability set。
4. 对每个**不同 receipt/observation 实现**补一个 file-backed before/after-CAS matrix。共享同一 owner
   receipt 实现的多种 kind 可在一张参数化用例中覆盖，但报告必须列出所有三元组。
5. capability 不附带 observation 时，显式断言其不参加 stale retirement；附带 observation 时，验证它只
   随精确 dependency stale 退休，并使用正确的 refresh ledger。

**退出条件：** 任何 definition 可声明、但当前 module 没有 responder 的 capability prerequisite 在 activation
或 acquire 处确定失败；绝不变成 LLM request、默认 capability、generic governance 或无限 retry。

#### A-4-2 剩余实施分包

`Capability` 的关闭单位是“不同 receipt/observation 实现”，不是 capability 字符串数量。A-4-0 的
production bootstrap/reducer inventory 与当前 owner inventory 共同覆盖 File、Git、Shell、LLM、Decision、
SSH、MCP、Web；Approval、assessment、memory 等非 capability receiver 不以默认集合伪装支持，而是在
owner/class/kind acquire 处走 typed unsupported route。

每个分包必须按以下次序交付，且单独提交：

1. owner producer-consumer：验证 canonical owner、required kinds、receipt 内容和其它 owner 隔离；
2. capability commit：验证 `CommitPrerequisiteResult` 的唯一 resource/observation 写入与 duplicate no-op；
3. file-backed before-CAS：摧毁全部进程对象后不泄漏未提交 result；
4. file-backed after-CAS：重建后零 owner query、零 candidate/session/attempt/dispatch/ticket/operation/effect；
5. 若带 observation：只对相交 dependency stale 退休，且只扣对应 ledger；若不带：显式断言不参加 stale；
6. 无 responder/owner mismatch/empty kinds：typed unsupported/infeasible，且步骤 4 的所有计数仍为零。

同一 owner 的多个 kind 只有在**同一 command/result、hash、observation 与 commit 分支**完全共享时，
才能使用参数化测试合并；报告中仍须列出每个三元组。File 的既有通过用例只可作为这个规则的样本，
不能替代其他 receiver。

#### A-4-2 当前 receiver inventory 收口证据

Capability receiver 的支持 kind 不再散落在各 `biz.handleRuntimeCapabilities` 调用点：File、Git、Shell、
MCP、SSH、Web、LLM、Decision 各自在自己的 `pkg/common.RuntimeCapabilityKinds()` 声明只读 inventory，
handler 只将该 inventory 交给 shared `ResolveRuntimeCapabilities`。该 helper 是唯一的 result、catalog
revision、observation hash 与 dependency 构造点；`RuntimeCapabilityKinds()` 每次返回新 slice，调用方不能
变更 owner authority。此处 `pkg/common` 仅含常量/纯 helper，不含状态迁移、Hub、I/O 或 retry。

不匹配或无 responder 的 owner/class/kind 不再退化为普通字符串错误：
`RuntimeCapabilityUnsupportedError{owner,class,kinds,reason}` 是 typed prerequisite failure。SM-4 将它
分类为 `runtime_capability_unsupported` / terminal contract defect，而不是 `CapabilityUnavailable` 的治理
分支，因此不会创建泛化 decision ticket、candidate/session/attempt/dispatch 或 effect。
`ResolveRuntimeCapabilities` 的 unsupported kind、`AcquireAndCommitRuntimeCapabilitiesPrerequisite` 的 absent
responder 与 `classifyRuntimeV2DriveFailure` 均保留该类型；
`TestRuntimeCapabilityOwnerInventoriesProduceCanonicalReceipts` 枚举八个 registered owner 的 inventory、
defensive-copy、class、result 与 owner-catalog observation；
`TestRuntimeCapabilityReceiverModulesPublishDeclaredInventory` 另外以真实 MCP、SSH、Web、LLM、Decision
module/EventHub producer-consumer 验证 subscription 与 owner-isolated receipt。
`TestResolveRuntimeCapabilitiesRejectsUnsupportedKind`、
`TestAcquireRuntimeCapabilitiesPrerequisiteReturnsTypedOwnerUnavailable`、
`TestClassifyRuntimeV2DriveFailureKeepsUnsupportedCapabilityOutOfGovernance` 覆盖 unsupported/absent-owner
边界。

A-4-2 已满足退出条件：所有 registered receiver 均有 owner inventory 和 producer-consumer 证据；所有
receipt 共享同一 command/result/hash/observation/commit branch，File/Git/Shell 已覆盖实际 owner 的 CAS
两侧与 fresh-process restart；unsupported/absent owner 不会进入 provider、candidate、ticket 或 effect。
后续不得重新复制同构 capability crash harness，下一张唯一工作卡为 A-5。

### 6.6 A-4-3：FailureTarget 的 local mapping 闭环（`done`）

**影响：SM-2；无跨 owner I/O。**

FailureTarget 必须仅从当前 persisted typed `Facts.Failure.TargetRefs` 和 signature 派生，不从错误消息、
workspace tree、历史 evidence 或 LLM 输出猜测。补充：

- pure reducer/local adapter 测试：缺 typed failure 或空 target 时 fail-fast；结果只包含当前 signature 和
  target refs；重复 commit canonical no-op；
- file-backed before-CAS：未提交 FailureTarget 在 restart 后不可见，重建后只能从同一 persisted failure
  重新派生；
- after-CAS：不改变 target/signature、不重复生成 observation、无 provider/attempt/dispatch；
- failure signature/target 变化时，旧 FailureTarget 必须按 SM-2 的 resource/freshness 合同退休；该跨
  focus 具体选择语义留给 P4-B，不在本卡扩大 target 集。

**退出条件：** local prerequisite 仍是 Orchestrator 内部映射，不被包装成 EventHub “假 owner”，且其 CAS
行为与外部 receipt 一样可验证。

已完成：local adapter 对缺失 typed identity fail-fast；commit 与 satisfied 判定均要求 mapping 与当前
`Failure.Signature + TargetRefs` 精确相等；`TestRuntimeV2FileBackedFailureTargetCommitCrashRestart` 覆盖
Run-owner CAS 前、CAS 后响应丢失、销毁 Hub/Run owner/Workspace responder 后的 full fresh-process restart，
并断言零 observation、provider/session/attempt/dispatch/ticket/effect；
`TestRuntimeControllerReacquiresFailureTargetWhenRecordedTypedFailureChanges` 覆盖 signature/target set 改变时
旧 mapping 在记录新 failure 的同一 SM-2 CAS 内清除并回到既有 prerequisite。duplicate no-op 继续由
`TestRuntimeV2StateRepositoryFailureTargetCommitCrashRestartIsAtomic` 守护。

#### A-4-3 精确实现边界

本卡只允许读取已持久化的 `Facts.Failure`，并且结果 identity 必须由 failure signature 与 canonical
target refs 决定。实现不得读取 workspace tree、manifest、历史 evidence、LLM body 或 diagnostics 文本来
补 target。推荐测试断言如下：

- `Facts.Failure == nil`、signature 为空或 target refs 为空时，在 prerequisite acquisition 直接返回 typed
  failure，且 aggregate/query/attempt/dispatch/ticket/effect 均不变化；
- 相同 signature/refs 的 duplicate commit 为 canonical no-op；
- fault 在 local result 写入前触发时，fresh process 看不到 result；fault 在 CAS 后/响应前触发时，reload
  复用完全相同的 result，不产生新 observation；
- signature 或 target set 变化时，旧 mapping 的 retirement 只能由 SM-2 invalidation 发生；本卡不扩大新
  target 的选择权，也不跨 focus 重建 selection。

### 6.7 A-4-4：ValidationScratch 的 local filesystem 闭环（`done`）

**影响：SM-2 local prerequisite；SM-4 restart；不得触及 candidate/operation。**

Validation scratch 是执行前置资源而非交付物证据。需验证：

当前实现已移除 `os.TempDir()` fallback：`AcquireValidationScratchPrerequisite` 只能接收 Workspace owner
已持久化的 `InternalDirectory`，并在其下建立 `validation-scratch/<workspace>/<run>/<focus>`。缺少该 root
会在 SM-2 prerequisite acquire 处确定失败；它不会再写入 process-global temporary directory。
`TestRuntimeV2FileBackedValidationScratchCommitCrashRestart` 已覆盖该 resource 的 before/after-CAS、全进程
重建、路径 root containment 与零 candidate/dispatch/effect。scratch 同时持久化本地 incarnation revision
marker：同一路径被删除后重建会获得新 revision，不能被旧 invalidation receipt 误当作同一资源。目录删除、
非目录、不可写、marker 丢失或 revision 不匹配均在 candidate 前转换为 SM-2 read-only stale。
`TestRuntimeV2FileBackedValidationScratchStaleInvalidationCrashRestart` 已覆盖 stale invalidation CAS 前/后
的全 actor fresh-process matrix；`TestRuntimeV2StateRepositoryValidationScratchStaleInvalidationIsAtomic`
覆盖 duplicate/no-op、无关 stale 零 revision/ledger 变化。该卡已关闭。

- scratch path 仅由 workspace/run/focus 等 scoped identity 推导，经过路径安全规范化，且在 test 中使用
  临时根目录；不得写运行环境的全局临时目录或 workspace deliverable；
- before-CAS 重启后，未提交 scratch reference 不得进入 aggregate；允许重建同一 deterministic scoped
  resource，但不得将旧进程的未持久化 reference 视作已满足 prerequisite；
- after-CAS 重启后，只有 committed reference/revision 可被使用；目录丢失、权限异常或 revision 不匹配
  必须进入 typed prerequisite failure/reacquire，不得进入 provider/attempt；
- duplicate 和无关 stale 不增加 revision/ledger；其清理策略必须由 owner 生命周期或 validation policy
  明确定义，不能由 TUI/Web 执行。

若实现无法在不污染 process temp 的前提下满足上述契约，先调整 local prerequisite 的 scoped root 注入和
测试隔离，再继续；不要把 filesystem I/O 下沉到 `workorchruntimecontract`。

#### A-4-4 resource-loss 完成证据

“已提交资源后来失效”已按以下合同完成；目录删除不会被归类为普通 candidate failure：

1. 在 `workorchruntimecontract` 仅补 owner-neutral local-resource DTO/domain 常量（如确有缺口）；不得
   放 probe、CAS 或状态迁移。
2. `AcquireValidationScratchPrerequisite` 产生 `RuntimePrerequisiteObservation`：kind 为 scratch、owner 为
   Orchestrator local mapping、subject 为稳定 scoped identity、revision 为已验证的 scratch revision。
3. candidate 前的 local probe 只检查该 scoped path 的存在、目录属性和可写性；发现删除、不可写或 revision
   不匹配时构造 typed `RuntimePrerequisiteStale`。
4. 只有无 active candidate/dispatch 的 read-only safe point 才调用
   `InvalidatePrerequisitesForReadOnlyStale`。它只退休 scratch 与精确相交 observation、只消耗
   `PrerequisiteRefreshes`，再回到 acquire；不得产生 provider call、candidate attempt、dispatch、ticket 或
   effect。
5. `TestRuntimeV2FileBackedValidationScratchStaleInvalidationCrashRestart` 证明目录删除后的 before/after
   invalidation-CAS：before 保持旧 aggregate 并在重启时只退休后重获；after 仅获取新 incarnation scratch；
   全程零 provider/session/attempt/dispatch/ticket/effect。
6. `TestRuntimeV2StateRepositoryValidationScratchStaleInvalidationIsAtomic` 证明 duplicate stale 无
   CAS/ledger 增量、无关 stale 不影响 scratch；probe 只在无 active dispatch 的 candidate 前 safe point 运行，
   不会由正常 Shell cleanup 触发执行期循环。

### 6.8 A-4 总体验收与提交边界

A-4 关闭前必须同时满足：

1. 6.2 中每个可达 kind 都有 before/after-CAS fresh-process test，或有可执行的不可达证明；
2. after-CAS 无重复 owner query，before-CAS 不泄漏 aggregate resource/observation；
3. 所有 acquisition 期间 candidate/session/attempt/provider completion/dispatch/ticket/operation/effect 均为零；
4. 外部 receipt 与 local mapping 都使用同一 SM-2 commit/invalidation 事实模型，但不伪装 owner；
5. 新增或修改的 `pkg` 只有 DTO、合同和纯 helper，不含状态迁移、EventHub、CAS、filesystem 或 retry。

**A-4 completed evidence (2026-07-25):** A-4-0 resolver inventory、A-4-1 ProviderCeiling、A-4-3
FailureTarget 与 A-4-4 ValidationScratch 的指定 fresh-process matrices 均已完成。A-4-2 以
`TestRuntimeV2FileBackedFileCapabilityCommitCrashRestart`、
`TestRuntimeV2FileBackedGitCapabilityCommitCrashRestart`、
`TestRuntimeV2FileBackedShellCapabilityCommitCrashRestart` 覆盖真实 receiver 的 CAS 两侧；
`TestRuntimeCapabilityOwnerInventoriesProduceCanonicalReceipts` 和
`TestRuntimeCapabilityReceiverModulesPublishDeclaredInventory` 覆盖全部 shared receiver/result/observation
branch；typed unavailable 的 adapter/classifier 回归保证零 candidate/governance/effect 路径。定向
Orchestrator、RuntimeV2、contract 测试及 `make verify-boundary-contract` 已通过。A-5 是唯一解锁的后续卡。

一个提交只覆盖一个 receipt kind 或一个不可拆分的同 owner receipt 实现。提交说明必须写明：影响的 SM、
producer/consumer、CAS 断点、query/ledger 期望变化、零副作用断言、执行命令和仍被阻塞的下一张卡。

## 7. 后续工作卡的具体执行方案

### 7.1 Q2 / A-5：mixed-owner refresh budget and exhaustion

**影响：SM-2 freshness/reducer；SM-4 restart；Run 仅 CAS。**

在 A-4 关闭后，用已证明的真实 owner 组成 matrix：先后提交外部 receipt 与 local prerequisite，分别制造
duplicate、无交集 stale、read-only stale 和 accepted receipt resource change。每种输入只允许改变其对应
ledger：

- read-only stale 只增加 `PrerequisiteRefreshes`；
- accepted receipt 的真实资源变更只增加 `PrerequisiteResourceChangeRefreshes`；
- duplicate/无交集 stale 无 CAS、无 query、无 ledger 变化；
- 只耗尽的一条账本生成确定的 `prerequisite_*_refresh_exhausted` terminal，另一账本和所有
  candidate/provider/dispatch/ticket/effect 不变；terminal 后没有 query loop。

测试必须在每次 ledger 临界值的 CAS 前/后中断并新建进程。禁止调高预算、把两账本合并、以重复 query
掩盖 stale，或让 capability owner 读取 Runtime aggregate。

#### A-5 完成证据

`TestMixedOwnerPrerequisiteRefreshLedgersRemainIndependent` 使用同一 SM-2 aggregate 组合 File 的
`workspace_tree` owner receipt 与 Orchestrator-local `ValidationScratch` observation，证明前者只增加
`PrerequisiteResourceChangeRefreshes`，后者只增加 `PrerequisiteRefreshes`；duplicate 与无交集变更不产生
新的 invalidation、ledger 消耗或 candidate/dispatch/governance/effect。

`TestMixedOwnerPrerequisiteRefreshExhaustionKeepsOtherLedgerStable` 则在两项 prerequisite 重获后分别耗尽
receipt 和 read-only ledger。两种 terminal 的 code 分别固定为
`prerequisite_receipt_refresh_exhausted` 与 `prerequisite_refresh_exhausted`，另一 ledger 保持原值，且重复
输入不增加 revision。这只是 reducer/controller 组件证据；A-5 仍需补齐这两个交错临界点的 file-backed
CAS 前、CAS 后 response-loss 与完整 fresh-process actor 重建矩阵，完成前不得进入 P4-B。

`TestRuntimeV2FileBackedMixedOwnerReceiptThenLocalScratchStaleCrashRestart` 已补齐混合的第一个
file-backed 轴：Git owner 的已接受 `repo-init` receipt 先以 receipt ledger 提交 resource invalidation；销毁
全部 Hub、Run owner、Git owner 与 workspace responder 后，删除 local scratch，再分别在 local stale CAS 前和
CAS 后中断。最终 fresh process 只重获 prerequisite，保留原 Git receipt/effect 恰一次，并得到
`PrerequisiteResourceChangeRefreshes=1`、`PrerequisiteRefreshes=1`。为支持该路径，read-only stale 入口只
拒绝**当前 authority** 的 live session/attempt；前一已 invalidated dispatch 的 candidate/session 是不可变审计
谱系，不能阻断新的 SM-2 freshness fact。

`TestRuntimeV2FileBackedGitReceiptResourceChangeRefreshExhaustionIsTerminal` 以真实 Git receipt 将 receipt
ledger 上限设为 1；fresh-process recovery 中第二个 resource change 直接提交
`prerequisite_receipt_refresh_exhausted`，不借用 read-only ledger。该测试分别在**该 terminal 写入** CAS 前和
CAS 后中断，再销毁 Hub、Run owner、Git owner、workspace responder 与 LLM observer 并启动全新进程：CAS 前
只允许从同一已持久化 owner receipt 收敛一次，CAS 后只允许 terminal delivery；两者均断言零新 candidate
request、dispatch、ticket 或 effect。

`TestRuntimeV2FileBackedValidationScratchStaleRefreshExhaustionIsTerminal` 则以真实本地目录 incarnation 将
read-only ledger 上限设为 1；第二次删除 scratch 后得到 `prerequisite_refresh_exhausted`，receipt ledger 保持
0，并在 terminal CAS 前/后 response-loss 后由 fresh-process reload 完成同样的无 query-loop 断言。至此两条
独立 ledger 的普通、duplicate、无交集、临界耗尽、CAS 两侧与完整 actor 重建矩阵均已覆盖；A-5 关闭，唯一
可修改的后续卡为 P4-B。

### 7.2 Q3 / P4-B：target selection、snapshot、failure target（`done`）

**影响：SM-2；File owner typed receipt；SM-4 restart。**

P4-B 只收口 resource/focus 事实的原子性：

1. 空 workspace 的 first target 必须由已持久化 Goal/DoD 所选择的 profile、workspace deliverable directory 的
   File manifest 和 owner-declared selector 合同共同约束；profile 可以封存无具体路径的 creation scope，但不得要求
   用户再次声明，也不得从自然语言或目录猜具体文件。LLM 仍须在该 scope 内提出具体 workspace-relative File target，
   并由 File owner 的 admissibility/feasibility gate 逐项确认。
2. selection、prospective new target、durable create receipt、post-create snapshot 与 failure-target 必须按
   定义的先后关系持久化；new target snapshot 只在 durable create receipt 已提交后取得。
3. 每个 selection/snapshot/failure target 都有 CAS 前/后、duplicate、stale、fresh-process matrix；旧
   selector/target/snapshot 不能在 restart 后复活。
4. ambiguity 或 owner 无法选择时，必须是 typed unresolved/infeasible route；只有真实外部 governance
   dependency 才能创建绑定 ticket。

完成证据：`TestRuntimeV2FileBackedEmptyWorkspaceCreationSelectionContinuesThroughCandidate` 与
`TestRuntimeV2FileBackedCreationSelectionCommitCrashRestart` 证明 empty workspace 从已持久化
Goal/DoD、deliverable directory manifest 和 File selector 得到 prospective target，并在 selection CAS
两侧由 fresh process 精确重查或复用；`TestRuntimeV2FileBackedEmptyWorkspaceCreationReceiptCrashRestartDoesNotReplayCreate`
证明 durable create 先于 post-create snapshot，且 create receipt 不重放。`TestRuntimeV2FileBackedTargetSnapshotCommitCrashRestart`
覆盖 snapshot 的 CAS 两侧、duplicate、read-only stale 与第三进程重获；
`TestRuntimeV2FileBackedFailureTargetCommitCrashRestart`、
`TestRuntimeV2FileBackedMCPOwnerFailureTargetReceiptRestartMatrix` 及 Shell owner stale 回归覆盖 local/
owner-observed failure target 的 CAS、duplicate、owner-catalog stale 与 reload。焦点切换后的旧 selection/
snapshot 退休由 `TestAdvanceSatisfiedCoverageFocusDropsPriorFocusTargetFacts` 和
`TestRuntimeV2FileBackedFocusScopedSelectionsStayRetiredAcrossDurableWrites` 固化，ambiguous/absent selector
则由 File-backed zero-provider 用例走 typed unresolved route；没有 generic decision ticket。

### 7.3 Q4 / P4-C：receipt、evidence、coverage、focus（`done`）

**影响：SM-2；SM-3 仅消费 sealed dispatch。**

accepted owner receipt → evidence → coverage → focus advance。验证 evidence 只满足本 run、本 focus、被 sealed
operation 绑定的 coverage；duplicate receipt 不重复 evidence/coverage；CAS 前后重启不跳过或重复 focus
advance。要求 normal、duplicate、stale、CAS 前/后和 reload；禁止用历史 evidence、summary、memory、projection
或 UI 状态关闭当前 coverage。

**完成证据：** `TestRuntimeV2FileBackedEvidenceCoverageFocusAdvanceCrashRestart` 使用两个 state-change
focus，覆盖已接受 owner receipt 的 evidence CAS 前中断、CAS 后响应丢失、销毁并重建 Run repository、Run/File
owner、EventHub 与 Orchestrator 后的恢复。两侧均断言 focus-1 的 evidence 仅关闭 `cwi-1`、在同一次 SM-2 CAS
推进到 focus-2、关闭 active dispatch，且恢复不得重新执行 owner operation 或创建 candidate/ticket。测试还从
持久化 evidence 读取真实幂等 event ID，验证重复提交不增加 state revision。既有
`TestRuntimeV2FileBackedClaimEvidenceAndDispatchCloseCrashMatrix`、
`TestRuntimeV2FileBackedFocusScopedSelectionsStayRetiredAcrossDurableWrites` 与
`TestDriveRuntimeV2FileBackedTwoFocusStateValidationAssessmentAndRestart` 分别守护 receipt/evidence close、
focus 资源退休和多 focus 正向收敛；`runtimev2/dispatch_commit_test.go` 守护纯 reducer 的 duplicate/stale
语义。

### 7.4 Q5 / P4-D：assessment、failure、repair（`done`）

验证 assessment-only focus 不运行 executable operation；failure signature、no-progress、validation-only 与总
repair budget 的变化严格按 reducer；有效新 evidence 不会清零同 focus 总预算，只有 focus advance 创建新的
ledger。同样要求 normal、duplicate、stale、CAS 前/后和 reload。

**完成证据：** `TestRuntimeV2StateRepositoryAssessmentCommitCrashRestartIsAtomic`、
`TestRuntimeV2StateRepositorySameGapAssessmentCrashRestartRetainsRepairWindow` 与
`TestRuntimeV2StateRepositoryPartialGapAssessmentCrashRestartPersistsSingleDelta` 分别覆盖 satisfied、相同 gap
和严格收窄的 partial gap；每一用例都在 CAS 前失败和 CAS 后响应丢失后销毁并重建 EventHub、Run owner、
repository adapter 与 controller，断言 assessment dispatch、coverage、active gap、no-progress shape、repair/
validation-only ledger 和 focus/terminal authority 只产生一次 delta。新增的
`TestRuntimeV2StateRepositoryFailureFactCrashRestartIsAtomic` 同样证明 SM-2 failure fact 与 terminal authority
不可分割，且 terminal duplicate 不改变 revision。`assessment_evidence_gate_test.go`、
`effective_progress_test.go`、`repair_directive_test.go`、`candidate_admission_test.go`、
`positive_convergence_fixture_test.go` 和 contract 的 `runtime_repair_ledger_test.go` 继续守护 assessment-only、
current-focus evidence、stale assessment retirement、same-gap shape fence、validation-only/no-progress 上限、
partial progress 与 focus advance 的纯语义。

### 7.5 Q6 / P5：sealed side effect and terminal delivery（`done`）

**影响：SM-3 dispatch、SM-2 evidence、SM-1 projection、SM-4 recovery。**

按独立子卡验证：candidate seal 前后、execution claim/fence、owner receipt、receipt→evidence、
operation-scoped governance grant、terminal Result/Run/Workspace delivery。每张卡在外部 side effect 前后及
Run CAS 前后分别中断，并要求：

- 已知 provider call 和 durable operation 均至多一次；
- receipt/evidence/coverage 只提交一次；
- governance continue 保留 execution key 并将 fence `N` 推进为 `N+1`；不得用普通 receipt 绕过 grant；
- reducer terminal 一旦 sealed，后续只允许 delivery retry；projection、TUI/Web、Resume 和 startup recovery
  均不得重算 authority 或重放 dispatch。

#### P5-A sealed candidate transaction（`done`）

SM-3 proposal 与 sealed dispatch 是不同的 Run-owner CAS 边界。`TestRuntimeV2StateRepositoryDispatchSealFreshProcessCrashRestart`
在 seal CAS 前失败和 CAS 后响应丢失两侧销毁并重建 EventHub、Run owner、repository adapter 与 controller：前者只保留
pending proposal，后者只保留原 dispatch ID；重复 seal 不增加 revision、completed dispatch 或任何 operation。既有
`TestRuntimeV2StateRepositoryCandidateProposalAndDispatchSealCrashRestartAreAtomic` 继续覆盖 proposal/seal 的细粒度
CAS 和 idempotency；本卡不执行 operation、创建 governance 或投影 terminal。

#### P5-B claim、owner receipt 与 receipt→evidence（`done`）

只验证 execution key/fence claim、receipt query/commit、receipt→evidence/dispatch close 的 exactly-once。需要按
safe-to-replay、at-most-once/replay-proof、owner-committed receipt 三类分别覆盖外部 side effect 前后、receipt CAS 前后，
并在每一恢复点重建全部 actor；已提交 receipt 必须被消费而非再次执行。不得在本卡修改 governance binding 或 terminal
delivery。

**完成证据：** `TestRuntimeV2FileBackedClaimEvidenceAndDispatchCloseCrashMatrix` 现在在 claim CAS 前、claim
CAS 后执行前、evidence/dispatch-close CAS 前和 CAS 后四个点均销毁并重建 EventHub、Run owner、File owner、
workspace/receipt responder 与 controller；safe-to-replay read 仅在无 receipt 时按 recovery ledger 重试，durable
write 的 receipt/evidence/coverage 只提交一次。`TestDriveRuntimeV2FileBackedRestartRecoversCrashAfterOwnerReceipt`
证明 owner-committed durable receipt 的 fresh-process drive 只消费 receipt，不重新调用 provider 或改写文件。
`TestRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure` 证明 at-most-once patch 在 receipt CAS
失败后只能以 sealed execution key/fence/replay proof 恢复，重建后的 File owner 对 delayed duplicate 只返回同一
receipt；后续 assessment/terminal 也不得生成第二个 state change。`runtimev2/executor_test.go`、
`operation_receipt_test.go` 和 `dispatch_commit_test.go` 保留 claim/fence、unknown outcome、safe replay、receipt
identity 与纯 aggregate duplicate 语义。

#### P5-C operation-scoped governance grant（`done`）

验证 governance-required receipt、ticket binding、continue grant 与 resume fence `N→N+1` 的 CAS/restart；旧 fence 的
普通 receipt 不能消费 grant，resolution duplicate/stale 不得创建第二次 effect。

**完成证据：** `TestRuntimeV2FileBackedOperationGovernanceGrantFreshProcessCrashRestart` 对 grant CAS 前中断和
CAS 后响应丢失分别销毁并重建 EventHub、Run owner、Decision owner、File owner、workspace/receipt responder 和
Orchestrator controller。两条路径都由 Run-owner aggregate 恢复同一 ticket、operation `ExecutionKey` 与唯一
`N→N+1` fence：旧 fence 携带 grant 的 preflight 被拒绝，原 governance receipt 保持为 `governance_required`，只有
resume fence 的 File write 获得 accepted receipt 并产生一次 durable effect；stale 与 duplicate resolution 均不能产生
第二次 owner invocation。`runtimev2/governance_test.go` 继续守护绑定、grant identity、dependency drift 和纯 aggregate
duplicate/stale 语义。

#### P5-D terminal delivery（`done`）

验证 terminal 已封印后 Result/Run/Workspace projection 的 delivery retry。任何 Resume、startup recovery、TUI/Web/SSE
仅可重投递，不能重算 authority、重新请求 candidate、重新执行 dispatch 或重开 governance。

**完成证据：** `TestDeliverRuntimeV2TerminalRestartsAfterOwnerProjectionAcknowledgementLoss` 在 Workspace release
和 Run terminal 各自已经落盘但确认丢失时，销毁并重建 EventHub、Result/Workspace/Run owner 及 Orchestrator，验证
Result 仅一份、Workspace 最终释放、Run 最终终态且 `RuntimeState` 的 terminal revision/authority 不变；延迟 stale
candidate 结果被拒绝，不能重新打开 authority。`TestDeliverRuntimeV2TerminalRestartsAfterResultPersistenceBeforeOwnerProjection`
与 `TestDeliverRuntimeV2PrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult` 覆盖 Result 已落盘和
Result 尚未落盘两侧的 fresh-process recovery；`TestRuntimeV2TerminalDeliveryBudgetExhaustionRetainsStartupRecoveryInventory`
与 `TestRuntimeV2StartupRecoveryDeliversPendingTerminalThroughOwnerChain` 证明 delivery budget 耗尽只保留 Run-owner
terminal-delivery inventory，启动恢复只重投递已封印 terminal。`TestDeliverRuntimeV2TerminalUsesTypedOwnerEventContracts`
守护 Result、Workspace、Run 的 concrete typed EventHub delivery 与 duplicate 幂等性。

### 7.6 Q7 / P6：隔离 scripted/live 验证

**影响：SM-1/SM-4 observation only；禁止借 live 修改运行时语义。**

P6 只在 P0–P5 全部关闭后执行。每轮使用新的临时 config、data root、workspace root、workspace/run ID 和
本次构建 binary；不得污染 `config/`、`.workorch-data`、`.workorch-workspaces`、default chat 或
workspace `DefaultLLMTarget`。

| 场景 | 必须观察 | 失败回流 |
| --- | --- | --- |
| scripted positive | empty workspace discovery → first durable change → validation/repair → assessment → terminal；每轮 revision/progress signature 有真实 delta | 首个不符的 P1–P5 卡 |
| adversarial corpus | wire mutation、semantic exhaustion、transport/configuration、stale、真实 governance、P4/P5 restart；每类只改变所属 ledger | P0 trace 与第一个 owner |
| deployed recovery | `active_no_dispatch`、`active_sealed_dispatch`、`terminal_delivery_pending` 重启后单飞收敛；projection watermark 一致 | P4/P5 或 projection 观察卡 |
| real provider reachability | 只验证已通过 canonical scripted contract 的 target、request identity、manifest/stable prefix 可达 | P0 增加 deterministic fault/trace，不直接改模型/预算 |

TUI/Web/SSE 只验证 committed revision、high-watermark、ticket、terminal 和 request audit identity；`/run resume`、
网页 activation 和 watcher 不得作为恢复补偿。

#### P6 当前证据（2026-07-25）

2026-07-25 的 real-provider isolated artifact
`/tmp/workorch-e2e/runtime-v2-real-provider-20260725T145126Z` 发现：通用 `software-dev/source_change`
profile 在空 deliverable 下只声明 `file/workspace_deliverable`，未声明 selector owner/subject。File owner 因而正确
返回 `absent`，SM-2 在任何 provider request、candidate attempt、dispatch、ticket 或 effect 之前以
`target_grounding_unresolved` 终态化。这不是让 kernel 从 Goal/DoD 猜文件名的理由。当前修复将
`software-dev + workspace_deliverable_create_scope` 作为 activation/profile 的显式、持久化 selector binding；
File 仅在 empty manifest 且 binding 完整时返回无路径 `scope`。SM-3 仍要求一个 durable File operation 的具体
workspace-relative target，并保持 owner feasibility/admissibility 与路径边界。缺少 binding 的空 workspace 继续返回
`absent`，作为零 provider-call 的负向回归。

对照 artifact `/tmp/workorch-e2e/runtime-v2-real-provider-bootstrap-20260725T145346Z` 已越过上述 selection，
首个 focus 得到 `create go.mod` 的 File selection；随后 ai-proxy 上游在 candidate transport 阶段连续返回
`502 upstream_unavailable ... EOF`。Runtime 以独立的 `providerTransportAttempts=3` 收敛为
`provider_transport_exhausted`，没有完整 candidate、attempt、dispatch、operation 或 effect。这是外部 provider
可达性阻断，不是 candidate/evidence/assessment 合同缺陷，也不能靠 Resume 或人工 ticket 修复。P6 verifier 必须将
它标记为 `external_provider_unavailable` 失败，并仅报告该首失真边界及 terminal/activeRun 基本一致性；不得把预期
缺失的 candidate/evidence/assessment 叠加成派生噪声。真实 provider 恢复后必须使用新的 fresh activation/Run
重新取证，旧 terminal Run 不能 Resume。

`WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR=/tmp/workorch-managed-p6-retry-1784981173`
配合 `WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1`、临时 config/data/workspace root 和随机端口执行
`WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file ./scripts/verify-workorch-managed-full.sh`。该轮在
MCP stdio workspace 路径完成后返回 `runState=completed`、`completedDispatches=4`、
`evidenceSequence=4`，并同时通过 deployed HTTP、workspace/template/soul 与 mock LLM workspace
execution 验证。此前 `validation_scratch:global` 被 acquisition/inspection 强制按当前 focus 解释，
造成“has no local resource dependency”；现改为由 persisted prerequisite 的 `Scope/FocusID` 共同派生
scratch directory、observation dependency 与 stale identity。当前 production bootstrap 与 declared-command
discovery 都复用 run-global scratch；它们不再把同一资源误解释为 focus-scoped prerequisite。

该证据只证明 scripted/mock provider 的当前 P6 样本，P6 仍保持 `in_progress`：每次继续必须使用新的
隔离根目录；全仓 `go test ./...`、boundary gate 和未配置真实 provider 的可达性仍须分别记录，不能将
本轮 mock 成功扩展为真实 provider 成功。

此后 scripted/mock 的唯一入口为 `make verify-runtime-v2-p6-scripted`。它固定使用 managed harness 的临时
binary、config、data root、workspace root、随机端口和 `verify-ci-mvp-file` mock LLM/MCP target，并总是保留
artifact；可用 `WORKORCH_P6_SCRIPTED_ARTIFACT_DIR=/tmp/workorch-e2e/runtime-v2-p6-scripted-...` 指定本轮根目录。
成功 artifact 的 `p6-scripted-summary.json` 是本轮 scripted gate 的判据；该入口仍不能替代 real-provider 或
deployed-recovery evidence。该摘要必须由 `verify_runtime_v2_p6_scripted_contract.py` 从 artifact 的实际
`managed-runtime-state.json` 和 step logs 编译：它要求临时 config/data/server log 均位于 artifact 根目录，
managed server 已停止，并包含 fmt、console build、vet、全仓 Go test、binary build 与 `verify-ci-mvp-file`
的唯一成功日志；后者还必须保留 HTTP、template/soul、mock LLM、MCP stdio 以及
`runState=completed`/dispatch/evidence marker。禁止使用静态 `passed` 摘要替代这些事实。

2026-07-25 再次通过 `make fmt-check`、`./scripts/verify-runtime-v2-positive-convergence-live.sh --self-test`、
`make verify-runtime-v2-positive-convergence-file`、`make verify-boundary-contract` 与 `git diff --check`。
同日以新的隔离根目录 `/tmp/workorch-p6-scripted-20260725T133447Z` 执行
`make verify-runtime-v2-p6-scripted`；其 `p6-scripted-summary.json` 为 `status=passed`，并依次通过
`make fmt-check`、console build、`make vet`、`go test -count=1 ./...` 与 `verify-ci-mvp-file`。后者的
HTTP、template/soul、mock LLM workspace 和 MCP stdio workspace 样本全部成功；MCP 样本记录
`runState=completed`、`completedDispatches=4`、`evidenceSequence=4`。该 artifact 只追加 scripted/mock
证据，不改变 P6 的完成判据。
在 P6 artifact 合同校验器接入后，再以全新隔离根目录
`/tmp/workorch-p6-scripted-contract-20260725T134228Z` 完整执行同一 gate；`artifactContractVersion=1` 的
摘要确认 config/data/server log 均为该 artifact 内的临时路径、managed server 已停止，并确认全部六项
scripted gate 为 true。该次结果验证 verifier 自身没有将旧 artifact 或仓库运行目录误作为正向证据。
最新部署的首个真实 provider 样本返回 `400 This response_format type is unavailable now`；其 catalog 仅声明
`chat_completions` 和容量，未声明 `response_format`。因此将该问题归类为 F0 provider capability/request
失真：adapter 不得由 OpenAI-compatible surface 或 catalog operation 虚构 JSON Schema/JSON object 支持，
未显式声明 JSON Schema 时必须省略 `response_format`，仍由 sealed manifest、prompt 和 strict decoder
形成候选边界。该修正后必须使用 fresh Run 重新执行 P6，旧的 configuration-ticket Run 不构成通过证据。
当前部署已可达且 catalog ready；在包含上述 F0 修正的 binary 重新部署前，现有 `provider_configuration`
ticket 和旧 Run 不得作为 real-provider 或 deployed-recovery 证据。待最新 binary、两个 fresh workspace、
真实 provider target 与三类 restart fixture manifest 可用后，按本节表格执行并将 artifact path、run IDs、
revision/watermark 与 receipt 唯一性结论补入本节。

#### P6 real-provider canonical gate（待外部 fixture；不得以 preflight 结案）

真实 provider 的唯一 P6 入口是 `make verify-real-runtime-closure`。该入口只复用 Runtime V2
positive-convergence live verifier：先检查 deployed `/healthz`、`/readyz` 与 projection watermark，再为两条
**fresh Runtime V2** fixture 明确发出 activation，并验证本次 activation 新建的 Run、sealed provider
ceiling、request/audit identity、owner receipt/evidence、assessment、terminal projection 与 watermark。它不再
依赖历史 scheduler/watchdog 指标、loadtest、`replan_continuation` 或运行时观测脚本；这些工具若仍保留，只能
作为独立的性能/观测工具，不构成 P6 通过条件。

在已部署、已配置 ready real provider，且已创建两条新的 fixture workspace 后执行：

```bash
WORKORCH_REAL_RUNTIME_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_REAL_RUNTIME_WORKSPACE_ID_A=<fresh-workspace-a> \
WORKORCH_REAL_RUNTIME_WORKSPACE_ID_B=<fresh-workspace-b> \
WORKORCH_REAL_RUNTIME_PROVIDER_ID=<ready-provider-id> \
WORKORCH_REAL_RUNTIME_MODEL_ID=<ready-model-id> \
WORKORCH_REAL_RUNTIME_ARTIFACT_DIR=/tmp/workorch-e2e/runtime-v2-real-provider-$(date -u +%Y%m%dT%H%M%SZ) \
make verify-real-runtime-closure
```

`WORKORCH_REAL_RUNTIME_PREFLIGHT_ONLY=1` 只保存并检查部署 readiness/watermark，**不能**作为 real-provider
success 或 P6 完成证据。缺失 workspace/provider/model 输入必须失败；脚本不得创建隐式 default target、复用
历史 completed Run，或通过 TUI/Web Resume 补偿。完整 run 的 artifact 至少保留两个 activation request、两个
fresh Run ID、每个 Run 的 final snapshot/observability、provider-ceiling/audit、owner receipts、terminal
projection 及 `readyz-final` watermark。P6 仍须另行取得 `active_no_dispatch`、`active_sealed_dispatch` 与
`terminal_delivery_pending` 的 deployed SM-4 restart artifact；real-provider positive 成功不能替代该三态证明。
除明确 `PREFLIGHT_ONLY` 外，脚本必须在访问部署端点前验证四个 fixture 输入；缺输入时以 exit 2 和可操作
的变量清单失败，不能被服务不可达错误掩盖。`--self-test` 固化此输入优先级；preflight 仍只检查 endpoint，
不生成 P6 success artifact。

三态 fixture 由外部在**服务重启后**准备；只读 verifier 不得 activation 或 Resume：

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_E2E_RESTART_FIXTURE_MANIFEST=/absolute/path/runtime-v2-restart-fixtures.json \
WORKORCH_E2E_ARTIFACT_DIR=/tmp/workorch-e2e/runtime-v2-restart-$(date -u +%Y%m%dT%H%M%SZ) \
make verify-runtime-v2-restart-recovery-live
```

manifest 必须且只能含 `active_no_dispatch`、`active_sealed_dispatch`、`terminal_delivery_pending` 三项，每项提供
`workspaceId` 与重启前已记录的 `runId`；三项必须使用不同的 workspace，避免一个 snapshot 被伪装成多个
recovery class。`./scripts/verify-runtime-v2-restart-recovery-live.sh --self-test` 覆盖此 manifest 的正反例。
每项还必须提供 `preRestart`：公共字段为 `stateRevision`、`authoritySnapshotId`、`candidateAttemptCount` 与
`dispatchCount`；`active_no_dispatch` 的 `dispatchCount` 必须为 `0`，`active_sealed_dispatch` 另提供
`sealedDispatchId` 与非空且不重复的 `operationExecutionKeys`，`terminal_delivery_pending` 另提供
`terminalCode`。验证器只接受每项收敛到 terminal 或真实 governance wait（terminal delivery 只能 terminal），并同时要求
projection watermark 一致、terminal workspace 不再持有 `activeRunId`，以及 owner receipt 的
`executionKey/fence` 无重复。sealed-dispatch 的 `outcome_unknown` 是显式 idempotency 结果，不能被 verifier
误当作第二次 effect；但它仍必须保留预先登记的 `sealedDispatchId` 和每个 `operationExecutionKey`。terminal
delivery 还要求 authority snapshot/terminal code/candidate attempt 数/dispatch 数与重启前完全一致。缺任一 fixture
或部署地址不可达均保留 P6
`in_progress`，不是 Runtime 语义失败的替代结论。

## 8. LLM 不稳定输出下的合同协议

LLM 是不可靠的 protocol client，不是合同 owner。系统不承诺任意模型一次输出正确 JSON；系统必须保证
每类输出进入确定、有界且不污染其他状态机的路径。

| 输出或依赖状态 | 首个接收层 | 可持久化信息 | 唯一后续动作 | 禁止 |
| --- | --- | --- | --- | --- |
| fragment、EOF、429/5xx、连接超时 | request session / SM-4 | request identity、continuation cursor、transport class | continuation 或 transport recovery | candidate attempt、repair/ticket、dispatch |
| 401/403、catalog/model/configuration 不可用 | provider dependency | sealed provider target、typed code、configuration dependency | actionable configuration route 或真实 bound governance | 盲重试、attempt、generic decision |
| complete JSON 但 wire/manifest 不合法 | strict decoder / SM-3 | field path、expected/actual、manifest/shape hash | attempt-0 diagnostic；仅 objective 完整时一次 correction | 忽略字段、放宽 schema、side effect |
| typed candidate 但 admission/owner feasibility 不合法 | SM-3 | operation identity、admissibility、rejected shape、objective | 一次 manifest-bound correction，或 deterministic terminal | 从目录/错误信息猜 target、ticket、repair |
| correction 重放或再次拒绝 | SM-3 | 两个 persisted shape/diagnostic | atomic `candidate_retry_exhausted` | attempt-2、重编译 authority、自动 Resume |
| authority/prerequisite stale | SM-2 | stale dependency/revision | invalidate → acquire prerequisite → new authority | 复用旧 document、计入 candidate/transport |

同一 sealed transaction 的 C0–C3 必须稳定，correction 只改变 C4，C5 只保留有界 ref。`stablePrefixHash`
漂移首先是 renderer defect。P0–P4 必要上下文缺失时不得提交 provider；审计只保存 hash、枚举、长度和
脱敏 diagnostic，不保存 prompt、raw document、workspace content、credential 或 endpoint。

## 9. 验证门禁和文档纪律

每卡先跑受影响选择器，再跑相邻 owner 与阶段门禁：

```bash
# L1: pure contracts and manifest/wire properties
go test ./internal/pkg/workorchruntimecontract ./internal/pkg/llmaudit -count=1

# L2: owner contracts (根据本卡实际触及的 owner 收缩或扩展)
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/file/... ./internal/modules/blocks/git/... \\
  ./internal/modules/blocks/shell/... -count=1

# L3: SM-2/SM-3 and fresh-process prerequisite suites
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/biz \\
  -run 'TestRuntimeV2FileBacked.*(Manifest|Selection|Snapshot|Admissibility|Stale|Refresh|ProviderCeiling|Capability|FailureTarget|ValidationScratch)' -count=1
go test ./internal/modules/kernel/run/... -count=1

# L4/L5: stage exit and repository boundaries
make verify-runtime-v2-positive-convergence-file
make fmt-check
go vet ./...
go test ./... -count=1
make verify-boundary-contract
git diff --check

# L6: only after P0–P5 are closed
make verify-runtime-v2-positive-convergence-live

# documentation authority: active core must not regain retired V1 graph/continuation contracts
! rg -n 'executionGraph|ConvergenceContract|incrementalArtifact|artifactContinuationContext' \
  docs/00-core/runtime-stage-contract.md docs/70-roadmap/active \
  --glob '!runtime-v2-validation-progress-closure-plan.md'
```

`verify-boundary-contract` 在没有 `rg` 的最小环境必须使用等价 `grep -R` fallback；工具缺失不能让边界
门禁被跳过。对 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch` 等仅能存在于
retired-surface 表的名称，应逐项人工复核其上下文，不能因名称扫描误报而跳过。

本文件是 active 目录中唯一的 Runtime V2 执行队列。`overview`、`current-state`、未关矩阵和 todo 索引
只摘要本文件状态，不复制状态迁移或定义第二个“下一步”。archive 的旧计划只能帮助定位回归 fixture，
不得重新定义预算、fallback、Resume 或 live 结论。发现与本文件冲突的 active 旧计划时，迁移必要的审计
线索到 archive 后删除其 active 副本。

## 10. Definition of Done

仅当以下条件全部满足时，`RUNTIME-VPC` 才可关闭：

1. Q1–Q6 逐卡退出，既有 C4–C6/P0–P3/A-0–A-3M 回归持续通过；
2. 一个 sealed authority transaction 只有一个 manifest，renderer、decoder、admission、fixture、owner
   feasibility 对其给出一致结论；
3. authority 缺失、stale 或矛盾绝不调用 provider、创建 candidate attempt 或无信息 decision；
4. complete invalid candidate 至多 attempt 0/1；不完整 document、transport 和 stale 从不污染 attempt；
5. restart 不复活 retired resource、旧 target/session 或未提交 focus advance，不重放已知 provider/durable
   effect，不重复关闭 coverage；
6. terminal authority 后只允许 delivery retry，projection、TUI/Web、Resume 和 restart 均不能改写它；
7. L1–L6 通过，审计中没有 prompt、raw document、snapshot content、credential 或 endpoint；
8. 本文件、组件矩阵、`current-state`、未关矩阵、active overview、todo 与 archive index 同步。

一次偶然的 real-provider 成功、放宽合同、提高预算、只通过单组件测试或在 live 中手工 Resume，均不构成
完成证据。
