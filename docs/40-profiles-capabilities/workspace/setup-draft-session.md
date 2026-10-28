# Workspace 规格创作

Status: normative

Type: profile-capability

## 创建与确认

Web 创建输入包含名称、Template、Goal、本地资源引用、可选工作目录和额外验收要求。工作目录留空时由 owner 解析；资源引用相对于该目录。页面编辑不是服务端已保存输入。Web 不使用 localStorage、sessionStorage、IndexedDB、cookie 或其他浏览器持久机制保存业务数据；未保存输入及客户端重放身份仅暂存在页面内存。

Workspace owner 持久保存创作输入与请求身份；派生验收要求后生成结构化规格提案，并进行六类独立审查。只有当前 input、derivation permit、proposal/specification hash 和 revision 精确匹配且审查通过，用户才可确认并准入。创建、编译、确认均不创建 Run；执行是后续独立 intent。

派生先核对 Goal 及资源中明确列出的功能、约束与验收主项；未被用户／模板条目具体承接的主项须进入候选补充项，保留完整条件、例外、阈值和范围，再补充边界。通用模板或 Goal 全文不能替代逐项覆盖；提取项仍是待审、待确认的派生提案。超出既有项数／字节边界明确失败，不摘要、截断或删难项。

旧 process-local setup draft session、直接 Goal/DoD 写入及手工绕过审查的创建入口已退休。本文保留原路径作为文档链接，不表示旧入口仍可使用。

## 结构化输出合同

新事务在 reserve 时选择并持久绑定 `workorch.authoring.owner-bound/v2` 及阶段 schema hash。派生仅返回 `AdditionalItems`；提案顶层仅为 `mapping`，保留维度／分组引用、候选来源分组、资源 ID 引用及验证声明；审查顶层仅为 `review`，包含 `Status`、`Reason`、`EvidenceRefs` 与 `RequirementIDs`。provider 不回显 protocol、input/template/resource hash、review kind 或 proposal hash。Workspace owner 从原 permit、封存 input／derivation 与阶段构造独立 typed 结果，生成 Required、资源事实及 canonical IDs，保留完整原 Goal、各候选原文与来源。原 provider bytes 不补字段、不重写；typed 结果不能冒充 provider 原文。

旧账本空 wire 身份继续按原 v1 解释：mapping 必须携带 `workorch.authoring.sealed-ref-mapping/v1` 及原三个 hash，审查必须携带原 kind/proposal hash。新旧合同不互相 fallback；已有失败、未决或已消费 permit 不因升级获得第二次调用许可。请求提供完整闭合 JSON Schema，独立审查仍绑定同一原 proposal，不能读取其他审查结论。`all_of` 不授权模型添加 `ConditionMode` 或其他合同之外的字段。

稳定 schema 与精确字段说明放入 C0，当前 Goal、候选验收项、资源和身份放入动态请求。派生、提案和审查均使用当前 DTO 的闭合字段形状：所有必填字段必须出现，字段名大小写精确，数组、对象和标量均不得为 `null`；布尔值与整数不得使用字符串、浮点数或指数表示替代。解析在 256 KiB、32 层的边界内拒绝未知／重复字段、多 root、尾随内容、非法 UTF-8 和孤立 UTF-16 surrogate，不提取 fenced JSON，不修复或改写 provider 原文。

`Requirements.SourceItemRefs` 仅引用封存候选验收项，每项恰好属于一个分组；`Obligations.CandidateItemRef` 对每个候选恰好出现一次，并引用包含该候选的 requirement。`GoalRequirement` 单独引用维度与资源，不把 Goal 混入候选分组或伪造成 DoD obligation。`ResourceIDs` 仅引用 owner 派生的当前资源 ID；资源章节、路径或 CWI 身份不能替代候选来源。Verification 仅提出 `Declaration`、`Status`、`ProfileID`、`EvidenceRequirementID`、`Scope`；`UNVERIFIED` 的三个绑定字段必须全空，`DECLARED` 必须绑定可信 Profile 声明，不能把自由文本的预期证据当作声明身份。

