package workspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	definitionadmission "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type specLedgerFixture struct {
	owner       *Biz
	ctx         context.Context
	workspaceID common.WorkspaceID
	candidate   workspacemodel.DefinitionAdmissionCandidate
	begin       workspaceevents.BeginDefinitionSpecificationCommand
}

func openSpecificationLedgerFixture(t *testing.T, root string) (*specLedgerFixture, func()) {
	t.Helper()
	owners := openFileDefinitionAdmissionOwners(t, root)
	workspaceID := common.WorkspaceID("ws-spec-ledger")
	w := &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, TemplateID: "template-ledger", DeliverableDirectory: "/tmp/ws-spec-ledger"}
	template := &workspacemodel.WorkspaceTemplate{ID: w.TemplateID, Name: "generic"}
	w.TemplateBindingHash = workspacemodel.WorkspaceTemplateSnapshotHash(template)
	if err := owners.repositories.Workspaces.Create(context.Background(), w); err != nil {
		owners.close()
		t.Fatal(err)
	}
	if err := owners.repositories.WorkspaceTemplates.Create(context.Background(), template); err != nil {
		owners.close()
		t.Fatal(err)
	}
	// Formal owner writes assign monotonically increasing revisions independent
	// of text identity, including A -> B -> A.
	if err := owners.workspace.manager.DefineGoal(context.Background(), workspaceevents.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-1", Text: "deliver outcome A"}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	if err := owners.workspace.manager.DefineDoD(context.Background(), workspaceevents.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-1", Items: []string{"evidence exists"}}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(context.Background(), workspaceID)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	goals, err := owners.repositories.Goals.ListByWorkspaceID(context.Background(), workspaceID)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	goal, _, err := currentGoalDefinition(goals, workspaceID)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	dods, err := owners.repositories.DoDs.ListByWorkspaceID(context.Background(), workspaceID)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	dod, _, err := currentDoDDefinition(dods, workspaceID)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	snapshot := specificationLedgerCandidate(stored, goal, dod, template)
	begin := workspaceevents.BeginDefinitionSpecificationCommand{Candidate: snapshot, Dimensions: []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}, ExpectedWorkspaceRevision: stored.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(stored)}
	return &specLedgerFixture{owner: owners.workspace, ctx: context.Background(), workspaceID: workspaceID, candidate: snapshot, begin: begin}, func() { owners.close() }
}
func specificationLedgerCandidate(w *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, template *workspacemodel.WorkspaceTemplate) workspacemodel.DefinitionAdmissionCandidate {
	dodInputs := make([]workspacemodel.DefinitionCandidateItem, 0, len(dod.Items))
	refs := make([]string, 0, len(dod.Items))
	for i, item := range dod.Items {
		ref := "dod:" + string(dod.ID) + ":item:" + sixDigitIndex(i+1)
		refs = append(refs, ref)
		dodInputs = append(dodInputs, workspacemodel.DefinitionCandidateItem{CandidateItemRef: ref, Text: item, SourceIdentity: "dod:" + string(dod.ID) + "@" + dod.DefinitionRevision})
	}
	c := workspacemodel.DefinitionAdmissionCandidate{SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: w.ID, CandidateID: "workspace-definition", CandidateRevision: 1, GoalInput: workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal:" + string(goal.ID), Text: goal.Text, SourceIdentity: "goal:" + string(goal.ID) + "@" + goal.DefinitionRevision}, DoDInputs: dodInputs, WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{TemplateRef: template.ID, Revision: "template:" + workspacemodel.WorkspaceTemplateSnapshotHash(template), SnapshotHash: workspacemodel.WorkspaceTemplateSnapshotHash(template)}, RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil), AdmissionCompilerVersion: workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: w.DeliverableDirectory, ProposedRelationInputs: []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: refs, TargetItemRefs: []string{"goal:" + string(goal.ID)}}}}
	return c.WithHash()
}
func sixDigitIndex(n int) string { return fmt.Sprintf("%06d", n) }
func specLedgerIdentity(c workspacemodel.DefinitionAdmissionCandidate, phase workspacemodel.DefinitionSpecificationPhase, kind, logical, hash string) workspacemodel.DefinitionSpecificationRequestIdentity {
	return workspacemodel.DefinitionSpecificationRequestIdentity{WorkspaceID: c.WorkspaceID, CandidateID: c.CandidateID, CandidateRevision: c.CandidateRevision, CandidateHash: c.CandidateHash, CompilationInputHash: workspacemodel.DefinitionSpecificationCompilationInputHash(c.CandidateHash, nil, []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}), Phase: phase, ReviewKind: kind, LogicalRequestID: logical, ProposalHash: hash}
}
func TestSpecificationLedgerSameLogicalRequestCannotChangePhase(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	proposalID := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "unchanged-logical-identity", "")
	permitted, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: proposalID})
	if err != nil || permitted.PermitID == "" {
		t.Fatalf("proposal reservation: %+v err=%v", permitted, err)
	}
	changed := proposalID
	changed.Phase = workspacemodel.DefinitionSpecificationCritic
	changed.ReviewKind = workspacemodel.GoalReviewDimensionCoverage
	changed.ProposalHash = "same-candidate-different-phase"
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: changed}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same logical request changed immutable phase: %v", err)
	}
	stored, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || stored.DefinitionSpecificationLedger.Epochs[0].Count != 1 || stored.DefinitionSpecificationLedger.Epochs[0].Records[0].Identity.Phase != workspacemodel.DefinitionSpecificationProposal {
		t.Fatalf("phase mutation altered owner ledger: %+v err=%v", stored, err)
	}
}

