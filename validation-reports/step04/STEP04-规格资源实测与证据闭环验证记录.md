# STEP04 规格资源实测与证据闭环验证记录

日期：2026-10-23。基线 `834aa350cc4e77379819666540b9912a7c22b64c` 加此前及本轮未提交修改。
本文是实施与验证记录，不是设计权威，也不是完整 STEP04、M3 或发布验收通过声明。

## 本轮交付

在既有“原请求接纳 → owner 观察 → 私有 reducer/CAS → 阻塞记录 → 显式恢复”生产框架中，
接入确认规格资源的实际文件读取和受限原始材料存储，不只增加孤立 helper。

1. Workspace 从当前精确封存解析 Specification.Resources 的原 ID/ref/required/expected hash，
   绑定已受理 request/payload/revision、SpecificationHash 和交付目录。
2. 新增 File concrete typed `/file/startup-preparation/resources/query`，只接受 `blocks/workspace`。
   不接受 Web/Application 自报目录、材料或 PASS，不回退到当前草稿来源。
3. File 使用既有 Linux confined opener，拒绝软链接、跨挂载、硬链接、非普通文件、路径逃逸和敏感路径；
   URI 不猜选。实际读后复核已打开文件及重开路径的 identity/size/ctime/mtime。
4. 每份 32 KiB、总计 128 KiB、最多 128 项。保留完整 bytes（含非 UTF-8/NUL）、字节数、SHA256 和版本；
   超限或失败不保存部分前缀，不伪造完整正文/版本。原诊断超过 4096 字节不截断，
   显式记录 `diagnostic_exceeds_byte_boundary`、原字节数/hash 与未保存原文事实，仍属不完整失败。
5. 显式 typed mapping 将原材料写入 Workspace 私有 `PreparationResourceEvidence`；
   与原阻塞记录及 `ResourceEvidenceHash` 同一 CAS 提交。file/memory/scoped 路径深克隆，
   校验材料、时间、revision、规格来源及逐项清单；终态材料/引用不可改写或删除。
   私有持久文件要求 0600，普通 Workspace JSON 与公共 HTTP 仅投影引用/机器码，不输出原材料。
6. 已确认内容相等仅记 `resource_observed_remaining_checks`；内容变化、原 hash 不可解释、读取失败和 owner 不可用分别阻塞。
   不把 Available 声明当当前事实，optional 声明也不升级成 PASS。
7. GET、完成重放、显式恢复、owner 重开不重新读取。CAS 失败不部分保存；回复丢失读回原材料；
   revision/epoch 漂移只保留历史、全部检查记 binding_changed/stale；中断恢复不补造未提交材料。
8. Web 展示资源历史引用及具体阻塞；拒绝错状态/格式和伪造 success，不产生执行请求。

遵循 go-multi-module-dev 的 Workspace/File owner 与 concrete EventHub 边界；
`fileobservation.Resource` 只含 detached 纯值，无 owner/topic/I/O 句柄。
无新增运行单元；本轮不修改 SM-1～SM-4 或执行授权链。
File 入站 inventory 为 21（原 20 + 资源观察），全仓 concrete topic baseline 为 323；
同步扩展完整 wrong-DTO/lifecycle 矩阵，不仅调整计数。

## 验证及证据索引

受限原证据根：`/tmp/workorch-step04-resource-tU2f6H`（临时目录，未提交原日志、二进制、配置或凭据）。
本轮新增 6 个 Go 测试函数和 1 个组件用例，并扩展 repository、真实 HTTP、恢复与完整 lifecycle 矩阵。

