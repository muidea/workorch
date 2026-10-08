package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
const CommandDestination = modulecommon.ModuleID

const (
	DefaultListLimit        = 100
	MaxListLimit            = 200
	MaxArtifactContentBytes = 1 << 20
)

type GetResultCommand struct {
	WorkspaceID common.WorkspaceID
	ResultID    common.ResultID
}

type ListResultsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
	Cursor      string
	Limit       int
}

type ListResultsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Cursor      string
	Limit       int
}

type ListArtifactsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
	Cursor      string
	Limit       int
}

type ListArtifactsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Cursor      string
	Limit       int
}

type ListArtifactsByResultCommand struct {
	WorkspaceID common.WorkspaceID
	ResultID    common.ResultID
	Cursor      string
	Limit       int
}

// ReadArtifactContentCommand asks the Result owner for the bounded content of
// one Result-owned artifact. URI is an opaque logical identity to callers;
// only Result may resolve it against Artifact state and material.
type ReadArtifactContentCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	URI         string
}

type SealWorkspaceResultDeletionCommand struct {
	WorkspaceID           common.WorkspaceID
	DeletionID            string
	DeletionGeneration    uint64
	DataDispositionIntent deletioncontract.Mode
	QuiescenceID          string
	RequestHash           string
}

func (c SealWorkspaceResultDeletionCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration, Mode: c.DataDispositionIntent, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash}.Normalize()
}

func (c SealWorkspaceResultDeletionCommand) Validate() error { return c.Scope().Validate() }

type PurgeWorkspaceResultsCommand struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	QuiescenceID       string
	RequestHash        string
}

func (c PurgeWorkspaceResultsCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration, Mode: deletioncontract.ModePurge, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash}.Normalize()
}

func (c PurgeWorkspaceResultsCommand) Validate() error {
	return c.Scope().Validate()
}

type ResultChangedData struct {
	NotificationID       string
	TerminalMaterialHash string
	ResultID             common.ResultID
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	GoalID               common.GoalID
	DoDID                common.DoDID
	State                common.ResultState
	Title                string
	Summary              string
	SourceType           string
	SourceRef            string
	EvidenceRefs         []string
	ArtifactRefs         []common.ArtifactID
}

func ResultChangedDataFromModel(item *resultmodel.Result) ResultChangedData {
	if item == nil {
		return ResultChangedData{}
	}
	return ResultChangedData{
		NotificationID:       ResultNotificationID(item),
		TerminalMaterialHash: strings.TrimSpace(item.TerminalMaterialHash),
		ResultID:             item.ID,
		WorkspaceID:          item.WorkspaceID,
		RunID:                item.RunID,
		GoalID:               item.GoalID,
		DoDID:                item.DoDID,
		State:                item.State,
		Title:                item.Title,
		Summary:              item.Summary,
		SourceType:           item.SourceType,
		SourceRef:            item.SourceRef,
		EvidenceRefs:         append([]string(nil), item.EvidenceRefs...),
		ArtifactRefs:         append([]common.ArtifactID(nil), item.ArtifactRefs...),
	}
}

// ResultNotificationID is stable across SM-4 replay, process restart and
// notification redelivery. Consumers use it as their idempotency identity.
func ResultNotificationID(item *resultmodel.Result) string {
	if item == nil || item.TerminalDelivery == nil || strings.TrimSpace(item.TerminalMaterialHash) == "" {
		return ""
	}
	return fmt.Sprintf("result-notification/%s/%s/%s", strings.TrimSpace(item.TerminalDelivery.TerminalDeliveryID), item.State, strings.TrimPrefix(strings.TrimSpace(item.TerminalMaterialHash), "result-material-sha256:"))
}

// Each command owns one concrete result shape. This prevents an unrelated
// topic from being accepted merely because another field in a union envelope
// happened to be populated.
type GetResultResult struct{ Result *resultmodel.Result }
type ListResultsByWorkspaceResult struct {
	Results    []*resultmodel.Result
	NextCursor string
}
type ListResultsByRunResult struct {
	Results    []*resultmodel.Result
	NextCursor string
}
type ListArtifactsByWorkspaceResult struct {
	Artifacts  []*resultmodel.Artifact
	NextCursor string
}
type ListArtifactsByRunResult struct {
	Artifacts  []*resultmodel.Artifact
	NextCursor string
}
type ListArtifactsByResultResult struct {
	Artifacts  []*resultmodel.Artifact
	NextCursor string
}

