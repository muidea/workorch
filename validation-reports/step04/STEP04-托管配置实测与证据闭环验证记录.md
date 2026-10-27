# STEP04 托管配置实测与证据闭环验证记录

日期：2026-10-23。基线 `834aa350cc4e77379819666540b9912a7c22b64c` 加此前及本轮未提交修改。
本文仅记录实施与验证，不是设计权威、完整 STEP04/M3、发布或全 CI 通过声明。

## 本轮交付

在已接通的“原请求接纳 → owner 观察 → 私有 reducer/CAS → 阻塞记录 → 原操作恢复”流程中，
接入真实 managed configuration 读取，而非仅增加未调用 helper。

1. Workspace 在 accepted 原操作、原 revision/epoch 仍适用时反查完整 exact seal，独立读取
   已确认 TemplateSnapshot.ContextHints 与 exact-bound Workspace.ContextHints 的声明。
   不查询最新 Template、不猜选合并来源，不调用 `EnsureManagedConfigs` 或任何生成/覆盖/补缺写入口。
2. 声明解析保留两个来源及各自顺序。仅支持 canonical 相对路径、JSON 对象；
   未知字段、错类型、重复同域目标、格式/路径歧义、深度/字节/项数超限均 fail-closed，不能静默跳过。
3. 内容规则区分：system 必须匹配完整默认对象（含额外键检查）；workspace preserve 允许合法 JSON 对象覆盖；
   workspace mergeMissing 只核对原递归补缺规则是否还会增加键，已有值（包括 null/非对象）不强制替换。
   preserve 不等于接受损坏 JSON。重复键、无效 UTF-8、有损 surrogate escape、尾随值/非对象与超深 JSON 阻塞；
   数值比较不经 float64，区分大于 2^53 的不同整数，同时接受相等的数值表示。
   内容原字节不因上述语义比较而改写。
4. 新 File concrete typed `/file/startup-preparation/managed-configs/query` 仅接受 `blocks/workspace`，
   echo 原 Workspace/request/payload/revision、内部目录和有序 scope/path。File 不接收 defaults 或 caller verdict。
   system 由既有 SystemDirectory 纯路径规则定位，workspace 使用原内部目录；无 workspace-first fallback。
5. 复用 confined/pinned 普通文件实读：无软链接、硬链接、跨挂载、路径逃逸；
   复核 inode/size/ctime/mtime。每份 32 KiB、总计 128 KiB、最多 128 项，不完整/超限不保存前缀。
   完整 bytes（包括非法 JSON/非 UTF-8）、字节数/hash、文件版本与原读取诊断受限保存。
   原诊断大于 4096 字节时沿用显式未保存原因、原字节数/hash，不截断、不冒充完整。
6. Workspace 私有 `PreparationManagedConfigEvidence` 保存默认语义对象的 JSON bytes、实际文件原文、
   原 request/revision/configuration/time 关联和完整性 hash；默认对象不通过 float64 解码重存。
   归档与原操作终态同一 CAS。file/memory/scoped 深克隆，append-only backstop 拒绝删改或重算 hash 后替换历史。
   文件持久材料要求 0600，普通 Workspace JSON 不输出该 archive。
7. 公共 attempt 只增加 `ManagedConfigEvidenceRef` / HTTP `managedConfigEvidenceRef`：
   `managed-config-observation:` 加 16 字节独立随机数的小写 hex，不是配置内容派生 hash。
   配置原文、文件内容摘要、预期内容与路径不进入准备公共响应/Web。既有封存 configuration binding 保持原合同。
8. 缺失/读取不完整、内容非法、内容冲突、声明非法与 owner 无有效回执分别投影具体阻塞码；
   匹配或确认无声明也只记 `managed_configs_observed_remaining_checks`，全部检查仍 blocked。
   不授予 ready、M3 或执行权限。GET、完成重放、恢复和 owner 重开不重新读取；
   CAS 写失败不部分保存，回复丢失对账原材料，revision/epoch 漂移只封存历史并记 binding_changed/stale。
   中断恢复不补造未提交材料。人工准备后必须另发新身份、当前 revision 的显式检查。

本轮沿用 go-multi-module-dev 的 Workspace/File owner 与 concrete EventHub 合同；未新增运行单元，
未修改 SM-1～SM-4、Start/Restart 消费授权链、外部 skill-hub 项目或 live 配置。
File 入站 inventory 从 21 增至 22（新增配置观察），完整 wrong-DTO/lifecycle 矩阵同步覆盖；
全仓 concrete topic baseline 从 323 增至 324，新增 query 的 owner/class/lane/message 合同一并验证。

## 验证与受限证据索引

受限证据根：`/tmp/workorch-step04-config-Xr5QJH`。临时原日志/二进制不提交 Git，也不等同长期证据归档。
新增 7 个 Go 测试函数、1 个 Web 组件用例；扩展 repository、HTTP、恢复及 lifecycle 断言。

