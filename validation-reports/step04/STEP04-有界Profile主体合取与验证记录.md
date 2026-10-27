# STEP04 有界 Profile 主体合取与验证记录

日期：2026-10-26（Asia/Shanghai）。基线 HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，加当前累积未提交修改。

## 本轮结论

按“优先主体流程、停止展开零散细节”的要求，本轮将既有资源观察接到**原清单完整合取、同次结算持久化、HTTP/Web 展示、原身份重放与 owner 重开**，并补齐一个有界 Profile 的 Skill/MCP 精确空选择消费者。

最终真实回环 HTTP 用例记录：`required=11 passed=10 outstanding=provider_cache; M3 not issued`。
这表示**该有界 Profile 的非 Provider required 检查全部通过**，不是整个 STEP04 只剩 Provider，更不是 ready 或 M3。
其余已选 MCP、缺失/禁用 MCP、Skill 来源缺失等组合仍按各自原事实阻塞，没有将精确空选择规则推广到它们。

本轮不改 Provider，不访问真实网关，不发推理/embedding，不启动 MCP，不创建生产 Run，不进入 STEP05。
整体成功的纯编译接线、有效性汇总、M3 最终结算以及生产部署验收仍是明确的主体待办，不用更多诊断小项替代它们。

## 主体代码变更

### 原 required 清单的完整合取

新增 `workorch-preparation-all-of-assessment/v1`。清单身份从原 candidate 重建，包括固定十类检查和全部规格资源项；逐项精确核对 ID、owner、required、数量及顺序。
不得删除 Provider 后重算一个自洽的成功汇总，也不得降 required、换 owner、重复或加入未知状态。

汇总记录 required 总数、通过数和全部未通过项，在原 owner 结算的同一次 CAS 保存；独立 persistence gate 从原来源重算。
GET、旧历史读取、普通重放不补造汇总，原失败/中断与全部 owner 材料保留。可选项失败仍显示在原 Checks 中，不阻塞 required all_of。

`blocked`/`checks_passed` 只表达检查清单的合取结果，不是资源 freshness 租约、ready、M3 或执行授权。
生产路径本轮仍只能到 blocked；纯聚合测试里的 checks_passed 不构成 owner 成功或生产 M3 正例。

### 精确空选择的有界正向消费

新增 `workorch-preparation-empty-selection-evaluation/v1`，仅消费本次正常、非 stale 结算中完整封存的原来源。
要求完整 Soul 阶段、真实 Registry 与 SkillHub 历史、原 scope/request/payload/revision/binding，并反查封存 Goal 原文/hash 和原 DoD 集合/hash。
从完整材料独立重编译 selection/context/references/status，与归档原值比较；只有真实选择、匹配、指令上下文和引用项均为空且无失败时，Skill 单项才能通过。

MCP 还要求完整双读及配置/依赖编译无失败、selected server/capability 均为空。缺材料、非空、漂移、中断或缺消费者版本仍阻塞。
原编译诊断（包括空选择原 blocked reason）不改写；消费者版本另行记录正向结论。
这只证明原时点的空选择闭合，不授持续有效性、Provider 权限或 M3，也不执行 MCP。

### HTTP/Web 与架构边界

新增窄 typed assessment DTO，严格核对版本、manifestHash、计数和未通过项完整顺序。Web 展示“原检查通过 X/Y、剩余 N 项”，仍将操作状态、当前有效性、真实 ActiveRun 和历史 M3 分轴。
缺旧汇总时明确无汇总，不在页面补算权威结论，不新增 Start 或浏览器持久存储。

按照 go-multi-module-dev 的 owner/EventHub 边界实现：本轮仅扩展 Workspace 私有准备结算；不新增运行单元、不跨 owner 直写，不改 Runtime SM-1～SM-4。
两个新增 state-like 字段登记为 derived_projection，不冒充 Runtime 迁移状态。现行 service-api 和 Workspace readme 已同步实际合同；本记录不是设计权威。

## 验证与原始证据

受限本地证据根：`/tmp/workorch-step04-mainline-ycUYfA`。不将原始诊断、运行配置或二进制提交 Git；临时路径不是长期归档承诺。

