# Web Block

## 功能定位

`blocks/web` 是 runtime web/search/fetch block。它负责受控的外部搜索和网页获取，并返回规范化 web result、source refs、truncation flags 与错误分类。

## 拥有边界

- 拥有 web provider config、search/fetch adapter 与 runtimeevent executor。
- 只处理外部信息获取能力。
- 不拥有 Memory、Result、Run 或业务完成状态。

## 主要交互

- 接收 `kernel/orchestrator` 派发的 `web/search`、`web/fetch` execution request。
- `blocks/llm` 可在 execution graph 中声明 web step。
- Web 输出由 `kernel/orchestrator` 作为外部 evidence candidate、context material 或 diagnostic 处理。
- 外部访问风险通过 runtime policy、governance 与 approval 流程控制。

## 组件交互合同

- Input contract: request 必须包含 search/fetch operation、provider/ref、bounded query/url、workspace/run/step scope。
- Output contract: 返回 normalized web result/error、source refs、summary、raw ref、truncation flags 和 provider diagnostic。
- Orchestrator contract: web content 是 context/evidence candidate；accepted evidence/result 由 orchestrator gate 判定。
- Approval contract: 外部访问风险或敏感 URL 必须受 runtime policy 和 approval flow 控制。
- LLM contract: LLM 只能声明 web candidate，不能把外部内容直接写成 owner state。

## 已知缺口

- 当前盘点未发现 Web block 职责缺失；provider/network failure 与 contract violation 的分类需要持续回归。

## 边界约束

- request 必须 bounded，并区分 provider/network failure 与 contract violation。
- 大内容必须使用 summary/raw ref。
- Web 内容不能直接成为 accepted Result。
