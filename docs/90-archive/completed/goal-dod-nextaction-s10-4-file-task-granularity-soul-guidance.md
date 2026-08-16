# S10.4 File And Task Granularity Soul Guidance

Status: completed

Owner: Soul/skill

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S10.2 Software Dev Stage Policy Binding](./goal-dod-nextaction-s10-2-software-dev-stage-policy-binding.md)
- [S10.3 Evidence Policy Coverage Binding](./goal-dod-nextaction-s10-3-evidence-policy-coverage-binding.md)

## Contract Audit

- Scope: software-dev task slicing, file granularity, and oversized output avoidance guidance.
- Observed state: software-dev Souls already guide target selection, direct state change, validation evidence, patch quality, and report-only avoidance. They do not yet explicitly require bounded task slices, single-file cohesion, or splitting large generated files into cohesive modules.
- Expected state: software-dev planning/execution/validation/artifact Souls instruct the LLM to plan and execute small verifiable slices, keep generated files at a maintainable size, split oversized outputs by responsibility, and validate each slice with direct evidence.
- Deviation: without explicit Soul guidance, the runtime may receive large, diffuse patches or monolithic files that slow convergence and make coverage evidence harder to bind, even though core scheduling and coverage are business-neutral.
- Owner: Soul/skill. The behavior is software-development strategy and must not be hardcoded into orchestrator, coverage, file block, or patch block logic.
- Event path: embedded software-dev Soul markdown -> soul repository -> workspace soul binding -> activation soul routing -> LLM stage prompt -> candidate plan/patch/assessment.
- Contract gap: missing reusable wording that turns Goal/DoD gaps into bounded implementation slices and keeps file-level output cohesive.
- Proposed change: update software-dev planning, execution, validation, and artifact Souls with file/task granularity rules, plus defaults tests that assert the guidance exists.
- Boundary check: no core logic, runtime contract, patch interpreter, or repository behavior is changed. The guidance contains software-development strategy only in software-dev Soul markdown/tests.
- Verification: defaults Soul tests, affected defaults package tests, full `go test ./...`, `git diff --check`, and boundary search.

## Guidance Rules

Planning:

- Convert each unresolved Goal/DoD gap into a bounded implementation slice with target files, expected state change, direct validation, and evidence refs.
- Prefer one cohesive responsibility per file and one primary target per state-changing slice.
- Split new modules by runtime entrypoint, domain logic, adapters, configuration, and tests when a single file would become broad or hard to review.

Execution:

- Implement one bounded slice per state-changing graph unless the runtime asks for a larger batch.
- Avoid large generated files, catch-all utility files, and unrelated broad rewrites.
- If a new or modified file becomes too broad, split by responsibility before adding more behavior.

Validation:

- Failed assessment should call out oversized or mixed-responsibility changes when they prevent traceable Goal/DoD evidence.
- nextActions should name the next bounded slice, target files, validation command, and evidence refs.

Artifact:

- Summaries should report slice boundaries and file responsibilities.
- Residual risks should include any intentionally deferred split or large-file risk.

## Acceptance

- Software-dev Soul guidance contains bounded slice and file responsibility rules.
- Core remains business-neutral.
- Tests assert the guidance is present in embedded Souls.

## Verification

- `go test ./internal/pkg/workorchdefaults -run 'TestEmbeddedSoftwareDevSoulsContainPlanningExecutionValidationArtifactGuidance' -count=1`
- `go test ./internal/pkg/workorchdefaults -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "bounded implementation slice|单文件职责|large generated file|monolithic|oversized" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg/workorchdefaults -g '!**/defaults/souls/**'`
