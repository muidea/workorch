package workorchprojectioncontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompleteProjectionRoundTripPreservesDiagnosticAndResolutionFields(t *testing.T) {
	payload, err := workorcheventpayload.FromMap(map[string]any{"answer": "continue"})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	want := struct {
		Decision Decision             `json:"decision"`
		Progress TaskProgress         `json:"progress"`
		Memory   MemoryRecallTrace    `json:"memory"`
		Runtime  WorkspaceRuntimeView `json:"runtime"`
	}{
		Decision: Decision{
			ID: "decision-1", WorkspaceID: "workspace-1", RunID: "run-1", StepID: "step-1",
			DecisionKind: "recovery", FailureClass: "contract", FailureSignature: "sig-1",
			ResolutionID: "resolution-1", ResolutionType: "provide_missing_info", ResolutionPayload: payload,
			ResolvedAt: "2026-08-04T10:00:00Z", ResolvedBy: "user", CreatedAt: "2026-08-04T09:00:00Z", UpdatedAt: "2026-08-04T10:00:00Z",
		},
		Progress: TaskProgress{
			WorkspaceID: "workspace-1", RunID: "run-1", BlockingTicketID: "decision-1",
			Items:        []ProgressItem{{ID: "cwi-1", Kind: "coverage", State: "active"}},
			Blockers:     []ProgressBlocker{{ID: "decision-1", Kind: "decision", Reason: "missing input"}},
			EvidenceRefs: []string{"evidence-1"},
			RepairLoop: RepairLoop{
				MissingDoDItems: []map[string]any{{"id": "dod-1"}},
				RecentAttempts:  []map[string]any{{"attempt": float64(2)}},
			},
		},
		Memory: MemoryRecallTrace{
			ID: "trace-1", RefreshSource: "runtime", SelectionPolicyVersion: "v2",
			CandidateMemoryIDs: []string{"memory-1"}, SelectedMemoryIDs: []string{"memory-1"},
			TypeQuota: map[string]int{"semantic": 1}, SelectionScores: []workorchmemorycontract.RecallSelectionScore{{ID: "memory-1", Score: 10}},
		},
		Runtime: WorkspaceRuntimeView{
			Projection: ProjectionStatusFromWorkspaceViewModel(WorkspaceViewModel{StreamGeneration: "generation-1", FailedWorkspaceIds: []string{"workspace-2"}, DegradedSince: "2026-08-04T08:00:00Z"}),
			RunCommand: &RunCommandStatus{
				WorkspaceID:     "workspace-1",
				RunID:           "run-1",
				State:           "accepted",
				Intent:          "resume",
				RequestID:       "request-1",
				RequestReason:   "operator_requested",
				Source:          "http",
				Outcome:         runtimecontract.RuntimeExecutionIntentReconciliationRequested,
				AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
				Diagnostic:      "current Run continuation accepted",
				AcceptedAt:      "2026-08-04T10:00:00Z",
				Revision:        8,
			},
			Activity: WorkspaceActivityFeed{StreamGeneration: "generation-1"},
		},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got typeofCompleteProjectionFixture
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantComparable := typeofCompleteProjectionFixture(want)
	if !reflect.DeepEqual(got, wantComparable) {
		t.Fatalf("round trip changed projection\nwant=%+v\n got=%+v", wantComparable, got)
	}
}

func TestRunCommandStatusJSONExposesOnlyCanonicalAdmissionFields(t *testing.T) {
	status := RunCommandStatus{
		WorkspaceID:     "workspace-1",
		RunID:           "run-1",
		State:           "accepted",
		Intent:          "resume",
		RequestID:       "request-1",
		RequestReason:   "operator_requested",
		Source:          "web",
		Outcome:         runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		Diagnostic:      "current Run continuation accepted",
		AcceptedAt:      "2026-08-23T00:00:00Z",
		Revision:        9,
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal run command status: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal run command status: %v", err)
	}
	want := []string{
		"workspaceId", "runId", "state", "intent", "requestId", "requestReason", "source", "outcome",
		"admissionReason", "diagnostic", "acceptedAt", "revision",
	}
	if len(fields) != len(want) {
		t.Fatalf("run command JSON fields = %+v, want exactly %v", fields, want)
	}
	for _, name := range want {
		if _, ok := fields[name]; !ok {
			t.Fatalf("run command JSON omitted canonical field %q: %+v", name, fields)
		}
	}
	for _, removed := range []string{"reason", "disposition", "error", "definitionRevision", "decisionId", "rejectedAt"} {
		if _, ok := fields[removed]; ok {
			t.Fatalf("run command JSON retained removed or mutually exclusive field %q: %+v", removed, fields)
		}
	}
	zeroData, err := json.Marshal(RunCommandStatus{})
	if err != nil {
		t.Fatalf("marshal zero run command status: %v", err)
	}
	var zeroFields map[string]any
	if err := json.Unmarshal(zeroData, &zeroFields); err != nil {
		t.Fatalf("unmarshal zero run command status: %v", err)
	}
	if revision, ok := zeroFields["revision"]; !ok || revision != float64(0) {
		t.Fatalf("run command JSON must always expose revision, got %+v", zeroFields)
	}
}

func TestPublicResourceContractsExcludeApplicationPrivateState(t *testing.T) {
	workspaceType := reflect.TypeOf(Workspace{})
	if _, ok := workspaceType.FieldByName("InternalDirectory"); ok {
		t.Fatal("public Workspace must not expose InternalDirectory")
	}
	runType := reflect.TypeOf(Run{})
	if _, ok := runType.FieldByName("RuntimeState"); ok {
		t.Fatal("public Run must not expose RuntimeState")
	}
}

func TestPublicSnapshotContractsRoundTripResourceAndObservabilityFields(t *testing.T) {
	want := struct {
		RunSnapshot       RunSnapshot       `json:"runSnapshot"`
		WorkspaceSnapshot WorkspaceSnapshot `json:"workspaceSnapshot"`
		Observability     RunObservability  `json:"observability"`
	}{
		RunSnapshot: RunSnapshot{
			Workspace:               Workspace{ID: "workspace-1", Title: "workspace", State: "active"},
			Run:                     Run{ID: "run-1", WorkspaceID: "workspace-1", State: "active", CreatedAt: "2026-08-04T10:00:00Z"},
			CoverageWorkItemContext: map[string]any{"scope": "selected"},
		},
		WorkspaceSnapshot: WorkspaceSnapshot{
			Workspace: Workspace{ID: "workspace-1", State: "active"},
			Run:       &Run{ID: "run-1", WorkspaceID: "workspace-1", State: "active"},
			Runs:      []Run{{ID: "run-1", WorkspaceID: "workspace-1", State: "active"}},
		},
		Observability: RunObservability{
			Run:      Run{ID: "run-1", WorkspaceID: "workspace-1", State: "active"},
			Timeline: []RuntimeTimelineItem{{ID: "timeline-1"}},
		},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		RunSnapshot       RunSnapshot       `json:"runSnapshot"`
		WorkspaceSnapshot WorkspaceSnapshot `json:"workspaceSnapshot"`
		Observability     RunObservability  `json:"observability"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed public snapshot contracts\nwant=%+v\n got=%+v", want, got)
	}
}

func TestCloneContextBundleDetachesNestedProtocolCollections(t *testing.T) {
	want := &ContextBundle{
		Recovery:              RecoveryPlan{BlockingResource: &BlockingResourceRef{AllowedUserActions: []string{"restart"}}},
		TaskProgress:          TaskProgress{Recovery: RecoveryPlan{BlockingResource: &BlockingResourceRef{AllowedUserActions: []string{"revise"}}}},
		Sections:              []ContextSection{{EvidenceRefs: []string{"evidence-1"}, Data: map[string]any{"state": "accepted"}}},
		RecentEvidence:        []ContextEvidence{{EvidenceRefs: []string{"evidence-2"}, Data: map[string]any{"kind": "validation"}}},
		CompressionCandidates: []ContextCompressionCandidate{{SourceRefs: []string{"section-1"}}},
	}
	got := CloneContextBundle(want)
	got.Sections[0].EvidenceRefs[0] = "changed"
	got.Sections[0].Data["state"] = "changed"
	got.RecentEvidence[0].EvidenceRefs[0] = "changed"
	got.RecentEvidence[0].Data["kind"] = "changed"
	got.CompressionCandidates[0].SourceRefs[0] = "changed"
	got.Recovery.BlockingResource.AllowedUserActions[0] = "changed"
	got.TaskProgress.Recovery.BlockingResource.AllowedUserActions[0] = "changed"
	if want.Sections[0].EvidenceRefs[0] != "evidence-1" || want.Sections[0].Data["state"] != "accepted" ||
		want.RecentEvidence[0].EvidenceRefs[0] != "evidence-2" || want.RecentEvidence[0].Data["kind"] != "validation" ||
		want.CompressionCandidates[0].SourceRefs[0] != "section-1" ||
		want.Recovery.BlockingResource.AllowedUserActions[0] != "restart" ||
		want.TaskProgress.Recovery.BlockingResource.AllowedUserActions[0] != "revise" {
		t.Fatalf("clone shares nested collections with input: %+v", want)
	}
}

type typeofCompleteProjectionFixture struct {
	Decision Decision             `json:"decision"`
	Progress TaskProgress         `json:"progress"`
	Memory   MemoryRecallTrace    `json:"memory"`
	Runtime  WorkspaceRuntimeView `json:"runtime"`
}
