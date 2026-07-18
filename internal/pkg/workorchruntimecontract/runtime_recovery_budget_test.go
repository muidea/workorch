package workorchruntimecontract

import "testing"

func TestRuntimeRecoveryBudgetsAreIndependentAndBounded(t *testing.T) {
	policy := RuntimeRecoveryPolicy{
		MaxProviderTransportAttempts: 1, MaxOperationRecoveryAttempts: 2,
		MaxCASReloadAttempts: 3, MaxTerminalDeliveryAttempts: 4,
	}
	ledger := RuntimeRecoveryLedger{}
	var consumed bool
	var err error
	ledger, consumed, err = ledger.Consume(RuntimeRecoveryBudgetProviderTransport, policy)
	if err != nil || !consumed || ledger.ProviderTransportAttempts != 1 || ledger.OperationRecoveryAttempts != 0 {
		t.Fatalf("provider budget consume = %+v consumed=%v err=%v", ledger, consumed, err)
	}
	unchanged, consumed, err := ledger.Consume(RuntimeRecoveryBudgetProviderTransport, policy)
	if err != nil || consumed || unchanged != ledger {
		t.Fatalf("exhausted provider budget = %+v consumed=%v err=%v", unchanged, consumed, err)
	}
	ledger, consumed, err = ledger.Consume(RuntimeRecoveryBudgetOperationRecovery, policy)
	if err != nil || !consumed || ledger.OperationRecoveryAttempts != 1 || ledger.ProviderTransportAttempts != 1 {
		t.Fatalf("operation budget must be independent: %+v consumed=%v err=%v", ledger, consumed, err)
	}
}
