# System Module

Status: active

Type: kernel-module-contract

Last Verified: 2026-08-30

## 功能范围与边界

`kernel/system` 只协调 Soul、WorkspaceTemplate 与 MCP Registry 内置资源的启动期 ensure。它没有 repository、
业务聚合或运行期状态机；部分成功后再次调用 `Start` 时，仍从第一项重发 exact ensure command，由各资源 owner 的幂等合同
收敛。

System 不解释 default 内容或版本、不自行写入/升级持久化对象，也不根据 Runtime 状态修复资源。Soul/Template owner 在
自己的 ensure 内创建缺失资源并自动更新 exact reviewed 未定制版本；MCP Registry 保持 create-if-missing。

## 组件交互及合同

| owner | 方向 | concrete contract | 顺序 |
| --- | --- | --- | --- |
| `blocks/soul` | System → Soul | `EnsureDefaultsCommand → EnsureDefaultsResult` | 1 |
| `blocks/workspace` | System → Workspace | `EnsureWorkspaceTemplateDefaultsCommand → EnsureWorkspaceTemplateDefaultsResult` | 2 |
| `blocks/mcpregistry` | System → MCP Registry | `EnsureDefaultsCommand → EnsureDefaultsResult` | 3 |

三组 Command/Result 都由接收 owner 定义并同步返回。System 逐项校验 missing subscriber、nil/wrong concrete result、
`Ensured=false` 与 owner error；任一失败立即停止，且不保存中间 System 状态。

### 生命周期合同

- Start 的 owner 票覆盖三项同步 ensure 的真实返回，保留原失败即停止与幂等重试语义，不新增业务进度账本。
- BeginShutdown 非等待式拒绝新 Start；取消调用方不等于 nested handler 完成。Quiesce 超时保留 System/Module 引用，最终 checked Stop 仅在真实排空后释放。
- Configure 不能覆盖未释放 owner；plugin Setup/Run/Quiesce/Teardown 保留 error cause。System 无订阅、timer 或仓库句柄；其本地排空不能代替共享任务与 Hub 屏障，不参与 SM-1～SM-4。
- `module_lifecycle_test.go` 使用真实 Hub 阻塞 ensure 验证 cancel、超时、guard 失败保引用及新预算重试。

## 不变量与非职责

- System 不取得 Soul、Workspace、MCP repository/service，也不定义通用 ensure envelope。
- 默认值的 validation、dedupe、CAS、create-if-missing 与持久化完全属于接收 owner。
- 用户定制、未知版本与用户新增 ID 必须保持不变；删除内置 ID 后，下次启动按当前嵌入值重新创建。
- System 不提供 migration/upgrade 状态机；Template/Soul 的自动或显式更新均复用资源 owner 的私有 guarded upgrade，
  System 不比较内容、不选择目标版本、不触碰 Workspace admission 或历史 Run。
- System 不参与 Definition admission、Run lifecycle、Runtime drive、governance、projection 或 deletion。
- 新增默认资源 owner 必须同时增加 owner-defined concrete contract、明确顺序和失败/重试验收。

## 能力验收

| 验收面 | 自动化证据 | 覆盖维度 |
| --- | --- | --- |
| Module lifecycle/dependency | `TestSystemModuleConfigureRequiresEventHub`、`TestSystemModuleIdentityStartStopContract`、`TestSystemModuleStartConvergesAfterPartialFailureWithoutOverwritingUserChanges` | EventHub required、identity、Start/Stop 与成功后的重复 Start |
| ordered concrete ensure | `TestSystemModuleStartEnsuresBuiltinDefaultsThroughEventHubConsumers`、`TestSystemStartFailsClosedForEveryEnsureContract` | 三 owner normal、missing/nil/wrong/false/error、fail-fast 顺序 |
| partial failure and retry | `TestSystemModuleStartConvergesAfterPartialFailureWithoutOverwritingUserChanges`、`TestSystemFreshProcessConvergesFileBackedPartialStartupWithoutOverwritingUserChanges` | 覆盖同一实例重试，以及 fresh Hub/System/owner 与 fresh file repository handles 的跨进程等价重建；均保持已有用户修改并补齐缺失 owner defaults |
| owner create-if-missing | `../../blocks/soul/biz/defaults_test.go`、`../../blocks/workspace/biz/defaults_test.go`、`../../blocks/mcpregistry/biz/defaults_test.go` | memory/file backend、重复 ensure、保留用户修改、补齐缺失资源 |
| owner boundary | `scripts/verify-boundary-contract.sh` | 无 repository/service、无 generic envelope、只发送三组 concrete contract |

`duplicate/stale/effect/receipt` 对 System 自身不适用：它不持久化业务状态或执行外部 effect；对应幂等与 CAS 由三类资源
owner 的验收覆盖。
