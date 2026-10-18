package server

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type ReadDefinitionAuthoringSuccessAuditFunc func(context.Context, string, string) (workorchmanagementcontract.DefinitionAuthoringSuccessAuditView, error)
type ReadDefinitionAuthoringDiagnosticFunc func(context.Context, string, string) (workorchmanagementcontract.DefinitionAuthoringDiagnosticView, error)
type ReadDefinitionAuthoringPreparationDiagnosticFunc func(context.Context, string, string) (workorchmanagementcontract.DefinitionAuthoringPreparationDiagnosticView, error)

type CreateDefinitionAuthoringFunc func(context.Context, string, string, workorchmanagementcontract.DefinitionAuthoringCreate) (workorchmanagementcontract.DefinitionAuthoringView, error)
type ReadDefinitionAuthoringFunc func(context.Context, string) (workorchmanagementcontract.DefinitionAuthoringView, error)
type CompileDefinitionAuthoringFunc func(context.Context, string, workorchmanagementcontract.DefinitionAuthoringCompile) (workorchmanagementcontract.DefinitionAuthoringView, error)
type SaveDefinitionAuthoringFunc func(context.Context, string, workorchmanagementcontract.DefinitionAuthoringSave) (workorchmanagementcontract.DefinitionAuthoringView, error)
type ConfirmDefinitionAuthoringFunc func(context.Context, string, workorchmanagementcontract.DefinitionAuthoringConfirm) (workorchmanagementcontract.DefinitionAuthoringView, error)

type DefinitionAuthoringPorts struct {
	Create                    CreateDefinitionAuthoringFunc
	Read                      ReadDefinitionAuthoringFunc
	Save                      SaveDefinitionAuthoringFunc
	Compile                   CompileDefinitionAuthoringFunc
	Confirm                   ConfirmDefinitionAuthoringFunc
	ReadSuccessAudit          ReadDefinitionAuthoringSuccessAuditFunc
	ReadDiagnostic            ReadDefinitionAuthoringDiagnosticFunc
	ReadPreparationDiagnostic ReadDefinitionAuthoringPreparationDiagnosticFunc
}
