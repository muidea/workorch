package events

import (
	"fmt"
	"reflect"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateRuntimeFileReadDependenciesResult 只核对回应身份和完整观察，不选择 stale/reopen。
func ValidateRuntimeFileReadDependenciesResult(command QueryRuntimeFileReadDependenciesCommand, result QueryRuntimeFileReadDependenciesResult) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if command.RequestID == "" || command.RequestID != strings.TrimSpace(command.RequestID) || command.IssuedAtStateRevision == 0 ||
		command.SourceReceiptHash == "" || command.SourceReceiptHash != strings.TrimSpace(command.SourceReceiptHash) ||
		command.OriginalDependencies.DefinitionRevision != command.Transaction.DefinitionRevision || len(command.OriginalDependencies.Entries) != 1 {
		return nil, false, fmt.Errorf("File read dependency query requires exact source identity")
	}
	if !reflect.DeepEqual(command.OriginalDependencies, command.OriginalDependencies.Normalized()) {
		return nil, false, fmt.Errorf("File read dependency source is not raw canonical")
	}
	if err := command.OriginalDependencies.Validate(); err != nil {
		return nil, false, err
	}
	if err := runtimecontract.ValidateRuntimeEvidenceDependencyPair(&command.OriginalDependencies, command.ApplicabilitySource, command.Transaction.DefinitionRevision); err != nil {
		return nil, false, err
	}
	expected := command.OriginalDependencies
	if command.ApplicabilitySource != nil {
		expected = command.ApplicabilitySource.OwnerRevisionSet
		if len(expected.Entries) != 1 {
			return nil, false, fmt.Errorf("File applicability source requires one exact resource")
		}
	}
	if result.RequestID != command.RequestID || result.IssuedAtStateRevision != command.IssuedAtStateRevision || result.SourceReceiptHash != command.SourceReceiptHash {
		return nil, false, fmt.Errorf("File read dependency observation identity mismatch")
	}
	switch result.Status {
	case RuntimeFileReadDependencyMissing:
		if result.CurrentDependencies != nil {
			return nil, false, fmt.Errorf("missing File read dependency cannot carry a current set")
		}
		return nil, false, nil
	case RuntimeFileReadDependencyObserved:
		set := result.CurrentDependencies
		if set == nil || set.DefinitionRevision != command.Transaction.DefinitionRevision || len(set.Entries) != 1 {
			return nil, false, fmt.Errorf("File read dependency observation requires the exact Definition and resource")
		}
		if !reflect.DeepEqual(*set, set.Normalized()) {
			return nil, false, fmt.Errorf("File read dependency observation is not raw canonical")
		}
		if err := set.Validate(); err != nil {
			return nil, false, err
		}
		if len(command.OriginalDependencies.Entries) != 1 {
			return nil, false, fmt.Errorf("File read dependency source requires one resource")
		}
		original, current := expected.Entries[0], set.Entries[0]
		original.Revision, current.Revision = "", ""
		original.DependencyHash, current.DependencyHash = "", ""
		if !reflect.DeepEqual(original, current) {
			return nil, false, fmt.Errorf("File read dependency observation resource identity mismatch")
		}
		return runtimecontract.CloneRuntimeOwnerRevisionSet(set), true, nil
	default:
		return nil, false, fmt.Errorf("unknown File read dependency observation status %q", result.Status)
	}
}
