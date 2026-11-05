package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func validationExecutionUseFixture() (RuntimeCWIReconciliationRequest, RuntimeCWIReconciliationBatch) {
	request := RuntimeCWIReconciliationRequest{RequestID: "request", WorkspaceID: "workspace", RunID: "run", Generation: 1, Scope: RuntimeCWIReconciliationScopeCurrentRun, DefinitionRevision: "definition", WorkspaceBaselineRevision: "tree", WorkspaceBaselineRef: "workspace-tree/tree"}
	var observations []RuntimeCWIReconciliationObservation
	for i := 0; i < 2; i++ {
		v, a := validationExecutionConjunctionFixture()
		native := fmt.Sprintf("native-%d", i)
		v.ExecutedChecks = []string{native}
		v.PassedChecks = []string{native}
		v.ProofClass = RuntimeValidationProofBehavioral
		a.NativeCheckRef = native
		for j := range v.ExecutionConjunction.Stages {
			s := &v.ExecutionConjunction.Stages[j]
			prefix := "shared"
			if j > 0 {
				prefix = fmt.Sprintf("consumer-%d", i)
			}
			s.Stdout.Storage = &RuntimeValidationOutputStorage{Reference: prefix + "/stdout"}
			s.Stderr.Storage = &RuntimeValidationOutputStorage{Reference: prefix + "/stderr"}
		}
		b := RuntimeCWIReconciliationBindingRef{CoverageWorkItemID: common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", i)), BindingID: fmt.Sprintf("binding-%d", i), BindingHash: fmt.Sprintf("binding-hash-%d", i), OwnerModuleIDs: []string{RuntimeModuleShell}, AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, RequiredEvidenceKinds: []string{EvidenceKindValidation}, TargetRefs: []string{"implementation"}, ValidationRecipeIDs: []string{v.RecipeID}, RequiredCheckRefs: []string{a.RequiredCheckRef}, ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{a}}
		request.Bindings = append(request.Bindings, b)
		observations = append(observations, RuntimeCWIReconciliationObservation{RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID, Generation: 1, CoverageWorkItemID: b.CoverageWorkItemID, BindingID: b.BindingID, BindingHash: b.BindingHash, OwnerModuleID: RuntimeModuleShell, ObservedRevision: "tree", Status: RuntimeCWIReconciliationObservationSatisfied, TargetRefs: b.TargetRefs, CheckRefs: b.RequiredCheckRefs, ValidationExecution: &RuntimeValidationExecutionUse{SourceReceiptHash: fmt.Sprintf("receipt-%d", i), SourceScopeHash: fmt.Sprintf("scope-%d", i), ConsumerBindingRefs: []string{"binding-0", "binding-1"}, SharedStageCount: 1, Validation: v}})
	}
	contract := RuntimeCWIReconciliationContract{SchemaVersion: RuntimeCWIReconciliationContractSchemaCurrent, DefinitionRevision: request.DefinitionRevision, Bindings: request.Bindings}
	request.BindingSetHash = StableRuntimeCWIReconciliationContractHash(contract)
	for i := range observations {
		observations[i].ValidationExecution.GroupID = RuntimeValidationExecutionGroupID(request.WorkspaceID, request.RunID, request.RequestID, request.Generation, request.BindingSetHash, request.Bindings)
	}
	batch := RuntimeCWIReconciliationBatch{RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID, Generation: 1, BindingSetHash: request.BindingSetHash, Observations: observations}
	batch.Hash = StableRuntimeCWIReconciliationBatchHash(batch)
	return request, batch
}

func TestValidationExecutionUseRequiresCompleteExactFanoutAndSavedOutputs(t *testing.T) {
	request, batch := validationExecutionUseFixture()
	if err := batch.ValidateForRequest(request); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"nil-conjunction", "negative-count", "oversized-count", "missing-consumer", "foreign-consumer", "duplicate-consumer", "reordered-consumers", "cross-workspace", "cross-request", "cross-generation", "raw-missing", "raw-omitted", "behavior-shared", "native-borrowed", "unclassified", "prefix-output", "prefix-time"} {
		t.Run(kind, func(t *testing.T) {
			r, b := request.Clone(), batch.Clone()
			u := b.Observations[1].ValidationExecution
			switch kind {
			case "nil-conjunction":
				u.Validation.ExecutionConjunction = nil
			case "negative-count":
				u.SharedStageCount = -1
			case "oversized-count":
				u.SharedStageCount = 100
			case "missing-consumer":
				b.Observations = b.Observations[1:]
			case "foreign-consumer":
				u.ConsumerBindingRefs[0] = "foreign"
			case "duplicate-consumer":
				u.ConsumerBindingRefs[0] = u.ConsumerBindingRefs[1]
			case "reordered-consumers":
				u.ConsumerBindingRefs[0], u.ConsumerBindingRefs[1] = u.ConsumerBindingRefs[1], u.ConsumerBindingRefs[0]
			case "cross-workspace":
				r.WorkspaceID = "other"
			case "cross-request":
				r.RequestID = "other"
			case "cross-generation":
				r.Generation++
			case "raw-missing":
				u.Validation.ExecutionConjunction.Stages[0].Stdout.Storage = nil
			case "raw-omitted":
				u.Validation.ExecutionConjunction.Stages[0].Stdout.Storage = &RuntimeValidationOutputStorage{OmittedReason: "not-saved"}
			case "behavior-shared":
				u.SharedStageCount = 2
			case "native-borrowed":
				u.Validation.PassedChecks = []string{"native-0"}
			case "unclassified":
				u.Validation.ProofClass = ""
			case "prefix-output":
				u.Validation.ExecutionConjunction.Stages[0].Stdout.Storage.Reference = "replaced"
			case "prefix-time":
				u.Validation.ExecutionConjunction.Stages[0].StartedAt = u.Validation.ExecutionConjunction.Stages[0].FinishedAt
			}
			// Rehashing a forged fact cannot repair its source or close a missing peer.
			b.Hash = StableRuntimeCWIReconciliationBatchHash(b)
			if b.ValidateForRequest(r) == nil {
				t.Fatal("invalid group admitted")
			}
		})
	}
}

func TestValidationExecutionUseCloneHashAndJSONPreserveOriginalFacts(t *testing.T) {
	request, batch := validationExecutionUseFixture()
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeCWIReconciliationBatch
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.ValidateForRequest(request); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, batch) {
		t.Fatal("original execution facts changed on reload")
	}
	for _, kind := range []string{"receipt", "scope", "consumers", "argv", "raw", "time"} {
		changed := batch.Clone()
		u := changed.Observations[0].ValidationExecution
		switch kind {
		case "receipt":
			u.SourceReceiptHash = "other"
		case "scope":
			u.SourceScopeHash = "other"
		case "consumers":
			u.ConsumerBindingRefs[0] = "other"
		case "argv":
			u.Validation.ExecutionConjunction.Stages[0].Argv[0] = "other"
		case "raw":
			u.Validation.ExecutionConjunction.Stages[0].Stdout.Storage.Reference = "other"
		case "time":
			u.Validation.ExecutionConjunction.Stages[0].StartedAt = "other"
		}
		if StableRuntimeCWIReconciliationBatchHash(changed) == batch.Hash || StableRuntimeCWIReconciliationObservationRef(changed.Observations[0]) == StableRuntimeCWIReconciliationObservationRef(batch.Observations[0]) {
			t.Fatal("execution use omitted from lineage", kind)
		}
		after, _ := json.Marshal(batch)
		if string(after) != string(body) {
			t.Fatal("clone mutated original", kind)
		}
	}
}