独立 Goal requirement 仅在 Required、`USER_EXPLICIT`、唯一 Goal 来源和全文精确一致时沿用 Goal 输入的 32 KiB 字节上限；普通分组 requirement、候选验收项及 obligation 仍为 4096 字节。边界在既有输入规格规范化后检查，不截断、摘要或改写超长文本，不扩大 provider 文档、请求或阶段预算。原 provider 文档独立按 bytes 保存，规格规范化不能覆盖诊断原文。

六类审查的应审集合由 Workspace owner 从 canonical proposal 的全部 Required requirements 派生，包含独立 exact Goal。每个 critic 请求以 `requiredRequirementIDs` 明确提供同一候选的完整集合；PASS 的 `RequirementIDs` 必须覆盖该集合，允许额外审查合法 optional 项，但不能替代缺失 required 项。没有资源引用、负面路径或回归声明不能据此免审，仍须检查相应约束是否遗漏；不接受 provider 自行声明 N/A 或空范围豁免。FAIL／UNKNOWN 可以保留实际未完成的审查范围，但不能确认。模型、持久 ledger、provider 本地解析和确认准入执行同一覆盖规则，漏审诊断保留 review kind 与缺失 Requirement IDs，不补写原结果。此门禁仅证明记录范围齐备，不证明自然语言语义充分性、不授予运行 credit，也不表示 M3 创建完成。

无绑定 `UNVERIFIED` 可以保存在草稿中，但即使六类审查全部 PASS，也必须显示为 `review_blocked`，不得确认或物化正式 Goal／DoD。确认要求所有 Verification 为 `DECLARED` 且声明非空，每个 Obligation 至少有一份声明。Workspace owner 在提案 completion、确认准备及 prepared 恢复时独立核对封存 Template 实际编译的 Profile，正式准入再核对当前 Workspace／Template 实际合同。列表摘要与详情也须核对该封存 Profile，不能因六审查 PASS 把不适用的自报声明显示为已审查或可确认。声明适用性必须覆盖该义务的全部确认后 DoD source，并在最终合并策略中保留贡献；声明通过不代表当前 capability／owner fact 已就绪，也不授予执行 proof 或终验 credit。

proposal 与 review 的动态输入提供 `sourceApplicableVerificationDeclarations`，按候选列出封存 Profile 的来源适用声明身份；由同一确认后来源解析与合并保留规则纯计算。该集合仅辅助精确引用，不能证明自然语言语义可验、替代 Profile 全文或自动提升为 `DECLARED`。文档性质本身不等于不可验证，存在普通 validation 也不证明任意文档属性可验；state_change、Git 与终态交付义务仍须按各自合同合取核验。

provider 原生 structured output 仅在能力明确支持时启用；不支持时仍提供完整提示合同并执行相同本地严格校验，不自动 fallback 或重发。显式 schema 构造错误必须明确失败，不能静默省略约束。JSON Schema 只约束结构，不替代原文保真、严格解析、来源／资源／Profile 校验、六类审查、用户确认或 owner 准入。

## 成功提案原文审计

v2 派生、提案及审查的成功 completion 均保存完整 provider document 和独立 wire 观察；owner 重开时按封存 wire 版本重解码原 document，与独立 typed 派生／mapping／审查精确比较；mapping 还重新编译 canonical proposal。原文 byte count／SHA256 与 permit、mapping 共同绑定 completion 身份，但不是准入或执行权威。document 必须完整且已保存，wire 可未观察、部分观察或超限未保存，必须明确标记 observed／complete／saved／reason；不得用 wire 缺口覆盖已知成功文档。

