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

// RuntimeExecutionAuthorization binds an SSH operation to the exact frozen
// target/session/credential catalog used by SM-2 and SM-3. Endpoint and secret
// material remain owner-private and are resolved only after freshness checks.
type RuntimeExecutionAuthorization struct {
	CatalogSnapshotID  string
	AcquisitionBinding string
	PrerequisiteID     string
	OwnerRevision      string
	CatalogObservation runtimecontract.RuntimePrerequisiteObservation
	Policy             runtimecontract.RuntimeOperationPolicyFact
	BindingRef         string
}

func (a RuntimeExecutionAuthorization) normalized() RuntimeExecutionAuthorization {
	a.CatalogSnapshotID = strings.TrimSpace(a.CatalogSnapshotID)
	a.AcquisitionBinding = strings.TrimSpace(a.AcquisitionBinding)
	a.PrerequisiteID = strings.TrimSpace(a.PrerequisiteID)
	a.OwnerRevision = strings.TrimSpace(a.OwnerRevision)
	a.CatalogObservation = a.CatalogObservation.Normalized()
	a.Policy = a.Policy.Normalized()
	a.BindingRef = strings.TrimSpace(a.BindingRef)
	return a
}

func (a RuntimeExecutionAuthorization) validateForOperation(operation runtimecontract.RuntimeOperation) error {
	a = a.normalized()
	if a.CatalogSnapshotID == "" || a.PrerequisiteID == "" || a.OwnerRevision == "" || a.BindingRef == "" {
		return fmt.Errorf("sealed SSH authorization requires catalog identity, revision and binding")
	}
	if err := a.CatalogObservation.Validate(); err != nil {
		return fmt.Errorf("sealed SSH catalog observation: %w", err)
	}
	if a.CatalogObservation.PrerequisiteID != a.PrerequisiteID ||
		a.CatalogObservation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility ||
		a.CatalogObservation.OwnerModuleID != runtimecontract.RuntimeModuleSSH ||
		a.CatalogObservation.ContentHash != a.CatalogSnapshotID ||
		!sshObservationHasOwnerRevision(a.CatalogObservation, a.OwnerRevision) {
		return fmt.Errorf("sealed SSH catalog observation does not match authorization")
	}
	if err := a.Policy.Validate(); err != nil {
		return fmt.Errorf("sealed SSH operation policy: %w", err)
	}
	matched, found := runtimecontract.RuntimeOperationPolicyForOperation(operation, []runtimecontract.RuntimeOperationPolicyFact{a.Policy})
	if !found || matched.ID != a.Policy.ID || a.Policy.OwnerModuleID != runtimecontract.RuntimeModuleSSH {
		return fmt.Errorf("sealed SSH operation policy does not select the operation")
	}
	data, err := OperationDataFromRuntimeOperation(operation)
	if err != nil {
		return err
	}
	if a.BindingRef != runtimeSSHBindingRef(data.TargetRef, data.SessionRef, data.CredentialRef) {
		return fmt.Errorf("sealed SSH owner binding does not match operation refs")
	}
	return nil
}

// SealedRuntimeOperation is an immutable copy of the operation persisted in
// the Orchestrator dispatch. SSH derives its owner-local request only after the
// complete payload, owner revision and execution scope have been validated.
type SealedRuntimeOperation struct {
	Operation     runtimecontract.RuntimeOperation
	Authorization RuntimeExecutionAuthorization
	PayloadHash   string
}

