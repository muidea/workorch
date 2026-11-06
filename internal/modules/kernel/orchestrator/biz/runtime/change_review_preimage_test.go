package runtime

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func auxiliaryChangePreimageFixture() (RuntimeAggregate, []rc.RuntimeValidationInputReviewFile) {
	a, files := changeReviewFixture()
	a.OperationReceipts = a.OperationReceipts[:1]
	a.CompletedDispatches = a.CompletedDispatches[:1]
	files = files[:1]
	before := rc.RuntimeValidationInputReviewFile{TargetRef: files[0].TargetRef, Content: "original existing target\r\n"}
	before.ContentHash = rc.RuntimeValidationInputContentHash(before.Content)
	a.OperationReceipts[0].BeforeHash = before.ContentHash
	d := &a.CompletedDispatches[0]
	d.OperationExecutions[0].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(a.OperationReceipts[0])
	d.CandidateHash = rc.StableRuntimeCandidateHash(d.Candidate)
	d.CandidateManifestHash = "original-manifest"
	manifest := &rc.RuntimeCandidateContractManifest{Hash: d.CandidateManifestHash, MutationInputCatalogs: []rc.RuntimeOperationAdmissibility{{ValidationSurfaces: []rc.RuntimeValidationSurface{{GenerationContext: &rc.RuntimeValidationGenerationContext{Files: []rc.RuntimeValidationInputReviewFile{before}}}}}}}
	a.CandidateAttempts = []rc.RuntimeCandidateAttemptRecord{{State: rc.RuntimeCandidateAttemptStateAccepted, DispatchID: d.ID, CandidateHash: d.CandidateHash, ManifestHash: d.CandidateManifestHash, LogicalRequestID: "original-request", Transaction: d.Transaction}}
	a.CandidateRequestSessions = []rc.RuntimeCandidateRequestSession{{LogicalRequestID: "original-request", DocumentKind: rc.RuntimeProviderDocumentOperations, ManifestHash: d.CandidateManifestHash, CandidateManifest: manifest, Transaction: d.Transaction}}
	return a, files
}

func TestChangeReviewAuxiliaryExistingTargetConsumesOriginalRequestPreimage(t *testing.T) {
	a, files := auxiliaryChangePreimageFixture()
	before, _ := json.Marshal(a)
	c, err := ChangeReviewContext(a, files)
	if err != nil || c.Changes[0].Baseline == nil || c.Changes[0].Baseline.Content != "original existing target\r\n" {
		t.Fatal("auxiliary existing-target preimage was already sealed on original candidate request", err)
	}
	after, _ := json.Marshal(a)
	if string(before) != string(after) {
		t.Fatal("query changed committed source")
	}
	var restored RuntimeAggregate
	if err = json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	again, err := ChangeReviewContext(restored, files)
	if err != nil || !reflect.DeepEqual(c, again) {
		t.Fatal("reload/duplicate changed source", err)
	}
	c.Changes[0].Baseline.Content = "caller mutation"
	if got, err := ChangeReviewContext(a, files); err != nil || got.Changes[0].Baseline.Content == "caller mutation" {
		t.Fatal("baseline aliases committed source", err)
	}
}

func TestChangeReviewAuxiliaryInputSubjectAndNewCurrentFact(t *testing.T) {
	a, files := auxiliaryChangePreimageFixture()
	surface := &a.CandidateRequestSessions[0].CandidateManifest.MutationInputCatalogs[0].ValidationSurfaces[0]
	original := surface.GenerationContext.Files[0]
	surface.GenerationContext = nil
	surface.ProofRequirementBindings = []rc.RuntimeValidationProofRequirementBinding{{InputReview: &rc.RuntimeValidationInputReviewSubject{Files: []rc.RuntimeValidationInputReviewFile{original}}}}
	files[0].Content = "new independently observed content"
	files[0].ContentHash = rc.RuntimeValidationInputContentHash(files[0].Content)
	c, err := ChangeReviewContext(a, files)
	if err != nil || c.Changes[0].Baseline == nil || *c.Changes[0].Baseline != original || c.Current[0] != files[0] {
		t.Fatal("current applicability rewrote original input preimage", err)
	}
	// A later legitimate transaction carries its own original source. It does
	// not reuse the previous query's baseline or any successful-query marker.
	changed := "new original preimage at a new transaction"
	original.Content = changed
	original.ContentHash = rc.RuntimeValidationInputContentHash(changed)
	surface.ProofRequirementBindings[0].InputReview.Files[0] = original
	a.CompletedDispatches[0].Transaction.FocusID = "new-transaction"
	a.CandidateAttempts[0].Transaction = a.CompletedDispatches[0].Transaction
	a.CandidateRequestSessions[0].Transaction = a.CompletedDispatches[0].Transaction
	a.OperationReceipts[0].BeforeHash = original.ContentHash
	a.CompletedDispatches[0].OperationExecutions[0].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(a.OperationReceipts[0])
	c, err = ChangeReviewContext(a, files)
	if err != nil || *c.Changes[0].Baseline != original {
		t.Fatal("new transaction reused stale preimage", err)
	}
}

