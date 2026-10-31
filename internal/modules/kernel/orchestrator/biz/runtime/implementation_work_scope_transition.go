package runtime

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// implementationWorkScopeSources compiles only from sealed startup and current
// owner facts. It does not substitute the terminal target selection or invent
// an owner revision from directory contents.
func implementationWorkScopeSources(a RuntimeAggregate, cwi common.CoverageWorkItemID) (runtimecontract.RuntimeImplementationWorkScopeSources, error) {
	if err := rejectRuntimeProgressAfterTermination(a); err != nil {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, err
	}
	if a.ActiveDispatch != nil || a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || a.Facts.Failure != nil || a.Facts.RepairDirective != nil {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, fmt.Errorf("%w: ordinary work scope cannot bypass dispatch, semantic purpose or repair", common.ErrConflict)
	}
	if !slices.Contains(runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts), cwi) {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, fmt.Errorf("%w: work scope CWI is not eligible", common.ErrConflict)
	}
	if runtimecontract.RuntimeImplementationWorkPrerequisitesPending(a.Facts, nil) {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, fmt.Errorf("%w: work scope requires complete owner prerequisites", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeImplementationWorkInputCatalogs(a.Facts, a.Definition.CapabilityContract); err != nil {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, err
	}
	owner := ""
	for _, focus := range a.Facts.DefinitionFocuses {
		if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == cwi && focus.ImplementationWorkPolicy != nil {
			owner = focus.ImplementationWorkPolicy.OwnerModuleID
		}
	}
	if owner == "" || a.Facts.Resources.Manifest == nil {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, fmt.Errorf("%w: work scope requires declared owner and committed manifest", common.ErrConflict)
	}
	var binding runtimecontract.RuntimeWorkspaceManifestBinding
	matches := 0
	for _, observation := range a.Facts.Resources.PrerequisiteObservations {
		if observation.Kind == runtimecontract.PrerequisiteKindManifest && observation.OwnerModuleID == owner && observation.ContentHash == a.Facts.Resources.Manifest.Revision {
			matches++
			binding = runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: *a.Facts.Resources.Manifest, Observation: observation}.Clone()
		}
	}
	if matches != 1 || binding.Validate() != nil {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, fmt.Errorf("%w: work scope requires unique current owner manifest observation", common.ErrConflict)
	}
	revision, err := runtimecontract.PendingRuntimeValidationInputRevision(a.SemanticPlanning, a.ImplementationWork, cwi)
	if err != nil {
		return runtimecontract.RuntimeImplementationWorkScopeSources{}, err
	}
	return runtimecontract.RuntimeImplementationWorkScopeSources{
		InputRevision: revision,
		WorkspaceID:   a.Facts.WorkspaceID, RunID: a.Facts.RunID, AuthoritySnapshotID: a.Decision.AuthoritySnapshot.ID,
		Seed: a.Facts.DefinitionSeed, Focuses: a.Facts.DefinitionFocuses, Profiles: a.Facts.Activation.ExecutionContract.ProfileContracts,
		CoverageWorkItemID: cwi, ManifestBinding: binding,
	}, nil
}

// CurrentRuntimeImplementationWorkReservation projects the one outstanding
// scope transaction; historical completed rounds never become current action.
func CurrentRuntimeImplementationWorkReservation(a RuntimeAggregate) (*runtimecontract.RuntimeImplementationWorkRound, error) {
	var current *runtimecontract.RuntimeImplementationWorkRound
	for _, round := range a.ImplementationWork.Rounds {
		if round.CompletedReceiptSetHash != "" || round.FailedReceiptHash != "" {
			continue
		}
		if current != nil {
			return nil, fmt.Errorf("%w: multiple current implementation work reservations", common.ErrConflict)
		}
		copy := round.Clone()
		current = &copy
	}
	return current, nil
}

// RuntimeImplementationWorkPurposePending 只读取 persisted purpose/round 与 exact CWI 声明。
// 它不选择 effect；terminal choice、repair、失败和其它 CWI 均不会得到工作豁免。
func RuntimeImplementationWorkPurposePending(a RuntimeAggregate) bool {
	if a.Termination != nil || a.Facts.Failure != nil || a.Facts.RepairDirective != nil || len(a.Facts.Focus.CoverageWorkItemIDs) != 1 {
		return false
	}
	cwi := a.Facts.Focus.CoverageWorkItemIDs[0]
	if runtimecontract.RuntimeImplementationWorkTerminalPurposeSelected(a.ImplementationWork, a.SemanticPlanning, cwi) {
		return false
	}
	for _, round := range a.ImplementationWork.Rounds {
		if round.CoverageWorkItemID == cwi && round.FailedReceiptHash != "" {
			return false
		}
		if round.CoverageWorkItemID == cwi && round.CompletedReceiptSetHash == "" {
			return true
		}
	}
	if a.ImplementationWork.ScopePreview != nil {
		return a.ImplementationWork.ScopePreview.Request.CoverageWorkItemID == cwi
	}
	if a.ActiveDispatch != nil || a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || !slices.Contains(runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts), cwi) {
		return false
	}
	for _, focus := range a.Facts.DefinitionFocuses {
		if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == cwi && focus.ImplementationWorkPolicy != nil {
			return true
		}
	}
	return false
}

