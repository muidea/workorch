package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The rejected document never becomes a Proposal. The correction must come
// from the strict wire diagnostic, survive a real serialization boundary, and
// pass the existing private admission transaction before it can be dispatched.
func TestCandidateParameterCardinalityCorrectionRestartAndBound(t *testing.T) {
	ctx := context.Background()
	initial := stateChangeAggregate(t)
	repository := &failureAuthorityCASRepository{aggregate: initial}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	ws, run := initial.Facts.WorkspaceID, initial.Facts.RunID
	pending, err := controller.BeginCandidateAttemptForTest(ctx, ws, run, 0, "cardinality-prefix")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(pending)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimeTestBindCandidateToCurrentFocus(t, pending, stateChangeCandidate())
	raw, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	var full map[string]json.RawMessage
	json.Unmarshal(raw, &full)
	var ops []map[string]json.RawMessage
	json.Unmarshal(full["operations"], &ops)
	// Duplicate content, leaving patchType and baseHash frozen. Values differ
	// so ordinary uniqueItems would incorrectly accept this document.
	var params []map[string]any
	json.Unmarshal(ops[0]["parameters"], &params)
	params = append(params, map[string]any{"name": "content", "string": "second intended change\n"})
	ops[0]["parameters"], _ = json.Marshal(params)
	full["operations"], _ = json.Marshal(ops)
	rejected, _ := json.Marshal(full)
	_, err = runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, rejected)
	var diagnostic *runtimecontract.RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.ParameterCardinality == nil {
		t.Fatalf("strict rejection lost facts: %v", err)
	}
	failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "cardinality-fixture", Reason: "repeated typed parameter entries", Diagnostic: runtimecontract.RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(diagnostic)}
	failed, err := controller.failCandidateAttempt(ctx, ws, run, 0, failure)
	if err != nil {
		t.Fatal(err)
	}
	last := failed.CandidateAttempts[len(failed.CandidateAttempts)-1]
	if last.Proposal != nil || last.Correction == nil || last.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || last.Correction.Witness == nil {
		t.Fatalf("no bounded correction: %+v", last)
	}
	if failed.ActiveDispatch != nil || !reflect.DeepEqual(failed.OperationReceipts, pending.OperationReceipts) || !reflect.DeepEqual(failed.Facts.Coverage, pending.Facts.Coverage) || !reflect.DeepEqual(failed.ExecutionBudget, pending.ExecutionBudget) {
		t.Fatal("schema rejection created effects, credit or capability budget usage")
	}
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{Class: last.Failure.Class, Code: last.Failure.Code, Signature: last.Failure.Signature, Diagnostic: last.Failure.Diagnostic, SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(manifest.Hash, last.Correction.Objectives, last.Correction.Witness)}
	consumer := llmmodels.RuntimeCandidateCorrectionInput{ManifestHash: manifest.Hash, Objectives: last.Correction.Objectives, Witness: *last.Correction.Witness}
	if err := consumer.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	if !candidateCrossesCorrectionWitness(candidate, manifest, *last.Correction.Witness) {
		t.Fatal("legal complete replacement did not cross bounded witness")
	}
	var changed runtimecontract.RuntimeCandidate
	copyBytes, _ := json.Marshal(candidate)
	if err := json.Unmarshal(copyBytes, &changed); err != nil {
		t.Fatal(err)
	}
	for i := range changed.StateChange.Operations[0].Operation.Parameters {
		p := &changed.StateChange.Operations[0].Operation.Parameters[i]
		if p.Name == "baseHash" {
			value := "foreign"
			p.String = &value
		}
	}
	if candidateCrossesCorrectionWitness(changed, manifest, *last.Correction.Witness) {
		t.Fatal("frozen preimage changed through witness")
	}
	if err := validatePersistedCandidateCorrection(failed, manifest); err != nil {
		t.Fatal(err)
	}
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "admit after restart", true: "retry exhausted"}[terminal], func(t *testing.T) {
			copy, err := cloneFailureAuthorityAggregate(failed)
			if err != nil {
				t.Fatal(err)
			}
			repo := &failureAuthorityCASRepository{aggregate: copy}
			restarted, _ := NewRuntimeController(repo)
			if _, err := restarted.Load(ctx, ws, run); err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.BeginCandidateAttemptForTest(ctx, ws, run, 1, "cardinality-prefix"); err != nil {
				t.Fatal(err)
			}
			if terminal {
				ended, err := restarted.failCandidateAttempt(ctx, ws, run, 1, failure)
				if err != nil {
					t.Fatal(err)
				}
				if ended.Decision.Terminal == nil || ended.Decision.Terminal.Code != "candidate_retry_exhausted" || ended.ActiveDispatch != nil {
					t.Fatalf("retry failed open: %+v", ended.Decision)
				}
				writes := repo.saveAttempts
				if _, err := restarted.BeginCandidateAttemptForTest(ctx, ws, run, 2, "cardinality-prefix"); err == nil || repo.saveAttempts != writes {
					t.Fatal("third attempt reopened authority")
				}
				return
			}
			if err := RecordFixtureCandidateFeasibilities(ctx, restarted, ws, run, 1, candidate); err != nil {
				t.Fatal(err)
			}
			result, err := restarted.AdmitAndSealCandidateAttemptForTest(ctx, ws, run, 1, candidate)
			if err != nil || !result.Admission.Accepted() || result.Aggregate.ActiveDispatch == nil {
				t.Fatalf("corrected dispatch: %+v err=%v", result.Admission, err)
			}
			if len(result.Aggregate.OperationReceipts) != len(pending.OperationReceipts) || !reflect.DeepEqual(result.Aggregate.Facts.Coverage, pending.Facts.Coverage) {
				t.Fatal("dispatch sealing manufactured execution credit")
			}
			writes := repo.saveAttempts
			_, err = restarted.failCandidateAttempt(ctx, ws, run, 0, failure)
			if !errors.Is(err, common.ErrConflict) || repo.saveAttempts != writes {
				t.Fatalf("stale rejection reopened correction: %v", err)
			}
		})
	}
	for name, mode := range map[string]failureAuthorityCASMode{"before write": failureAuthorityCASFailBeforeWrite, "lost response": failureAuthorityCASLoseResponse} {
		t.Run(name, func(t *testing.T) {
			repo := &failureAuthorityCASRepository{aggregate: pending, mode: mode}
			c, _ := NewRuntimeController(repo)
			_, err := c.failCandidateAttempt(ctx, ws, run, 0, failure)
			if !errors.Is(err, errFailureAuthorityCASCut) {
				t.Fatalf("missing injected failure: %v", err)
			}
			if mode == failureAuthorityCASFailBeforeWrite && !reflect.DeepEqual(repo.aggregate, pending) {
				t.Fatal("pre-write cut changed authority")
			}
			repo.mode = failureAuthorityCASNormal
			if mode == failureAuthorityCASFailBeforeWrite {
				if _, err := c.failCandidateAttempt(ctx, ws, run, 0, failure); err != nil {
					t.Fatal(err)
				}
			}
			restarted, _ := NewRuntimeController(repo)
			loaded, err := restarted.Load(ctx, ws, run)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.StateRevision != failed.StateRevision || !reflect.DeepEqual(loaded.CandidateAttempts, failed.CandidateAttempts) {
				t.Fatal("CAS recovery changed original correction identity")
			}
			if _, err := restarted.Load(ctx, "foreign-workspace", run); !errors.Is(err, common.ErrNotFound) {
				t.Fatal("scope isolation lost")
			}
		})
	}
}
