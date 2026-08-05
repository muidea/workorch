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
	client "github.com/muidea/workorch/internal/pkg/workorchclient"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// remoteBackend implements port.Backend using an HTTP client to a running workorch --server.
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

// NewRemote creates a port.Backend that communicates with a remote workorch --server via HTTP.
func NewRemote(addr string) port.Backend {
	return NewRemoteWithOptions(addr)
}

func NewRemoteProjectionEvents(addr string) port.Backend {
	return NewRemoteWithOptions(addr, WithRemoteEventStreamView("projection"))
}

func NewRemoteWithOptions(addr string, opts ...RemoteOption) port.Backend {
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

func (b *remoteBackend) Workspace() port.WorkspaceBackend { return b.ws }
func (b *remoteBackend) Goal() port.GoalBackend           { return b.goal }
func (b *remoteBackend) DoD() port.DoDBackend             { return b.dod }
func (b *remoteBackend) Run() port.RunBackend             { return b.run }
func (b *remoteBackend) Decision() port.DecisionBackend   { return b.dec }
func (b *remoteBackend) Approval() port.ApprovalBackend   { return b.appr }
func (b *remoteBackend) Soul() port.SoulBackend           { return b.soul }
func (b *remoteBackend) Skill() port.SkillBackend         { return b.sk }
func (b *remoteBackend) MCP() port.MCPBackend             { return b.mcp }
func (b *remoteBackend) Memory() port.MemoryBackend       { return b.mem }
func (b *remoteBackend) Template() port.TemplateBackend   { return b.tmpl }
func (b *remoteBackend) Config() port.ConfigBackend       { return b.cfg }
func (b *remoteBackend) Events() port.EventsBackend       { return b.evt }
func (b *remoteBackend) ProjectionEvents() port.WorkspaceProjectionEventsBackend {
	return &remoteProjectionEventsBackend{cl: b.cl}
}

// ── Remote Workspace ──

type remoteWorkspaceBackend struct{ cl *client.Client }

func workspaceProjectionFromClient(ws *projectioncontract.Workspace) projectioncontract.Workspace {
	if ws == nil {
		return projectioncontract.Workspace{}
	}
	return projectioncontract.CloneWorkspace(*ws)
}

func (b *remoteWorkspaceBackend) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	ws, err := b.cl.CreateWorkspace(ctx, req)
	if err != nil {
		return nil, err
	}
	info := workspaceProjectionFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	ws, err := b.cl.GetWorkspace(ctx, string(id))
	if err != nil {
		return nil, err
	}
	info := workspaceProjectionFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) List(ctx context.Context) ([]projectioncontract.Workspace, error) {
	projection, err := b.cl.ListWorkspaceProjection(ctx)
	if err != nil {
		return nil, err
	}
	list := projection.Workspaces
	result := make([]projectioncontract.Workspace, 0, len(list))
	for _, ws := range list {
		item := workspaceProjectionFromClient(&ws)
		result = append(result, item)
	}
	return result, nil
}

func (b *remoteWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	projection, err := b.cl.GetWorkspaceSnapshotProjection(ctx, string(id))
	if err != nil {
		return nil, err
	}
	snapshot := workspaceSnapshotFromClient(projection.Snapshot)
	snapshot.Workspace.Projection = projectioncontract.CloneProjectionStatus(projection.Projection)
	return &snapshot, nil
}

func (b *remoteWorkspaceBackend) RuntimeView(ctx context.Context, req port.ReadWorkspaceRuntimeViewRequest) (*projectioncontract.WorkspaceRuntimeView, error) {
	if b == nil || b.cl == nil {
		return nil, common.ErrInvalidArgument
	}
	if strings.TrimSpace(string(req.WorkspaceID)) == "" {
		return nil, common.ErrInvalidArgument
	}
	projection, err := b.cl.GetWorkspaceRuntimeView(ctx, string(req.WorkspaceID), req.ActivityLimit, req.BeforeCursor)
	if err != nil {
		return nil, err
	}
	view := projectioncontract.CloneWorkspaceRuntimeView(projection.RuntimeView)
	// Prefer envelope projection metadata when present.
	if status := projectioncontract.CloneProjectionStatus(projection.Projection); status.WorkspaceRevision != 0 || status.GlobalRevision != 0 || status.State != "" {
		view.Projection = status
	}
	return &view, nil
}

