package workspace

import (
	"context"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func testAdmissionCandidate() workspacemodel.DefinitionAdmissionCandidate {
	return (workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1,
		GoalInput:                 workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal", Text: "goal", SourceIdentity: "user"},
		DoDInputs:                 []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: "dod", Text: "done", SourceIdentity: "user"}},
		WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{TemplateRef: "template", Revision: "r1", SnapshotHash: "template-hash"},
		AdmissionCompilerVersion:  workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1",
		ProposedRelationInputs: []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "all", RelationKind: "all_of", SourceItemRefs: []string{"dod"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
}

func TestDefinitionAdmissionOwnerSubmitPromoteAndAcknowledge(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaces: repo}
	candidate := testAdmissionCandidate()
	submitted, err := biz.submitDefinitionAdmissionCandidate(ctx, workspaceevents.SubmitDefinitionAdmissionCandidateCommand{Candidate: candidate})
	if err != nil || submitted.Disposition != workspaceevents.DefinitionAdmissionCandidateCreated {
		t.Fatalf("submit = %+v err=%v", submitted, err)
	}
	duplicate, err := biz.submitDefinitionAdmissionCandidate(ctx, workspaceevents.SubmitDefinitionAdmissionCandidateCommand{Candidate: candidate})
	if err != nil || duplicate.Disposition != workspaceevents.DefinitionAdmissionCandidateDuplicate {
		t.Fatalf("duplicate = %+v err=%v", duplicate, err)
	}
	admission := (workspacemodel.DefinitionAdmissionResult{WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash, Disposition: workspacemodel.DefinitionAdmissionAdmittedAllOf, AdmissionCompilerVersion: candidate.AdmissionCompilerVersion, CanonicalAdmittedInputHash: "admitted-hash"}).WithHash()
	promoted, err := biz.promoteDefinitionAdmissionCandidate(ctx, workspaceevents.PromoteDefinitionAdmissionCandidateCommand{WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash, AdmissionResult: admission, BlockingTicketID: "ticket-1"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if promoted.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
		t.Fatalf("promotion = %+v", promoted)
	}
	ack, err := biz.acknowledgeWorkspaceDefinitionClarification(ctx, workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand{WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash, BlockingTicketID: "ticket-1", ResolutionID: "resolution-1", ResolutionBindingHash: "binding-hash", DefinitionRevision: promoted.DefinitionRevision, StartupInputHash: promoted.StartupInputHash})
	if err != nil || !ack.Transitioned || ack.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible {
		t.Fatalf("ack = %+v err=%v", ack, err)
	}
}
