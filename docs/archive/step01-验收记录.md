# STEP01 验收记录（历史归档）

Status: archived

Type: historical-acceptance-record

Last Verified: 2026-10-17

## 归档说明

本记录按用户明确要求归档，不属于当前设计规范，不新增验收标准或 Runtime 权威。
验收时为基于 `304d4b8d2735f0ad9a3b8fc9df14b797740cc07e` 的未提交候选；
对应代码随后提交为 `fa48b96eb855481642f81fa5e3a47e9c6bcfb4a3`。
下文“未提交或推送”描述验收当时状态，不表示归档时状态。
验证二进制绑定的是下文列明的候选构建输入，不能将其冒称为从后续提交重新构建的产物。
本次归档仅整理文档，未重新执行验收。

原记录来源：`/tmp/workorch-step01-m5uurybi/STEP01-验收记录.md`。
以下保留验收原文，仅将证据相对链接改为原受限证据目录的绝对路径。
原始日志、截图、配置、二进制与运行数据未提交到 Git；链接仅供原验收机器本地追溯，
不是仓库内的持久附件，可能随临时目录清理而失效。记录中的哈希不代替原始证据；
如证据已不可用，必须重新验收，不能仅凭本归档认定新的源码或产物通过。

---

# STEP01 验收记录

日期：2026-10-17。范围仅为《Workorch_渐进式收口与验收策略_第一步功能清单》的 Web 草稿创建、持久化、找回与有界恢复。

结论：**Web 草稿创建、持久化与找回闭环通过。**

这是下面精确源码树的未提交候选验证，不是 `304d4b8` 原提交通过；不表示 M3、Agent Loop、全业务 live、全库 CI 或发布通过。本轮未提交或推送修改。

## 1. 构建、配置与证据身份

| 项目 | 实际值 |
|---|---|
| 基准 HEAD | `304d4b8d2735f0ad9a3b8fc9df14b797740cc07e` |
| 候选构建输入 SHA256 | `44300116eb399de419694f5d0181729af9b51cec0b8ac9c3c736b4313d07ef97` |
| 嵌入控制台 SHA256 | `fa0a4da30e93d01fc9affb3eb4f12457e3bf1f93408baaf2df23f8a70f4e764a` |
| 二进制 SHA256 | `804e61d7c8c117e4dc4cc31c8ebfee9c5cbd0ec6705dd00a0642ba7d9a739c69` |
| 配置 SHA256 | `ee6fe407ef1c53a4443fba76ece7de58f896636c42d1c728f87266eef975598a` |
| 后端与数据根 | file；`/tmp/workorch-step01-m5uurybi/data` |
| 本机服务 | `http://127.0.0.1:39855`；无 API key、模型目标为隔离不可调用占位配置 |

新目录内实际执行 `npm run build` 和 `go build -buildvcs=true -o /tmp/workorch-step01-m5uurybi/workorch ./application/workorch`，每次启动核验 `/proc/<pid>/exe` 字节身份；验证结束再次核验源码、console 与二进制。旧 `--binary` 参数被拒绝。

原始证据：[构建命令](/tmp/workorch-step01-m5uurybi/build-commands.json)、[构建身份](/tmp/workorch-step01-m5uurybi/build-identity.json)、[候选身份](/tmp/workorch-step01-m5uurybi/candidate.json)、[tracked diff](/tmp/workorch-step01-m5uurybi/candidate.patch)、[console 日志](/tmp/workorch-step01-m5uurybi/console-build.log)、[Go 构建日志](/tmp/workorch-step01-m5uurybi/go-build.log)。`candidate.patch` 不是完整候选快照，未跟踪源码也由完整构建输入清单纳入身份。

HTTP 与浏览器共用同一二进制、配置和数据根。证据根为受限临时目录，不提交原始诊断、配置及运行数据至 Git。

## 2. 本轮修复与 F01–F10 对应

本轮修复部分 metadata 已持久化后 owner 的 InvalidArgument/NotFound 不能证明“尚未保存”的错误分类；HTTP 返回带原身份的 503，Web 普通 400/404 不释放原创建身份。只有明确的投递前拒绝标记才允许首次创建释放身份。另修复已打开 Web 的 SSE 阻止停机：停机仅取消只读 SSE 等待/回放，仍等待真实 reader、handler 与连接退出；不取消普通 mutation，不提前释放依赖。

| 编号 | 实现对应与证据 |
|---|---|
| F01 | `WorkspaceSetupModal.tsx` 独立纯保存；生产截图和实际请求日志证明只调用创建 |
| F02 | 创建 command 校验、目录 layout；HTTP 非法字段/模板/32769 字符/越界路径与原文顺序回读 |
| F03 | Workspace owner 创建身份；HTTP 同 requestId 重放/冲突，Web 未决身份保留回归 |
| F04 | 真实文件仓和独立 authoring GET；实际 record 与创建来源核验 |
| F05 | 服务端草稿详情明确 M1/draft、目录历史准备、不等于执行就绪；生产截图 |
| F06 | 真实刷新、关闭页再新建页，均经服务端列表打开同一草稿 |
| F07 | 真实进程退出/新 PID 启动，独立 GET 与生产页面均找回原身份 |
| F08 | 创建阶段类型、HTTP 恢复 DTO 与 Web 提示；保存前、metadata 后、目录阶段各自故障证据 |
| F09 | 原 requestId/payload 恢复，目录故障解除后实际完成；响应丢失 HTTP 与迟到/跨会话组件回归 |
| F10 | 本机来源拒绝、零执行/零用量、已有目录哨兵保全；未放宽鉴权 |

