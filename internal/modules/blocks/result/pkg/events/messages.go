package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
const CommandDestination = modulecommon.ModuleID

type GetResultCommand struct {
	ResultID common.ResultID
}

type ListResultsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type ListResultsByRunCommand struct {
	RunID common.RunID
}

type DeleteResultCommand struct {
	ResultID common.ResultID
}

type ListArtifactsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type ListArtifactsByRunCommand struct {
	RunID common.RunID
}

type ListArtifactsByResultCommand struct {
	ResultID common.ResultID
}

type DeleteArtifactCommand struct {
	ArtifactID common.ArtifactID
}

type ResultChangedData struct {
	ResultID     common.ResultID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	GoalID       common.GoalID
	DoDID        common.DoDID
	State        common.ResultState
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
	EvidenceRefs []string
	ArtifactRefs []common.ArtifactID
}

func ResultChangedDataFromModel(item *resultmodel.Result) ResultChangedData {
	if item == nil {
		return ResultChangedData{}
	}
	return ResultChangedData{
		ResultID:     item.ID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		GoalID:       item.GoalID,
		DoDID:        item.DoDID,
		State:        item.State,
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
		ArtifactRefs: append([]common.ArtifactID(nil), item.ArtifactRefs...),
	}
}

type ResultCommandResult struct {
	Result    *resultmodel.Result
	Results   []*resultmodel.Result
	Artifacts []*resultmodel.Artifact
}

// CloneResult keeps EventHub callers and the Result owner from sharing mutable
// slices or nested projection values across the owner boundary.
func CloneResult(item *resultmodel.Result) *resultmodel.Result {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	cloned.ArtifactRefs = append([]common.ArtifactID(nil), item.ArtifactRefs...)
	cloned.DoDAssessment = cloneDoDAssessment(item.DoDAssessment)
	cloned.SummaryDetail = cloneResultSummary(item.SummaryDetail)
	if item.RuntimeCloseout != nil {
		closeout := *item.RuntimeCloseout
		closeout.SatisfiedCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), item.RuntimeCloseout.SatisfiedCoverageWorkItemIDs...)
		closeout.UnsatisfiedCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), item.RuntimeCloseout.UnsatisfiedCoverageWorkItemIDs...)
		closeout.MissingEvidenceKinds = append([]string(nil), item.RuntimeCloseout.MissingEvidenceKinds...)
		closeout.TargetRefs = append([]string(nil), item.RuntimeCloseout.TargetRefs...)
		closeout.EvidenceRefs = append([]string(nil), item.RuntimeCloseout.EvidenceRefs...)
		closeout.ArtifactRefs = append([]common.ArtifactID(nil), item.RuntimeCloseout.ArtifactRefs...)
		closeout.OperationOutcomes = append(closeout.OperationOutcomes[:0:0], item.RuntimeCloseout.OperationOutcomes...)
		if item.RuntimeCloseout.Gaps != nil {
			closeout.Gaps = append(closeout.Gaps[:0:0], item.RuntimeCloseout.Gaps...)
			for idx := range closeout.Gaps {
				closeout.Gaps[idx].CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), item.RuntimeCloseout.Gaps[idx].CoverageWorkItemIDs...)
				closeout.Gaps[idx].MissingEvidenceKinds = append([]string(nil), item.RuntimeCloseout.Gaps[idx].MissingEvidenceKinds...)
				closeout.Gaps[idx].TargetRefs = append([]string(nil), item.RuntimeCloseout.Gaps[idx].TargetRefs...)
				closeout.Gaps[idx].EvidenceRefs = append([]string(nil), item.RuntimeCloseout.Gaps[idx].EvidenceRefs...)
			}
		}
		cloned.RuntimeCloseout = &closeout
	}
	if item.TerminalDelivery != nil {
		identity := *item.TerminalDelivery
		cloned.TerminalDelivery = &identity
	}
	return &cloned
}

func CloneResults(items []*resultmodel.Result) []*resultmodel.Result {
	if items == nil {
		return nil
	}
	ret := make([]*resultmodel.Result, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneResult(item))
	}
	return ret
}

func CloneArtifact(item *resultmodel.Artifact) *resultmodel.Artifact {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}

func CloneArtifacts(items []*resultmodel.Artifact) []*resultmodel.Artifact {
	if items == nil {
		return nil
	}
	ret := make([]*resultmodel.Artifact, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneArtifact(item))
	}
	return ret
}

