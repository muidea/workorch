# SSH 能力组件

## 功能说明

`blocks/ssh` 使用结构化 target、session 和 credential refs 执行受控远程命令，并返回规范化远程执行结果。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 SSH 操作；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 禁止的 执行通知。
- Runtime capability preflight 通过 Orchestrator 定义的 typed EventHub command 确认 SSH command capability 当前在线。
- 远端输出只形成有界 typed evidence，不作为跨组件 map 返回。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed target/session/credential refs、command、
  timeout 和 policy；result 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并返回远程 exit code、有界输出、
  target ref、owner stable factual category、完整安全 original diagnostic 和 evidence/ref，不返回跨组件 Status 双权威。
- runtime policy、Governance 和 Approval 在远程副作用执行前完成准入。

## 当前内聚收口切片（2026-08-16）

- `RuntimeAdapter` 仍是唯一 SSH capability adapter；原 366 行文件按 operation/result ingress、target/session/
  credential validation、SSH transport/credential materialization 与 output bounder 分成同 package 的四个职责文件。
  当前 diagnostic 只是在唯一 ingress 上映射 timeout/command failure，没有足够独立规则形成额外 facade 或 package。
- `ProbeOperation` 继续只验证结构化 target/session/credential 和本地 private-key 文件，不打开网络；执行仍只有
  一次受 timeout 约束的 SSH process。临时 private key/askpass 仍由 transport 内同一 cleanup 闭包释放，secret
  不进入 typed result、diagnostic 或 projection。
- C22-05 SSH 清理后，非测试 Go 为 17 个文件/1,054 LOC，测试保持 5 个文件/414 LOC，package 保持 4；
  最大生产文件从 `ssh_runtime_adapter.go` 366 行降为 `owner.go` 248 行，生产 `CC>30` 继续为 0。

## 相关约束

- target、session 和 credential 必须使用结构化引用；secret 不得进入事件、投影或日志。
- 远程副作用必须有 sealed canonical state/operation admission；权限缺失必须返回独立 typed
  `GovernanceRequirement`，普通失败或 budget exhaustion 不能创建 ticket。
- 本 block 不拥有远程业务状态、Run 状态或 Result 状态。
- 远程输出必须经过 evidence binding 才能用于 coverage。
- 本 owner 私有 code 只在 SSH Biz 边界映射为 factual category；不得选择 correction、retry、terminal 或下一
  canonical state。unknown code 保留原 code/message 并 typed fail-fast。
