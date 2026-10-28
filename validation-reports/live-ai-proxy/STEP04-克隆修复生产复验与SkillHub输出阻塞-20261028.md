# STEP04 克隆修复生产复验与 SkillHub 输出阻塞

日期：2026-10-28。结论：资源初始化的 Clone/CAS 缺陷已在真实生产部署复验通过；新的显式准备检查通过 7/10，仍 blocked，不能进入 STEP05。本轮未改生产代码、未手工修改配置或历史数据、未重启服务，未创建 Run。显式初始化和 SkillHub status 产生的 owner／资源效果按下文记录。

## 部署和输入身份

- 生产地址：`http://127.0.0.1:8090/console/`；实际 Chrome 共享标签页。
- 当前提交：`884478cccb7db8bc15ab7bed8bed5b484745213e`。
- 当前构建输入 SHA256：`a3c5326c281a6376141a5bb316088a179c96289b1902eb03a005ba02290e5bc2`；console 树 SHA256：`32ad36fd16c3e61f7df3562bbaebfd4a309707db0565b6b748ebc09d36835b6f`，均与上一轮最终隔离验收相等。
- 实际 PID：556170；运行二进制 SHA256：`d62a25ea3531d3c53e06d3fbde7d62e8f197b5b8e7d1e407ffdfa0a90ebac9ee`，与上一轮修复版隔离二进制逐字节一致。Go buildinfo 仍记录 `86d3548e…+dirty`，不能声称它是 clean `884478cc` 构建或可信 attestation。
- 原 Workspace：`ws-aaa791dedd9782de319271b3a15a2f98ae4c397418af13e2fbf32098671211b7`；沿用原 Goal/67 DoD、确认和 eligible admission，不清空或重新生成数据。
- 开始时 revision 24、无初始化 journal、Run 0；原 owner SHA256 仍为 `67e22d9686c7ea62b30714852b9fdc1cdb19b2627374e278e84c6010560c9b20`。原准备检查仍为 6/10、blocked/stale。

## 原资源初始化复验通过

使用真实 HTTP 重放原 request `422c8103-cac1-48ac-8abe-24ef3bb284c8`、expected revision 24，返回 200/completed，revision 26、attempts 1。因为页面刷新后原未接纳请求只存在旧证据中，本次直接使用其精确原身份重放，没有点击生成新初始化身份。

真实 File owner 结果保存缺失前像和完整后像：system、Workspace 两份 `lsp-bridge/mcp-config.json` 各 260 bytes，SHA256 均为 `7e1d73ac8b85201ee132f09e4640208a02f5579033e853c572c200a75f3b72e1`；实际文件读取与各自回执逐字节相等。项目 `.agents/skills` 创建完成，业务文件仍为 0。

初始化前后完整 owner JSON 的差异仅为 `Revision` 和新增 `PreparationResourceInitializations`；旧准备检查、authoring、候选、StartupInput、admission 及其他字段均未改变。普通初始化重放和 recover 均为 200，响应 bytes 与首次完成响应相同，owner bytes 不变。生产页面只读刷新显示同一原请求 completed、一次派发预算，仍保留旧检查结果，未将初始化提升为 ready。

## 新显式检查：7/10

从生产浏览器点击一次“显式检查准备”，新 request 为 `b3aab948-2cb0-4a5b-88cb-8f42753dde77`，expected revision 26，完成后 revision 28。原检查完整保留，新增独立检查和证据。

| 项目 | 新检查结果 |
| --- | --- |
| directory_boundary、exact_seal、first_activation_no_run、managed_configuration、memory_selection、persistence_integrity、soul_selection | passed |
| skill_selection | blocked：`skill_selection_sources_incomplete` |
| mcp_selection | blocked：`mcp_selection_skill_source_incomplete` |
| provider_cache | blocked：`provider_cache_miss` |

检查后的初次 GET 为 `currentValidity=not_evaluated`，后续只读 GET 为 stale；历史 7/10 不因刷新变成当前有效或 ready。页面显示完整结构编译 `skill_source_unavailable`，没有 bootstrap 或 M3。

## SkillHub 阻塞的内部原因

