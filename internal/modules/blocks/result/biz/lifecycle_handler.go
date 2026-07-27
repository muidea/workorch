package result

import (
	"context"
	"errors"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) lifecycleTopics() []string {
	return []string{resultevents.TopicResultCommandApplyTerminalDelivery}
}

func (m *Biz) handleLifecycleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeLifecycleCommand(ev)
	setResultLifecycleResult(result, response, err)
}

func setResultLifecycleResult(result magicevent.Result, data resultevents.ResultLifecycleCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Biz) executeLifecycleCommand(ev magicevent.Event) (resultevents.ResultLifecycleCommandResult, error) {
	if m == nil || m.service == nil {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if ev.ID() != resultevents.TopicResultCommandApplyTerminalDelivery {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(resultevents.ApplyRuntimeTerminalDeliveryCommand)
	if !ok || ev.Source() != "kernel/orchestrator" {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	return m.applyTerminalDelivery(ev.Context(), cmd)
}

func (m *Biz) applyTerminalDelivery(ctx context.Context, cmd resultevents.ApplyRuntimeTerminalDeliveryCommand) (resultevents.ResultLifecycleCommandResult, error) {
	if err := cmd.Validate(); err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	// The caller supplies only sealed outcome material. Result.State and the
	// owner-local delivery fence are derived here, never accepted as targets.
	item := resultFromTerminalMaterial(cmd.Material)
	identity := cmd.Identity().Normalized()
	item.TerminalDelivery = &identity
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch cmd.Action {
	case runtimecontract.RuntimeActionComplete:
		item.State = common.ResultStateAccepted
		item.AcceptedAt = now
		item.RejectedAt = ""
		item.AcceptedBy = ModuleID
	case runtimecontract.RuntimeActionFail:
		item.State = common.ResultStateRejected
		item.RejectedAt = now
		item.AcceptedAt = ""
		item.AcceptedBy = ""
	default:
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	existing, err := m.service.GetResult(ctx, item.ID)
	if err == nil {
		if existing.WorkspaceID == item.WorkspaceID && existing.RunID == item.RunID && sameTerminalDeliveryIdentity(existing.TerminalDelivery, item.TerminalDelivery) {
			return resultevents.ResultLifecycleCommandResult{
				Result:       existing,
				Transitioned: false,
				Diagnostic:   "result already recorded",
				DeliveryAck:  resultTerminalDeliveryAck(existing, cmd, runtimecontract.RuntimeDeliveryAcknowledgedNoop),
			}, nil
		}
		return resultevents.ResultLifecycleCommandResult{}, common.ErrConflict
	}
	if !errors.Is(err, common.ErrNotFound) {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	ctx = resultCommandContext(ctx, item.WorkspaceID)
	if err := m.service.CreateResult(ctx, item); err != nil {
		if errors.Is(err, common.ErrConflict) {
			// Another Result owner delivery may have won the create race. Re-read
			// its immutable terminal fence before treating this as a conflict.
			existing, getErr := m.service.GetResult(ctx, item.ID)
			if getErr == nil && existing.WorkspaceID == item.WorkspaceID && existing.RunID == item.RunID && sameTerminalDeliveryIdentity(existing.TerminalDelivery, item.TerminalDelivery) {
				return resultevents.ResultLifecycleCommandResult{
					Result: existing, Transitioned: false, Diagnostic: "result already recorded",
					DeliveryAck: resultTerminalDeliveryAck(existing, cmd, runtimecontract.RuntimeDeliveryAcknowledgedNoop),
				}, nil
			}
		}
		// An owner write error is not an acknowledgement, even when storage may
		// already contain the Result (the crash-after-persist boundary). A retry
		// re-enters above, observes the durable record, and returns the idempotent
		// acknowledgement without allowing Workspace/Run to advance early.
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	m.publishResultChanged(ctx, item)
	return resultevents.ResultLifecycleCommandResult{
		Result:       item,
		Transitioned: true,
		DeliveryAck:  resultTerminalDeliveryAck(item, cmd, runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded),
	}, nil
}

func resultFromTerminalMaterial(material resultevents.ResultMaterial) *resultmodel.Result {
	material = material.Normalized()
	return &resultmodel.Result{
		ID: material.ID, WorkspaceID: material.WorkspaceID, RunID: material.RunID,
		GoalID: material.GoalID, DoDID: material.DoDID, Title: material.Title, Summary: material.Summary,
		SourceType: material.SourceType, SourceRef: material.SourceRef,
		EvidenceRefs: append([]string(nil), material.EvidenceRefs...), ArtifactRefs: append([]common.ArtifactID(nil), material.ArtifactRefs...),
		DoDAssessment: material.DoDAssessment, SummaryDetail: material.SummaryDetail, RuntimeCloseout: material.RuntimeCloseout,
	}
}

func sameTerminalDeliveryIdentity(left, right *runtimecontract.RuntimeTerminalDeliveryIdentity) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.TerminalDeliveryID == r.TerminalDeliveryID &&
		l.WorkspaceID == r.WorkspaceID && l.RunID == r.RunID &&
		l.AuthoritySnapshotID == r.AuthoritySnapshotID &&
		l.DefinitionRevision == r.DefinitionRevision && l.EvidenceRevision == r.EvidenceRevision &&
		l.DispatchID == r.DispatchID && l.Action == r.Action &&
		l.TerminalCode == r.TerminalCode && l.TerminalSummary == r.TerminalSummary
}

func resultTerminalDeliveryAck(
	item *resultmodel.Result,
	cmd resultevents.ApplyRuntimeTerminalDeliveryCommand,
	state runtimecontract.RuntimeDeliveryAcknowledgedState,
) *runtimecontract.RuntimeDeliveryAcknowledgement {
	if item == nil {
		return nil
	}
	cmd = cmd.Normalized()
	ack := runtimecontract.RuntimeDeliveryAcknowledgement{
		WorkspaceID:         item.WorkspaceID,
		RunID:               item.RunID,
		TerminalDeliveryID:  cmd.TerminalDeliveryID,
		Owner:               runtimecontract.RuntimeDeliveryOwnerResult,
		AcknowledgedState:   state,
		AuthoritySnapshotID: cmd.AuthoritySnapshotID,
		CausationID:         cmd.CausationID,
		Action:              cmd.Action,
		TerminalCode:        cmd.TerminalCode,
	}.Normalized()
	return &ack
}
