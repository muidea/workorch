package biz

import (
	"context"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestStartupPreparationApplicationExactReadAndDetachedProjection(t *testing.T) {
	for _, scenario := range []string{"unchecked", "unknown", "unregistered", "unbound_diagnostic", "scope", "invalid_state", "wrong_type", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			hub := magicevent.NewHub(32)
			defer hub.Terminate(context.Background())
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			calls := 0
			reply := workspaceevents.QueryWorkspacePreparationStateResult{WorkspaceID: "ws", WorkspaceRevision: 2, ActiveRunID: "actual-run", State: "not_checked", CurrentValidity: "not_checked"}
			if scenario == "unknown" || scenario == "unregistered" || scenario == "unbound_diagnostic" {
				reply.State, reply.CurrentValidity = "unknown", "stale"
				reply.Record = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: "ws", Attempts: []workspace.WorkspacePreparationAttempt{{
					CreationEpoch: strings.Repeat("a", 32), Checks: []workspace.WorkspacePreparationCheckResult{{ID: "exact_seal", Owner: "workspace", Required: true, State: "pending", Reason: "not_observed"}}, RequestID: "original-request", PayloadHash: "payload", ExpectedWorkspaceRevision: 1, CommittedWorkspaceRevision: 2, State: workspace.WorkspacePreparationUnknown, StartedAt: time.Now().UTC(),
					Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"},
				}}}
			}
			if scenario == "unregistered" || scenario == "unbound_diagnostic" {
				reply.SkillHubProjectIssue = "project_registration_required"
			}
			if scenario == "unregistered" {
				reply.Record.Version, reply.State = workspace.WorkspacePreparationWorkflowVersion, "blocked"
				a := &reply.Record.Attempts[0]
				a.State, a.CompletedAt = workspace.WorkspacePreparationBlocked, a.StartedAt.Add(time.Second)
				a.CreationEpoch = strings.Repeat("a", 32)
				a.SkillHubEffectRef, a.SkillHubEffectState = "skillhub-status:"+strings.Repeat("b", 32), workspace.PreparationSkillHubUnresolved
				a.Checks = []workspace.WorkspacePreparationCheckResult{{ID: "exact_seal", Owner: "workspace", Required: true, State: "blocked", Reason: "check_pipeline_incomplete"}}
			}
			if scenario == "scope" {
				reply.WorkspaceID = "other"
			}
			if scenario == "invalid_state" {
				reply.State = "ready"
			}
			if err := owner.Subscribe(workspaceevents.TopicWorkspacePreparationStateQuery, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				command, ok := ev.Data().(workspaceevents.QueryWorkspacePreparationStateCommand)
				if !ok || command.WorkspaceID != "ws" || ev.Source() != ModuleID || ev.Destination() != workspaceevents.CommandDestination {
					t.Fatal("wrong typed query")
				}
				if scenario == "wrong_type" {
					result.Set("not a typed reply", nil)
					return
				}
				result.Set(reply, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer owner.Unsubscribe(workspaceevents.TopicWorkspacePreparationStateQuery)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			view, err := b.ReadStartupPreparationView(ctx, "ws")
			valid := scenario == "unchecked" || scenario == "unknown" || scenario == "unregistered"
			if (err == nil) != valid {
				t.Fatalf("view=%+v err=%v", view, err)
			}
			if scenario == "cancelled" {
				if calls != 0 {
					t.Fatal("cancelled query sent")
				}
				return
			}
			if calls != 1 {
				t.Fatalf("implicit work/query count=%d", calls)
			}
			if !valid {
				return
			}
			if view.ValidateFor("ws") != nil || view.ActiveRunID != "actual-run" {
				t.Fatal("projection lost independent owner facts")
			}
			if scenario == "unregistered" && (view.SkillHubProjectIssue != "project_registration_required" || view.Record.Attempts[0].SkillHubEffectState != "unresolved") {
				t.Fatal("negative projection lost original diagnostic/fence")
			}
			if scenario == "unknown" {
				if view.OperationState != "unknown" || view.CurrentValidity != "stale" || view.Record.Attempts[0].Binding.InputHash != "input" {
					t.Fatal("projection settled unknown or changed source identity")
				}
				view.Record.Attempts[0].Binding.InputHash = "caller mutation"
				if reply.Record.Attempts[0].Binding.AuthoringRef.InputHash != "input" {
					t.Fatal("public DTO aliases owner")
				}
			}
		})
	}
}

func TestExactStartupApplicationSenderRejectsUnboundOwnerReply(t *testing.T) {
	for _, scenario := range []string{"unbound_reply", "wrong_type", "cancelled", "zero_revision"} {
		t.Run(scenario, func(t *testing.T) {
			hub := magicevent.NewHub(32)
			defer hub.Terminate(context.Background())
			owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			calls := 0
			if err := owner.Subscribe(workspaceevents.TopicWorkspaceExactConfirmedStartupQuery, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				command, ok := ev.Data().(workspaceevents.QueryExactConfirmedStartupSnapshotCommand)
				if !ok || command.WorkspaceID != "ws" || command.ExpectedWorkspaceRevision != 3 || ev.Source() != ModuleID {
					t.Error("wrong exact query identity")
				}
				if scenario == "wrong_type" {
					result.Set(workspaceevents.QueryWorkspacePreparationStateResult{}, nil)
					return
				}
				result.Set(workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer owner.Unsubscribe(workspaceevents.TopicWorkspaceExactConfirmedStartupQuery)
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: "ws", ExpectedWorkspaceRevision: 3}
			if scenario == "cancelled" {
				cancel()
			}
			if scenario == "zero_revision" {
				command.ExpectedWorkspaceRevision = 0
			}
			if _, err := b.QueryExactConfirmedStartupSnapshot(ctx, command); err == nil {
				t.Fatal("unbound/cancelled exact snapshot accepted")
			}
			want := 1
			if scenario == "cancelled" || scenario == "zero_revision" {
				want = 0
			}
			if calls != want {
				t.Fatalf("query count=%d want=%d", calls, want)
			}
		})
	}
}
