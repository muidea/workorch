package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ProbeRuntimeOperationFeasibility validates a concrete shell operation without
// starting a process. It deliberately shares the execution policy helpers so
// candidate admission and owner execution cannot disagree on basic viability.
func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibility(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation) *runtimemodel.FailureFact {
	failure, _ := a.ProbeRuntimeOperationFeasibilityWithObservation(workspaceDirectory, inventory, operation, nil)
	return failure
}

func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibilityWithObservation(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation, observation *runtimemodel.RuntimePrerequisiteObservation) (*runtimemodel.FailureFact, *runtimemodel.RuntimePrerequisiteStale) {
	return a.ProbeRuntimeOperationFeasibilityWithObservationAndValidationRecipes(workspaceDirectory, inventory, operation, observation, nil)
}

// ProbeRuntimeOperationFeasibilityWithObservationAndValidationRecipes is the
// owner-side preflight for a sealed managed-validation selection. The manifest
// passes exact recipes here; this method never reinterprets profile text or
// accepts a command supplied by the candidate.
func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibilityWithObservationAndValidationRecipes(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation, observation *runtimemodel.RuntimePrerequisiteObservation, recipes []runtimemodel.RuntimeValidationRecipe) (*runtimemodel.FailureFact, *runtimemodel.RuntimePrerequisiteStale) {
	operation = operation.Normalized()
	inventory = inventory.Normalized()
	request, err := shellProbeRequestFromOperation(workspaceDirectory, operation, recipes)
	if err != nil {
		code := "shell_operation_invalid"
		if operation.Kind == runtimemodel.RuntimeBlockKindManagedValidationSession {
			code = "validation_recipe_unavailable"
		}
		return shellOperationInfeasible(operation, code, err), nil
	}
	command, args, err := shellCommandSpec(request.Command, request.Args)
	if err != nil {
		return shellOperationInfeasible(operation, "shell_command_invalid", err), nil
	}
	directory, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
	if err != nil || directory == "" {
		return shellOperationInfeasible(operation, "shell_working_directory_invalid", common.ErrInvalidArgument), nil
	}
	info, statErr := os.Stat(directory)
	if statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = common.ErrInvalidArgument
		}
		return shellOperationInfeasible(operation, "shell_working_directory_unavailable", statErr), nil
	}
	if inventory.ObservedRevision != a.runtimeAdmissibilityPolicyRevision() {
		if observation != nil {
			value := observation.Normalized()
			for _, dependency := range value.Dependencies {
				if dependency.Domain == runtimemodel.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == runtimemodel.RuntimeModuleShell && dependency.SubjectRef == "shell_runtime_policy" && dependency.Revision == inventory.ObservedRevision {
					return nil, &runtimemodel.RuntimePrerequisiteStale{PrerequisiteID: value.PrerequisiteID, ObservationID: value.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: a.runtimeAdmissibilityPolicyRevision(), Reason: "runtime shell operation policy changed"}
				}
			}
		}
		return shellOperationInfeasible(operation, "operation_admissibility_stale", fmt.Errorf("runtime shell operation admissibility policy is stale")), nil
	}
	if observation != nil {
		for _, dependency := range observation.Normalized().Dependencies {
			// The Shell owner observes this target, but File is the owner that
			// reports durable workspace_target revisions. The dependency must use
			// the resource owner so a File receipt intersects it exactly.
			if dependency.Domain != runtimemodel.RuntimeResourceWorkspaceTarget || dependency.OwnerModuleID != runtimemodel.RuntimeModuleFile {
				continue
			}
			if observedRevision, ok := shellWorkspaceTargetRevision(directory, dependency.SubjectRef); !ok || observedRevision != dependency.Revision {
				if !ok {
					observedRevision = "missing"
				}
				return nil, &runtimemodel.RuntimePrerequisiteStale{PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime shell validation target changed after admissibility acquisition"}
			}
		}
	}
	execCommand, execArgs, _ := normalizeShellExecutionCommand(command, args)
	if _, lookErr := exec.LookPath(execCommand); lookErr != nil {
		return shellOperationInfeasible(operation, "shell_executable_unavailable", lookErr), nil
	}
	if err := validateShellCommandPolicy(execCommand, execArgs, directory, shellInputHasManagedValidation(request)); err != nil {
		return shellOperationInfeasible(operation, "shell_command_policy_rejected", err), nil
	}
	if err := validateValidationShellEvidencePolicy(request.ValidationEvidence, execCommand, execArgs); err != nil {
		return shellOperationInfeasible(operation, "shell_validation_contract_rejected", err), nil
	}
	if _, err := shellCommandTimeout(request.TimeoutMillis, a.timeout); err != nil {
		return shellOperationInfeasible(operation, "shell_command_timeout_policy_rejected", err), nil
	}
	if err := validateShellValidationPreflight(directory, execCommand, execArgs, request); err != nil {
		return shellOperationInfeasible(operation, "validation_target_missing", err), nil
	}
	return nil, nil
}

