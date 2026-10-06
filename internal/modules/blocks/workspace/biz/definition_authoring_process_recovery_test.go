package workspace

import (
	"context"
	"encoding/json"
	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

// 子进程在持久切点直接退出，不运行 owner teardown，父进程只打开测试目录。
func TestAuthoringProcessConfirmationRecovery(t *testing.T) {
	if root := os.Getenv("WORKORCH_AUTHORING_PROCESS_ROOT"); root != "" {
		ctx := context.Background()
		owners := openFileDefinitionAdmissionOwners(t, root)
		cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-process-confirm")
		encoded, err := json.Marshal(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "original-confirmation.json"), encoded, 0600); err != nil {
			t.Fatal(err)
		}
		w, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		journal, err := owners.workspace.prepareDefinitionAuthoringConfirmation(ctx, w, cmd)
		if err != nil {
			t.Fatal(err)
		}
		next := *w
		next.DefinitionAuthoringConfirmation = &journal
		if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &next, w.Revision); err != nil {
			t.Fatal(err)
		}
		cut := os.Getenv("WORKORCH_AUTHORING_PROCESS_CUT")
		if cut == "goal" || cut == "dod" {
			if err := owners.repositories.Goals.Create(ctx, &journal.Goal); err != nil {
				t.Fatal(err)
			}
		}
		if cut == "dod" {
			if err := owners.repositories.DoDs.Create(ctx, &journal.DoD); err != nil {
				t.Fatal(err)
			}
		}
		os.Exit(0)
	}
	for _, cut := range []string{"prepared", "goal", "dod"} {
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestAuthoringProcessConfirmationRecovery$", "-test.count=1")
			command.Env = append(os.Environ(), "WORKORCH_AUTHORING_PROCESS_ROOT="+root, "WORKORCH_AUTHORING_PROCESS_CUT="+cut)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("子进程切点失败: %v\n%s", err, output)
			}
			data, err := os.ReadFile(filepath.Join(root, "original-confirmation.json"))
			if err != nil {
				t.Fatal(err)
			}
			var cmd workspaceevents.ConfirmDefinitionAuthoringCommand
			if err := json.Unmarshal(data, &cmd); err != nil {
				t.Fatal(err)
			}
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			ctx := context.Background()
			first, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd})
			if err != nil || first.ValidateFor(cmd) != nil {
				t.Fatalf("独立进程确认恢复: %+v %v", first, err)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			revision := current.Revision
			if current.DefinitionAuthoringConfirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || current.StartupInput != nil || current.ActiveRunID != "" || len(current.DefinitionAuthoring.DerivationRequests) != 1 || len(current.DefinitionAuthoring.SpecificationRequests) != 7 {
				t.Fatal("恢复改变原编译或提前启动")
			}
			second, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd})
			if err != nil || second.ValidateFor(cmd) != nil {
				t.Fatal(err)
			}
			current, err = owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || current.Revision != revision {
				t.Fatal("确认重放再次持久写入")
			}
		})
	}
}

func TestAuthoringProcessDerivationPermitRecovery(t *testing.T) {
	const helperRoot = "WORKORCH_AUTHORING_DERIVATION_PROCESS_ROOT"
	const helperCut = "WORKORCH_AUTHORING_DERIVATION_PROCESS_CUT"
	id := workspace.DefinitionAuthoringWorkspaceID("process-derive")
	ctx := context.Background()
	if root := os.Getenv(helperRoot); root != "" {
		owners := openFileDefinitionAdmissionOwners(t, root)
		template := workspace.WorkspaceTemplate{ID: "t"}
		if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
			t.Fatal(err)
		}
		input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: "t", GoalText: "目标"}
		create := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "process-derive", WorkspaceID: id, Title: "任务", DeliverableDirectory: "/project", InternalDirectory: "/internal", Input: input}
		if _, err := owners.workspace.createDefinitionAuthoring(ctx, create); err != nil {
			t.Fatal(err)
		}
		reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: id, InputHash: input.InputHash(), RequestID: "original-derive", ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
		result, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveReserve: &reserve})
		if err != nil || result.Derivation.FreshPermitID == "" {
			t.Fatalf("首次持久 reserve: %v", err)
		}
		if os.Getenv(helperCut) == "completed" {
			derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"验收"}}
			done := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: id, PermitID: result.Derivation.FreshPermitID, Proposal: &derived}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveComplete: &done}); err != nil {
				t.Fatal(err)
			}
		}
		os.Exit(0)
	}
	for _, cut := range []string{"reserved", "completed"} {
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestAuthoringProcessDerivationPermitRecovery$", "-test.count=1")
			command.Env = append(os.Environ(), helperRoot+"="+root, helperCut+"="+cut)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("子进程派生切点: %v\n%s", err, output)
			}
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			current, err := owners.repositories.Workspaces.Get(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			revision := current.Revision
			original := current.DefinitionAuthoring.DerivationRequests[0].Clone()
			reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: id, InputHash: current.DefinitionAuthoring.InputHash, RequestID: "original-derive", ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
			replay, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveReserve: &reserve})
			if err != nil || replay.Derivation.FreshPermitID != "" || replay.Derivation.Request.PermitID != original.PermitID {
				t.Fatalf("重启重放重新授权: %+v %v", replay, err)
			}
			if (replay.Derivation.Request.Proposal != nil) != (cut == "completed") {
				t.Fatal("未决/完成状态未恢复")
			}
			current, err = owners.repositories.Workspaces.Get(ctx, id)
			if err != nil || current.Revision != revision || len(current.DefinitionAuthoring.DerivationRequests) != 1 || current.ActiveRunID != "" || current.StartupInput != nil {
				t.Fatal("重启重放改变 owner")
			}
		})
	}
}

