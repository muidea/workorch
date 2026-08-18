package events

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest
type AcquireRuntimeCapabilitiesResult struct {
	Acquisition runtimecontract.RuntimeCapabilityAcquisition
}

func ResolveRuntimeCapabilities(command AcquireRuntimeCapabilitiesCommand, ownerModuleID string, supportedKinds ...string) (AcquireRuntimeCapabilitiesResult, error) {
	resolved, err := runtimecontract.ResolveRuntimeCapabilityAcquisitionWithPrerequisites(
		runtimecontract.RuntimeCapabilityAcquisitionRequest(command),
		ownerModuleID,
		[]runtimecontract.RuntimeCapabilityPrerequisiteFact{{
			ID:            "shell/managed-validation/workspace-manifest/v1",
			OwnerModuleID: ownerModuleID,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Requirement: runtimecontract.RuntimeRequirement{
				Kind:          runtimecontract.PrerequisiteKindManifest,
				OwnerModuleID: runtimecontract.RuntimeModuleFile,
				Reason:        "managed validation catalog requires the File owner workspace manifest",
			},
		}},
		supportedKinds...,
	)
	return AcquireRuntimeCapabilitiesResult{Acquisition: resolved}, err
}
