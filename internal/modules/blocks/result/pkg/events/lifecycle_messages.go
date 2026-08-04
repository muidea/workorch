package events

import (
	"fmt"
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RUNTIME-SMO result lifecycle contracts.
//
// Result owner alone constructs and persists accepted/rejected Result from a
// sealed terminal delivery fact. Callers never set Result.State as a target.

// ResultMaterial is immutable outcome content sealed by the Orchestrator. It
// deliberately excludes Result owner lifecycle fields (State, timestamps,
// AcceptedBy and TerminalDelivery); Result Biz derives those after validating
// the terminal fact.
type ResultMaterial struct {
	ID              common.ResultID
	WorkspaceID     common.WorkspaceID
	RunID           common.RunID
	GoalID          common.GoalID
	DoDID           common.DoDID
	Title           string
	Summary         string
	SourceType      string
	SourceRef       string
	EvidenceRefs    []string
	ArtifactRefs    []common.ArtifactID
	DoDAssessment   resultmodel.DoDAssessment
	SummaryDetail   resultmodel.ResultSummary
	RuntimeCloseout *runtimecontract.RuntimeResultCloseout
}

func (m ResultMaterial) Normalized() ResultMaterial {
	m.Title = strings.TrimSpace(m.Title)
	m.Summary = strings.TrimSpace(m.Summary)
	m.SourceType = strings.TrimSpace(m.SourceType)
	m.SourceRef = strings.TrimSpace(m.SourceRef)
	m.EvidenceRefs = append([]string(nil), m.EvidenceRefs...)
	m.ArtifactRefs = append([]common.ArtifactID(nil), m.ArtifactRefs...)
	return m
}

func (m ResultMaterial) Validate() error {
	m = m.Normalized()
	if m.ID == "" || m.WorkspaceID == "" || m.RunID == "" || m.GoalID == "" || m.DoDID == "" || m.Summary == "" {
		return fmt.Errorf("%w: result material requires identity, scope and summary", common.ErrInvalidArgument)
	}
	return nil
}

// ApplyRuntimeTerminalDeliveryCommand is an SM-4 terminal delivery request for
// the Result owner. The payload carries sealed terminal identity and immutable
// outcome material; Result Biz decides accepted/rejected.
type ApplyRuntimeTerminalDeliveryCommand struct {
	runtimecontract.RuntimeTerminalDeliveryIdentity
	SourceOwner string
	CausationID string
	Material    ResultMaterial
}

func (c ApplyRuntimeTerminalDeliveryCommand) Normalized() ApplyRuntimeTerminalDeliveryCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.TerminalDeliveryID = strings.TrimSpace(c.TerminalDeliveryID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	c.Outcome = c.Outcome.Normalized()
	return c
}

func (c ApplyRuntimeTerminalDeliveryCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != "kernel/orchestrator" || c.CausationID == "" {
		return fmt.Errorf("%w: result terminal delivery requires workspace, run and source", common.ErrInvalidArgument)
	}
	if err := c.Identity().Validate(); err != nil {
		return fmt.Errorf("%w: result terminal delivery identity: %v", common.ErrInvalidArgument, err)
	}
	if err := c.Material.Validate(); err != nil {
		return err
	}
	if c.Material.WorkspaceID != c.WorkspaceID || c.Material.RunID != c.RunID {
		return fmt.Errorf("%w: result material scope does not match terminal delivery", common.ErrConflict)
	}
	return nil
}

func (c ApplyRuntimeTerminalDeliveryCommand) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	return c.Normalized().RuntimeTerminalDeliveryIdentity
}

// ResultLifecycleCommandResult is the synchronous result of a terminal delivery.
type ResultLifecycleCommandResult struct {
	Result       *resultmodel.Result
	Transitioned bool
	DeliveryAck  *runtimecontract.RuntimeDeliveryAcknowledgement
}
