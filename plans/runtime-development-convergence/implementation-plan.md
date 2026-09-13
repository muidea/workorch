# 逐项代码收口任务

Status: in-progress — CL-00/CL-01 首个切片、CL-02 因果反馈及只读进度上下文、CL-08 semantic 预检/session 边界已接线；完整任务尚未关闭，基线见 [总览](README.md)

任务依赖是工程安排，不是生产 Runtime 的任务图。文中的新测试描述是待补场景，不冒充仓库已有测试。按用户后续要求，本阶段不考虑数据兼容性，不实现旧协议/归档/hash/renderer 回退；当前协议的同版本恢复和安全不变量继续强制验证。

## 执行规则

- 每个切片须一起包含 producer、接收 DTO、owner 私有事件、消费端、合同及适用测试；禁止先放开操作，再补 owner/freshness/预算守卫。
- 每项改动描述列明受影响 SM、主写字段、协同写集合、effect 前 seal、CAS cut、当前协议和数据使用边界。不得把这些统一推迟到 CL-08。
- 不要求每个组件都改代码。核查无偏差的组件记录“不改”和边界回归依据，避免为目录对称拆新运行单元。
- 本表的完成只能由代码与证据证明；不得以部分切片关闭完整任务，不提交运行数据。

## 任务索引

