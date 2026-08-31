package result

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) lifecycleTopics() []string {
	return []string{
		resultevents.TopicResultFactTerminalDelivery,
		resultevents.TopicResultCommandSealWorkspaceDeletion,
		resultevents.TopicResultCommandPurgeWorkspace,
	}
}

func (m *Biz) handleLifecycleCommand(ev magicevent.Event, result magicevent.Result) {
	workspaceID, ok := resultEventWorkspaceID(ev)
	if !ok {
		setCommandResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	unlock := m.lockWorkspaceMutation(workspaceID)
	defer unlock()
	if ev.ID() == resultevents.TopicResultCommandSealWorkspaceDeletion || ev.ID() == resultevents.TopicResultCommandPurgeWorkspace {
		response, err := m.executeWorkspaceDeletionCommand(ev.Context(), ev)
		setCommandResult(result, response, err)
		return
	}
	var response resultevents.ResultLifecycleCommandResult
	err := m.workspaceDeletions.WithWorkspaceActiveFence(ev.Context(), workspaceID, func(guarded context.Context) error {
		var executeErr error
		response, executeErr = m.executeLifecycleCommand(guarded, ev)
		return executeErr
	})
	setResultLifecycleResult(result, response, err)
}

func resultEventWorkspaceID(ev magicevent.Event) (common.WorkspaceID, bool) {
	if ev == nil {
		return "", false
	}
	switch command := ev.Data().(type) {
	case resultevents.RuntimeTerminalDeliveryFact:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.SealWorkspaceResultDeletionCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.PurgeWorkspaceResultsCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	default:
		return "", false
	}
}

func setResultLifecycleResult(result magicevent.Result, data resultevents.ResultLifecycleCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	data.Result = resultevents.CloneResult(data.Result)
	if data.DeliveryAck != nil {
		ack := *data.DeliveryAck
		data.DeliveryAck = &ack
	}
	result.Set(data, nil)
}

func (m *Biz) executeLifecycleCommand(ctx context.Context, ev magicevent.Event) (resultevents.ResultLifecycleCommandResult, error) {
	if m == nil || m.service == nil {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if ev.ID() != resultevents.TopicResultFactTerminalDelivery {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(resultevents.RuntimeTerminalDeliveryFact)
	if !ok || ev.Source() != "kernel/orchestrator" {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	boundCtx, err := resultCommandContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	return m.submitLifecycleInternalEvent(boundCtx, lifecycleInternalEvent{
		Kind:             lifecycleInternalEventTerminalDeliveryObserved,
		TerminalDelivery: &cmd,
	})
}

func (m *Biz) applyTerminalDelivery(ctx context.Context, cmd resultevents.RuntimeTerminalDeliveryFact) (resultevents.ResultLifecycleCommandResult, error) {
	if err := cmd.Validate(); err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	cmd = cmd.Normalized()
	material := cmd.Material.Normalized()
	materialHash, err := resultevents.CanonicalResultMaterialHash(material)
	if err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	ctx, err = resultCommandContext(ctx, material.WorkspaceID)
	if err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	resultID := canonicalRuntimeResultID(material.WorkspaceID, material.RunID)
	artifacts := terminalArtifacts(cmd, resultID, material.ArtifactRefs)
	artifactIDs := make([]common.ArtifactID, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifactIDs = append(artifactIDs, artifact.ID)
	}
	targetState, err := resultStateForTerminalOutcome(cmd.Outcome.Kind)
	if err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}

	// The caller supplies sealed material only. The owner derives Result ID,
	// child Artifact IDs, lifecycle state, timestamps and immutable fences.
	item := resultFromTerminalMaterial(material, resultID, artifactIDs, materialHash)
	identity := cmd.Identity().Normalized()
	item.TerminalDelivery = &identity
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.State = common.ResultStateDraft
	item.CreatedAt = now
	item.UpdatedAt = now
	// Without child metadata there is no partial aggregate to reconcile, so
	// persist the terminal state in one write. Deliveries with formal Artifact
	// children use draft as an owner-private pending state until every child is
	// durable, then complete the explicit draft -> accepted/rejected CAS.
	if len(artifacts) == 0 {
		if err := applyTerminalResultTransition(item, targetState, now); err != nil {
			return resultevents.ResultLifecycleCommandResult{}, err
		}
	}

	created := false
	existing, err := m.service.GetResult(ctx, resultID)
	if errors.Is(err, common.ErrNotFound) {
		if createErr := m.service.CreateResult(ctx, item); createErr != nil {
			if !errors.Is(createErr, common.ErrConflict) {
				return resultevents.ResultLifecycleCommandResult{}, createErr
			}
		} else {
			created = true
		}
		existing, err = m.service.GetResult(ctx, resultID)
	}
	if err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	if !sameTerminalResult(existing, item) {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrConflict
	}
	if err := m.ensureTerminalArtifacts(ctx, artifacts); err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}

	if existing.State == targetState {
		// A persisted terminal Result may have lost its asynchronous notification
		// before the SM-4 acknowledgement reached Orchestrator. Re-emit the same
		// stable notification before acknowledging an idempotent replay.
		m.publishResultChanged(ctx, existing)
		acknowledgedState := runtimecontract.RuntimeDeliveryAcknowledgedNoop
		if created {
			acknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded
		}
		return resultevents.ResultLifecycleCommandResult{
			Result:       existing,
			Transitioned: created,
			DeliveryAck:  resultTerminalDeliveryAck(existing, cmd, acknowledgedState),
		}, nil
	}
	if existing.State != common.ResultStateDraft {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrConflict
	}
	finalized := resultevents.CloneResult(existing)
	if err := applyTerminalResultTransition(finalized, targetState, now); err != nil {
		return resultevents.ResultLifecycleCommandResult{}, err
	}
	transitioned := true
	if err := m.service.UpdateResult(ctx, finalized); err != nil {
		if !errors.Is(err, common.ErrConflict) {
			return resultevents.ResultLifecycleCommandResult{}, err
		}
		winner, getErr := m.service.GetResult(ctx, resultID)
		if getErr != nil || !sameTerminalResult(winner, item) || winner.State != targetState {
			return resultevents.ResultLifecycleCommandResult{}, common.ErrConflict
		}
		finalized = winner
		transitioned = false
	}
	m.publishResultChanged(ctx, finalized)
	return resultevents.ResultLifecycleCommandResult{
		Result:       finalized,
		Transitioned: transitioned,
		DeliveryAck:  resultTerminalDeliveryAck(finalized, cmd, runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded),
	}, nil
}

func resultFromTerminalMaterial(material resultevents.ResultMaterial, resultID common.ResultID, artifactIDs []common.ArtifactID, materialHash string) *resultmodel.Result {
	material = material.Normalized()
	// Normalized returns detached slices and nested values, so the owner can
	// map raw artifact references to canonical child IDs without a second full
	// CloneResult pass.
	item := &resultmodel.Result{
		ID: resultID, WorkspaceID: material.WorkspaceID, RunID: material.RunID,
		GoalID: material.GoalID, DoDID: material.DoDID, Title: material.Title, Summary: material.Summary,
		SourceType: material.SourceType, SourceRef: material.SourceRef,
		EvidenceRefs: material.EvidenceRefs, ArtifactRefs: artifactIDs,
		DoDAssessment: material.DoDAssessment, SummaryDetail: material.SummaryDetail, RuntimeCloseout: material.RuntimeCloseout,
		TerminalMaterialHash: materialHash,
	}
	canonicalizeResultArtifactRefs(item, material.ArtifactRefs, artifactIDs)
	return item
}

func canonicalizeResultArtifactRefs(item *resultmodel.Result, sourceRefs, artifactIDs []common.ArtifactID) {
	if item == nil {
		return
	}
	bySource := make(map[common.ArtifactID]common.ArtifactID, len(sourceRefs))
	for idx, sourceRef := range sourceRefs {
		if idx < len(artifactIDs) {
			bySource[sourceRef] = artifactIDs[idx]
		}
	}
	mapRefs := func(values []common.ArtifactID) []common.ArtifactID {
		if len(values) == 0 {
			return nil
		}
		ret := make([]common.ArtifactID, 0, len(values))
		for _, value := range values {
			if canonical, ok := bySource[value]; ok {
				ret = append(ret, canonical)
			}
		}
		if len(ret) == 0 {
			return nil
		}
		return ret
	}
	item.ArtifactRefs = append([]common.ArtifactID(nil), artifactIDs...)
	item.SummaryDetail.ArtifactRefs = mapRefs(item.SummaryDetail.ArtifactRefs)
	// RuntimeCloseout is formal Orchestrator authority and its hash binds the
	// sealed source references. Keep those refs byte-for-byte canonical here;
	// canonical child IDs belong only to the Result-owner projection surfaces.
	for _, coverage := range []*workorchresultcontract.DoDCoverage{
		item.DoDAssessment.GoalCoverage,
		item.DoDAssessment.AcceptanceDimensions,
		item.DoDAssessment.DeliverableCoverage,
	} {
		if coverage == nil {
			continue
		}
		for idx := range coverage.Items {
			coverage.Items[idx].ArtifactRefs = mapRefs(coverage.Items[idx].ArtifactRefs)
		}
	}
}

func canonicalRuntimeResultID(workspaceID common.WorkspaceID, runID common.RunID) common.ResultID {
	return runtimecontract.RuntimeTerminalResultID(workspaceID, runID)
}

func canonicalRuntimeArtifactID(resultID common.ResultID, sourceRef string) common.ArtifactID {
	digest := sha256.Sum256([]byte(strings.TrimSpace(sourceRef)))
	return common.ArtifactID(fmt.Sprintf("%s/artifact/%s", resultID, hex.EncodeToString(digest[:])))
}

func terminalArtifacts(cmd resultevents.RuntimeTerminalDeliveryFact, resultID common.ResultID, sourceRefs []common.ArtifactID) []*resultmodel.Artifact {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ret := make([]*resultmodel.Artifact, 0, len(sourceRefs))
	for _, rawRef := range sourceRefs {
		sourceRef := strings.TrimSpace(string(rawRef))
		if sourceRef == "" {
			continue
		}
		id := canonicalRuntimeArtifactID(resultID, sourceRef)
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", cmd.WorkspaceID, cmd.RunID, resultID, cmd.TerminalDeliveryID, sourceRef)))
		ret = append(ret, &resultmodel.Artifact{
			ID: id, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, ResultID: resultID,
			ArtifactType: "runtime_reference", Category: "terminal", URI: sourceRef,
			State: common.ArtifactStateAvailable, SourceRef: sourceRef,
			TerminalDeliveryID: cmd.TerminalDeliveryID,
			MaterialHash:       "artifact-material-sha256:" + hex.EncodeToString(digest[:]),
			CreatedAt:          now, UpdatedAt: now,
		})
	}
	return ret
}

func (m *Biz) ensureTerminalArtifacts(ctx context.Context, expected []*resultmodel.Artifact) error {
	for _, item := range expected {
		current, err := m.service.GetArtifact(ctx, item.ID)
		if errors.Is(err, common.ErrNotFound) {
			if createErr := m.service.createTerminalArtifact(ctx, item); createErr != nil && !errors.Is(createErr, common.ErrConflict) {
				return createErr
			}
			current, err = m.service.GetArtifact(ctx, item.ID)
		}
		if err != nil {
			return err
		}
		if !sameTerminalArtifact(current, item) {
			return common.ErrConflict
		}
	}
	return nil
}

func sameTerminalArtifact(left, right *resultmodel.Artifact) bool {
	return left != nil && right != nil && left.ID == right.ID &&
		left.WorkspaceID == right.WorkspaceID && left.RunID == right.RunID && left.ResultID == right.ResultID &&
		left.ArtifactType == right.ArtifactType && left.Category == right.Category && left.State == right.State &&
		strings.TrimSpace(left.SourceRef) == strings.TrimSpace(right.SourceRef) &&
		strings.TrimSpace(left.URI) == strings.TrimSpace(right.URI) &&
		strings.TrimSpace(left.TerminalDeliveryID) == strings.TrimSpace(right.TerminalDeliveryID) &&
		strings.TrimSpace(left.MaterialHash) == strings.TrimSpace(right.MaterialHash)
}

func sameTerminalResult(left, right *resultmodel.Result) bool {
	return left != nil && right != nil && left.ID == right.ID &&
		left.WorkspaceID == right.WorkspaceID && left.RunID == right.RunID &&
		left.GoalID == right.GoalID && left.DoDID == right.DoDID &&
		left.Title == right.Title && left.Summary == right.Summary &&
		left.SourceType == right.SourceType && left.SourceRef == right.SourceRef &&
		reflect.DeepEqual(left.EvidenceRefs, right.EvidenceRefs) &&
		reflect.DeepEqual(left.ArtifactRefs, right.ArtifactRefs) &&
		reflect.DeepEqual(left.DoDAssessment, right.DoDAssessment) &&
		reflect.DeepEqual(left.SummaryDetail, right.SummaryDetail) &&
		reflect.DeepEqual(left.RuntimeCloseout, right.RuntimeCloseout) &&
		strings.TrimSpace(left.TerminalMaterialHash) == strings.TrimSpace(right.TerminalMaterialHash) &&
		sameTerminalDeliveryIdentity(left.TerminalDelivery, right.TerminalDelivery)
}

func resultStateForTerminalOutcome(kind runtimecontract.TerminalOutcomeKind) (common.ResultState, error) {
	switch kind {
	case runtimecontract.TerminalOutcomeKindCompleted:
		return common.ResultStateAccepted, nil
	case runtimecontract.TerminalOutcomeKindFailed,
		runtimecontract.TerminalOutcomeKindCancelled,
		runtimecontract.TerminalOutcomeKindSuperseded:
		return common.ResultStateRejected, nil
	default:
		return "", common.ErrInvalidArgument
	}
}

func applyTerminalResultTransition(item *resultmodel.Result, target common.ResultState, now string) error {
	if item == nil || !validResultTransition(item.State, target) || (target != common.ResultStateAccepted && target != common.ResultStateRejected) {
		return common.ErrConflict
	}
	item.State = target
	item.UpdatedAt = now
	if target == common.ResultStateAccepted {
		item.AcceptedAt = now
		item.RejectedAt = ""
		item.AcceptedBy = ModuleID
	} else {
		item.RejectedAt = now
		item.AcceptedAt = ""
		item.AcceptedBy = ""
	}
	return nil
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
		l.DispatchID == r.DispatchID && l.Outcome == r.Outcome
}

func resultTerminalDeliveryAck(
	item *resultmodel.Result,
	cmd resultevents.RuntimeTerminalDeliveryFact,
	state runtimecontract.RuntimeDeliveryAcknowledgedState,
) *runtimecontract.RuntimeDeliveryAcknowledgement {
	if item == nil {
		return nil
	}
	cmd = cmd.Normalized()
	ack := runtimecontract.RuntimeDeliveryAcknowledgement{
		WorkspaceID:         item.WorkspaceID,
		RunID:               item.RunID,
		ResultID:            item.ID,
		TerminalDeliveryID:  cmd.TerminalDeliveryID,
		Owner:               runtimecontract.RuntimeDeliveryOwnerResult,
		AcknowledgedState:   state,
		AuthoritySnapshotID: cmd.AuthoritySnapshotID,
		CausationID:         cmd.CausationID,
		Outcome:             cmd.Outcome,
	}.Normalized()
	return &ack
}
