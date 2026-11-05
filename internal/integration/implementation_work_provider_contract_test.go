package integration_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A permissive mock could silently hide a renderer regression even while the
// full public flow eventually completes. The mock consumes only the current
// published focus and evidence; it cannot infer either from prior calls.
func TestImplementationWorkProviderRequiresCurrentFocusAndEvidence(t *testing.T) {
	focus := runtimecontract.CoverageFocus{ID: "current-focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"}}
	for _, arm := range []string{"current", "missing_ref", "unknown_ref", "inline_only", "duplicate_inline", "missing_focus", "missing_evidence"} {
		t.Run(arm, func(t *testing.T) {
			decision := map[string]any{"focusRef": "context.focus"}
			context := map[string]any{"focus": focus, "allowedAcceptedEvidenceRefs": []string{"accepted-owner-evidence"}}
			switch arm {
			case "missing_ref":
				delete(decision, "focusRef")
			case "unknown_ref":
				decision["focusRef"] = "old.focus"
			case "inline_only":
				delete(decision, "focusRef")
				decision["focus"] = focus
			case "duplicate_inline":
				decision["focus"] = focus
			case "missing_focus":
				delete(context, "focus")
			case "missing_evidence":
				delete(context, "allowedAcceptedEvidenceRefs")
			}
			raw := map[string]json.RawMessage{}
			for key, value := range map[string]any{"decision": decision, "context": context, "outputContract": map[string]any{"state": "need_assessment"}} {
				b, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				raw[key] = b
			}
			before, _ := json.Marshal(raw)
			provider := &implementationWorkFullflowProvider{}
			payload, _, err := provider.candidate(raw)
			if arm != "current" {
				if err == nil || len(payload) != 0 {
					t.Fatal("mock supplied an assessment despite a missing or ambiguous input")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var assessment struct {
				FocusID             string
				CoverageWorkItemIDs []common.CoverageWorkItemID
				Items               []struct {
					CoverageWorkItemID common.CoverageWorkItemID
					EvidenceRefs       []string
				}
			}
			if err := json.Unmarshal(payload, &assessment); err != nil {
				t.Fatal(err)
			}
			if assessment.FocusID != focus.ID || !reflect.DeepEqual(assessment.CoverageWorkItemIDs, focus.CoverageWorkItemIDs) || len(assessment.Items) != 2 {
				t.Fatal("mock changed the exact current focus")
			}
			for i, item := range assessment.Items {
				if item.CoverageWorkItemID != focus.CoverageWorkItemIDs[i] || !reflect.DeepEqual(item.EvidenceRefs, []string{"accepted-owner-evidence"}) {
					t.Fatal("mock invented evidence or changed a consumer")
				}
			}
			after, _ := json.Marshal(raw)
			if string(before) != string(after) || len(provider.calls) != 0 || len(provider.failures) != 0 {
				t.Fatal("pure mock replay modified the request or performed a call")
			}
		})
	}
}

// These are distinct observations, never a replacement of the failure poll.
func TestImplementationWorkFailureSnapshotsKeepOriginalAndStoppedOwnerSeparate(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp/workorch-validation", "snapshot-contract-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	target := filepath.Join(dir, "workorch-public-work-original.json")
	original := &runmodel.Run{ID: "run-test", Revision: 2, FailureReason: "原始失败\x00完整"}
	originalBytes, _ := json.Marshal(map[string]*runmodel.Run{string(original.ID): original})
	if err := implementationWorkFullflowWriteFailureSnapshot(target, original); err != nil {
		t.Fatal(err)
	}
	stopped := *original
	stopped.Revision = 3
	if err := implementationWorkFullflowWriteFailureSnapshot(target+".after-stop.json", &stopped); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != string(originalBytes) {
		t.Fatal("stopped observation replaced original bytes", err)
	}
	after, err := os.ReadFile(target + ".after-stop.json")
	expected, _ := json.Marshal(map[string]*runmodel.Run{string(stopped.ID): &stopped})
	if err != nil || string(after) != string(expected) {
		t.Fatal("stopped observation lost owner data", err)
	}
	for _, entry := range []struct {
		path     string
		body     []byte
		revision uint64
	}{{target, body, original.Revision}, {target + ".after-stop.json", after, stopped.Revision}} {
		indexBody, err := os.ReadFile(entry.path + ".index.json")
		if err != nil {
			t.Fatal(err)
		}
		var index struct {
			Bytes                        int
			SHA256                       string
			RunID                        common.RunID
			RunRevision, RuntimeRevision uint64
		}
		if err := json.Unmarshal(indexBody, &index); err != nil {
			t.Fatal(err)
		}
		if index.Bytes != len(entry.body) || index.SHA256 != fmt.Sprintf("%x", sha256.Sum256(entry.body)) || index.RunID != original.ID || index.RunRevision != entry.revision || index.RuntimeRevision != 0 {
			t.Fatal("snapshot index does not identify exact original bytes/revisions")
		}
	}
	for _, rejected := range []string{filepath.Join(dir, "other.json"), "/tmp/workorch-public-work-outside.json", filepath.Join(dir, "..", "..", "workorch-public-work-escape.json")} {
		if err := implementationWorkFullflowWriteFailureSnapshot(rejected, original); err == nil {
			t.Fatal("invalid output boundary accepted", rejected)
		}
	}
}
