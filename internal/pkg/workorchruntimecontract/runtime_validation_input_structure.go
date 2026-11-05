package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"unicode/utf8"
)

const RuntimeValidationInputStructureConstraintRef = "validation/input-revision/postimage-structure"
const RuntimeOperationCandidateRequiredShapeInputStructure RuntimeOperationCandidateRequiredShape = "authorized_input_owner_structure"

// File supplies exact postimage bytes; the validation owner supplies only a
// structural observation. Neither fact is a review, receipt or acceptance.
type RuntimeValidationInputStructureRequest struct {
	WorkspaceID           string
	RunID                 string
	Transaction           AuthorityTransaction
	CandidateManifestHash string
	Operation             RuntimeOperation
	Revision              *RuntimeValidationInputRevision
	Proof                 RuntimeOperationReplayProof
	Postimage             RuntimeValidationInputReviewFile
}

type RuntimeValidationInputStructureObservation struct {
	RequestHash   string
	OwnerModuleID string
	Code          string
	CheckRef      string
	TargetRefs    []string
	Proof         RuntimeOperationReplayProof
	Postimage     RuntimeValidationInputReviewFile
}

func (r RuntimeValidationInputStructureRequest) Validate() error {
	if r.Revision == nil || r.Revision.Validate() != nil || r.WorkspaceID != string(r.Revision.WorkspaceID) || r.RunID != string(r.Revision.RunID) || r.Transaction.AuthoritySnapshotID == "" || r.CandidateManifestHash == "" || r.Operation.ID != r.Proof.OperationID || !r.Revision.MatchesReplayProof(&r.Proof) || len(r.Operation.TargetRefs) != 1 || !reflect.DeepEqual(r.Operation.TargetRefs, r.Proof.TargetRefs) || r.Postimage.TargetRef != r.Proof.TargetRefs[0] || r.Postimage.ContentHash != r.Proof.AfterHash {
		return fmt.Errorf("input structure request lost exact candidate/revision/File postimage")
	}
	h := sha256.Sum256([]byte(r.Postimage.Content))
	if !utf8.ValidString(r.Postimage.Content) || hex.EncodeToString(h[:]) != r.Postimage.ContentHash || len(r.Postimage.Content) > RuntimeValidationDiagnosticSourceMaxBytes {
		return fmt.Errorf("input structure postimage bytes/hash/capacity invalid")
	}
	maxBytes := RuntimeValidationDiagnosticSourceMaxBytes
	if r.Revision.Binding.InputReview != nil {
		_, maxBytes, _ = RuntimeValidationInputReviewBounds(*r.Revision.Binding.InputReview)
	}
	size := 0
	for _, f := range r.Revision.InputFiles() {
		if f.TargetRef == r.Postimage.TargetRef {
			size += len(r.Postimage.Content)
		} else {
			size += len(f.Content)
		}
	}
	if size > maxBytes {
		return fmt.Errorf("input structure postimage exceeds selected corpus capacity")
	}
	return nil
}

func (r RuntimeValidationInputStructureRequest) Hash() string {
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (o *RuntimeValidationInputStructureObservation) Clone() *RuntimeValidationInputStructureObservation {
	if o == nil {
		return nil
	}
	c := *o
	c.TargetRefs = append([]string(nil), o.TargetRefs...)
	c.Proof = o.Proof
	c.Proof.TargetRefs = append([]string(nil), o.Proof.TargetRefs...)
	return &c
}

func (o RuntimeValidationInputStructureObservation) ValidateForRequest(r RuntimeValidationInputStructureRequest) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if o.OwnerModuleID != RuntimeModuleShell || o.RequestHash != r.Hash() || !reflect.DeepEqual(o.Proof, r.Proof) || o.Postimage != r.Postimage || (o.Code == "" && (o.CheckRef != "" || len(o.TargetRefs) != 0)) {
		return fmt.Errorf("input structure observation is not bound to original request")
	}
	return nil
}

func RuntimeInputStructureRequest(workspaceID, runID string, transaction AuthorityTransaction, manifestHash string, operation RuntimeOperation, revision *RuntimeValidationInputRevision, proof RuntimeOperationReplayProof, postimage RuntimeValidationInputReviewFile) RuntimeValidationInputStructureRequest {
	return RuntimeValidationInputStructureRequest{WorkspaceID: workspaceID, RunID: runID, Transaction: transaction, CandidateManifestHash: manifestHash, Operation: operation.Normalized(), Revision: revision.Clone(), Proof: proof.Clone(), Postimage: postimage}
}

