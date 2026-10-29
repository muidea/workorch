# STEP04 SkillHub v0.13.7 部署复验与原回执恢复

日期：2026-10-28。验证对象：`http://127.0.0.1:8090/console/` 当前部署，以及该部署二进制逐字节副本的隔离服务。本轮只验证并记录，不修改产品源码、配置、旧业务账本，不重启生产服务，不启动 Run。记录不是设计权威或完整 STEP04 退出证明。

## 结论

- 新 SkillHub 登记与原回执恢复链路在同部署二进制隔离环境通过，包括真实 v0.13.7 CLI、响应丢失、真实进程停止/启动、只查询恢复及幂等检查。准备结果 10/11，仅 `provider_cache_miss` 阻塞。
- 现有 AI Proxy Workspace 的历史门禁保持有效：仍为 7/10、blocked/stale，旧 status unresolved 不能由新登记回执补证。生产浏览器显示新初始化说明及禁用动作，HTTP 精确重放/恢复未写业务状态；新请求被 409 拒绝。
- 未取得 ready、M3 或 STEP05 启动条件；没有远端模型调用、embedding 或 Run 创建。隔离正例不能记作现有 AI Proxy 业务已通过。

## 实际产物身份

生产进程 PID `676265`，路径 `/home/fedquery/codespace/agentSuite/workorch/bin/workorch`。本轮前后 `/proc/676265/exe` SHA256 相同：

`3b2d9dce762afd55e7e479a607c7f5a59226bda24d63ab4ebe1ea81a93935e5e`

实际 `go version -m` 显示 Go 1.26.7、`vcs.revision=884478cccb7db8bc15ab7bed8bed5b484745213e`、`vcs.modified=true`。当前仓库提交为 `fd78c941`，不能将此产物称作 clean fd78c941 构建，也未建立全部构建输入与提交的等价证明。

生产 `workorch-console.js`、`WorkspaceDetailPage-BXFfAoNy.js` 已与当前本地嵌入资产逐字节比较一致；实际浏览器也读到新登记说明。隔离服务复制实际运行产物，没有重新构建，没有追补构建前快照或调用构建身份工具伪造源码绑定。

## 生产旧数据验证

Workspace：`ws-aaa791dedd9782de319271b3a15a2f98ae4c397418af13e2fbf32098671211b7`。

原 authoring admitted、activation eligible，Workspace revision 28；Goal 原文、67 项 DoD 均保留。准备仍为 blocked/stale，历史通过 7/10：

| 未通过项 | 原因 |
| --- | --- |
| Skill | `skill_selection_sources_incomplete` |
| MCP | `mcp_selection_skill_source_incomplete` |
| Provider | `provider_cache_miss` |

旧检查请求为 `b3aab948-2cb0-4a5b-88cb-8f42753dde77`，预期 revision 26；其 status effect `skillhub-status:a3479389ed8432da6febaaeb68dde187` 仍 unresolved，保留旧混合 stdout 的 `invalid_status` 诊断。原初始化 `422c8103-cac1-48ac-8abe-24ef3bb284c8` 为 v1 completed，File 派发 1 次，expected/committed revision 为 24/26；没有自动升级 v1 或追加登记。

| 真实 HTTP 操作 | 结果 | owner/Hub 变化 |
| --- | --- | --- |
| 原初始化身份重放 | 200 | 无 |
| 原初始化身份显式恢复 | 200 | 无 |
| 原 blocked 检查显式恢复 | 200 | 无 |
| 当前 revision 新初始化 | 409 | 无 |
| 当前 revision 新检查 | 409 | 无 |

生产 Chrome 重载后打开原准备面，确认新登记/仅查询恢复文案；初始化、恢复初始化、新检查、恢复原检查均禁用，只读刷新可用。浏览器没有点击创建新执行，没有发起 Provider 刷新或探测。

最终读取时间 `2026-10-28T10:57:06Z`：authoring、preparation、Goal、DoD、Run 与使用事件同投影比较全部一致，Workspace 文件 bytes 相同；Hub state/global-state/registration 文件清单和 hashes 相同，生产二进制未变。Run 仍 0，历史 LLM usage 仍 8 个事件。owner SHA256：

`172eb4f5dac2025c6b033832e9f3c2b6f2d8bc43552a012a0ed7c2b45ad70fc5`

