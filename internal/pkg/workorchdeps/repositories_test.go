package workorchdeps

import (
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	approvalcommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	eventstreamcommon "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/common"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	runcommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	skillcommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	soulcommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	orchestratorcommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	workspacecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
)

func TestRepositoryConsumerIDsMatchOwnerModuleIDs(t *testing.T) {
	tests := []struct {
		consumer RepositoryConsumerID
		moduleID string
	}{
		{consumer: repositoryConsumerWorkspace, moduleID: workspacecommon.ModuleID},
		{consumer: repositoryConsumerDecision, moduleID: decisioncommon.ModuleID},
		{consumer: repositoryConsumerApproval, moduleID: approvalcommon.ModuleID},
		{consumer: repositoryConsumerEventStream, moduleID: eventstreamcommon.ModuleID},
		{consumer: repositoryConsumerGovernance, moduleID: governancecommon.ModuleID},
		{consumer: repositoryConsumerMCP, moduleID: mcpcommon.ModuleID},
		{consumer: repositoryConsumerMemory, moduleID: memorycommon.ModuleID},
		{consumer: repositoryConsumerOrchestrator, moduleID: orchestratorcommon.ModuleID},
		{consumer: repositoryConsumerResult, moduleID: resultcommon.ModuleID},
		{consumer: repositoryConsumerRun, moduleID: runcommon.ModuleID},
		{consumer: repositoryConsumerSkill, moduleID: skillcommon.ModuleID},
		{consumer: repositoryConsumerSoul, moduleID: soulcommon.ModuleID},
	}

	for _, test := range tests {
		if string(test.consumer) != test.moduleID {
			t.Errorf("repository consumer %q does not match owner ModuleID %q", test.consumer, test.moduleID)
		}
	}
}

func TestRepositoryConsumerFamilyMatrixIsExact(t *testing.T) {
	bindings := []struct {
		consumer RepositoryConsumerID
		family   repositoryFamily
	}{
		{consumer: repositoryConsumerWorkspace, family: repositoryFamilyWorkspace},
		{consumer: repositoryConsumerDecision, family: repositoryFamilyDecision},
		{consumer: repositoryConsumerApproval, family: repositoryFamilyApproval},
		{consumer: repositoryConsumerEventStream, family: repositoryFamilyEventStream},
		{consumer: repositoryConsumerGovernance, family: repositoryFamilyGovernance},
		{consumer: repositoryConsumerMCP, family: repositoryFamilyMCP},
		{consumer: repositoryConsumerMemory, family: repositoryFamilyMemory},
		{consumer: repositoryConsumerOrchestrator, family: repositoryFamilyOrchestrator},
		{consumer: repositoryConsumerResult, family: repositoryFamilyResult},
		{consumer: repositoryConsumerRun, family: repositoryFamilyRun},
		{consumer: repositoryConsumerSkill, family: repositoryFamilySkill},
		{consumer: repositoryConsumerSoul, family: repositoryFamilySoul},
	}

	for _, consumerBinding := range bindings {
		for _, familyBinding := range bindings {
			err := validateRepositoryAccess(consumerBinding.consumer, familyBinding.family)
			if consumerBinding.family == familyBinding.family {
				if err != nil {
					t.Errorf("consumer %q rejected from own %s family: %v", consumerBinding.consumer, familyBinding.family, err)
				}
				continue
			}
			if err == nil {
				t.Errorf("consumer %q resolved sibling %s family", consumerBinding.consumer, familyBinding.family)
			}
		}
	}

	for _, consumer := range []RepositoryConsumerID{"", "blocks/unknown", " blocks/workspace", "blocks/workspace "} {
		if err := validateRepositoryAccess(consumer, repositoryFamilyWorkspace); err == nil {
			t.Errorf("unknown or non-canonical consumer %q resolved workspace repositories", consumer)
		}
	}
	if err := validateRepositoryAccess(repositoryConsumerWorkspace, repositoryFamily("unknown")); err == nil {
		t.Fatal("workspace consumer resolved an unknown repository family")
	}
}

func TestRepositoryResolversRejectSiblingConsumerBeforeProviderLookup(t *testing.T) {
	tests := []struct {
		name     string
		consumer RepositoryConsumerID
		family   repositoryFamily
		resolve  func(RepositoryConsumerID) *cd.Error
	}{
		{name: "workspace", consumer: repositoryConsumerDecision, family: repositoryFamilyWorkspace, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveWorkspaceRepositories(consumer)
			return err
		}},
		{name: "decision", consumer: repositoryConsumerApproval, family: repositoryFamilyDecision, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveDecisionRepositories(consumer)
			return err
		}},
		{name: "approval", consumer: repositoryConsumerEventStream, family: repositoryFamilyApproval, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveApprovalRepositories(consumer)
			return err
		}},
		{name: "eventstream", consumer: repositoryConsumerGovernance, family: repositoryFamilyEventStream, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveEventStreamRepositories(consumer)
			return err
		}},
		{name: "governance", consumer: repositoryConsumerMCP, family: repositoryFamilyGovernance, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveGovernanceRepositories(consumer)
			return err
		}},
		{name: "mcp", consumer: repositoryConsumerMemory, family: repositoryFamilyMCP, resolve: func(consumer RepositoryConsumerID) *cd.Error { _, err := ResolveMCPRepositories(consumer); return err }},
		{name: "memory", consumer: repositoryConsumerOrchestrator, family: repositoryFamilyMemory, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveMemoryRepositories(consumer)
			return err
		}},
		{name: "orchestrator", consumer: repositoryConsumerResult, family: repositoryFamilyOrchestrator, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveOrchestratorRepositories(consumer)
			return err
		}},
		{name: "result", consumer: repositoryConsumerRun, family: repositoryFamilyResult, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveResultRepositories(consumer)
			return err
		}},
		{name: "run", consumer: repositoryConsumerSkill, family: repositoryFamilyRun, resolve: func(consumer RepositoryConsumerID) *cd.Error { _, err := ResolveRunRepositories(consumer); return err }},
		{name: "skill", consumer: repositoryConsumerSoul, family: repositoryFamilySkill, resolve: func(consumer RepositoryConsumerID) *cd.Error {
			_, err := ResolveSkillRepositories(consumer)
			return err
		}},
		{name: "soul", consumer: repositoryConsumerWorkspace, family: repositoryFamilySoul, resolve: func(consumer RepositoryConsumerID) *cd.Error { _, err := ResolveSoulRepositories(consumer); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.resolve(test.consumer)
			if err == nil {
				t.Fatalf("consumer %q resolved %s repositories", test.consumer, test.family)
			}
			message := err.Error()
			if !strings.Contains(message, "not authorized") || !strings.Contains(message, string(test.consumer)) || !strings.Contains(message, string(test.family)) {
				t.Fatalf("access rejection = %q, want consumer and family", message)
			}
		})
	}
}
