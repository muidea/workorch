# STEP04 生产 fresh 入口接线与验证记录

日期：2026-10-21（Asia/Shanghai）。源码基线：`834aa350cc4e77379819666540b9912a7c22b64c` 加现有未提交工作树。
本轮未 commit/push；此前子增量及用户修改保留，不把 HEAD 当作已测完整源码。

## 结论与授权边界

已将批准的 no-Run 协议 v1 的 reservation-before-forward、原结果结算和编译前恢复接入生产 execution ingress。
用户对旧 Workspace 兼容边界回复“当前阶段不用考虑历史兼容性”：缺 creation fence 明确阻塞新 Restart，
不补标记、不自动迁移；Resume 保持既有原输入 continuation 语义，但禁止复用已登记 fresh RequestID。

这只是 STEP04 的入口子增量，不是全部第四阶段完成：没有首次 M3 completion/final CAS、完整资源 readiness、
准备写入口或准备 Web；未进入 STEP05，不声称 Start/Restart 已消费准备回执。
没有真实 provider/embedding/外部 catalog 调用，没有读取 live 配置或改动既有服务/业务数据。

使用 go-multi-module-dev v2.6.0：沿用 Workspace/Run owner 与 concrete typed EventHub。
Workspace 私有 reducer/CAS 管理协调状态；Run 继续独占 SM-1 lifecycle/持久原回执。
本轮不新增 SM-2/SM-3/SM-4 authority 或运行单元，不在 owner 锁内执行跨 owner I/O。

## 落地合同

1. 新 Restart 校验完整 bootstrap、exact admission 后，先封存 reservation/epoch，后转发 Run。
   只有本次 reservation CAS 明确成功返回的首次调用能转发；CAS loser、重复、写后丢回复、重启均无第二次许可。
   reservation-before-forward 中断保留 unknown，不能用查询空结果放行。
2. 原身份优先于当前 Goal/DoD/admission；同键异完整 typed payload 冲突。
   已结算历史先于后继 pending，完整原结果零写重放；其他新请求遇 pending 不转发、不猜原 bootstrap。
3. reservation 同时封存 Workspace-domain `PayloadHash` 和独立 `RunPayloadHash`，二者不能互换。
   settlement 持久校验 reservation 的 Run hash 与 receipt 完全相同，不将 hash 作为成功凭据。
   正常 Run 响应之后仍须独立查询精确持久原 receipt，才可 Workspace 私有结算。
4. 新 typed `/workspace/command/recover-execution-intent` 在当前 bootstrap 编译前使用。
   它是显式 execution POST 内的 owner command，不是 GET；只接收公开执行语义身份 Workspace/request/restart。
   原完整 payload 身份从 owner 不可变登记读取，不由调用者传 hash/成功标志，也不重建旧 bootstrap。
   无登记不写、不 forward、不授转发许可，后续完整 ingress 仍重新校验并 reserve。
   public reason/source 仍是审计元数据，不改变已登记执行 payload。
5. `fresh_intent_unresolved/fresh_intent_result_unknown` 以 `409/accepted=false/state=pending` 原样传递。
   `requestId` 保持本次调用，三个 `pending*` 字段绑定阻塞原身份；禁止断言 RunID 或混入 resolved outcome。
   HTTP、local/remote backend、Web parser 保持同一分类；Web 只提示保留原身份，不误报拒绝、接受、没有 Run 或零转发。
6. EventHub inventory 增加上述一个 Workspace-owned state-lane command：316→317，有 concrete message、
   owner receiver、Application sender、README 和完整性测试依据；不是仅修改计数。
   `state-transition-owners.manifest` 指向唯一实际 settlement dispatcher/reducer。

## 验证与适用范围

新增生产 ingress / Application / transport / Web 定向覆盖：

- 32 并发同键：转发前读到已持久 reservation，只一次 forward/epoch；同键异 payload 和跨 kind 冲突。
- reservation、settlement 的 CAS 前失败/写后丢回复；reserve 后 forward 前中断；Run 接纳后丢回复；
  query 缺结果、错类型和 owner 错误；pending 不清、不重发，显式恢复原结果。
- 当前 startup/admission 漂移后原请求恢复；新请求 pending 阻塞；旧 settled 不被后继 pending 遮盖；
  foreign source/scope、取消、非法 typed input 拒绝且状态不变。
- 真实 Run owner 接纳后丢回复，再经 Workspace recovery 消费持久原回执；恢复前后 Run 全量对象相等。
  Run 是测试显式创建的内存 fixture，不表示测试全程没有 Run。
