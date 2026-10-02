package workspace

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAuthoringSaveReceiptRejectsForgedIdentityAndSurvivesABA(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	template := workspace.WorkspaceTemplate{ID: "t"}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	id := workspace.DefinitionAuthoringWorkspaceID("save-receipt")
	create := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "save-receipt", WorkspaceID: id, Title: "任务", DeliverableDirectory: "/project", InternalDirectory: "/internal", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: "t", GoalText: "A"}}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, create); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	input := current.DefinitionAuthoring.Input.Clone()
	input.Revision++
	input.GoalText = "B"
	first := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "first", Input: input, ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
	submit := func(cmd workspaceevents.SaveDefinitionAuthoringCommand) (definitionSpecificationTransition, error) {
		return owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &cmd})
	}
	saved, err := submit(first)
	if err != nil {
		t.Fatal(err)
	}
	receipt := saved.Authoring.Receipt
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	forged := first
	forged.RequestID = "forged"
	forged.Input = current.DefinitionAuthoring.Input.Clone()
	forged.ExpectedWorkspaceRevision = current.Revision
	forged.ExpectedInputHash = "invented"
	if _, err := submit(forged); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("伪造身份被接受: %v", err)
	}
	altered := first
	altered.ExpectedInputHash = "invented"
	if _, err := submit(altered); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("RequestID 被复用: %v", err)
	}
	for _, text := range []string{"A", "B"} {
		next := current.DefinitionAuthoring.Input.Clone()
		next.Revision++
		next.GoalText = text
		cmd := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "next/" + next.InputHash(), Input: next, ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
		if _, err := submit(cmd); err != nil {
			t.Fatal(err)
		}
		current, err = owners.repositories.Workspaces.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	revision, hash := current.Revision, current.DefinitionAuthoring.InputHash
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	// 已成功保存的重放不依赖模板仍然存在，也不能恢复到 ABA 前的输入。
	if err := owners.repositories.WorkspaceTemplates.Delete(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	replay, err := submit(first)
	if err != nil || replay.Authoring.Receipt != receipt || replay.Authoring.Record.InputHash != hash {
		t.Fatalf("原保存回执未恢复: %+v %v", replay, err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || current.Revision != revision || len(current.DefinitionAuthoring.SaveReceipts) != 3 || len(current.DefinitionAuthoring.DerivationRequests) != 0 || current.StartupInput != nil {
		t.Fatalf("重放改变 owner: %+v %v", current, err)
	}
}

func TestAuthoringSaveReceiptLimitAndConcurrentCAS(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	template := workspace.WorkspaceTemplate{ID: "t"}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	id := workspace.DefinitionAuthoringWorkspaceID("save-limit")
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: "t", GoalText: "目标"}
	create := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "save-limit", WorkspaceID: id, Title: "任务", DeliverableDirectory: "/project", InternalDirectory: "/internal", Input: input}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, create); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	submit := func(cmd workspaceevents.SaveDefinitionAuthoringCommand) (definitionSpecificationTransition, error) {
		return owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &cmd})
	}
	var commands [2]workspaceevents.SaveDefinitionAuthoringCommand
	for n := range commands {
		next := input.Clone()
		next.Revision++
		next.GoalText = fmt.Sprintf("并发 %d", n)
		commands[n] = workspaceevents.SaveDefinitionAuthoringCommand{RequestID: fmt.Sprintf("compete-%d", n), Input: next, ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
	}
	start := make(chan struct{})
	var wait sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, cmd := range commands {
		wait.Add(1)
		go func() { defer wait.Done(); <-start; _, err := submit(cmd); outcomes <- err }()
	}
	close(start)
	wait.Wait()
	close(outcomes)
	success := 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if !errors.Is(err, common.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("同 CAS 成功数=%d", success)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || len(current.DefinitionAuthoring.SaveReceipts) != 1 {
		t.Fatal("竞争留下额外回执")
	}
	var first workspaceevents.SaveDefinitionAuthoringCommand
	for n := 1; n < workspace.DefinitionAuthoringSaveReceiptLimit; n++ {
		cmd := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: fmt.Sprintf("no-op-%d", n), Input: current.DefinitionAuthoring.Input.Clone(), ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
		if n == 1 {
			first = cmd
		}
		if _, err := submit(cmd); err != nil {
			t.Fatal(err)
		}
		current, err = owners.repositories.Workspaces.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	revision := current.Revision
	overflow := first
	overflow.RequestID = "overflow"
	overflow.ExpectedWorkspaceRevision = revision
	if _, err := submit(overflow); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("回执上限未拒绝: %v", err)
	}
	if _, err := submit(first); err != nil {
		t.Fatalf("额度耗尽后原回执不可重放: %v", err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || current.Revision != revision || len(current.DefinitionAuthoring.SaveReceipts) != workspace.DefinitionAuthoringSaveReceiptLimit {
		t.Fatal("额度耗尽或重放改变持久状态")
	}
}
