package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultMaterialRuntimeAdapterPreservesRawShapeAndDetachedFields(t *testing.T) {
	material := runtimecontract.RuntimeTerminalResultMaterial{
		WorkspaceID: "workspace", RunID: "run", GoalID: "goal", DoDID: "dod", Summary: "summary",
		EvidenceRefs: []string{}, ArtifactRefs: []common.ArtifactID{},
		DoDAssessment: workorchresultcontract.DoDAssessment{Items: []workorchresultcontract.DoDAssessmentItem{{EvidenceRefs: []string{"evidence"}}}},
		SummaryDetail: workorchresultcontract.ResultSummary{KnownLimitations: []string{}},
	}
	adapted := ResultMaterialFromRuntimeTerminalMaterial(material)
	if !reflect.DeepEqual(adapted.RuntimeTerminalMaterial(), material) || adapted.EvidenceRefs == nil || adapted.ArtifactRefs == nil || adapted.SummaryDetail.KnownLimitations == nil {
		t.Fatal("concrete adaptation changed raw value or nil/empty identity")
	}
	neutralJSON, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	ownerJSON, err := json.Marshal(adapted)
	if err != nil || string(neutralJSON) != string(ownerJSON) {
		t.Fatalf("adapter changed concrete JSON shape: %v", err)
	}
	adapted.DoDAssessment.Items[0].EvidenceRefs[0] = "changed"
	if material.DoDAssessment.Items[0].EvidenceRefs[0] != "evidence" {
		t.Fatal("owner adapter aliases compiler material")
	}
	copy := adapted.RuntimeTerminalMaterial()
	copy.DoDAssessment.Items[0].EvidenceRefs[0] = "other"
	if adapted.DoDAssessment.Items[0].EvidenceRefs[0] != "changed" {
		t.Fatal("neutral adapter aliases Result material")
	}
}

func TestResultMaterialUnifiedHashRetainsConcreteShapeAndNormalization(t *testing.T) {
	material := ResultMaterial{
		WorkspaceID: " workspace ", RunID: "run", GoalID: "goal", DoDID: "dod", Summary: " summary ",
		EvidenceRefs: []string{"b", "a", "b", " "}, ArtifactRefs: []common.ArtifactID{"artifact-b", "artifact-a"},
		DoDAssessment: workorchresultcontract.DoDAssessment{Items: []workorchresultcontract.DoDAssessmentItem{{EvidenceRefs: []string{"b", "a"}}}},
	}
	normalized := material.Normalized()
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	want := "result-material-sha256:" + hex.EncodeToString(digest[:])
	got, err := CanonicalResultMaterialHash(material)
	if err != nil || got != want {
		t.Fatalf("hash shape changed: got=%s want=%s err=%v", got, want, err)
	}
	neutralHash, err := material.RuntimeTerminalMaterial().CanonicalHash()
	if err != nil || neutralHash != got {
		t.Fatalf("owner and compiler hashes disagree: %v", err)
	}
	if err := material.ValidateRaw(); err == nil {
		t.Fatal("raw validation normalized owner material")
	}
	if err := normalized.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
	if err := material.Validate(); err != nil {
		t.Fatal(err)
	}
}
