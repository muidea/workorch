package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateLikeNameRequiresExplicitStateVocabularySuffix(t *testing.T) {
	for _, name := range []string{"WorkspaceState", "RuntimeStatus", "planningPhase", "CandidateDisposition", "OperationOutcome", "loadMode", "RunLifecycle", "DriveStage"} {
		if !isStateLikeName(name) {
			t.Fatalf("%q was not classified as state-like", name)
		}
	}
	for _, name := range []string{"EvidenceQuality", "CheckpointType", "StatefulValue"} {
		if isStateLikeName(name) {
			t.Fatalf("%q was incorrectly classified as state-like", name)
		}
	}
}

func TestVerifyRequiresInventoryForBareStateLikeStringFieldInOwnerModel(t *testing.T) {
	root := writeBareOwnerModelFixture(t)
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "model.go:Snapshot.Status") {
		t.Fatalf("verify error = %v, want uninventoried owner-model field rejection", err)
	}
}

func TestVerifyRequiresInventoryForNullableBareStateLikeStringFieldInOwnerModel(t *testing.T) {
	root := writeBareOwnerModelFixtureWithField(t, "Status *string")
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "model.go:Snapshot.Status") {
		t.Fatalf("verify error = %v, want uninventoried nullable owner-model field rejection", err)
	}
}

func TestVerifyAllowsOnlyExplicitNonAuthorityClassificationsForBareOwnerModelField(t *testing.T) {
	for _, classification := range []string{"immutable_outcome", "immutable_trace", "derived_projection"} {
		t.Run(classification, func(t *testing.T) {
			root := writeBareOwnerModelFixture(t)
			row := "internal/modules/blocks/demo/pkg/models/model.go|Snapshot.Status|" + classification + "|internal/modules/blocks/demo|-\n"
			if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte(row), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
				t.Fatalf("explicit %s bare-field classification rejected: %v", classification, err)
			}
		})
	}
}

func TestVerifyRequiresNamedTypeForTransitionBearingOwnerModelField(t *testing.T) {
	root := writeBareOwnerModelFixture(t)
	row := "internal/modules/blocks/demo/pkg/models/model.go|Snapshot.Status|owner_private_state|internal/modules/blocks/demo|-\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "use a named state type for transition authority") {
		t.Fatalf("verify error = %v, want named transition-state type rejection", err)
	}
}

func TestVerifyDoesNotInventoryBareStateFieldsOutsideOwnerModels(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	path := filepath.Join(root, "internal/modules/blocks/demo/pkg/events/snapshot.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package events\ntype Snapshot struct { Status string }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("non-owner-model observation must stay outside persistent inventory: %v", err)
	}
}

func TestTargetStateFieldRequiresOwnerPrivateTransition(t *testing.T) {
	for _, name := range []string{"State", "NextState", "TargetState", "DesiredState", "InitialState", "TransitionState", "RequestedTargetState"} {
		if !isTargetStateField(name) {
			t.Fatalf("%q was not classified as a target-state field", name)
		}
	}
	for _, name := range []string{"CurrentState", "ObservedState", "StateFilter", "DesiredStateFilter"} {
		if isTargetStateField(name) {
			t.Fatalf("%q was incorrectly classified as a target-state field", name)
		}
	}
	for _, name := range []string{"TotalState", "TokenStatus", "TopicMode", "ToolStage"} {
		if isForbiddenCrossOwnerStateField(name, true) {
			t.Fatalf("observed carrier field %q was incorrectly treated as a To-state target", name)
		}
	}
	for _, name := range []string{"ToState", "ToStatus", "ToPhase", "ToDisposition", "ToOutcome", "ToMode", "ToLifecycle", "ToStage"} {
		if !isForbiddenCrossOwnerStateField(name, true) {
			t.Fatalf("directed field %q was not rejected inside an observed carrier", name)
		}
	}
}

