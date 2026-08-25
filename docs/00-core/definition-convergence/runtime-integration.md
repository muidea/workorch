# Definition Convergence Runtime Integration

Status: normative

Type: core-contract-part

Last Verified: 2026-08-25

## State-Machine Binding

| Machine | Definition-convergence responsibility | Boundary |
| --- | --- | --- |
| SM-1 | seal exact startup-input identity; for a Definition-gap Restart persist ticket-bound `ReplacementIntent` and wait for Decision acknowledgement before administrative delivery | does not select dependency, rank, focus or next action; does not read Decision/Workspace repository or treat clarification as continuation |
| SM-2 | validate/seal scheduling contract, all-of gate witness, partition selection and ready projection; commit composite prospective owner facts; admit exact-current gap observation; settle/reopen and re-project | does not call provider, execute effects, interpret domain wording or mutate startup input |
| SM-3 | bounded proof/context/gap/operation documents plus ordinary governance and dedicated clarification sealed dispatch | does not choose the next CWI, write scheduling graph/startup input or directly create a ticket |
| SM-4 | require complete startup/scheduling/projection facts before drive; idempotently recover clarification wait and delivery | does not rebuild authority from projection/LLM or mix a new Definition into Resume |

The owner-local sequence is:

```text
admitted activation fact
  -> SM-2 seals scheduling contract and ready projection/partition/witness
  -> SM-3 bounded proof request
       semantic gap -> persisted diagnostic
         -> SM-2 admits exact gap and enters need_governance
         -> SM-3 seals RuntimeDefinitionClarificationDispatch
         -> Decision owner opens a run-scoped clarification ticket
         -> new candidate admission/promotion + explicit Restart
         -> Run owner persists ticket-bound ReplacementIntent
         -> Decision replacement-resolution acknowledgement
         -> predecessor administrative delivery -> fresh successor
       proof accepted -> capability-owner feasibility
         -> prospective acquisition: File outer feasibility + immutable hypothetical manifest delta
           -> Shell dependency catalog/observation-bound pure satisfaction result
           -> SM-2 exact-current composite CAS + seal gate
         -> SM-2 seals binding/reconciliation
         -> SM-3 operation candidate/dispatch/receipt
         -> SM-2 settles or reopens and re-projects
```

All aggregate mutation still uses the sole `ApplyInternalEvent -> RuntimeTransitionCoordinator.Commit` boundary. Scheduling,
ready projection and clarification dispatch are Orchestrator-private facts, not EventHub target-state topics.

For `prospective_owner_target`, the pre-seal declaration's exact `ProspectiveSatisfactionContractRef` is the only authority to
join the acquisition owner and required owner facts. The semantic manifest keeps required-owner
`OwnerFactDependencyCatalogs` separate from provider-selectable `OwnerCatalogs`: dependency catalogs participate in hash,
freshness, CAS and seal, but never appear as LLM-selectable operations/surfaces. File and Shell return only their own factual
outer/nested observations. Shell typed stale uses the existing bounded catalog-refresh transition; `would_not_satisfy` permits
no provider correction because the exact target is activation-sealed: it is persisted as the complete composite fact and the
next drive deterministically closes the common `semantic_contract_failed` terminal while preserving
`prospective_owner_fact_unsatisfied` as its typed original reason. None of these owner results can directly change SM-2 or either
capability owner's state.

## Governance, Resume And Restart

- ordinary governance resolution stays on the same startup input, scheduling contract, partition, focus and gate-witness lineage;
- if an ordinary resolution reveals a stale owner fact, SM-2 re-acquires/reconciles against the same graph; it never reads the
  latest Workspace Definition or asks LLM to rebuild it;
- a runtime Definition-gap ticket has no `continue` resolution. The predecessor remains governed until a new all-of-admitted
  startup input, explicit Restart, matching ticket-bound replacement acknowledgement and terminal delivery complete;
- Resume loads the same `StartupInputHash`, Definition revision, partitions, scheduling, focus/witness, credit and recovery ledger;
  it does not rerun the compiler, change priority or replay durable effects;
- Restart uses Workspace owner's current eligible startup input to construct a fresh bootstrap/partition/schedule. The successor
  inherits no ready queue, binding, credit, LLM document or context expansion; prior owner effects are re-observed by reconciliation;
- missing current scheduling/partition facts fail bootstrap/aggregate validation before drive; projection/event history/default
  order cannot synthesize them. `fresh_activation_required` is reserved for a readable Run with no sealed bootstrap.

## Three Prerequisite Lanes

| Lane | Producer and timing | Valid outcome |
| --- | --- | --- |
| static declared Definition/Profile | pure Workorchd compiler before Run | promotion refusal or pre-Run clarification ticket |
| dynamic owner readiness | capability owner typed acquisition/result after Run | SM-2 prerequisite wait, reconciliation or typed terminal before provider/dispatch/effect |
| runtime semantic gap | bounded SM-3 document under current authority | SM-2 governed wait plus dedicated run-scoped clarification ticket |

The lanes cannot impersonate one another: static compilation does not probe live capabilities, dynamic readiness does not rewrite
the Definition, and provider diagnostics do not become compiler facts or capability receipts.

## Component Landing

| Component | Owns in this contract | Must not |
| --- | --- | --- |
| `blocks/workspace` | candidate ledger/hash, all-of promotion, exact Goal/DoD, Template/Profile binding, resolved directory, activation admission and retired lineage | Runtime focus/schedule or assisted-draft authority |
| `blocks/decision` | distinct candidate-scoped and run-scoped clarification ticket commands/results with deterministic identity | mix scopes, use ordinary continue/fail, read Workspace/Run repository or start a Run |
| `workorchd/setupdraft` | optional bounded authoring draft adopted only through Workspace candidate command | force LLM authoring, offer legal one-of, write canonical Definition or imply Runtime zero-LLM |
| `workorchd/activationcore` | pure admission plus deterministic mapping/graph/gate/partition compilation from owner-sealed inputs | infer free-text prerequisites, read live capability repository, create tickets or choose focus |
| `workorchd` use case | orchestrate candidate/admission/promotion, pre-Run ticket and execution intent | own Definition/ticket state or continue after unresolved admission |
| `workorchruntimecontract` | pure startup/mapping/relation/schedule/gate/partition/gap DTO validation and hashing | EventHub, repository, CAS, one-of terminal evaluator or transition policy |
| `kernel/orchestrator` | SM-2/3/4 scheduling, ready/witness, bounded documents, clarification dispatch and recovery | domain inference, startup mutation, direct ticket write or second queue owner |
| `blocks/llm` | startup-bound C1/C5 rendering and factual provider result | ordering/dependency/ticket authority or input rewriting |
| capability blocks | current readiness, feasibility and receipt facts; File prospective delta and required-owner pure satisfaction observation | choose CWI, graph, rank, partition, correction or target state; read/mutate a sibling owner |
| `blocks/run` | opaque aggregate persistence and ticket/startup-bound replacement transaction | decode schedule, choose focus, terminate predecessor before acknowledgement or continue old Run |
| projection/Web/TUI | typed startup/ticket/rank/context explanation | offer one-of, parse display text into action or drive Runtime |

No new framework unit is introduced; all responsibilities remain within the existing 1 Initiator, 4 Modules and 17 Blocks.
