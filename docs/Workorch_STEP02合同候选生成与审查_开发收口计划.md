# Workorch STEP02：合同候选生成与审查开发／收口计划

> **状态：讨论稿，待方案评审；不是实施授权或验收报告。** 本轮只核对源码并编写 Markdown，未修改项目代码、运行测试／CI、启动服务或调用真实 provider。
>
> 固定参考：`muidea/workorch@0df7498ae372ece1be9c63fb999c2b068889e31a`。第一阶段按已报告通过并归档承接；原始验收记录与候选构建身份仍待核对，本稿不宣称已独立复验通过。

## 1. 本阶段只闭合一个用户结果

**已保存的 M1 输入 → 显式生成 → 派生验收 → 合同候选 → 六类独立审查 → 持久化 → 看懂并重新找到同一份结果。**

成功终点是候选达到 `ready_to_confirm` 且可审阅；负例须形成可查询事实及明确阻塞，但不能替代成功路径通过。用户能核对输入、源承诺、审查依据和剩余缺口。

本阶段不包含完整 PUT 编辑／修订闭环、用户确认／正式封存、M3 创建完成、准入／激活、Run、executionPlan、CWI 或执行结果；也不要求先重做全部 22 单元。已有其他入口不必全局删除，但本阶段入口不得隐式创建、保存修改或确认。

“固定输入”指已保存的精确版本；其他会话修改后，旧结果不得冒充新输入结果。生成时 sealed input/template snapshot 只是不可变绑定，不等于用户已确认／封存正式合同。

## 2. 现有能力与本次最小收口

复用既有后台编排、派生、候选编译、六审、Workspace ledger／permit、持久结果及查询。[1][2][3][4]

现有 Web 按钮串联 create／PUT／compile，不等于已存 M1 的纯生成闭环已收口。本次固定十项：

| 编号 | 必须交付的功能 | 收口口径 |
|---|---|---|
| F2-01 | 已存 M1 的只读进入与定位 | 展示服务端输入及版本；未保存文本不能成为生成依据；开页、刷新、列表读取不触发 compile |
| F2-02 | 显式生成精确输入 | 点击只提交该输入身份，不隐式 create／PUT；过期身份拒绝并回读，不悄悄生成用户未看到的新草稿 |
| F2-03 | 受理与结果分离 | 202／`compilationAccepted` 只表示任务受理；200 也可能只是原状态返回；显示 owner 的实际进度／结果 |
| F2-04 | 保留源承诺的候选 | Goal 原文、用户要求、模板必需项和派生项来源可追溯；required 与 all_of 不被删减、替换或弱化 |
| F2-05 | 六类独立审查 | 每类恰好一份，独立读取同一 proposal，结论、理由、覆盖、引用和 hash 可查 |
| F2-06 | 完整只读候选呈现 | 展示维度→要求→义务、验证声明及来源；用户能够核对 Goal 和每项 required 的去向 |
| F2-07 | 可辨认的阻塞 | 区分准备失败、明确失败、审查阻塞与结果未决；给出阶段／请求身份，不统一承诺“重试即可” |
| F2-08 | 持久结果可找回 | 从 Workspace 正式入口重开／刷新／重启后查询，找回同一候选及审查；不靠日志或页面内存重建权威 |
| F2-09 | 身份与恢复防护 | 重放不触发重复物理调用；旧响应不能覆盖新会话／版本；reserve 未决不触发第二次物理调用 |
| F2-10 | 有界调用与阶段隔离 | 沿既有 owner 预算及配置绑定，保留 usage／失败事实；所有路径均不产生 confirm、准入或 Run 副作用 |

**两项建议合同增量，不能写成现状已具备：**

