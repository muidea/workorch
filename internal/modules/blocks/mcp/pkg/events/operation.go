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

// RuntimeExecutionAuthorization is the detached MCP owner fact selected from
// the Runtime operation-admissibility snapshot. It carries no endpoint or
// credential material: the MCP owner resolves the exact current Registry
// aggregate and requires its revision to remain equal before transport
// execution.
type RuntimeExecutionAuthorization struct {
	CatalogSnapshotID  string
	AcquisitionBinding string
	RegistryRevision   string
	Capability         runtimecontract.RuntimeMCPCapabilityContext
	Policy             runtimecontract.RuntimeOperationPolicyFact
	ValidationSurface  *runtimecontract.RuntimeValidationSurface
}

func (a RuntimeExecutionAuthorization) normalized() RuntimeExecutionAuthorization {
	a.CatalogSnapshotID = strings.TrimSpace(a.CatalogSnapshotID)
	a.AcquisitionBinding = strings.TrimSpace(a.AcquisitionBinding)
	a.RegistryRevision = strings.TrimSpace(a.RegistryRevision)
	a.Capability = runtimecontract.SealMCPCapabilityEffect(a.Capability)
	a.Capability.Tags = append([]string(nil), a.Capability.Tags...)
	a.Capability.StageHints = append([]string(nil), a.Capability.StageHints...)
	a.Policy = a.Policy.Normalized()
	if a.ValidationSurface != nil {
		value := a.ValidationSurface.Normalized()
		a.ValidationSurface = &value
	}
	return a
}

func (a RuntimeExecutionAuthorization) validateForOperation(operation runtimecontract.RuntimeOperation) error {
	a = a.normalized()
	operation = operation.Normalized()
	if a.CatalogSnapshotID == "" || a.RegistryRevision == "" {
		return fmt.Errorf("sealed MCP authorization requires catalog identity and revision")
	}
	if err := a.Policy.Validate(); err != nil {
		return fmt.Errorf("sealed MCP operation policy: %w", err)
	}
	capability := a.Capability
	if capability.ID == "" || capability.ServerID == "" || !capability.Enabled || capability.Status != "enabled" ||
		capability.OperationKind != operation.Kind || a.Policy.OwnerModuleID != runtimecontract.RuntimeModuleMCP ||
		a.Policy.OperationKind != operation.Kind || a.Policy.SubjectRef != string(capability.ID) ||
		a.Policy.EffectScope != runtimecontract.RuntimeEffectScope(capability.EffectScope) ||
		a.Policy.IdempotencyClass != runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass) {
		return fmt.Errorf("sealed MCP authorization capability and policy do not match")
	}
	matched, found := runtimecontract.RuntimeOperationPolicyForOperation(operation, []runtimecontract.RuntimeOperationPolicyFact{a.Policy})
	if !found || matched.ID != a.Policy.ID {
		return fmt.Errorf("sealed MCP authorization does not select the operation")
	}
	data, err := OperationDataFromRuntimeOperation(operation)
	if err != nil {
		return err
	}
	if string(capability.ServerID) != data.ServerID {
		return fmt.Errorf("sealed MCP capability server does not match the operation")
	}
	switch operation.Kind {
	case runtimecontract.StepKindMCPToolCall:
		if capability.Kind != "tool" || capability.Name != data.Tool {
			return fmt.Errorf("sealed MCP tool capability does not match the operation")
		}
		if a.ValidationSurface != nil {
			if err := a.ValidationSurface.Validate(); err != nil {
				return fmt.Errorf("sealed MCP validation surface: %w", err)
			}
			if a.ValidationSurface.OwnerModuleID != runtimecontract.RuntimeModuleMCP || a.ValidationSurface.OperationKind != operation.Kind ||
				a.ValidationSurface.ID != runtimecontract.RuntimeMCPToolValidationRecipeID(data.ServerID, data.Tool) {
				return fmt.Errorf("sealed MCP validation surface does not match the operation")
			}
		}
	case runtimecontract.StepKindMCPResourceRead:
		if capability.Kind != "resource" || firstNonEmpty(capability.URI, capability.Name) != data.URI {
			return fmt.Errorf("sealed MCP resource capability does not match the operation")
		}
	case runtimecontract.StepKindMCPPromptGet:
		if capability.Kind != "prompt" || capability.Name != data.Prompt {
			return fmt.Errorf("sealed MCP prompt capability does not match the operation")
		}
	default:
		return fmt.Errorf("sealed runtime operation is not owned by MCP")
	}
	return nil
}

// SealedRuntimeOperation binds the complete operation payload to the exact
// MCP capability/policy fact persisted by Orchestrator. No independently
// mutable Data field crosses EventHub.
type SealedRuntimeOperation struct {
	Operation     runtimecontract.RuntimeOperation
	Authorization RuntimeExecutionAuthorization
	PayloadHash   string
}

