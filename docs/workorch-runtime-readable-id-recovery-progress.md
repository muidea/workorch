# WorkOrch Runtime Readable ID and Recovery Progress

本文跟踪 `workorch-runtime-readable-id-and-recovery-contract.md` 的逐项收口进度。

## 当前阶段

- 阶段：P0 runtime ID 与 provider retry 边界收口
- 状态：本地收口完成，待部署核对
- 最近核对 workspace：`ws-c51fa60493105285`
- 最近问题 run：`run-506f7364eb5ecb36`
- 最近本地验证：`go test ./... --count=1` 已通过；`bash -n scripts/verify-workorch-runtime-core-flow.sh` 已通过；`git diff --check` 已通过

## 进度表

| ID | 优先级 | 内容 | 状态 | 验收 |
| --- | --- | --- | --- | --- |
| RID-01 | P0 | 所有 provider retry ID 改为 run-scoped | 已落地，待部署核对 | 不再出现裸 `step-...provider-retry`；脚本已检查非 run-scoped step |
| RID-02 | P0 | 用 stage/loop/node 替代 hash token | 已落地，待部署核对 | provider retry、repair、follow-up、assessment correction、graph node 均使用可读 label，不再保留旧 `zz-llm-*` / hash token 兼容分支 |
| RID-03 | P0 | graph scope 使用 `graph-<seq>/<node-seq>-<node>` | 已落地，待部署核对 | runtime 生成的 graph node 已统一使用 `graph-<seq>/<node-seq>-<node>`，并在输入写入 `graphSeq` / `graphNodeSeq` |
| RID-04 | P0 | provider retry schema/contract invalid 退出 retry 分支 | 已落地，待部署核对 | retry 后 contract invalid 已转 runtime repair，repair root 指回 providerRetryRootStepId |
| RID-05 | P0 | contract violation 统一恢复状态机 | 已落地，待部署核对 | read-only graph、缺 targetRef、assessment graph 已进入 repair/correction/rejected 口径；缺 targetRef repair 会继承 state-changing 约束 |
| RID-06 | P1 | rejected result root cause 归因调整 | 已落地，待部署核对 | provider retry / runtime repair 终态 rejected result 的 SourceRef 指向结构化 root，外壳 step 保留为 EvidenceRefs |
| RID-07 | P1 | 验证脚本支持 handled violation | 已落地，待部署核对 | 脚本已将存在 runtime repair / assessment correction / rejected result / decision ticket 的违规降级为 warning，并补 fixture 回归覆盖未治理违规与 graph scope 失败 |
| RID-08 | P1 | memory recall 去除旧 ID 噪音 | 已落地，待部署核对 | LLM 输入中的 memory context 已去除 workspace/run/source/embedding 过程字段；低价值 provider retry 流水不进入 LLM memory context，审计 trace 仍保留候选、排除和完整 ID |
| RID-09 | P1 | 补充测试矩阵 | 已落地，待部署核对 | 已覆盖 ID、retry、contract violation、result、memory context、script 关键口径，真实运行核对继续作为部署验收 |
| PRR-01 | P0 | provider retry / runtime repair root normalizer | 已落地，待部署核对 | provider retry root 不再指向 provider retry 或 repair-of-provider-retry 外壳 |
| PRR-02 | P0 | repair/provider retry readable ID 使用归一 root label | 已落地，待部署核对 | 不再生成 `repair-*-of-provider-retry-*` 作为 provider retry root；ID 保持 run-scoped 且可读 |
| PRR-03 | P0 | provider/config terminal policy 收口 | 已落地，待部署核对 | `auth_unavailable` / `provider_unavailable` 进入 WorkOrch runtime provider decision，`resultPolicy=none` |
| PRR-04 | P0 | running failed-step 补派闭环 | 已本地收口，待部署核对 | latest failed step 后必须进入 repair / decision / rejected result / terminal failed，不停留 running |
| PRR-05 | P1 | provider retry memory recall trace 收口 | 已本地收口，待部署核对 | provider retry 流水不进入 LLM context；trace 记录 candidate/excluded/score reason |
| PRR-07 | P0 | state-changing repair retry 违规闭环 | 已本地收口，待部署核对 | provider retry 返回 read-only graph 后继续 repair 或治理终态，不悬挂 running |
| PRR-08 | P0 | currentStep 读模型按最新 checkpoint 选择 | 已本地收口，待部署核对 | status/snapshot 指向真实最新 step，不受 ID 字典序影响 |
| PRR-09 | P1 | activation memory 可见性过滤 | 已本地收口，待部署核对 | activation executionContext 不注入纯 provider_retry 流水 memory |
| PRR-10 | P0 | 缺失文件读取循环转 state-changing repair | 已本地收口，待部署核对 | DoD run 反复 `file/read` 缺失目标文件时必须创建文件、写 result 或开 decision，不继续只读循环 |
| PRR-11 | P1 | recovery step 携带 graph 上下文的校验边界 | 已本地收口，待部署核对 | provider retry / runtime repair 可保留失败 graph node context；普通 graph dispatch 仍强制 graph-scoped ID |