func TestSpecificationLedgerBeginDoesNotAdmitAndReservesBeforePermit(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	first, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil {
		t.Fatal(err)
	}
	if first.CandidateHash != f.candidate.CandidateHash || first.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(f.candidate.CandidateHash, f.begin.Resources, f.begin.Dimensions) || first.GoalRevision == "" || first.DoDRevision == "" {
		t.Fatalf("unsealed begin: %+v", first)
	}
	w, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.DefinitionAdmissionCandidates) != 0 || w.StartupInput != nil || w.ActivationAdmission != nil || w.ActiveRunID != "" || w.DefinitionSpecificationLedger == nil || w.DefinitionSpecificationLedger.Count != 1 {
		t.Fatalf("begin prematurely admitted: %+v", w)
	}
	id := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "logical-1", "")
	permitted, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id})
	if err != nil || permitted.Disposition != workspaceevents.DefinitionSpecificationPermitted || permitted.PermitID == "" || permitted.Consumed != 1 {
		t.Fatalf("reserve: %+v err=%v", permitted, err)
	}
	stored, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || stored.DefinitionSpecificationLedger.Epochs[0].Records[0].PermitID != permitted.PermitID {
		t.Fatalf("permit not persisted: %+v err=%v", stored, err)
	}
	duplicate, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id})
	if err != nil || duplicate.Disposition != workspaceevents.DefinitionSpecificationAlreadyReserved || duplicate.PermitID != "" || duplicate.Consumed != 1 {
		t.Fatalf("duplicate permit escaped: %+v err=%v", duplicate, err)
	}
}
func TestSpecificationLedgerCompletionReplaysTypedResultsAndCriticIsGrounded(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	proposalID := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "proposal-1", "")
	permitted, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: proposalID})
	if err != nil {
		t.Fatal(err)
	}
	proposal := workspacemodel.GoalSpecification{Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: f.candidate.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(f.candidate.GoalInput.Text), Dimensions: []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}, Requirements: []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{f.candidate.GoalInput.CandidateItemRef}, Text: "deliver"}}, Obligations: []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{f.candidate.DoDInputs[0].CandidateItemRef}, Text: "verify"}}, Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Declaration: "pending proof", Status: workspacemodel.GoalVerificationUnverified}}}
	completion := workspacemodel.DefinitionSpecificationRequestCompletion{Proposal: &proposal}
	completed, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: proposalID, PermitID: permitted.PermitID, Completion: completion})
	if err != nil || completed.Completed == nil || completed.Completed.Proposal == nil {
		t.Fatalf("proposal completion: %+v err=%v", completed, err)
	}
	retry, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: proposalID, PermitID: permitted.PermitID, Completion: completion})
	if err != nil || retry.Disposition != workspaceevents.DefinitionSpecificationAlreadyCompleted {
		t.Fatalf("completion retry: %+v err=%v", retry, err)
	}
	modified := completion.Clone()
	modified.Proposal.Requirements[0].Text = "different"
	if _, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: proposalID, PermitID: permitted.PermitID, Completion: modified}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed completion accepted: %v", err)
	}
	duplicate, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: proposalID})
	if err != nil || duplicate.PermitID != "" || duplicate.Completed == nil || duplicate.Completed.Proposal.ContentHash() != proposal.ContentHash() {
		t.Fatalf("typed replay: %+v err=%v", duplicate, err)
	}
	kind := workspacemodel.GoalReviewNegativePath
	criticID := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationCritic, kind, "critic-1", proposal.ContentHash())
	critic, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: criticID})
	if err != nil {
		t.Fatal(err)
	}
	review := workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewFail, SpecificationHash: proposal.ContentHash(), Reason: "missing negative path", RequirementIDs: []string{"requirement"}}
	finished, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: criticID, PermitID: critic.PermitID, Completion: workspacemodel.DefinitionSpecificationRequestCompletion{Review: &review}})
	if err != nil || finished.Completed == nil || finished.Completed.Review == nil {
		t.Fatalf("review completion: %+v err=%v", finished, err)
	}
	query, err := f.owner.queryDefinitionSpecificationRequest(f.ctx, workspaceevents.QueryDefinitionSpecificationRequestCommand{Identity: criticID})
	if err != nil || !reflect.DeepEqual(*query.Completed.Review, review) {
		t.Fatalf("query failed to recover typed review: %+v err=%v", query, err)
	}
	invalid := criticID
	invalid.LogicalRequestID = "untrusted"
	invalid.ProposalHash = "not-owner-hash"
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: invalid}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ungrounded critic admitted: %v", err)
	}
}
func TestSpecificationLedgerEpochCapacityNeverDropsHistory(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	first, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index < int(workspacemodel.DefinitionSpecificationEpochLimit); index++ {
		goalText := fmt.Sprintf("new outcome %d", index)
		if err := f.owner.manager.UpdateGoal(f.ctx, workspaceevents.UpdateGoalCommand{WorkspaceID: f.workspaceID, GoalID: "goal-1", Text: goalText}); err != nil {
			t.Fatal(err)
		}
		w, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		goals, err := f.owner.goals.ListByWorkspaceID(f.ctx, f.workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		goal, _, err := currentGoalDefinition(goals, f.workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		dods, err := f.owner.dods.ListByWorkspaceID(f.ctx, f.workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		dod, _, err := currentDoDDefinition(dods, f.workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		template, err := f.owner.templates.Get(f.ctx, w.TemplateID)
		if err != nil {
			t.Fatal(err)
		}
		c := specificationLedgerCandidate(w, goal, dod, template)
		cmd := workspaceevents.BeginDefinitionSpecificationCommand{Candidate: c, ExpectedWorkspaceRevision: w.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(w)}
		if _, err := f.owner.beginDefinitionSpecification(f.ctx, cmd); err != nil {
			t.Fatalf("begin epoch %d: %v", index, err)
		}
	}
	if err := f.owner.manager.UpdateGoal(f.ctx, workspaceevents.UpdateGoalCommand{WorkspaceID: f.workspaceID, GoalID: "goal-1", Text: "one-more-definition"}); err != nil {
		t.Fatal(err)
	}
	w, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	goals, err := f.owner.goals.ListByWorkspaceID(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	goal, _, err := currentGoalDefinition(goals, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	dods, err := f.owner.dods.ListByWorkspaceID(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	dod, _, err := currentDoDDefinition(dods, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	template, err := f.owner.templates.Get(f.ctx, w.TemplateID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := specificationLedgerCandidate(w, goal, dod, template)
	_, err = f.owner.beginDefinitionSpecification(f.ctx, workspaceevents.BeginDefinitionSpecificationCommand{Candidate: candidate, ExpectedWorkspaceRevision: w.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(w)})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("history overflow should fail closed: %v", err)
	}
	after, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || after.DefinitionSpecificationLedger.Count != workspacemodel.DefinitionSpecificationEpochLimit || after.DefinitionSpecificationLedger.Epochs[0].GoalRevision != first.GoalRevision {
		t.Fatalf("epoch history silently dropped: %+v err=%v", after.DefinitionSpecificationLedger, err)
	}
}

func TestSpecificationLedgerRejectsTamperedPersistedReviewAndBudget(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	proposal := workspacemodel.GoalSpecification{
		Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: f.candidate.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(f.candidate.GoalInput.Text),
		Dimensions:    []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}},
		Requirements:  []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{f.candidate.GoalInput.CandidateItemRef}, Text: "deliver"}},
		Obligations:   []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{f.candidate.DoDInputs[0].CandidateItemRef}, Text: "verify"}},
		Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Status: workspacemodel.GoalVerificationUnverified, Declaration: "not yet verified"}},
	}
	id := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "proposal", "")
	permit, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: id, PermitID: permit.PermitID, Completion: workspacemodel.DefinitionSpecificationRequestCompletion{Proposal: &proposal}}); err != nil {
		t.Fatal(err)
	}
	reviewKind := workspacemodel.GoalReviewDimensionCoverage
	criticID := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationCritic, reviewKind, "critic", proposal.ContentHash())
	criticPermit, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: criticID})
	if err != nil {
		t.Fatal(err)
	}
	review := workspacemodel.GoalSpecificationCompletenessReview{Kind: reviewKind, Status: workspacemodel.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "reviewed", EvidenceRefs: []string{f.candidate.GoalInput.CandidateItemRef}, RequirementIDs: []string{"requirement"}}
	if _, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: criticID, PermitID: criticPermit.PermitID, Completion: workspacemodel.DefinitionSpecificationRequestCompletion{Review: &review}}); err != nil {
		t.Fatal(err)
	}
	w, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	valid := w.DefinitionSpecificationLedger.Clone()
	if err := valid.Validate(f.workspaceID); err != nil {
		t.Fatalf("valid persisted result rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*workspacemodel.DefinitionSpecificationRequestLedger)
	}{
		{"review hash", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].Records[1].Completion.Review.SpecificationHash = "wrong"
		}},
		{"compilation input hash", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].CompilationInputHash = "wrong"
		}},
		{"compilation dimension mutation", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].Dimensions[0].Activated = false
		}},
		{"proposal dimension mutation", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].Records[0].Completion.Proposal.Dimensions[0].Activated = false
		}},
		{"review kind", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].Records[1].Completion.Review.Kind = workspacemodel.GoalReviewRegression
		}},
		{"proposal text binding", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].Records[0].Completion.Proposal.GoalTextHash = "wrong"
		}},
		{"critic before proposal", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].Records[0], l.Epochs[0].Records[1] = l.Epochs[0].Records[1], l.Epochs[0].Records[0]
		}},
		{"excess proposal attempts", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			e := &l.Epochs[0]
			for n := 2; n < 4; n++ {
				other := id
				other.LogicalRequestID = fmt.Sprintf("proposal-%d", n)
				e.Records[n] = workspacemodel.DefinitionSpecificationRequestRecord{Identity: other, PermitID: other.PermitID(), State: workspacemodel.DefinitionSpecificationReserved}
			}
			e.Count = 4
		}},
		{"hidden slot", func(l *workspacemodel.DefinitionSpecificationRequestLedger) {
			l.Epochs[0].Records[2] = l.Epochs[0].Records[0].Clone()
		}},
		{"hidden epoch", func(l *workspacemodel.DefinitionSpecificationRequestLedger) { l.Epochs[1].GoalRevision = "hidden" }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mutated := valid.Clone()
			tt.mutate(&mutated)
			if err := mutated.Validate(f.workspaceID); err == nil {
				t.Fatal("tampered ledger accepted")
			}
		})
	}
	// A damaged persisted result must fail closed on fresh owner read, not
	// merely when issuing the first permit in this process.
	bad := *w
	invalid := valid.Clone()
	invalid.Epochs[0].Records[1].Completion.Review.SpecificationHash = "wrong"
	bad.DefinitionSpecificationLedger = &invalid
	if err := f.owner.workspaces.CompareAndSwap(f.ctx, &bad, w.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("corrupt persisted review escaped validation: %v", err)
	}
}

