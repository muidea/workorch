package workorchruntimecontract

import (
	"testing"
)

func TestStableRuntimeOperationShapeIgnoresSummaryMetadata(t *testing.T) {
	path := "main.go"
	cmd := "go"
	args := []string{"test", "./..."}
	summaryA := "first try"
	summaryB := "retry with different wording"
	left := RuntimeOperation{
		ID: "op-1", Kind: StepKindShellCommand, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args},
			{Name: "summary", String: &summaryA},
		},
	}
	right := RuntimeOperation{
		ID: "op-2", Kind: StepKindShellCommand, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args},
			{Name: "summary", String: &summaryB},
		},
	}
	if StableRuntimeOperationShape(left) != StableRuntimeOperationShape(right) {
		t.Fatalf("summary-only change must not create a new shape")
	}
	// args change must create a new shape
	args2 := []string{"test", "./pkg"}
	changed := RuntimeOperation{
		ID: "op-3", Kind: StepKindShellCommand, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args2},
			{Name: "summary", String: &summaryA},
		},
	}
	if StableRuntimeOperationShape(left) == StableRuntimeOperationShape(changed) {
		t.Fatalf("args change should create a new shape")
	}
}

func TestStableRuntimeOperationShapeIncludesWebRequestAndDependencies(t *testing.T) {
	query := "runtime contract"
	left := RuntimeOperation{
		ID: "web-1", Kind: StepKindWebSearch, DependsOn: []string{"prepare-1"},
		Parameters: []OperationParameter{{Name: "query", String: &query}},
	}
	changedQuery := "runtime contract updates"
	requestChanged := left
	requestChanged.ID = "web-2"
	requestChanged.Parameters = []OperationParameter{{Name: "query", String: &changedQuery}}
	if StableRuntimeOperationShape(left) == StableRuntimeOperationShape(requestChanged) {
		t.Fatal("query change must create a new operation shape")
	}

	dependencyChanged := left
	dependencyChanged.ID = "web-3"
	dependencyChanged.DependsOn = []string{"prepare-2"}
	if StableRuntimeOperationShape(left) == StableRuntimeOperationShape(dependencyChanged) {
		t.Fatal("dependency graph change must create a new operation shape")
	}
}

func TestStableRuntimeOperationShapeIncludesOldText(t *testing.T) {
	path := "cmd/proxy/main.go"
	patchType := "text_replace"
	oldA := "a"
	oldB := "b"
	newText := "c"
	left := RuntimeOperation{
		ID: "op-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldA}, {Name: "newText", String: &newText},
		},
	}
	right := RuntimeOperation{
		ID: "op-2", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldB}, {Name: "newText", String: &newText},
		},
	}
	if StableRuntimeOperationShape(left) == StableRuntimeOperationShape(right) {
		t.Fatalf("different oldText should produce different shapes")
	}
}

func TestStableRuntimeOperationShapeIncludesWebFilters(t *testing.T) {
	query := "workorch runtime"
	providerA := "provider-a"
	providerB := "provider-b"
	left := RuntimeOperation{
		ID: "web-1", Kind: StepKindWebSearch,
		Parameters: []OperationParameter{
			{Name: "query", String: &query},
			{Name: "providerId", String: &providerA},
		},
	}
	right := RuntimeOperation{
		ID: "web-2", Kind: StepKindWebSearch,
		Parameters: []OperationParameter{
			{Name: "query", String: &query},
			{Name: "providerId", String: &providerB},
		},
	}
	if StableRuntimeOperationShape(left) == StableRuntimeOperationShape(right) {
		t.Fatalf("providerId change must create a new web search shape")
	}
}

func TestStableRuntimeOperationShapeIncludesFileListRecursive(t *testing.T) {
	pathValue := "internal"
	recTrue := true
	recFalse := false
	left := RuntimeOperation{
		ID: "list-1", Kind: StepKindFileList,
		Parameters: []OperationParameter{
			{Name: "path", String: &pathValue},
			{Name: "recursive", Boolean: &recTrue},
		},
	}
	right := RuntimeOperation{
		ID: "list-2", Kind: StepKindFileList,
		Parameters: []OperationParameter{
			{Name: "path", String: &pathValue},
			{Name: "recursive", Boolean: &recFalse},
		},
	}
	if StableRuntimeOperationShape(left) == StableRuntimeOperationShape(right) {
		t.Fatalf("recursive change must create a new file/list shape")
	}
}
