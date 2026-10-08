package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeResultCloseoutCloneRawPreservesAndDetaches(t *testing.T) {
	original := RuntimeResultCloseout{
		CloseoutID:         " 原始身份 ",
		OriginalDiagnostic: &RuntimeOriginalDiagnostic{TargetRefs: []string{" 原始目标 "}},
		RequirementOutcomes: []RuntimeRequirementOutcome{{
			MissingEvidenceKinds: []string{"validation"},
			EvidenceItems:        []RuntimeRequirementEvidenceItem{{EvidenceRef: "evidence"}},
			AcceptedEvidenceRefs: []string{"accepted"},
		}},
		AcceptedEvidenceRefs: []string{"accepted"}, DeliverableRefs: []string{"deliverable"},
		OwnerRevisionSet:               RuntimeOwnerRevisionSet{Entries: []RuntimeOwnerRevisionRef{{Revision: "revision"}}},
		KnownLimitations:               []string{"limitation"},
		SatisfiedCoverageWorkItemIDs:   []common.CoverageWorkItemID{"satisfied"},
		UnsatisfiedCoverageWorkItemIDs: []common.CoverageWorkItemID{"unsatisfied"},
		Gaps:                           []RuntimeAssessmentGap{{CoverageWorkItemIDs: []common.CoverageWorkItemID{"gap"}}},
		MissingEvidenceKinds:           []string{"missing"}, TargetRefs: []string{"target"},
		EvidenceRefs: []string{"evidence"}, ArtifactRefs: []common.ArtifactID{"artifact"},
		OperationOutcomes: []RuntimeOperationCloseout{{ReceiptRef: "receipt"}},
	}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	cloned := original.CloneRaw()
	if !reflect.DeepEqual(original, cloned) {
		t.Fatal("原值克隆改变内容或集合身份")
	}
	cloned.OriginalDiagnostic.TargetRefs[0] = "changed"
	cloned.RequirementOutcomes[0].MissingEvidenceKinds[0] = "changed"
	cloned.RequirementOutcomes[0].EvidenceItems[0].EvidenceRef = "changed"
	cloned.RequirementOutcomes[0].AcceptedEvidenceRefs[0] = "changed"
	cloned.AcceptedEvidenceRefs[0] = "changed"
	cloned.DeliverableRefs[0] = "changed"
	cloned.OwnerRevisionSet.Entries[0].Revision = "changed"
	cloned.KnownLimitations[0] = "changed"
	cloned.SatisfiedCoverageWorkItemIDs[0] = "changed"
	cloned.UnsatisfiedCoverageWorkItemIDs[0] = "changed"
	cloned.Gaps[0].CoverageWorkItemIDs[0] = "changed"
	cloned.MissingEvidenceKinds[0] = "changed"
	cloned.TargetRefs[0] = "changed"
	cloned.EvidenceRefs[0] = "changed"
	cloned.ArtifactRefs[0] = "changed"
	cloned.OperationOutcomes[0].ReceiptRef = "changed"
	after, err := json.Marshal(original)
	if err != nil || string(before) != string(after) {
		t.Fatal("修改克隆材料污染原值")
	}
	for _, value := range []RuntimeResultCloseout{{}, {
		RequirementOutcomes:  []RuntimeRequirementOutcome{{MissingEvidenceKinds: []string{}, EvidenceItems: []RuntimeRequirementEvidenceItem{}, AcceptedEvidenceRefs: []string{}}},
		AcceptedEvidenceRefs: []string{}, DeliverableRefs: []string{}, KnownLimitations: []string{},
		OwnerRevisionSet:             RuntimeOwnerRevisionSet{Entries: []RuntimeOwnerRevisionRef{}},
		SatisfiedCoverageWorkItemIDs: []common.CoverageWorkItemID{}, UnsatisfiedCoverageWorkItemIDs: []common.CoverageWorkItemID{},
		Gaps: []RuntimeAssessmentGap{}, MissingEvidenceKinds: []string{}, TargetRefs: []string{}, EvidenceRefs: []string{},
		ArtifactRefs: []common.ArtifactID{}, OperationOutcomes: []RuntimeOperationCloseout{},
	}} {
		if !reflect.DeepEqual(value, value.CloneRaw()) {
			t.Fatal("克隆改变 nil/empty 身份")
		}
	}
}

func TestRuntimeResultCloseoutValidateRawRejectsNormalizationRepair(t *testing.T) {
	original := formalCloseoutFixture(t, TerminalOutcomeKindCompleted, RuntimeRequirementSatisfied)
	if err := original.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RuntimeResultCloseout){
		func(value *RuntimeResultCloseout) { value.CloseoutID = " " + value.CloseoutID },
		func(value *RuntimeResultCloseout) {
			value.AcceptedEvidenceRefs = append(value.AcceptedEvidenceRefs, value.AcceptedEvidenceRefs[0])
		},
		func(value *RuntimeResultCloseout) { value.RequirementOutcomes[0].EvidenceItems[0].BindingRef += " " },
	} {
		invalid := original.CloneRaw()
		mutate(&invalid)
		before := invalid.CloneRaw()
		if err := invalid.ValidateRaw(); err == nil {
			t.Fatal("原值校验接受被规范化修复的材料")
		}
		if !reflect.DeepEqual(invalid, before) {
			t.Fatal("校验改写原材料")
		}
	}
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var reopened RuntimeResultCloseout
	if err := json.Unmarshal(payload, &reopened); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ValidateRaw(); err != nil || !reflect.DeepEqual(original, reopened) {
		t.Fatalf("序列化重载改变原材料: %v", err)
	}
}