func (b *remoteWorkspaceBackend) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*projectioncontract.Workspace, error) {
	if b == nil || b.cl == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	var (
		ws  *projectioncontract.Workspace
		err error
	)
	if target == nil {
		ws, err = b.cl.ClearWorkspaceLLMTarget(ctx, string(id))
	} else {
		value := target.Normalized()
		if err := value.Validate(); err != nil {
			return nil, err
		}
		ws, err = b.cl.UpdateWorkspaceLLMTarget(ctx, string(id), value)
	}
	if err != nil {
		return nil, err
	}
	info := workspaceProjectionFromClient(ws)
	return &info, nil
}

func workspaceSnapshotFromClient(input projectioncontract.WorkspaceSnapshot) projectioncontract.WorkspaceSnapshot {
	return projectioncontract.CloneWorkspaceSnapshot(input)
}

func cloneRunProjectionPointer(item *projectioncontract.Run) *projectioncontract.Run {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneRun(*item)
	return &ret
}

func cloneResultProjectionPointer(item *projectioncontract.Result) *projectioncontract.Result {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneResult(*item)
	return &ret
}

func runProjectionsFromClient(items []projectioncontract.Run) []projectioncontract.Run {
	return projectioncontract.CloneRuns(items)
}

func (b *remoteWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return b.cl.DeleteWorkspace(ctx, string(id), purge)
}

// ── Remote Goal ──

type remoteGoalBackend struct{ cl *client.Client }

func (b *remoteGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*workorchmanagementcontract.Goal, error) {
	return b.cl.CreateGoal(ctx, string(wsID), text)
}
func (b *remoteGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.Goal, error) {
	return b.cl.ListGoals(ctx, string(wsID))
}
func (b *remoteGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*workorchmanagementcontract.Goal, error) {
	return b.cl.GetGoal(ctx, string(wsID), string(id))
}
func (b *remoteGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*workorchmanagementcontract.Goal, error) {
	return b.cl.UpdateGoal(ctx, string(wsID), string(id), text)
}

// ── Remote DoD ──

type remoteDoDBackend struct{ cl *client.Client }

func (b *remoteDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*workorchmanagementcontract.DoD, error) {
	return b.cl.CreateDoD(ctx, string(wsID), items)
}
func (b *remoteDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.DoD, error) {
	return b.cl.ListDoDs(ctx, string(wsID))
}
func (b *remoteDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*workorchmanagementcontract.DoD, error) {
	return b.cl.GetDoD(ctx, string(wsID), string(id))
}
func (b *remoteDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*workorchmanagementcontract.DoD, error) {
	return b.cl.UpdateDoD(ctx, string(wsID), string(id), items)
}

// ── Remote Run ──

type remoteRunBackend struct{ cl *client.Client }

func (b *remoteRunBackend) Command(ctx context.Context, req port.WorkspaceRunCommand) (*port.RunCommandResult, error) {
	intent, err := port.ValidateWorkspaceRunCommand(req)
	if err != nil {
		return nil, err
	}
	switch intent {
	case port.RunCommandIntentResume:
		return b.runCommandViaActivation(ctx, req, port.RunCommandIntentResume)
	case port.RunCommandIntentRestart:
		return b.runCommandViaActivation(ctx, req, port.RunCommandIntentRestart)
	default:
		return nil, fmt.Errorf("%w: unsupported run command intent %q", common.ErrInvalidArgument, intent)
	}
}

func (b *remoteRunBackend) runCommandViaActivation(ctx context.Context, req port.WorkspaceRunCommand, intent port.RunCommandIntent) (*port.RunCommandResult, error) {
	payload := &workorchmanagementcontract.WorkspaceExecutionCommandRequest{
		Intent:    string(intent),
		RequestID: req.RequestID,
		Reason:    req.Reason,
		Source:    normalizedRunCommandSource(req.Source),
		Priority:  req.Priority,
	}
	act, err := b.cl.ResumeWorkspaceExecution(ctx, string(req.WorkspaceID), payload)
	if err != nil {
		return nil, err
	}
	// HTTP activation is asynchronous and returns transport acceptance only.
	// The typed owner admission is observed later through projection events; do
	// not synthesize an admission fact from an HTTP 202 response.
	if act == nil {
		return nil, nil
	}
	return nil, fmt.Errorf("%w: synchronous activation response omitted owner admission fact", common.ErrConflict)
}

