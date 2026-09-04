package workorchruntimecontract

import "testing"

func TestRuntimeRecoveryLedgerRejectsSplitCASReloadCounter(t *testing.T) {
	ledger := RuntimeRecoveryLedger{CASReloadAttempts: 1}
	if err := ledger.Validate(DefaultRuntimeRecoveryPolicy()); err == nil {
		t.Fatal("CAS reload counter without its atomic nonterminal recovery record was accepted")
	}
}

func TestRuntimeRecoveryLedgerRejectsEveryPartialRetiredLeaseMarker(t *testing.T) {
	policy := DefaultRuntimeRecoveryPolicy()
	for _, testCase := range []struct {
		name  string
		lease RuntimeDriveLease
	}{
		{name: "workspace", lease: RuntimeDriveLease{WorkspaceID: "ws-v2"}},
		{name: "run", lease: RuntimeDriveLease{RunID: "run-v2"}},
		{name: "aggregate revision", lease: RuntimeDriveLease{AggregateRevision: 1}},
		{name: "generation", lease: RuntimeDriveLease{RecoveryGeneration: 1}},
		{name: "reason", lease: RuntimeDriveLease{Reason: RuntimeDriveLeaseReasonDeliveryRetry}},
		{name: "signature", lease: RuntimeDriveLease{FailureSignature: "terminal-delivery"}},
		{name: "recovery request", lease: RuntimeDriveLease{RecoveryRequestID: "terminal-request"}},
		{name: "due at", lease: RuntimeDriveLease{DueAt: "2026-09-04T00:00:00Z"}},
		{name: "state", lease: RuntimeDriveLease{State: RuntimeDriveLeaseRunning}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ledger := RuntimeRecoveryLedger{RetiredTerminalDeliveryLease: testCase.lease}
			if err := ledger.Validate(policy); err == nil {
				t.Fatalf("partial retired terminal-delivery lease was treated as absent: %+v", testCase.lease)
			}
		})
	}
}

func TestRuntimeRecoveryLedgerRejectsTerminalRecoveryAuthorityAsRetiredPreTerminalLease(t *testing.T) {
	policy := DefaultRuntimeRecoveryPolicy()
	base := RuntimeDriveLease{
		WorkspaceID:        "ws-v2",
		RunID:              "run-v2",
		AggregateRevision:  7,
		RecoveryGeneration: 3,
		Reason:             RuntimeDriveLeaseReasonDeliveryRetry,
		FailureSignature:   "terminal-delivery",
		DueAt:              "2026-09-04T00:00:00Z",
		State:              RuntimeDriveLeaseRunning,
	}
	for _, testCase := range []struct {
		name  string
		lease RuntimeDriveLease
	}{
		{
			name:  "canonical terminal delivery signature",
			lease: base,
		},
		{
			name: "terminal recovery request binding",
			lease: func() RuntimeDriveLease {
				lease := base
				lease.RecoveryRequestID = "terminal-request"
				return lease
			}(),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ledger := RuntimeRecoveryLedger{RetiredTerminalDeliveryLease: testCase.lease}
			if err := ledger.Validate(policy); err == nil {
				t.Fatalf("terminal recovery authority was accepted as retired pre-terminal lease: %+v", testCase.lease)
			}
		})
	}
}
