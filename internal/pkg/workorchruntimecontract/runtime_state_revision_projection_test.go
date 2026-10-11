package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func assertStateRevisionProjectionReference(t testing.TB, value RuntimeStateRecord) {
	t.Helper()
	before := value.Clone()
	want := value.Definition.Normalized().Revision == value.Facts.Normalized().DefinitionRevision
	if got := strings.TrimSpace(value.Definition.Revision) == strings.TrimSpace(value.Facts.DefinitionRevision); got != want {
		t.Fatal("opaque state revision 标量投影偏离原完整规范化")
	}
	if !reflect.DeepEqual(value, before) {
		t.Fatal("revision 投影改写原 record")
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeStateRecord
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(loaded.Definition.Revision) == strings.TrimSpace(loaded.Facts.DefinitionRevision), loaded.Definition.Normalized().Revision == loaded.Facts.Normalized().DefinitionRevision; got != want {
		t.Fatal("JSON 重载后 revision 投影漂移")
	}
}

func stateRevisionProjectionFixture() RuntimeStateRecord {
	return RuntimeStateRecord{Definition: RuntimeDefinitionContext{Revision: " definition ", CapabilityContract: []string{"b", "a", "a"}, GoalDefinition: strings.Repeat("原", 4096)}, Facts: RuntimeFacts{DefinitionRevision: "definition", Resources: RuntimeResourceFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{runtimeOwnerCatalogSurfaceReuseFixture(true)}}}}
}

func TestStateRevisionProjectionKeepsRawValuesAndIndependentNormalization(t *testing.T) {
	for _, revision := range []string{"", " \t\r\n", " definition ", " 原 ", string([]byte{0xff, 0x80}), "revision\x00", strings.Repeat("原", 32768)} {
		value := stateRevisionProjectionFixture()
		value.Definition.Revision, value.Facts.DefinitionRevision = revision, revision
		assertStateRevisionProjectionReference(t, value)
		value.Facts.DefinitionRevision += "different"
		assertStateRevisionProjectionReference(t, value)
	}
	value := stateRevisionProjectionFixture()
	original := testing.AllocsPerRun(20, func() { _ = value.Definition.Normalized().Revision == value.Facts.Normalized().DefinitionRevision })
	current := testing.AllocsPerRun(20, func() {
		_ = strings.TrimSpace(value.Definition.Revision) == strings.TrimSpace(value.Facts.DefinitionRevision)
	})
	if current != 0 || current >= original {
		t.Fatalf("revision 查询仍复制无关正文：current=%v original=%v", current, original)
	}
}

func FuzzStateRevisionProjectionReference(f *testing.F) {
	f.Add(" definition ", "definition")
	f.Add(string([]byte{0xff, 0x80}), "\x00")
	f.Fuzz(func(t *testing.T, definition, facts string) {
		value := stateRevisionProjectionFixture()
		value.Definition.Revision, value.Facts.DefinitionRevision = definition, facts
		assertStateRevisionProjectionReference(t, value)
	})
}

func BenchmarkStateRevisionProjection(b *testing.B) {
	value := stateRevisionProjectionFixture()
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = value.Definition.Normalized().Revision == value.Facts.Normalized().DefinitionRevision
				} else {
					_ = strings.TrimSpace(value.Definition.Revision) == strings.TrimSpace(value.Facts.DefinitionRevision)
				}
			}
		})
	}
}
