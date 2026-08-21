package result

import (
	"fmt"
	"testing"

	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

var (
	resultMaterialBenchmarkSink     *resultmodel.Result
	resultMaterialHashBenchmarkSink string
)

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

func TestLargeResultMaterialReferenceNormalizationAndAllocationGrowth(t *testing.T) {
	const referenceCount = 1024
	material, artifactIDs := largeResultMaterialFixture(referenceCount)
	if err := material.Validate(); err != nil {
		t.Fatalf("large material validation: %v", err)
	}
	hash, err := resultevents.CanonicalResultMaterialHash(material)
	if err != nil || hash == "" {
		t.Fatalf("large material hash=%q err=%v", hash, err)
	}
	reordered := material
	reordered.EvidenceRefs = append([]string(nil), material.EvidenceRefs...)
	reordered.ArtifactRefs = append([]common.ArtifactID(nil), material.ArtifactRefs...)
	for left, right := 0, len(reordered.EvidenceRefs)-1; left < right; left, right = left+1, right-1 {
		reordered.EvidenceRefs[left], reordered.EvidenceRefs[right] = reordered.EvidenceRefs[right], reordered.EvidenceRefs[left]
		reordered.ArtifactRefs[left], reordered.ArtifactRefs[right] = reordered.ArtifactRefs[right], reordered.ArtifactRefs[left]
	}
	reorderedHash, err := resultevents.CanonicalResultMaterialHash(reordered)
	if err != nil || reorderedHash != hash {
		t.Fatalf("reordered large material hash=%q want=%q err=%v", reorderedHash, hash, err)
	}
	item := resultFromTerminalMaterial(material, "workspace-material/run-material/result/dod-assessment", artifactIDs, hash)
	if len(item.EvidenceRefs) != referenceCount || len(item.ArtifactRefs) != referenceCount || len(item.SummaryDetail.ArtifactRefs) != referenceCount {
		t.Fatalf("large result refs evidence=%d artifact=%d summary=%d", len(item.EvidenceRefs), len(item.ArtifactRefs), len(item.SummaryDetail.ArtifactRefs))
	}
	material.EvidenceRefs[0] = "mutated"
	material.ArtifactRefs[0] = "artifact-mutated"
	if item.EvidenceRefs[0] == "mutated" || item.ArtifactRefs[0] == "artifact-mutated" {
		t.Fatal("large result mapping aliases terminal material")
	}

	small, _ := largeResultMaterialFixture(256)
	large, _ := largeResultMaterialFixture(referenceCount)
	smallAllocs := testing.AllocsPerRun(5, func() {
		resultMaterialHashBenchmarkSink, _ = resultevents.CanonicalResultMaterialHash(small)
	})
	largeAllocs := testing.AllocsPerRun(5, func() {
		resultMaterialHashBenchmarkSink, _ = resultevents.CanonicalResultMaterialHash(large)
	})
	// This is a toolchain-relative growth guard, not a wall-clock or absolute
	// allocation contract. Quadrupling refs must not create super-linear
	// allocation growth with a broad margin for encoding implementation drift.
	if largeAllocs > smallAllocs*6+128 {
		t.Fatalf("large material allocations grew unexpectedly: 256=%0.0f 1024=%0.0f", smallAllocs, largeAllocs)
	}
}

func BenchmarkCanonicalResultMaterialHash1024Refs(b *testing.B) {
	material, _ := largeResultMaterialFixture(1024)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var err error
		resultMaterialHashBenchmarkSink, err = resultevents.CanonicalResultMaterialHash(material)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResultFromTerminalMaterial1024Refs(b *testing.B) {
	material, artifactIDs := largeResultMaterialFixture(1024)
	hash, _ := resultevents.CanonicalResultMaterialHash(material)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		resultMaterialBenchmarkSink = resultFromTerminalMaterial(material, "workspace-material/run-material/result/dod-assessment", artifactIDs, hash)
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

func largeResultMaterialFixture(count int) (resultevents.ResultMaterial, []common.ArtifactID) {
	evidenceRefs := make([]string, count)
	artifactRefs := make([]common.ArtifactID, count)
	artifactIDs := make([]common.ArtifactID, count)
	for idx := range count {
		evidenceRefs[idx] = fmt.Sprintf("workorch://evidence/%04d", idx)
		artifactRefs[idx] = common.ArtifactID(fmt.Sprintf("workorch://artifact/%04d", idx))
		artifactIDs[idx] = common.ArtifactID(fmt.Sprintf("workspace-material/run-material/result/dod-assessment/artifact/%04d", idx))
	}
	return resultevents.ResultMaterial{
		WorkspaceID: "workspace-material", RunID: "run-material", GoalID: "goal-material", DoDID: "dod-material",
		Summary: "large result material", EvidenceRefs: evidenceRefs, ArtifactRefs: artifactRefs,
		SummaryDetail: workorchresultcontract.ResultSummary{ArtifactRefs: append([]common.ArtifactID(nil), artifactRefs...)},
	}, artifactIDs
}
