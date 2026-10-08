package events

import (
	"fmt"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
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
	DoDAssessment   workorchresultcontract.DoDAssessment
	SummaryDetail   workorchresultcontract.ResultSummary
	RuntimeCloseout *runtimecontract.RuntimeResultCloseout
}

// ResultMaterialFromRuntimeTerminalMaterial 是 receiving-owner concrete 适配。
// Go typed conversion 固定完整字段与编码形状；raw clone 保全 nil/empty 并隔离调用方。
func ResultMaterialFromRuntimeTerminalMaterial(material runtimecontract.RuntimeTerminalResultMaterial) ResultMaterial {
	return ResultMaterial(material.CloneRaw())
}

// RuntimeTerminalMaterial 返回独立的 owner-neutral 原值，不规范化或重算 hash。
func (m ResultMaterial) RuntimeTerminalMaterial() runtimecontract.RuntimeTerminalResultMaterial {
	return runtimecontract.RuntimeTerminalResultMaterial(m).CloneRaw()
}

func (m ResultMaterial) CloneRaw() ResultMaterial {
	return ResultMaterialFromRuntimeTerminalMaterial(runtimecontract.RuntimeTerminalResultMaterial(m))
}

func (m ResultMaterial) Normalized() ResultMaterial {
	return ResultMaterial(runtimecontract.RuntimeTerminalResultMaterial(m).Normalized())
}

func (m ResultMaterial) ValidateRaw() error {
	return runtimecontract.RuntimeTerminalResultMaterial(m).ValidateRaw()
}

func (m ResultMaterial) Validate() error {
	// 共享合同先执行 closeout raw gate，再允许规范化材料字段。
	return runtimecontract.RuntimeTerminalResultMaterial(m).Validate()
}

// CanonicalResultMaterialHash binds the complete normalized sealed material
// to the owner-local Result lifecycle fence. JSON is used only as a
// deterministic encoding of this concrete typed value; it is never used as a
// cross-owner transport or authority reconstruction bridge.
func CanonicalResultMaterialHash(material ResultMaterial) (string, error) {
	return runtimecontract.RuntimeTerminalResultMaterial(material).CanonicalHash()
}

// RuntimeTerminalDeliveryFact is an immutable SM-4 terminal delivery fact for
// the Result owner. The payload carries sealed terminal identity and immutable
// outcome material; Result Biz decides accepted/rejected.
type RuntimeTerminalDeliveryFact struct {
	runtimecontract.RuntimeTerminalDeliveryFact
	Material ResultMaterial
}

func (c RuntimeTerminalDeliveryFact) Normalized() RuntimeTerminalDeliveryFact {
	c.RuntimeTerminalDeliveryFact = c.RuntimeTerminalDeliveryFact.Normalized()
	c.Material = c.Material.Normalized()
	return c
}

func (c RuntimeTerminalDeliveryFact) Validate() error {
	if err := c.Material.Validate(); err != nil {
		return err
	}
	c = c.Normalized()
	if c.SourceOwner != "kernel/orchestrator" {
		return fmt.Errorf("%w: result terminal delivery requires workspace, run and source", common.ErrInvalidArgument)
	}
	if err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil {
		return fmt.Errorf("%w: result terminal delivery fact: %v", common.ErrInvalidArgument, err)
	}
	if err := c.Material.Validate(); err != nil {
		return err
	}
	if c.Material.WorkspaceID != c.WorkspaceID || c.Material.RunID != c.RunID {
		return fmt.Errorf("%w: result material scope does not match terminal delivery", common.ErrConflict)
	}
	if c.Material.RuntimeCloseout != nil && c.Material.RuntimeCloseout.CloseoutID != "" {
		closeout := c.Material.RuntimeCloseout.Normalized()
		if closeout.DefinitionRevision != c.DefinitionRevision || closeout.TerminalOutcome != c.Outcome {
			return fmt.Errorf("%w: formal runtime closeout does not match terminal delivery authority", common.ErrConflict)
		}
	}
	return nil
}

func (c RuntimeTerminalDeliveryFact) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	return c.Normalized().RuntimeTerminalDeliveryFact.Identity()
}

// ResultLifecycleCommandResult is the synchronous result of a terminal delivery.
type ResultLifecycleCommandResult struct {
	Result       *resultmodel.Result
	Transitioned bool
	DeliveryAck  *runtimecontract.RuntimeDeliveryAcknowledgement
}
