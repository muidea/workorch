# 验证、能力试验与发布矩阵

Status: in-progress — 验证实施计划，已通过的子切片以实施任务中的实际结果为准；不表示新路径全部通过，基线见 [总览](README.md)

以下验证是对原验收的补充与细化，不减少原 DoD、现有 Core 不变量或 full DCE 要求。具体场景的 owner 与代码任务见 [实施任务](implementation-plan.md)。

当前阶段不考虑数据兼容性：使用当前协议 fresh 数据，取消旧 hash/renderer/归档迁移和跨版本恢复验收；同版本 crash/restart、stale/duplicate、预算及已发布输入版本不可变仍为必需项。

## 1. 分层证明，禁止互相代替

| 层级 | 要证明什么 | 不能据此宣称什么 |
| --- | --- | --- |
| V0 静态边界 | 唯一 owner、私有 transition gate、类型与目录中立性 | 能合法运行完整工作循环 |
| V1 reducer / 合同 | 合法/非法状态、hash/scope、budget、stale/duplicate/restart | 真实能力 owner 实际完成 effect |
| V2 deterministic owner 主线 | scripted provider + 真实 File/Shell/Run/Result 的实际 effect 和完整事务 | 真实模型能自主选择、生成和完成开发工作 |
| V3 真实模型能力 | 同一有限政策下重复完成小型实际交付，独立核验产物 | 当前完整 MCP LSP-Bridge Goal 已完成或通用 grader 可靠 |
| V4 原 Workspace 验收 | 原 Goal/24 DoD、有效补充、交付物及正式 Result 全部完成 | WorkOrch 所有发布场景已覆盖 |
| V5 full DCE | 全部 required scenario/variant 的真实 producer、artifact 与断言 | 任意未来 Goal 都能完成 |

“正确拒绝一次”“Run 正常结束”“CI 通过”和“Goal 已完成”必须分别记录。每层保留实际命令、被选中的测试数、环境身份、结果及证据引用；零测试、skip、超时或未执行不算通过。

## 2. 新路径回归矩阵

新测试按以下稳定场景 ID 归档；ID 是验证索引，不是新生产状态或诊断枚举。

