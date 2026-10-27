# STEP04 逐项裁决与原事实消费验证记录

日期：2026-10-23。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动。本轮完成 File 三类及 Soul 的版本化单项消费者，不表示完整 STEP04、整体资源 ready、首次历史 M3、全 CI 或发布验收通过。

## 本轮收口

此前原操作能保存完整资源观察，但完整事实一律仍以 blocked 展示。本轮增加 `workorch-preparation-check-evaluation/v1`，先接通能够从已有完整原材料独立复验的消费者，不以 helper 成功标志替代证明，不新增资源外发或执行授权。

1. 原 accepted 操作封存裁决版本、完整清单及目录／托管配置／规格资源的精确 `SubjectHash`。该 hash 只绑定原 manifest requirement、两目录及资源引用／原 expected hash，不包含配置 bytes 或密钥，不作为成功凭据，不进入公共 DTO。结算只允许改 check 的 State／Reason，不能换 owner、required、对象身份或版本。
2. File 单项裁决：完整精确目录及有效权限、原托管配置策略、规格资源原 hash 满足时为 passed；实际观察到目录权限不足、配置格式／内容冲突、资源内容改变时为 failed，保留原原因。材料缺失、不完整、原 hash 未知仍 blocked。没有泛用 PASS、猜选路径或默认 N/A。
3. 托管配置在原 CAS 前另从确认模板和 Workspace 原前像独立重编完整声明，比较数量、scope/path/policy 和 DefaultsJSON。空集、子集、替换默认值或策略后重哈希仍拒绝。scoped repository 的 Soul sidecar 不在 metadata 内；这里不从不完整 metadata 重算整个 Workspace 配置身份，原完整配置绑定和最终 revision/epoch fence 保持不变。
4. Soul 单项消费完整六阶段选择、原配置身份、完整前后 typed records、原 request/payload/revision，以及 accepted 后精确 snapshot。该 `ObservationSnapshotHash` 在受理时由同一 detached 正式 Goal/DoD 和原 owner 前像纯编译，区别于受理前 manifest snapshot，不在恢复时猜补。任一阶段 unbound 仍 blocked，完整事实保留，不自动授 PASS/N/A。
5. 私有结算将原 archive 和单项结果同一次 CAS 保存。repository 读回从完整原材料纯复算，三后端拒绝换对象、改 snapshot、缺材料授信和完成历史覆盖。GET／重放／恢复／owner 重开不重新观察，也不把旧 blocked 历史升级成通过。binding_changed 和中断恢复不授单项通过。
6. 公共 Go／TypeScript 同步校验版本、单项 owner/required/reason、原历史引用和整体状态。Web 分开显示 passed／failed／blocked，并明确“原检查时点的单项结论；passed 不代表当前仍有效、整体 ready 或 M3”。公共形状校验不是 owner 证明校验。

整体 operation 仍只能按本版 accepted → blocked 结算，currentValidity 仍独立为 not_evaluated／stale。单项 passed 不证明持续 currentness、跨 owner 原子快照、全 required 合取或首次 M3。

按 go-multi-module-dev 核对，沿用现有 Workspace 私有 reducer/CAS、Application 延续和资源 owner typed EventHub 分工；没有新增运行单元或 topic，没有跨 owner repository/service 调用。Runtime SM-1～SM-4、Start/Restart 执行授权不变；本轮不进入 STEP05，不读取 live 配置、不改外部 SkillHub。

## 验证与证据索引

受限证据根：`/tmp/workorch-step04-evaluation-evidence-UfU9cL`，权限 0700。原日志、诊断和二进制不提交 Git，临时目录不等于长期归档。

| 检查 | 结果与原证据 |
| --- | --- |
| 最终受影响 Go 全包 | 17 个有测试包通过、2 个无测试包；`affected-go-corrected-complete.log`，`-count=1` |
| 单项模型反证 | 最终全包及 `soul-evaluation-corrected.log`：完整 bytes、原 requirement、scope／ref、错 owner／required／版本、改内容后重哈希、缺 archive、伪 ready/N/A、stale/legacy、完整配置声明、Soul snapshot／双读／缺阶段／unbound |
| 原操作与三后端 | 最终全包／race：File／Soul 真实 owner、原结算写失败／回执丢失／revision 漂移、GET／重放／恢复／file reopen；memory/file/scoped 原 CAS、snapshot 不可替换、完成结果不可改写、clone |
| 真实回环 HTTP/file owners | 最终全包／race 包含 `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen`；File 三类及 Soul 单项通过，其他项和整体仍 blocked；移开原资源后读回／重放／恢复／owner 重开不重取，私有原文不泄漏，准备阶段不新增推理、embedding 或 Run |
| 定向 race | 5 个实际运行测试的包通过；`race-corrected-complete.log`，不是全仓 race |
| Web | 13 文件、150 项组件通过；`web-components-corrected-final.log`。11 项规则通过；`web-rules-complete-final.log` |
| 边界／vet／格式／diff | `boundary-complete-final.log`、`vet-corrected-final.log`、`fmt-corrected-final.log`、`diff-corrected-final.log` |
| 生产 Web 构建 | `web-build-complete-final.log`；通过，保留既有 chunk 大小告警，嵌入产物已更新 |
| 本地输入／产物身份 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function-final.log` |

最终全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/pkg/repository ./internal/pkg/workorchmanagementcontract -run 'TestPreparation(CheckEvaluation|ManagedEvaluation|PerCheck|SoulJudgment|Evaluation|SoulEvidence|Directory|ManagedConfigs|Resource|WorkflowHTTP)' -count=1
```

