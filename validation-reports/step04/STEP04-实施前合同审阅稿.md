# STEP04 实施前合同审阅稿

> 2026-10-22 补充：用户确认保留 SkillHub 现行 status 的项目状态保存与配置服务端访问。
> 本文涉及“零网络/零资源写入”的表述不再覆盖该显式外部检查；其他 owner、no-Run、GET、
> 零付费和不自动 install/sync/use/apply/repair 的限制不变。该修订不授予 M3 或执行权，
> 详见现行 instruction-assets 合同与 `STEP04-SkillHub现行行为适配与验证记录.md`。

日期：2026-10-21（Asia/Shanghai）。源码基线：`d93463f37983575723393578c15faff51cf1d844`。

状态（2026-10-21 更新）：**用户已明确批准 §3 精确 no-Run 协议 v1 并授权实现；完整协议尚未实现、未签发 M3**。
用户回复原文：“批准该精确协议并实现”。本轮只读子能力的进展与证据单列于
`STEP04-只读准备事实收口与验证记录.md`，不将批准或子能力测试视为整个协议通过。
本文是计划 §7 的精确变更候选及批准记录，
不是当前规范、外部 issue、生产票据或验收通过记录。用户已请求 STEP04 开发与完成后的验证；
原计划另要求既有 fresh execution intent 入口的合同变更在实施前冻结并批准，本文落实该前置步骤。

补充确认（2026-10-21）：用户对旧 Workspace 新 Restart 的兼容边界回复“当前阶段不用考虑历史兼容性”。
据此缺 creation fence 明确阻塞新 Restart，不自动补标记，不增加迁移或兼容放行；既有 Resume 不变。
生产入口进展另见 `STEP04-生产fresh入口接线与验证记录.md`，本文件仍不是全协议完成记录。

## 1. 实施范围

按用户指定计划，仅处理已经准备好资源的 Workspace：exact seal 只读查询、稳定清单、
typed owner 观察、纯编译、显式检查、Workspace 准备账本、历史 M3、原操作恢复及 HTTP/Web。
缺 required 事实继续阻塞；不修改原 Goal/DoD/Profile 要求。

不自动物化或安装资源、不写托管配置、不 clone、不修改业务文件、不刷新外部 catalog、
不启动 MCP、不调用推理/embedding、不提交执行意图、不创建 Run 或 Runtime。
完整 Start/Restart 准备回执消费与自动激活仍属于 STEP05。

计划披露的 EventHub inventory 311/310 差异作为独立基线问题核对，不能直接改数字凑绿；
本次静态检查只确认测试仍固定为 310，未重跑 CI，未证明差异来源或修复完成。

## 2. 已核对的修改依据

- `workspace/biz/execution_intent.go`：当前在 admission 检查后转发给 Run owner；
  provider selection 的私有事件可能为 Noop，不能充当每个 fresh intent 的持久 reservation。
- `workspace/biz/execution_intent_internal_event.go`：已有 Workspace-local reducer/CAS 可沿用，
  但现有事件不记录准备 epoch、pending request 或 M3。
- `run/biz/workspace_execution_intent.go`：Run owner 在 `workspaceExecutionMu` 下处理 restart，
  包括首次 Run 和 replacement；Run owner 继续独占这些生命周期行为。
- `run/biz/run_queries.go`：普通 Workspace Run 列表未参加上述锁排序，并对 retired Workspace
  返回空集合；它不是首次激活无历史的完整证明。
- `workorchd/biz/workspace_command_gateway.go`：现有公开入口先编译 restart bootstrap，
  再提交 Workspace intent；新的准备入口不得复用这条含副作用的编译链。

上述路径均相对于 `internal/modules/`。这些是静态源码事实，不是运行证据。

## 3. 提议冻结的 no-Run 协议 v1

### 身份与持久状态

Workspace owner 在自身持久状态中维护协议版本、creation epoch、单调 preparation epoch、
准备 attempts、唯一首次 completionID，以及 fresh intent reservation。
这些状态随现有 repository round-trip 和 CAS 保全；Eventstream 或 Web 不得重建。

每个 reservation 绑定 Workspace、RequestID、intent kind、StartupInputHash 与完整规范化
typed payload 的身份。只对本来不含凭据的 typed bootstrap/payload 计算身份；不得把配置密钥
或密钥派生 hash 写入账本。scope 或 payload 不同即冲突，不能只比较 RequestID 或 StartupInputHash。

每个准备 attempt 绑定预期 owner revision、creation/preparation epoch、exact confirmation、
candidate/Definition/StartupInput、manifest/policy/compiler 版本与 owner observations。
M3 的 completionID 按 Workspace＋creation epoch 唯一，不能按 Definition 版本重复签发。
历史完成记录保留原绑定；新 Definition 不继承旧版本的当前完成状态。

### 两个 owner 的边界

| 接收 owner | typed 操作 | 允许的效果 |
| --- | --- | --- |
| Workspace | 接纳 fresh intent 的既有入口 | 私有 reducer/CAS 先登记 reservation、推进 epoch、失效当前 readiness，再 forward |
| Workspace | 准备 attempt 开始/提交/恢复 | 写本 owner 账本；final CAS 复核 exact identity、revision、epoch 与无 pending |
| Run | 首次激活只读观察 | 在 `workspaceExecutionMu` 同一顺序边界下读取完整历史、retirement 与 replacement/request 事实 |
| Run | 原请求结果只读查询 | 返回已持久化的精确结果或 unknown；不 activate、不恢复 replacement、不补发通知 |

