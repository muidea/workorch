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

func TestSemanticWorkWirePreservesExcludedTargetRefs(t *testing.T) {
	content := &rc.RuntimeImplementationWorkContent{
		ProtocolRef:        rc.RuntimeImplementationWorkContentV1,
		ManifestRevision:   "rev-1",
		ExcludedTargetRefs: []string{"checks/input_test.go", "checks/spec_test.go"},
		Snapshots: []rc.TargetSnapshot{
			{TargetRef: "a.go", Content: "package main\n", ContentHash: "hash-a", ContentState: rc.SnapshotContentStateComplete, Size: 13},
		},
	}
	projection := runtimeSemanticWorkProjection{Content: content}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var restored runtimeSemanticWorkProjection
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projection, restored) || restored.Content.Hash() != content.Hash() {
		t.Fatalf("wire round-trip lost ExcludedTargetRefs: got=%+v want=%+v", restored.Content, content)
	}

	// Omitting ExcludedTargetRefs must decode to nil (matching raw contract equality).
	plain := runtimeSemanticWorkProjection{
		Content: &rc.RuntimeImplementationWorkContent{
			ProtocolRef:      rc.RuntimeImplementationWorkContentV1,
			ManifestRevision: "rev-1",
			Snapshots:        content.Snapshots,
		},
	}
	plainEncoded, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	var plainRestored runtimeSemanticWorkProjection
	if err := json.Unmarshal(plainEncoded, &plainRestored); err != nil {
		t.Fatal(err)
	}
	if plainRestored.Content == nil || plainRestored.Content.ExcludedTargetRefs != nil || plainRestored.Content.ReviewSnapshots != nil || !reflect.DeepEqual(plain, plainRestored) {
		t.Fatalf("empty ExcludedTargetRefs/ReviewSnapshots decoded as non-nil or drifted: %+v", plainRestored.Content)
	}

	withReview := runtimeSemanticWorkProjection{
		Content: &rc.RuntimeImplementationWorkContent{
			ProtocolRef:        rc.RuntimeImplementationWorkContentV1,
			ManifestRevision:   "rev-2",
			ExcludedTargetRefs: []string{"checks/input_test.go"},
			Snapshots:          content.Snapshots,
			ReviewSnapshots: []rc.TargetSnapshot{
				{TargetRef: "checks/input_test.go", Content: "package checks // updated\n", ContentHash: "hash-rev", ContentState: rc.SnapshotContentStateComplete, Size: 25},
			},
		},
	}
	withReviewEncoded, err := json.Marshal(withReview)
	if err != nil {
		t.Fatal(err)
	}
	var withReviewRestored runtimeSemanticWorkProjection
	if err := json.Unmarshal(withReviewEncoded, &withReviewRestored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(withReview, withReviewRestored) || withReviewRestored.Content.Hash() != withReview.Content.Hash() {
		t.Fatalf("wire round-trip lost ReviewSnapshots: got=%+v want=%+v", withReviewRestored.Content, withReview.Content)
	}
	// cacheLayoutJSON must place reviewSnapshots before SnapshotObservations.
	rawWrap, _ := json.Marshal(struct {
		Work runtimeSemanticWorkProjection `json:"workContext"`
	}{withReview})
	laidOut, err := cacheLayoutJSON(rawWrap, "semantic-shared")
	if err != nil {
		t.Fatal(err)
	}
	reviewPos := strings.Index(string(laidOut), `"reviewSnapshots":`)
	obsPos := strings.Index(string(laidOut), `"SnapshotObservations":`)
	reviewObsPos := strings.Index(string(laidOut), `"reviewSnapshotObservations":`)
	if reviewPos < 0 || obsPos < 0 || reviewObsPos < 0 || !(reviewPos < obsPos && obsPos < reviewObsPos) {
		t.Fatalf("unexpected cache layout order for reviewSnapshots: %s", string(laidOut))
	}
	for _, badCase := range []string{"missing-obs", "wrong-target", "mixed-body", "empty-non-nil"} {
		var w runtimeSemanticWorkWire
		_ = json.Unmarshal(withReviewEncoded, &w)
		var badRaw []byte
		switch badCase {
		case "missing-obs":
			w.Content.ReviewSnapshotObservations = nil
			badRaw, _ = json.Marshal(w)
		case "wrong-target":
			w.Content.ReviewSnapshotObservations[0].TargetRef = "foreign"
			badRaw, _ = json.Marshal(w)
		case "mixed-body":
			w.Content.ReviewSnapshotObservations[0].Content = "inline"
			badRaw, _ = json.Marshal(w)
		case "empty-non-nil":
			badRaw = []byte(`{"content":{"ProtocolRef":"` + rc.RuntimeImplementationWorkContentV1 + `","Snapshots":[],"SnapshotObservations":[],"reviewSnapshots":[],"reviewSnapshotObservations":[],"ManifestRevision":"rev-2"}}`)
		}
		var out runtimeSemanticWorkProjection
		if json.Unmarshal(badRaw, &out) == nil {
			t.Fatalf("invalid reviewSnapshots wire accepted (%s)", badCase)
		}
	}
}
