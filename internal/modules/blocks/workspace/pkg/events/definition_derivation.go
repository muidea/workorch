package events

import (
	"reflect"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceDerivationReserve = "/workspace/definition-derivation/reserve"
const TopicWorkspaceDerivationComplete = "/workspace/definition-derivation/complete"

type ReserveDefinitionDerivationCommand struct {
	WorkspaceID          common.WorkspaceID
	WorkspaceRevision    uint64
	InputHash            string
	RequestID            string
	Resources            []workspace.DefinitionAuthoringResourceSnapshot
	ResourceSnapshotHash string
	ProviderBindingHash  string
}

func (c ReserveDefinitionDerivationCommand) Validate() error {
	if c.WorkspaceID == "" || c.InputHash == "" || len(c.InputHash) > 256 || c.RequestID == "" || len(c.RequestID) > 128 || c.ProviderBindingHash == "" || len(c.ProviderBindingHash) > 256 || len(c.Resources) > workspace.DefinitionAuthoringResourceLimit || c.ResourceSnapshotHash != workspace.DefinitionAuthoringResourcesHash(c.Resources) || (len(c.Resources) > 0 && c.WorkspaceRevision == 0) {
		return common.ErrInvalidArgument
	}
	return nil
}

type CompleteDefinitionDerivationCommand struct {
	WorkspaceID common.WorkspaceID
	PermitID    string
	Proposal    *workspace.DefinitionDerivedAcceptance
	FailureCode string
}

func (c CompleteDefinitionDerivationCommand) Validate() error {
	if c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256 || (c.Proposal == nil) == (c.FailureCode == "") {
		return common.ErrInvalidArgument
	}
	if c.FailureCode != "" && c.FailureCode != "provider_failed" && c.FailureCode != "invalid_proposal" {
		return common.ErrInvalidArgument
	}
	return nil
}

// FreshPermitID 仅在首次记账 CAS 成功后返回；Request 中的身份不能作为重发许可。
type DefinitionDerivationRequestResult struct {
	Request       workspace.DefinitionDerivationRequest
	FreshPermitID string
	Consumed      uint32
	Remaining     uint32
}

func (r DefinitionDerivationRequestResult) validate() error {
	if err := r.Request.Validate(); err != nil {
		return err
	}
	if r.Consumed == 0 || r.Consumed > workspace.DefinitionDerivationRequestLimit || r.Remaining != workspace.DefinitionDerivationRequestLimit-r.Consumed {
		return common.ErrConflict
	}
	if r.FreshPermitID != "" && (r.FreshPermitID != r.Request.PermitID || r.Request.Proposal != nil || r.Request.FailureCode != "") {
		return common.ErrConflict
	}
	return nil
}

func (r DefinitionDerivationRequestResult) ValidateReserve(c ReserveDefinitionDerivationCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := r.validate(); err != nil {
		return err
	}
	if r.Request.Input.WorkspaceID != c.WorkspaceID || r.Request.Input.InputHash() != c.InputHash || r.Request.RequestID != c.RequestID || r.Request.ProviderBindingHash != c.ProviderBindingHash || r.Request.ResourceSnapshotHash != c.ResourceSnapshotHash || !reflect.DeepEqual(r.Request.Resources, c.Resources) {
		return common.ErrConflict
	}
	return nil
}

func (r DefinitionDerivationRequestResult) ValidateCompletion(c CompleteDefinitionDerivationCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := r.validate(); err != nil {
		return err
	}
	if r.FreshPermitID != "" || r.Request.Input.WorkspaceID != c.WorkspaceID || r.Request.PermitID != c.PermitID || r.Request.FailureCode != c.FailureCode || !reflect.DeepEqual(r.Request.Proposal, c.Proposal) {
		return common.ErrConflict
	}
	return nil
}
