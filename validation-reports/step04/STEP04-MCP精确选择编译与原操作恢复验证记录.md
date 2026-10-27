# STEP04 MCP 精确选择编译与原操作恢复验证记录

日期：2026-10-26。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加累计未提交源码。
本轮承接完整 registry 观察，补齐私有精确选择编译及其原操作结算、独立读回和恢复保护。
不表示 MCP 连接就绪、全部 STEP04、M3、全 CI 或发布验收通过；未自动提交累计修改。

## 实现与边界

1. 新增纯 typed `workorch-preparation-mcp-selection/v1`，从 Workspace 原配置及确认时 Template 快照提取默认 server/capability ID，保留既有 Workspace-over-Template 递归覆盖语义。集合排序去重仅用于准备预览，不改变已有激活路径或构造 Run。
2. 默认集合与同次原操作完整 Skill selection 中的必需引用取并集。严格检查 ID、类型、重复声明键、数量、对象存在/启用、capability 所属 server 被显式选中，以及 server 的 tool/resource/prompt allowlist。缺 Skill 来源为明确失败，不作为空选择；不自动补 server，不修复或改写原声明。
3. 编译保留原配置身份、同次 Skill archive ref/content hash、必需/选中 ID、完整选中 owner 值及失败码。完整原 registry 前后像仍原样保存，选中子集不能取代它。成功复制的是 owner 返回的既有规范化 typed 值，不声称保存 MCP 仓库文件原始字节。
4. 仅原非 stale settlement、在 Skill 编译之后附加 `Selection`，随原 blocked CAS 私有保存。首次归档从真实 current 重派生 MCP 声明并逐项比较；配置身份核对原 accepted 操作绑定。完成材料 append-only，三后端 clone/file/scoped reopen 保持。
5. 独立读回先验证既有完整 Skill 来源，再从同 request/payload 的 Skill archive 和完整 registry 重编译，精确核对全部结果。不以自洽重哈希或 caller 的成功标志授信。
6. 恢复/GET/重放/重开不再观察，不补编旧 registry-only 历史；revision 漂移切点只保留原 registry、不产生新选择。旧历史的可读性不允许通过 CAS 回写覆盖当前完成记录。
7. 公共 DTO/Web 本轮未新增字段或新原因码；仍仅暴露原 `mcpRegistryEvidenceRef` 和 `blocked/mcp_registry_observed_connection_pending`。选择细分失败仅保存在私有材料；尚未完成公共细分展示。编译成功、真实空集合均不授 PASS/N/A、连接 readiness 或 M3。

按 go-multi-module-dev 核对，沿用 Workspace 私有 reducer/CAS 和既有 typed owner 查询；无新运行单元/topic、跨 owner service/repository 或可变资源句柄。Runtime SM-1～SM-4、Start/Restart 执行授权与 STEP05 均未变。

## 验证与证据索引

受限原证据根：`/tmp/workorch-step04-mcp-selection-pq0UZZ`（0700）。日志、源码快照和二进制不提交 Git；临时目录不是长期证据归档。

| 验证 | 证据与范围 |
| --- | --- |
| 精确纯编译 | `preparation_mcp_selection_test.go`：默认/required 合并、clone、空值/缺来源区分、17 类精确负向分类、合法来源、原文与完整 revision 反证、typed 覆盖与重复键拒绝 |
| 激活语义对照 | `TestPreparationMCPDefaultsPreserveActivationSetSemantics`：有效输入与既有 activation 默认解析/递归覆盖/required union 按集合等价；不要求旧无序结果顺序一致 |
| 原操作切点 | `TestPreparationMCPRegistryOriginalCutsAndReopen`：完整/空库、错误类型、双读漂移、超限、写失败/回执丢失、revision 漂移、恢复与零 Run；缺 Skill 明确失败，stale 无选择编译 |
| 三后端 CAS | `TestPreparationMCPRegistryRepositoryOriginalCASCloneAndReopen`：memory/file/scoped，含另存 Soul 绑定；独立合法但伪造声明的首次 CAS 拒绝、内容/版本篡改、原历史 append-only/clone/reopen |
| 真实回环 HTTP | `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen`：空库与非空选择两分支，真实 MCP/File/Workspace/Soul/Skill/Memory/Run owners，固定非敏感 provider/SkillHub fixture；首次双读，GET/重放/恢复/owner 重开后仍两次，结果始终 blocked、零 Run |
| 定向测试 | `targeted-final2.log`；随后新增 Skill version/scope 反例由最终回归覆盖 |
| 受影响包 | 最终 21 个有测试包通过、4 个包无测试；`affected-go-final.log`，`-count=1`；`affected-go.log` 仅为中途全包结果 |
| 定向 race | `race.log`：5 个有测试包通过，不是全仓 race |
| 静态/边界 | `vet.log`、`fmt.log`、`diff.log`、`boundary-final.log` 通过；边界日志保留既有 pycache binary-match 提示 |
| 构建身份 | `source-snapshot-final.json`、`build-final.log`、`build-identity-final.json`、`build-produce-final.log`、`build-verify-function-final.log` |

