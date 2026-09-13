# 合同、状态转换与当前数据边界

Status: proposed — 非现行设计合同；基线及范围见 [总览](README.md)

本文描述待审定的目标合同。涉及新结构、阶段和事件的中文名称都是设计占位，不代表已存在的 Go API、enum 或协议版本号。CL-01 必须先核对既有类型与写集合，优先扩展现有 ledger/generation，不并行建立第二份权威状态。

按用户后续要求，本阶段不考虑数据兼容性；不增加旧版本加载、hash 回退、renderer 或归档迁移。当前协议的同版本重启与已封存输入保护仍必须完成。

## 1. 不变量

1. Workspace 封存的 Goal/DoD、DefinitionRevision、StartupInputHash 不被 Runtime LLM 改写。新增验收不得冲突，只能保留原义务并细化。
2. CWI 是既有验收单元，不改造成可由模型增删的任意任务图。CWI 内实现工作、诊断和输入准备不能自授终结 credit。
3. 每个 operation，包括草稿发布、诊断执行和实现修改，仍须走 SM-2 范围认可、SM-3 candidate/admission/sealed dispatch、capability owner 执行与回执。
4. 每个有副作用操作在 effect 前持久化 ExecutionKey/Fence 和 dispatch authority；恢复只能幂等重驱，不能凭投影或当前配置重编 authority。
5. 相同输入的更多请求、文件 hash 改变、成功退出或模型声称完成不等于验收推进。验收只能使用 exact、fresh、已裁决证据。
6. 新能力必须具备正常完成路径和明确的能力/信息/预算边界。禁止以反复提升预算或增加 diagnostic 特例代替通用事实合同。

<a id="component-boundaries"></a>

## 2. 组件职责与接收合同

跨 owner I/O 仅使用 EventHub 和接收 owner 定义的 concrete command/result。下表中的请求、回执与通知均是事实/意图输入，不能携带对其他 owner 的状态赋值指令。

| 组件 | 拥有的职责/状态 | 本轮收口与明确禁止 |
| --- | --- | --- |
| `workorchruntime` Initiator | repository provider/factory | 核查接线；不添加 Runtime 策略、EventHub facade 或代替 owner 保存状态 |
| `application/workorchd` | typed bootstrap、Start/Resume 入口、HTTP/SSE/projection | 传递 sealed version 与读模型；不选择迭代步骤、不编译 proof、不恢复 authority |
| `kernel/orchestrator` | SM-2/3/4、Runtime 语义、事实归约、证据与终结裁决 | 复用 owner 私有 dispatcher 和同一 aggregate；新增合法路径，不新增旁路调度器 |
| `kernel/governance` | policy/risk facts、owner-local governance trace | 给出风险/政策事实；不决定 CWI 完成或代替 SM-2 恢复 |
| `kernel/system` | owner 默认资源启动协调 | 仅协调 ensure；不能把默认资源更新热灌入旧 Run |
| `workspace` | 定义、Template、admission、封存输入、activation admission | 新 Start/Restart 选择当前配置；维护定义身份；不将 runtime 草稿回写 Goal/DoD |
| `run` | SM-1、replacement/lineage、opaque Runtime CAS | 校验生命周期及 CAS；不解释 CWI、下一动作或验证结论 |
| `file` | 文件字节、副作用、target/revision/feasibility 事实 | 确认 scoped 实现修改或 create-only 发布；不决定草稿一致性、补充资格或 CWI 状态 |
| `shell` | 受管运行、验证输入身份/结构、执行与诊断事实 | 扩展输入 bundle 与诊断 producer；不把错误行变成写授权，不选择下一动作 |
| `llm` | provider 调用及响应事实、请求渲染 | 返回有身份的候选/评审；不直接调用下一 owner、不发 ticket、不封存绑定 |
| `git` | Git 副作用与匹配 revision 的回执 | 证明交付 snapshot；日志、模型总结和普通 Shell 输出不能替代 Git owner receipt |
| `mcp` | typed MCP 操作与回执 | 证明声明的能力事实；不承担 MCP LSP-Bridge 的业务实现或 Runtime 调度 |
| `ssh`、`web` | 各自 typed 操作事实 | 核查新事实 DTO 的 scope/budget 接线；不因新增开发循环获得额外执行权限 |
| `decision` | ticket、resolution 及其 owner-local 生命周期 | 仅处理具体未决信息；解票只返回事实，不能改准则、绑定或 Runtime 状态 |
| `approval` | 风险授权及自身生命周期 | 授权受保护副作用；不能批准降低 DoD、伪造“一致”或发验证 credit |
| `result` | 已裁决 Result/Artifact 的保存与读取 | 保存完整闭环结果及限制；不能重新推断完成状态 |
| `memory` | MemoryItem/RecallTrace | 提供带来源历史上下文，不把旧经验变成当前 owner 事实 |
| `skill`、`soul` | 各自内容、binding/revision/provenance | 提供声明与提示；不能通过文本扩展 adapter 能力、执行预算或修改旧封存合同 |
| `mcpregistry` | server/capability catalog 与选择元数据 | 提供当前已声明能力快照；不得直接派发或宣布验证通过 |
| `eventstream` | audit 事件及排序 | 仅审计事实；不驱动状态或通过投影重建 authority |

