package events

import (
	"fmt"
	"reflect"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const RuntimeValidationReadDependencyMaxFiles = 8192

// Read-only original copied-input selector, not executable authorization.
// The owner independently obtains the directory and reverse-proves ReadSource.
type QueryRuntimeValidationReadDependenciesCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	Operation             runtimecontract.RuntimeOperation
	RequestID             string
	IssuedAtStateRevision uint64
	SourceReceiptHash     string
	OriginalDependencies  runtimecontract.RuntimeOwnerRevisionSet
	ApplicabilitySource   *runtimecontract.RuntimeEvidenceApplicabilityDependencies
}

type QueryRuntimeValidationReadDependenciesResult struct {
	RequestID             string
	IssuedAtStateRevision uint64
	SourceReceiptHash     string
	Missing               bool
	CurrentDependencies   *runtimecontract.RuntimeOwnerRevisionSet
}

func (c QueryRuntimeValidationReadDependenciesCommand) Validate() error {
	if c.RequestID == "" || c.RequestID != strings.TrimSpace(c.RequestID) || c.IssuedAtStateRevision == 0 ||
		c.SourceReceiptHash == "" || c.SourceReceiptHash != strings.TrimSpace(c.SourceReceiptHash) ||
		c.Operation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession || c.Operation.ID != c.OperationID ||
		runtimecontract.ValidateRuntimeOperationContract(c.Operation) != nil || runtimecontract.ValidateRuntimeOperationExecutionContextScope(c.RuntimeOperationExecutionContext) != nil ||
		c.Execution.OperationHash != runtimecontract.StableRuntimeOperationHash(c.Operation) ||
		!reflect.DeepEqual(c.Operation.TargetRefs, c.TargetRefs) || !reflect.DeepEqual(c.Operation.CoverageWorkItemIDs, c.CoverageWorkItemIDs) ||
		c.EvidenceKind != runtimecontract.EvidenceKindValidation || c.ApplicabilitySource == nil ||
		len(c.OriginalDependencies.Entries) == 0 || len(c.OriginalDependencies.Entries) > RuntimeValidationReadDependencyMaxFiles {
		return fmt.Errorf("Shell copied-input query requires exact original managed-validation source")
	}
	if err := runtimecontract.ValidateRuntimeEvidenceDependencyPair(&c.OriginalDependencies, c.ApplicabilitySource, c.Transaction.DefinitionRevision); err != nil {
		return err
	}
	if !reflect.DeepEqual(c.OriginalDependencies, c.ApplicabilitySource.OwnerRevisionSet) {
		return fmt.Errorf("Shell copied-input applicability must preserve original selector snapshot")
	}
	for _, entry := range c.OriginalDependencies.Entries {
		if entry.OwnerKind != "capability" || entry.OwnerInstanceID != runtimecontract.RuntimeModuleShell+"/"+string(c.WorkspaceID) || entry.ResourceKind != string(runtimecontract.RuntimeResourceWorkspaceTarget) {
			return fmt.Errorf("Shell copied-input selector owner/resource mismatch")
		}
	}
	return nil
}

func (r QueryRuntimeValidationReadDependenciesResult) ValidateFor(c QueryRuntimeValidationReadDependenciesCommand) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if err := c.Validate(); err != nil {
		return nil, false, err
	}
	if r.RequestID != c.RequestID || r.IssuedAtStateRevision != c.IssuedAtStateRevision || r.SourceReceiptHash != c.SourceReceiptHash || r.Missing == (r.CurrentDependencies != nil) {
		return nil, false, fmt.Errorf("Shell copied-input observation fence/body mismatch")
	}
	if r.Missing {
		return nil, false, nil
	}
	set := r.CurrentDependencies
	if set.Validate() != nil || !reflect.DeepEqual(*set, set.Normalized()) || set.DefinitionRevision != c.Transaction.DefinitionRevision || len(set.Entries) != len(c.OriginalDependencies.Entries) {
		return nil, false, fmt.Errorf("Shell copied-input observation requires complete canonical set")
	}
	for index, original := range c.OriginalDependencies.Entries {
		current := set.Entries[index]
		original.Revision, original.DependencyHash = "", ""
		current.Revision, current.DependencyHash = "", ""
		if !reflect.DeepEqual(original, current) {
			return nil, false, fmt.Errorf("Shell copied-input observation changed selector identity")
		}
	}
	return runtimecontract.CloneRuntimeOwnerRevisionSet(set), true, nil
}
