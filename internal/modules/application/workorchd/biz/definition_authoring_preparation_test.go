package biz

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAuthoringPreparationCapturePreservesCauseAndPersistenceFailure(t *testing.T) {
	for _, failure := range []error{nil, common.ErrConflict, &workspace.PreparationDiagnosticLimitError{Limit: 32}, errors.New("owner unavailable")} {
		t.Run(authoringPreparationPersistenceFailureClass(failure), func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			defer hub.Terminate(ctx)
			owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			defer owner.Unsubscribe(workspaceevents.TopicWorkspacePreparationDiagnosticRecord)
			cause := errors.New("原始失败\r\n bearer sk_test_marker \x00\xff\n")
			identity := workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: "ws", InputHash: "input", TemplateSnapshotHash: "template", ObservedWorkspaceRevision: 7, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive/input"}
			calls := 0
			if err := owner.Subscribe(workspaceevents.TopicWorkspacePreparationDiagnosticRecord, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				command := ev.Data().(workspaceevents.RecordPreparationDiagnosticCommand)
				if !bytes.Equal(command.Diagnostic.Cause, []byte(cause.Error())) || command.Diagnostic.Validate() != nil || command.Diagnostic.ObservedWorkspaceRevision != 7 {
					t.Error("pre-wrapper original cause bytes changed")
				}
				if failure != nil {
					result.Set(nil, def.WrapError(def.Unexpected, failure, "diagnostic persistence"))
					return
				}
				result.Set(workspaceevents.RecordPreparationDiagnosticResult{WorkspaceID: "ws", WorkspaceRevision: 8, Diagnostic: workspaceeventmapping.DefinitionAuthoringPreparationDiagnosticMetadataDTO(command.Diagnostic)}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			err := b.captureAuthoringPreparationFailure(ctx, identity, "config", cause)
			var captured *authoringPreparationError
			if !errors.As(err, &captured) || !errors.Is(err, cause) || calls != 1 || captured.Persisted != (failure == nil) || captured.PersistenceFailureClass != authoringPreparationPersistenceFailureClass(failure) || captured.DiagnosticID == "" {
				t.Fatalf("capture lost original or persistence status: %+v calls=%d", captured, calls)
			}
			if failure != nil && !errors.Is(err, failure) {
				t.Fatal("persistence failure missing from joined chain")
			}
			var logged bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
			b.logAuthoringCompilationFailure(ctx, "ws", "input", "derivation", err)
			slog.SetDefault(previous)
			if strings.Contains(logged.String(), "sk_test_marker") || strings.Contains(logged.String(), cause.Error()) || failure != nil && strings.Contains(logged.String(), "owner-queryable") {
				t.Fatalf("unsafe or unconfirmed persistence log: %s", logged.String())
			}
		})
	}
}

func TestAuthoringPreparationCaptureOversizeCauseIsHashedNotTruncated(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspacePreparationDiagnosticRecord)
	cause := errors.New(strings.Repeat("z", workspace.DefinitionAuthoringPreparationCauseLimit+1))
	identity := workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: "ws", InputHash: "input", TemplateSnapshotHash: "template", ObservedWorkspaceRevision: 1, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive/input"}
	if err := owner.Subscribe(workspaceevents.TopicWorkspacePreparationDiagnosticRecord, func(ev magicevent.Event, result magicevent.Result) {
		command := ev.Data().(workspaceevents.RecordPreparationDiagnosticCommand)
		if command.Diagnostic.Cause != nil || command.Diagnostic.CauseSaved || command.Diagnostic.CauseBytes != uint64(len(cause.Error())) || command.Diagnostic.CauseSHA256 == "" || command.Diagnostic.NotSavedReason == "" {
			t.Error("oversize original error was truncated or marked complete")
		}
		result.Set(workspaceevents.RecordPreparationDiagnosticResult{WorkspaceID: "ws", WorkspaceRevision: 2, Diagnostic: workspaceeventmapping.DefinitionAuthoringPreparationDiagnosticMetadataDTO(command.Diagnostic)}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	err := b.captureAuthoringPreparationFailure(ctx, identity, "config", cause)
	var captured *authoringPreparationError
	if !errors.As(err, &captured) || !captured.Persisted || err.Error() != cause.Error() || !errors.Is(err, cause) {
		t.Fatal("oversize boundary replaced original business failure")
	}
}

func TestAuthoringPreparationCaptureCancellationDoesNotCrossPersistenceBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cause := errors.New("原始配置失败")
	b := &Biz{}
	err := b.captureAuthoringPreparationFailure(ctx, workspace.DefinitionAuthoringPreparationDiagnostic{Stage: workspace.DefinitionAuthoringPreparationDerivation}, "config", cause)
	var captured *authoringPreparationError
	if !errors.As(err, &captured) || !errors.Is(err, cause) || !errors.Is(err, context.Canceled) || captured.Persisted || captured.PersistenceFailureClass != "cancelled" {
		t.Fatalf("cancelled persistence fabricated receipt: %v", err)
	}
}

func TestAuthoringPreparationBudgetCapturesOnlyTypedNoPermitRefusal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspacePreparationDiagnosticRecord)
	identity := workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: "ws", InputHash: "input", TemplateSnapshotHash: "template", ObservedWorkspaceRevision: 4, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive/input"}
	cause := errors.New("原始 request guard\r\n")
	calls := 0
	if err := owner.Subscribe(workspaceevents.TopicWorkspacePreparationDiagnosticRecord, func(ev magicevent.Event, result magicevent.Result) {
		calls++
		command := ev.Data().(workspaceevents.RecordPreparationDiagnosticCommand)
		if command.Diagnostic.PhaseCode != "request_budget" || !bytes.Equal(command.Diagnostic.Cause, []byte(cause.Error())) {
			t.Error("budget raw cause replaced by typed wrapper")
		}
		result.Set(workspaceevents.RecordPreparationDiagnosticResult{WorkspaceID: "ws", WorkspaceRevision: 5, Diagnostic: workspaceeventmapping.DefinitionAuthoringPreparationDiagnosticMetadataDTO(command.Diagnostic)}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	for _, failure := range []error{common.ErrConflict, errors.New("lost reserve response"), &workspace.DefinitionAuthoringBudgetRefusalError{Stage: workspace.DefinitionAuthoringPreparationReview, RequestID: identity.TargetRequestID, Budget: "request", Limit: 16, Cause: cause}} {
		if err := b.captureAuthoringBudgetRefusal(ctx, identity, failure); err != failure || calls != 0 {
			t.Fatal("unknown reserve status claimed preflight or retry authority")
		}
	}
	refusal := &workspace.DefinitionAuthoringBudgetRefusalError{Stage: identity.Stage, RequestID: identity.TargetRequestID, Budget: "request", Limit: 16, Cause: cause}
	err := b.captureAuthoringBudgetRefusal(ctx, identity, refusal)
	var captured *authoringPreparationError
	var typed *workspace.DefinitionAuthoringBudgetRefusalError
	if calls != 1 || !errors.As(err, &captured) || !captured.Persisted || !errors.As(err, &typed) || typed != refusal || !errors.Is(err, cause) {
		t.Fatalf("typed refusal lost no-permit meaning: %v", err)
	}
}

func TestAuthoringPreparationCurrentTracksNextUnreservedTargetOnly(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	identity := workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, ObservedWorkspaceRevision: 1, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive/" + record.InputHash, PhaseCode: "config"}
	deriveFailure := workspace.NewDefinitionAuthoringPreparationDiagnostic(identity, []byte("config failed"))
	record.PreparationDiagnostics = []workspace.DefinitionAuthoringPreparationDiagnostic{deriveFailure}
	if !authoringPreparationDiagnosticCurrent(record, nil, deriveFailure) {
		t.Fatal("next derivation preparation not current")
	}
	next, pending, _, err := record.ReserveDerivation("historical-pending", "binding", nil)
	if err != nil {
		t.Fatal(err)
	}
	query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 2, Record: authoringTestRecord(next), CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
	view, err := definitionAuthoringView(query)
	if err != nil || view.State != "outcome_unresolved" || view.PreparationDiagnostics[0].Current {
		t.Fatalf("non-stable pending was masked by preparation failure: %+v %v", view, err)
	}
	record, err = next.CompleteDerivation(pending.PermitID, nil, "provider_failed")
	if err != nil {
		t.Fatal(err)
	}
	record, derivation, _, err := record.ReserveDerivation(identity.TargetRequestID, "binding", nil)
	if err != nil {
		t.Fatal(err)
	}
	acceptance := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}}
	record, err = record.CompleteDerivation(derivation.PermitID, &acceptance, "")
	if err != nil {
		t.Fatal(err)
	}
	derivation = record.DerivationRequests[1]
	if authoringPreparationDiagnosticCurrent(record, &derivation, deriveFailure) {
		t.Fatal("reserved derivation retained current preparation failure")
	}
	mapping, audit, proposal, err := testkit.Proposal(derivation, workspace.DefinitionAuthoringVerificationMapping{Status: workspace.GoalVerificationUnverified, Declaration: "待评估"})
	if err != nil {
		t.Fatal(err)
	}
	record, request, _, err := record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal/" + derivation.PermitID, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "binding"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteSpecificationMapping(request.PermitID, mapping, audit)
	if err != nil {
		t.Fatal(err)
	}
	kinds := specifications.ReviewKinds()
	identity.Stage, identity.DerivationPermitID, identity.ProposalHash = workspace.DefinitionAuthoringPreparationReview, derivation.PermitID, proposal.ContentHash()
	identity.ReviewKind, identity.TargetRequestID = kinds[1], "review/"+kinds[1]+"/"+derivation.PermitID
	future := workspace.NewDefinitionAuthoringPreparationDiagnostic(identity, []byte("later review preparation failed"))
	if authoringPreparationDiagnosticCurrent(record, &derivation, future) {
		t.Fatal("future unreserved review was presented as current target")
	}
	identity.ReviewKind, identity.TargetRequestID = kinds[0], "review/"+kinds[0]+"/"+derivation.PermitID
	current := workspace.NewDefinitionAuthoringPreparationDiagnostic(identity, []byte("next review preparation failed"))
	if !authoringPreparationDiagnosticCurrent(record, &derivation, current) {
		t.Fatal("next unreserved review preparation not current")
	}
	record, _, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: identity.TargetRequestID, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: identity.ReviewKind, ProposalHash: identity.ProposalHash, ProviderBindingHash: "binding"})
	if err != nil {
		t.Fatal(err)
	}
	if authoringPreparationDiagnosticCurrent(record, &derivation, current) || authoringPreparationDiagnosticCurrent(record, &derivation, future) {
		t.Fatal("reserved pending review leaked preparation retry hint")
	}
}

