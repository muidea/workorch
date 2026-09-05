package orchestrator

// File-backed MCP owner grounding recovery tests.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	workorcheventpayload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeMCPRoundTripper preserves the MCP adapter's real HTTP request /
// response path without opening a loopback listener. The state-sequence test
// is consequently runnable in constrained CI sandboxes as well as on a
// deployed host; it does not mock the MCP owner or EventHub receipt contract.
type runtimeMCPRoundTripper func(*http.Request) (*http.Response, error)

func (f runtimeMCPRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// TestRuntimeFileBackedMCPOwnerFailureTargetReceiptRestartMatrix keeps the
// actual MCP HTTP observation, the Run owner's durable aggregate and the File
// snapshot prerequisite in one recovery sequence.  A validation entry is only
// operation lineage; only the MCP response's structured failureTargets field
// can become implementation repair authority.
//
// The two subtests exercise the receipt boundary on both sides of the Run CAS:
// a pre-commit interruption leaves no target/evidence fact and hands the exact
// safe-to-replay execution to SM-4, whereas a durable receipt with a lost
// response is reused without another MCP call. Neither recovery path may
// manufacture a target from the validation entry.
func TestRuntimeFileBackedMCPOwnerFailureTargetReceiptRestartMatrix(t *testing.T) {
	patchVariants := []struct {
		name      string
		want      string
		candidate func(target string) runtimecontract.RuntimeCandidate
	}{
		{
			name: "file_write", want: "package repaired\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType, content := "file_write", "package repaired\n"
				return runtimePatchCandidate("mcp-grounded-patch", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}})
			},
		},
		{
			name: "text_replace", want: "package repaired\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType, oldText, newText := "text_replace", "package fixture\n", "package repaired\n"
				return runtimePatchCandidate("mcp-grounded-patch", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", want: "package repaired\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType := "unified_diff"
				diff := "--- a/implementation.go\n+++ b/implementation.go\n@@ -1 +1 @@\n-package fixture\n+package repaired\n"
				return runtimePatchCandidate("mcp-grounded-patch", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, variant := range patchVariants {
		for _, tc := range []struct {
			name                 string
			crashAfterReceipt    bool
			wantMCPCalls         int
			wantRecoveryAttempts int
		}{
			{name: "before_receipt_cas", wantMCPCalls: 2, wantRecoveryAttempts: 1},
			{name: "after_receipt_cas_response_loss", crashAfterReceipt: true, wantMCPCalls: 1},
		} {
			t.Run(variant.name+"/"+tc.name, func(t *testing.T) {
				ctx := context.Background()
				calls := 0
				client := &http.Client{Transport: runtimeMCPRoundTripper(func(request *http.Request) (*http.Response, error) {
					calls++
					if request.Method != http.MethodPost || request.URL.String() != "http://mcp-grounding.test/runtime" {
						return nil, errors.New("unexpected MCP HTTP request")
					}
					payload, encodeErr := json.Marshal(map[string]any{
						"content":        []map[string]any{{"type": "text", "text": "implementation failed"}},
						"failureTargets": []string{"implementation.go"},
						"isError":        true,
					})
					if encodeErr != nil {
						return nil, encodeErr
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload)), Request: request}, nil
				})}

				hub := magicevent.NewHub(32)
				t.Cleanup(func() { hub.Terminate(ctx) })
				root := t.TempDir()
				deliverable, internalDirectory := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
				if err := os.MkdirAll(deliverable, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(internalDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(deliverable, "implementation.go"), []byte("package fixture\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(deliverable, "validation-entry.go"), []byte("package fixture\n"), 0o644); err != nil {
					t.Fatal(err)
				}

				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = fileOwner.Stop(ctx) })

				mcpServer := &mcpmodel.Server{
					ID: "mcp-grounding", Name: "grounding", Transport: "http", Endpoint: "http://mcp-grounding.test/runtime",
					Enabled: true, AllowedTools: []string{"lookup.references"}, TimeoutMillis: 1000,
				}
				adapter := mcpbiz.NewRuntimeAdapterWithHTTPClient(runtimeFileBackedMCPServerGetter{server: mcpServer}, client)
				mcpOwner, err := mcpbiz.NewWithDependencies(mcpbiz.Dependencies{EventHub: hub, RuntimeAdapter: adapter})
				if err != nil {
					t.Fatal(err)
				}
				if err := mcpOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { mcpOwner.Teardown(ctx) })

				inventory, observation := acquireFileBackedMCPAdmissibility(t, ctx, hub, deliverable, "validation-entry.go")
				fileInventory, fileObservation := acquireFileBackedFileAdmissibility(t, ctx, hub, deliverable)
				fileManifest := acquireFileBackedManifest(t, ctx, hub, deliverable)
				aggregate := runtimeFileBackedMCPFailureAggregate(t, inventory, observation, fileInventory, fileObservation, fileManifest)
				assertRuntimeFileBackedMCPTypedProofCatalog(t, aggregate, inventory.SnapshotID)
				runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
				run := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
				bootstrap := runtimeBootstrap()
				bootstrap.Definition = aggregate.Definition
				bootstrap.Activation = aggregate.Facts.Activation
				run.RuntimeBootstrap = &bootstrap
				record := runtimeStateRecordFromAggregate(aggregate)
				run.RuntimeState = &record
				if err := runs.Create(ctx, run); err != nil {
					t.Fatal(err)
				}
				if persistedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || persistedRun.RuntimeState == nil || persistedRun.RuntimeState.StateRevision != aggregate.StateRevision {
					t.Fatalf("file-backed initial runtime state = %+v err=%v", persistedRun, err)
				}
				runOwner := runmodule.New()
				if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = runOwner.Stop(ctx) })
				repository := runtimeStateRepository{hub: hub}

				controller, err := runtime.NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				grounded := controllerMustLoadRuntimeAggregate(t, controller)
				if grounded.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
					var prerequisite runtimecontract.RuntimePrerequisite
					for _, pending := range grounded.Facts.Prerequisites.Pending {
						if pending.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && pending.SubjectRef == "validation-entry.go" {
							prerequisite = pending
							break
						}
					}
					if prerequisite.ID == "" {
						t.Fatalf("MCP validation focus has no target snapshot prerequisite: %+v", grounded.Facts.Prerequisites.Pending)
					}
					snapshot := acquireFileBackedMCPTargetSnapshot(t, ctx, hub, grounded, prerequisite, deliverable)
					grounded, err = runtimeTestControllerCommitPrerequisite(controller, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
						PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot.Snapshot, Observation: &snapshot.Observation,
					})
					if err != nil || grounded.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
						t.Fatalf("commit MCP validation target snapshot=%+v err=%v", grounded, err)
					}
				}
				if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "mcp-owner-grounding-prefix"); err != nil {
					t.Fatal(err)
				}
				candidate := runtimeFileBackedMCPFailureCandidate()
				if _, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil {
					t.Fatal(err)
				}
				proposalAggregate := controllerMustLoadRuntimeAggregate(t, controller)
				manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
					proposalAggregate.Decision,
					runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(proposalAggregate.Facts.Resources),
				)
				if err != nil {
					t.Fatal(err)
				}
				candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
				candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
				if err != nil {
					t.Fatal(err)
				}
				probeResult := hub.Send(magicevent.NewEventWithContext(
					mcpevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleMCP,
					magicevent.NewHeader(), ctx, mcpevents.ProbeRuntimeOperationFeasibilityCommand{
						WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: proposalAggregate.Decision.AuthorityTransaction(),
						WorkspaceDeliverableDirectory: deliverable, Operation: candidate.Validation.Operations[0].Operation, Admissibility: inventory,
						CapabilityStateBinding: inventory.AcquisitionBinding, Observation: &observation,
						CandidateManifestHash: runtimePendingCandidateManifestHashForBizTest(t, ctx, controller, "ws-v2", "run-v2", 0), OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, candidate.Validation.Operations[0].Operation.Kind),
					},
				))
				if probeResult == nil {
					t.Fatal("MCP owner returned no feasibility result")
				}
				probeData, probeErr := probeResult.Get()
				if probeErr != nil {
					t.Fatal(probeErr)
				}
				probe, ok := probeData.(mcpevents.ProbeRuntimeOperationFeasibilityResult)
				if !ok || probe.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || probe.Stale != nil || probe.Feedback != nil {
					t.Fatalf("MCP feasibility response = %#v", probeData)
				}
				if _, err := runtimeTestControllerRecordCandidateFeasibility(controller, ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
					OperationID: candidate.Validation.Operations[0].Operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID,
					AcquisitionBinding: inventory.AcquisitionBinding, CandidateManifestHash: probe.CandidateManifestHash,
					Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}, ReplayProof: probe.ReplayProof,
				}); err != nil {
					t.Fatal(err)
				}
				sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
				if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
					t.Fatalf("MCP validation seal=%+v err=%v", sealed, err)
				}

				// MCP owns the receipt query/commit protocol.  The handler is kept
				// behind a swappable controller pointer to model a reconstructed
				// Orchestrator process without replacing any capability owner.
				receiptController := controller
				receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
				receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
					command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
					receipt, found, queryErr := receiptController.QueryOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, event.Source(), command.GovernanceGrant)
					if queryErr != nil {
						result.Set(nil, runtimeOperationMagicFailure(queryErr))
						return
					}
					result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found, Receipt: func() *runtimecontract.RuntimeOperationReceipt {
						if !found {
							return nil
						}
						return &receipt
					}()}, nil)
				})
				receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
					command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
					next, receipt, commitErr := runtimeTestControllerCommitOperationReceipt(receiptController, event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Fence, event.Source(), command.Receipt)
					if commitErr != nil {
						result.Set(nil, runtimeOperationMagicFailure(commitErr))
						return
					}
					result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: next.StateRevision}, nil)
				})
				t.Cleanup(func() {
					receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
					receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
				})

				var receiptCASFailure *failWhenRuntimeStateRepository
				if !tc.crashAfterReceipt {
					receiptCASFailure = &failWhenRuntimeStateRepository{
						RuntimeStateRepository: repository, err: errors.New("injected MCP receipt CAS failure"),
						match: func(next runtime.RuntimeAggregate) bool { return len(next.OperationReceipts) == 1 },
					}
					failing, newErr := runtime.NewRuntimeController(receiptCASFailure)
					if newErr != nil {
						t.Fatal(newErr)
					}
					receiptController = failing
				}
				if tc.crashAfterReceipt {
					func() {
						defer func() {
							if recovered := recover(); recovered != "runtime-test-crash-after-owner-receipt" {
								t.Fatalf("unexpected post-receipt crash marker: %v", recovered)
							}
						}()
						_, _ = runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeOperationExecutor{hub: hub}})
						t.Fatal("expected crash after durable MCP receipt")
					}()
				} else {
					// Execute exactly one SM-3 step. The production coordinator must not
					// consume SM-4 recovery budget inside the receipt observation event;
					// a fresh owner-private event after restart owns that transition.
					executionResult, executionErr := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
						Kind:        runtime.RuntimeInternalEventDispatchExecutionRequested,
						Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
						Transaction: &runtime.RuntimeInternalTransactionPayload{},
					})
					if executionErr != nil || executionResult.OperationCall == nil {
						t.Fatalf("prepare MCP owner call: handoff=%+v err=%v", executionResult.OperationCall, executionErr)
					}
					_, _ = (eventRuntimeOperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, executionResult.OperationCall.Request)
					executionResult, executionErr = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
						Kind: runtime.RuntimeInternalEventDispatchExecutionRequested, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
						Transaction: &runtime.RuntimeInternalTransactionPayload{},
					})
					operationID, recoveryRequired := runtime.RuntimeOperationRecoveryBudgetRequired(executionErr)
					if !recoveryRequired || operationID != candidate.Validation.Operations[0].Operation.ID ||
						executionResult.Aggregate.Facts.Recovery.OperationRecoveryAttempts != 0 || !receiptCASFailure.failed || calls != 1 {
						t.Fatalf("pre-receipt CAS interruption did not hand off exact recovery: operation=%q required=%v attempts=%d failedCAS=%v calls=%d err=%v",
							operationID, recoveryRequired, executionResult.Aggregate.Facts.Recovery.OperationRecoveryAttempts, receiptCASFailure.failed, calls, executionErr)
					}
				}

				beforeRestart, err := repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
				if beforeRestart.Facts.Recovery.OperationRecoveryAttempts != 0 || beforeRestart.ActiveDispatch == nil || len(beforeRestart.ActiveDispatch.OperationExecutions) != 1 {
					t.Fatalf("receipt boundary crossed SM-4 or dispatch scope before restart: %+v", beforeRestart)
				}
				beforeRestartExecution := beforeRestart.ActiveDispatch.OperationExecutions[0]
				if tc.crashAfterReceipt {
					if len(beforeRestart.OperationReceipts) != 1 || beforeRestartExecution.State != runtimecontract.OperationExecutionOwnerCommitted ||
						beforeRestart.Facts.Resources.FailureTarget != nil || len(beforeRestart.Facts.Evidence.Rejected) != 0 {
						t.Fatalf("durable receipt response loss persisted wrong boundary: %+v", beforeRestart)
					}
				} else if len(beforeRestart.OperationReceipts) != 0 || beforeRestart.Facts.Failure != nil || beforeRestart.Facts.Resources.FailureTarget != nil || len(beforeRestart.Facts.Evidence.Rejected) != 0 ||
					beforeRestartExecution.State != runtimecontract.OperationExecutionClaimed || beforeRestartExecution.OperationID != candidate.Validation.Operations[0].Operation.ID {
					t.Fatalf("pre-CAS interruption leaked MCP failure authority: %+v", beforeRestart)
				}

				restarted, err := runtime.NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				receiptController = restarted
				_, restartErr := runtimeTestControllerExecuteDispatch(restarted, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: hub})
				if restartErr != nil {
					t.Fatalf("restart MCP execution: %v", restartErr)
				}
				persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
				if calls != tc.wantMCPCalls || persisted.Facts.Recovery.OperationRecoveryAttempts != tc.wantRecoveryAttempts ||
					len(persisted.OperationReceipts) != 1 || len(persisted.Facts.Evidence.Rejected) != 1 ||
					persisted.Facts.Resources.FailureTarget == nil || persisted.Facts.Resources.FailureTarget.TargetObservation == nil ||
					len(persisted.Facts.Resources.FailureTarget.TargetRefs) != 1 || persisted.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" ||
					persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || persisted.ActiveDispatch != nil ||
					runtimeFileBackedHasTargetSnapshot(persisted.Facts.Resources.TargetSnapshots, "implementation.go") {
					t.Fatalf("restart did not bind the exact owner observation once: calls=%d aggregate=%+v", calls, persisted)
				}
				// Reconstructed duplicate delivery must query the committed MCP owner
				// receipt by its sealed execution key/fence. It cannot reissue the
				// remote call, recreate FailureTarget authority, or consume another
				// aggregate revision before File grounding starts.
				sealedDispatchID := sealed.Aggregate.ActiveDispatch.ID
				var sealedValidation runtime.SealedDispatch
				for _, dispatch := range persisted.CompletedDispatches {
					if dispatch.ID == sealedDispatchID {
						sealedValidation = dispatch
						break
					}
				}
				if sealedValidation.ID == "" || sealedValidation.Candidate.Validation == nil || len(sealedValidation.Candidate.Validation.Operations) != 1 || len(sealedValidation.OperationExecutions) != 1 {
					t.Fatalf("persisted MCP validation dispatch is unavailable for duplicate delivery: %+v", persisted.CompletedDispatches)
				}
				beforeDuplicateRevision := persisted.StateRevision
				ownerCatalog, _ := runtimeFileBackedOperationAdmissibility(t, persisted, runtimecontract.RuntimeModuleMCP, runtimecontract.StepKindMCPToolCall)
				duplicate, duplicateErr := (eventRuntimeOperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, runtime.RuntimeOperationExecutionRequest{
					WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealedValidation.ID, Transaction: sealedValidation.Transaction,
					Operation: sealedValidation.Candidate.Validation.Operations[0].Operation, Execution: sealedValidation.OperationExecutions[0],
					EvidenceKind: runtimecontract.EvidenceKindValidation, TargetRole: persisted.Facts.Focus.TargetRole, OwnerCatalog: &ownerCatalog,
				})
				if duplicateErr != nil || duplicate.Receipt.ExecutionKey != persisted.OperationReceipts[0].ExecutionKey || duplicate.Receipt.Fence != persisted.OperationReceipts[0].Fence {
					t.Fatalf("duplicate MCP delivery did not reuse owner receipt result=%+v err=%v", duplicate, duplicateErr)
				}
				if calls != tc.wantMCPCalls {
					t.Fatalf("duplicate MCP delivery replayed remote call: calls=%d want=%d", calls, tc.wantMCPCalls)
				}
				afterDuplicate, loadErr := repository.Load(ctx, "ws-v2", "run-v2")
				if loadErr != nil || afterDuplicate.StateRevision != beforeDuplicateRevision || len(afterDuplicate.OperationReceipts) != 1 || len(afterDuplicate.Facts.Evidence.Rejected) != 1 ||
					afterDuplicate.Facts.Resources.FailureTarget == nil || afterDuplicate.Facts.Resources.FailureTarget.TargetObservation == nil {
					t.Fatalf("duplicate MCP delivery mutated persisted authority aggregate=%+v err=%v", afterDuplicate, loadErr)
				}

				prerequisite := runtimeFileBackedMCPFailureTargetSnapshotPrerequisite(t, persisted)
				snapshot := acquireFileBackedMCPTargetSnapshot(t, ctx, hub, persisted, prerequisite, deliverable)
				persisted, err = runtimeTestControllerCommitPrerequisite(restarted, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot.Snapshot, Observation: &snapshot.Observation,
				})
				if err != nil || !runtimeFileBackedHasCompleteTargetSnapshot(persisted.Facts.Resources.TargetSnapshots, "implementation.go", snapshot.Snapshot.ContentHash) ||
					!runtimeFileBackedHasCompleteTargetSnapshot(persisted.Facts.Resources.TargetSnapshots, "validation-entry.go", "") ||
					!runtimeFileBackedHasPrerequisiteObservation(persisted.Facts.Resources.PrerequisiteObservations, snapshot.Observation) {
					t.Fatalf("File snapshot was not committed from MCP-grounded target: targetSnapshots=%+v observations=%+v err=%v",
						persisted.Facts.Resources.TargetSnapshots, persisted.Facts.Resources.PrerequisiteObservations, err)
				}
				if _, err := runtimeTestControllerCommitPrerequisite(restarted, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot.Snapshot, Observation: &snapshot.Observation,
				}); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("duplicate target snapshot commit = %v, want conflict", err)
				}
				if persisted.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
					t.Fatalf("MCP-grounded File snapshot did not reduce to target-bound state change: state=%s terminal=%+v failure=%+v", persisted.Decision.State, persisted.Decision.Terminal, persisted.Facts.Failure)
				}

				// P3's durable-owner half starts only after the MCP observation and
				// File snapshot are both persisted.  The patch has a replay proof
				// bound to that exact target; a receipt-CAS crash must not allow a
				// later drive to retarget, duplicate the write, or lose the original
				// resource delta.
				target := "implementation.go"
				fileCandidate := variant.candidate(target)
				if variant.name == "file_write" {
					baseHash := snapshot.Snapshot.ContentHash
					fileCandidate.StateChange.Operations[0].Operation.Parameters = append(fileCandidate.StateChange.Operations[0].Operation.Parameters, runtimecontract.OperationParameter{Name: "baseHash", String: &baseHash})
				}
				fileCandidate.StateChange.Operations[0].Operation.CoverageWorkItemIDs = nil
				filePrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(persisted.Definition, persisted.Facts.Activation.ExecutionContract)
				currentFileInventory, currentFileObservation := runtimeFileBackedOperationAdmissibility(t, persisted, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeBlockKindPatchApply)
				candidateRequests := 0
				contextBound := false
				llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
				llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
					candidateRequests++
					command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
					resources := command.CandidateContext.Resources
					if command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange &&
						resources.FailureTarget != nil && resources.FailureTarget.TargetObservation != nil &&
						len(resources.FailureTarget.TargetRefs) == 1 && resources.FailureTarget.TargetRefs[0] == target &&
						runtimeFileBackedHasCompleteTargetSnapshot(resources.TargetSnapshots, target, snapshot.Snapshot.ContentHash) &&
						command.CandidateContext.Focus.TargetRole == persisted.Facts.Focus.TargetRole {
						contextBound = true
					}
					result.Set(runtimeCandidateResultForTest(t, command, fileCandidate), nil)
				})
				t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
				patchOwner := newRuntimeTestOwner()
				if err := patchOwner.ConfigureRuntimeRepository(repository); err != nil {
					t.Fatal(err)
				}
				patchModule := newTestBiz(patchOwner, hub, nil)
				sealedPatch, err := patchModule.requestAndSealRuntimeCandidateWithRetry(ctx, persisted, filePrefix)
				if err != nil || sealedPatch.ActiveDispatch == nil || candidateRequests != 1 || !contextBound {
					t.Fatalf("MCP-grounded File patch seal: active=%t requests=%d contextBound=%t decision=%s err=%v",
						sealedPatch.ActiveDispatch != nil, candidateRequests, contextBound, sealedPatch.Decision.State, err)
				}
				if sealedPatch.ActiveDispatch.Candidate.StateChange == nil || len(sealedPatch.ActiveDispatch.Candidate.StateChange.Operations) != 1 {
					t.Fatalf("MCP-grounded File patch sealed unexpected candidate=%+v", sealedPatch.ActiveDispatch.Candidate)
				}
				sealedPatchOperationID := sealedPatch.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.ID
				failingPatchController, err := runtime.NewRuntimeController(&failWhenRuntimeStateRepository{
					RuntimeStateRepository: repository, err: errors.New("injected grounded File receipt CAS failure"),
					match: func(next runtime.RuntimeAggregate) bool {
						return len(next.OperationReceipts) == 2 && next.ActiveDispatch != nil && next.ActiveDispatch.ID == sealedPatch.ActiveDispatch.ID
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				receiptController = failingPatchController
				func() {
					defer func() {
						if recovered := recover(); recovered != "runtime-test-crash-after-owner-receipt-commit-error" {
							t.Fatalf("unexpected grounded File receipt crash marker: %v", recovered)
						}
					}()
					_, _ = runtimeTestControllerExecuteDispatch(restarted, ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptCommitErrorExecutor{delegate: eventRuntimeOperationExecutor{hub: hub}})
					t.Fatal("expected crash after MCP-grounded File receipt CAS failure")
				}()
				if content, readErr := os.ReadFile(filepath.Join(deliverable, target)); readErr != nil || string(content) != variant.want {
					t.Fatalf("grounded File durable effect before restart=%q err=%v", content, readErr)
				}
				crashedPatch, err := repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil || crashedPatch.ActiveDispatch == nil || len(crashedPatch.OperationReceipts) != 1 || len(crashedPatch.Facts.Evidence.Accepted) != 0 {
					t.Fatalf("grounded File receipt-CAS crash persisted wrong aggregate=%+v err=%v", crashedPatch, err)
				}
				patchExecution, found := runtimeDriveOperationExecutionRecord(crashedPatch.ActiveDispatch.OperationExecutions, sealedPatchOperationID)
				if !found || patchExecution.ReplayProof == nil || patchExecution.ExecutionKey == "" || patchExecution.Fence == 0 {
					t.Fatalf("grounded File crash lost sealed replay proof=%+v", patchExecution)
				}
				recoveredPatch, err := runtime.NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				receiptController = recoveredPatch
				completedPatch, err := runtimeTestControllerExecuteDispatch(recoveredPatch, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: hub})
				if err != nil || completedPatch.ActiveDispatch != nil || len(completedPatch.OperationReceipts) != 2 || len(completedPatch.Facts.Evidence.Accepted) != 1 ||
					completedPatch.Facts.Resources.FailureTarget == nil || completedPatch.Facts.Resources.FailureTarget.TargetRefs[0] != target || calls != tc.wantMCPCalls {
					t.Fatalf("grounded File receipt restart did not converge exactly once aggregate=%+v calls=%d err=%v", completedPatch, calls, err)
				}
				reloaded, err := repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil || runtimeFileBackedHasTargetSnapshot(reloaded.Facts.Resources.TargetSnapshots, "implementation.go") ||
					!runtimeFileBackedHasTargetSnapshot(reloaded.Facts.Resources.TargetSnapshots, "validation-entry.go") || len(reloaded.OperationReceipts) != 2 ||
					reloaded.Facts.Resources.FailureTarget == nil || reloaded.Facts.Resources.FailureTarget.TargetObservation == nil ||
					reloaded.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
					!runtimeFileBackedHasPendingPrerequisite(reloaded.Facts.Prerequisites.Pending, runtimecontract.PrerequisiteKindTargetSnapshot, "implementation.go") ||
					!runtimeFileBackedHasPendingPrerequisite(reloaded.Facts.Prerequisites.Pending, runtimecontract.PrerequisiteKindOperationAdmissibility, "") {
					t.Fatalf("resource-change reconstruction did not retain target authority and invalidate its stale File facts: aggregate=%+v err=%v", reloaded, err)
				}
				if _, err := runtimeTestControllerCommitPrerequisite(recoveredPatch, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot.Snapshot, Observation: &snapshot.Observation,
				}); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("stale pre-change target snapshot commit = %v, want conflict", err)
				}
				if _, err := runtimeTestControllerCommitPrerequisite(recoveredPatch, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: currentFileInventory.PrerequisiteID, OperationAdmissibility: &currentFileInventory, Observation: &currentFileObservation,
				}); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("stale pre-change File admissibility commit = %v, want conflict", err)
				}
				unchanged, err := repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil || unchanged.StateRevision != reloaded.StateRevision || runtimeFileBackedHasTargetSnapshot(unchanged.Facts.Resources.TargetSnapshots, "implementation.go") ||
					!runtimeFileBackedHasTargetSnapshot(unchanged.Facts.Resources.TargetSnapshots, "validation-entry.go") ||
					unchanged.Facts.Resources.FailureTarget == nil || len(unchanged.OperationReceipts) != 2 {
					t.Fatalf("stale target snapshot changed resource-invalidated authority: aggregate=%+v err=%v", unchanged, err)
				}
				freshPrerequisite := runtimeFileBackedMCPFailureTargetSnapshotPrerequisite(t, unchanged)
				freshSnapshot := acquireFileBackedMCPTargetSnapshot(t, ctx, hub, unchanged, freshPrerequisite, deliverable)
				if freshSnapshot.Snapshot.ContentHash == snapshot.Snapshot.ContentHash || freshSnapshot.Observation.ObservationID == "" ||
					freshSnapshot.Observation.ObservationID == snapshot.Observation.ObservationID {
					t.Fatalf("reacquired target snapshot did not advance owner freshness: old=%+v fresh=%+v", snapshot, freshSnapshot)
				}
				// A fresh owner response must not become visible before its Run CAS
				// commits.  Reconstructing the controller after this failure proves
				// that the same generation can be committed exactly once without
				// reviving the rejected pre-change response.  The matching
				// post-commit response-loss case below proves the inverse boundary:
				// restart consumes the new snapshot as committed authority and does
				// not try to re-commit the same owner response.
				failingFreshSnapshot, err := runtime.NewRuntimeController(&failWhenRuntimeStateRepository{
					RuntimeStateRepository: repository, err: errors.New("injected fresh File snapshot CAS failure"),
					match: func(next runtime.RuntimeAggregate) bool {
						return runtimeFileBackedHasCompleteTargetSnapshot(next.Facts.Resources.TargetSnapshots, "implementation.go", freshSnapshot.Snapshot.ContentHash)
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := runtimeTestControllerCommitPrerequisite(failingFreshSnapshot, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: freshPrerequisite.ID, TargetSnapshot: &freshSnapshot.Snapshot, Observation: &freshSnapshot.Observation,
				}); err == nil {
					t.Fatal("fresh target snapshot CAS failure was not surfaced")
				}
				notCommitted, err := repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil || runtimeFileBackedHasTargetSnapshot(notCommitted.Facts.Resources.TargetSnapshots, "implementation.go") ||
					!runtimeFileBackedHasTargetSnapshot(notCommitted.Facts.Resources.TargetSnapshots, "validation-entry.go") || notCommitted.StateRevision != unchanged.StateRevision {
					t.Fatalf("fresh target snapshot leaked across failed CAS aggregate=%+v err=%v", notCommitted, err)
				}
				responseLostSnapshotRepository := &panicAfterRuntimeStateSaveRepository{
					RuntimeStateRepository: repository,
					marker:                 "fresh-file-snapshot-response-loss",
					match: func(next runtime.RuntimeAggregate) bool {
						return runtimeFileBackedHasCompleteTargetSnapshot(next.Facts.Resources.TargetSnapshots, "implementation.go", freshSnapshot.Snapshot.ContentHash)
					},
				}
				responseLostSnapshot, err := runtime.NewRuntimeController(responseLostSnapshotRepository)
				if err != nil {
					t.Fatal(err)
				}
				func() {
					defer func() {
						if recovered := recover(); recovered != "fresh-file-snapshot-response-loss" {
							t.Fatalf("unexpected fresh File snapshot response-loss marker: %v", recovered)
						}
					}()
					_, _ = runtimeTestControllerCommitPrerequisite(responseLostSnapshot, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
						PrerequisiteID: freshPrerequisite.ID, TargetSnapshot: &freshSnapshot.Snapshot, Observation: &freshSnapshot.Observation,
					})
					t.Fatal("expected fresh File snapshot response loss after durable CAS")
				}()
				if !responseLostSnapshotRepository.fired {
					t.Fatal("fresh File snapshot response-loss CAS point was not reached")
				}
				recoveredPatch, err = runtime.NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				reacquired, err := recoveredPatch.Load(ctx, "ws-v2", "run-v2")
				if err != nil || !runtimeFileBackedHasCompleteTargetSnapshot(reacquired.Facts.Resources.TargetSnapshots, "implementation.go", freshSnapshot.Snapshot.ContentHash) ||
					!runtimeFileBackedHasTargetSnapshot(reacquired.Facts.Resources.TargetSnapshots, "validation-entry.go") ||
					!runtimeFileBackedHasPendingPrerequisite(reacquired.Facts.Prerequisites.Pending, runtimecontract.PrerequisiteKindOperationAdmissibility, "") {
					t.Fatalf("fresh target snapshot response-loss restart did not retain committed authority: aggregate=%+v err=%v", reacquired, err)
				}
				if _, err := runtimeTestControllerCommitPrerequisite(recoveredPatch, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: freshPrerequisite.ID, TargetSnapshot: &freshSnapshot.Snapshot, Observation: &freshSnapshot.Observation,
				}); err == nil {
					t.Fatal("duplicate fresh target snapshot after response loss unexpectedly committed")
				}
				afterDuplicateSnapshot, err := repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil || afterDuplicateSnapshot.StateRevision != reacquired.StateRevision ||
					!runtimeFileBackedHasCompleteTargetSnapshot(afterDuplicateSnapshot.Facts.Resources.TargetSnapshots, "implementation.go", freshSnapshot.Snapshot.ContentHash) ||
					!runtimeFileBackedHasTargetSnapshot(afterDuplicateSnapshot.Facts.Resources.TargetSnapshots, "validation-entry.go") {
					t.Fatalf("duplicate fresh target snapshot after response loss mutated authority: aggregate=%+v err=%v", afterDuplicateSnapshot, err)
				}
				freshFileInventory, freshFileObservation := acquireFileBackedFileAdmissibility(t, ctx, hub, deliverable)
				if freshFileInventory.ObservedRevision == currentFileInventory.ObservedRevision ||
					freshFileObservation.ObservationID == currentFileObservation.ObservationID {
					t.Fatalf("reacquired File admissibility did not advance owner freshness: old=%+v fresh=%+v", currentFileInventory, freshFileInventory)
				}
				failingFreshInventory, err := runtime.NewRuntimeController(&failWhenRuntimeStateRepository{
					RuntimeStateRepository: repository, err: errors.New("injected fresh File admissibility CAS failure"),
					match: func(next runtime.RuntimeAggregate) bool {
						return runtimeFileBackedHasOperationAdmissibility(next.Facts.Resources.OperationAdmissibilities, freshFileInventory.SnapshotID)
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := runtimeTestControllerCommitPrerequisite(failingFreshInventory, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: freshFileInventory.PrerequisiteID, OperationAdmissibility: &freshFileInventory, Observation: &freshFileObservation,
				}); err == nil {
					t.Fatal("fresh File admissibility CAS failure was not surfaced")
				}
				notCommitted, err = repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil || runtimeFileBackedHasOperationAdmissibility(notCommitted.Facts.Resources.OperationAdmissibilities, freshFileInventory.SnapshotID) ||
					notCommitted.StateRevision != reacquired.StateRevision {
					t.Fatalf("fresh File admissibility leaked across failed CAS aggregate=%+v err=%v", notCommitted, err)
				}
				responseLostInventoryRepository := &panicAfterRuntimeStateSaveRepository{
					RuntimeStateRepository: repository,
					marker:                 "fresh-file-admissibility-response-loss",
					match: func(next runtime.RuntimeAggregate) bool {
						return runtimeFileBackedHasOperationAdmissibility(next.Facts.Resources.OperationAdmissibilities, freshFileInventory.SnapshotID)
					},
				}
				responseLostInventory, err := runtime.NewRuntimeController(responseLostInventoryRepository)
				if err != nil {
					t.Fatal(err)
				}
				func() {
					defer func() {
						if recovered := recover(); recovered != "fresh-file-admissibility-response-loss" {
							t.Fatalf("unexpected fresh File admissibility response-loss marker: %v", recovered)
						}
					}()
					_, _ = runtimeTestControllerCommitPrerequisite(responseLostInventory, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
						PrerequisiteID: freshFileInventory.PrerequisiteID, OperationAdmissibility: &freshFileInventory, Observation: &freshFileObservation,
					})
					t.Fatal("expected fresh File admissibility response loss after durable CAS")
				}()
				if !responseLostInventoryRepository.fired {
					t.Fatal("fresh File admissibility response-loss CAS point was not reached")
				}
				recoveredPatch, err = runtime.NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				reacquired, err = recoveredPatch.Load(ctx, "ws-v2", "run-v2")
				if err != nil || len(reacquired.Facts.Resources.OperationAdmissibilities) != 2 ||
					!runtimeFileBackedHasOperationAdmissibility(reacquired.Facts.Resources.OperationAdmissibilities, freshFileInventory.SnapshotID) {
					t.Fatalf("fresh File admissibility response-loss restart did not retain committed authority: inventory=%+v observation=%+v aggregate=%+v err=%v", freshFileInventory, freshFileObservation, reacquired, err)
				}
				if _, err := runtimeTestControllerCommitPrerequisite(recoveredPatch, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
					PrerequisiteID: freshFileInventory.PrerequisiteID, OperationAdmissibility: &freshFileInventory, Observation: &freshFileObservation,
				}); err == nil {
					t.Fatal("duplicate fresh File admissibility after response loss unexpectedly committed")
				}
				afterDuplicateInventory, err := repository.Load(ctx, "ws-v2", "run-v2")
				if err != nil || afterDuplicateInventory.StateRevision != reacquired.StateRevision ||
					len(afterDuplicateInventory.Facts.Resources.OperationAdmissibilities) != 2 ||
					!runtimeFileBackedHasOperationAdmissibility(afterDuplicateInventory.Facts.Resources.OperationAdmissibilities, freshFileInventory.SnapshotID) {
					t.Fatalf("duplicate fresh File admissibility after response loss mutated authority: aggregate=%+v err=%v", afterDuplicateInventory, err)
				}
			})
		}
	}
}

