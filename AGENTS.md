# WorkOrch Agent Instructions

Before changing WorkOrch functionality, read `docs/00-core/runtime-stage-contract.md`.

That document is the authoritative design contract for WorkOrch runtime evolution. Changes that affect runtime behavior, orchestration flow, LLM execution, memory / recall, soul / skill / tool / MCP capability selection, repair / replan, verification / acceptance, or profile mapping must be designed, implemented, and verified against it.

Changes that affect LLM provider request rendering, prompt sections, context budget, cache planning, cache usage projection, provider adapters, memory recall, or runtime evidence packaging must also be checked against `docs/00-core/runtime-request-cache-note.md`.

Core runtime changes must remain functionally neutral. Do not hard-code software development, code patching, test framework, language stack, or business-domain behavior into the core runtime. Put feature-specific behavior in profile / capability layers.

LLM cache behavior is a core runtime contract. Keep the provider-facing stable prefix stable:

- `C0` may contain only long-lived system/language/schema/runtime JSON contract text.
- `C1` contains current Goal/DoD definition content and may change only when definition revision changes.
- `C2` contains declarative capability/governance contracts and may change only when capability or governance policy changes.
- Runtime-stage details such as `assessmentOnly`, `stateChangingOutputRequired`, `repairObjective`, provider retry, recovery mode, current failure, current progress, memory recall, tool results, snapshots, and requested action must live in `C3`/`C4`/`C5`, not in `C0`.
- If a change can make `stablePrefixHash` vary within the same run without a stable contract change, treat it as a request rendering bug and add or update regression tests.

If older docs or local closure plans conflict with the stage contract and neutrality design, treat `docs/00-core/runtime-stage-contract.md` as the source of truth and update the older docs or profile-specific docs instead of weakening the core contract.
