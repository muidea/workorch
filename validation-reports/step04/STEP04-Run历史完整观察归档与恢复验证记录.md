# STEP04 Run 历史完整观察归档与恢复验证记录

日期：2026-10-26。范围仅 STEP04；基线 HEAD 为 `834aa350cc4e77379819666540b9912a7c22b64c`，验证对象包含累计未提交源码。未提交修改，未运行全仓 CI、收费模型或既有服务的 live 操作。

## 本轮收口与边界

接续 Provider 缓存期限只读失效投影，补齐原 Run owner 观察结果的私有历史归档，不改变 no-Run 判据或授予 M3：

- Application 保留既有 typed `ObserveWorkspacePreparationHistoryResult` 的完整原值：request、观察时间、Workspace scope、generation head、Run/pending/replacement 计数与 retirement fence。这里的完整材料是 owner 原 query DTO，不声称另存每条 Run aggregate。
- Workspace 在原 accepted → blocked 结算 CAS 内追加 `workorch-preparation-run-history-evidence/v1`，绑定原 request/payload、完整 preparation binding、受理后 revision 和 creation/preparation epoch。原材料与 attempt 引用一一对应，clone 隔离、0600 保存、历史不可覆盖或删除。
- 读回重验完整 typed 值、hash、原 scope/revision/时间窗和逐项 required/run/blocked 身份；重新计算 hash 不能洗掉原材料关联或完成历史不可变门禁。不可编码时间、超界或无法保全完整材料的观察不授完整归档引用，不截断或改写原值。
- 有历史、retirement 分别保持 `run_history_present`、`run_history_retired`；空仍为 `run_history_empty_fence_not_final`，不升级 passed/N/A/ready。观察后发生绑定漂移时保留原材料，只以 `binding_changed` 阻塞。
- 公开仅投影随机 `runHistoryEvidenceRef`（`run-history-observation:` 加 32 位小写 hex），Web 显示原阻塞码和引用。不公开原 retirement、内容 hash 或执行权限；缺引用明确不等于无 Run。
- GET、同原请求 check/recover 与 owner/repository 重开不重查 Run、不回填旧无材料历史、不创建 Run。结算写失败保留 accepted 切点；恢复不补造历史。已持久后丢回复通过原身份读回，不重复观察。

沿用 go-multi-module-dev 的 concrete typed EventHub 与 owner 私有 reducer/CAS；没有新运行单元、跨 owner repository/service facade、运行调度或执行授权。SM-1～SM-4 未变。

静态边界对 `WorkspaceRetirementFence.Phase` 的处理只允许精确的 Workspace `SettleWorkspacePreparationBlockedCommand`／Run model／`Phase` 三元组，作为原历史事实归档；不是整个类型或所有命令的豁免。同级 State/TargetPhase/DesiredState，以及其他命令仍拒绝。

现行入口同步至 Workspace readme 与 service-api；本记录不作为设计权威。

## 验证与原证据

受限证据根：`/tmp/workorch-step04-run-history-UOmNfO`（0700）。

| 验证 | 结果及原索引 |
| --- | --- |
| 新增 Go 定向 | 五包通过，`targeted-complete.log`；models/events/biz/repository/managementcontract 的 `TestPreparationRunHistory*` |
| 定向 race | 上述五包、同选择器通过，`race-final.log`；不是全包或全仓 race |
| 受影响全包 | 21 个有测试包通过、2 个包无测试，`affected-go-final.log`；包含真实回环 HTTP fixture |
| 静态工具 | `stateownercheck-final.log` 全包通过，含精确历史字段与同级／其他命令拒绝反例 |
| Web 定向 | TypeScript 检查及 preparation 51 项通过，`web-targeted.log` |
| Web 完整组件 | 13 文件、180 项通过，`web-components.log`；保留已有 Ant Design 警告 |
| Web 规则 | 11 项通过，`web-rules.log` |
| 生产控制台 | TypeScript/Vite 构建通过，嵌入资源已更新，`console-build.log`；保留大 chunk 警告 |
| 静态／格式 | `vet-final.log`、`fmt-final.log` 和 `diff-check.log` 退出 0 |
| 边界 | `boundary-final.log` 通过；保留已有 pycache binary-match 提示 |
| 二进制及身份 | `source-snapshot.json`、`build.log`、`build-identity.json`、`build-produce.log`、`build-verify-function.log`；构建、produce、公开 Python verify() 退出 0 |

