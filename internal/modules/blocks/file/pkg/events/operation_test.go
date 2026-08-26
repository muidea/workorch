package events

import (
	"slices"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestContractsDeclareFileAsQueryAndExecutionReceiver(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 15 {
		t.Fatalf("contract count = %d, want 15", len(contracts))
	}
	queryTopics := map[string]struct{}{
		TopicAcquireRuntimeCapabilities:           {},
		TopicAcquireRuntimeOperationAdmissibility: {},
		TopicProbeRuntimeOperationFeasibility:     {},
		TopicProbeRuntimeSemanticProofFeasibility: {},
		TopicQueryRuntimeCWIReconciliation:        {},
		TopicAcquireTargetSnapshot:                {}, TopicAcquireRuntimeTargetSelection: {},
		TopicAcquireWorkspaceManifest: {}, TopicConfirmRuntimeRepairSurface: {},
	}
	for _, contract := range contracts {
		if contract.Owner != "file" || !slices.Equal(contract.ReceiverOwners, []string{"file"}) {
			t.Fatalf("contract %s owner=%q receivers=%v", contract.Topic, contract.Owner, contract.ReceiverOwners)
		}
		if _, query := queryTopics[contract.Topic]; query {
			if contract.CommandClass != eventcontract.CommandClassRead || contract.LanePolicy != eventcontract.LanePolicyQuery {
				t.Fatalf("query contract %s class=%s lane=%s", contract.Topic, contract.CommandClass, contract.LanePolicy)
			}
			delete(queryTopics, contract.Topic)
		}
	}
	if len(queryTopics) != 0 {
		t.Fatalf("missing File query contracts: %v", queryTopics)
	}
	if len(MessageContracts()) != len(contracts) {
		t.Fatalf("message contract count = %d, want %d", len(MessageContracts()), len(contracts))
	}
}

func TestFileInspectionOperationDataFromRuntimeOperation(t *testing.T) {
	path := "internal/main.go"
	recursive := true
	maxEntries := int64(25)
	list, err := FileListOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "list-1", Kind: runtimecontract.StepKindFileList,
		Parameters: []runtimecontract.OperationParameter{
			{Name: "path", String: &path}, {Name: "recursive", Boolean: &recursive}, {Name: "maxEntries", Integer: &maxEntries},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if list.Path != path || !list.Recursive || list.MaxEntries != maxEntries {
		t.Fatalf("file/list data = %+v", list)
	}

	optional := true
	maxBytes := int64(4096)
	startLine := int64(10)
	endLine := int64(20)
	read, err := FileReadOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "read-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "optional", Boolean: &optional}, {Name: "maxBytes", Integer: &maxBytes},
			{Name: "startLine", Integer: &startLine}, {Name: "endLine", Integer: &endLine},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if read.Path != path || !read.Optional || read.MaxBytes != maxBytes || read.StartLine != startLine || read.EndLine != endLine {
		t.Fatalf("file/read data = %+v", read)
	}
}

func TestFileWriteAndPatchPreviewOperationDataFromRuntimeOperation(t *testing.T) {
	target := "README.md"
	content := "typed file write\n"
	write, err := FileWriteOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if write.TargetRef != target || write.Content != content || !write.ContentPresent {
		t.Fatalf("file/write data = %+v", write)
	}

	patchType := "file_write"
	preview, err := PatchPreviewOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "preview-1", Kind: runtimecontract.RuntimeBlockKindPatchPreview, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType}, {Name: "content", String: &content},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.TargetRef != target || preview.PatchType != patchType || preview.Content != content || !preview.ContentPresent {
		t.Fatalf("patch/preview data = %+v", preview)
	}
}

func TestSealedRuntimeOperationBindsPayloadAndExecutionScope(t *testing.T) {
	content := "sealed\n"
	operation := runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"README.md"},
		Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}},
	}
	sealed, err := SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch",
		Execution: testExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws/run/dispatch/write-1", Fence: 1,
			ExpectedEffectScope: runtimecontract.ResolveExpectedEffectScope(operation, nil),
		}, nil),
		OperationID: "write-1", TargetRefs: []string{"README.md"},
	}
	validated, err := sealed.ValidateForExecution(context, runtimecontract.StepKindFileWrite)
	if err != nil || validated.ID != operation.ID || validated.TargetRefs[0] != "README.md" {
		t.Fatalf("validated operation=%+v err=%v", validated, err)
	}
	idempotencyDrift := context
	idempotencyDrift.Execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
	if _, err := sealed.ValidateForExecution(idempotencyDrift, runtimecontract.StepKindFileWrite); err == nil || !strings.Contains(err.Error(), "idempotency class mismatch") {
		t.Fatalf("sealed File operation accepted idempotency drift: %v", err)
	}
	authorityDrift := context
	authorityDrift.Execution.AuthorityHash = "forged"
	if _, err := sealed.ValidateForExecution(authorityDrift, runtimecontract.StepKindFileWrite); err == nil || !strings.Contains(err.Error(), "authority hash mismatch") {
		t.Fatalf("sealed File operation accepted authority hash drift: %v", err)
	}

	mutated := sealed
	changed := "different\n"
	mutated.Operation.Parameters[0].String = &changed
	if _, err := mutated.ValidateForExecution(context, runtimecontract.StepKindFileWrite); err == nil {
		t.Fatal("mutated operation payload reused the sealed hash")
	}
	mismatchedContext := context
	mismatchedContext.TargetRefs = []string{"other.md"}
	if _, err := sealed.ValidateForExecution(mismatchedContext, runtimecontract.StepKindFileWrite); err == nil {
		t.Fatal("sealed operation accepted a different execution target")
	}
}
