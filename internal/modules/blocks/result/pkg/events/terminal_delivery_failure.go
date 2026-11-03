package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// TerminalDeliveryStage identifies the receiving owner's failed boundary. It
// is diagnostic data only; it cannot authorize a retry, overwrite or ACK.
type TerminalDeliveryStage string

const (
	TerminalDeliveryStageFence            TerminalDeliveryStage = "workspace_fence"
	TerminalDeliveryStageCommand          TerminalDeliveryStage = "command_admission"
	TerminalDeliveryStageContext          TerminalDeliveryStage = "workspace_context"
	TerminalDeliveryStageInternalEvent    TerminalDeliveryStage = "internal_event"
	TerminalDeliveryStageValidation       TerminalDeliveryStage = "fact_validation"
	TerminalDeliveryStageMaterialHash     TerminalDeliveryStage = "material_hash"
	TerminalDeliveryStageResultRead       TerminalDeliveryStage = "result_read"
	TerminalDeliveryStageResultCreate     TerminalDeliveryStage = "result_create"
	TerminalDeliveryStageResultReadBack   TerminalDeliveryStage = "result_read_back"
	TerminalDeliveryStageResultCompare    TerminalDeliveryStage = "result_compare"
	TerminalDeliveryStageArtifact         TerminalDeliveryStage = "artifact_reconcile"
	TerminalDeliveryStageResultTransition TerminalDeliveryStage = "result_transition"
	TerminalDeliveryStageResultCAS        TerminalDeliveryStage = "result_cas"
	TerminalDeliveryStageResultWinner     TerminalDeliveryStage = "result_cas_winner"
)

// The exact original error is retained as Cause. MismatchFields contains only
// owner-computed field paths, never raw business values or unrelated config.
type TerminalDeliveryFailure struct {
	Stage          TerminalDeliveryStage
	ResultID       common.ResultID
	MismatchFields []string
	Cause          error
}

func (e *TerminalDeliveryFailure) Error() string {
	return fmt.Sprintf("result terminal delivery stage=%s result=%s mismatchFields=%s: %v", e.Stage, e.ResultID, strings.Join(e.MismatchFields, ","), e.Cause)
}

func (e *TerminalDeliveryFailure) Unwrap() error { return e.Cause }
