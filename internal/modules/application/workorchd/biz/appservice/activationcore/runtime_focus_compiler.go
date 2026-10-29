package activationcore

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/bootstrapcompiler"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func compileRuntimeCoverageFocuses(g *workspace.Goal, d *workspace.DoD, a runtimecontract.RuntimeActivationContext) ([]runtimecontract.CoverageFocus, error) {
	goal, dod := bootstrapInputs(g, d)
	return bootstrapcompiler.CompileCoverageFocuses(goal, dod, a)
}
func compileRuntimeCoverage(g *workspace.Goal, d *workspace.DoD, a runtimecontract.RuntimeActivationContext) (bootstrapcompiler.CoverageCompilation, error) {
	goal, dod := bootstrapInputs(g, d)
	return bootstrapcompiler.CompileCoverage(goal, dod, a)
}
func resolveCoverageEvidencePolicy(s, i string, a runtimecontract.RuntimeActivationContext, b bool) (runtimecontract.RuntimeSourceEvidencePolicy, error) {
	return bootstrapcompiler.ResolveEvidencePolicy(s, i, a, b)
}
