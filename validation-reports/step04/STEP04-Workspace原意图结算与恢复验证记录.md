# STEP04 Workspace 原意图结算与恢复验证记录

日期：2026-10-21（Asia/Shanghai）。范围：已批准的 no-Run 协议 v1 的 Workspace 私有结算子增量。
源码基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加现有未提交工作树；未提交或推送本轮修改。

## 结论及边界

已补齐原 reservation → 独立 Run 原回执查询 → Workspace 私有结算/CAS → 不可变历史重放。
这不是生产 fresh intent 入口接线完成：`executeExecutionIntent` 仍未使用这些私有事务，
不能宣称生产路径已经 reserve-before-forward、未知结果不重发，或已具备 readiness/M3。
初始工作目标包含入口接线；本轮实际先完成它所需的多请求历史、结算及恢复基础，不把初始目标记为已实现。

使用 go-multi-module-dev v2.6.0：保留原 Workspace/Run owner 和 concrete typed EventHub，
不新增运行单元，不注入跨 owner repository/service，不在 Workspace 锁内做 Run 查询。
Workspace 协调账本是本轮写入范围；SM-1 只被读取持久原回执，SM-2/SM-3/SM-4 无迁移或权限变更。
没有 STEP05、真实模型、embedding、外部 catalog 刷新、现有服务或 live 配置操作。

## 已落地内容

1. `WorkspacePreparationFence` 保留 creation identity，增加最多 128 条不可变 `Settled` 历史。
   reservation 推进 epoch；结算不推进 epoch，而是原子保留原 reservation 与完整 Run 原回执。
   历史和 pending 的数量必须精确对应 epoch，request 不得重复或同时出现在两个集合。
   满容量仍可读取和重放原请求，新登记明确阻塞；不淘汰旧历史。
2. persistence gate 只允许未决项登记或该未决项的精确追加结算；拒绝跳过 reservation、
   删除/替换/重开历史、同次结算并登记新请求、修改 creation identity 及 ordinary Update 结算。
   原 legacy 边界保持，不补造 creation epoch。
3. `workspaceFreshIntentSettled` 私有事实逐项反查原完整 typed Workspace payload 与独立 Run payload hash。
   两个 hash 的 domain 不同，不能互换。只消费原接纳回执，不把 replay/continuation 响应视为原证明。
   已结算时精确比较原回执并零写返回，changed original 冲突。新事务登记仍与结算分开 CAS。
4. `reconcileWorkspaceFreshIntent` 先解析原身份，再读 Run owner；不依赖当前 Goal/DoD 或 eligible 投影。
   已结算直接返回 detached 原值；未知、错误、缺响应或错类型不清除 pending、不 forward。
   query 之后重新读取 Workspace 并通过私有 reducer/CAS 核对，最多四次 CAS conflict 重读；
   Run query 不自动重试。写后丢回复可以读取原结算，不授第二个转发许可。
5. `workspaceFreshIntentReserved` 优先识别已结算原身份；后继请求 pending 时仍能识别旧完成请求。
   同 request 异完整 payload 冲突，历史容量耗尽不产生新登记或淘汰。
6. `state-transition-owners.manifest` 登记唯一 `workspace.fresh_intent_settlement`，
   Workspace README 更新为当前实际实现，不将私有 helper 描述为生产入口已接通。

## 验证覆盖

新增 12 个测试函数（包含一个由父测试在两个独立进程调用的 helper）：

- Workspace biz：24 并发只结算一次；admission 漂移后对账；已完成原身份重放；下一请求独立预留；
  三种合法原接纳原因；满容量；17 类未知/漂移/错来源/取消反例；CAS 前失败及写后丢回复；
  unknown、owner 错误和错类型零写/零 forward；真实 Run owner EventHub 配对。
- 模型：raw 结构/epoch/重复身份/receipt 校验、clone 隔离、128 条容量和 append-only 迁移矩阵。
- memory/file/scoped repositories：Get/List/调用者副本隔离；普通 Update、CAS 对历史改写的拒绝；
  metadata 更新保全；7 类 raw 损坏/未知字段在 Get/List 拒绝且文件字节不变。
- 文件 owner 两个独立子进程：分别冷读结算前失败的 pending 和写后丢回复的 settled，
  核对原 request/receipt、revision 及磁盘原字节不变，均不能获得第二次转发许可。
  这是真实子进程冷重载，不是同路径第二句柄；但不是生产服务 stop/start 验收。

