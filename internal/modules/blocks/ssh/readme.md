# SSH 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/ssh` 使用结构化 target、session 和 credential refs 执行受控远程命令，并返回规范化远程执行结果。

## 组件交互及合同

- `kernel/orchestrator` 通过 SSH owner 在 `blocks/ssh/pkg/events` 定义的 `ExecuteOperationCommand/Result` 同步投递
  SSH 操作；本组件直接订阅 EventHub，不启动通用 runtime executor 或第二套执行通知。
- Runtime capability、operation-admissibility、operation-feasibility 与 semantic-proof feasibility 均使用 SSH owner
  定义的 concrete typed command/result。
- 远端输出只形成有界 typed evidence，不作为跨组件 map 返回。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed target/session/credential refs、command、
  timeout 和 policy；result 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并返回远程 exit code、有界输出、
  target ref、owner stable factual category、完整安全 original diagnostic 和 evidence/ref，不返回跨组件 Status 双权威。
- runtime policy、Governance 和 Approval 在远程副作用执行前完成准入。
- effect 执行前后通过 Orchestrator receipt query/commit concrete command 完成幂等握手；SSH 只返回远端 owner observation。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/execute；receipt query/commit | 完整 |
| 远端 SSH endpoint | SSH-private adapter | sealed target/session/credential refs、command、timeout | 完整；secret 不跨 EventHub |
| Governance / Approval | 间接 sealed fact | 由 Orchestrator 完成准入，SSH 只返回 typed requirement | 完整；无 owner service/repository 直连 |

## 相关约束

- target、session 和 credential 必须使用结构化引用；secret 不得进入事件、投影或日志。
- 远程副作用必须有 sealed canonical state/operation admission；权限缺失必须返回独立 typed
  `GovernanceRequirement`，普通失败或 budget exhaustion 不能创建 ticket。
- 本 block 不拥有远程业务状态、Run 状态或 Result 状态。
- 远程输出必须经过 evidence binding 才能用于 coverage。
- 本 owner 私有 code 只在 SSH Biz 边界映射为 factual category；不得选择 correction、retry、terminal 或下一
  canonical state。unknown code 保留原 code/message 并 typed fail-fast。

## 待完善测试点

- [ ] 覆盖 credential/session/target secret 不进入 EventHub、日志、projection、raw diagnostic。
- [ ] 覆盖连接、命令、取消和 timeout 的资源释放，以及后台远端进程的有界处理。
- [ ] 覆盖 exit code、target ref、truncation、provider error 的稳定 factual category 映射。
- [ ] 覆盖 governance fence 缺失、stale、wrong execution key 时远程副作用不发生。
- [ ] 覆盖远端 effect 后 receipt commit 前崩溃的查询重放与不可安全重放操作的 fail-closed 行为。
- [ ] 覆盖大 stdout/stderr 只返回有界摘要/raw ref，不直接取得 coverage credit。
