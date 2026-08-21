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

// SealedRuntimeOperation is the immutable Web operation persisted by the
// Orchestrator. Web derives its private request only after the complete
// operation payload and execution scope have been verified.
type SealedRuntimeOperation struct {
	Operation   runtimecontract.RuntimeOperation
	PayloadHash string
}

func SealRuntimeOperation(operation runtimecontract.RuntimeOperation) (SealedRuntimeOperation, error) {
	operation = operation.Normalized()
	if !webOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleWeb {
		return SealedRuntimeOperation{}, fmt.Errorf("runtime operation kind %q is not owned by Web", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return SealedRuntimeOperation{}, err
	}
	payload, err := json.Marshal(operation)
	if err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("marshal sealed Web operation: %w", err)
	}
	var detached runtimecontract.RuntimeOperation
	if err := json.Unmarshal(payload, &detached); err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("clone sealed Web operation: %w", err)
	}
	sum := sha256.Sum256(payload)
	return SealedRuntimeOperation{Operation: detached, PayloadHash: hex.EncodeToString(sum[:])}, nil
}

func (s SealedRuntimeOperation) ValidateForExecution(context runtimecontract.RuntimeOperationExecutionContext) (runtimecontract.RuntimeOperation, error) {
	operation := s.Operation.Normalized()
	if !webOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleWeb {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed runtime operation is not owned by Web")
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, err
	}
	if operation.ID != strings.TrimSpace(context.OperationID) {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed Web operation identity mismatch")
	}
	if !sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed Web operation scope mismatch")
	}
	payload, err := json.Marshal(operation)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("marshal received Web operation: %w", err)
	}
	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	actualHash := strings.TrimSpace(s.PayloadHash)
	if len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1 {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed Web operation payload hash mismatch")
	}
	sealed, err := SealRuntimeOperation(operation)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, err
	}
	return sealed.Operation, nil
}

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindWebSearch && operation.Kind != runtimecontract.StepKindWebFetch {
		return OperationData{}, fmt.Errorf("operation kind %q is not a web operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{SafeSearch: true}
	for _, p := range operation.Parameters {
		switch p.Name {
		case "providerId":
			ret.ProviderID = *p.String
		case "query":
			ret.Query = *p.String
		case "locale":
			ret.Locale = *p.String
		case "recency":
			ret.Recency = *p.String
		case "domains":
			ret.Domains = append([]string(nil), p.StringList...)
		case "excludeDomains":
			ret.ExcludeDomains = append([]string(nil), p.StringList...)
		case "maxResults":
			ret.MaxResults = *p.Integer
		case "safeSearch":
			ret.SafeSearch = *p.Boolean
		case "url":
			ret.URL = *p.String
		case "maxBytes":
			ret.MaxBytes = *p.Integer
		case "extractMode":
			ret.ExtractMode = *p.String
		}
	}
	return ret, nil
}

func webOperationKind(kind string) bool {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	return kind == runtimecontract.StepKindWebSearch || kind == runtimecontract.StepKindWebFetch
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
