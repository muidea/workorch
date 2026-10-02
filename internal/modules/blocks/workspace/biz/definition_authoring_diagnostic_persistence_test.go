package workspace

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringFailureDiagnosticSurvivesFileReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-diagnostic", Revision: 1, GoalText: "目标", TemplateID: "t", AcceptanceRequirements: []string{"验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", Name: "模板", DoDItems: []string{"模板验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var derivation workspace.DefinitionDerivationRequest
	var err error
	record, derivation, _, err = record.ReserveDerivation("derive", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	acceptance := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充"}}
	record, err = record.CompleteDerivation(derivation.PermitID, &acceptance, "")
	if err != nil {
		t.Fatal(err)
	}
	specRequest := workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	record, specRequest, _, err = record.ReserveSpecification(specRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	raw := []byte("  {\"未知字段\":\"sk_test_marker\"}\r\n")
	diagnostic := workspace.NewDefinitionAuthoringDiagnostic("decode: 未知字段\n原始错误", raw)
	completion := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, PermitID: specRequest.PermitID, FailureCode: "invalid_proposal", Diagnostic: &diagnostic}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &completion}); err != nil {
		t.Fatal(err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	query := workspaceevents.QueryAuthoringDiagnosticCommand{WorkspaceID: input.WorkspaceID, PermitID: specRequest.PermitID}
	result, err := owners.workspace.queryAuthoringDiagnostic(ctx, query)
	if err != nil || !reflect.DeepEqual(result.Diagnostic, diagnostic) || !bytes.Equal(result.Diagnostic.Payload, raw) {
		t.Fatalf("原始诊断重开偏差: %+v %v", result, err)
	}
	result.Diagnostic.Payload[0] = '!'
	fresh, err := owners.workspace.queryAuthoringDiagnostic(ctx, query)
	if err != nil || !bytes.Equal(fresh.Diagnostic.Payload, raw) {
		t.Fatalf("查询未隔离: %+v %v", fresh, err)
	}
	before, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &completion}); err != nil {
		t.Fatal(err)
	}
	after, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil || after.Revision != before.Revision || after.ActiveRunID != "" || after.StartupInput != nil {
		t.Fatalf("重放改写或启动: %+v %v", after, err)
	}
	wrong := query
	wrong.WorkspaceID = "other-workspace"
	if _, err := owners.workspace.queryAuthoringDiagnostic(ctx, wrong); err == nil {
		t.Fatal("跨工作空间诊断查询未拒绝")
	}
	changed := diagnostic.Clone()
	changed.ParseError = "改写"
	completion.Diagnostic = &changed
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &completion}); err == nil {
		t.Fatal("变更诊断重放未拒绝")
	}
}