// RuntimeSemanticPrerequisiteRequiredForAggregate 在工作 purpose 期间保留完整 owner 事实前置，
// 仅延后终验 debt；terminal/repair/普通路径仍取得全部旧 prerequisites。
func RuntimeSemanticPrerequisiteRequiredForAggregate(a RuntimeAggregate, value runtimecontract.RuntimePrerequisite) bool {
	if !RuntimeImplementationWorkPurposePending(a) {
		return true
	}
	var targets []string
	for _, round := range a.ImplementationWork.Rounds {
		if round.CoverageWorkItemID == a.Facts.Focus.CoverageWorkItemIDs[0] && round.CompletedReceiptSetHash == "" && round.WorkingBinding != nil {
			for _, proposal := range round.WorkingBinding.Proposals {
				targets = append(targets, runtimecontract.RuntimeSemanticProposalTargetRefs(proposal)...)
			}
		}
	}
	return runtimecontract.RuntimeImplementationWorkPrerequisiteRequired(value, a.Facts.Focus.ID, targets)
}

func RuntimeSemanticPrerequisitesPendingForAggregate(a RuntimeAggregate) bool {
	if !RuntimeImplementationWorkPurposePending(a) {
		terminalWork := false
		for _, choice := range a.ImplementationWork.PurposeChoices {
			if slices.Contains(a.Facts.Focus.CoverageWorkItemIDs, choice.CoverageWorkItemID) && choice.Purpose == runtimecontract.RuntimeImplementationWorkPurposeTerminal {
				terminalWork = true
			}
		}
		if !terminalWork {
			return SemanticPlanningPrerequisitesPending(a.Facts.Prerequisites.Pending)
		}
	}
	for _, value := range a.Facts.Prerequisites.Pending {
		if RuntimeSemanticPrerequisiteRequiredForAggregate(a, value) {
			return true
		}
	}
	return false
}

// NextRuntimeImplementationWorkScopeRequest 只投影 SM-2 已封存输入下的只读
// scope 需求，不授权实现；下一轮 effect 仍要新的模型提议及 owner feasibility。
func NextRuntimeImplementationWorkScopeRequest(a RuntimeAggregate) (*runtimecontract.RuntimeImplementationWorkScopeRequest, error) {
	current, err := CurrentRuntimeImplementationWorkReservation(a)
	if err != nil || current != nil || a.ActiveDispatch != nil || a.Termination != nil || a.Facts.Failure != nil || a.Facts.RepairDirective != nil || a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired {
		return nil, err
	}
	if len(a.Facts.Focus.CoverageWorkItemIDs) != 1 {
		return nil, nil
	}
	cwi := a.Facts.Focus.CoverageWorkItemIDs[0]
	// A retained focus can temporarily lose planning eligibility when an exact
	// prerequisite is reopened. That is absence of a work-scope demand, not an
	// internal semantic failure. Explicit scope reservation still rechecks the
	// eligibility gate below and cannot bypass prerequisite settlement.
	if !slices.Contains(runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts), cwi) {
		return nil, nil
	}
	declared := false
	for _, focus := range a.Facts.DefinitionFocuses {
		if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == cwi && focus.ImplementationWorkPolicy != nil {
			declared = true
		}
	}
	if !declared {
		return nil, nil
	}
	if runtimecontract.RuntimeImplementationWorkTerminalPurposeSelected(a.ImplementationWork, a.SemanticPlanning, cwi) {
		return nil, nil
	}
	if a.ImplementationWork.ScopePreview != nil {
		return nil, nil
	}
	for _, round := range a.ImplementationWork.Rounds {
		if round.CoverageWorkItemID == cwi && round.FailedReceiptHash != "" {
			return nil, fmt.Errorf("%w: failed work round cannot gain successor scope", common.ErrConflict)
		}
	}
	// 缺 owner 前像时沿既有 prerequisite 取得事实，不用文件存在替代 revision。
	if a.Facts.Resources.Manifest == nil || runtimecontract.RuntimeImplementationWorkPrerequisitesPending(a.Facts, nil) {
		return nil, nil
	}
	sources, err := implementationWorkScopeSources(a, cwi)
	if err != nil {
		return nil, err
	}
	request, err := sources.Compile()
	if err != nil {
		return nil, err
	}
	return &request, nil
}

// RuntimeImplementationWorkScopeForCurrentSources is a read-only projection for
// the owner adapter. The reservation event independently repeats every check.
func RuntimeImplementationWorkScopeForCurrentSources(a RuntimeAggregate, cwi common.CoverageWorkItemID) (runtimecontract.RuntimeImplementationWorkScopeSources, error) {
	return implementationWorkScopeSources(a, cwi)
}