具体 topic、Command/Result 和 lane 在各接收 owner 的 `pkg/events` 定义并逐项登记；
Workspace mutation 使用 Workspace state lane，Run 观察在 Run owner 内排序。
不新增运行单元，不传递 repository/client/锁或 callback，不持 owner 锁做跨 owner I/O。

### 接纳与重放顺序

1. 对支持的全部 fresh ingress，在任何 forward 前完成 reservation CAS；不能只处理 Web。
   既有 restart 同时承担首次激活与 replacement，两者都参加；Resume 不接受新 bootstrap，
   仍复用原 Run 输入，不变成 fresh intent。
2. 同键同完整 payload 先解析原登记/结果，不重复递增 epoch。已完成返回原记录；
   pending/unknown 返回原操作身份并保持受阻，不能以回读为由再次 forward。
   同键异 payload 始终冲突。
3. 未决 reservation 不被新 fresh intent 绕过；返回明确冲突/受阻，不静默排队或自动执行。
4. reservation CAS 后才允许最初一次 forward。forward 的错误、缺结果、错类型、丢回复或
   进程中断不证明 Run 拒绝；不能因此删除 reservation。
5. 结果结算必须精确核对原请求与 payload。Run 已持久化的接纳事实意味着该 Workspace
   已不再适用首次 no-Run M3；这不能被后续 Run 结束或普通列表隐藏所逆转。

### M3 提交顺序

1. 准备开始时封存 expected revision/epoch 与 exact seal；资源长检查不持 Workspace/Run 锁。
2. Run owner 的完整只读观察不得以 retired、查询错误、unknown、过滤或不完整库存证明空历史。
   有任何历史 Run、replacement 或未决请求均不能作为首次无历史正例。
3. 所有 required 检查满足后，Workspace final CAS 仍重核当前 seal、revision、epoch、
   observations 有效性及无 unresolved reservation。冲突不能覆盖或改用新身份静默提交。
4. final CAS 先成功：可保留历史 M3；随后 fresh intent 的 reservation 先使当前 readiness 失效。
   reservation 先成功：原 attempt 的 final CAS 必须失败/stale，不能签发 M3。
5. 以上只证明准备完成时的 no-Run 顺序，不保证外部资源此后不漂移，
   不表示 Start 已消费准备回执，也不授予执行权限。

### unknown、重启与迁移

- Run 查询返回空库存、找不到 RequestID、超时或 TTL 到期，均不能清除 pending；
  “未见接纳”不等于“以后不会接纳”。GET、服务重启和显式准备恢复均不重发执行。
- 只允许确定的、精确绑定的持久 owner 结果结算。若要在未接纳时释放 pending，
  必须另有 Run owner 持久拒绝/取消 fence，且所有迟到接纳路径都会核验；
  没有这样的证明就保持 unknown。本版不以新增自动取消或自动重试解决该情况。
- 原 reservation/attempt/M3 随服务 stop/start 恢复；进程相关观察按新 epoch 失效，
  不触发后台探测或收费调用。准备恢复不是 execution Restart。
- 首版 M3 正例仅支持协议启用后、创建时持久标识的 Workspace。
  legacy Workspace 缺此标识则阻塞；不凭创建时间、空库存或无 ActiveRun 自动升级。
  旧版本不明在途消息不能被迁移为“确定没有”。既有历史执行记录不被删除或重写。
- 回退停止签发新 M3，保留历史记录、reservation 与任何 fence；不删除协调状态后放行旧消息。

## 4. 实施后的必验断言

沿计划 A01–A20 逐项建立索引。协议的最低故障矩阵包括：

- reserve 与 final CAS 两种先后、并发 fresh intent、同键异 payload、多标签同键；
- reservation CAS 后/forward 前中断、forward 接纳后丢回复、owner 结果结算前中断；
- 重启后 pending 保留，unknown/空列表/缺请求/过期均不解锁，旧 forward 迟到；
- retirement、历史 terminal Run、replacement、legacy Workspace、epoch/revision 漂移；
- exact 原请求在 Run 已创建后仍能查询原登记，不因首次 M3 范围判断而丢失身份；
- 只读观察与恢复不产生 Run/runtime/推理/embedding/网络刷新/资源写入；
- STEP03 与既有 Resume/Restart 的受影响回归；不宣称新准备回执已被执行 gate 消费。

最终真实 HTTP/Web/服务 stop/start 在全新隔离、仅回环环境验证；不提交真实执行意图。
fixture 准备调用与 STEP04 增量分账，记录 build/source/binary 身份及独立 owner readback。
任何 required 生产事实未取得都保持阻塞，不能用 fixture 的 PASS 替代。

## 5. 原提案交付与后续批准

本次仅完成计划阅读、上述静态路径核对与合同候选；未修改生产代码、未运行项目测试/构建/CI、
未调用 provider、未改现有服务或数据。原计划文件保持不变。

原待确认问题：**是否批准按本文 §3 的精确 no-Run 协议 v1 修改既有 fresh intent 入口，
并按上述有界范围继续 STEP04 实现和无收费验证？**
若不批准入口变更，仅能交付准备子能力和时点观察，完整 M3 仍不能声明收口。

该问题现已获批准，不再作为待用户选择的阻塞项。后续必须逐项落实 §3 的持久
reservation/epoch、原请求结果只读对账、legacy 边界及 final CAS，不能只凭当前完整历史
观察替代整个协议。本文前述“本次”静态研究说明是原提案时点，不描述后续实施结果。
