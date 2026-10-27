# STEP04 Memory 完整库存持久结算与恢复验证记录

日期：2026-10-23。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动。本记录仅覆盖 Memory 完整本地库存观察及其原操作持久结算子链，不表示完整 STEP04、资源 readiness、首次历史 M3、全 CI 或发布验收通过。

## 本轮收口

此前 Memory 的普通分页查询不足以证明完整库存，activation selection 路径还可能执行 embedding。本轮增加专用只读观察，不复用有副作用的选择路径，也不把本地库存当作模式或选择结论。

- Memory owner 定义 `/memory/startup-preparation/inventory/query` 和 concrete typed command/result，仅接受 `application/workorchd` 来源。在既有 Workspace mutation lock 与删除 fence 内执行一次 repository 完整读取，不分页、不筛掉异常行、不执行 search、embedding、provider 调用或写入。
- `workorch-preparation-memory-inventory/v1` 保留 Workspace、随机 owner epoch、观察时间及全部 MemoryItem 原值，包括正文、摘要、Tags、Vector 和原分数。列表按 ID 排序；nil 行、跨 scope、重复 ID、无效类型/UTF-8、非有限浮点或超过 256 项/1 MiB 等边界时整体拒绝，不将部分结果标为完整。真实空库显式返回完整空数组，不因此授予 ready。
- Application 在首次显式准备检查内作一次有界 typed 观察，核验 scope、完整合同及本次时间区间。GET、精确重放和原操作恢复不重新读取 Memory。错误、缺失或无效材料明确为 `memory_inventory_unavailable_not_saved`；有效完整库存仍为 `memory_inventory_observed_selection_pending`。
- Workspace 反查原 accepted 操作、受理 revision 与精确封存，封装完整 `workorch-preparation-memory-inventory-evidence/v1` 历史材料；与原 blocked 结算在同一次 CAS 内追加。引用与 archive 一一对应，绑定原 request/payload、binding、scope/revision、时间和 required memory_selection/memory 检查。已知漂移保持历史但逐项 `binding_changed`、currentValidity stale，不恢复旧 credit。
- memory/file/scoped repository 校验历史 append-only，禁止删除、重绑、改 epoch 后 rehash 覆盖；全部嵌套材料 detached clone。私有 archive 不进入普通 Workspace JSON，file wire 按 0600 保存。
- 公共 v2 blocked attempt 只投影 `memoryInventoryEvidenceRef`（`memory-inventory-observation:` 加 32 位随机小写 hex），Go/TS 同步严格校验状态、格式、required owner/reason。Web 只显示历史引用，缺引用不表示无记忆、选择完成或资源就绪。

按 go-multi-module-dev 核对，沿用既有 Memory Block、Workspace/Application Module 及 typed EventHub 边界，没有新增运行单元或跨 owner repository/service 调用。新增一个有登记来源的只读 topic，合同基线 324 → 325。Runtime SM-1～SM-4、Start/Restart 执行授权不变；本轮不进入 STEP05。

## 验证及证据索引

受限证据根：`/tmp/workorch-step04-memory-evidence-DfwAhu`，目录权限 0700。原日志、诊断及二进制不提交 Git；临时目录不等于长期归档，独立复核前须另行保全。

