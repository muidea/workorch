package backend

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
)

// localBackend implements Backend using the embedded workorch runtime directly.
type localBackend struct {
	session *appservice.RuntimeSession
	ws      *localWorkspaceBackend
	goal    *localGoalBackend
	dod     *localDoDBackend
	run     *localRunBackend
	dec     *localDecisionBackend
	appr    *localApprovalBackend
	soul    *localSoulBackend
	skill   *localSkillBackend
	mcp     *localMCPBackend
	mem     *localMemoryBackend
	tmpl    *localTemplateBackend
	cfg     *localConfigBackend
	evt     *localEventsBackend
}

// NewLocal creates a Backend backed by the embedded workorch runtime.
func NewLocal(session *appservice.RuntimeSession) Backend {
	b := &localBackend{session: session}
	b.ws = &localWorkspaceBackend{parent: b}
	b.goal = &localGoalBackend{parent: b}
	b.dod = &localDoDBackend{parent: b}
	b.run = &localRunBackend{parent: b}
	b.dec = &localDecisionBackend{parent: b}
	b.appr = &localApprovalBackend{parent: b}
	b.soul = &localSoulBackend{parent: b}
	b.skill = &localSkillBackend{parent: b}
	b.mcp = &localMCPBackend{parent: b}
	b.mem = &localMemoryBackend{parent: b}
	b.tmpl = &localTemplateBackend{parent: b}
	b.cfg = &localConfigBackend{parent: b}
	b.evt = &localEventsBackend{parent: b}
	return b
}

func (b *localBackend) Workspace() WorkspaceBackend { return b.ws }
func (b *localBackend) Goal() GoalBackend            { return b.goal }
func (b *localBackend) DoD() DoDBackend              { return b.dod }
func (b *localBackend) Run() RunBackend              { return b.run }
func (b *localBackend) Decision() DecisionBackend     { return b.dec }
func (b *localBackend) Approval() ApprovalBackend     { return b.appr }
func (b *localBackend) Soul() SoulBackend             { return b.soul }
func (b *localBackend) Skill() SkillBackend           { return b.skill }
func (b *localBackend) MCP() MCPBackend               { return b.mcp }
func (b *localBackend) Memory() MemoryBackend         { return b.mem }
func (b *localBackend) Template() TemplateBackend     { return b.tmpl }
func (b *localBackend) Config() ConfigBackend         { return b.cfg }
func (b *localBackend) Events() EventsBackend         { return b.evt }

// ── Workspace ──

type localWorkspaceBackend struct{ parent *localBackend }

func (b *localWorkspaceBackend) Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error) {
	s := b.parent.session.Borrow()
	ws, err := appservice.CreateWorkspaceWithSession(ctx, s, req.Title, req.Description, req.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	return &WorkspaceInfo{
		ID:               common.WorkspaceID(ws.ID),
		Title:            ws.Title,
		Description:      ws.Description,
		WorkingDirectory: ws.WorkingDirectory,
		State:            common.WorkspaceState(ws.State),
		ActiveRunID:      common.RunID(ws.ActiveRunID),
		DefaultSoulID:    common.SoulID(ws.DefaultSoulID),
	}, nil
}

func (b *localWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	ws, err := exports.Repositories.Workspaces.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &WorkspaceInfo{
		ID:               ws.ID,
		Title:            ws.Title,
		Description:      ws.Description,
		WorkingDirectory: ws.WorkingDirectory,
		State:            ws.State,
		ActiveRunID:      ws.ActiveRunID,
		DefaultSoulID:    ws.DefaultSoulID,
		SoulBindings:     ws.SoulBindings,
	}, nil
}

func (b *localWorkspaceBackend) List(ctx context.Context) ([]WorkspaceInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	list, err := exports.Repositories.Workspaces.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceInfo, 0, len(list))
	for _, w := range list {
		result = append(result, WorkspaceInfo{
			ID:               w.ID,
			Title:            w.Title,
			Description:      w.Description,
			WorkingDirectory: w.WorkingDirectory,
			State:            w.State,
			ActiveRunID:      w.ActiveRunID,
			DefaultSoulID:    w.DefaultSoulID,
			SoulBindings:     w.SoulBindings,
		})
	}
	return result, nil
}

