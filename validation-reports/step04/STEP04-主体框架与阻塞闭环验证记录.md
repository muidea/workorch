# STEP04 主体框架与阻塞闭环验证记录

日期：2026-10-22（Asia/Shanghai）。基线 HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，包含累积未提交修改。

## 本轮结论与范围

按用户“先完成主体框架和流程，业务跑通后再完善细节”的要求，本轮完成**第一批阻塞路径的主体闭环**：

已封存输入 → 显式检查 → Workspace CAS 受理 → 私有 reducer/CAS 保存逐项 blocked →
HTTP/Web 查询、原请求重放及显式恢复 → 文件 owner 关闭/重开后读回。

这不是完整资源检查业务正例。资源观察组合流水线尚未接入，所有检查项明确保持
`blocked/check_pipeline_incomplete`，没有 PASS、正 readiness、M3 或执行授权。
不把既有 Soul/Skill/provider/Run 观察子用例的通过记录升级为本流程的成功资源凭据。

本轮没有修改 Start/Restart、Runtime drive 或 SM-1～SM-4 迁移；新增状态属于
Workspace owner-private `workspace.preparation` 与 `workspace.preparation_settlement`，
在唯一 owner/state inventory 登记。没有新运行单元、跨 owner repository/service 或收费调用。

## 已接通的主体

1. 新增 concrete check/recover EventHub 合同，Workspace state lane，仅接收 Application source。
   请求只有 Workspace、RequestID 和原 expected revision，不接收 caller manifest、PASS 或已验证标志。
2. 新增准备记录 v2：原意图 hash、精确封存/配置/manifest/policy/compiler 绑定、
   creation/preparation epoch、全部检查项 ID/owner/required 与 pending/blocked 结果。
   不保存资源原文、凭据或原始诊断。v1 trace 不迁移为 v2 正例。
3. owner 从真实确认/正式行反查 exact snapshot，纯编译稳定清单，接纳时再核对完整当前封存；
   受理与确定阻塞分别 CAS。初始 CAS 丢回复不继续，不把未知写结果当作明确未受理。
4. 同键同完整输入只返回原记录，同键异输入冲突。accepted/unknown 不能被新键绕过；
   显式恢复只将原中断记录记为 blocked，不重跑观察或执行。revision/epoch 漂移保留原绑定，
   标记 binding_changed/stale，不将新版本冒充原检查。
5. file/memory repository 增加 v2 持久 backstop：非 CAS、删除/覆盖历史、修改原绑定、
   删除检查或伪造 ready 均拒绝；嵌套检查 clone 不共享可变存储。历史最多 64 次，不淘汰旧身份。
6. HTTP 增加 `/activation-preparation/check`、`/activation-preparation/recover` POST；
   仅两个请求字段，4KiB 边界，拒绝未知字段和尾随 JSON。200 blocked 不是 ready，202 只是 accepted/unknown。
   GET 保持纯读，所有响应 no-store，公共错误不返回 owner 原始诊断。
7. Web 详情页增加独立“激活前准备”入口，分开展示操作状态、当前有效性与已有 Run 投影。
   开页/刷新只 GET；显式提交单飞，POST 后 GET 对账；错误后保留原请求及原 revision，
   GET 未见原请求不证明拒绝、不解锁新键。重开从服务端找原未决操作；Workspace/key/迟到响应隔离，
   不使用浏览器持久存储。原操作已完成但其他页面有新未决操作时，继续锁定新原身份。

当前 v2 恢复的安全前提是本版本检查路径**没有外部效果**。后续接入 SkillHub status 等实际观察时，
必须先补齐对应的效果阶段、受限原文/观察引用与不确定结果对账，不能把本轮“中断即确定阻塞”的
恢复器直接当作外部未知效果的清除协议。fresh-intent reservation 不因准备恢复而清除。

## 验证结果

原始证据根：`/tmp/workorch-step04-workflow-oghHNu`（本机临时受限目录，不提交原始材料或二进制）。