| ID / 任务 | 正例与必须观察到的推进 | 必须拒绝或有界退出的反例 |
| --- | --- | --- |
| VC-01 / CL-03 | source change 已完成，仍在同 Run/DoD CWI 内多轮实现，最后取得 fresh validation | source receipt 冒充 DoD proof；LLM 自建 CWI/NextAction；无范围 seal 的 effect |
| VC-02 / CL-04 | 未发布草稿遗漏一个合取项，修订同一原义务的候选后评审一致、create-only 发布 | 修改已发布文件伪装草稿；有冲突/unknown 混合仍当纯遗漏；修订时丢限定词 |
| VC-03 / CL-05 | 已发布输入只测工具清单，补充文档义务；original + supplement 完整评审并实际执行 | 只执行 supplement；伪造 failed receipt；改原检查；路径换名重新获得补充额度 |
| VC-04 / CL-06 | unattributed failure 经受管只读诊断、owner 确认实现范围、修复后重新验证 | 把测试行当实现写授权；从猜测路径直接 patch；诊断免费运行代码；重复读无进展 |
| VC-05 / CL-07 | 准入后新不可变验证实现保留原义务，并通过适用正/反校准和真实验证 | 未启用协议、削减五语言或文档义务、覆盖旧输入、Approval 豁免冲突 |
| VC-06 / CL-08 | 完整语义 inconclusive → exact ticket → 补事实 → 新 request 重新评审，同 Run 继续 | 超时/格式错误制造 ticket；解票即授 credit；错 scope/旧 subject/二次信息额度绕过 |
| VC-07 / CL-01、08 | 草稿→补充→诊断→修复均进入统一可追溯预算，物理调用幂等记账 | 新版本/new path/restart 重置额度；unknown 算剩余；新 Run 自动绕过总上限 |
| VC-08 / CL-03、08 | 实现变更只使相交证据失效；新的检查重新关闭该 CWI | 无关 CWI 一起清空；旧 owner revision 继续 credit；新 receipt 复用旧 assessment |
| VC-09 / CL-04、08 | 多文件发布/回执/评审的每个 crash cut 后从 owner 存储恢复 | 部分发布当整包完成；回放 provider；重复 effect/credit；投影补造 authority |
| VC-10 / CL-09 | 失败、有 effect、未验证准确显示；完成时三轴及 Result/Workspace/Run ack 一致 | “写入 1 文件”显示 Goal 进度完成；未验证判缺失；UI 解除 wait 或恢复 failed Run |
| VC-11 / CL-10 | 当前协议同 Workspace 新 activation 选当前配置，同版本 recovery 保持已封存输入 | 热更新 active Run 的 Soul/Profile；改已封存评审 payload；未知协议静默降级 |
| VC-12 / CL-11 | 非软件 template 沿相同中立 Core 完成已声明工作；两个 Workspace 并发隔离 | Core 出现 Go/MCP/LSP 专用选择；跨 workspace 借 receipt、budget 或 ticket |
| VC-13 / CL-02 | semantic repair 收到 exact CWI reopen → receipt 的有界事实；原生 check 通过但行为证据不足仍保留真实结果；fresh-load 反馈与稳定 prefix 不变 | 无来源/错 scope/重复/篡改反馈；裁剪后丢失因果事实；失败反馈变成 effect/credit 权限 |
| VC-14 / CL-08 | semantic 三个入口预检通过后才创建 session；恢复复用 exact session，并结算唯一 physical consumption | 预检拒绝留下 open session；重启重复 preflight/已知 provider；session 身份漂移；CAS-before/response-loss 重复扣预算 |
| VC-15 / CL-02 | semantic 看到全量 sealed CWI 验收计数及当前 manifest/batch 的观察；同版本 owner reload 重建，进度单独变化只进入 C4 | eligible 数冒充 Goal 总量；记录数量冒充 credit；缺失/重复/错源/超限或裁剪被静默接受；跨 owner 设置状态 |
| VC-16 / CL-01 | 六个当前内置模板正常准入；typed validation guidance 随 payload 重载保留；两侧 merge 前及 final activation 独立校验 | 未知工作合同被忽略；错层字段、布尔字符串、混合列表或数值截断改变政策；无效下层被 overlay 隐藏；新增配置冒充已启用执行能力 |
| VC-17 / CL-01、08 | 显式物理限额进入 admission/bootstrap/共享 Run 账本；真实 File/Shell 完成与 restart；submission/operation claim 的 pre/post-CAS、duplicate 与重载 | 未知/缺字段/非整数/扩大限额；准入与 bootstrap 漂移；重 hash 扩额；换 request/purpose 或 unknown 刷新余额；预算投影成为第二份状态 owner |
| VC-18 / CL-01、08 | 可选原 CWI proof-batch 限额进入同一 admission/activation；SM-3 current/history 计量、CAS/restart/精确重放；真实 File/Shell 正常完成及耗尽终态 | pending 窗口误计；换 generation/request/catalog 刷新额度；同批次 correction 重复计；超额创建新 batch/session/provider/effect/ticket；把规划批次冒充工作迭代 |
| VC-19 / CL-02 | semantic C4 展示 Run 内已提交工作尾部，含已完成 CWI 的成功结果；receipt/dispatch/execution 反证、显式省略、重启重投影及 C0～C3 稳定 | 缺字段冒充未执行；窗口外坏来源被忽略；截断/重排/错 owner/外来 CWI；raw 参数/日志泄漏；历史 bound/accepted 冒充验收或 repair authority |
| VC-20 / CL-02 | semantic C4 观察 pre-submission 两个 Run 物理余额、可见原 CWI current/history 批次及原 durable seal 用量/余额；CAS/restart/duplicate/unknown 保持消费，真实 File/Shell 主线对账，稳定 prefix 不变 | 漏维度/CWI、错源/旧 revision、unknown 退款、失败/退休清零、治理续派重复计数、余额溢出或协调重 hash 掩盖改值；null 附加 cap 当无限执行、快照当操作或 credit 授权 |
| VC-21 / CL-09 | 当前 Result feedback 唯一渲染；完整原因、原评估不变、当前 credit 独立；Result 已落盘但 ack 丢失后同版本 fresh owner 重投完全相同的 material/hash | 缺失/旧/未来版本被默认或兼容渲染；拒绝时改写 archive；旧 satisfied 评估覆盖当前未满足 coverage；删除 input-review provenance 后仍接受 |

