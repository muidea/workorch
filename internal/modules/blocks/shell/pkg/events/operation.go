package events

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeExecutionSealInput contains the persisted, detached facts that may
// influence Shell execution but are not fields of RuntimeOperation. The
// Orchestrator must populate it only from its sealed Runtime aggregate.
type RuntimeExecutionSealInput struct {
	CatalogObservation        runtimecontract.RuntimePrerequisiteObservation
	TargetRole                string
	ValidationScopeTargetRefs []string
	EvidencePolicies          []runtimecontract.RuntimeEvidencePolicy
	// ValidationProofBinding is selected by Orchestrator from the exact
	// CoverageEvidenceBinding and the sealed Shell owner catalog. Shell may
	// consume this mapping but must never infer it from command parameters or
	// process output.
	ValidationProofBinding runtimecontract.RuntimeValidationProofRequirementBinding
}

// RuntimeExecutionAuthorization binds one Shell operation to the exact owner
// catalog used by SM-2/SM-3 and to every detached value consumed by the Shell
// owner. It contains no process handle or mutable owner resource.
type RuntimeExecutionAuthorization struct {
	CatalogSnapshotID         string
	AcquisitionBinding        string
	PrerequisiteID            string
	OwnerRevision             string
	CatalogObservation        runtimecontract.RuntimePrerequisiteObservation
	Policy                    runtimecontract.RuntimeOperationPolicyFact
	TargetRole                string
	ValidationRecipes         []runtimecontract.RuntimeValidationRecipe
	ValidationScopeTargetRefs []string
	EvidencePolicies          []runtimecontract.RuntimeEvidencePolicy
	ValidationProofBinding    runtimecontract.RuntimeValidationProofRequirementBinding
}

func (a RuntimeExecutionAuthorization) normalized() RuntimeExecutionAuthorization {
	a.CatalogSnapshotID = strings.TrimSpace(a.CatalogSnapshotID)
	a.AcquisitionBinding = strings.TrimSpace(a.AcquisitionBinding)
	a.PrerequisiteID = strings.TrimSpace(a.PrerequisiteID)
	a.OwnerRevision = strings.TrimSpace(a.OwnerRevision)
	a.CatalogObservation = a.CatalogObservation.Normalized()
	a.Policy = a.Policy.Normalized()
	a.TargetRole = strings.TrimSpace(a.TargetRole)
	a.ValidationRecipes = runtimecontract.NormalizeRuntimeValidationRecipes(a.ValidationRecipes)
	a.ValidationScopeTargetRefs = normalizeExecutionStrings(a.ValidationScopeTargetRefs)
	a.EvidencePolicies = runtimecontract.NormalizeRuntimeEvidencePolicies(a.EvidencePolicies)
	a.ValidationProofBinding = a.ValidationProofBinding.Clone()
	return a
}

