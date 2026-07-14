# Runtime Validation Timeout And External Dependency Closure Plan

## Contract Audit

- Scope:
  - `ws-705249a8d7816330/run-076/graph-006/001-acquire-compile-evidence`
  - `ws-db040d24a190f6e5/run-076/graph-003/001-tidy`
- Observed state:
  - A validation `shell/command` reached runtime dispatch without `timeoutMillis` and was rejected by `runtime_validation_dispatch_contract_rejected`.
  - A shell validation failure was classified as `external_dependency_missing`, but the run ended as `failed` after repair exhaustion instead of entering an external dependency blocker.
- Expected state:
  - Runtime dispatch normalization must make validation shell commands bounded before the hard validation gate.
  - External dependency diagnostics must take precedence over repair or convergence budget exhaustion and block on decision with external dependency context.
- Deviation:
  - Validation dispatch still depends on LLM to provide `timeoutMillis` in some graph continuation paths.
  - External dependency recovery can be overwritten by generic convergence exhaustion handling.
- Owner: core orchestrator.
- Event path:
  - LLM graph output -> graph step spec -> runtime dispatch normalization -> validation gate -> shell runtime.
  - shell failure -> validation diagnostic -> recovery action -> blocker/terminal.
- Contract gap:
  - Dispatch normalization did not cover all validation shell nodes.
  - External dependency blocker priority was not absolute inside runtime repair exhaustion handling.
- Proposed change:
  - Add a generic validation dispatch normalizer before runtime validation gate and persist the normalized step input.
  - Keep the low-level validation gate strict: missing timeout is still invalid if normalization was bypassed.
  - Prevent convergence budget escalation from overriding external dependency blocker context.
  - Align projection resource health with runtime resource lifecycle so `failed` and `timeout` process outcomes are treated as closed resources, while cleanup failures remain degraded.
  - Keep graph terminal continuation full-step loading scoped to graph node continuation so non-graph follow-up dispatch still uses summary-first reads.
- Boundary check:
  - No template, language, command, module, domain, or file-name special case.
  - All behavior is driven by structured requiredOutput, validationEvidence, nextAction, convergence state, and validation diagnostics.
- Verification:
  - Unit tests for validation timeout normalization.
  - Unit/integration tests for external dependency blocker precedence over convergence budget.
  - `gofmt`, focused orchestrator tests, and `git diff --check`.

## Checklist

- [x] Validation shell dispatch normalization adds bounded `timeoutMillis`.
- [x] Runtime validation gate still rejects missing timeout if called directly.
- [x] External dependency blocker is not overridden by convergence budget exhaustion.
- [x] Resource health projection does not report closed failed/timeout runtime resources as leaks.
- [x] Non-graph follow-up dispatch keeps summary-first step reads.
- [x] Focused regression tests pass.
