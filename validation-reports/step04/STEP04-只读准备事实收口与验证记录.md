# STEP04 只读准备事实收口与验证记录

日期：2026-10-21（Asia/Shanghai）。检出基线：`834aa350cc4e77379819666540b9912a7c22b64c`，
验证对象为该基线加本轮未提交源码，不是已提交 SHA 的独立验收。

## 结论与授权

本轮收口两个只读准备子能力：LLM catalog cache-only 观察、Run owner 完整且排序的历史观察，
及 Application 的 typed sender。两者都尚未接成完整准备检查流程。
**完整 STEP04、no-Run 协议、M3 和 live 验收尚未完成。**

用户已明确批准《STEP04-实施前合同审阅稿》§3 的精确 no-Run 协议 v1，回复原文为
“批准该精确协议并实现”。本轮不再将该授权列为阻塞，也不将批准等同于实现通过。
现有 fresh execution ingress 尚未修改；reservation/epoch 与 M3 final CAS 仍需后续落地。
未读取 live 配置、未调用真实 provider/embedding、未修改现有服务和数据，未提交真实执行意图。

按 `go-multi-module-dev` v2.6.0 的边界要求，未新增运行单元；跨 owner 只用 concrete typed
EventHub command/result，repository 窄读取能力仅在 Run owner 内使用，不跨 owner 注入。
本轮涉及 SM-1 的只读排序观察，不新增 lifecycle transition；SM-2/SM-3/SM-4 权威、预算、
dispatch、恢复和 delivery 不变。观察不选择动作、不启动 Runtime、不授 execution credit。

## 已落地内容

| 子能力 | 实际实现与验证边界 |
| --- | --- |
| F03 provider cache-only | 新 topic `/llm/startup-preparation/catalog-cache/query`，exact Application source。只读现存缓存，不 HTTP、不等待 refresh、不 fallback/淘汰/改写。miss 为 unavailable，精确 TTL 及超 LKG 为 stale；保留完整模型声明与原 revision，不将 fresh 解释为可连接/有授权/ready。 |
| 配置与完整性 | 随冻结配置生命周期产生随机 OwnerEpoch，重建 owner 不复用；不导出 endpoint、credential 或 credential-derived revision。完整结果模型数、字节、字段与嵌套列表有界，损坏/超限整体拒绝。异常缓存诊断不清洗后提升为成功；detached clone 保留 nil/空列表形状。embedding 零 context/output 元数据仅原值观察。 |
| no-Run 完整历史子能力 | 新 topic `/run/startup-preparation/history/query`，exact Workspace/Application source。在 `workspaceExecutionMu` 下与 fresh restart 同序；repository 一次读取 generation head、全部 Run、pending/replacement 数和 retirement fence，含 terminal/retired。删除 Run 后 watermark 保留。 |
| 三种持久实现 | memory、file、workspace-scoped file 均提供窄 `RunPreparationHistoryReader`；缺能力不 fallback 普通列表。scoped file 精确读取物理目录并保留错误，不用吞错 Glob；部分目录/损坏 JSON/缺 watermark/异常文件或观察坐标符号链接不能变成空历史，不修复文件，不创建查询目录。 |
| Application 接线 | 两个准备子用例保留父 scope/context，独立 2 秒预算，校验 typed result、scope 和观察身份，无自动重试。真实 Application→LLM/Run owner 联合测试覆盖实际 source/destination/DTO，非仅宽松 observer mock。 |
| 生命周期与合同登记 | 新 topic、message、read/query lane、router、scope extractor、result clone 和生命周期切点同步登记。总 inventory 为 313＋LLM cache-only 1＋Run history 1＝315；Run 订阅由 22 增至 23，完整失败切点与逆序释放同步覆盖，非直接改数字凑绿。 |
| 文档归位 | STEP03/STEP04 开发计划从 `docs/` 移至 `validation-reports/step03`、`validation-reports/step04`；历史正文可恢复。现行已落地边界写入 LLM/Run/Application readme，不把计划、批准或中间记录当成当前设计权威。 |

源码与定向证据入口：

- LLM：`internal/modules/blocks/llm/biz/catalog_cache_observation{,_test}.go`、
  `pkg/events/catalog_cache{,_test}.go`；miss/fresh/TTL/LKG、unfinished refresh、配置轮换、
  原值/clone/bounds、损坏、cancel、并发、source 与 lifecycle，观察链 HTTP counter 为零。
- Run：`internal/modules/blocks/run/biz/preparation_history{,_test}.go`、
  `pkg/events/preparation_history{,_test}.go`、`pkg/models/preparation_history{,_test}.go`；
  真 Hub receiver、source/DTO/scope、完整退役可见性、同 fresh lock 排序与不驱动。
- repository：`internal/pkg/repository/run_preparation_history_test.go` 三后端 reopen、
  删除 watermark、retirement、clone、scope/cancel；
  `internal/pkg/workorchrepo/run_preparation_history_test.go` 部分/损坏/符号链接及读取前后物理树不变。
- Application：`activation_preparation_catalog_test.go`、`activation_preparation_history_test.go`、
  `activation_preparation_owners_test.go`，含真实两个 owner 接线、重复只读和零 Run mutation。

## 验证与原始证据

受限原始目录：`/tmp/workorch-step04-readonly-facts-wWgujY`（临时目录，不保证跨机器或系统清理后存在）。
未将原始诊断、live 配置或凭据纳入 Git。本记录只索引本机证据，不称评审方独立复跑或可信 attestation。

