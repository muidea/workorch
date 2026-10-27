# STEP04 原执行策略输入归档与纯编译验证记录

日期：2026-10-27。范围：继续非 Provider 主链；不是 STEP04 全部退出报告。

## 本轮落地

1. 原显式检查的 Application continuation 双读非敏感进程输入：配置对话语言、LC_ALL/LC_MESSAGES/LANG 原值、Runtime 修复上限、文档恢复上限。缺少真实 loader、取消、非法值、完整性超界或双读漂移不构造默认观察。
2. Workspace 原 `accepted → blocked` 私有结算在已有受限 seal archive 内保存版本化 `ExecutionInputs`。模板取自原确认 authoring snapshot；完整 Workspace 配置前像由本 owner 实际捕获，不接收 caller 模板、策略或成功标志。
3. `WorkspaceConfigurationSnapshot` 保持原配置 hash 的字段名、顺序和 nil 规范化；测试独立保留旧 wire 算法作等价比较。仅明确捕获时 clone，不给已有配置 hash 热路径增加全量深拷贝。
4. 原来源、模板全文、配置 hash、scope/revision、观察时窗及持久引用均重新核验；完成材料沿用 clone、0600、append-only 门禁。分目录适配器在本地持久锁内读取真实 Soul sidecar 并比较完整前像；底层 metadata 仅核对自身保存字段。不承诺对外部文件修改的跨文件原子性。
5. `activationports.CompileOriginalPreparedExecutionPolicy` 从完整持久原材料纯编译 typed execution contract、Workspace evidence policies 和原对话语言；复用既有策略 decoder，无 context/Hub/config loader/环境读取。缺来源、漂移或无效策略拒绝。它是已实现且测试的纯 helper，**尚不是生产完整 PreparedBootstrap 的组装/归档接线**。
6. HTTP/管理 DTO/Web 仅增加 `executionInputVersion` 和“原执行策略输入”展示。该版本描述原材料已保存，不表示策略编译成功、当前 ready、完整 bootstrap 或 M3。GET、重放、恢复、重启不补造旧原输入。

本轮使用 go-multi-module-dev 的既有 owner/typed EventHub 边界：不新增运行单元或 topic，不跨 owner 直读/写库。Workspace preparation 仍是原 owner-private 事务，不改变 SM-1～SM-4，不进入 STEP05。没有修改 Provider 选择、授权、预算或探测行为；配置前像中的模型目标仅是既有 Workspace 声明，不含网关凭据，也不是授权。

## 自动回归

以下为作者本轮执行结果，不是独立 CI 或发布结论：

| 验证 | 结果 |
| --- | --- |
| Workspace/Application biz，`-run 'TestPreparation\|TestPreparedBootstrap\|TestExactConfirmedStartup' -count=1` | 2 包通过，含回环 HTTP fixture；57.522s / 11.710s |
| 新原输入/进程输入定向 `-race` | 2 包通过；8.828s / 1.115s |
| Workspace models/startuppreparation/events、activationports、managementcontract | 5 包通过 |
| repository/workorchrepo，`-run '(Preparation\|WorkspaceConfiguration\|WorkspaceScoped)'` | 2 个有匹配测试包通过；file/memory 两个包本次 regex 无匹配，不记为额外测试通过 |
| `go vet` Workspace、Application biz、workorchrepo、managementcontract | 通过 |
| Web `npm run test:components -- --reporter=dot` | 13 文件、196 项通过；保留既有 Ant Design warning |
| STEP04 verifier 单元测试 | 13 项通过；新增完整私有输入/真实 sidecar 来源反例 |
| `GOCACHE=/tmp/workorch-go-cache make verify-boundary-contract` | 通过 |
| `git diff --check` | 通过 |

新增覆盖：真实 loader 双读与原语言优先级；缺失/取消/配置或 locale 漂移；内存、文件、分目录存储的首次 CAS、clone、重开；配置/模板/时间/版本自洽重哈希篡改；完整 sidecar 前像；中断、stale、旧无材料历史和重复调用不回填；公开版本校验、只读开页及零自动执行。