`internal/services/workorch`、TUI、Web 是进程/接口层，不新增 framework unit；共享 `workorchruntimecontract` 只容纳纯值、校验、clone/hash 等，不放 I/O、CAS 或选择策略。每个接收 DTO 的 definition/request/owner revision 等身份必须在对应 owner 校验，不能只依赖调用方。

## 3. 四个状态机的主写集合

| SM | 唯一 owner | 新路径允许的内部行为 | 明确不允许 |
| --- | --- | --- | --- |
| SM-1 | Run | lifecycle、replacement 与 opaque CAS 沿用现行私有事件 | 根据 failure code 选修复、从 failed 自动回到 running、热迁移 aggregate |
| SM-2 | Orchestrator | 归约当前事实、选择同 CWI 的有界工作范围、认可输入资格/绑定、关闭或重开相关证据、决定等待/终结 | 执行文件/命令、信任模型自己授予的操作/完成权 |
| SM-3 | Orchestrator | 封存文档请求/草稿候选事务、admission、dispatch、回执事务 | 从文档自行决定下一轮目标，绕过 SM-2 改 obligation 或直接接受最终证据 |
| SM-4 | Orchestrator | 调度已持久化待交付工作、lease/recovery、终结交付 | 根据最新目录/配置重建旧绑定，在恢复时补造新计划或刷新额度 |

CL-01 为新增字段指定一个主写 SM；确需协调写入时，必须列出一个 owner 内部事件的完整写集合、前置条件和单次 CAS 边界，复用现有 mutation transaction。禁止多个 SM 各自维护一份“当前任务/输入/预算”。接收外部命令或回执的 adapter 只转成本 owner 私有事件，不暴露内部 transition API 给其他组件。

新增路径至少要登记以下内部事件语义；它们不是拟新增的公开事件名，具体实现应优先复用当前 private event：

| 事件语义 / 主写 SM | 接收前提 | 本 owner 内唯一合法结果 | 禁止结果 / 验证索引 |
| --- | --- | --- | --- |
| 工作范围待归约 / SM-2 | 原 CWI、当前 owner 事实、封存能力/预算 | 认可有界工作范围，或明确等待/终止 | 无事实猜 scope、直接 effect；VC-01/04 |
| 候选文档完成 / SM-3 | exact request/manifest、完整 document admission | 持久化候选及事务事实 | 文档自己选择后续目标/授 credit；VC-02/09 |
| 输入评审事实待归约 / SM-2 | 当前完整 report、原准则、exact 输入版本 | 按草稿/纯遗漏/冲突/unknown 和启用协议选择唯一合法路径 | 伪造 failed receipt、普通修复获得更替权；VC-02/03/05/06 |
| 中间工作回执待归约 / SM-2 | SM-3 已持久化的匹配 receipt/reconciliation 事实 | 关闭当前中间工作，重新观察并选择本 CWI 后续 | 中间 effect 变成 terminal validation；VC-01/08 |
| 最终证明待验收 / SM-2 | fresh exact proof、完整义务、当前 binding | 正常 assessment、CWI 关闭或精确缺口反馈 | 复用 stale/不完整/其他 CWI 的 proof；VC-08/10 |
| 待交付工作恢复 / SM-4 | 已持久化 lease、request/dispatch/continuation/delivery 身份 | 幂等交付或 readback，事实回到正常 owner 私有归约 | 重建 authority、重新选择工作、刷新预算；VC-07/09/11 |

