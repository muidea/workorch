package orchestrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This uses the production sender -> real Git -> real original-receipt handler
// -> file-backed Run aggregate, not the test-only raw archive oracle below.
func runtimeAssertGitCommitSnapshotOwnerQuery(t *testing.T, ctx context.Context, module *Biz, directory string) {
	t.Helper()
	aggregate, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.Candidate.StateChange == nil {
			continue
		}
		for _, member := range dispatch.Candidate.StateChange.Operations {
			operation := member.Operation
			if operation.Kind != runtimecontract.StepKindGitWrite {
				continue
			}
			data, err := gitevents.OperationDataFromRuntimeOperation(operation)
			if err != nil {
				t.Fatal(err)
			}
			if data.Operation != "commit" {
				continue
			}
			for _, execution := range dispatch.OperationExecutions {
				if execution.OperationID != operation.ID {
					continue
				}
				matches++
				observation, err := module.queryRuntimeGitCommitSnapshotFromAggregate(ctx, aggregate, dispatch.ID, operation.ID, execution.OwnerReceiptHash, "terminal-snapshot-owner-query")
				if err != nil || observation.CurrentDependencies == nil || observation.SnapshotHead != strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD")) ||
					observation.SnapshotRootTree != strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD^{tree}")) {
					t.Fatalf("production Git snapshot query cannot reverse-prove original receipt and current full graph: %+v %v", observation, err)
				}
				receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
				if !found || !reflect.DeepEqual(observation.CurrentDependencies, receipt.Evidence.OwnerRevisionSet) || observation.SnapshotAddress != *receipt.Evidence.ContentAddress {
					t.Fatal("production query lost complete original dependency/address identity")
				}
				duplicate, err := module.queryRuntimeGitCommitSnapshotFromAggregate(ctx, aggregate, dispatch.ID, operation.ID, execution.OwnerReceiptHash, "terminal-snapshot-owner-query")
				if err != nil || !reflect.DeepEqual(duplicate, observation) {
					t.Fatalf("duplicate current query drifted: %v", err)
				}
				// Technical sender/receiver calibration: these fixture bytes are
				// not a Runtime-reviewed subject. The exact review-source compiler
				// is independently exercised by the real validation mainline.
				input := runtimecontract.RuntimeValidationProofRequirementBinding{
					RequirementHash: "byte-membership-calibration", DefinitionRevision: aggregate.Facts.DefinitionRevision,
					CoverageWorkItemID: "cwi-edit", DeclarationRef: "byte-membership-calibration", ValidationRecipeID: "byte-membership-calibration",
					RequiredCheckRefs: []string{"byte-membership-check"}, CheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "byte-membership-check", ObservationProtocolRef: "byte-membership-native", NativeCheckRef: "byte-membership-native"}},
				}
				content, err := os.ReadFile(filepath.Join(directory, "main.go"))
				if err != nil {
					t.Fatal(err)
				}
				input.InputReview = &runtimecontract.RuntimeValidationInputReviewSubject{ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "main.go", Content: string(content), ContentHash: runtimecontract.RuntimeValidationInputContentHash(string(content))}}}
				input.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*input.InputReview, input)
				member, err := module.queryRuntimeGitCommitSnapshotWithInputs(ctx, aggregate, dispatch.ID, operation.ID, execution.OwnerReceiptHash, "snapshot-input-membership-calibration", &input)
				if err != nil || member.ProtocolRef != gitevents.RuntimeCommitSnapshotInputQueryProtocol || member.InputSubjectHash != input.InputReview.Hash || len(member.InputMemberships) != 1 || member.InputMemberships[0].BlobID != strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD:main.go")) {
					t.Fatalf("production V2 sender lost exact input/blob membership: %+v %v", member, err)
				}
				for _, validation := range aggregate.CompletedDispatches {
					if validation.Candidate.Validation == nil {
						continue
					}
					for _, original := range validation.OperationExecutions {
						if _, err := module.queryRuntimeGitCommitSnapshotWithReviewedInputsFromAggregate(ctx, aggregate, dispatch.ID, operation.ID, execution.OwnerReceiptHash, "legacy-review-source", validation.ID, original.OperationID, original.OwnerReceiptHash); err == nil {
							t.Fatal("legacy unreviewed validation was automatically promoted to reviewed membership")
						}
					}
				}
				for _, bad := range []string{"", strings.Repeat("0", 64)} {
					if _, err := module.queryRuntimeGitCommitSnapshotFromAggregate(ctx, aggregate, dispatch.ID, operation.ID, bad, "terminal-snapshot-owner-query"); err == nil {
						t.Fatal("query accepted forged source receipt hash")
					}
				}
				// Test-owned repository: a later real HEAD must not be hidden by
				// completed delivery or the historical evidence revision.
				runtimeGitOutput(t, directory, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "later owner observation")
				later, err := module.queryRuntimeGitCommitSnapshotFromAggregate(ctx, aggregate, dispatch.ID, operation.ID, execution.OwnerReceiptHash, "terminal-snapshot-owner-query")
				if err != nil || later.CurrentDependencies != nil || later.SnapshotHead != observation.SnapshotHead || later.ObservedHead == observation.SnapshotHead {
					t.Fatalf("real current HEAD drift not reported: %+v %v", later, err)
				}
				laterInput, err := module.queryRuntimeGitCommitSnapshotWithInputs(ctx, aggregate, dispatch.ID, operation.ID, execution.OwnerReceiptHash, "snapshot-input-membership-calibration", &input)
				if err != nil || laterInput.CurrentDependencies != nil || !reflect.DeepEqual(laterInput.InputMemberships, member.InputMemberships) {
					t.Fatalf("V2 original input membership concealed current HEAD drift: %v", err)
				}
			}
		}
	}
	if matches != 1 {
		t.Fatalf("snapshot query selected %d commit executions", matches)
	}
	persisted, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(persisted)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only query/current HEAD drift changed aggregate, terminal seal, credit or budget")
	}
}

