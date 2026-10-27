# STEP04 Memory 原模式与精确选择裁决验证记录

日期：2026-10-23。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动。本轮只收口 Memory 原模式观察、有界确定性选择及其单项消费者，不表示完整 STEP04、整体资源 ready、M3、全 CI 或发布验收通过。

## 本轮收口

1. Application 在原 accepted 操作的整体预算内，向 LLM owner 真实查询 active provider、configuration revision 和 defaultEmbedding，在完整 Memory 库存观察前后各读一次，并双读 exact seal。只保存相关无凭据 typed 字段；不读取 live 配置文件，不保存 API key 或密钥派生摘要。首次配置 target 立即 detached clone，避免后读覆盖前像。
2. 新增 `workorch-preparation-memory-selection/v1` 来源，绑定原 Workspace/revision、accepted `ObservationSnapshotHash`、前后配置、库存与观察时序。Workspace 从原 accepted 身份与当前 exact seal 独立核验来源，不接收 caller mode、选择结果或 PASS。
3. 将原 activation 的 `activation-memory-selection/v1` 选择算法机械提取至 `internal/pkg/memoryselection`，原 activation helpers 委托同一纯算法；原 200 候选、8 项、16 KiB、4096 estimated tokens、可见性和类型 quota 不变。独立保留原算法作为测试 oracle，比较完整结果，不以只比较 selected IDs 代替。
4. defaultEmbedding 确实未配置时，按原 list-only 算法编译完整候选、选中、排除、逐项 score/reason、quota、hit 和预算；超过 200 个候选明确阻塞，不截成 200。完整库存仍保留原 256 项/1 MiB 边界。
5. 非空 semantic 库存为 `blocked/memory_semantic_evidence_required`，在可见性或 quota 过滤前判断，不回退 list-only，不调用 embedding。真实空库保留实际 mode，沿用既有 activation 的空库 fast path。完整 list-only 或真实空库可在原裁决版本下单项 passed，但整体 operation 仍 blocked。
6. 配置不可用、错误 DTO、前后 revision/target 漂移均不能当作 nil embedding。已取得的完整库存保留，选择仍 pending；本轮尚未细分这些配置失败的公开诊断，不能据 pending 推断未配置 embedding。
7. 来源、完整库存和 Workspace 纯编译结果随原 blocked CAS 保存。读回从完整原材料重新计算并比较；拒绝 selected-only、替换预算/原文/快照后自洽重哈希及完成历史覆盖。memory/file/scoped 三后端 deep clone config target、完整候选与选中/排除材料。无来源的旧库存不自动升级。
8. 公共 Go/TypeScript 同步约束单项结果的裁决版本、owner/required、随机历史引用和原因；Web 只展示原时点结果，不自动 POST/recover/执行。GET、同键重放、恢复和 owner 重开不重新读取配置或库存，不写 Run recall trace，不新增 Run。

按 go-multi-module-dev 核对，继续沿用 Application typed 延续、Memory/LLM owner 观察和 Workspace 私有 reducer/CAS，无新增运行单元、topic、跨 owner repository/service 或验证成功缓存。`Mode` 登记为纯 derived projection，不是状态迁移权威。Runtime SM-1～SM-4、Start/Restart 授权与 STEP05 行为未改变。

## 验证及原证据索引

受限证据根：`/tmp/workorch-step04-memory-selection-a8sO9k`，权限 0700。原日志和二进制不提交 Git；临时目录不是长期归档。