1. **compile 输入前置条件。** 当前 POST `/api/v1/workorch/workspaces/:workspaceId/authoring/compile` 没有请求体 DTO，只在服务器收到请求后读取当前输入。后台的 hash 检查防止“捕获后漂移”，不能防止延迟提交生成另一份最新草稿。建议新增必填的预期输入身份，例如 `expectedInputHash`，并核验绑定模板快照；若需调用方显式提交模板身份，同步提供快照 hash 并作为前置条件。字段名／DTO 在合同评审中定稿。过期返回冲突并回读，必须在 reserve／provider 前拒绝；不能兼容性降级成“缺字段就用当前输入”。同步修改全部相关调用方和合同测试，不盲目再加会因 ledger 更新而变化的 `expectedWorkspaceRevision`。[1][2]
2. **完整候选的只读投影。** 当前 `DefinitionAuthoringView.proposal` 有条目、来源、资源、verifications、reviews 和 hashes，但没有完整 Dimensions／Requirements／Obligations 数组。建议在既有查询上扩展最小只读投影，让原 Goal、required、all_of、义务绑定可逐项复核；不新增可写候选仓库，不把 File 正文或凭据塞进 DTO。具体字段必须与 canonical 模型对齐。[5]

## 3. 组件边界与固定合同

| 边界／owner | 本阶段责任 | 必须保持的合同 |
|---|---|---|
| Web → workorchd | 显式动作、只读查询、展示与会话隔离 | command 带预期输入身份；query 不推进业务；受理不等于成功，投影不授予确认资格 |
| workorchd application | 编排 background task，准备 payload，调用与解析，投影结果 | 顺序 derive → proposal → 六审；沿 typed EventHub；不成为结果 owner 或新持久任务队列 |
| Workspace | 输入／模板身份、canonical 候选、permit／ledger、审查及诊断持久化 | owner-specific typed command/result；observation → reducer → CAS；只凭 FreshPermit 授权新物理调用 |
| File | 获取／校验资源事实与快照 | 引用及来源可核对；应用层不得绕过 owner 直接读文件并自封事实 |
| LLM | 配置与真实 provider document 调用 | 按精确配置／payload／permit 调用；完整、失败、未决区别清楚；不拥有 canonical identity 或业务 PASS 权 |
| Workspace → query／Web | 返回已持久事实 | 读模型有精确输入／候选绑定；SSE 仅用于失效通知及回读，不作为结果存储 |

provider 只提供 mapping；Workspace 从持久原文 decode／校验并编译 canonical 内容。模型不能改写来源、required、Goal 原文及身份。沿 concrete typed 合同协作，不跨模块直连 repository，不用临时 Run 填上下文。[3][4]

## 4. 身份、状态和语义不得走样

### 4.1 身份与查询

复用 GET `/api/v1/workorch/workspaces/:workspaceId/authoring` 及既有 diagnostics／preparation-diagnostics／success-byte-audits。查询与证据至少关联 workspace、input revision/hash、模板／资源／配置绑定、derivationPermitId、request identity、proposalHash、specificationHash；新增投影字段另行定稿。[1][5]

- inputHash 包含 revision，A→B→A 也不能复用旧结果；不能排序／去重输入而丢失原要求
- `proposalHash` 与最终 `specificationHash` 不可混用。现有 review 字段虽然名为 `specificationHash`，实际绑定的是 `proposal.proposalHash`；两端遵守现有语义，不静默改名或换义[8]
- 并发、迟到结果和配置／模板漂移须由 owner 精确绑定与 CAS 保护。客户端禁用按钮只是体验防重，不能替代服务端许可
- 完整重放读取持久结果，不重新读取可变配置以重造历史；未完成代次不能静默换 provider／模型

### 4.2 状态解释

