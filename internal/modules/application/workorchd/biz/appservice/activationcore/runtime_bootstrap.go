package activationcore

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/bootstrapcompiler"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func bootstrapInputs(g *workspace.Goal, d *workspace.DoD) (*bootstrapcompiler.GoalInput, *bootstrapcompiler.DoDInput) {
	var goal *bootstrapcompiler.GoalInput
	var dod *bootstrapcompiler.DoDInput
	if g != nil {
		goal = &bootstrapcompiler.GoalInput{ID: g.ID, WorkspaceID: g.WorkspaceID, Text: g.Text}
	}
	if d != nil {
		dod = &bootstrapcompiler.DoDInput{ID: d.ID, WorkspaceID: d.WorkspaceID, Items: append([]string(nil), d.Items...)}
	}
	return goal, dod
}
func RuntimeBootstrap(g *workspace.Goal, d *workspace.DoD, r, h string, i *runtimecontract.RuntimeDefinitionChangeImpact, a runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	return RuntimeBootstrapWithSourceObligations(g, d, r, h, i, a, nil)
}
func RuntimeBootstrapWithSourceObligations(g *workspace.Goal, d *workspace.DoD, r, h string, i *runtimecontract.RuntimeDefinitionChangeImpact, a runtimecontract.RuntimeActivationContext, s *runtimecontract.RuntimeSourceObligationInventory) (*runtimecontract.RuntimeBootstrap, error) {
	goal, dod := bootstrapInputs(g, d)
	return bootstrapcompiler.RuntimeBootstrapWithSourceObligations(goal, dod, r, h, i, a, s)
}
func runtimeOperationAdmissibilityRequirements(k []string) ([]runtimecontract.RuntimeRequirement, error) {
	return bootstrapcompiler.OperationAdmissibilityRequirements(k)
}
func runtimeFocusCapabilityKinds(f []runtimecontract.CoverageFocus) ([]string, error) {
	return bootstrapcompiler.FocusCapabilityKinds(f)
}