受影响全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/modules/blocks/run/pkg/events ./internal/modules/blocks/run/pkg/models ./internal/modules/blocks/run/biz ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

新增回归覆盖：空历史、有 watermark 但可见行零、pending/replacement 计数、retirement 完整字段；scope/request/revision/time/binding/epoch/hash 与缺失／孤儿／重复反例；memory/file/scoped 三后端 clone/CAS/不可变历史；实际 Run owner 空历史和停止 owner 后的重放；fixture 的错类型／错 scope／写失败／丢回复／revision 漂移与重开。HTTP 回归核对公开原引用与重开后的私有完整材料仍相等，Web 核对零自动检查、恢复或执行。

历史非空与 retirement 部分为 typed fixture，不冒充真实历史执行或真实 retirement。owner/repository reopen 在同一测试进程内，不是独立 OS 进程重启；实际回环 HTTP fixture 不是本轮新二进制部署或 Chrome 验收。本轮没有改变 live 配置，没有收费推理或 embedding，也没有创建 Run。

首次 `boundary.log` 拒绝新增的原 retirement Phase；核对为历史观察后增加上述精确三元组审核及反例，最终边界通过，没有跳过 gate。补工具测试时曾缺闭合括号，`stateownercheck.log` 保留编译失败；修正后 `stateownercheck-final.log` 通过。首次完整 Go/race 后又补强不可编码材料检查，最终 `affected-go-final.log`／`race-final.log` 针对补强后源码重跑通过，不借用先前结果。

`build.log` 保留 module stat-cache 只读告警，构建退出 0。未修改既有 build-identity CLI verify，不记为 CLI verify 通过；使用公开 Python verify() 按以下固定四字段核对。

## 最终本地输入／产物身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加累计未提交源码。
- buildInputSha256：`b8a6b6ac66bc620141096f958d9f3b077b29a23d45aee699bc04f7e1d5cf1d42`
- consoleTreeSha256：`4fb4339f247627ea3c8ce1a2549c57d80cb580af985405cff5e3854fa8120156`
- binarySha256：`9e0f56317ece64cb501bb35321b23b9d471b1775ae8862d6658e94391c62260a`

身份只证明本地输入／产物字节一致，不是可信构建 attestation、部署来源或 M3 凭据，不得归到干净 HEAD。前轮二进制不作为本轮源码产物证据。

## 尚未闭合的退出条件

1. Provider 授权、既有 ceiling/预算精确消费、当前 owner epoch 及持续 freshness。
2. MCP required connection/连接 owner 事实与 epoch；空选择合法适用性政策。
3. 非空 semantic Memory 的精确 query/target/预算和真实语义证明；Skill 辅助引用唯一来源/root、真实读取及 freshness。不能猜相对路径或回退原模式。
4. 全 required 合取、no-Run 观察与 reservation/epoch 的最终顺序核验、final CAS、首次 M3 正结算及并发／中断／迟到／漂移矩阵。本轮完整原 query 归档不关闭此门槛。
5. 最终产物生产 HTTP 成功／阻塞、实际 Chrome 和独立 OS 进程 stop/start。
6. 既有 public fullflow Profile 断言、Orchestrator 套件成本及 build-identity CLI 问题仍独立跟踪；未验证全 CI，不宣称完整 STEP04 或 live 验收退出条件已满足。