收尾首次使用 `includePayload=true` 与原 summary 投影比较产生断言失败，确认是请求投影不同；完整响应单独保留，随后使用相同投影精确比较通过。没有改写原始响应来消除差异。

## 同部署二进制隔离正例

隔离 Workspace：`ws-620e29728545d5b183624d56bc96f7b10559e77cccab96d9c793cc15e1c01136`。服务仅回环 `41433`，使用独立配置、file repository、Hub home、项目及空全局技能目录。只对隔离子进程禁用 SkillHub service bridge；不修改生产 Hub 配置。测试结束后所有隔离进程已停止。

1. 正常 HTTP 创建固定小型草稿，通过本地 fixture 完成候选审查和精确确认到 eligible；共 8 次 fixture 创作推理，远端调用 0。
2. 显式初始化经真实 File owner 创建资源，随后真实 SkillHub v0.13.7 登记空项目。
3. 包装器在真实登记及原回执持久后故意丢弃 stdout，原 stdout 另存受限证据；Workorch 正确保留 unresolved，File attempt 1、登记 attempt 1。GET/重放不重发；登记未决时新 check 返回 409。
4. SIGTERM 停止首个服务，启动第二个 OS 进程；原身份 recover 只执行 `project register --query --request-id <原ID>`。初始化 completed，登记/查询预算累计 2，File 仍为 1；Hub state bytes 未变。
5. CLI 原始调用日志证明恰好一次 register、一次 query；completed 后重放及恢复不再调用 CLI、不写 owner。
6. 新显式准备检查真实观察 global/project 空库存，Skill 和 MCP 均 passed；全部 required 通过 10/11，唯一阻塞为 Provider。该隔离候选额外含 1 项资源要求，故清单为 11 项，与生产 10 项不能直接混算。
7. 再次停止/启动第三个 OS 进程，GET/原身份恢复保持历史结果、owner bytes 和 CLI 日志不变。

三个进程 PID 为 `679227`、`679272`、`679323`。STEP04 阶段新增 provider HTTP 0、embedding 0、Run 0；不授 ready/M3。隔离正例没有执行浏览器验证；生产浏览器记录只证明旧业务页的新文案和门禁。

## 原始证据索引

原材料受限存储，不提交 Git。目录 0700、普通文件 0600；每根 `evidence-index.json` 列出相对路径、bytes、SHA256，符号链接另记目标。

- 生产：`.workorch-data/validation/step04-skillhub-v0137-production-live-20261028/`。包括真实请求/响应/headers、前后 owner/Goal/DoD/usage/Run、实际 buildinfo/资产、Hub hashes、浏览器 AX/PNG、执行脚本与 `final-verification-summary.json`。
- 隔离：`.workorch-data/validation/step04-v0137-deployed-isolated-20261028/`。包括实际部署产物副本及身份、三次进程日志、完整 HTTP trace、CLI 原 stdout/stderr 与调用账本、故障注入说明、隔离 owner/Hub 原回执和 `verification-summary.json`（pass=true）。本轮 runner 原文亦保存在生产证据根。

## 剩余项与验证限制

1. **当前业务旧 status 对账仍阻塞。** v0.13.7 登记回执不能反向证明 v0.13.5 无原 request-id 的 status 效果。需要合法的旧操作结算依据/协议，或经用户明确选择用新 Workspace 重建验证；不能删账本、借升级解锁或复用旧 credit。本轮未做这些动作。
2. **Provider 独立阻塞保持。** 本轮不刷新模型缓存、不推理、不探测授权；不能据隔离 Skill/MCP passed 声称整体准备完成。
3. **标准 STEP04 verifier 需同步。** 只读发现 `scripts/verify-workspace-preparation-step04.py` 仍按旧初始化 revision +2/旧有限投影断言，status-only fixture 不支持新 register/query。本轮未执行或修改该工具，不声称标准 verifier 已通过；应保留新预算/回执门禁更新工具后做确定性复验。
4. **完整源码/产物绑定未建立。** 实际部署为旧 revision 加 dirty 构建；本轮结果绑定实际二进制 SHA，不能当作 clean 当前提交 attestation。未运行全仓测试或全 CI，不宣称这些通过。

本轮可确认新登记主体和原回执恢复具备无收费实际进程验证证据；现有业务的旧 fence 与 Provider 仍限制继续进入执行阶段。
