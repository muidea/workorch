package workorchruntimecontract

import "testing"

func TestRuntimeValidationSurfaceReadinessContract(t *testing.T) {
	base := RuntimeValidationSurface{
		ID:                 "mcp/tool_call/mcp-lsp-bridge/lsp_status",
		OwnerModuleID:      RuntimeModuleMCP,
		OperationKind:      StepKindMCPToolCall,
		InputSchemaHash:    "schema-v1",
		CheckRefs:          []string{"mcp/tool_call/mcp-lsp-bridge/lsp_status"},
		ProofApplicability: RuntimeValidationProofDirect,
		EffectScope:        RuntimeEffectNone,
		IdempotencyClass:   RuntimeOperationIdempotencySafeToReplay,
	}
	tests := []struct {
		name    string
		mutate  func(*RuntimeValidationSurface)
		wantErr bool
	}{
		{name: "ready", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfaceReady
			value.InputBindingRef = "mcp-validation-input/empty/status/schema-v1"
		}},
		{name: "prerequisite required", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfacePrerequisiteRequired
			value.ReadinessReason = "input_binding_required"
			value.PrerequisiteRef = "mcp-validation-input/definition/schema-v1"
		}},
		{name: "unavailable", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfaceUnavailable
			value.ReadinessReason = "unsafe_effect"
		}},
		{name: "ready without binding", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfaceReady
		}, wantErr: true},
		{name: "prerequisite with binding", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfacePrerequisiteRequired
			value.ReadinessReason = "input_binding_required"
			value.InputBindingRef = "caller-invented-input"
			value.PrerequisiteRef = "mcp-validation-input/definition/schema-v1"
		}, wantErr: true},
		{name: "unavailable with prerequisite", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfaceUnavailable
			value.ReadinessReason = "invalid_input_schema"
			value.PrerequisiteRef = "unsupported"
		}, wantErr: true},
		{name: "unknown proof applicability", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfaceReady
			value.InputBindingRef = "mcp-validation-input/empty/status/schema-v1"
			value.ProofApplicability = "claimed_by_provider"
		}, wantErr: true},
		{name: "missing proof applicability", mutate: func(value *RuntimeValidationSurface) {
			value.Readiness = RuntimeValidationSurfaceReady
			value.InputBindingRef = "mcp-validation-input/empty/status/schema-v1"
			value.ProofApplicability = ""
		}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base.Clone()
			test.mutate(&value)
			err := value.Validate()
			if test.wantErr && err == nil {
				t.Fatalf("invalid validation surface accepted: %+v", value)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("valid validation surface rejected: %v", err)
			}
		})
	}
}