func TestChangeReviewAuxiliaryPreimageRejectsWrongOrMissingOriginalSource(t *testing.T) {
	for _, name := range []string{"attempt-missing", "attempt-duplicate", "attempt-not-accepted", "attempt-transaction", "attempt-candidate", "attempt-manifest", "request-missing", "request-id", "request-duplicate", "request-kind", "request-transaction", "request-prefix", "request-manifest", "manifest-body", "content-missing", "content-tampered", "target", "workspace", "run", "current-catalog-is-not-original-source", "unrelated-request-is-not-original-source"} {
		t.Run(name, func(t *testing.T) {
			a, files := auxiliaryChangePreimageFixture()
			session := &a.CandidateRequestSessions[0]
			switch name {
			case "attempt-missing":
				a.CandidateAttempts = nil
			case "attempt-duplicate":
				a.CandidateAttempts = append(a.CandidateAttempts, a.CandidateAttempts[0])
			case "attempt-not-accepted":
				a.CandidateAttempts[0].State = rc.RuntimeCandidateAttemptStatePending
			case "attempt-transaction":
				a.CandidateAttempts[0].Transaction.FocusID = "different"
			case "attempt-candidate":
				a.CandidateAttempts[0].CandidateHash = "different"
			case "attempt-manifest":
				a.CandidateAttempts[0].ManifestHash = "different"
			case "request-missing":
				a.CandidateRequestSessions = nil
			case "request-id":
				session.LogicalRequestID = "other-workspace-request"
			case "request-duplicate":
				a.CandidateRequestSessions = append(a.CandidateRequestSessions, *session)
			case "request-kind":
				session.DocumentKind = rc.RuntimeProviderDocumentSemanticPlan
			case "request-transaction":
				session.Transaction.FocusID = "different"
			case "request-prefix":
				session.StablePrefixHash = "different"
			case "request-manifest":
				session.ManifestHash = "different"
			case "manifest-body":
				session.CandidateManifest.Hash = "different"
			case "content-missing":
				session.CandidateManifest.MutationInputCatalogs = nil
			case "content-tampered":
				session.CandidateManifest.MutationInputCatalogs[0].ValidationSurfaces[0].GenerationContext.Files[0].Content += "altered"
			case "target":
				session.CandidateManifest.MutationInputCatalogs[0].ValidationSurfaces[0].GenerationContext.Files[0].TargetRef = "other-resource"
			case "workspace":
				a.CompletedDispatches[0].WorkspaceID = "other"
			case "run":
				a.CompletedDispatches[0].RunID = "other"
			case "current-catalog-is-not-original-source":
				a.Facts.Resources.OperationAdmissibilities = session.CandidateManifest.MutationInputCatalogs
				session.CandidateManifest.MutationInputCatalogs = nil
			case "unrelated-request-is-not-original-source":
				other := *session
				other.LogicalRequestID = "unrelated"
				a.CandidateRequestSessions = []rc.RuntimeCandidateRequestSession{other}
			}
			if _, err := ChangeReviewContext(a, files); err == nil {
				t.Fatal("invalid auxiliary preimage source accepted")
			}
		})
	}
}

func auxiliaryChangeGovernanceFixture() (RuntimeAggregate, []rc.RuntimeValidationInputReviewFile) {
	a, files := auxiliaryChangePreimageFixture()
	a.CompletedDispatches[0].State = rc.RuntimeDispatchStateCommitted
	source := a.CompletedDispatches[0].Clone()
	resume := source.Clone()
	resume.ID = "governance-continuation"
	resume.Transaction.FocusID = "continuation-transaction"
	a.OperationReceipts[0].ExecutionKey += "/continuation"
	resume.OperationExecutions[0].ExecutionKey = a.OperationReceipts[0].ExecutionKey
	resume.OperationExecutions[0].OwnerReceiptRef = "runtime-receipt/" + a.OperationReceipts[0].ExecutionKey
	resume.OperationExecutions[0].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(a.OperationReceipts[0])
	a.CompletedDispatches = append(a.CompletedDispatches, resume)
	a.OperationGovernanceBindings = []rc.RuntimeOperationGovernanceBinding{{OperationDispatchID: source.ID, State: rc.RuntimeOperationGovernanceBindingGranted, Grant: &rc.RuntimeOperationGovernanceGrant{ResumeDispatchID: resume.ID}}}
	return a, files
}

