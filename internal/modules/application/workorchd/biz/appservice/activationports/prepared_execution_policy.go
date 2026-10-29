package activationports

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
)

func CompileOriginalPreparedExecutionPolicy(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (preparationcompiler.PreparedExecutionPolicy, error) {
	return preparationcompiler.CompileOriginalPreparedExecutionPolicy(w, a)
}
func CompilePreparedExecutionPolicy(s startuppreparation.ExactConfirmedStartupSnapshot, m startuppreparation.Manifest, i workspace.PreparationExecutionInputs) (preparationcompiler.PreparedExecutionPolicy, error) {
	return preparationcompiler.CompilePreparedExecutionPolicy(s, m, i)
}
