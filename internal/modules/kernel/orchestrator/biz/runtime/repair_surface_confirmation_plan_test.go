package runtime

import (
	"context"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCurrentRuntimeRepairSurfaceConfirmationPlanEligibilityMatrix(t *testing.T) {
	t.Run("terminal and exhausted facts cannot reopen a confirmation obligation", func(t *testing.T) {
		aggregate, _ := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("9", 64))
		for name, mutate := range map[string]func(*RuntimeAggregate){
			"sealed terminal": func(a *RuntimeAggregate) {
				a.Decision = terminalDecision(a.Decision, runtimecontract.ConvergenceStateFailed, "target_grounding_unresolved", "already sealed")
			},
			"repair budget exhausted": func(a *RuntimeAggregate) { a.Facts.Repair.Exhausted = true },
		} {
			t.Run(name, func(t *testing.T) {
				changed := cloneRuntimeAggregateForMCPProvenanceTest(t, aggregate)
				mutate(&changed)
				obligation, _, err := runtimeRepairSurfaceConfirmationObligation(changed)
				if err != nil || obligation.OwnerModuleID != "" {
					t.Fatalf("closed authority reopened owner obligation: %+v err=%v", obligation, err)
				}
			})
		}
	})

	t.Run("historical or uncommitted proof cannot open a query", func(t *testing.T) {
		aggregate, _ := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("8", 64))
		for name, mutate := range map[string]func(*RuntimeAggregate){
			"other focus":           func(a *RuntimeAggregate) { a.Facts.Focus.ID = "other-focus" },
			"other definition":      func(a *RuntimeAggregate) { a.Facts.DefinitionRevision = "other-definition" },
			"retired binding":       func(a *RuntimeAggregate) { a.Facts.Coverage[0].CurrentBindingRef.BindingID = "other-binding" },
			"missing exact receipt": func(a *RuntimeAggregate) { a.OperationReceipts = nil },
		} {
			t.Run(name, func(t *testing.T) {
				changed := cloneRuntimeAggregateForMCPProvenanceTest(t, aggregate)
				mutate(&changed)
				plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(changed)
				if err == nil || plan.Required() {
					t.Fatalf("invalid lineage opened a confirmation query: plan=%+v err=%v", plan, err)
				}
			})
		}
	})

	t.Run("pending observation requires one owner query", func(t *testing.T) {
		aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("1", 64))
		var requestID string
		for name, candidate := range map[string]RuntimeAggregate{
			"current":                         aggregate,
			"restart-before-confirmation-CAS": cloneRuntimeAggregateForMCPProvenanceTest(t, aggregate),
		} {
			plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(candidate)
			if err != nil || !plan.Required() ||
				!runtimecontract.SameRuntimeValidationFailureObservation(plan.Observation, observation) ||
				plan.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
				plan.Protocol != runtimecontract.RuntimeRepairSurfaceConfirmationProtocolFileWorkspaceRegularTargets ||
				plan.SubjectDomain != runtimecontract.RuntimeResourceWorkspaceTree ||
				plan.SubjectRef != runtimecontract.RuntimeRepairSurfaceFileWorkspaceSubjectRef ||
				plan.ExpectedSubjectRevision == "" {
				t.Fatalf("%s pending repair-surface plan = %+v err=%v", name, plan, err)
			}
			if requestID == "" {
				requestID = plan.RequestID
			} else if plan.RequestID != requestID || plan.IssuedAtStateRevision != aggregate.StateRevision {
				t.Fatalf("%s reload changed confirmation request fence: %+v", name, plan)
			}
		}
	})

	t.Run("stale-only history remains refreshable", func(t *testing.T) {
		aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("2", 64), func(aggregate *RuntimeAggregate) {
			aggregate.Facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 2
		})
		beforeRetries := aggregate.Facts.Repair.ValidationOnlyRetries
		if beforeRetries >= aggregate.Facts.ConvergencePolicy.Normalized().MaxValidationOnlyRetriesPerFocus {
			t.Fatalf("stale-refresh fixture has no remaining validation-only allowance: repair=%+v policy=%+v", aggregate.Facts.Repair, aggregate.Facts.ConvergencePolicy)
		}
		repository := &durableRuntimeStateRepository{aggregate: aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		confirmation := bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, runtimecontract.RuntimeRepairSurfaceConfirmation{
			Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
			Disposition: runtimecontract.RuntimeRepairSurfaceStale, ObservedRevision: "tree-stale-v1", Reason: "owner revision changed",
		})
		stale, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
		if err != nil {
			t.Fatal(err)
		}
		if plan, planErr := CurrentRuntimeRepairSurfaceConfirmationPlan(stale); planErr != nil || plan.Required() {
			t.Fatalf("stale observation queried before prerequisite reacquisition: plan=%+v err=%v", plan, planErr)
		}
		var snapshotPrerequisiteID string
		for _, prerequisite := range stale.Facts.Prerequisites.Pending {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "main.go" {
				snapshotPrerequisiteID = prerequisite.ID
				break
			}
		}
		if snapshotPrerequisiteID == "" {
			t.Fatalf("stale observation did not request current target snapshot: %+v", stale.Facts.Prerequisites.Pending)
		}
		refreshed, err := controller.commitPrerequisite(context.Background(), stale.Facts.WorkspaceID, stale.Facts.RunID, PrerequisiteResult{
			PrerequisiteID: snapshotPrerequisiteID,
			TargetSnapshot: &runtimecontract.TargetSnapshot{
				TargetRef: "main.go", Content: "package main", ContentHash: "content-refreshed-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		refreshed = commitRefreshedFileRepairSurfaceCatalogForTest(t, controller, aggregate, refreshed, "tree-refreshed-v2")
		if len(refreshed.Facts.Prerequisites.Pending) != 0 || refreshed.Decision.State != runtimecontract.ConvergenceStateNeedValidation ||
			refreshed.Facts.Repair.ValidationOnlyRetries != beforeRetries {
			t.Fatalf("stale prerequisite refresh did not return to a budget-neutral validation handoff: state=%s terminal=%#v pending=%+v directive=%+v repair=%+v", refreshed.Decision.State, refreshed.Decision.Terminal, refreshed.Facts.Prerequisites.Pending, refreshed.Facts.RepairDirective, refreshed.Facts.Repair)
		}
		for name, candidate := range map[string]RuntimeAggregate{
			"current": refreshed,
			"reload":  cloneRuntimeAggregateForMCPProvenanceTest(t, refreshed),
		} {
			plan, planErr := CurrentRuntimeRepairSurfaceConfirmationPlan(candidate)
			if planErr != nil || !plan.Required() || !runtimecontract.SameRuntimeValidationFailureObservation(plan.Observation, observation) {
				t.Fatalf("%s stale refresh plan = %+v err=%v", name, plan, planErr)
			}
		}
	})

	t.Run("administratively terminated runtime cannot query an owner", func(t *testing.T) {
		aggregate, _ := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("6", 64))
		repository := &durableRuntimeStateRepository{aggregate: aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		terminated, err := controller.requestTermination(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID,
			runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the run")
		if err != nil {
			t.Fatal(err)
		}
		plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(terminated)
		if err != nil || plan.Required() {
			t.Fatalf("terminated runtime repair-surface plan = %+v err=%v", plan, err)
		}
	})

	t.Run("confirmed and rejected histories are terminal", func(t *testing.T) {
		for _, disposition := range []runtimecontract.RuntimeRepairSurfaceDisposition{
			runtimecontract.RuntimeRepairSurfaceConfirmed,
			runtimecontract.RuntimeRepairSurfaceRejected,
		} {
			t.Run(string(disposition), func(t *testing.T) {
				aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("3", 64))
				repository := &durableRuntimeStateRepository{aggregate: aggregate}
				controller, err := NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
					Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
					Disposition: disposition, ObservedRevision: "tree-terminal-v1",
				}
				confirmation = bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, confirmation)
				if disposition == runtimecontract.RuntimeRepairSurfaceConfirmed {
					confirmation.ConfirmedTargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
				} else {
					confirmation.Reason = "owner rejected the candidate surface"
				}
				terminal, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(terminal)
				if err != nil || plan.Required() {
					t.Fatalf("terminal %s repair-surface plan = %+v err=%v", disposition, plan, err)
				}
			})
		}
	})

	t.Run("confirmed repair partial and fresh validation never reopen confirmation", func(t *testing.T) {
		fixture := newConfirmedRepairMultiOperationFixture(t)
		coordinator, err := NewDispatchCoordinator(fixture.repository)
		if err != nil {
			t.Fatal(err)
		}

		first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
		attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
		partial, err := coordinator.commitEvidence(context.Background(), first)
		if err != nil {
			t.Fatal(err)
		}
		if partial.Aggregate.ActiveDispatch == nil || partial.Aggregate.Facts.Resources.FailureTarget != nil ||
			len(partial.Aggregate.RepairSurfaceConfirmations) == 0 {
			t.Fatalf("partial repair fixture lost terminal confirmation lineage: %+v", partial.Aggregate)
		}
		plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(partial.Aggregate)
		if err != nil || plan.Required() {
			t.Fatalf("partial repair reopened confirmation: plan=%+v err=%v", plan, err)
		}

		second := fixture.acceptedEvidence(t, 1, "tree-v2", "tree-v3")
		attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &second, "tree-v2", "tree-v3")
		settled, err := coordinator.commitEvidence(context.Background(), second)
		if err != nil {
			t.Fatal(err)
		}
		if settled.Aggregate.ActiveDispatch != nil || settled.Aggregate.Facts.Resources.FailureTarget != nil ||
			settled.Aggregate.Facts.RepairDirective == nil || len(settled.Aggregate.RepairSurfaceConfirmations) == 0 {
			t.Fatalf("settled repair fixture lost fresh-validation lineage: %+v", settled.Aggregate)
		}
		for name, aggregate := range map[string]RuntimeAggregate{
			"current": settled.Aggregate,
			"reload":  cloneRuntimeAggregateForMCPProvenanceTest(t, settled.Aggregate),
		} {
			plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(aggregate)
			if err != nil || plan.Required() {
				t.Fatalf("%s fresh-validation aggregate reopened confirmation: plan=%+v err=%v", name, plan, err)
			}
		}
	})
}

