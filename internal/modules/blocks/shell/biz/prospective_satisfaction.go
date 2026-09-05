package shell

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellProspectiveValidationNotSatisfiedReason = "prospective workspace manifest does not make an activation-sealed Shell validation recipe selectable"

// handleRuntimeOwnerFactProspectiveSatisfaction is a pure Shell-owner query.
// It evaluates only activation-sealed recipes against the File-authored
// prospective target-ref value carried by the request. It does not call File,
// inspect the workspace, start a process, or mutate owner/Runtime state.
func (m *Biz) handleRuntimeOwnerFactProspectiveSatisfaction(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	request := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest(command)
	if _, err := common.BindRuntimeWorkspaceContext(event.Context(), request.WorkspaceID); err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := shellevents.ValidateRuntimeOwnerFactProspectiveSatisfactionCommand(command, ModuleID, modulecommon.ProspectiveSatisfactionContractRef); err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := validateShellProspectiveValidationRecipeIdentities(request); err != nil {
		setCapabilityError(result, err)
		return
	}
	if currentRevision := m.runtimeAdapter.runtimeAdmissibilityPolicyRevision(); request.OwnerCatalog.ObservedRevision != currentRevision {
		response, err := shellevents.NewRuntimeOwnerFactProspectiveSatisfactionStaleResult(
			command, currentRevision, "runtime Shell owner catalog is stale",
		)
		if err != nil {
			setCapabilityError(result, fmt.Errorf("%w: %v", common.ErrConflict, err))
			return
		}
		result.Set(response, nil)
		return
	}
	recipeRefs := shellProspectiveSelectableValidationRecipeRefs(request)
	outcome := runtimecontract.RuntimeOwnerFactProspectiveWouldSatisfy
	reason := ""
	if len(recipeRefs) == 0 {
		outcome = runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy
		reason = shellProspectiveValidationNotSatisfiedReason
	}
	response, err := shellevents.NewRuntimeOwnerFactProspectiveSatisfactionResult(command, outcome, recipeRefs, reason)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

// validateShellProspectiveValidationRecipeIdentities proves that every Shell
// prerequisite surface covered by the prospective-satisfaction contract still
// names the exact activation-sealed recipe from which the owner published its
// input schema. An ID-only match is insufficient: command, ordered args and
// timeout are all part of shellValidationRecipeSchemaHash. This is a pure
// fail-closed check and performs no workspace inspection or owner-state write.
func validateShellProspectiveValidationRecipeIdentities(request runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest) error {
	request = request.Normalized()
	constraint, err := runtimecontract.ResolveRuntimeProspectiveTargetSatisfactionConstraint(
		request.OwnerCatalog, request.Requirement, modulecommon.ProspectiveSatisfactionContractRef,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrConflict, err)
	}
	recipe, found := runtimecontract.RuntimeValidationRecipeForID(request.ValidationRecipes, constraint.ValidationRecipeID)
	if !found {
		return fmt.Errorf("%w: prospective Shell validation surface %q has no exact activation-sealed recipe", common.ErrConflict, constraint.ValidationRecipeID)
	}
	if schemaHash := shellValidationRecipeSchemaHash(recipe); schemaHash != constraint.InputSchemaHash {
		return fmt.Errorf("%w: prospective Shell validation surface %q recipe identity drift", common.ErrConflict, constraint.ValidationRecipeID)
	}
	if !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(
		request.ProspectiveWorkspaceManifestDelta.ProspectiveTargetRefs,
		constraint,
	) {
		return fmt.Errorf("%w: prospective Shell validation surface %q target violates its owner-authored all-of constraint", common.ErrConflict, constraint.ValidationRecipeID)
	}
	return nil
}

func shellProspectiveSelectableValidationRecipeRefs(request runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest) []string {
	request = request.Normalized()
	constraint, err := runtimecontract.ResolveRuntimeProspectiveTargetSatisfactionConstraint(
		request.OwnerCatalog, request.Requirement, modulecommon.ProspectiveSatisfactionContractRef,
	)
	if err != nil || !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(
		request.ProspectiveWorkspaceManifestDelta.ProspectiveTargetRefs, constraint,
	) {
		return nil
	}
	applicable := runtimeValidationRecipesApplicableToManifest(
		request.ValidationRecipes,
		request.ProspectiveWorkspaceManifestDelta.ResultingTargetRefs,
	)
	ret := make([]string, 0, len(applicable))
	for _, recipe := range applicable {
		if recipe.ID == constraint.ValidationRecipeID {
			ret = append(ret, recipe.ID)
		}
	}
	return ret
}
