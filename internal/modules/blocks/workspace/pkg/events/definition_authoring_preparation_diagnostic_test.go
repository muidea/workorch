package events

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestPreparationDiagnosticTypedTopicsAndStagePayloadInvariants(t *testing.T) {
	for _, topic := range []string{TopicWorkspacePreparationDiagnosticRecord, TopicWorkspacePreparationDiagnosticQuery} {
		found := false
		for _, contract := range Contracts() {
			if contract.Topic != topic {
				continue
			}
			found = true
			if contract.Owner != "workspace" || contract.Metadata.StateMachine != eventcontract.StateMachineNone {
				t.Fatal("preparation introduced another Runtime authority")
			}
			if topic == TopicWorkspacePreparationDiagnosticRecord && (contract.CommandClass != eventcontract.CommandClassWrite || contract.LanePolicy != eventcontract.LanePolicyState) {
				t.Fatal("record outside private state lane")
			}
			if topic == TopicWorkspacePreparationDiagnosticQuery && (contract.CommandClass != eventcontract.CommandClassRead || contract.LanePolicy != eventcontract.LanePolicyQuery) {
				t.Fatal("raw query outside read lane")
			}
		}
		if !found {
			t.Fatalf("topic unregistered: %s", topic)
		}
	}
	identity := workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: "ws", InputHash: "input", TemplateSnapshotHash: "template", ObservedWorkspaceRevision: 1, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "request", PhaseCode: "resources"}
	d := workspace.NewDefinitionAuthoringPreparationDiagnostic(identity, []byte("cause"))
	if (RecordPreparationDiagnosticCommand{Diagnostic: d}).Validate() != nil {
		t.Fatal("derivation preparation rejected")
	}
	for name, mutate := range map[string]func(*workspace.DefinitionAuthoringPreparationDiagnostic){
		"wrong-stage-resource": func(d *workspace.DefinitionAuthoringPreparationDiagnostic) {
			d.Stage = workspace.DefinitionAuthoringPreparationProposal
			d.DerivationPermitID = "permit"
		},
		"proposal-missing-permit": func(d *workspace.DefinitionAuthoringPreparationDiagnostic) {
			d.Stage = workspace.DefinitionAuthoringPreparationProposal
			d.PhaseCode = "render"
		},
		"review-missing-proposal": func(d *workspace.DefinitionAuthoringPreparationDiagnostic) {
			d.Stage = workspace.DefinitionAuthoringPreparationReview
			d.PhaseCode = "render"
			d.DerivationPermitID = "permit"
			d.ReviewKind = workspace.GoalReviewTraceability
		},
		"unknown-phase": func(d *workspace.DefinitionAuthoringPreparationDiagnostic) { d.PhaseCode = "owner-mutation" },
		"zero-revision": func(d *workspace.DefinitionAuthoringPreparationDiagnostic) { d.ObservedWorkspaceRevision = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := d.Clone()
			mutate(&bad)
			bad.DiagnosticID = bad.ExpectedDiagnosticID()
			if (RecordPreparationDiagnosticCommand{Diagnostic: bad}).Validate() == nil {
				t.Fatal("malformed typed preparation accepted")
			}
		})
	}
	for _, command := range []QueryPreparationDiagnosticCommand{{}, {WorkspaceID: "ws"}, {DiagnosticID: d.DiagnosticID}} {
		if command.Validate() == nil {
			t.Fatal("query missing scope/identity accepted")
		}
	}
}