## 4. 合同对象及事实传递

以下是需要覆盖的语义，不要求按表各建一个 struct，更不能各建 repository。

| 合同语义 | 最少信息 | 权威边界 |
| --- | --- | --- |
| 不变准则锚点 | workspace/run、DefinitionRevision、StartupInputHash、原 CWI/check、原文来源、必需义务映射 | 来自 sealed Definition/Profile，不能由模型重述替代 |
| 当前事实包 | owner/source ref、观察 revision、交付树/目标身份、已执行操作及结果、freshness、截断/缺失项 | owner 提供事实，SM-2 归约；引用不明就不得使用 |
| 本轮工作范围 | 原 CWI/generation、事实缺口、允许 purpose/operation、边界与后置观测、预算身份 | SM-2 私有事件认可；不是外部 NextAction，也不是新的终结 CWI |
| 验证输入候选 | 不可变候选 revision、完整文件集/字节 hash、原 required-check 映射、补充关系、基础版本 | 候选事务归 SM-3；一致性资格与 binding 归 SM-2；File 只拥有实际文件 |
| 完整评审 | exact subject/request、逐义务 covered/missing/conflicting/unknown、理由和输入 refs | LLM 是语义观察者；完整性/身份校验与后续选择只在 Orchestrator |
| 执行反馈 | dispatch/execution/receipt、实际入口与检查、返回状态、有界 expected/observed、错误位置、缺失和截断 | 诊断不等于验收证据；能力不足不能伪装成产品失败 |
| 已发布输入身份 | 发布文件集、File receipt、Shell fresh observation、准则映射、评审身份、binding preimage | 发布资格由 SM-2 认可，物理字节由 File 管理；不能仅凭路径存在判断 |

任何跨 owner 变更请求还须带所需的 target revision、operation selector、binding/manifest identity；有副作用时额外绑定 ExecutionKey/Fence。内容改变、native/recipe/catalog 改变必须重新验证适用身份，不能移植旧“一致”结论。

有限上下文不等于允许默默丢信息：保留完整受管 artifact/ref，明确已展示部分、缺失部分及安全的后续读取方式。owner 容量不足须返回 typed 不可用；确需分批时先定义完整 membership 和汇总校验，不通过拆批漏掉合取义务。

## 5. CWI 内有界工作循环

下面是顺序关系，不是新增状态机或可由 LLM 执行的 action graph：

```text
owner 当前事实 → SM-2 识别本 CWI 的确切缺口并认可工作范围
                       ↓
             SM-3 文档/候选/封存 dispatch
                       ↓
               capability owner 执行并回执
                       ↓
        SM-2 归约新事实 → 尚缺实现/输入/信息：下一有界轮
                       └→ fresh 最终验证 → assessment → CWI/整体验收
```

必须保留默认 source change → DoD → report → Git 的终结依赖。source change 首次满足以后，当前未完成 DoD 仍可产生合法的实现工作；这种工作是同 CWI 的受约束子事务，不修改已封存 DoD，也不要求先伪造一个验证失败。

中间操作必须先有自己的 exact 工作绑定与 reconciliation 合同；不是“未有最终 proof 就可以无绑定写文件”。它可以取得真实执行回执，但不取得 DoD terminal credit。工作目标完成后只能重观测并申请 fresh final binding，不能把准备/修复 receipt 改名成 validation。

下一轮选择须说明：当前原义务、可用证据、仍缺事实、为何选择该工作、能力和预算是否允许、预期新增何种可验证事实。模型可提出实现假设，不能自报目标已 grounded。普通实现缺口可进入工作准备；没有任何可确认 scope、缺外部依赖或合同不支持时明确等待/失败。

## 6. 三条验证输入路径

### 6.1 未发布草稿：先审查，再不可变发布

适用于尚未成为 active proof 的新输入，不用于绕过已发布保护：

1. SM-2 认可本 CWI 的输入准备工作；SM-3 在已有有界文档事务中封存候选草稿和原准则映射。
2. Shell 按自身 adapter 提供静态格式、结构和可执行性事实；LLM 按 exact 草稿与原准则独立评审。不能用作者自己的符合性声明代替审查输入。
3. 纯遗漏或可确定的草稿冲突允许在同一预算内提出下一不可变候选 revision；旧候选仍保留，不能原地改评审内容。无法判断进入现有信息治理。
4. 一致且身份/能力检查通过后，SM-2 才认可发布范围，SM-3 封存 File create-only 发布操作。发布路径由 File 确认，不接受覆盖已有输入。
5. File durable receipt 与 Shell 对实际字节的 fresh observation 必须匹配已评审版本；SM-2 才能封存最终 proof binding，随后真实执行与 assessment。