关键新增测试：

- `TestPreparationCheckEvaluationExactRawAndNoAggregateSuccess`
- `TestPreparationCheckEvaluationFailureStaleAndLegacy`
- `TestPreparationManagedEvaluationRequiresOriginalCompleteDeclarations`
- `TestPreparationCheckEvaluationSoulRequiresExactAcceptedSnapshotAndFullDoubleRead`
- `TestPreparationEvaluationRepositoryOriginalCASAndImmutableReadback`
- `TestPreparationPerCheckJudgmentShapeNeverGrantsReadiness`
- `TestPreparationSoulJudgmentRequiresVersionAndOriginalReference`

HTTP 使用无收费本地 fixture 准备候选，不使用真实模型或 live 配置。owner／HTTP transport 在同一测试进程内关闭重开，不是独立 OS 进程 stop/start。本轮未执行生产 Chrome 验收；组件测试不能替代该证据。

### 失败与修正

1. `initial-go.log`：旧测试将所有单项 blocked 等同“没有整体 ready”；改成精确单项结果断言，保留整体 blocked 和全部 required 检查。
2. `affected-go.log`：listener 被 sandbox 拒绝；获准后使用隔离回环 fixture 继续。`boundary.log` 因默认 GOCACHE 只读失败，改用 `/tmp/workorch-go-cache` 后边界检查通过。
3. `affected-go-network.log`、`http-isolated.log`、`http-diagnostic*.log`：初版配置声明 gate 从 scoped metadata 重算完整配置身份，漏掉独立 Soul sidecar，导致有效结算 409。修正为原完整 binding 加独立完整声明比较；`http-corrected.log`、最终全包及 race 通过。临时阶段码／布尔诊断已移除，没有输出原配置或凭据。
4. `soul-evaluation-final.log`：新增 unbound 反例暴露重验将有效未绑定事实改为泛化原因，导致 archive 原因关联不一致；修正为保留原完整观察原因且 blocked。`affected-go-complete-final.log`、`race-complete-final.log` 中旧断言仍预期 unbound passed，随后更新精确预期；最终完整回归和 race 通过，未删除该场景。
5. `web-components-final.log`：新增无效 ready 反例未显式作负向输入类型转换，TypeScript 拒绝编译；修正测试输入表达后 `web-components-corrected-final.log` 的 150 项通过。
6. Go build 有 module stat-cache 只读告警但退出 0。既有 build-identity CLI verify 问题未在本轮修复，不记 CLI 通过；使用公开 Python `verify()`，固定完整四字段核验本地字节一致性。边界脚本保留既有 `__pycache__` binary-match 提示，末尾明确通过。未运行全仓 CI。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终未提交源码，不是单独验证该干净提交。
- buildInputSha256：`0154bc584e5cc33deb3b4d4ab319fd1c10dd33a633681c50e7e57f8e77d7ab9f`
- consoleTreeSha256：`43ed997b4c2174cbd75d90a4c3ee4c763114d11f53d670e98d991e3288eb74cc`
- binarySha256：`b04de7483263e93fae8d859d50088e94a88e7371ce4d62b9350620db3ad089e6`

该身份仅核验本地源码、控制台和二进制字节，不是可信构建 attestation、独立部署验收或 M3 证明，不能沿用前轮身份替代本轮。

## 后续收口顺序

1. 继续消费原事实：Memory 原模式／精确选择／semantic 必需证据；MCP 完整 registry／required connection；provider 精确 target/config/capability/授权/freshness。不能用库存、引用或 cache declaration 代替通过。
2. 完成 Skill 辅助引用唯一来源/root、真实读取、状态/freshness 及 daemon/local 一致性，保留用户接受的 SkillHub 现行行为，不在 Workorch 中猜安装路径或改外部项目。
3. 接入 exact_seal／persistence 等剩余单项消费者及全 required 合取，再完成 no-Run 完整历史观察、reservation/epoch、final CAS 和首次 M3 生产正结算；补齐并发／中断／迟到／漂移矩阵。
4. 在最终源码和产物身份上完成生产 Chrome、真实 HTTP 成功／阻塞展示和独立 OS 进程重启；未完成前不称完整 STEP04 已达到 live 退出条件。
5. 已披露的 public fullflow Profile 断言、Orchestrator 累计套件耗时、build-identity CLI 问题单独跟踪，本轮局部通过不代表这些已关闭。

未自动提交累计改动。STEP05 完整 Start/Restart 回执消费仍排除在本轮范围外。