1. global status：真实进程 exit 0，stdout 6270 bytes、stderr 0，完整归档；已安装材料观察无 failure，10 个 reference。不能把这些 reference 数量等同执行 Skill 的最终选择数。
2. project status：真实进程 exit 0，stdout 760 bytes、stderr 0，完整归档。但首次发现 `.agents/skills` 对应目录未在 SkillHub 注册时，输出依次包含“当前目录未注册”“是否创建新的项目工作区？[Y/n]”、创建目录与成功提示，最后才输出 JSON，JSON 中 `scope=project`、`skill_count=0`、`items=[]`。
3. `internal/pkg/skillhubpreparation/status.go:38` 的现行合同要求完整 stdout 是唯一、无歧义的 JSON 文档。这份混合输出被正确判为 `invalid_status`，项目安装材料未生成；后续 Skill 来源不完整，MCP 选择依赖同一 Skill 来源而阻塞，没有伪造连接或选择证明。
4. 只读源码核对发现 `/home/fedquery/codespace/skillSuite/skill-hub/internal/clis/skill-hub/dependencies.go:215` 的 `EnsureProjectWorkspace` 会打印这些提示，并在 stdin 空输入时走默认注册分支。本轮原始 stdout 与该分支一致。SkillHub 二进制 buildinfo 单独归档；源码对照用于解释行为，不冒充该外部项目二进制的构建 attestation。
5. 当前 owner 将此 status 效果保留为 unresolved，页面禁止新检查／初始化绕过。本轮没有重复执行 status，没有通过截取最后一个 JSON、改写原始 stdout、清除 journal 或修改已保存失败来获得通过。

因此 `.agents/skills` 目录已经存在，并不保证首次 project status 能返回机器可解析的纯 JSON；创建基础设施与 SkillHub 登记／观察协议是两个边界。当前现象不是资源初始化再次失败。

## Provider 与安全边界

新进程的 LLM owner catalog 为 unavailable／`cache_miss`，没有持久 catalog revision 或 fetched/expiry 值。本轮没有刷新模型目录、发外部授权探测、推理或 embedding；因此不能将上一进程的容量观察当作当前缓存或授权证明。

正式 Goal/67 DoD 的前后 HTTP 内容相等，封存输入、候选、admission 和原检查保持；全局 Run 库存仍 0。LLM usage 事件仍为原 8 个身份，新增 0（旧查询带原 payload，当前查询为 summary 投影，仅以相同有序事件身份及数量证明零新增，不声称两种 DTO 完整相等）。交付目录没有业务文件。服务未由本轮重启，未执行 install/sync/use/apply/repair 或 Start/Restart。

## 后续收口顺序

1. 先收口 SkillHub 的 JSON 模式与首次项目登记交互：明确机器输出／诊断分离及实际允许的 status 效果，增加“已创建 `.agents/skills`、尚未登记的新项目”的真实 CLI 回归，不能只使用纯 JSON fixture。
2. 为当前已封存的 `invalid_status`／unresolved 操作补精确、可审计的对账恢复路径；保留完整原输出、真实 exit、登记事实和旧失败。不得清空数据、改写旧结论、盲重跑或换 request 绕过效果门禁。
3. 取得完整 Skill 来源后重新执行显式准备检查，核验 Skill 选择及 MCP 实际结果；当前 MCP 是前置来源阻塞，不能提前断言 MCP 连接自身失败或会自动通过。
4. Provider 独立收口 cache 与授权证明。本次不将该范围混入 SkillHub 修复，也不使用真实收费调用证明全部权限。

## 证据

受限归档：`.workorch-data/validation/step04-clone-fix-production-live-20261028/`，54 个文件及 `evidence-index.json`，目录 0700、文件 0600，忽略且不提交原始材料。索引 SHA256：`12b27f49b700f6cb31059845ef5b11313c31e1e4ecd50b9330688546ed044795`。

入口：`verification-summary.json`、`process-identity.json`、`source-snapshot-current.json`、`go-buildinfo.txt`、`owner-before.json`／`owner-after-initialize.json`／`owner-after-check.json`／`owner-final.json`、原身份 HTTP request/response、真实文件 readback、SkillHub stdout、usage/Run/Goal/DoD HTTP 材料及浏览器 AX/截图。临时证据仅放 `/tmp/workorch-validation/step04-clone-live-20261028/`，未新增项目根临时目录。本轮只增加此脱敏验证记录，未修改代码或提交 Git。
