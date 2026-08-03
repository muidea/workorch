package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type AcquireRuntimeCapabilitiesCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	PrerequisiteID      string
	OwnerModuleID       string
	CapabilityClass     runtimecontract.RuntimeCapabilityClass
	RequiredKinds       []string
}

type AcquireRuntimeCapabilitiesResult struct {
	Acquisition runtimecontract.RuntimeCapabilityAcquisition
}

func ResolveRuntimeCapabilities(command AcquireRuntimeCapabilitiesCommand, ownerModuleID string, supportedKinds ...string) (AcquireRuntimeCapabilitiesResult, error) {
	resolved, err := runtimecontract.ResolveRuntimeCapabilityAcquisition(runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
		PrerequisiteID: command.PrerequisiteID, OwnerModuleID: command.OwnerModuleID, CapabilityClass: command.CapabilityClass, RequiredKinds: command.RequiredKinds,
	}, ownerModuleID, supportedKinds...)
	return AcquireRuntimeCapabilitiesResult{Acquisition: resolved}, err
}
