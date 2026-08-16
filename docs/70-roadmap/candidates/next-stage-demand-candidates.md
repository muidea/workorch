# WorkOrch Next-Stage Demand Candidates

Status: active

Type: roadmap-candidates

Last Verified: 2026-08-16

## Promotion Rule

Every candidate must preserve single-owner transitions and typed EventHub boundaries. It cannot be implemented by adding a
shared target-state API, generic Runtime adapter registry, projection-driven recovery or a fifth Runtime state machine.

## C2 Distributed Scheduling And Worker Fleets

Possible direction: durable distributed admission, remote workers, leases, heartbeats and bounded retry.

Before promotion define:

- whether scheduling/lease state has a new sole owner or remains Run-owner state;
- worker command/result identities and duplicate/late completion semantics;
- loss, timeout, cancellation and process/network partition behavior;
- observability and load/capacity gates.

## C3 Multi-Tenant Authorization And Quota

Possible direction: tenant/project/user identity, resource visibility, authorization and execution/provider quota.

Before promotion define identity propagation, owner-local authorization checks, repository partitioning, audit redaction, quota
owner and failure semantics. UI/API filtering cannot substitute for storage and command authorization.

## C4 External Capability Distribution

Possible direction: governed discovery, installation and enablement of external capability-owner packages.

Before promotion define publisher identity, artifact integrity, trust/approval policy, owner registration lifecycle, typed
operation catalog compatibility and uninstall/restart behavior. External capability code must still own concrete operations and
must not register a generic next-action/authority callback.

## C7 Collaboration And Sub-Agent Work

Possible direction: multiple operators, shared knowledge assets and delegated/sub-agent execution.

Before promotion define collaboration objects and sole owners, delegation scope, identity/audit, conflict handling, governance
responsibility and how delegated evidence binds back to one Run authority without introducing peer reducers.

## C10 Advanced LLM Budget And Provider Policy

Possible direction: provider failover policy, provider-native/tokenizer precision and differentiated context-too-large handling.

Before promotion prove:

- failover cannot mutate a Run-sealed provider/model/ceiling without an explicit owner transaction;
- C0/C1/C2 stable prefix remains provider-neutral;
- native usage and estimates retain provenance;
- errors map to typed facts without provider-text control flow;
- no map-based authority rebuild or hidden retry owner appears.

Historical context-budget work is archived at
[llm-context-budget-todo.md](../../90-archive/completed/llm-context-budget-todo.md).