所有 VC 场景同时执行适用的 scope、stale、duplicate、预算、重启维度。不能只保留大量拒绝例而缺一条从允许输入到 terminal success 的真实 owner 正例。

逐 CWI durable-dispatch 额度补充（CL-01/08）：同一原 CWI 的输入新增与实现修复共用封存次数，额度充足应完成原验收，
不足应在下一 dispatch seal 前停止；使用真实 File/Shell、file-backed Run/Result 和 scripted provider 验证。
`TestDurableDispatchBudgetSealCASRestartDuplicateAndStale` 覆盖封存 CAS 前后与恢复，治理恢复原回归在限额 1 下运行；
`TestTerminalFailureRetainsPendingFeasibilityCatalogInSameCAS` 覆盖终态 cleanup 的 catalog 留存、CAS 前后、重载与零额外 effect。
不将 logical seal 计量等同于成功工作次数，也不把 scripted provider 结果作为真实模型或完整 DCE 证据。

VC-13 的当前子切片对应 shared `runtime_semantic_execution_feedback_test.go` /
`runtime_validation_execution_feedback_test.go`、Orchestrator `biz/runtime/semantic_execution_feedback_test.go`、
LLM provider/integrity 回归及 File/Shell input-addition 主线中的来源重载断言；实际运行结果记在 CL-02 实施条目。
这些用例不覆盖 VC-01～VC-12 的全部新能力，也不是 V3/V4 证明。

VC-14 对应 `biz/runtime_semantic_preflight_test.go` 与既有 semantic mainline/recovery regression；包含真实 LLM renderer
的无调用拒绝和 detached aggregate 的 CAS cut matrix。它证明既有请求事务边界，不证明真实模型完成开发；结果见 CL-08。
owner 写入边界另由 `biz/runtime/semantic_request_contract_test.go` 覆盖：禁止无请求意图阶段创建 session、拒绝旧 logical/
manifest/prefix/attempt/ceiling/recovery 上限以及跨 batch/CWI/order 的 ExpectedDocument，拒绝时零 CAS/状态/预算变更；
exact request 同版本重启与重复 ensure 不分配新请求。late-preflight 测试在预检响应前通过 owner 私有事件终止意图，
验证旧成功响应与 restart 均不重新开请求。

VC-15 对应 shared/LLM `runtime_semantic_progress_test.go`，以及 File/Shell input-addition 主线和 preflight failure
CAS-restart 的 owner reload 断言。只证明请求的只读事实链路与原验收状态隔离，不证明通用工作循环已完成；实际结果
见 CL-02 追加子切片。CWI 当前证据引用数不等于历史操作总数，不取代完整成功/失败执行反馈。


<a id="crash-cuts"></a>

## 3. 确定性崩溃窗口

使用 failpoint、owner response loss 或确定性测试 hook，不能用 sleep 概率制造故障。每个窗口使用隔离 fresh 数据；恢复必须重新加载 owner repository/opaque aggregate，不能沿用进程内存或人工修改快照。