// ReadArtifactContentResult is copied before crossing EventHub and is capped
// so the read contract never exposes a repository, path, reader or unbounded
// owner-private material.
type ReadArtifactContentResult struct {
	ArtifactID    common.ArtifactID
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	URI           string
	FileName      string
	ContentType   string
	Content       []byte
	ContentSHA256 string
}

func (r ReadArtifactContentResult) Clone() ReadArtifactContentResult {
	r.Content = append([]byte(nil), r.Content...)
	return r
}

func (r ReadArtifactContentResult) Validate() error {
	if r.ArtifactID == "" || r.WorkspaceID == "" || r.RunID == "" || strings.TrimSpace(r.URI) == "" ||
		strings.TrimSpace(r.FileName) == "" || strings.TrimSpace(r.ContentType) == "" || len(r.Content) == 0 || len(r.Content) > MaxArtifactContentBytes ||
		len(strings.TrimSpace(r.ContentSHA256)) != sha256.Size*2 {
		return fmt.Errorf("%w: invalid bounded artifact content result", common.ErrInvalidArgument)
	}
	if strings.Contains(r.FileName, "/") || strings.Contains(r.FileName, "\\") || r.FileName == "." || r.FileName == ".." {
		return fmt.Errorf("%w: invalid artifact content file name", common.ErrInvalidArgument)
	}
	hash, err := hex.DecodeString(strings.TrimSpace(r.ContentSHA256))
	if err != nil || len(hash) != sha256.Size {
		return fmt.Errorf("%w: invalid artifact content hash", common.ErrInvalidArgument)
	}
	want := sha256.Sum256(r.Content)
	if !strings.EqualFold(hex.EncodeToString(want[:]), r.ContentSHA256) {
		return fmt.Errorf("%w: artifact content hash mismatch", common.ErrInvalidArgument)
	}
	return nil
}

type SealWorkspaceResultDeletionResult struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	FenceID            string
	Revision           uint64
	AckHash            string
}

type PurgeWorkspaceResultsResult struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	FenceID            string
	PurgeID            string
	Revision           uint64
	AckHash            string
	PurgeProofHash     string
}

func NormalizeListWindow(cursor string, limit int) (string, int, error) {
	cursor = strings.TrimSpace(cursor)
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return "", 0, fmt.Errorf("%w: result list limit must be between 1 and %d", common.ErrInvalidArgument, MaxListLimit)
	}
	return cursor, limit, nil
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
		closeout := item.RuntimeCloseout.CloneRaw()
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
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[GetResultCommand, GetResultResult]("result", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicResultCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListResultsByWorkspaceCommand, ListResultsByWorkspaceResult]("result", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicResultCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[ListResultsByRunCommand, ListResultsByRunResult]("result", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicResultCommandListByRun).MessageContract(),
		eventcontract.CommandTopic[ListArtifactsByWorkspaceCommand, ListArtifactsByWorkspaceResult]("result", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicArtifactCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[ListArtifactsByRunCommand, ListArtifactsByRunResult]("result", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicArtifactCommandListByRun).MessageContract(),
		eventcontract.CommandTopic[ListArtifactsByResultCommand, ListArtifactsByResultResult]("result", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicArtifactCommandListByResult).MessageContract(),
		eventcontract.CommandTopic[ReadArtifactContentCommand, ReadArtifactContentResult]("result", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicArtifactCommandReadContent).MessageContract(),
		eventcontract.CommandTopic[SealWorkspaceResultDeletionCommand, SealWorkspaceResultDeletionResult]("result", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicResultCommandSealWorkspaceDeletion).MessageContract(),
		eventcontract.CommandTopic[PurgeWorkspaceResultsCommand, PurgeWorkspaceResultsResult]("result", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicResultCommandPurgeWorkspace).MessageContract(),
	}
	for _, topic := range []string{
		TopicResultCreated,
		TopicResultAccepted,
		TopicResultRejected,
		TopicResultSuperseded,
	} {
		ret = append(ret, eventcontract.DataTopic[ResultChangedData]("result", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret, eventcontract.CommandTopic[RuntimeTerminalDeliveryFact, ResultLifecycleCommandResult]("result", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicResultFactTerminalDelivery).MessageContract())
	return ret
}
