# STEP04 精确封存与持久绑定消费者验证记录

日期：2026-10-26。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加累计未提交改动。
本轮完成 `exact_seal`、`persistence_integrity` 的原时点材料消费，不是整体资源 readiness、首次 M3、完整 STEP04、全 CI 或发布验收通过；未自动提交。

## 已实现

1. 原检查首次受理后的正常 owner 延续，执行两次既有精确封存 query：真实正式 Goal/DoD、完整创作来源、六审、候选、StartupInput/admission、配置/目录及完整 all_of 编译门禁保持不变。外部 settlement command 不接受 Seal/PASS；原材料只由 Workspace owner 构造。
2. `workorch-preparation-seal-evidence/v1` 保存两份完整 detached 输入、原 binding/request/payload、受理后精确 revision 与观察时间；两读须完全相等。新增私有 `PreparationSealEvidence`，公开仅投影随机 `sealEvidenceRef`（`seal-observation:` 加 32 位小写 hex）。每份 snapshot 仍限 2 MiB，双份 archive 有显式总字节上限，不以截断后材料授通过。
3. 原 CAS 封存材料并裁决两项：`seal_observed_remaining_checks`、`persistence_binding_observed_remaining_checks`。两项只允许 required/workspace、原 evaluation v1 和整体 blocked 操作；不是其他资源、全 required 合取或 M3 的证明。这里的 persistence 指原 owner 持久绑定完整性，不承诺磁盘永久可靠或跨文件原子性。
4. 独立读回重验完整材料及原 accepted binding：受理前/后 snapshot 的 revision/hash 关系、实际正式行与 materialized journal、原 candidate、原 StartupInput/admission、完整原账本与六审及正式 DoD 顺序。复用既有 `ValidateActivationAdmissionPersistence` / `ConfirmedAuthoringAdmissionSource`；不另造语义 admission 编译器。历史来源从原 authoring history 定位，不以新版本来源替代。
5. Snapshot 的纯 DTO/hash/clone 形状由 model 与 startuppreparation 共享；原精确 query 的独立 Validate 与 all_of 编译不迁走、不删除。没有成功 session、caller 已验证标志或 hash 命中授信入口。
6. 原材料 clone 隔离、file/scoped wire 显式私有保存，纳入 0600 私有文件边界；首次 CAS 后 append-only。stale 可保留实际原观察，但两项仍 blocked，禁止只给其中一项恢复旧 credit；缺材料、两读漂移、中断恢复均不授通过。旧无材料历史保持原结论，GET/replay/recover 不补取、不升级。
7. 公共 Go/TypeScript 合同和页面同步增加引用及两项结论展示；反例拒绝错 owner/required/version/ref、伪 ready 和任意成功原因。页面无自动检查/恢复/执行，无浏览器业务持久化。

按 go-multi-module-dev 核对，沿用既有 Workspace owner query、私有 reducer/CAS 和应用只读投影；没有新增运行单元/topic、跨 owner service/repository 或执行授权。Runtime SM-1～SM-4、Start/Restart、STEP05 不变。现行合同更新于 Workspace readme 和 service-api；本记录不是设计权威。

## 验证与证据索引

受限原证据根：`/tmp/workorch-step04-seal-consumer-b18WKF`（0700）。原日志、身份快照和二进制不提交 Git；临时目录不等于长期证据归档。

