package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	client "github.com/muidea/workorch/internal/pkg/workorchclient"
)

// remoteBackend implements Backend using an HTTP client to a running workorch --server.
type remoteBackend struct {
	cl   *client.Client
	ws   *remoteWorkspaceBackend
	goal *remoteGoalBackend
	dod  *remoteDoDBackend
	run  *remoteRunBackend
	dec  *remoteDecisionBackend
	appr *remoteApprovalBackend
	soul *remoteSoulBackend
	sk   *remoteSkillBackend
	mcp  *remoteMCPBackend
	mem  *remoteMemoryBackend
	tmpl *remoteTemplateBackend
	cfg  *remoteConfigBackend
	evt  *remoteEventsBackend
}

type RemoteOption func(*remoteOptions)

type remoteOptions struct {
	eventStreamView string
}

// NewRemote creates a Backend that communicates with a remote workorch --server via HTTP.
func NewRemote(addr string) Backend {
	return NewRemoteWithOptions(addr)
}

func NewRemoteProjectionEvents(addr string) Backend {
	return NewRemoteWithOptions(addr, WithRemoteEventStreamView("projection"))
}

func NewRemoteWithOptions(addr string, opts ...RemoteOption) Backend {
	options := remoteOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	cl := client.New(addr)
	b := &remoteBackend{cl: cl}
	b.ws = &remoteWorkspaceBackend{cl: cl}
	b.goal = &remoteGoalBackend{cl: cl}
	b.dod = &remoteDoDBackend{cl: cl}
	b.run = &remoteRunBackend{cl: cl}
	b.dec = &remoteDecisionBackend{cl: cl}
	b.appr = &remoteApprovalBackend{cl: cl}
	b.soul = &remoteSoulBackend{cl: cl}
	b.sk = &remoteSkillBackend{cl: cl}
	b.mcp = &remoteMCPBackend{cl: cl}
	b.mem = &remoteMemoryBackend{cl: cl}
	b.tmpl = &remoteTemplateBackend{cl: cl}
	b.cfg = &remoteConfigBackend{cl: cl}
	b.evt = &remoteEventsBackend{cl: cl, streamView: strings.TrimSpace(options.eventStreamView)}
	return b
}

func WithRemoteEventStreamView(view string) RemoteOption {
	return func(options *remoteOptions) {
		if options != nil {
			options.eventStreamView = strings.TrimSpace(view)
		}
	}
}

func (b *remoteBackend) Workspace() WorkspaceBackend { return b.ws }
func (b *remoteBackend) Goal() GoalBackend           { return b.goal }
func (b *remoteBackend) DoD() DoDBackend             { return b.dod }
func (b *remoteBackend) Run() RunBackend             { return b.run }
func (b *remoteBackend) Decision() DecisionBackend   { return b.dec }
func (b *remoteBackend) Approval() ApprovalBackend   { return b.appr }
func (b *remoteBackend) Soul() SoulBackend           { return b.soul }
func (b *remoteBackend) Skill() SkillBackend         { return b.sk }
func (b *remoteBackend) MCP() MCPBackend             { return b.mcp }
func (b *remoteBackend) Memory() MemoryBackend       { return b.mem }
func (b *remoteBackend) Template() TemplateBackend   { return b.tmpl }
func (b *remoteBackend) Config() ConfigBackend       { return b.cfg }
func (b *remoteBackend) Events() EventsBackend       { return b.evt }

// ── Remote Workspace ──

type remoteWorkspaceBackend struct{ cl *client.Client }

func workspaceInfoFromClient(ws *client.Workspace) WorkspaceInfo {
	if ws == nil {
		return WorkspaceInfo{}
	}
	return WorkspaceInfo{
		ID:                   common.WorkspaceID(ws.ID),
		Title:                ws.Title,
		Description:          ws.Description,
		DeliverableDirectory: ws.DeliverableDirectory,
		State:                common.WorkspaceState(ws.State),
		ActiveRunID:          common.RunID(ws.ActiveRunID),
		BlockingTicketID:     common.TicketID(ws.BlockingTicketID),
		BlockingApprovalID:   common.ApprovalID(ws.BlockingApprovalID),
		TemplateID:           ws.TemplateID,
		ProfileID:            ws.ProfileID,
		DefaultSoulID:        common.SoulID(ws.DefaultSoulID),
		SoulBindings:         commonSoulBindings(ws.SoulBindings),
	}
}

