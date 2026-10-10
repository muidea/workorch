package biz

import (
	"context"
	"errors"
	"fmt"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionAuthoringViewSelectsCompletedRequestAfterFailedAttempt(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var err error
	record, first, _, err := record.ReserveDerivation("attempt-1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(first.PermitID, nil, "provider_failed")
	if err != nil {
		t.Fatal(err)
	}
	record, second, _, err := record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	acceptance := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"明确验收"}}
	record, err = record.CompleteDerivation(second.PermitID, &acceptance, "")
	if err != nil {
		t.Fatal(err)
	}
	query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: 2, Record: authoringTestRecord(record), CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
	view, err := definitionAuthoringView(query)
	if err != nil || view.State != "derived" || view.DerivationPermitID != second.PermitID || view.Proposal == nil || len(view.Proposal.Items) != 1 || view.ConfirmationRecovery != nil {
		t.Fatalf("historical failure masked completed request: %+v %v", view, err)
	}
}

func TestDefinitionAuthoringViewKeepsFailureAndConfirmationVisible(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var err error
	record, failed, _, err := record.ReserveDerivation("attempt-1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(failed.PermitID, nil, "provider_failed")
	if err != nil {
		t.Fatal(err)
	}
	record, current, _, err := record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 2, Record: authoringTestRecord(record), CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
	view, err := definitionAuthoringView(query)
	if err != nil || view.State != "outcome_unresolved" || view.DerivationPermitID != current.PermitID {
		t.Fatalf("multiple attempts should select the current stable request: %+v %v", view, err)
	}
	// Even a pending historical attempt must not hide the confirmed admission
	// and exact review identity in the management projection.
	query.Confirmation = &workspace.DefinitionAuthoringConfirmation{InputHash: record.InputHash, DerivationPermitID: current.PermitID}
	if _, err := definitionAuthoringView(query); err == nil {
		t.Fatal("invalid confirmation was accepted by the projection")
	}
}

func TestDefinitionAuthoringViewExposesProposalFailureAndUnresolvedOutcome(t *testing.T) {
	for _, failure := range []string{"", "invalid_proposal"} {
		t.Run(failure, func(t *testing.T) {
			input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t"}
			template := workspace.WorkspaceTemplate{ID: "t"}
			record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
			record, derivation, _, err := record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
			if err != nil {
				t.Fatal(err)
			}
			acceptance := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"明确验收"}}
			record, err = record.CompleteDerivation(derivation.PermitID, &acceptance, "")
			if err != nil {
				t.Fatal(err)
			}
			record, proposal, _, err := record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
			if err != nil {
				t.Fatal(err)
			}
			if failure != "" {
				record, err = record.CompleteSpecification(proposal.PermitID, nil, nil, failure)
				if err != nil {
					t.Fatal(err)
				}
			}
			query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 4, Record: authoringTestRecord(record), CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
			view, err := definitionAuthoringView(query)
			want := "outcome_unresolved"
			if failure != "" {
				want = "failed"
			}
			if err != nil || view.State != want || len(view.Requests) != 1 || view.Requests[0].FailureCode != failure || view.Proposal == nil || view.Proposal.ProposalHash != "" {
				t.Fatalf("proposal outcome hidden as derived: %+v %v", view, err)
			}
		})
	}
}

