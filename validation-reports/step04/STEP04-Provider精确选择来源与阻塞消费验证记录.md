# STEP04 Provider 精确选择来源与阻塞消费验证记录

日期：2026-10-26。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加累计未提交源码。
本轮承接 MCP 选择阻塞消费者，完成 Provider 冻结选择来源、私有精确编译、公共阻塞合同与 Web 展示。
不是 Provider 就绪、首次 M3、完整 STEP04、全 CI 或发布验收通过的声明；未自动提交累计修改。

## 实现与责任边界

1. LLM cache-only typed query 附带 `workorch-preparation-provider-configuration/v1`：冻结的 DefaultChat 和完整、排序的 exact model override 声明。沿用同次随机 owner epoch，不输出 endpoint、API key 或凭据衍生配置 hash。查询仍不刷新目录、不等待刷新、不发起推理或 embedding。
2. Workspace 原非 stale settlement 从上述完整观察与真实 Workspace 原选择编译 `workorch-preparation-provider-selection/v1`，随原私有 archive/CAS 封存来源、有效选择、原 descriptor、override ID、epoch/catalog revision 和失败码。显式 Workspace 选择优先；nil 才使用同次冻结 owner 默认，不猜选目录第一个模型。
3. 原 Workspace 选择按值复制、保留原声明；只对有效选择执行既有规范化。独立读回复算全部编译；首次 CAS 精确反查真实 Workspace 前像，完整配置身份使用原 accepted binding，不以缺少另存 Soul 的 scoped metadata 重算配置 hash。自洽重哈希的伪造来源仍被拒绝。
4. exact override 沿用既有激活的声明语义，不反驳目录已知 operation，不能凭 override 补造 reasoning 支持。本轮不生成第二份 ProviderCeiling，不重算预算，不改 activation 选择路径。
5. 新版 Configuration 存在时才编译。旧无 Configuration 历史保持原 cache-only 原因；新非 stale archive 不得省略 Selection；stale 不补编；GET、重放、恢复和 owner 重开不升级或覆盖历史。
6. 公共有限原因共 8 项：授权待证明、来源绑定冲突、配置来源不可用、选择非法、模型缺失、能力不完整、operation 不支持、reasoning 不支持；cache miss/expired 保留原分类。新原因要求原 evaluation version、required/llm/blocked 形状及原 evidence ref；未知后缀、原配置串、伪造 PASS 均拒绝。Web 保留原 code，附有限中文解释，零自动检查/恢复/Run，不新增浏览器持久化。
7. 成功编译仍为 `provider_selection_authorization_pending`，只证明点时选择与声明绑定，不证明远端权限、连通性、持续 freshness、预算、当前 epoch 仍存活或 M3。

按 go-multi-module-dev 核对，沿用既有 LLM/Workspace owner 和 typed EventHub query，Workspace 私有 reducer/CAS 持有判定；没有新增运行单元/topic、跨 owner service/repository 或执行授权。Runtime SM-1～SM-4、Start/Restart 与 STEP05 未变。现行合同更新至 LLM/Workspace readme 和 `docs/50-product-interface/service-api.md`；本记录不作为设计权威。

## 验证与受限原始证据

证据根：`/tmp/workorch-step04-provider-selection-0RvTwX`（0700）。原始日志、源码身份快照和二进制未提交 Git；该临时目录不是长期证据归档。