| 状态 | ID | 任务 | 前置依赖 |
| --- | --- | --- | --- |
| [ ] | [CL-00](#cl-00) | 基线与可复现反例 | 无 |
| [ ] | [CL-01](#cl-01) | 版本化合同与单 owner 写集合 | CL-00 |
| [ ] | [CL-02](#cl-02) | 事实上下文与完整执行反馈 | CL-01 |
| [ ] | [CL-03](#cl-03) | 同 CWI 的有界实现循环 | CL-01、CL-02 |
| [ ] | [CL-04](#cl-04) | 未发布验证草稿事务 | CL-03 |
| [ ] | [CL-05](#cl-05) | 已发布输入纯遗漏补充 | CL-04 |
| [ ] | [CL-06](#cl-06) | 无归因失败的只读诊断与实现修复 | CL-03、CL-02 |
| [ ] | [CL-07](#cl-07) | 错误验证实现的受治理版本更替 | 单独合同准入；CL-04～CL-06、相关 CL-08 |
| [ ] | [CL-08](#cl-08) | 跨路径预算、治理和恢复不变量 | 随 CL-01～CL-06 实施；路径集成后汇总 |
| [ ] | [CL-09](#cl-09) | Result/TUI/Web 的进度与交付反馈 | CL-02～CL-06、CL-08 |
| [ ] | [CL-10](#cl-10) | Profile/Soul 当前配置与同版本恢复 | CL-01、CL-03～CL-06、CL-08；CL-09 接线 |
| [ ] | [CL-11](#cl-11) | 工作能力 harness 与 DCE producers | 对应路径与 CL-08～CL-10；producer 可分场景完成 |
| [ ] | [CL-12](#cl-12) | 原 Workspace 受控部署验收 | CL-11、G0～G3；显式部署/激活授权 |

<a id="cl-00"></a>

## CL-00：固定基线和完整反例

Owner / SM：测试与证据工作，不新增 production 状态；覆盖 SM-1～SM-4 的观测。

代码/材料落点：[评审主线测试](../../internal/modules/kernel/orchestrator/biz/runtime_validation_input_review_mainline_test.go)、[补充主线测试](../../internal/modules/kernel/orchestrator/biz/runtime_validation_input_addition_mainline_test.go)、[Profile 身份反例](../../internal/pkg/workorchruntimecontract/runtime_profile_identity_test.go)、[DCE verifier 测试](../../scripts/test_verify_deployed_convergence_manifest.py)。

- [ ] 固定本轮 commit、服务版本、原 Goal/DoD 原文/definition hash、template/profile/soul/capability 身份，区分历史不同协议的失败原因。
- [ ] 核对目标可达性：原义务是否可判定、所需工具链/后端和证据能力是否存在、最短工作链能否容纳于输入/history/预算上限。分别记录定义歧义、能力未实现、依赖未安装和政策容量不足；不能通过删除义务让检查通过。
- [ ] 保存 `run-024` 纯遗漏与更早无归因失败的最小脱敏复现，不依赖开发机 `/tmp` 文件存在。真实日志、secret、原始 prompt 不入 Git；保留制品用受控路径及 digest 索引。
- [ ] 以 `TestDriveRuntimeInputReviewV2OmissionAndRestart` 的遗漏事实构造当前协议能力场景；新实现必须证明补充后真实验证，不能只删除失败/越权反例使测试变绿。
- [ ] 固定至少一个“目标合理、能力齐备且可完成”的小 Go 交付案例，以及一个非软件开发 template 案例，前者用于真实工作闭环，后者用于 core 中立性。

完成门槛：反例可独立运行并显示原失败阶段、owner 来源、未完成义务和实际副作用；区分“缺路径”“能力/环境不支持”“模型输出错误”“原准则确实不满足”。不把所有历史 failure code 归并成同一种修复权限。

<a id="cl-01"></a>

## CL-01：版本化合同与状态写集合

Owner / SM：Orchestrator SM-2/3/4；Run SM-1 opaque CAS，Workspace 拥有协议选择。主写归属先审定再落字段。

代码落点：[共享 Runtime 合同](../../internal/pkg/workorchruntimecontract)、[内部事件合同](../../internal/modules/kernel/orchestrator/biz/runtime/internal_event_contract.go)、[写集合](../../internal/modules/kernel/orchestrator/biz/runtime/internal_event_write_set.go)、[coverage lineage](../../internal/modules/kernel/orchestrator/biz/runtime/coverage_proof_lineage.go)、[状态 owner 清单](../../scripts/state-model-owners.manifest)、[转换 owner 清单](../../scripts/state-transition-owners.manifest)。

- [ ] 逐项比对已有 semantic ledger、CWI generation、input review/addition/repair、binding preimage，先复用再扩展；为必须新增的字段记录 source、主写 SM、clone/hash、持久化和读取方。
- [ ] 定义中间工作 purpose 与最终 proof 的区别，以及草稿/发布/补充的 exact identity；不能新增 authoritative backlog、第二个 workspace plan 或可执行 action graph。
- [ ] 定义接收 owner 的 typed command/result、跨 owner scope 校验及 private-event 映射；拒绝 map/JSON 中转 authority、可调用 sibling reducer、跨 owner repository。
- [ ] 为新能力指定显式当前协议、完整封存 hash 和未知版本拒绝，不做旧版回退。纯 DTO helper 不承担 owner 策略、I/O 或 CAS。
- [ ] 列出有限的总迭代、草稿 revision、诊断、补充、修复、信息补充、history/input 容量与 provider/operation 预算。每项有明确数值/来源、预留时点、耗尽结果；换文件/版本不得重新计数。未确定的额度使对应能力保持关闭。
- [ ] 定义一个 owner 事件的原子写集合；若 SM-2/3 协调，明确先后和同 CAS 反证，不各存一份“当前输入状态”。

测试：validate/clone/hash、缺少当前必需字段、未知版本、非法 transition、字段跨 SM 写入、scope mismatch、预算边界及同版本 fresh-load preimage 反证。

合同同步：Core 的 state-machines/candidate-and-dispatch/execution-budget、Orchestrator readme、受影响 owner `pkg/events` 与 readme。完成门槛是 G0 评审通过且当前协议能稳定封存/重载；此任务本身不算新闭环可用。

已实施子切片（工作区变更，未提交）：Profile 结构化声明无损归一化、同层重复不能被 overlay 吞掉、完整 proof 约束进入封存 hash、Application 形态检查、Workspace 私有 admission 拒绝和 final bootstrap 防御复核。没有新增 SM transition 或执行权限；新增 `runtime_profile_identity_test.go`、`definition_profile_identity_test.go`、`runtime_profile_input_test.go`。CL-01 的工作范围/草稿/共享预算新合同仍待实施。

一般实现工作的声明/可执行性分离子切片：新增 `RuntimeImplementationWorkPolicy`，经 activationports 的封闭无损
decoder 进入原 Profile candidate/hash/raw clone；Workspace 私有 admission 与 Runtime execution contract 分别拒绝
未实现的 `runtime-implementation-work/v1` 执行。声明约束单一 effect owner、带 target 合同的 operation/variant、
selector/candidate/mandatory constraints、1～8 轮及每轮 1～8 target，并强制 fresh observation、fresh validation 和
原输入保护。该 8 为声明容量/政策硬上界，不是已开通预算或既有物理限额的提升；没有默认启用或新计数器。
测试覆盖逐字段 hash/equality、raw clone/JSON 重载、重复/越界/未知字段、两侧 merge 前拒绝，以及 Workspace
candidate 持久化后新 owner 重载/duplicate 零 startup/activation 与零改写。
本子切片不完成 CL-01：尚缺 exact CWI 的工作意图、预期新增事实、owner observation preimage、工作 scope/binding、
轮次预留/消费和 SM-2/3 同 CAS 写集合。CL-03 的两轮实际实现→重观测→fresh validation→Result 仍未实现，
因此保留执行支持门禁；不得把此门禁或本组测试当作 VC-01 正常推进证据。

后续 exact CWI 投影子切片：Application 将匹配 scope/id 的工作 policy 深拷贝到 immutable Definition；
wildcard/exact 同时声明必须一致，合并后的原终结义务仍 validation-only。Runtime bootstrap、facts 与
semantic manifest 编译独立反证封存 Profile source，拒绝丢失、注入、跨 CWI 和逐字段漂移；
Definition equality 与 raw clone/同版本 JSON 重载覆盖新字段。没有给 mutable focus、最终 proof、
owner-fact acquisition 或 validation-failure repair 增加权限，也没有修改旧数据或默认模板。

写集合：本切片只扩充 activation 的不可变 Definition 声明，沿既有 SM-2 activation/facts 校验和 Run
opaque CAS 保存；SM-1/3/4 无新转换、计数器或 authority。新增 controller 重启反例证明单独注入有效
Definition policy 而无 Profile source 时，重复初始化零写入、不修复持久化输入。
后续能力投影子切片已将匹配 exact CWI policy 的 operation kinds 编入 Definition capability contract、
owner 全局 state-change capability 与 operation-admissibility prerequisites；共享纯编译器先核对封存
Profile source，按 owner/kind 确定性去重，不加载未匹配工作策略，不改最终 validation proof。
bootstrap 独立拒绝缺失及 owner/class 错配；共享测试覆盖注入、缺项、scope、JSON 重载，Application
测试覆盖准入投影和 terminal proof 隔离，真实 File owner 测试覆盖现有 typed EventHub 能力获取。
此子切片没有新增 SM-1/2/3/4 转换或 owner；仅补齐启动输入与既有能力获取的前置清单，执行门禁保持。
后续范围观察子切片：共享纯 compiler 绑定 exact CWI/Definition/Profile policy、请求 authority 和当前 owner manifest；
Orchestrator Biz 只读适配层复用 File typed target-selection，保留范围/revision/负事实，不写入 terminal-proof
`Resources.TargetSelection`。request hash 不以观察时间刷新；owner revision 变化返回可识别 stale，不做本地重试。
共享测试覆盖身份/source/前像、重 hash 篡改、raw clone 与同版本 JSON；真实 File 测试覆盖完整 inventory
不被单轮目标数裁剪、零写入、owner 重建、旧 revision 拒绝、新 revision 新 fence 及 unsupported 负事实。
这仍只是可独立验证的只读适配层：尚无 drive/private-event consumer 或工作 observation 的 aggregate 写集合；
不声称 owner 范围已经进入生产工作 binding，也不把 owner 重建测试当作 Runtime CAS/restart 矩阵。
来源校验补充：request/observation 必须从独立提供的封存 Definition/Profile、当前 CWI、authority 和 manifest
重新编译比对，不能以请求自身 hash 作为来源授权。只读适配器在 owner I/O 前执行该检查；测试覆盖协同
修改 policy/身份并重新 hash 的拒绝、缺失 Profile、旧树观察和同版本 JSON 重载。sources 不是新权威副本，
后续生产 consumer 必须从 owner 当前 aggregate 提供，不能从待验证请求重建；本切片未完成该生产接线。
下一必需切片仍是：将 owner scope/当前事实前像接入同一 aggregate 的 owner-private 消费，接入有身份的
工作意图和选择理由，再完成 SM-2 working binding / SM-3 dispatch 的原子轮次预留与回执后重观测。
在真实 owner 两轮工作→fresh validation→Result 正例及 CAS/restart 矩阵通过前，不打开执行门禁、
不勾选 CL-01/CL-03 或 VC-01 完成。

该 exact CWI 子切片已通过 shared contract、activationcore、activationports、Workspace 和 LLM 受影响全包测试；
Orchestrator `biz/runtime` 全包（109.375s）；shared/activationcore 新合同及 raw-clone 定向 race、Runtime
重启定向 race；`make fmt-check`、`make vet`、`go build ./...`、`make verify-boundary-contract`、
`git diff --check`。未运行整仓全量测试、真实模型/部署验收，也未提交代码。以上均不替代 VC-01 工作循环正例。

该声明子切片已通过 shared contract、activationports、Workspace 全部子包、activationcore 与 LLM 全部子包测试；
三个新增合同/准入测试包的定向 race；Runtime Profile/Definition/预算/结构定向回归；`make fmt-check`、
`make vet`、`go build ./...`、`make verify-boundary-contract` 和 `git diff --check`。
未执行整仓全量、真实 owner 的一般实现循环、真实模型或部署目标验收；没有部署、提交或修改旧运行数据。

该子切片的写集合与验证边界：

- Workspace 仍通过既有 admission private event 持久化 candidate 和拒绝结果；没有增加状态字段或开放内部 transition。
- SM-1 lifecycle/CAS、SM-2 reducer、SM-3 dispatch transaction 和 SM-4 drive 的转换均不改；仅其消费的 Profile 封存身份更严格，不能把旧缺字段 hash 当成当前身份。
- 反例先复现后修复：非法声明被 normalization 清除、Profile 比较折叠重复 owner-fact/variant、proof 约束变更不改变 hash；修复后原义务不变，非法输入保留并拒绝。
- 已通过 shared contract、Workspace biz、activationcore、activationports 的定向与 race 测试；Orchestrator `biz/runtime` 全包测试及 Profile/input-review 定向测试；`make fmt-check`、`make vet`、`make verify-boundary-contract`、`go build ./...` 和计划文件的 99 个本地路径引用检查。
- 首轮 `go test -timeout=15m -count=1 ./...` 全仓通过（Orchestrator biz 876.518s）。全量运行期间补充的 Profile 比较及 owner-fact proof identity 修正，已重跑上述受影响包/race、Runtime 全包、构建、vet 和边界检查；后续 CL-02 子切片的最终全仓回归已包含这些修正，结果见下文。
- 以上不证明真实模型完成开发、当前部署在推进或 CL-00～CL-12 已关闭；本切片未执行部署、激活、数据迁移或提交。

追加子切片（工作区变更，未提交）：Profile raw→typed 输入的封闭字段和值类型。

- 核对 CL-03 时确认其 CL-01 工作范围/共享预算前置尚未实现；不能把 owner-fact acquisition 或 validation-repair
  的专用权限泛化为一般实现。先修复当前输入可静默丢失未知合同、布尔义务、列表成员和预算小数的问题。
- `runtime_profile_input.go` 按实际对象层级校验全部 decoder 字段；Template/Workspace merge 前分别检查，final
  activation 独立检查。未知/错层字段、错误 primitive/null、混合/空白成员列表、非有限/非整数/越界 budget 拒绝，
  不创建假 typed default。repair 的原 exact decoder 与 Workspace semantic admission 仍是既有 owner 路径。
- 内置 `software-dev.validationEnvironmentPolicy` 只有配置与测试引用，没有生产 consumer，故不作为已支持 policy
  放行。删除当前默认资源的该副本，将隔离、受限前台、禁止 detached 与清理提示保留在实际进入 typed activation 的
  `validationPolicies.notes`；既有 scratch/bounded 声明、原 Goal/DoD、required CWI、proof 与依赖不变。
  更新 defaults 测试和静态边界检查，拒绝该未消费字段回流。没有编辑历史 fixture、归档、部署数据或增加迁移指纹。
- Application 只做无损协议形态检查，Workspace 仍拥有 admission；SM-1～SM-4 的写集合、事件、状态、预算与权限均不变。
  该切片不是 CL-03 工作循环，更不证明实际部署已推进。未知新合同须先实现完整 producer/consumer，不能靠加字段启用。
- 11 个独立反例先复现旧 guard 接受，再由新 guard 拒绝；另有所有字段类别的错误类型/空白列表/数值容量、两侧
  merge-before 与直接 activation 反例、六个默认模板正例、有效指导的 typed snapshot/reload 回归。
  `runtime_profile_decoder_contract_test.go` 从实际 decoder AST 核对 11 类对象的字段及转换类型，防止 allowlist 与
  decoder 单边修改；仅用于验证，不进入 production，更不是 authority registry。
- 本次通过：`go test ./internal/modules/application/workorchd/... ./internal/modules/blocks/workspace/... ./internal/pkg/workorchdefaults ./internal/pkg/workorchruntimecontract ./internal/modules/blocks/llm/... -count=1 -timeout=5m`；
  Runtime 内核全包 80.036s；真实 File/Shell historical-assessment uninterrupted 主线 13.397s；activationports/defaults
  全包 race；`make fmt-check`、`make vet`、`make verify-boundary-contract`、`go build ./...`、`git diff --check`。
  大范围回归后新增的 decoder AST 测试另由最终 activationports/defaults 全包 race 覆盖。本次未重跑全仓或完整
  Orchestrator Biz 重启矩阵；未执行真实模型、部署、Run 激活、归档更新或 Git 提交。


追加子切片（工作区变更，未提交）：显式 Run 级共享物理预算。

- 新增当前协议 `RuntimeExecutionLimits{ProtocolRef=runtime-execution-limits/v1, MaxProviderCalls,
  MaxCapabilityOperations}`，只允许 1～4096 调用、1～8192 操作，来源是当前默认 budget dimension 硬上限；
  两项必须同时给出。未声明采用当前默认，不改内置模板。它不是工作范围、逐 CWI 总迭代或草稿/诊断/补充额度，
  因而不关闭 CL-01/CL-08，也不开放 CL-03。
- Application 对 Template/Workspace raw 输入分别精确检查，再遵循既有 Template 优先规则。effective limits 的完整
  hash 进入 Workspace candidate policy binding；final bootstrap 反查该 binding。typed execution contract 的 equality、
  prefix identity、normal/raw clone 保留字段。无新 framework 单元、跨 owner command 或可变共享计数器。
- 唯一消费端是既有 `RuntimeExecutionBudgetPolicy`/allocation ledger。bootstrap/facts 校验对限额反证，重 hash
  不能扩大 budget。SM-3 原 provider submission/operation claim private event 仍在同 CAS 预留，SM-2 沿用 exhaustion
  terminal，SM-4 只重放 persisted ledger；SM-1 无新写集合。修改 request、用途、物理 ID 不刷新余额，unknown 仍占额。
- 新增 shared/application/Runtime tests：协议、缺项、类型、越界、两侧 premerge/final activation、admission 与 bootstrap
  身份、clone/reload、策略重 hash 漂移、provider/operation claim 的 CAS 前中断/写后响应丢失、duplicate/stale 与零新
  reservation、unknown 占额、operations/semantic/whole-document-recovery 共用物理限额。
- `TestDriveRuntimeExecutionLimitsPreserveRealOwnerCompletionAcrossRestart` 在真实 File/Shell、file-backed Run/Result
  的原 historical-assessment/input-addition 主线上显式选择 64 调用/32 操作，完成 uninterrupted 和 after_input_receipt
  两个场景，并核对实际 provider 调用/operation receipt 与原账本一致，重启不追加调用。provider 为 scripted，保留原
  Goal/DoD、旧输入不可变和全部原验收断言；不证明一般实现循环或真实模型可靠性。
- 已通过：shared、Application 全树、Workspace/Run/LLM 全树回归；Runtime 内核全包 97.039s；上述真实 owner 主线
  31.304s；ExecutionLimits/ProviderBudget/BudgetExhaustion 定向 race（Runtime 5.562s）；fmt、vet、build 与边界检查。
  最终补充的 final-bootstrap 反例另由 activationports/activationcore/shared 全包 race 覆盖且通过（1.984s/1.161s/3.701s），
  随后 fmt、boundary 和 `git diff --check` 再次通过。
  未重跑全仓 `./...` 或完整 Orchestrator Biz 矩阵，未执行真实模型/部署/激活/归档改写/Git 提交。

追加子切片（工作区变更，未提交）：逐原 CWI 的证明规划批次上限。

- `runtime-execution-limits/v1` 可选 `MaxProofBatchesPerCWI` / raw `maxProofBatchesPerCWI`，显式值只接受 1～17，
  上界来自既有 16 个 retired generation 加当前一代；省略不附加此逻辑上限。完整值进入 admission/bootstrap/hash，
  Template 优先不变，未知/零/非整数/超界拒绝。无兼容 decoder、旧归档回填或默认模板启用。
- 不复用 CWI reopen count：重开也可能来自事实失效、修复或信息补充，不等价于一般实现迭代。消费只从当前和
  retired semantic batch 的 selected CWI 汇总，pending 窗口不计；同批次纠正/recovery 不重复计，物理调用仍共用
  原预算。新路径、内容、catalog、manifest、request ID 和重启不刷新原 CWI 余额。
- SM-3 `semantic_plan_batch_compiled` 的唯一私有 writer fresh-load 后与 requesting batch 同 CAS 占额；修正了
  原 exact replay 分支被 active-document/next-sequence 前置 guard 遮挡的问题。CAS 前失败不泄漏、写后响应丢失精确
  重放零写；不足时零新 batch/session/provider。aggregate/opaque-state 均反查原 ledger，无第二份 counter owner。
  SM-4 原 failure ingress 提交私有事实、SM-2 原 budget terminal 裁决，仍通过 Result→Workspace→Run 交付；SM-1 不变。
- 回归覆盖 raw declaration/premerge/final bootstrap 身份、共享只读计量、current/history、pending 排除、clone/reload、
  批次 pre/post-CAS、duplicate/stale、catalog retirement→独立 CWI binding retirement→新请求零写耗尽。
- 真实 File/Shell + file-backed Run/Result + scripted provider：显式 64 calls/32 operations/4 proof batches 下完成
  uninterrupted 与 after_input_receipt 两场景（30.019s）；逐 CWI 上限 1 时沿生产错误边界有界失败并完整交付，原输入
  不变、零额外 session/allocation/provider/effect/ticket，终态重启零重复写（8.540s）。这些是事务证据，不是真实模型验收。
- 已通过 Runtime 全包（101.110s）、shared 全包、Application/Run 全树；proof/limits 定向 race（Runtime 5.097s、
  shared 1.055s、activationports 1.070s）；fmt、vet、boundary、`go build ./...`、`git diff --check`。
  此切片不关闭 CL-01/08，更不开放 CL-03：一般工作范围、总工作迭代、草稿/补充/诊断专用合同仍未完成；本轮未执行
  全仓测试、完整 Orchestrator Biz 矩阵、真实模型、部署、激活、历史归档更新或 Git 提交。

追加子切片（工作区变更，未提交）：逐原 CWI 持久化 dispatch 封存上限。

- 新增可选 `MaxDurableDispatchesPerCWI`，raw 精确整数 1～8192；复用完整 admission/bootstrap/hash，未修改默认模板。
- 不添加跨组件 counter；SM-3 final seal 从 accepted attempt 与原 dispatch 的 sealed binding CWI 计数，同一 CAS 占额。
  多文件/多操作只占一次；合法 governance resume 继承额度，失败、失效、换 generation 和重启不退额。
- aggregate 与 opaque-state fresh-load 同时验证 admission lineage 和上限；耗尽是独立 typed logical-budget fact，
  SM-4 原 failure ingress 交由 SM-2 terminal 处理，SM-1 不变。已有 provider 消费不回退，不宣称零请求。
- 本子切片不关闭 CL-01/03/08：工作范围、通用有界实现、草稿/诊断/纯遗漏补充仍待实施；不修改旧归档或部署配置。
- 真实 File/Shell 负向主线揭示并修复了共享终态 cleanup 缺口：pending attempt 已持有 feasibility 时，SM-2/SM-4
  failure 关闭它也必须在同 CAS 留存 exact owner catalog，不能只在普通候选拒绝路径留存。未放宽 fresh-load 校验。
- 验证通过：shared 全包、activationcore/activationports 全包、LLM/Run 全树、Runtime 全包（最终 111.915s）；
  durable/limits/catalog terminal/governance 定向 race（Runtime 16.746s），fmt/boundary/vet/build/diff-check。
  真实 File/Shell + file-backed Run/Result 的显式限额成功/耗尽/重启及 proof-budget 回归组通过（80.076s）；
  同 CWI 先新增输入再修复，durable cap 2 完成原验收（27.461s），cap 1 阻止修复 dispatch 并完整交付失败。
  CAS-before/response-loss/duplicate/stale、连续治理恢复及 terminal catalog 防篡改均通过；未执行全仓测试、
  完整 Biz 矩阵、真实 LLM、deployed DCE、部署/激活或 Git 提交。CL-03 等未完成项保持开放。


<a id="cl-02"></a>
## CL-02：事实上下文与完整执行反馈

Owner / SM：File/Shell 等 owner 生产事实；Orchestrator SM-2 归约、SM-3 封存 request/receipt；LLM 只渲染/调用；SM-4 恢复事实引用。

代码落点：[Shell validation](../../internal/modules/blocks/shell/biz/shell_runtime_validation.go)、[诊断](../../internal/modules/blocks/shell/biz/shell_diagnostic.go)、[LLM request context](../../internal/modules/blocks/llm/biz/runtime_candidate_context.go)、[semantic projection](../../internal/modules/blocks/llm/biz/runtime_semantic_projection.go)、[input review projection](../../internal/modules/blocks/llm/biz/runtime_input_review_projection.go)、[Runtime feedback](../../internal/modules/kernel/orchestrator/biz/runtime_result_feedback.go)。

- [ ] 把执行反馈从“digest/错误位置”补足为准则、实际对象、实际执行、expected/observed、失败类别、缺失/截断、owner/revision/dispatch refs；值为自报告时明确不是独立验证事实。
- [ ] 请求上下文能表达当前事实、已接受/失效证据、仍缺义务、可选的合法工作范围和预算，不给模型输出状态赋值入口。
- [ ] 完整材料使用受管 ref，超限明确缺什么及允许如何读取；不得默默截掉合取准则、把来源不明摘要当完整证据或暴露 secret。
- [ ] 长期 contract/schema 保持 C0，封存 Goal/DoD 保持 C1，声明能力保持 C2；当前 revision/失败/草稿/diagnostic 放 C3～C5 的相应层，不为本轮反馈改变 stable prefix。

测试：完整成功与失败反馈、超限/截断、空错误位置、缺失 owner ref、stale/mixed revision、同一启动输入跨轮 `stablePrefixHash` 稳定、无来源事实不能进入 authority、restart 后反馈可还原。

合同同步：[request/cache](../../docs/00-core/runtime-request-cache-note.md)、[context assembly](../../docs/00-core/runtime-request-context-assembly.md)、LLM/Shell readme。完成门槛：下一次请求能准确区分产品缺陷、验证输入缺陷和证据不可判定；没有因此扩大 effect 权限。

已接线子切片（工作区变更，未提交）：semantic repair proof-batch 按每个 CWI 的持久化 reopen trigger → rejected
evidence → retired binding/dispatch → committed execution/owner receipt 提取 typed 有界反馈；与 operations 共用安全
validation check projection，经既有 EventHub context 进入 LLM C4。接收端做 scope/batch/hash/结构校验，渲染后做
exact-value integrity，C0～C3 不改。原 Definition focus 和运行期 focus 是不同身份，不按名称连接。

- 不增加状态/主写字段、执行权限、预算账户或旧数据兼容。SM-2 只使用既有 semantic-contract failure 私有事件；
  SM-3 provider session 建立前检查新请求的证据来源，SM-4 只读驱动，SM-1 不变。
- 无来源 synthetic repair fixture 保留 scoped order/batch 与 stale replay 回归，但不能再伪造 provider 成功；缺来源
  必须在创建 request session/调用 provider 前失败。真实 File/Shell 主线负责正例与重启证明。
- 最终代码 `go test -timeout=20m -count=1 ./...` 全仓通过：Orchestrator biz 891.564s，Runtime 内核 99.726s。
  包含真实 File/Shell 的 historical-assessment、assertion-feedback、entrypoint 和输入维护重启场景，并通过 typed owner
  reload 比对请求中的 exact feedback。原生 check 通过但交付行为证据不足时保留真实观测，不伪造 failed check。
- shared/LLM 全包与 Runtime 全包、semantic/feedback 来源及 provider 错源/裁剪等定向 race 通过；
  `make fmt-check`、`make vet`、`make verify-boundary-contract`、`go build ./...`、`git diff --check` 通过，
  计划文件 95 个本地路径引用检查通过。初轮矩阵的错误断言和 5 分钟测试超时不计通过；修正后以最终全仓结果为准。
- CL-02 仍未整体关闭：一般实现轮次的完整成功/失败反馈、诊断闭环、Result/UI 完整呈现需随 CL-03～CL-09 继续；
  此切片不开放一般实现循环、草稿、纯遗漏补充或错误验证版本替换，不代表 G1/G3/G4 通过。

追加子切片（工作区变更，未提交）：semantic 请求的只读覆盖进度。

- shared `RuntimeSemanticCoverageProgress` 从完整 sealed CWI inventory 计算验收总量，仅对当前 manifest/batch 展开
  CWI `ObservedState`、当前证据引用数与重开数；绑定 workspace/run、Definition/startup、seed/manifest、batch 和来源
  revision/hash。记录数量不是累计操作数、fresh proof 或 credit，缺失/重复/错源/未知状态与超限拒绝。
- Orchestrator 在 preflight/session 之前投影；LLM 通过既有 typed context 接收，在 C4 完整渲染并逐值检查，
  不变更 C0～C3。SM-2 仍只有既有私有 failure 路径，SM-3 不增加 dispatch/修复权限，SM-4 只读重建，SM-1 不变。
  没有新持久化账本、状态类型、预算账户、迁移或旧请求 fallback；单 owner 边界检查没有新增豁免。
- shared/LLM 新增规范化、错源、漏项、重复、hash/容量、成员/计数一致性、clone/reload、渲染后裁剪/协调篡改和
  C0～C3 稳定性回归；既有真实 File/Shell 主线每次 semantic 请求通过 Run owner fresh-load 比对进度，preflight
  failure 的 CAS-before/response-loss 重启也比对当前快照。
- 本切片不关闭 CL-02：一般实现轮次的完整结果/预算上下文、CL-03 同 CWI 工作循环及后续草稿、补充、诊断、
  Result/UI 和真实部署验收仍待实现。证据数量字段不是完整执行结果反馈的替代。
- 本次最终字段命名下已通过：shared contract 与 `blocks/llm/...` 全包；新 progress 合同/渲染与 preflight failure
  CAS-restart 定向 race；Orchestrator semantic 全部定向回归及真实 File/Shell historical-assessment 的 uninterrupted
  主线（20.194s）。`make fmt-check`、`make vet`、`make verify-boundary-contract`、`go build ./...`、
  `git diff --check` 与 96 个计划本地路径引用检查通过。初轮旧 fixture 错配、跨 owner `State` 字段被拦截不计通过；
  已按当前合同修正为 exact snapshot fixture 与 `ObservedState`，没有放宽检查器。
- Runtime 内核全包在最终代码上复跑通过：`go test ./internal/modules/kernel/orchestrator/biz/runtime -count=1 -timeout=5m`
  （88.413s）。本次没有重跑 `go test ./...`，上一个子切片的全仓记录不能视为本次结果；真实模型/部署验证未执行。
- 扩大的 Orchestrator semantic + File/Shell 验证输入/行为修复/重启矩阵通过（532.511s），命令为
  `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeSemantic|TestSemantic|TestDriveRuntime(ValidationInput|InvalidSupplement|OrdinaryMutation|RepeatedInput|AssertionFeedback|EntrypointValidation)' -count=1 -timeout=10m`。
  该矩阵在最终 `State`→`ObservedState` DTO 命名调整和追加 preflight 来源断言之前启动；之后已用上述最终
  shared/LLM/Runtime 全包、semantic + uninterrupted 主线及定向 race 覆盖改动，没有把矩阵描述为最终全仓回归。

追加子切片（工作区变更，未提交）：semantic 请求的已提交操作历史。

- 此前普通 semantic 请求只有 CWI 验收计数，`ExecutionFeedback` 只覆盖 exact validation repair；已完成源代码
  工作的成功回执不能作为有界执行结果进入后续证明请求。本切片补充 `ExecutionHistory`，不泛化 repair feedback。
- Orchestrator 从当前 validated aggregate 的全部 receipt/dispatch/execution 反证来源，再按提交顺序取最近 16 条，
  显式给出 total/omitted/sequence；原 CWI 可已完成并退出当前 manifest。bound CWI 表示历史工作归属，accepted/changed
  只是 owner observation，不能伪造验收 credit。无回执的 claimed/unknown 不进入此计数，也不意味着未执行或可重试。
- typed request value 绑定 workspace/run、Definition/startup/seed/current manifest 与同一进度来源 revision；字段包含
  owner/kind、receipt ref/hash/fence、target/before/after、observed effect/state、changed 和安全 validation check facts。
  不传 raw summary/output/diagnostic/operation parameters，不按业务语言解释数据。64 KiB 或 provider ceiling 不足
  明确拒绝；截断旧记录必须显式计数，当前选中窗口不得静默裁剪，原 exact repair origin 仍独立且不可省略。
- 跨 owner 使用原 typed LLM command，接收 DTO/clone/validate 和 C4 renderer/integrity 同步接线。`ObservedState`
  表达 immutable owner outcome，不是目标状态；初轮 `ObservedOutcome` 被边界检查拦截，已改用现有观察合同，未
  放宽检查器。P3 逐值校验拒绝 hash 协调重算后的裁剪/注入/替换；C0～C3/stable prefix 不变，无新持久化字段或预算。
- SM-2 仍仅消费原 semantic failure 私有事件；SM-3 新 session/provider 前验证来源，SM-4 只读重投影，SM-1 不变。
  生产接线没有新 owner/状态机、action、working binding 或归档迁移。
- 新增 shared 容量/序号/CWI/hash/clone/reload/漏项反例、LLM 完整渲染/变更隔离/篡改拒绝、Runtime 真实私有
  receipt commit→零 credit 的成功工作投影、错源/duplicate/seed/manifest/tamper 拒绝；原 File/Shell 主线每个请求
  经 Run owner fresh-load 对照回执，并断言后续请求确实看到已完成 source work。已有 source/input effect 和终态
  重启断言保持不变；初轮 fixture 缺新字段及观察字段命名失败不计通过。
- 最终代码已通过 shared 与 LLM 全树；Orchestrator `TestRuntimeSemantic|TestSemantic|TestDriveRuntime(ExecutionLimits|ProofBudget)`
  主线（51.609s）；定向 race（shared 1.074s、LLM 1.289s、Runtime 1.907s）；activationcore/activationports/Run 回归，
  fmt/vet/build/boundary/diff-check；最终 Runtime 全包通过（110.856s）。较早包含 ValidationInputReview 的扩展矩阵通过
  （135.457s），但在 ObservedState 命名及最后 seed/manifest gate 前运行，不把它记作最终全矩阵验证。
- CL-02 仍未整体关闭：一般工作范围/每轮目标选择、非特定 repair 的完整失败归因与材料读取、草稿/诊断/输入补充、
  Result/UI 仍待对应合同落地。该切片不开放 CL-03，不证明真实模型或原 Workspace 已能完成 Goal；本轮未部署、
  激活 Run、改写历史归档或提交 Git。

### CL-02 追加：Run 物理预算与原 CWI 证明批次的只读反馈

当前补充（工作区变更，未提交）：持久化工作预算反馈。

- `RuntimeSemanticExecutionBudget` 增加 `durableDispatchLimit`、`visibleDurableDispatches`；从 validated aggregate 的
  原 accepted dispatch lineage 派生用量，不从成功回执数估算、不另建计数器。失败/失效不退款，治理续派不重复扣额。
- 原 CWI 全量来源先校验，再按 semantic 可见窗口展示；即使隐藏的 CWI 负值、外来身份或越额也拒绝。
  未声明附加 cap 时 limit/remaining 显式 null，used 不丢失，不能当成无限工作或实施授权。
- shared clone/hash/32 KiB 边界及 LLM C4 逐值完整性包含新增值，重 hash 不能掩盖渲染后退款/删项；C0～C3 不变。
  真实 File/Shell 主线每次请求与 Run fresh-load 的 durable seal 用量对账，覆盖附加输入、实现修复、限额终止及重启。
- SM-3 只装配观察，SM-2/SM-4 沿用既有失败/重投影合同，SM-1 不变；本补充不关闭 CL-02/03，也不开放一般工作权限。
- 本次 shared 全包与 Runtime 内核全包（141.894s）通过；真实 File/Shell 限额、durable cap 正/反例、input-review
  omission/restart 与 terminal feedback/delivery 定向组通过（148.629s）。定向 race：shared 1.343s、LLM 1.339s、
  Runtime 13.758s。fmt、vet、build、boundary 通过。扩展回归另发现文档引用已改名测试及 appservice 一次失败，
  文档引用已修正且 component inventory 复跑通过，appservice 单独全包复跑通过（43.796s），日志保留于
  `/tmp/workorch-feedback-appservice.AnR7tt`。一次扩展失败原因尚未定位，不宣称全仓一次通过、真实模型或部署验收完成。

工作区变更，未提交。新增 `RuntimeSemanticExecutionBudget`，补齐下一轮规划的预算事实；不开放一般实施工作。

- Orchestrator 验证完整 current aggregate，以同一 exact revision 构造 pre-submission 观察；复用原 budget view 的
  provider-call/capability-operation 两维，以及 current/history semantic batches 的逐原 CWI 用量，没有新账本。
- SM-3 新 session 前组装/校验；SM-2 仅沿原 semantic failure 处理来源失败；SM-4 只从 current aggregate 重投影，
  不从更新后的 revision 倒推旧余额。SM-1 与各资源 owner 不变，没有新增状态机、私有事件、预算或操作权限。
- shared clone/hash/容量/身份/余额等式与 LLM C4 renderer、`P3.semantic_execution_budget` 完整接线；typed 缺字段不
  fallback，post-render 协调重 hash 不能掩盖改值，C0～C3/stable prefix 不因余额变化而漂移。
- unknown/reserved 不退款，generation retirement/重启不刷新 proof 用量；null 附加 proof cap 不表示无限执行。
  未声明 `ExecutionLimits` 时沿用已有封存策略，不引入当前默认值作为额外上限；该观察也不是全部策略额度清单。
- shared 反例覆盖两维/可见 CWI 漏项、wrong scope/revision、越额/整数溢出、unknown 退款、proof cap 漂移、隐藏非法
  用量和 detached clone/reload；LLM 覆盖缺字段、传输改值、完整渲染与稳定前缀。Runtime 覆盖 provider submission
  pre/post-CAS、response loss、same-version reload、confirmed/unknown 结算、duplicate，以及原 proof-budget CAS/退休矩阵。
- 真实 File/Shell input-addition 主线逐请求通过 Run owner 对账：请求快照只比 callback 时的账本少本次 physical
  reservation，已有 effect 和 proof 消费不丢失。原 no-document recovery 测试另断言每次物理请求占额、零虚构 proof batch。
  初轮把新断言放入不满足完整 semantic coverage preimage 的旧低层 fixture 导致失败；已撤回该测试接线，改用完整
  semantic request fixture 验证 CAS/settlement，没有放宽生产来源校验或原验收。
- CL-02/03 仍未关闭：一般工作范围、每轮实施目标/预期新事实、草稿、纯遗漏补充及无归因诊断仍缺完整合法路径；
  不以本切片证明真实模型求解、原 Workspace 完成或 release DCE 通过。未部署、激活 Run 或改写历史归档。

本切片验证：

- 最终 shared 与 LLM 全树通过，Runtime 全包通过（93.949s）；追加的容量反例另经 shared 定向 race 通过（1.050s）。
- 最终 Orchestrator `TestRuntimeSemantic|TestSemantic|TestDriveRuntime(ExecutionLimits|ProofBudget)` 主线通过（43.490s）；
  后加 no-document budget 断言通过定向 race（1.143s）。shared/LLM/Runtime 预算、批次及相关反馈的定向 race
  通过（1.097s/1.497s/2.934s）。fmt、vet、build、boundary 和 diff-check 通过。
- 较早包含 ValidationInputReview/InputReviewV2 的扩展矩阵通过（160.140s），运行早于最后的独立封存 policy
  ceiling 与 evidence-revision 校验，不作为最终完整矩阵。最终完整 `TestDriveRuntimeInputReviewV2OmissionAndRestart`
  重复 3 次通过（23.941s）。当时只筛选 restart 子测试会遗漏前置 omission 基线并与零比较；这一结果不说明生产
  请求发生重放。该 harness 缺陷已在下列 RT-002 核对中修复，原调用数对比仍保留。
- 修改本切片前启动的 `go test ./... -count=1 -timeout=20m` **未通过**：Orchestrator Biz 1118.052s，观察到
  `TestDriveRuntimeInputReviewV2OmissionAndRestart/after_review_failure` 在 native preparation 的 File write 入口报
  `missing observer`。不是本切片的最终全仓验证，也不能用后续定向通过抹掉该失败。
- 依赖源码存在相关风险：magicCommon v1.5.15 `event/hub.go` 的 Subscribe 在 10ms 控制面入队超时后静默返回；
  SimpleObserver 已保存本地注册，Module Start 无失败回执。尚未证明该分支是此次失败的唯一原因，未改依赖缓存或
  用 sleep/业务 retry 绕过。该订阅可靠性问题列为后续集成遗留项，最终全仓通过与 release gate 仍未成立。

### RT-002 追加：依赖订阅缺陷的确定性复现与回归独立性

本切片只修改 WorkOrch 测试/诊断和实施记录，不修改 SM-1～SM-4、Base Biz、capability owner、依赖源码或历史归档。
遵循单 owner 边界：订阅控制面的修复归 `magicCommon/event`，不得让业务 owner 用重试或本地状态补写 Hub 订阅。

- 实际构建默认使用 checked-in vendor；已确认其中 v1.5.15 Subscribe/Unsubscribe 与模块缓存相同，均包含
  10ms 入队超时静默返回。版本查询当前最高为 v1.5.15，本机 magicCommon 仓库也尚无该修复，未改动该仓库。
- 新增 `scripts/diagnostics/eventhub_subscription_test.go`：公共 Hub/Observer API + `testing/synctest` 将
  controller 暂停并填满控制队列，虚拟时间推进后检查确认、调用可达性、精确重复及取消后的残留 observer。
  确定性复现 Subscribe 的本地 ghost registration 与 Unsubscribe 的 callback-free routing；没有访问内部字段、
  模块缓存写入、真实服务、wall-clock sleep 或业务重试。注入阻塞 ID 是测试控制点，不表示生产 ID 会阻塞。
- 诊断断言的是应满足的正确合同；当前依赖导致红灯，不能反转断言把缺陷变成通过。使用显式
  `eventhubdiagnostic` tag，尚未作为普通 suite 门禁。正式依赖修复接入后应升格为常规回归，并重跑整仓。
- `TestDriveRuntimeInputReviewV2OmissionAndRestart` 改为首次选中子测试惰性建立 uninterrupted 基准；
  单独选择 restart 也比较完整调用数，不以 `baseline != 0` 跳过验收，保留原 owner/receipt/credit/restart 断言。
- 这些证据确认依赖存在缺陷，但没有唯一归因上轮 `missing observer`；也没有补齐 CL-03 一般实现循环、
  CL-04/05 草稿与补充、CL-06 诊断或 full DCE。框架修复需要本轮跨仓范围确认及后续正式版本接入，不能只改
  vendor 却声称声明的依赖已修复。

验证结果及范围：

- 默认 vendored 依赖的诊断连续 3 次失败；追加取消路由断言后的 race 诊断也连续 3 次失败，`-mod=readonly`
  选择 go.mod 依赖的诊断连续 3 次同样失败。均复现缺陷，不是通过的回归。完整命令和复现机制见
  [诊断说明](../../scripts/diagnostics/README.md)。
- 单独筛选 `after_review_failure` 重复 3 次通过（16.117s）；完整 omission/restart 父测试 race 通过（36.827s）。
  不以此抵消先前的整仓失败。
- fmt、受影响 Biz 与显式诊断的 vet、boundary 与 diff-check 通过；本切片没有生产代码变化，不宣称
  整仓/release 或已部署 Workspace 验收通过。

### RT-002 追加：正式 magicCommon v1.5.17 接入

用户要求更新最新 magicCommon 后，WorkOrch 从 v1.5.15 更新至正式 v1.5.17，`go.sum` 与 `go mod vendor`
同步；其它依赖版本保持不变，不修改框架所属仓库或模块缓存源码。此前“未有正式修复版本”的记录为当时事实，
不再表示当前状态。正式 tag 为 `1aed4edc0b248c86b9909392e8ab5ef4c2b4e459`。

- Hub/SimpleObserver 返回 checked error、BackgroundRoutine 新增 Context 方法；所有受影响测试替身同步适配。
- Base Biz 的 void 必需订阅/任务包装按框架合同 fail-fast，由 framework guard 将失败转为 lifecycle error；
  不吞错、不在各业务组件增加重试/延时或订阅状态机。
- 端侧 generic/projection/server-stop 订阅检查失败，组合订阅失败回滚前段，不报告虚假 live。
  进程启动失败进入清理，检查式关闭失败保留 started 以便继续清理，不伪装已停止。
- 现有 owner 在 Stop/Teardown 中取消/排空 worker，尚不提供独立 Quiesce。进程层以不暴露 Quiescer 的薄 shell
  委托 DefaultService 的 checked ordered shutdown，再由 Application 排空/释放共享基础设施；没有复制插件管理、
  运行单元或业务状态。SM-1～SM-3、原 Goal/DoD、模板、封存输入和归档未因依赖升级修改。
- 旧 opt-in 诊断移到 `internal/pkg/events/eventhub_subscription_test.go`，进入普通 suite。新合同分别校验
  已准入等待真实确认、拥塞明确拒绝且保持 local/Hub 原状态、后续显式尝试可成功、duplicate 和取消幂等。
- 首轮整仓回归暴露旧 fixture 的错误清理顺序：System 使用 defer Hub shutdown 却在 t.Cleanup 停 owner；
  四个 Git restart 场景及下一轮发现的三个 validation scratch restart helper 在模拟全进程重启时漏停 File owner。
  后续成组回归发现 claim/evidence crash matrix 漏停 Shell owner、两个 governance restart 把 Hub 清理登记在 owner
  清理之后（LIFO 下提前关闭）。已统一为显式停止旧进程 owner、把 Hub final cleanup 先登记，没有豁免框架错误。
- 首次隔离 CI-safe MVP 在 activation 后未调度：新版 Hub 正确拒绝 canceled context，而异步 Run 通知在 retirement query
  时仍沿用已结束的 HTTP request context。SM-4 两个通知入口现于 admission 前绑定已有 Orchestrator lifecycle/workspace
  context；同步 command 仍响应 caller cancellation，owner Stop/retirement/source/singleflight 门禁保持，无新增状态机。

已运行验证：

- EventHub/必需订阅的专项 race 20 轮通过；`-mod=readonly` 的订阅回归 20 轮通过，vendor 与模块源码一致。
- `go mod verify` 全部模块校验通过；magicEngine 保持 v1.5.3。
- Base 完整 race 3 轮通过，含 sync/async/timer 提交失败不得吞错；process/backend 专项 race 3 轮通过。
- SM-4 active/resume/continuation 在 canceled caller 下准入、duplicate 合并、owner canceled/retired/wrong-source 拒绝，及 failed-start 清理专项 race 3 轮通过。
- 上游 event/task/framework application/service/plugin common 全包 race 通过。
- 全树编译检查、受影响 process service 全树、修正后的 System/Git/scratch restart 回归、fmt/vet/boundary 通过。
- 修正后的 claim/evidence crash matrix 和两个 governance restart 定向回归通过。
- 两次隔离 managed service 的构建、启动、HTTP/console 检查及退出通过；二进制依赖信息确认 v1.5.17。
- 隔离 `verify-ci-mvp-file` 全部通过：HTTP、template/soul、LLM execution、MCP stdio；MCP Run completed，4 completed dispatches、evidenceSequence=3。
- 最终 `go test -json ./... -count=1 -timeout=25m` 全仓通过（退出码 0，无 failed test/package）。不以依赖专项或 mock 部署验收替代整体
  Workspace 开发闭环、真实模型或 full DCE 验收；CL-03～CL-12 的未完成项仍保留。未提交、部署到现有实例或改写归档。

CL-02 追加子切片（工作区变更，未提交）：封存预期与实测区分、同源 owner diagnostic 反馈。

- 发现 semantic 已有回执/检查反馈缺少原 owner 故障码、revision 和 validation diagnostic，历史未区分请求对象/预期作用
  与实际回执。现在 `ExpectedEffectScope` 取原 sealed execution，`RequestedTargetRefs` 取原 sealed operation，
  rejected/none effect 不会被 expected durable effect 覆盖。所有值仍由既有 receipt/dispatch 反证，未添加第二份执行账本。
- 共享纯 `ProjectRuntimeSemanticExecutionDiagnostic` 同时接线历史及 exact repair feedback；保留原 owner/class/code/
  signature、expected/observed revision、完整 feedback hash、truncated/redacted 及 typed validation diagnostic。
  `summaryOnly=true` 明示仅摘要，`reportedExitCode` 不是进程执行/检查通过的证明；缺失 diagnostic 明示 null，
  原本必须具有来源的 repair feedback 不允许缺失。原文、preview、私有 detail ref、维护/治理授权不透传。
- SM-3 在新 session 前由原 typed request 携带，LLM 使用原 C4 renderer/integrity 完整保护；SM-2 原 failure private event、
  SM-4 只读重投影和 SM-1 均无新写集合或 transition。clone/hash/同版本重载、64 KiB 集合/provider ceiling 仍有效，
  不改变 C0～C3/stable prefix，不热替换配置、不改归档、不扩展 repair 为一般工作权限。
- 新增/扩展反例覆盖错误 owner、缺失修复诊断、摘要冒充完整、敏感字段/raw preview、hash 前像篡改、clone 隔离、
  accepted/rejected 回执零虚构 credit、预期/实际作用不同、C4 裁剪/退出码/截断标志篡改。真实 File/Shell 主线逐请求
  经 Run owner fresh-load 对账；保留原输入不可变、全部验收、预算和终态重启断言。
- 已通过 shared 全包、LLM 全树、Runtime 内核全包（93.751s）；真实 owner 的显式预算 uninterrupted/after_input_receipt
  两场景（37.305s）及原 historical-assessment 全部五个重启切点（95.570s）；定向 race、fmt/vet/build/boundary。
  随后补充摘要显式标记与预期/实际差异反例的最终定向 race 通过（Runtime 3.683s），上述真实 owner 两场景
  最终重跑通过（39.116s），fmt/vet/build/boundary 再次通过。此切片不关闭 CL-02/03：完整材料受控读取、
  一般工作范围/迭代授权、新路径反馈、草稿/纯遗漏补充/诊断循环和真实部署验收仍未完成。
  本轮未运行全仓 `./...` 测试、真实模型或部署验收，未激活已有 workspace、修改归档或 Git 提交。

<a id="cl-03"></a>

## CL-03：同一 CWI 内的一般实现循环

Owner / SM：SM-2 选择/认可本轮范围；SM-3 候选与封存操作；SM-4 重驱；File 等 owner 执行。SM-1 不选择工作。

代码落点：[reducer](../../internal/modules/kernel/orchestrator/biz/runtime/reducer.go)、[CWI ledger](../../internal/modules/kernel/orchestrator/biz/runtime/cwi_ledger.go)、[semantic planning](../../internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go)、[binding compile](../../internal/modules/kernel/orchestrator/biz/runtime/semantic_binding_compile.go)、[repair continuation](../../internal/modules/kernel/orchestrator/biz/runtime/semantic_repair_continuation.go)、[File owner](../../internal/modules/blocks/file/biz)。

- [ ] source-change 完成以后，未完成 DoD 能基于真实实现缺口产生同 CWI 的 bounded 实现工作，不要求先具备特定 rejected validation receipt。
- [ ] 复用 owner selector、prospective target 确认、working binding 和 reconciliation；允许“先做受约束工作再得到最终证明”，不允许无绑定 effect。
- [ ] 实现修改可能使相交 evidence 失效；只重开精确依赖的 CWI，无关证据保持 current。准备/修复 effect 不能提前满足 DoD。
- [ ] 每轮封存选择理由、原义务、工作范围和预期新增事实；完成当前工作后重新归约，不自动增生后继 Run 或终结 CWI。

测试：从不完整小项目经至少两轮实质实现再成功验证；source-change 已满足后仍能开发；无验证失败也能确认缺口；未 grounded target/受保护输入/越界路径拒绝；durable effect 不能直接 credit；effect 前后 CAS/restart/duplicate。

合同同步：Core semantic-convergence、Orchestrator、software-dev default CWI/proof contract、File。完成门槛：一个新协议真实 owner 垂直正例同 Run 完成；不是仅多生成一份计划文档。

<a id="cl-04"></a>

## CL-04：未发布验证草稿生命周期

Owner / SM：SM-3 持有候选文档事务；SM-2 决定继续修订/发布资格；Shell 生产结构事实；File 拥有发布 effect；SM-4 恢复。

代码落点：[input review 合同](../../internal/pkg/workorchruntimecontract/runtime_validation_input_review.go)、[payload](../../internal/pkg/workorchruntimecontract/runtime_validation_input_review_payload.go)、[semantic transaction](../../internal/modules/kernel/orchestrator/biz/runtime/semantic_planning_transaction.go)、[input review reducer](../../internal/modules/kernel/orchestrator/biz/runtime/validation_input_review.go)、[Shell input package](../../internal/modules/blocks/shell/biz/validation_input_package.go)、[LLM review projection](../../internal/modules/blocks/llm/biz/runtime_input_review_projection.go)。

- [ ] 引入有界的未发布候选 revision 语义，复用现有 SM-3 ledger；不先写正式验证文件再靠普通 patch 修正，也不新建第二个草稿 owner。
- [ ] 逐项核对原准则与完整候选字节，区分 missing、conflicting、unknown；完整报告与 request 身份匹配后才由 SM-2 分类。
- [ ] 已知草稿问题在封存额度内修订；未知语义走信息治理；解析/超时沿 request 合同。格式 correction 不得伪装为业务修订，二者仍消耗共享物理请求预算。
- [ ] 静态检查不运行代码；必要试运行单独授权/隔离。准入不要求产品已经满足所有 DoD。
- [ ] 一致草稿只经 File create-only 发布；多文件整包 receipt + Shell fresh exact readback 完成后才能用于最终 binding。部分发布既不 credit 也不静默清理旧文件。

测试：缺文档断言草稿修订后完整；确定冲突草稿改正但原准则不变；无法判断 ticket；发布前 bytes 漂移、已有路径、部分发布、评审 CAS/File effect 前后 crash；旧 report 重复/错 scope 不生效；额度耗尽终止。

合同同步：candidate-and-dispatch、semantic-convergence、Orchestrator、File/Shell/LLM readme、software-dev 输入准备。完成门槛：草稿失败可有界改正，未经评审的输入不能成为 proof；当前协议恢复与权限拒绝一致。

<a id="cl-05"></a>

## CL-05：已发布输入纯遗漏的合取补充

Owner / SM：SM-2 从真实 omission report 认可准备范围；SM-3 发布事务；Shell 定义/观察 bundle；File create-only；SM-4 恢复。

代码落点：[omission 分类](../../internal/modules/kernel/orchestrator/biz/runtime/validation_input_review.go)、[obligations 合同](../../internal/pkg/workorchruntimecontract/runtime_validation_input_obligations.go)、[addition 合同](../../internal/pkg/workorchruntimecontract/runtime_validation_input_addition.go)、[Shell supplement](../../internal/modules/blocks/shell/biz/validation_input_supplement.go)、[主线测试](../../internal/modules/kernel/orchestrator/biz/runtime_validation_input_addition_mainline_test.go)。

- [ ] 当前协议以完整 exact-current 的纯 missing 评审为独立来源；不能伪造 behavior-unobserved attestation 或省略授权依据。
- [ ] 将全部遗漏绑定原 CWI/check 与共享预算，经过 CL-04 草稿修订/评审及 owner-confirmed create-only 发布；不制造失败 receipt 或通用 patch 权限。
- [ ] whole-bundle 评审、fresh observation 和 dispatch preimage 均保留原 required checks 与有效补充；最终执行不能仅选择新检查。
- [ ] 同一 CWI/check 的内容/路径变化不刷新补充额度，已有无效补充不触发无限再添加；混合冲突或 unknown 不误判纯遗漏。

测试：精简复现 `run-024`，原 tools 检查保留且补充文档义务后真实执行；原检查失败时补充通过仍不 credit；混合结论、缺项报告、伪造 receipt、跨 CWI 复用、duplicate、bundle capacity、restart 和 shared-budget 耗尽。

合同同步：Orchestrator 当前“纯遗漏明确失败”段落通过新协议区分；Profile input addition、Shell bundle、File create-only。完成门槛：明确证明新路径正常完成，同时旧保护测试不被删掉。

<a id="cl-06"></a>

## CL-06：无归因失败的有界诊断和实现修复

Owner / SM：Shell/File 提供诊断和目标事实；SM-2 选择有界诊断/工作范围；SM-3 受管执行；SM-4 恢复。

代码落点：[Shell attribution 测试](../../internal/modules/blocks/shell/biz/validation_failure_attribution_test.go)、[Shell diagnostic](../../internal/modules/blocks/shell/biz/shell_diagnostic.go)、[grounding](../../internal/modules/kernel/orchestrator/biz/internal/grounding/validation_failure.go)、[repair](../../internal/modules/kernel/orchestrator/biz/runtime/validation_failure_repair.go)、[File owner](../../internal/modules/blocks/file/biz)、[LLM context](../../internal/modules/blocks/llm/biz/runtime_candidate_context.go)。

- [ ] 将“可读诊断信息”与“可修改实现范围”拆开：unattributed 不得直接 patch，但可在声明预算内取得缺失事实。
- [ ] 封存诊断目的、读/执行范围及来源；诊断会运行代码时使用正常 dispatch/budget，不能藏在纯 probe 中。
- [ ] LLM 假设必须引用事实；File 独立确认 scope/revision/保护边界，SM-2 再决定是否进行 CL-03 实现工作。
- [ ] 后续仍 fresh validation；没有归因或真实外部依赖分别明确失败/治理，不把内部缺诊断变成用户来编写验收 JSON。

测试：没有 observed implementation target 的失败，经只读事实确认正常源文件后修复并通过；测试行不授予改断言权；猜测越界、旧源码、重复读取无进展、截断未补齐、外部依赖缺失及诊断耗尽；全链 restart/duplicate。

合同同步：Shell diagnostic、File target confirmation、Core semantic/repair 与 LLM feedback。完成门槛：不是增加一个错误码映射，而是生产者和消费者能用 typed 缺失事实描述走完整合法路径。

<a id="cl-07"></a>

## CL-07：错误验证实现的受治理版本更替（单独准入）

Owner / SM：SM-2 版本资格及绑定裁决；SM-3 候选/发布；Decision 只管未决信息；File/Shell 管字节与执行；SM-4 恢复，SM-1 不变。

代码候选落点：[input version](../../internal/modules/blocks/shell/biz/validation_input_version.go)、[input protection](../../internal/pkg/workorchruntimecontract/runtime_validation_input_protection.go)、[review](../../internal/modules/kernel/orchestrator/biz/runtime/validation_input_review.go)、[proof lineage](../../internal/pkg/workorchruntimecontract/runtime_coverage_proof_lineage.go)、Decision/Approval 的既有 typed events。具体字段需先完成独立合同审定。

- [ ] 先提交准入审定：适用范围、何为原准则冲突、旧检查逻辑身份、有效补充保留规则、校准来源、inconclusive ticket、能力/预算边界及默认关闭策略。审定前不放宽 input protection。
- [ ] 使用新不可变版本，不改旧文件/历史报告；新当前绑定保留全部原义务及有效补充映射，明确被替代的是错误执行实现而非准则。
- [ ] LLM 可作一致性判断，但校准必须有适用正/反例及真实 owner 执行；不强制用户先提供外部参考产品，不把模型解释或 fixture 身份本身当等价证明。
- [ ] unresolved 只提供具体事实 ticket；拒绝通过 Approval 豁免冲突或普通 repair 改测试。新版本通过前不迁移 credit，fresh 证据取得后才正常 assessment。

测试：原实现错误且新版本保持准则；新版本弱化限定词/漏一语言/只测新检查必须拒绝；已发布原文件 hash 不变；不同输入版本报告混用、评审 stale、版本切换 CAS/restart、duplicate 不再消耗额度；未准入政策不得隐式启用。

合同同步：单独版本化 Core/Profile/owner 协议；不得直接覆盖旧 original + supplement all-of 定义。完成门槛：审定和校准都通过才允许启用，否则保持明确不支持；原案例若依赖此能力则 CL-12 不能宣称完成。

<a id="cl-08"></a>

## CL-08：跨路径预算、治理和恢复矩阵

Owner / SM：Orchestrator SM-2/3/4；Run SM-1 lifecycle/CAS；Decision/Approval 各自内部状态。禁止跨 owner 共用可变计数器。

代码落点：[execution budget](../../internal/modules/kernel/orchestrator/biz/runtime/execution_budget.go)、[governance](../../internal/modules/kernel/orchestrator/biz/runtime/governance.go)、[aggregate invariant](../../internal/modules/kernel/orchestrator/biz/runtime/aggregate_invariant.go)、[state repository](../../internal/modules/kernel/orchestrator/biz/runtime_state_repository.go)、[internal event write-set 测试](../../internal/modules/kernel/orchestrator/biz/runtime/internal_event_write_set_test.go)。

- [ ] 同一 Run/原 CWI/check 跨草稿、补充、诊断、修复共享上界；物理调用 reserved/confirmed/unknown/reconciled 在调用前后落账，重启和 path/hash 改变不刷新。
- [ ] 对 [crash cut 表](verification-matrix.md#crash-cuts) 的每个窗口做受管重启：exact identity、effect 至多一次、credit 至多一次、未知调用不重放、同 request/ticket 不重复开销。
- [ ] 真正 inconclusive 的 ticket resolution 只补事实；身份不匹配、stale/duplicate、容量不足及信息额度耗尽在各 owner 内正确处理。
- [ ] no-progress 判定不因注释、新文件名、另一个 request/hash 重置；真实局部工作也不重置全局硬预算。
- [ ] formal state/transition manifest 与代码一致；fresh-load invariant 能反证新增合法与非法中间状态，SM-4 不补写缺失 authority。

测试：各新路径同时覆盖 transition/invariant/idempotency/restart/stale/duplicate/budget；两个 Workspace 并发隔离；旧 sealed aggregate 不迁移。完成门槛：CL-03～CL-06 的所有关键窗口和共用额度都通过；CL-07 若启用同等覆盖。

已实施子切片（工作区变更，未提交）：统一 semantic coverage-order、proof-batch、attempt-1 correction 的
`请求意图 private event → 只读预检 → exact session CAS → submitted/allocation CAS → provider` 顺序。
此前初始/纠正入口提前创建 session；预检拒绝被送入要求已有 physical settlement 的 provider-terminal 路径，不能正常
收尾。本切片移除两个入口的提前 ensure，新请求预检失败复用既有 SM-2 semantic failure；已有 exact session 跳过
preflight，但仍经 SM-3 ensure 精确重验 manifest/prefix/correction/transaction/expected-document，SM-4 恢复原后继。

- 主写集合仍为 Orchestrator owner-private semantic ledger、request session、physical allocation 与既有 terminal/recovery
  协同事务；SM-1、capability owner、跨 owner EventHub 合同和预算上限不变，没有新增 schema、兼容路径或执行权限。
- 新增 `biz/runtime_semantic_preflight_test.go`：真实 LLM rendered preflight（充足预算成功对照）、typed 拒绝和非法 result；初始/batch/correction
  零新 session/provider/allocation；prepared/submitted/document-complete/recovery-pending CAS 前中断和写后响应丢失；
  semantic failure CAS 前后与 duplicate terminal；exact session 的五类身份漂移拒绝；physical consumption 唯一计数。测试恢复使用序列化分离的 aggregate，避免内存
  shallow slice 让失败 CAS 污染持久化前像。
- 对应原 semantic 主线更新两处 session 时点断言，保留 order/proof/correction、原 generation identity、provider-at-most-once、
  owner feasibility/binding seal 的成功证明；不降低验收要求。
- `go test -timeout=20m -count=1 ./...` 全仓通过（Orchestrator biz 876.789s，Runtime 105.939s）。全仓运行期间追加的
  recovery-pending、session 漂移、failure CAS 与真实渲染成功对照已单独重跑；最终新增 20 个场景及既有 semantic 主线的
  定向 race 测试通过（3.418s）。`make fmt-check`、`make vet`、`make verify-boundary-contract`、`go build ./...` 和
  `git diff --check` 均通过。未提交、未部署、未激活或改写现有数据。
- 此项只补齐 CL-08 的既有请求事务边界，**不关闭 CL-08**，也不实现 CL-03 一般工作循环、CL-04 草稿修订、CL-05 纯缺项
  补充或 CL-06 诊断执行权限；这些仍按前置合同逐项实施。

后续 owner 边界收口（工作区变更，未提交）：SM-3 的 session writer 原先允许 required/order-accepted/
proof-batch-document-accepted 创建 session，并且只对已有 session 反查部分身份；新建 session 可在预检后用过期意图落账。
现已由同一 owner-local `SemanticExpectedDocument` 纯投影统一 rendering 与写入准入，新建/复用都反查 ledger logical
request、manifest/prefix、active batch/CWI/order、修正身份/轮次、Definition seed、output ceiling 和 recovery 上限。
semantic ExpectedDocument 不再由 writer 覆盖身份/重算 hash 来修补输入；非法阶段或旧预检结果在任何新 CAS/分配前拒绝。

- 影响 SM-3 准入与 SM-4 exact replay 校验；SM-2 意图/failure 转换保持原 private event，SM-1、外部 owner 和预算策略不变。
- 新增 25 个 owner 准入/精确 batch/重启场景及 1 个 late-preflight 私有事件时序场景；包含损坏 hash、active batch 缺失/重复；更新三个旧测试夹具为先封存意图，
  再创建 session，补齐显式 provider ceiling；不引入测试专用兼容路径或放宽原验收。
- 首轮 `go test -timeout=20m -count=1 ./...`：Orchestrator Biz 通过（864.976s），其余包通过，Runtime 包两项旧断言
  把 required 阶段当成 session 准入而失败，故该命令整体未通过。随后只更新测试：acquisition successor 先拒绝 required，
  重启保持原 settlement，再经真实 private request-intent event 证明可以继续；target-selection guard 的反例在 request-open
  阶段单独验证 owner fact，避免被 phase gate 掩盖，同时增加 required 拒绝对照。没有放宽 production gate。
- 修正后 `go test ./internal/modules/kernel/orchestrator/biz/runtime -count=1 -timeout=10m` 全包通过（82.739s）；
  semantic/owner/acquisition/presemantic 定向 race 通过（Runtime 8.417s，Biz 3.479s）。`make fmt-check`、`make vet`、
  `make verify-boundary-contract`、`go build ./...`、`git diff --check` 再次通过。生产代码在首轮全仓后未改；其他包结果
  沿用该次全仓成功输出，未重新执行完整 `./...` 命令。全仓启动后新增的三个 hash/batch 反例已包含在本次 Runtime 全包
  与定向 race 内，不冒称已包含在首轮全仓测试二进制内。未提交、未部署、未激活或改写现有数据。
- 仍不关闭 CL-08，也不声称 CL-03～CL-06 或真实 workspace 持续开发目标已达成。

<a id="cl-09"></a>

## CL-09：Result、TUI、Web 与整体目标跟踪

当前子切片（工作区变更，未提交）：移除旧反馈 renderer。

- 仅当前 `ResultFeedbackVersion=3` 进入 Run record、aggregate、initialize 与 Result material；缺失、0/1/2 及未来版本
  明确拒绝。无旧 renderer、默认值、归档改写或跨版本升级。当前版本的 input-review provenance 必须完整反证。
- SM-4 始终输出完整反馈；当前 coverage 不满足时不恢复历史 satisfied credit，原评估/证据保持原样。
  SM-2 仍唯一裁决，Result 只保存；SM-1 和各 capability owner 的事件/写集合不变。
- 原旧版本成功夹具改为显式当前版本；旧/未知版本改测零改写拒绝；保留真实 Result 已持久化但 ack 丢失后的同版本
  fresh owner 重投与完全相同的 material/hash 断言。当前 input-review omission/restart 仍明确失败，不冒充 CL-05 完成。
- CL-09 仍未关闭：草稿/一般工作/诊断/补充等新路径及 UI 全链路反馈须随对应功能继续，不以 renderer 清理代替整体交付。
- 扩展 `go test ./internal/modules/kernel/orchestrator/biz -skip '^TestDriveRuntime' -count=1 -timeout=15m` 运行
  298.499s 后在旧 accepted Result 夹具失败：夹具仅在历史 assessment 放目标文字，current coverage 缺 TargetText。
  已显式补齐 current coverage 的 SourceScope/TargetText，保留原结果文字断言，不恢复旧渲染或放宽来源合同。
  最终 ResultProjection/TerminalFeedback/Feedback/Result 落盘后重启定向组通过（0.093s）；未再次运行完整非 Drive
  扩展组或完整 `go test ./...`。预算与三 owner 主线、Runtime 全包及 race 的本次验证见 CL-02 补充记录。

Owner / SM：SM-2 唯一裁决，SM-4 交付；Result 保存，workorchd/进程层呈现；SM-1 仅生命周期。

代码落点：[result feedback](../../internal/modules/kernel/orchestrator/biz/runtime_result_feedback.go)、[result projection](../../internal/modules/kernel/orchestrator/biz/runtime_result_projection.go)、[closeout contract](../../internal/pkg/workorchruntimecontract/runtime_result_closeout.go)、[projection contract](../../internal/pkg/workorchprojectioncontract)、[Runtime view](../../internal/pkg/workorchport/workspace_runtime_view.go)、[Web progress](../../web/workorch-console/src/runtimeProgressPresentation.ts)、[input review details](../../web/workorch-console/src/components/RuntimeInputReviewDetails.tsx)、[TUI/CLI](../../internal/services/workorch)、[observer](../../scripts/observe_runtime_progress.py)。

- [ ] 分开活动、durable 工作、fresh 验证、CWI/Goal 验收；显示总义务/已评估/已接受/失效/未判定，不用操作数包装完成度。
- [ ] 成功和失败都能回答做了什么、实际结果、还缺什么、下一轮为何选择/为何不能继续，并给出 exact owner/evidence refs。
- [ ] 区分草稿遗漏、已发布遗漏、输入冲突、能力不足与产品失败；不得把未测文档表述为确认产品没文档。
- [ ] Result 保持三轴及终结 hash，当前 renderer 完整投影；不维护旧 renderer 兼容，projection lag 不能改 authority 或发 Resume。

测试：failed 有 effect 但零验收、waiting、partial/fresh/stale、成功、当前 feedback 必需字段、SSE/reconnect、projection lag、Result→Workspace→Run ack crash/duplicate。完成门槛：API、TUI、Web、正式 Result 对同一 Run 的原因和验收数一致。

<a id="cl-10"></a>

## CL-10：Profile/Soul 当前配置选择与同版本恢复

Owner / SM：Workspace admission、Soul/Skill 内容 owner、System 默认资源协调；workorchd 只组装输入；SM-1 replacement，SM-2/3/4 使用 sealed input。

代码落点：[templates](../../internal/pkg/workorchdefaults/defaults/templates.json)、[software-dev souls](../../internal/pkg/workorchdefaults/defaults/souls/software-dev)、[default revisions](../../internal/pkg/workorchdefaults/revisions.go)、[Workspace activation 测试](../../internal/modules/blocks/workspace/biz/current_configuration_admission_test.go)、[activation ports](../../internal/modules/application/workorchd/biz/appservice/activationports)、[Run replacement](../../internal/modules/blocks/run/biz/replacement_internal_event.go)。

- [ ] 声明新协议和有限工作政策；提示词解释工作/证据/评审区别，不用 Soul 文本代替 owner 实现或权限。
- [ ] 新 Start/Restart 自动选当前配置且封存；同 Run Resume/recovery 不换协议。旧配置/default overrides 按现有 owner 合同处理，不批量回填旧 Workspace。
- [ ] 常规 setup 仍只需 template、Goal、DoD；目录可空并按现行规则解析，不强制用户提交 raw contextHints、target/recipe/check JSON。
- [ ] 先在隔离验证环境显式 opt-in。面向用户的默认启用须过阶段门槛；CL-07 有独立 capability/policy 开关，不绑定成默认“自动修复”。
- [ ] 仅验证当前协议 Definition/startup/Runtime/evidence 的同版本重载；replacement lineage metadata 按 Run owner 合同检查，不新增旧数据兼容代码。

测试：当前协议同 Workspace 新 Run 加载当前默认且既有 Run seal 不变；unknown version 和缺少当前必需字段拒绝；active Resume 不热更；failed 不能复活；唯一 successor、replacement crash cut、当前反馈重载与目录默认。

合同同步：Workspace builtin defaults、Core activation/recovery、Orchestrator、Soul/System/Application 边界。完成门槛：当前协议同 Workspace 可开始新 Run，同版本恢复不越过封存语义；不承诺旧数据跨版本继续，SD-001 仍延期。

<a id="cl-11"></a>

## CL-11：工作能力 harness 和完整 DCE producer 接线

Owner / SM：验证工程不拥有生产状态；真实 SM-1～SM-4、capability owner 与 Result 必须参与运行。详细场景见 [验证矩阵](verification-matrix.md)。

代码落点：[主线测试目录](../../internal/modules/kernel/orchestrator/biz)、[软件开发验证入口](../../scripts/verify-software-dev-goal-dod-e2e.sh)、[managed harness](../../scripts/verify-workorch-managed-full.sh)、[DCE schema](../../scripts/deployed-convergence-evidence-manifest.schema.json)、[verifier/registry](../../scripts/verify_deployed_convergence_manifest.py)、[release gate 测试](../../scripts/test_release_convergence_gate.py)、[CI](../../.github/workflows/ci.yml)、[Makefile](../../Makefile)。

- [ ] 将 CL-03～CL-06 组合成真实 File/Shell/Run/Result 的同 Run deterministic 闭环，已实现的 scripted provider fixture 明确只证明合同。
- [ ] 增加真实模型试验入口：固定服务/模型/输入/预算/依赖身份、fresh Workspace、保留所有尝试、独立核验产物。入口尚未实现前不能只写一个虚构 make target 声称支持。
- [ ] 在试验前固定成功率门槛及试验次数；至少重复运行并记录每次结果、成本和失败阶段。参考方案见验证矩阵，禁止挑选成功样本或失败后临时放宽标准。
- [ ] 按 DC-01～DC-12 的每个 required variant 实现 producer，生成 owner/read artifacts、mandatory assertions 并自验证；producer 测试与 exact registry 登记一起提交。先完成适用 CI-safe 子集不等于 full。
- [ ] 保持 manifest 安全与同 revision 聚合，缺 producer/空 ref/zero selected tests/手工成功 payload 明确失败；最终 full release gate 不降级。

测试：每个 producer 至少有成功 artifact、遗漏 assertion、错误 scope/hash、stale/duplicate、错误 provider class 和无效注册反例；harness timeout/重启/并发隔离；非软件模板正例。

合同同步：现有 DCE/服务验收合同仅补真实入口和证据映射，不降低原要求；新增验证补充归相应 Core/Profile/Interface。完成门槛：能力试验和 DCE 各自通过且可复验，registry 中每项均可追到真实 producer regression。

<a id="cl-12"></a>

## CL-12：原 Workspace 的受控部署与最终验收

Owner / SM：用户授权激活；Workspace/Run 按现有 owner 流程；Orchestrator SM-2/3/4 推进；capability owners 交付；Result 存结论。

- [ ] 获得独立部署/激活授权后记录实际 build revision、配置、工具链和依赖；不因实施本文自动重启正在使用的实例。
- [ ] 核对原 24 DoD 原文/定义身份及所有有效补充，无删除、单语言缩减或 best-effort 替代 strict release；先做环境能力检查，缺依赖如实报告。
- [ ] 使用原 Goal/DoD 全文在当前协议 fresh 验收 Workspace 中激活，不强制复用旧 ID/数据；现有 `run-024` 数据不动。不中途手改输入/aggregate/ticket 以推进，不让脚本承担开发或状态裁决。
- [ ] 观测多轮实质实现、输入准备/审查、必要诊断/修复、fresh 验证、逐项 CWI assessment、report/Git 和 Result ack。若仍无合法路径，按合同归因后回到对应 CL 项，不现场加旁路。
- [ ] 保存逐项 owner receipt/evidence、实际产物入口、代码快照、命令结果、资源/性能测量和完整剩余义务；重新构建/验证交付 snapshot。
- [ ] 原 Goal 完成、full DCE 与产品读模型一致全部满足后关闭本轮 TODO；Go 基线未完成前不启动 software-dev 多语言 adapter 扩展。

停止门槛：任何验收弱化、跨 owner 状态写、无 seal effect、旧历史改写、预算绕过或证据伪装立即停止启用并定位任务。模型/能力/预算原因导致 terminal 也须保存失败证据；不能将“安全失败”登记为目标已完成。

## 单项交付记录模板

实施者可在任务对应的提交/PR 描述中使用以下字段，长期证据由 Git 与受控 artifacts 保存，不在 `docs/` 建收口日志：

```text
任务 / 前置依赖：
涉及 owner / SM / 主写集合：
新合法路径 / 保留的禁止路径：
当前协议、配置与数据使用边界：
代码及同步合同：
执行过的测试（含选择数、环境、结果）：
正例 / 反例 / CAS-restart / stale-duplicate / budget 证据：
剩余限制 / 是否允许启用 / 后续任务：
提交号 / 受控 artifact 索引：
```