普通沙箱不允许 loopback socket，相关 HTTP 测试在获准的隔离回环执行环境中复验通过。首次边界命令因默认 Go cache 只读失败，指定上述 GOCACHE 后通过。不把这些环境失败记为通过，也未运行全量 CI。

## 生产构建 HTTP / OS 进程验证

命令：

```sh
GOCACHE=/tmp/workorch-go-cache python3 scripts/verify-workspace-preparation-step04.py --evidence-dir /home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-execution-inputs-20261027-d
```

最终结果 `pass=true`。证据目录受限且 Git ignored，不提交原配置、业务输入、原始 HTTP 或运行日志。索引：`result.json`、`build-identity.json`、`http-trace.json`、`original-owner-record.bin`、`final-owner-record.bin`、`process-1.log` 至 `process-3.log`，以及逐请求原始响应。

- 3 个不同原请求，各自一次显式检查；全部原输入随原结算保存，验证器逐项检查私有完整模板、配置前像、真实 Soul sidecar 和 process before/after，不只看公开 marker。
- 正常资源时 10/11；移走原 managed config 后新检查 9/11；人工恢复原文件后新检查回到 10/11。前两次原记录不被覆盖。
- 丢失 HTTP body 后 GET 对账，以及原请求 check/recover 重放，保持同一身份与完整历史。
- 3 个独立 OS 进程、2 次 stop/start；重启后全部原操作读回/恢复不改写 owner bytes，不重做 SkillHub status。
- 8 次本地 authoring provider fixture 响应只用于准备候选，不是真实模型。STEP04 阶段 Provider HTTP（含 models GET）0，embedding 0，Run 0；业务文件未修改。
- 空 SkillHub fixture 只证明本轮有界空选择分支，不是实际非空资源 readiness。
- `ready=false`、`M3Issued=false`、`browserVerified=false`。本轮未重跑实际浏览器；组件测试和生产资源构建不能替代该证据。
- PIDs 3798691、3800324、3802025 已停止；没有修改现有服务或用户数据。

最终构建身份：

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` + 未提交源码 |
| buildInputSha256 | `e44b0d89fb37dd6c072875e57e939c26c31adbdc75daa90d1f192bdbc3ff4705` |
| consoleTreeSha256 | `3712830b5ba6f330e62adca5ace99f8447a74ad524b0d976a637d4fb6b1d538f` |
| binarySha256 | `ca91cdeb7c23d8c6be1003494db376869b91152a2d43d46ed31faf17dc34bfe2` |

验证器结束时使用公开 Python `build_identity.verify()` 完整比对本地源码/控制台/二进制原值，不称为 CLI verify 或可信构建 attestation。

开发失败证据分别保留在相邻 `-a/-b/-c` 目录：a 构建期源文件变化被身份门禁拒绝；b/c 暴露 metadata 与真实 sidecar 分离导致的结算 409。修复后以新目录 d 完整重跑，不把先前失败改成通过。临时定位代码已移除。另修正进程设置观察顺序，保持已有 Soul 配置双读的漂移测试语义；最终累计回归通过。

## 尚未完成的主体项

1. 将已归档的原执行策略与真实 Soul/Skill/Memory/MCP 等选择材料组装为完整 PreparedBootstrap，接到生产原操作的不可变归档/恢复链；当前纯策略 helper 和来源义务摘要均不能冒充此项。
2. owner current-validity 完整合取，以及按 creation epoch 唯一的历史 M3 正向 final CAS。已有 point-in-time PASS、原 no-Run fence 或 10/11 不提供持续有效性或完成权。
3. 有界空资源 fixture 之外的非空 Skill/MCP、semantic Memory 等输入，仍须消费其要求的真实 owner 事实；缺证维持明确阻塞，不能靠空集合替代、启动 MCP 或收费调用补绿。

Provider 授权问题继续排除在本轮实现范围外。不宣称“除 Provider 外全部完成”，不宣称整个 STEP04、真实正向 live、全 CI 或发布验收通过。本轮可以重跑的是已验证的无收费阻塞主链。