// Independent test-only owner archive oracle. Production must use a Git-owned
// typed read boundary; neither Runtime nor Result may read this private file.
type runtimeGitSnapshotOracleScope struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	DispatchID         string
	OperationID        string
	DefinitionRevision string
	ExecutionKey       string
	Fence              uint64
	OperationHash      string
	AuthorityHash      string
}

func runtimeAssertGitCommitSnapshotMaterial(t *testing.T, state runtimecontract.RuntimeStateRecord, directory, internalDirectory string, result resultmodel.Result) {
	t.Helper()
	commitCount := 0
	for _, dispatch := range state.CompletedDispatches {
		if dispatch.Candidate.StateChange == nil {
			continue
		}
		for _, member := range dispatch.Candidate.StateChange.Operations {
			operation := member.Operation
			if operation.Kind != runtimecontract.StepKindGitWrite {
				continue
			}
			commit := false
			for _, parameter := range operation.Parameters {
				commit = commit || (parameter.Name == "operation" && parameter.String != nil && *parameter.String == "commit")
			}
			if !commit {
				continue
			}
			commitCount++
			matches := 0
			for _, execution := range dispatch.OperationExecutions {
				if execution.OperationID != operation.ID {
					continue
				}
				matches++
				if execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
					t.Fatal("snapshot commit is not evidence-committed")
				}
				receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(state.OperationReceipts, execution.ExecutionKey, execution.Fence)
				if !found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != execution.OwnerReceiptHash || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed {
					t.Fatal("snapshot lacks exact accepted durable owner receipt")
				}
				address := receipt.Evidence.ContentAddress
				if address == nil || address.Validate() != nil || address.Ref != "git/commit-object-snapshot/v1/"+address.Digest || receipt.Evidence.ContentHash != address.Digest || !slices.Contains(receipt.Evidence.EvidenceRefs, address.Ref) {
					t.Fatal("snapshot raw identity is not bound to owner receipt")
				}
				scope := runtimeGitSnapshotOracleScope{WorkspaceID: state.Facts.WorkspaceID, RunID: state.Facts.RunID, DispatchID: dispatch.ID, OperationID: operation.ID,
					DefinitionRevision: state.Facts.DefinitionRevision, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationHash: execution.OperationHash, AuthorityHash: execution.AuthorityHash}
				scopeBody, _ := json.Marshal(scope)
				scopeHash := sha256.Sum256(scopeBody)
				path := filepath.Join(internalDirectory, "git", "commit-object-snapshots", hex.EncodeToString(scopeHash[:])+".json")
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(body)
				if hex.EncodeToString(digest[:]) != address.Digest {
					t.Fatal("receipt content address differs from original archive bytes")
				}
				var record struct {
					ProtocolRef string
					Scope       runtimeGitSnapshotOracleScope
					Head        string
					RootTree    string
					Objects     []struct {
						Kind string
						ID   string
						Body []byte
					}
				}
				if err := json.Unmarshal(body, &record); err != nil || record.ProtocolRef != "git/commit-object-snapshot/v1" || record.Scope != scope {
					t.Fatalf("archive original sealed execution source: %v", err)
				}
				// HEAD equality here is an independent fixture observation, not
				// proof that production has a terminal-currentness consumer.
				if strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD")) != record.Head || strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD^{tree}")) != record.RootTree ||
					!slices.Contains(receipt.Evidence.EvidenceRefs, "workorch://git/commit/"+record.Head) {
					t.Fatal("actual snapshot HEAD/tree/commit receipt mismatch")
				}
				command := exec.Command("git", "--no-replace-objects", "ls-tree", "-r", "-t", "-z", record.Head)
				command.Dir = directory
				listed, err := command.Output()
				if err != nil {
					t.Fatal(err)
				}
				objects := map[string]bool{record.Head: true, record.RootTree: true}
				for _, row := range bytes.Split(listed, []byte{0}) {
					if len(row) == 0 {
						continue
					}
					metadata, _, ok := bytes.Cut(row, []byte{'\t'})
					fields := strings.Fields(string(metadata))
					if !ok || len(fields) != 3 {
						t.Fatal("invalid independent Git tree oracle")
					}
					objects[fields[2]] = true
				}
				dependencies := receipt.Evidence.OwnerRevisionSet
				if dependencies == nil || dependencies.Validate() != nil || len(objects) != len(record.Objects) || len(objects) != len(dependencies.Entries) {
					t.Fatal("snapshot archive/dependencies omit reachable objects")
				}
				for _, object := range record.Objects {
					if !objects[object.ID] {
						t.Fatal("duplicate or extra archived object")
					}
					delete(objects, object.ID)
					command := exec.Command("git", "--no-replace-objects", "cat-file", object.Kind, object.ID)
					command.Dir = directory
					actual, err := command.Output()
					if err != nil || !bytes.Equal(actual, object.Body) {
						t.Fatalf("raw object bytes changed: %v", err)
					}
					objectDigest := sha256.Sum256(object.Body)
					matches := 0
					for _, dependency := range dependencies.Entries {
						if dependency.ResourceID == object.ID && dependency.SnapshotID == object.ID && dependency.OwnerKind == "capability" && dependency.OwnerInstanceID == "blocks/git/"+string(state.Facts.WorkspaceID) && dependency.ResourceKind == "git_object" && dependency.Revision == hex.EncodeToString(objectDigest[:]) {
							matches++
						}
					}
					if matches != 1 {
						t.Fatal("actual object bytes lack unique owner dependency")
					}
				}
				facts := 0
				for _, fact := range state.Facts.Evidence.Accepted {
					if fact.ID != execution.EvidenceFactID {
						continue
					}
					facts++
					if runtimecontract.StableRuntimeEvidenceFactHash(fact) != execution.EvidenceFactHash || fact.DispatchID != dispatch.ID || fact.OperationID != operation.ID ||
						!reflect.DeepEqual(fact.ContentAddress, address) || fact.ContentHash != address.Digest || !reflect.DeepEqual(fact.OwnerRevisionSet, dependencies) || !slices.Contains(fact.EvidenceRefs, address.Ref) {
						t.Fatal("receipt snapshot identity did not reach exact accepted evidence")
					}
					seal := state.TerminalMaterialSeal
					if seal == nil || result.RuntimeCloseout == nil || len(state.DeliveryAcknowledgements) != 3 || !reflect.DeepEqual(seal.Material.RuntimeCloseout, result.RuntimeCloseout) ||
						!slices.Contains(seal.Material.EvidenceRefs, address.Ref) || !slices.Contains(result.EvidenceRefs, address.Ref) || !slices.Contains(result.RuntimeCloseout.EvidenceRefs, address.Ref) || !slices.Contains(result.RuntimeCloseout.AcceptedEvidenceRefs, string(fact.ID)) {
						t.Fatal("snapshot ref/fact ID lost in immutable seal or real formal Result")
					}
				}
				if facts != 1 {
					t.Fatal("commit receipt has no unique accepted evidence fact")
				}
			}
			if matches != 1 {
				t.Fatal("commit has no unique sealed execution")
			}
		}
	}
	if commitCount != 1 {
		t.Fatalf("expected one real commit, got %d", commitCount)
	}
}
