package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type QueryRuntimeCWIReconciliationCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	RequestID                     string
	Generation                    uint64
	DefinitionRevision            string
	BindingSetHash                string
	OwnerModuleID                 string
	Binding                       runtimecontract.RuntimeCWIReconciliationBindingRef
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	WorkspaceBaselineRevision     string
	WorkspaceBaselineRef          string
	ValidationRecipes             []runtimecontract.RuntimeValidationRecipe
}

type QueryRuntimeCWIReconciliationResult struct {
	Observation runtimecontract.RuntimeCWIReconciliationObservation
}

func (c QueryRuntimeCWIReconciliationCommand) ValidateForOwner(owner string) error {
	return runtimecontract.ValidateRuntimeCWIOwnerReconciliationRequest(runtimeCWIOwnerReconciliationRequest(c), owner)
}

func runtimeCWIOwnerReconciliationRequest(c QueryRuntimeCWIReconciliationCommand) runtimecontract.RuntimeCWIOwnerReconciliationRequest {
	return runtimecontract.RuntimeCWIOwnerReconciliationRequest{WorkspaceID: c.WorkspaceID, RunID: c.RunID, RequestID: c.RequestID, Generation: c.Generation, DefinitionRevision: c.DefinitionRevision, BindingSetHash: c.BindingSetHash, OwnerModuleID: c.OwnerModuleID, Binding: c.Binding, WorkspaceDeliverableDirectory: c.WorkspaceDeliverableDirectory, WorkspaceInternalDirectory: c.WorkspaceInternalDirectory, WorkspaceBaselineRevision: c.WorkspaceBaselineRevision, WorkspaceBaselineRef: c.WorkspaceBaselineRef, ValidationRecipes: c.ValidationRecipes}
}