func TestSpecificationLedgerRevisionSequenceNineToTenAndContentReversion(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	first, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil {
		t.Fatal(err)
	}
	// Parse the formal numeric sequence: lexical comparison of unpadded 10/9
	// or comparison of content hashes cannot establish owner revision ordering.
	previous := workspacemodel.DefinitionSpecificationEpoch{GoalRevision: definitionRevisionForText("goal", 9, "A"), DoDRevision: definitionRevisionForItems("dod", 9, []string{"original"})}
	if !newerSpecificationFormalRevisions(definitionRevisionForText("goal", 10, "B"), previous.DoDRevision, previous) ||
		!newerSpecificationFormalRevisions(definitionRevisionForText("goal", 11, "A"), previous.DoDRevision, previous) {
		t.Fatal("9->10 or A->B->A formal Goal revision failed to advance")
	}
	if newerSpecificationFormalRevisions(previous.GoalRevision, definitionRevisionForItems("dod", 8, []string{"regressed"}), previous) ||
		!newerSpecificationFormalRevisions("goal:10:unsupported", "dod:9:unsupported", previous) {
		// Unpadded owner revisions are accepted by the parser, but a DoD
		// regression is never accepted even if Goal advances.
		t.Fatal("unexpected formal revision regression or parsing")
	}
	for step := 1; step <= 9; step++ {
		text := fmt.Sprintf("outcome-%d", step)
		if step == 9 {
			text = "deliver outcome A"
		}
		if err := f.owner.manager.UpdateGoal(f.ctx, workspaceevents.UpdateGoalCommand{WorkspaceID: f.workspaceID, GoalID: "goal-1", Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	w, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	goals, err := f.owner.goals.ListByWorkspaceID(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	goal, _, err := currentGoalDefinition(goals, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	dods, err := f.owner.dods.ListByWorkspaceID(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	dod, _, err := currentDoDDefinition(dods, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	template, err := f.owner.templates.Get(f.ctx, w.TemplateID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := specificationLedgerCandidate(w, goal, dod, template)
	result, err := f.owner.beginDefinitionSpecification(f.ctx, workspaceevents.BeginDefinitionSpecificationCommand{Candidate: candidate, ExpectedWorkspaceRevision: w.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(w)})
	if err != nil || result.GoalRevision == first.GoalRevision || !newerSpecificationFormalRevisions(result.GoalRevision, result.DoDRevision, workspacemodel.DefinitionSpecificationEpoch{GoalRevision: first.GoalRevision, DoDRevision: first.DoDRevision}) {
		t.Fatalf("owner Goal 9->10 or A->B->A did not open new epoch: %+v err=%v", result, err)
	}
}

func TestSpecificationLedgerBeginInputIdentityRejectsResourceDimensionSwap(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	f.begin.Resources = []workspacemodel.GoalSpecificationResource{{ID: "input", Ref: "workspace/input", SnapshotHash: "snapshot-1", Required: true, Available: true}}
	f.begin.Dimensions[0].ResourceIDs = []string{"input"}
	first, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil {
		t.Fatal(err)
	}
	if first.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(f.candidate.CandidateHash, f.begin.Resources, f.begin.Dimensions) || len(first.Resources) != 1 {
		t.Fatalf("Begin omitted owner input identity: %+v", first)
	}
	first.Resources[0].SnapshotHash = "caller mutation"
	mutated := f.begin
	mutated.Resources = append([]workspacemodel.GoalSpecificationResource(nil), f.begin.Resources...)
	mutated.Resources[0].SnapshotHash = "snapshot-2"
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, mutated); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same formal epoch accepted different resource identity: %v", err)
	}
	mutated = f.begin
	mutated.Dimensions = []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: false, ResourceIDs: []string{"input"}}}
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, mutated); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same formal epoch accepted different dimension identity: %v", err)
	}
	id := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "bound-input", "")
	id.CompilationInputHash = first.CompilationInputHash
	idOther := id
	idOther.CompilationInputHash = workspacemodel.DefinitionSpecificationCompilationInputHash(f.candidate.CandidateHash, mutated.Resources, mutated.Dimensions)
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: idOther}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different provider input obtained a permit: %v", err)
	}
	permitted, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id})
	if err != nil || permitted.PermitID == "" {
		t.Fatalf("owner input permit failed: %+v err=%v", permitted, err)
	}
	proposal := workspacemodel.GoalSpecification{Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: f.candidate.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(f.candidate.GoalInput.Text), Dimensions: f.begin.Dimensions, Requirements: []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{f.candidate.GoalInput.CandidateItemRef}, Text: "deliver"}}, Obligations: []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{f.candidate.DoDInputs[0].CandidateItemRef}, Text: "verify"}}, Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Status: workspacemodel.GoalVerificationUnverified, Declaration: "pending"}}}
	proposal.Resources = append([]workspacemodel.GoalSpecificationResource(nil), f.begin.Resources...)
	proposal.Resources[0].SnapshotHash = "snapshot-2"
	if _, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: id, PermitID: permitted.PermitID, Completion: workspacemodel.DefinitionSpecificationRequestCompletion{Proposal: &proposal}}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("proposal changed original resource but completed: %v", err)
	}
	proposal.Resources[0].SnapshotHash = "snapshot-1"
	if _, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: id, PermitID: permitted.PermitID, Completion: workspacemodel.DefinitionSpecificationRequestCompletion{Proposal: &proposal}}); err != nil {
		t.Fatalf("exact owner input could not complete: %v", err)
	}
	replay, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil || replay.CompilationInputHash != first.CompilationInputHash || replay.Resources[0].SnapshotHash != "snapshot-1" {
		t.Fatalf("restart/replay lost immutable resource identity: %+v err=%v", replay, err)
	}
}