func assertRuntimeFileBackedMCPTypedProofCatalog(t *testing.T, aggregate runtime.RuntimeAggregate, snapshotID string) {
	t.Helper()
	if aggregate.SemanticPlanning.Manifest == nil {
		t.Fatal("MCP restart fixture has no sealed semantic manifest")
	}
	assertCatalogs := func(scope string, catalogs []runtimecontract.RuntimeOperationAdmissibility) {
		t.Helper()
		found := false
		for _, raw := range catalogs {
			catalog := raw.Normalized()
			if catalog.OwnerModuleID == runtimecontract.RuntimeModuleShell {
				t.Fatalf("%s installed MCP proof as Shell inventory: %+v", scope, catalog)
			}
			if catalog.OwnerModuleID != runtimecontract.RuntimeModuleMCP {
				continue
			}
			if found || catalog.SnapshotID != snapshotID || len(catalog.ValidationRecipes) != 0 {
				t.Fatalf("%s MCP catalog identity drifted: %+v", scope, catalog)
			}
			found = true
			surface, ok := runtimecontract.RuntimeValidationSurfaceForID(
				catalog.ValidationSurfaces,
				runtimecontract.RuntimeMCPToolValidationRecipeID("mcp-grounding", "lookup.references"),
			)
			if !ok || surface.OwnerModuleID != runtimecontract.RuntimeModuleMCP ||
				surface.OperationKind != runtimecontract.StepKindMCPToolCall ||
				surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect ||
				len(surface.ProofRequirementBindings) != 0 || len(surface.ProofRequirementPreparations) != 0 {
				t.Fatalf("%s lost MCP-owned typed proof surface: %+v", scope, catalog.ValidationSurfaces)
			}
		}
		if !found {
			t.Fatalf("%s has no MCP owner catalog: %+v", scope, catalogs)
		}
	}
	assertCatalogs("current facts", aggregate.Facts.Resources.OperationAdmissibilities)
	assertCatalogs("sealed manifest", aggregate.SemanticPlanning.Manifest.OwnerCatalogs)
}

