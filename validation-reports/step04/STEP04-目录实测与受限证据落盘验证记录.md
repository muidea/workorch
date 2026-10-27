# STEP04 目录实测与受限证据落盘验证记录

日期：2026-10-23（Asia/Shanghai）。基线：`834aa350cc4e77379819666540b9912a7c22b64c` 加累积未提交源码。

## 本批结果与边界

完成第一批真实 File 观察接线：原检查 CAS 受理 → Application cache/history 观察 →
Workspace 精确解析原目录并向 File 验证 → 原操作结算与受限材料同一 CAS →
公共历史引用 → GET/原请求重放/恢复/owner 重开。

本批只完成目录身份/有效权限观察及该类材料的持久主链；不是通用全资源证据流水线完成，
更不是完整 F03、资源 ready、最终 M3 或整个 STEP04 通过。目录项即使实际观察完整，仍为
`blocked/directory_observed_remaining_checks`，不以它替代管理配置、规格资源或其他 required 项。

按 go-multi-module-dev 的现有 owner 分工实施：Application 发起原操作，Workspace 解析封存并裁决私有状态，
File 只观察文件系统；跨 owner 使用 concrete typed EventHub。新增纯值包 `fileobservation` 无 topic、
I/O handle 或业务权威；File result 与 Workspace 材料显式 typed 映射，JSON 仅用于持久化和内容 checksum。
没有新运行单元，没有更改 SM-1～SM-4、Start/Restart、STEP05 或执行授权链。

## 已实现

1. File 新增 Workspace-only `/file/startup-preparation/directories/query`。检查原 Workspace/request/payload/revision
   和两个 owner 解析的绝对目录；公开 HTTP 不接收路径、File verdict 或成功凭据。
2. Linux 逐段无软链接打开目录，固定 fd，使用有效进程凭据进行 ACL-aware R/W/X 观察，
   随后重开路径并比较设备、inode、mode、uid/gid、mtime/ctime。无 mkdir、write probe、chmod、配置生成或修复。
   查询预算保留 parent context；不将 deadline 当作已经开始的 OS 调用完成回执。其他平台不能精确观察时明确失败。
3. 返回点时身份、权限及失败原 bytes/字节数/SHA256；不清洗、归一化或截断诊断。
   诊断最多 4096 bytes，超界/错误类型/错 scope/不完整 DTO 拒绝，不标成完整已保存观察。
   目录不存在、软链接或无法观察与有效权限不足分别分类；成功读取不是未来可写或原子文件系统保证。
4. Workspace 仅对仍 accepted、原 revision/epoch 未知漂移的原操作解析 exact seal；跨 owner I/O 不持 definition/reducer 锁。
   私有 `workspacePreparationBlocked` fact 携带真实材料；reducer 校验原身份、时间、revision、内容 hash 和由材料推导的原因。
   原结算 CAS 同时写材料和 `DirectoryEvidenceHash`；写失败无半份引用，丢回复读回原材料。
5. `WorkspacePreparationDirectoryEvidence` 是 owner-private 历史材料，不进入普通 Workspace JSON、通知/SSE 或 HTTP。
   file repository 用显式私有 wire 保存，含材料的文件必须 0600；memory/file/scoped 均深克隆。
   读回校验原 bytes/hash 与原操作一一对应；repository backstop 禁止材料孤立、删除、覆盖或仅重算 hash 改写历史。
6. 结算时 revision/epoch 漂移保留材料历史，逐项 `binding_changed`、currentValidity=stale；不授 current credit。
   GET、普通同键重放、已完成恢复不重读 File；未提交材料在显式恢复时不补造，原 accepted 收为 interrupted blocked。
   没有为 SkillHub 的潜在效果添加自动重放；其效果阶段仍待后续独立接线。
7. 公共 management DTO/Web 只显示 `directoryEvidenceHash` 历史引用和负向原因，严格拒绝把引用放在 accepted/unknown。
   页面明确说明历史引用不代表当前资源 ready，不提供 M3 或执行按钮。生产内嵌 Web 已重新构建。
8. File 事件库存由 19 增至 20：原 19 加目录观察 1；公共 concrete topic 库存由 321 增至 322。
   同步更新 delivery/message、完整 wrong-DTO/lifecycle 矩阵、owner README 与现行 service-api。

## 已执行验证