最终受影响包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/blocks/mcpregistry/... ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/pkg/repository ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore -run 'TestPreparationMCP|TestPreparationWorkflowHTTP' -count=1
```

纯模型测试的非空 registry revision 由 test helper 按 owner 格式计算，不冒充真实 owner 证据。HTTP 的非空完整 revision 则由真实 MCP Registry owner 派生；fixture 只预置 metadata，`never-start` 不执行。候选准备使用本地非收费 provider，准备阶段没有真实推理/embedding 或 MCP transport 调用。owner 重开在同一测试进程内，不是独立 OS 进程 stop/start；本轮未跑生产 Chrome、未重新构建 Web，也不以局部 HTTP/组件或旧构建证据代替完整生产验收。

## 本轮发现、修正与复验

1. 初次真实 HTTP 结算返回 409（`targeted.log`、`http-debug-config.log`）：底层 scoped metadata 将 Soul 绑定另存，新校验却重算完整配置 hash。修复为真实 metadata 独立派生声明、完整配置身份绑定原 accepted 操作；不将不完整 metadata 当作完整配置，不降低声明逐项比较或历史不可变门禁。新增有 Soul 绑定的三后端反例与正例，最终 HTTP/回归通过。
2. `fixed-http.log` 发现旧 context-less 历史夹具仍携带新版 MCP 编译对新版 Skill archive 的 content hash 引用。旧快照反例改为同时不含尚未存在的 MCP 编译，验证可读但 CAS 不可覆盖；没有给新编译忽略引用漂移。
3. `http-nonempty*.log` 中新增夹具错误期待 owner 既有规范化会保留末尾 CRLF；实际既有 owner 对 Description trim。该既有行为本轮未改。最终夹具使用内部 CRLF，并逐项比较选中值与真实完整 owner 回执；原 Skill 指令 bytes/NUL/CRLF 全链仍保留。不把 owner typed snapshot 称为磁盘原字节。
4. Go build 退出 0，但保留 module stat-cache 只读告警。已有 build-identity CLI verify 问题本轮不改，使用公开 Python `verify()`、固定完整四字段核对；不记作 CLI verify 通过。

## 最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮累计未提交源码，不是该干净提交的独立验证。
- buildInputSha256：`f00a6fba51a37443d094b70875b7903bd0dcf2410de350999c4499cf0cdffed4`
- consoleTreeSha256：`97283a5a8317ba0a663ab7f71856f2887d62fa3355fabe18704bd3aff7181b6a`
- binarySha256：`96a21a5a49027b95158162998d20c310f982ef29c5f69850dd25fbddf1a58caa`

这些身份仅核对本地输入/嵌入控制台/产物字节；不是可信构建 attestation、部署来源证明或 M3 凭据。上轮二进制与哈希不用于声明本轮源码已验证。

## 剩余收口顺序

1. MCP 选择编译的公共分轴/精确阻塞消费者、required connection 与原模式/连接 owner 事实、持续有效性；不能把本轮成功编译当成连接成功，也不能把空选择无依据变成 N/A。
2. Provider 精确 target/config/capability/授权/freshness；Memory 非空 semantic 精确查询/target/预算与真实语义证明；Skill 辅助引用唯一来源/root、真实读取与 freshness。保持零收费，不回退模式、不改外部 SkillHub。
3. exact_seal/persistence 等剩余单项消费者、全部 required 合取、完整 no-Run 证据、reservation/epoch/final CAS、首次 M3 正结算及并发/中断/迟到/漂移矩阵。
4. 最终源码和产物上的生产 HTTP 成功/阻塞、Chrome 与独立 OS 进程重启。当前局部 blocked 链不满足完整 STEP04 live 退出条件。
5. 既有 public fullflow Profile 断言、Orchestrator 套件成本、build-identity CLI 问题独立跟踪；本轮未重跑全仓 CI，不能宣称全部已绿。

本轮不进入 STEP05、不创建 Run、不连接真实 MCP、不读写 live 配置，不使用真实模型或 embedding。
