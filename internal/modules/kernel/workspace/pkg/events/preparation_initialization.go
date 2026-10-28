package events

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspacePreparationInitializeResources = "/workspace/activation-preparation/resources/initialize"
const TopicWorkspacePreparationRecoverResources = "/workspace/activation-preparation/resources/recover"

type InitializeWorkspacePreparationCommand struct {
	WorkspaceID               common.WorkspaceID
	RequestID                 string
	ExpectedWorkspaceRevision uint64
}

func (c InitializeWorkspacePreparationCommand) Validate() error {
	if len(c.RequestID) > 256 {
		return common.ErrInvalidArgument
	}
	return (CheckWorkspacePreparationCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision}).Validate()
}

type InitializeWorkspacePreparationResult struct {
	RequestID                 string
	ExpectedWorkspaceRevision uint64
	State                     QueryWorkspacePreparationStateResult
}

func (r InitializeWorkspacePreparationResult) ValidateFor(c InitializeWorkspacePreparationCommand) error {
	if c.Validate() != nil || r.RequestID != c.RequestID || r.ExpectedWorkspaceRevision != c.ExpectedWorkspaceRevision || r.State.ValidateFor(QueryWorkspacePreparationStateCommand{WorkspaceID: c.WorkspaceID}) != nil {
		return common.ErrConflict
	}
	for _, v := range r.State.ResourceInitializations {
		if v.RequestID == c.RequestID && v.ExpectedWorkspaceRevision == c.ExpectedWorkspaceRevision {
			return nil
		}
	}
	return common.ErrConflict
}

// Only bounded metadata leaves the private initialization journal.
type PreparationInitializationView struct {
	RequestID                  string `json:"requestId"`
	ExpectedWorkspaceRevision  uint64 `json:"expectedWorkspaceRevision"`
	CommittedWorkspaceRevision uint64 `json:"committedWorkspaceRevision"`
	State                      string `json:"state"`
	Attempts                   int    `json:"attempts"`
}

func (v PreparationInitializationView) Validate(revision uint64) error {
	if (CheckWorkspacePreparationCommand{WorkspaceID: "scope", RequestID: v.RequestID, ExpectedWorkspaceRevision: v.ExpectedWorkspaceRevision}).Validate() != nil || v.CommittedWorkspaceRevision <= v.ExpectedWorkspaceRevision || v.CommittedWorkspaceRevision > revision || v.Attempts < 1 || v.Attempts > workspace.PreparationInitializationResultLimit || v.State != "pending" && v.State != "blocked" && v.State != "completed" {
		return common.ErrConflict
	}
	return nil
}
