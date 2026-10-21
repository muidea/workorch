# Workorch STEP03 精确确认与合同封存开发收口计划

> **讨论稿，待范围与合同评审。本文不是开发授权或验收通过报告。** 固定参考 `muidea/workorch@32c88cfc173a47531d88b6c53228d8abe2105277`；本轮只静态核对源码、已有材料并编写方案，未修改项目、运行测试／CI／服务或调用 provider。

## 1 本阶段闭合的用户结果

建议第三阶段完成：**已保存的 `ready_to_confirm` 候选 → 人工审阅并精确确认 → 可恢复的正式物化／合同封存 → owner 准入 `eligible` → 刷新、重开和重启后找回同一结果；全过程无 Run。**

成功必须包含正式事实、准入身份及独立回读。HTTP 200、正式 Goal／DoD 已出现或阻塞处理正确，均不能单独替代成功链通过。

本阶段终点称“确认、封存与准入闭环”，**不等同完整 M3**。最终 M3 仍要求已确认封存、必要资源具备、启动检查完成且尚无 Run；provider、Soul／Skill／MCP／Memory 等当前资源与能力检查留到后续激活准备阶段。现有 no-Run snapshot 只是显示投影；`CompileWorkspaceExecutionRestartBootstrap` 虽不建 Run，却可能写 managed configs、读取 Memory／调用 embedding，不能作为本阶段 GET 或确认中的纯检查。[6]

保留默认人工激活及用户明确授权的有界自动激活方向，后续另定合同。本阶段不做 Run、Runtime、executionPlan／CWI、执行派发、完整修订工作流或 TUI 创建入口，也不默认再跑收费 LLM。

## 2 十项最小功能

| 编号 | 必须交付的结果 | 收口约束 |
|---|---|---|
| F3-01 | 从已保存候选进入独立确认面 | 复用完整候选呈现；开页只读，不串联 create／PUT／compile |
| F3-02 | 确认用户实际看到的精确版本 | 固定 Workspace 与现有五字段请求；原 Goal、required／all_of、来源、义务、六审及 DECLARED 可核对 |
| F3-03 | owner 独立核验确认资格 | 核对持久 ledger、唯一六审、完整覆盖、声明适用性及当前绑定；客户端 ready 标记不授予权限 |
| F3-04 | 正式确认可持久恢复 | 复用 journal、确定性 Goal／DoD after-image；部分写、冲突和重放可判定 |
| F3-05 | 同一确认进入准入与封存 | 复用 typed admission 与 all_of 规则；保持 candidate／startup／admission 身份链，不重新生成候选 |
| F3-06 | 明确显示成功、进行中与阻塞 | 区分确认物化、准入、澄清 ticket、pending 与 eligible；不宣称当前运行资源全部就绪 |
| F3-07 | 丢响应、刷新与重启后找回 | GET 回读 owner 事实；仅显式重放原确认意图，不换目标或自动 POST |
| F3-08 | 抵御漂移、并发和迟到结果 | 初次 revision 栅栏、精确身份、session／query 隔离共同生效；不自动“确认最新版本” |
| F3-09 | 保留阻塞治理并补齐票据交付 | 展示 violation、offending refs 和已验证 ticket；限定显式恢复原票据交付，不自动改约束／解决票据，不伪造 Run |
| F3-10 | 有证据地守住阶段边界 | 确认／准入零新增 LLM 推理／embedding，零 Run／执行类派发；允许必要 File 资源读取核验 |

现有 Editor／Setup 的 confirm handler 已提交精确身份；create／save／compile 耦合在外围生成流程。建议从已保存候选增加独立确认入口，复用既有 DTO、GET／POST 与候选组件，不先造新后端 API，也不将 STEP02 生成按钮改成隐式确认。[1][5]

## 3 固定合同与恢复语义

| 组件 | 责任与最小变化 |
|---|---|
| Web | 新增聚焦已保存候选的确认面；复用呈现、请求与会话守卫，补显式恢复及终态展示 |
| workorchd | 复用现有 route、确认／准入协调与投影；补精确恢复接线及 HTTP 断言 |
| Workspace | 保持 journal／物化／admission 的 owner 权威；补切点、幂等、漂移与可见性证明 |
| File | 经 owner 合同核验当前资源事实；不由 application 绕过 owner 读文件自封结论 |
| Decision／治理读面 | 核对原持久 ticket／resolution provenance，限定补交同一票据；治理展示不授予准入 |
| LLM／Run／Orchestrator | 不增加本阶段业务流程；以调用计数、inventory 和派发断言证明隔离 |

