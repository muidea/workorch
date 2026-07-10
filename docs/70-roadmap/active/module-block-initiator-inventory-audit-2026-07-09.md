# Module / Block / Initiator Inventory Audit

Status: completed

## 2026-07-10 收口进展

- Gap 1 已关闭：initiator exports 已补齐 `CoverageWorkItems`；appservice runtime session 逐项校验 26 个 repository exports 与 EventHub，并对缺失项返回具名错误。
- Gap 2 已关闭：Goal/DoD 定义变化会清理失效 `ActiveRunID`；activation 仅恢复唯一 active/blocking 非终态 run，多个候选返回 conflict。
- Gap 3 的 appservice/server 测试装配缺口已关闭：测试通过显式 owner lifecycle fixture 启动 7 个 blocks 与 kernel owner modules；`go test ./internal/modules/application/workorchd/biz/appservice -count=1` 和 `go test ./internal/modules/application/workorchd/service/server -count=1` 均通过。
- workorchd scheduler 已关闭 Envelope/map command 遗留：投递方定义并直接发送 `WorkspaceRunRequestedCommand`，消费方返回 `WorkspaceRunRequestedResult`。
- runtime block 链路的代码缺口已关闭：`internal/pkg/runtimeevent` 定义 `ExecuteCommand`/`ExecuteCommandResult` 与 accepted/progress/completed/failed 具体 data；orchestrator 和 7 个 blocks 直接使用 magicCommon `event.Hub` 的 `Send/Post/Subscribe`，不再通过 Envelope 顶层 payload、通用 map 或 JSON 转换交互。
- `internal/initiators/...` 与 `internal/services/workorch/...` 全量测试已通过；services backend 测试已改为显式启动 owner lifecycle，并直接模拟 typed scheduler result。
- 5 个 appservice 子包测试夹具遗留已关闭：`activationports`、`contentports`、`contextregistryports`、`qualityports`、`runports` 已改为具体 owner command/result observer 或显式 owner lifecycle，不再通过 Envelope response 模拟 owner command。
- `GOCACHE=/tmp/workorch-go-build go test -p 1 ./internal/modules/... -count=1` 已全量通过。默认并行执行曾因大量测试 runtime 同时创建 EventHub/BackgroundRoutine 而在 10 分钟超时；该现象属于测试环境资源饥饿，不是组件合同失败，最终门禁以串行全量结果为准。

Date: 2026-07-09

Related:

- `module-block-contract-closure-validation-plan.md`
- `../../00-core/runtime-stage-contract.md`
- `../../30-module-architecture/llm-provider-assembly.md`

## Purpose

This audit freezes the current registered `module`, runtime `block`, and
`initiator` inventory and checks whether each component still matches its
functional definition. The check is structural and contract-oriented: lifecycle,
owner state, cross-module boundary, and runtime neutrality are more important
than file size.

## Functional Definition Baseline

| Kind | Functional definition |
| --- | --- |
| initiator | Process-level infrastructure provider. It may provide repository providers, EventHub wrappers, runtime policy, config loading, route registry, client factory, or background routine. It must not own workspace/run/result business state or business strategy. |
| kernel module | Formal owner of one business or governance state family. It owns repository access for that state, subscribes to owner command/query topics, publishes formal events, and must be the authority for its state transitions. |
| application module | Product/API/application use-case surface. It may assemble routes, projections, command facades, context registry, and configuration surfaces. It must not become the owner of kernel state or repair owner state through projection. |
| runtime block | Runtime capability adapter for technical execution or external systems. It produces normalized execution results/evidence candidates, but must not own formal business state, decide run completion, or mutate coverage/result state directly. |
| process service | Process mode selection and user surfaces under `internal/services/<entry>`. It is not a plugin module and must depend on stable `workorchport` / `workorchclient` contracts rather than owner repositories. |

## Registered Inventory Snapshot

