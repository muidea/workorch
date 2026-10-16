package file

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileMutationSeparatesActualHistoricalAndPostconditionReads(t *testing.T) {
	for _, variant := range []string{patchTypeFileWrite, patchTypeTextReplace, patchTypeUnifiedDiff} {
		t.Run(variant, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "target.txt")
			before, after := []byte("旧原文\nbefore\n"), []byte("旧原文\nafter\n")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			request := PatchApplyRequest{WorkspaceDirectory: workspace, TargetRef: "target.txt", PatchType: variant,
				Content: string(after), ContentPresent: true, OldText: "before", NewText: "after", NewTextPresent: true,
				Diff: "--- a/target.txt\n+++ b/target.txt\n@@ -1,2 +1,2 @@\n 旧原文\n-before\n+after\n", BaseHash: sha256Hex(before)}
			ret, err := NewRuntimeAdapter().ApplyPatch(request)
			if err != nil || ret.patchRead == nil || ret.patchPostcondition == nil {
				t.Fatalf("missing actual pair: %v %+v", err, ret)
			}
			if ret.patchRead.ContentHash != sha256Hex(before) || ret.patchPostcondition.ContentHash != sha256Hex(after) || ret.patchPostcondition.Purpose != "mutation_postcondition" {
				t.Fatal("historical read was replaced, or expected content was presented as input")
			}
			scope := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: "ws", Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition"}}
			operation := runtimecontract.RuntimeOperation{ID: "mutation", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"target.txt"}}
			reads, err := filePatchOwnerDependencies(scope, operation, ret.patchRead)
			if err != nil {
				t.Fatal(err)
			}
			proof, err := filePatchApplicabilityDependencies(scope, operation, ret.patchPostcondition)
			if err != nil || runtimecontract.ValidateRuntimeEvidenceDependencyPair(reads, proof, "definition") != nil {
				t.Fatalf("pair: %v %+v", err, proof)
			}
			if proof.ProtocolRef != runtimecontract.RuntimeEvidenceApplicabilityDependenciesV1 || proof.PurposeRef != "" {
				t.Fatal("undeclared operation was upgraded to historical effect use")
			}
			historicalScope := scope
			historicalScope.HistoricalEffectProofContractRef = runtimecontract.RuntimeFileHistoricalEffectProofV1
			historical, err := filePatchApplicabilityDependencies(historicalScope, operation, ret.patchPostcondition)
			if err != nil || !reflect.DeepEqual(historical.OwnerRevisionSet, proof.OwnerRevisionSet) ||
				!runtimecontract.RuntimeEvidenceIsHistoricalEffect(runtimecontract.EvidenceFact{Kind: runtimecontract.EvidenceKindStateChange, OwnerRevisionSet: reads, ApplicabilityDependencies: historical}) {
				t.Fatal("owner purpose lost actual historical reads/postcondition")
			}
			historicalScope.HistoricalEffectProofContractRef = "unpublished-purpose"
			if _, err := filePatchApplicabilityDependencies(historicalScope, operation, ret.patchPostcondition); err == nil {
				t.Fatal("unknown owner purpose accepted")
			}
			body, err := json.Marshal(struct {
				WorkspaceID, TargetRef, ContentHash string
				Size                                int64
			}{"ws", "target.txt", sha256Hex(after), int64(len(after))})
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(append([]byte("file-target-content/v1\x00"), body...))
			entry := proof.OwnerRevisionSet.Entries[0]
			if entry.Revision != sha256Hex(after) || entry.DependencyHash != hex.EncodeToString(digest[:]) || entry.ResourceKind != "workspace_target_content" {
				t.Fatal("postcondition differs from independent actual-byte selector oracle")
			}
			changed := operation.Clone()
			changed.ID = "other-execution"
			equivalent, err := filePatchApplicabilityDependencies(scope, changed, ret.patchPostcondition)
			if err != nil || !reflect.DeepEqual(equivalent, proof) {
				t.Fatal("execution identity leaked into resource selector")
			}
			request.ReplayProof = &runtimecontract.RuntimeOperationReplayProof{OwnerModuleID: ModuleID, OperationID: "mutation", TargetRefs: []string{"target.txt"}, BeforeHash: sha256Hex(before), AfterHash: sha256Hex(after), AdmissibilitySnapshotID: "snapshot"}
			replay, err := NewRuntimeAdapter().ApplyPatch(request)
			if err != nil || replay.patchRead == nil || replay.patchRead.Purpose != "replay_postcondition" || replay.patchPostcondition == nil {
				t.Fatalf("replay actual pair: %v", err)
			}
			replayProof, err := filePatchApplicabilityDependencies(scope, operation, replay.patchPostcondition)
			if err != nil || !reflect.DeepEqual(replayProof, proof) {
				t.Fatal("recovery changed applicability selector")
			}
			request.ReplayProof = nil
			request.BaseHash = "wrong"
			request.Content = "different-not-a-recovery-postcondition"
			failed, err := NewRuntimeAdapter().ApplyPatch(request)
			if err == nil || failed.patchRead == nil || failed.patchPostcondition != nil {
				t.Fatal("rejected mutation supplied current postcondition proof")
			}
		})
	}
}
