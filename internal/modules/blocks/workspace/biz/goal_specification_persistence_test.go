package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestDefinitionAdmissionFileBackedSpecificationRetainsReviewAndRejectsTamper(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	candidate := testAdmissionCandidate()
	if err := owners.repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: candidate.WorkspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	candidate.Specification = &workspacemodel.GoalSpecification{
		Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: candidate.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(candidate.GoalInput.Text),
		Dimensions:    []workspacemodel.GoalSpecificationDimension{{ID: "goal-dimension", Required: true, Activated: true}},
		Requirements:  []workspacemodel.GoalSpecificationRequirement{{ID: "goal-requirement", DimensionID: "goal-dimension", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{"goal"}, Text: "deliver goal"}},
		Obligations:   []workspacemodel.GoalSpecificationObligation{{ID: "goal-obligation", RequirementID: "goal-requirement", DoDItemRefs: []string{"dod"}, Text: "fulfill DoD"}},
		Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "goal-verification", ObligationID: "goal-obligation", Status: workspacemodel.GoalVerificationUnverified, Declaration: "unverified proposal"}},
	}
	hash := candidate.Specification.ContentHash()
	for _, kind := range []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility} {
		candidate.Specification.Reviews = append(candidate.Specification.Reviews, workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewUnknown, SpecificationHash: hash, Reason: "awaiting review", EvidenceRefs: []string{"goal"}, RequirementIDs: []string{"goal-requirement"}})
	}
	candidate = candidate.WithHash()
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}
	first, err := owners.workspace.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate})
	if err != nil || first.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired {
		t.Fatalf("persist specification: %+v err=%v", first, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	replayed, err := owners.workspace.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate})
	if err != nil || replayed.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || replayed.AdmissionResult.AdmissionResultHash != first.AdmissionResult.AdmissionResultHash {
		t.Fatalf("recover specification: %+v err=%v", replayed, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, candidate.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.DefinitionAdmissionCandidates[0].Specification == nil || stored.DefinitionAdmissionCandidates[0].Specification.AdmissionHash() != candidate.Specification.AdmissionHash() {
		t.Fatalf("persisted specification drift: %+v err=%v", stored, err)
	}
	changed := candidate.Clone()
	changed.Specification.Reviews[0].Reason = "forged review"
	if err := stored.DefinitionAdmissionCandidates[0].Validate(); err != nil || changed.Specification.Reviews[0].Reason == stored.DefinitionAdmissionCandidates[0].Specification.Reviews[0].Reason {
		t.Fatalf("persisted candidate alias: %v", err)
	}
	if _, err := owners.workspace.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: changed}); err == nil {
		t.Fatal("accepted mutated review under original candidate hash")
	}
}

func TestDefinitionAdmissionOwnerRejectsSpecificationDowngradeAcrossEventHub(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	candidate := testAdmissionCandidate()
	candidate.Specification = &workspacemodel.GoalSpecification{
		Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: "goal", GoalTextHash: workspacemodel.StableDefinitionTextHash(candidate.GoalInput.Text),
		Dimensions:    []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}},
		Requirements:  []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{"goal"}, Text: "goal"}},
		Obligations:   []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: "done"}},
		Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Declaration: "pending", Status: workspacemodel.GoalVerificationUnverified}},
	}
	hash := candidate.Specification.ContentHash()
	for _, kind := range []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility} {
		candidate.Specification.Reviews = append(candidate.Specification.Reviews, workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewUnknown, SpecificationHash: hash})
	}
	candidate = candidate.WithHash()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: candidate.WorkspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	owner, _ := newDefinitionAdmissionProvenanceTestBiz(t, repo)
	first := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate}
	if result, err := owner.submitDefinitionAdmissionIntent(ctx, first); err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired {
		t.Fatalf("seed specification candidate: %+v err=%v", result, err)
	}
	archived, err := repo.Get(ctx, candidate.WorkspaceID)
	if err != nil || len(archived.DefinitionAdmissionCandidates) != 1 || archived.DefinitionAdmissionCandidates[0].Specification == nil {
		t.Fatalf("archived specification unreadable: %+v err=%v", archived, err)
	}
	legacy := candidate.Clone()
	legacy.CandidateRevision++
	legacy.Specification = nil
	legacy = legacy.WithHash()
	command := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: legacy, ExpectedCandidateRevision: 1}
	if _, err := owner.submitDefinitionAdmissionIntent(ctx, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("direct Workspace submission downgraded specification: %v", err)
	}
	result := owner.EventHub().Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAdmissionIntent, definitionAdmissionCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil || result.Error() == nil {
		t.Fatalf("EventHub accepted downgraded specification: %+v", result)
	}
	// The real Hub wraps a returned owner conflict as an EventHub error. A
	// direct handler invocation also proves the reducer preserved the cause.
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAdmissionIntent, definitionAdmissionCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	handlerResult := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	owner.handleDefinitionAdmissionCommand(event, handlerResult)
	if !errors.Is(handlerResult.Error(), common.ErrConflict) {
		t.Fatalf("owner handler returned %v, want conflict", handlerResult.Error())
	}
	stored, err := repo.Get(ctx, candidate.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.DefinitionAdmissionCandidates[0].CandidateHash != candidate.CandidateHash {
		t.Fatalf("downgrade persisted despite rejection: %+v err=%v", stored, err)
	}
}

func TestSpecificationAdmissionStartupHashSealsFullReviewAndRejectsDrift(t *testing.T) {
	candidate := testAdmissionCandidate()
	candidate.Specification = &workspacemodel.GoalSpecification{
		Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: candidate.GoalInput.CandidateItemRef,
		GoalTextHash:  workspacemodel.StableDefinitionTextHash(candidate.GoalInput.Text),
		Dimensions:    []workspacemodel.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}},
		Requirements:  []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, Text: "goal", SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{"goal"}}},
		Obligations:   []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", Text: "done", DoDItemRefs: []string{"dod"}}},
		Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Status: workspacemodel.GoalVerificationUnverified, Declaration: "proposal"}},
	}
	hash := candidate.Specification.ContentHash()
	for _, kind := range []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility} {
		candidate.Specification.Reviews = append(candidate.Specification.Reviews, workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewUnknown, SpecificationHash: hash})
	}
	candidate = candidate.WithHash()
	result := workspacemodel.DefinitionAdmissionResult{WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash, Disposition: workspacemodel.DefinitionAdmissionAdmittedAllOf, AdmissionCompilerVersion: candidate.AdmissionCompilerVersion, CanonicalAdmittedInputHash: "hash"}.WithHash()
	startup := startupInputFromCandidate(candidate, result)
	if startup.SpecificationHash != candidate.Specification.AdmissionHash() || startup.ValidateCandidate(candidate) != nil {
		t.Fatalf("specification startup binding invalid: %+v", startup)
	}
	legacy := candidate.Clone()
	legacy.Specification = nil
	legacy = legacy.WithHash()
	legacyStartup := startupInputFromCandidate(legacy, result)
	if legacyStartup.SpecificationHash != "" || legacyStartup.StartupInputHash == startup.StartupInputHash {
		t.Fatal("specification failed to change startup input hash")
	}
	altered := candidate.Clone()
	altered.Specification.Reviews[0].Reason = "changed review"
	altered = altered.WithHash()
	if err := startup.ValidateCandidate(altered); err == nil {
		t.Fatal("startup accepted different review under original seal")
	}
	missing := startup
	missing.SpecificationHash = ""
	missing = missing.WithHash()
	if err := missing.ValidateCandidate(candidate); err == nil {
		t.Fatal("startup accepted missing specification binding")
	}
}