func SealRuntimeOperation(operation runtimecontract.RuntimeOperation, catalog runtimecontract.RuntimeOperationAdmissibility, observation runtimecontract.RuntimePrerequisiteObservation) (SealedRuntimeOperation, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindSSHCommand || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleSSH {
		return SealedRuntimeOperation{}, fmt.Errorf("runtime operation kind %q is not owned by SSH", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return SealedRuntimeOperation{}, err
	}
	catalog = catalog.Normalized()
	if err := catalog.Validate(); err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("SSH owner catalog: %w", err)
	}
	if catalog.OwnerModuleID != runtimecontract.RuntimeModuleSSH || !containsOperationKind(catalog.OperationKinds, operation.Kind) {
		return SealedRuntimeOperation{}, fmt.Errorf("SSH owner catalog does not contain operation kind %q", operation.Kind)
	}
	policy, found := runtimecontract.RuntimeOperationPolicyForOperation(operation, catalog.OperationPolicies)
	if !found {
		return SealedRuntimeOperation{}, fmt.Errorf("SSH owner catalog has no exact operation policy")
	}
	data, err := OperationDataFromRuntimeOperation(operation)
	if err != nil {
		return SealedRuntimeOperation{}, err
	}
	bindingRef := runtimeSSHBindingRef(data.TargetRef, data.SessionRef, data.CredentialRef)
	if !executionStringIncluded(catalog.TargetRefs, data.TargetRef) || !executionStringIncluded(catalog.ConstraintRefs, bindingRef) {
		return SealedRuntimeOperation{}, fmt.Errorf("SSH operation refs are outside owner catalog")
	}
	authorization := RuntimeExecutionAuthorization{
		CatalogSnapshotID: catalog.SnapshotID, PrerequisiteID: catalog.PrerequisiteID,
		AcquisitionBinding: catalog.AcquisitionBinding,
		OwnerRevision:      catalog.ObservedRevision, CatalogObservation: observation, Policy: policy, BindingRef: bindingRef,
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
	operation := s.Operation.Normalized()
	authorization := s.Authorization.normalized()
	if operation.Kind != runtimecontract.StepKindSSHCommand || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleSSH {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed runtime operation is not owned by SSH")
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	if operation.ID != strings.TrimSpace(context.OperationID) {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed SSH operation identity mismatch")
	}
	if !sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed SSH operation scope mismatch")
	}
	if err := authorization.validateForOperation(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	if context.Execution.ExpectedEffectScope != authorization.Policy.EffectScope {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed SSH operation effect does not match claimed execution")
	}
	if strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID ||
		strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed SSH owner catalog does not match claimed execution")
	}
	payload, detachedOperation, detachedAuthorization, err := sealedRuntimeOperationPayload(operation, authorization)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, err
	}
	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	actualHash := strings.TrimSpace(s.PayloadHash)
	if len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1 {
		return runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("sealed SSH operation payload hash mismatch")
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
		return nil, runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("marshal sealed SSH operation: %w", err)
	}
	var detached struct {
		Operation     runtimecontract.RuntimeOperation
		Authorization RuntimeExecutionAuthorization
	}
	if err := json.Unmarshal(payload, &detached); err != nil {
		return nil, runtimecontract.RuntimeOperation{}, RuntimeExecutionAuthorization{}, fmt.Errorf("clone sealed SSH operation: %w", err)
	}
	return payload, detached.Operation, detached.Authorization, nil
}

func sshObservationHasOwnerRevision(observation runtimecontract.RuntimePrerequisiteObservation, revision string) bool {
	for _, dependency := range observation.Normalized().Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == runtimecontract.RuntimeModuleSSH &&
			dependency.SubjectRef == "ssh_runtime_policy" && dependency.Revision == revision {
			return true
		}
	}
	return false
}

func runtimeSSHBindingRef(targetRef, sessionRef, credentialRef string) string {
	return "ssh-binding/" + strings.TrimSpace(targetRef) + "/" + strings.TrimSpace(sessionRef) + "/" + strings.TrimSpace(credentialRef)
}

func executionStringIncluded(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
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

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindSSHCommand {
		return OperationData{}, fmt.Errorf("operation kind %q is not ssh/command", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{TargetRef: operation.TargetRefs[0]}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "sessionRef":
			ret.SessionRef = *parameter.String
		case "credentialRef":
			ret.CredentialRef = *parameter.String
		case "command":
			ret.Command = *parameter.String
		case "timeoutMillis":
			ret.TimeoutMillis = *parameter.Integer
		}
	}
	return ret, nil
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
	left = normalize(left)
	right = normalize(right)
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