原始证据根：`/tmp/workorch-step04-file-UrWjZu`（本机受限临时目录，不提交原日志、私有材料、二进制或 live 配置）。

| 检查 | 结果与证据 |
| --- | --- |
| 完整受影响 Go 回归 | 16 个有测试包通过，1 个包无测试；`affected-go.log`、`application-server.log` |
| 定向 race | Workspace biz、repository、File biz 3 包通过；`race.log` |
| 真实回环 HTTP + owner 重开 | `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 通过；`http-workflow.log` |
| Web 组件 | 13 文件、138 项通过；`web-components.log` |
| Web 规则 | 11 项通过；`web-rules.log` |
| 生产 Web | 构建通过；`console-build.log`，仍有既有 chunk 大小提示 |
| vet / fmt / boundary | 通过；`vet.log`、`fmt.log`、`boundary.log` |
| Go 二进制 | 构建退出 0；`build-final.log`，含受限 stat-cache 写入告警 |
| patch whitespace | `git diff --check` 通过 |

完整回归范围：Workspace biz/models/events/startuppreparation、File 全子包、repository 全子包、
fileobservation、公共 events、management contract，以及 Application biz/server。
主要包级耗时：Workspace biz 75.716 秒、Application biz 64.074 秒、server 26.144 秒；不是单测逐条耗时。

新增 6 个 Go 测试函数与 1 个组件用例；并扩展既有 repository、真实 HTTP、观察和完整 lifecycle 矩阵。
覆盖实际目录/文件/软链接/不存在、零写入、实际权限、错 source/revision、shutdown、任意诊断 bytes、
bytes/hash/上界、0600、普通 JSON 防泄露、深克隆、原材料重开、写失败/丢回复、revision 漂移、
完成历史篡改和恢复不补造。有效权限反例在非 root 时断言不可写；root 不以 mode bits 猜测实际权限。

HTTP 使用生产 transport/Application ports、真实 File/Workspace/Run owners 与持久 repository。
候选准备仍为原固定非敏感 fixture（8 次文档调用，属于准备前置）；STEP04 新增推理/embedding 0，
cache-only fixture 读取 1，Run inventory 0。目录实测材料与公共 hash 在 GET/重放/恢复/owner 重开后保持原值。
owner 重开不是独立 OS 进程 stop/start，组件测试不是实际浏览器验收。

未执行全仓 CI、独立进程重启、生产浏览器或收费 live；没有运行新二进制做部署验证。
继承的公开 fullflow 策略断言和 orchestrator 累计超时未在本批重跑，不能宣布已经修复或全 CI 通过。

## 最终构建内容身份

先重建内嵌 Web，再 capture；capture 完成后执行最终 Go build，再 produce 校验构建前后输入。
以下是当前 dirty tree 的本机内容关联，不是基线提交独自的结果、可信 attestation 或部署证明。

| 身份 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`，加本次及此前未提交源码 |
| buildInputSha256 | `72fed59a577f83824b6b0196cdd587918d8661767b03d214059f7e4cf89a2482` |
| consoleTreeSha256 | `f9aec8cf5a7782a48160c364931793c3e19cfc46bf13831ca08f77e404dd4b00` |
| binarySha256 | `3bfc5b1bd1ad9cbc247dc56acbfa0e2eed0a8773c261a6c885e68473624624c6` |

对应 `source-snapshot.json`、`build-identity.json`、`workorch`。本批使用 capture/produce，未宣称 CLI verify 通过。
临时目录不等同长期证据归档；本文本身不在构建输入集合内。

## 按顺序仍需收口

1. File 现有 managed configs 的内容/合并语义和原 reviewed resource/currentness，不能以目录成功代替。
2. Soul/Skill/SkillHub 既有观察汇入持久阶段；SkillHub 沿用户接受的现行行为，但须先封存效果阶段与未知不重发边界。
3. MCP/Memory/provider 精确原模式、目标选择、完整性、版本和 freshness；不探测、不 embedding、不自动降级。
4. 全资源原始证据及引用汇总、全部 required 合取、纯编译与最终 reservation/epoch/Run 历史 CAS，形成可满足的 M3 正例。
5. 主体正例后，补生产 HTTP/Web 正负与恢复、独立 OS 进程重启，以及最终源码/二进制/退出矩阵。

本批未提交 Git；保留全部已有未提交修改。