| 当前事实 | 本阶段含义与安全动作 |
|---|---|
| 202 + `compilationAccepted` | 托管任务已受理，可能仍显示 draft 或旧准备诊断；只读观察，不宣布落库／成功 |
| `derived`／`reviewing` | 以当前持久事实展示阶段；缺少审查不能填默认 PASS；不新增 `compiling` 权威状态 |
| `ready_to_confirm` | owner 判定候选达到现有可确认条件；本阶段止于展示，不调用 confirm |
| Workspace 摘要 `reviewed` | 摘要投影，不等于 confirmed，也不能单凭该标签判定资格 |
| `review_blocked` | 候选存在但审查／验证声明等条件未满足；保留理由与缺口，不把结构齐全当通过 |
| `preparation_failed` | 前置准备阻塞；仅在能证明尚无该调用 permit 时，修复条件后可显式恢复，由 owner 决定是否发新 permit |
| request／view `failed` | 明确失败事实；同输入 compile 返回原事实，不能无条件自动重试 |
| `outcome_unresolved`／reserved 无完成 | 结果未知；保留身份并查询。不能假装失败、补造 completion、换 key 再发或清预算重来 |

同 derivation 下任一 reserve 未决槽位优先于其他已知失败／准备诊断，阻止新的物理调用。一次 request `completed` 也可能产生 FAIL／UNKNOWN review，不等于审查 PASS。[2][3]

现有 **15 秒只适用于 `preparation_failed` 显式恢复已受理、却仍读到原诊断的页面观察窗**。到期停止该特殊观察并保留最后事实；既不宣称事务取消，也不授权重发。它不是全流程编译／模型超时。重启恢复指重新查询 owner 的 durable facts，不承诺内存任务自动续跑或通用 exactly-once。[2][8]

### 4.3 Goal、六审与可确认条件

六类精确枚举：`dimension_coverage`、`negative_path`、`regression`、`resource_environment`、`traceability`、`verifier_feasibility`。[6][7]

- 原始 Goal 本身是 required；逐条保留原文和来源。每个 required 承诺所需判据按 all_of，optional 不得抵扣缺项
- 每类审查都须覆盖全部 required requirements，包含 exact Goal；不接受 N/A、空范围或“无风险／无资源所以免审”替代
- 六审各看相同且不含其他 reviews 的 proposal，不能读彼此结论后跟票；独立指输入／判断隔离，不虚称必需六家厂商或六个模型
- PASS 需要具体 reason、有效 evidenceRefs、requirementIds、精确 proposal hash 及完整 required coverage；FAIL 表示遗漏／弱化，UNKNOWN 表示事实不足
- 全部 verification 均须是诚实的 DECLARED，满足真实 Profile 适用性及逐 obligation evidence binding。DECLARED 只是能力声明，绝不是执行证据；六条 PASS 加任一 UNVERIFIED 仍应 `review_blocked`
- 静态结构／引用／覆盖校验不能替代语义充分性复核，特别不能用生成后的候选反推“原始承诺清单”

保留原文的澄清、分组和映射优化可属于候选构建；改变已设定 Goal、验收条件或合同约束，必须有说明理由的 ticket 并按对象取得批准。当前尚无 Run 就不能伪造 Run 身份。不得靠改 Goal、降低 required、随机换身份或清预算逃过失败；自动 correction 不列入本阶段最小范围。

## 5. 分三个增量交付，每步都交付可用结果

### S2-A：确定性纵向闭环

固定两个合同增量；用小型 M1 输入、预列源承诺和确定性 provider fixture，一次打通 **正式 Web 动作 → 真实模块／EventHub／持久路径 → derive → proposal → 六审 → 完整展示 → 重开找回**。

fixture 仅替代 provider 出站结果，不能把真实模块／EventHub／存储同时 mock 掉。首个增量就包含六审与持久，并合法达到 `ready_to_confirm`，无确认／Run 副作用。

**退出门槛：** 精确输入守卫、原承诺映射、六审隔离与绑定、合法 DECLARED、持久读回和一条本地 Web 主链全部可复核。若只能返回阻塞，应记录正确处理的负例，不能替代该成功门槛。

### S2-B：恢复、负例、版本及累计回归