组织继续遵循 [go-multi-module-dev v2.6.0](https://github.com/muidea/skills-repo/tree/main/skills/go-multi-module-dev)：复用现有运行单元与共享 Base Biz，跨 owner 使用 concrete typed EventHub；不为页面或纯映射新增 Module／Block。

不要求所有组件都改代码，也不展开全部 22 单元。正式结果的详细身份优先用既有 owner 查询核对，不因当前页面 DTO 未直接列出 DefinitionRevision 就预设必须加字段。

### 3.1 请求和身份

复用 POST `/api/v1/workorch/workspaces/:workspaceId/authoring/confirm`，请求恰为 `inputHash`、`derivationPermitId`、`proposalHash`、`specificationHash`、`expectedWorkspaceRevision`。不提交客户端编辑后的 Goal／DoD 或候选全文，不增加任意 UUID 逃过幂等。[1][2]

身份各有用途：input hash 绑定已保存输入；proposal hash 绑定被六审的 proposal；specification hash 绑定待确认规范；candidate ID／revision／hash 绑定准入谱系；DefinitionRevision／StartupInputHash 绑定提升后的封存结果。Workspace revision 是并发栅栏，不能代替上述身份。历史 review 字段虽名为 `specificationHash`，实际绑定 proposalHash，不能悄悄改义。

Web 冻结当前看到的五字段与 Workspace，再显式发送；HTTP/application 负责协调，Workspace 决定确认、物化、准入与投影事实，File 提供资源事实，Decision 拥有澄清 ticket／resolution。沿现有 typed EventHub 协作，不跨 owner 直写仓库。

### 3.2 物化、准入和可见性

现有顺序是 **journal `prepared` → ensure Goal → ensure DoD → `materialized` → admission／promotion**。这是可恢复的多步流程，不是跨 owner 原子事务。[2][3]

- `prepared` 的部分正式行对普通正式查询隐藏；恢复须比对确定性 after-image，冲突行不能覆盖
- `materialized` 且精确匹配的正式 Goal／DoD 可以读取，即使之后准入阻塞；不能承诺失败时全部回滚或全链零写入
- `clarification_required` 保留确认及 candidate／violation 事实，不表示 eligible；普通不合法 candidate 不产生新的 startup promotion
- 已纠正 candidate 的 `pending_clarification_resolution` 可能已有 startup promotion，但尚未 eligible；必须取得匹配的 Decision 持久 resolution／ack，不能用按钮关闭 ticket 代替
- 成功显示至少核对同 Workspace、`admitted`、`activationState=eligible` 与非空 `startupInputHash`；收口证据还须核对原确认、正式行、candidate 和 startup 绑定

以上页面分类是 owner 事实投影，不另建一套前端权威状态机。

### 3.3 不确定结果与重放

409 可能发生在确认已物化后的回读／准入环节，因此不能据此推断“没有副作用”。409、网络断开、超时或异常响应均保留原意图，再 GET 对账；不刷新 expected revision 后自动确认。[4][5]

重开从 owner 的 `confirmationRecovery` 恢复。其 revision 可以是当前 revision：初次准备受旧 revision 栅栏约束，已有 journal 的精确重放则由 journal 身份校验。只能采纳经过 Workspace、状态、模板、hash 和合法 revision 验证的恢复 tuple，不能客户端自行拼接。恢复仍需显式动作；查询、轮询和重开均不推进确认。

当前 helper 只覆盖 `ready_to_confirm`／`confirming`／`admitting`，不能直接把所有 `clarification_required` 也放开。若 Decision 暂不可用或回包／通知丢失，GET 只能显示“票据交付未完成”，不会修复它。

**需冻结一条窄恢复合同：** 已确认且 journal 为 materialized、当前 candidate 与原 authoring ref 精确一致、票据缺失／尚未验证时，可显式“恢复原确认／票据交付”。复用现有确认 DTO、owner 校验及同一 deterministic ticket 身份，保留当前漂移与栅栏；不得换 candidate、改规格、解决票据或新调 provider。Decision 创建原票据，或验证已持久原记录并补交丢失通知；恢复成功仅表示票据可验证，业务仍为阻塞，不是 eligible。须以端到端切点证明这一范围，不能无条件扩展通用 helper。[3][8]

其余缺失／不匹配恢复信息保持只读并明确阻塞。关闭页面只终止观察，不取消后台事实。

模板、配置、资源漂移须在已有必要边界重新核验；历史物化回执重放与新的准入校验分别处理。File 最后核验至 Workspace CAS 间没有跨文件原子保证，不把测试通过写成消除了全部外部改写窗口。

## 4 三个交付增量

### S3-A 确定性纵向成功链

使用经合法 owner 路径形成的固定 reviewed candidate fixture，先打通真实 owner／持久存储／EventHub／HTTP 的精确确认、物化、准入及回读，并接通最小 Web 确认面。fixture 只替代既有 provider 输出，不伪造 owner ledger 或确认结果。

**退出：** 一次人工动作能达到 eligible，重新查询身份一致，零新增 provider／Run；只有阻塞负例不算成功。

### S3-B 部分写与恢复闭环

覆盖各 journal／admission 切点、回包丢失、409 前后、冲突正式行、漂移、并发、ticket provenance 与页面会话隔离。先映射既有测试，仅补缺口与受影响回归。

**退出：** 同意图恢复、不重复物化／票据／promotion、不覆盖冲突行；既有事实不丢失、不把未知写成失败或成功。共享合同改动影响的 STEP01／02 回归有当前构建证据。

### S3-C 一条生产构建 Web 与重启链

经另外授权后，在限定环境用非敏感小样本完成真实进程／HTTP／浏览器成功链、一条阻塞展示，以及进程重启后同身份查询／恢复。优先复用既有已审候选；若需重建，明确 STEP02 fixture 边界并单独计量准备过程。

**退出：** 当前 binary／console／source 绑定、实际浏览器与重启可复核；确认／准入仍零新增推理／embedding、零执行派发／Run。此前真实 LLM 证据按既定范围承接；仅实际改变相关内容／协议／质量保证时，另提有界验证与授权。

依赖为合同冻结 → A → B → C → 综合收口。STEP02 的 R1、索引及候选等价说明缺口已关闭；遗留 R2“15 秒观察窗后新诊断仍锁住恢复”须独立修复并在综合退出前完成定向复核，不阻止本稿讨论或另获授权的独立准备。继承 CI 红灯如仍存在必须披露，不写全 CI／全仓发布通过。[5]

## 5 验收矩阵

U＝纯规则／单 owner；P＝真实模块与持久边界；H＝HTTP／真实进程；W＝组件或实际浏览器，报告须明确区分。每行不要求独立新建测试或收费运行。[7]

| 编号 | 覆盖与通过条件 | 层级 |
|---|---|---|
| A3-01 显式确认 | 开页／刷新／取消只读；点击仅精确 confirm；双击单飞且 owner 防重；无 create／PUT／compile／执行串联 | P/H/W |
| A3-02 首发身份守卫 | 五字段缺失／错误、跨 Workspace、stale 首发在 prepare 前拒绝，无新增 journal／正式行；此断言不泛化到全部 409 | U/P/H |
| A3-03 独立资格反证 | 未完成 ledger、少／重复六审、FAIL／UNKNOWN、错 hash、缺 Goal／required coverage、UNVERIFIED／不适用声明均被 owner 拒绝物化 | U/P |
| A3-04 journal 切点 | prepare 前、prepared、Goal 后、DoD 后、materialized 重启恢复；部分行隐藏，冲突不覆盖，同 journal 重放无重复正式行 | P/H |
| A3-05 漂移核验 | prepare／prepared 恢复／最终准入各必要边界覆盖模板、资源、配置及目录漂移；停止继续不抹历史，回执重放与准入分开 | U/P/H |
| A3-06 承诺与身份保真 | 原 Goal、required／all_of、来源及义务绑定到正式行；两种规范 hash 不混淆；DECLARED 不冒充运行证据 | U/P/H + 人工 |
| A3-07 准入成功 | exact candidate、Profile proof 与 prerequisite closure 合法；owner all_of 通过，candidate／startup／eligible 精确一致，重复无额外 promotion | U/P/H/W |
| A3-08 准入阻塞 | 非法 alternative／terminal policy、缺 proof、错 owner facts 或 hash 均保留 violation／ticket／确认事实；普通无效 candidate 无新 startup promotion／Run | U/P/H/W |
| A3-09 ticket 与 pending | 拒绝伪造／未持久／过期／错 scope resolution；Decision 失败、票据已存但响应／通知丢失后显式恢复同一票据，仍显示业务阻塞；pending 不误报 eligible，不自动改约束／解决票据 | U/P/H/W |
| A3-10 不确定与并发 | 409 在物化前后、丢响应、双窗口、CAS、重启、candidate revision 断档；GET 对账原意图，显式精确恢复，不换 lineage／自动确认新版本 | U/P/H/W |
| A3-11 页面隔离 | Workspace 切换、迟到 GET／POST、revision 倒退、异常恢复字段、关闭重开；旧响应不改新会话，终态不自动重试，查询不新增业务写入／provider | P/H/W |
| A3-12 综合退出 | 累计受影响回归；R2 截止后新诊断可解锁、旧诊断不解锁且零自动 POST；Run inventory、执行入口、推理／embedding 增量均为零 | P/H/W |

公共守卫：权限及来源核验有效；DTO 有界，凭据与敏感资源正文不外泄。File 等必要 owner 读取核验允许发生，“零能力副作用”专指执行类派发，不指零资源查询。

## 6 变更治理与收口交付

改变已设定 Goal、验收条件或合同约束，必须有说明理由的 ticket，并对变更后的精确候选取得明确批准。现有 pre-seal clarification 不能被宣传成已具备通用 post-seal 修订 API。变更治理范围内，本阶段只做阻塞／来源展示及限定的原票据交付恢复；后续变更另走新 Definition 身份，只有已有真实 Run 时才讨论其后继新 Run，不能创建 dummy Run 补上下文。

**确认完成后的业务产出：**

- 已完成 journal 及其精确物化的正式 Goal／DoD，可追溯到用户确认的原候选
- owner admission candidate 及来源绑定、DefinitionRevision／StartupInputHash，以及绑定模板、Profile policy 和项目目录的封存事实
- 精确匹配的 eligible 事实及可重新查询的结果摘要；不生成 Run 或执行产物
- 阻塞路径另给 violation、已验证 ticket 或交付未完成事实；pending 与 eligible 分开，不抹掉已完成的确认／物化

实施后的工程交付最小集：

1. 独立确认面及必要接线改动；同步更新确实变化的合同与文档
2. 现有测试映射、补充断言及逐案例实际结果；未运行项单列
3. 构建／环境／fixture 身份，以及 input、permit、proposal、specification、confirmation、candidate、startup 的证据链
4. 关键 HTTP／浏览器／重启记录、部分写恢复及零新增 provider／Run／执行派发计数
5. 简明 Markdown 收口报告：成功链、阻塞、遗留、累计回归和 CI 范围分别说明；原始证据持久受限保存并可定位

**当前需要确认的一项范围决策：第三阶段止于确认／封存／eligible 且无 Run，完整 M3 的剩余资源与启动检查留到后续阶段。**

技术评审随后冻结既有五字段与 journal／recovery 的精确绑定、独立确认面的终态与显式恢复，特别是 materialized 原意图的窄范围票据交付恢复。当前源码未证明必须新增 wire 字段；若实施中发现投影不足，再列具体字段及用途评审。方案确定后另行授权开发与验收，不从“讨论第三阶段”推导实施许可。

## 固定源码依据

以下链接用于核对现有合同，不代表本轮执行通过。

1. [HTTP confirm 与 GET](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/application/workorchd/service/server/definition_authoring.go#L166-L186)；[五字段 DTO 与现有投影](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/pkg/workorchmanagementcontract/definition_authoring.go#L112-L118)
2. [owner 确认、journal 与物化](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/kernel/workspace/biz/definition_authoring_confirmation_internal_event.go)；[正式行可见性](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/kernel/workspace/biz/definition_authoring_formal_reads.go#L11-L45)
3. [application 确认后准入](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/application/workorchd/biz/definition_authoring_admission.go)；[owner admission／promotion 与 Decision 绑定](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/kernel/workspace/biz/definition_admission_internal_event.go)
4. [confirmationRecovery 与状态投影](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/application/workorchd/biz/definition_authoring_view.go#L247-L276)
5. [Web 资格、恢复与会话守卫](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/web/workorch-console/src/workspaceAuthoring.ts)；[现有精确确认 handler](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/web/workorch-console/src/pages/components/WorkspaceAuthoringEditor.tsx#L250-L269)
6. [no-Run 显示投影](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_activation_snapshot_without_run.go)；[后续 bootstrap 编译边界](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/application/workorchd/biz/appservice/activationports/workspace_execution_intent.go)；[activation flow](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/docs/00-core/definition-convergence/activation-flow.md)
7. [既有 confirmation 切点测试](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/kernel/workspace/biz/definition_authoring_confirmation_test.go)；[既有 admission 测试](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/kernel/workspace/biz/definition_admission_test.go)
8. [Decision 原票据的创建与幂等交付恢复](https://github.com/muidea/workorch/blob/32c88cfc173a47531d88b6c53228d8abe2105277/internal/modules/blocks/decision/biz/definition_clarification.go#L149-L187)
