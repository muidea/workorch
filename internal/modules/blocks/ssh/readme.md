# SSH 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-21

## 功能范围与边界

`blocks/ssh` 是 `ssh/command` 的唯一 capability owner。它不拥有 Run、Runtime 或远端业务状态，只负责：

- 在进程启动时冻结 owner-private target/session/credential 配置；
- 发布 bounded capability、admissibility 与 feasibility fact；
- 校验 Orchestrator 封存的执行身份、治理 grant 和完整 operation payload；
- 使用严格 host-key 验证执行远端命令，并以 owner 配置的 observation command 取得前后状态事实；
- 返回唯一的 `RuntimeOperationReceipt`，不选择 retry、repair、terminal 或下一 action。

影响状态机为 SM-2（catalog、feasibility、governance requirement）和 SM-3（sealed dispatch、执行、receipt）。SM-1、SM-4
没有 SSH 私有迁移。

## 配置合同

SSH 配置位于应用配置的 `[ssh]` 子树，但只由 SSH owner 读取并在启动时解析。保存后的修改需重启才生效。

```toml
[ssh]
defaultTimeout = '30s'
secretsDirectory = 'ssh'

[ssh.targets.target-a]
host = 'build.internal'
port = 22
knownHostsFile = 'known_hosts'
hostKeyAlias = 'build.internal'
observationRef = 'deployment-state'
observationCommand = 'sha256sum /srv/app/current/revision'

[ssh.sessions.session-a]
targetRef = 'target-a'
username = 'operator'
credentialRefs = ['credential-a']

[ssh.credentials.credential-a]
privateKeyFile = 'id_ed25519'
```

- `targetRef`、`sessionRef`、`credentialRef` 必须是最多 64 字节的 opaque ID；不得使用 URL、host、username 或路径作为 ID。
- credential 恰好声明 `passwordEnv`、`privateKeyEnv`、`privateKeyFile` 之一；配置类型不存在 inline password/private-key 字段。
- `secretsDirectory`、`knownHostsFile`、`privateKeyFile` 必须是规范化相对路径；启动时解析 symlink 后仍须位于配置目录/owner secrets 目录内。
- private-key 文件必须是普通文件且不可被 group/world 访问，known_hosts 不可被 group/world 写入；secret 与 known_hosts
  均有 1 MiB 上限并在启动时读取、冻结（password 另限 64 KiB，且不得含换行/NUL）。
- 每个 session 绑定一个 target 和明确的 credential allowlist；不存在或错绑的引用在 feasibility/execute 阶段 fail-closed。
- catalog 最多包含 32 个 target、session、credential 及 32 个合法绑定；admissibility 以
  `ssh-binding/<targetRef>/<sessionRef>/<credentialRef>` 发布可选 opaque 组合，不发布 endpoint、username 或 secret。
- 每个 target 必须配置 known_hosts identity 和 observation command。空 catalog 合法，但不会产生可执行 SSH target。

## EventHub 交互合同

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/semantic-proof command/result | 完整；仅返回 owner fact |
| `kernel/orchestrator` | 双向 | `ExecuteOperationCommand/Result`、receipt query/commit | 完整；execute result 只含 receipt |
| Governance / Approval | 间接 sealed fact | SSH 返回 approval requirement；resume 携带 exact typed grant | 完整；无直接 service/repository 依赖 |
| 远端 SSH endpoint | owner-private adapter | frozen target/session/credential、strict known_hosts、command/observation | 完整；endpoint/secret 不跨 EventHub |

`ssh/command` 的 Runtime operation 只允许：

- `TargetRefs[0]`：opaque `targetRef`；
- `sessionRef`、`credentialRef`：opaque owner refs；
- `command`：最多 16 KiB；
- 可选 `timeoutMillis`：1,000～300,000。

三个引用必须来自同一条 owner 发布的 `ssh-binding` constraint。`host`、`port`、`username`、`password`、`privateKey`、
`privateKeyPath` 在 Runtime operation 中均为 forbidden。Orchestrator
把持久化 operation 克隆为 `SealedRuntimeOperation` 并附完整 payload hash；SSH 在任何 receipt query、治理或网络动作前校验
source、scope、operation ID、target/CWI scope 与 payload hash。强调用没有同步 result 时直接返回，`Post` 不会触发网络动作。

