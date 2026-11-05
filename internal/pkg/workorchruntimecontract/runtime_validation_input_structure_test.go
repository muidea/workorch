package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestInputStructureExactSourceReplayAndAdmission(t *testing.T) {
	v, _ := inputRevisionFixture(t)
	f := v.InputFiles()[0]
	post := RuntimeValidationInputReviewFile{TargetRef: f.TargetRef, Content: "changed actual input", ContentHash: RuntimeValidationInputContentHash("changed actual input")}
	proof := RuntimeOperationReplayProof{OwnerModuleID: v.OwnerModuleID, OperationID: "edit", TargetRefs: []string{f.TargetRef}, BeforeHash: f.ContentHash, AfterHash: post.ContentHash, AdmissibilitySnapshotID: "catalog", AcquisitionBinding: "acquisition"}
	operation := RuntimeOperation{ID: "edit", Kind: StepKindFileEdit, TargetRefs: []string{f.TargetRef}}
	txn := AuthorityTransaction{AuthoritySnapshotID: "authority"}
	request := RuntimeInputStructureRequest(string(v.WorkspaceID), string(v.RunID), txn, "manifest", operation, v, proof, post)
	fact := RuntimeValidationInputStructureObservation{OwnerModuleID: RuntimeModuleShell, RequestHash: request.Hash(), Proof: proof, Postimage: post}
	if err := fact.ValidateForRequest(request); err != nil {
		t.Fatal(err)
	}
	contract, _ := RuntimeOperationContractForKind(operation.Kind)
	manifest := RuntimeCandidateContractManifest{Hash: "manifest", Operations: []RuntimeCandidateOperationManifest{{Kind: operation.Kind, OwnerModuleID: RuntimeModuleFile, InputRevision: v, Parameters: contract.Parameters, CandidateConstraints: []RuntimeOperationCandidateConstraint{RuntimeInputStructureConstraint("newText")}}}}
	record := RuntimeCandidateFeasibilityRecord{OperationID: "edit", AdmissibilitySnapshotID: "catalog", AcquisitionBinding: "acquisition", CandidateManifestHash: "manifest", Observation: &RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityFeasible}, ReplayProof: &proof, InputStructure: &fact}
	if err := ValidateRuntimeInputStructureRecord(manifest, operation, record, txn); err != nil {
		t.Fatal(err)
	}
	frozen := manifest.Clone()
	frozen.Operations[0].CandidateConstraints = nil
	original := record
	original.InputStructure = nil
	if err := ValidateRuntimeInputStructureRecord(frozen, operation, original, txn); err != nil {
		t.Fatal("changed already-frozen transaction", err)
	}
	if ValidateRuntimeInputStructureRecord(frozen, operation, record, txn) == nil {
		t.Fatal("undeclared fact became authority")
	}
	raw, _ := json.Marshal(record)
	var restored RuntimeCandidateFeasibilityRecord
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record, restored) || ValidateRuntimeInputStructureRecord(manifest, operation, restored, txn) != nil {
		t.Fatal("fresh load lost exact join")
	}
	for name, mutate := range map[string]func(*RuntimeValidationInputStructureRequest){
		"workspace":   func(r *RuntimeValidationInputStructureRequest) { r.WorkspaceID = "other" },
		"run":         func(r *RuntimeValidationInputStructureRequest) { r.RunID = "other" },
		"manifest":    func(r *RuntimeValidationInputStructureRequest) { r.CandidateManifestHash = "other" },
		"transaction": func(r *RuntimeValidationInputStructureRequest) { r.Transaction.AuthoritySnapshotID = "other" },
		"preimage":    func(r *RuntimeValidationInputStructureRequest) { r.Proof.BeforeHash = "stale" },
		"postimage":   func(r *RuntimeValidationInputStructureRequest) { r.Postimage.Content += "\n" },
		"target":      func(r *RuntimeValidationInputStructureRequest) { r.Operation.TargetRefs = []string{"other"} },
	} {
		t.Run(name, func(t *testing.T) {
			r := RuntimeInputStructureRequest(request.WorkspaceID, request.RunID, txn, "manifest", operation, v, proof, post)
			mutate(&r)
			if fact.ValidateForRequest(r) == nil {
				t.Fatal("stale/foreign facts accepted")
			}
		})
	}
	restored.InputStructure = nil
	if ValidateRuntimeInputStructureRecord(manifest, operation, restored, txn) == nil {
		t.Fatal("missing observation became feasible")
	}
	copied := cloneRuntimeCandidateFeasibilities([]RuntimeCandidateFeasibilityRecord{record})
	copied[0].InputStructure.Proof.TargetRefs[0] = "changed"
	if record.InputStructure.Proof.TargetRefs[0] != f.TargetRef {
		t.Fatal("clone aliases original fact")
	}
}