主要本轮文件：`biz/definition_authoring_creation.go`、`definition_authoring_http_integration_test.go`；`service/server/definition_authoring.go`、`http_response_helpers.go` 及测试；`internal/pkg/workorchmanagementcontract/definition_authoring_creation_recovery.go`；Web 的 `creationRecovery.ts`、`api.ts`、`WorkspaceSetupModal.tsx` 及 transport/authoring 测试；`scripts/build_identity.py`、`verify-workspace-draft-step01.py` 和构建拒绝测试。停机修复限于 server 的 `server_state*.go`、`http_types.go`、`http_events.go` 和 `http_shutdown_test.go`。路径均以仓库 `/home/fedquery/codespace/agentSuite/workorch` 为根。

go-multi-module-dev 生命周期约束用于保留“取消不等于完成”的真实排空门禁；未改变 SM-1/SM-2/SM-3/SM-4、owner 权威或 Agent Loop。此前 Embedding 展示改动保留在候选中，其测试纳入累计回归；本轮未扩展 Embedding 行为或进行模型 live。

## 3. A01–A12 逐项结果

PASS 按所列层级成立：真实生产 Web/HTTP/进程与确定性故障/组件规则分别标明，不把组件测试冒称浏览器网络故障注入。

| 用例 | 结果 | 实际操作与原始证据 |
|---|---|---|
| A01 正常保存 | PASS | CUA 操作嵌入生产控制台，仅“保存草稿”；[保存截图](/tmp/workorch-step01-m5uurybi/web-save.jpg)、[DOM](/tmp/workorch-step01-m5uurybi/web-save-dom.txt)、`browser-saved-ws-873610…json` 独立读回。HTTP 指定已有目录、哨兵字节保全亦通过 |
| A02 重复提交 | PASS | HTTP 同身份/同 payload 重放返回同一草稿、revision 不变；trace #1–3。重复点击/未决重放规则由 Web 组件覆盖，不声明生产浏览器双击注入 |
| A03 身份冲突 | PASS | trace #4 返回 409，#5 回读原草稿未改变；无替代草稿 |
| A04 非法输入 | PASS | trace #6–10：缺名称/目标、无效模板、32769 字符、越界路径，明确错误，无静默截断 |
| A05 刷新找回 | PASS | 真正 reload 清除页面内存，经列表打开；[刷新截图](/tmp/workorch-step01-m5uurybi/web-refresh.jpg)、[DOM](/tmp/workorch-step01-m5uurybi/web-refresh-dom.txt)，身份/原始目标一致 |
| A06 关闭重开 | PASS | 关闭原页面，新建浏览器 tab 重新打开列表及详情；[重开截图](/tmp/workorch-step01-m5uurybi/web-reopen.jpg)、[DOM](/tmp/workorch-step01-m5uurybi/web-reopen-dom.txt)。无浏览器持久业务存储由代码边界及组件测试共同证明；未宣称清空整个浏览器 profile |
| A07 真正重启 | PASS | 页面保持打开/SSE 在途，PID 1607642 正常退出，PID 1612585 启动同产物及配置，再 reload/list/详情；[重启截图](/tmp/workorch-step01-m5uurybi/web-restart.jpg)、[进程回执](/tmp/workorch-step01-m5uurybi/browser-processes.json)。独立 HTTP 另有 PID 1602698→1602736 |
| A08 持久化失败 | PASS | 真实 HTTP 在 metadata 写入前设置隔离存储障碍，返回 unknown/owner_persistence，不伪称已保存；解除后原请求恢复。额外 empty bindings 场景已保存 metadata 后返回 503 并保留原身份，同请求恢复。Web 标记传播及普通400/404身份保留由真实 api transport+组件测试覆盖 |
| A09 保存后目录失败 | PASS | 生产 Web 真实文件障碍导致503，显示已持久/准备待恢复；独立 GET 的 directoryPrepared=false。移开并保留测试障碍后点击“按原请求恢复草稿保存”，同ID/同requestId/同payload、revision=1且目录完成；[失败截图](/tmp/workorch-step01-m5uurybi/web-directory-failure.jpg)、[恢复截图](/tmp/workorch-step01-m5uurybi/web-directory-recovered.jpg)、[最终详情](/tmp/workorch-step01-m5uurybi/web-final-viewport.jpg) |
| A10 响应丢失 | PASS | 真实 HTTP 保存后丢弃成功 body，保留原 payload 重放；另经服务端列表发现、读取 replay、原身份恢复。页面在/页面关闭与未知响应不造新身份的交互规则由组件测试覆盖；不声明对生产浏览器注入了响应丢包 |
| A11 迟到与读写隔离 | PASS | Web 组件控制旧请求晚返回/关闭重开/跨 scope 的调度，验证 fresh 会话不被覆盖；HTTP及浏览器反复独立 GET，正常草稿回读逐值一致，未产生额外 mutation。不是浏览器网络延迟注入 |
| A12 访问与零执行 | PASS | trace #26–28 非法来源403，#29列表未增加，#30 runs=[]；本机边界不放宽。浏览器日志只有3个 POST，均为 workspace-authoring；所有草稿无派生/规格/确认记录，持久事件中LLM用量=0；哨兵未改 |

