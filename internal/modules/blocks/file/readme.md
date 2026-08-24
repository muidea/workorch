# 文件能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-23

## 功能范围与边界

`blocks/file` 提供受 workspace 路径边界约束的文件 list、read、write、edit、patch preview/apply 等运行时能力，并返回规范化执行结果。

## 组件交互及合同

- `kernel/orchestrator` 通过 File owner 在 `blocks/file/pkg/events` 定义的具体 command/result 同步投递文件操作；本组件直接订阅 EventHub，不启动通用 runtime executor，也不发布第二套执行通知。
- Runtime target snapshot、workspace manifest 与 capability preflight 均使用 File owner 定义的 typed EventHub command/result；结果只表达 owner 观测，manifest revision 对文件路径和内容变化敏感。
- Definition selector（例如 `file/workspace_deliverable`）描述声明侧 subject/scope；File operation selector（例如
  `workspace-relative-new-file-target`）描述具体 capability admission。两者由 File owner 各自解释但不是 alias：File
  只对请求中的 exact selector 返回 factual selection/feasibility/receipt，不把声明 selector 改写成 operation selector，
  也不决定 CWI、重试、reopen 或 Runtime 状态。
- prospective target 的创建前 admission 与创建后 reconciliation 是两条 File-private 只读语义：admission 对已存在
  路径继续返回 `unsupported`，防止 create-only `file/write` 重放；reconciliation 只核验 sealed owner/path 的当前
  manifest 事实，精确存在返回 `satisfied + concrete target/current revision`，缺失返回 `unsatisfied`，歧义、非规范
  或越出 workspace/manifest 的路径返回 `unverifiable`。`file/workspace_deliverable` 表达完整 bounded manifest scope，
  因此多文件返回 `satisfied + 全部 TargetRefs`；只有 unique/profile selector 的多匹配才是 ambiguous。该观察不返回
  retain/reopen/CWI 状态。
- 对 profile 显式绑定的空工作区，File 可以返回不含具体路径的 `scope` selection；它只授权 LLM 提出一个具体 workspace-relative File target，并由同一 owner feasibility/admissibility gate 验证。File 不从 Goal、DoD、目录名或文件后缀推断路径；未绑定 profile 的空工作区仍返回 `absent`。
- Runtime state-change/validation operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并携带本 owner
  stable factual category、完整安全 original diagnostic 与 evidence；不返回跨组件 Status 双权威或 map。
- request 必须携带 workspace/run/authority/dispatch/operation identity 与 File-owned `SealedRuntimeOperation`；该值包含从已持久化 dispatch 复制的 typed operation 及其全量 payload hash。File 先核对精确 Orchestrator source、hash、operation ID、target/CWI scope，再在 owner 内解包 path/ref 和有界输入，不接收一份可与 seal 漂移的独立执行 `Data`；
  result 返回唯一 receipt outcome、target refs、side-effect metadata、bounded evidence/ref 与 original diagnostic。
- Orchestrator 负责 state-changing admission、approval、evidence quality、coverage binding 和 canonical decision。
- effect 执行前后通过 Orchestrator 定义的 receipt query/commit concrete command 完成幂等握手；File 只提交 owner observation，不直接写 Runtime aggregate。query 的 `Found/Receipt` 必须一致，已找到的 receipt 必须精确匹配 operation/execution/fence；commit 返回值必须与提交的完整 receipt 相同，否则作为跨 owner conflict fail closed。该校验由六个 capability owner 共用同一 typed event-contract helper。
- 对跨越信任边界的敏感 file/edit、file/write、patch/apply（密钥、`.env`、`.git`、CI workflow），File owner 先返回 `governance_required` 且不执行副作用；普通 workspace-local 文件的变更规模不构成 approval。resume 只在 exact `ExecutionKey + next Fence` 的 typed grant 经 Orchestrator preflight 验证，且 grant 的 subject、requested action、risk 与 authorization hash 再次匹配 File owner 预期后执行；普通布尔参数或调用方声明不能绕过 approval。

- 所有同步 EventHub handler 在读取、查询 receipt 或执行 effect 前拒绝 nil result；执行 handler 还要求精确 Orchestrator source。文件变更统一编译为 File-private patch effect：`file/edit` 映射为 `text_replace`，`file/write` 映射为 create-only `file_write`，公共 command 不能直接调用 mutation adapter。
- `file/write` 是 create-only 操作，目标已存在时必须拒绝且保持原内容不变。File owner 的 operation
  admissibility catalog 因此只为该 kind 发布 `prospective_owner_target`；已有目标只能选择同一 catalog 中允许
  `exact_target | existing_owner_scope` 的 `file/edit` 或 `patch/apply` variant。实际 replacement 仍须绑定 owner
  观测到的目标 revision；其中 `patch/apply file_write` 必须携带在执行时再次核对的 `baseHash`，
  `text_replace | unified_diff` 也必须通过对应的 owner preflight/replay 合同。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/snapshot/manifest/reconciliation/execute；receipt query/commit | 完整；所有合同均为 concrete typed command/result |
| Workspace filesystem | File-private adapter | scoped list/read/create/edit/patch 与 owner observation | 完整；adapter 不拥有 Runtime 状态 |
| 其他 owner | 无直接 I/O | 只经 Orchestrator sealed request/fact 协作 | 完整；无跨 owner service/repository |

## 相关约束

- 所有路径必须限定在 workspace internal/deliverable 允许范围内，并防止路径逃逸。
- workspace manifest 只有在完整遍历、路径归一、entry metadata 与 content hash 全部成功时才能封存；除显式排除树和
  不跟随的 unsafe/broken symlink 外，任何 walk/rel/stat/hash 错误都 typed fail-closed，不能静默缩小 inventory 后
  产生 selected/create admission。
