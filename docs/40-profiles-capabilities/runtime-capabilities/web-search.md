# Web Search And Fetch Capability

Status: normative

Type: profile-capability

Last Verified: 2026-08-21

## Ownership

`blocks/web` owns provider selection, URL/network policy, bounded transport, normalized content evidence and factual receipts
for `web/search` and `web/fetch`. Web configuration is declared under `webSearch`; profile/Soul may allow or narrow use but
cannot bypass owner policy.

## Operations

| Kind | Required input | Optional bounds | Receipt evidence |
| --- | --- | --- | --- |
| `web/search` | non-empty `query` (≤ 4096 bytes) | `providerId`, locale/recency/domain filters, `maxResults` | actual bounded title/snippet summary, content hash, raw/source refs, truncation |
| `web/fetch` | HTTP(S) `url` (≤ 4096 bytes) | `providerId`, `maxBytes`, `extractMode` | actual bounded extracted summary, content hash, raw/source refs, final-target truncation |

System bounds cap both provider and caller values: at most 20 results, 1 MiB fetch content, 5 redirects and 60 seconds.
The legacy `approvalId` operation parameter is forbidden. Governance authority is carried only by the sealed execution
`GovernanceGrant`.

## URL And Network Policy

- Search sends a bounded POST only to the configured provider; redirects must remain on the exact provider origin.
- Fetch uses an owner-controlled direct GET. The configured legacy `fetchEndpoint` remains readable for configuration
  compatibility but is not a Runtime fetch authority and is not called.
- Userinfo URLs, metadata endpoints, link-local/unspecified/multicast addresses, invalid ports/zones and configured denied
  domains are rejected before access.
- Plain HTTP, loopback and private-network access require an exact approval grant. Redirect targets are checked before follow;
  an already granted initial target cannot authorize a different risky redirect target.
- Each hostname is resolved once per execution; the classified address set is pinned into the transport dialer, with proxy
  routing disabled. DNS rebinding therefore cannot substitute a different target after preflight.

Approval matches the current execution key/fence and the exact provider, action, query/URL, bounds, policy decision and
authorization hash. It authorizes access only; it does not accept resulting evidence.

## Receipts, Recovery And Evidence

Search/fetch publish `effect=none + at_most_once`. Before external access the owner validates the sealed operation and asks
Orchestrator to confirm the current claimed execution/receipt fence. A missing receipt is executable only when that exact
claim is current; after a submitted request loses its receipt, recovery must produce `outcome_unknown`, not issue another
provider request.

The concrete cross-owner result contains only `RuntimeOperationReceipt`. Actual Web content crosses as an independently
bounded evidence summary plus SHA-256 hash and opaque refs; the normalized full payload is stored under the workspace internal
directory and addressed as `workorch://web/evidence/<hash>`. Orchestrator alone decides whether this factual evidence satisfies
the sealed CWI binding.

Provider/network/protocol errors are mapped to fixed, bounded diagnostics. Raw transport errors, endpoints, credentials,
headers and provider bodies do not enter Runtime feedback, aggregate, audit or projection.

## Verification

- `go test ./internal/modules/blocks/web/... -count=1`
- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- `make verify-boundary-contract`
- `scripts/verify-workspace-web-search-e2e.sh` (deployed provider verification)
