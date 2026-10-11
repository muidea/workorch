package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func validationExecutionConjunctionFixture() (RuntimeValidationObservation, RuntimeValidationCheckAttestationBinding) {
	digest := sha256.Sum256(nil)
	empty := RuntimeValidationOutputIdentity{SHA256: hex.EncodeToString(digest[:]), Complete: true}
	source := StableRuntimeValidationExecutionStagesHash([]RuntimeValidationExecutionStageIdentity{{ID: "source", Scope: "owner-scope"}})
	identities := []RuntimeValidationExecutionStageIdentity{{ID: "owner-first", Scope: "complete-project", RequireTests: true}, {ID: "owner-second", Scope: "exact-reviewed-input", RequireTests: true, RequireBehavior: true}}
	conjunction := RuntimeValidationExecutionConjunction{ProtocolRef: "owner/opaque-all-of/v1", RequiredStagesHash: StableRuntimeValidationExecutionStagesHash(identities), BehaviorTargetRefs: []string{"implementation"}, Complete: true}
	for index, identity := range identities {
		start := time.Date(2026, 10, 11, 0, 0, index*2, 0, time.UTC)
		conjunction.Stages = append(conjunction.Stages, RuntimeValidationExecutionStage{Identity: identity, Argv: []string{"owner-tool", "exact-arg"}, StartedAt: start.Format(time.RFC3339Nano), FinishedAt: start.Add(time.Second).Format(time.RFC3339Nano), Started: true, TerminalComplete: true, Stdout: empty, Stderr: empty, SourceBefore: source, SourceAfter: source, Tests: 1, PassedTests: 1})
	}
	observation := RuntimeValidationObservation{RecipeID: "recipe", ObservationProtocolRef: conjunction.ProtocolRef, ExecutedChecks: []string{"native"}, PassedChecks: []string{"native"}, ExecutionConjunction: &conjunction}
	binding := RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "required", NativeCheckRef: "native", ObservationProtocolRef: conjunction.ProtocolRef, ExecutionStagesHash: conjunction.RequiredStagesHash}
	return observation, binding
}

func TestRuntimeValidationExecutionConjunctionRequiresExactCompleteOwnerFacts(t *testing.T) {
	observation, binding := validationExecutionConjunctionFixture()
	if observation.ValidateFailureDetails() != nil || binding.Validate() != nil || !RuntimeValidationAttestationPassed(observation, binding) {
		t.Fatal("完整中立 owner 合取事实未接受")
	}
	for _, scenario := range []string{"missing", "incomplete", "missing-stage", "reordered", "wrong-scope", "unstarted", "timeout", "cleanup", "known-failure", "nonzero", "unfinished", "source-change", "cross-source", "zero-tests", "partial-tests", "test-requirement-downgrade", "behavior-requirement-downgrade", "output-incomplete", "bad-output-hash", "missing-behavior", "wrong-protocol", "wrong-stages-hash", "raw-ref", "over-bound", "overlap"} {
		t.Run(scenario, func(t *testing.T) {
			changed := observation.Clone()
			c := changed.ExecutionConjunction
			switch scenario {
			case "missing":
				changed.ExecutionConjunction = nil
			case "incomplete":
				c.Complete = false
			case "missing-stage":
				c.Stages = c.Stages[:1]
			case "reordered":
				c.Stages[0], c.Stages[1] = c.Stages[1], c.Stages[0]
			case "wrong-scope":
				c.Stages[1].Identity.Scope = "subset"
			case "unstarted":
				c.Stages[1].Started = false
			case "timeout":
				c.Stages[1].TimedOut = true
			case "cleanup":
				c.Stages[1].CleanupFailed = true
			case "known-failure":
				c.Stages[1].KnownFailure = true
			case "nonzero":
				c.Stages[1].ExitCode = 1
			case "unfinished":
				c.Stages[1].TerminalComplete = false
			case "source-change":
				c.Stages[1].SourceAfter = StableRuntimeValidationExecutionStagesHash(nil)
			case "cross-source":
				c.Stages[1].SourceBefore, c.Stages[1].SourceAfter = StableRuntimeValidationExecutionStagesHash(nil), StableRuntimeValidationExecutionStagesHash(nil)
			case "zero-tests":
				c.Stages[1].Tests, c.Stages[1].PassedTests = 0, 0
			case "partial-tests":
				c.Stages[1].Tests++
			case "test-requirement-downgrade":
				c.Stages[1].Identity.RequireTests = false
			case "behavior-requirement-downgrade":
				c.Stages[1].Identity.RequireBehavior = false
			case "output-incomplete":
				c.Stages[1].Stdout.Complete, c.Stages[1].Stdout.OmittedReason = false, "capacity"
			case "bad-output-hash":
				c.Stages[1].Stdout.SHA256 = "self-declared"
			case "missing-behavior":
				c.BehaviorTargetRefs = nil
			case "wrong-protocol":
				c.ProtocolRef = "other"
			case "wrong-stages-hash":
				c.RequiredStagesHash = StableRuntimeValidationExecutionStagesHash(nil)
			case "raw-ref":
				c.Stages[1].Identity.ID = " raw "
			case "over-bound":
				for len(c.Stages) <= RuntimeValidationExecutionMaxStages {
					c.Stages = append(c.Stages, c.Stages[0])
				}
			case "overlap":
				c.Stages[1].StartedAt = c.Stages[0].StartedAt
			}
			if RuntimeValidationAttestationPassed(changed, binding) {
				t.Fatal("不完整或漂移合取仍通过准确 native mapping")
			}
			if changed.ExecutionConjunction != nil && !reflect.DeepEqual(changed.ExecutionConjunction, changed.Normalized().ExecutionConjunction) {
				t.Fatal("normalization 清洗非法原始 owner 合取事实")
			}
		})
	}
	legacy := binding
	legacy.ExecutionStagesHash = ""
	ordinary := observation.Clone()
	ordinary.ExecutionConjunction = nil
	if !RuntimeValidationAttestationPassed(ordinary, legacy) || RuntimeValidationAttestationPassed(ordinary, binding) {
		t.Fatal("旧 seal 被隐式提升或新 seal 缺合取被接受")
	}
}

