# STEP04 Provider 精确容量预算观察与消费验证记录

日期：2026-10-26。范围仅 STEP04；基线 HEAD 为 `834aa350cc4e77379819666540b9912a7c22b64c`，验证对象包含累计未提交源码。不自动提交，不修改 live 配置或既有服务，不进入 STEP05。

## 本轮收口及责任边界

1. 在既有 LLM cache-only typed query 增加可选精确 Target；nil 才使用本次冻结 owner 默认。Workspace 只有首次接纳 CAS 明确成功后，才向原 Application continuation 返回 detached 原 provider 选择声明及受理配置 hash。该声明不持久为新权威、不回传用于授权；重放和恢复不返回新观察意图。
2. LLM owner 只从同次已观察的 fresh cache 和冻结配置调用既有纯 `ResolveProviderModelTarget`，生成 `workorch-preparation-provider-budget-observation/v1`。保留精确目标、context/output/input/request-byte 容量、估算比例和 capability revision；不刷新目录、不等待刷新、不调用模型、不创建 request allocation。
3. 未复用要求 Run/authority 身份、可能走 catalog 获取路径的 Runtime ceiling query；没有伪造 Run。容量观察不携带 endpoint、API key、credential-derived ConfigurationRevision 或私有 cache-key。原 DTO 明确只定义非 secret 容量字段，不将该预览称为完整 Runtime ProviderCeiling。
4. Workspace 在原 accepted → blocked 结算保存完整 owner 观察与选择编译，核对原 Workspace 目标，读回独立重编译。容量不可用、目标冲突分别明确阻塞，不用 owner 默认或相邻模型代替。新容量字段省略 nil，旧无容量历史不在 GET、重放或恢复时补算，不因新增字段重写旧 hash。
5. 新有限公开原因只有 `provider_selection_budget_observed_authorization_pending`、`provider_selection_budget_unavailable`、`provider_selection_budget_source_conflict`。三者均要求原 evaluation version、required/llm/blocked 形状和原 evidence ref。公共 JSON 不输出私有容量材料或配置身份；Web 保留原 code 和有限中文说明，不增自动请求或浏览器持久化。
6. 容量预览只关闭点时精确容量观察和原操作消费，不证明远端授权、连通性、当前 owner epoch、持续 freshness、实际请求预算或 Run 物理预算；不授 PASS、N/A、ready、M3 或执行权限。miss/expired 不附容量，原失败分类保留。

沿用 go-multi-module-dev 的 concrete typed EventHub、既有 owner 和 Workspace 私有 reducer/CAS；没有新运行单元/topic、跨 owner service/repository、Runtime 驱动或 Start/Restart 变更。SM-1～SM-4 未变。现行合同同步到 LLM/Workspace readme 和 service-api；本记录不是设计权威。

## 验证及证据索引

受限证据根：`/tmp/workorch-step04-provider-budget-jvDuUR`（0700）。原日志及二进制仅在本机，未将凭据、live 配置或原诊断提交到 Git。

| 验证 | 结果与原证据 |
| --- | --- |
| Provider 定向 Go | 五包通过，`targeted-final.log`；首次接纳声明隔离另见 `continuation.log` |
| 定向 race | LLM biz/events、Workspace models/biz、managementcontract 五包有匹配测试并通过，`provider-race.log`；同命令的 Workspace events 为 `[no tests to run]`，不计为该包已执行 race 用例 |
| 新增 result 反例 | 全包回归后补加 exact/default/fresh-source 反例；events 定向 race 再次通过，`events-final-race.log` |
| 受影响全包 | 20 个有测试包通过、2 个包无测试，`affected-go.log`；不是全仓 CI 或 Orchestrator 全套 |
| Web 定向 | TypeScript 与 preparation 54 项通过，`web-targeted.log` |
| Web 全组件 | TypeScript 与 13 文件、183 项通过，`web-components.log`；保留已有 Ant Design 警告 |
| Web 规则 | 11 项通过，`web-rules.log` |
| 生产控制台 | TypeScript/Vite 构建通过并更新实际嵌入资源，`console-build.log`；保留大 chunk 警告 |
| 静态/格式 | `vet.log`、`fmt-final.log`、`diff-check.log` 退出 0 |
| 边界 | `boundary-rerun.log` 通过；保留已有 pycache binary-match 提示 |
| 构建身份工具 | 16 项通过，`build-identity-tests.log` |
| 本轮二进制 | `source-snapshot.json`、`build.log`、`build-identity.json`、`build-produce.log`、`build-verify-function.log`；build、produce 和公开 Python verify() 退出 0 |