## 治理、执行与回执

1. SSH 先通过 receipt query 验证当前 active claimed `ExecutionKey + Fence`。有 persisted governance binding 时，查询必须携带
   完全相同的 grant；省略、附加或漂移均 conflict。Found/Receipt 一致性、query identity 与 commit 完整 receipt equality
   由 capability owners 共享的 typed validator 校验。
2. 每个远端命令都要求 `GovernanceApproval`。不可变 authorization context 绑定 target/session/credential refs、
   operation kind、command hash 与 timeout；未获 grant 时只提交 `governance_required` receipt，零网络调用。
3. grant 必须精确匹配 approval identity、subject、action、risk 和 authorization hash，随后 SSH 才解析 owner-private endpoint
   与 secret。
4. transport 固定使用 `-F /dev/null`、`StrictHostKeyChecking=yes`、owner-captured known_hosts、
   `GlobalKnownHostsFile=/dev/null` 和 `IdentitiesOnly=yes`，且认证方式绑定所选 password/private-key credential。密码通过临时
   0600 文件供 askpass 读取，不进入 child environment value；
   私钥写入 owner 临时 0600 identity 文件。临时目录按单次调用释放。
5. SSH 在同一有界 timeout 内执行 `observation-before -> command -> observation-after`。只有前后 observation hash 不同时才报告
   `remote_durable + Changed=true`；相同则返回 `no_op + none + ZeroWork=true`。命令失败后仍尝试 after observation，若事实已改变，
   rejected receipt 仍如实携带 remote change；after observation 不可得时 effect 保持 `unknown`，不得伪造 durable evidence。
6. `remote_durable` receipt 携带 `BeforeHash/AfterHash` 与 `remote_resource` change。stdout/stderr 不跨 EventHub；evidence 只保存全量
   stream hash、字节数、截断标记、duration、exit code 与 opaque content-addressed ref。失败 diagnostic 明确标记 raw output omitted/
   redacted，不承诺 output preview 或 raw ref。

`ssh/command` 固定为 `remote_durable + at_most_once`。owner receipt 已提交而响应丢失时，同 key/fence 查询返回原 receipt，零二次
调用；side effect 后 receipt 未提交的 claimed recovery 由 Orchestrator 进入 `outcome_unknown`，不得自动重放远端命令。

本合同移除了旧版 raw host/username/credential operation schema，并推进全局 `RuntimeOperationContractRevision`。已经封存旧 revision
的 active Runtime 不做降级解释；部署后应通过 workspace-scoped restart 生成 fresh bootstrap/candidate，不能继续重放旧 SSH dispatch。

## 生命周期与资源限制

- SSH 配置非法时 Configure 失败；缺少 EventHub 时 Start 失败。Stop 反向解除订阅并清空内存 credential snapshot。
- timeout/cancel 终止本地 `ssh` child；SSH 协议不能通用证明远端 shell 在连接中断后已终止。因此“任意远端后台进程清理”不是
  本组件可承诺的合同。需要该语义的 target 必须通过 owner 配置的远端 wrapper/作业协议提供独立可观察状态，不能由 Runtime
  把普通 command 猜成可安全重放。
- owner catalog revision 包含冻结配置与 secret/known-hosts 内容 hash，不包含其明文；restart 后配置变化通过 SM-2 stale fact
  触发重新封存。

## 回归覆盖

- [x] Runtime/EventHub wire 不含 endpoint、username、password、private key/path；sealed payload drift 在网络调用前拒绝。
- [x] 配置拒绝 absolute/traversal path、symlink escape、宽权限私钥、错绑 session/credential 与 option-shaped endpoint。
- [x] strict host-key、禁用用户 SSH config、password/private-key 临时文件及清理。
- [x] governance missing/drift、wrong producer、nil result、active receipt authorization 与 persisted grant 省略均 fail-closed。
- [x] before/after changed、no-op、bounded output/hash/ref、resource change 与 raw-output omission。
- [x] at-most-once claimed recovery、owner receipt response-loss replay由 Orchestrator 通用 SM-3 回归覆盖。

## Deployed verification

真实 OpenSSH endpoint 的 host-key rotation、网络抖动、cancel/timeout 属于部署环境验收，不是当前 code contract
缺口；验证结果不得放宽上述 host-key、at-most-once、receipt 或 fail-closed 约束。
