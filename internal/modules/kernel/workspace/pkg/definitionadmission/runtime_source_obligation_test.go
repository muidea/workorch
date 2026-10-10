package definitionadmission

import (
	"reflect"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func sourceObligationAdmissionFixture(t *testing.T) (workspacemodel.DefinitionAdmissionCandidate, workspacemodel.WorkspaceStartupInputContract, workspacemodel.WorkspaceActivationAdmission) {
	t.Helper()
	candidate := candidateWithSpecification()
	second := candidate.Specification.Obligations[0]
	second.ID, second.Text = "obligation-B", "B 必须独立验证"
	candidate.Specification.Obligations[0].Text = "A 必须独立验证"
	candidate.Specification.Obligations = append(candidate.Specification.Obligations, second)
	verification := candidate.Specification.Verifications[0]
	verification.ID, verification.ObligationID = "verification-B", second.ID
	candidate.Specification.Verifications = append(candidate.Specification.Verifications, verification)
	for n := range candidate.Specification.Reviews {
		candidate.Specification.Reviews[n].SpecificationHash = candidate.Specification.ContentHash()
	}
	candidate = candidate.WithHash()
	return sourceObligationSealFixture(candidate)
}

func sourceObligationSealFixture(candidate workspacemodel.DefinitionAdmissionCandidate) (workspacemodel.DefinitionAdmissionCandidate, workspacemodel.WorkspaceStartupInputContract, workspacemodel.WorkspaceActivationAdmission) {
	startup := workspacemodel.WorkspaceStartupInputContract{
		WorkspaceID: candidate.WorkspaceID, DefinitionRevision: candidate.GoalInput.SourceIdentity + "|" + candidate.DoDInputs[0].SourceIdentity,
		GoalDefinitionRef: "goal-materialized", GoalTextHash: workspacemodel.StableDefinitionTextHash(candidate.GoalInput.Text),
		DoDDefinitionRefs: []string{"dod-materialized"}, DoDItemSetHash: workspacemodel.StableDefinitionStringSetHash([]string{candidate.DoDInputs[0].Text}),
		WorkspaceTemplateRef: candidate.WorkspaceTemplateSnapshot.TemplateRef, TemplateRevision: candidate.WorkspaceTemplateSnapshot.Revision,
		TemplateSnapshotHash: candidate.WorkspaceTemplateSnapshot.SnapshotHash, RuntimeProfileContractsHash: candidate.RuntimeProfileContractsHash,
		ResolvedProjectWorkspaceDirectory: candidate.ResolvedProjectWorkspaceDirectory,
		CandidateItemToItemRefMappings: []workspacemodel.CandidateItemToItemRefMapping{
			{CandidateItemRef: candidate.GoalInput.CandidateItemRef, ItemRef: "goal-materialized"},
			{CandidateItemRef: candidate.DoDInputs[0].CandidateItemRef, ItemRef: "dod-materialized"},
		},
	}
	if candidate.Specification != nil {
		startup.SpecificationHash = candidate.Specification.AdmissionHash()
	}
	startup = startup.WithHash()
	admission := workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash,
		State: workspacemodel.WorkspaceActivationEligible,
	}
	return candidate, startup, admission
}

func TestCompileRuntimeSourceObligationsRetainsSharedDoDAndDetachedGraph(t *testing.T) {
	candidate, startup, admission := sourceObligationAdmissionFixture(t)
	before := candidate.Clone()
	inventory, err := CompileRuntimeSourceObligationInventory(candidate, startup, admission)
	if err != nil {
		t.Fatal(err)
	}
	if inventory == nil || inventory.SchemaVersion != runtimecontract.RuntimeSourceObligationInventorySchemaCurrent || len(inventory.Obligations) != 2 || len(inventory.Verifications) != 2 || len(inventory.Sources) != 2 ||
		inventory.CandidateHash != candidate.CandidateHash || inventory.SpecificationHash != startup.SpecificationHash ||
		inventory.DefinitionRevision != startup.DefinitionRevision || inventory.StartupInputHash != startup.StartupInputHash {
		t.Fatalf("source graph lost identity: %+v", inventory)
	}
	for _, obligation := range inventory.Obligations {
		if !reflect.DeepEqual(obligation.DoDItemRefs, []string{candidate.DoDInputs[0].CandidateItemRef}) {
			t.Fatalf("shared DoD was split or omitted: %+v", obligation)
		}
	}
	for _, source := range inventory.Sources {
		if source.SourceScope == "dodItem" && (source.SourceID != "dod-001" || source.ItemRef != "dod-materialized" || source.Text != candidate.DoDInputs[0].Text) {
			t.Fatalf("formal source mapping drift: %+v", source)
		}
	}
	inventory.Obligations[0].DoDItemRefs[0] = "changed"
	inventory.Requirements[0].ResourceIDs[0] = "changed"
	if !reflect.DeepEqual(candidate, before) {
		t.Fatal("compiler returned aliases into candidate")
	}
}

