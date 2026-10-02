package workspace

import (
	"context"
	"sync"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestDefinitionAuthoringRevisionChangesTemplateAndDirectoryAtomically(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	for _, id := range []string{"old", "new"} {
		if err := owners.repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	id := workspace.DefinitionAuthoringWorkspaceID("switch-template")
	cmd := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "switch-template", WorkspaceID: id, Title: "任务", DeliverableDirectory: "/old", InternalDirectory: "/internal", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: "old", GoalText: "用户目标"}}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	before, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	directory := "/new"
	input := cmd.Input.Clone()
	input.Revision++
	input.TemplateID = "new"
	save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", ExpectedInputHash: before.DefinitionAuthoring.InputHash, Input: input, ExpectedWorkspaceRevision: before.Revision, DeliverableDirectory: &directory}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	after, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.TemplateID != "new" || after.DeliverableDirectory != directory || after.DefinitionAuthoring.Input.Revision != 2 || after.DefinitionAuthoring.Input.GoalText != "用户目标" || after.DefinitionAuthoring.TemplateSnapshot.ID != "new" || after.DefinitionAuthoring.InputHash == before.DefinitionAuthoring.InputHash {
		t.Fatalf("revision did not atomically update inputs: %+v %v", after, err)
	}
	revision := after.Revision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatalf("lost response replay failed: %v", err)
	}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); err != nil {
		t.Fatalf("initial create retry after edits failed: %v", err)
	}
	after, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.Revision != revision || after.DefinitionAuthoring.InputHash != input.InputHash() {
		t.Fatal("initial creation replay reapplied old inputs")
	}
	input.Revision++
	input.TemplateID = "old"
	save.Input = input
	save.RequestID = "save-" + input.InputHash()
	save.ExpectedInputHash = after.DefinitionAuthoring.InputHash
	save.ExpectedWorkspaceRevision = revision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatalf("ABA revision failed: %v", err)
	}
	after, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.DefinitionAuthoring.InputHash == before.DefinitionAuthoring.InputHash || after.DefinitionAuthoring.Input.Revision != 3 {
		t.Fatal("ABA revision reused initial identity")
	}
	revision = after.Revision
	input.Revision++
	input.TemplateID = "missing"
	save.Input = input
	save.RequestID = "save-" + input.InputHash()
	save.ExpectedInputHash = after.DefinitionAuthoring.InputHash
	save.ExpectedWorkspaceRevision = revision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err == nil {
		t.Fatal("missing template accepted")
	}
	after, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.Revision != revision || after.TemplateID != "old" {
		t.Fatal("failed revision partially changed configuration")
	}
}

func TestAuthoringRevisionFencesHistoricalDerivationCompletion(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	template := workspace.WorkspaceTemplate{ID: "t"}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	id := workspace.DefinitionAuthoringWorkspaceID("completion-interleave")
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: "t", GoalText: "旧目标"}
	create := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "completion-interleave", WorkspaceID: id, Title: "任务", DeliverableDirectory: "/project", InternalDirectory: "/internal", Input: input}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, create); err != nil {
		t.Fatal(err)
	}
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: id, InputHash: input.InputHash(), RequestID: "old-derive", ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
	reserved, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveReserve: &reserve})
	if err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	next := input.Clone()
	next.Revision++
	next.GoalText = "新目标"
	save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "new-save", Input: next, ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"旧验收"}}
	complete := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: id, PermitID: reserved.Derivation.FreshPermitID, Proposal: &derived}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveComplete: &complete}); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || current.DefinitionAuthoring.InputHash != next.InputHash() || current.ActiveRunID != "" || current.StartupInput != nil {
		t.Fatal("旧完成改变新输入")
	}
	revision := current.Revision
	proposal := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: id, InputHash: next.InputHash(), RequestID: "old-proposal", DerivationPermitID: reserved.Derivation.FreshPermitID, ProviderBindingHash: "provider"}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecReserve: &proposal}); err == nil {
		t.Fatal("旧派生可为新输入授权 proposal")
	}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveComplete: &complete}); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || current.Revision != revision || len(current.DefinitionAuthoring.SpecificationRequests) != 0 {
		t.Fatal("拒绝或完成重放额外写入")
	}
}

