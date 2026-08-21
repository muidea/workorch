# WorkOrch Repository Structure

Status: normative

Type: module-architecture

Last Verified: 2026-08-16

## Baseline

WorkOrch 是单 `go.mod` 仓库。这里的 “Module/Block” 是 framework 运行单元分类，不是多个 Go module。

当前 22-unit baseline：1 Initiator、4 Modules、17 Blocks。

```text
application/workorch/                    binary entry
internal/
  initiators/workorchruntime/            repository provider/factory (1 Initiator)
  modules/
    application/workorchd/               HTTP/SSE/bootstrap/projection (1 Module)
    kernel/{orchestrator,governance,system}/                 (3 Modules)
    blocks/
      {workspace,run,result,decision,approval,memory}/
      {skill,soul,mcpregistry,eventstream}/
      {llm,file,git,shell,ssh,web,mcp}/                       (17 Blocks)
    base/                              shared Base Biz; not a framework unit
  pkg/                                 shared values/contracts/infrastructure
  services/workorch/                   process CLI/TUI/session; not a framework unit
web/workorch-console/                  React console source
web/consoleassets/                     generated embedded assets
docs/                                  current normative documentation only
config/                                runtime configuration
scripts/                               verification and operational helpers
```

## Unit Roles

| Kind | Allowed role |
| --- | --- |
| Initiator | 创建 repository provider/factory；不导出 EventHub、Runtime policy 或 bootstrap |
| Module | 多 owner 协调、application composition 或 startup coordination |
| Stateful Block | 单一资源 owner，内部拥有 model/repository/lifecycle |
| Capability Block | 执行 concrete typed operation，并返回事实结果 |
| Base Biz | 同 owner 内复用生命周期辅助，不是跨 owner facade |
| Process service | CLI/TUI/local-remote backend 和 agent session orchestration |

## Typical Unit Shape

```text
module.go
plugin.go
biz/
service/        optional protocol adapter
pkg/
  events/       owner-defined concrete EventHub command/result
  models/       owner-local model
  common/       owner-local constants
readme.md       component interaction and implementation contract
```

不是每个 unit 都必须有每个可选目录。owner unit 不导出供 sibling 直接调用的 service/repository facade。

## Stable Shared Packages

| Package group | Role |
| --- | --- |
| `workorchruntimecontract` | neutral Runtime bootstrap/state/candidate/dispatch/evidence/governance contracts |
| `workorchprojectioncontract`, `workorchresultcontract` | read/output value contracts |
| `workorchmanagementcontract`, `workorchquerycontract`, `workorchport`, `workorchclient` | protocol/client boundaries |
| `eventcontract`, `runtimeevent`, `workorcheventpayload`, `workorcheventrecordcontract` | typed event infrastructure/value contracts |
| `workorchconfig`, `workorchdefaults`, `workorchmanagedconfig`, `workorchruntimepolicy` | configuration/default/policy assembly |
| `repository`, `workorchrepo` | memory/file repository interfaces and implementations |
| `common`, `runtimepath`, `dialoglanguage`, `doditems`, `soulbinding` | shared value helpers |

Shared packages不能成为隐藏的状态 owner、service locator 或 map-based authority bridge。

## Generated And Runtime Data

- `web/consoleassets` 由 console build 生成并嵌入二进制。
- `.workorch-data`、`.workorch-workspaces`、`bin` 和 `node_modules` 不提交。
- 当前事实以本文件、组件 `readme.md` 和源码为准；历史结构通过 Git 查询，不在 `docs/` 复制维护。

## References

- [Architecture overview](architecture-overview.md)
- [Block boundaries](block-boundaries.md)
- [Component alignment](../01-overview/component-documentation-alignment.md)
- [Runtime contract](../00-core/runtime-stage-contract.md)
