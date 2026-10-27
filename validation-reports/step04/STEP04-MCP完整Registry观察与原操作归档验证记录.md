# STEP04 MCP 完整 Registry 观察与原操作归档验证记录

日期：2026-10-23。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交源码。本轮仅收口 MCP 完整 registry 只读观察、原操作私有归档与公共阻塞展示，不表示 MCP 连接就绪、完整 STEP04、M3、全 CI 或发布验收通过。

## 本轮实现

1. 复用 MCP Registry owner 的 `ReadRegistrySnapshotCommand/RegistrySnapshotResult`，只为该 query 增加 exact `application/workorchd` source；不扩大写入或 `EnsureDefaults` 权限，不新增 topic。Registry 继续拥有 metadata，连接与执行仍归 MCP capability owner。
2. 准备响应上限为完整 JSON 1 MiB、256 个 server、4096 个 capability。超限明确报错，不截断、不筛选或返回成功空值；不改变既有 Runtime 查询上限。完整内容包括 owner 既有规范化快照的全部 server、capability、环境引用、schema 和内容 revision，不声称返回持久化文件的原始字节。
3. `ValidatePreparation()` 独立核验完整内容 revision、身份顺序与重复、capability/server 关联和 binding 唯一性；环境引用转换不得靠 trim/reorder 掩盖改变，非法 UTF-8 拒绝。detached clone 保留原 typed 值、schema、切片与显式空列表形状。内容 revision 不是成功凭据或连接 epoch。
4. Application 在原 accepted 操作的既有 5 秒整体观察预算内执行 exact seal → 完整 registry → 完整 registry → exact seal；完整前后值必须精确相等。错误 DTO、超限、双读漂移、取消或来源不匹配不会被当成空 registry。
5. Workspace 独立反查原 accepted 身份、受理 revision、`ObservationSnapshotHash`、binding、request/payload 与时序，构造 `workorch-preparation-mcp-registry-evidence/v1` 私有历史。原事实、随机引用与负向结算在同一 blocked CAS 保存；memory/file/scoped 三后端深克隆，file 私有材料要求 0600。
6. 读回重新验证完整前后值、内容 revision、原 accepted 关联、引用与原因码一一对应。完成历史 append-only；同时删除引用和归档也不能保留“已观察”的分类。替换原文、子集、scope、绑定、时序或裁决版本后自洽重哈希仍拒绝。
7. 公共 Go/TypeScript 只投影 `mcpRegistryEvidenceRef`（`mcp-registry-observation:` 加 32 位随机小写 hex）及 required `mcp_selection`/`mcpregistry` 的 `blocked/mcp_registry_observed_connection_pending`。真实空 registry 同样阻塞，不推断无需 MCP。无效、超限、漂移或未保存为 `mcp_registry_observation_unavailable_not_saved`，不补造历史。
8. Web 只展示历史引用和阻塞提示；GET、同键重放、显式恢复及 owner 重开不重读 registry。已知 revision/epoch 漂移保留原历史并标 `binding_changed/stale`。不 connect、initialize、refresh、EnsureDefaults 或调用工具，不授精确 selection、connection readiness、持续 freshness 或 M3。

按 go-multi-module-dev 核对，沿用 Application typed 延续、MCP Registry owner 查询和 Workspace 私有 reducer/CAS，没有新增运行单元、topic 或跨 owner repository/service。Runtime SM-1～SM-4、Start/Restart 授权及 STEP05 行为不变；文档更新仅维护当前已落地合同。

## 验证与证据索引

受限证据根：`/tmp/workorch-step04-mcp-registry-QQ1UWE`（0700）。原日志、快照和二进制不提交 Git；临时路径不是长期证据归档。

| 验证 | 结果与原证据 |
| --- | --- |
| 最终受影响 Go 全包 | 20 个有测试包通过、4 个无测试包；`affected-go-final-exact-clone.log`，`-count=1` |
| 完整 owner query | `TestPreparationMCPRegistryReadIsCompleteBoundedAndReadOnly`：完整/空值、零写入、clone、子集/替换/重复/非法编码/转换失真/未知 server/合法大快照超限拒绝 |
| exact publisher | `TestMCPRegistryCommandSourceAuthorizationMatrix`：保留完整 source 矩阵，不接受 padded source 或扩大 mutation 权限 |
| 原操作故障切点 | `TestPreparationMCPRegistryOriginalCutsAndReopen`：完整/空值、错 DTO、有效内容双读漂移、超限、结算写失败、回执丢失、Workspace revision 漂移、重放/恢复/file owner 重开、零 Run |
| 三后端与独立读回反证 | `TestPreparationMCPRegistryRepositoryOriginalCASCloneAndReopen`：原 CAS、clone/reopen、私有 JSON 隔离、erase/snapshot/binding/reason/owner/required/passed/subset/source/evaluation/scope/time/ref/duplicate/legacy 反例 |
| 真实回环 HTTP | `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen`：真实 MCP Registry/File/Workspace/Soul/Skill/Memory/Run owners；MCP 空 registry 不 ensure，首次恰好双读，GET/重放/恢复/owner 重开读取次数仍为 2，原 accepted snapshot 与私有历史保留，整体仍 blocked、零 Run |
| 定向 race | 5 个实际执行测试的包通过；`race-final-exact-clone.log`，不是全仓 race |
| Web | `web-components.log`：13 文件、152 项组件通过；`web-rules.log`：11 项规则通过；MCP 历史引用、错误公共投影拒绝和零自动 POST/恢复/Run 覆盖 |
| 边界/vet/格式/diff | `boundary-exact-clone.log`、`vet-exact-clone.log`、`fmt-exact-clone.log`、`diff-final.log` 通过 |
| 生产 Web 构建 | `web-build.log`；通过，保留既有 chunk 大小告警；嵌入产物已更新 |
| 本地二进制与输入身份 | `source-snapshot-exact-clone.json`、`build-exact-clone.log`、`build-identity-exact-clone.json`、`build-produce-exact-clone.log`、`build-verify-function-exact-clone.log` |

