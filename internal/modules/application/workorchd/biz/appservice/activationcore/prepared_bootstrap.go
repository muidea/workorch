package activationcore

import (
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparedBootstrapCompilerVersion = preparationcompiler.PreparedBootstrapCompilerVersion

func CompilePreparedBootstrap(s startuppreparation.ExactConfirmedStartupSnapshot, m startuppreparation.Manifest, a runtimecontract.RuntimeActivationContext) (preparationcompiler.PreparedBootstrapCompilation, error) {
	return preparationcompiler.CompilePreparedBootstrap(s, m, a)
}
