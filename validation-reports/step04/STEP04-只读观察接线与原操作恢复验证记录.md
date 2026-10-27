# STEP04 只读观察接线与原操作恢复验证记录

日期：2026-10-22。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前未提交源码。
本记录不是设计权威，也不表示完整 STEP04、M3、全 CI 或发布验收通过。

## 本轮已完成

将既有 provider cache-only 与完整 Run 历史两个真实 owner 查询接到显式检查生产流程，而非继续保留为独立子用例：

1. Workspace exact-seal/manifest 反查和首次 CAS 持久接纳原 request/revision/payload、封存绑定与 epoch。
2. 只有初始 CAS 明确成功的 Application 调用得到非持久延续提示。提示不是凭据，不进入公共 HTTP，也不由调用者提交回 owner。
3. Application 在 owner 事务之间调用 LLM cache-only 与 Run retirement-aware 完整历史 typed query；每项至多两秒、组合至多五秒，不持 Workspace 锁执行 I/O，不 retry/fallback。
4. 新内部 state-lane `settle-blocked` 只接受 exact Application source、原意图及有界负向分类。Workspace 私有 reducer/CAS 再核 revision/epoch，记录阻塞；没有资源 PASS、N/A、ready 或 M3。
5. GET、重复提交、显式恢复和 owner 重开不重跑观察。原请求取消或接纳/结算写入丢回复时保留原身份；恢复只收口原中断，不使用后台 context 偷交、不新建身份。
6. 观察期间 revision/epoch 漂移时全部仍为 `binding_changed`/stale。恢复先完成后到达的观察不改写原终态。
7. HTTP/Web 展示具体缓存/历史原因；前后端 validator 拒绝正向伪状态、错误 checkID 的原因码。内嵌生产 Web 已重建。

provider 负向分类为 cache miss、expired、unavailable 或 declaration only；Run 分类为 unavailable、retired、history present 或 empty/final-fence-not-proven。
它们是业务阻塞原因，不是原始 owner 结果的替代归档或可消费 evidence。当前路径不归档完整查询 DTO、不给 observation-ref 成功凭据；原 owner 的完整缓存/历史仍走原查询合同。不能根据分类重建资源证明。

本轮未接入 SkillHub status：该命令可能保存检查状态，不能套用无外部效果查询的中断恢复规则。保留用户已接受的现行 SkillHub 行为，不修改外部 skill-hub 项目。

## 验证与原始证据索引

受限本地证据目录：`/tmp/workorch-step04-observation-fODO8x`。未将运行二进制、原始日志、配置或凭据提交到 Git。临时目录不是长期归档，可能被系统清理。

| 项目 | 实际结果及证据 |
|---|---|
| Workspace 定向 | `workspace-targeted.log`：workflow/observation 通过；补充取消场景后的 `workspace-observation-final.log` 通过 |
| 真实完整生产调用 | `TestPreparationObservationWholeApplicationUsesActualResourceOwners`：Application + Workspace/file + 实际 LLM/Run receiver，缓存 miss/空历史准确阻塞，Run inventory 仍为零；不是模型质量或 M3 正例 |
| 分类/边界 | miss/fresh/expired、历史 watermark/retirement、错误 scope/DTO；先持久接纳再读取，每次仅两次查询；重复/GET/恢复不增读 |
| 故障与恢复 | 接纳丢回复、结算写失败、结算丢回复、观察期漂移、请求取消、文件 owner 关闭后重开、迟到结算；无重复查询或历史覆盖 |
| 完整包回归 | `affected-go.log`：Workspace biz/models/events/startuppreparation、repository 根/memory/file、managementcontract、公共 events，共九包通过；`application-server.log`：Application biz 与 server 两包通过。新增取消反例随后另跑最终定向/race 通过 |
| resource owner | `resource-owners.log`：LLM/Run biz 的 `^TestPreparation` 定向通过；无真实收费请求 |
| race | `race-final.log`：Workspace `TestPreparation(Observation\|Workflow)` 最终定向通过 |
| 真实 HTTP | `http-workflow.log`：`TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 通过。真实 HTTP transport/ports/Workspace/File/Decision/Run owner；缓存读取与 authoring provider 使用 fixture。新增准备阶段 cache-only fixture 读取一次；GET/check 重放/recover/owner 重开不再读取 |
| Web | `web-components.log`：13 文件、137 项组件测试通过；`web-rules.log`：11 项规则测试通过 |
| 静态与结构 | `vet.log` exit 0；`fmt.log`、`boundary-final.log` 通过；`git diff --check` 通过；新增一条实际 topic，inventory 从 320 到 321，有 typed message、owner 和 state lane 登记 |
| 生产构建 | `console-build.log`、`build.log` exit 0；Web 保留既有大 chunk 警告；Go 输出只读 module stat-cache 警告但构建成功 |

HTTP fixture 创作准备阶段仍有八次固定非敏感 provider-document 调用；STEP04 新增推理/embedding 为零，不启动 Run。该 HTTP 用例是同测试进程中服务/owner close/reopen，**不是独立 OS 进程重启或生产浏览器验收**。

## 本轮源码与产物内容身份

使用 `build_identity.py capture` → `go build` → `produce`；构建前后输入字节核对通过。
`sourceCommit` 仅标识基线，实际构建包含本轮及先前累积未提交改动，不是干净 HEAD 构建。

| 身份 | 值 |
|---|---|
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` + dirty source |
| buildInputSha256 | `dd117ac840e978cdea15651c125d932e85d4ac26b1b7f88f4774013d37571227` |
| consoleTreeSha256 | `dcd66c7c565e5b3b9ccce3d80040c05ae09c8db43ea29b5b2a9210e9ba2fd0b0` |
| binarySha256 | `80ad54805203f4cc3e95347d240d0d62800bfe69df37f50eb82b59beedc3d5e1` |

原文件：`source-snapshot.json`、`build-identity.json`、`identity-produce.log`、`workorch`。
这里只是本地源码/控制台/二进制内容一致性，不是可信 attestation、部署证明或新二进制的实运行证据；不声称 CLI verify 通过。

## 仍未完成与后续顺序

1. 真实 File owner 目录/权限/托管配置/规格资源观察，以及对应的严格当前性和只读边界。
2. 将既有 Soul/Skill 选择与当前 SkillHub 行为接入持久分阶段事务，补原始观察/诊断的受限持久引用和效果未知恢复；不能用本轮负向分类替代完整证据。
3. MCP/Memory 的原模式、完整选择与适用性事实；required online/semantic 事实缺失时保持明确阻塞，不隐式 probe、embedding 或降级。
4. 完整证据 bundle、纯编译、freshness 与 all-required 结论；结合既有 creation/reservation/Run 排序协议完成最终 CAS 和历史 M3，取得至少一个真实可满足有界 profile 正例。
5. 基于最终流程完成独立进程 stop/start、生产构建浏览器与适用验收矩阵。先前公开 fullflow/套件耗时等全 CI 阻塞本轮未重跑或宣称解决。

无 SM-1～SM-4 Runtime 行为变更，无 STEP05 回执消费、执行授权、收费模型调用或 live 配置修改。按 go-multi-module-dev 的 owner/EventHub 边界，资源查询仍由 Application 编排，Workspace 独占 reducer/CAS，未新增 framework 运行单元或跨 owner repository 访问。
