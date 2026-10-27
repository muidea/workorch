# STEP04 Run 原响应回执与只读对账验证记录

日期：2026-10-21（Asia/Shanghai）。源码基线：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮及此前未提交修改。
本文为实施记录，不是设计权威、固定提交的独立评审或完整 STEP04 验收结论。

## 本轮结论

已实现已批准 no-Run 协议 v1 所需的 Run 原响应持久回执、独立只读查询及 Workspace typed sender，
并通过受影响累计、定向 race、静态门禁与隔离构建。**Workspace reservation 结算和生产 fresh ingress
尚未接通；没有签发 readiness/M3，不能宣布完整 STEP04 通过或已可进行完整第四步 live 验证。**

前两批的只读事实、creation fence/private reservation 的原记录继续保留；不得把其旧哈希当成本轮源码身份。

## 已落地内容与边界

1. Run-private `executionReceiptObserved` reducer/dispatcher 在首次真实新 Run activation 或 replacement
   接纳后，CAS 追加版本化原响应；Workspace、原 RequestID、完整规范化 typed payload hash、记录时间和 admission
   不可改写。普通 Update 不得增加/抹除，CAS 保持 immutable prefix，每 Run 上限 128，不淘汰历史。
2. 原响应不仅做字段格式检查：reducer 反查实际 immutable acceptance 来源；原始持久化读取也反查
   activation 或当前/retired replacement 来源并重算完整 payload 身份。跨 Run 重复 RequestID、错来源或漂移拒绝。
   完整 command JSON 与来源 identity compiler 有独立一致性断言，避免未来新增字段漏绑。
3. `/run/workspace-execution/receipt/query` 仅接受 exact `blocks/workspace`，使用 concrete Command/Result、
   query lane 和 `workspaceExecutionMu` 排序；在 repository 同一锁内读取完整库存、generation head 与 retirement。
   terminal/retired 不被普通可见列表隐藏；返回 detached 原值。
4. 合法结果中的 `Receipt=nil` 明确表示 unknown。缺能力、损坏、不完整、错 hash/scope/request、owner 错误、
   超时/取消或错类型不改写为“确定未接纳”。不调用 Restart replay、不 activate、不通知 continuation、不调度
   replacement、不写回、不根据当前生命周期补造旧响应。
5. Workspace sender 保留原身份和父 context，使用最多两秒的只读预算，校验响应并隔离返回值；不重试、不清除或
   结算 pending、不授予第二次 forward。真实 Workspace sender→Run receiver 配对测试已通过；此 sender 尚未接入
   production execution ingress 的原操作恢复分支。
6. 已知非法 fresh 身份、完整 reader 不可用或原库存读取失败，在首次新增 effect 前拒绝。既有 legacy/replay/
   governance 分支不回填回执，显式 Restart replay 原 continuation/replacement 语义保留，不能拿来替代只读查询。
7. 接纳与回执是先后两个 owner CAS，不是跨文件原子事务。接纳完成、回执 CAS 前失败时保持 unknown；回执已写而
   回复丢失时可只读找回原响应。错误不能据此解释为“Run 未创建”或释放 Workspace pending。

影响 SM：SM-1 owner 内增加原响应历史事实持久化；不选择新的 Run 生命周期目标，不修改 SM-2/3/4 authority、
dispatch、reducer、预算或交付。Run opaque Runtime CAS 仍保全全部非 opaque 字段。没有新增运行单元或跨 owner
repository/service；EventHub inventory 的本轮新增 topic 和私有 dispatcher 已逐项登记并通过边界检查。

## 定向覆盖

- `run/biz/execution_intent_receipt_test.go`：首次原响应、CAS 前失败/写后丢回复、独立子进程冷重载、原 bytes
  不变、unknown、不从显式 replay 回填、错身份/缺 reader/取消、fresh 顺序、drain、retired、返回隔离、完整输入
  身份、source reducer 反例及已知拒绝零 effect。子进程覆盖三个切点，各自 Configure/Start/query/Stop，
  但不是 HTTP 服务进程验收或完整 Runtime drive。
- `run/biz/workspace_execution_intent_test.go`：补充 terminal predecessor replacement 完成后仍读取原 predecessor
  admission，successor 不产生第二份回执，读取前后库存完全不变。既有九个 replacement SIGKILL/重开切点保留通过。
- Run models/events：canonical identity、immutable prefix、clone、容量、跨 Run 歧义、raw source/payload 反证及 nil unknown。
- `repository/run_execution_intent_receipt_test.go`：memory/file/scoped 三 backend 的 CAS/Update、保全与重开、读取隔离、
  metadata 保全、删除后 unknown 但历史 watermark 保留、取消/scope，以及冷物理路径的原始文件损坏拒绝且不修复。
  同进程新建 file repository 句柄会复用路径缓存，故不将该动作冒称磁盘重载；冷路径和独立子进程分别覆盖原始解码。