func TestVerifyRejectsCrossOwnerCommandWithTargetState(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	path := filepath.Join(root, "internal/modules/blocks/demo/pkg/events/messages.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`package events
type AdvanceWidgetCommand struct {
	WidgetID string
	TargetState string
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "AdvanceWidgetCommand") || !strings.Contains(err.Error(), "TargetState") {
		t.Fatalf("verify error = %v, want cross-owner target-state rejection", err)
	}
}

func TestVerifyRejectsCrossOwnerCommandTargetStateHiddenByAliasOrEmbedding(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "named underlying type",
			source: `package events
type transitionFields struct { DesiredState string }
type AdvanceWidgetCommand transitionFields
`,
		},
		{
			name: "anonymous embedded type",
			source: `package events
type transitionFields struct { State string }
type AdvanceWidgetCommand struct { transitionFields }
`,
		},
		{
			name: "named intent field",
			source: `package events
type transitionIntent struct { TargetState string }
type AdvanceWidgetCommand struct { Intent transitionIntent }
`,
		},
		{
			name: "benign field name cannot bless intent",
			source: `package events
type transitionPayload struct { State string }
type AdvanceWidgetCommand struct { Context transitionPayload }
`,
		},
		{
			name: "same type is rescanned outside fact context",
			source: `package events
type transitionPayload struct { State string }
type AdvanceWidgetCommand struct {
	Observed transitionPayloadFact
	Intent transitionPayload
}
type transitionPayloadFact transitionPayload
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newEmptyVerificationRepository(t)
			path := filepath.Join(root, "internal/modules/blocks/demo/pkg/events/messages.go")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
			if err == nil || !strings.Contains(err.Error(), "AdvanceWidgetCommand") {
				t.Fatalf("verify error = %v, want hidden target-state rejection", err)
			}
		})
	}
}

func TestVerifyRejectsCrossOwnerCommandTargetStateHiddenByRepositoryType(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	sharedPath := filepath.Join(root, "internal/pkg/shared/request.go")
	if err := os.MkdirAll(filepath.Dir(sharedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sharedPath, []byte("package shared\ntype TransitionRequest struct { TargetState string }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(root, "internal/modules/blocks/demo/pkg/events/messages.go")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commandPath, []byte(`package events
import "github.com/muidea/workorch/internal/pkg/shared"
type AdvanceWidgetCommand shared.TransitionRequest
`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "AdvanceWidgetCommand") || !strings.Contains(err.Error(), "TargetState") {
		t.Fatalf("verify error = %v, want repository-qualified target-state rejection", err)
	}
}

func TestVerifyRejectsRepositoryLocalDotOrBlankImportInEveryProductionEventsFile(t *testing.T) {
	for _, alias := range []string{".", "_"} {
		t.Run(alias, func(t *testing.T) {
			root := newEmptyVerificationRepository(t)
			sharedPath := filepath.Join(root, "internal/pkg/shared/payload.go")
			if err := os.MkdirAll(filepath.Dir(sharedPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sharedPath, []byte("package shared\ntype Payload struct { TargetState string }\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			eventsDir := filepath.Join(root, "internal/modules/blocks/demo/pkg/events")
			if err := os.MkdirAll(eventsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(eventsDir, "a.go"), []byte("package events\ntype AdvanceWidgetCommand struct { Data Shared }\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			body := "package events\nimport " + alias + " \"github.com/muidea/workorch/internal/pkg/shared\"\ntype Shared struct{}\n"
			if alias == "." {
				body = "package events\nimport . \"github.com/muidea/workorch/internal/pkg/shared\"\ntype Shared Payload\n"
			}
			if err := os.WriteFile(filepath.Join(eventsDir, "b.go"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
			if err == nil || !strings.Contains(err.Error(), "dot/blank import") || !strings.Contains(err.Error(), "b.go") {
				t.Fatalf("verify error = %v, want repository-local %q import rejection in indirect command DTO file", err, alias)
			}
		})
	}
}

func TestVerifyAllowsQueryFilters(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	path := filepath.Join(root, "internal/modules/blocks/demo/pkg/events/messages.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`package events
type ListWidgetsCommand struct {
	StateFilter string
	DesiredStateFilter []string
	CurrentState string
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("query filters must be allowed: %v", err)
	}
}

func TestVerifyRejectsUnregisteredFactNamingEscape(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	path := filepath.Join(root, "internal/modules/blocks/demo/pkg/events/messages.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`package events
type WidgetFact struct { State string }
type RecordWidgetFactCommand struct { Data WidgetFact }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "RecordWidgetFactCommand") || !strings.Contains(err.Error(), "State") {
		t.Fatalf("verify error = %v, want unregistered fact carrier rejection", err)
	}
}

func TestVerifyAllowsExplicitApprovalResolutionFactButStillRejectsTargetState(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	path := filepath.Join(root, "internal/modules/blocks/eventstream/pkg/events/messages.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`package events
type ApprovalResolutionFact struct { State string }
type RecordApprovalResolutionCommand struct { Data ApprovalResolutionFact }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("explicit immutable Approval fact must be allowed: %v", err)
	}

	if err := os.WriteFile(path, []byte(`package events
type ApprovalResolutionFact struct { State string; TargetState string }
type RecordApprovalResolutionCommand struct { Data ApprovalResolutionFact }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "TargetState") {
		t.Fatalf("explicit fact carrier must not hide target-state authority: %v", err)
	}
}

func TestPreparationDiagnosticStageIsObservationButTargetStateStaysForbidden(t *testing.T) {
	for _, fields := range []string{"Stage string", "Stage string; State string", "Stage string; TargetStage string", "Stage string; DesiredState string"} {
		t.Run(fields, func(t *testing.T) {
			root := newEmptyVerificationRepository(t)
			modelPath := filepath.Join(root, "internal/modules/kernel/workspace/pkg/models/diagnostic.go")
			commandPath := filepath.Join(root, "internal/modules/kernel/workspace/pkg/events/messages.go")
			for path, source := range map[string]string{
				modelPath:   "package models\ntype DefinitionAuthoringPreparationDiagnostic struct { " + fields + " }\n",
				commandPath: "package events\nimport models \"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models\"\ntype RecordPreparationDiagnosticCommand struct { Diagnostic models.DefinitionAuthoringPreparationDiagnostic }\n",
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// 只验证精确字段例外；新增 sibling 状态仍不能成为 caller authority。
			err := verifyCrossOwnerCommandContracts(root)
			if fields == "Stage string" {
				if err != nil {
					t.Fatalf("不可变失败位置不能被当作 transition intent：%v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "RecordPreparationDiagnosticCommand") {
				t.Fatalf("新增目标状态必须被拒绝，fields=%s，err=%v", fields, err)
			}
		})
	}
}

func TestRuntimeStateRecordObservedStateIsAllowedOnlyAtRunOwnerStoreEdge(t *testing.T) {
	writeFixture := func(t *testing.T, commandName, fields string) string {
		t.Helper()
		root := newEmptyVerificationRepository(t)
		contractPath := filepath.Join(root, "internal/pkg/workorchruntimecontract/runtime_state.go")
		if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contractPath, []byte("package workorchruntimecontract\ntype RuntimeStateRecord struct { "+fields+" }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		commandPath := filepath.Join(root, "internal/modules/blocks/run/pkg/events/messages.go")
		if err := os.MkdirAll(filepath.Dir(commandPath), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "package events\nimport runtimecontract \"github.com/muidea/workorch/internal/pkg/workorchruntimecontract\"\ntype " + commandName + " struct { RuntimeState *runtimecontract.RuntimeStateRecord }\n"
		if err := os.WriteFile(commandPath, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("exact opaque store edge", func(t *testing.T) {
		root := writeFixture(t, "StoreRuntimeStateRecordCommand", "State string")
		if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
			t.Fatalf("Run owner opaque Runtime state store must be allowed: %v", err)
		}
	})
	t.Run("different command", func(t *testing.T) {
		root := writeFixture(t, "SmuggleRuntimeStateCommand", "State string")
		err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
		if err == nil || !strings.Contains(err.Error(), "SmuggleRuntimeStateCommand") {
			t.Fatalf("verify error = %v, want non-store Runtime state rejection", err)
		}
	})
	t.Run("target state remains forbidden", func(t *testing.T) {
		root := writeFixture(t, "StoreRuntimeStateRecordCommand", "State string; TargetState string")
		err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
		if err == nil || !strings.Contains(err.Error(), "TargetState") {
			t.Fatalf("verify error = %v, want nested target state rejection", err)
		}
	})
}

func TestVerifyRejectsInvariantWithMultipleTypedConstants(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte(`package demo
type FixedState string
const (
	FixedStateReady FixedState = "ready"
	FixedStateDone FixedState = "done"
)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|FixedState|invariant_value|application/demo|-\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "exactly 1") {
		t.Fatalf("verify error = %v, want invariant constant-count rejection", err)
	}
}

func TestVerifyRejectsTransitionBearingMachineWithoutNamedStateType(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte("demo.lifecycle|resource_state_machine|application/demo|application/demo/state.go|application/demo/state.go|applyDemoEvent|demoInternalEvent|applyDemoEvent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "without a named state type") {
		t.Fatalf("verify error = %v, want missing named state type rejection", err)
	}
}

func TestVerifyAllowsSameOwnerCoordinatedTransitions(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\ntype RunState string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|RunState|authoritative_state_machine|application/demo|demo.lifecycle,demo.replacement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transitions := "demo.lifecycle|resource_state_machine|application/demo|application/demo/lifecycle_internal_event.go|application/demo/lifecycle_internal_event.go|submitLifecycleInternalEvent|lifecycleInternalEvent|reduceLifecycleInternalEvent\n" +
		"demo.replacement|owner_private_transaction|application/demo|application/demo/replacement_internal_event.go|application/demo/replacement_internal_event.go|submitReplacementInternalEvent|replacementInternalEvent|reduceReplacementInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "application/demo/lifecycle_internal_event.go"), []byte("package demo\ntype lifecycleInternalEvent struct{}\nfunc reduceLifecycleInternalEvent(lifecycleInternalEvent) {}\nfunc submitLifecycleInternalEvent(event lifecycleInternalEvent) { reduceLifecycleInternalEvent(event) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/replacement_internal_event.go"), []byte("package demo\ntype replacementInternalEvent struct{}\nfunc reduceReplacementInternalEvent(replacementInternalEvent) {}\nfunc submitReplacementInternalEvent(event replacementInternalEvent) { reduceReplacementInternalEvent(event) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transitions), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("same-owner coordinated transitions must be allowed: %v", err)
	}
}

func TestVerifyRejectsNoopDispatcherThatDoesNotConsumeDeclaredInternalEvent(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\ntype RunState string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/internal_event.go"), []byte("package demo\ntype lifecycleInternalEvent struct{}\nfunc unrelated() {}\nfunc submitLifecycleInternalEvent(event lifecycleInternalEvent) { _ = event; unrelated() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|RunState|authoritative_state_machine|application/demo|demo.lifecycle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transition := "demo.lifecycle|resource_state_machine|application/demo|application/demo/internal_event.go|application/demo/internal_event.go|submitLifecycleInternalEvent|lifecycleInternalEvent|reduceLifecycleInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transition), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "does not consume") {
		t.Fatalf("verify error = %v, want no-op dispatcher rejection", err)
	}
}

func TestVerifyRejectsBenignPrefixedCallOutsideExactTransitionSink(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	if err := os.MkdirAll(filepath.Join(root, "application/demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\ntype RunState string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/internal_event.go"), []byte("package demo\ntype lifecycleInternalEvent struct{}\nfunc createRunLineage(any) {}\nfunc submitLifecycleInternalEvent(event lifecycleInternalEvent) { createRunLineage(event) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|RunState|authoritative_state_machine|application/demo|demo.lifecycle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transition := "demo.lifecycle|resource_state_machine|application/demo|application/demo/internal_event.go|application/demo/internal_event.go|submitLifecycleInternalEvent|lifecycleInternalEvent|runs.Create\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transition), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "does not consume") {
		t.Fatalf("verify error = %v, want exact transition-sink enforcement", err)
	}
}

func TestVerifyRejectsShadowedInternalEventAtExactSink(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type lifecycleInternalEvent struct{}
type unrelatedEvent struct{}
func reduceLifecycleInternalEvent(any) {}
func submitLifecycleInternalEvent(event lifecycleInternalEvent) {
	{
		event := unrelatedEvent{}
		reduceLifecycleInternalEvent(event)
	}
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent")
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "does not consume") {
		t.Fatalf("verify error = %v, want shadowed event rejection", err)
	}
}

func TestVerifyRejectsShadowedReceiverAtExactMutationSink(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type lifecycleInternalEvent struct{}
type repository struct{}
func (repository) Update(any) {}
func submitLifecycleInternalEvent(runs repository, event lifecycleInternalEvent) {
	{
		runs := repository{}
		runs.Update(event)
	}
}
`, "lifecycleInternalEvent", "runs.Update")
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "outside its exact transition sinks") {
		t.Fatalf("verify error = %v, want shadowed receiver rejection", err)
	}
}

func TestVerifyRejectsAmbiguousDispatcherSymbolInSamePackage(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type lifecycleInternalEvent struct{}
func reduceLifecycleInternalEvent(lifecycleInternalEvent) {}
func submitLifecycleInternalEvent(event lifecycleInternalEvent) { reduceLifecycleInternalEvent(event) }
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent")
	other := `package demo
type otherDispatcher struct{}
func (*otherDispatcher) submitLifecycleInternalEvent(event lifecycleInternalEvent) { reduceLifecycleInternalEvent(event) }
`
	if err := os.WriteFile(filepath.Join(root, "application/demo/other.go"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Fatalf("verify error = %v, want ambiguous dispatcher symbol rejection", err)
	}
}

func TestVerifyRejectsSameNamedForeignInternalEventType(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	demoDir := filepath.Join(root, "application/demo")
	otherDir := filepath.Join(root, "internal/other")
	for _, directory := range []string{demoDir, otherDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(demoDir, "internal_event.go"), []byte("package demo\ntype LifecycleInternalEvent struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "event.go"), []byte("package other\ntype LifecycleInternalEvent struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dispatcher := `package demo
import "github.com/muidea/workorch/internal/other"
func reduceLifecycleInternalEvent(any) {}
func submitLifecycleInternalEvent(event other.LifecycleInternalEvent) { reduceLifecycleInternalEvent(event) }
`
	if err := os.WriteFile(filepath.Join(demoDir, "dispatcher.go"), []byte(dispatcher), 0o644); err != nil {
		t.Fatal(err)
	}
	transition := "demo.private|owner_private_transaction|application/demo|application/demo/internal_event.go|application/demo/dispatcher.go|submitLifecycleInternalEvent|LifecycleInternalEvent|reduceLifecycleInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transition), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "exact manifest internal event type") {
		t.Fatalf("verify error = %v, want same-named foreign event rejection", err)
	}
}

func TestVerifyRejectsManifestInternalEventTypeAlias(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	demoDir := filepath.Join(root, "application/demo")
	otherDir := filepath.Join(root, "internal/other")
	for _, directory := range []string{demoDir, otherDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(otherDir, "event.go"), []byte("package other\ntype LifecycleInternalEvent struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "package demo\n" +
		"import \"github.com/muidea/workorch/internal/other\"\n" +
		"type LifecycleInternalEvent " + "= other.LifecycleInternalEvent\n" +
		"func reduceLifecycleInternalEvent(any) {}\n" +
		"func submitLifecycleInternalEvent(event LifecycleInternalEvent) { reduceLifecycleInternalEvent(event) }\n"
	if err := os.WriteFile(filepath.Join(demoDir, "internal_event.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	transition := "demo.private|owner_private_transaction|application/demo|application/demo/internal_event.go|application/demo/internal_event.go|submitLifecycleInternalEvent|LifecycleInternalEvent|reduceLifecycleInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transition), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "declares no event type") {
		t.Fatalf("verify error = %v, want aliased internal event type rejection", err)
	}
}

func TestVerifyRejectsValidReducerInOneBranchAndUndeclaredMutationInAnother(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type lifecycleInternalEvent struct{ Reduce bool }
type aggregate struct{ State string }
type repository struct{}
func (repository) CompareAndSwap(*aggregate, uint64) {}
func reduceLifecycleInternalEvent(lifecycleInternalEvent) *aggregate { return &aggregate{} }
func submitLifecycleInternalEvent(runs repository, event lifecycleInternalEvent) {
	if event.Reduce {
		reduceLifecycleInternalEvent(event)
	} else {
		runs.CompareAndSwap(&aggregate{State: "forced"}, 1)
	}
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent")
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "CompareAndSwap") {
		t.Fatalf("verify error = %v, want alternate mutation branch rejection", err)
	}
}

func TestVerifyRejectsGuardedConstantMutationOutsideExactSink(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type lifecycleInternalEvent struct{ Mutate bool }
type aggregate struct{ State string }
type repository struct{}
func (repository) Update(*aggregate) {}
func reduceLifecycleInternalEvent(lifecycleInternalEvent) *aggregate { return &aggregate{} }
func submitLifecycleInternalEvent(runs repository, event lifecycleInternalEvent) {
	_ = reduceLifecycleInternalEvent(event)
	if !event.Mutate {
		return
	}
	runs.Update(&aggregate{State: "forced"})
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent")
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "runs.Update") {
		t.Fatalf("verify error = %v, want guarded constant mutation rejection", err)
	}
}

func TestVerifyRejectsDeclaredPersistenceReceivingRawEventInsteadOfReducerOutput(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type aggregate struct{ State string }
type lifecycleInternalEvent struct{ Next *aggregate }
type repository struct{}
func (repository) Update(*aggregate) {}
func reduceLifecycleInternalEvent(lifecycleInternalEvent) *aggregate { return &aggregate{} }
func submitLifecycleInternalEvent(runs repository, event lifecycleInternalEvent) {
	_ = reduceLifecycleInternalEvent(event)
	runs.Update(event.Next)
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent,runs.Update")
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "outside its exact transition sinks") {
		t.Fatalf("verify error = %v, want raw event persistence rejection", err)
	}
}

func TestVerifyAllowsExactReducerOutputToReachDeclaredPersistence(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type aggregate struct{ State string }
type lifecycleInternalEvent struct{}
type repository struct{}
func (repository) Update(*aggregate) {}
func reduceLifecycleInternalEvent(lifecycleInternalEvent) *aggregate { return &aggregate{} }
func submitLifecycleInternalEvent(runs repository, event lifecycleInternalEvent) {
	next := reduceLifecycleInternalEvent(event)
	runs.Update(next)
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent,runs.Update")
	if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("exact reducer-derived persistence must be allowed: %v", err)
	}
}

func TestVerifyRejectsReducerOutputOverwrittenByRawEventBeforePersistence(t *testing.T) {
	root := writePrivateDispatcherFixture(t, `package demo
type aggregate struct{ State string }
type lifecycleInternalEvent struct{ Next *aggregate }
type repository struct{}
func (repository) Update(*aggregate) {}
func reduceLifecycleInternalEvent(lifecycleInternalEvent) *aggregate { return &aggregate{} }
func submitLifecycleInternalEvent(runs repository, event lifecycleInternalEvent) {
	next := reduceLifecycleInternalEvent(event)
	if event.Next != nil {
		next = event.Next
	}
	runs.Update(next)
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent,runs.Update")
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "outside its exact transition sinks") {
		t.Fatalf("verify error = %v, want reducer provenance clobber rejection", err)
	}
}

func TestVerifyRequiresExactReducerProvenanceForStateSelectorAssignment(t *testing.T) {
	valid := writePrivateDispatcherFixture(t, `package demo
type lifecycleInternalEvent struct{ State string }
type owner struct{ state string }
func reduceLifecycleInternalEvent(lifecycleInternalEvent) string { return "ready" }
func (m *owner) submitLifecycleInternalEvent(event lifecycleInternalEvent) {
	next := reduceLifecycleInternalEvent(event)
	m.state = next
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent")
	if err := verify(valid, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("exact reducer-derived state assignment must be allowed: %v", err)
	}

	invalid := writePrivateDispatcherFixture(t, `package demo
type lifecycleInternalEvent struct{ State string }
type owner struct{ state string }
func reduceLifecycleInternalEvent(lifecycleInternalEvent) string { return "ready" }
func (m *owner) submitLifecycleInternalEvent(event lifecycleInternalEvent) {
	_ = reduceLifecycleInternalEvent(event)
	m.state = event.State
}
`, "lifecycleInternalEvent", "reduceLifecycleInternalEvent")
	err := verify(invalid, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "without exact reducer-derived provenance") {
		t.Fatalf("verify error = %v, want direct event state assignment rejection", err)
	}
}

func TestVerifyRejectsTransitionDispatcherOutsideOwnerRoot(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	if err := os.MkdirAll(filepath.Join(root, "application/demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal/other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/internal_event.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/other/dispatcher.go"), []byte("package other\nfunc submitDemoInternalEvent() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transition := "demo.private|owner_private_transaction|application/demo|application/demo/internal_event.go|internal/other/dispatcher.go|submitDemoInternalEvent|demoInternalEvent|reduceDemoInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transition), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "outside owner root") {
		t.Fatalf("verify error = %v, want out-of-owner dispatcher rejection", err)
	}
}

func TestVerifyRejectsCoordinatedTransitionsAcrossOwners(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\ntype RunState string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|RunState|authoritative_state_machine|application/demo|demo.lifecycle,demo.foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transitions := "demo.lifecycle|resource_state_machine|application/demo|application/demo/lifecycle_internal_event.go|application/demo/lifecycle_internal_event.go|submitLifecycleInternalEvent|lifecycleInternalEvent|reduceLifecycleInternalEvent\n" +
		"demo.foreign|owner_private_transaction|internal/other|internal/other/foreign_internal_event.go|internal/other/foreign_internal_event.go|submitForeignInternalEvent|foreignInternalEvent|reduceForeignInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transitions), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "transition demo.foreign is owned by internal/other") {
		t.Fatalf("verify error = %v, want cross-owner coordination rejection", err)
	}
}

func TestUniqueOwnerPrivateTransitionSinkHasOneDispatcherCall(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	ownerRoot := filepath.Join(root, "internal/modules/blocks/run")
	if err := os.MkdirAll(ownerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dispatcherPath := filepath.Join(ownerRoot, "workspace_retirement_internal_event.go")
	if err := os.WriteFile(dispatcherPath, []byte(`package run
type owner struct{ workspacePurger purger }
type purger struct{}
func (purger) PurgeWorkspaceResources() {}
func (m *owner) submitWorkspaceRetirementInternalEvent() {
	m.workspacePurger.PurgeWorkspaceResources()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	transition := transitionOwner{
		DispatcherFile:   filepath.ToSlash(dispatcherPath),
		DispatcherSymbol: "submitWorkspaceRetirementInternalEvent",
		TransitionSinks:  []string{"m.workspacePurger.PurgeWorkspaceResources"},
	}
	if err := verifyUniqueOwnerPrivateTransitionSinkCalls(root, ownerRoot, dispatcherPath, transition, []string{"PurgeWorkspaceResources"}); err != nil {
		t.Fatalf("single owner-private sink call must pass: %v", err)
	}
	transitions := map[string]transitionOwner{"run.workspace_retirement": transition}
	if err := verifyGlobalPrivilegedSinkReferences(root, transitions); err != nil {
		t.Fatalf("single declared global sink reference must pass: %v", err)
	}
	externalBypassPath := filepath.Join(root, "application/bypass.go")
	if err := os.WriteFile(externalBypassPath, []byte(`package application
func bypassRunOwner(m *owner) {
	purge := m.workspacePurger.PurgeWorkspaceResources
	purge()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyGlobalPrivilegedSinkReferences(root, transitions); err == nil || !strings.Contains(err.Error(), "application/bypass.go") || !strings.Contains(err.Error(), "PurgeWorkspaceResources") {
		t.Fatalf("global privileged sink bypass error = %v, want external helper rejection", err)
	}

	bypassPath := filepath.Join(ownerRoot, "bypass.go")
	if err := os.WriteFile(bypassPath, []byte(`package run
func bypassWorkspaceRetirement(m *owner) {
	purge := m.workspacePurger.PurgeWorkspaceResources
	purge()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verifyUniqueOwnerPrivateTransitionSinkCalls(root, ownerRoot, dispatcherPath, transition, []string{"PurgeWorkspaceResources"})
	if err == nil || !strings.Contains(err.Error(), "2 production references") || !strings.Contains(err.Error(), "bypassWorkspaceRetirement") {
		t.Fatalf("duplicate privileged sink error = %v, want exact secondary-call rejection", err)
	}
}

func TestCanonicalVerificationRequiresEveryPrivilegedTransition(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("minimal fixture verification must remain available to focused tests: %v", err)
	}
	err := verifyCanonicalRepository(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "omits required privileged transitions") || !strings.Contains(err.Error(), "workspace.deletion") || !strings.Contains(err.Error(), "eventstream.workspace_deletion") {
		t.Fatalf("canonical privileged transition inventory error = %v, want exhaustive omission rejection", err)
	}
}

func TestRepositoryProviderAccessIsRestrictedToInitiatorAndWorkorchdeps(t *testing.T) {
	root := newEmptyVerificationRepository(t)
	allowedPath := filepath.Join(root, "internal/pkg/workorchdeps/repositories.go")
	if err := os.MkdirAll(filepath.Dir(allowedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allowedPath, []byte("package workorchdeps\nfunc resolve(runtime interface{ RepositoryProvider() any }) { _ = runtime.RepositoryProvider() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyRepositoryProviderAccess(root); err != nil {
		t.Fatalf("workorchdeps provider resolution must pass: %v", err)
	}

	bypassPath := filepath.Join(root, "application/demo/repositories.go")
	if err := os.MkdirAll(filepath.Dir(bypassPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bypassPath, []byte("package demo\nfunc bypass(runtime interface{ RepositoryProvider() any }) { provider := runtime.RepositoryProvider; _ = provider }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verifyRepositoryProviderAccess(root)
	if err == nil || !strings.Contains(err.Error(), "application/demo/repositories.go") || !strings.Contains(err.Error(), "RepositoryProvider") {
		t.Fatalf("repository provider bypass error = %v, want production caller rejection", err)
	}
}

func newEmptyVerificationRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"application", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/muidea/workorch\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeBareOwnerModelFixture(t *testing.T) string {
	t.Helper()
	return writeBareOwnerModelFixtureWithField(t, "Status string")
}

func writeBareOwnerModelFixtureWithField(t *testing.T, field string) string {
	t.Helper()
	root := newEmptyVerificationRepository(t)
	path := filepath.Join(root, "internal/modules/blocks/demo/pkg/models/model.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package models\ntype Snapshot struct { "+field+" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writePrivateDispatcherFixture(t *testing.T, source, internalEventType, sinks string) string {
	t.Helper()
	root := newEmptyVerificationRepository(t)
	path := filepath.Join(root, "application/demo/internal_event.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	transition := "demo.private|owner_private_transaction|application/demo|application/demo/internal_event.go|application/demo/internal_event.go|submitLifecycleInternalEvent|" + internalEventType + "|" + sinks + "\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transition), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