func TestRuntimeRepairSurfaceConfirmationRequestIDBindsRevisionAndRefreshLineage(t *testing.T) {
	aggregate, _ := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("4", 64))
	plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(aggregate)
	if err != nil || !plan.Required() {
		t.Fatalf("repair-surface plan = %+v err=%v", plan, err)
	}
	same := stableRuntimeRepairSurfaceConfirmationRequestID(plan.Observation, plan.OwnerModuleID, plan.Protocol, plan.SubjectRef, plan.ExpectedSubjectRevision, plan.IssuedAtStateRevision, "initial")
	changedRevision := stableRuntimeRepairSurfaceConfirmationRequestID(plan.Observation, plan.OwnerModuleID, plan.Protocol, plan.SubjectRef, plan.ExpectedSubjectRevision, plan.IssuedAtStateRevision+1, "initial")
	changedSubjectRevision := stableRuntimeRepairSurfaceConfirmationRequestID(plan.Observation, plan.OwnerModuleID, plan.Protocol, plan.SubjectRef, plan.ExpectedSubjectRevision+"-changed", plan.IssuedAtStateRevision, "initial")
	changedRefresh := stableRuntimeRepairSurfaceConfirmationRequestID(plan.Observation, plan.OwnerModuleID, plan.Protocol, plan.SubjectRef, plan.ExpectedSubjectRevision, plan.IssuedAtStateRevision, "stale-response-signature")
	if same != plan.RequestID || changedRevision == same || changedSubjectRevision == same || changedRefresh == same || changedRevision == changedRefresh {
		t.Fatalf("request identity did not bind state revision, subject revision and refresh lineage: plan=%q same=%q state=%q subject=%q refresh=%q", plan.RequestID, same, changedRevision, changedSubjectRevision, changedRefresh)
	}
}