| 检查 | 最终结果 | 原始证据 |
| --- | --- | --- |
| 受影响 Go 完整回归 | 15 个有测试包通过，2 个包无测试文件 | `affected-go-final.log` |
| 定向 race | models、management、repository 根包、Workspace biz、Application biz 通过；repository 三个子包为 no tests to run，不计作实际 race 用例通过 | `race-final.log` |
| 回环 HTTP 六场景 | workflow、empty_selection、mcp_selected、mcp_missing、mcp_disabled、skill_refs_missing 全通过 | `http-six-scenarios.log`；最终源码另由完整 Go/race 覆盖 |
| 最终有界 HTTP 正向消费 | 11 required，10 passed，只剩 provider_cache；同原身份重放、恢复及 owner 重开一致 | `http-bounded-final.log` |
| Web 组件 | 13 文件、194 项通过 | `web-components.log` |
| Web 规则 | 11 项通过 | `web-rules.log` |
| vet | 受影响范围退出 0 | `vet.log` |
| 完整结构边界 | 退出 0 | `boundary-complete.log` |
| 生产 Web 构建 | 退出 0；Vite 既有大 chunk 告警保留 | `console-build.log` |
| Go 二进制构建 | capture 后重新构建，退出 0 | `binary-build-final.log`、`workorch` |
| 构建身份测试 | 17 项通过 | `build-identity-tests.log` |
| 构建身份 CLI verify | 固定四字段身份校验退出 0 | `expected-identity.json`、`build-identity-verify.log` |

完整 Go 范围：Application biz/server，Workspace 全子树，management contract，repository 全子树。
命令为 `GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/workspace/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/repository/... -count=1`。

定向 race 在 models、management、repository 全子树、Workspace biz、Application biz 执行，筛选：
`TestPreparation(AllOf|Assessment|WorkflowHTTP|NoRun|SkillHub|Record|Memory|Provider|MCP)`，`-race -count=1`。

HTTP 使用测试进程中的真实回环 listener、生产 transport、持久 Workspace/File/Soul/Memory/Registry/Run owner。
SkillHub status 使用本地适配器 fixture，Provider 使用 cache-miss fixture；前序候选由本地 provider fixture 准备，不代表真实模型验证。
空选择场景通过真实持久 Soul allowlist 排除现有 catalog，观察与编译完整、结果精确为空，不通过缺失观察或删 required 达成。
原身份重放/GET/recover 不重复 status 调用，既有 Run inventory 为零断言保留。
这里的 owner 关闭/重开不是独立 OS 进程 stop/start，也没有执行实际 Chrome 验收。

反例覆盖：缺 Skill/MCP 原材料、缺消费者版本、假计数、删除 Provider 后重新计算自洽汇总、缺项/重复/降 required/换 owner、未知状态、clone 隔离。
Web 新测试覆盖整体汇总、伪造成功拒绝、empty selection 消费版本，以及 stale/真实 Run 与整体 blocked 分轴；没有执行调用。

早期失败保留，不抹除：回环监听曾因 sandbox 权限拒绝；旧场景固定非空数量断言不适用于新增精确空选择；boundary 首轮有默认 Go cache 只读及新增派生字段未登记问题。
完成对应修正后以上最终回归通过。原失败日志仍留在证据根，不把首轮失败说成通过，也没有放宽生产门禁。

## 最终本地内容身份

生产 Web 构建后 capture，随后重新构建二进制，再 produce 并按独立固定的四字段 JSON 执行 CLI verify。

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`，加当前未提交构建输入 |
| buildInputSha256 | `6c2e73dcde99e5285f885e9934e7424d1c74bd7e603fcb1b2019be1ec5df28a1` |
| consoleTreeSha256 | `d374b80a759c6c71fc68acb42319ae427d153861ec16269b77529eb331ff089d` |
| binarySha256 | `b1b7b628ff884c3209440e2130d62c4805d4c0379cf90eed0e9fb207ec9ce95a` |

对应 `source-snapshot.json`、`build-identity.json`、`expected-identity.json`、`workorch`。
这是本机字节身份与构建前后输入一致性记录，不是可信构建 attestation，也不表示最终 Git 提交、部署来源或二进制运行验收已经成立。

## 后续只沿主体出口推进

1. **先完成成功事务接线**：原 owner 检查完整通过后，消费现有纯 bootstrap 编译，汇总真实有效期/epoch，在原操作里保存成功结果；Provider 未获可信事实时仍阻塞，不能跳过。
2. **完成 F07 唯一历史 M3**：最终 CAS 重核 exact seal、creation/preparation epoch、pending 和 retirement-aware no-Run 证据；保存 completionID，与当前有效性分轴。覆盖原身份恢复与 reservation 竞争。当前这些成功结算尚未完成，不能归因成仅外部 Provider 阻塞。
3. **再做生产验收**：最终构建的回环 HTTP/实际浏览器、正向/阻塞/恢复、独立进程 stop/start，保留历史与重启失效证据。不存在正例条件时如实报告 blocked，不用 fixture PASS 冒充生产事实。

非空资源组合、更多诊断和展示优化排在上述主体出口之后。本轮未运行全仓 CI，不声称全 CI、完整 STEP04 或 live 验收通过；既有 integration/orchestrator 验证阻塞未在本轮修复。
