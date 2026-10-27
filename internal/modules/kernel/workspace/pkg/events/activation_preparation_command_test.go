package events

import (
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestPreparationRunHistorySettlementRequiresExactOriginalFacts(t *testing.T) {
	fixture := func() SettleWorkspacePreparationBlockedCommand {
		return SettleWorkspacePreparationBlockedCommand{
			Original:            CheckWorkspacePreparationCommand{WorkspaceID: "ws", RequestID: "original", ExpectedWorkspaceRevision: 1},
			ProviderCacheReason: "provider_cache_unavailable", RunHistoryReason: "run_history_empty_fence_not_final",
			RunHistoryFacts: &workspace.PreparationRunHistoryFacts{
				Version: workspace.PreparationRunHistoryEvidenceVersion, WorkspaceRevision: 2,
				Observation: runevents.ObserveWorkspacePreparationHistoryResult{
					RequestID: "original", ObservedAt: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
					History: runmodel.WorkspacePreparationHistory{WorkspaceID: "ws"},
				},
			},
		}
	}
	c := fixture()
	if c.Validate() != nil {
		t.Fatal("complete original observation rejected")
	}
	for _, change := range []func(*SettleWorkspacePreparationBlockedCommand){
		func(c *SettleWorkspacePreparationBlockedCommand) { c.RunHistoryFacts.Version = "future" },
		func(c *SettleWorkspacePreparationBlockedCommand) { c.RunHistoryFacts.WorkspaceRevision++ },
		func(c *SettleWorkspacePreparationBlockedCommand) { c.Original.ExpectedWorkspaceRevision = ^uint64(0) },
		func(c *SettleWorkspacePreparationBlockedCommand) {
			c.RunHistoryFacts.Observation.History.WorkspaceID = "other"
		},
		func(c *SettleWorkspacePreparationBlockedCommand) { c.RunHistoryFacts.Observation.RequestID = "other" },
		func(c *SettleWorkspacePreparationBlockedCommand) {
			c.RunHistoryFacts.Observation.ObservedAt = time.Time{}
		},
		func(c *SettleWorkspacePreparationBlockedCommand) { c.RunHistoryFacts.Observation.History.RunCount = 1 },
		func(c *SettleWorkspacePreparationBlockedCommand) {
			c.RunHistoryFacts.Observation.History.GenerationHead = 1
		},
		func(c *SettleWorkspacePreparationBlockedCommand) { c.RunHistoryReason = "run_history_present" },
		func(c *SettleWorkspacePreparationBlockedCommand) { c.RunHistoryReason = "no_run" },
	} {
		c = fixture()
		change(&c)
		if c.Validate() == nil {
			t.Fatal("foreign, incomplete or misclassified history accepted")
		}
	}
	// A legacy negative classification remains readable, but carries no material
	// from which the owner could fabricate an archive or a no-Run permit.
	c = fixture()
	c.RunHistoryFacts = nil
	if c.Validate() != nil {
		t.Fatal("legacy classification rejected")
	}
}

func TestPreparationNegativeSettlementCannotConveyPositiveOrForeignCheckReason(t *testing.T) {
	c := SettleWorkspacePreparationBlockedCommand{Original: CheckWorkspacePreparationCommand{WorkspaceID: "ws", RequestID: "original", ExpectedWorkspaceRevision: 1}, ProviderCacheReason: "provider_declaration_only", RunHistoryReason: "run_history_empty_fence_not_final"}
	if c.Validate() != nil {
		t.Fatal("valid negative settlement rejected")
	}
	for _, reason := range []string{"", "pass", "ready", "not_applicable", "run_history_present", "private raw diagnostic"} {
		bad := c
		bad.ProviderCacheReason = reason
		if bad.Validate() == nil {
			t.Fatalf("accepted provider classification %q", reason)
		}
	}
	for _, reason := range []string{"pass", "no_run", "provider_cache_miss", "ready"} {
		bad := c
		bad.RunHistoryReason = reason
		if bad.Validate() == nil {
			t.Fatalf("accepted history classification %q", reason)
		}
	}
	for _, reason := range []string{"pass", "ready", "provider_cache_miss", "souls_observed_remaining_checks"} {
		bad := c
		bad.SoulReason = reason
		if bad.Validate() == nil {
			t.Fatal("Soul verdict accepted without complete facts", reason)
		}
	}
	c.SoulReason = "soul_observation_unavailable_not_saved"
	if c.Validate() != nil {
		t.Fatal("explicit unarchived Soul failure rejected")
	}
	for _, reason := range []string{"ready", "pass", "provider_cache_miss", "skill_registry_observed_selection_pending"} {
		bad := c
		bad.SkillRegistryReason = reason
		if bad.Validate() == nil {
			t.Fatal("registry verdict accepted without complete facts", reason)
		}
	}
	c.SkillRegistryReason = "skill_registry_observation_unavailable_not_saved"
	if c.Validate() != nil {
		t.Fatal("explicit unarchived registry failure rejected")
	}
}

func TestPreparationFinalOwnersSettlementBindsOriginalAndRetainsMissingFacts(t *testing.T) {
	original := CheckWorkspacePreparationCommand{WorkspaceID: "ws", RequestID: "original", ExpectedWorkspaceRevision: 1}
	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	c := SettleWorkspacePreparationBlockedCommand{Original: original, ProviderCacheReason: "provider_cache_unavailable", RunHistoryReason: "run_history_unavailable",
		FinalOwners: &workspace.PreparationOwnerRevalidation{Version: workspace.PreparationOwnerRevalidationVersion, WorkspaceID: original.WorkspaceID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), WorkspaceRevision: 2, StartedAt: at, CompletedAt: at.Add(time.Second)},
	}
	if c.Validate() != nil {
		t.Fatal("missing final owner facts must remain an explicit negative observation")
	}
	for _, mutate := range []func(*workspace.PreparationOwnerRevalidation){
		func(r *workspace.PreparationOwnerRevalidation) { r.Version = "future" },
		func(r *workspace.PreparationOwnerRevalidation) { r.WorkspaceID = "foreign" },
		func(r *workspace.PreparationOwnerRevalidation) { r.RequestID = "foreign" },
		func(r *workspace.PreparationOwnerRevalidation) { r.PayloadHash = "foreign" },
		func(r *workspace.PreparationOwnerRevalidation) { r.WorkspaceRevision++ },
		func(r *workspace.PreparationOwnerRevalidation) { r.CompletedAt = r.StartedAt.Add(-time.Second) },
	} {
		bad := c
		r := c.FinalOwners.Clone()
		mutate(&r)
		bad.FinalOwners = &r
		if bad.Validate() == nil {
			t.Fatal("foreign or invalid final owner observations accepted")
		}
	}
}