func TestAuthoringSpecificationConcurrentReserveUsesOneFreshPermit(t *testing.T) {
	for _, kind := range []string{"", workspace.GoalReviewNegativePath} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			confirmation := seedReviewedAuthoringConfirmation(t, owners, "ws-reserve-competition")
			cmd := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: confirmation.WorkspaceID, InputHash: confirmation.InputHash, RequestID: "second-attempt", DerivationPermitID: confirmation.DerivationPermitID, ReviewKind: kind, ProviderBindingHash: "provider"}
			if kind != "" {
				cmd.ProposalHash = confirmation.ProposalHash
			}
			start := make(chan struct{})
			outcomes := make(chan definitionSpecificationTransition, 2)
			failures := make(chan error, 2)
			var wait sync.WaitGroup
			for range 2 {
				wait.Add(1)
				go func() {
					defer wait.Done()
					<-start
					result, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: cmd.WorkspaceID, AuthoringSpecReserve: &cmd})
					outcomes <- result
					failures <- err
				}()
			}
			close(start)
			wait.Wait()
			close(outcomes)
			close(failures)
			for err := range failures {
				if err != nil {
					t.Fatal(err)
				}
			}
			fresh := 0
			permit := ""
			for result := range outcomes {
				if result.AuthoringSpecification.FreshPermitID != "" {
					fresh++
				}
				if permit != "" && permit != result.AuthoringSpecification.Request.PermitID {
					t.Fatal("竞争产生不同 permit")
				}
				permit = result.AuthoringSpecification.Request.PermitID
			}
			if fresh != 1 {
				t.Fatalf("fresh permit 数=%d", fresh)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || len(current.DefinitionAuthoring.SpecificationRequests) != 8 {
				t.Fatal("竞争留下额外请求")
			}
			revision := current.Revision
			cmd.RequestID = "third-attempt"
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: cmd.WorkspaceID, AuthoringSpecReserve: &cmd}); err == nil {
				t.Fatal("第三次尝试被接受")
			}
			current, err = owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || current.Revision != revision || current.ActiveRunID != "" || current.StartupInput != nil {
				t.Fatal("拒绝额外写入或创建 Run")
			}
		})
	}
}

func TestAuthoringRevisionFencesHistoricalSpecificationCompletion(t *testing.T) {
	for _, kind := range []string{"", workspace.GoalReviewNegativePath} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			reviewed := seedReviewedAuthoringConfirmation(t, owners, "ws-spec-completion")
			current, err := owners.repositories.Workspaces.Get(ctx, reviewed.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			var proposal workspace.GoalSpecification
			for _, item := range current.DefinitionAuthoring.SpecificationRequests {
				if item.Proposal != nil {
					proposal = item.Proposal.Clone()
					break
				}
			}
			cmd := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: current.ID, InputHash: reviewed.InputHash, RequestID: "late-result", DerivationPermitID: reviewed.DerivationPermitID, ReviewKind: kind, ProviderBindingHash: "provider"}
			if kind != "" {
				cmd.ProposalHash = reviewed.ProposalHash
			}
			reserved, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, AuthoringSpecReserve: &cmd})
			if err != nil {
				t.Fatal(err)
			}
			current, err = owners.repositories.Workspaces.Get(ctx, current.ID)
			if err != nil {
				t.Fatal(err)
			}
			next := current.DefinitionAuthoring.Input.Clone()
			next.Revision++
			next.GoalText = "新目标"
			save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "new-input", Input: next, ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, Authoring: &save}); err != nil {
				t.Fatal(err)
			}
			complete := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: current.ID, PermitID: reserved.AuthoringSpecification.FreshPermitID}
			if kind == "" {
				complete.Proposal = &proposal
			} else {
				complete.Review = &workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "逐项覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{"goal-req"}}
			}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, AuthoringSpecComplete: &complete}); err != nil {
				t.Fatal(err)
			}
			current, err = owners.repositories.Workspaces.Get(ctx, current.ID)
			if err != nil {
				t.Fatal(err)
			}
			revision := current.Revision
			if current.DefinitionAuthoring.InputHash != next.InputHash() {
				t.Fatal("旧规格完成覆盖新输入")
			}
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: reviewed}); err == nil {
				t.Fatal("旧审查确认新输入")
			}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, AuthoringSpecComplete: &complete}); err != nil {
				t.Fatal(err)
			}
			current, err = owners.repositories.Workspaces.Get(ctx, current.ID)
			if err != nil || current.Revision != revision || current.ActiveRunID != "" || current.StartupInput != nil || current.DefinitionAuthoringConfirmation != nil {
				t.Fatal("拒绝旧确认或 completion 重放额外写入")
			}
		})
	}
}