func (c *authorityComponent) reserveImplementationWorkScope(ctx context.Context, workspace common.WorkspaceID, run common.RunID, request runtimecontract.RuntimeImplementationWorkScopeRequest) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspace == "" || run == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	a, err := c.repository.Load(ctx, workspace, run)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Storage rollover must be valid before admitting work/physical effects.
	// Complete history is retained; segment count is not an execution budget.
	if err := a.SemanticPlanning.ValidateHistoryStorage(); err != nil {
		return RuntimeAggregate{}, err
	}

	// 已持久 exact replay 只读返回，不能因当前 phase/resource 变化重开轮次。
	for _, round := range a.ImplementationWork.Rounds {
		if round.ScopeRequest.RequestID == request.RequestID {
			if !reflect.DeepEqual(round.ScopeRequest, request) {
				return RuntimeAggregate{}, common.ErrConflict
			}
			return a, nil
		}
	}
	if preview := a.ImplementationWork.ScopePreview; preview != nil && preview.Request.RequestID == request.RequestID {
		if !reflect.DeepEqual(preview.Request, request) {
			return RuntimeAggregate{}, common.ErrConflict
		}
		return a, nil
	}
	sources, err := implementationWorkScopeSources(a, request.CoverageWorkItemID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := request.ValidateForSources(sources); err != nil {
		return RuntimeAggregate{}, err
	}
	ordinal, preceding := 1, ""
	for _, round := range a.ImplementationWork.Rounds {
		if round.CoverageWorkItemID == request.CoverageWorkItemID {
			if round.CompletedReceiptSetHash == "" {
				return RuntimeAggregate{}, fmt.Errorf("%w: same CWI work round is still outstanding", common.ErrConflict)
			}
			ordinal, preceding = round.Ordinal+1, round.CompletedReceiptSetHash
		}
	}
	if ordinal > request.Policy.MaxRoundsPerCWI+1 || len(a.ImplementationWork.Rounds) > len(a.Facts.DefinitionSeed.CoverageItems)*runtimecontract.MaxRuntimeImplementationWorkRounds || a.ImplementationWork.ScopePreview != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: implementation purpose preview budget exhausted or already reserved", common.ErrConflict)
	}
	_ = preceding // 前轮 receipt 由 semantic purpose manifest 从原账本独立投影。
	expected := a.StateRevision
	a.ImplementationWork = a.ImplementationWork.Clone()
	a.ImplementationWork.ScopePreview = &runtimecontract.RuntimeImplementationWorkScopePreview{Request: request.Clone()}
	a.StateRevision++
	if err := c.transitions.Commit(ctx, a, expected); err != nil {
		return RuntimeAggregate{}, err
	}
	return a, nil
}

func (c *authorityComponent) observeImplementationWorkScope(ctx context.Context, workspace common.WorkspaceID, run common.RunID, observation runtimecontract.RuntimeImplementationWorkScopeObservation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspace == "" || run == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	a, err := c.repository.Load(ctx, workspace, run)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	for _, round := range a.ImplementationWork.Rounds {
		if round.ScopeRequest.RequestID != observation.Request.RequestID {
			continue
		}
		if !reflect.DeepEqual(round.ScopeRequest, observation.Request) {
			return RuntimeAggregate{}, common.ErrConflict
		}
		if round.ScopeObservation != nil {
			if !reflect.DeepEqual(*round.ScopeObservation, observation) {
				return RuntimeAggregate{}, fmt.Errorf("%w: work scope observation cannot replace an accepted fact", common.ErrConflict)
			}
			return a, nil
		}
	}
	if preview := a.ImplementationWork.ScopePreview; preview != nil && preview.Request.RequestID == observation.Request.RequestID {
		if !reflect.DeepEqual(preview.Request, observation.Request) {
			return RuntimeAggregate{}, common.ErrConflict
		}
		if preview.Observation != nil {
			if !reflect.DeepEqual(*preview.Observation, observation) {
				return RuntimeAggregate{}, common.ErrConflict
			}
			return a, nil
		}
	}
	sources, err := implementationWorkScopeSources(a, observation.Request.CoverageWorkItemID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := observation.ValidateForSources(sources); err != nil {
		return RuntimeAggregate{}, err
	}
	a.ImplementationWork = a.ImplementationWork.Clone()
	if preview := a.ImplementationWork.ScopePreview; preview != nil && reflect.DeepEqual(preview.Request, observation.Request) {
		expected := a.StateRevision
		copy := observation.Clone()
		preview.Observation = &copy
		a.StateRevision++
		if err := c.transitions.Commit(ctx, a, expected); err != nil {
			return RuntimeAggregate{}, err
		}
		return a, nil
	}
	for index := range a.ImplementationWork.Rounds {
		round := &a.ImplementationWork.Rounds[index]
		if !reflect.DeepEqual(round.ScopeRequest, observation.Request) {
			continue
		}
		if round.WorkingBinding != nil || round.DispatchID != "" {
			return RuntimeAggregate{}, common.ErrConflict
		}
		expected := a.StateRevision
		copy := observation.Clone()
		round.ScopeObservation = &copy
		round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(*round)
		a.StateRevision++
		if err := c.transitions.Commit(ctx, a, expected); err != nil {
			return RuntimeAggregate{}, err
		}
		return a, nil
	}
	return RuntimeAggregate{}, fmt.Errorf("%w: work scope result has no persisted exact reservation", common.ErrConflict)
}