| Cut | 中断位置 | 恢复后的关键断言 |
| --- | --- | --- |
| R-01 | SM-2 工作范围/预算认可 CAS 前后 | 无已封存范围则零 effect；已封存时保留同 CWI、scope、预算身份 |
| R-02 | provider allocation/submitted marker 后，terminal response CAS 前 | submitted-unknown 沿现有合同处理，不重放物理调用，不虚增剩余额度 |
| R-03 | 完整草稿/评审 response CAS 后，调用方收到结果前 | readback 同一结果；只消费一次 attempt；重复 response 不改变选择 |
| R-04 | 草稿评审完成，发布绑定/dispatch seal 前后 | 不一致或未 seal 零 File effect；已 seal 不按当前内容重编旧授权 |
| R-05 | File effect commit 后，Runtime receipt CAS 前 | 同 ExecutionKey/Fence readback，effect 至多一次；receipt 对齐前不给 credit |
| R-06 | 多文件发布的中间成员完成后 | 整包 membership 不变；缺任一 receipt/字节匹配不得生效；不覆盖/静默删文件 |
| R-07 | Shell fresh observation 后，SM-2 最终 binding CAS 前后 | 评审字节/owner revision/bundle 身份匹配；漂移先失效，不能拿旧一致结论 |
| R-08 | 执行验证后，evidence / assessment CAS 前后 | 同一 exact proof 至多 credit 一次；rejected/inconclusive 不变成功 |
| R-09 | 诊断完成或实现修复后，下一轮选择前 | 事实可恢复、额度不刷新；只有 SM-2 决定后续，SM-4 不猜下一动作 |
| R-10 | ticket 创建/事实 resolution/continuation CAS 前后 | 唯一 ticket 和 resolution；仅消费一次事实；重评 exact subject 后才继续 |
| R-11 | evidence invalidation 与 fresh revalidation 之间 | 相交集确定且可恢复；无关 CWI current；stale evidence 不能进入终结 |
| R-12 | Result→Workspace→Run 每个 ack 前后 | 结论/hash 相同，顺序及幂等符合现行合同，无双 Result/双 active Run |
| R-13 | 显式 Restart 的 replacement intent 与 successor activation 前后 | 唯一 successor、旧 payload 不变、新配置只入新 Run；SM-1 不选下一工作 |

同一矩阵还要注入 stale/duplicate 和不同 Workspace 的错 scope 消息，证明拒绝事件零第二 effect/credit/额度变更；必要的拒绝审计不应被误计作工作推进。

## 4. harness 工程与真实模型试验

### 4.1 可复现能力试验集

- 小型真实 Go 项目：缺少一项功能，需要两轮以上实现、实际交付入口验证和最终 Git/Result；不能由 harness 预置完成产品。
- 测试先行项目：草稿不完整、现有发布输入遗漏、无归因失败分别触发 VC-02～VC-04，验证它们能回到同一目标闭环。
- 已知错误实现、源码字符串伪检验、测试内替代实现、绕过交付入口、零测试、skip 和声明成功但无执行等校准负例。
- 真正信息不足、依赖不可用、明确不可完成/预算不足案例，验证有界且如实的 waiting/terminal，不把它们计成正例成功。
- 至少一个非软件 template 案例，使用其已声明真实能力；不以软件专用字段或 Go 验证命令作为 Core 必需条件。

fixture 可以准备隔离项目初态和依赖，但不得预制 successful aggregate、owner receipt、assessment 或 Result。执行中的 harness 只做观测和声明的故障注入，不能代写开发补丁、修 ticket、切换策略、复制成功历史或代替 SM-2 选择。

### 4.2 试验政策与统计

CL-11 在运行前固定：服务 commit、模型/provider 身份、参数、template/soul/profile/adapter revision、任务集、初始项目、依赖、最大 tokens/费用/物理请求/operation/轮次/墙钟时长、允许的治理介入及结果判定方式。

建议的初始准入方案为：对每类小型正例做 **5 次独立 fresh Workspace/Run 试验并要求 5 次均完成**，其中包括预先指定的重启试验；所有安全负例零错误 credit。此数字是待审定的能力试验政策，不是对原 Workspace DoD 新增业务准则，也不能据此声称统计上保证普遍成功。若成本约束需要调整，须在执行前说明并固定，不能看见失败后改分母或阈值。

每次保留：

