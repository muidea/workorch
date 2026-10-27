# STEP04 Provider 缓存原观察持久结算与恢复验证记录

日期：2026-10-23。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动。本记录限定于 provider cache-only 历史观察子链及所列验证，不表示完整 STEP04、M3、全 CI 或发布验收通过。

## 本轮收口

此前原检查只持久保存 miss/expired/declaration_only 原因，没有归档其完整 LLM owner 观察。本轮将同一次观察接入原准备操作，不增加任何 provider 查询、refresh 或收费调用。

- `PreparationProviderCacheFacts` 保留完整 `ObserveProviderCatalogCacheResult`：Workspace、随机 owner epoch、观察时间、目录 revision、获取/过期时间、全部模型及 capability 原值。嵌套列表 deep clone，不 normalize 原观察；沿用原 owner 的 4096 模型、1 MiB 及字段有界校验。
- Workspace 独立反查原 accepted 操作和精确封存，将原 binding、request/payload、受理 revision 与事实封装为 `workorch-preparation-provider-cache-evidence/v1`，与原 blocked 结算同一次 CAS 保存。没有另开观察或结算事务，也不凭调用方 verdict、hash 命中或成功标志授信。
- 私有 archive 不进入普通 Workspace JSON、公共 HTTP 或 Web。file wire 以 0600 保存；memory/file/scoped repository 校验引用一一对应、原 scope/revision/binding/时间/原因码及 append-only 历史，禁止删除或 rehash 后覆盖完成材料。
- 公共 v2 blocked attempt 仅增加 `providerCacheEvidenceRef`（`provider-cache-observation:` 加 32 位随机小写 hex）；Go/TS 校验其格式、状态及 required provider_cache/llm 检查。Web 只展示历史引用，缺引用不等于缓存为空。
- miss/expired/fresh 仍分别结算为 miss/expired/declaration_only 阻塞。单点观察不证明持续 freshness、owner epoch 仍活跃、连通性、授权、精确 selection、ready 或 M3。
- GET、精确重放、原操作恢复和 owner 重开不重查 LLM、不刷新、不补造未提交材料。已知 revision/epoch 漂移保留历史，逐项 binding_changed/currentValidity stale，不恢复旧 credit。旧无 archive 的记录可以按原合同读取，不能凭原因码重建材料。

按 go-multi-module-dev 核对，仅扩展既有 Workspace 协调材料、Application typed 接线和 Web 读模型，没有新增运行单元、topic、跨 owner repository/service 调用；Runtime SM-1～SM-4、Start/Restart 执行授权均不变。边界扫描只登记 LLM `ProviderModelCatalog.State` 这个精确点时观察字段，新增 sibling target/desired/stage 状态仍被拒绝。

## 验证与原始索引

受限证据根：`/tmp/workorch-step04-provider-evidence-tAsiwc`，目录权限 0700。原日志和二进制不提交 Git；临时目录不是长期归档，后续独立复核需另行保全。

| 检查 | 结果与索引 |
| --- | --- |
| 受影响 Go 全包 | 22 个有测试包通过、4 个无测试包；`affected-go.log` |
| 原操作切点及实际 LLM owner | `owners-http.log`、最终全包/race：miss/fresh/expired、错误 scope/type、写入失败、落盘回执丢失、revision 漂移及 file-owner 重开 |
| 实际 cache-only 零 HTTP | 真实 LLM owner 指向隔离回环计数服务；miss 保存完整历史，随后停止 LLM owner 再重放/恢复，计数始终 0、无 Run |
| repository 保真及不可变历史 | memory/file/scoped 三后端；`repository.log`、最终 `repository-permissions-final.log`；包含 nested slice clone、原值 reopen、0600、删除/重绑/改 epoch 后 rehash 拒绝 |
| 定向 race | 五个实际运行测试的包通过；`race.log`，repository 权限补强另见 `repository-permissions-final.log` |
| 真实回环 HTTP/file owners | `owners-http.log`、最终全包/race；公共 ref 对应私有原观察，篡改拒绝，GET/check/recover/reopen 不增加缓存读取或创建 Run |
| Web 全量 | 13 文件 147 项组件、11 项规则通过；`web-components.log`、`web-rules.log` |
| 边界、vet、格式、diff | `stateowner.log`、`boundary-final.log`/`boundary-post-build.log`、`vet.log`、`fmt-final.log`、`diff-final.log` |
| 生产 Web 构建 | `web-build.log`；通过，保留既有 chunk 大小告警，内嵌产物已更新 |
| 本地输入/产物绑定 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function.log` |

受影响全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/blocks/llm/... ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/tools/stateownercheck -count=1
```

