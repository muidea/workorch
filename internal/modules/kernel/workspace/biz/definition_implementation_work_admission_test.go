package workspace

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDefinitionAdmissionImplementationWorkUnavailableSurvivesReload(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	policy := runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations:     []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileWrite}},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		TargetSelectorRef:     "work-scope", TargetSelectorOwnerRef: "profile/example", TargetSelectorSubjectRef: "implementation",
		TargetSelectorCandidateRefs: []string{"resource-a"}, MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1,
		RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
	candidate := testAdmissionCandidate()
	candidate.RuntimeProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "example", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy,
	}}}}
	if err := runtimecontract.ValidateRuntimeProfileContracts(candidate.RuntimeProfileContracts); err != nil {
		t.Fatal(err)
	}
	candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
	candidate = candidate.WithHash()
	// No Run, provider, capability or Decision dependency is present. Only
	// Workspace's private admission event can persist this bounded disposition.
	biz := &Biz{workspaces: repo}
	result, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate})
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired || result.StartupInput != nil || result.ActivationAdmission != nil ||
		!strings.Contains(result.AdmissionResult.CanonicalViolation, "implementation_work_protocol_unavailable") {
		t.Fatalf("unsupported work escaped admission: %+v", result)
	}
	stored, err := repo.Get(ctx, candidate.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var restored workspacemodel.Workspace
	if err := json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	reloadedRepo := memoryrepo.NewWorkspaceRepository()
	if err := reloadedRepo.Create(ctx, &restored); err != nil {
		t.Fatal(err)
	}
	fresh := &Biz{workspaces: reloadedRepo}
	duplicate, err := fresh.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate})
	if err != nil || duplicate.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || duplicate.AdmissionResult.AdmissionResultHash != result.AdmissionResult.AdmissionResultHash {
		t.Fatalf("reload changed admission: %+v, %v", duplicate, err)
	}
	stored, err = reloadedRepo.Get(ctx, candidate.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatal("duplicate admission rewrote workspace or granted activation")
	}
}