| 信息 | 判读方式 |
| --- | --- |
| 原定义和实际交付 snapshot | 确认任务/范围不随执行失败变化 |
| owner 事实、完整请求/回执身份链、当前准则/输入版本 | 证明选择和 effect 有来源；原始内容受控保存并脱敏 |
| 实际功能结果、逐项 CWI 三轴、正式 Result | 独立核验实际交付物；不能只看模型总结或 WorkOrch 自报成功 |
| 实质实现轮次、输入修订、诊断、修复、验收证据变化 | 区分活动量、有限工作与目标验收，不把 hash 变化视为进步 |
| tokens/物理请求/耗时、unknown、无进展和失败阶段 | 在固定预算下统计全部尝试，不隐藏失败或截断数据 |
| 重启 epoch、exact scope、duplicate 与预算变化 | 核对安全性和可重复性 |

独立核验可以是案例专用测试、实际协议客户端、受管测量和人工证据复核；不能只让生成输入的同一段模型输出给自己打分。LLM 评审的语义结论可以使用，但必须与身份检查、真实执行和案例校准分开记录。

未通过时按任务/阶段归因：定义不可判定、能力缺失、上下文不完整、范围不能确认、模型未完成、输入评审、真实产品错误、预算或恢复合同。不能把每次失败转成新默认预算或新错误码补丁。

## 5. 原 MCP LSP-Bridge MVP 验收索引

来源是 `ws-a585a713bc3db2f0/run-024` 封存 Definition 的 24 条原 DoD；以下仅为测试设计摘要，**不是替代原文的第二份 DoD**。CL-00/CL-12 必须校验实际原文及 DefinitionRevision，所有原限定词、合取关系和 Goal 义务继续有效；新增证据方法也需一致性评审。

| 原条目 | 不能漏掉的义务 | 计划保留的证明方向 |
| --- | --- | --- |
| 01 | 提供并文档化九个工具 | 实际 MCP tools/call 入口与工具文档共同覆盖；只测 tools/list 不完整 |
| 02 | Go/Rust/Python/TypeScript/Shell 的 definition 全部返回真实路径与位置 | 对实际 bridge + 五种真实 LSP 后端的严格集成证据 |
| 03 | 未保存内容经 sync 后反映在 hover/definition/diagnostics | 磁盘保持旧内容、同步新内存内容，再从实际入口查询 |
| 04 | 语法错误返回描述、行、列、级别 | 已知语法错误 fixture 与真实 diagnostics，不只断言数组非空 |
| 05 | references/hover/diagnostics 可限制上限 | 原三个工具各自的可配置边界与返回值检查 |
| 06 | 超限有 truncated 或等价标志，不静默丢上下文 | 超限与未超限对照、完整性字段及截断行为 |
| 07 | 状态、活跃/空闲、重启次数、依赖存在与运行状态 | 真实实例生命周期前后 status 对照 |
| 08 | 指定及全部关闭，之后可重新初始化 | 进程/实例事实及后续实际请求，不仅返回 success |
| 09 | repair 默认只返回建议 | 默认调用前后无恢复副作用，建议内容可追溯 |
| 10 | apply=true 只允许安全实例级恢复 | 合法恢复与非法/越界动作负例，无任意命令权限 |
| 11 | 预热后的 Definition/Hover MCP 内部处理小于 50 ms，排除 LSP 计算 | 固定计时区间和环境，保存原始测量；不以整体耗时或临时改百分位替代原义 |
| 12 | 三语言同时开启，Server 自身常驻内存低于 64 MB，不含子进程 | 固定测量对象与环境、区分主/子进程；统计口径无法判定先澄清 |
| 13 | mcp-config.json 配置超时及各输出上限并在运行时生效 | 改配置前后实际请求行为，不只解析 JSON |
| 14 | kill LSP 后下次请求自动重启并初始化 | 真实 PID/instance epoch 与成功查询证明 |
| 15 | TTL 空闲回收 | 受控时间/等待与 owner 实例事实，避免只看日志 |
| 16 | 超上限时 LRU 回收空闲实例 | 多实例访问次序与回收对象，保持活跃实例安全 |
| 17 | Linux path/URI 转换正确 | 合法路径及编码边界的实际转换测试 |
| 18 | 十个并发 Goroutine，Request ID 匹配且无 race | 真正并发请求和 `go test -race`，不是串行循环 |
| 19 | 指定 build/test/race 命令稳定通过 | `go build -buildvcs=false ./...`、`go test ./...`、`go test -race ./...` 的真实执行记录 |
| 20 | 至少有 Go 真实 LSP 集成测试 | gopls 实际启动、协议交互与断言，无 mock 替代 |
| 21 | 本地可按依赖尽力执行，多语言严格集成是 CI 发布闸门 | 本地 missing/skip 记未验证；strict release 不跳五语言 |
| 22 | 配置样例含运行时、性能与多语言命令 | 样例完整性及实际可加载/生效性 |
| 23 | 集成/开发/运行时性能/DoD 状态/LLM System Prompt 文档 | 各部分有实质内容，与实际接口和结果一致，不靠关键词凑数 |
| 24 | Go 源码、测试、文档入库且可复现构建/验证 | 实际 Git owner snapshot，与验收的代码和输入 revision 一致 |

