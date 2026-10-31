package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func semanticSegmentedHistoryFixture(t *testing.T, count int) RuntimeSemanticPlanningLedger {
	t.Helper()
	base := semanticSealedLedgerFixture(t)
	ledger := base.WithoutHistory()
	for i := 0; i < count; i++ {
		snapshot := base.WithoutHistory().Clone()
		snapshot.SchemaVersion = ledger.SchemaVersion
		snapshot.LogicalRequestID = fmt.Sprintf("independent-request-%d", i)
		g := RuntimeSemanticPlanningGeneration{RetiredAtStateRevision: uint64(i + 2), Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
			InvalidationSignatures: []string{fmt.Sprintf("invalidation-%d", i)}, PrerequisiteIDs: []string{"semantic-history-catalog"}, Snapshot: snapshot}
		var err error
		ledger, err = ledger.AppendHistory(g)
		if err != nil {
			t.Fatal(err)
		}
	}
	return ledger
}

func TestRuntimeSemanticHistoryRejectsMissingWholeSegment(t *testing.T) {
	ledger := semanticSegmentedHistoryFixture(t, 33)
	ledger.HistoryArchive.Segments = ledger.HistoryArchive.Segments[1:]
	if err := ledger.Validate(); err == nil {
		t.Fatal("missing complete archive segment accepted")
	}
}

func TestRuntimeSemanticHistorySegmentsPreserveBodiesRestartAndBudget(t *testing.T) {
	for _, count := range []int{0, 16, 17, 32, 33} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ledger := semanticSegmentedHistoryFixture(t, count)
			if err := ledger.Validate(); err != nil {
				t.Fatal(err)
			}
			if ledger.HistoryCount() != count || len(ledger.History) > 16 {
				t.Fatal("rollover lost history or overflowed the tail")
			}
			if (ledger.HistoryArchive != nil) != (count > 16) {
				t.Fatal("unexpected archive")
			}
			if count > 16 && ledger.SchemaVersion != RuntimeSemanticPlanningLedgerSchemaSegmented {
				t.Fatal("old readers could silently ignore archive")
			}
			if count > 16 && referenceSemanticLedgerBeforeSeedSession(ledger, ledger.Normalized(), newRuntimeSemanticValidationSession()) == nil {
				t.Fatal("legacy validator accepted segmented schema")
			}
			data, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			if count <= 16 && strings.Contains(string(data), "HistoryArchive") {
				t.Fatal("legacy wire changed without rollover")
			}
			var reopened RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(data, &reopened); err != nil {
				t.Fatal(err)
			}
			if err := reopened.Validate(); err != nil {
				t.Fatal(err)
			}
			for i, g := range reopened.AllHistory() {
				if g.RetiredAtStateRevision != uint64(i+2) || g.Snapshot.LogicalRequestID != fmt.Sprintf("independent-request-%d", i) || g.Snapshot.History != nil || g.Snapshot.HistoryArchive != nil {
					t.Fatal("reordered, nested or rewritten generation")
				}
			}
			if !reflect.DeepEqual(RuntimeSemanticProofBatchUsage(ledger), RuntimeSemanticProofBatchUsage(reopened)) {
				t.Fatal("restart refunded proof budget")
			}
			usage := RuntimeSemanticProofBatchUsage(reopened)
			if len(usage) == 0 {
				t.Fatal("fixture has no selected proof batch")
			}
			for _, n := range usage {
				if n != count+1 {
					t.Fatalf("archival refunded selected batches: %v", usage)
				}
			}
			limits := &RuntimeExecutionLimits{MaxProofBatchesPerCWI: 1}
			if count > 0 && ValidateRuntimeSemanticProofBatchLimits(reopened, limits) == nil {
				t.Fatal("archive bypassed original proof budget")
			}
			cloned := ledger.Clone()
			if !reflect.DeepEqual(cloned.HistoryArchive, ledger.HistoryArchive) {
				t.Fatal("clone rewrote archive")
			}
			if count > 16 {
				cloned.HistoryArchive.Segments[0][0].Snapshot.Manifest.OwnerCatalogs[0].TargetRefs[0] = "different"
				if reflect.DeepEqual(cloned, ledger) || ledger.HistoryArchive.Segments[0][0].Snapshot.Manifest.OwnerCatalogs[0].TargetRefs[0] != "main.go" {
					t.Fatal("archive clone aliases original")
				}
				raw := cloneRuntimeSemanticPlanningLedgerRaw(ledger)
				raw.HistoryArchive.Segments[0][0].Snapshot.Manifest.Hash = "other"
				if ledger.HistoryArchive.Segments[0][0].Snapshot.Manifest.Hash == "other" {
					t.Fatal("raw clone aliases archive")
				}
			}
		})
	}
}