func runtimePendingCandidateManifestHashForBizTest(t *testing.T, ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int) string {
	t.Helper()
	aggregate, err := controller.Load(ctx, workspaceID, runID)
	if err != nil {
		t.Fatal(err)
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	_, record, found := pendingRuntimeCandidateAttemptForTest(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found || record.ManifestHash == "" {
		t.Fatalf("candidate attempt %d has no pending manifest identity", attempt)
	}
	return record.ManifestHash
}

type runtimeFileBackedMCPServerGetter struct{ server *mcpmodel.Server }

func controllerMustLoadRuntimeAggregate(t *testing.T, controller *runtime.RuntimeController) runtime.RuntimeAggregate {
	t.Helper()
	aggregate, err := controller.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}

func runtimeString(value string) *string { return &value }

func runtimeFileBackedOperationAdmissibility(t *testing.T, aggregate runtime.RuntimeAggregate, ownerModuleID, operationKind string) (runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	for _, inventory := range aggregate.Facts.Resources.OperationAdmissibilities {
		if inventory.OwnerModuleID != ownerModuleID {
			continue
		}
		matched := false
		for _, available := range inventory.OperationKinds {
			if runtimecontract.CanonicalRuntimeBlockKind(available) == runtimecontract.CanonicalRuntimeBlockKind(operationKind) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(aggregate.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
		if found {
			return inventory, observation
		}
	}
	t.Fatalf("missing persisted %s admissibility for %s: %+v", ownerModuleID, operationKind, aggregate.Facts.Resources.OperationAdmissibilities)
	return runtimecontract.RuntimeOperationAdmissibility{}, runtimecontract.RuntimePrerequisiteObservation{}
}

func runtimeFileBackedHasPendingPrerequisite(values []runtimecontract.RuntimePrerequisite, kind runtimecontract.PrerequisiteKind, subjectRef string) bool {
	for _, value := range values {
		if value.Kind == kind && value.OwnerModuleID == runtimecontract.RuntimeModuleFile && value.SubjectRef == subjectRef {
			return true
		}
	}
	return false
}

func runtimeFileBackedHasOperationAdmissibility(values []runtimecontract.RuntimeOperationAdmissibility, snapshotID string) bool {
	for _, value := range values {
		if value.OwnerModuleID == runtimecontract.RuntimeModuleFile && value.SnapshotID == snapshotID {
			return true
		}
	}
	return false
}

func runtimeFileBackedHasTargetSnapshot(values []runtimecontract.TargetSnapshot, targetRef string) bool {
	for _, value := range values {
		if value.TargetRef == targetRef {
			return true
		}
	}
	return false
}

func runtimeFileBackedHasCompleteTargetSnapshot(values []runtimecontract.TargetSnapshot, targetRef, contentHash string) bool {
	for _, value := range values {
		if value.TargetRef == targetRef && value.Complete() && (contentHash == "" || value.ContentHash == contentHash) {
			return true
		}
	}
	return false
}

func runtimeFileBackedHasPrerequisiteObservation(values []runtimecontract.RuntimePrerequisiteObservation, expected runtimecontract.RuntimePrerequisiteObservation) bool {
	for _, value := range values {
		if value.PrerequisiteID == expected.PrerequisiteID && value.ObservationID == expected.ObservationID &&
			value.OwnerModuleID == expected.OwnerModuleID && value.ContentHash == expected.ContentHash {
			return true
		}
	}
	return false
}

func (g runtimeFileBackedMCPServerGetter) GetServer(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.server == nil || g.server.ID != id {
		return nil, common.ErrNotFound
	}
	return g.server, nil
}

func (g runtimeFileBackedMCPServerGetter) RegistrySnapshot(context.Context) (*mcpmodel.RegistrySnapshot, string, error) {
	if g.server == nil {
		return nil, "", common.ErrNotFound
	}
	capabilities := []*mcpmodel.Capability{{
		ID: common.MCPCapabilityID(string(g.server.ID) + ":tool:lookup.references"), ServerID: g.server.ID,
		Kind: mcpmodel.CapabilityKindTool, Name: "lookup.references", Enabled: true,
		EffectScope:                  string(runtimecontract.RuntimeEffectNone),
		IdempotencyClass:             string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		ValidationProofApplicability: mcpmodel.ValidationProofApplicabilityDirect,
		Schema: workorcheventpayload.New(
			workorcheventpayload.StringField("type", "object"),
			workorcheventpayload.ObjectField("properties", workorcheventpayload.New(
				workorcheventpayload.ObjectField("path", workorcheventpayload.New(workorcheventpayload.StringField("type", "string"))),
			)),
			workorcheventpayload.BoolField("additionalProperties", false),
		),
	}}
	return &mcpmodel.RegistrySnapshot{Servers: mcpmodel.CloneServers([]*mcpmodel.Server{g.server}), Capabilities: capabilities}, "test-mcp-registry-" + string(g.server.ID), nil
}

func acquireFileBackedMCPAdmissibility(t *testing.T, ctx context.Context, hub magicevent.Hub, deliverable string, targetRefs ...string) (runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleMCP,
		magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "authority-mcp-grounding", PrerequisiteID: "operation-admissibility:mcp-grounding",
			WorkspaceDeliverableDirectory: deliverable, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			RequiredKinds: []string{runtimecontract.StepKindMCPToolCall}, RequiredTargetRefs: append([]string(nil), targetRefs...), MaxItems: 4,
		},
	))
	if result == nil {
		t.Fatal("MCP owner returned no admissibility result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(mcpevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Observation.ObservationID == "" || response.Acquisition.Admissibility.ObservedRevision == "" {
		t.Fatalf("MCP admissibility response = %#v", data)
	}
	// The capability catalog is selector-scoped, while this restart matrix also
	// carries one File-observed validation input. Seal that acquisition-request
	// target into the pre-semantic test catalog and reissue the matching owner
	// observation identity; no target is invented after semantic seal.
	inventory := response.Acquisition.Admissibility.Normalized()
	inventory.TargetRefs = append([]string(nil), targetRefs...)
	inventory = inventory.Normalized()
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		t.Fatalf("MCP target-scoped admissibility fixture: %v", err)
	}
	observation := response.Acquisition.Observation.Normalized()
	observation.ContentHash = inventory.SnapshotID
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatalf("MCP target-scoped admissibility observation: %v", err)
	}
	return inventory, observation
}