| Kind | ID / component | Code path | Current fit |
| --- | --- | --- | --- |
| initiator | `workorch/runtime` | `internal/initiators/workorchruntime` | Fits initiator role: repository provider, EventHub owner wrapper, runtime policy. |
| application module | `application/workorchd` | `internal/modules/application/workorchd` | Fits application role; borrowed exports are validated at the runtime-session boundary. |
| kernel module | `kernel/workspace` | `internal/modules/kernel/workspace` | Fits owner role for Workspace / Goal / DoD; definition changes clear stale active-run bindings. |
| kernel module | `kernel/run` | `internal/modules/kernel/run` | Fits owner role for Run / Step lifecycle and runtime checkpoints. |
| kernel module | `kernel/orchestrator` | `internal/modules/kernel/orchestrator` | Fits orchestration authority role for convergence, evidence, coverage, repair/replan, and block dispatch. |
| kernel module | `kernel/result` | `internal/modules/kernel/result` | Fits Result / Artifact owner role. |
| kernel module | `kernel/decision` | `internal/modules/kernel/decision` | Fits DecisionTicket / Resolution owner role. |
| kernel module | `kernel/approval` | `internal/modules/kernel/approval` | Fits Approval owner role. |
| kernel module | `kernel/governance` | `internal/modules/kernel/governance` | Fits governance trace / recommendation role. |
| kernel module | `kernel/eventstream` | `internal/modules/kernel/eventstream` | Fits audit event storage/query role; not an owner-state repair authority. |
| kernel module | `kernel/memory` | `internal/modules/kernel/memory` | Fits memory and recall trace owner role. |
| kernel module | `kernel/mcp` | `internal/modules/kernel/mcp` | Fits MCP server/capability registry role. |
| kernel module | `kernel/skill` | `internal/modules/kernel/skill` | Fits skill registry role. |
| kernel module | `kernel/soul` | `internal/modules/kernel/soul` | Fits soul catalog/stage policy metadata role. |
| kernel module | `kernel/system` | `internal/modules/kernel/system` | Fits infrastructure/status role; intentionally narrow. |
| runtime block | `blocks/file` | `internal/modules/blocks/file` | Fits block role; registers file list/read/edit/patch execution kinds. |
| runtime block | `blocks/git` | `internal/modules/blocks/git` | Fits block role; registers git read/write execution kinds. |
| runtime block | `blocks/llm` | `internal/modules/blocks/llm` | Fits LLM provider/context block role; cache-prefix and critical-field contracts remain high-risk and need dedicated tests when changed. |
| runtime block | `blocks/mcp` | `internal/modules/blocks/mcp` | Fits MCP invocation block role. |
| runtime block | `blocks/shell` | `internal/modules/blocks/shell` | Fits shell execution block role. |
| runtime block | `blocks/ssh` | `internal/modules/blocks/ssh` | Fits SSH execution block role. |
| runtime block | `blocks/web` | `internal/modules/blocks/web` | Fits web/search/fetch block role. |
| process/product surface | workorch service / port / client / web | `internal/services/workorch`, `internal/pkg/workorchport`, `internal/pkg/workorchclient`, `web` | Fits process/product surface role; should stay outside `internal/modules`. |

## Closed Gaps

### Gap 1. Borrowed runtime exports were under-validated

- Area: `application/workorchd` runtime session.
- Resolution:
  - `ExportsFromInitiator` now copies `orchestratorRepos.CoverageWorkItems` into `RepositoryExports`.
  - Runtime session construction validates all 26 repository exports and EventHub, returning a named missing-dependency error instead of a downstream generic `invalid argument`.
  - Pairwise tests cover initiator exports -> appservice session assembly.

### Gap 2. Workspace definition-change contract was not fully closed

- Area: `kernel/workspace`.
- Resolution:
  - Goal/DoD definition changes clear stale `ActiveRunID` when reopening a terminal or superseded workspace.
  - Workspace-state and definition notification counts are aligned with the asynchronous publish contract and covered by regression tests.
  - Activation only restores a unique active/blocking non-terminal run; multiple candidates return conflict.

### Gap 3. Application module tests exposed broad invalid-argument failures

- Area: `application/workorchd/biz/appservice` and `application/workorchd/service/server`.
- Resolution:
  - Appservice and server tests now use an explicit owner lifecycle fixture that starts the required kernel modules and runtime blocks.
  - Owner command tests send concrete owner-defined command types and assert concrete result types; no Envelope response fixture is used for these command paths.
  - `appservice/...`, `service/server`, and the serial `internal/modules/...` suite all pass.

## Boundary Checks

| Check | Current result |
| --- | --- |
| Entry side-effect imports live in `application/workorch/main.go` | Pass. The entry explicitly imports the initiator, application module, kernel modules, and runtime blocks. |
| Runtime blocks own no formal workspace/run/result state | Pass by structure. Blocks register `runtimeevent.Executor` handlers and hold adapters/executors, not owner repositories. |
| Kernel modules expose owner command/query topics | Pass by structure. Kernel modules subscribe to module-specific EventHub topics in `Start`. |
| Application/service code stays out of `internal/modules/kernel/*/biz` direct owner implementation | Pass for production interaction paths. Application/service code uses owner EventHub ports or stable contracts; repository access remains test-fixture setup only. |
| Cross-component command/data/result ownership | Pass for the audited closure paths. Owner or sender packages define concrete message types; consumers import those definitions and use magicCommon EventHub directly. Runtime block and scheduler paths do not use Envelope/map/JSON conversion bridges. |
| Core runtime neutrality | Pass for this change set. Event transport changes add no software-development, provider-specific, or business-domain strategy to orchestrator core. |

## Verification Run

```bash
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
GOCACHE=/tmp/workorch-go-build go test -p 1 ./internal/modules/... -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/initiators/... -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/services/workorch/... -count=1
```

Result: all pass on 2026-07-10.

The default parallel modules suite previously exceeded 10 minutes because many test runtimes created EventHub and BackgroundRoutine instances concurrently. The corresponding tests pass individually and the serial full suite passes, so closure uses the serial full-suite result as the deterministic gate.

## Final Conclusion

- All 22 registered components have a Chinese root `readme.md` describing responsibility, interaction relationships, interaction contracts, and known gaps or continuing regression risks.
- The three initial functional gaps are closed.
- The audited cross-component runtime block and workorchd scheduler event paths use sender/owner-defined concrete command/data/result types without Envelope payload maps or JSON conversion.
- No missing functional responsibility remains in the 22-component inventory at this audit point.
