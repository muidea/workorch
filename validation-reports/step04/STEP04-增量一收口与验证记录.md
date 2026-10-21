# STEP04 增量一：只读合同与纯边界

日期：2026-10-21（Asia/Shanghai）。基线 HEAD：`d93463f37983575723393578c15faff51cf1d844`。

后续补强、当前源码/二进制身份及剩余门槛见 `STEP04-增量一补强与剩余门槛.md`。
本文件的构建身份仅对应本轮原始快照，不代表后续修改后的源码树。

结论：**增量一的只读查询、清单、结构编译与持久投影基础已实现，并完成作者侧定向及累计验证；
尚无独立评审结论，STEP04 整体未完成，不能开展或宣称完整 M3/live 验收通过。**
代码未提交。用户原有 STEP03/STEP04 计划文件保持不变。

本记录按用户指定 STEP04 计划 §9 的三个增量组织。实施前合同审阅稿是此前的候选材料，
不是实现或验收事实；本增量没有先行修改其 §3 所列 fresh execution intent/no-Run 协议。

## 1. 本轮实现范围

| 计划项 | 实现及边界 |
| --- | --- |
| F01 | Workspace-owned `QueryExactConfirmedStartupSnapshot`：非零 expected revision，反查原 authoring/六审/确认/准入和实际正式 Goal/DoD，重新读 revision 排除混合观察；Application 有 concrete typed sender。缺失/漂移拒绝，不补 admission、不打开 ticket |
| F02 | `CompilePreparationManifest`：稳定检查 ID、required、来源、owner、对象、证据和 freshness 规则；128 项/256 KiB 上限；optional 资源的理由仅来自原确认规格。重新完整编译比较，不接受删 required、替换 owner 或自造 N/A |
| F04 | `CompilePreparedBootstrap`：detached typed 输入，绑定原 snapshot/manifest/Profile，使用原 candidate/source obligations；结构 Validate 与 bootstrap 字节身份，无 Hub/client/repository/callback、InitialFacts 或 Runtime initialize/drive。尚不构成资源/配置来源真实性证明 |
| F06 | Workspace 只读状态 query + `GET /workspaces/:workspaceId/activation-preparation`，生产 Application port 完整接线；无记录返回 `not_checked`，不隐式检查/编译/恢复。原操作状态与当前已知有效性独立 |
| 基线 inventory | 查实 `304d4b8d` 已加入目录准备完成 topic，但总量断言仍为 310；保留既有 311 项并登记本轮两个只读 topic，最终 delivery/message/生产发送接收审计共 313 项 |

F01 snapshot 保存实际正式 DoD 的原顺序及逐项来源，不按 canonical candidate 的排序代替正式顺序。
完整 snapshot 上限 2 MiB；编译 bootstrap 上限 4 MiB，超限拒绝，不截断后标成功。

准备记录在 Workspace 私有持久字段中，memory/file repository 都深拷贝并保留 CAS/reopen 身份。
首增量只有最多 64 条的操作 trace 形状和读取验证器，**没有创建/迁移准备记录的生产 command/reducer**；
四种操作状态 `accepted/blocked/failed/unknown` 当前只在测试中通过真实 repository 注入验证。
它不能表达正 readiness、M3 或激活权限。状态归属清单按当前只读 trace 分类；后续接入真实写入时必须登记
Workspace-local transition family，不能沿用 trace 分类来豁免 reducer 审查。

已知 revision、TTL、配置、确认/封存或检查/编译版本漂移只投影 `currentValidity=stale`，
不覆盖原 accepted/unknown、不删除原 request/payload 身份、不猜测旧操作已经结束。
`activeRunId` 是独立已有 Workspace 投影；其空值不是 no-Run/history/在途意图证明。

HTTP 响应为 lowerCamelCase 的窄管理 DTO，禁止缓存，scope/状态不合法则拒绝；owner 原始错误不进入公共响应。
未开放检查 POST、M3 签发、自动补齐或恢复执行入口。GET 不调用 F01/F02/F04。

按 go-multi-module-dev v2.6.0 沿用既有 Workspace/application owner 与 concrete typed EventHub/query lane/drain，
未增加运行单元或跨 owner repository/service 访问。未修改 SM-1/SM-2/SM-3/SM-4 执行转换或既有激活行为。

## 2. 作者实际执行与结果

本地受限证据根：`/tmp/workorch-step04-increment1-7neOJU`（临时证据，不纳入 Git）。
日志原样保留；报告不复制测试原始诊断或任何 live 配置。

### 累计 Go 回归

