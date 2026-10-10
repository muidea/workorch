package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeAssessmentProofDependenciesBindFullBodyAndDetachRawClones(t *testing.T) {
	dependencies := (RuntimeOwnerRevisionSet{DefinitionRevision: "definition", Entries: []RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: "owner/ws", ResourceKind: "workspace_target", ResourceID: "input",
		Revision: "original-bytes", DependencyHash: "original-selector",
	}}}).WithHash()
	original := (RuntimeAssessmentAdjudication{
		DispatchID: "assessment", AuthorityInputID: "authority", FocusID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"},
		Proofs:      []RuntimeAssessmentAdjudicationProof{{CoverageWorkItemID: "cwi", EvidenceKind: EvidenceKindValidation, EvidenceFactID: "evidence", EvidenceFactHash: "fact-hash", OwnerRevisionSet: &dependencies}},
		Disposition: AssessmentDispositionSatisfied,
	}).Normalized()
	if err := original.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeAssessmentAdjudication
	if err := json.Unmarshal(encoded, &loaded); err != nil || !reflect.DeepEqual(loaded, original) {
		t.Fatalf("assessment dependency reload: %v", err)
	}
	for name, clone := range map[string]RuntimeAssessmentAdjudication{
		"normalized": original.Normalized(), "clone": original.Clone(), "raw": cloneRuntimeAssessmentAdjudicationRaw(original), "reload": loaded,
	} {
		clone.Proofs[0].OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if original.Proofs[0].OwnerRevisionSet.Entries[0].Revision != "original-bytes" {
			t.Fatalf("%s shares mutable proof dependencies", name)
		}
	}
	for _, field := range []string{"definition", "owner", "resource", "revision", "snapshot", "fence", "selector", "hash", "missing"} {
		t.Run(field, func(t *testing.T) {
			changed := original.Clone()
			set := changed.Proofs[0].OwnerRevisionSet
			switch field {
			case "definition":
				set.DefinitionRevision = "other"
			case "owner":
				set.Entries[0].OwnerInstanceID = "other"
			case "resource":
				set.Entries[0].ResourceID = "other"
			case "revision":
				set.Entries[0].Revision = "other"
			case "snapshot":
				set.Entries[0].SnapshotID = "other"
			case "fence":
				set.Entries[0].Fence = 1
			case "selector":
				set.Entries[0].DependencyHash = "other"
			case "hash":
				set.SetHash = "forged"
			case "missing":
				changed.Proofs[0].OwnerRevisionSet = nil
			}
			// 保留旧 SetHash，证明 adjudication hash 绑定完整正文而非只消费 caller hash。
			if StableRuntimeAssessmentAdjudicationHash(changed) == StableRuntimeAssessmentAdjudicationHash(original) {
				t.Fatal("assessment hash ignores original dependency body")
			}
		})
	}
	for _, scenario := range []string{"raw", "duplicate", "conflict", "hash"} {
		t.Run(scenario, func(t *testing.T) {
			changed := original.Clone()
			set := changed.Proofs[0].OwnerRevisionSet
			switch scenario {
			case "raw":
				set.Entries[0].ResourceID = " input"
			case "duplicate":
				set.Entries = append(set.Entries, set.Entries[0])
			case "conflict":
				entry := set.Entries[0]
				entry.Revision = "conflicting"
				set.Entries = append(set.Entries, entry)
			case "hash":
				set.SetHash = "forged"
			}
			if !reflect.DeepEqual(changed.Proofs[0].OwnerRevisionSet, changed.Normalized().Proofs[0].OwnerRevisionSet) {
				t.Fatal("normalization rewrites raw dependency diagnostics")
			}
			if err := changed.ValidateRaw(); err == nil {
				t.Fatal("malformed proof dependencies accepted")
			}
		})
	}
	for _, entries := range [][]RuntimeOwnerRevisionRef{nil, {}} {
		raw := original.Clone()
		raw.Proofs[0].OwnerRevisionSet.Entries = entries
		for _, detached := range []RuntimeAssessmentAdjudication{raw.Normalized(), raw.Clone(), cloneRuntimeAssessmentAdjudicationRaw(raw)} {
			if !reflect.DeepEqual(detached.Proofs[0].OwnerRevisionSet.Entries, entries) {
				t.Fatal("raw nil/empty dependency representation lost")
			}
		}
	}
}