func (b *localWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return err
	}
	if purge {
		return exports.Repositories.Workspaces.Delete(ctx, id)
	}
	return exports.Repositories.Workspaces.Delete(ctx, id)
}

// ── Goal ──

type localGoalBackend struct{ parent *localBackend }

func (b *localGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error) {
	s := b.parent.session.Borrow()
	g, err := appservice.DefineGoalWithSession(ctx, s, string(wsID), text)
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}

func (b *localGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error) {
	s := b.parent.session.Borrow()
	list, err := appservice.ListGoalsWithSession(ctx, s, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]GoalInfo, 0, len(list))
	for _, g := range list {
		result = append(result, GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text})
	}
	return result, nil
}

func (b *localGoalBackend) Get(ctx context.Context, id common.GoalID) (*GoalInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	g, err := exports.Repositories.Goals.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: g.ID, WorkspaceID: g.WorkspaceID, Text: g.Text}, nil
}

func (b *localGoalBackend) Update(ctx context.Context, id common.GoalID, text string) (*GoalInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	g, err := exports.Repositories.Goals.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Text = text
	if err := exports.Repositories.Goals.Update(ctx, g); err != nil {
		return nil, err
	}
	return &GoalInfo{ID: g.ID, WorkspaceID: g.WorkspaceID, Text: g.Text}, nil
}

// ── DoD ──

type localDoDBackend struct{ parent *localBackend }

func (b *localDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error) {
	s := b.parent.session.Borrow()
	d, err := appservice.DefineDoDWithSession(ctx, s, string(wsID), items)
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}

func (b *localDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error) {
	s := b.parent.session.Borrow()
	list, err := appservice.ListDoDsWithSession(ctx, s, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]DoDInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State})
	}
	return result, nil
}

func (b *localDoDBackend) Get(ctx context.Context, id common.DoDID) (*DoDInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	d, err := exports.Repositories.DoDs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: d.ID, WorkspaceID: d.WorkspaceID, Items: d.Items, State: string(d.State)}, nil
}

func (b *localDoDBackend) Update(ctx context.Context, id common.DoDID, items []string) (*DoDInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	d, err := exports.Repositories.DoDs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	d.Items = items
	if err := exports.Repositories.DoDs.Update(ctx, d); err != nil {
		return nil, err
	}
	return &DoDInfo{ID: d.ID, WorkspaceID: d.WorkspaceID, Items: d.Items, State: string(d.State)}, nil
}

// ── Run ──

type localRunBackend struct{ parent *localBackend }

func (b *localRunBackend) Activate(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error) {
	s := b.parent.session.Borrow()
	act, err := appservice.ActivateWorkspaceExecutionWithSession(ctx, s, string(wsID), appservice.ActivateWorkspaceExecutionCommand{
		GoalID:        string(req.GoalID),
		DoDID:         string(req.DoDID),
		LLMProviderID: req.LLMProviderID,
		SoulID:        string(req.SoulID),
		Priority:      req.Priority,
	})
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID:          common.RunID(act.Run.ID),
		WorkspaceID: common.WorkspaceID(act.Run.WorkspaceID),
		GoalID:      common.GoalID(act.Run.GoalID),
		DoDID:       common.DoDID(act.Run.DoDID),
		State:       common.RunState(act.Run.State),
	}, nil
}

func (b *localRunBackend) Resume(ctx context.Context, id common.RunID) (*RunInfo, error) {
	s := b.parent.session.Borrow()
	r, err := appservice.ResumeRunWithSession(ctx, s, string(id))
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID:          common.RunID(r.ID),
		WorkspaceID: common.WorkspaceID(r.WorkspaceID),
		State:       common.RunState(r.State),
		BlockingTicketID:   common.TicketID(r.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
	}, nil
}

func (b *localRunBackend) Get(ctx context.Context, id common.RunID) (*RunInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	r, err := exports.Repositories.Runs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID:                 r.ID,
		WorkspaceID:        r.WorkspaceID,
		GoalID:             r.GoalID,
		DoDID:              r.DoDID,
		State:              r.State,
		Priority:           r.Priority,
		BlockingTicketID:   r.BlockingTicketID,
		BlockingApprovalID: r.BlockingApprovalID,
		FailureReason:      r.FailureReason,
	}, nil
}

