package backend

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
	"github.com/muidea/workorch/internal/pkg/common"
)

// remoteBackend implements Backend using an HTTP client to a running workorchd.
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

// NewRemote creates a Backend that communicates with a remote workorchd via HTTP.
func NewRemote(addr string) Backend {
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
	b.evt = &remoteEventsBackend{}
	return b
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

func (b *remoteWorkspaceBackend) Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error) {
	ws, err := b.cl.CreateWorkspace(ctx, req.Title, req.Description, req.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	return &WorkspaceInfo{
		ID: common.WorkspaceID(ws.ID), Title: ws.Title, Description: ws.Description,
		WorkingDirectory: ws.WorkingDirectory, State: common.WorkspaceState(ws.State),
		ActiveRunID: common.RunID(ws.ActiveRunID), DefaultSoulID: common.SoulID(ws.DefaultSoulID),
	}, nil
}

func (b *remoteWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error) {
	ws, err := b.cl.GetWorkspace(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &WorkspaceInfo{
		ID: common.WorkspaceID(ws.ID), Title: ws.Title, Description: ws.Description,
		WorkingDirectory: ws.WorkingDirectory, State: common.WorkspaceState(ws.State),
		ActiveRunID: common.RunID(ws.ActiveRunID),
	}, nil
}

func (b *remoteWorkspaceBackend) List(ctx context.Context) ([]WorkspaceInfo, error) {
	list, err := b.cl.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceInfo, 0, len(list))
	for _, ws := range list {
		result = append(result, WorkspaceInfo{
			ID: common.WorkspaceID(ws.ID), Title: ws.Title, Description: ws.Description,
			WorkingDirectory: ws.WorkingDirectory, State: common.WorkspaceState(ws.State),
			ActiveRunID: common.RunID(ws.ActiveRunID),
		})
	}
	return result, nil
}

func (b *remoteWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error) {
	snapshot, err := b.cl.GetWorkspaceSnapshot(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return decodeSnapshotInfo(snapshot)
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
func (b *remoteGoalBackend) Get(ctx context.Context, id common.GoalID) (*GoalInfo, error) {
	g, err := b.cl.GetGoal(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) Update(ctx context.Context, id common.GoalID, text string) (*GoalInfo, error) {
	return b.Get(ctx, id)
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
func (b *remoteDoDBackend) Get(ctx context.Context, id common.DoDID) (*DoDInfo, error) {
	d, err := b.cl.GetDoD(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) Update(ctx context.Context, id common.DoDID, items []string) (*DoDInfo, error) {
	return b.Get(ctx, id)
}

// ── Remote Run ──

type remoteRunBackend struct{ cl *client.Client }

func (b *remoteRunBackend) Activate(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error) {
	act, err := b.cl.ActivateWorkspaceExecution(ctx, string(wsID), &client.ActivationRequest{
		GoalID: string(req.GoalID), DoDID: string(req.DoDID), LLMProviderID: req.LLMProviderID,
		SoulID: string(req.SoulID), Priority: req.Priority,
	})
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID: common.RunID(act.Run.ID), WorkspaceID: common.WorkspaceID(act.Run.WorkspaceID),
		State: common.RunState(act.Run.State),
	}, nil
}
func (b *remoteRunBackend) Resume(ctx context.Context, id common.RunID) (*RunInfo, error) {
	r, err := b.cl.ResumeRun(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &RunInfo{ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID), State: common.RunState(r.State)}, nil
}
func (b *remoteRunBackend) Get(ctx context.Context, id common.RunID) (*RunInfo, error) {
	r, err := b.cl.GetRun(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &RunInfo{ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID), State: common.RunState(r.State)}, nil
}
func (b *remoteRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error) {
	list, err := b.cl.ListRuns(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]RunInfo, 0, len(list))
	for _, r := range list {
		result = append(result, RunInfo{ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID), State: common.RunState(r.State)})
	}
	return result, nil
}
func (b *remoteRunBackend) Status(ctx context.Context, id common.RunID) (*RunStatusInfo, error) {
	s, err := b.cl.GetRunStatus(ctx, string(id), "", 0)
	if err != nil {
		return nil, err
	}
	return &RunStatusInfo{
		ID: common.RunID(s.ID), WorkspaceID: common.WorkspaceID(s.WorkspaceID),
		State: common.RunState(s.State), StepCount: s.StepCount,
		DecisionCount: s.DecisionCount, ApprovalCount: s.ApprovalCount,
		BlockingTicketID:   common.TicketID(s.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(s.BlockingApprovalID),
		FailureReason:      s.FailureReason,
	}, nil
}

// ── Remote Decision ──

type remoteDecisionBackend struct{ cl *client.Client }

func (b *remoteDecisionBackend) Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error) {
	d, err := b.cl.ResolveDecision(ctx, string(req.TicketID), req.ResolutionType, req.Payload)
	if err != nil {
		return nil, err
	}
	return &DecisionInfo{ID: common.TicketID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State}, nil
}
func (b *remoteDecisionBackend) BatchResolve(ctx context.Context, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error) {
	ids := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		ids[i] = string(id)
	}
	list, err := b.cl.BatchResolveDecisions(ctx, ids, resolutionType, payload)
	if err != nil {
		return nil, err
	}
	result := make([]DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DecisionInfo{ID: common.TicketID(d.ID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State})
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
		result = append(result, DecisionInfo{ID: common.TicketID(d.ID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State})
	}
	return result, nil
}
func (b *remoteDecisionBackend) Get(ctx context.Context, id common.TicketID) (*DecisionInfo, error) {
	d, err := b.cl.GetDecision(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &DecisionInfo{ID: common.TicketID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State}, nil
}

// ── Remote Approval ──

type remoteApprovalBackend struct{ cl *client.Client }

func (b *remoteApprovalBackend) Approve(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error) {
	a, err := b.cl.Approve(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State}, nil
}
func (b *remoteApprovalBackend) Reject(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error) {
	a, err := b.cl.Reject(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State}, nil
}
func (b *remoteApprovalBackend) BatchResolve(ctx context.Context, ids []common.ApprovalID, action string) ([]ApprovalInfo, error) {
	_idStrings := make([]string, len(ids))
	for i, id := range ids {
		_idStrings[i] = string(id)
	}
	list, err := b.cl.BatchResolveApprovals(ctx, _idStrings, action)
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State})
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
		result = append(result, ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State})
	}
	return result, nil
}
func (b *remoteApprovalBackend) Get(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error) {
	a, err := b.cl.GetApproval(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State}, nil
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
func (b *remoteMemoryBackend) Get(ctx context.Context, id common.MemoryID) (*MemoryItemInfo, error) {
	item, err := b.cl.GetMemory(ctx, string(id))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Update(ctx context.Context, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	item, err := b.cl.UpdateMemory(ctx, string(id), clientMemoryFromInfo(m))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Delete(ctx context.Context, id common.MemoryID) error {
	return b.cl.DeleteMemory(ctx, string(id))
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
	return t, nil
}
func (b *remoteTemplateBackend) Get(ctx context.Context, id string) (*TemplateInfo, error) {
	return &TemplateInfo{ID: id}, nil
}
func (b *remoteTemplateBackend) List(ctx context.Context) ([]TemplateInfo, error) { return nil, nil }
func (b *remoteTemplateBackend) Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error) {
	return t, nil
}
func (b *remoteTemplateBackend) Delete(ctx context.Context, id string) error { return nil }

type remoteConfigBackend struct{ cl *client.Client }

func (b *remoteConfigBackend) Get(ctx context.Context) (*ServiceConfig, error) {
	return &ServiceConfig{}, nil
}

type remoteEventsBackend struct{}

func (b *remoteEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	return func() {}, nil
}

// Ensure remoteBackend implements Backend.
var _ Backend = (*remoteBackend)(nil)