func projectionInfoFromClient(input client.ProjectionStatus) ProjectionInfo {
	return ProjectionInfo{
		State:                   input.State,
		GlobalRevision:          input.GlobalRevision,
		WorkspaceRevision:       input.WorkspaceRevision,
		SourceEventRevision:     input.SourceEventRevision,
		ProjectedSourceRevision: input.ProjectedSourceRevision,
		UpdatedAt:               input.UpdatedAt,
		RebuiltAt:               input.RebuiltAt,
		Degraded:                input.Degraded,
		StaleReason:             input.StaleReason,
		Error:                   input.Error,
	}
}

func (b *remoteWorkspaceBackend) Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error) {
	ws, err := b.cl.CreateWorkspace(ctx, req.Title, req.Description, req.DeliverableDirectory, req.TemplateID)
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error) {
	ws, err := b.cl.GetWorkspace(ctx, string(id))
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) List(ctx context.Context) ([]WorkspaceInfo, error) {
	projection, err := b.cl.ListWorkspaceProjection(ctx)
	if err != nil {
		return nil, err
	}
	list := projection.Workspaces
	result := make([]WorkspaceInfo, 0, len(list))
	for _, ws := range list {
		item := workspaceInfoFromClient(&ws)
		item.Projection = projectionInfoFromClient(projection.Projection)
		result = append(result, item)
	}
	return result, nil
}

func (b *remoteWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error) {
	projection, err := b.cl.GetWorkspaceSnapshotProjection(ctx, string(id))
	if err != nil {
		return nil, err
	}
	snapshot, err := decodeSnapshotInfo(projection.Snapshot)
	if err != nil || snapshot == nil {
		return snapshot, err
	}
	snapshot.Projection = projectionInfoFromClient(projection.Projection)
	snapshot.Workspace.Projection = snapshot.Projection
	return snapshot, nil
}

func (b *remoteWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return b.cl.DeleteWorkspace(ctx, string(id), purge)
}

// ── Remote Goal ──

type remoteGoalBackend struct{ cl *client.Client }

