# Web 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/web` 提供受控的外部搜索和网页获取能力，并返回规范化内容摘要、source refs、truncation flags、provider diagnostic、错误和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Web owner 在 `blocks/web/pkg/events` 定义的 `ExecuteOperationCommand/Result` 同步投递
  Web search/fetch；本组件直接订阅 EventHub，不启动通用 runtime executor 或第二套执行通知。
- Runtime capability、operation-admissibility、operation-feasibility 与 semantic-proof feasibility 均使用 Web owner
  定义的 concrete typed command/result。
- Runtime Web operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并返回本 owner stable factual
  category、完整安全 original diagnostic 与有界 evidence/ref；不返回跨组件 Status 双权威。
- 需要校验外部访问授权时，本 block 导入 `blocks/approval/pkg/events`，通过 EventHub 查询 Approval；不注入 Approval service/repository。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed operation、provider/ref 和有界
  query/url；result 返回唯一 receipt outcome、source refs、bounded summary、truncation、provider diagnostic 和
  evidence/ref。
- operation 前后通过 Orchestrator receipt query/commit concrete command 完成幂等握手；Web 不把 provider response 直接写入 Runtime aggregate。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/execute；receipt query/commit | 完整 |
| `blocks/approval` | Web → Approval | scoped external-access approval query | 完整；无 service/repository 注入 |
| 外部 Web provider | Web-private adapter | sealed search/fetch request 与有界 response | 完整 |

## 相关约束

- query、URL、响应大小和执行时间必须有界，并区分 provider/network failure 与 contract violation。
- 大内容只能通过摘要和 raw ref 传递。
- 本 block 不拥有 Memory、Result、Run 或业务完成状态。
- Web 内容只作为 context/evidence candidate，不能直接写入 owner 状态或形成 accepted Result。
- 本 owner 私有 code 只在 Web Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。

## 待完善测试点

- [ ] 覆盖 URL scheme/host/redirect allowlist、内网地址与 DNS rebinding/SSRF 拒绝。
- [ ] 覆盖 Approval 缺失、跨 workspace/run、stale 或 wrong operation scope 时不发起外部访问。
- [ ] 覆盖 provider/network/timeout/protocol/contract failure 的稳定分类和安全原始诊断。
- [ ] 覆盖响应截断、大内容 raw ref、source ref 去重以及敏感 header/body 不进入事件。
- [ ] 覆盖外部请求已提交但 receipt commit 前崩溃的重放，确认不安全重复请求 fail-closed。
