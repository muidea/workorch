package workspace

import (
	"bytes"
	"context"
	"reflect"
	"sync"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
)

func correctiveOwnerFixture(t *testing.T, owners *fileDefinitionAdmissionOwners) (*workspace.Workspace, workspaceevents.ReserveAuthoringSpecificationCommand) {
	t.Helper()
	ctx := context.Background()
	confirmed := seedReviewedAuthoringConfirmationWithProof(t, owners, "ws-feedback-owner", true)
	w, err := owners.repositories.Workspaces.Get(ctx, confirmed.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	r := w.DefinitionAuthoring.Clone()
	// Build a synthetic, settled six-review fixture with one concrete issue.
	first := &r.SpecificationRequests[1]
	review := *first.Review
	review.Status = workspace.GoalReviewUnknown
	review.Issues = []workspace.GoalSpecificationReviewIssue{{Class: workspace.AuthoringIssueDesign, Detail: "Independent expected output missing", RequirementIDs: review.RequirementIDs, EvidenceRefs: review.EvidenceRefs}}
	first.Review, first.ReviewAudit = nil, nil
	r, err = r.CompleteReviewDocument(first.PermitID, review, *reviewTestAudit(t, &review))
	if err != nil {
		t.Fatal(err)
	}
	w.DefinitionAuthoring = &r
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, w, w.Revision); err != nil {
		t.Fatal(err)
	}
	w, err = owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := r.SpecificationFeedback(confirmed.DerivationPermitID, confirmed.ProposalHash)
	if err != nil {
		t.Fatal(err)
	}
	return w, workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: w.ID, InputHash: r.InputHash, DerivationPermitID: confirmed.DerivationPermitID, RequestID: "correct-once", PreviousProposalHash: confirmed.ProposalHash, FeedbackHash: feedback.ContentHash(), ProviderBindingHash: "provider", WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "proposal")}
}

func TestAuthoringCorrectiveReservationCASCompetitionAndOriginalFailureReopen(t *testing.T) {
	for _, competingID := range []bool{false, true} {
		t.Run(map[bool]string{false: "duplicate", true: "competing-id"}[competingID], func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			before, command := correctiveOwnerFixture(t, owners)
			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make(chan definitionSpecificationTransition, 2)
			errors := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					cmd := command
					if competingID && i == 1 {
						cmd.RequestID = "competing-correction"
					}
					result, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: cmd.WorkspaceID, AuthoringSpecReserve: &cmd})
					results <- result
					errors <- err
				}(i)
			}
			close(start)
			wg.Wait()
			close(results)
			close(errors)
			failures, fresh := 0, 0
			permit := ""
			for err := range errors {
				if err != nil {
					failures++
				}
			}
			for result := range results {
				if result.AuthoringSpecification != nil && result.AuthoringSpecification.FreshPermitID != "" {
					fresh++
					permit = result.AuthoringSpecification.FreshPermitID
				}
			}
			if fresh != 1 || failures != map[bool]int{false: 0, true: 1}[competingID] {
				t.Fatalf("fresh=%d failures=%d", fresh, failures)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			mapping := current.DefinitionAuthoring.SpecificationRequests[0].Mapping.Clone()
			// Valid wire, actual design change, but missing issue response is a known
			// semantic failure, not an unresolved reservation or another model permit.
			mapping.DesignDecisions = []workspace.GoalSpecificationDesignDecision{{CandidateItemRef: mapping.Obligations[0].CandidateItemRef, Decision: "Fixed rule", VerificationBasis: "Independent assertion"}}
			audit, err := testkit.Audit(mapping)
			if err != nil {
				t.Fatal(err)
			}
			complete := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: before.ID, PermitID: permit, Mapping: &mapping, SuccessAudit: &audit}
			settled, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &complete})
			if err != nil || settled.AuthoringSpecification.Request.FailureCode != "invalid_proposal" {
				t.Fatalf("known failure not settled: %v", err)
			}
			failed, err := owners.repositories.Workspaces.Get(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			diagnostic := settled.AuthoringSpecification.Request.Diagnostic
			// Typed result is metadata only; retrieve restricted original bytes instead.
			original, err := owners.workspace.queryAuthoringDiagnostic(ctx, workspaceevents.QueryAuthoringDiagnosticCommand{WorkspaceID: before.ID, PermitID: permit})
			if err != nil {
				t.Fatal(err)
			}
			if diagnostic == nil || !bytes.Equal(original.Diagnostic.ProviderDocument.Content, audit.Document.Content) {
				t.Fatal("original failure bytes lost")
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			if _, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: before.ID}); err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &complete}); err != nil {
				t.Fatal(err)
			}
			reopened, err := owners.repositories.Workspaces.Get(ctx, before.ID)
			if err != nil || !reflect.DeepEqual(failed, reopened) {
				t.Fatal("failure replay changed ledger", err)
			}
			command.RequestID = "reroll"
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecReserve: &command}); err == nil {
				t.Fatal("failed revision granted another call")
			}
		})
	}
}
