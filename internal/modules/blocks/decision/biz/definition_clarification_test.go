package decision

import (
	"context"
	"errors"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestWorkspaceDefinitionClarificationQuestionMatchesRequiredAction(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{
			name:   "definition relation",
			action: decisionevents.DefinitionClarificationActionRewriteAllOf,
			want:   "Revise the Definition into explicit all-of semantics",
		},
		{
			name:   "profile proof contract",
			action: decisionevents.DefinitionClarificationActionFixTemplateProfile,
			want:   "Select or fix the Workspace Template/Profile proof contract",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workspaceDefinitionClarificationQuestion(tt.action); got != tt.want {
				t.Fatalf("workspaceDefinitionClarificationQuestion(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func workspaceDefinitionClarificationCommandForBizTest() decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand {
	command := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand{
		SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent,
		WorkspaceID:   "workspace-1", CandidateID: "workspace-definition", CandidateRevision: 1,
		CandidateHash: "candidate-hash-1", ViolationCode: "definition_one_of_unsupported",
		OffendingRefs: []string{"dod:item-1", "goal:item-1"}, CanonicalSignatureVersion: "v1",
		CanonicalSignature: "one-of:dod:item-1", RequiredActionKind: decisionevents.DefinitionClarificationActionRewriteAllOf,
	}
	command.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(command)
	return command
}

func TestOpenWorkspaceDefinitionClarificationReplayMatchesPersistedFieldsNotOnlyHash(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	for _, test := range []struct {
		name    string
		mutate  func(*decisionmodel.DefinitionClarificationBinding)
		wantErr bool
	}{
		{name: "exact replay"},
		{name: "forged offending refs with unchanged hash", wantErr: true, mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.OffendingRefs = []string{"dod:item-forged"}
		}},
		{name: "forged canonical signature with unchanged hash", wantErr: true, mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.CanonicalSignature = "forged"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tickets := memory.NewDecisionTicketRepository()
			binding := workspaceDefinitionClarificationBindingForCommand(command)
			if test.mutate != nil {
				test.mutate(binding)
			}
			ticket := workspaceDefinitionClarificationTicketForCommand(command)
			ticket.State = common.DecisionTicketStateOpen
			ticket.DefinitionClarification = binding
			if err := tickets.Create(context.Background(), ticket); err != nil {
				t.Fatalf("seed ticket: %v", err)
			}
			owner := &Biz{tickets: tickets}
			result, err := owner.openWorkspaceDefinitionClarification(context.Background(), command)
			if test.wantErr {
				if !errors.Is(err, common.ErrConflict) {
					t.Fatalf("forged replay error = %v, want conflict", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("exact replay: %v", err)
			}
			if result.Disposition != decisionevents.DefinitionClarificationDuplicate {
				t.Fatalf("disposition = %q, want duplicate", result.Disposition)
			}
		})
	}
}

func TestQueryWorkspaceDefinitionClarificationFactReadsExactPersistedTicketAndResolution(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	binding := workspaceDefinitionClarificationBindingForCommand(command)
	resolutionCommand := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID,
		OldCandidateID: command.CandidateID, OldCandidateRevision: command.CandidateRevision, OldCandidateHash: command.CandidateHash,
		NewCandidateID: command.CandidateID, NewCandidateRevision: 2, NewCandidateHash: "candidate-hash-2",
		AdmissionResultHash: "admission-hash-2", DefinitionRevision: "definition-2", StartupInputHash: "startup-2",
	}
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(command.TicketID, resolutionCommand.AdmissionResultHash+"\x00"+resolutionCommand.StartupInputHash)
	binding.ResolutionID = resolutionID
	binding.ResolutionBindingHash = resolutionCommand.BindingHash(resolutionID)
	binding.ResolutionCandidateID = resolutionCommand.NewCandidateID
	binding.ResolutionCandidateRevision = resolutionCommand.NewCandidateRevision
	binding.ResolutionCandidateHash = resolutionCommand.NewCandidateHash
	binding.AdmissionResultHash = resolutionCommand.AdmissionResultHash
	binding.DefinitionRevision = resolutionCommand.DefinitionRevision
	binding.StartupInputHash = resolutionCommand.StartupInputHash
	ticket := workspaceDefinitionClarificationTicketForCommand(command)
	ticket.State = common.DecisionTicketStateResolved
	ticket.ResolvedAt = "2026-08-25T12:00:00Z"
	// Notification delivery is a later idempotent cut point and may already be
	// finalized when another owner queries the durable clarification fact.
	ticket.ResolutionNotificationPublishedAt = "2026-08-25T12:00:01Z"
	ticket.DefinitionClarification = binding
	if err := tickets.Create(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	if err := resolutions.Create(context.Background(), &decisionmodel.Resolution{
		ID: resolutionID, TicketID: command.TicketID, Type: decisionevents.DefinitionClarificationResolutionCandidateAdjusted,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("definitionRevision", resolutionCommand.DefinitionRevision),
			workorcheventpayload.StringField("startupInputHash", resolutionCommand.StartupInputHash),
		),
	}); err != nil {
		t.Fatal(err)
	}
	owner := &Biz{tickets: tickets, resolutions: resolutions}
	query := decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{WorkspaceID: command.WorkspaceID, TicketID: command.TicketID}
	result, err := owner.queryWorkspaceDefinitionClarificationFact(context.Background(), query)
	if err != nil || result.ValidateFor(query) != nil || !result.Found || result.Ticket == nil || result.Resolution == nil {
		t.Fatalf("persisted fact=%+v err=%v", result, err)
	}
	if !result.Ticket.RequestIdentity.EqualNormalized(definitioncontract.WorkspaceDefinitionClarificationRequestIdentity(command)) || result.Resolution.ResolutionIdentity != definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity(resolutionCommand) {
		t.Fatalf("persisted fact identity drift: %+v", result)
	}
}

func TestQueryWorkspaceDefinitionClarificationFactRejectsRetiredOrPartialResolution(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	for _, test := range []struct {
		name   string
		mutate func(*decisionmodel.DefinitionClarificationBinding)
	}{
		{name: "retired superseded ticket", mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.ResolutionID = "resolution-superseded"
			binding.ResolutionBindingHash = "reconciliation-binding"
		}},
		{name: "partial admission resolution", mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.ResolutionID = "resolution-partial"
			binding.ResolutionBindingHash = "resolution-binding"
			binding.ResolutionCandidateID = command.CandidateID
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tickets := memory.NewDecisionTicketRepository()
			binding := workspaceDefinitionClarificationBindingForCommand(command)
			test.mutate(binding)
			ticket := workspaceDefinitionClarificationTicketForCommand(command)
			ticket.State = common.DecisionTicketStateResolved
			ticket.ResolvedAt = "2026-08-25T12:00:00Z"
			ticket.DefinitionClarification = binding
			if err := tickets.Create(context.Background(), ticket); err != nil {
				t.Fatal(err)
			}
			owner := &Biz{tickets: tickets, resolutions: memory.NewDecisionResolutionRepository()}
			_, err := owner.queryWorkspaceDefinitionClarificationFact(context.Background(), decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{
				WorkspaceID: command.WorkspaceID, TicketID: command.TicketID,
			})
			if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("query error=%v, want conflict", err)
			}
		})
	}
}