补齐下表反例、持久切点、并发／迟到和预算／配置漂移；使用真实 owner＋受控 adapter，不要求每种故障都收费或浏览器注入。定向回归 Stage1 受影响的创建、列表／查询、部分持久化身份及 binary binding／source。

**退出门槛：** 失败分类、无二次物理调用、旧结果隔离、诊断持久失败不伪装已保存均成立；共享合同变动影响的累计回归有当前构建证据；未决问题列明，不留下“统一重试”逃生口。

### S2-C：一条有界真实 provider 路径及人工语义复核

授权、数据范围与预算就绪后，选一个已支持模板及小型非敏感 M1 输入，经正式链完成实际 derive／proposal／六审并找回。人工按预固定源承诺清单核对 exact Goal、required、all_of 和逐义务绑定，不仅依赖六审自报 PASS。

**退出门槛：** provider／配置、传输、原始结果、usage、ledger、候选和展示身份一致，满足成功门槛。若为 `review_blocked`，保留证据并说明缺口；不得弱化用户 Goal 凑绿，调整输入另走变更流程。

**依赖：** 方案／合同冻结 → S2-A → S2-B → S2-C → 收口评审；预算准备可并行。Stage1 原件待核对不阻止讨论、准备或获准的确定性实施；退出前以可绑定归档原件＋受影响回归建立基线可信度，不因附件缺失机械重跑 Stage1／22 单元。

## 6. 验收矩阵

U＝单 owner／纯规则；P＝真实模块与持久边界；W＝本地公开 Web；L＝有限真实 provider。层级可组合到同一候选构建，不要求每项 A02／A11 或每个故障都变成独立真实浏览器／收费运行。先将现有 authoring 测试映射到 A2 案例，仅补缺失或受改动影响的断言，不重建整套测试，也不重跑全部 22 单元。

| 案例 | 操作与必须包含的反例 | 独立观察／通过条件 | 层级 |
|---|---|---|---|
| A2-01 显式边界 | 开页、列表、刷新；点击生成 | 前者零变更／零出站；后者只生成已存输入；无隐式 create／PUT／confirm／Run | U/P + W |
| A2-02 精确输入 | 延迟提交、另会话更新、A→B→A、模板漂移 | 过期前置条件在 reserve 前拒绝并回读；不能生成未见版本或覆盖新结果 | U/P |
| A2-03 源承诺 | 独立 Goal、多 required／多判据；漏项、合并弱化、optional 替代 | 对照预固定原文清单及 all_of；所有缺口阻塞，不以 schema 合法当语义完整 | U/P + 人工 |
| A2-04 六审合同 | 少类、重复类、FAIL／UNKNOWN、空 reason、无效引用、少 Goal 覆盖、错 hash | 恰好六类、独立输入与全 required scope；任一反例不得 ready | U/P + W 展示 |
| A2-05 验证声明 | 全 PASS + UNVERIFIED、伪 Profile、不适用声明、缺 obligation binding | 均阻塞；合法 DECLARED 可查来源，但不显示“运行验证已通过” | U/P |
| A2-06 受理与重放 | 202 旧状态、200 原状态、双击／并发、响应丢失、已完成重放 | 只读确认结果；既有请求身份一致；完成重放的物理调用增量为零 | U/P + W |
| A2-07 结果分类 | 非法／截断输出、provider 拒绝／失败、submitted 无结果 | 失败／不完整／未决区分；保留责任与 usage，未知不记零、不换 key 重发 | U/P |
| A2-08 持久切点 | reserve 后响应丢失、出站后未决、settlement 失败、完成后回包丢失、重启 | 以 owner 事实读回；无 fresh permit 不二次调用；诊断未落库明确披露 | P |
| A2-09 准备恢复 | 资源／配置／render 失败，修复后显式恢复，旧诊断／新诊断 | 前置条件与新许可分开；15 秒仅特殊观察窗，到期不冒充取消或重试授权 | U/P + W／假时钟 |
| A2-10 预算与边界 | typed budget 拒绝、普通 conflict、未知 reserve、额度竞争、配置更换 | 分类不混淆；实际／未知 usage 可追溯；不无限调用、不静默 fallback | U/P |
| A2-11 查询与会话 | 刷新、重开、切 Workspace、迟到 GET、revision 倒退、投影中断 | 找回精确候选；旧响应不污染当前页；query 串行／失效有效；不隐式生成 | P + W |
| A2-12 累计与真实路径 | Stage1 受影响回归；一条限定模板／数据／预算真实链 | 当前构建及原始证据绑定；人工 Goal 覆盖复核；无阶段越界或凭据外泄 | P/W + L |