定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/pkg/repository ./internal/pkg/workorchmanagementcontract -run 'TestPreparationProviderCache|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen|TestPreparationObservation' -count=1
```

模型反例包括重复模型、错误 epoch/state/time/binding/revision、修改后重算 hash；保留 embedding 的零 contextWindowTokens 元数据，不因此要求该字段或授予 ready。HTTP 使用本地 fixture 准备候选，没有真实模型费用、live 配置读取、外部 skill-hub 项目变更或 Run；准备阶段不新增推理/embedding。owner 关闭重开不是独立 OS 进程 stop/start，本轮未执行生产 Chrome 验收。

### 失败与修正

1. `targeted-go.log`：revision_drift 子例失败。首版 archive transition 错误要求结算时 revision 仍为 accepted revision；修正为保存原历史，最终 drift 门禁仍逐项 binding_changed/stale，不放宽 currentness。`targeted-go-corrected.log`、最终全包及 race 通过。
2. `boundary.log`：曾将非状态引用字段加入 state-model inventory，扫描器不将其视为状态声明；移除该错误登记，不改变状态清单要求。
3. `boundary-corrected.log`：完整 catalog 的派生观察 State 被当成目标状态。增加精确 type+field 的观察登记及 sibling authority 反例，不白名单整个 command 或载体。最终 boundary 通过。
4. Go build 输出 module stat-cache 只读告警，退出 0。既有 build-identity CLI verify 问题未修，不记 CLI 通过；使用公开 Python `verify()`，外部固定完整四字段后核验当前输入及产物。未运行全仓 CI。

## 本轮最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终未提交源码，不是单独验证该干净提交。
- buildInputSha256：`891b4d1ee53878c36d0bb9e35a1540470a6f7dea4b128858e98808b6b9fa9b94`
- consoleTreeSha256：`8f7f2aef37fbe991a734acfe2789818d6ab226a111553ea0a40dba1bea32c308`
- binarySha256：`1d229884f3e3a41d907e9770a56e0a73ce2f6a7848678378268bfd31ebf221b3`

此身份只证明本地源码/内嵌控制台/二进制字节一致性，不是可信构建 attestation、部署或验收证明。前轮构建身份不能替代本轮。

## 剩余门槛

1. Skill 辅助引用的唯一来源/root 映射及实际读取、来源状态/freshness、daemon/local 一致性；当前 Registry 只有引用字符串，不能猜选安装路径。
2. MCP/Memory required owner 的真实观察及原操作持久消费；provider 的当前 target/config/capability/授权等完整判定。本轮只补缓存原观察归档，不声称完整 provider readiness。
3. 全 required 合取、no-Run/epoch/final CAS 与首次历史 M3 的生产正结算，及对应竞争/中断矩阵。
4. 生产 Chrome、独立 OS 进程重启证据。完整 STEP04 尚不满足 live 退出条件。
5. 单独跟踪已披露的 public fullflow Profile 断言、Orchestrator 累计套件耗时及 build-identity CLI 问题，不把本轮局部通过写成全 CI 通过。

完整 Start/Restart 准备回执消费属于 STEP05，不在本轮范围。未自动提交改动。