func TestQueryWorkspaceDefinitionClarificationFactRejectsOuterTicketSemanticDrift(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	mutations := []struct {
		name   string
		mutate func(*decisionmodel.Ticket)
	}{
		{name: "decision kind", mutate: func(ticket *decisionmodel.Ticket) { ticket.DecisionKind = "other" }},
		{name: "reason", mutate: func(ticket *decisionmodel.Ticket) { ticket.Reason = "other" }},
		{name: "question", mutate: func(ticket *decisionmodel.Ticket) { ticket.Question = "other" }},
		{name: "failure class", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureClass = "other" }},
		{name: "failure signature", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureSignature = "other" }},
		{name: "allowed recovery actions", mutate: func(ticket *decisionmodel.Ticket) { ticket.AllowedRecoveryActions = []string{"other"} }},
		{name: "step id", mutate: func(ticket *decisionmodel.Ticket) { ticket.StepID = "step-1" }},
		{name: "context", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.Context = workorcheventpayload.New(workorcheventpayload.StringField("forged", "value"))
		}},
		{name: "auto governance", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.AutoGovernance = workorcheventpayload.New(workorcheventpayload.StringField("forged", "value"))
		}},
		{name: "runtime governance proposal", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.RuntimeGovernanceProposal = &decisionmodel.RuntimeGovernanceResolutionProposal{ResolutionID: "resolution-forged"}
		}},
		{name: "open resolved at", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedAt = "2026-08-25T12:00:00Z" }},
		{name: "open resolved by", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedBy = "operator" }},
		{name: "open resolution notification", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolutionNotificationPublishedAt = "2026-08-25T12:00:01Z" }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			tickets := memory.NewDecisionTicketRepository()
			ticket := workspaceDefinitionClarificationTicketForCommand(command)
			ticket.State = common.DecisionTicketStateOpen
			test.mutate(ticket)
			if err := tickets.Create(context.Background(), ticket); err != nil {
				t.Fatal(err)
			}
			owner := &Biz{tickets: tickets, resolutions: memory.NewDecisionResolutionRepository()}
			_, err := owner.queryWorkspaceDefinitionClarificationFact(context.Background(), decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{
				WorkspaceID: command.WorkspaceID, TicketID: command.TicketID,
			})
			if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("query error=%v, want conflict", err)
			}
		})
	}
}

func TestWorkspaceDefinitionClarificationTicketSemanticsRejectsResolvedTimeDrift(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	valid := workspaceDefinitionClarificationTicketForCommand(command)
	valid.State = common.DecisionTicketStateResolved
	valid.ResolvedAt = "2026-08-25T12:00:00Z"
	if !workspaceDefinitionClarificationTicketSemanticsMatch(valid, command) {
		t.Fatal("resolved Definition clarification with owner-stamped time was rejected")
	}
	withNotification := decisionmodel.CloneTicket(valid)
	withNotification.ResolutionNotificationPublishedAt = "2026-08-25T12:00:01Z"
	if !workspaceDefinitionClarificationTicketSemanticsMatch(withNotification, command) {
		t.Fatal("resolved Definition clarification rejected optional published-notification cut point")
	}

	for _, test := range []struct {
		name   string
		mutate func(*decisionmodel.Ticket)
	}{
		{name: "resolved at missing", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedAt = "" }},
		{name: "resolved by forged", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedBy = "operator" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ticket := decisionmodel.CloneTicket(valid)
			test.mutate(ticket)
			if workspaceDefinitionClarificationTicketSemanticsMatch(ticket, command) {
				t.Fatalf("resolved state-time drift was accepted: %+v", ticket)
			}
		})
	}
}