func TestRuntimeOperationAdmissibilityExposesOnlyReadyValidationSurfaces(t *testing.T) {
	ready := RuntimeValidationSurface{
		ID: "mcp/tool_call/mcp-lsp-bridge/lsp_status", OwnerModuleID: RuntimeModuleMCP, OperationKind: StepKindMCPToolCall,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "status-schema-v1", InputBindingRef: "empty-status-input",
		CheckRefs: []string{"mcp/tool_call/mcp-lsp-bridge/lsp_status"}, ProofApplicability: RuntimeValidationProofDirect, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
	}
	requiresInput := RuntimeValidationSurface{
		ID: "mcp/tool_call/mcp-lsp-bridge/lsp_definition", OwnerModuleID: RuntimeModuleMCP, OperationKind: StepKindMCPToolCall,
		Readiness: RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "input_binding_required",
		InputSchemaHash: "definition-schema-v1", PrerequisiteRef: "definition-input-v1",
		CheckRefs: []string{"mcp/tool_call/mcp-lsp-bridge/lsp_definition"}, ProofApplicability: RuntimeValidationProofDirect, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
	}
	inventory := RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-catalog", OwnerModuleID: RuntimeModuleMCP,
		OperationKinds: []string{StepKindMCPToolCall}, SelectorRefs: []string{"mcp-lsp-bridge"},
		ValidationRecipeRefs: []string{ready.ID}, ValidationSurfaces: []RuntimeValidationSurface{requiresInput, ready},
		ObservedRevision: "mcp-catalog-v1",
	}
	inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		t.Fatalf("ready-only validation catalog rejected: %v", err)
	}
	if got := RuntimeReadyValidationRecipeRefs(inventory.ValidationSurfaces); len(got) != 1 || got[0] != ready.ID {
		t.Fatalf("ready recipe refs = %v, want only %q", got, ready.ID)
	}
	if got := RuntimeSelectableValidationRecipeRefs(inventory.ValidationSurfaces); len(got) != 1 || got[0] != ready.ID {
		t.Fatalf("selectable recipe refs = %v, want only %q", got, ready.ID)
	}
	auxiliary := ready
	auxiliary.ProofApplicability = RuntimeValidationProofAuxiliary
	if got := RuntimeSelectableValidationRecipeRefs([]RuntimeValidationSurface{auxiliary}); len(got) != 0 {
		t.Fatalf("auxiliary ready surface became selectable: %v", got)
	}
	atMostOnce := ready
	atMostOnce.EffectScope = RuntimeEffectEphemeralSession
	atMostOnce.IdempotencyClass = RuntimeOperationIdempotencyAtMostOnce
	if got := RuntimeSelectableValidationRecipeRefs([]RuntimeValidationSurface{atMostOnce}); len(got) != 1 || got[0] != ready.ID {
		t.Fatalf("at-most-once ephemeral validation surface was not selectable: %v", got)
	}
	durable := ready
	durable.EffectScope = RuntimeEffectWorkspaceDurable
	if got := RuntimeSelectableValidationRecipeRefs([]RuntimeValidationSurface{durable}); len(got) != 0 {
		t.Fatalf("durable validation surface became selectable: %v", got)
	}
	unknownReplay := ready
	unknownReplay.IdempotencyClass = RuntimeOperationIdempotencyUnknown
	if got := RuntimeSelectableValidationRecipeRefs([]RuntimeValidationSurface{unknownReplay}); len(got) != 0 {
		t.Fatalf("replay-unknown validation surface became selectable: %v", got)
	}
	changedApplicability := inventory
	changedApplicability.ValidationSurfaces = append([]RuntimeValidationSurface(nil), inventory.ValidationSurfaces...)
	changedApplicability.ValidationSurfaces[1].ProofApplicability = RuntimeValidationProofAuxiliary
	if StableRuntimeOperationAdmissibilitySnapshotID(changedApplicability) == inventory.SnapshotID {
		t.Fatal("admissibility snapshot identity ignored validation proof applicability")
	}
	changedApplicability.ValidationRecipeRefs = nil
	changedApplicability.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(changedApplicability)
	if err := changedApplicability.Validate(); err != nil {
		t.Fatalf("ready auxiliary surface should remain audit-valid: %v", err)
	}

	leaked := inventory
	leaked.ValidationRecipeRefs = append(leaked.ValidationRecipeRefs, requiresInput.ID)
	leaked.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(leaked)
	if err := leaked.Validate(); err == nil {
		t.Fatal("prerequisite-required validation surface leaked into provider recipe refs")
	}

	changedReadiness := inventory
	changedReadiness.ValidationSurfaces = append([]RuntimeValidationSurface(nil), inventory.ValidationSurfaces...)
	changedReadiness.ValidationSurfaces[0].Readiness = RuntimeValidationSurfaceUnavailable
	changedReadiness.ValidationSurfaces[0].ReadinessReason = "invalid_input_schema"
	changedReadiness.ValidationSurfaces[0].PrerequisiteRef = ""
	if StableRuntimeOperationAdmissibilitySnapshotID(changedReadiness) == inventory.SnapshotID {
		t.Fatal("admissibility snapshot identity ignored validation readiness")
	}

	changedBinding := inventory
	changedBinding.ValidationSurfaces = append([]RuntimeValidationSurface(nil), inventory.ValidationSurfaces...)
	changedBinding.ValidationSurfaces[1].InputBindingRef = "empty-status-input-v2"
	if StableRuntimeOperationAdmissibilitySnapshotID(changedBinding) == inventory.SnapshotID {
		t.Fatal("admissibility snapshot identity ignored validation input binding")
	}
}
