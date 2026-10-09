package eventstream

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func workspaceClarificationAuditTicket() decisionevents.TicketChangedData {
	return decisionevents.TicketChangedData{
		NotificationID: "decision-notification/created/ws-pre-seal/ticket-pre-seal",
		TicketID:       "ticket-pre-seal", WorkspaceID: "ws-pre-seal",
		DecisionKind: "workspace_definition_clarification",
		Reason:       "需要修正 Template/Profile 证明合同", State: common.DecisionTicketStateOpen,
	}
}

func TestServicePersistsWorkspacePreSealClarificationNotificationsAcrossReopen(t *testing.T) {
	for _, resolutionType := range []string{
		decisionevents.DefinitionClarificationResolutionCandidateAdjusted,
		decisionevents.DefinitionClarificationResolutionCandidateSuperseded,
	} {
		t.Run(resolutionType, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "event-records")
			repo := file.NewEventRecordRepository(path)
			service, hub := newEventStreamTestService(t, repo)
			if err := service.Start(); err != nil {
				t.Fatal(err)
			}
			defer service.Stop()
			ticket := workspaceClarificationAuditTicket()
			resolved := decisionevents.TicketResolvedData{
				NotificationID: "decision-notification/resolved/ws-pre-seal/ticket-pre-seal/resolution-pre-seal",
				Ticket:         ticket, ResolutionID: "resolution-pre-seal", ResolutionType: resolutionType,
			}
			resolved.Ticket.State = common.DecisionTicketStateResolved
			for range 2 {
				hub.Send(magicevent.NewEventWithContext(decisionevents.TopicTicketCreated, decisioncommon.ModuleID,
					decisionevents.NotificationDestination, magicevent.NewHeader(), ctx, ticket))
				hub.Send(magicevent.NewEventWithContext(decisionevents.TopicTicketResolved, decisioncommon.ModuleID,
					decisionevents.NotificationDestination, magicevent.NewHeader(), ctx, resolved))
			}
			query := workorchquerycontract.EventRecordQuery{WorkspaceID: ticket.WorkspaceID, IncludePayload: true}
			records, err := repo.List(ctx, query)
			if err != nil || len(records) != 2 {
				t.Fatalf("合法 workspace-scoped 通知未持久化或 exact replay 重复追加：%+v %v", records, err)
			}
			for index, record := range records {
				wantID, wantTopic := ticket.NotificationID, decisionevents.TopicTicketCreated
				if index == 1 {
					wantID, wantTopic = resolved.NotificationID, decisionevents.TopicTicketResolved
				}
				if string(record.ID) != wantID || record.Topic != wantTopic || record.WorkspaceID != ticket.WorkspaceID ||
					record.RunID != "" || record.StepID != "" || record.Revision != int64(index+1) || record.StreamPosition <= 0 {
					t.Fatalf("审计身份或 scope 漂移：%+v", record)
				}
			}
			reopened, err := file.NewEventRecordRepository(path).List(ctx, query)
			if err != nil || !reflect.DeepEqual(records, reopened) {
				t.Fatalf("文件重开改变 workspace-scoped 审计记录：%+v %v", reopened, err)
			}
		})
	}
}

func TestServiceRejectsMalformedWorkspacePreSealClarificationBeforeAppend(t *testing.T) {
	ticket := workspaceClarificationAuditTicket()
	for name, mutate := range map[string]func(*decisionevents.TicketChangedData){
		"ordinary runless ticket": func(d *decisionevents.TicketChangedData) { d.DecisionKind = "runtime_governance" },
		"unknown kind":            func(d *decisionevents.TicketChangedData) { d.DecisionKind = "unknown" },
		"mixed run scope":         func(d *decisionevents.TicketChangedData) { d.RunID = "run-a" },
		"mixed step scope":        func(d *decisionevents.TicketChangedData) { d.StepID = "step-a" },
		"missing workspace":       func(d *decisionevents.TicketChangedData) { d.WorkspaceID = "" },
		"missing ticket":          func(d *decisionevents.TicketChangedData) { d.TicketID = "" },
		"missing notification":    func(d *decisionevents.TicketChangedData) { d.NotificationID = "" },
		"wrong notification scope": func(d *decisionevents.TicketChangedData) {
			d.NotificationID = "decision-notification/created/other/ticket-pre-seal"
		},
		"resolved created notification": func(d *decisionevents.TicketChangedData) { d.State = common.DecisionTicketStateResolved },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := ticket
			mutate(&invalid)
			assertRejectedClarificationAudit(t, decisionevents.TopicTicketCreated, decisioncommon.ModuleID, invalid)
		})
	}
	assertRejectedClarificationAudit(t, decisionevents.TopicTicketCreated, "untrusted/module", ticket)

	resolved := decisionevents.TicketResolvedData{
		NotificationID: "decision-notification/resolved/ws-pre-seal/ticket-pre-seal/resolution-pre-seal",
		Ticket:         ticket, ResolutionID: "resolution-pre-seal",
		ResolutionType: decisionevents.DefinitionClarificationResolutionCandidateAdjusted,
	}
	resolved.Ticket.State = common.DecisionTicketStateResolved
	for name, mutate := range map[string]func(*decisionevents.TicketResolvedData){
		"open resolved notification": func(d *decisionevents.TicketResolvedData) { d.Ticket.State = common.DecisionTicketStateOpen },
		"ordinary resolution": func(d *decisionevents.TicketResolvedData) {
			d.ResolutionType = string(decisioncommon.ResolutionTypeReplan)
		},
		"runtime replacement resolution": func(d *decisionevents.TicketResolvedData) {
			d.ResolutionType = decisionevents.DefinitionClarificationResolutionReplacementBound
		},
		"missing resolution": func(d *decisionevents.TicketResolvedData) { d.ResolutionID = "" },
		"wrong resolution scope": func(d *decisionevents.TicketResolvedData) {
			d.NotificationID = "decision-notification/resolved/other/ticket-pre-seal/resolution-pre-seal"
		},
		"run continuation": func(d *decisionevents.TicketResolvedData) { d.ContinuationAction = "resume" },
		"resume run":       func(d *decisionevents.TicketResolvedData) { d.ResumeRunID = "run-a" },
		"successor run":    func(d *decisionevents.TicketResolvedData) { d.SuccessorRunID = "run-a" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := resolved
			mutate(&invalid)
			assertRejectedClarificationAudit(t, decisionevents.TopicTicketResolved, decisioncommon.ModuleID, invalid)
		})
	}
	assertRejectedClarificationAudit(t, decisionevents.TopicTicketResolved, "untrusted/module", resolved)
}

func assertRejectedClarificationAudit(t *testing.T, topic, source string, payload any) {
	t.Helper()
	repo := newCountingEventRecordRepository()
	service, hub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()
	hub.Send(magicevent.NewEventWithContext(topic, source, decisionevents.NotificationDestination,
		magicevent.NewHeader(), context.Background(), payload))
	if repo.AppendCount() != 0 {
		t.Fatalf("非法澄清通知越过 append 边界：%s %s", topic, source)
	}
}