func TestSaveDefinitionAuthoringViewReplaysLostResponse(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(_ magicevent.Event, result magicevent.Result) {
		copy := record.Clone()
		result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: 3, Record: authoringTestRecord(copy), CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: copy.TemplateSnapshotHash}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave, func(ev magicevent.Event, result magicevent.Result) {
		calls++
		cmd := ev.Data().(workspaceevents.SaveDefinitionAuthoringCommand)
		if cmd.Input.GoalText != input.GoalText {
			result.Set(nil, nil)
			return
		}
		receipt := workspace.DefinitionAuthoringSaveReceipt{RequestID: cmd.RequestID, RequestHash: workspace.DefinitionAuthoringSaveRequestHash(cmd.RequestID, cmd.Input, cmd.ExpectedWorkspaceRevision, cmd.ExpectedInputHash, cmd.DeliverableDirectory), InputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, WorkspaceRevision: 3}
		record.SaveReceipts = []workspace.DefinitionAuthoringSaveReceipt{receipt}
		result.Set(workspaceevents.SaveDefinitionAuthoringResult{Record: *authoringTestRecord(record), Receipt: receipt}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	request := workorchmanagementcontract.DefinitionAuthoringSave{RequestID: "original-save", TemplateID: "t", Input: workorchmanagementcontract.DefinitionAuthoringInput{GoalText: "目标"}, ExpectedWorkspaceRevision: 2, ExpectedInputHash: record.InputHash}
	view, err := b.SaveDefinitionAuthoringView(ctx, "ws", request)
	if err != nil || view.InputHash != record.InputHash || calls != 1 {
		t.Fatalf("lost response retry changed owner state: %+v %v calls=%d", view, err, calls)
	}
	request.Input.GoalText = "修改"
	if _, err := b.SaveDefinitionAuthoringView(ctx, "ws", request); !errors.Is(err, common.ErrConflict) || calls != 2 {
		t.Fatalf("stale edit was accepted: %v calls=%d", err, calls)
	}
}

func TestSaveDefinitionAuthoringViewFencesPostSaveObservation(t *testing.T) {
	for _, replaced := range []bool{false, true} {
		t.Run(fmt.Sprint(replaced), func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			defer hub.Terminate(ctx)
			owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
			defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave)
			input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
			template := workspace.WorkspaceTemplate{ID: "t"}
			record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
			saved := false
			queries := 0
			if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(_ magicevent.Event, result magicevent.Result) {
				queries++
				copy := record.Clone()
				revision := uint64(1)
				if saved {
					revision = 3
					if replaced {
						copy.Input.Revision++
						copy.Input.GoalText = "后续修订"
						copy.InputHash = copy.Input.InputHash()
					}
				}
				result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: revision, Record: authoringTestRecord(copy), CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: copy.TemplateSnapshotHash}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave, func(ev magicevent.Event, result magicevent.Result) {
				cmd := ev.Data().(workspaceevents.SaveDefinitionAuthoringCommand)
				receipt := workspace.DefinitionAuthoringSaveReceipt{RequestID: cmd.RequestID, RequestHash: workspace.DefinitionAuthoringSaveRequestHash(cmd.RequestID, cmd.Input, cmd.ExpectedWorkspaceRevision, cmd.ExpectedInputHash, cmd.DeliverableDirectory), InputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, WorkspaceRevision: 2}
				record.SaveReceipts = []workspace.DefinitionAuthoringSaveReceipt{receipt}
				saved = true
				result.Set(workspaceevents.SaveDefinitionAuthoringResult{Record: *authoringTestRecord(record), Receipt: receipt}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			request := workorchmanagementcontract.DefinitionAuthoringSave{RequestID: "save", TemplateID: "t", Input: workorchmanagementcontract.DefinitionAuthoringInput{GoalText: "目标"}, ExpectedWorkspaceRevision: 1, ExpectedInputHash: record.InputHash}
			missing := request
			missing.TemplateID = " "
			if _, err := b.SaveDefinitionAuthoringView(ctx, "ws", missing); !errors.Is(err, common.ErrInvalidArgument) || queries != 0 {
				t.Fatal("缺少模板仍访问 owner")
			}
			view, err := b.SaveDefinitionAuthoringView(ctx, "ws", request)
			if replaced {
				if !errors.Is(err, common.ErrConflict) {
					t.Fatalf("后续输入被冒充原保存结果: %+v %v", view, err)
				}
			} else if err != nil || view.InputHash != record.InputHash || view.WorkspaceRevision != 3 {
				t.Fatalf("同输入合法推进被拒绝: %+v %v", view, err)
			}
			if queries != 2 {
				t.Fatalf("查询数=%d", queries)
			}
		})
	}
}
