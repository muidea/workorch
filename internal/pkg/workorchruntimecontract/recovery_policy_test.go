package workorchruntimecontract

import "testing"

func TestRuntimeRecoveryPolicyAllowsWatchdogExternalRetryRequiresExternalBoundary(t *testing.T) {
	testCases := []struct {
		name   string
		policy RuntimeRecoveryPolicy
		want   bool
	}{
		{
			name: "external read idempotent",
			policy: RuntimeRecoveryPolicy{
				SideEffectPolicy:  SideEffectPolicyExternalRead,
				IdempotencyPolicy: IdempotencyPolicyIdempotent,
				WatchdogRecovery:  WatchdogRecoveryRetryExternal,
			},
			want: true,
		},
		{
			name: "external write dedupe key",
			policy: RuntimeRecoveryPolicy{
				SideEffectPolicy:  SideEffectPolicyExternalWrite,
				IdempotencyPolicy: IdempotencyPolicyDedupeKeyRequired,
				WatchdogRecovery:  WatchdogRecoveryRetryExternal,
				IdempotencyKey:    "request-001",
			},
			want: true,
		},
		{
			name: "workspace read idempotent is not external",
			policy: RuntimeRecoveryPolicy{
				SideEffectPolicy:  SideEffectPolicyWorkspaceRead,
				IdempotencyPolicy: IdempotencyPolicyIdempotent,
				WatchdogRecovery:  WatchdogRecoveryRetryExternal,
			},
			want: false,
		},
		{
			name: "none idempotent is not external",
			policy: RuntimeRecoveryPolicy{
				SideEffectPolicy:  SideEffectPolicyNone,
				IdempotencyPolicy: IdempotencyPolicyIdempotent,
				WatchdogRecovery:  WatchdogRecoveryRetryExternal,
			},
			want: false,
		},
		{
			name: "external dedupe key missing",
			policy: RuntimeRecoveryPolicy{
				SideEffectPolicy:  SideEffectPolicyExternalRead,
				IdempotencyPolicy: IdempotencyPolicyDedupeKeyRequired,
				WatchdogRecovery:  WatchdogRecoveryRetryExternal,
			},
			want: false,
		},
		{
			name: "external manual recovery",
			policy: RuntimeRecoveryPolicy{
				SideEffectPolicy:  SideEffectPolicyExternalRead,
				IdempotencyPolicy: IdempotencyPolicyIdempotent,
				WatchdogRecovery:  WatchdogRecoveryManualOnly,
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.policy.AllowsWatchdogExternalRetry(); got != tc.want {
				t.Fatalf("AllowsWatchdogExternalRetry() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRuntimeRecoveryPolicyWatchdogRecoverySafetyForNonExternalRetry(t *testing.T) {
	policy := RuntimeRecoveryPolicy{
		SideEffectPolicy:  SideEffectPolicyWorkspaceRead,
		IdempotencyPolicy: IdempotencyPolicyIdempotent,
		WatchdogRecovery:  WatchdogRecoveryRetryExternal,
	}
	if got := policy.WatchdogRecoverySafety(); got != "retry_external_not_external" {
		t.Fatalf("WatchdogRecoverySafety() = %q, want retry_external_not_external", got)
	}
}
