# Workspace 规格创作

Status: normative

Type: profile-capability

## 创建与确认

Web 创建输入包含名称、Template、Goal、本地资源引用、可选工作目录和额外验收要求。工作目录留空时由 owner 解析；资源引用相对于该目录。页面编辑不是服务端已保存输入。Web 不使用 localStorage、sessionStorage、IndexedDB、cookie 或其他浏览器持久机制保存业务数据；未保存输入及客户端重放身份仅暂存在页面内存。

Workspace owner 持久保存创作输入与请求身份；派生验收要求后生成结构化规格提案，并进行六类独立审查。只有当前 input、derivation permit、proposal/specification hash 和 revision 精确匹配且审查通过，用户才可确认并准入。创建、编译、确认均不创建 Run；执行是后续独立 intent。

旧 process-local setup draft session、直接 Goal/DoD 写入及手工绕过审查的创建入口已退休。本文保留原路径作为文档链接，不表示旧入口仍可使用。

## 原始失败诊断与数据保真

提案和审查文档仍执行原有严格 JSON、资源快照、Goal/验收项身份、来源及验证绑定校验。`invalid_proposal` / `invalid_review` 是解析或语义校验失败，不代表 provider 调用失败；`outcome_unresolved` 表示完成结果尚未记录，可能仍有后台任务执行，不能仅凭该状态推断请求中断。

失败诊断由 Workspace owner 随原 completion 持久记录，保留解析错误及 provider 原始响应，不脱敏、不替换、不重新序列化、不静默截断。原始响应按 bytes 存储，编码可逆，字节数和 SHA256 用于校验读取完整性。诊断不是提案、审查证据、准入或执行权威。

原始响应单条保存上限为 256 KiB，原始错误单条保存上限为 64 KiB；沿用既有 112 条规格请求总量上限，每条仅保留一个诊断结果。超限必须明确标记对应内容未保存，并保留原始字节数、hash 和未保存原因；不得保存片段冒充完整原文，也不得把已知解析失败降为未决。旧失败记录没有诊断时明确显示未记录，不伪造历史原因。

常规创作查询返回错误与诊断元信息，不把原始响应混入周期轮询。单条原文仅通过 Workspace 与 permit 精确限定的只读管理查询按需取得；Web 以纯文本展示并支持原字节读取，不执行 provider 返回的 HTML 或指令。诊断查看不调用 provider，不启动 Run，不恢复或更改确认身份。

包含诊断的 Workspace 文件通过原子发布使用 0600 权限。原始内容留在 owner 运行存储，不提交 Git，不额外附加配置密钥，不复制到通用日志；权限保护不能替代管理接口的部署访问边界。

## 恢复边界

新建向导每次重新打开均为空白，不读取旧浏览器恢复记录。关闭／刷新后页面内存的未保存输入与重放身份丢失；有未保存编辑或未决请求时关闭须明确确认，已提交事务不会因此被取消。已保存草稿从 Workspace 列表／详情显式打开，服务端查询失败不能触发自动替代创建。

确认恢复参数由已校验的 owner journal 投影，绑定 Workspace、input／permit／proposal／specification 身份；用户须显式选择恢复。ExpectedWorkspaceRevision 仅约束首次准备，已开始的 journal 精确重放不依赖浏览器保存原 revision。页面仍保留原确认时不得以查询结果替换，缺少合法恢复身份时保持阻塞。关闭、切换 Workspace 或删除后的迟到响应不能重新注入旧视图。

已 reserve 的请求不会因查询或重复 compile 自动获得第二次 provider 调用权限。完成请求重放必须包含同一原始诊断，内容变化是冲突。输入修订会产生新身份并重新审查，不是恢复或改写原失败结果。

## 相关合同

- [Service Command API](../../50-product-interface/service-api.md)
- [Definition Admission](../../00-core/definition-convergence/definition-admission.md)
- [Workspace Clarification](../../00-core/definition-convergence/clarification.md)
