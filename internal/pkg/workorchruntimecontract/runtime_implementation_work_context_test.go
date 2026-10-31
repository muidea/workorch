package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func workContentSnapshot(target, body string) TargetSnapshot {
	h := sha256.Sum256([]byte(body))
	return TargetSnapshot{TargetRef: target, Content: body, ContentHash: hex.EncodeToString(h[:]), ContentState: SnapshotContentStateComplete, Size: int64(len(body))}
}
func TestImplementationWorkContentAndIntentAreExactOriginalInputs(t *testing.T) {
	m := semanticWorkValidationManifest(t)
	w := m.ProofObligations[0].ImplementationWork
	w.PurposeChoiceOnly, w.EffectAvailable = true, true
	w.Scope.Content = &RuntimeImplementationWorkContent{ProtocolRef: RuntimeImplementationWorkContentV1, ManifestRevision: w.Scope.Selection.ObservedRevision}
	for _, target := range w.Scope.Selection.TargetRefs {
		w.Scope.Content.Snapshots = append(w.Scope.Content.Snapshots, workContentSnapshot(target, "完整原文\n"))
	}
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	d := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: m.Hash, DefinitionSeedHash: m.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, ImplementationPurpose: RuntimeImplementationWorkPurposeTerminal}
	for _, item := range m.CoverageItems {
		d.CoverageOrder = append(d.CoverageOrder, item.ID)
	}
	d.ImplementationIntent = &RuntimeImplementationIntent{DeclarationRef: m.ProofObligations[0].DeclarationRef, ContentHash: w.Scope.Content.Hash(), RemainingWork: "请求独立核验剩余原义务", Reason: "实际内容已可被原验收检查"}
	wire, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, wire)
	if err != nil || StableRuntimeSemanticPlanDocumentHash(decoded) != StableRuntimeSemanticPlanDocumentHash(d) {
		t.Fatalf("exact wire replay: %v", err)
	}
	schema, err := RuntimeSemanticPlanJSONSchemaForRequest(m, d.PlanStep, "", 0, nil)
	if err != nil || !strings.Contains(schema, "implementationIntent") || !strings.Contains(schema, w.Scope.Content.Hash()) {
		t.Fatalf("missing exact intent schema: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeSemanticPlanDocument){
		"missing":                   func(v *RuntimeSemanticPlanDocument) { v.ImplementationIntent = nil },
		"other-original-obligation": func(v *RuntimeSemanticPlanDocument) { v.ImplementationIntent.DeclarationRef = "another" },
		"stale-content":             func(v *RuntimeSemanticPlanDocument) { v.ImplementationIntent.ContentHash = "stale" },
		"empty-reason":              func(v *RuntimeSemanticPlanDocument) { v.ImplementationIntent.Reason = " " },
	} {
		t.Run(name, func(t *testing.T) {
			bad := d.Clone()
			mutate(&bad)
			if bad.ValidateAgainstManifest(m) == nil {
				t.Fatal("accepted missing/stale purpose binding")
			}
		})
	}
	clone := m.Clone()
	clone.ProofObligations[0].ImplementationWork.Scope.Content.Snapshots[0].Content = "altered"
	clone.Hash = StableRuntimeSemanticPlanManifestHash(clone)
	if clone.Validate() == nil {
		t.Fatal("hash-only altered original bytes trusted")
	}
	if w.Scope.Content.Snapshots[0].Content != "完整原文\n" {
		t.Fatal("clone aliases original content")
	}
	raw, _ := json.Marshal(m)
	var restored RuntimeSemanticPlanManifest
	if json.Unmarshal(raw, &restored) != nil || restored.Validate() != nil {
		t.Fatal("restart lost complete input")
	}
	if err := w.Scope.Content.Validate(w.Scope); err != nil {
		t.Fatal(err)
	}
	bad := w.Scope.Content.Clone()
	bad.Snapshots = nil
	if bad.Validate(w.Scope) == nil {
		t.Fatal("missing file body silently accepted")
	}
	bad = w.Scope.Content.Clone()
	bad.Snapshots[0].ContentState = SnapshotContentStateTruncated
	if bad.Validate(w.Scope) == nil {
		t.Fatal("partial body accepted as complete")
	}
}