func acquireFileBackedManifest(t *testing.T, ctx context.Context, hub magicevent.Hub, deliverable string) fileevents.AcquireWorkspaceManifestResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireWorkspaceManifest, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, fileevents.AcquireWorkspaceManifestCommand{
			WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "authority-file-grounding",
			PrerequisiteID: "manifest:file-grounding", WorkspaceDeliverableDirectory: deliverable,
		},
	))
	if result == nil {
		t.Fatal("File owner returned no workspace manifest")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.AcquireWorkspaceManifestResult)
	if !ok || response.Manifest.Revision == "" || response.Observation.ObservationID == "" {
		t.Fatalf("File workspace manifest response = %#v", data)
	}
	return response
}

func acquireFileBackedFileAdmissibility(t *testing.T, ctx context.Context, hub magicevent.Hub, deliverable string) (runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, fileevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "authority-file-grounding", PrerequisiteID: "operation-admissibility:file-grounding",
			WorkspaceDeliverableDirectory: deliverable, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			RequiredKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, RequiredTargetRefs: []string{"implementation.go", "validation-entry.go"}, MaxItems: 4,
		},
	))
	if result == nil {
		t.Fatal("File owner returned no admissibility result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Observation.ObservationID == "" || response.Acquisition.Admissibility.ObservedRevision == "" {
		t.Fatalf("File admissibility response = %#v", data)
	}
	return response.Acquisition.Admissibility, response.Acquisition.Observation
}