func (a RuntimeExecutionAuthorization) validateForOperation(operation runtimecontract.RuntimeOperation) error {
	if err := runtimecontract.ValidateRuntimeEvidencePolicies(a.EvidencePolicies); err != nil {
		return fmt.Errorf("sealed Shell evidence policies: %w", err)
	}
	a = a.normalized()
	if a.CatalogSnapshotID == "" || a.PrerequisiteID == "" || a.OwnerRevision == "" {
		return fmt.Errorf("sealed Shell authorization requires catalog identity and revision")
	}
	if err := a.CatalogObservation.Validate(); err != nil {
		return fmt.Errorf("sealed Shell catalog observation: %w", err)
	}
	if a.CatalogObservation.PrerequisiteID != a.PrerequisiteID ||
		a.CatalogObservation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility ||
		a.CatalogObservation.OwnerModuleID != runtimecontract.RuntimeModuleShell ||
		a.CatalogObservation.ContentHash != a.CatalogSnapshotID ||
		!observationHasOwnerRevision(a.CatalogObservation, runtimecontract.RuntimeResourceOwnerCatalog, "shell_runtime_policy", a.OwnerRevision) {
		return fmt.Errorf("sealed Shell catalog observation does not match authorization")
	}
	if err := a.Policy.Validate(); err != nil {
		return fmt.Errorf("sealed Shell operation policy: %w", err)
	}
	matched, found := runtimecontract.RuntimeOperationPolicyForOperation(operation, []runtimecontract.RuntimeOperationPolicyFact{a.Policy})
	if !found || matched.ID != a.Policy.ID || a.Policy.OwnerModuleID != runtimecontract.RuntimeModuleShell {
		return fmt.Errorf("sealed Shell operation policy does not select the operation")
	}
	data, err := OperationDataFromRuntimeOperation(operation)
	if err != nil {
		return err
	}
	if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
		if _, found := runtimecontract.RuntimeValidationRecipeForID(a.ValidationRecipes, data.ValidationRecipeID); !found {
			return fmt.Errorf("sealed Shell managed-validation recipe is absent from owner catalog")
		}
		if err := a.ValidationProofBinding.Validate(); err != nil {
			return fmt.Errorf("sealed Shell managed-validation proof binding: %w", err)
		}
		if a.ValidationProofBinding.ValidationRecipeID != data.ValidationRecipeID || len(operation.BindingRefs) != 1 {
			return fmt.Errorf("sealed Shell managed-validation proof binding does not match operation")
		}
	} else if len(a.ValidationScopeTargetRefs) != 0 {
		return fmt.Errorf("sealed shell/command must not carry managed-validation target scope")
	} else if !runtimeValidationProofBindingEmpty(a.ValidationProofBinding) {
		return fmt.Errorf("sealed shell/command must not carry managed-validation proof binding")
	}
	return nil
}

// SealedRuntimeOperation is the complete Shell owner payload. Unlike the
// retired ExecuteOperationCommand.Data shape, no independently mutable
// command, recipe, diagnostic scope or target role crosses EventHub.
type SealedRuntimeOperation struct {
	Operation     runtimecontract.RuntimeOperation
	Authorization RuntimeExecutionAuthorization
	PayloadHash   string
}

func SealRuntimeOperation(operation runtimecontract.RuntimeOperation, catalog runtimecontract.RuntimeOperationAdmissibility, input RuntimeExecutionSealInput) (SealedRuntimeOperation, error) {
	if err := runtimecontract.ValidateRuntimeEvidencePolicies(input.EvidencePolicies); err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("Shell execution evidence policies: %w", err)
	}
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindShellCommand && operation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return SealedRuntimeOperation{}, fmt.Errorf("runtime operation kind %q is not owned by Shell", operation.Kind)
	}
	if runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleShell {
		return SealedRuntimeOperation{}, fmt.Errorf("runtime operation kind %q is not owned by Shell", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return SealedRuntimeOperation{}, err
	}
	catalog = catalog.Normalized()
	if err := catalog.Validate(); err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("Shell owner catalog: %w", err)
	}
	if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell || !containsOperationKind(catalog.OperationKinds, operation.Kind) {
		return SealedRuntimeOperation{}, fmt.Errorf("Shell owner catalog does not contain operation kind %q", operation.Kind)
	}
	policy, found := runtimecontract.RuntimeOperationPolicyForOperation(operation, catalog.OperationPolicies)
	if !found {
		return SealedRuntimeOperation{}, fmt.Errorf("Shell owner catalog has no exact operation policy")
	}
	authorization := RuntimeExecutionAuthorization{
		CatalogSnapshotID:         catalog.SnapshotID,
		AcquisitionBinding:        catalog.AcquisitionBinding,
		PrerequisiteID:            catalog.PrerequisiteID,
		OwnerRevision:             catalog.ObservedRevision,
		CatalogObservation:        input.CatalogObservation,
		Policy:                    policy,
		TargetRole:                input.TargetRole,
		ValidationRecipes:         catalog.ValidationRecipes,
		ValidationScopeTargetRefs: input.ValidationScopeTargetRefs,
		EvidencePolicies:          input.EvidencePolicies,
		ValidationProofBinding:    input.ValidationProofBinding,
	}.normalized()
	if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession && !catalogContainsValidationProofBinding(catalog, authorization.ValidationProofBinding) {
		return SealedRuntimeOperation{}, fmt.Errorf("Shell owner catalog does not contain selected validation proof binding")
	}
	if err := authorization.validateForOperation(operation); err != nil {
		return SealedRuntimeOperation{}, err
	}
	payload, detachedOperation, detachedAuthorization, err := sealedRuntimeOperationPayload(operation, authorization)
	if err != nil {
		return SealedRuntimeOperation{}, err
	}
	sum := sha256.Sum256(payload)
	return SealedRuntimeOperation{Operation: detachedOperation, Authorization: detachedAuthorization, PayloadHash: hex.EncodeToString(sum[:])}, nil
}

