package file

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const mutationResultMaxBytes = 4 * 1024 * 1024

// Directory descriptor belongs only to the File owner; platform implementations
// provide access, and unsupported platforms fail before any effect.
type mutationResultStore struct{ fd int }

const mutationResultDeliveryLimit = 2 // original submission plus one original-material redelivery; never an effect retry

type mutationResultIdentity struct {
	Protocol  string
	Context   contract.RuntimeOperationExecutionContext
	Operation contract.RuntimeOperation
}

func mutationResultKey(scope contract.RuntimeOperationExecutionContext) string {
	b, _ := json.Marshal(struct {
		Workspace, Run, Key string
		Fence               uint64
	}{string(scope.WorkspaceID), string(scope.RunID), scope.Execution.ExecutionKey, scope.Execution.Fence})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (m *Biz) lockMutationResult(scope contract.RuntimeOperationExecutionContext) func() {
	key := mutationResultKey(scope)
	decoded, _ := hex.DecodeString(key[:2])
	index := int(decoded[0]) % len(m.mutationResultLocks)
	lock := &m.mutationResultLocks[index]
	lock.Lock()
	return lock.Unlock
}

func mutationDeliveryFailure(s *mutationResultStore, scope contract.RuntimeOperationExecutionContext, stage string, cause error) error {
	key := mutationResultKey(scope)
	d := &contract.RuntimeOriginalDiagnostic{SchemaVersion: contract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ModuleID, OriginalClass: "result_delivery", OriginalCode: "file_result_" + stage, OriginalMessage: "File operation result unavailable at " + stage, Signature: "file-result-" + key, OperationID: scope.OperationID}
	b := []byte(cause.Error())
	h := sha256.Sum256(b)
	d.DetailsBytes = int64(len(b))
	d.ContentType = "text/plain; charset=utf-8"

	if s != nil && len(b) <= mutationResultMaxBytes {
		d.OmittedReason = "diagnostic_capacity_exceeded"
		// At most eight complete errors for one original execution. An immutable
		// slot is either exact existing bytes or reserved once; never overwrite or
		// silently truncate a later error when the boundary is full.
		for i := 1; i <= 8; i++ {
			name := fmt.Sprintf("diagnostic-%d.txt", i)
			created, err := s.write(name, b)
			if err != nil {
				d.OmittedReason = "diagnostic_storage_failed"
				break
			}
			if !created {
				saved, e := s.read(name)
				if e != nil {
					d.OmittedReason = "diagnostic_storage_failed"
					break
				}
				if !bytes.Equal(saved, b) {
					continue
				}
			}
			d.DetailsRef = "workorch://file-operation-results/" + key + "/" + name
			d.DetailsHash = hex.EncodeToString(h[:])
			d.OmittedReason = ""
			break
		}
	} else {
		d.OmittedReason = "diagnostic_not_saved"
	}

	if s != nil && d.OmittedReason == "" {
		encoded, err := json.Marshal(d)
		if err != nil {
			d.OmittedReason = "diagnostic_index_save_failed"
		} else if created, saveErr := s.write("first-failure.json", encoded); saveErr != nil {
			d.OmittedReason = "diagnostic_index_save_failed"
		} else if !created && savedMutationFailure(s, scope) == nil {
			d.OmittedReason = "diagnostic_index_invalid"
		}
	}
	return &contract.RuntimeReceiptDeliveryError{ExecutionKey: scope.Execution.ExecutionKey, Fence: scope.Execution.Fence, Diagnostic: d, Cause: cause}
}

// A durable guard precedes the effect. Its existence is never permission to
// execute after restart. Only a complete original saved receipt can be delivered.
func (m *Biz) prepareMutationResult(ctx context.Context, protocol string, scope contract.RuntimeOperationExecutionContext, op contract.RuntimeOperation) (*mutationResultStore, contract.RuntimeOperationReceipt, bool, error) {
	if protocol == "" && scope.WorkspaceInternalDirectory == "" {
		return nil, contract.RuntimeOperationReceipt{}, false, nil
	}
	if protocol != fileevents.MutationResultDeliveryV1 {
		return nil, contract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	relative, relErr := filepath.Rel(scope.WorkspaceDeliverableDirectory, scope.WorkspaceInternalDirectory)
	if relErr != nil || relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return nil, contract.RuntimeOperationReceipt{}, false, mutationDeliveryFailure(nil, scope, "prepare", fmt.Errorf("File result storage must be outside the deliverable tree"))
	}
	s, err := openMutationResultStore(scope.WorkspaceInternalDirectory, mutationResultKey(scope), true)
	if err != nil {
		return nil, contract.RuntimeOperationReceipt{}, false, mutationDeliveryFailure(nil, scope, "prepare", err)
	}
	identity := mutationResultIdentity{protocol, scope, op}
	b, err := json.Marshal(identity)
	if err != nil {
		s.close()
		return nil, contract.RuntimeOperationReceipt{}, false, err
	}
	created, err := s.write("started.json", b)
	if err != nil {
		s.close()
		return nil, contract.RuntimeOperationReceipt{}, false, mutationDeliveryFailure(nil, scope, "prepare", err)
	}
	if created {
		return s, contract.RuntimeOperationReceipt{}, false, nil
	}
	if err = validateMutationResultIdentity(s, identity); err != nil {
		s.close()
		return nil, contract.RuntimeOperationReceipt{}, false, err
	}
	// No live physical effect on this path, including a crash before result save.
	receipt, err := m.deliverSavedMutationResult(ctx, s, scope)
	if err != nil {
		s.close()
		return nil, contract.RuntimeOperationReceipt{}, false, err
	}
	return s, receipt, true, nil
}
func validateMutationResultIdentity(s *mutationResultStore, want mutationResultIdentity) error {
	b, err := s.read("started.json")
	if err != nil {
		return err
	}
	var got mutationResultIdentity
	if err = json.Unmarshal(b, &got); err != nil {
		return err
	}
	canonical, marshalErr := json.Marshal(got)
	expected, expectedErr := json.Marshal(want)
	if marshalErr != nil || expectedErr != nil || !bytes.Equal(canonical, expected) {
		return fmt.Errorf("%w: File result original scope differs", common.ErrConflict)
	}
	return nil
}
func (m *Biz) commitMutationResult(ctx context.Context, s *mutationResultStore, scope contract.RuntimeOperationExecutionContext, receipt contract.RuntimeOperationReceipt) (contract.RuntimeOperationReceipt, error) {
	if s == nil {
		return m.commitRuntimeOperationReceipt(ctx, scope.WorkspaceID, scope.RunID, scope.Transaction.AuthoritySnapshotID, scope.DispatchID, scope.Execution, receipt)
	}
	attached, err := orchestratorevents.AttachRuntimeOperationReceiptFeedback(receipt, contract.RuntimeFeedbackScope{WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, AuthoritySnapshotID: scope.Transaction.AuthoritySnapshotID, DispatchID: scope.DispatchID}, ModuleID)
	if err != nil {
		return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "receipt_build", err)
	}
	b, err := json.Marshal(attached)
	if err != nil {
		return contract.RuntimeOperationReceipt{}, err
	}
	created, err := s.write("receipt.json", b)
	if err == nil && !created {
		previous, readErr := s.read("receipt.json")
		if readErr != nil {
			err = readErr
		} else if !bytes.Equal(previous, b) {
			err = fmt.Errorf("%w: saved original receipt differs", common.ErrConflict)
		}
	}
	if err != nil {
		return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "result_save", err)
	}
	return m.deliverSavedMutationResult(ctx, s, scope)
}
func (m *Biz) deliverSavedMutationResult(ctx context.Context, s *mutationResultStore, scope contract.RuntimeOperationExecutionContext) (contract.RuntimeOperationReceipt, error) {
	b, err := s.read("receipt.json")
	if err != nil {
		if prior := savedMutationFailure(s, scope); prior != nil {
			return contract.RuntimeOperationReceipt{}, prior
		}
		return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "original_result_missing", err)
	}
	var receipt contract.RuntimeOperationReceipt
	if err = json.Unmarshal(b, &receipt); err != nil {
		return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "result_decode", err)
	}
	if err = receipt.Validate(); err != nil {
		return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "result_validate", err)
	}
	if receipt.OperationID != scope.OperationID || receipt.ExecutionKey != scope.Execution.ExecutionKey || receipt.Fence != scope.Execution.Fence {
		return contract.RuntimeOperationReceipt{}, common.ErrConflict
	}
	var lastFailure error
	for i := 1; i <= mutationResultDeliveryLimit; i++ {
		// Always query first, including after the last reserved submission. A lost
		// response must not spend another attempt or regenerate the receipt.
		saved, found, queryErr := m.queryRuntimeOperationReceipt(ctx, scope.WorkspaceID, scope.RunID, scope.DispatchID, scope.OperationID, scope.Execution)
		if queryErr != nil {
			return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "receipt_query", queryErr)
		}
		if found {
			if !contract.SameRuntimeOperationReceipt(saved, receipt) {
				return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "receipt_mismatch", common.ErrConflict)
			}
			return saved, nil
		}
		if rejected, stopErr := s.read("rejected.txt"); stopErr == nil {
			return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "receipt_commit", errors.New(string(rejected)))
		} else if !errors.Is(stopErr, os.ErrNotExist) {
			return contract.RuntimeOperationReceipt{}, stopErr
		}
		if ctx.Err() != nil {
			return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "receipt_commit", ctx.Err())
		}
		reserved, reserveErr := s.write(fmt.Sprintf("attempt-%d", i), []byte(fileevents.MutationResultDeliveryV1))
		if reserveErr != nil {
			return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "attempt_save", reserveErr)
		}
		if !reserved {
			continue
		}
		committed, commitErr := m.commitRuntimeOperationReceipt(ctx, scope.WorkspaceID, scope.RunID, scope.Transaction.AuthoritySnapshotID, scope.DispatchID, scope.Execution, receipt)
		if commitErr == nil {
			return committed, nil
		}
		// Save every original error before any later delivery attempt. Failure of
		// diagnostic persistence stops delivery; it cannot be hidden by success.
		failure := mutationDeliveryFailure(s, scope, "receipt_commit", commitErr)
		lastFailure = failure
		var stored *contract.RuntimeReceiptDeliveryError
		if errors.As(failure, &stored) && stored.Diagnostic.OmittedReason != "" {
			return contract.RuntimeOperationReceipt{}, failure
		}
		if !retryableMutationReceiptDelivery(ctx, commitErr) {
			if _, err = s.write("rejected.txt", []byte(commitErr.Error())); err != nil {
				return contract.RuntimeOperationReceipt{}, failure
			}
			// It may have committed before returning an invalid/lost response.
			got, found, qerr := m.queryRuntimeOperationReceipt(ctx, scope.WorkspaceID, scope.RunID, scope.DispatchID, scope.OperationID, scope.Execution)
			if qerr == nil && found && contract.SameRuntimeOperationReceipt(got, receipt) {
				return got, nil
			}
			return contract.RuntimeOperationReceipt{}, failure
		}
	}
	got, found, qerr := m.queryRuntimeOperationReceipt(ctx, scope.WorkspaceID, scope.RunID, scope.DispatchID, scope.OperationID, scope.Execution)
	if qerr == nil && found && contract.SameRuntimeOperationReceipt(got, receipt) {
		return got, nil
	}
	if lastFailure != nil {
		return contract.RuntimeOperationReceipt{}, lastFailure
	}
	if prior := savedMutationFailure(s, scope); prior != nil {
		return contract.RuntimeOperationReceipt{}, prior
	}
	return contract.RuntimeOperationReceipt{}, mutationDeliveryFailure(s, scope, "delivery_exhausted", fmt.Errorf("original File receipt delivery exhausted after %d reserved submissions", mutationResultDeliveryLimit))
}

