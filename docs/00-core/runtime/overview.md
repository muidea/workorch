# Runtime Semantic Topics

Status: normative

Type: core-contract-index

Last Verified: 2026-08-18

| Topic | Contract | Question |
| --- | --- | --- |
| Owner and language | [Ownership And Language](ownership-and-language.md) | Who owns state and how external facts enter Runtime |
| Semantic convergence | [Semantic Convergence](semantic-convergence.md) | How Definition seed, CWI, binding and evidence converge |
| Candidate transaction | [Candidate And Dispatch](candidate-and-dispatch.md) | How a proposal is evaluated, sealed and dispatched |
| Coordinated machines | [Four State Machines](state-machines.md) | Legal transitions and recovery across SM-1–SM-4 |
| Recoverable wait | [Recoverable Wait](recoverable-wait.md) | Which persisted condition, owner probe and trigger may safely resume a bounded wait |
| Execution budget | [Execution Budget](execution-budget.md) | How calls/effects are allocated, consumed, recovered and projected without a second ledger |
| Cross-machine rules | [Invariants And Change Gates](invariants-and-change-gates.md) | Global invariants, governance and change gates |
| End-to-end sequences | [Runtime Flow](flows/overview.md) | How activation、drive、execution、evidence and restart hand off |
| Verification | [Core Runtime Verification](verification.md) | Which invariant、restart、duplicate and guard assertions must pass |

These contracts are coordinated parts of the single Runtime design; they are not alternative execution models.