func TestAuthoringPreparationRawViewKeepsExactBytesAndScope(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspacePreparationDiagnosticQuery)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
	raw := []byte("原文 sk_test_marker\r\n\x00\xff")
	diagnostic := workspace.NewDefinitionAuthoringPreparationDiagnostic(workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: "ws", InputHash: "input", TemplateSnapshotHash: "template", ObservedWorkspaceRevision: 2, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive/input", PhaseCode: "config"}, raw)
	if err := owner.Subscribe(workspaceevents.TopicWorkspacePreparationDiagnosticQuery, func(ev magicevent.Event, result magicevent.Result) {
		command := ev.Data().(workspaceevents.QueryPreparationDiagnosticCommand)
		result.Set(workspaceevents.QueryPreparationDiagnosticResult{WorkspaceID: command.WorkspaceID, Diagnostic: diagnostic}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(nil, def.WrapError(def.NotFound, common.ErrNotFound, "query unavailable"))
	}); err != nil {
		t.Fatal(err)
	}
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	view, err := b.ReadDefinitionAuthoringPreparationDiagnosticView(ctx, "ws", diagnostic.DiagnosticID)
	if err != nil || view.CauseBase64 != base64.StdEncoding.EncodeToString(raw) || view.CauseSHA256 != diagnostic.CauseSHA256 || view.CauseBytes != uint64(len(raw)) || !view.CauseSaved {
		t.Fatalf("raw diagnostic mutated: %+v %v", view, err)
	}
	if _, err := b.ReadDefinitionAuthoringPreparationDiagnosticView(ctx, "other-workspace", diagnostic.DiagnosticID); err == nil {
		t.Fatal("cross-workspace raw scope accepted")
	}
	if _, err := b.ReadDefinitionAuthoringPreparationDiagnosticView(ctx, "ws", "different-diagnostic"); err == nil {
		t.Fatal("different diagnostic ID accepted")
	}
}
