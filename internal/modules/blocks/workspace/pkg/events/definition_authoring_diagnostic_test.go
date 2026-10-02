package events

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringDiagnosticResultRejectsScopePhaseAndContentDrift(t *testing.T) {
	command := QueryAuthoringDiagnosticCommand{WorkspaceID: "ws", PermitID: "permit"}
	result := QueryAuthoringDiagnosticResult{WorkspaceID: command.WorkspaceID, PermitID: command.PermitID, Phase: workspace.DefinitionSpecificationProposal, FailureCode: "invalid_proposal", Diagnostic: workspace.NewDefinitionAuthoringDiagnostic("错误\n原文", []byte("  原文\r\n"))}
	if err := result.ValidateFor(command); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*QueryAuthoringDiagnosticResult){
		"workspace": func(r *QueryAuthoringDiagnosticResult) { r.WorkspaceID = "other" },
		"permit":    func(r *QueryAuthoringDiagnosticResult) { r.PermitID = "other" },
		"phase":     func(r *QueryAuthoringDiagnosticResult) { r.Phase = workspace.DefinitionSpecificationCritic },
		"failure":   func(r *QueryAuthoringDiagnosticResult) { r.FailureCode = "provider_failed" },
		"bytes":     func(r *QueryAuthoringDiagnosticResult) { r.Diagnostic.Payload[0] = '!' },
	} {
		t.Run(name, func(t *testing.T) {
			changed := result
			changed.Diagnostic = result.Diagnostic.Clone()
			mutate(&changed)
			if changed.ValidateFor(command) == nil {
				t.Fatal("诊断响应漂移未拒绝")
			}
		})
	}
}