| 验证 | 实际结果 / 证据 |
| --- | --- |
| 受影响 Go 包完整回归 | `final-affected-go.log`：17 个有测试包通过、另 1 包无测试，`-count=1`，无 skip；涵盖 managedconfig/fileobservation、File 全子包、Workspace biz/models/events/startuppreparation、repository 全子包、公共 events、management contract、Application biz/server |
| 最后新增公共 DTO 定向断言 | `final-contract.log`：managementcontract 全包通过，覆盖随机引用格式、错状态/版本和伪造 PASS/错用途原因；该新增测试在上述完整命令启动后加入，单独复验 |
| 定向 race | `race.log`：Workspace biz、File biz、repository 的 Managed/Directory/Resource/WorkflowRepository 用例通过 |
| 真实回环 HTTP | `http-workflow-final.log`：`TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 通过 |
| Web 组件 | `web-components.log`：13 文件、140 项通过 |
| Web 规则 | `web-rules.log`：11 项通过 |
| 生产 Web 构建 | `console-build.log`：通过，保留既有大 chunk 提示 |
| vet / fmt / boundary | `vet.log`、`fmt.log`、`boundary.log`：退出 0；boundary 保留既有 pycache binary-match 提示 |
| Go 二进制 | `build.log`：退出 0；仍有只读 Go stat-cache 写入告警，不称干净日志 |
| 源码/产物内容身份 | `source-snapshot.json`、`build-identity.json`、`identity-produce.log`：capture → build → produce 完成 |

自动回归覆盖真实两域文件、原字节与精确数值、缺失/非法/冲突、保留覆盖及补缺、无声明/错声明、
软链接/硬链接、单份/总字节上限、错 source/scope/回执身份、取消/shutdown、私有 clone/0600/reopen、
历史篡改、CAS 写失败/丢回复/漂移、原操作恢复不重读与 Web 仅随机历史引用。

真实 HTTP 使用生产 transport/Application ports、真实 Workspace/File/Run owners 与临时文件持久库。
前置 authoring 使用固定本地 fixture（8 次文档调用）；STEP04 新增推理 0、embedding 0，
cache-only fixture 读取 1，Run inventory 0。
测试按两个确认来源准备所有声明的文件；首次检查后删除配置文件，再通过 GET、同请求 POST、
显式 recover 及 owner 重开验证原始材料/引用和阻塞结果不变，不以不存在的文件产生新证明。
owner 重开不是独立 OS 进程 stop/start，Web 组件验证不是实际 Chrome。

初次完整命令虽排除名称含 HTTP 的用例，仍因其他用例监听回环端口而受 sandbox socket 限制失败
（`affected-go.log`）。随后在允许本机监听的环境完整重跑，无跳过，结果见 final-affected-go.log。
HTTP 初测仅准备 system 文件，而创建流程也在 Workspace 来源保存了同路径声明，正确阻塞为
`managed_config_observation_incomplete`（`http-workflow.log`）；测试改为按两个真实声明来源完整准备，
未修改生产 fallback 或放宽断言，最终通过。

未执行全仓 CI、收费 live、实际 Chrome、新二进制部署或独立 OS 进程重启。
继承的公开 fullflow/profile 断言与 orchestrator 包累计超时本轮未重跑/修复；不宣称全 CI 通过。

## 构建内容身份

以下关联本机 dirty tree 的构建输入和产物，不是基线提交单独的验证结果、可信 attestation 或部署证明。
本轮使用 capture/produce，不宣称已修复既有 CLI verify 问题。

| 身份 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` 加未提交修改 |
| buildInputSha256 | `a68d71b11a28af12d7f753dbaf727555998c71966fab0b82e4d30ef9d3be467d` |
| consoleTreeSha256 | `58c49efb5e8b08861b707a03ea0c8d50814e332e835a3e339242d9f89c9a8194` |
| binarySha256 | `09a23bdec651e9fd6479bf558710144efec7ecf66bb27b8f6af627ecb1241873` |

本记录不在构建输入集合内；归档后再次 capture，与构建前 snapshot 比较，以防将旧产物关联到新输入。

## 剩余退出门槛与后续顺序

1. 将既有 Soul/Skill/SkillHub 精确观察汇入持久阶段，保留用户接受的 SkillHub 现行效果；
   外部效果阶段须先持久封存，unknown 不自动重发。
2. MCP/Memory/provider 的原模式、精确选择、完整性、版本与 freshness；cache/history 当前仍为负向分类，
   尚非完整原始事实归档。禁止探测/embedding/自动降级。
3. 全部 required 真实事实合取、纯编译和最终 reservation/epoch/完整 Run 历史 CAS，建立可满足的 M3 正例。
   当前 v2 仍只有 accepted/unknown/blocked/failed，没有 readiness/M3 正状态。
4. 主体正例后补生产 HTTP/Web 正负/故障恢复、独立 OS 进程重启和最终源码/产物/退出矩阵。
   文件时点观察不承诺读取至 Workspace CAS 的跨文件原子性。

严格限 STEP04，未进入 STEP05、Runtime fullflow、执行派发或收费验证；未提交 Git，保留此前全部修改。