func TestSpecificationLedgerBeginReplayAfterReserve(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	first, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil {
		t.Fatal(err)
	}
	id := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "lost-begin-response", "")
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id}); err != nil {
		t.Fatal(err)
	}
	replay, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("same begin must replay original snapshot despite Workspace ledger revision: first=%+v replay=%+v err=%v", first, replay, err)
	}
	stored, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || stored.DefinitionSpecificationLedger.Count != 1 || stored.DefinitionSpecificationLedger.Epochs[0].Count != 1 {
		t.Fatalf("begin replay reset ledger: %+v err=%v", stored, err)
	}
}

func TestSpecificationLedgerCompiledSixCriticAdmission(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	f.candidate.RuntimeProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "generic", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Description: "declaration"}}}}
	f.candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(f.candidate.RuntimeProfileContracts)
	f.candidate = f.candidate.WithHash()
	f.begin.Candidate = f.candidate
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	proposal := workspacemodel.GoalSpecification{
		Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: f.candidate.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(f.candidate.GoalInput.Text),
		Dimensions:    []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}},
		Requirements:  []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{f.candidate.GoalInput.CandidateItemRef}, Text: "deliver"}},
		Obligations:   []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{f.candidate.DoDInputs[0].CandidateItemRef}, Text: "verify"}},
		Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Status: workspacemodel.GoalVerificationDeclared, Declaration: "exact proof", ProfileID: "generic", EvidenceRequirementID: "dod-001", Scope: "dodItem"}},
	}
	proposalID := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "proposal", "")
	proposalPermit, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: proposalID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: proposalID, PermitID: proposalPermit.PermitID, Completion: workspacemodel.DefinitionSpecificationRequestCompletion{Proposal: &proposal}}); err != nil {
		t.Fatal(err)
	}
	kinds := []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility}
	compiled := f.candidate.Clone()
	compiled.Specification = &proposal
	ref := workspacemodel.DefinitionSpecificationCompilationRef{InputCandidateHash: f.candidate.CandidateHash, CompilationInputHash: workspacemodel.DefinitionSpecificationCompilationInputHash(f.candidate.CandidateHash, f.begin.Resources, f.begin.Dimensions), ProposalHash: proposal.ContentHash(), ProposalPermitID: proposalPermit.PermitID}
	for index, kind := range kinds {
		id := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationCritic, kind, "critic-"+kind, ref.ProposalHash)
		permitted, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id})
		if err != nil {
			t.Fatal(err)
		}
		review := workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewPass, SpecificationHash: ref.ProposalHash, Reason: "externally reviewed", EvidenceRefs: []string{f.candidate.GoalInput.CandidateItemRef}, RequirementIDs: []string{"requirement"}}
		if _, err := f.owner.completeDefinitionSpecificationRequest(f.ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: id, PermitID: permitted.PermitID, Completion: workspacemodel.DefinitionSpecificationRequestCompletion{Review: &review}}); err != nil {
			t.Fatal(err)
		}
		ref.CriticPermitIDs[index] = permitted.PermitID
		compiled.Specification.Reviews = append(compiled.Specification.Reviews, review)
	}
	compiled.CompilationRef = &ref
	compiled = compiled.WithHash()
	compiledResult, err := definitionadmission.Compile(compiled)
	if err != nil || compiledResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("compiled proposal not admissible: %+v err=%v", compiledResult, err)
	}
	w, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: compiled, CurrentTemplate: true, ExpectedWorkspaceRevision: w.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(w)}
	forged := compiled.Clone()
	forged.Specification.Reviews[0].Reason = "forged pass"
	forged = forged.WithHash()
	cmd.Candidate = forged
	if _, err := f.owner.submitDefinitionAdmissionIntent(f.ctx, cmd); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("critic review substitution accepted: %v", err)
	}
	cmd.Candidate = compiled
	cmd.Candidate.CompilationRef = &ref
	cmd.Candidate.CompilationRef.CriticPermitIDs[0] = ref.CriticPermitIDs[1]
	cmd.Candidate = cmd.Candidate.WithHash()
	if _, err := f.owner.submitDefinitionAdmissionIntent(f.ctx, cmd); err == nil {
		t.Fatal("swapped critic permit accepted")
	}
	cmd.Candidate = compiled
	admitted, err := f.owner.submitDefinitionAdmissionIntent(f.ctx, cmd)
	if err != nil || admitted.Disposition != workspaceevents.DefinitionAdmissionIntentEligible || admitted.Candidate.CandidateHash != compiled.CandidateHash || admitted.StartupInput == nil {
		t.Fatalf("reviewed candidate not admitted: %+v err=%v", admitted, err)
	}
	stored, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.ActivationAdmission == nil || stored.StartupInput == nil || stored.DefinitionSpecificationLedger.Epochs[0].Count != 7 {
		t.Fatalf("admission lost durable provenance: %+v err=%v", stored, err)
	}
}

