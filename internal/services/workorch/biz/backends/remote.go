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

func workspaceInfoFromClient(ws *projectioncontract.Workspace) port.WorkspaceInfo {
	if ws == nil {
		return port.WorkspaceInfo{}
	}
	return port.WorkspaceInfo{
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
		DefaultLLMTarget:     runtimecontract.CloneProviderModelTarget(ws.DefaultLLMTarget),
		DefaultSoulID:        common.SoulID(ws.DefaultSoulID),
		SoulBindings:         commonSoulBindings(ws.SoulBindings),
		ContextHints:         payloadFromRemoteMap(ws.ContextHints),
		EvidencePolicies:     payloadsFromRemoteMaps(ws.EvidencePolicies),
		Projection:           port.CloneProjectionStatus(ws.Projection),
		ViewModel:            port.WorkspaceViewModelInfoFromProjection(ws.ViewModel),
	}
}

func (b *remoteWorkspaceBackend) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*port.WorkspaceInfo, error) {
	ws, err := b.cl.CreateWorkspace(ctx, req)
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*port.WorkspaceInfo, error) {
	ws, err := b.cl.GetWorkspace(ctx, string(id))
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) List(ctx context.Context) ([]port.WorkspaceInfo, error) {
	projection, err := b.cl.ListWorkspaceProjection(ctx)
	if err != nil {
		return nil, err
	}
	list := projection.Workspaces
	result := make([]port.WorkspaceInfo, 0, len(list))
	for _, ws := range list {
		item := workspaceInfoFromClient(&ws)
		result = append(result, item)
	}
	return result, nil
}

func (b *remoteWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*port.WorkspaceSnapshotInfo, error) {
	projection, err := b.cl.GetWorkspaceSnapshotProjection(ctx, string(id))
	if err != nil {
		return nil, err
	}
	snapshot := workspaceSnapshotInfoFromClient(projection.Snapshot)
	snapshot.Projection = port.CloneProjectionStatus(projection.Projection)
	snapshot.Workspace.Projection = snapshot.Projection
	return &snapshot, nil
}

func (b *remoteWorkspaceBackend) RuntimeView(ctx context.Context, req port.ReadWorkspaceRuntimeViewRequest) (*port.WorkspaceRuntimeViewInfo, error) {
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
	view := port.WorkspaceRuntimeViewInfoFromProjection(projection.RuntimeView)
	// Prefer envelope projection metadata when present.
	if status := port.CloneProjectionStatus(projection.Projection); status.WorkspaceRevision != 0 || status.GlobalRevision != 0 || status.State != "" {
		view.Projection = status
	}
	return &view, nil
}