| 验证 | 最终结果与证据 |
| --- | --- |
| 受影响包完整回归 | 11 包通过，`affected-go-test-final.log` |
| 新流程定向 race | Workspace biz、repository、server 三包通过，`race-final.log` |
| 真实回环 HTTP | `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 通过，`http-workflow-final.log` |
| Web 组件 | 13 文件、136 项通过；新增准备面 7 项，`web-components-final.log` |
| Web 规则 | npm test 最终统计 11 项、0 失败，`web-rules.log` |
| 生产 Web 构建 | 通过，`console-build-final.log`，内嵌 assets 已重建 |
| Go 二进制构建 | 退出 0，`go-build-release.log`、`workorch-release` |
| vet | 退出 0、无输出，`vet-final.log` |
| 格式与结构边界 | 通过，`fmt-final.log`、`boundary-final.log`；git diff --check 通过 |

完整回归范围：Workspace biz/models/events/startuppreparation、Application biz/server、
repository 根包/memory/file、management contract、公共 events。
Workspace biz 69.850 秒，Application biz 62.343 秒，server 30.704 秒；
这些是包级耗时，不作为每个测试的独立耗时。

新增 owner 用例覆盖受理与完成两处持久切点（受理后丢回复、完成写失败、完成后丢回复）、
普通重放不续跑、显式原身份恢复、文件 owner 重开、同键异输入、过期首发、缺原请求恢复、
并发同键只产生一条记录、漂移后 stale，以及不可改写历史。
repository 用例独立覆盖 memory/file/scoped 三种入口的 CAS、clone 与重开。
HTTP 参数用例覆盖 check/recover、accepted/unknown/blocked、未配置、非法输入、未知字段、
超界、尾随文档、错 scope/请求、伪造 ready 与安全错误。

真实 HTTP 使用生产 transport、正式 application ports 与真实持久 owner；仅创作候选准备使用固定
非敏感 provider fixture。候选准备阶段既有 8 次 fixture 文档调用单列，STEP04 检查/GET/重放/
恢复/owner 重开新增 provider 调用 0，Run inventory 为 0。准备路径不调用 SkillHub status。
这里的重开是同测试进程内服务与 owner 关闭/重建，不是独立 OS 进程 stop/start，也不是实际浏览器验证。

中间验证发现并修复：新事件合同对另一个 owner 模型包的导入、私有迁移登记的精确 sink 对应关系、
新增 application port 的缺失错误粒度。没有放宽对应门禁或原业务测试。
Vite 仍有既有大 chunk 告警；Go 构建有受限环境下 stat-cache 写入告警，但退出 0，产物及输入检查成功。
未执行全仓 CI，不据上述结果宣称全 CI 或完整 STEP04 通过。

## 最终构建内容身份

先完成生产 Web 构建，再 capture 当前构建输入，再构建二进制，再执行 build_identity.py produce，
校验构建前后输入并记录产物。没有用 HEAD 代替实际 dirty tree。

| 身份 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`，加当前未提交源码 |
| buildInputSha256 | `cdecba29e80f466843f4633b63007674f082be706cb40af5048266cf354c5f0f` |
| consoleTreeSha256 | `83d773a09e052e52b9a59372fdca44abb164cf5c0bf11e2960123b317ee94349` |
| binarySha256 | `71fa37f4b7f11b82c48c84a53ae792b22bc3488dc34fb060dcaf0b799cb331c0` |

对应 `source-snapshot-release.json`、`build-identity-release.json` 和 `workorch-release`。
这是本机内容关联记录，不是可信构建 attestation；未运行 build_identity CLI verify，
也没有运行该二进制进行实际浏览器或独立进程重启验收。临时材料的保留不等于已完成长期归档。

## 下一批主体收口

1. 将实际资源 owner 观察组合接入：先冻结逐项证据、有效期、效果阶段和恢复边界，再接已有
   Soul/Skill/provider/Run 子用例，补齐 File 配置/目录、MCP、Memory 和规格资源事实；
   保留当前明确阻塞，不生成占位 PASS。
2. 组合纯结构编译与全部 required 真实结果，扩展持久结果/诊断引用及资源变化后的新检查。
   仅在完整流水线安全成立后支持正向 readiness。
3. 按已批准 no-Run 协议实现最终 M3 CAS：完整历史观察、creation/preparation epoch、
   fresh reservation 竞争、唯一历史完成与当前失效分轴。不得从 ActiveRunID 为空推出 no-Run。
4. 主体正例跑通后，再补齐生产 Web 正/负/恢复、独立真实进程 stop/start 与最终退出证据。

STEP05 的 Start/Restart 准备回执消费、Runtime fullflow、执行派发及新的收费 live 不在本轮范围。
