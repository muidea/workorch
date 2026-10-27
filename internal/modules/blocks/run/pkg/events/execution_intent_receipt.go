package events

import (
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const TopicQueryWorkspaceExecutionIntentReceipt = "/run/workspace-execution/receipt/query"

type QueryWorkspaceExecutionIntentReceiptCommand struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
	PayloadHash string
}

func (c QueryWorkspaceExecutionIntentReceiptCommand) Validate() error {
	if runmodel.ValidatePreparationHistoryWorkspaceID(c.WorkspaceID) != nil || !runmodel.ValidExecutionIntentIdentity(c.RequestID) || !runmodel.ValidExecutionIntentPayloadHash(c.PayloadHash) {
		return common.ErrInvalidArgument
	}
	return nil
}

type QueryWorkspaceExecutionIntentReceiptResult struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
	PayloadHash string
	ObservedAt  time.Time
	// Nil explicitly means unknown, never not-admitted or safe-to-retry.
	Receipt *runmodel.WorkspaceExecutionIntentReceipt
}

func (r QueryWorkspaceExecutionIntentReceiptResult) Clone() QueryWorkspaceExecutionIntentReceiptResult {
	if r.Receipt != nil {
		value := *r.Receipt
		r.Receipt = &value
	}
	return r
}

func (r QueryWorkspaceExecutionIntentReceiptResult) ValidateFor(c QueryWorkspaceExecutionIntentReceiptCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.RequestID != c.RequestID || r.PayloadHash != c.PayloadHash || r.ObservedAt.IsZero() {
		return common.ErrConflict
	}
	if r.Receipt != nil && (r.Receipt.Validate() != nil || r.Receipt.Admission.WorkspaceID != c.WorkspaceID || r.Receipt.Admission.RequestID != c.RequestID || r.Receipt.PayloadHash != c.PayloadHash) {
		return common.ErrConflict
	}
	return nil
}

// WorkspaceExecutionIntentPayloadHash binds all normalized concrete command
// fields. JSON is used only for a local identity, never for owner transport.
func WorkspaceExecutionIntentPayloadHash(c WorkspaceExecutionIntentCommand) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	c = c.Normalized()
	if c.Kind != runtimecontract.RuntimeExecutionIntentRestart || runmodel.ValidatePreparationHistoryWorkspaceID(c.WorkspaceID) != nil || !runmodel.ValidExecutionIntentIdentity(c.RequestID) {
		return "", common.ErrInvalidArgument
	}
	return runmodel.ExecutionIntentPayloadHash(c.WorkspaceID, c.RequestID, *c.Bootstrap)
}
