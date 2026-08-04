package events

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest
type AcquireRuntimeCapabilitiesResult struct {
	Acquisition runtimecontract.RuntimeCapabilityAcquisition
}

func ResolveRuntimeCapabilities(command AcquireRuntimeCapabilitiesCommand, ownerModuleID string, supportedKinds ...string) (AcquireRuntimeCapabilitiesResult, error) {
	resolved, err := runtimecontract.ResolveRuntimeCapabilityAcquisition(runtimecontract.RuntimeCapabilityAcquisitionRequest(command), ownerModuleID, supportedKinds...)
	return AcquireRuntimeCapabilitiesResult{Acquisition: resolved}, err
}
