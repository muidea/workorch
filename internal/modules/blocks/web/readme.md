# Web 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-21

## 功能范围与边界

`blocks/web` 是 `web/search` 与 `web/fetch` 的 capability owner。它负责 provider 选择、URL/网络策略、DNS 与
redirect preflight、有界 transport、内容规范化和 factual receipt；不拥有 Runtime action、CWI credit、重试、治理
继续或 terminal 裁决。

Search 只向已配置的同 origin HTTP provider 提交有界 POST。Fetch 不再委托 gateway 先访问目标再回报 final URL，而是由
Web owner 直接执行 GET：每个 hostname 只解析一次，解析地址同时用于策略判定和实际 dial，禁用 proxy，且每次 redirect
都在 follow 前重新检查 URL、DNS、domain policy、`MaxRedirects` 与必要的 exact grant。

## 组件交互及合同

- `kernel/orchestrator` 通过 Web owner 在 `blocks/web/pkg/events` 定义的 concrete typed command/result 同步获取
  capability、operation-admissibility、operation-feasibility、semantic-proof feasibility，并执行 Web operation。
- Execute command 只携带 `RuntimeOperationExecutionContext + SealedRuntimeOperation`。Web 在网络访问前校验
  Orchestrator producer、非空同步 Result、完整 sealed payload hash、operation/scope、`ExecutionKey + fence`、
  `ExpectedEffectScope=none`，并向 Orchestrator 查询当前 claimed receipt authority。
- Web 不直接查询 Approval owner，也不接受 operation 参数 `approvalId`。需要 plain HTTP、loopback、private network 或
  超 provider result limit 时，Web 返回 exact `RuntimeGovernanceRequirement`；继续执行只接受 Orchestrator 封存且逐字段匹配
  subject/action/risk/authorization hash 与当前 execution key/fence 的 `GovernanceGrant`。
- Search/fetch 都发布 `effect=none + at_most_once` owner policy。外部请求已提交而 receipt 未提交时进入 Runtime
  `outcome_unknown`，不得把“无 receipt”误当作安全重放许可；provider POST 也不被声明为幂等请求。Found/Receipt 一致性、
  query identity 与 commit 完整 receipt equality 由 capability owners 共享的 typed validator 校验。
- 跨 owner 的唯一执行结果是 `RuntimeOperationReceipt`。receipt evidence 携带真实但有界的 normalized content summary、
  SHA-256 content hash、opaque source refs、truncation/quality tags，以及 `workorch://web/evidence/<hash>` raw ref。完整规范化
  内容只写入 workspace internal directory 下的 Web-owner evidence 文件，不把 raw provider body 放入 aggregate。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/execute；exact receipt query/commit；sealed governance grant | 完整 |
| `blocks/approval` | 无直接 I/O | Approval 由 Orchestrator governance transaction 间接绑定 | 完整；不存在普通 Approval 查询旁路 |
| Search provider | Web-private adapter | bounded POST；same-origin redirect；bounded response | 完整 |
| Fetch target | Web-private adapter | DNS-pinned direct GET；pre-follow redirect/SSRF/grant guard | 完整 |

## 有界与安全不变量

- query/URL 最多 4096 bytes；provider identity 最多 128 bytes；locale/recency/extract mode 最多 64 bytes；domain filter
  每组最多 32 项、单项最多 253 bytes。
- result 上限为 20，fetch body 上限为 1 MiB，redirect 上限为 5，owner timeout 上限为 60 秒；provider/config 值也会
  被相同系统 ceiling 裁剪。
- 只允许 HTTP/HTTPS；userinfo、metadata、link-local、unspecified、multicast、非法 port/IPv6 zone 和 deny-domain
  target 在访问前拒绝。plain HTTP、loopback 与 private network 必须先取得 exact approval grant。
- search summary/fetch summary 最多 8 KiB；title、snippet、source、content type 分别独立有界；最多返回一个 raw ref
  加每个结果一个去重 source ref。
- transport/DNS/TLS/connection 错误只映射为固定 owner-safe diagnostic，不把 provider endpoint、target URL、header、
  credential 或底层 `http.Client` 错误写入 Runtime feedback。
- Web 内容只形成 context/evidence candidate；是否接受及是否满足 CWI 仍由 Orchestrator SM-2 根据 sealed binding 裁决。

## 回归覆盖

已覆盖：

- producer、Post/nil Result、sealed payload drift、stale receipt query 均在零 provider call 下 fail closed；
- legacy `approvalId` 拒绝、exact grant target/provider 漂移、大 result set 与 private/loopback 访问；
- redirect target 在 follow 前阻断、DNS 单次解析并绑定实际 dial、redirect hard limit、metadata/allow/deny policy；
- provider cross-origin redirect、timeout/response limit/bad shape 的稳定安全错误；
- normalized search/fetch summary、hash、source/raw ref、owner-private evidence 持久化与 truncation；
- Web operation 参数 ceiling、`effect=none + at_most_once` descriptor/owner policy，以及 provider timeout/response ceiling。

仍需 deployed verification：使用真实 search provider 运行 fresh-data persisted chain，并确认外部请求提交后进程崩溃会由
SM-3 按 at-most-once 收敛为 `outcome_unknown`，不会重复提交。该项属于 `RUNTIME-LIVE`，不是本组件的第二套恢复机制。