静态 feasibility 不得加载/运行项目代码。必要的编译、加载、试执行必须是显式授权的受管操作，具备隔离、资源预算和 receipt；不能隐藏在 discovery/probe 内。草稿准入不要求产品功能已经全部通过，避免“必须先完成产品才能编写测试”的循环依赖。

多文件发布中途崩溃时，部分 File effect 可以存在，但不能取得整包资格或 credit；恢复读取相同 ExecutionKey 的回执并补完原事务，不覆盖未知旧文件、不静默删除已写文件。

### 6.2 已发布输入纯遗漏：create-only 合取补充

这条路径明确改变基线“v2 纯遗漏即失败”的合同，只能由新 activation 选择的新协议启用：

1. 来源必须是已持久化、完整且 exact-current 的纯 missing 评审；有 conflicting/unknown 混合项不能走此路径。
2. SM-2 根据原 CWI/check、完整遗漏清单和预封存 Profile 权限认可一次有界补充准备；不能伪造 rejected validation receipt 去套用旧 behavior-unobserved route。
3. 补充草稿经过 6.1 的完整评审与 File create-only 发布；不得修改原输入及其有效断言。
4. Shell 对 original + admitted supplements 的完整 bundle 返回 fresh identity；LLM 的一致性评审必须覆盖该 bundle 与全部原义务。
5. 最终验证仍合取执行原必需检查与补充检查；全部 fresh proof 经 assessment 后才能取得 credit。补充无效或预算耗尽明确终止，不通过换路径再加一次。

本路径只解决“没覆盖”，不解决“原检查与原准则矛盾”。当前通用入口必须重新证明完整授权链，不能只删除既有特殊补充守卫。

### 6.3 错误验证实现：单独准入的版本更替

默认关闭。CL-07 必须单独审定合同、启用政策和正反校准后才能实现/启用对应权限；本文不是开放修改验收的授权。

“原准则不可变”不等于“错误测试实现永远正确”。若旧断言与原准则矛盾，叠加测试无法纠正它；但普通 implementation repair 也不能据此改测试。拟定路径为：

1. 精确指出原准则、旧实现冲突、受影响 check 与证据，先区分准则歧义和验证实现错误。
2. 生成新的不可变输入版本；保留旧字节、评审、receipt、binding 及因果引用，不覆盖或重写历史。
3. 对新版本完整评审，保持原 required-check 逻辑身份、所有原义务及有效补充的映射；既有 original + supplement 合同不能被隐式降级。
4. 给出适用于该准则的独立正例、负例及已知错误实现校准；校准证明有限范围，不宣称通用语义等价或任意 Goal 的可信 grader。
5. 仅在准入协议允许、评审一致且校准/owner 事实满足时，由 SM-2 私有事件变更新的当前绑定，旧证据退役但历史仍可审计；新版本必须真实执行后重新评估。

新的当前绑定可不再执行“已证明与准则冲突的旧实现”，但这属于显式的新版本协议，不是准则/义务被移除。若无法证明保留原验收意图，只能提供具体问题 ticket 或明确不支持；Approval 不得豁免冲突。任何人工提出的新验收同样受一致性约束。

## 7. 无归因失败与只读诊断

失败仅含测试位置、截断输出或 `validation_test_failure_unattributed` 时，先返回“尚无实现修改授权”。新合同允许：

1. SM-2 以 exact failed check/receipt 和缺失信息选择有界只读诊断；参数仍由 File/Shell 等 owner 确认范围。
2. 取得当前源码/调用关系/构建或运行日志等允许的事实。会运行代码的诊断仍是受管执行，不视作免费读取。
3. LLM 提出有来源的因果假设与候选实现范围；File 确认路径、版本、selector 和保护边界，Shell 提供适用事实。
4. SM-2 才认可正常实现工作；SM-3 封存 effect；完成后 fresh validation，不能将诊断与修改 receipt 当成功证据。

测试断言位置、README 中的建议、模型猜测或新文件名均不能直接得到写权限。诊断结果仍不足时，区分真实外部缺失信息、环境能力不足、已耗尽诊断额度；分别按既有治理/等待/terminal 合同处理，而不是反复增补 error-code 白名单。

