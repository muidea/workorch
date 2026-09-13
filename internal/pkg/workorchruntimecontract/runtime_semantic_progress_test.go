package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func semanticProgressFixture() (RuntimeSemanticPlanRequestContext, RuntimeSemanticPlanManifest, RuntimeFacts) {
	items := []RuntimeCoverageItem{
		{ID: "done", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "first criterion"},
		{ID: "current", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-2", TargetText: "second criterion"},
		{ID: "remaining", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-3", TargetText: "third criterion"},
	}
	c := RuntimeSemanticPlanRequestContext{WorkspaceID: "ws", RunID: "run", Definition: RuntimeDefinitionContext{Revision: "definition", StartupInputHash: "startup"},
		DefinitionSeed: RuntimeDefinitionSeed{Hash: "seed", CoverageItems: items},
		PlanStep:       RuntimeSemanticPlanStepProofBatch, BatchID: "batch", BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"current"},
	}
	m := RuntimeSemanticPlanManifest{Hash: "manifest", CoverageItems: items[1:]}
	f := RuntimeFacts{WorkspaceID: "ws", RunID: "run", DefinitionRevision: "definition", DefinitionSeed: c.DefinitionSeed,
		EvidenceRevision: "evidence-7", Coverage: InitialRuntimeCoverageFacts(items)}
	for i := range f.Coverage {
		switch f.Coverage[i].ID {
		case "done":
			f.Coverage[i].State = RuntimeCoverageItemStateSatisfied
		case "current":
			f.Coverage[i].EvidenceRefs = []string{"raw-sensitive-ref-not-for-provider", "second-history-ref"}
			f.Coverage[i].ReopenLineage = []RuntimeCWIReopenLineage{{}}
		}
	}
	return c, m, f
}

func TestSemanticCoverageProgressSeparatesAcceptanceFromRecordedWork(t *testing.T) {
	c, m, f := semanticProgressFixture()
	before, _ := json.Marshal(f)
	p, err := BuildRuntimeSemanticCoverageProgress(c, m, f, 9)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 3 || p.Satisfied != 1 || p.Open != 2 || p.Unsatisfied != 0 || len(p.VisibleItems) != 1 ||
		p.VisibleItems[0].ObservedState != RuntimeCoverageItemStateOpen || p.VisibleItems[0].RecordedEvidenceCount != 2 || p.VisibleItems[0].ReopenCount != 1 {
		t.Fatalf("history became terminal credit or lost whole-goal totals: %+v", p)
	}
	encoded, _ := json.Marshal(p)
	if strings.Contains(string(encoded), "raw-sensitive") || strings.Contains(string(encoded), "criterion") || strings.Contains(string(encoded), `"coverageWorkItemId":"done"`) {
		t.Fatalf("progress expanded hidden history or unrelated CWI: %s", encoded)
	}
	after, _ := json.Marshal(f)
	if string(before) != string(after) {
		t.Fatal("read-only projection changed facts")
	}
	var reloaded RuntimeFacts
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	replay, err := BuildRuntimeSemanticCoverageProgress(c, m, reloaded, 9)
	if err != nil || !reflect.DeepEqual(p, replay) {
		t.Fatalf("snapshot reload changed progress: %v", err)
	}
	clone := p.Clone()
	clone.VisibleItems[0].RecordedEvidenceCount++
	if p.VisibleItems[0].RecordedEvidenceCount != 2 {
		t.Fatal("clone aliased progress")
	}
	c.PlanStep, c.BatchID, c.BatchCoverageWorkItemIDs = RuntimeSemanticPlanStepCoverageOrder, "", nil
	order, err := BuildRuntimeSemanticCoverageProgress(c, m, f, 9)
	if err != nil || len(order.VisibleItems) != 2 || order.Total != 3 || order.Satisfied != 1 {
		t.Fatalf("order scope lost global progress: %+v %v", order, err)
	}
}

func TestSemanticCoverageProgressRejectsIncompleteOrForeignOwnerFacts(t *testing.T) {
	for _, field := range []string{"workspace", "run", "definition", "seed", "missing", "duplicate", "foreign", "source", "state", "revision"} {
		t.Run(field, func(t *testing.T) {
			c, m, f := semanticProgressFixture()
			switch field {
			case "workspace":
				f.WorkspaceID = "foreign"
			case "run":
				f.RunID = "foreign"
			case "definition":
				f.DefinitionRevision = "foreign"
			case "seed":
				f.DefinitionSeed.Hash = "foreign"
			case "missing":
				f.Coverage = f.Coverage[1:]
			case "duplicate":
				f.Coverage[0] = f.Coverage[1]
			case "foreign":
				f.Coverage[0].ID = "foreign"
			case "source":
				f.Coverage[0].SourceID = "foreign"
			case "state":
				f.Coverage[0].State = "unknown"
			case "revision":
				f.EvidenceRevision = ""
			}
			if _, err := BuildRuntimeSemanticCoverageProgress(c, m, f, 9); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

func TestSemanticCoverageProgressUsesCanonicalSeedTextWithoutDroppingItems(t *testing.T) {
	c, m, f := semanticProgressFixture()
	c.DefinitionSeed.CoverageItems[0].TargetText += "  "
	if _, err := BuildRuntimeSemanticCoverageProgress(c, m, f, 9); err != nil {
		t.Fatalf("canonical seed text rejected: %v", err)
	}
	c.DefinitionSeed.CoverageItems[0] = c.DefinitionSeed.CoverageItems[1]
	if _, err := BuildRuntimeSemanticCoverageProgress(c, m, f, 9); err == nil {
		t.Fatal("duplicate sealed inventory accepted")
	}
}

func TestSemanticCoverageProgressRejectsTamperedOrIncompleteTransport(t *testing.T) {
	for _, field := range []string{"missing", "manifest", "startup", "batch", "batch_sequence", "scope", "duplicate", "count", "state_count", "hash", "revision", "capacity"} {
		t.Run(field, func(t *testing.T) {
			c, m, f := semanticProgressFixture()
			p, err := BuildRuntimeSemanticCoverageProgress(c, m, f, 9)
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "missing":
				p.VisibleItems = nil
			case "manifest":
				p.ManifestHash = "foreign"
			case "startup":
				p.StartupInputHash = "foreign"
			case "batch":
				p.BatchID = "foreign"
			case "batch_sequence":
				p.BatchSequence++
			case "scope":
				p.VisibleItems[0].CoverageWorkItemID = "done"
			case "duplicate":
				p.VisibleItems = append(p.VisibleItems, p.VisibleItems[0])
			case "count":
				p.Total++
			case "state_count":
				p.Open, p.Satisfied = 0, 3
			case "revision":
				p.SourceStateRevision = 0
			case "capacity":
				p.EvidenceRevision = strings.Repeat("r", RuntimeSemanticCoverageProgressMaxBytes)
			}
			p.Hash = StableRuntimeSemanticCoverageProgressHash(p)
			if field == "hash" {
				p.Hash = "invalid"
			}
			if err := p.Validate(c, m); err == nil {
				t.Fatal("invalid transport accepted")
			}
		})
	}
}
