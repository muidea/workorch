package events

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidateRuntimeFileReadDependenciesResultRejectsPartialOrWrongSource(t *testing.T) {
	set := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: runtimecontract.RuntimeModuleFile + "/ws", ResourceKind: "workspace_target", ResourceID: "source.txt", Revision: "original", DependencyHash: "selector",
	}}}).WithHash()
	command := QueryRuntimeFileReadDependenciesCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition"}},
		RequestID:                        "request", IssuedAtStateRevision: 7, SourceReceiptHash: "receipt", OriginalDependencies: set,
	}
	response := QueryRuntimeFileReadDependenciesResult{RequestID: "request", IssuedAtStateRevision: 7, SourceReceiptHash: "receipt", Status: RuntimeFileReadDependencyObserved, CurrentDependencies: runtimecontract.CloneRuntimeOwnerRevisionSet(&set)}
	for _, scenario := range []string{"request", "revision", "receipt", "status", "nil", "raw", "hash", "definition", "resource", "owner", "missing_with_set", "bad_command"} {
		t.Run(scenario, func(t *testing.T) {
			value, source := response, command
			value.CurrentDependencies = runtimecontract.CloneRuntimeOwnerRevisionSet(response.CurrentDependencies)
			switch scenario {
			case "request":
				value.RequestID = "other"
			case "revision":
				value.IssuedAtStateRevision++
			case "receipt":
				value.SourceReceiptHash = "other"
			case "status":
				value.Status = "partial"
			case "nil":
				value.CurrentDependencies = nil
			case "raw":
				value.CurrentDependencies.Entries[0].Revision = " original"
			case "hash":
				value.CurrentDependencies.SetHash = "wrong"
			case "definition":
				value.CurrentDependencies.DefinitionRevision = "other"
				*value.CurrentDependencies = value.CurrentDependencies.WithHash()
			case "resource":
				value.CurrentDependencies.Entries[0].ResourceID = "other"
				*value.CurrentDependencies = value.CurrentDependencies.WithHash()
			case "owner":
				value.CurrentDependencies.Entries[0].OwnerInstanceID = "other"
				*value.CurrentDependencies = value.CurrentDependencies.WithHash()
			case "missing_with_set":
				value.Status = RuntimeFileReadDependencyMissing
			case "bad_command":
				source.RequestID = " request"
			}
			if _, _, err := ValidateRuntimeFileReadDependenciesResult(source, value); err == nil {
				t.Fatal("非法或部分 response 被接受")
			}
		})
	}
	current, found, err := ValidateRuntimeFileReadDependenciesResult(command, response)
	if err != nil || !found || !reflect.DeepEqual(current, &set) {
		t.Fatalf("exact observation rejected: %v", err)
	}
	current.Entries[0].Revision = "caller"
	if response.CurrentDependencies.Entries[0].Revision != "original" {
		t.Fatal("response shared mutable set")
	}
	response.Status, response.CurrentDependencies = RuntimeFileReadDependencyMissing, nil
	if set, found, err := ValidateRuntimeFileReadDependenciesResult(command, response); err != nil || found || set != nil {
		t.Fatalf("missing invalid: %v", err)
	}
}
