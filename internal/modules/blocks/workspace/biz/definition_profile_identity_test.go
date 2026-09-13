package workspace

import (
	"context"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDefinitionProfileIdentityRejectionSurvivesOwnerRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	candidate := testAdmissionCandidate()
	if err := owners.repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: candidate.WorkspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	// The later optional declaration must not erase the required one. There is
	// no Run/provider/capability owner in this fixture, so rejection is pre-seal.
	candidate.RuntimeProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "profile", AcceptanceDimensions: []runtimecontract.RuntimeAcceptanceDimension{
		{ID: "behavior", Required: true}, {ID: "behavior", Required: false},
	}, EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "behavior", Scope: "acceptanceDimension", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}}}}
	candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
	candidate = candidate.WithHash()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate}
	first, err := owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if first.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired || first.AdmissionResult.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid || first.StartupInput != nil || first.ActivationAdmission != nil {
		t.Fatalf("invalid profile promoted: %+v", first)
	}
	before, err := owners.repositories.Workspaces.Get(ctx, candidate.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	duplicate, err := owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || duplicate.AdmissionResult.AdmissionResultHash != first.AdmissionResult.AdmissionResultHash || duplicate.StartupInput != nil || duplicate.ActivationAdmission != nil {
		t.Fatalf("restart changed rejection: %+v", duplicate)
	}
	after, err := owners.repositories.Workspaces.Get(ctx, candidate.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("duplicate changed persisted Workspace after fresh-owner reopen")
	}
	if len(after.DefinitionAdmissionCandidates) != 1 || len(after.DefinitionAdmissionCandidates[0].RuntimeProfileContracts[0].AcceptanceDimensions) != 2 {
		t.Fatal("persisted candidate lost conflicting declaration")
	}
	tampered := intent
	tampered.Candidate = candidate.Clone()
	tampered.Candidate.RuntimeProfileContracts[0].AcceptanceDimensions = tampered.Candidate.RuntimeProfileContracts[0].AcceptanceDimensions[:1]
	if _, err := owners.workspace.submitDefinitionAdmissionIntent(ctx, tampered); err == nil {
		t.Fatal("same candidate hash admitted modified obligations")
	}
}
