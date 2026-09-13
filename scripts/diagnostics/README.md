# EventHub dependency regression

The original opt-in diagnostic reproduced the v1.5.15 silent rejection defect.
After upgrading to v1.5.17, it is a normal regression in
`internal/pkg/events/eventhub_subscription_test.go` (no build tag). It injects
infrastructure faults, does not access deployed Workspaces, and does not modify
the module cache. A red result is not a successful regression or release gate.

## EventHub subscription acknowledgement

```bash
go test ./internal/pkg/events -run TestEventHubSubscriptionBackpressure -count=20 -timeout=30s
go test -race ./internal/pkg/events ./internal/modules/base/biz -run 'TestEventHubSubscriptionBackpressure|TestRequiredSubscriptionFailure' -count=20 -timeout=1m
```

Default Go module selection tests WorkOrch's checked-in `vendor` implementation.
`-mod=readonly` tests the dependency selected by `go.mod` instead. Neither mode
selects an unrelated sibling repository automatically.

`TestEventHubSubscriptionBackpressure` uses public Observer/Hub APIs and
`testing/synctest`: one identity callback holds the subscription controller,
another command occupies its single pending slot, and virtual time advances
beyond the current 10 ms enqueue timeout. There are no wall-clock sleeps,
private-field access, live services, or business retries.

Required outcomes:

- Accepted Subscribe/Unsubscribe changes must wait for controller acknowledgement.
- A full control queue must return an explicit ResourceExhausted error and leave
  both SimpleObserver callbacks and Hub routing unchanged; a later explicit
  attempt can succeed. This test does not introduce a business retry policy.
- An acknowledged subscription must route immediately; an exact duplicate returns
  Duplicated without damaging routing or creating a local-only callback.
- An acknowledged unsubscription must remove routing, not leave a callback-free
  observer returning `no result returned`.

The blocking identity is fault injection, not a claim that WorkOrch's normal
observer IDs block. This demonstrates a dependency failure path; it does not
uniquely attribute a historical `missing observer` failure to that path.

Fix ownership remains `magicCommon/event`. WorkOrch consumes the official module
and generated vendor, without a local framework fork. Shared Base Biz tests
verify required subscription failures interrupt lifecycle rather than being
swallowed. Process-service tests cover checked shutdown and the existing owner
Stop-before-infrastructure-drain contract. Full WorkOrch regression and deployed
Goal/DoD acceptance remain separate requirements.
