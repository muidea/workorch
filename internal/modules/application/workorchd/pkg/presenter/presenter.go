package presenter

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type Snapshot struct {
	Slice     string         `json:"slice"`
	Workspace *WorkspaceView `json:"workspace"`
	Run       *RunView       `json:"run"`
	Step      *StepView      `json:"step,omitempty"`
	Steps     []StepView     `json:"steps,omitempty"`
	Blocking  *BlockingView  `json:"blocking,omitempty"`
	Observed  []ObservedView `json:"observed"`
}

type WorkspaceView struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	WorkingDirectory string `json:"workingDirectory"`
	State            string `json:"state"`
	ActiveRunID      string `json:"activeRunId"`
}

type RunView struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspaceId"`
	State              string `json:"state"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
}

type StepView struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	State  string         `json:"state"`
	Input  map[string]any `json:"input,omitempty"`
	Output map[string]any `json:"output,omitempty"`
}

type BlockingView struct {
	DecisionTicket *DecisionTicketView `json:"decisionTicket,omitempty"`
	Approval       *ApprovalView       `json:"approval,omitempty"`
}

type DecisionTicketView struct {
	ID     string `json:"id"`
	RunID  string `json:"runId"`
	StepID string `json:"stepId"`
	Reason string `json:"reason"`
	State  string `json:"state"`
}

type ApprovalView struct {
	ID     string `json:"id"`
	RunID  string `json:"runId"`
	StepID string `json:"stepId"`
	Reason string `json:"reason"`
	State  string `json:"state"`
}

type ObservedView struct {
	ID           string         `json:"id"`
	Version      string         `json:"version"`
	Topic        string         `json:"topic"`
	OccurredAt   string         `json:"occurredAt"`
	SourceModule string         `json:"sourceModule"`
	SubjectType  string         `json:"subjectType"`
	SubjectID    string         `json:"subjectId"`
	WorkspaceID  string         `json:"workspaceId"`
	RunID        string         `json:"runId"`
	StepID       string         `json:"stepId"`
	Payload      map[string]any `json:"payload"`
}

type BlockingRefs struct {
	TicketID   common.TicketID
	ApprovalID common.ApprovalID
}

func PrintBanner(w io.Writer) {
	fmt.Fprintln(w, "== workorch observable run snapshot ==")
	fmt.Fprintln(w, "acceptance target: workspace/run execution, tool calls, patches, validation results, runtime checkpoints, decisions, and approvals")
}

func PrintSnapshot(w io.Writer, title string, data *Snapshot) error {
	if _, err := fmt.Fprintf(w, "\n## %s\n", title); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func BuildSnapshot(
	slice string,
	ws *workspacemodel.Workspace,
	r *runmodel.Run,
	step *runmodel.Step,
	steps []*runmodel.Step,
	ticket *decisionmodel.Ticket,
	approval *approvalmodel.Approval,
	observedItems []observed.SnapshotItem,
) *Snapshot {
	blocking := &BlockingView{}
	if ticket != nil {
		blocking.DecisionTicket = MapTicket(ticket)
	}
	if approval != nil {
		blocking.Approval = MapApproval(approval)
	}
	if blocking.DecisionTicket == nil && blocking.Approval == nil {
		blocking = nil
	}

	return &Snapshot{
		Slice:     slice,
		Workspace: MapWorkspace(ws),
		Run:       MapRun(r),
		Step:      MapStep(step),
		Steps:     MapSteps(steps),
		Blocking:  blocking,
		Observed:  MapObserved(observedItems),
	}
}

func MapWorkspace(item *workspacemodel.Workspace) *WorkspaceView {
	if item == nil {
		return nil
	}
	return &WorkspaceView{
		ID:               string(item.ID),
		Title:            item.Title,
		Description:      item.Description,
		WorkingDirectory: item.WorkingDirectory,
		State:            string(item.State),
		ActiveRunID:      string(item.ActiveRunID),
	}
}

func MapRun(item *runmodel.Run) *RunView {
	if item == nil {
		return nil
	}
	return &RunView{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
	}
}

func MapStep(item *runmodel.Step) *StepView {
	if item == nil {
		return nil
	}
	return &StepView{
		ID:     string(item.ID),
		Kind:   item.Kind,
		State:  string(item.State),
		Input:  clonePresenterMap(item.Input),
		Output: clonePresenterMap(item.Output),
	}
}

func MapSteps(items []*runmodel.Step) []StepView {
	if len(items) == 0 {
		return nil
	}
	out := make([]StepView, 0, len(items))
	for _, item := range items {
		mapped := MapStep(item)
		if mapped == nil {
			continue
		}
		out = append(out, *mapped)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func MapTicket(item *decisionmodel.Ticket) *DecisionTicketView {
	if item == nil {
		return nil
	}
	return &DecisionTicketView{
		ID:     string(item.ID),
		RunID:  string(item.RunID),
		StepID: string(item.StepID),
		Reason: item.Reason,
		State:  string(item.State),
	}
}

func MapApproval(item *approvalmodel.Approval) *ApprovalView {
	if item == nil {
		return nil
	}
	return &ApprovalView{
		ID:     string(item.ID),
		RunID:  string(item.RunID),
		StepID: string(item.StepID),
		Reason: item.Reason,
		State:  string(item.State),
	}
}

func MapObserved(items []observed.SnapshotItem) []ObservedView {
	if len(items) == 0 {
		return nil
	}
	out := make([]ObservedView, 0, len(items))
	for _, item := range items {
		out = append(out, ObservedView{
			ID:           item.ID,
			Version:      item.Version,
			Topic:        item.Topic,
			OccurredAt:   item.OccurredAt,
			SourceModule: item.SourceModule,
			SubjectType:  item.SubjectType,
			SubjectID:    item.SubjectID,
			WorkspaceID:  item.WorkspaceID,
			RunID:        item.RunID,
			StepID:       item.StepID,
			Payload:      item.PayloadMap(),
		})
	}
	return out
}

func clonePresenterMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = clonePresenterValue(v)
	}
	return out
}

func clonePresenterSlice(input []any) []any {
	if input == nil {
		return nil
	}
	out := make([]any, len(input))
	for i, v := range input {
		out[i] = clonePresenterValue(v)
	}
	return out
}

func clonePresenterValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return clonePresenterMap(typed)
	case []any:
		return clonePresenterSlice(typed)
	default:
		return typed
	}
}