普通创作记录及 DTO 只携带审计 metadata。显式 `GET /workspaces/:workspaceId/authoring/success-byte-audits/:permitId` 经 Application 的 concrete typed EventHub command/result 向 Workspace owner 读取当前记录中该已完成派生、提案或 v2 审查的原文（也支持待确认 change candidate）。CanonicalHash 对派生指向派生内容，对提案及审查指向原 proposal；permit 区分每份独立审查；不复用失败诊断，不接受跨 Workspace 或未知 permit。响应以 Base64 可逆编码返回 document／wire 与完整性 metadata，使用 `Cache-Control: no-store`，沿用管理接口部署访问边界与 owner 0600 存储保护；未保存内容不伪装为完整原文。重复读取及进程重开后的读取不写 Workspace、不增加预算、不调用 provider、不创建 Run。

## 原始失败诊断与数据保真

提案和审查文档仍执行原有严格 JSON、资源快照、Goal/验收项身份、来源及验证绑定校验。`invalid_proposal` / `invalid_review` 是解析或语义校验失败，不代表 provider 调用失败；`outcome_unresolved` 表示完成结果尚未记录，可能仍有后台任务执行，不能仅凭该状态推断请求中断。

失败诊断由 Workspace owner 随原 completion 持久记录，保留解析错误及 provider 原始响应，不脱敏、不替换、不重新序列化、不静默截断。原始响应按 bytes 存储，编码可逆，字节数和 SHA256 用于校验读取完整性。诊断不是提案、审查证据、准入或执行权威。

原始响应单条保存上限为 256 KiB，原始错误单条保存上限为 64 KiB；沿用既有 112 条规格请求总量上限，每条仅保留一个诊断结果。超限必须明确标记对应内容未保存，并保留原始字节数、hash 和未保存原因；不得保存片段冒充完整原文，也不得把已知解析失败降为未决。旧失败记录没有诊断时明确显示未记录，不伪造历史原因。

常规创作查询返回错误分类与诊断元信息，不把原始错误、响应或 provider 观察混入周期轮询。单条原文仅通过 Workspace 与 permit 精确限定的只读管理查询按需取得；Web 以纯文本展示并支持原字节读取，不执行 provider 返回的 HTML 或指令。诊断查看不调用 provider，不启动 Run，不恢复或更改确认身份。

列表元信息不含 `parseError` 原文属于正常响应。Web 按需读取后核对 Workspace／permit／phase／failureCode 及全部 hash、bytes、保存和 provider 观测 metadata，再校验真实原文的字节数与 SHA256；身份切换或迟到响应不得显示旧原文。只有解析错误已保存也可读取，不依赖 payload 或 wire 已保存。

包含诊断的 Workspace 文件通过原子发布使用 0600 权限。原始内容留在 owner 运行存储，不提交 Git，不额外附加配置密钥，不复制到通用日志；权限保护不能替代管理接口的部署访问边界。

## 前置准备失败

资源获取、provider 配置及请求构造发生在派生／提案／审查目标的 reserve 之前。此时失败由 Workspace owner 的独立 `PreparationDiagnostics` 保存，不能伪造 provider permit 或失败 completion。`preparation_failed` 表示当前输入、模板及尚未 reserve 的目标存在已知前置失败，不代表 provider 请求结果未决。

诊断绑定 Workspace、input／template hash、目标阶段／请求身份、适用 review／proposal 身份、失败阶段和资源引用。原始原因按 bytes 保存，不脱敏、不 trim、不重写；单条原文上限 64 KiB，超限保留完整字节数、SHA256 和明确未保存原因，不保存片段。累计创作 ledger 最多 32 条诊断，原文最多 2 MiB；确认归档沿用最多 16 份历史记录，每份有界但会复制原账本，不表示整个 Workspace 的物理原文总量只有 2 MiB。集合满时明确拒绝新记录，不淘汰旧原因。