| 检查 | 结果与索引 |
| --- | --- |
| 最终受影响 Go 全包 | 21 个有测试包通过、3 个无测试包；`affected-go-final.log` |
| Memory owner / 模型 | `owner-targeted.log`、`memory-final.log` 及最终全包：完整/真实空库存、nil/错 scope/重复/UTF-8/条数/字节/vector 边界、取消、repository 错误、错误来源、删除 tombstone、drain、epoch 与 deep clone |
| 原操作切点和恢复 | `archive-final.log` 及最终全包：valid/empty、错误 scope/type/time、写入失败、落盘回执丢失、revision 漂移、replay/recover/reopen；历史观察计数保持 1，不新增 Run |
| repository 私有材料 | 最终全包/race 中 memory/file/scoped：0600、完整 reopen、嵌套 clone、删除/重绑/改 epoch 或原因后 rehash 拒绝 |
| 真实回环 HTTP/file owners | `http-final.log` 及最终全包/race；真实 Memory owner 保存正文/vector，检查后删除当前项，GET/check/recover 和 owner 重开仍保留原历史，不泄漏正文/epoch，不新增推理/embedding/Run |
| 定向 race | 6 个实际运行测试的包通过；`race-final.log`：Memory biz/models、Workspace biz、Application biz、repository、management contract |
| Web | 13 文件 148 项组件、11 项规则通过；`web-components.log`、`web-rules.log` |
| 边界、vet、格式、diff | `boundary-final.log`、`vet-final.log`、`fmt-final.log`、`diff-final.log` |
| 生产 Web 构建 | `web-build.log`；通过，保留既有 chunk 大小告警；嵌入产物已更新 |
| 本地输入/产物绑定 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function.log` |

最终全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/blocks/memory/... ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

关键定向测试：

- `TestPreparationMemoryInventoryCompleteOrRejectWithoutFiltering`
- `TestPreparationMemoryInventorySourceDrainAndOwnerEpoch`
- `TestPreparationMemoryInventoryRawCloneAndClosedIdentity`
- `TestPreparationMemoryInventoryArchiveOriginalCutsAndReopen`
- `TestPreparationMemoryInventoryRepositoryPrivateCloneAndImmutableHistory`
- `TestPreparationMemoryInventoryReferenceNeverGrantsReadiness`
- `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen`

HTTP 使用无收费本地 fixture 准备候选，没有调用真实模型或读取 live 配置。owner 关闭重开不是独立 OS 进程 stop/start；本轮未执行生产 Chrome 验收。组件测试不能替代这些待完成证据。

### 失败与修正

1. `archive-targeted.log`：初版 observer fixture 构造参数不符，修正为现有两参数接口后继续验证。
2. `archive-corrected.log`、`archive-diagnostic*.log`、`http.log`、`http-diagnostic.log`：Application 曾复用普通 `error` 接口变量接收 `Result.Get()` 的 `*def.Error`，typed nil 装箱导致有效库存误判。改为独立 `getErr` 接收，不放宽结果校验；`archive-final.log`、`http-final.log`、最终全包和 race 通过。
3. `race.log`、`vet.log`：新增删除 fence 测试复用错误变量造成编译类型冲突，改独立变量；`race-final.log`、`vet-final.log` 通过。
4. Go build 输出 module stat-cache 只读告警，退出 0。既有 build-identity CLI verify 问题未在本轮修复，不记 CLI 通过；本轮使用公开 Python `verify()`，以外部固定四字段核验当前本地源码、控制台和二进制。未运行全仓 CI，不声称全 CI 通过。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终未提交源码，不是单独验证该干净提交。
- buildInputSha256：`b162b479ff42082534ce27ad034c1caf30632bfcbaca5e0bbbff619ac107c65f`
- consoleTreeSha256：`0a5e1aaf7a60c34dc11387accafc3e7d88cf195286661ed1197d30766293eb53`
- binarySha256：`5f58ef323748401ec8ba5fe86c04c328a45ea648cfaeaba462a7ae1e5cedb926`

该身份只核验本地输入与产物字节一致性，不是可信构建 attestation、独立部署验收或 M3 证明，不能沿用前轮身份代替本轮。

## 剩余门槛

1. Memory 原模式、精确选择与 semantic 所需证据的独立消费。本轮只有完整库存历史，不改成 list-only，不授 selection/currentness/ready。
2. MCP registry 与 required 连接事实的真实 owner 观察、原操作持久消费；provider 当前 target/config/capability/授权等完整判定。
3. Skill 辅助引用唯一来源/root 映射及真实读取、状态/freshness、daemon/local 一致性，不能凭 Registry 引用字符串猜安装路径。
4. 全 required 合取、no-Run/epoch/final CAS 与首次历史 M3 生产正结算，以及竞争/中断矩阵。
5. 生产 Chrome 和独立 OS 进程重启证据；完整 STEP04 仍未达到 live 退出条件。
6. 单独跟踪已披露的 public fullflow Profile 断言、Orchestrator 累计套件耗时和 build-identity CLI 问题，不把局部通过写成全 CI 通过。

未自动提交累计改动。完整 Start/Restart 准备回执消费仍属 STEP05。