func TestCompileRuntimeSourceObligationsRejectsSealAndApplicabilityDrift(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*workspacemodel.DefinitionAdmissionCandidate, *workspacemodel.WorkspaceStartupInputContract, *workspacemodel.WorkspaceActivationAdmission)
	}{
		{"candidate hash", func(c *workspacemodel.DefinitionAdmissionCandidate, _ *workspacemodel.WorkspaceStartupInputContract, _ *workspacemodel.WorkspaceActivationAdmission) {
			c.CandidateHash = "wrong"
		}},
		{"specification hash", func(_ *workspacemodel.DefinitionAdmissionCandidate, s *workspacemodel.WorkspaceStartupInputContract, a *workspacemodel.WorkspaceActivationAdmission) {
			s.SpecificationHash = "wrong"
			*s = s.WithHash()
			a.StartupInputHash = s.StartupInputHash
		}},
		{"definition revision", func(_ *workspacemodel.DefinitionAdmissionCandidate, s *workspacemodel.WorkspaceStartupInputContract, a *workspacemodel.WorkspaceActivationAdmission) {
			s.DefinitionRevision = "wrong"
			*s = s.WithHash()
			a.DefinitionRevision = s.DefinitionRevision
			a.StartupInputHash = s.StartupInputHash
		}},
		{"startup hash", func(_ *workspacemodel.DefinitionAdmissionCandidate, s *workspacemodel.WorkspaceStartupInputContract, a *workspacemodel.WorkspaceActivationAdmission) {
			s.StartupInputHash = "wrong"
			a.StartupInputHash = s.StartupInputHash
		}},
		{"pending", func(_ *workspacemodel.DefinitionAdmissionCandidate, _ *workspacemodel.WorkspaceStartupInputContract, a *workspacemodel.WorkspaceActivationAdmission) {
			a.State = workspacemodel.WorkspaceActivationPendingClarificationResolution
			a.BlockingTicketID = "ticket"
		}},
		{"wrong profile scope", func(c *workspacemodel.DefinitionAdmissionCandidate, s *workspacemodel.WorkspaceStartupInputContract, a *workspacemodel.WorkspaceActivationAdmission) {
			c.Specification.Verifications[0].Scope = "deliverable"
			for n := range c.Specification.Reviews {
				c.Specification.Reviews[n].SpecificationHash = c.Specification.ContentHash()
			}
			*c = c.WithHash()
			*c, *s, *a = sourceObligationSealFixture(*c)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate, startup, admission := sourceObligationAdmissionFixture(t)
			tc.mutate(&candidate, &startup, &admission)
			if value, err := CompileRuntimeSourceObligationInventory(candidate, startup, admission); err == nil || value != nil {
				t.Fatalf("accepted source drift: value=%+v err=%v", value, err)
			}
		})
	}
}

func TestCompileRuntimeSourceObligationsPreservesCriterionBytes(t *testing.T) {
	candidate, _, _ := sourceObligationAdmissionFixture(t)
	candidate.Specification.Obligations[0].Text = " \tA 判据\n "
	candidate.Specification.Requirements[0].Text = " 要求原文\n "
	candidate.Specification.Verifications[0].Declaration = " 验证声明\n "
	// candidate 的显式规格 hash 规范化不覆盖投影中的原始正文。
	for n := range candidate.Specification.Reviews {
		candidate.Specification.Reviews[n].SpecificationHash = candidate.Specification.ContentHash()
	}
	candidate.CandidateHash = candidate.WithHash().CandidateHash
	candidate, startup, admission := sourceObligationSealFixture(candidate)
	inventory, err := CompileRuntimeSourceObligationInventory(candidate, startup, admission)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Obligations[0].Text != candidate.Specification.Obligations[0].Text ||
		inventory.Requirements[0].Text != candidate.Specification.Requirements[0].Text ||
		inventory.Verifications[0].Declaration != candidate.Specification.Verifications[0].Declaration {
		t.Fatalf("criterion bytes changed: %+v", inventory)
	}
	changed := inventory.Clone()
	changed.Obligations[0].Text = strings.TrimSpace(changed.Obligations[0].Text)
	if changed.Validate() == nil || changed.WithHash().Hash == inventory.Hash {
		t.Fatal("criterion whitespace drift did not change exact inventory identity")
	}
}

func TestCompileRuntimeSourceObligationsDoesNotInventSpecification(t *testing.T) {
	candidate, _, _ := sourceObligationAdmissionFixture(t)
	candidate.Specification = nil
	candidate = candidate.WithHash()
	candidate, startup, admission := sourceObligationSealFixture(candidate)
	inventory, err := CompileRuntimeSourceObligationInventory(candidate, startup, admission)
	if err != nil || inventory != nil {
		t.Fatalf("invented absent specification: %+v %v", inventory, err)
	}
}
