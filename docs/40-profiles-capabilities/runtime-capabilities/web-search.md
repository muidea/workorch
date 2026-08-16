# Web Search And Fetch Capability

Status: active

Type: profile-capability

Last Verified: 2026-08-16

## Ownership

`blocks/web` owns provider selection, URL/network policy, bounded HTTP transport and factual receipts for `web/search` and
`web/fetch`. Web configuration is declared under `webSearch`; profile/Soul may allow or narrow use but cannot bypass owner
policy.

## Operations

| Kind | Required input | Optional bounds | Output |
| --- | --- | --- | --- |
| `web/search` | non-empty `query` | `providerId`, positive `maxResults`, matching approval ID | bounded result items and source refs |
| `web/fetch` | valid `url` | `providerId`, positive `maxBytes`, matching approval ID | final URL, content metadata, bounded body/source ref |

System bounds cap provider and caller values. Current defaults are 8 results (system maximum 20), 256 KiB fetch content
(system maximum 1 MiB), and 3 redirects (system maximum 5).

## URL And Network Policy

- only HTTP/HTTPS URLs are syntactically supported;
- userinfo URLs, metadata endpoints, link-local/unspecified/multicast addresses and configured denied domains are denied;
- configured allow domains are enforced when present;
- plain HTTP, loopback and private-network access require a matching Approval rather than being silently allowed;
- DNS resolution and final redirect URL are reclassified, so an allowed public URL cannot redirect around policy.

Approval must match Run, action, provider and query/URL context. It only authorizes execution; it does not accept resulting
evidence.

## Receipts And Evidence

The owner returns provider identity, query/URL, result counts, final URL, truncation, duration, approval identity, target refs
and a typed failure where applicable. Search/fetch content becomes accepted evidence only after Orchestrator validates current
dispatch, CWI binding and quality.

## Failure Boundary

Invalid input, policy denial, approval requirement/mismatch, provider unavailable, timeout, bad gateway response and content
limit are factual outcomes. Web owner does not choose candidate correction, retry, governance continuation or terminal state.

## Verification

- `go test ./internal/modules/blocks/web/... -count=1`
- `scripts/verify-workspace-web-search-e2e.sh`