func TestRuntimeSemanticHistorySegmentsRejectCorruptionAcrossBoundary(t *testing.T) {
	base := semanticSegmentedHistoryFixture(t, 17)
	for name, mutate := range map[string]func(*RuntimeSemanticPlanningLedger){
		"generation count":           func(l *RuntimeSemanticPlanningLedger) { l.HistoryArchive.GenerationCount-- },
		"missing archive":            func(l *RuntimeSemanticPlanningLedger) { l.HistoryArchive = nil },
		"legacy schema with archive": func(l *RuntimeSemanticPlanningLedger) { l.SchemaVersion = RuntimeSemanticPlanningLedgerSchemaCurrent },
		"protocol":                   func(l *RuntimeSemanticPlanningLedger) { l.HistoryArchive.ProtocolRef = "unknown" },
		"short segment": func(l *RuntimeSemanticPlanningLedger) {
			l.HistoryArchive.Segments[0] = l.HistoryArchive.Segments[0][:15]
		},
		"missing tail":            func(l *RuntimeSemanticPlanningLedger) { l.History = nil },
		"tail overflow":           func(l *RuntimeSemanticPlanningLedger) { l.History = append(l.History, l.HistoryArchive.Segments[0]...) },
		"cross segment duplicate": func(l *RuntimeSemanticPlanningLedger) { l.History[0] = l.HistoryArchive.Segments[0][0].Clone() },
		"order":                   func(l *RuntimeSemanticPlanningLedger) { l.History[0].RetiredAtStateRevision = 1 },
		"body": func(l *RuntimeSemanticPlanningLedger) {
			l.HistoryArchive.Segments[0][0].Snapshot.Manifest.Hash = "forged"
		},
		"duplicate raw refs": func(l *RuntimeSemanticPlanningLedger) {
			l.HistoryArchive.Segments[0][0].PrerequisiteIDs = []string{"semantic-history-catalog", "semantic-history-catalog"}
		},
		"duplicate signature": func(l *RuntimeSemanticPlanningLedger) {
			l.History[0].InvalidationSignatures = l.HistoryArchive.Segments[0][0].InvalidationSignatures
		},
		"nested archive": func(l *RuntimeSemanticPlanningLedger) {
			l.HistoryArchive.Segments[0][0].Snapshot.HistoryArchive = &RuntimeSemanticHistoryArchive{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := cloneRuntimeSemanticPlanningLedgerRaw(base)
			mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("corrupt archived body/lineage accepted")
			}
		})
	}
}

func TestRuntimeSemanticHistoryRolloverDoesNotMutatePreimage(t *testing.T) {
	ledger := semanticSegmentedHistoryFixture(t, 16)
	before, _ := json.Marshal(ledger)
	g := ledger.History[15].Clone()
	g.RetiredAtStateRevision++
	g.Snapshot.LogicalRequestID = "next-request"
	g.InvalidationSignatures = []string{"next-invalidation"}
	rolled, err := ledger.AppendHistory(g)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("rollover rewrote detached preimage")
	}
	if err := rolled.Validate(); err != nil {
		t.Fatal(err)
	}
	g.Snapshot.Manifest.Hash = "mutated-caller"
	if rolled.History[0].Snapshot.Manifest.Hash == g.Snapshot.Manifest.Hash {
		t.Fatal("appended body aliases caller")
	}
}

func TestRuntimeSemanticHistoryArchiveScopeIsolation(t *testing.T) {
	a := semanticSegmentedHistoryFixture(t, 17)
	workspace, run := a.Manifest.WorkspaceID, a.Manifest.RunID
	if err := ValidateRuntimeSemanticHistoryScope(a, workspace, run); err != nil {
		t.Fatal(err)
	}
	b := a.Clone()
	b.HistoryArchive.Segments[0][0].Snapshot.Manifest.WorkspaceID = "other-workspace"
	if ValidateRuntimeSemanticHistoryScope(b, workspace, run) == nil {
		t.Fatal("cross-workspace archive accepted")
	}
	if err := ValidateRuntimeSemanticHistoryScope(a, workspace, run); err != nil {
		t.Fatal("another workspace mutated original archive", err)
	}
	b = a.Clone()
	b.HistoryArchive.Segments[0][0].Snapshot.Manifest.RunID = "other-run"
	if ValidateRuntimeSemanticHistoryScope(b, workspace, run) == nil {
		t.Fatal("cross-run archive accepted")
	}
}
