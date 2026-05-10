# Runtime Recovery Governance Closure Design

## Background

`ws-5d0f8f4ba809c441` exposed a neutral runtime recovery gap:

- a run can reach a `blocked_on_decision` state after repair attempts are exhausted;
- the blocker already carries structured recovery actions such as `retry_same_step`, `replan`, `provide_information`, and `terminate`;
- auto-governance currently treats the ticket as a generic missing-information question and may escalate it because the human-facing question contains choice/approval wording;
- the run therefore cannot continue even when a safe, structured recovery action is available.

This must be fixed in WorkOrch's framework layer without adding software-development, language, compiler, or domain-specific rules to kernel modules.

## Goals

- Keep WorkOrch core capability-neutral.
- Let structured runtime recovery tickets drive automatic continuation when policy allows it.
- Avoid endless repair loops when a model returns `completionState=continue` without an actionable continuation.
- Preserve human escalation when provider/config/human-required recovery is involved.
- Make the behavior traceable through auto-governance traces and decision resolution payloads.

## Non-Goals

- Do not add Go, JavaScript, test, compiler, or software-specific error matching to kernel/governance/orchestrator.
- Do not bypass decision validation.
- Do not change existing workspace data compatibility in this iteration.
- Do not silently approve provider or WorkOrch runtime configuration changes.

## Recovery Model

Runtime recovery decisions are driven by structured ticket context, not by free-text wording.

Required context keys:

- `recoveryPolicy`: identifies the recovery policy.
- `allowedRecoveryActions`: ordered list of actions allowed by the runtime.
- `failureClass`: neutral failure class, if known.
- `failureSignature`: neutral repeated-failure signature, if known.
- `humanRequired`, `requiresHuman`, `manualRequired`, `requiresHumanFields`: optional hard manual gates.
- `scope`, `dependency`: optional boundary markers for WorkOrch runtime vs target workspace dependencies.

Supported actions:

- `retry_same_step`: retry current failed step with the resolution payload as additional input.
- `replan`: supersede the current run and create a fresh plan from current Goal/DoD/context.
- `provide_information`: resume with supplied missing information.
- `terminate`: reject the current path.

## Auto-Governance Rule

For `recoveryPolicy=runtime_repair_exhausted_requires_strategy`, auto-governance may resolve the ticket as `replan` when all conditions hold:

- auto ticket resolution is enabled;
- the run-level auto-ticket budget still allows resolution;
- `allowedRecoveryActions` contains `replan`;
- no explicit human/manual-required context marker is present;
- the ticket is not a WorkOrch runtime provider/config recovery ticket;
- the failure class is not a provider/auth/rate-limit/context-size class that requires external configuration or operator intervention.

The generated resolution payload must include:

- `answer: "replan"`;
- `recoveryAction: "replan"`;
- `source: "auto_governance"`;
- `confidence`;
- `reason`;
- semantic normalization metadata.

Policy trace:

- `policyMatched=runtime_repair_exhausted_auto_replan`;
- `Action=answer`;
- `Status=resolved`;
- evidence must include the ticket ID and, when available, the active Goal ID.

## Manual Escalation Rules

Auto-governance must escalate when:

- the ticket has explicit human/manual-required markers;
- `allowedRecoveryActions` does not contain `replan`;
- the failure class or dependency indicates WorkOrch runtime provider/config recovery;
- the run budget is exhausted;
- ticket kind is unsupported;
- structured context is insufficient for a safe action.

## No-Actionable Continuation Handling

When an LLM step reports a non-completed `completionState` without a next action:

- the orchestrator may dispatch one neutral follow-up asking for a concrete continuation or final DoD assessment;
- if the follow-up still has no actionable continuation, the runtime must not create an unbounded continuation chain;
- the repeated failure should enter the normal repair/exhaustion path, where the structured recovery ticket can be auto-replanned if allowed.

## Boundary Contract

Core packages may use only neutral concepts:

- recovery policy;
- action list;
- failure class/signature;
- scope/dependency;
- manual gate markers;
- Goal/DoD references.

Software-development-specific acceptance rules must stay in workspace templates, souls, skills, or policy configuration.

## Verification

Required focused checks:

- governance auto-resolves eligible repair-exhausted tickets with `recoveryAction=replan`;
- governance escalates repair-exhausted tickets when provider/config recovery is detected;
- governance escalates repair-exhausted tickets with manual-required markers;
- app decision validation accepts the auto-generated structured replan payload;
- orchestrator continues to treat unresolved decision tickets as blocked and resolved replan tickets as run supersession;
- boundary contract verification passes.