- 大内容必须使用摘要和 raw ref，不能进入默认事件或投影。
- `file/read` 只保留有界内容窗口，同时以固定缓冲区流式完成全文件 hash、行数与 binary 检查；workspace manifest 同样流式计算每个文件 hash，`MaxBytes`/manifest 不按文件大小分配内存。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result、Memory 或 coverage 状态。
- 本 owner 私有 code 只在 File Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。
- 文件写入成功只表示技术动作成功，不等于业务完成。
- 完整 target snapshot 必须携带 target ref、content、content hash 和明确的 complete content state；missing、truncated、binary 或 redacted 内容不得作为可 patch snapshot 提交。
- CWI reconciliation 仅使用 binding 声明的 exact target、owner selector 或可直接映射到 manifest 的 artifact ref；
  unknown/semantic artifact URI、ambiguous selector 和 workspace absent 分别返回 `unverifiable` 或 `unsatisfied`，不从
  Goal/DoD、目录名或文件名猜测交付目标。
- reconciliation 不得调用 prospective-target admission selector。创建后目标存在是 post-effect satisfied fact，不是新的
  create admission；只有 Orchestrator SM-2 可以根据该事实与 sealed binding/receipt lineage 选择 retain 或 reopen。
- prospective path 只能按 selector 自身的 canonical admission 能力解释：`workspace-relative-new-file-target`、
  `file/workspace_deliverable` 的完整 bounded owner scope，以及 `file/profile_declared_candidates` 中已声明的 candidate
  可以复核。`workspace_deliverable` 可在非空清单上保留一个经 File 路径/manifest eligibility 校验的 absent prospective
  target；它不会被当前 1/N 项替换，候选仍只能使用该 sealed target。profile prospective 必须始终属于 sealed candidate
  set，且在已有其他 candidate match 时不得被静默忽略；尚未出现的另一目标返回 typed `unsupported`。
  `file/unique_workspace_entry` 是纯当前清单基数 selector，创建前无论当前清单为 0/1/N，只要同时携带
  prospective path 就必须返回 typed `unsupported` 且零写，创建后 reconciliation 对同一非法组合返回
  `unverifiable`。未知 selector 同样不得取得 prospective 语义。`.git`、`.workorch-data`、`node_modules`、`vendor`、
  `dist`、`build` 根或嵌套树不属于 deliverable manifest；上述三种可创建 selector 对其中的 prospective path 必须在
  effect 前返回 typed `unsupported` 且零写，旧 binding 的 reconciliation 保持 factual `unverifiable`。所有 absent
  prospective admission 还必须按完整 500-entry File listing 预留 observation capacity；目录与文件都消费该上限，
  嵌套目标同时预留目标文件及所有缺失父目录。容量不足时返回 typed `unsupported`，不得授权 effect 后必然无法
  reconciliation 的第 501 项。File owner 在最终 create effect 前按 workspace 串行重读完整 inventory 并重算缺失
  条目；stale admission 或并发 create 只能在总 entry 数不超过 500 时提交，拒绝时连父目录也不得创建。

## 已覆盖的关键回归

- [x] `file/write` create-only、已有目标拒绝、exact revision/baseHash replacement 与各 patch variant replay proof。
- [x] 路径逃逸、symlink、binary、redacted、truncated 与不完整 snapshot 的 fail-closed 行为。
- [x] 敏感 `file/edit`/patch governance-before-effect、grant target/action/authorization 精确匹配，以及普通 workspace-local 大变更无需额外授权。
- [x] effect 已发生但 receipt commit 丢失后的 owner proof 重放，以及已有 receipt 的零副作用 duplicate。
- [x] sealed operation payload/hash、target/context 与 source 漂移在 receipt query/effect 前失败；所有强 handler 对 nil result fail-closed。
- [x] `file/read` 有界内容、晚段 line range、全文件 hash/binary 检查，以及 manifest revision 对 content change 的失效。
- [x] manifest revision 对 create/delete/rename 的组合失效与还原；`.workorch-data`、`.git`、`node_modules`、`vendor`、`dist`、`build` 不进入 deliverable manifest。
- [x] 注入 walk、entry-info、content-hash 错误时，manifest 与 unique/prospective selection 全部 fail-closed 且零写，
  不把不完整 inventory 伪装成完整 cardinality。
- [x] File 唯一消费的跨 owner result（receipt query/commit）覆盖 wrong type、owner error、Found/Receipt 不一致、invalid/stale identity 与 payload drift；prerequisite/reconciliation 是 File 生产、Orchestrator 消费的 concrete result，不在 File 内反向解码。
- [x] prospective create 前 admission（含 unique selector，以及 workspace-deliverable/profile 的 excluded、未参与当前
  inventory、越出 sealed candidates 等 0/1/N × prospective typed fail-closed/零写矩阵）、创建后
  exact reconciliation、重复查询、fresh-owner restart、资源删除后的当前 observation、四类 selector × 0/1/N
  cardinality × prospective post-effect matrix、ambiguous/out-of-workspace fail-closed 与 reconciliation 零二次写。
- [x] snapshot 大文件、400 文件 manifest 与大文件 patch replay 均有 allocation benchmark；bounded read、文件与目录
  混合时恰好 500 个可纳入 entry 成功、第 501 个才 typed overflow、嵌套 create 为缺失父目录预留 capacity，以及
  replay 零二次写均由回归测试约束；被排除的 owner-internal/generated tree 不消费该 entry bound。

## 当前验证结论

- 当前没有已知的本地可执行合同或测试缺口；真实文件系统差异继续作为 deployed verification 观察项，不改变上述 owner 合同。