```bash
GOCACHE=/tmp/workorch-go-cache go test \
  ./internal/modules/kernel/workspace/... \
  ./internal/modules/application/workorchd/biz/appservice/activationcore \
  ./internal/modules/application/workorchd/biz \
  ./internal/modules/application/workorchd/service/server \
  ./internal/pkg/repository/... ./internal/pkg/events \
  ./internal/pkg/workorchmanagementcontract -count=1 -timeout=10m
```

最终执行 exit 0，17 个有测试包通过，另 2 个包无测试。包含原 STEP02/STEP03、确认/准入、
持久化、HTTP、owner 边界及 EventHub 审计回归；不是 `go test ./...` 或全 CI。
最终原始日志：`affected-go-tests-final.log`，SHA-256：
`aa5e72da44004f1e9129fa04f9edc141324972c973e3522872aa5fcb14c015cb`。

### 定向 race 与静态检查

```bash
GOCACHE=/tmp/workorch-go-cache go test -race \
  ./internal/modules/kernel/workspace/biz \
  ./internal/modules/kernel/workspace/pkg/models \
  ./internal/modules/kernel/workspace/pkg/startuppreparation \
  ./internal/modules/application/workorchd/biz \
  ./internal/modules/application/workorchd/service/server \
  ./internal/pkg/repository ./internal/pkg/workorchmanagementcontract \
  -run '^(TestPreparation|TestPreparedBootstrap|TestExactConfirmedStartup|TestExactStartupApplication|TestStartupPreparation|TestWorkspacePreparation)' \
  -count=1 -timeout=5m
```

7 个包通过，exit 0；`targeted-race.log` SHA-256：
`16bf552e9dc079a5547cf7b00843264523a70468c133a574b74558bc2d058b8f`。

- 上述累计范围的 `go vet`：exit 0，`vet.log` 为空。
- `make fmt-check`：exit 0，`fmt-final.log` 为空；`git diff --check` 通过。
- `GOCACHE=/tmp/workorch-go-cache make verify-boundary-contract`：exit 0。
  `boundary-r2.log` SHA-256：`6ff1d21f5d0d89fc056c459400eb2a79e29fbc3ec60b65b3c0e9440d86101819`。
  其中已有脚本打印两条 `__pycache__` binary-match 提示；末尾明确为 verification passed，不宣称无任何提示。
- 纯边界 AST 回归禁止新纯编译文件引入 I/O/Hub/adapter，禁止 InitialFacts/drive 等入口；
  它是来源边界回归，不宣称数学意义的传递纯性证明，另有输入保全/结果隔离/owner 零写行为断言。

### 真实 HTTP 的准确覆盖层级

`TestStartupPreparationHTTPFileOwnersReadOnlyAndReopen` 在真实临时回环 listener、生产 HTTP/Application ports
和真实 file-backed Workspace/File/Decision owners 下完成：

- 创建 fixture 后才取基数；两轮各 3 次 GET，期间关闭并重建 owners/listener；
- 每次读取保持原 revision、无记录、待检查，不生成准备记录；
- 每轮 storage 与资源目录全部文件内容 SHA-256 集合保持不变，provider-document 调用增量为 0；
- 独立 Run repository inventory 保持空；准备 GET 不提交任何执行意图。

这是 Go 集成测试里的真实 HTTP 与 owner/repository reopen，**不是独立生产二进制进程 stop/start，
也不是生产浏览器验收**。后两项留待增量三，不把截图、组件测试或该 reopen 冒充已完成。
其他累计用例包含原本的显式执行回归，不能将整套累计测试解释为全程没有任何测试 Run。

## 3. 源码与构建身份

构建前捕获源码/内嵌控制台身份，执行 `go build -o <证据根>/workorch ./application/workorch`，
再经原 `build_identity.produce` 重新读取完整输入并比较。该二进制未启动，未更改现有服务；
控制台源码/产物未改，本轮未执行新的 console build。

| 字段 | 值 |
| --- | --- |
| sourceCommit | `d93463f37983575723393578c15faff51cf1d844`，加本轮未提交源码；不是最终新提交 SHA |
| buildInputSha256 | `c8dbc64ba436473eb092ae1cfa7c44e0809627737d91a5b1465ec9ebe54d505c` |
| consoleTreeSha256 | `045e1a379991d29078f1bbab1752b882e2986cad8b770b4be2dc3f40df60960b` |
| binarySha256 | `9d6ec1ca04f8c26add5aba189656fe49bb235a784ba1b9c7fdc10ee54c984c03` |

`source-snapshot-r2.json`、`build-identity.json`、`build-identity-produce.log` 关联上述字节身份。
构建 exit 0；`build.log` 保留 Go 对只读 module stat cache 的非致命写入警告，不将其删去。
这是本地输入/产物字节关联，不是可信 attestation，也不是部署或运行验收。

