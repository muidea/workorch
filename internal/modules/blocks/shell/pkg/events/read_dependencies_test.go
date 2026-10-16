package events

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellCopiedInputQueryResultRequiresExactCompleteSource(t *testing.T) {
	op, _, _, _, context := shellSealFixture(t)
	context.Transaction = runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority", DefinitionRevision: "definition"}
	context.EvidenceKind = runtimecontract.EvidenceKindValidation
	set := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: context.Transaction.DefinitionRevision, Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: runtimecontract.RuntimeModuleShell + "/" + string(context.WorkspaceID), ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget), ResourceID: "input.go", Revision: "actual-bytes", DependencyHash: "actual-selector"}}}).WithHash()
	command := QueryRuntimeValidationReadDependenciesCommand{RuntimeOperationExecutionContext: context, Operation: op, RequestID: "query", IssuedAtStateRevision: 7, SourceReceiptHash: "original-receipt", OriginalDependencies: set, ApplicabilitySource: &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesV1, OwnerRevisionSet: set}}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	result := QueryRuntimeValidationReadDependenciesResult{RequestID: command.RequestID, IssuedAtStateRevision: command.IssuedAtStateRevision, SourceReceiptHash: command.SourceReceiptHash, CurrentDependencies: runtimecontract.CloneRuntimeOwnerRevisionSet(&set)}
	current, observed, err := result.ValidateFor(command)
	if err != nil || !observed || !reflect.DeepEqual(current, &set) {
		t.Fatalf("exact result: %v", err)
	}
	current.Entries[0].Revision = "caller-mutation"
	if result.CurrentDependencies.Entries[0].Revision != "actual-bytes" {
		t.Fatal("result shared entries with caller")
	}
	for _, name := range []string{"request", "revision", "receipt", "missing-set", "empty", "definition", "resource", "hash", "raw"} {
		t.Run(name, func(t *testing.T) {
			bad := result
			bad.CurrentDependencies = runtimecontract.CloneRuntimeOwnerRevisionSet(result.CurrentDependencies)
			switch name {
			case "request":
				bad.RequestID = "wrong"
			case "revision":
				bad.IssuedAtStateRevision++
			case "receipt":
				bad.SourceReceiptHash = "wrong"
			case "missing-set":
				bad.Missing = true
			case "empty":
				bad.CurrentDependencies.Entries = nil
				*bad.CurrentDependencies = bad.CurrentDependencies.WithHash()
			case "definition":
				bad.CurrentDependencies.DefinitionRevision = "other"
				*bad.CurrentDependencies = bad.CurrentDependencies.WithHash()
			case "resource":
				bad.CurrentDependencies.Entries[0].ResourceID = "other.go"
				*bad.CurrentDependencies = bad.CurrentDependencies.WithHash()
			case "hash":
				bad.CurrentDependencies.SetHash = "wrong"
			case "raw":
				bad.CurrentDependencies.Entries[0].Revision = " actual-bytes"
			}
			if _, _, err := bad.ValidateFor(command); err == nil {
				t.Fatal("malformed response accepted")
			}
		})
	}
	result.Missing, result.CurrentDependencies = true, nil
	if current, observed, err := result.ValidateFor(command); err != nil || current != nil || observed {
		t.Fatalf("actual missing: %v", err)
	}
}