| 检查 | 结果与证据 |
| --- | --- |
| 封存专测 | `seal-final.log`、`source-rehash-final.log`；后者在最终测试补强后执行通过，覆盖三后端、来源替换、完整自洽重哈希后仍须反查原账本、clone/owner reopen、历史来源定位、stale/interrupted/missing/两读不等 |
| 定向 HTTP/公共形状 | `targeted-corrected-final.log`：Workspace biz、application biz、managementcontract 三个实际命中测试的包通过，另三个指定包无匹配测试；包含四条真实回环 HTTP 正确单项结论及整体 blocked，不虚列六包均执行了测试 |
| 受影响全包 | `affected-go-final.log`：18 个有测试包通过、2 个包无测试，`-count=1`；不是全仓测试 |
| 准备路径 race | `race-final.log`：6 包通过，Workspace biz 为 203.747 秒；不是全仓 race，不是包超时或阻塞的证明 |
| 最后来源反证补强 race | `source-rehash-race-final.log`：Workspace biz 的两项 Seal 测试通过。最后只增加了自洽重哈希反例的测试断言，未再修改生产代码；全包/宽 race 在该测试补强前已启动，最后断言另行普通/race 执行通过 |
| Web 定向 | `web-targeted.log`：TypeScript 和准备组件 47 项通过 |
| Web 全量 | `web-components.log`：13 文件、176 项组件通过；`web-rules.log`：11 个 Node 规则测试文件入口通过。保留既有 AntD deprecated 警告 |
| 静态/边界/格式 | `vet-final.log`、`boundary-final.log`、`fmt-final.log`、`fmt-final-confirmed.log`、`diff-final.log`，均退出 0；保留既有 pycache binary-match 提示 |
| 生产控制台 | `web-build.log`，`make console-build` 通过并更新实际嵌入资源，保留 chunk size 告警；不等于 Chrome 验收 |
| 最终二进制/输入身份 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function-final.log`；构建、produce、公开 Python verify() 均退出 0 |

最终受影响全包：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

准备路径 race 使用以下六包与 `-race -run 'TestPreparation' -count=1`：

```text
./internal/modules/kernel/workspace/pkg/models
./internal/modules/kernel/workspace/pkg/startuppreparation
./internal/modules/kernel/workspace/biz
./internal/modules/application/workorchd/biz
./internal/pkg/repository
./internal/pkg/workorchmanagementcontract
```

### 证据边界

- `TestPreparationSealOriginalOwnerMaterialCASAndReopen` 从真实 Workspace owner 的既有确认/准入路径准备固定来源，随后核对 memory/file/scoped 三后端原 CAS 与不可变历史。历史来源分支的额外构造只验证原账本查找，不声称完成新 successor 准入。
- `TestPreparationSealStaleInterruptedAndMissingNeverGrantCredit` 核对 revision 漂移保留原材料但零 credit，中断/缺材料不授通过，两读不等被拒绝。
- 既有四条 `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 仍经真实回环 HTTP 与 File/Workspace 等 owners，新增两项精确原因和引用断言；原资源通过、Provider/MCP/no-Run 等阻塞及零 Run 边界继续保持。候选准备使用固定非敏感 fixture，不是收费模型质量验证。
- owner/repository/HTTP reopen 均在同一测试进程内，不是独立 OS 进程 stop/start。组件测试不是实际 Chrome 故障注入。本轮未启动新二进制的独立生产服务/Chrome，未读取 live 配置，未执行真实推理/embedding/MCP，不重跑全仓 CI。

## 失败与修正

1. `targeted-initial.log`：新消费者已取得真实封存材料，但旧全 blocked 断言仍将两项单项通过视为错误。改为精确核对两项 owner/reason/ref；保留整体 blocked 和其他原资源/阻塞断言，没有删除路径或放宽验收。
2. `seal-first.log`：新测试误用了不存在的账本字段；修正为真实 `SpecificationRequests` 六审来源反例，未改生产合同。
3. `targeted-final.log`、`affected-go.log`、`race.log`：新独立反证将排序后的第一项（directory）误当 exact_seal，因而未触达 Seal 的 owner 反查。改为按稳定 check ID 修改；没有扩大 Seal 对其他 owner 的权限。最终相应回归通过。
4. 复核补强了新 archive 的 v2/原 evaluation 形状、受理前后完整 snapshot 关系、不可表示目录拒绝和两项 mixed stale 禁止授信；相关最终定向、全包、race、边界和构建在生产修正后执行。
5. 最终构建退出 0，保留 module stat-cache 只读告警。既有 build-identity CLI verify 问题未混入本轮；使用公开 Python verify() 与固定四字段核对，不记为 CLI verify 通过。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮累计未提交源码。
- buildInputSha256：`28955239dfd4ceb714ebcbe229ca06a00dc7b3e3430f7748afd58cd2181690b7`
- consoleTreeSha256：`ee2d51fd9b5da1283f1ec66f3b2ef6c29e0d07e0298d6eb89181fc4fb33721e8`
- binarySha256：`032589834353176936dc574a0cbb8c661fbcd31f4585c4ae2ea28b51daf0429d`

verify() 使用上述固定四字段，不从当前树自推期望。身份只核对本地构建输入、实际嵌入控制台和二进制字节，不是可信构建 attestation、部署来源或 M3 凭据；不能归到干净 HEAD，也不能沿用上轮二进制证明本轮源码。

## 剩余与下一顺序

1. Provider 授权、既有预算/ceiling 精确消费和持续 freshness/current epoch；MCP required connection 与连接 owner 事实/epoch、空选择合法适用性政策。
2. Memory 非空 semantic 精确 query/target/预算/真实语义证明；Skill 辅助引用唯一来源/root、真实读取及 freshness。不伪造证明、不回退原模式、不改外部 SkillHub。
3. 全 required 合取、完整 no-Run 材料及 reservation/epoch/final CAS、首次 M3 正结算和并发/中断/迟到/漂移矩阵。原时点两项 passed 不能代替这些门槛。
4. 最终源码/产物上的生产 HTTP 成功/阻塞、实际 Chrome、独立 OS 重启验收。完整 STEP04 仍未满足 live 退出条件。
5. public fullflow Profile 断言、Orchestrator 套件成本、build-identity CLI 等既有独立问题继续跟踪，不据本轮通过宣称全 CI 绿。
