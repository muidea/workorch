package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeValidationExecutionUse is a use of one fresh owner execution, not a
// cached PASS or another operation receipt. Consumers are fixed before I/O;
// each retains its complete native observation and independent behavior stage.
type RuntimeValidationExecutionUse struct {
	GroupID             string
	SourceReceiptHash   string
	SourceScopeHash     string
	ConsumerBindingRefs []string
	SharedStageCount    int
	Validation          RuntimeValidationObservation
}

const RuntimeValidationExecutionMaxConsumers = 8

// Group identity contains the original request and exact consumer bindings.
// It is not an identity for reusable historical execution results.
func RuntimeValidationExecutionGroupID(workspace common.WorkspaceID, run common.RunID, requestID string, generation uint64, bindingSetHash string, bindings []RuntimeCWIReconciliationBindingRef) string {
	return stableFormalValueHash("runtime-validation-execution-group", struct {
		Workspace      common.WorkspaceID
		Run            common.RunID
		RequestID      string
		Generation     uint64
		BindingSetHash string
		Bindings       []RuntimeCWIReconciliationBindingRef
	}{workspace, run, requestID, generation, bindingSetHash, bindings})
}

func RuntimeValidationExecutionScopeHash(scope []string, recipes []RuntimeValidationRecipe) string {
	return stableFormalValueHash("runtime-validation-execution-scope", struct {
		Scope   []string
		Recipes []RuntimeValidationRecipe
	}{scope, recipes})
}

func (u RuntimeValidationExecutionUse) Clone() RuntimeValidationExecutionUse {
	u.ConsumerBindingRefs = slices.Clone(u.ConsumerBindingRefs)
	u.Validation = cloneRuntimeValidationObservationRaw(u.Validation)
	return u
}

func (u RuntimeValidationExecutionUse) ValidateForRequest(request RuntimeCWIReconciliationRequest, binding RuntimeCWIReconciliationBindingRef) error {
	if len(u.ConsumerBindingRefs) < 2 || len(u.ConsumerBindingRefs) > RuntimeValidationExecutionMaxConsumers || !reflect.DeepEqual(u.ConsumerBindingRefs, normalizeStrings(u.ConsumerBindingRefs)) || !slices.Contains(u.ConsumerBindingRefs, binding.BindingID) {
		return fmt.Errorf("validation execution use requires exact unique consumers")
	}
	var bindings []RuntimeCWIReconciliationBindingRef
	for _, ref := range u.ConsumerBindingRefs {
		found := false
		for _, b := range request.Bindings {
			if b.BindingID == ref {
				bindings = append(bindings, b.Clone())
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("validation execution use has foreign consumer")
		}
	}
	if u.GroupID != RuntimeValidationExecutionGroupID(request.WorkspaceID, request.RunID, request.RequestID, request.Generation, request.BindingSetHash, bindings) {
		return fmt.Errorf("validation execution group request or consumer identity mismatch")
	}
	if !boundedValidationExecutionRef(u.SourceReceiptHash, 1024) || !boundedValidationExecutionRef(u.SourceScopeHash, 1024) {
		return fmt.Errorf("validation execution use lacks original accepted receipt and scope")
	}
	c := u.Validation.ExecutionConjunction
	if c == nil || c.Validate() != nil || u.SharedStageCount <= 0 || u.SharedStageCount >= len(c.Stages) {
		return fmt.Errorf("validation execution use lacks original ordered conjunction")
	}
	for _, stage := range c.Stages[:u.SharedStageCount] {
		if stage.Identity.RequireBehavior {
			return fmt.Errorf("independent behavior cannot be a shared stage")
		}
	}
	for _, stage := range c.Stages {
		if stage.Stdout.Storage == nil || stage.Stderr.Storage == nil || stage.Stdout.Storage.Reference == "" || stage.Stderr.Storage.Reference == "" {
			return fmt.Errorf("validation execution use lacks original saved stage output")
		}
	}
	if len(binding.ValidationRecipeIDs) != 1 || len(binding.RequiredCheckRefs) == 0 || len(binding.ValidationCheckAttestations) == 0 || u.Validation.RecipeID != binding.ValidationRecipeIDs[0] {
		return fmt.Errorf("validation execution use recipe mismatch")
	}
	return u.Validation.ValidateFailureDetails()
}

// ValidateRuntimeValidationExecutionUses closes the complete fan-out. Every
// consumer must retain identical actual shared facts, including original
// timestamps/output references, and its own exact native/behavior observation.
func ValidateRuntimeValidationExecutionUses(request RuntimeCWIReconciliationRequest, observations []RuntimeCWIReconciliationObservation) error {
	for _, observation := range observations {
		if err := observation.ValidateForRequest(request); err != nil {
			return err
		}
	}
	for _, observation := range observations {
		u := observation.ValidationExecution
		if u == nil {
			continue
		}
		for _, ref := range u.ConsumerBindingRefs {
			var peer *RuntimeCWIReconciliationObservation
			for i := range observations {
				if observations[i].BindingID == ref && observations[i].OwnerModuleID == observation.OwnerModuleID {
					peer = &observations[i]
					break
				}
			}
			if peer == nil || peer.ValidationExecution == nil {
				return fmt.Errorf("validation execution group is missing a consumer")
			}
			p := peer.ValidationExecution
			if p.GroupID != u.GroupID || p.SharedStageCount != u.SharedStageCount || p.Validation.RecipeID != u.Validation.RecipeID || p.Validation.ObservationProtocolRef != u.Validation.ObservationProtocolRef || !reflect.DeepEqual(p.ConsumerBindingRefs, u.ConsumerBindingRefs) ||
				!reflect.DeepEqual(p.Validation.ExecutionConjunction.Stages[:p.SharedStageCount], u.Validation.ExecutionConjunction.Stages[:u.SharedStageCount]) {
				return fmt.Errorf("validation execution group changed original shared execution")
			}
		}
	}
	return nil
}