func TestRuntimeDeliveryAcknowledgementMaterialScopeAndDetachedNormalization(t *testing.T) {
	closeout := formalCloseoutFixture(t, TerminalOutcomeKindCompleted, RuntimeRequirementSatisfied)
	ack := RuntimeDeliveryAcknowledgement{
		WorkspaceID: "workspace-1", RunID: "run-1", ResultID: RuntimeTerminalResultID("workspace-1", "run-1"),
		Owner: RuntimeDeliveryOwnerResult, AcknowledgedState: RuntimeDeliveryAcknowledgedResultRecorded,
		TerminalDeliveryID: "delivery-1", AuthoritySnapshotID: "authority-1", CausationID: "cause-1",
		Outcome: closeout.TerminalOutcome, ResultCloseout: &closeout,
	}
	if err := ack.Validate(); err != nil {
		t.Fatal(err)
	}
	before := ack.CloneRaw()
	normalized := ack.Normalized()
	normalized.ResultCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] = "changed"
	if !reflect.DeepEqual(ack, before) {
		t.Fatal("规范化回执共享原材料指针")
	}
	for _, mutate := range []func(*RuntimeDeliveryAcknowledgement){
		func(value *RuntimeDeliveryAcknowledgement) {
			value.Owner = RuntimeDeliveryOwnerWorkspace
			value.ResultID = ""
			value.AcknowledgedState = RuntimeDeliveryAcknowledgedWorkspaceReleased
		},
		func(value *RuntimeDeliveryAcknowledgement) { value.ResultCloseout.CloseoutHash = "changed" },
		func(value *RuntimeDeliveryAcknowledgement) { value.Outcome.Code = "other-terminal" },
	} {
		invalid := before.CloneRaw()
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatal("回执接受错误 owner、损坏材料或错配终态")
		}
	}
}

func TestRuntimeDeliveryAcknowledgementValidateRawRejectsNoncanonicalIdentity(t *testing.T) {
	closeout := formalCloseoutFixture(t, TerminalOutcomeKindCompleted, RuntimeRequirementSatisfied)
	ack := RuntimeDeliveryAcknowledgement{
		WorkspaceID: "workspace-1", RunID: "run-1", ResultID: RuntimeTerminalResultID("workspace-1", "run-1"),
		Owner: RuntimeDeliveryOwnerResult, AcknowledgedState: RuntimeDeliveryAcknowledgedResultRecorded,
		TerminalDeliveryID: "delivery-1", AuthoritySnapshotID: "authority-1", CausationID: "cause-1",
		Outcome: closeout.TerminalOutcome, ResultCloseout: &closeout,
	}
	if err := ack.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RuntimeDeliveryAcknowledgement){
		func(value *RuntimeDeliveryAcknowledgement) { value.TerminalDeliveryID += " " },
		func(value *RuntimeDeliveryAcknowledgement) { value.CausationID += " " },
		func(value *RuntimeDeliveryAcknowledgement) { value.Outcome.Summary += " " },
	} {
		invalid := ack.CloneRaw()
		mutate(&invalid)
		if err := invalid.ValidateRaw(); err == nil {
			t.Fatal("原值回执校验接受被规范化掩盖的身份变化")
		}
	}
}

func TestRuntimeDeliveryAcknowledgementCloneRawDetachesPersistedRecord(t *testing.T) {
	closeout := formalCloseoutFixture(t, TerminalOutcomeKindCompleted, RuntimeRequirementSatisfied)
	ack := RuntimeDeliveryAcknowledgement{Owner: RuntimeDeliveryOwnerResult, ResultCloseout: &closeout}
	cloned := ack.CloneRaw()
	if !reflect.DeepEqual(ack, cloned) {
		t.Fatal("回执原值克隆改变内容")
	}
	cloned.ResultCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] = "changed"
	if ack.ResultCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] != "evidence-1" {
		t.Fatal("回执克隆共享嵌套材料")
	}
	record := RuntimeStateRecord{DeliveryAcknowledgements: []RuntimeDeliveryAcknowledgement{ack}}
	copy := record.Clone()
	copy.DeliveryAcknowledgements[0].ResultCloseout.OwnerRevisionSet.Entries[0].Revision = "changed"
	if reflect.DeepEqual(copy.DeliveryAcknowledgements, record.DeliveryAcknowledgements) {
		t.Fatal("opaque record 克隆未复制回执材料")
	}
	if ack.ResultCloseout.OwnerRevisionSet.Entries[0].Revision != "tree-2" {
		t.Fatal("opaque record 克隆污染原材料")
	}
	if (RuntimeDeliveryAcknowledgement{}).CloneRaw().ResultCloseout != nil {
		t.Fatal("空回执克隆伪造材料")
	}
}