func RuntimeInputStructureConstraint(parameter string) RuntimeOperationCandidateConstraint {
	return RuntimeOperationCandidateConstraint{Ref: RuntimeValidationInputStructureConstraintRef + "/" + parameter, Kind: RuntimeOperationCandidateConstraintTargetApplicability, ParameterName: parameter, RequiredShape: RuntimeOperationCandidateRequiredShapeInputStructure}
}

// Applicability comes from the frozen candidate declaration, never from the
// current binary adding a requirement to an already issued transaction.
func RuntimeInputStructureConstraintForOperation(operation RuntimeOperation, authorization RuntimeCandidateOperationManifest) (RuntimeOperationCandidateConstraint, bool, error) {
	declared := false
	for _, c := range authorization.CandidateConstraints {
		declared = declared || c.RequiredShape == RuntimeOperationCandidateRequiredShapeInputStructure
	}
	for _, v := range authorization.Variants {
		for _, c := range v.CandidateConstraints {
			declared = declared || c.RequiredShape == RuntimeOperationCandidateRequiredShapeInputStructure
		}
	}
	if !declared {
		return RuntimeOperationCandidateConstraint{}, false, nil
	}
	_, constraints, err := RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
	if err != nil {
		return RuntimeOperationCandidateConstraint{}, false, err
	}
	var result RuntimeOperationCandidateConstraint
	for _, c := range constraints {
		if c.RequiredShape == RuntimeOperationCandidateRequiredShapeInputStructure {
			if result.Ref != "" {
				return result, false, fmt.Errorf("ambiguous input structure constraint")
			}
			result = c
		}
	}
	if result.Ref != "" && authorization.InputRevision == nil {
		return result, false, fmt.Errorf("input structure constraint lacks original revision")
	}
	return result, result.Ref != "", nil
}

// Fresh-load and admission share the same reverse join. Absence is not a PASS;
// failed structural candidates retain the File/Shell fact but no replay grant.
func ValidateRuntimeInputStructureRecord(m RuntimeCandidateContractManifest, operation RuntimeOperation, record RuntimeCandidateFeasibilityRecord, transaction AuthorityTransaction) error {
	var revision *RuntimeValidationInputRevision
	var required bool
	for _, a := range m.Operations {
		if a.Kind == operation.Kind {
			revision = a.InputRevision
			var err error
			_, required, err = RuntimeInputStructureConstraintForOperation(operation, a)
			if err != nil {
				return err
			}
		}
	}
	if !required {
		if record.InputStructure != nil {
			return fmt.Errorf("unrelated candidate carries input structure fact")
		}
		return nil
	}
	if record.Observation == nil {
		return fmt.Errorf("input structure requires owner outcome")
	}
	outcome := record.Observation.Outcome
	if outcome != RuntimeOwnerFeasibilityFeasible && (outcome != RuntimeOwnerFeasibilityCandidateConstraintViolated || record.Observation.CandidateConstraintViolation == nil || record.Observation.CandidateConstraintViolation.RequiredShape != RuntimeOperationCandidateRequiredShapeInputStructure) {
		if record.InputStructure != nil {
			return fmt.Errorf("input structure fact on unrelated failure")
		}
		return nil
	}
	fact := record.InputStructure
	if fact == nil {
		return fmt.Errorf("input revision lost frozen File/Shell structure observation")
	}
	request := RuntimeInputStructureRequest(string(revision.WorkspaceID), string(revision.RunID), transaction, m.Hash, operation, revision, fact.Proof, fact.Postimage)
	if err := fact.ValidateForRequest(request); err != nil {
		return err
	}
	if fact.Proof.OperationID != record.OperationID || fact.Proof.AdmissibilitySnapshotID != record.AdmissibilitySnapshotID || fact.Proof.AcquisitionBinding != record.AcquisitionBinding {
		return fmt.Errorf("input structure File source mismatch")
	}
	if outcome == RuntimeOwnerFeasibilityFeasible {
		if fact.Code != "" || record.ReplayProof == nil || !reflect.DeepEqual(*record.ReplayProof, fact.Proof) {
			return fmt.Errorf("input structure failure cannot become feasible")
		}
	} else if fact.Code == "" || record.ReplayProof != nil {
		return fmt.Errorf("input structure failure missing original rejection")
	}
	return nil
}
