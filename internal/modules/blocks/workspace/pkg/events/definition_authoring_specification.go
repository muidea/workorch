package events

import (
	"fmt"
	"reflect"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceAuthoringSpecificationReserve = "/workspace/definition-authoring/specification/reserve"
const TopicWorkspaceAuthoringSpecificationComplete = "/workspace/definition-authoring/specification/complete"

type ReserveAuthoringSpecificationCommand struct {
	WorkspaceID         common.WorkspaceID
	InputHash           string
	RequestID           string
	DerivationPermitID  string
	ReviewKind          string
	ProposalHash        string
	ProviderBindingHash string
}

func (c ReserveAuthoringSpecificationCommand) Request() workspace.DefinitionAuthoringSpecificationRequest {
	phase := workspace.DefinitionSpecificationProposal
	if c.ReviewKind != "" {
		phase = workspace.DefinitionSpecificationCritic
	}
	return workspace.DefinitionAuthoringSpecificationRequest{RequestID: c.RequestID, DerivationPermitID: c.DerivationPermitID, Phase: phase, ReviewKind: c.ReviewKind, ProposalHash: c.ProposalHash, ProviderBindingHash: c.ProviderBindingHash}
}

func (c ReserveAuthoringSpecificationCommand) Validate() error {
	if c.WorkspaceID == "" || c.InputHash == "" || len(c.InputHash) > 256 {
		return common.ErrInvalidArgument
	}
	request := c.Request()
	request.PermitID = request.ExpectedPermitID()
	return request.Validate()
}

type CompleteAuthoringSpecificationCommand struct {
	WorkspaceID common.WorkspaceID
	PermitID    string
	Proposal    *workspace.GoalSpecification
	Review      *workspace.GoalSpecificationCompletenessReview
	FailureCode string
	Diagnostic  *workspace.DefinitionAuthoringDiagnostic
}

func (c CompleteAuthoringSpecificationCommand) Validate() error {
	if c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256 {
		return common.ErrInvalidArgument
	}
	outcomes := 0
	if c.Proposal != nil {
		outcomes++
	}
	if c.Review != nil {
		outcomes++
	}
	if c.FailureCode != "" {
		outcomes++
		if c.Diagnostic != nil && (c.Diagnostic.Validate() != nil || c.FailureCode == "provider_failed" && c.Diagnostic.ProviderCode == "") {
			return common.ErrInvalidArgument
		}
	} else if c.Diagnostic != nil {
		return common.ErrInvalidArgument
	}
	if outcomes != 1 {
		return common.ErrInvalidArgument
	}
	return nil
}

// FreshPermitID exists only after the owner CAS successfully records the first
// reservation. A replay of a reserved request cannot authorize another call.
type AuthoringSpecificationRequestResult struct {
	Request       workspace.DefinitionAuthoringSpecificationRequest
	FreshPermitID string
	Consumed      uint32
	Remaining     uint32
}

func (r AuthoringSpecificationRequestResult) validate() error {
	if err := r.Request.Validate(); err != nil {
		return err
	}
	if r.Consumed == 0 || r.Consumed > workspace.DefinitionAuthoringSpecificationRequestLimit || r.Remaining != workspace.DefinitionAuthoringSpecificationRequestLimit-r.Consumed {
		return fmt.Errorf("authoring specification request budget mismatch")
	}
	if r.FreshPermitID != "" && (r.FreshPermitID != r.Request.PermitID || r.Request.Proposal != nil || r.Request.Review != nil || r.Request.FailureCode != "") {
		return common.ErrConflict
	}
	return nil
}

func (r AuthoringSpecificationRequestResult) ValidateReserve(c ReserveAuthoringSpecificationCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := r.validate(); err != nil {
		return err
	}
	if r.Request.RequestID != c.RequestID || r.Request.PermitID != c.Request().ExpectedPermitID() {
		return common.ErrConflict
	}
	return nil
}

func (r AuthoringSpecificationRequestResult) ValidateCompletion(c CompleteAuthoringSpecificationCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := r.validate(); err != nil {
		return err
	}
	if r.FreshPermitID != "" || r.Request.PermitID != c.PermitID || r.Request.FailureCode != c.FailureCode || !reflect.DeepEqual(r.Request.Proposal, c.Proposal) || !reflect.DeepEqual(r.Request.Review, c.Review) || !reflect.DeepEqual(r.Request.Diagnostic, c.Diagnostic) {
		return common.ErrConflict
	}
	return nil
}
