package server

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type CreateDefinitionAuthoringFunc func(context.Context, string, string, workorchmanagementcontract.DefinitionAuthoringCreate) (workorchmanagementcontract.DefinitionAuthoringView, error)
type ReadDefinitionAuthoringFunc func(context.Context, string) (workorchmanagementcontract.DefinitionAuthoringView, error)
type SaveDefinitionAuthoringFunc func(context.Context, string, workorchmanagementcontract.DefinitionAuthoringSave) (workorchmanagementcontract.DefinitionAuthoringView, error)
type ConfirmDefinitionAuthoringFunc func(context.Context, string, workorchmanagementcontract.DefinitionAuthoringConfirm) (workorchmanagementcontract.DefinitionAuthoringView, error)

type DefinitionAuthoringPorts struct {
	Create  CreateDefinitionAuthoringFunc
	Read    ReadDefinitionAuthoringFunc
	Save    SaveDefinitionAuthoringFunc
	Compile ReadDefinitionAuthoringFunc
	Confirm ConfirmDefinitionAuthoringFunc
}