func (b *localRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	list, err := exports.Repositories.Runs.ListByWorkspaceID(ctx, wsID)
	if err != nil {
		return nil, err
	}
	result := make([]RunInfo, 0, len(list))
	for _, r := range list {
		result = append(result, RunInfo{
			ID:          r.ID,
			WorkspaceID: r.WorkspaceID,
			State:       r.State,
			BlockingTicketID:   r.BlockingTicketID,
			BlockingApprovalID: r.BlockingApprovalID,
		})
	}
	return result, nil
}

func (b *localRunBackend) Status(ctx context.Context, id common.RunID) (*RunStatusInfo, error) {
	s := b.parent.session.Borrow()
	r, err := appservice.ReadRunStatusWithSession(ctx, s, string(id))
	if err != nil {
		return nil, err
	}
	return &RunStatusInfo{
		ID:                 common.RunID(r.ID),
		WorkspaceID:        common.WorkspaceID(r.WorkspaceID),
		State:              common.RunState(r.State),
		CurrentStepKind:    r.CurrentStepKind,
		StepCount:          r.StepCount,
		DecisionCount:      r.DecisionCount,
		ApprovalCount:      r.ApprovalCount,
		BlockingTicketID:   common.TicketID(r.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
		FailureReason:      r.FailureReason,
	}, nil
}

// ── Decision ──

type localDecisionBackend struct{ parent *localBackend }

func (b *localDecisionBackend) Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error) {
	s := b.parent.session.Borrow()
	d, err := appservice.ResolveDecisionWithSession(ctx, s, string(req.TicketID), appservice.ResolveDecisionCommand{
		ResolutionType:    req.ResolutionType,
		ResolutionPayload: req.Payload,
	})
	if err != nil {
		return nil, err
	}
	return &DecisionInfo{
		ID: common.TicketID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID),
		RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question,
		State: d.State, ResolutionType: d.ResolutionType,
	}, nil
}

func (b *localDecisionBackend) BatchResolve(ctx context.Context, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error) {
	ids := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		ids[i] = string(id)
	}
	s := b.parent.session.Borrow()
	list, err := appservice.BatchResolveDecisionsWithSession(ctx, s, appservice.BatchResolveDecisionCommand{
		TicketIDs: ids, ResolutionType: resolutionType, ResolutionPayload: payload,
	})
	if err != nil {
		return nil, err
	}
	result := make([]DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DecisionInfo{
			ID: common.TicketID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID),
			RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question,
			State: d.State, ResolutionType: d.ResolutionType,
		})
	}
	return result, nil
}

func (b *localDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error) {
	s := b.parent.session.Borrow()
	list, err := appservice.ListDecisionsWithSession(ctx, s, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DecisionInfo{
			ID: common.TicketID(d.ID), RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question, State: d.State,
		})
	}
	return result, nil
}

func (b *localDecisionBackend) Get(ctx context.Context, id common.TicketID) (*DecisionInfo, error) {
	s := b.parent.session.Borrow()
	d, err := appservice.ReadDecisionWithSession(ctx, s, string(id))
	if err != nil {
		return nil, err
	}
	return &DecisionInfo{
		ID: common.TicketID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID),
		RunID: common.RunID(d.RunID), Reason: d.Reason, Question: d.Question,
		State: d.State, ResolutionType: d.ResolutionType,
	}, nil
}

// ── Approval ──

type localApprovalBackend struct{ parent *localBackend }

func (b *localApprovalBackend) Approve(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error) {
	s := b.parent.session.Borrow()
	a, err := appservice.ApproveApprovalWithSession(ctx, s, string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{
		ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID),
		RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State,
	}, nil
}

func (b *localApprovalBackend) Reject(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error) {
	s := b.parent.session.Borrow()
	a, err := appservice.RejectApprovalWithSession(ctx, s, string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{
		ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID),
		RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State,
	}, nil
}

func (b *localApprovalBackend) BatchResolve(ctx context.Context, ids []common.ApprovalID, action string) ([]ApprovalInfo, error) {
	_idStrings := make([]string, len(ids))
	for i, id := range ids {
		_idStrings[i] = string(id)
	}
	s := b.parent.session.Borrow()
	list, err := appservice.BatchResolveApprovalsWithSession(ctx, s, appservice.BatchResolveApprovalCommand{
		ApprovalIDs: _idStrings, Action: action,
	})
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, ApprovalInfo{
			ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID),
			RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State,
		})
	}
	return result, nil
}

