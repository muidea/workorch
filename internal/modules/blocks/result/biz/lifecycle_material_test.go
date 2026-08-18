package result

import (
	"testing"

	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

var resultMaterialBenchmarkSink *resultmodel.Result

func TestResultFromTerminalMaterialDetachesMutableReferences(t *testing.T) {
	material := resultMaterialFixture()
	hash, err := resultevents.CanonicalResultMaterialHash(material)
	if err != nil {
		t.Fatal(err)
	}
	result := resultFromTerminalMaterial(material, "workspace-material/run-material/result/dod-assessment", []common.ArtifactID{"artifact-canonical"}, hash)
	material.EvidenceRefs[0] = "mutated"
	material.ArtifactRefs[0] = "artifact-mutated"
	if result.EvidenceRefs[0] != "evidence-0" || result.ArtifactRefs[0] != "artifact-canonical" {
		t.Fatalf("terminal material shares mutable references: %+v", result)
	}
}

func BenchmarkResultFromTerminalMaterial(b *testing.B) {
	material := resultMaterialFixture()
	hash, _ := resultevents.CanonicalResultMaterialHash(material)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		resultMaterialBenchmarkSink = resultFromTerminalMaterial(material, "workspace-material/run-material/result/dod-assessment", []common.ArtifactID{"artifact-canonical"}, hash)
	}
}

func resultMaterialFixture() resultevents.ResultMaterial {
	evidenceRefs := make([]string, 16)
	artifactRefs := make([]common.ArtifactID, 16)
	for idx := range evidenceRefs {
		evidenceRefs[idx] = "evidence-0"
		artifactRefs[idx] = "artifact-0"
	}
	return resultevents.ResultMaterial{
		WorkspaceID:  "workspace-material",
		RunID:        "run-material",
		GoalID:       "goal-material",
		DoDID:        "dod-material",
		Summary:      "summary",
		EvidenceRefs: evidenceRefs,
		ArtifactRefs: artifactRefs,
	}
}
