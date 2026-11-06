package llm

import (
	"encoding/json"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"strings"
	"testing"
)

func TestSemanticSnapshotBodiesPrecedeChangingObservationMetadata(t *testing.T) {
	content := &rc.RuntimeImplementationWorkContent{ProtocolRef: rc.RuntimeImplementationWorkContentV1, ManifestRevision: "revision-a", Snapshots: []rc.TargetSnapshot{{TargetRef: "a", Content: "原文\r\n", ContentHash: "hash-a", ObservedAt: "time-a", Size: 8, LineCount: 1}, {TargetRef: "b", Content: strings.Repeat("second body", 50), ContentHash: "hash-b", ObservedAt: "time-a", Size: 550}}}
	projection := runtimeSemanticWorkProjection{Content: content}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var restored runtimeSemanticWorkProjection
	if err := json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(projection, restored) {
		t.Fatal("snapshot metadata lost", err)
	}
	render := func(p runtimeSemanticWorkProjection) string {
		raw, _ := json.Marshal(struct {
			Work runtimeSemanticWorkProjection `json:"workContext"`
		}{p})
		raw, e := cacheLayoutJSON(raw, "semantic-shared")
		if e != nil {
			t.Fatal(e)
		}
		return string(raw)
	}
	base := render(projection)
	changed := projection
	changed.Content = content.Clone()
	changed.Content.Snapshots[0].ObservedAt = "new observation"
	changed.Content.ManifestRevision = "revision-b"
	next := render(changed)
	a, b := semanticContentPrefixEnd(base), semanticContentPrefixEnd(next)
	if a == 0 || b == 0 || base[:a] != next[:b] || base == next {
		t.Fatal("metadata change interrupted exact body prefix")
	}
	changed.Content.Snapshots[0].Content += "real change"
	different := render(changed)
	if base[:a] == different[:semanticContentPrefixEnd(different)] {
		t.Fatal("changed content reused stale prefix")
	}
	var wire runtimeSemanticWorkWire
	_ = json.Unmarshal(encoded, &wire)
	for _, name := range []string{"missing", "wrong-target", "mixed-body"} {
		t.Run(name, func(t *testing.T) {
			var w runtimeSemanticWorkWire
			_ = json.Unmarshal(encoded, &w)
			switch name {
			case "missing":
				w.Content.SnapshotObservations = nil
			case "wrong-target":
				w.Content.SnapshotObservations[0].TargetRef = "foreign"
			case "mixed-body":
				w.Content.SnapshotObservations[0].Content = "unexpected"
			}
			raw, _ := json.Marshal(w)
			var p runtimeSemanticWorkProjection
			if json.Unmarshal(raw, &p) == nil {
				t.Fatal("invalid observation join accepted")
			}
		})
	}
}