- 文件 owner 两个独立子进程冷读 pending/settled；增加公开 preflight recovery 层断言，原 revision/磁盘 bytes 不变。
  这是子进程冷重载，不是生产服务 stop/start 验收。
- 缺 creation fence 的新 Restart 阻塞，Resume 正常且不补标记；已有 owner test fixture 改为创建时显式登记新标识，
  不是 production CAS 升级旧数据。
- HTTP mux 的 pending 分类；local/EventHub 与 remote/真实回环 HTTP 结果逐字段相等；
  Web parser 严格身份/分类与实际 Board 挂载提示，无自动重试。

受限原始证据根：`/tmp/workorch-step04-ingress-5sE19M`。不提交原始 fixture、日志或二进制。

| 验证 | 结果 | 原始证据 |
| --- | --- | --- |
| 受影响 Go 包完整回归 | 29 个测试包通过，5 个包无测试 | `affected-go-test.log` |
| 五包定向 race | 全通过 | `focused-race.log` |
| 最终冷进程/入口补充 race | 通过 | `cold-ingress-race-final.log` |
| 保留 Resume 原 runtime 就绪 guard 后的 gateway race/vet | 通过 | `gateway-race-final.log`、`gateway-vet-final.log` |
| Web 规则 | 11 个测试文件通过；不把默认非 verbose 文件计数写成逐条用例数 | `web-rules-final.log` |
| Web 组件 | 12 个文件、129 项通过，含新增 Board pending 挂载 | `web-components-final.log` |
| TypeScript/Vite 生产构建 | 通过，保留既有 chunk-size warning | `web-build-final.log` |
| vet（Workspace/Application/server/runtime contract/backend） | 通过 | `vet.log` |
| fmt-check / boundary | 通过 | `fmt-check-final.log`、`boundary-final.log` |
| 最终 Go build（未运行产物） | exit 0；保留 module stat-cache 只读 warning | `build-release.log`、`workorch-release` |
| 构建前 capture / 构建后 produce | 输入一致，成功 | `source-snapshot-release.json`、`build-identity-release.json`、`identity-produce-release.log` |

完整 Go 回归覆盖 Workspace、repository/workorchrepo、workorchd biz/server、Run、LLM、events、Runtime/admission contract、
local/remote backend 和 process Web contract。首次沙箱执行的既有 httptest 被拒绝监听；随后在授权隔离回环环境完整复验通过，
不记首次失败为通过。初次发现的旧创建 fixture 缺标识和 event package 跨 owner import 已修正并由最终回归覆盖。
最后的测试源码补充（冷进程公开 recovery 断言）在完整回归之后，已单独 race 复验。
随后保留 Resume 原 runtime 就绪 guard（仅 Restart 原请求对账可先于当前编译），由 gateway 定向 race/vet 复验；
完整包回归与最终源码的这项差异显式分账，最终二进制已重新构建，未将较早 build-final 身份用于最终源码。
误传 `--help` 的一次组件命令仅输出帮助，不列为组件通过证据，正式 `web-components-final.log` 才是执行结果。

最终构建身份：

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`（加未提交源码） |
| buildInputSha256 | `fe3e0e09bf50a935190f413194259d2f9f2e38ab438e8ca7ed423893e03d4226` |
| consoleTreeSha256 | `dac115ba997bdb66fa357a498b9a55515b33a3f8e37dc6083c47816d45fa1c4c` |
| binarySha256 | `93542f68af1e6b8aad9215b722a9a6e1a1419166f429276ee67836074a4993be` |

这是本地构建输入/产物字节身份，不是可信 attestation、部署来源或验收通过证明；不声称 verify CLI、全 CI、
完整 Runtime fullflow 或收费 live 通过。生成的嵌入 Web 产物随本轮源码保留。

## 剩余 STEP04 门槛

1. F03：其余配置/目录、Soul/Skill/MCP/Memory 真实 owner 观察、完整 freshness 和准确阻塞。
2. F05/F08：显式准备检查接纳、精确幂等、有界 attempt/private reducer/CAS、原检查恢复。
3. F07：唯一首次 M3、完整 observation/final CAS；reserve/final CAS 两种先后及 retirement/unknown/迟到消息完整矩阵。
4. 增量三：准备 Web 分轴展示与迟到响应 fence、生产 HTTP/浏览器和真实服务进程 stop/start。
5. 原独立 CI 阻塞（integration 默认 Profile 断言、Orchestrator 套件累计超时）未在本轮修复或宣称通过。

STEP05 的准备回执消费、完整执行授权与 Runtime fullflow 仍不在本轮范围。