- `workspace/biz/preparation_execution_receipt_test.go`：真实 Run owner 配对、零自动转发/创建、原结果、错误/错类型/
  漂移/取消、父 scope、预算和零重试。测试显式制造的 Run 接纳仅为 fixture，不是准备接口启动执行。

## 本轮执行证据

受限原始证据目录：`/tmp/workorch-step04-receipt-wduRlw`（本机临时证据，不随 Git 提交，可能被系统清理）。
测试均使用隔离、非敏感 fixture；未读取 live 配置、未使用真实模型或 embedding、未改现有服务和运行数据。

| 检查 | 结果 | 原始索引 |
| --- | --- | --- |
| Workspace 全、repository 全、scoped repository、Application biz、Run 全、LLM 全、events 累计 Go 回归，`-count=1` | 24 测试包通过，5 包无测试 | `affected-go-test-complete.log` |
| Run biz/events/models、repository、Workspace biz 定向 `-race` | 5 包通过 | `focused-race-complete.log` |
| Orchestrator 既有 startup/bootstrap/raw-boundary/notification 定向兼容测试 | 5 个顶层测试通过（含子例）；不是全包或 fullflow | `orchestrator-startup-compatibility.log` |
| 受影响路径 `go vet` | 通过 | `vet-complete.log` |
| `make fmt-check`、`git diff --check` | 通过 | `fmt-complete.log`、`diff-check-complete.log` |
| `make verify-boundary-contract` | 通过，包含 owner manifest/typed closure | `boundary-complete.log` |
| 隔离 `go build ./application/workorch` | 通过，未启动二进制 | `build-complete.log`、`workorch-complete` |
| build identity capture/produce | 成功；构建前后输入一致 | `source-snapshot-complete.json`、`build-identity-complete.json` |

最初累计运行中的 HTTP fixture 受 sandbox 监听限制；经本机回环测试授权后按同一包集合复验。
中间实现曾暴露 wrapper 缺失可选 reader、dispatcher 签名/typed sender 不完整和测试 fixture 问题，已修复并保留末次
完整成功日志；不把中间失败写成最终通过。原始日志中的 module stat-cache 只读警告及边界脚本 pycache 提示保留，
相应最终命令均返回 0，未通过扩大写权限或改写相关工具来隐藏它们。

末次构建身份：

- `sourceCommit`: `834aa350cc4e77379819666540b9912a7c22b64c`，含当前未提交源码，不表示仅构建该固定 SHA。
- `buildInputSha256`: `7c4a35900d0f8115ee19d903f9bc318625ce888e4e4259a6c4146c7df5c2155c`
- `consoleTreeSha256`: `045e1a379991d29078f1bbab1752b882e2986cad8b770b4be2dc3f40df60960b`
- `binarySha256`: `c61369fb4554ae61460a07f378bad153f5b2c85d5e3dc4fe7775ecf430afc442`

构建输入按工具声明的范围计算，源码/控制台/二进制内容身份不等于可信构建 attestation、部署事实或验收通过。
未运行或声称 `build_identity.py verify` CLI 通过；不以旧 HEAD 替代 dirty tree 身份。

## 仍需继续收口

1. no-Run 协议生产主线：Workspace reservation 的持久结算/历史保全，所有 fresh ingress 的 reserve-before-forward，
   同键先读取原身份、unknown 保留且零重发；不借用 legacy replay 或空 Run 列表清除 pending。
2. F03 其余真实资源观察：配置/目录完整性、Soul/Skill/MCP/Memory 的实际 owner 事实、cache-only freshness 和准确阻塞。
   已有 LLM cache-only 与 Run complete-history 子能力不能替代这些要求。
3. F05/F08：显式检查接纳、精确幂等、有界 attempts、Workspace 私有 reducer/CAS、结果账本与原操作恢复。
4. F07：legacy 边界、历史首次 M3、exact observations、revision/creation/preparation epoch/pending final CAS，
   以及与 fresh reservation 两种顺序的完整并发/崩溃矩阵。
5. 增量三：Web 分轴状态、显式检查/恢复和迟到响应 fence；生产构建真实 HTTP、浏览器和服务 stop/start 验收。
6. 整仓 CI 的两个已披露阻塞（公开 fullflow Profile 断言、Orchestrator 累计包超时）未在本轮修复或证明消失；
   未跑全仓 CI，不能宣布全 CI 或部署链通过。完整 Start/Restart 准备回执消费与 STEP05 仍不在本轮范围。

本轮未 commit/push；前序未提交改动与验证记录均保留。