现有 `build_identity.py verify` CLI 本轮两次 exit 2：第一次将包含 schemaVersion 的 manifest 当成外部身份，
输入形状不符；第二次提供正确四字段后，确认 `_read_expected_identity()` 返回带 schema 的已规范值，
而 `verify()` 再按四字段输入校验，发生重复规范化。原日志为 `build-identity-verify.log`、
`build-identity-verify-r2.log`。**本轮未修改该独立 CLI 缺陷**。

随后按公开 `build_identity.verify(root, console, binary, manifest, expectedFourFields, artifacts)` 函数合同，
使用本节固定四字段直接调用并完整重读校验，exit 0：`build-identity-function-verify.log`，SHA-256：
`e8b850bab94fdc95217c5d42d28aa1e1d6c97dda298ed91320d879186b45cdd0`。
不能将其表述为 CLI verify 通过。修复该通用 CLI 须单独跟踪，不混入本轮 STEP04 功能变更。

## 4. 首次失败与修正，不覆盖历史

- 初次累计回归遭遇默认 GOCACHE 只读、沙箱禁止 listener；后续指定 `/tmp` 缓存并在允许回环测试的环境重跑。
- EventHub 审计发现 F01 缺少 Application sender，已补 concrete typed sender 和精确错误回包测试，最终审计通过。
- 初次边界检查发现新增准备 trace 类型未登记；按当前无 writer/reducer 的真实只读 trace 分类登记，未虚构迁移实现。
- 定向编译测试曾因模板未声明 managed-validation owner fact 被正确拒绝；只在 fixture 封存前补声明，生产校验未放宽。
- 两处测试编译拼写错误（HTTP NotFound helper、嵌套 authoring ref 路径）已修正，最终累计/race 包含相应回归。

初次日志 `affected-go-tests.log`、`boundary.log` 和中间通过日志均保留，不将失败重标为通过。

## 5. A01–A20 验收索引与剩余项

| 编号 | 当前层级及未完成边界 |
| --- | --- |
| A01 | 定向/真实 file owner：错 scope/revision/封存/原六审、正式行漂移拒绝，无 fallback/ticket；通过本增量要求 |
| A02 | 纯清单 required/owner/来源/规则与防删改通过；真实 owner PASS/N/A 裁决未实现 |
| A03 | typed detached 结构编译、稳定/输入保全、源码纯边界通过；不代表资源 readiness |
| A04 | 真实 HTTP 重复读取/owner reopen、字节零写和零新调用通过；生产 Web/关页场景待增量三 |
| A05–A08 | F03 尚未实现：真实资源/权限/symlink、catalog 完整性/revision、cache-only freshness、Memory/MCP 等实际观察仍待实现 |
| A09 | 只有记录身份/有界验证及查询；真实检查接纳、同键重放和并发去重待 F05 |
| A10 | 已知 revision/config/seal/检查版本/TTL 的纯失效投影通过；owner 观察和进程 epoch 失效待后续 |
| A11 | file/memory 隔离、CAS 保全和 file owner reopen 通过；尚无 attempt/M3 写入切点及真实进程重启验收 |
| A12–A14 | §7 fresh intent reservation、epoch/pending、Run retirement-aware 排序观察、unknown/legacy 迁移均未实现，不能签发 M3 |
| A15 | F07 未实现，无 M3 正例或 completionID |
| A16 | 本增量准备读/纯编译链路不创建 Run/Runtime/semantic plan/dispatch；真实 HTTP 用例独立 inventory 为空 |
| A17 | GET DTO/no-store/scope/原错误不泄露/实际 ActiveRunID 分轴通过；POST/202、M3 及生产 Web 待后续 |
| A18 | 没有新增 Web 代码或浏览器存储；会话/revision 迟到响应及原操作恢复 UI 待增量三 |
| A19 | typed topic/inventory、query lane/drain、secret-canary 公共错误、大小/记录上限通过；F03/F05 deadline/观察预算待实现 |
| A20 | 受影响 STEP02/STEP03 累计回归通过；既有 Start/Restart/Resume 未改，未消费新准备回执，不宣称全链受保护 |

后续按原计划先进行**增量一独立复核**，不能用本作者记录代替；通过后进入增量二 F03/F05/F07/F08 和 §7 协议，
再做增量三 Web、production build HTTP/浏览器/真实服务 stop/start。不得跳过 required 条件、自动物化配置、
刷新外部 catalog、调用真实推理/embedding 或用 fixture PASS 替代生产 owner 事实。

本轮未运行全 CI、managed deployed、收费模型、生产 Web 或完整 STEP04 live。
未取得全阶段退出条件，仍不能标记 STEP04/M3 全部收口。