最终全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/blocks/mcpregistry/... ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/blocks/mcpregistry/biz ./internal/modules/kernel/workspace/biz ./internal/pkg/repository ./internal/pkg/workorchmanagementcontract ./internal/modules/application/workorchd/biz -run 'TestPreparationMCP|TestMCPRegistryCommandSource|TestPreparationWorkflowHTTP|TestPreparationObservationProductionNegativeClassification|TestPreparationWorkflowTypedApplication' -count=1
```

原操作故障测试使用真实 MCP owner 预先生成完整回执，再用 concrete reply proxy 注入错误；不是每个故障都在纯生产 provider 路径注入。有效漂移的两份快照均来自真实 owner，不手造内容 revision。HTTP 使用真实空 registry owner，候选准备与模式观察使用本地无收费 fixture，不构成真实模型或 MCP transport 验证。owner 关闭重开在同一测试进程内，不是独立 OS 进程 stop/start；未执行生产 Chrome，不以组件测试替代浏览器证据。

### 失败、修正与复验

1. `targeted.log` 初次测试误用 repository 方法；`targeted-rerun.log` 的合法大快照夹具未按 CAS 合同递增 revision。修正测试夹具，未放宽生产校验。该早期运行已中止，不计通过。
2. `cuts-debug.log` 暴露测试 helper 把 typed nil error 存入已有 error interface，且在 EventHub handler 内调用 Fatal 导致等待。改用独立错误变量，并在测试线程预先读取真实 owner 值再安装故障代理；最终切点及 race 通过。
3. `boundary.log` 把 facts 类型名中的 Registry 误判为可变句柄。明确命名为纯 `PreparationMCPObservationFacts`，不修改或豁免边界规则。`boundary-final.log` 随后因默认 Go cache 目录只读失败；使用既定 `/tmp/workorch-go-cache` 后最终边界通过，保留既有 pycache binary-match 提示。
4. `affected-go.log` 中两组旧测试仍期待通用 `check_pipeline_incomplete`。对缺 MCP owner 的夹具改为精确 `mcp_registry_observation_unavailable_not_saved`，仍要求 blocked；`affected-go-final.log` 及最终 exact-clone 全包通过。
5. `independent-validation.log` 与早期 `race.log` 发现同时删除归档和引用时，独立材料校验尚未反证“已观察”原因码。补齐必要关联检查；`independent-validation-rerun.log`、最终全包及 race 通过。
6. 最后补强显式空切片的精确保真 clone，并重新运行受影响全包/race/vet/格式/边界及构建身份，最终以带 `exact-clone` 的证据为准；先前二进制不用于该最终源码的验证声明。
7. Go build 有 module stat-cache 只读告警但退出 0。既有 build-identity CLI verify 问题不在本轮处理；调用公开 Python `verify()` 并固定完整四字段核验，不记 CLI verify 通过。未运行全仓 CI。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终累计未提交源码，不是独立验证该干净提交。
- buildInputSha256：`04fb198c27a62e3af3dcf43722f6695adf1096bbc49be4d3a0bbb9437ca0c258`
- consoleTreeSha256：`97283a5a8317ba0a663ab7f71856f2887d62fa3355fabe18704bd3aff7181b6a`
- binarySha256：`1e360f279cd502ea82ee6c03bc94e21608d928688f63b9b00e84a60bf15935f9`

这些值仅核对本地完整构建输入、嵌入控制台与二进制字节一致，不是可信构建 attestation、部署验收或 M3 凭据。

## 仍需收口

1. MCP 从完整 registry 到精确封存选择的确定性编译、required connection/原模式与真实连接 owner 证明，以及持续有效性；完整历史引用不能代替这些事实。
2. Provider 精确 target/config/capability/授权/freshness；Memory 非空 semantic 的精确查询/target/预算与真实语义证明；Skill 辅助引用唯一来源/root、真实读取与 freshness。不得回退模式或用声明授 credit，不改外部 SkillHub。
3. exact_seal/persistence 等剩余单项消费者、全 required 合取、完整 no-Run 历史证据、reservation/epoch/final CAS 与首次 M3 生产正结算及并发/中断/迟到/漂移矩阵。
4. 最终源码/产物上的生产 HTTP 成功/阻塞、生产 Chrome 及独立 OS 进程重启验收。当前局部 blocked 链通过不满足完整 STEP04 live 退出条件。
5. 既有 public fullflow Profile 断言、Orchestrator 累计套件耗时与 build-identity CLI 问题继续独立跟踪；本轮不声明这些问题已解决或全 CI 通过。

未自动提交累计修改。不进入 STEP05，不创建 Run，不使用真实模型/embedding，不连接真实 MCP，不修改 live 配置。