还需保留 Profile 原有 source-change、validation-report、version-control-snapshot 三项，不能只算 24 项测试通过。Go 是交付实现语言，五种 LSP 产品语言是原要求；[SD-001](../../TODO.md#sd-001软件开发-template-多语言能力扩展) 的延期不减少后者。

本表不规定把特定工具名、语言名或 grader 路径硬编码进 Orchestrator。具体检查实现归受控 Workspace 输入、Profile 或 capability adapter，Core 只消费中立 typed 合同。

## 6. 完整 DCE producer 收口清单

基线 registry 为空。下表是待实现 producer 的覆盖清单，不是已经通过的场景；详细 mandatory assertions 和 artifact 规则以 [现行 DCE 合同](../../docs/50-product-interface/deployed-convergence-scenarios.md) 及 [verifier](../../scripts/verify_deployed_convergence_manifest.py) 为准，不在计划中复制另一套 schema。

| 状态 | Scenario / variant | producer 必须真实产生的关键证明 |
| --- | --- | --- |
| [ ] | DC-01 / ci-safe | 多次 drive、effect 前 seal、真实 owner receipt、正式 terminal delivery |
| [ ] | DC-01 / real-provider | 同上但真实 provider；不能复用 ci-safe manifest |
| [ ] | DC-02 / real-provider | submitted/terminal CAS crash window、未知提交零 replay、幂等 readback |
| [ ] | DC-03 / ci-safe | durable effect 与 Runtime receipt CAS 两侧重启，exact key/fence，零第二 credit |
| [ ] | DC-04 / profile | 指定非 generic profile 的 owner mutation、精确 stale、fresh proof 再关闭 |
| [ ] | DC-05 / ci-safe | typed wait、匹配 trigger/probe、重启恢复、deadline/guard 终止 |
| [ ] | DC-06 / ci-safe | provider/capability allocation、unknown/reconciliation、exhaustion、可核对子账 |
| [ ] | DC-07 / ci-safe | genuine Decision/Approval dependency、exact resolution、系统错误零伪 ticket |
| [ ] | DC-08 / ci-safe | replacement intent、ack 顺序、唯一 successor 与 predecessor payload 不变 |
| [ ] | DC-09 / ci-safe | required CWI 三轴、exact refs/revisions/hash、HTTP/Web 与正式 closeout |
| [ ] | DC-10 / ci-safe | 两个隔离 Workspace 并发、重启、通知和 SSE；每个 scope 都有完整 refs |
| [ ] | DC-11 / definition | pre-seal 歧义仅 candidate/ticket，零 Run/effect；corrected all-of 正确 admission |
| [ ] | DC-12 / definition | runtime 隐式歧义、旧 Run 阻塞、显式修订/Restart、专用 resolution 及 crash cuts |

登记一项必须同时交付：实际执行入口、真实 owner/read artifacts、全部必需断言、exact scenario/variant registry、producer 正反回归、保留证据运行方式。禁止只登记 target 名、拷贝 verifier fixture、手写“passed”或用一个场景的 artifact 冒充另一场景。

聚合要求同 ServiceRevision、正确 provider class、完整 scoped refs、digest 与时间/epoch；artifact 不得越目录、经过 symlink 或混入凭据。没有相应 producer 即保持 release gate 失败，不降低 full 到 ci-safe。

## 7. 可执行入口与操作边界

以下是后续代码切片的验证命令，**不是本次文档创建已经执行的命令**。先定向测试，再完整基线；Go 测试显式关闭结果缓存并使用仓库的 15 分钟超时配置。

```bash
go test -timeout=15m -count=1 ./internal/pkg/workorchruntimecontract/...
go test -timeout=15m -count=1 ./internal/modules/kernel/orchestrator/biz/runtime
go test -timeout=15m -count=1 ./internal/modules/blocks/shell/biz ./internal/modules/blocks/file/biz
go test -timeout=15m -count=1 ./internal/modules/kernel/orchestrator/biz -run 'TestDriveRuntime.*InputReview|TestDriveRuntime.*ValidationInput|TestDriveRuntimeAssertionFeedback'

make fmt-check
make vet
make verify-boundary-contract
make test GOTEST_FLAGS=-count=1
make test-race GOTEST_FLAGS=-count=1
make cli-test
make console-test
make verify-deployed-convergence-manifest-tests
make verify-software-dev-goal-dod-e2e
```

上述 `-run` 选择既有相应测试；实现新场景后须核对实际测试列表/数量并补充选择器，不能漏测。最后一个 software-dev target 是合同/参考 fixture 校准，不是实际模型完成能力或 full DCE。

managed 验证应在专用 checkout/容器运行，显式确认没有指向部署数据的环境覆盖；默认创建独立临时数据。不得使用会清空数据的 `make run-file`。保留 artifacts 的 CI-safe 基础运行示例：

```bash
WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file \
WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 \
./scripts/verify-workorch-managed-full.sh
```

这一入口会构建和启动其管理的实例，只能证明对应基础组合；本身不生成完整 DCE。真实模型能力入口尚待 CL-11 实现，不能把现有 mock providers 的成功改名为真实模型试验。

以下路径是保留证据目录占位，使用前必须替换为实际 isolated artifact root；aggregate gate 只读取 producer 证据，不自动运行场景：

```bash
DEPLOYED_CONVERGENCE_MANIFEST_ROOT=/explicit/retained/artifacts \
  make verify-deployed-convergence-ci-safe

DEPLOYED_CONVERGENCE_MANIFEST_ROOT=/explicit/retained/artifacts \
DEPLOYED_CONVERGENCE_PROFILE=full \
  make verify-deployed-convergence-release
```

在真正发布前还需确认打包入口强制 full，任何低 profile 不能绕过。未部署、未启动真实 provider、未取得场景 manifest 时，记录“未验证”，不填写“通过”。

## 8. 启用、暂停与最终关闭

- G1 之前不得把新政策作为默认模板能力；G1/G2 中可在显式隔离试验中 opt-in，不能热影响既有 Run。
- 任一跨 owner 状态写、未 seal effect、错误 credit、原准则弱化、预算绕过或历史 payload 改写，立即停止对应新能力启用并回到责任任务；不继续用正例掩盖安全反例。
- 普通模型失败、能力不足或预算耗尽也须如实计入能力试验；调整方案后使用新固定基线重新试验，不抹去失败记录。
- 原 Workspace 的完成以 V4 全部证据为准；本轮完整 release 收口还须 V5。只能关闭达到门槛的任务，不能因“已安全失败”或“所有单测绿”关闭 Goal 完成项。
- 全部完成后将有效规范与测试入口归入现有 Core/Profile/Interface，移除本临时计划入口并保留 Git/受控制品证据；不把阶段进度搬进 `docs/`。