受影响全包命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/modules/blocks/llm/biz ./internal/modules/blocks/llm/pkg/events ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向 race 选择器为 `TestPreparationProvider|TestPreparationCatalogCache`。新增覆盖包括：容量字段正数及组合上限、NaN/Inf/混合失败字段拒绝；缺配置/过期/miss/异目标拒绝；默认与显式目标分离；catalog 与 exact override 的容量逐字段等于既有生产纯 resolver；私有配置标识不外传；clone 隔离；重哈希篡改编译及完成历史均拒绝；初次声明与原配置绑定，重复/恢复不授新观察。

真实 LLM owner 集成回归分别覆盖 owner 默认与不同的 Workspace 显式目标，使用本机 `/v1/models` fixture 显式预热缓存。预备阶段请求基数为 1；准备操作、重复及恢复阶段新增 HTTP 为 0，其他请求为 0，Run inventory 为 0。停止 LLM owner 后重放/恢复仍返回原值；file owner/repository 重开保留原容量。它是同测试进程内的 owner/repository reopen，不是独立 OS stop/start；本机 HTTP fixture 不是新二进制部署或 Chrome 验收。

失败与修正原样保留：`initial.log` 为 sandbox 禁止 fixture socket；`targeted.log` 为测试中 error 变量遮蔽导致编译失败，修正后 `targeted-final.log` 通过。`boundary-final.log` 因遗漏 GOCACHE 而命中只读默认缓存，重设隔离缓存后 `boundary-rerun.log` 通过。没有跳过 gate 或放宽断言。`build.log` 保留 module stat-cache 只读告警，但构建退出 0。

## 本轮本地输入及产物身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加累计未提交源码。
- buildInputSha256：`d4aa0c2fd8dce4222c6eeef34e102470d1f36c2f6fabf5b851182a580b323af9`
- consoleTreeSha256：`7197ea63ccc98943af316df3c787d66c7f9e8f74edccf0bdede925abad6228ea`
- binarySha256：`9ce0a6187e073fe6f7d024453bbc3bf325f1a497839a47bf100600b2c6e8323f`

公开 Python verify() 使用上述固定四字段核对；既有 CLI verify 问题未在本轮修复，不记为 CLI verify 通过。身份只证明本地输入/产物字节一致，不是可信构建 attestation、部署来源或 M3 凭据。上轮 Run 历史产物不作为本轮源码的构建证据。

## 未闭合项及下一步

1. 用户选择提供网关只读模型授权接口，但后续提供的是连接信息，尚无真实 endpoint/method、精确 target/operation 权限语义、有效期及可信回执结构。连接信息或 `/models` 列表不能证明推理授权；不探测猜测的管理路径、不发收费推理/embedding，保持 blocked。接入前需核对实际接口合同；若网关尚无此接口，应单独确认候选合同和网关实现边界。
2. Provider 仍需实际请求/Run 预算的既有权威消费、当前 owner epoch 与持续有效性，不能把本轮容量预览提升为执行额度。
3. MCP required connection、连接 owner 事实/epoch及空选择适用性；Memory 非空 semantic 的精确 query/target/预算/真实语义证明；Skill 辅助引用唯一 root 来源、真实读取及 freshness 仍需继续，不猜路径、不回退模式、不补造授权或语义事实。
4. 全 required 合取、no-Run reservation/epoch/有序观察/final CAS、首次 M3 正结算和并发/中断/迟到/漂移矩阵仍未完成；已有观察归档不能替代该门槛。
5. 最终产物的生产 HTTP 成功/阻塞、实际 Chrome 和独立 OS stop/start 尚待最终功能闭合后执行。本轮没有新的生产进程/浏览器 live 验收，没有修改既有服务或 live 配置。
6. public fullflow Profile 断言、Orchestrator 套件成本和 build-identity CLI 既有问题独立跟踪；不宣称全 CI、完整 STEP04 或 live 验收退出条件已满足。
