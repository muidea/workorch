# Workspace 规格创作

Status: normative

Type: profile-capability

## 创建与确认

Web 创建输入包含名称、Template、Goal、本地资源引用、可选工作目录和额外验收要求。工作目录留空时由 owner 解析；资源引用相对于该目录。页面编辑不是服务端已保存输入。Web 不使用 localStorage、sessionStorage、IndexedDB、cookie 或其他浏览器持久机制保存业务数据；未保存输入及客户端重放身份仅暂存在页面内存。

Workspace owner 持久保存创作输入与请求身份；派生验收要求后生成结构化规格提案，并进行六类独立审查。只有当前 input、derivation permit、proposal/specification hash 和 revision 精确匹配且审查通过，用户才可确认并准入。创建、编译、确认均不创建 Run；执行是后续独立 intent。

旧 process-local setup draft session、直接 Goal/DoD 写入及手工绕过审查的创建入口已退休。本文保留原路径作为文档链接，不表示旧入口仍可使用。

## 结构化输出合同

提案与独立审查使用不同的版本化输出合同：顶层分别仅为 `specification` 与 `review`。请求提供完整 JSON Schema，根对象及所有嵌套对象均禁止未知字段；字段名、数组项、类型和枚举必须与 typed DTO 一致。提案的 `Reviews` 必须为空数组，审查结论只能来自独立审查事务。`all_of` 是现有条件的合取语义，不授权模型添加 `ConditionMode` 或其他合同之外的字段。

稳定 schema 与精确字段说明放入 C0，当前 Goal、候选验收项、资源和身份放入动态请求。Verification 使用 `ObligationID`、`Declaration`、`Status`、`ProfileID`、`EvidenceRequirementID`、`Scope` 等既有字段；`DECLARED` 必须绑定可信 Profile 声明，不能把自由文本的预期证据当作声明身份。

provider 原生 structured output 仅在能力明确支持时启用；不支持时仍提供完整提示合同并执行相同本地严格校验，不自动 fallback 或重发。显式 schema 构造错误必须明确失败，不能静默省略约束。JSON Schema 只约束结构，不替代原文保真、严格解析、来源／资源／Profile 校验、六类审查、用户确认或 owner 准入。

## 原始失败诊断与数据保真

提案和审查文档仍执行原有严格 JSON、资源快照、Goal/验收项身份、来源及验证绑定校验。`invalid_proposal` / `invalid_review` 是解析或语义校验失败，不代表 provider 调用失败；`outcome_unresolved` 表示完成结果尚未记录，可能仍有后台任务执行，不能仅凭该状态推断请求中断。

失败诊断由 Workspace owner 随原 completion 持久记录，保留解析错误及 provider 原始响应，不脱敏、不替换、不重新序列化、不静默截断。原始响应按 bytes 存储，编码可逆，字节数和 SHA256 用于校验读取完整性。诊断不是提案、审查证据、准入或执行权威。

原始响应单条保存上限为 256 KiB，原始错误单条保存上限为 64 KiB；沿用既有 112 条规格请求总量上限，每条仅保留一个诊断结果。超限必须明确标记对应内容未保存，并保留原始字节数、hash 和未保存原因；不得保存片段冒充完整原文，也不得把已知解析失败降为未决。旧失败记录没有诊断时明确显示未记录，不伪造历史原因。

常规创作查询返回错误与诊断元信息，不把原始响应混入周期轮询。单条原文仅通过 Workspace 与 permit 精确限定的只读管理查询按需取得；Web 以纯文本展示并支持原字节读取，不执行 provider 返回的 HTML 或指令。诊断查看不调用 provider，不启动 Run，不恢复或更改确认身份。

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

已 reserve 的请求不会因查询或重复 compile 自动获得第二次 provider 调用权限。完成请求重放必须包含同一原始诊断，内容变化是冲突。输入修订会产生新身份并重新审查，不是恢复或改写原失败结果。

`failed` 的同输入编译只返回原失败，不调度新的 provider 请求；部署新的请求合同也不会清除旧失败或授予新 permit。只有真实输入或模板修订才应保存为新 revision，失败历史和既有预算继续保留，修订不保证解除预算耗尽。仅服务代码修复后的新验证须由用户显式创建新事务，不能靠无意义改动 Goal、随机替换请求身份或清理账本绕过原失败。`preparation_failed` 的未 reserve 目标恢复与此不同，不得混用提示或调用规则。

## 相关合同

- [Service Command API](../../50-product-interface/service-api.md)
- [Definition Admission](../../00-core/definition-convergence/definition-admission.md)
- [Workspace Clarification](../../00-core/definition-convergence/clarification.md)
