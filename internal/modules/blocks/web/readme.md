# Web 能力组件

## 功能说明

`blocks/web` 提供受控的外部搜索和网页获取能力，并返回规范化内容摘要、source refs、truncation flags、provider diagnostic、错误和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 Web search/fetch；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 禁止的 执行通知。
- Runtime capability preflight 通过 Orchestrator 定义的 typed EventHub command 查询本 block 实际提供的 search/fetch kinds。
- Runtime Web operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并返回本 owner stable factual
  category、完整安全 original diagnostic 与有界 evidence/ref；不返回跨组件 Status 双权威。
- 需要校验外部访问授权时，本 block 导入 `blocks/approval/pkg/events`，通过 EventHub 查询 Approval；不注入 Approval service/repository。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed operation、provider/ref 和有界
  query/url；result 返回唯一 receipt outcome、source refs、bounded summary、truncation、provider diagnostic 和
  evidence/ref。

## 当前内聚收口切片（2026-08-16）

- `RuntimeAdapter` 仍是唯一 Web capability adapter；原 863 行文件已按 operation ingress/admissibility、search、
  fetch、provider transport、provider configuration cache、URL/SSRF policy、Approval typed query、result mapping、
  content byte bounder 与纯 helper 分成同 package 的 owner-private 文件。没有新增 Block、service/repository 注入、
  EventHub round trip、provider retry、receipt owner 或通用 transport executor。
- Approval 仍只通过 scoped typed getter 查询并校验 run/action/provider/context；policy helper 只返回 allowed/denied/
  approval-required facts，不创建 Approval，不修改 Runtime authority。search/fetch 仍各执行至多一次 provider request，
  final URL 继续重走同一 SSRF/approval policy。
- response body 与 fetch content 分别在 transport 和 content bounder 处执行原有 byte ceiling；result normalization
  继续返回 source/hash/truncation/evidence，不写 Memory/Result，也不把外部内容直接判为 coverage。
- C22-05 Web 切片完成后，非测试 Go 为 25 个文件/2,026 LOC，测试为 6 个文件/765 LOC，package 保持 6；
  最大生产文件从 `runtime_adapter.go` 863 行降为 `owner.go` 364 行，至少 500 行生产文件从 1 个降为 0，生产
  `CC>30` 继续为 0。

## 相关约束

- query、URL、响应大小和执行时间必须有界，并区分 provider/network failure 与 contract violation。
- 大内容只能通过摘要和 raw ref 传递。
- 本 block 不拥有 Memory、Result、Run 或业务完成状态。
- Web 内容只作为 context/evidence candidate，不能直接写入 owner 状态或形成 accepted Result。
- 本 owner 私有 code 只在 Web Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。