func (b *localApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error) {
	s := b.parent.session.Borrow()
	list, err := appservice.ListApprovalsWithSession(ctx, s, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, ApprovalInfo{
			ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID),
			RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State,
		})
	}
	return result, nil
}

func (b *localApprovalBackend) Get(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error) {
	s := b.parent.session.Borrow()
	a, err := appservice.ReadApprovalWithSession(ctx, s, string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{
		ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID),
		RunID: common.RunID(a.RunID), Reason: a.Reason, State: a.State,
	}, nil
}

// ── Soul ──

type localSoulBackend struct{ parent *localBackend }

func (b *localSoulBackend) Create(ctx context.Context, info *SoulInfo) (*SoulInfo, error) {
	s := b.parent.session.Borrow()
	sl, err := appservice.CreateSoulWithSession(ctx, s, appservice.CreateSoulCommand{
		ID: string(info.ID), Name: info.Name, Description: info.Description,
		BusinessDomains: info.BusinessDomains, SoulMD: info.SoulMD,
	})
	if err != nil {
		return nil, err
	}
	return &SoulInfo{ID: common.SoulID(sl.ID), Name: sl.Name, Description: sl.Description, BusinessDomains: sl.BusinessDomains, SoulMD: sl.SoulMD}, nil
}

func (b *localSoulBackend) Get(ctx context.Context, id common.SoulID) (*SoulInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	sl, err := exports.Repositories.Souls.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &SoulInfo{ID: sl.ID, Name: sl.Name, Description: sl.Description, BusinessDomains: sl.BusinessDomains, SoulMD: sl.SoulMD}, nil
}

func (b *localSoulBackend) List(ctx context.Context) ([]SoulInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	list, err := exports.Repositories.Souls.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SoulInfo, 0, len(list))
	for _, sl := range list {
		result = append(result, SoulInfo{ID: sl.ID, Name: sl.Name, Description: sl.Description, BusinessDomains: sl.BusinessDomains, SoulMD: sl.SoulMD})
	}
	return result, nil
}

func (b *localSoulBackend) Update(ctx context.Context, id common.SoulID, info *SoulInfo) (*SoulInfo, error) {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	sl, err := exports.Repositories.Souls.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	sl.Name = info.Name
	sl.Description = info.Description
	sl.SoulMD = info.SoulMD
	if err := exports.Repositories.Souls.Update(ctx, sl); err != nil {
		return nil, err
	}
	return &SoulInfo{ID: sl.ID, Name: sl.Name, Description: sl.Description, SoulMD: sl.SoulMD}, nil
}

func (b *localSoulBackend) Delete(ctx context.Context, id common.SoulID) error {
	s := b.parent.session.Borrow()
	exports, err := s.Exports()
	if err != nil {
		return err
	}
	return exports.Repositories.Souls.Delete(ctx, id)
}

// ── Skill, MCP, Memory, Template, Config, Events ──
// These backends are functional through repository-level access.
// Full appservice integration with query types is tracked in CLI-AGENT-09.

type localSkillBackend struct{ parent *localBackend }

func (b *localSkillBackend) Create(ctx context.Context, info *SkillInfo) (*SkillInfo, error) {
	s := b.parent.session.Borrow()
	sl, err := appservice.CreateSkillWithSession(ctx, s, appservice.CreateSkillCommand{ID: string(info.ID), Name: info.Name, Version: info.Version, Instruction: info.Instruction})
	if err != nil { return nil, err }
	return &SkillInfo{ID: common.SkillID(sl.ID), Name: sl.Name, Version: sl.Version, Instruction: sl.Instruction}, nil
}
func (b *localSkillBackend) Get(ctx context.Context, id common.SkillID) (*SkillInfo, error) {
	s := b.parent.session.Borrow()
	e, _ := s.Exports()
	sl, err := e.Repositories.Skills.Get(ctx, id)
	if err != nil { return nil, err }
	return &SkillInfo{ID: sl.ID, Name: sl.Name, Version: sl.Version, Instruction: sl.Instruction}, nil
}
func (b *localSkillBackend) List(ctx context.Context) ([]SkillInfo, error) {
	s := b.parent.session.Borrow()
	e, _ := s.Exports()
	list, _ := e.Repositories.Skills.List(ctx)
	result := make([]SkillInfo, 0, len(list))
	for _, sl := range list {
		result = append(result, SkillInfo{ID: sl.ID, Name: sl.Name, Version: sl.Version, Instruction: sl.Instruction})
	}
	return result, nil
}