各层公共断言：拒绝越权，只传获准资源，不把密钥／敏感原文写入公开日志或 Git。真实 provider 不能替代语义反例及恢复覆盖。

## 7. 调用预算与执行授权

实现评审时据当前 owner 限额、实际模型和用户预算固定：允许 provider／配置、可发送数据、输入输出上限、阶段与单请求时间／调用／费用界限、并发及未知 usage 处置。已有主链含 derive、proposal、六个独立 review 槽位；是否命中重放、每步实际物理调用及费用须以 ledger／适配器事实核对，不在本稿另造随意的调用次数上限。

本轮不授权收费或外发草稿；执行前补齐缺少的数据／provider／费用授权，不重复询问已有授权。fixture 须证明未误连网络；预算耗尽、来源不明、依赖漂移、required 缺口或结果未决时停止并保留事实，不修改目标／身份绕过限制。

## 8. 收口证据与后续边界

交付一份简明 Markdown 收口报告及其原始材料，最低包括：

- **构建身份：** 精确 commit、binary/source 绑定、环境／配置标识；统一关联本次候选构建
- **业务身份：** 输入 revision/hash、模板／资源绑定、request／permit、proposalHash／specificationHash，以及六审对应关系
- **案例证据：** 固定源承诺清单、每项实际断言和命中数、fixture／真实模块边界、调用计数、owner 结果、关键 Web 观察及原始响应
- **诚实结论：** 分开写方案评审、实现、确定性集成、Web、真实 provider、Stage1 证据承接和遗留阻塞；未运行不写 PASS，阻塞处理通过不替代成功路径通过

原始材料存于持久、受限的附件／证据存储，报告关联位置和身份；不能只留 `/tmp` 或把凭据／敏感原文提交 Git。不同运行可组合到同一候选构建，受合同变化影响的旧证据须重验。

**先评审范围及两个合同增量，再另行授权开发；下一阶段才讨论用户确认／正式封存。** 本阶段交付可复核候选与审查，不替用户确认。

## 固定源码依据

以下均固定到本稿参考 SHA；用于区分“已有实现”与“本稿建议”，不是执行通过证据。

1. [authoring HTTP 入口与查询](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/internal/modules/application/workorchd/service/server/definition_authoring.go)
2. [后台受理、输入核验及 lifecycle](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/internal/modules/application/workorchd/biz/definition_authoring_background.go)
3. [实际 authoring proposal／六审与重放路径](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/internal/modules/application/workorchd/biz/definition_authoring_specification.go)
4. [派生验收、资源／配置准备及 permit](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/internal/modules/application/workorchd/biz/definition_derivation.go)
5. [现有 management DTO](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/internal/pkg/workorchmanagementcontract/definition_authoring.go)
6. [六审 required coverage 校验](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/internal/modules/kernel/workspace/pkg/models/goal_specification_review_coverage.go)
7. [canonical specification、验证声明与审查规则](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/internal/modules/kernel/workspace/pkg/models/goal_specification.go)
8. [Web 身份、确认资格与准备恢复观察](https://github.com/muidea/workorch/blob/0df7498ae372ece1be9c63fb999c2b068889e31a/web/workorch-console/src/workspaceAuthoring.ts)
