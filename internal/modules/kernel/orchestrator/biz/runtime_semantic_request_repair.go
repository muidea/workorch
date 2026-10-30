package orchestrator

import (
	"context"
	"errors"
	"fmt"
	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"time"
)

// The original complete provider document is re-read through its restricted
// artifact boundary before the first recovery CAS. No projection or prose
// matching can authorize this path. Decision response loss replays the exact
// persisted repair intent; provider submission is owned by a later drive.
func (m *Biz) recoverRuntimeSemanticRequestContract(ctx context.Context, ws common.WorkspaceID, run common.RunID) (runtime.RuntimeAggregate, bool, error) {
	a, err := m.runtimeController.Load(ctx, ws, run)
	if errors.Is(err, common.ErrNotFound) {
		return a, false, nil
	}
	if err != nil {
		return a, false, err
	}
	if a.DefinitionClarification == nil {
		return a, false, nil
	}
	proof, err := runtime.CompileRuntimeSemanticRequestRepairProof(a)
	if err != nil {
		if a.DefinitionClarification.RequestContractRepair != nil {
			return a, false, err
		}
		return a, false, nil
	}

	// The original SM-1 wait must be acknowledged before its exact continuation
	// can be delivered. This also closes a crash before the first wait handoff.
	record := runtimeStateRecordFromAggregate(a)
	handoff, governed, handoffErr := runtimeGovernanceLifecycleHandoffFromState(&record)
	if handoffErr != nil || !governed {
		return a, false, common.ErrConflict
	}
	runState, err := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, ws, run)
	if err != nil {
		return a, false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, err)
	}
	if runState.State == common.RunStateActive || runState.State == common.RunStatePending {
		if err := m.applyRunGovernanceWait(ctx, ws, run, handoff.AuthoritySnapshotID, handoff.DefinitionRevision, handoff.EvidenceRevision, handoff.DispatchID, handoff.TicketID, "", a.StateRevision); err != nil {
			return a, false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, err)
		}
	} else if runState.State != common.RunStateBlockedOnDecision || runState.BlockingTicketID != proof.Request.TicketID || runState.BlockingApprovalID != "" {
		return a, false, common.ErrConflict
	}
	payload, err := loadCompletedRuntimeCandidateRequestDocument(ctx, m.evidence.ArtifactContinuations, a, proof.Expected.LogicalRequestID)
	if err != nil {
		return a, false, err
	}
	if runtimecontract.StableRuntimeProviderResponseHash(payload) != proof.Observation.SemanticDocumentHash {
		return a, false, fmt.Errorf("%w: request repair original document hash mismatch", common.ErrConflict)
	}
	normalized, _ := runtimecontract.NormalizeRuntimeCandidatePayloadDocument(payload)
	gap, err := runtimecontract.DecodeRuntimeDefinitionGapPayloadWithManifest(proof.Manifest, proof.Expected, normalized)
	if err != nil {
		return a, false, err
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnosticFromDefinitionGap(gap)
	if !reflect.DeepEqual(diagnostic, proof.Observation.OriginalDiagnostic) || gap.GapKind != proof.Observation.GapKind || !reflect.DeepEqual(gap.OffendingItemRefs, proof.Observation.OffendingItemRefs) {
		return a, false, fmt.Errorf("%w: request repair original gap source mismatch", common.ErrConflict)
	}
	if a.DefinitionClarification.RequestContractRepair == nil {
		updated, e := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticRequestRepairPrepared, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: ws, RunID: run}, Authority: &runtime.RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof}})
		if e != nil {
			return a, false, e
		}
		a = updated.Aggregate
	}
	proof = a.DefinitionClarification.RequestContractRepair.Clone()
	cmd := decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand{WorkspaceID: ws, RunID: run, Proof: proof}
	if err := cmd.Validate(); err != nil {
		return a, false, err
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	bindOwnerCommandLane(event, decisionevents.Contracts(), decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract, "decision", string(cmd.Proof.Request.TicketID), ws, run)
	result := m.EventHub().Send(event)
	if result == nil {
		return a, false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, common.ErrConflict)
	}
	if err := prerequisiteResultError(result); err != nil {
		return a, false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, err)
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return a, false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, prerequisiteMagicError(magicErr))
	}
	response, ok := data.(decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult)
	if !ok {
		return a, false, common.ErrConflict
	}
	if err := response.ValidateFor(cmd); err != nil {
		return a, false, err
	}
	lease := runtimecontract.RuntimeDriveLease{WorkspaceID: ws, RunID: run, Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: runtime.SemanticRequestRepairDriveLeaseSignature(proof, response.Resolution), DueAt: time.Now().UTC().Format(time.RFC3339Nano)}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticRequestRepairResolved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: ws, RunID: run}, Authority: &runtime.RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof, RequestContractRepairResolution: &response.Resolution, RequestContractRepairDriveLease: &lease}})
	return updated.Aggregate, err == nil, err
}