HTTP 全量请求/响应 bytes、字节数和哈希见 [http-trace.json](/tmp/workorch-step01-m5uurybi/http-trace.json) 及 `http-*-request.bin` / `http-*-response.bin`；[HTTP总结](/tmp/workorch-step01-m5uurybi/result.json)、[Web独立核验总结](/tmp/workorch-step01-m5uurybi/web-result.json)、[核验脚本](/tmp/workorch-step01-m5uurybi/finalize-evidence.py)。HTTP trace 只记录 HTTP 验收器；浏览器请求另见两个 browser-process 日志，未混成一个网络抓包。

[重启后截图便览](/tmp/workorch-step01-m5uurybi/web-restart-summary.jpg) 仅裁去全页截图下方无关列表；原始完整截图 `web-restart.jpg` 保留不变。

## 4. 草稿身份与重启核对

正常生产 Web 草稿：

- Workspace：`ws-873610a161ace671b9429f4b2b8bcbef7784bba81bc25f6d3b9f61579f529087`
- requestId：`34619b10-8216-40fe-a041-5950a1e89f2c`
- inputHash：`definition-authoring-sha256:517a3b2e5f538b8f49289a7e822f2647f4c591c3b20964514c5bc4752f5cc196`
- 原目标保留两行；saved/prereboot/partial/final 四次独立回读逐值相等，均 draft、revision=1。

目录故障恢复草稿：`ws-ec51e2334650033831502197002238c8d610b5c1682c52dad34c5999d37c13d3`；requestId `fb650aec-e46b-4af1-9a48-d55fa4d882cb`。partial/final 原输入、inputHash、replay、revision均相同，仅真实目录准备由false变true。最终列表八份（六份HTTP场景+两份Web场景），两个Web身份各仅一份。

原始 owner record 位于 `data/internal/workspaces/<id>/workspace.json`。独立 GET 位于 `browser-<阶段>-<id>.json`；列表和Run回读同前缀。两个浏览器验收进程停机耗时分别约0.016秒，exitCode均0，未强杀、未放宽预算。

## 5. 累计回归与限制

- 受影响 workorchd / workspace / managementcontract Go 包累计回归通过：[日志](/tmp/workorch-step01-m5uurybi/workorch-step01-affected-tests-final.log)。
- HTTP停机/stream/handler门禁 race 回归通过：[日志](/tmp/workorch-step01-m5uurybi/workorch-step01-shutdown-race-final.log)；覆盖回放取消后 reader 仍未退出时继续超时并保留依赖，再真实退出后幂等重试成功。
- 相关 go vet、boundary contract 通过：[vet](/tmp/workorch-step01-m5uurybi/workorch-step01-vet-final.log)、[boundary](/tmp/workorch-step01-m5uurybi/workorch-step01-boundary-final.log)。boundary存在扫描 Python pycache 的告警，非失败。
- 前端规则54项、组件101项通过：[规则日志](/tmp/workorch-step01-m5uurybi/workorch-step01-web-unit-final.log)、[组件日志](/tmp/workorch-step01-m5uurybi/workorch-step01-web-components-final.log)；组件既有 useForm 提示不是失败，不代替以上生产证据。
- controlled-build拒绝测试5项、build identity测试16项通过：[5项](/tmp/workorch-step01-m5uurybi/workorch-step01-verifier-tests-final.log)、[16项](/tmp/workorch-step01-m5uurybi/workorch-step01-build-identity-tests-final.log)。`git diff --check`通过。
- 初次新增回放测试误用了不存在的view=audit，未进入reader；已改为既有view=full并重跑上述最终回归。旧失败日志保留于 `/tmp/workorch-step01-affected-tests.log` 和 `/tmp/workorch-step01-shutdown-race.log`，不冒称PASS。
- 较早候选 `/tmp/workorch-step01-syr0n88w` 的Web停机失败被保留；不纳入本次新源码的PASS证据。当前只采信本记录的构建身份。浏览器瞬态toast定位超时改用实际持久M1状态核验；目录恢复后通过reload加载列表，不以旧列表缓存证明新草稿已发现。
- Chrome扩展自动导航曾被跨站防护拒绝；用户打开的可用页面实际在应用内浏览器，后续CUA验收均在该生产页面完成，未绕过或更改鉴权。
- 未运行全库CI或整个业务fullflow，未取得SG3/发布结论；这些仍是独立后续要求。允许另行界定下一切片，不自动进入编译/审查/确认/执行。
- 本轮隔离服务已正常停止，验收数据和障碍原始字节全部保留；未操作用户8090服务或live配置，未删除用户数据。