func TestSpecificationLedgerBudgetExhaustionDoesNotGrantPermit(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	id := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "proposal-1", "")
	for attempt := 0; attempt < workspacemodel.DefinitionSpecificationAttemptLimit; attempt++ {
		id.LogicalRequestID = fmt.Sprintf("proposal-%d", attempt)
		if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id}); err != nil {
			t.Fatal(err)
		}
	}
	id.LogicalRequestID = "proposal-over-limit"
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("proposal attempt budget granted permit: %v", err)
	}
	stored, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || stored.DefinitionSpecificationLedger.Epochs[0].Count != workspacemodel.DefinitionSpecificationAttemptLimit {
		t.Fatalf("failed reserve consumed/refunded budget: %+v err=%v", stored.DefinitionSpecificationLedger, err)
	}
}

func TestSpecificationLedgerAdmissionRejectsLegacyBypassAndForgedReview(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	legacy := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: f.candidate, CurrentTemplate: true}
	current, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	legacy.ExpectedWorkspaceRevision = current.Revision
	legacy.ExpectedConfigurationHash = workspacemodel.WorkspaceConfigurationHash(current)
	if _, err := f.owner.submitDefinitionAdmissionIntent(f.ctx, legacy); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("legacy path bypassed compiler: %v", err)
	}
	forged := f.candidate.Clone()
	forged.Specification = &workspacemodel.GoalSpecification{Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: forged.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(forged.GoalInput.Text), Dimensions: []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}, Requirements: []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{forged.GoalInput.CandidateItemRef}, Text: "deliver"}}, Obligations: []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{forged.DoDInputs[0].CandidateItemRef}, Text: "verify"}}, Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Status: workspacemodel.GoalVerificationUnverified, Declaration: "unverified"}}}
	hash := forged.Specification.ContentHash()
	kinds := []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility}
	ref := workspacemodel.DefinitionSpecificationCompilationRef{InputCandidateHash: f.candidate.CandidateHash, CompilationInputHash: workspacemodel.DefinitionSpecificationCompilationInputHash(f.candidate.CandidateHash, f.begin.Resources, f.begin.Dimensions), ProposalPermitID: "fake-proposal", ProposalHash: hash}
	for i, kind := range kinds {
		forged.Specification.Reviews = append(forged.Specification.Reviews, workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewPass, SpecificationHash: hash, Reason: "claimed review", EvidenceRefs: []string{forged.GoalInput.CandidateItemRef}, RequirementIDs: []string{"requirement"}})
		ref.CriticPermitIDs[i] = fmt.Sprintf("fake-critic-%d", i)
	}
	forged.CompilationRef = &ref
	forged = forged.WithHash()
	legacy.Candidate = forged
	if _, err := f.owner.submitDefinitionAdmissionIntent(f.ctx, legacy); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged reviews admitted: %v", err)
	}
	stored, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || len(stored.DefinitionAdmissionCandidates) != 0 {
		t.Fatalf("forged candidate persisted: %+v err=%v", stored, err)
	}
}

