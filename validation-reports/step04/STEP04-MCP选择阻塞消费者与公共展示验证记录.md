# STEP04 MCP 选择阻塞消费者与公共展示验证记录

日期：2026-10-26。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加累计未提交源码。
本轮承接私有 MCP 精确选择编译，完成原操作的版本化阻塞消费、公共合同与 Web 展示。
这不是 MCP 连接 readiness、首次 M3、完整 STEP04、全 CI 或发布验收通过的声明；未提交累计修改。

## 本轮实现与责任边界

1. 原非 stale settlement 在完整 Skill/MCP 选择编译后，私有保存 `workorch-preparation-mcp-selection-evaluation/v1`，从归档内容派生公共原因，随原 Workspace CAS 结算。不接受 application/caller 提供的分类代替 owner 消费。
2. 选择为空、选择成功但缺连接证明，以及 12 类来源/配置/数量/必需引用/server/capability 失败，共 14 个有限原因码。未知版本、未知后缀、夹带原始配置的原因，以及用成功分类伪装 FailureCode，均不被接受。结果始终为 blocked；空选择不自动授予 N/A。
3. 新原因必须绑定 record v2、原逐项 evaluation version、required MCP Registry owner、blocked attempt/check 和原完整 evidence ref。独立读回仍重编并反查同 request/payload 的完整 Skill/registry 来源；重算 hash 不能替代逐项关联验证。
4. 新增带 Selection 的 archive 首次 CAS 必须携带消费者版本；旧无消费者版本的历史继续旧 `mcp_registry_observed_connection_pending`，GET/重放/恢复不升级、不补编。历史可读不等于允许降级或覆盖已完成记录。
5. 公共 DTO 不新增字段；HTTP 保留原原因和证据引用，Web 额外显示有限中文说明。页面不自动检查/恢复/启动 Run，不持久化浏览器业务数据。原始 registry 与 Skill 材料保持私有归档，不通过公共原因输出原始配置。
6. 更新 Workspace owner readme 和现行 service-api 合同；验证记录仅置于本目录，不将实施记录作为 docs 设计权威。

按 go-multi-module-dev 核对，沿用现有 Workspace 私有 reducer/CAS、typed owner 查询和 HTTP 只读投影；无新运行单元/topic、跨 owner repository/service 或执行授权。Runtime SM-1～SM-4、Start/Restart 与 STEP05 未变。

## 验证结果与原始证据索引

受限证据根：`/tmp/workorch-step04-mcp-consumer-lxsItJ`（0700）。日志、快照与二进制不提交 Git；该临时目录不是长期原证据归档。

| 验证 | 结果与证据 |
| --- | --- |
| 定向 Go | 5 包通过；`targeted-final.log`。覆盖有限原因、消费者绑定、三后端 CAS、原操作切点及四条真实隔离 HTTP 路径 |
| 受影响 Go 全包 | 21 个有测试包通过、4 个包无测试；`affected-go.log`，`-count=1`。不是全仓测试 |
| 定向 race | 5 包通过；`race.log`。不是全仓 race |
| Web 定向 | 准备页面 37 项通过，包含 14 类展示及无自动 POST/Run 断言；`web-targeted-final.log`，含 TypeScript 检查 |
| Web 全量组件 | 13 文件、166 项通过；`web-components.log`，保留既有 AntD deprecated 警告 |
| Web 规则入口 | 11 个测试文件入口通过；`web-rules.log`。该入口使用 Node test 汇总，不将文件数当作独立业务断言数量 |
| 静态与边界 | `vet.log`、`fmt.log`、`diff.log`、`boundary.log` 通过；保留既有 pycache binary-match 提示 |
| 生产 Web 构建 | `make console-build` 通过；`web-build.log`。已更新嵌入资源，保留 chunk size 告警；不是浏览器验收 |
| 二进制及身份 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function-final.log`；构建、produce 和公开 Python verify() 均退出 0 |

受影响 Go 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/blocks/mcpregistry/... ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向 Go 与 race 使用以下五包，以及 `-run 'TestPreparationMCP|TestPreparationWorkflowHTTP' -count=1`；race 额外使用 `-race`：

```text
./internal/modules/kernel/workspace/pkg/models
./internal/modules/kernel/workspace/biz
./internal/pkg/repository
./internal/pkg/workorchmanagementcontract
./internal/modules/application/workorchd/biz
```

`TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 的四个子例分别核对空选择、有效非空选择、缺 capability、server 禁用，对应精确原因。使用真实 File/Workspace/Soul/Skill/Memory/MCP Registry/Run owners，候选 provider 与 SkillHub 使用固定非敏感本地 fixture；MCP fixture 只预置 metadata，完整 revision 由真实 owner 派生，`never-start` 不执行。首次双读后，GET/重放/恢复/owner 重开保持读取计数为 2、零 Run，不连接 MCP transport。