func runtimeValidationProofBindingEmpty(binding runtimecontract.RuntimeValidationProofRequirementBinding) bool {
	return binding.RequirementHash == "" && binding.DefinitionRevision == "" && binding.CoverageWorkItemID == "" &&
		binding.DeclarationRef == "" && binding.ValidationRecipeID == "" && len(binding.RequiredCheckRefs) == 0 && len(binding.CheckAttestations) == 0
}

func catalogContainsValidationProofBinding(catalog runtimecontract.RuntimeOperationAdmissibility, selected runtimecontract.RuntimeValidationProofRequirementBinding) bool {
	selected = selected.Clone()
	if selected.Validate() != nil {
		return false
	}
	surface, found := runtimecontract.RuntimeValidationSurfaceForID(catalog.ValidationSurfaces, selected.ValidationRecipeID)
	if !found || surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady || surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect {
		return false
	}
	for _, candidate := range surface.ProofRequirementBindings {
		candidate = candidate.Clone()
		if candidate.RequirementHash == selected.RequirementHash && candidate.DefinitionRevision == selected.DefinitionRevision &&
			candidate.CoverageWorkItemID == selected.CoverageWorkItemID && candidate.DeclarationRef == selected.DeclarationRef &&
			candidate.ValidationRecipeID == selected.ValidationRecipeID && sameExecutionStrings(candidate.RequiredCheckRefs, selected.RequiredCheckRefs) &&
			runtimecontract.SameRuntimeValidationCheckAttestationBindings(candidate.CheckAttestations, selected.CheckAttestations) {
			return true
		}
	}
	return false
}

func (s SealedRuntimeOperation) ValidateForExecution(context runtimecontract.RuntimeOperationExecutionContext) (runtimecontract.RuntimeOperation, RuntimeExecutionAuthorization, error) {
	if err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	if err := runtimecontract.ValidateRuntimeEvidencePolicies(s.Authorization.EvidencePolicies); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed Shell evidence policies: %w", err)
	}
	operation := s.Operation.Normalized()
	authorization := s.Authorization.normalized()
	if operation.Kind != runtimecontract.StepKindShellCommand && operation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed runtime operation is not owned by Shell")
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	if operation.ID != strings.TrimSpace(context.OperationID) {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed Shell operation identity mismatch")
	}
	if !sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed Shell operation scope mismatch")
	}
	if err := authorization.validateForOperation(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	if err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed Shell execution authority: %w", err)
	}
	if strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID ||
		strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed Shell owner catalog does not match claimed execution")
	}
	payload, detachedOperation, detachedAuthorization, err := sealedRuntimeOperationPayload(operation, authorization)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	actualHash := strings.TrimSpace(s.PayloadHash)
	if len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1 {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed Shell operation payload hash mismatch")
	}
	return detachedOperation, detachedAuthorization, nil
}