## 本轮验收命令

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
bash -n scripts/verify-workorch-runtime-core-flow.sh
git diff --check
go test ./... -count=1
```

## 本轮记录

- RID-01：
  - `providerRetryStepID` 改为显式接收 `runID`。
  - provider retry ID 生成规则改为 `<run-id>/provider-retry-<attempt>-of-<root-label>`。
  - 验证脚本新增非 run-scoped step ID 检查。
- RID-02：
  - repair ID 改为 `<run-id>/repair-<attempt>-of-<root-label>`。
  - graph follow-up / completion follow-up / assessment correction 改为可读 label，不再使用 hash token。
  - repeated graph node 的冲突路径改为 readable root label，不再使用 `step-<hash>`。
  - 移除旧 `zz-llm-*` ID 的 runtime label 兼容分支；嵌套 provider retry label 会截断到当前 stage，避免 retry ID 重复嵌套。
- RID-03：
  - runtime 生成 graph node ID 统一改为 `<run-id>/graph-<seq>/<node-seq>-<node>`。
  - 删除首个 graph node 的 `/block/<node>` fallback，重复 node 与首轮 node 使用同一套 graph scope 规则。
  - 验证脚本新增 graph node step ID 与 `graphSeq` / `graphNodeSeq` 检查。
- RID-04：
  - 增加 provider retry contract invalid 回归测试。
  - provider retry contract invalid 进入 runtime repair 时，`repairOfRootStepId` 归一到 `providerRetryRootStepId`，不再指向 retry 外壳。
- RID-05：
  - `patch/apply targetRef is required` / `targetRef must be set` 被纳入 state-changing repair 判断。
  - provider retry 产出的 contract invalid repair 输入会携带 `repairObjective=produce_state_changing_implementation` 与 `stateChangingOutputRequired=true`。
  - patch graph 缺 `targetRef` 的底层错误从泛化 `invalid argument` 改为明确 `patch targetRef is required`。
  - 增加 StartRun 级回归测试，确认缺 `targetRef` 的 LLM graph contract violation 会进入 runtime repair，并继承 state-changing 约束。
- RID-07：
  - 验证脚本新增 `executionGraph` 中 `patch/apply` 必要字段检查。
  - 对已被 runtime repair、assessment correction、rejected result 或 decision ticket 覆盖的违规输出 warning，不再直接失败。
- RID-06：
  - rejected result 写入时识别 provider retry / runtime repair 外壳。
  - `SourceRef` 归一到 `providerRetryRootStepId` 或 `repairOfRootStepId`，并把 root 与外壳 step 同时保留在 `EvidenceRefs`。
- RID-08：
  - 动态 memory recall 注入 LLM 的 `selectedContext.memories` 只保留 `id/memoryType/summary/content/tags/importance/confidence/visibility/provenance/vectorScore`。
  - 移除 LLM memory context 中的 `workspaceId/runId/sourceType/sourceRef/embeddingProviderId/embeddingModel/vectorReady/vectorDimension` 过程字段，完整 lineage 保留在 memory recall trace。
  - `tool_feedback` 中仅记录 provider retry 流水、且不带 `accepted_input` 的 Memory 会保留在 candidate/excluded trace，但不再进入 LLM 可见 memory context。
  - `accepted_input` 相关 Skill / Tool / MCP / Soul / Prompt Memory 增加选择加权，避免被流程流水挤占。
- RID-09：
  - 新增 `runtime_verify_script_test.go`，用临时 fixture 覆盖：
    - 未治理 invalid patch graph 必须 fail。
    - 已由 runtime repair 覆盖的 invalid patch graph 必须 pass with warning。
    - graph node 旧 `/block/` ID 必须 fail。
    - graph node 缺 `graphSeq` 必须 fail。
- 本地收口完成，待部署核对：
  - `ws-c51fa60493105285` 暴露的 provider retry / runtime repair 交织 root 污染已通过 recovery root normalizer 收口。
  - `run-71acc18bb58ff359` 暴露的 running failed-step 无后续动作问题已通过 provider/config decision 与 failed-step 补派闭环收口。
  - provider retry 流水 memory 不再进入 LLM 可见 recall，step-input trace 会保留 candidate / excluded / score reason。
  - `docs/workorch-runtime-provider-retry-repair-closure-design.md` 中 PRR-01 ~ PRR-05 均已本地落地，后续以真实部署 run 作为最终验收。
- PRR-01 / PRR-02：
  - 新增 recovery root normalizer，provider retry / runtime repair root 遇到 provider retry 或 repair-of-provider-retry 外壳时归一到结构化业务 root。
  - `runtimeRepairFollowUpStepID` 使用 `runtimeRepairRootStepID` 的归一 label，不再使用直接 failed provider retry 外壳 label。
  - 回归覆盖 `repair-001-of-provider-retry-001` 不再成为 provider retry root，provider retry contract violation 的 repair ID 不带 provider retry 外壳。
- PRR-03 / PRR-04：
  - `provider_unavailable`、`provider_schema_drift`、`auth_or_config_error` 这类 provider/config failure 进入 provider decision，`resultPolicy=none`。
  - `DispatchNextStep` 对 latest failed provider/config step 会阻塞到 WorkOrch runtime provider decision，不写业务 rejected result。
- PRR-05：
  - step-input memory refresh 在 selected 为空但存在候选时仍写 `memoryRetrieval` 与 trace。
  - provider retry process memory 保留为 candidate/excluded，不进入 LLM 可见 `selectedContext.memories`。
  - 回归覆盖 only provider retry `tool_feedback` memory 时 trace 记录 `excluded_low_value_process_memory`。
- 服务层预期同步：
  - `application/workorch` 进程级测试已从 provider/config failed 预期改为 `blocked_on_decision`，并校验 provider decision ticket。
  - `workorchd` appservice / HTTP server 测试已同步 provider/config blocker 合同：无 LLM provider 配置不再写业务 failure reason，不增加 failed outcome 计数。
  - 决策票据 resolve 后仍由 resume 路径负责继续执行；未显式 resume 的持久化 run 保持阻塞状态，避免把 resolve 操作误当作业务失败。
- 待部署核对：
  - 重新部署后继续核对 `ws-c51fa60493105285` 新 run，确认脚本不再出现 `provider retry root points at retry step` 与 unhandled state-changing violation。
- 2026-05-12 部署核对 `run-506f7364eb5ecb36`：
  - root/ID 归一已改善，未再出现 provider retry root 指向 provider retry 或 repair-of-provider-retry 外壳。
  - 仍出现 `provider-retry-002-of-repair-001-of-planning-001` 在 `stateChangingOutputRequired=true` 下返回 read-only executionGraph。
  - 该失败后 run 仍为 `running`，无 decision/result/next step，PRR-04/PRR-07 未完全收口。
  - run status 的 `currentStepId` 显示 `step-1`，真实最新失败 step 为 `provider-retry-002-of-repair-001-of-planning-001`，PRR-08 待修复。
  - activation `executionContext.memories` 仍包含历史 provider retry tool_feedback，PRR-09 待修复。
- 本地继续收口：
  - `dispatchRuntimeRepairFollowUp` 派发 provider retry / runtime repair 时使用 `uniqueDispatchStepID`，避免已有 recovery step ID 冲突导致闭环中断。
  - `previousExecutionStepID` 增加 `repairOfStepId` / `failedStepId` 链路识别，terminal step 选择不再只依赖 `previousStepId`。
  - run status 优先使用最新 runtime checkpoint 的 `CurrentStepID`，避免按 step ID 字典序误判 `step-1` 为 current step。
  - activation memory fallback 与显式选择均过滤纯 `provider_retry` tool_feedback，LLM 可见 memory context 去除 workspace/run/source/embedding 等过程字段。
  - 新增回归覆盖 state-changing provider retry violation 后继续 recovery、status currentStep checkpoint 选择、activation provider retry memory 过滤。
- 2026-05-12 部署核对 `ws-16f85e1f5e65e4a8` / `run-01e4229c3cad1e62`：
  - 当前 run 为 `blocked_on_decision`，验证脚本通过；存在 1 条 state-changing 非 patch graph warning，但已由 runtime repair 覆盖，不属于 unhandled violation。
  - 本轮新增 PRR-10：`file/read` 缺失目标文件失败在 DoD run 中会继承 state-changing 约束，repair guidance 要求通过 `patch/apply file_write` 创建缺失文件。
  - 本轮新增 PRR-11：验证脚本区分普通 graph dispatch step 与 recovery step。provider retry / runtime repair 保留 `graphNodeId` 作为失败上下文时，不再误判为 graph-scoped ID 违规。
  - 验证脚本新增 fixture 覆盖 repeated missing file reads without patch/result/ticket 必须失败，以及 provider retry graph context 作为 recovery step 必须通过。
