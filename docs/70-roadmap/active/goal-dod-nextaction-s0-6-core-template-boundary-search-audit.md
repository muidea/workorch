# Goal/DoD NextAction S0.6 Core Template Boundary Search Audit

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S10.1 Template Stage Policy Schema](./goal-dod-nextaction-s10-1-template-stage-policy-schema.md)
- [S10.2 Software Dev Stage Policy Binding](./goal-dod-nextaction-s10-2-software-dev-stage-policy-binding.md)
- [S10.3 Evidence Policy Coverage Binding](./goal-dod-nextaction-s10-3-evidence-policy-coverage-binding.md)
- [S10.4 File Task Granularity Soul Guidance](./goal-dod-nextaction-s10-4-file-task-granularity-soul-guidance.md)
- [S10.5 Template Capability MCP/LSP Binding](./goal-dod-nextaction-s10-5-template-capability-mcp-lsp-binding.md)
- [S10.6 Non Software Template Isolation](./goal-dod-nextaction-s10-6-non-software-template-isolation.md)

## Contract Audit

- Scope: core/template boundary search across kernel, workorchd appservice, port/client, LLM context and template/profile strategy.
- Observed state: S10 moves software-development-specific strategy into template/profile/Soul/skill data. Core slices consume structured fields and avoid template, language, command-output and provider-specific inference. Boundary searches are recorded in individual slice verification sections.
- Expected state: WorkOrch core remains a business-neutral scheduling and governance framework; template/profile/Soul/policy carry business strategy.
- Deviation: S0.6 originally had no standalone boundary-search audit, so the later S10 and per-slice searches were not indexed from the P0 baseline.
- Owner: all modules, with enforcement through per-slice boundary search and `verify-boundary-contract`.
- Event path: template/profile data -> workspace binding -> neutral core interpretation -> projection/evidence. Core must not infer behavior from template names or business text.
- Contract gap: documentation gap only. Later S10 slices and per-slice searches provide the implementation evidence.
- Proposed change: record the boundary-search baseline as S0.6 and keep the reusable search commands visible.
- Boundary check: this document adds audit instructions only and does not change runtime behavior.
- Verification: reusable boundary searches, linked S10 tests, and roadmap coverage audit.

## Boundary Search Commands

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "stateChangingOutputRequired|repairObjective|providerTimeoutStateChangingRetry|assessmentOnly|runtimeRepairFollowUp|currentProgressDigest|memoryRecall|Cache-Control|cache_control" internal/modules/blocks/llm internal/modules/kernel
rg -n "definitionChangeImpact|coverageTargetSet|repairActionPackage|processEvidenceTimeline|requiredOutput|blocked_on_decision|contextGate" internal/modules/kernel internal/modules/blocks/llm
rg -n "internal/modules/application/workorch(?!d)" application internal scripts Makefile README.md --pcre2
rg -n "workorchclient|biz/backends" internal/services/workorch/service internal/services/workorch/biz/agent internal/services/workorch/biz/session -g "*.go"
rg -n "internal/services/workorch|internal/pkg/workorchclient|biz/backends" internal/pkg/workorchport -g "*.go"
rg -n "workorchport|internal/services/workorch|framework/application|EventHub|event\\.Hub" internal/pkg/workorchclient -g "*.go"
rg -n "kernel/.*/(biz|service|repository)|repository\\." internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient -g "*.go"
```

## Verification

- S10.1-S10.6 completed and document template/profile/Soul/policy strategy boundaries.
- Individual S1-S11 closure docs include changed-file boundary searches.
- `python3` roadmap coverage audit reports S0.6 as covered by this document.