| 验证 | 结果与证据 |
| --- | --- |
| 定向 Go | 7 包通过，`targeted-final4.log`；其后新增原声明保真回归在最终全包与 race 中执行通过 |
| 最终受影响 Go | 23 个有测试包通过、4 个包无测试，`affected-go-final.log`，`-count=1`；不是全仓测试 |
| 最终定向 race | 7 包通过，`race-final.log`；包含原声明保真、来源/失败矩阵、CAS、真实回环 HTTP 与 owner 重开，不是全仓 race |
| Web 定向 | TypeScript 与准备页面 45 项通过，`web-targeted.log` |
| Web 全量组件 | 13 文件、174 项通过，`web-components.log`；保留既有 AntD deprecated 警告 |
| Web 规则入口 | 11 个 Node 测试文件入口通过，`web-rules.log`；文件数不是独立业务断言数量 |
| 静态与边界 | `vet-final.log`、`boundary-final.log`、`fmt-final.log`、`diff-final.log`，均退出 0；保留既有 pycache binary-match 提示 |
| 生产 Web 构建 | `make console-build` 通过，`web-build.log`；嵌入资源已更新，保留 chunk size 告警，不是浏览器验收 |
| 最终二进制与身份 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function-final.log`；构建、produce 和公开 Python verify() 均退出 0 |

最终受影响包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/blocks/llm/... ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向与 race 使用以下七包及 `-run 'TestPreparationProvider|TestPreparationCatalogCache|TestPreparationWorkflowHTTP|TestPreparationObservation' -count=1`；race 额外使用 `-race`：

```text
./internal/modules/blocks/llm/pkg/events
./internal/modules/blocks/llm/biz
./internal/modules/kernel/workspace/pkg/models
./internal/modules/kernel/workspace/biz
./internal/pkg/repository
./internal/pkg/workorchmanagementcontract
./internal/modules/application/workorchd/biz
```

### HTTP、暖缓存与恢复证据的准确范围

- 既有四条 `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 真实回环 HTTP 路径通过，保持空选择、有效非空选择、缺 capability、禁用等原断言，不以新 Provider 分类改写旧 fixture 的结论。
- 新 `TestPreparationProviderSelectionWarmActualOwnerPreservesSourceAndZeroInference` 在检查前显式向本地非敏感 HTTP fixture 请求一次 `/v1/models`，由真实 CatalogService 预热。检查使用真实 LLM owner 的 cache-only 观察，并验证选中来源/descriptor 与阻塞结算。检查、重放、恢复、owner 重开后累计 models 请求仍为 1、其他请求为 0、Run 为 0；不将 setup 的请求计成检查自动刷新，也不声称整个用例零 HTTP。
- 冷缓存真实 owner 同样保留冻结选择来源并返回 cache miss，不因选择编译补造目录或成功状态。测试同时覆盖冻结配置 loader 被调用者修改、nested clone 隔离、凭据 canary 不外泄及新 owner 新 epoch。
- owner 重开在同一测试进程内，不是独立 OS 进程 stop/start。组件测试不是实际 Chrome 故障注入。没有启动本轮二进制的独立生产服务或 Chrome，没有真实模型/embedding、真实 MCP 或 live 配置访问，没有重跑全仓 CI。

## 本轮发现与修正

1. `targeted.log` 首次因 sandbox 禁止回环 listener 失败。仅对本地 fixture 测试提升执行权限后通过，没有绕过业务安全边界。
2. `targeted-final3.log` 中，初版曾对旧无 Configuration 观察编译，覆盖原 cache miss 原因，四条 HTTP 断言失败。已修正生产接线为仅新版完整来源编译；旧材料保留原结论，未弱化旧断言。最终定向、受影响全包与 race 均在修正后通过。
3. 复核发现既有 `CloneProviderInferenceSelection` 会规范化。新归档 clone 改为原值复制；带空格原声明独立保留，有效选择单独规范化，新增反例在最终全包/race 中通过；没有改变 Runtime 通用 clone 合同。
4. 初次 `boundary.log` 使用默认 Go cache，因只读文件系统失败；`boundary-final.log` 改用既定 `/tmp/workorch-go-cache` 后通过。未放宽边界扫描。
5. 最终 Go build 退出 0，同时保留 module stat-cache 只读告警。既有 build-identity CLI verify 问题未在本轮修复；使用公开 Python verify() 与固定四字段核对，不记为 CLI verify 通过。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮累计未提交源码。
- buildInputSha256：`3fe614389077259f627035f51b7cb6f55fb7e01c9c8f98228012a2c8527feeb9`
- consoleTreeSha256：`80d8f1982cb845ba7d38ea3f965d72a6cdeaeafeb8c47a0835a0abf16dc7372e`
- binarySha256：`32f9c737f8fabbeb7693152175e602ecddf2b702add6d0bf079ceb45f5154af9`

verify() 使用上述固定四字段，不从当前树自推期望值。身份仅核对本地源码、实际嵌入控制台与产物字节，不是可信构建 attestation、部署来源证明或 M3 凭据；不将结果归到干净 HEAD，不使用上轮二进制证明本轮源码。

## 仍未完成及后续顺序

1. Provider 授权、既有预算/ceiling 的精确消费，以及持续 freshness/current owner epoch 证明；本轮仅闭合选择来源和声明编译。
2. MCP required connection、连接 owner 事实/epoch、空选择合法适用性政策；Memory 非空 semantic 精确 query/target/预算/真实语义证明；Skill 辅助引用唯一来源/root、真实读取与 freshness。不得把空选择自动当 N/A、伪造语义证明或回退原模式。
3. exact_seal/persistence 等剩余单项消费者、全 required 合取、完整 no-Run 材料及 reservation/epoch/final CAS、首次 M3 正结算和并发/中断/迟到/漂移矩阵。
4. 在最终源码/产物上完成生产 HTTP 成功及阻塞、实际 Chrome、独立 OS 重启验收。目前仍不满足完整 STEP04 live 退出条件。
5. 既有 public fullflow Profile 断言、Orchestrator 套件成本和 build-identity CLI 问题独立跟踪，不以本轮局部通过宣称全 CI 绿。