func TestChangeReviewAuxiliaryGovernanceConsumesOriginalAdmission(t *testing.T) {
	for _, name := range []string{"exact", "two-hops", "missing-source", "wrong-candidate", "wrong-manifest", "wrong-workspace", "uncommitted-source", "duplicate-grant", "cycle"} {
		t.Run(name, func(t *testing.T) {
			a, files := auxiliaryChangeGovernanceFixture()
			switch name {
			case "two-hops":
				next := a.CompletedDispatches[1].Clone()
				next.ID += "/next"
				next.Transaction.FocusID = "next-transaction"
				a.CompletedDispatches = append(a.CompletedDispatches, next)
				// Only the last dispatch actually executed the selected effect.
				a.CompletedDispatches[1].OperationExecutions = nil
				a.OperationGovernanceBindings = append(a.OperationGovernanceBindings, rc.RuntimeOperationGovernanceBinding{OperationDispatchID: a.CompletedDispatches[1].ID, State: rc.RuntimeOperationGovernanceBindingGranted, Grant: &rc.RuntimeOperationGovernanceGrant{ResumeDispatchID: next.ID}})
			case "missing-source":
				a.OperationGovernanceBindings[0].OperationDispatchID = "missing"
			case "wrong-candidate":
				a.CompletedDispatches[0].CandidateHash = "different"
			case "wrong-manifest":
				a.CompletedDispatches[0].CandidateManifestHash = "different"
			case "wrong-workspace":
				a.CompletedDispatches[0].WorkspaceID = "other"
			case "uncommitted-source":
				a.CompletedDispatches[0].State = rc.RuntimeDispatchStateSealed
			case "duplicate-grant":
				a.OperationGovernanceBindings = append(a.OperationGovernanceBindings, a.OperationGovernanceBindings[0])
			case "cycle":
				a.OperationGovernanceBindings[0].OperationDispatchID = a.CompletedDispatches[1].ID
			}
			c, err := ChangeReviewContext(a, files)
			if name == "exact" || name == "two-hops" {
				if err != nil || c.Changes[0].Baseline == nil || c.Changes[0].Baseline.Content != "original existing target\r\n" {
					t.Fatal("committed continuation lost original preimage", err)
				}
			} else if err == nil {
				t.Fatal("invalid governance source accepted")
			}
		})
	}
}

func TestChangeReviewFixedRestartAuxiliaryPreimage(t *testing.T) {
	path := os.Getenv("WORKORCH_CHANGE_REVIEW_RESTART_ARCHIVE")
	if path == "" {
		t.Skip("explicit protected fixed owner archive only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState rc.RuntimeStateRecord }
	if err = json.Unmarshal(raw, &runs); err != nil {
		t.Fatal(err)
	}
	a, err := ReadCommittedRuntimeOwnerStateRecord(runs["run-002"].RuntimeState)
	if err != nil {
		t.Fatal(err)
	}
	var files []rc.RuntimeValidationInputReviewFile
	for _, catalog := range a.Facts.Resources.OperationAdmissibilities {
		for _, surface := range catalog.ValidationSurfaces {
			if surface.GenerationContext != nil {
				files = append(files, surface.GenerationContext.Files...)
			}
		}
	}
	c, err := ChangeReviewContext(a, files)
	if err != nil {
		t.Fatal("fixed original auxiliary source must be consumable", err)
	}
	if len(c.Changes) != 1 || c.Changes[0].Baseline == nil || c.Changes[0].TargetRef != "internal/gateway/server.go" || len(c.Changes[0].Baseline.Content) != 9325 {
		t.Fatal("fixed preimage incomplete")
	}
	m := *a.SemanticPlanning.Manifest
	work, err := SemanticWorkContext(a, m)
	if err != nil {
		t.Fatal(err)
	}
	semantic, err := SemanticChangeReviewContext(a, rc.RuntimeSemanticPlanRequestContext{PlanStep: rc.RuntimeSemanticPlanStepCoverageOrder, WorkContext: work}, m)
	if err != nil || semantic == nil || !reflect.DeepEqual(c.Changes, semantic.Changes) {
		t.Fatal("actual failing semantic stage lost original preimage", err)
	}
	candidate, err := CandidateChangeReviewContext(a, rc.RuntimeCandidateContext{RequiredState: rc.ConvergenceStateNeedStateChange, ImplementationWorkContent: work.Content})
	if err != nil || candidate == nil || !reflect.DeepEqual(c.Changes, candidate.Changes) {
		t.Fatal("candidate consumer differs from semantic consumer", err)
	}
}