func (b *remoteWorkspaceBackend) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*port.WorkspaceInfo, error) {
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
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func workspaceSnapshotInfoFromClient(input projectioncontract.WorkspaceSnapshot) port.WorkspaceSnapshotInfo {
	return port.WorkspaceSnapshotInfo{
		Workspace:               workspaceInfoFromClient(&input.Workspace),
		ViewModel:               port.WorkspaceViewModelInfoFromProjection(input.ViewModel),
		Projection:              port.CloneProjectionStatus(input.Workspace.Projection),
		Outcome:                 port.WorkspaceOutcomeInfoFromProjection(input.Outcome),
		TaskProgress:            port.TaskProgressInfoFromProjection(input.TaskProgress),
		RunHeartbeat:            port.RunHeartbeatInfoFromProjection(input.RunHeartbeat),
		ConvergenceStatus:       port.ConvergenceStatusInfoFromProjection(input.ConvergenceStatus),
		RunCommand:              port.RunCommandStatusInfoFromProjection(input.RunCommand),
		SchedulerDiagnostics:    projectioncontract.CloneSchedulerDiagnostics(input.SchedulerDiagnostics),
		ContextBundle:           projectioncontract.CloneContextBundle(input.ContextBundle),
		CoverageSummary:         payloadFromRemoteMap(input.CoverageSummary),
		GapBaseline:             port.GapBaselineInfoFromProjection(input.GapBaseline),
		Acceptance:              port.AcceptanceProjectionInfoFromProjection(input.Acceptance),
		TaskDecompositions:      port.TaskDecompositionInfosFromProjection(input.TaskDecompositions),
		TaskPlans:               port.TaskPlanInfosFromProjection(input.TaskPlans),
		Goals:                   port.GoalInfosFromManagement(input.Goals),
		DoDs:                    port.DoDInfosFromManagement(input.DoDs),
		Runs:                    runInfosFromClient(input.Runs),
		Results:                 port.ResultInfosFromProjection(input.Results),
		Artifacts:               port.ArtifactInfosFromProjection(input.Artifacts),
		ArtifactContinuations:   port.ArtifactContinuationInfosFromProjection(input.ArtifactContinuations),
		Patches:                 port.PatchInfosFromProjection(input.Patches),
		MemoryRecallTraces:      port.MemoryRecallTraceInfosFromProjection(input.MemoryRecallTraces),
		LLMContextBudgets:       port.LLMContextBudgetTraceInfosFromProjection(input.LLMContextBudgets),
		LLMContextBudgetSummary: input.LLMContextBudgetSummary,
		LLMUsageTraces:          port.LLMUsageTraceInfosFromProjection(input.LLMUsageTraces),
		LLMUsageSummary:         port.LLMUsageSummaryInfoFromProjection(input.LLMUsageSummary),
		EventRecords:            port.EventRecordInfosFromProjection(input.EventRecords),
		Decisions:               port.DecisionInfosFromProjection(input.Decisions),
		Approvals:               port.ApprovalInfosFromProjection(input.Approvals),
	}
}

func runInfosFromClient(items []projectioncontract.Run) []port.RunInfo {
	ret := make([]port.RunInfo, 0, len(items))
	for _, item := range items {
		if mapped := remoteRunInfoFromClientRun(&item); mapped != nil {
			ret = append(ret, *mapped)
		}
	}
	return ret
}

func (b *remoteWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return b.cl.DeleteWorkspace(ctx, string(id), purge)
}

// ── Remote Goal ──

type remoteGoalBackend struct{ cl *client.Client }

func (b *remoteGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*port.GoalInfo, error) {
	g, err := b.cl.CreateGoal(ctx, string(wsID), text)
	if err != nil {
		return nil, err
	}
	return &port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.GoalInfo, error) {
	list, err := b.cl.ListGoals(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]port.GoalInfo, 0, len(list))
	for _, g := range list {
		result = append(result, port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text})
	}
	return result, nil
}
func (b *remoteGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*port.GoalInfo, error) {
	g, err := b.cl.GetGoal(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*port.GoalInfo, error) {
	g, err := b.cl.UpdateGoal(ctx, string(wsID), string(id), text)
	if err != nil {
		return nil, err
	}
	return &port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}

// ── Remote DoD ──

type remoteDoDBackend struct{ cl *client.Client }

func (b *remoteDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*port.DoDInfo, error) {
	d, err := b.cl.CreateDoD(ctx, string(wsID), items)
	if err != nil {
		return nil, err
	}
	return &port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.DoDInfo, error) {
	list, err := b.cl.ListDoDs(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]port.DoDInfo, 0, len(list))
	for _, d := range list {
		result = append(result, port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State})
	}
	return result, nil
}
func (b *remoteDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*port.DoDInfo, error) {
	d, err := b.cl.GetDoD(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*port.DoDInfo, error) {
	d, err := b.cl.UpdateDoD(ctx, string(wsID), string(id), items)
	if err != nil {
		return nil, err
	}
	return &port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}

// ── Remote Run ──

type remoteRunBackend struct{ cl *client.Client }

func (b *remoteRunBackend) Command(ctx context.Context, req port.WorkspaceRunCommand) (*port.RunInfo, error) {
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

func (b *remoteRunBackend) runCommandViaActivation(ctx context.Context, req port.WorkspaceRunCommand, intent port.RunCommandIntent) (*port.RunInfo, error) {
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
	if act == nil {
		return nil, nil
	}
	return remoteRunInfoFromClientRun(&act.Run), nil
}

func remoteRunInfoFromClientRun(r *projectioncontract.Run) *port.RunInfo {
	if r == nil {
		return nil
	}
	return &port.RunInfo{
		ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
		GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
		DefinitionRevision:  r.DefinitionRevision,
		PredecessorRunID:    common.RunID(r.Lineage.PredecessorRunID),
		PredecessorResultID: common.ResultID(r.Lineage.PredecessorResultID),
		SuccessorRunID:      common.RunID(r.Lineage.SuccessorRunID),
		State:               common.RunState(r.State),
		BlockingTicketID:    common.TicketID(r.BlockingTicketID),
		BlockingApprovalID:  common.ApprovalID(r.BlockingApprovalID),
		FailureReason:       r.FailureReason,
		SealedLLMTarget:     runtimecontract.CloneProviderModelTarget(r.SealedLLMTarget),
		RuntimeActivation:   port.CloneRuntimeActivationStatus(r.RuntimeActivation),
	}
}

func (b *remoteRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*port.RunInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	r, err := b.cl.GetRun(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return remoteRunInfoFromClientRun(r), nil
}
func (b *remoteRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.RunInfo, error) {
	list, err := b.cl.ListRuns(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]port.RunInfo, 0, len(list))
	for _, r := range list {
		if item := remoteRunInfoFromClientRun(&r); item != nil {
			result = append(result, *item)
		}
	}
	return result, nil
}
func (b *remoteRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*port.RunStatusInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	s, err := b.cl.GetRunStatus(ctx, string(wsID), string(id), "", 0)
	if err != nil {
		return nil, err
	}
	ret := port.RunStatusInfoFromProjection(*s)
	return &ret, nil
}

// ── Remote Decision ──

type remoteDecisionBackend struct{ cl *client.Client }

func (b *remoteDecisionBackend) Resolve(ctx context.Context, req port.ResolveDecisionRequest) (*port.DecisionInfo, error) {
	d, err := b.cl.ResolveDecision(ctx, string(req.WorkspaceID), string(req.TicketID), req.ResolutionType, req.Payload)
	if err != nil {
		return nil, err
	}
	return port.DecisionInfoPtrFromProjection(d), nil
}
func (b *remoteDecisionBackend) BatchResolve(ctx context.Context, workspaceID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload workorcheventpayload.Payload) ([]port.DecisionInfo, error) {
	ids := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		ids[i] = string(id)
	}
	list, err := b.cl.BatchResolveDecisions(ctx, string(workspaceID), ids, resolutionType, payload)
	if err != nil {
		return nil, err
	}
	return port.DecisionInfosFromProjection(list), nil
}
func (b *remoteDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.DecisionInfo, error) {
	list, err := b.cl.ListDecisions(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	return port.DecisionInfosFromProjection(list), nil
}
func (b *remoteDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*port.DecisionInfo, error) {
	d, err := b.cl.GetDecision(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return port.DecisionInfoPtrFromProjection(d), nil
}

func payloadFromRemoteMap(input map[string]any) workorcheventpayload.Payload {
	payload, _ := workorcheventpayload.FromMap(input)
	return payload
}

func payloadsFromRemoteMaps(input []map[string]any) []workorcheventpayload.Payload {
	payloads, _ := workorcheventpayload.FromMapSlice(input)
	return payloads
}

// ── Remote Approval ──

type remoteApprovalBackend struct{ cl *client.Client }

func (b *remoteApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*port.ApprovalInfo, error) {
	a, err := b.cl.Approve(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	return port.ApprovalInfoPtrFromProjection(a), nil
}
func (b *remoteApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*port.ApprovalInfo, error) {
	a, err := b.cl.Reject(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	return port.ApprovalInfoPtrFromProjection(a), nil
}
func (b *remoteApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]port.ApprovalInfo, error) {
	_idStrings := make([]string, len(ids))
	for i, id := range ids {
		_idStrings[i] = string(id)
	}
	list, err := b.cl.BatchResolveApprovals(ctx, string(wsID), _idStrings, action, payload...)
	if err != nil {
		return nil, err
	}
	return port.ApprovalInfosFromProjection(list), nil
}
func (b *remoteApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.ApprovalInfo, error) {
	list, err := b.cl.ListApprovals(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	return port.ApprovalInfosFromProjection(list), nil
}
func (b *remoteApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*port.ApprovalInfo, error) {
	a, err := b.cl.GetApproval(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return port.ApprovalInfoPtrFromProjection(a), nil
}

// ── Stub remote backends ──

type remoteSoulBackend struct{ cl *client.Client }

func (b *remoteSoulBackend) Create(ctx context.Context, s *port.SoulInfo) (*port.SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Get(ctx context.Context, id common.SoulID) (*port.SoulInfo, error) {
	return &port.SoulInfo{ID: id}, nil
}
func (b *remoteSoulBackend) List(ctx context.Context) ([]port.SoulInfo, error) { return nil, nil }
func (b *remoteSoulBackend) Update(ctx context.Context, id common.SoulID, s *port.SoulInfo) (*port.SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Delete(ctx context.Context, id common.SoulID) error { return nil }

type remoteSkillBackend struct{ cl *client.Client }

func (b *remoteSkillBackend) Create(ctx context.Context, s *port.SkillInfo) (*port.SkillInfo, error) {
	return s, nil
}
func (b *remoteSkillBackend) Get(ctx context.Context, id common.SkillID) (*port.SkillInfo, error) {
	return &port.SkillInfo{ID: id}, nil
}
func (b *remoteSkillBackend) List(ctx context.Context) ([]port.SkillInfo, error) { return nil, nil }

type remoteMCPBackend struct{ cl *client.Client }

func (b *remoteMCPBackend) CreateServer(ctx context.Context, s *port.MCPServerInfo) (*port.MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*port.MCPServerInfo, error) {
	return &port.MCPServerInfo{ID: id}, nil
}
func (b *remoteMCPBackend) ListServers(ctx context.Context) ([]port.MCPServerInfo, error) {
	return nil, nil
}
func (b *remoteMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *port.MCPServerInfo) (*port.MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) ListCapabilities(ctx context.Context) ([]port.MCPCapabilityInfo, error) {
	return nil, nil
}

type remoteMemoryBackend struct{ cl *client.Client }

func (b *remoteMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *port.MemoryItemInfo) (*port.MemoryItemInfo, error) {
	item, err := b.cl.CreateMemory(ctx, string(wsID), port.MemoryMutationFromInfo(m))
	if err != nil {
		return nil, err
	}
	return port.MemoryItemInfoPtrFromManagement(item), nil
}
func (b *remoteMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*port.MemoryItemInfo, error) {
	item, err := b.cl.GetMemory(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return port.MemoryItemInfoPtrFromManagement(item), nil
}
func (b *remoteMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *port.MemoryItemInfo) (*port.MemoryItemInfo, error) {
	item, err := b.cl.UpdateMemory(ctx, string(wsID), string(id), port.MemoryMutationFromInfo(m))
	if err != nil {
		return nil, err
	}
	return port.MemoryItemInfoPtrFromManagement(item), nil
}
func (b *remoteMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return b.cl.DeleteMemory(ctx, string(wsID), string(id))
}
func (b *remoteMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.MemoryItemInfo, error) {
	return b.Search(ctx, wsID, port.MemorySearchRequest{})
}
func (b *remoteMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req port.MemorySearchRequest) ([]port.MemoryItemInfo, error) {
	items, err := b.cl.SearchMemory(ctx, workorchmanagementcontract.MemorySearchQuery{
		WorkspaceID: string(wsID),
		Query:       req.Query,
		Tags:        append([]string(nil), req.Tags...),
		Types:       append([]string(nil), req.Types...),
		Limit:       req.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret := make([]port.MemoryItemInfo, 0, len(items))
	for _, item := range items {
		info := port.MemoryItemInfoPtrFromManagement(&item)
		ret = append(ret, *info)
	}
	return ret, nil
}

type remoteTemplateBackend struct{ cl *client.Client }

func (b *remoteTemplateBackend) Create(ctx context.Context, t *port.TemplateInfo) (*port.TemplateInfo, error) {
	item, err := b.cl.CreateWorkspaceTemplate(ctx, port.ManagementTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return port.TemplateInfoPtrFromManagement(item), nil
}
func (b *remoteTemplateBackend) Get(ctx context.Context, id string) (*port.TemplateInfo, error) {
	item, err := b.cl.GetWorkspaceTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	return port.TemplateInfoPtrFromManagement(item), nil
}
func (b *remoteTemplateBackend) List(ctx context.Context) ([]port.TemplateInfo, error) {
	items, err := b.cl.ListWorkspaceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]port.TemplateInfo, 0, len(items))
	for i := range items {
		if info := port.TemplateInfoPtrFromManagement(&items[i]); info != nil {
			ret = append(ret, *info)
		}
	}
	return ret, nil
}
func (b *remoteTemplateBackend) Update(ctx context.Context, id string, t *port.TemplateInfo) (*port.TemplateInfo, error) {
	item, err := b.cl.UpdateWorkspaceTemplate(ctx, id, port.ManagementTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return port.TemplateInfoPtrFromManagement(item), nil
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
	publishStatus := func(status port.SubscriptionStatusInfo) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionConnecting})
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
		publishStatus(port.SubscriptionStatusInfo{
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
			publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionConnecting, StreamRevision: liveRevision})
		}
	}
}

func (b *remoteProjectionEventsBackend) streamProjectionEventsOnce(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) (bool, int64) {
	publishStatus := func(status port.SubscriptionStatusInfo) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.cl.BaseURL()+"/api/v1/workorch/events/stream", nil)
	if err != nil {
		publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionDegraded, LastError: err.Error()})
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
		publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionDegraded, LastError: err.Error()})
		return false, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionDegraded, LastError: fmt.Sprintf("projection sse status %d", resp.StatusCode)})
		return false, 0
	}
	// Connected: only now report live.
	publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionLive})
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
				onStatus(ctx, port.SubscriptionStatusInfo{
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
func DecodeProjectionSSE(lines []string, eventID string) (port.WorkspaceProjectionChangedInfo, bool) {
	return decodeProjectionSSE(lines, eventID)
}

func decodeProjectionSSE(lines []string, eventID string) (port.WorkspaceProjectionChangedInfo, bool) {
	if len(lines) == 0 {
		return port.WorkspaceProjectionChangedInfo{}, false
	}
	var dto workspaceProjectionChangedSSE
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &dto); err != nil {
		return port.WorkspaceProjectionChangedInfo{}, false
	}
	// Accept both top-level concrete DTO and legacy envelope topic form.
	if strings.TrimSpace(dto.WorkspaceID) == "" {
		return port.WorkspaceProjectionChangedInfo{}, false
	}
	sections := make([]port.WorkspaceProjectionSection, 0, len(dto.ChangedSections))
	for _, section := range dto.ChangedSections {
		if strings.TrimSpace(section) == "" {
			continue
		}
		sections = append(sections, port.WorkspaceProjectionSection(section))
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
	return port.WorkspaceProjectionChangedInfo{
		ID:          id,
		WorkspaceID: common.WorkspaceID(dto.WorkspaceID),
		RunID:       common.RunID(dto.RunID),
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