真实 Run owner 配对中的 Run 由测试显式准备，使用内存 repository 和固定 bootstrap；
对账前后 Run 全量对象相等。它证明本消费者没有新增 Run/生命周期写入，不表示整个测试从未创建 fixture Run。
纯 reducer/容量/损坏反例使用 typed fixture，不能替代真实 owner 证明。

## 执行记录

受限本地证据根：`/tmp/workorch-step04-settlement-0FShK9`。原始日志、测试二进制和 fixture 不提交 Git。

| 检查 | 结果 | 原始证据 |
| --- | --- | --- |
| 受影响包完整 Go 回归 | 首轮及最终树均为 24 个测试包通过、5 个包无测试 | `affected-go-test.log`、`affected-go-test-final.log` |
| 最终新增 repository 损坏反例 | 通过 | `repository-final.log` |
| 最终定向 race：Workspace biz/models、repository | 3 个包通过，包含两个冷进程切点 | `focused-race-final.log` |
| vet：Workspace/repository/workorchrepo | 通过 | `vet.log` |
| fmt-check、git diff --check | 通过 | `fmt-check.log`、`diff-check.log` |
| boundary contract | 首轮及最终树均通过 | `boundary.log`、`boundary-final.log` |
| 最终源码 go build | 成功，未运行产物 | `build-final.log`、`workorch-final` |
| 构建前 capture、构建后 produce | 成功，输入一致 | `source-snapshot-final.json`、`build-identity-final.json`、`identity-produce.log` |

完整包回归命令：

```sh
GOCACHE=/tmp/workorch-go-cache go test \
  ./internal/modules/kernel/workspace/... \
  ./internal/pkg/repository/... ./internal/pkg/workorchrepo/... \
  ./internal/modules/application/workorchd/biz \
  ./internal/modules/blocks/run/... ./internal/modules/blocks/llm/... \
  ./internal/pkg/events -count=1
```

上述回归中的既有 HTTP fixture 仅使用隔离回环监听；没有真实 provider 或收费调用。
构建日志保留 Go module stat-cache 只读 warning，命令 exit 0；未为隐藏 warning 修改权限。
boundary 日志保留既有 `__pycache__` grep 提示，不将提示重写为测试失败或静默删除。

最终构建字节身份：

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`（加未提交源码；不以 HEAD 替代工作树） |
| buildInputSha256 | `26e64a1b840bd71a5c93fadcc617037adf343f4c1356e7218ea96b97d39c55a0` |
| consoleTreeSha256 | `045e1a379991d29078f1bbab1752b882e2986cad8b770b4be2dc3f40df60960b` |
| binarySha256 | `7b59e676c8721674f7256619da239510e8d83974f6d828117249e092c9a0e586` |

该身份仅说明本地源码/控制台/二进制字节及构建前后输入一致，不是可信构建 attestation、部署来源或验收通过证明。
没有运行或声称 `build_identity.py verify` CLI 通过；没有全仓 test、全 CI、部署链或收费 live 通过结论。

## 仍待逐项完成

1. 所有生产 fresh intent ingress 的 reservation-before-forward、唯一最初转发和精确结算接线；
   同 pending 的 finite typed 对外结果、跨 kind/异 payload 冲突及 legacy 兼容边界；Resume 保持原义。
   **当前生产 execution ingress 未变，不能用本报告替它作安全保证。**
2. F03 其余配置/目录、Soul/Skill/MCP/Memory 的真实 owner 观察，以及 cache-only freshness 与准确阻塞。
3. F05/F08 显式检查受理、完整幂等、attempt/reducer/CAS 和原检查操作恢复。
4. F07 首次 M3、唯一 completion、完整 observations 的 final CAS；reservation/final CAS 两种并发顺序、
   legacy、retirement、unknown、迟到消息及重启矩阵。当前没有签发 M3。
5. 增量三 Web 分轴和迟到响应 fence、生产构建 HTTP/浏览器以及真实服务进程 stop/start 证据。
6. 既有 integration 默认 Profile 断言和 Orchestrator 包级累计超时仍单列 CI 验证阻塞，
   本轮未重跑或修复它们，不由上述局部通过推断全 CI 通过。

STEP05 的 Start/Restart 准备回执消费、完整执行授权及 Runtime fullflow 不在本轮收口范围。
