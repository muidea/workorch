package definitionconfiguration

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeprojection"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateExecutionPolicy authenticates current derived policy against full
// owner inputs, independently of the historical business admission hash.
func ValidateExecutionPolicy(w *workspace.Workspace, t *workspace.WorkspaceTemplate, execution rc.RuntimeExecutionContract) error {
	current, err := Compile(w, t)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rc.NormalizeRuntimeProfileContracts(current.ProfileContracts), rc.NormalizeRuntimeProfileContracts(execution.ProfileContracts)) {
		return fmt.Errorf("%w: execution Profile differs from current owner declaration", common.ErrConflict)
	}
	candidate, present, err := w.ValidateActivationAdmissionPersistence()
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	count := 0
	for _, binding := range candidate.AdmissionPolicyBindings {
		if binding.PolicyRef != executionLimitsAdmissionRef {
			continue
		}
		count++
		if execution.ExecutionLimits == nil || binding.SnapshotHash != rc.StableRuntimeExecutionLimitsHash(*execution.ExecutionLimits) || binding.Revision != execution.ExecutionLimits.ProtocolRef {
			return fmt.Errorf("%w: execution budgets differ from confirmed business boundary", common.ErrConflict)
		}
	}
	if count > 1 || (execution.ExecutionLimits != nil && count != 1) {
		return fmt.Errorf("%w: execution budget source is missing or ambiguous", common.ErrConflict)
	}
	return nil
}

// ValidateExecutionActivation also authenticates the complete evidence-policy
// declaration. An Application projection cannot inject or omit current policy.
func ValidateExecutionActivation(w *workspace.Workspace, t *workspace.WorkspaceTemplate, activation rc.RuntimeActivationContext) error {
	if err := ValidateExecutionPolicy(w, t, activation.ExecutionContract); err != nil {
		return err
	}
	current, err := workspace.WorkspaceForCurrentTemplate(w, t)
	if err != nil {
		return err
	}
	expected, err := runtimeprojection.CompileRuntimeEvidencePolicies("Workspace", current.EvidencePolicies)
	if err != nil {
		return err
	}
	if rc.ValidateRuntimeEvidencePolicies(activation.EvidencePolicies) != nil || !rc.SameRuntimeEvidencePolicies(expected, activation.EvidencePolicies) {
		return fmt.Errorf("%w: execution evidence policy differs from current owner declaration", common.ErrConflict)
	}
	return nil
}

// ValidateExecutionBootstrap keeps the current method and original business
// source independent. A current policy never authenticates invented history.
func ValidateExecutionBootstrap(w *workspace.Workspace, t *workspace.WorkspaceTemplate, bootstrap rc.RuntimeBootstrap) error {
	if err := ValidateExecutionActivation(w, t, bootstrap.Activation); err != nil {
		return err
	}
	candidate, present, err := w.ValidateActivationAdmissionPersistence()
	if err != nil || !present {
		return err
	}
	expected, err := definitionadmission.CompileRuntimeSourceObligationInventory(candidate, *w.StartupInput, *w.ActivationAdmission)
	if err != nil {
		return err
	}
	if bootstrap.Definition.StartupInputHash != w.StartupInput.StartupInputHash || bootstrap.Definition.Revision != w.StartupInput.DefinitionRevision || !reflect.DeepEqual(expected, bootstrap.Definition.SourceObligations) || !reflect.DeepEqual(expected, bootstrap.DefinitionSeed.SourceObligations) {
		return fmt.Errorf("%w: execution source differs from confirmed business inventory", common.ErrConflict)
	}
	return nil
}