owner 重开发生在同一个测试进程，不是独立 OS 进程 stop/start。组件覆盖不是实际 Chrome 故障注入。本轮没有启动新构建的独立生产服务或 Chrome，没有真实推理/embedding、真实 MCP 或 live 配置访问，也没有重跑全仓 CI。

## 本轮发现与修正

1. `web-targeted.log` 初次因新 TypeScript 条件多一个右括号失败。改为明确的多行条件，最终 TypeScript、37 项定向与166项组件测试通过。
2. `targeted.log` 的 disabled fixture 保留 enabled capability，被真实 Registry owner 的既有 invariant 正确拒绝，因而只得到 not_saved。修正 fixture 同时禁用 server/capability，未放宽生产规则；最终四条 HTTP 路径通过。
3. 复核时发现初版 FailureCode 映射复用了包含两类成功编译原因的白名单。改为独立 12 值失败白名单，并新增伪造成功后缀/raw-config/未知版本反例；最终定向、全包及 race 均在修正后执行通过。
4. Go build 退出 0，同时保留 module stat-cache 只读告警。既有 build-identity CLI verify 问题本轮未修，使用公开 Python `verify()` 和固定四字段核对；不记为 CLI verify 通过。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮累计未提交源码。
- buildInputSha256：`f7d6b1c8d5244dfe8f619648d3d96405a67575c93f70900cb9a24fc1dfb5f897`
- consoleTreeSha256：`8e51ca7304193934181826521bb4216b10148481f1dd5c70c9a303db58cc6689`
- binarySha256：`b0a0c9ec880d1b748e03680527f77873d49409d82de7ad4dd1f1238db87922dc`

verify() 使用上列固定四字段，不从当前树自推期望值。身份仅核对本地源码/嵌入控制台/产物字节，不是可信构建 attestation、部署来源证明或 M3 凭据；不能将该结果归到干净 HEAD，也不引用上一轮产物证明本轮源码。

## 尚未完成与后续顺序

1. MCP required connection、原模式/连接 owner 事实和持续有效性；空选择的合法适用性/N/A 判据仍未完成。编译和本轮细分阻塞不能代替这些证明。
2. Provider 精确 target/config/capability/授权/freshness；Memory 非空 semantic 的精确查询、target、预算与真实语义证明；Skill 辅助引用唯一来源/root、真实读取与 freshness。不得伪造语义证明、回退原模式或修改外部 SkillHub。
3. exact_seal/persistence 等剩余单项消费者、所有 required 合取、完整 no-Run 证据、reservation/epoch/final CAS、首次 M3 正结算和并发/中断/迟到/漂移矩阵。
4. 最终源码/产物上的生产 HTTP 成功与阻塞、实际 Chrome、独立 OS 重启验收。当前 blocked 链尚不满足完整 STEP04 live 退出条件。
5. 既有 public fullflow Profile 断言、Orchestrator 套件成本与 build-identity CLI 问题独立跟踪；不据本轮局部通过宣称全 CI 已绿。