| 验证 | 结果 / 证据 |
| --- | --- |
| 受影响完整 Go 包 | 16 个有测试包通过，另 1 包无测试；`final-affected-go.log`；Workspace biz/models/events/startuppreparation、File 全子包、repository 全子包、fileobservation、公共 events、management contract、Application biz/server |
| 最终定向合同 | `final-contract.log`：资源/目录、repository 历史与原字节断言通过；其中无匹配用例的包不计作定向用例通过 |
| 定向 race | `final-race.log`：Workspace biz、repository、File biz 的 Resource/Directory/WorkflowRepository 用例通过 |
| 真实回环 HTTP | `http-workflow-final.log`：`TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 通过 |
| Web 组件 | `web-components.log`：13 文件、139 项通过 |
| Web 规则 | `web-rules.log`：11 项通过 |
| 生产 Web 构建 | `console-build.log`：通过；保留既有大 chunk 提示 |
| vet / fmt / boundary | 通过；`final-vet.log`、`final-fmt.log`、`final-boundary.log`；boundary 保留既有 pycache binary-match 提示 |
| Go 二进制 | `build-final.log`：退出 0；存在受限 stat-cache 写入告警，不冒充干净构建日志 |
| 构建输入关联 | `source-snapshot.json`、`build-identity.json`，capture 后构建，produce 检查构建前后原输入 |

覆盖真实文件原字节和版本、内容漂移、缺失、软链接/硬链接/FIFO、路径边界、逐文件/总字节上界、
超长诊断显式未保存、错 source/scope/revision、取消及 shutdown、伪造 DTO/rehash、0600、深克隆、
持久重开、历史篡改、CAS 写失败/丢回复/漂移与恢复不重读。

HTTP 使用生产 transport/Application ports 和真实 File/Workspace/Run owners、文件持久库。
前置 authoring 为既有非敏感本地 fixture 的 8 次文档调用；STEP04 新增推理 0、embedding 0，
cache-only fixture 读取 1，Run inventory 0。原 `requirements.txt` 的完整资源字节、历史引用和阻塞状态，
经 GET、同请求重放、显式恢复与 owner 重开均保持原值。
owner 重开不等同独立 OS 进程 stop/start，组件验证不等同实际 Chrome。

未执行全仓 CI、收费 live、新二进制部署、独立进程重启或实际浏览器验证。
继承的公开 fullflow/profile 断言与 orchestrator 包累计超时，本轮未重跑/修复，不能宣称全 CI 通过。

## 构建内容身份

以下仅关联本机 dirty tree 的构建输入与产物，不是基线提交单独的结果、可信 attestation 或部署证明。
本轮使用 capture/produce，不宣称既有 CLI verify 问题已经修复。

| 身份 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` 加未提交源码 |
| buildInputSha256 | `6532b694f4e242bcb4f1626d577bbf2ce17a09a8a140c90001c1d4eb2e8bddd1` |
| consoleTreeSha256 | `b3e0a05f87d0879b63e8c11e16cb0380d67a1bb3a62115d64eab5a07b98c00c3` |
| binarySha256 | `f127fb31a1a51031332e596866a7d021a5b5c0c892ce444457c3c7608b656dbe` |

临时原证据目录不等同长期证据归档；本文不在构建输入集合内。
初次 produce 成功生成清单。归档后重复 produce 因清单已存在而按合同拒绝覆盖，未改写原清单；
随后重新 capture 到 `source-snapshot-after-report.json`，与构建前 `source-snapshot.json` 的 `cmp` 退出 0。

## 尚未退出的门槛与后续顺序

1. File managed configuration 实读：区分系统覆盖、工作区保留和 mergeMissing 的既有语义，
   从确认配置推导精确路径与检查规则；不能用默认内容全相等代替，不能调用生成/合并写入接口。
2. 将 Soul/Skill/SkillHub 既有观察汇入持久阶段；SkillHub 保留用户接受的现行行为，
   但外部效果阶段须先持久封存，unknown 不自动重发。
3. MCP/Memory/provider 的精确原模式、选择、完整性、版本和 freshness；
   cache/history 当前仍仅负向分类，尚非完整原始事实归档。不探测、不 embedding、不自动降级。
4. 全部 required 真实事实合取、纯编译及最终 reservation/epoch/完整 Run 历史 CAS，
   形成可满足的 M3 正例。当前全部逐项仍为 blocked，不是资源 ready。
5. 主体正例后完成生产 HTTP/Web 正负/恢复、独立 OS 进程重启及最终源码/产物/退出矩阵。
   文件观察是历史时点，不承诺 File 读取至 Workspace CAS 跨文件原子性。

本轮严格限 STEP04；未进入 STEP05、运行派发、收费模型或外部 skill-hub 项目修改。
未提交 Git，保留此前全部未提交改动。
