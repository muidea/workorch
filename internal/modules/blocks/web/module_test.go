package web

import (
	"context"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestWebApprovalGetterPublishesScopedOwnerCommand(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	approvalID := common.ApprovalID("approval-web-scope")
	runID := common.RunID("run-web-scope")
	var captured events.Envelope
	hub.Subscribe(approvalevents.TopicApprovalCommandGet, func(_ context.Context, envelope events.Envelope) error {
		captured = envelope
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:    approvalID,
					RunID: runID,
					State: common.ApprovalStateApproved,
				},
			},
		})
	})

	item, err := (webApprovalGetter{hub: hub}).GetForRun(ctx, approvalID, runID)
	if err != nil {
		t.Fatalf("GetForRun() error = %v", err)
	}
	if item == nil || item.ID != approvalID {
		t.Fatalf("approval = %+v, want %s", item, approvalID)
	}
	if captured.Topic != approvalevents.TopicApprovalCommandGet {
		t.Fatalf("topic = %q, want %q", captured.Topic, approvalevents.TopicApprovalCommandGet)
	}
	if captured.SourceModule != ModuleID || captured.SubjectType != "approval" || captured.SubjectID != string(approvalID) {
		t.Fatalf("envelope source/subject = %q %q/%q", captured.SourceModule, captured.SubjectType, captured.SubjectID)
	}
	if captured.RunID != string(runID) || captured.Payload[events.PayloadKeyRunID] != string(runID) {
		t.Fatalf("run scope = envelope %q payload %q, want %q", captured.RunID, captured.Payload[events.PayloadKeyRunID], runID)
	}
}