| 检查 | 范围与记录 |
| --- | --- |
| 受影响包累计 Go 回归 | LLM/Run 全子包、Application biz、Workspace 全子包、repository 全子包、workorchrepo、events；`-count=1 -timeout=5m`，见 `affected-go-test-final-v2.log`。使用测试临时目录/回环 fixture，不是生产 HTTP 或收费 live。 |
| 定向 race | 上述新准备能力涉及的 8 个包，`-race -run '^Test.*Preparation(Catalog\|History\|Observation)' -count=1 -timeout=3m`，见 `preparation-race-final.log`。不是所有相关包全测试集 race。 |
| vet | LLM/Run 全子包、Application biz、repository 全子包、workorchrepo、events；`vet-final.log`。 |
| 格式/边界 | `make fmt-check`、`git diff --check`、`make verify-boundary-contract`；格式日志及 `boundary-final.log`。 |
| 构建 | `go build -o <证据目录>/workorch-final ./application/workorch`，`build-final.log`；没有启动该二进制，也没有重跑生产 Web 构建或浏览器。 |

过程失败单独保留：首次累计 `affected-go-test.log` 的 Run lifecycle 两项失败来自新订阅加入后旧固定
数量/切点未同步；修复后 `run-retest.log` 通过，最终累计重新执行。真实 owner 联合测试还发现并修复
Application 与 Run history source 不匹配；最终测试使用实际 sender→receiver，不以 mock 结果替代。
首次 sandbox 中旧累计尝试遇本机 listener 限制，后续仅为测试回环 fixture 使用隔离外执行；没有修改鉴权。

源码/构建身份由 `build_identity.py capture/produce` 留存，最终值在后续结果节；
`produce` 复核构建前后指定源码和嵌入控制台输入一致，不证明工具链因果、发布部署或 acceptance。
未把已披露有问题的 CLI `verify` 写为通过。

### 最终执行结果与身份

- 最终累计：**24 个有测试包通过，5 个包无测试**；上述定向 race 的 **8 个包通过**。
  vet、fmt-check、diff-check、boundary gate 与二进制 build 均返回 exit 0。
- `build-final.log` 保留 Go 向只读 module stat cache 写入被拒的警告；该构建实际返回 0 并生成产物，
  随后 `produce` 完成字节校验。没有将警告删掉或写成无警告构建。
- `source-snapshot-final.json`、`source-snapshot-after.json` 在最终回归/构建后的原值比较一致；
  `build-identity-final.json` 指向 `workorch-final`。这是 dirty tree 的内容身份，不冒充仅 HEAD 构建。

| 字段 | 最终值 |
| --- | --- |
| sourceCommit（检出基线） | `834aa350cc4e77379819666540b9912a7c22b64c`＋本轮未提交源码 |
| buildInputSha256 | `9b76f1008ee1cf08457c94ed2febb7c89c2cdcd1876aae5c61d686671543c741` |
| consoleTreeSha256 | `045e1a379991d29078f1bbab1752b882e2986cad8b770b4be2dc3f40df60960b` |
| binarySha256 | `4b4bda76be6a2aa726dce54b15ede8b1493256d559d4a39432e78be65e2fced1` |
| affected-go-test-final-v2.log SHA256 | `2f27c46b5682ac2d353fa9b37a5fbf1303fd31ad54ffcd8e974cfa3ab6fd5818` |
| preparation-race-final.log SHA256 | `af8da14a3fdfbb9f057d772c49137d6801e1ef12147d161f770fb977ac65cadf` |
| boundary-final.log SHA256 | `8ab706d3219b60dc3b77c6e09720ef5b821e46e659d34c282f74576a36fbb293` |

旧 `source-snapshot-before.json`、`build-identity.json` 与 `affected-go-test-final.log` 是随后
补充 clone 空列表形状保真之前的中间快照/结果，不作为上表最终源码的证据。

## 仍需逐项收口

1. **F03 其余资源事实**：真实目录/权限、托管配置、Soul/Skill/MCP/Memory 完整性/版本/原模式及准确阻塞。
   缺 required 事实不能用声明或 fixture PASS 替代；不得为检查自动联网刷新、物化或 embedding。
2. **已批准 no-Run 协议的剩余实现**：新建 Workspace creation marker、全部 fresh ingress 的 durable
   reservation/epoch、同键完整 payload 反证、原请求结果只读对账、pending/unknown 不重发不凭空清除、
   legacy 阻塞、M3 final CAS 及两种先后/崩溃/迟到回执矩阵。当前历史观察不是这些门禁的替代品。
3. **F05/F08/F07**：显式检查接纳、准备私有 reducer/CAS、精确幂等与结果、原操作恢复，以及满足全部
   required＋无历史＋顺序门后的唯一历史 M3。当前没有准备写入口或可签发 M3 的生产链。
4. **增量三**：HTTP 检查/恢复、Web 分轴展示/迟到 fence、生产构建浏览器及独立服务 stop/start 验收。
   本轮 repository reopen 和测试回环不能替代这些证据。完整 Start/Restart 回执消费仍属 STEP05。

继承验证阻塞未在本轮默认修复/重跑：public fullflow 默认 Profile/DoD policy 断言、Orchestrator biz
累计 15 分钟超时、build-identity CLI verify 问题。没有新的全仓 CI 结果，不宣称全 CI、完整 STEP04 或发布验收通过。
