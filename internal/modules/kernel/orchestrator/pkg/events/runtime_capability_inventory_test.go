package events_test

import (
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	sshcommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCapabilityOwnerInventoriesProduceCanonicalReceipts(t *testing.T) {
	owners := []struct {
		moduleID string
		kinds    func() []string
		classFor func(string) runtimecontract.RuntimeCapabilityClass
	}{
		{moduleID: filecommon.ModuleID, kinds: filecommon.RuntimeCapabilityKinds, classFor: descriptorCapabilityClass},
		{moduleID: gitcommon.ModuleID, kinds: gitcommon.RuntimeCapabilityKinds, classFor: descriptorCapabilityClass},
		{moduleID: shellcommon.ModuleID, kinds: shellcommon.RuntimeCapabilityKinds, classFor: descriptorCapabilityClass},
		{moduleID: mcpcommon.ModuleID, kinds: mcpcommon.RuntimeCapabilityKinds, classFor: descriptorCapabilityClass},
		{moduleID: sshcommon.ModuleID, kinds: sshcommon.RuntimeCapabilityKinds, classFor: descriptorCapabilityClass},
		{moduleID: webcommon.ModuleID, kinds: webcommon.RuntimeCapabilityKinds, classFor: descriptorCapabilityClass},
		{moduleID: decisioncommon.ModuleID, kinds: decisioncommon.RuntimeCapabilityKinds, classFor: descriptorCapabilityClass},
		// LLM candidate generation is not a Runtime operation descriptor, but
		// its owner still uses the same capability receipt contract.
		{moduleID: llmcommon.ModuleID, kinds: llmcommon.RuntimeCapabilityKinds, classFor: func(string) runtimecontract.RuntimeCapabilityClass {
			return runtimecontract.RuntimeCapabilityClassValidation
		}},
	}
	for _, owner := range owners {
		owner := owner
		t.Run(owner.moduleID, func(t *testing.T) {
			original := owner.kinds()
			if len(original) == 0 {
				t.Fatal("owner capability inventory is empty")
			}
			original[0] = "mutated"
			if next := owner.kinds(); len(next) == 0 || next[0] == "mutated" {
				t.Fatalf("owner capability inventory is mutable: %v", next)
			}
			for _, kind := range owner.kinds() {
				class := owner.classFor(kind)
				if class == "" {
					t.Fatalf("owner %s kind %q has no declared capability class", owner.moduleID, kind)
				}
				result, err := orchestratorevents.ResolveRuntimeCapabilities(orchestratorevents.AcquireRuntimeCapabilitiesCommand{
					WorkspaceID: "ws-inventory", RunID: "run-inventory", AuthoritySnapshotID: "authority-inventory",
					PrerequisiteID: "capability:" + owner.moduleID + ":" + kind, ModuleID: owner.moduleID,
					CapabilityClass: class, RequiredKinds: []string{kind},
				}, owner.moduleID, owner.kinds()...)
				if err != nil {
					t.Fatalf("owner %s kind %s receipt: %v", owner.moduleID, kind, err)
				}
				if result.Observation == nil || result.Observation.OwnerModuleID != owner.moduleID || result.Observation.Kind != runtimecontract.PrerequisiteKindCapability ||
					len(result.Observation.Dependencies) != 1 || result.Observation.Dependencies[0].Domain != runtimecontract.RuntimeResourceOwnerCatalog ||
					result.Observation.Dependencies[0].OwnerModuleID != owner.moduleID {
					t.Fatalf("owner %s kind %s observation=%+v", owner.moduleID, kind, result.Observation)
				}
				var got []string
				switch class {
				case runtimecontract.RuntimeCapabilityClassStateChange:
					got = result.Capabilities.StateChangeKinds
				case runtimecontract.RuntimeCapabilityClassValidation:
					got = result.Capabilities.ValidationKinds
				case runtimecontract.RuntimeCapabilityClassGovernance:
					got = result.Capabilities.GovernanceKinds
				}
				if len(got) != 1 || got[0] != kind {
					t.Fatalf("owner %s kind %s capability result=%+v", owner.moduleID, kind, result.Capabilities)
				}
			}
		})
	}
}

func descriptorCapabilityClass(kind string) runtimecontract.RuntimeCapabilityClass {
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return ""
	}
	return descriptor.CapabilityClass
}