func TestSpecificationLedgerFormalGoalRevisionEpochAndBudgetDoNotReset(t *testing.T) {
	f, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	first, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin)
	if err != nil {
		t.Fatal(err)
	}
	firstID := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "epoch-1", "")
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: firstID}); err != nil {
		t.Fatal(err)
	}
	// Neither another candidate hash nor a new logical request resets the same
	// formal-definition epoch's budget.
	other := firstID
	other.LogicalRequestID = "same-epoch-extra"
	other.CandidateHash = "other-candidate-hash"
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: other}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("candidate swap escaped epoch fence: %v", err)
	}
	if err := f.owner.manager.UpdateGoal(f.ctx, workspaceevents.UpdateGoalCommand{WorkspaceID: f.workspaceID, GoalID: "goal-1", Text: "deliver outcome B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.queryDefinitionSpecificationRequest(f.ctx, workspaceevents.QueryDefinitionSpecificationRequestCommand{Identity: firstID}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("old permit survived formal change: %v", err)
	}
	if err := f.owner.manager.UpdateGoal(f.ctx, workspaceevents.UpdateGoalCommand{WorkspaceID: f.workspaceID, GoalID: "goal-1", Text: "deliver outcome A"}); err != nil {
		t.Fatal(err)
	}
	w, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	goals, err := f.owner.goals.ListByWorkspaceID(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	goal, _, err := currentGoalDefinition(goals, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	dods, err := f.owner.dods.ListByWorkspaceID(f.ctx, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	dod, _, err := currentDoDDefinition(dods, f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	template, err := f.owner.templates.Get(f.ctx, w.TemplateID)
	if err != nil {
		t.Fatal(err)
	}
	fresh := specificationLedgerCandidate(w, goal, dod, template)
	fresh.CandidateID = "workspace-definition"
	fresh = fresh.WithHash()
	begin := workspaceevents.BeginDefinitionSpecificationCommand{Candidate: fresh, ExpectedWorkspaceRevision: w.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(w)}
	second, err := f.owner.beginDefinitionSpecification(f.ctx, begin)
	if err != nil || second.GoalRevision == first.GoalRevision || second.CandidateHash == first.CandidateHash {
		t.Fatalf("Goal A-B-A did not advance formal revision: %+v err=%v", second, err)
	}
	stored, err := f.owner.workspaces.Get(f.ctx, f.workspaceID)
	if err != nil || stored.DefinitionSpecificationLedger.Count != 2 || stored.DefinitionSpecificationLedger.Epochs[0].Count != 1 || stored.DefinitionSpecificationLedger.Epochs[1].Count != 0 {
		t.Fatalf("epoch budget reset or lost: %+v err=%v", stored.DefinitionSpecificationLedger, err)
	}
}

func TestSpecificationLedgerFileReopenConsumesReservedPermitWithoutRetry(t *testing.T) {
	root := t.TempDir()
	f, close := openSpecificationLedgerFixture(t, root)
	if _, err := f.owner.beginDefinitionSpecification(f.ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	id := specLedgerIdentity(f.candidate, workspacemodel.DefinitionSpecificationProposal, "", "lost-response", "")
	if _, err := f.owner.reserveDefinitionSpecificationRequest(f.ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id}); err != nil {
		t.Fatal(err)
	}
	close()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	duplicate, err := owners.workspace.reserveDefinitionSpecificationRequest(context.Background(), workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: id})
	if err != nil || duplicate.Disposition != workspaceevents.DefinitionSpecificationAlreadyReserved || duplicate.PermitID != "" || duplicate.Consumed != 1 {
		t.Fatalf("reserved replay issued physical permit: %+v err=%v", duplicate, err)
	}
}
