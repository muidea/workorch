package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeTargetDeclarationNormalizesAndBindsMissingSnapshot(t *testing.T) {
	declaration := RuntimeTargetDeclaration{TargetRef: "src\\main.go", TargetRole: "deliverable", Mode: RuntimeTargetDeclarationNewFile}.Normalized()
	if err := declaration.Validate(); err != nil {
		t.Fatal(err)
	}
	if declaration.TargetRef != "src/main.go" || declaration.Revision == "" {
		t.Fatalf("declaration = %+v", declaration)
	}
	snapshot := TargetSnapshot{TargetRef: declaration.TargetRef, ContentState: SnapshotContentStateMissing, Revision: "missing-target-v1", WorkspaceTreeRevision: "tree-v1"}
	if !RuntimeTargetSnapshotSatisfiesDeclaration(snapshot, &declaration) {
		t.Fatalf("declared missing snapshot was rejected: %+v", snapshot)
	}
	changed := declaration
	changed.Mode = RuntimeTargetDeclarationExistingFile
	changed = changed.Normalized()
	if RuntimeTargetSnapshotSatisfiesDeclaration(snapshot, &changed) {
		t.Fatal("existing-file declaration accepted a missing snapshot")
	}
}

func TestRuntimeTargetDeclarationRejectsNonCanonicalTarget(t *testing.T) {
	for _, target := range []string{"../main.go", "/tmp/main.go", "src/../main.go"} {
		value := RuntimeTargetDeclaration{TargetRef: target, TargetRole: "deliverable", Mode: RuntimeTargetDeclarationNewFile}.Normalized()
		if err := value.Validate(); err == nil {
			t.Fatalf("target %q was accepted", target)
		}
	}
}

func TestRuntimeTargetDeclarationUsesWebJSONFieldNames(t *testing.T) {
	value := RuntimeTargetDeclaration{TargetRef: "main.go", TargetRole: "implementation", Mode: RuntimeTargetDeclarationNewFile}.Normalized()
	payload, err := json.Marshal(value)
	if err != nil || !strings.Contains(string(payload), `"targetRef"`) || strings.Contains(string(payload), `"TargetRef"`) {
		t.Fatalf("web payload=%s err=%v", payload, err)
	}
}
