# STEP04 Provider 缓存期限只读失效投影验证记录

日期：2026-10-26。范围仅 STEP04；基线 HEAD 为 `834aa350cc4e77379819666540b9912a7c22b64c`，验证对象包含累计未提交源码。未提交修改，未运行全仓 CI。

## 本轮收口

接续《STEP04-精确封存与持久绑定消费者验证记录》，补齐已知 Provider 缓存期限的只读失效消费：

- Workspace 状态投影以最新操作的 `ProviderCacheEvidenceRef` 定位完整原材料，并核对原 request/payload、Workspace/revision、binding、时间、hash 与一一对应关系。缺失、重复或冲突拒绝读取。
- 到达原 catalog `ExpiresAt`（含等于）仅投影 `currentValidity=stale`；原 catalog state、检查 reason、blocked 操作、request、引用和历史不变。
- 未过期仍是 `not_evaluated`，不签发 PASS、ready、M3 或权限；cache miss 无期限时不虚构到期日。
- 只读取最新操作自己的材料。已过期前驱不得覆盖没有新观察的 accepted 后继；旧无材料记录不补造观察或期限。
- GET、重放、恢复与 owner 重开均不重查 LLM/Run，不刷新 catalog、不写新账本、不启动 Run。未写入 attempt 的全 required 最早 `ExpiresAt`，该合取尚未实现。

落点为 Workspace models 的纯期限投影与现有状态查询；没有新增运行单元、EventHub topic、跨 owner facade、公共字段或 Runtime 行为。SM-1～SM-4 均未变。按 go-multi-module-dev 约束保留 owner 边界，不把缓存未到期提升为资源可用性。

## 验证及原证据

受限证据根：`/tmp/workorch-step04-provider-expiry-iVoS2T`（0700）。

| 验证 | 结果及原索引 |
| --- | --- |
| 定向回归 | `targeted.log`，Workspace models/biz 两包通过；`TestPreparationProviderExpiry*` 和 `TestPreparationState*` |
| 定向 race | `race.log`，上述两包及相同选择器通过；不是全包或全仓 race |
| 受影响全包 | `affected-go.log`，18 个有测试包通过，2 个包无测试；Workspace、Application biz/activationcore/server、repository、managementcontract、workorchrepo、events |
| 静态检查 | `vet.log`，Workspace 全树、Application biz、managementcontract 通过 |
| 格式/差异 | `fmt.log` 和最终 `git diff --check` 通过 |
| 边界 | `boundary-final.log` 通过；保留已有 pycache binary-match 提示 |
| 二进制及身份 | `source-snapshot.json`、`build.log`、`build-identity.json`、`build-produce.log`、`build-verify-function.log`；构建、produce、公开 Python verify() 均退出 0 |

受影响全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

新增 owner 测试覆盖 fresh 的截止前/等于/截止后、原已过期、cache miss、八类缺失/重复/绑定/hash 反例、file owner 重开、真实查询和原操作重放/恢复，以及无新观察的后继隔离；整个查询/恢复增量 owner 观察调用为零、Run inventory 为零。测试中的时间边界由显式传入纯投影时间控制，没有等待真实 TTL、没有收费调用。

owner reopen 是同一测试进程中的 repository/owner 重开，不是独立 OS 进程重启。受影响 Application 回归包含既有真实回环 HTTP fixture，但本轮新增期限边界不是生产浏览器或新二进制部署验收。Web 源码/资源本轮未改，未将上一轮 176 项组件测试重新记为本轮执行；二进制直接嵌入现有生产控制台字节。

首次 `boundary.log` 因未指定 GOCACHE，默认缓存目录只读而退出；显式使用 `/tmp/workorch-go-cache` 的 `boundary-final.log` 通过。`build.log` 保留 module stat-cache 只读告警，构建退出 0。没有修复既有 build-identity CLI verify 问题，不记为该 CLI 通过。

## 最终本地输入/产物身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加累计未提交源码。
- buildInputSha256：`a0f0ac0460576ab7112199d43a81588f2dd5169fcaf1a2d62d4ae6336584a3cc`
- consoleTreeSha256：`ee2d51fd9b5da1283f1ec66f3b2ef6c29e0d07e0298d6eb89181fc4fb33721e8`
- binarySha256：`f5b9425816688a84fabba03cf5f6bac22917b90c46c878351f860d68a947cc53`

公开 Python verify() 使用以上固定四字段核对。它仅证明本地字节一致，不是可信构建 attestation、部署来源或 M3 凭据；不能归到干净 HEAD。上一轮 Seal 增量二进制不作为本轮源码的产物证据。

## 尚未闭合的退出条件

1. Provider 授权、既有 ceiling/预算精确消费、当前 owner epoch 及持续 freshness。本轮仅消费原到期日，不是完整 currentness。
2. MCP required connection/连接 owner 事实及 epoch；空选择的合法适用性政策。不能以 registry membership 代替连接事实。
3. 非空 semantic Memory 的精确 query/target/预算与真实语义证明；Skill 辅助引用的唯一来源/root、真实读取与 freshness。不得回退原模式或伪造证明。
4. 全 required 合取、完整 no-Run 材料与 reservation/epoch/final CAS、首次 M3 正结算及并发/中断/迟到/漂移矩阵。
5. 最终产物生产 HTTP 成功/阻塞、实际 Chrome 和独立 OS 进程重启验收。
6. 既有 public fullflow Profile 断言、Orchestrator 套件成本、build-identity CLI 问题仍独立跟踪。本轮不宣称全 CI 绿、整个 STEP04 完成或已满足 live 验收退出条件。