func TestAuthoringProcessDecisionAdmissionRecovery(t *testing.T) {
	const rootKey = "WORKORCH_AUTHORING_DECISION_PROCESS_ROOT"
	const cutKey = "WORKORCH_AUTHORING_DECISION_PROCESS_CUT"
	ctx := context.Background()
	if root := os.Getenv(rootKey); root != "" {
		owners := openFileDefinitionAdmissionOwners(t, root)
		first := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-process-decision")
		if err := owners.workspace.Start(); err != nil {
			t.Fatal(err)
		}
		app := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
		blocked, err := app.ConfirmAndAdmitDefinitionAuthoring(ctx, first)
		if err != nil || blocked.BlockingTicketID == "" {
			t.Fatalf("创建澄清: %v", err)
		}
		second := reviewedAuthoringSuccessor(t, owners, first.WorkspaceID, true)
		if os.Getenv(cutKey) == "decision-persisted" {
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: second}); err != nil {
				t.Fatal(err)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, second.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
			cmd := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
			pending, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, cmd)
			if err != nil || pending.PriorClarification == nil {
				t.Fatalf("pending candidate: %v", err)
			}
			identity, err := pending.PriorClarification.DecisionRequestIdentity()
			if err != nil {
				t.Fatal(err)
			}
			opened, err := governanceports.OpenWorkspaceDefinitionClarification(ctx, owners.hub, decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(identity))
			if err != nil {
				t.Fatal(err)
			}
			cmd.ClarificationTicket = &workspaceevents.WorkspaceDefinitionClarificationTicketFact{RequestIdentity: identity, TicketBinding: opened.BindingValue()}
			pending, err = owners.workspace.submitDefinitionAuthoringAdmission(ctx, cmd)
			if err != nil || pending.ActivationAdmission == nil || pending.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution {
				t.Fatalf("pending seal: %v", err)
			}
			resolve := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{TicketID: opened.TicketID, WorkspaceID: candidate.WorkspaceID, OldCandidateID: pending.PriorClarification.Candidate.CandidateID, OldCandidateRevision: pending.PriorClarification.Candidate.CandidateRevision, OldCandidateHash: pending.PriorClarification.Candidate.CandidateHash, NewCandidateID: pending.Candidate.CandidateID, NewCandidateRevision: pending.Candidate.CandidateRevision, NewCandidateHash: pending.Candidate.CandidateHash, AdmissionResultHash: pending.AdmissionResult.AdmissionResultHash, DefinitionRevision: pending.StartupInput.DefinitionRevision, StartupInputHash: pending.StartupInput.StartupInputHash}
			if _, err := governanceports.ResolveWorkspaceDefinitionClarificationByAdmission(ctx, owners.hub, resolve); err != nil {
				t.Fatal(err)
			}
			current, err = owners.repositories.Workspaces.Get(ctx, second.WorkspaceID)
			if err != nil || current.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution || current.ActiveRunID != "" {
				t.Fatal("Decision step 提前推进 Workspace")
			}
		}
		if os.Getenv(cutKey) == "resolved" {
			admitted, err := app.ConfirmAndAdmitDefinitionAuthoring(ctx, second)
			if err != nil || admitted.ActivationAdmission == nil || admitted.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
				t.Fatalf("决议准入: %v", err)
			}
		}
		data, err := json.Marshal(second)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "original-successor.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
	for _, cut := range []string{"before-resolution", "decision-persisted", "resolved"} {
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			child := exec.Command(os.Args[0], "-test.run=^TestAuthoringProcessDecisionAdmissionRecovery$", "-test.count=1")
			child.Env = append(os.Environ(), rootKey+"="+root, cutKey+"="+cut)
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("决议子进程: %v\n%s", err, output)
			}
			data, err := os.ReadFile(filepath.Join(root, "original-successor.json"))
			if err != nil {
				t.Fatal(err)
			}
			var cmd workspaceevents.ConfirmDefinitionAuthoringCommand
			if err := json.Unmarshal(data, &cmd); err != nil {
				t.Fatal(err)
			}
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			app := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
			result, err := app.ConfirmAndAdmitDefinitionAuthoring(ctx, cmd)
			if err != nil || result.ActivationAdmission == nil || result.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
				t.Fatalf("决议进程恢复: %+v %v", result, err)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			revision := current.Revision
			scoped := common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
			tickets, err := owners.repositories.Tickets.ListByWorkspaceID(scoped, cmd.WorkspaceID)
			if err != nil || len(tickets) != 1 || tickets[0].State != common.DecisionTicketStateResolved || current.ActiveRunID != "" {
				t.Fatal("决议未持久恢复或重复创建")
			}
			replay, err := app.ConfirmAndAdmitDefinitionAuthoring(ctx, cmd)
			if err != nil || replay.StartupInput.StartupInputHash != result.StartupInput.StartupInputHash {
				t.Fatal("重放 startup 身份变化")
			}
			current, err = owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || current.Revision != revision {
				t.Fatal("决议重放额外写入")
			}
		})
	}
}
