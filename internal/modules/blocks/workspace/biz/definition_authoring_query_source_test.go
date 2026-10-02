package workspace

import (
	"context"
	"reflect"
	"strings"
	"testing"

	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringQueryAndAdmissionRejectSourceTampering(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmationWithProof(t, owners, "ws-query-source", true)
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	if _, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}); err != nil {
		t.Fatal(err)
	}
	query := workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: current.ID}
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || view.AdmissionCandidate == nil || view.ActivationAdmission == nil {
		t.Fatalf("准入查询缺少来源: %v", err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*workspace.DefinitionAuthoringRecord){
		"request_identity": func(r *workspace.DefinitionAuthoringRecord) { r.SpecificationRequests[1].PermitID = "forged" },
		"proposal": func(r *workspace.DefinitionAuthoringRecord) {
			r.SpecificationRequests[0].Proposal.Requirements[0].Text += " changed"
		},
		"review": func(r *workspace.DefinitionAuthoringRecord) { r.SpecificationRequests[1].Review.Reason += " changed" },
		"derivation": func(r *workspace.DefinitionAuthoringRecord) {
			r.DerivationRequests[0].Template.Description += " changed"
		},
		"input": func(r *workspace.DefinitionAuthoringRecord) { r.Input.GoalText += " changed" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := stored.DefinitionAuthoring.Clone()
			change(&changed)
			for _, withCandidate := range []bool{false, true} {
				observation := view
				changedDTO := workspaceeventmapping.DefinitionAuthoringRecordDTO(changed)
				observation.Record = &changedDTO
				if !withCandidate {
					observation.AdmissionCandidate, observation.AdmissionResult, observation.ActivationAdmission = nil, nil, nil
				}
				if observation.ValidateFor(query) == nil {
					t.Fatalf("查询忽略坏来源: candidate=%t", withCandidate)
				}
			}
			aggregate := *stored
			aggregate.DefinitionAuthoring = &changed
			if _, _, err := aggregate.ValidateActivationAdmissionPersistence(); err == nil {
				t.Fatal("持久准入忽略坏来源")
			}
		})
	}
	for name, change := range map[string]func(*workspaceevents.QueryDefinitionAuthoringResult){
		"missing_record":       func(r *workspaceevents.QueryDefinitionAuthoringResult) { r.Record = nil },
		"missing_confirmation": func(r *workspaceevents.QueryDefinitionAuthoringResult) { r.Confirmation = nil },
		"candidate_ref": func(r *workspaceevents.QueryDefinitionAuthoringResult) {
			candidate := r.AdmissionCandidate.Clone()
			candidate.AuthoringRef.ProposalHash = "forged"
			r.AdmissionCandidate = &candidate
		},
		"confirmation_hash": func(r *workspaceevents.QueryDefinitionAuthoringResult) {
			confirmation := r.Confirmation.Clone()
			confirmation.SpecificationHash = "forged"
			r.Confirmation = &confirmation
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := view
			change(&changed)
			if changed.ValidateFor(query) == nil {
				t.Fatal("查询接受缺失或替换的来源")
			}
		})
	}
	t.Run("missing_predecessor_history", func(t *testing.T) {
		journal := stored.DefinitionAuthoringConfirmation.Clone()
		journal.FormalSequence, journal.PreviousInputHash = 2, "unobserved-predecessor"
		journal.Goal.PreviousGoalID, journal.Goal.PreviousDefinitionHash = "unobserved-goal", "unobserved-hash"
		journal.DoD.PreviousDoDID, journal.DoD.PreviousDefinitionHash = "unobserved-dod", "unobserved-hash"
		journal.Goal.DefinitionRevision = strings.Replace(journal.Goal.DefinitionRevision, ":00000000000000000001:", ":00000000000000000002:", 1)
		journal.DoD.DefinitionRevision = strings.Replace(journal.DoD.DefinitionRevision, ":00000000000000000001:", ":00000000000000000002:", 1)
		if journal.Validate(stored.DefinitionAuthoring, stored.ID) != nil {
			t.Fatal("无历史 fixture 应有结构合法的后继 journal")
		}
		for _, sealed := range []bool{false, true} {
			aggregate := *stored
			aggregate.DefinitionAuthoringConfirmation = &journal
			if !sealed {
				aggregate.StartupInput, aggregate.ActivationAdmission = nil, nil
			}
			if _, _, err := aggregate.ValidateActivationAdmissionPersistence(); err == nil {
				t.Fatalf("准入接受没有历史锚点的后继: sealed=%t", sealed)
			}
		}
		observation := view
		observation.Confirmation = &journal
		observation.AdmissionCandidate, observation.AdmissionResult, observation.ActivationAdmission = nil, nil, nil
		if observation.ValidateFor(query) == nil {
			t.Fatal("查询接受没有历史锚点的后继")
		}
	})
	after, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || !reflect.DeepEqual(stored, after) {
		t.Fatalf("只读反证改变 owner: %v", err)
	}
}