诊断语义 ID 不包含观察时 Workspace revision；该 revision 仅限制首次保存。相同输入／目标／原因的重复失败返回原诊断，不重复 CAS 或消耗容量；新失败仍须通过当前 scope 和 revision 栅栏。迟到失败不得绑定新输入或覆盖已 reserve、确认或封存的事务。诊断保存失败必须明确报告，不能声称原文已持久、可查询。

常规查询只提供诊断身份、阶段、hash／bytes／保存状态及当前／历史标记。原始原因通过 `GET /workspaces/:workspaceId/authoring/preparation-diagnostics/:diagnosticId` 按需读取，可逆编码并以原字节校验；非 UTF-8 内容不能用替代字符冒充原文，仍可下载原 bytes。原文继续受 0600 原子文件和管理访问边界保护，不复制到通用日志。

资源修复后，用户可显式再次 compile，同输入重新观察事实；已有持久请求优先按原 ledger 恢复，不自动重发 provider。早期已完成审查为 FAIL／UNKNOWN 而下一份独立审查尚未 reserve 时，当前前置失败仍可显式恢复并仅补齐缺失审查；全部六份审查完成后仍按原 FAIL／UNKNOWN 阻止确认，不能因准备恢复改判为通过。目标已 reserve 或确认已开始时，旧前置失败仅为历史，不阻止当前正常进展。未知 reserve 冲突或响应丢失不能被猜成“未取得 permit”；只有 owner 明确返回未签发 permit 的预算拒绝可作为前置失败记录。

## 恢复边界

新建向导每次重新打开均为空白，不读取旧浏览器恢复记录。关闭／刷新后页面内存的未保存输入与重放身份丢失；有未保存编辑或未决请求时关闭须明确确认，已提交事务不会因此被取消。已保存草稿从 Workspace 列表／详情显式打开，服务端查询失败不能触发自动替代创建。

确认恢复参数由已校验的 owner journal 投影，绑定 Workspace、input／permit／proposal／specification 身份；用户须显式选择恢复。ExpectedWorkspaceRevision 仅约束首次准备，已开始的 journal 精确重放不依赖浏览器保存原 revision。页面仍保留原确认时不得以查询结果替换，缺少合法恢复身份时保持阻塞。关闭、切换 Workspace 或删除后的迟到响应不能重新注入旧视图。

确认 journal 已 prepared 时，模板或资源漂移会阻止继续物化；系统不提供 abandon／清空 journal，不能换 hash 或改写正式行绕过。恢复原模板内容及资源 exact bytes 后，由 owner 重新验证原快照和 input／permit／proposal／specification／configuration 身份，再显式恢复同一确认事务。prepared 阶段必须核对当前模板，materialized 的 exact 重放只返回原 after-image，不能因此获得当前 activation 资格；资源绑定的确认重放每次仍由 File 重新验证。原 snapshot 恢复也不会覆盖冲突 Goal／DoD 行：冲突保持阻塞，需要排查正式状态，不能把部分 materialization 宣称已完成。确认恢复不重新调用 provider、不自动创建 Run。

已 reserve 的请求不会因查询或重复 compile 自动获得第二次 provider 调用权限。完成请求重放必须包含同一原始诊断，内容变化是冲突。输入修订会产生新身份并重新审查，不是恢复或改写原失败结果。

`failed` 的同输入编译只返回原失败，不调度新的 provider 请求；部署新的请求合同也不会清除旧失败或授予新 permit。只有真实输入或模板修订才应保存为新 revision，失败历史和既有预算继续保留，修订不保证解除预算耗尽。仅服务代码修复后的新验证须由用户显式创建新事务，不能靠无意义改动 Goal、随机替换请求身份或清理账本绕过原失败。`preparation_failed` 的未 reserve 目标恢复与此不同，不得混用提示或调用规则。

## 相关合同

- [Service Command API](../../50-product-interface/service-api.md)
- [Definition Admission](../../00-core/definition-convergence/definition-admission.md)
- [Workspace Clarification](../../00-core/definition-convergence/clarification.md)
