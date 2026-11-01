package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
)

func workContentFixture() (RuntimeCandidateContext, RuntimeCandidateContractManifest) {
	snapshot := func(target, body string) TargetSnapshot {
		h := sha256.Sum256([]byte(body))
		return TargetSnapshot{TargetRef: target, Content: body, ContentHash: hex.EncodeToString(h[:]), Size: int64(len(body)), ContentState: SnapshotContentStateComplete}
	}
	c := RuntimeCandidateContext{ImplementationWorkContent: &RuntimeImplementationWorkContent{ProtocolRef: RuntimeImplementationWorkContentV1, ManifestRevision: "frozen-tree", Snapshots: []TargetSnapshot{snapshot("main.txt", "main原文\n"), snapshot("large.txt", "unselected original\n")}}}
	m := RuntimeCandidateContractManifest{RequiredOperationSequence: []RuntimeRequiredOperationVariant{{RequirementID: "edit", TargetRef: "main.txt"}, {RequirementID: "create", TargetRef: "new.txt"}}, ImplementationWorkingBinding: &RuntimeImplementationWorkingBinding{IntentBinding: &RuntimeImplementationWorkIntentBinding{Source: RuntimeImplementationWorkPurposeChoice{ScopeRequestID: "scope-a"}, Intent: &RuntimeImplementationIntent{ContentHash: c.ImplementationWorkContent.Hash()}}, Proposals: []RuntimeSemanticProofProposal{{Target: RuntimeOperationTargetAuthorization{ProspectiveTargetRefs: []string{"new.txt"}}}}}}
	return c, m
}

func TestCandidateWorkContentExactSelectedBytesAndReload(t *testing.T) {
	c, m := workContentFixture()
	before, _ := json.Marshal(c)
	got, err := ProjectRuntimeCandidateWorkContent(c, m)
	if err != nil || len(got.Snapshots) != 1 || got.Snapshots[0].Content != "main原文\n" || !reflect.DeepEqual(got.AbsentTargets, []string{"new.txt"}) {
		t.Fatalf("projection=%+v err=%v", got, err)
	}
	encoded, _ := json.Marshal(c)
	var reload RuntimeCandidateContext
	if err := json.Unmarshal(encoded, &reload); err != nil {
		t.Fatal(err)
	}
	again, err := ProjectRuntimeCandidateWorkContent(reload, m)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatal("restart changed frozen input", err)
	}
	got.Snapshots[0].Content = "mutation"
	after, _ := json.Marshal(c)
	if string(after) != string(before) {
		t.Fatal("projection aliased owner source")
	}
	normalized := c.Normalized()
	normalized.ImplementationWorkContent.Snapshots[0].Content = "mutation"
	if c.ImplementationWorkContent.Snapshots[0].Content != "main原文\n" {
		t.Fatal("normalized transport aliased source")
	}
}

func TestCandidateWorkContentRejectsMissingChangedForeignAndIncomplete(t *testing.T) {
	cases := map[string]func(*RuntimeCandidateContext, *RuntimeCandidateContractManifest){
		"missing": func(c *RuntimeCandidateContext, m *RuntimeCandidateContractManifest) {
			c.ImplementationWorkContent = nil
		},
		"changed": func(c *RuntimeCandidateContext, m *RuntimeCandidateContractManifest) {
			c.ImplementationWorkContent.Snapshots[0].Content += "drift"
		},
		"foreign": func(c *RuntimeCandidateContext, m *RuntimeCandidateContractManifest) {
			m.ImplementationWorkingBinding.IntentBinding.Intent.ContentHash = "other-workspace"
		},
		"incomplete": func(c *RuntimeCandidateContext, m *RuntimeCandidateContractManifest) {
			c.ImplementationWorkContent.Snapshots[0].ContentState = "unavailable"
			m.ImplementationWorkingBinding.IntentBinding.Intent.ContentHash = c.ImplementationWorkContent.Hash()
		},
		"duplicate": func(c *RuntimeCandidateContext, m *RuntimeCandidateContractManifest) {
			c.ImplementationWorkContent.Snapshots = append(c.ImplementationWorkContent.Snapshots, c.ImplementationWorkContent.Snapshots[0])
			m.ImplementationWorkingBinding.IntentBinding.Intent.ContentHash = c.ImplementationWorkContent.Hash()
		},
		"absent existing target": func(c *RuntimeCandidateContext, m *RuntimeCandidateContractManifest) {
			m.RequiredOperationSequence[0].TargetRef = "missing-existing.txt"
		},
		"unexpected": func(c *RuntimeCandidateContext, m *RuntimeCandidateContractManifest) {
			m.ImplementationWorkingBinding = nil
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c, m := workContentFixture()
			mutate(&c, &m)
			if _, err := ProjectRuntimeCandidateWorkContent(c, m); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}
