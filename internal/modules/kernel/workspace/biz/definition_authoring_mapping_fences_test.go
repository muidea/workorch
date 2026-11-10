package workspace

import (
	"context"
	"reflect"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// Completed owner results are immutable at CAS. Coordinated source edits are
// rejected before publication; subsequent reads consume the original result.
func TestAuthoringCompletedCurrentAndHistoryRejectRewriteBeforePublication(t *testing.T) {
	for _, scope := range []string{"current", "history"} {
		t.Run(scope, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			confirmation := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-mapping-fences")
			if scope == "history" {
				if err := owners.workspace.Start(); err != nil {
					t.Fatal(err)
				}
				application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
				if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation); err != nil {
					t.Fatal(err)
				}
				reviewedAuthoringSuccessor(t, owners, confirmation.WorkspaceID, false)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			input := current.DefinitionAuthoring.Input.Clone()
			input.Revision++
			input.GoalText = "下一份真实修订目标"
			save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-fence", Input: input, ExpectedInputHash: current.DefinitionAuthoring.InputHash, ExpectedWorkspaceRevision: current.Revision}
			reserve := workspaceevents.ReserveDefinitionDerivationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "derive"), WorkspaceID: current.ID, InputHash: current.DefinitionAuthoring.InputHash, RequestID: "derive-fence", ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
			observation := definitionSpecificationObservation{workspace: current, template: &current.DefinitionAuthoring.TemplateSnapshot}
			for _, event := range []definitionSpecificationInternalEvent{{WorkspaceID: current.ID, Authoring: &save}, {WorkspaceID: current.ID, DeriveReserve: &reserve}} {
				transition, err := reduceDefinitionSpecificationInternalEvent(observation, event)
				if err != nil || transition.Next == nil {
					t.Fatalf("合法前像必须能够生成有效 CAS 后像：%v", err)
				}
			}
			record := current.DefinitionAuthoring
			if scope == "history" {
				if len(current.DefinitionAuthoringHistory) != 1 {
					t.Fatal("必须使用真实确认后继归档")
				}
				record = &current.DefinitionAuthoringHistory[0].Record
			}
			proposal := record.SpecificationRequests[0].Proposal.Clone()
			proposal.Verifications[0].ProfileID = "invented"
			resealAuthoringProposalClaimForTest(t, record, proposal)
			if err := record.Validate(); err != nil {
				t.Fatalf("反例必须到达独立封存 Profile 门禁：%v", err)
			}
			persisted, err := owners.repositories.Workspaces.Get(ctx, current.ID)
			if err != nil {
				t.Fatal(err)
			}
			if validateAuthoringMappingOutcomes(*record) == nil {
				t.Fatal("offline audit did not expose unaccepted semantic drift")
			}
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err == nil {
				t.Fatal("completed owner result rewritten at CAS")
			}
			after, err := owners.repositories.Workspaces.Get(ctx, current.ID)
			if err != nil || !reflect.DeepEqual(persisted, after) {
				t.Fatalf("rejected rewrite changed original: %v", err)
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			for range 3 {
				if _, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: current.ID}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