func SealRuntimeOperation(operation runtimecontract.RuntimeOperation, catalog runtimecontract.RuntimeOperationAdmissibility) (SealedRuntimeOperation, error) {
	operation = operation.Normalized()
	if !mcpOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleMCP {
		return SealedRuntimeOperation{}, fmt.Errorf("runtime operation kind %q is not owned by MCP", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return SealedRuntimeOperation{}, err
	}
	catalog = catalog.Normalized()
	if err := catalog.Validate(); err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("MCP owner catalog: %w", err)
	}
	if catalog.OwnerModuleID != runtimecontract.RuntimeModuleMCP || !containsOperationKind(catalog.OperationKinds, operation.Kind) {
		return SealedRuntimeOperation{}, fmt.Errorf("MCP owner catalog does not contain operation kind %q", operation.Kind)
	}
	policy, found := runtimecontract.RuntimeOperationPolicyForOperation(operation, catalog.OperationPolicies)
	if !found || policy.SubjectRef == "" {
		return SealedRuntimeOperation{}, fmt.Errorf("MCP owner catalog has no exact operation policy")
	}
	var capability runtimecontract.RuntimeMCPCapabilityContext
	for _, candidate := range catalog.MCPCapabilities {
		candidate = runtimecontract.SealMCPCapabilityEffect(candidate)
		if string(candidate.ID) == policy.SubjectRef {
			capability = candidate
			break
		}
	}
	var validationSurface *runtimecontract.RuntimeValidationSurface
	if operation.Kind == runtimecontract.StepKindMCPToolCall {
		data, _ := OperationDataFromRuntimeOperation(operation)
		recipeID := runtimecontract.RuntimeMCPToolValidationRecipeID(data.ServerID, data.Tool)
		if surface, found := runtimecontract.RuntimeValidationSurfaceForID(catalog.ValidationSurfaces, recipeID); found {
			value := surface.Normalized()
			validationSurface = &value
		}
	}
	authorization := RuntimeExecutionAuthorization{
		CatalogSnapshotID:  catalog.SnapshotID,
		AcquisitionBinding: catalog.AcquisitionBinding,
		RegistryRevision:   catalog.ObservedRevision,
		Capability:         capability,
		Policy:             policy,
		ValidationSurface:  validationSurface,
	}.normalized()
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

func (s SealedRuntimeOperation) ValidateForExecution(context runtimecontract.RuntimeOperationExecutionContext) (runtimecontract.RuntimeOperation, RuntimeExecutionAuthorization, error) {
	if err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	operation := s.Operation.Normalized()
	authorization := s.Authorization.normalized()
	if !mcpOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleMCP {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed runtime operation is not owned by MCP")
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	if operation.ID != strings.TrimSpace(context.OperationID) {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed MCP operation identity mismatch")
	}
	if !sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed MCP operation scope mismatch")
	}
	if err := authorization.validateForOperation(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	if err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed MCP execution authority: %w", err)
	}
	if strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID ||
		strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed MCP owner catalog does not match claimed execution")
	}
	payload, detachedOperation, detachedAuthorization, err := sealedRuntimeOperationPayload(operation, authorization)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	actualHash := strings.TrimSpace(s.PayloadHash)
	if len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1 {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed MCP operation payload hash mismatch")
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
		return nil, runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("marshal sealed MCP operation: %w", err)
	}
	var detached struct {
		Operation     runtimecontract.RuntimeOperation
		Authorization RuntimeExecutionAuthorization
	}
	if err := json.Unmarshal(payload, &detached); err != nil {
		return nil, runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("clone sealed MCP operation: %w", err)
	}
	return payload, detached.Operation, detached.Authorization, nil
}

func mcpOperationKind(kind string) bool {
	switch runtimecontract.CanonicalRuntimeBlockKind(kind) {
	case runtimecontract.StepKindMCPToolCall, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindMCPPromptGet:
		return true
	default:
		return false
	}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func sameExecutionStrings(left, right []string) bool {
	normalize := func(values []string) []string {
		seen := map[string]struct{}{}
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
		return ret
	}
	left, right = normalize(left), normalize(right)
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
	toStrings := func(values []common.CoverageWorkItemID) []string {
		ret := make([]string, 0, len(values))
		for _, value := range values {
			ret = append(ret, string(value))
		}
		return ret
	}
	return sameExecutionStrings(toStrings(left), toStrings(right))
}

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	switch operation.Kind {
	case runtimecontract.StepKindMCPToolCall, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindMCPPromptGet:
	default:
		return OperationData{}, fmt.Errorf("operation kind %q is not an mcp operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{}
	for _, p := range operation.Parameters {
		switch p.Name {
		case "serverId":
			ret.ServerID = *p.String
		case "tool":
			ret.Tool = *p.String
		case "uri", "resourceUri":
			ret.URI = *p.String
		case "prompt", "name":
			ret.Prompt = *p.String
		case "summary":
			ret.Summary = *p.String
		case "stream":
			ret.Stream = *p.Boolean
		default:
			if name, ok := strings.CutPrefix(p.Name, "input."); ok {
				ret.Input = append(ret.Input, operationParameterWithName(name, p))
			} else if name, ok := strings.CutPrefix(p.Name, "arguments."); ok {
				ret.Arguments = append(ret.Arguments, operationParameterWithName(name, p))
			}
		}
	}
	return ret, nil
}

func operationParameterWithName(name string, p runtimecontract.OperationParameter) runtimecontract.OperationParameter {
	ret := runtimecontract.OperationParameter{Name: strings.TrimSpace(name), StringList: append([]string(nil), p.StringList...)}
	if p.String != nil {
		value := *p.String
		ret.String = &value
	}
	if p.Integer != nil {
		value := *p.Integer
		ret.Integer = &value
	}
	if p.Boolean != nil {
		value := *p.Boolean
		ret.Boolean = &value
	}
	return ret
}
