package eventmapping

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func preparationDTOFixture() (workspace.DefinitionAuthoringRecord, workspace.DefinitionAuthoringPreparationDiagnostic) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	r := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	d := workspace.NewDefinitionAuthoringPreparationDiagnostic(workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: "ws", InputHash: r.InputHash, TemplateSnapshotHash: r.TemplateSnapshotHash, ObservedWorkspaceRevision: 1, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "target", PhaseCode: "render"}, []byte("exact cause\r\n"))
	r.PreparationDiagnostics = []workspace.DefinitionAuthoringPreparationDiagnostic{d}
	return r, d
}

func TestPreparationDiagnosticDTOCompleteInternalModelMetadataOnlyJSON(t *testing.T) {
	r, d := preparationDTOFixture()
	dto := DefinitionAuthoringRecordDTO(r)
	if err := dto.Validate(); err != nil {
		t.Fatal(err)
	}
	model, err := dto.Model()
	if err != nil || !reflect.DeepEqual(model, r) {
		t.Fatalf("internal typed reconstruction not exact: %v", err)
	}
	cloned := dto.Clone()
	cloned.PreparationDiagnostics[0].Cause[0]++
	if dto.Validate() != nil || cloned.Validate() == nil {
		t.Fatal("DTO clone or raw validation bypass")
	}
	wire, err := json.Marshal(dto)
	if err != nil || bytes.Contains(wire, []byte("\"Cause\"")) || bytes.Contains(wire, []byte("exact cause")) {
		t.Fatalf("ordinary JSON exposed prep rawcause: %s %v", wire, err)
	}
	var decoded workspaceevents.DefinitionAuthoringRecord
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Validate() == nil {
		t.Fatal("metadata-only HTTP observation claimed complete stored rawcause")
	}
	cmd := workspaceevents.RecordPreparationDiagnosticCommand{Diagnostic: d}
	result := workspaceevents.RecordPreparationDiagnosticResult{WorkspaceID: "ws", WorkspaceRevision: 2, Diagnostic: DefinitionAuthoringPreparationDiagnosticMetadataDTO(d)}
	if result.ValidateFor(cmd) != nil {
		t.Fatal("valid receipt rejected")
	}
	result.Duplicate = true
	cmd.Diagnostic.ObservedWorkspaceRevision = 2
	if result.ValidateFor(cmd) != nil || result.Diagnostic.ObservedWorkspaceRevision != 1 {
		t.Fatal("new observation duplicate rejected or rebound")
	}
	copy := result.DeepClone()
	copy.Diagnostic.Cause[0]++
	if result.ValidateFor(cmd) != nil || copy.ValidateFor(cmd) == nil {
		t.Fatal("receipt clone alias")
	}
	wrong := cmd.DeepClone()
	wrong.Diagnostic.TargetRequestID = "other"
	wrong.Diagnostic.DiagnosticID = wrong.Diagnostic.ExpectedDiagnosticID()
	if result.ValidateFor(wrong) == nil {
		t.Fatal("wrong target receipt accepted")
	}
	query := workspaceevents.QueryPreparationDiagnosticCommand{WorkspaceID: "ws", DiagnosticID: d.DiagnosticID}
	read := workspaceevents.QueryPreparationDiagnosticResult{WorkspaceID: "ws", Diagnostic: d}
	if read.ValidateFor(query) != nil {
		t.Fatal("query rejected")
	}
	query.WorkspaceID = "other"
	if read.ValidateFor(query) == nil {
		t.Fatal("query wrong scope accepted")
	}
	query.WorkspaceID = "ws"
	query.DiagnosticID = "other"
	if read.ValidateFor(query) == nil {
		t.Fatal("query wrong ID accepted")
	}
	readClone := read.DeepClone()
	readClone.Diagnostic.Cause[0]++
	if reflect.DeepEqual(readClone.Diagnostic.Cause, read.Diagnostic.Cause) {
		t.Fatal("query rawbytes alias")
	}
}