func (b *remoteGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error) {
	g, err := b.cl.CreateGoal(ctx, string(wsID), text)
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error) {
	list, err := b.cl.ListGoals(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]GoalInfo, 0, len(list))
	for _, g := range list {
		result = append(result, GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text})
	}
	return result, nil
}
func (b *remoteGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*GoalInfo, error) {
	g, err := b.cl.GetGoal(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*GoalInfo, error) {
	g, err := b.cl.UpdateGoal(ctx, string(wsID), string(id), text)
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}

// ── Remote DoD ──

type remoteDoDBackend struct{ cl *client.Client }

func (b *remoteDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error) {
	d, err := b.cl.CreateDoD(ctx, string(wsID), items)
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error) {
	list, err := b.cl.ListDoDs(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]DoDInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State})
	}
	return result, nil
}
func (b *remoteDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*DoDInfo, error) {
	d, err := b.cl.GetDoD(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*DoDInfo, error) {
	d, err := b.cl.UpdateDoD(ctx, string(wsID), string(id), items)
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}

// ── Remote Run ──

type remoteRunBackend struct{ cl *client.Client }

func (b *remoteRunBackend) Activate(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error) {
	act, err := b.cl.ActivateWorkspaceExecution(ctx, string(wsID), &client.ActivationRequest{
		GoalID: string(req.GoalID), DoDID: string(req.DoDID),
		LLMProviderID:  req.LLMProviderID,
		DialogLanguage: req.DialogLanguage, SoulID: string(req.SoulID), Priority: req.Priority,
		ExecutionContextPatch: req.ExecutionContextPatch,
	})
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID: common.RunID(act.Run.ID), WorkspaceID: common.WorkspaceID(act.Run.WorkspaceID),
		GoalID: common.GoalID(act.Run.GoalID), DoDID: common.DoDID(act.Run.DoDID),
		ExecutionContext:   act.Run.ExecutionContext,
		State:              common.RunState(act.Run.State),
		BlockingTicketID:   common.TicketID(act.Run.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(act.Run.BlockingApprovalID),
		FailureReason:      act.Run.FailureReason,
	}, nil
}
func (b *remoteRunBackend) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID, executionContextPatch ...map[string]any) (*RunInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	r, err := b.cl.ResumeRun(ctx, string(wsID), string(id), executionContextPatch...)
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
		GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
		ExecutionContext:   r.ExecutionContext,
		State:              common.RunState(r.State),
		BlockingTicketID:   common.TicketID(r.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
		FailureReason:      r.FailureReason,
	}, nil
}
func (b *remoteRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	r, err := b.cl.GetRun(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
		GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
		ExecutionContext:   r.ExecutionContext,
		State:              common.RunState(r.State),
		BlockingTicketID:   common.TicketID(r.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
		FailureReason:      r.FailureReason,
	}, nil
}
func (b *remoteRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error) {
	list, err := b.cl.ListRuns(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]RunInfo, 0, len(list))
	for _, r := range list {
		result = append(result, RunInfo{
			ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
			GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
			ExecutionContext:   r.ExecutionContext,
			State:              common.RunState(r.State),
			BlockingTicketID:   common.TicketID(r.BlockingTicketID),
			BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
			FailureReason:      r.FailureReason,
		})
	}
	return result, nil
}
func (b *remoteRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	s, err := b.cl.GetRunStatus(ctx, string(wsID), string(id), "", 0)
	if err != nil {
		return nil, err
	}
	return &RunStatusInfo{
		ID: common.RunID(s.ID), WorkspaceID: common.WorkspaceID(s.WorkspaceID),
		State: common.RunState(s.State), StepCount: s.StepCount,
		CurrentStepID:    common.StepID(s.CurrentStepID),
		CurrentStepKind:  canonicalRuntimeStepKind(s.CurrentStepKind),
		CurrentStepState: common.StepState(s.CurrentStepState),
		DecisionCount:    s.DecisionCount, ApprovalCount: s.ApprovalCount,
		BlockingTicketID:   common.TicketID(s.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(s.BlockingApprovalID),
		FailureReason:      s.FailureReason,
	}, nil
}

// ── Remote Decision ──

type remoteDecisionBackend struct{ cl *client.Client }

func (b *remoteDecisionBackend) Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error) {
	d, err := b.cl.ResolveDecision(ctx, string(req.WorkspaceID), string(req.TicketID), req.ResolutionType, req.Payload)
	if err != nil {
		return nil, err
	}
	return &DecisionInfo{ID: common.TicketID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State, ResolutionType: d.ResolutionType, AutoGovernance: mapRemoteAutoGovernanceSummary(d.AutoGovernance)}, nil
}
func (b *remoteDecisionBackend) BatchResolve(ctx context.Context, workspaceID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error) {
	ids := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		ids[i] = string(id)
	}
	list, err := b.cl.BatchResolveDecisions(ctx, string(workspaceID), ids, resolutionType, payload)
	if err != nil {
		return nil, err
	}
	result := make([]DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DecisionInfo{ID: common.TicketID(d.ID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State, AutoGovernance: mapRemoteAutoGovernanceSummary(d.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error) {
	list, err := b.cl.ListDecisions(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DecisionInfo{ID: common.TicketID(d.ID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State, AutoGovernance: mapRemoteAutoGovernanceSummary(d.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*DecisionInfo, error) {
	d, err := b.cl.GetDecision(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &DecisionInfo{ID: common.TicketID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State, AutoGovernance: mapRemoteAutoGovernanceSummary(d.AutoGovernance)}, nil
}

// ── Remote Approval ──

type remoteApprovalBackend struct{ cl *client.Client }

func (b *remoteApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	a, err := b.cl.Approve(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}
func (b *remoteApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	a, err := b.cl.Reject(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}
func (b *remoteApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string) ([]ApprovalInfo, error) {
	_idStrings := make([]string, len(ids))
	for i, id := range ids {
		_idStrings[i] = string(id)
	}
	list, err := b.cl.BatchResolveApprovals(ctx, string(wsID), _idStrings, action)
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error) {
	list, err := b.cl.ListApprovals(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	a, err := b.cl.GetApproval(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}

func mapRemoteAutoGovernanceSummary(summary *client.AutoGovernanceSummary) *AutoGovernanceSummaryInfo {
	if summary == nil {
		return nil
	}
	ret := &AutoGovernanceSummaryInfo{
		TraceID:           summary.TraceID,
		Action:            summary.Action,
		Status:            summary.Status,
		RiskLevel:         summary.RiskLevel,
		PolicyMatched:     summary.PolicyMatched,
		PolicyBlocked:     summary.PolicyBlocked,
		Confidence:        summary.Confidence,
		Reason:            summary.Reason,
		PromptFingerprint: summary.PromptFingerprint,
		OccurredAt:        summary.OccurredAt,
	}
	if len(summary.EvidenceRefs) > 0 {
		ret.EvidenceRefs = append([]string(nil), summary.EvidenceRefs...)
	}
	return ret
}

// ── Stub remote backends ──

type remoteSoulBackend struct{ cl *client.Client }

func (b *remoteSoulBackend) Create(ctx context.Context, s *SoulInfo) (*SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Get(ctx context.Context, id common.SoulID) (*SoulInfo, error) {
	return &SoulInfo{ID: id}, nil
}
func (b *remoteSoulBackend) List(ctx context.Context) ([]SoulInfo, error) { return nil, nil }
func (b *remoteSoulBackend) Update(ctx context.Context, id common.SoulID, s *SoulInfo) (*SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Delete(ctx context.Context, id common.SoulID) error { return nil }

type remoteSkillBackend struct{ cl *client.Client }

func (b *remoteSkillBackend) Create(ctx context.Context, s *SkillInfo) (*SkillInfo, error) {
	return s, nil
}
func (b *remoteSkillBackend) Get(ctx context.Context, id common.SkillID) (*SkillInfo, error) {
	return &SkillInfo{ID: id}, nil
}
func (b *remoteSkillBackend) List(ctx context.Context) ([]SkillInfo, error) { return nil, nil }

type remoteMCPBackend struct{ cl *client.Client }

func (b *remoteMCPBackend) CreateServer(ctx context.Context, s *MCPServerInfo) (*MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error) {
	return &MCPServerInfo{ID: id}, nil
}
func (b *remoteMCPBackend) ListServers(ctx context.Context) ([]MCPServerInfo, error) { return nil, nil }
func (b *remoteMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *MCPServerInfo) (*MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error) {
	return nil, nil
}

type remoteMemoryBackend struct{ cl *client.Client }

func (b *remoteMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	item, err := b.cl.CreateMemory(ctx, string(wsID), clientMemoryFromInfo(m))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*MemoryItemInfo, error) {
	item, err := b.cl.GetMemory(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	item, err := b.cl.UpdateMemory(ctx, string(wsID), string(id), clientMemoryFromInfo(m))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return b.cl.DeleteMemory(ctx, string(wsID), string(id))
}
func (b *remoteMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error) {
	return b.Search(ctx, wsID, MemorySearchRequest{})
}
func (b *remoteMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req MemorySearchRequest) ([]MemoryItemInfo, error) {
	items, err := b.cl.SearchMemory(ctx, client.MemorySearchQuery{
		WorkspaceID: string(wsID),
		Query:       req.Query,
		Tags:        append([]string(nil), req.Tags...),
		Types:       append([]string(nil), req.Types...),
		Limit:       req.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret := make([]MemoryItemInfo, 0, len(items))
	for _, item := range items {
		info := memoryInfoFromClient(&item)
		ret = append(ret, *info)
	}
	return ret, nil
}

func clientMemoryFromInfo(info *MemoryItemInfo) *client.MemoryItem {
	if info == nil {
		return &client.MemoryItem{}
	}
	return &client.MemoryItem{
		ID:                  string(info.ID),
		WorkspaceID:         string(info.WorkspaceID),
		RunID:               string(info.RunID),
		SourceType:          info.SourceType,
		SourceRef:           info.SourceRef,
		MemoryType:          info.MemoryType,
		Content:             info.Content,
		Summary:             info.Summary,
		Tags:                append([]string(nil), info.Tags...),
		Importance:          info.Importance,
		Confidence:          info.Confidence,
		Visibility:          info.Visibility,
		EmbeddingProviderID: info.EmbeddingProviderID,
		EmbeddingModel:      info.EmbeddingModel,
		EmbeddingText:       info.EmbeddingText,
		Vector:              append([]float64(nil), info.Vector...),
	}
}

func memoryInfoFromClient(item *client.MemoryItem) *MemoryItemInfo {
	if item == nil {
		return nil
	}
	return &MemoryItemInfo{
		ID:                  common.MemoryID(item.ID),
		WorkspaceID:         common.WorkspaceID(item.WorkspaceID),
		RunID:               common.RunID(item.RunID),
		SourceType:          item.SourceType,
		SourceRef:           item.SourceRef,
		MemoryType:          item.MemoryType,
		Content:             item.Content,
		Summary:             item.Summary,
		Tags:                append([]string(nil), item.Tags...),
		Importance:          item.Importance,
		Confidence:          item.Confidence,
		Visibility:          item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID,
		EmbeddingModel:      item.EmbeddingModel,
		EmbeddingText:       item.EmbeddingText,
		VectorReady:         item.VectorReady,
		VectorDimension:     item.VectorDimension,
		VectorScore:         item.VectorScore,
		Vector:              append([]float64(nil), item.Vector...),
	}
}

type remoteTemplateBackend struct{ cl *client.Client }

func (b *remoteTemplateBackend) Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error) {
	item, err := b.cl.CreateWorkspaceTemplate(ctx, clientTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) Get(ctx context.Context, id string) (*TemplateInfo, error) {
	item, err := b.cl.GetWorkspaceTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) List(ctx context.Context) ([]TemplateInfo, error) {
	items, err := b.cl.ListWorkspaceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]TemplateInfo, 0, len(items))
	for i := range items {
		if info := templateInfoFromClient(&items[i]); info != nil {
			ret = append(ret, *info)
		}
	}
	return ret, nil
}
func (b *remoteTemplateBackend) Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error) {
	item, err := b.cl.UpdateWorkspaceTemplate(ctx, id, clientTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) Delete(ctx context.Context, id string) error {
	return b.cl.DeleteWorkspaceTemplate(ctx, id)
}

func clientTemplateFromInfo(info *TemplateInfo) *client.WorkspaceTemplate {
	if info == nil {
		return &client.WorkspaceTemplate{}
	}
	return &client.WorkspaceTemplate{
		ID:               info.ID,
		Name:             info.Name,
		Description:      info.Description,
		GoalText:         info.GoalText,
		DoDItems:         append([]string(nil), info.DoDItems...),
		DefaultSoulID:    string(info.DefaultSoulID),
		SoulBindings:     cloneStringMap(info.SoulBindings),
		EvidencePolicies: cloneTemplateEvidencePolicies(info.EvidencePolicies),
	}
}

func templateInfoFromClient(item *client.WorkspaceTemplate) *TemplateInfo {
	if item == nil {
		return nil
	}
	return &TemplateInfo{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    common.SoulID(item.DefaultSoulID),
		SoulBindings:     cloneStringMap(item.SoulBindings),
		EvidencePolicies: cloneTemplateEvidencePolicies(item.EvidencePolicies),
	}
}

type remoteConfigBackend struct{ cl *client.Client }

func (b *remoteConfigBackend) Get(ctx context.Context) (*ServiceConfig, error) {
	cfg, err := b.cl.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &ServiceConfig{}, nil
	}
	providers := make([]LLMProvider, 0, len(cfg.LLM.Providers))
	for _, item := range cfg.LLM.Providers {
		providers = append(providers, LLMProvider{
			ID:         item.ID,
			Provider:   item.Provider,
			APISurface: item.APISurface,
			BaseURL:    item.BaseURL,
			Model:      item.Model,
			Timeout:    item.Timeout,
			Disabled:   item.Disabled,
			Streaming:  item.Streaming,
		})
	}
	return &ServiceConfig{
		ServerAddr:        cfg.Server.Addr,
		Backend:           cfg.Repository.Backend,
		DataDir:           cfg.Repository.DataDir,
		DefaultProvider:   cfg.LLM.DefaultProviderID,
		MaxConcurrentRuns: cfg.Server.MaxConcurrentRuns,
		Providers:         providers,
		LLMOutput: LLMOutputConfig{
			MaxPatchContentBytes:               cfg.Runtime.LLMOutput.MaxPatchContentBytes,
			MaxGraphBytes:                      cfg.Runtime.LLMOutput.MaxGraphBytes,
			MaxGraphFileWrites:                 cfg.Runtime.LLMOutput.MaxGraphFileWrites,
			SchemaInvalidLargePreviewThreshold: cfg.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold,
			MaxArtifactChunkBytes:              cfg.Runtime.LLMOutput.MaxArtifactChunkBytes,
			PreviousChunkTailBytes:             cfg.Runtime.LLMOutput.PreviousChunkTailBytes,
			MaxChunkRetryAttempts:              cfg.Runtime.LLMOutput.MaxChunkRetryAttempts,
			MaxArtifactPlanAttempts:            cfg.Runtime.LLMOutput.MaxArtifactPlanAttempts,
			MaxArtifactReplanAttempts:          cfg.Runtime.LLMOutput.MaxArtifactReplanAttempts,
		},
	}, nil
}

type remoteEventsBackend struct {
	cl          *client.Client
	streamView  string
	mu          sync.Mutex
	lastEventID string
}

func (b *remoteEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	if b == nil || b.cl == nil || handler == nil {
		return func() {}, nil
	}
	streamCtx, cancel := context.WithCancel(ctx)
	var once sync.Once
	go b.streamEvents(streamCtx, handler)
	return func() {
		once.Do(cancel)
	}, nil
}

func (b *remoteEventsBackend) streamEvents(ctx context.Context, handler EventHandler) {
	backoff := 200 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		connected := b.streamEventsOnce(ctx, handler)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = 200 * time.Millisecond
		} else if backoff < 2*time.Second {
			backoff *= 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (b *remoteEventsBackend) streamEventsOnce(ctx context.Context, handler EventHandler) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.cl.BaseURL()+"/api/v1/workorch/events/stream", nil)
	if err != nil {
		return false
	}
	query := req.URL.Query()
	if streamView := strings.TrimSpace(b.streamView); streamView != "" {
		query.Set("view", streamView)
	}
	if lastEventID := b.lastEventIDSnapshot(); lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
		query.Set("lastEventId", lastEventID)
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "text/event-stream")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	handler(ctx, Event{
		Type: EventWorkspaceListChanged,
		Payload: map[string]any{
			"reason": "event_stream_connected",
		},
	})
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	var dataLines []string
	var eventID string
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return true
		default:
		}
		line := scanner.Text()
		if line == "" {
			b.dispatchSSEData(ctx, dataLines, eventID, handler)
			dataLines = nil
			eventID = ""
			continue
		}
		if strings.HasPrefix(line, "id:") {
			eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return true
}

func (b *remoteEventsBackend) dispatchSSEData(ctx context.Context, lines []string, eventID string, handler EventHandler) {
	if len(lines) == 0 {
		return
	}
	var envelope events.Envelope
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &envelope); err != nil {
		return
	}
	evt := convertEnvelope(envelope)
	if evt.Type == "" {
		return
	}
	if evt.ID == "" {
		evt.ID = strings.TrimSpace(eventID)
	}
	b.rememberLastEventID(evt.ID)
	handler(ctx, evt)
}

func (b *remoteEventsBackend) lastEventIDSnapshot() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastEventID
}

func (b *remoteEventsBackend) rememberLastEventID(id string) {
	if b == nil || strings.TrimSpace(id) == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastEventID = strings.TrimSpace(id)
}

// Ensure remoteBackend implements Backend.
var _ Backend = (*remoteBackend)(nil)
