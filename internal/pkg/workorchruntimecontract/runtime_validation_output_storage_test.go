package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func validationOutputStorageFixture() RuntimeValidationObservation {
	observation, _ := validationExecutionConjunctionFixture()
	for index := range observation.ExecutionConjunction.Stages {
		stage := &observation.ExecutionConjunction.Stages[index]
		stage.Stdout.Storage = &RuntimeValidationOutputStorage{Reference: "owner-output/stdout-" + stage.Identity.ID}
		stage.Stderr.Storage = &RuntimeValidationOutputStorage{Reference: "owner-output/stderr-" + stage.Identity.ID}
	}
	return observation
}

func TestValidationOutputStoragePreservesEmptyExtensionJSONAndHash(t *testing.T) {
	observation, _ := validationExecutionConjunctionFixture()
	c := *observation.ExecutionConjunction
	// 独立旧 wire 结构固定字段次序；不由新结构删除字段反推旧正文。
	type oldOutput struct {
		Bytes         int64
		SHA256        string
		SavedBytes    int64
		Complete      bool
		OmittedReason string
	}
	type oldStage struct {
		Identity         RuntimeValidationExecutionStageIdentity
		Argv             []string
		StartedAt        string
		FinishedAt       string
		ExitCode         int64
		Started          bool
		TimedOut         bool
		CleanupFailed    bool
		Stdout           oldOutput
		Stderr           oldOutput
		SourceBefore     string
		SourceAfter      string
		Tests            int
		PassedTests      int
		TerminalComplete bool
		KnownFailure     bool
		FailureCode      string
	}
	old := struct {
		ProtocolRef        string
		RequiredStagesHash string
		Stages             []oldStage
		BehaviorTargetRefs []string
		Complete           bool
	}{ProtocolRef: c.ProtocolRef, RequiredStagesHash: c.RequiredStagesHash, BehaviorTargetRefs: c.BehaviorTargetRefs, Complete: c.Complete}
	for _, s := range c.Stages {
		old.Stages = append(old.Stages, oldStage{s.Identity, s.Argv, s.StartedAt, s.FinishedAt, s.ExitCode, s.Started, s.TimedOut, s.CleanupFailed,
			oldOutput{s.Stdout.Bytes, s.Stdout.SHA256, s.Stdout.SavedBytes, s.Stdout.Complete, s.Stdout.OmittedReason},
			oldOutput{s.Stderr.Bytes, s.Stderr.SHA256, s.Stderr.SavedBytes, s.Stderr.Complete, s.Stderr.OmittedReason},
			s.SourceBefore, s.SourceAfter, s.Tests, s.PassedTests, s.TerminalComplete, s.KnownFailure, s.FailureCode})
	}
	want, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(c)
	if err != nil || string(got) != string(want) {
		t.Fatalf("空扩展改变原 JSON：%v", err)
	}
	kind := "runtime-validation-execution-conjunction"
	digest := sha256.Sum256(append([]byte(kind+"\x00"), want...))
	if stableFormalValueHash(kind, c) != kind+"-sha256:"+hex.EncodeToString(digest[:]) {
		t.Fatal("空扩展改变原合取身份")
	}
}

