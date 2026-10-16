# Local Operator Access

Status: normative

Type: product-interface-security-contract

## Scope

The first live release uses `workorch-local-operator/v1`: the trusted subject is the operator of the local machine,
including local CLI processes. It is not an OS-account login, a remote user identity or an external identity integration.
All local-machine processes are inside this trust boundary; an untrusted local process or proxy is not isolated from the
operator. Do not expose this listener through a local relay, reverse proxy, tunnel or container port publication.

The production HTTP/Web listener must bind an explicit loopback IP and numeric port. Wildcards, non-loopback IPs and DNS
hostnames are rejected before listening and before application validation. `localhost` is a convenience spelling mapped
directly to `127.0.0.1`, without DNS resolution. Defaults and supplied configurations use explicit loopback addresses.
An injected listener is independently checked before serving.

## Request Admission

Transport-provided peer and listener addresses must be loopback. Browser-supplied actor, authorization or proxy headers
cannot establish another principal. Forwarded, X-Real-IP and X-Forwarded-* headers are rejected, not interpreted as trust.
HTTP Host must be localhost or a loopback IP, preventing DNS-rebinding hosts from reaching the application.

When Origin is present, it must be one canonical origin with the same HTTP/TLS scheme, host and port as the request.
Null, ambiguous, cross-origin, userinfo/path-bearing or wrong-port origins are rejected. Sec-Fetch-Site, when present,
must be same-origin or none. Origin-less local CLI requests remain within the local-machine trust boundary.
Rejected requests return 403 with no-store before business routing, without rewriting or logging business input.

After admission the server creates the private identity:

- protocol: `workorch-local-operator/v1`;
- subject: `local-machine-operator`;
- permission basis: `loopback-peer-and-same-origin/v1`.

This identity proves transport admission only. It does not constitute candidate confirmation, a bounded authorization
grant, Decision approval, execution authorization, evidence credit or permission to bypass owner checks. Those intents
must still bind their exact versions, scope, expiry, budgets and owner-controlled receipts. A missing grant remains manual
mode; no implicit automatic execution follows from opening the Web console.

## Verification

`service/server/local_operator_boundary_test.go` verifies listener rejection, production wrapper coverage, peer/listener
provenance, proxy-header spoofing, DNS hosts, origin/fetch-site rejection, private identity and original input preservation.
HTTP lifecycle tests continue to verify real handler/connection drain and retry; socket tests require local loopback access.