func runtimeFileBackedMCPFailureAggregate(
	t *testing.T,
	inventory runtimecontract.RuntimeOperationAdmissibility,
	observation runtimecontract.RuntimePrerequisiteObservation,
	fileInventory runtimecontract.RuntimeOperationAdmissibility,
	fileObservation runtimecontract.RuntimePrerequisiteObservation,
	fileManifest fileevents.AcquireWorkspaceManifestResult,
) runtime.RuntimeAggregate {
	t.Helper()
	facts := runtimeAggregate(t, 1).Facts
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	// The validation proof is MCP-only, but an owner-grounded rejected
	// validation may require an implementation repair before validation resumes.
	// patch/apply is therefore an admissible repair operation, not a validation
	// proof and receives no CWI binding reference.
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindMCPToolCall, runtimecontract.RuntimeBlockKindPatchApply}
	facts.Focus.TargetRefs = []string{"validation-entry.go"}
	facts.Focus.TargetRole = "implementation"
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.ValidationRecipeID = runtimecontract.RuntimeMCPToolValidationRecipeID("mcp-grounding", "lookup.references")
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		ValidationKinds:  []string{runtimecontract.StepKindMCPToolCall},
	}
	facts.Activation.MCPCapabilities = []runtimecontract.RuntimeMCPCapabilityContext{{
		ID: "mcp-grounding:tool:lookup.references", ServerID: "mcp-grounding", Kind: "tool",
		OperationKind: runtimecontract.StepKindMCPToolCall, Name: "lookup.references", Enabled: true,
		EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
	}}
	// Seal the actual File-owner manifest response as the resource-owner
	// declaration. The Runtime core can then ground both validation and repair
	// targets without inferring File from a path or from the MCP diagnostic.
	facts.Resources.Manifest = &fileManifest.Manifest
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory, fileInventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation, fileObservation, fileManifest.Observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall},
	}, {
		ID: fileInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply},
	}, {
		ID: fileManifest.Observation.PrerequisiteID, Kind: fileManifest.Observation.Kind,
		OwnerModuleID: fileManifest.Observation.OwnerModuleID,
	}}
	// Keep the current-schema CWI unbound here. The shared semantic fixture below
	// compiles and seals the MCP validation binding, updating binding lineage and
	// CWI convergence state in the same canonical projection.
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindMCPToolCall},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimeDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindMCPToolCall, runtimecontract.RuntimeBlockKindPatchApply}
	return runtimeTestAggregateWithSemanticPlanning(t, runtime.RuntimeAggregate{StateRevision: 1, Definition: definition, Facts: facts, Decision: decision})
}

