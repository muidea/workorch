package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const WorkspaceExecutionIntentReceiptVersion = "workorch-execution-intent-receipt/v1"
const WorkspaceExecutionIntentReceiptLimit = 128

type WorkspaceExecutionIntentReceipt struct {
	Version     string
	PayloadHash string
	RecordedAt  time.Time
	Admission   runtimecontract.RuntimeExecutionIntentAdmission
}

func ValidExecutionIntentIdentity(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func ValidExecutionIntentPayloadHash(value string) bool {
	bytes, err := hex.DecodeString(value)
	return err == nil && len(bytes) == 32 && hex.EncodeToString(bytes) == value
}

func (r WorkspaceExecutionIntentReceipt) Validate() error {
	a := r.Admission
	if r.Version != WorkspaceExecutionIntentReceiptVersion || !ValidExecutionIntentPayloadHash(r.PayloadHash) || r.RecordedAt.IsZero() || a.Validate() != nil || a != a.Normalized() || a.Kind != runtimecontract.RuntimeExecutionIntentRestart || !ValidExecutionIntentIdentity(a.RequestID) || !ValidExecutionIntentIdentity(string(a.RunID)) || ValidatePreparationHistoryWorkspaceID(a.WorkspaceID) != nil {
		return common.ErrConflict
	}
	// Only initial durable acceptance is evidence. Replay/continuation and
	// governance projections cannot retroactively stand in for that response.
	switch a.Reason {
	case runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
		runtimecontract.RuntimeExecutionIntentReasonPredecessorSupersedeRequested,
		runtimecontract.RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested:
	default:
		return common.ErrConflict
	}
	return nil
}

func ValidateExecutionIntentReceipts(item *Run) error {
	if item == nil || len(item.ExecutionIntentReceipts) > WorkspaceExecutionIntentReceiptLimit {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	for _, r := range item.ExecutionIntentReceipts {
		if r.Validate() != nil || r.Admission.WorkspaceID != item.WorkspaceID || r.Admission.RunID != item.ID || seen[r.Admission.RequestID] {
			return common.ErrConflict
		}
		expected, err := executionReceiptSourceHash(item, r)
		if err != nil || expected != r.PayloadHash {
			return common.ErrConflict
		}
		seen[r.Admission.RequestID] = true
	}
	return nil
}

// ExecutionIntentPayloadHash is a local, credential-free identity compiler,
// not an EventHub payload or a trusted-success token. Restart's independent
// selection fields have already been checked against this exact bootstrap by
// the command validator. Keeping one compiler lets reload re-prove the receipt
// against the immutable original activation/replacement source as well.
type executionIntentIdentityInput struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	RequestID          string
	Kind               runtimecontract.RuntimeExecutionIntentKind
	GoalID             common.GoalID
	DoDID              common.DoDID
	DefinitionRevision string
	StartupInputHash   string
	Bootstrap          *runtimecontract.RuntimeBootstrap
}

func ExecutionIntentPayloadHash(workspaceID common.WorkspaceID, requestID string, bootstrap runtimecontract.RuntimeBootstrap) (string, error) {
	if ValidatePreparationHistoryWorkspaceID(workspaceID) != nil || !ValidExecutionIntentIdentity(requestID) || runtimecontract.ValidateRuntimeBootstrapRaw(bootstrap) != nil {
		return "", common.ErrInvalidArgument
	}
	d := bootstrap.Definition.Normalized()
	encoded, err := json.Marshal(struct {
		Domain string
		Intent executionIntentIdentityInput
	}{Domain: "workorch-run-execution-intent/v1", Intent: executionIntentIdentityInput{workspaceID, "", requestID, runtimecontract.RuntimeExecutionIntentRestart, d.GoalID, d.DoDID, d.Revision, d.StartupInputHash, &bootstrap}})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func executionReceiptSourceHash(item *Run, r WorkspaceExecutionIntentReceipt) (string, error) {
	id := r.Admission.RequestID
	if r.Admission.Reason == runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
		if item.ActivationRequestID != id || item.RuntimeBootstrap == nil {
			return "", common.ErrConflict
		}
		d := item.RuntimeBootstrap.Definition.Normalized()
		if item.GoalID != d.GoalID || item.DoDID != d.DoDID || item.DefinitionRevision != d.Revision || item.StartupInputHash != d.StartupInputHash {
			return "", common.ErrConflict
		}
		return ExecutionIntentPayloadHash(item.WorkspaceID, id, *item.RuntimeBootstrap)
	}
	var source *ReplacementIntent
	check := func(i *ReplacementIntent) bool {
		if i == nil || i.RequestID != id {
			return true
		}
		if source != nil {
			return false
		}
		source = i
		return true
	}
	if !check(item.ReplacementIntent) {
		return "", common.ErrConflict
	}
	for index := range item.RetiredReplacementIntents {
		if !check(&item.RetiredReplacementIntents[index]) {
			return "", common.ErrConflict
		}
	}
	if source == nil || source.WorkspaceID != item.WorkspaceID || source.Bootstrap == nil {
		return "", common.ErrConflict
	}
	d := source.Bootstrap.Definition.Normalized()
	if source.GoalID != d.GoalID || source.DoDID != d.DoDID || source.DefinitionRevision != d.Revision || source.StartupInputHash != d.StartupInputHash {
		return "", common.ErrConflict
	}
	return ExecutionIntentPayloadHash(item.WorkspaceID, id, *source.Bootstrap)
}

// Generic metadata updates preserve the full prefix. A CAS can append one
// owner-authored receipt but cannot rewrite history; opaque Runtime CAS must
// additionally preserve all Run-owner fields through its existing write gate.
func ValidateExecutionIntentReceiptTransition(current, next *Run, allowAppend bool) error {
	if current == nil || next == nil || ValidateExecutionIntentReceipts(current) != nil || ValidateExecutionIntentReceipts(next) != nil || current.ID != next.ID || current.WorkspaceID != next.WorkspaceID {
		return common.ErrConflict
	}
	before, after := current.ExecutionIntentReceipts, next.ExecutionIntentReceipts
	if len(after) < len(before) || len(after) > len(before)+1 || (!allowAppend && len(after) != len(before)) || !slices.Equal(before, after[:len(before)]) {
		return common.ErrConflict
	}
	return nil
}

// CompileExecutionIntentReceipt consumes the complete owner inventory, not a
// visibility-filtered projection. Absence is unknown, not a rejection fence.
func CompileExecutionIntentReceipt(workspaceID common.WorkspaceID, requestID string, items []*Run) (*WorkspaceExecutionIntentReceipt, error) {
	if ValidatePreparationHistoryWorkspaceID(workspaceID) != nil || !ValidExecutionIntentIdentity(requestID) {
		return nil, common.ErrInvalidArgument
	}
	var found *WorkspaceExecutionIntentReceipt
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || ValidateExecutionIntentReceipts(item) != nil {
			return nil, common.ErrConflict
		}
		for _, receipt := range item.ExecutionIntentReceipts {
			if receipt.Admission.RequestID != requestID {
				continue
			}
			if found != nil { // duplicate across two Runs is not an arbitrary winner
				return nil, common.ErrConflict
			}
			value := receipt
			found = &value
		}
	}
	return found, nil
}