| 验证 | 结果与原证据 |
| --- | --- |
| 最终受影响 Go 全包 | 24 个有测试包通过、3 个无测试包；`affected-go-final.log`，`-count=1` |
| 模型与算法定向 | `model-rerun.log`；原模式、空库、semantic 非空、201 候选、原文/预算/快照替换、clone、stale 和 legacy |
| 原操作切点 | 最终全包及 `race-final.log`：list/semantic/空库、配置首次/后次失败、错误类型、revision/target 漂移、结算写失败、回执丢失、Workspace revision 漂移、重放/恢复及 file owner 重开；配置和库存读取次数不因恢复增加，零 Run |
| 三后端 | 最终全包及 race；旧库存及新选择历史两组，原 CAS、deep clone、读回重编与完成历史不可改写 |
| 真实回环 HTTP | `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 在最终全包和 race 中执行；真实 Memory/File/Workspace/Soul/Skill/Run owners，typed active-configuration fixture。断言 list-only 原模式、精确 accepted snapshot、原 selected bytes、配置恰好双读及重开不重查；整体仍 blocked |
| 纯算法短时 fuzz | `fuzz.log`，3 秒预算，实际约 4 秒，4322 次执行；独立原算法完整结果等价。不是无限域证明 |
| 定向 race | 6 个实际运行测试的包通过，`race-final.log`；不是全仓 race |
| Web | `web-components.log`：13 文件、151 项组件通过；`web-rules.log`：11 项规则通过 |
| 边界/vet/格式/diff | `boundary-final.log`、`vet.log`、`fmt-final.log`、`diff-final.log` 通过 |
| 生产 Web 构建 | `web-build.log`；通过，保留既有 chunk 大小告警，嵌入产物已更新 |
| 本地二进制及输入身份 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function-final.log` |

最终全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/pkg/memoryselection ./internal/modules/blocks/memory/... ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/biz/appservice/activationports ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/pkg/memoryselection ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/pkg/repository ./internal/pkg/workorchmanagementcontract -run 'TestSharedSelection|FuzzSharedSelectionReference|TestPreparationMemory|TestPreparationWorkflowHTTP' -count=1
```

HTTP 候选准备只使用本地无收费 fixture；active configuration 的模式反例也是 concrete typed fixture，不等于实测生产 LLM 配置或 semantic 模型质量。HTTP/owner 关闭重开在同一测试进程内，不是独立 OS 进程 stop/start。本轮未执行生产 Chrome 或收费 live，不以组件测试替代这些证据。

### 失败与修正

1. `model.log`：新测试的动态 string 未显式转换 MemoryID，legacy 反例保留了其他 File 旧 passed。修正测试类型与全部旧 check 形状后，`model-rerun.log` 及最终全包通过。
2. `boundary.log`：共享算法类型 alias 不符合仓库禁止 facade re-export 规则；删除 alias，helper 直接返回 focused package 类型，不放宽扫描规则。
3. `boundary-corrected.log`：新纯编译结果的 `Mode` 尚未登记；按实际无迁移性质登记为 derived projection，不修改 Runtime 状态机或 topic 数。`boundary-final.log` 通过，仍有既有 `__pycache__` binary-match 提示。
4. Go build 的 module stat-cache 出现只读告警，退出 0。既有 build-identity CLI verify 问题未在本轮处理；使用公开 Python `verify()`，传入固定完整四字段核验，不记 CLI verify 通过。未运行全仓 CI。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终累计未提交源码，并非独立验证该干净提交。
- buildInputSha256：`43d64ac29e0343325108550062e78fbfdd39a2167ca3e176d9c9ebc922b2f8d6`
- consoleTreeSha256：`573b877965cdc4323e0bc0ff6bc73f2d5ba4f052f58e16e9b0eda268749019a7`
- binarySha256：`12f89c2ea4459b225df3e902b46538c1f24cde588c26acd70cdc3f8022281b7c`

这些身份只校验本地完整输入、嵌入控制台和二进制字节一致，不是可信构建 attestation、独立部署验收或 M3 凭据。

## 仍需收口

1. Memory 非空 semantic 必需的精确查询/target/预算/真实语义证明及相应授权；不得通过清空 target 或退回 list-only 绕过。原时点编译也不替代持续 currentness/epoch 核验。
2. MCP registry/required connection，provider 精确 target/config/capability/授权/freshness，以及 Skill 辅助引用唯一来源/root、真实读取和 freshness；保留用户接受的 SkillHub 现行行为，不改外部项目。
3. exact_seal/persistence 等剩余单项消费者、全 required 合取、no-Run 完整原历史证据、reservation/epoch/final CAS 与首次 M3 正结算及并发/中断/迟到/漂移矩阵。
4. 在最终源码/产物上执行生产 Chrome、HTTP 成功/阻塞与独立 OS 进程重启；本轮局部通过不能声明完整 STEP04 live 退出条件已满足。
5. 既有 public fullflow Profile 断言、Orchestrator 累计套件耗时、build-identity CLI 问题继续独立跟踪。

未自动提交累计改动。本轮不进入 STEP05，不创建 Run、不调用真实模型或 embedding。