func TestRuntimeValidationExecutionConjunctionBindsReceiptFactAndClone(t *testing.T) {
	observation, _ := validationExecutionConjunctionFixture()
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	receipt.Evidence.Validation = &observation
	fact := EvidenceFact{ID: "evidence", Validation: &observation}
	originalReceipt, originalFact := StableRuntimeOperationReceiptHash(receipt), StableRuntimeEvidenceFactHash(fact)
	for _, scenario := range []string{"argv", "source", "output", "scope", "complete", "tests", "time", "behavior", "requirements"} {
		changed := observation.Clone()
		c := changed.ExecutionConjunction
		switch scenario {
		case "argv":
			c.Stages[1].Argv[1] = "other"
		case "source":
			c.Stages[1].SourceAfter = StableRuntimeValidationExecutionStagesHash(nil)
		case "output":
			c.Stages[1].Stdout.Bytes++
		case "scope":
			c.Stages[1].Identity.Scope = "other"
		case "complete":
			c.Complete = false
		case "tests":
			c.Stages[1].Tests++
		case "time":
			c.Stages[1].FinishedAt = c.Stages[1].StartedAt
		case "behavior":
			c.BehaviorTargetRefs[0] = "other"
		case "requirements":
			c.RequiredStagesHash = StableRuntimeValidationExecutionStagesHash(nil)
		}
		changedReceipt, changedFact := receipt.Clone(), fact
		changedReceipt.Evidence.Validation, changedFact.Validation = &changed, &changed
		if StableRuntimeOperationReceiptHash(changedReceipt) == originalReceipt || StableRuntimeEvidenceFactHash(changedFact) == originalFact {
			t.Fatalf("receipt/fact hash 未绑定完整合取正文：%s", scenario)
		}
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeOperationReceipt
	if err := json.Unmarshal(body, &reloaded); err != nil || reloaded.Validate() != nil || StableRuntimeOperationReceiptHash(reloaded) != originalReceipt {
		t.Fatalf("receipt 重载丢失完整执行合取：%v", err)
	}
	for _, copy := range []RuntimeOperationReceipt{receipt.Clone(), receipt.Normalized(), cloneRuntimeOperationReceiptRaw(receipt), reloaded} {
		copy.Evidence.Validation.ExecutionConjunction.Stages[0].Argv[0] = "caller"
		copy.Evidence.Validation.ExecutionConjunction.BehaviorTargetRefs[0] = "caller"
		if observation.ExecutionConjunction.Stages[0].Argv[0] != "owner-tool" || observation.ExecutionConjunction.BehaviorTargetRefs[0] != "implementation" {
			t.Fatal("返回副本共享执行事实")
		}
	}
}
