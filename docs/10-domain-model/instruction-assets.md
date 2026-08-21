# Soul And Skill Instruction Assets

Status: normative

Type: domain-model

Last Verified: 2026-08-21

## Soul

Soul 是治理与行为边界的声明对象，由 `blocks/soul` 唯一拥有。它可以声明阶段约束、allowed tool kinds、capability
allowlist 与风险边界，但不能选择 Runtime 下一动作、改写 Definition 或授予 evidence credit。

Workspace 可以声明默认 Soul 以及按语义位置绑定的 Soul。activation 只消费 owner 返回的 exact revision/snapshot，运行期
不能从 live resource 或 projection 静默替换。

## Skill

Skill来源必须保持分离：`blocks/skill`只拥有WorkOrch Registry资源；SkillHub是Application-private external catalog，
workspace-local资产也不能冒充Registry owner state。Registry create由owner签发canonical content-hash provenance，get/list
使用concrete bounded result；所有来源进入Runtime前都转换为detached typed provenance。

activation最多封存5项Skill；instruction单项上限8 KiB、总计24 KiB，metadata总计16 KiB。Skill只进入volatile C3；
LLM/Orchestrator不得在request或drive时直读Registry/Catalog、重新选择/裁剪，或把Skill提升到C0～C2。正文仅为受预算
约束的context，不成为operation authorization或业务结果。

## Invariants

- Soul/Skill 的选择结果与 exact snapshot 必须可追溯；
- owner resource mutation 只影响后续 fresh activation，不改写 active Run；
- instruction content 可约束 candidate，但不能绕过 capability schema、sealed dispatch 或 governance；
- concrete software-development behavior stays in Profile/Soul/Skill, not neutral orchestrator core.

## Related Documents

- [Domain Model Overview](overview.md)
- [Context Assembly Flow](../00-core/runtime-request-context-assembly.md)
- [Soul Routing](../40-profiles-capabilities/workspace/soul-routing.md)