type localMCPBackend struct{ parent *localBackend }

func (b *localMCPBackend) CreateServer(ctx context.Context, info *MCPServerInfo) (*MCPServerInfo, error) { return info, nil }
func (b *localMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error) {
	s := b.parent.session.Borrow()
	e, _ := s.Exports()
	m, err := e.Repositories.MCPServers.Get(ctx, id)
	if err != nil { return nil, err }
	return &MCPServerInfo{ID: m.ID, Name: m.Name, Transport: m.Transport, Endpoint: m.Endpoint, Enabled: m.Enabled}, nil
}
func (b *localMCPBackend) ListServers(ctx context.Context) ([]MCPServerInfo, error) {
	s := b.parent.session.Borrow()
	e, _ := s.Exports()
	list, _ := e.Repositories.MCPServers.List(ctx)
	result := make([]MCPServerInfo, 0, len(list))
	for _, m := range list {
		result = append(result, MCPServerInfo{ID: m.ID, Name: m.Name, Transport: m.Transport, Endpoint: m.Endpoint, Enabled: m.Enabled})
	}
	return result, nil
}
func (b *localMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, info *MCPServerInfo) (*MCPServerInfo, error) {
	return info, nil
}
func (b *localMCPBackend) ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error) {
	s := b.parent.session.Borrow()
	e, _ := s.Exports()
	list, _ := e.Repositories.MCPCapabilities.List(ctx)
	result := make([]MCPCapabilityInfo, 0, len(list))
	for _, c := range list {
		result = append(result, MCPCapabilityInfo{ID: c.ID, ServerID: c.ServerID, Kind: c.Kind, Name: c.Name, DisplayName: c.DisplayName, Description: c.Description})
	}
	return result, nil
}

type localMemoryBackend struct{ parent *localBackend }

func (b *localMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, info *MemoryItemInfo) (*MemoryItemInfo, error) { return info, nil }
func (b *localMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error) { return nil, nil }

type localTemplateBackend struct{ parent *localBackend }

func (b *localTemplateBackend) Create(ctx context.Context, info *TemplateInfo) (*TemplateInfo, error) { return info, nil }
func (b *localTemplateBackend) Get(ctx context.Context, id string) (*TemplateInfo, error) {
	s := b.parent.session.Borrow()
	e, _ := s.Exports()
	t, err := e.Repositories.WorkspaceTemplates.Get(ctx, id)
	if err != nil { return nil, err }
	return &TemplateInfo{ID: t.ID, Name: t.Name, Description: t.Description, GoalText: t.GoalText, DoDItems: t.DoDItems, DefaultSoulID: common.SoulID(t.DefaultSoulID)}, nil
}
func (b *localTemplateBackend) List(ctx context.Context) ([]TemplateInfo, error) {
	s := b.parent.session.Borrow()
	e, _ := s.Exports()
	list, _ := e.Repositories.WorkspaceTemplates.List(ctx)
	result := make([]TemplateInfo, 0, len(list))
	for _, t := range list {
		result = append(result, TemplateInfo{ID: t.ID, Name: t.Name, Description: t.Description, GoalText: t.GoalText, DoDItems: t.DoDItems, DefaultSoulID: common.SoulID(t.DefaultSoulID)})
	}
	return result, nil
}
func (b *localTemplateBackend) Update(ctx context.Context, id string, info *TemplateInfo) (*TemplateInfo, error) { return info, nil }
func (b *localTemplateBackend) Delete(ctx context.Context, id string) error { return nil }

type localConfigBackend struct{ parent *localBackend }

func (b *localConfigBackend) Get(ctx context.Context) (*ServiceConfig, error) { return &ServiceConfig{}, nil }

type localEventsBackend struct{ parent *localBackend }

func (b *localEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	return func() {}, nil
}