## 8. 预算、进展和治理

- 新逻辑 request、草稿 revision、补充、诊断、实施修复均消耗封存的相应额度及共享物理调用/operation 预算；新增路径、内容、recipe revision、重启均不刷新同 Run/原 CWI/check 额度。
- CL-01 必须列明总轮数、草稿修订、补充次数、诊断轮数、输入包容量和物理调用上限的来源及数值。无明确有限值即不得启用；本计划不自行提高既有 repair/information/provider 限额。
- semantic document 格式 correction 与业务草稿修订是不同原因，不能把完成一次格式 correction 当成获得新业务额度；submitted-unknown 仍占用预算且不得重放 provider。
- 每轮必须记录预期与实际新增事实。新 owner 事实消除缺口、实际受管工作及 fresh final evidence 分层呈现；注释变动、改文件名、重复读取不清零 no-progress debt。局部进步也不能刷新全局硬上限。
- ticket 仅承载具体、完整的业务无法判断事项，绑定 exact subject 与原 CWI/check。传输、空响应、解析、内部合同或预算错误不制造业务 ticket。
- LLM 可以判断验证实现与准则是否一致；无法判断由 Orchestrator 发起 typed governance 协作，Decision 管 ticket。ticket 答案只补事实，必须重新评审 exact 输入；不能直接授予一致、完成或恢复状态。
- 已有信息补充额度与 history 容量守卫继续有效；新协议若要调整必须显式版本化和验证，不得通过换 subject hash 绕开原 CWI/check 计数。

## 9. 事实反馈、完成与界面

一次执行反馈至少包括：原目标/DoD 身份、本轮选择理由、实际执行对象与 effect、实际运行检查、失败/缺失信息、已接纳与已失效证据、剩余原义务、可用预算、合法的下一步或不能继续的原因。

展示四个层次：活动发生、durable 工作完成、fresh 检查结果、CWI/Goal 已验收。不能把前两层累计成为“验收完成百分比”。Result 保持 coverage、assessment、evidence 三轴，未评估/无法判断/跳过不算成功；失败 Run 也必须保留完整反馈和可追溯原因。

最终成功仍要求所有 required CWI 完成、原 DoD 及有效补充全部满足、证据 fresh、report 和版本控制义务完成，再执行既有 Result→Workspace→Run 终结交付。UI/Eventstream/观察脚本不得根据日志、轮次或文件增量反向推进状态。

## 10. 当前协议与数据使用边界

1. 新合同由 Workspace activation 封存的 Profile/adapter/protocol 明确选择；不能按 diagnostic、磁盘形态或最新 Soul 猜版本。
2. 同一 Workspace 的新 Start/显式 Restart 自动选择当前可用配置；普通 Resume 和进程恢复继续使用该 Run 已封存合同。无需重建 Workspace，不等于热升级旧 Run。
3. 本阶段仅支持当前代码/协议生成的数据，不为旧 hash、旧 renderer 或旧 Run schema 开发兼容和迁移；未知或不匹配的身份拒绝，不能静默降级或补造 authority。
4. 现有部署数据不删除、不回填。新验收使用隔离 fresh 数据，不承诺已有 Workspace/Run 跨版本继续；复用的是原 Goal/DoD 全文与验收范围，不强制复用旧 ID。
5. terminal Run 不得 Resume 复活。当前协议下的新 activation/replacement 仍按 owner 流程执行，不能自动循环创建 successor 规避预算；同版本已封存准则、输入版本、receipt 历史继续不可变。
6. same-version restart、response-loss、duplicate、current-configuration activation 均须回归。只发布代码不等于默认能力已启用；最终启用受 G0～G4 和显式部署授权约束。

## 11. 生效方式

实现每条路径时，同步修订 [Core](../../docs/00-core/runtime-stage-contract.md)、[Orchestrator](../../internal/modules/kernel/orchestrator/readme.md)、相关 owner/profile 合同与测试。组件 readme 保持既有四段骨架；本文不成为 Runtime 使用的配置、dispatcher registry 或第二份规范。

最小可启用版本应先完成 CL-03～CL-06 的一般闭环；CL-07 未准入时明确报告“不支持错误验证实现更替”。不得把这一限制隐藏在“自动修复”产品表述中。
