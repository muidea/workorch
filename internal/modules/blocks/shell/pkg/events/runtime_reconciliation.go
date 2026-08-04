package events

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type QueryRuntimeCWIReconciliationCommand runtimecontract.RuntimeCWIOwnerReconciliationRequest

type QueryRuntimeCWIReconciliationResult struct {
	Observation runtimecontract.RuntimeCWIReconciliationObservation
}

func (c QueryRuntimeCWIReconciliationCommand) ValidateForOwner(owner string) error {
	return runtimecontract.ValidateRuntimeCWIOwnerReconciliationRequest(runtimecontract.RuntimeCWIOwnerReconciliationRequest(c), owner)
}