func sealedRuntimeOperationPayload(operation runtimecontract.RuntimeOperation, authorization RuntimeExecutionAuthorization) ([]byte, runtimecontract.RuntimeOperation, RuntimeExecutionAuthorization, error) {
	value := struct {
		Operation     runtimecontract.RuntimeOperation
		Authorization RuntimeExecutionAuthorization
	}{Operation: operation.Normalized(), Authorization: authorization.normalized()}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("marshal sealed Shell operation: %w", err)
	}
	var detached struct {
		Operation     runtimecontract.RuntimeOperation
		Authorization RuntimeExecutionAuthorization
	}
	if err := json.Unmarshal(payload, &detached); err != nil {
		return nil, runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("clone sealed Shell operation: %w", err)
	}
	return payload, detached.Operation, detached.Authorization, nil
}

func observationHasOwnerRevision(observation runtimecontract.RuntimePrerequisiteObservation, domain runtimecontract.RuntimeResourceDomain, subjectRef, revision string) bool {
	for _, dependency := range observation.Normalized().Dependencies {
		if dependency.Domain == domain && dependency.OwnerModuleID == runtimecontract.RuntimeModuleShell && dependency.SubjectRef == subjectRef && dependency.Revision == revision {
			return true
		}
	}
	return false
}

func containsOperationKind(values []string, wanted string) bool {
	wanted = runtimecontract.CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if runtimecontract.CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}

func normalizeExecutionStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func sameExecutionStrings(left, right []string) bool {
	left, right = normalizeExecutionStrings(left), normalizeExecutionStrings(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameExecutionCoverage(left, right []common.CoverageWorkItemID) bool {
	leftValues := make([]string, 0, len(left))
	for _, value := range left {
		leftValues = append(leftValues, string(value))
	}
	rightValues := make([]string, 0, len(right))
	for _, value := range right {
		rightValues = append(rightValues, string(value))
	}
	return sameExecutionStrings(leftValues, rightValues)
}

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	switch operation.Kind {
	case runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindManagedValidationSession:
	default:
		return OperationData{}, fmt.Errorf("operation kind %q is not a shell-owned runtime operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{TargetRefs: append([]string(nil), operation.TargetRefs...)}
	if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
		ret.ManagedValidation = runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			OriginalNodeKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			CleanupPolicy:    "process_group_on_timeout_or_completion", AutoCleanup: true, AutoCleanupSet: true,
		}
	}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "recipeId":
			if parameter.String != nil {
				ret.ValidationRecipeID = strings.TrimSpace(*parameter.String)
			}
		case "command":
			ret.Command = *parameter.String
		case "args":
			ret.Args = append([]string(nil), parameter.StringList...)
		case "timeoutMillis":
			ret.TimeoutMillis = *parameter.Integer
		case "stateChanging":
			ret.StateChanging = *parameter.Boolean
		case "targetRef":
			ret.TargetRef = strings.TrimSpace(*parameter.String)
		case "targetRefs":
			ret.TargetRefs = append([]string(nil), parameter.StringList...)
		case "verificationScope":
			ret.VerificationScope = *parameter.String
		case "boundedReason":
			ret.BoundedReason = *parameter.String
		case "validationScratchDir":
			ret.ValidationScratchDirectory = *parameter.String
		case "validationScratchAutoCleanup":
			ret.ValidationScratchAutoCleanup = *parameter.Boolean
			ret.ValidationScratchAutoCleanupSet = true
		}
	}
	if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
		if ret.ValidationRecipeID == "" {
			return OperationData{}, fmt.Errorf("managed validation operation requires recipeId")
		}
		if ret.ValidationScratchAutoCleanupSet {
			ret.ManagedValidation.AutoCleanup = ret.ValidationScratchAutoCleanup
			ret.ManagedValidation.AutoCleanupSet = true
		}
	}
	return ret, nil
}