func cloneResultSummary(item workorchresultcontract.ResultSummary) workorchresultcontract.ResultSummary {
	cloned := item
	cloned.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	cloned.ArtifactRefs = append([]common.ArtifactID(nil), item.ArtifactRefs...)
	cloned.KnownLimitations = append([]string(nil), item.KnownLimitations...)
	cloned.ResidualRisks = append([]string(nil), item.ResidualRisks...)
	cloned.NextActions = append([]string(nil), item.NextActions...)
	cloned.Diagnostics = item.Diagnostics.Clone()
	if item.DoDItems != nil {
		cloned.DoDItems = append(cloned.DoDItems[:0:0], item.DoDItems...)
		for idx := range cloned.DoDItems {
			cloned.DoDItems[idx].EvidenceRefs = append([]string(nil), item.DoDItems[idx].EvidenceRefs...)
		}
	}
	return cloned
}

func cloneDoDAssessment(item workorchresultcontract.DoDAssessment) workorchresultcontract.DoDAssessment {
	cloned := item
	if item.Items != nil {
		cloned.Items = append(cloned.Items[:0:0], item.Items...)
		for idx := range cloned.Items {
			cloned.Items[idx].EvidenceRefs = append([]string(nil), item.Items[idx].EvidenceRefs...)
		}
	}
	cloned.GoalCoverage = cloneDoDCoverage(item.GoalCoverage)
	cloned.AcceptanceDimensions = cloneDoDCoverage(item.AcceptanceDimensions)
	cloned.DeliverableCoverage = cloneDoDCoverage(item.DeliverableCoverage)
	return cloned
}

func cloneDoDCoverage(item *workorchresultcontract.DoDCoverage) *workorchresultcontract.DoDCoverage {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.Items != nil {
		cloned.Items = append(cloned.Items[:0:0], item.Items...)
		for idx := range cloned.Items {
			cloned.Items[idx].EvidenceRefs = append([]string(nil), item.Items[idx].EvidenceRefs...)
			cloned.Items[idx].ArtifactRefs = append([]common.ArtifactID(nil), item.Items[idx].ArtifactRefs...)
		}
	}
	return &cloned
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{}
	for _, topic := range []string{
		TopicResultCommandGet,
		TopicResultCommandListByWorkspace,
		TopicResultCommandListByRun,
		TopicResultCommandDelete,
		TopicArtifactCommandListByWorkspace,
		TopicArtifactCommandListByRun,
		TopicArtifactCommandListByResult,
		TopicArtifactCommandDelete,
	} {
		class := eventcontract.CommandClassWrite
		lane := eventcontract.LanePolicyState
		switch topic {
		case TopicResultCommandGet, TopicResultCommandListByWorkspace, TopicResultCommandListByRun, TopicArtifactCommandListByWorkspace, TopicArtifactCommandListByRun, TopicArtifactCommandListByResult:
			class = eventcontract.CommandClassRead
			lane = eventcontract.LanePolicyQuery
		}
		switch topic {
		case TopicResultCommandGet:
			ret = append(ret, eventcontract.CommandTopic[GetResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandListByWorkspace:
			ret = append(ret, eventcontract.CommandTopic[ListResultsByWorkspaceCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandListByRun:
			ret = append(ret, eventcontract.CommandTopic[ListResultsByRunCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandDelete:
			ret = append(ret, eventcontract.CommandTopic[DeleteResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandListByWorkspace:
			ret = append(ret, eventcontract.CommandTopic[ListArtifactsByWorkspaceCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandListByRun:
			ret = append(ret, eventcontract.CommandTopic[ListArtifactsByRunCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandListByResult:
			ret = append(ret, eventcontract.CommandTopic[ListArtifactsByResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandDelete:
			ret = append(ret, eventcontract.CommandTopic[DeleteArtifactCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		}
	}
	for _, topic := range []string{
		TopicResultCreated,
		TopicResultAccepted,
		TopicResultRejected,
		TopicResultSuperseded,
	} {
		ret = append(ret, eventcontract.DataTopic[ResultChangedData]("result", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret, eventcontract.CommandTopic[ApplyRuntimeTerminalDeliveryCommand, ResultLifecycleCommandResult]("result", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicResultCommandApplyTerminalDelivery).MessageContract())
	return ret
}