func TestValidationOutputStorageRequiresExactCompleteReferences(t *testing.T) {
	observation := validationOutputStorageFixture()
	if err := observation.ValidateFailureDetails(); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"missing", "empty", "omitted", "both", "whitespace", "control", "utf8", "bound", "reason-bound"} {
		t.Run(scenario, func(t *testing.T) {
			changed := observation.Clone()
			storage := changed.ExecutionConjunction.Stages[0].Stdout.Storage
			switch scenario {
			case "missing":
				changed.ExecutionConjunction.Stages[0].Stdout.Storage = nil
			case "empty":
				storage.Reference = ""
			case "omitted":
				storage.Reference, storage.OmittedReason = "", "owner_storage_failed"
			case "both":
				storage.OmittedReason = "owner_storage_failed"
			case "whitespace":
				storage.Reference = " ref "
			case "control":
				storage.Reference = "ref\x00"
			case "utf8":
				storage.Reference = string([]byte{0xff})
			case "bound":
				storage.Reference = strings.Repeat("r", 1025)
			case "reason-bound":
				storage.Reference, storage.OmittedReason = "", strings.Repeat("r", 513)
			}
			if changed.ValidateFailureDetails() == nil {
				t.Fatal("缺失或非法 storage 被标为完整合取")
			}
			if !reflect.DeepEqual(changed, changed.Normalized()) {
				t.Fatal("规范化清洗非法 storage 原值")
			}
		})
	}
	observation.ExecutionConjunction.Complete = false
	output := &observation.ExecutionConjunction.Stages[0].Stdout
	output.Storage = &RuntimeValidationOutputStorage{OmittedReason: "owner_storage_failed"}
	if err := observation.ValidateFailureDetails(); err != nil || !output.Complete {
		t.Fatalf("持久失败被误当容量不足或丢弃诊断：%v", err)
	}
	output.Complete, output.OmittedReason = false, "capacity"
	output.Storage = &RuntimeValidationOutputStorage{Reference: "owner/saved-prefix"}
	if err := observation.ValidateFailureDetails(); err != nil {
		t.Fatalf("已保存的不完整原文不能保全：%v", err)
	}
}

func TestValidationOutputStorageBindsReceiptFactReloadAndDetachedCopies(t *testing.T) {
	observation := validationOutputStorageFixture()
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	receipt.Evidence.Validation = &observation
	fact := EvidenceFact{ID: "evidence", Validation: &observation}
	beforeReceipt, beforeFact := StableRuntimeOperationReceiptHash(receipt), StableRuntimeEvidenceFactHash(fact)
	for _, scenario := range []string{"reference", "omission", "drop"} {
		changed := observation.Clone()
		output := &changed.ExecutionConjunction.Stages[0].Stdout
		switch scenario {
		case "reference":
			output.Storage.Reference = "owner/other-output"
		case "omission":
			output.Storage.Reference, output.Storage.OmittedReason = "", "storage_failed"
		case "drop":
			output.Storage = nil
		}
		r, f := receipt.Clone(), fact
		r.Evidence.Validation, f.Validation = &changed, &changed
		if StableRuntimeOperationReceiptHash(r) == beforeReceipt || StableRuntimeEvidenceFactHash(f) == beforeFact {
			t.Fatal("storage 正文未进入 receipt/fact 身份")
		}
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeOperationReceipt
	if err := json.Unmarshal(body, &loaded); err != nil || loaded.Validate() != nil || !reflect.DeepEqual(loaded, receipt) || StableRuntimeOperationReceiptHash(loaded) != beforeReceipt {
		t.Fatalf("重载丢失原 storage：%v", err)
	}
	for _, copy := range []RuntimeOperationReceipt{receipt.Clone(), receipt.Normalized(), cloneRuntimeOperationReceiptRaw(receipt), loaded} {
		copy.Evidence.Validation.ExecutionConjunction.Stages[0].Stdout.Storage.Reference = "caller"
		copy.Evidence.Validation.ExecutionConjunction.Stages[0].Stderr.Storage.OmittedReason = "caller"
		if StableRuntimeOperationReceiptHash(receipt) != beforeReceipt {
			t.Fatal("副本共享原 storage 指针")
		}
	}
}

func FuzzValidationOutputStorageRawPreservation(f *testing.F) {
	f.Add("owner/ref", "", true)
	f.Add("", "owner_storage_failed", false)
	f.Add(string([]byte{0xff}), "\x00", true)
	f.Fuzz(func(t *testing.T, ref, reason string, complete bool) {
		observation := validationOutputStorageFixture()
		observation.ExecutionConjunction.Complete = complete
		observation.ExecutionConjunction.Stages[0].Stdout.Storage = &RuntimeValidationOutputStorage{Reference: ref, OmittedReason: reason}
		copy := observation.Normalized()
		if !reflect.DeepEqual(copy, observation) {
			t.Fatal("规范化改变原 storage")
		}
		copy.ExecutionConjunction.Stages[0].Stdout.Storage.Reference = "changed"
		if observation.ExecutionConjunction.Stages[0].Stdout.Storage.Reference != ref {
			t.Fatal("副本共享原 storage")
		}
		_ = observation.ValidateFailureDetails()
	})
}
