package activationports

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
)

const OriginalPreparedBootstrapVersion = preparationcompiler.OriginalPreparedBootstrapVersion

func CompileOriginalPreparedBootstrap(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (preparationcompiler.OriginalPreparedBootstrap, error) {
	return preparationcompiler.CompileOriginalPreparedBootstrap(w, a)
}