func runtimeFileBackedMCPFailureCandidate() runtimecontract.RuntimeCandidate {
	serverID, tool, path := "mcp-grounding", "lookup.references", "validation-entry.go"
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "mcp-observed-failure", Kind: runtimecontract.StepKindMCPToolCall, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{path},
		Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}, {Name: "input.path", String: &path}},
	}}}}}
}

func runtimeFileBackedMCPFailureTargetSnapshotPrerequisite(t *testing.T, aggregate runtime.RuntimeAggregate) runtimecontract.RuntimePrerequisite {
	t.Helper()
	var matched runtimecontract.RuntimePrerequisite
	for _, prerequisite := range aggregate.Facts.Prerequisites.Pending {
		if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || prerequisite.SubjectRef != "implementation.go" {
			continue
		}
		if prerequisite.OwnerModuleID == "" || matched.ID != "" {
			t.Fatalf("MCP failure target scheduled an ambiguous owner snapshot: %+v", aggregate.Facts.Prerequisites.Pending)
		}
		matched = prerequisite
	}
	if matched.ID == "" {
		t.Fatalf("MCP failure target did not schedule an owner target snapshot: %+v", aggregate.Facts.Prerequisites.Pending)
	}
	return matched
}

func acquireFileBackedMCPTargetSnapshot(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate runtime.RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, deliverable string) fileevents.AcquireTargetSnapshotResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireTargetSnapshot, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, fileevents.AcquireTargetSnapshotCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverable, TargetRef: prerequisite.SubjectRef, MaxBytes: 16 * 1024,
		},
	))
	if result == nil {
		t.Fatal("File owner returned no target snapshot")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.AcquireTargetSnapshotResult)
	if !ok || !response.Snapshot.Complete() || response.Observation.ObservationID == "" {
		t.Fatalf("File target snapshot response = %#v", data)
	}
	// The typed owner response must seal the exact completed prerequisite and
	// target revision. The fixture never rewrites a File observation into a
	// different owner merely to satisfy the reducer.
	observation := response.Observation.Normalized()
	if err := observation.Validate(); err != nil {
		t.Fatalf("owner-grounded target snapshot observation: %v", err)
	}
	if observation.PrerequisiteID != prerequisite.ID || observation.Kind != prerequisite.Kind ||
		observation.OwnerModuleID != prerequisite.OwnerModuleID || observation.ContentHash != response.Snapshot.ContentHash ||
		len(observation.Dependencies) != 1 {
		t.Fatalf("target snapshot observation does not match completed prerequisite: prerequisite=%+v observation=%+v", prerequisite, observation)
	}
	dependency := observation.Dependencies[0]
	if dependency.Domain != runtimecontract.RuntimeResourceWorkspaceTarget || dependency.OwnerModuleID != prerequisite.OwnerModuleID ||
		dependency.SubjectRef != response.Snapshot.TargetRef || dependency.Revision != response.Snapshot.ContentHash {
		t.Fatalf("target snapshot observation does not bind exact owner revision: prerequisite=%+v observation=%+v", prerequisite, observation)
	}
	return response
}