func remoteRunProjectionFromClient(r *projectioncontract.Run) *projectioncontract.Run {
	if r == nil {
		return nil
	}
	ret := projectioncontract.CloneRun(*r)
	return &ret
}

func (b *remoteRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.Run, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	r, err := b.cl.GetRun(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return remoteRunProjectionFromClient(r), nil
}
func (b *remoteRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Run, error) {
	list, err := b.cl.ListRuns(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	return projectioncontract.CloneRuns(list), nil
}
func (b *remoteRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.RunStatus, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	s, err := b.cl.GetRunStatus(ctx, string(wsID), string(id), "", 0)
	if err != nil {
		return nil, err
	}
	ret := projectioncontract.CloneRunStatus(*s)
	return &ret, nil
}

// ── Remote Decision ──

type remoteDecisionBackend struct{ cl *client.Client }

func (b *remoteDecisionBackend) Resolve(ctx context.Context, req port.ResolveDecisionRequest) (*projectioncontract.Decision, error) {
	d, err := b.cl.ResolveDecision(ctx, string(req.WorkspaceID), string(req.TicketID), req.ResolutionType, req.Payload)
	if err != nil {
		return nil, err
	}
	ret := projectioncontract.CloneDecision(*d)
	return &ret, nil
}
func (b *remoteDecisionBackend) BatchResolve(ctx context.Context, workspaceID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload workorcheventpayload.Payload) ([]projectioncontract.Decision, error) {
	ids := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		ids[i] = string(id)
	}
	list, err := b.cl.BatchResolveDecisions(ctx, string(workspaceID), ids, resolutionType, payload)
	if err != nil {
		return nil, err
	}
	return projectioncontract.CloneDecisions(list), nil
}
func (b *remoteDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Decision, error) {
	list, err := b.cl.ListDecisions(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	return projectioncontract.CloneDecisions(list), nil
}
func (b *remoteDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*projectioncontract.Decision, error) {
	d, err := b.cl.GetDecision(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	ret := projectioncontract.CloneDecision(*d)
	return &ret, nil
}

func payloadFromRemoteMap(input map[string]any) workorcheventpayload.Payload {
	payload, _ := workorcheventpayload.FromMap(input)
	return payload
}

// ── Remote Approval ──

type remoteApprovalBackend struct{ cl *client.Client }

func (b *remoteApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	a, err := b.cl.Approve(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	ret := projectioncontract.CloneApproval(*a)
	return &ret, nil
}
func (b *remoteApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	a, err := b.cl.Reject(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	ret := projectioncontract.CloneApproval(*a)
	return &ret, nil
}
func (b *remoteApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error) {
	_idStrings := make([]string, len(ids))
	for i, id := range ids {
		_idStrings[i] = string(id)
	}
	list, err := b.cl.BatchResolveApprovals(ctx, string(wsID), _idStrings, action, payload...)
	if err != nil {
		return nil, err
	}
	return projectioncontract.CloneApprovals(list), nil
}
func (b *remoteApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Approval, error) {
	list, err := b.cl.ListApprovals(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	return projectioncontract.CloneApprovals(list), nil
}
func (b *remoteApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*projectioncontract.Approval, error) {
	a, err := b.cl.GetApproval(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	ret := projectioncontract.CloneApproval(*a)
	return &ret, nil
}

// ── Stub remote backends ──

type remoteSoulBackend struct{ cl *client.Client }

func (b *remoteSoulBackend) Create(ctx context.Context, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return b.cl.CreateSoul(ctx, s)
}
func (b *remoteSoulBackend) Get(ctx context.Context, id common.SoulID) (*workorchmanagementcontract.Soul, error) {
	return b.cl.GetSoul(ctx, string(id))
}
func (b *remoteSoulBackend) List(ctx context.Context) ([]workorchmanagementcontract.Soul, error) {
	return b.cl.ListSouls(ctx)
}
func (b *remoteSoulBackend) Update(ctx context.Context, id common.SoulID, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return b.cl.UpdateSoul(ctx, string(id), s)
}
func (b *remoteSoulBackend) Delete(ctx context.Context, id common.SoulID) error {
	return b.cl.DeleteSoul(ctx, string(id))
}

type remoteSkillBackend struct{ cl *client.Client }

func (b *remoteSkillBackend) Create(ctx context.Context, s *workorchmanagementcontract.SkillMutation) (*workorchmanagementcontract.Skill, error) {
	return b.cl.CreateSkill(ctx, s)
}
func (b *remoteSkillBackend) Get(ctx context.Context, id common.SkillID) (*workorchmanagementcontract.Skill, error) {
	return b.cl.GetSkill(ctx, string(id), "")
}
func (b *remoteSkillBackend) List(ctx context.Context) ([]workorchmanagementcontract.Skill, error) {
	return b.cl.ListSkills(ctx, "")
}

type remoteMCPBackend struct{ cl *client.Client }

func (b *remoteMCPBackend) CreateServer(ctx context.Context, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return b.cl.CreateMCPServer(ctx, s)
}
func (b *remoteMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*workorchmanagementcontract.MCPServer, error) {
	return b.cl.GetMCPServer(ctx, string(id))
}
func (b *remoteMCPBackend) ListServers(ctx context.Context) ([]workorchmanagementcontract.MCPServer, error) {
	return b.cl.ListMCPServers(ctx)
}
func (b *remoteMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return b.cl.UpdateMCPServer(ctx, string(id), s)
}
func (b *remoteMCPBackend) ListCapabilities(ctx context.Context) ([]workorchmanagementcontract.MCPCapability, error) {
	return b.cl.ListMCPCapabilities(ctx)
}

type remoteMemoryBackend struct{ cl *client.Client }

func (b *remoteMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	item, err := b.cl.CreateMemory(ctx, string(wsID), m)
	if err != nil {
		return nil, err
	}
	return item, nil
}
func (b *remoteMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*workorchmanagementcontract.MemoryItem, error) {
	item, err := b.cl.GetMemory(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return item, nil
}
func (b *remoteMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	item, err := b.cl.UpdateMemory(ctx, string(wsID), string(id), m)
	if err != nil {
		return nil, err
	}
	return item, nil
}
func (b *remoteMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return b.cl.DeleteMemory(ctx, string(wsID), string(id))
}
func (b *remoteMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.MemoryItem, error) {
	return b.Search(ctx, wsID, workorchmanagementcontract.MemorySearchQuery{})
}
func (b *remoteMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req workorchmanagementcontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	req.WorkspaceID = string(wsID)
	return b.cl.SearchMemory(ctx, req)
}

type remoteTemplateBackend struct{ cl *client.Client }

func (b *remoteTemplateBackend) Create(ctx context.Context, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return b.cl.CreateWorkspaceTemplate(ctx, t)
}
func (b *remoteTemplateBackend) Get(ctx context.Context, id string) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return b.cl.GetWorkspaceTemplate(ctx, id)
}
func (b *remoteTemplateBackend) List(ctx context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	return b.cl.ListWorkspaceTemplates(ctx)
}
func (b *remoteTemplateBackend) Update(ctx context.Context, id string, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return b.cl.UpdateWorkspaceTemplate(ctx, id, t)
}
func (b *remoteTemplateBackend) Delete(ctx context.Context, id string) error {
	return b.cl.DeleteWorkspaceTemplate(ctx, id)
}

type remoteConfigBackend struct{ cl *client.Client }

func (b *remoteConfigBackend) Get(ctx context.Context) (*port.ServiceConfig, error) {
	cfg, err := b.cl.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &port.ServiceConfig{}, nil
	}
	return port.ServiceConfigPtrFromConfiguration(cfg), nil
}

func (b *remoteConfigBackend) ListChatModelTargets(ctx context.Context) ([]port.ChatModelTargetOption, error) {
	configuration, err := b.cl.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	catalog, err := b.cl.GetLLMModelCatalog(ctx, runtimecontract.BuiltinAIProxyProviderID)
	if err != nil {
		return nil, err
	}
	if configuration != nil && catalog != nil {
		configuration.LLM.Catalog = *catalog
	}
	return port.ChatModelTargetOptionsFromConfiguration(configuration), nil
}

func (b *remoteConfigBackend) RefreshChatModelTargets(ctx context.Context) ([]port.ChatModelTargetOption, error) {
	if _, err := b.cl.RefreshLLMModelCatalog(ctx, runtimecontract.BuiltinAIProxyProviderID); err != nil {
		return nil, err
	}
	return b.ListChatModelTargets(ctx)
}

type remoteEventsBackend struct {
	cl          *client.Client
	streamView  string
	mu          sync.Mutex
	lastEventID string
}

func (b *remoteEventsBackend) Subscribe(ctx context.Context, handler port.EventHandler) (func(), error) {
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

func (b *remoteEventsBackend) streamEvents(ctx context.Context, handler port.EventHandler) {
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

func (b *remoteEventsBackend) streamEventsOnce(ctx context.Context, handler port.EventHandler) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.cl.BaseURL()+"/api/v1/workorch/events/stream", nil)
	if err != nil {
		return false
	}
	query := req.URL.Query()
	if streamView := strings.TrimSpace(b.streamView); streamView != "" {
		query.Set("view", streamView)
	}
	if lastEventID := b.lastEventIDSnapshot(); lastEventID != "" {
		req.Header.Set("Last-port.Event-ID", lastEventID)
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
	handler(ctx, port.Event{
		Type: port.EventWorkspaceListChanged,
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

func (b *remoteEventsBackend) dispatchSSEData(ctx context.Context, lines []string, eventID string, handler port.EventHandler) {
	if len(lines) == 0 {
		return
	}
	var envelope backendEventData
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &envelope); err != nil {
		return
	}
	evt := convertBackendEvent(envelope)
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

// ── Remote Projection Events port.Backend (typed SSE) ──

type remoteProjectionEventsBackend struct {
	cl          *client.Client
	mu          sync.Mutex
	lastEventID string
}

func (b *remoteProjectionEventsBackend) SubscribeWorkspaceProjection(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) (func(), error) {
	if b == nil || b.cl == nil {
		return func() {}, fmt.Errorf("%w: remote projection backend unavailable", common.ErrInvalidArgument)
	}
	if onChanged == nil {
		return func() {}, fmt.Errorf("%w: projection changed handler required", common.ErrInvalidArgument)
	}
	streamCtx, cancel := context.WithCancel(ctx)
	var once sync.Once
	go b.streamProjectionEvents(streamCtx, onChanged, onStatus)
	return func() {
		once.Do(cancel)
	}, nil
}

func (b *remoteProjectionEventsBackend) streamProjectionEvents(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) {
	publishStatus := func(status port.SubscriptionStatus) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	publishStatus(port.SubscriptionStatus{State: port.SubscriptionConnecting})
	backoff := 200 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		connected, liveRevision := b.streamProjectionEventsOnce(ctx, onChanged, onStatus)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = 200 * time.Millisecond
			continue
		}
		publishStatus(port.SubscriptionStatus{
			State:          port.SubscriptionReconnecting,
			StreamRevision: liveRevision,
			LastError:      "projection sse disconnected",
		})
		if backoff < 2*time.Second {
			backoff *= 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			publishStatus(port.SubscriptionStatus{State: port.SubscriptionConnecting, StreamRevision: liveRevision})
		}
	}
}

func (b *remoteProjectionEventsBackend) streamProjectionEventsOnce(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) (bool, int64) {
	publishStatus := func(status port.SubscriptionStatus) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.cl.BaseURL()+"/api/v1/workorch/events/stream", nil)
	if err != nil {
		publishStatus(port.SubscriptionStatus{State: port.SubscriptionDegraded, LastError: err.Error()})
		return false, 0
	}
	query := req.URL.Query()
	query.Set("view", "projection")
	if lastEventID := b.lastEventIDSnapshot(); lastEventID != "" {
		req.Header.Set("Last-port.Event-ID", lastEventID)
		query.Set("lastEventId", lastEventID)
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "text/event-stream")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		publishStatus(port.SubscriptionStatus{State: port.SubscriptionDegraded, LastError: err.Error()})
		return false, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		publishStatus(port.SubscriptionStatus{State: port.SubscriptionDegraded, LastError: fmt.Sprintf("projection sse status %d", resp.StatusCode)})
		return false, 0
	}
	// Connected: only now report live.
	publishStatus(port.SubscriptionStatus{State: port.SubscriptionLive})
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	var dataLines []string
	var eventID string
	var lastRevision int64
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return true, lastRevision
		default:
		}
		line := scanner.Text()
		if line == "" {
			info, ok := decodeProjectionSSE(dataLines, eventID)
			dataLines = nil
			eventID = ""
			if !ok {
				continue
			}
			b.rememberLastEventID(info.ID)
			if info.Revision.StreamRevision > lastRevision {
				lastRevision = info.Revision.StreamRevision
			}
			if onStatus != nil {
				onStatus(ctx, port.SubscriptionStatus{
					State:            port.SubscriptionLive,
					StreamGeneration: info.Revision.StreamGeneration,
					StreamRevision:   info.Revision.StreamRevision,
					LastEventAt:      info.UpdatedAt,
				})
			}
			onChanged(ctx, info)
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
	return false, lastRevision
}

type workspaceProjectionChangedSSE struct {
	ID               string `json:"id"`
	StreamGeneration string `json:"streamGeneration,omitempty"`
	StreamRevision   int64  `json:"streamRevision"`
	WorkspaceID      string `json:"workspaceId"`
	RunID            string `json:"runId,omitempty"`
	Revision         struct {
		StreamGeneration        string `json:"streamGeneration,omitempty"`
		StreamRevision          int64  `json:"streamRevision,omitempty"`
		WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
		ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
		RuntimeStateRevision    uint64 `json:"runtimeStateRevision,omitempty"`
		EvidenceSequence        uint64 `json:"evidenceSequence,omitempty"`
		ActivityRevision        int64  `json:"activityRevision,omitempty"`
	} `json:"revision"`
	ChangedSections []string `json:"changedSections"`
	ProjectionState string   `json:"projectionState"`
	UpdatedAt       string   `json:"updatedAt"`
}

// DecodeProjectionSSE is the production remote projection SSE decoder entry point.
// Cross-boundary contract tests must call this (not a copied DTO + json.Unmarshal)
// against frames produced by the server serializer (CR06).
func DecodeProjectionSSE(lines []string, eventID string) (projectioncontract.WorkspaceProjectionChanged, bool) {
	return decodeProjectionSSE(lines, eventID)
}

func decodeProjectionSSE(lines []string, eventID string) (projectioncontract.WorkspaceProjectionChanged, bool) {
	if len(lines) == 0 {
		return projectioncontract.WorkspaceProjectionChanged{}, false
	}
	var dto workspaceProjectionChangedSSE
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &dto); err != nil {
		return projectioncontract.WorkspaceProjectionChanged{}, false
	}
	// Accept both top-level concrete DTO and legacy envelope topic form.
	if strings.TrimSpace(dto.WorkspaceID) == "" {
		return projectioncontract.WorkspaceProjectionChanged{}, false
	}
	sections := make([]projectioncontract.WorkspaceProjectionSection, 0, len(dto.ChangedSections))
	for _, section := range dto.ChangedSections {
		if strings.TrimSpace(section) == "" {
			continue
		}
		sections = append(sections, projectioncontract.WorkspaceProjectionSection(section))
	}
	streamRevision := dto.StreamRevision
	if streamRevision == 0 {
		streamRevision = dto.Revision.StreamRevision
	}
	id := strings.TrimSpace(dto.ID)
	if id == "" {
		id = strings.TrimSpace(eventID)
	}
	streamGeneration := strings.TrimSpace(dto.StreamGeneration)
	if streamGeneration == "" {
		streamGeneration = strings.TrimSpace(dto.Revision.StreamGeneration)
	}
	return projectioncontract.WorkspaceProjectionChanged{
		ID:          id,
		WorkspaceID: dto.WorkspaceID,
		RunID:       dto.RunID,
		Revision: projectioncontract.WorkspaceRefreshRevision{
			StreamGeneration:        streamGeneration,
			StreamRevision:          streamRevision,
			WorkspaceRevision:       dto.Revision.WorkspaceRevision,
			ProjectedSourceRevision: dto.Revision.ProjectedSourceRevision,
			RuntimeStateRevision:    dto.Revision.RuntimeStateRevision,
			EvidenceSequence:        dto.Revision.EvidenceSequence,
			ActivityRevision:        dto.Revision.ActivityRevision,
		},
		ChangedSections: sections,
		ProjectionState: dto.ProjectionState,
		UpdatedAt:       dto.UpdatedAt,
	}, true
}

func (b *remoteProjectionEventsBackend) lastEventIDSnapshot() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastEventID
}

func (b *remoteProjectionEventsBackend) rememberLastEventID(id string) {
	if b == nil || strings.TrimSpace(id) == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastEventID = strings.TrimSpace(id)
}

// Ensure remoteBackend implements port.Backend.
var _ port.Backend = (*remoteBackend)(nil)