func shellProbeRequestFromOperation(workspaceDirectory string, operation runtimemodel.RuntimeOperation, recipes []runtimemodel.RuntimeValidationRecipe) (RuntimeCommandRequest, error) {
	if operation.Kind != runtimemodel.StepKindShellCommand && operation.Kind != runtimemodel.RuntimeBlockKindManagedValidationSession {
		return RuntimeCommandRequest{}, common.ErrInvalidArgument
	}
	request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspaceDirectory}
	if operation.Kind == runtimemodel.RuntimeBlockKindManagedValidationSession {
		request.ManagedValidation = runtimemodel.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimemodel.RuntimeBlockKindManagedValidationSession, OriginalNodeKind: runtimemodel.RuntimeBlockKindManagedValidationSession}
	}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "recipeId":
			if parameter.String != nil {
				request.ValidationRecipeID = strings.TrimSpace(*parameter.String)
			}
		case "command":
			if parameter.String != nil {
				request.Command = *parameter.String
			}
		case "args":
			request.Args = append([]string(nil), parameter.StringList...)
		case "timeoutMillis":
			if parameter.Integer != nil {
				request.TimeoutMillis = *parameter.Integer
			}
		case "stateChanging":
			if parameter.Boolean != nil {
				request.StateChanging = *parameter.Boolean
			}
		}
	}
	if operation.Kind == runtimemodel.RuntimeBlockKindManagedValidationSession {
		recipe, found := runtimemodel.RuntimeValidationRecipeForID(recipes, request.ValidationRecipeID)
		if !found {
			return RuntimeCommandRequest{}, fmt.Errorf("validation recipe %q is unavailable", request.ValidationRecipeID)
		}
		request.Command = recipe.Command
		request.Args = append([]string(nil), recipe.Args...)
		request.TimeoutMillis = recipe.TimeoutMillis
	}
	request.TargetRefs = append([]string(nil), operation.TargetRefs...)
	if len(request.TargetRefs) == 1 {
		request.TargetRef = request.TargetRefs[0]
	}
	return request, nil
}

func shellOperationInfeasible(operation runtimemodel.RuntimeOperation, code string, err error) *runtimemodel.FailureFact {
	message := "shell operation is not currently feasible"
	if err != nil {
		message = err.Error()
	}
	failure := &runtimemodel.FailureFact{
		Class: runtimemodel.FailureClassCandidateContractFailure,
		Code:  code, Signature: "operation-admissibility/" + operation.Kind + "/" + code,
		Message: message, TargetRefs: append([]string(nil), operation.TargetRefs...),
	}
	class := ""
	targetType := ""
	switch strings.TrimSpace(code) {
	case "shell_executable_unavailable", "shell_working_directory_unavailable":
		class = runtimemodel.ValidationDiagnosticEnvironmentMissing
		targetType = runtimemodel.ValidationEvidenceTargetEnvironment
	case "shell_command_invalid", "shell_command_policy_rejected", "shell_validation_contract_rejected", "shell_command_timeout_policy_rejected", "validation_target_missing":
		class = runtimemodel.ValidationDiagnosticCommandInvalid
		targetType = runtimemodel.ValidationEvidenceTargetCommandContract
	}
	if class != "" {
		commandKind := ""
		for _, parameter := range operation.Parameters {
			if parameter.Name == "command" && parameter.String != nil {
				commandKind = filepath.Base(strings.TrimSpace(*parameter.String))
				break
			}
		}
		failure.ValidationDiagnostic = &runtimemodel.ValidationFailureDiagnostic{
			SchemaVersion: runtimemodel.ValidationFailureDiagnosticSchemaVersion,
			Class:         class, Reason: message, RecommendedEvidenceTargetType: targetType,
			Stage: "shell_validation_preflight_failed", Operation: operation.Kind, CommandKind: commandKind,
			CommandPolicyViolation: class == runtimemodel.ValidationDiagnosticCommandInvalid,
			TargetRefs:             append([]string(nil), operation.TargetRefs...),
		}
	}
	return failure
}