func retryableMutationReceiptDelivery(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var classification *contract.RuntimeReceiptDeliveryError
	if errors.As(err, &classification) {
		return classification.Retryable
	}
	// magicCommon v1.5.18 exposes no typed admission cause. Recognize only its
	// exact pre-dispatch queue rejection, never arbitrary Timeout/Unexpected or
	// owner semantic errors. The same original receipt still passes every gate.
	var admission *cd.Error
	return errors.As(err, &admission) && admission.Cause == nil && admission.Code == cd.Timeout && admission.Message == "event admission timed out or was canceled"
}
func (m *Biz) handleDeliverMutationResult(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if !requireOrchestratorRuntimeSource(event, result) {
		return
	}
	cmd, ok := event.Data().(fileevents.DeliverMutationResultCommand)
	if !ok {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	op, err := validateSealedRuntimeFileOperation(event, cmd.RuntimeOperationExecutionContext, cmd.SealedOperation, cmd.SealedOperation.Operation.Kind)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if op.Kind != contract.StepKindFileEdit && op.Kind != contract.StepKindFileWrite && op.Kind != contract.RuntimeBlockKindPatchApply {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	leave, err := m.enterOwner(false)
	if setPrerequisiteError(result, err) {
		return
	}
	defer leave()
	unlock := m.lockMutationResult(cmd.RuntimeOperationExecutionContext)
	defer unlock()
	// Query the Runtime owner before reading or publishing any local result.
	if _, _, err = m.queryRuntimeOperationReceipt(event.Context(), cmd.WorkspaceID, cmd.RunID, cmd.DispatchID, cmd.OperationID, cmd.Execution); setPrerequisiteError(result, err) {
		return
	}
	s, err := openMutationResultStore(cmd.WorkspaceInternalDirectory, mutationResultKey(cmd.RuntimeOperationExecutionContext), false)
	if errors.Is(err, os.ErrNotExist) {
		result.Set(fileevents.DeliverMutationResultResult{}, nil)
		return
	}
	if setPrerequisiteError(result, err) {
		return
	}
	defer s.close()
	if err = validateMutationResultIdentity(s, mutationResultIdentity{fileevents.MutationResultDeliveryV1, cmd.RuntimeOperationExecutionContext, op}); setPrerequisiteError(result, err) {
		return
	}
	_, err = m.deliverSavedMutationResult(event.Context(), s, cmd.RuntimeOperationExecutionContext)
	if setPrerequisiteError(result, err) {
		return
	}
	result.Set(fileevents.DeliverMutationResultResult{Found: true}, nil)
}

func errorBytes(err error) []byte {
	if err != nil {
		return []byte(err.Error())
	}
	return nil
}

// Diagnostic preservation is never permission to commit or execute.
func savedMutationFailure(s *mutationResultStore, scope contract.RuntimeOperationExecutionContext) error {
	raw, err := s.read("first-failure.json")
	if err != nil {
		return nil
	}
	var d contract.RuntimeOriginalDiagnostic
	if json.Unmarshal(raw, &d) != nil || d.Validate() != nil || d.OwnerModuleID != ModuleID || d.OperationID != scope.OperationID {
		return nil
	}
	prefix := "workorch://file-operation-results/" + mutationResultKey(scope) + "/"
	if !strings.HasPrefix(d.DetailsRef, prefix) || d.OmittedReason != "" {
		return nil
	}
	original, err := s.read(strings.TrimPrefix(d.DetailsRef, prefix))
	if err != nil {
		return nil
	}
	hash := sha256.Sum256(original)
	if int64(len(original)) != d.DetailsBytes || hex.EncodeToString(hash[:]) != d.DetailsHash {
		return nil
	}
	return &contract.RuntimeReceiptDeliveryError{ExecutionKey: scope.Execution.ExecutionKey, Fence: scope.Execution.Fence, Diagnostic: d.CloneOriginal()}
}
