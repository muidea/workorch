package git

import (
	"context"
	"fmt"
	"strings"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindRead = modulecommon.StepKindRead
const StepKindWrite = modulecommon.StepKindWrite

// gitWorkspaceRepositorySelector is the stable owner selector used by
// semantic-plan proposals. Git operations act on the repository owned by the
// workspace; they must not leave target scope empty or force the LLM to invent
// a path-like target for repository state.
const gitWorkspaceRepositorySelector = "workspace-repository"

type Biz struct {
	basebiz.Base
	lifecycle          basebiz.Lifecycle
	runtimeAdapter     *RuntimeAdapter
	capabilityReceipts *CapabilityStateReceiptStore
	started            bool
	subscribedTopics   []string
}

type Dependencies struct {
	EventHub                magicevent.Hub
	BackgroundRoutine       task.BackgroundRoutine
	RuntimeAdapter          *RuntimeAdapter
	CapabilityStateReceipts *CapabilityStateReceiptStore
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine, deps ...Dependencies) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	m := &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine)}
	if len(deps) > 0 {
		m.runtimeAdapter = deps[0].RuntimeAdapter
		m.capabilityReceipts = deps[0].CapabilityStateReceipts
	}
	if m.runtimeAdapter != nil && m.capabilityReceipts != nil {
		return m, nil
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return nil, err
	}
	if m.runtimeAdapter == nil {
		adapter, err := NewRuntimeAdapterFromTimeoutString(cfg.Runtime.Git.DefaultTimeout)
		if err != nil {
			return nil, err
		}
		m.runtimeAdapter = adapter
	}
	if m.capabilityReceipts == nil {
		receipts, err := NewCapabilityStateReceiptStore(cfg.Repository.DataDir)
		if err != nil {
			return nil, err
		}
		m.capabilityReceipts = receipts
	}
	return m, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	leaveStart, err := m.lifecycle.Enter()
	if err != nil {
		return err
	}
	defer leaveStart()
	if m.started {
		return nil
	}
	subscriptions := []struct {
		topic   string
		handler magicevent.ObserverFunc
	}{
		{gitevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities},
		{gitevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility},
		{gitevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility},
		{gitevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility},
		{gitevents.TopicAcquireCapabilityState, m.handleGitCapabilityState},
		{gitevents.TopicQueryRuntimeCWIReconciliation, m.handleRuntimeCWIReconciliationQuery},
		{gitevents.TopicExecuteOperation, m.handleGitOperation},
		{gitevents.TopicQueryRuntimeCommitSnapshot, m.handleRuntimeCommitSnapshotQuery},
	}
	// A failed Start keeps every completed subscription reachable. Retrying
	// resumes at the first unfinished topic rather than registering duplicates.
	for _, subscription := range subscriptions[len(m.subscribedTopics):] {
		if err := m.SubscribeFuncChecked(subscription.topic, m.lifecycleHandler(subscription.handler, subscription.topic == gitevents.TopicExecuteOperation)); err != nil {
			return err
		}
		m.subscribedTopics = append(m.subscribedTopics, subscription.topic)
	}
	m.started = true
	return nil
}

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 最终释放仍由应用共享 Hub/任务屏障后的调用方授权；本地等待只是防御。
	if err := m.BeginShutdown(ctx); err != nil {
		return err
	}
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	// Lifecycle calls are serialized by their caller. Do not hold an owner
	// lock across Hub control actions. Unsubscribe completion is a resource
	// receipt, not proof of handler quiescence; the caller must drain first.
	for len(m.subscribedTopics) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.subscribedTopics) - 1
		if err := m.UnsubscribeFuncChecked(m.subscribedTopics[last]); err != nil {
			return err
		}
		m.subscribedTopics = m.subscribedTopics[:last]
		m.started = false
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.started = false
	m.runtimeAdapter = nil
	m.capabilityReceipts = nil
	return ctx.Err()
}

// handleRuntimeCWIReconciliationQuery reports only the current Git owner
// state.  Repository existence/revision is a retention fact for an already
// accepted current Run; it is never a new coverage proof for workspace_full
// restart and it never writes a capability receipt or Runtime aggregate.
func (m *Biz) handleRuntimeCWIReconciliationQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.QueryRuntimeCWIReconciliationCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if !runtimeCWIReconciliationBindingHasGitProof(command.Binding) {
		observation := gitReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, "binding has no Git operation proof", "git_proof_unavailable")
		result.Set(gitevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
		return
	}
	snapshot, err := m.runtimeAdapter.CapabilityState(event.Context(), "reconciliation/"+command.RequestID+"/"+command.Binding.BindingID, "workspace_deliverable", command.WorkspaceDeliverableDirectory)
	if err != nil {
		observation := gitReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, "Git repository state is unavailable", "repository_state_unavailable")
		result.Set(gitevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
		return
	}
	status := runtimecontract.RuntimeCWIReconciliationObservationSatisfied
	message, code := "", ""
	// CapabilityState encodes an absent repository as an empty RepoRoot in its
	// revision. Treat that as an unsatisfied existing fact, not as proof that a
	// future restart may claim a Git CWI.
	if !gitRevisionHasRepository(snapshot.ObservedRevision) {
		status = runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied
		message, code = "Git repository is absent from the current workspace", "repository_absent"
	}
	if status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied &&
		(len(command.Binding.RequiredCheckRefs) != 0 || len(command.Binding.MetricConstraints) != 0 || len(command.Binding.ArtifactRefs) != 0) {
		// Repository presence/revision is a Git fact, not proof for a
		// binding-specific validation, metric, or artifact requirement.
		status = runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
		message, code = "Git owner cannot independently verify binding-specific proof", "git_proof_incomplete"
	}
	observation := gitReconciliationObservation(command, status, snapshot.ObservedRevision, command.Binding.TargetRefs, message, code)
	result.Set(gitevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
}

func runtimeCWIReconciliationBindingHasGitProof(binding runtimecontract.RuntimeCWIReconciliationBindingRef) bool {
	for _, kind := range binding.AllowedOperationKinds {
		if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.StepKindGitRead || runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.StepKindGitWrite {
			return true
		}
	}
	return false
}

func gitRevisionHasRepository(revision string) bool {
	revision = strings.TrimSpace(revision)
	return strings.HasPrefix(revision, gitRepositoryRevisionPrefix)
}

func gitReconciliationObservation(command gitevents.QueryRuntimeCWIReconciliationCommand, status runtimecontract.RuntimeCWIReconciliationObservationStatus, revision string, targetRefs []string, message, code string) runtimecontract.RuntimeCWIReconciliationObservation {
	if strings.TrimSpace(revision) == "" {
		revision = command.WorkspaceBaselineRevision
	}
	value := runtimecontract.RuntimeCWIReconciliationObservation{
		RequestID: command.RequestID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		Generation: command.Generation, CoverageWorkItemID: command.Binding.CoverageWorkItemID,
		BindingID: command.Binding.BindingID, BindingHash: command.Binding.BindingHash,
		OwnerModuleID: ModuleID, ObservedRevision: revision, Status: status,
		TargetRefs: append([]string(nil), targetRefs...),
	}
	if status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		value.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ModuleID,
			OriginalClass: "reconciliation", OriginalCode: code, OriginalMessage: message,
			Signature: "git-reconciliation/" + code, TargetRefs: append([]string(nil), targetRefs...), ObservedRevision: revision,
		}
	}
	return value.Normalized()
}

func (m *Biz) handleGitCapabilityState(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.AcquireCapabilityStateCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || m.capabilityReceipts == nil || command.WorkspaceID == "" || command.PrerequisiteID == "" || command.AcquisitionKey == "" || command.WorkspaceDeliverableDirectory == "" || command.CapabilityStateKind != "repository_state" || command.SubjectRef == "" {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	workspaceTreeRevision, err := m.acquireFileWorkspaceTreeRevision(
		event.Context(), command.WorkspaceID, command.RunID, command.AuthoritySnapshotID,
		"git-workspace-tree/"+command.AcquisitionKey, command.WorkspaceDeliverableDirectory,
	)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if snapshot, found, err := m.capabilityReceipts.Load(command.WorkspaceID, command.AcquisitionKey); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		if snapshot.WorkspaceTreeRevision != workspaceTreeRevision {
			setCapabilityError(result, fmt.Errorf("%w: git capability acquisition %q has a different File workspace-tree revision", common.ErrConflict, command.AcquisitionKey))
			return
		}
		observation := gitCapabilityStateObservation(command.PrerequisiteID, snapshot)
		result.Set(gitevents.AcquireCapabilityStateResult{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Snapshot: snapshot, Observation: observation}, nil)
		return
	}
	snapshot, err := m.runtimeAdapter.CapabilityState(event.Context(), command.PrerequisiteID, command.SubjectRef, command.WorkspaceDeliverableDirectory)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	snapshot.AcquisitionKey = command.AcquisitionKey
	// The receipt key is accepted by Git only once it names a durable owner
	// snapshot.  Downstream components compare this opaque binding but never
	// derive repository semantics from it.
	snapshot.AcquisitionBinding = command.AcquisitionKey
	snapshot.WorkspaceTreeRevision = workspaceTreeRevision
	snapshot.QueryReceiptRef = "capability-state-receipt/" + command.AcquisitionKey
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	snapshot, err = m.capabilityReceipts.StoreIfAbsent(command.WorkspaceID, snapshot)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := gitCapabilityStateObservation(command.PrerequisiteID, snapshot)
	result.Set(gitevents.AcquireCapabilityStateResult{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Snapshot: snapshot, Observation: observation}, nil)
}

// acquireFileWorkspaceTreeRevision keeps Git's repository-state observation
// bound to the File owner's exact workspace-tree revision. Git does not hash
// or infer that File-owned value itself; the nested typed EventHub query is a
// read-only owner observation and never changes Runtime authority.
func (m *Biz) acquireFileWorkspaceTreeRevision(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID, prerequisiteID, deliverable string) (string, error) {
	authoritySnapshotID = strings.TrimSpace(authoritySnapshotID)
	prerequisiteID = strings.TrimSpace(prerequisiteID)
	deliverable = strings.TrimSpace(deliverable)
	if m == nil || m.EventHub() == nil || workspaceID == "" || runID == "" || authoritySnapshotID == "" || prerequisiteID == "" || deliverable == "" {
		return "", common.ErrInvalidArgument
	}
	result := m.EventHub().Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireWorkspaceManifest,
		ModuleID,
		fileevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(),
		ctx,
		fileevents.AcquireWorkspaceManifestCommand{
			WorkspaceID: workspaceID, RunID: runID, AuthoritySnapshotID: authoritySnapshotID,
			PrerequisiteID: prerequisiteID, WorkspaceDeliverableDirectory: deliverable,
		},
	))
	if result == nil {
		return "", common.ErrInvalidArgument
	}
	if eventErr := result.Error(); eventErr != nil {
		if eventErr.Cause != nil {
			return "", eventErr.Cause
		}
		return "", eventErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		if resultErr.Cause != nil {
			return "", resultErr.Cause
		}
		return "", resultErr
	}
	response, ok := data.(fileevents.AcquireWorkspaceManifestResult)
	if !ok || response.WorkspaceID != workspaceID || response.RunID != runID ||
		strings.TrimSpace(response.AuthoritySnapshotID) != authoritySnapshotID ||
		strings.TrimSpace(response.Observation.PrerequisiteID) != prerequisiteID {
		return "", fmt.Errorf("%w: File owner returned an invalid workspace-tree observation", common.ErrConflict)
	}
	binding := runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: response.Manifest, Observation: response.Observation}
	if err := binding.Validate(); err != nil {
		return "", fmt.Errorf("%w: File owner returned an invalid workspace-tree binding: %v", common.ErrConflict, err)
	}
	return strings.TrimSpace(binding.Manifest.Revision), nil
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.AcquireRuntimeOperationAdmissibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := gitevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	// A capability-bound Git inventory is derived from the exact durable owner
	// receipt requested by Orchestrator.  It must not perform a second,
	// independently timed CapabilityState acquisition here.
	var snapshot runtimecontract.RuntimeCapabilityStateSnapshot
	var err error
	binding := strings.TrimSpace(command.CapabilityStateBinding)
	if binding != "" {
		if m.capabilityReceipts == nil {
			setCapabilityError(result, common.ErrInvalidArgument)
			return
		}
		var found bool
		snapshot, found, err = m.capabilityReceipts.Load(command.WorkspaceID, binding)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		if !found || snapshot.AcquisitionBinding != binding {
			setCapabilityError(result, fmt.Errorf("%w: git capability acquisition binding %q is unavailable", common.ErrConflict, binding))
			return
		}
	} else {
		// Owners/profiles without a declared capability-state prerequisite retain
		// the existing read-only inventory behavior.
		snapshot, err = m.runtimeAdapter.CapabilityState(event.Context(), command.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
	}
	workspaceTreeRevision := strings.TrimSpace(snapshot.WorkspaceTreeRevision)
	manifestBinding := command.WorkspaceManifestBinding.Normalized()
	if !manifestBinding.Empty() {
		if err := manifestBinding.Validate(); err != nil {
			setCapabilityError(result, err)
			return
		}
		manifestRevision := strings.TrimSpace(manifestBinding.Manifest.Revision)
		if workspaceTreeRevision != "" && workspaceTreeRevision != manifestRevision {
			setCapabilityError(result, fmt.Errorf("%w: Git capability and File manifest workspace-tree revisions differ", common.ErrConflict))
			return
		}
		workspaceTreeRevision = manifestRevision
	}
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibilityWithBinding(
		command.PrerequisiteID,
		ModuleID,
		command.RequiredKinds,
		nil,
		[]string{gitWorkspaceRepositorySelector},
		[]string{"repository-state-snapshot:" + snapshot.SnapshotID, "repository-state-snapshot", "owner-execution-policy"},
		snapshot.ObservedRevision,
		binding,
	)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	// Git owns the executable sub-operation vocabulary.  Keep the concrete
	// variant facts on this owner-authored inventory so semantic planning and
	// candidate admission never infer an init/add/commit (or read) operation
	// from the broad git kind or selector alone.
	inventory.OperationVariants = gitAdmissibilityOperationVariants(command.RequiredKinds)
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := gitAdmissibilityObservation(command.PrerequisiteID, inventory, workspaceTreeRevision)
	result.Set(gitevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Admissibility: inventory, Observation: observation}}, nil)
}

// gitAdmissibilityOperationVariants is the Git owner's explicit sub-operation
// target-policy surface. Git acts on the owner-controlled repository scope;
// it never receives prospective workspace-target authority from this catalog.
// ExactTarget remains available for bounded pathspecs, while
// ExistingOwnerScope covers repository-wide operations such as status and
// commit. The selector/constraint values are opaque owner facts, not a global
// registry or a Runtime inference rule.
func gitAdmissibilityOperationVariants(operationKinds []string) []runtimecontract.RuntimeOperationAdmissibilityVariant {
	selectorRefs := []string{gitWorkspaceRepositorySelector}
	constraintRefs := []string{"repository-state-snapshot", "owner-execution-policy"}
	ret := make([]runtimecontract.RuntimeOperationAdmissibilityVariant, 0)
	for _, rawKind := range operationKinds {
		kind := runtimecontract.CanonicalRuntimeBlockKind(rawKind)
		for _, supported := range SupportedRuntimeOperationVariants() {
			if runtimecontract.CanonicalRuntimeBlockKind(supported.Kind) != kind || strings.TrimSpace(supported.VariantID) == "" {
				continue
			}
			ret = append(ret, runtimecontract.RuntimeOperationAdmissibilityVariant{
				Kind: kind, VariantID: strings.TrimSpace(supported.VariantID),
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyExactTarget,
					runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
				},
				SelectorRefs: append([]string(nil), selectorRefs...), ConstraintRefs: append([]string(nil), constraintRefs...),
			})
		}
	}
	return ret
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.ProbeRuntimeOperationFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := gitevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	failure, stale := m.runtimeWorkspaceTreeFeasibility(event.Context(), command)
	if failure == nil && stale == nil {
		failure = m.runtimeOperationFeasibility(event.Context(), command)
	}
	if stale == nil && failure != nil && failure.Code == "operation_admissibility_stale" && command.Observation != nil {
		if observed, err := m.runtimeAdapter.CapabilityState(event.Context(), command.Admissibility.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory); err == nil {
			stale = gitAdmissibilityStale(*command.Observation, command.Admissibility, observed.ObservedRevision)
			if stale != nil {
				failure = nil
			}
		}
	}
	category := ownerFeasibilityCategory(failure)
	observation, err := gitevents.NewProbeRuntimeOperationFeasibilityResult(command, category, failure, stale, nil, nil)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation, nil)
}

// runtimeWorkspaceTreeFeasibility re-observes the File-owned workspace tree
// for Git admission. The Git status revision is intentionally separate: a
// repository that was already dirty can keep the same
// porcelain rows while the contents behind an already-dirty path have changed.
func (m *Biz) runtimeWorkspaceTreeFeasibility(ctx context.Context, command gitevents.ProbeRuntimeOperationFeasibilityCommand) (*runtimecontract.FailureFact, *runtimecontract.RuntimePrerequisiteStale) {
	binding := strings.TrimSpace(command.CapabilityStateBinding)
	if command.Observation == nil {
		if binding != "" {
			failure := runtimecontract.RuntimeOperationInfeasibleFailure(command.Operation, "operation_admissibility_stale", "runtime Git operation admissibility omitted File workspace-tree freshness")
			return failure, nil
		}
		return nil, nil
	}
	observation := command.Observation.Normalized()
	expected := ""
	for _, dependency := range observation.Dependencies {
		if dependency.Domain != runtimecontract.RuntimeResourceWorkspaceTree || dependency.OwnerModuleID != runtimecontract.RuntimeModuleFile || dependency.SubjectRef != "workspace_deliverable" {
			continue
		}
		if expected != "" && expected != strings.TrimSpace(dependency.Revision) {
			failure := runtimecontract.RuntimeOperationInfeasibleFailure(command.Operation, "operation_admissibility_stale", "runtime Git operation admissibility has conflicting File workspace-tree dependencies")
			return failure, nil
		}
		expected = strings.TrimSpace(dependency.Revision)
	}
	if expected == "" {
		if binding != "" {
			failure := runtimecontract.RuntimeOperationInfeasibleFailure(command.Operation, "operation_admissibility_stale", "runtime Git operation admissibility omitted File workspace-tree freshness")
			return failure, nil
		}
		return nil, nil
	}
	if binding != "" {
		if binding != strings.TrimSpace(command.Admissibility.AcquisitionBinding) || m.capabilityReceipts == nil {
			return nil, nil
		}
		snapshot, found, err := m.capabilityReceipts.Load(command.WorkspaceID, binding)
		if err != nil || !found || snapshot.AcquisitionBinding != binding {
			return nil, nil
		}
		if strings.TrimSpace(snapshot.WorkspaceTreeRevision) == "" || strings.TrimSpace(snapshot.WorkspaceTreeRevision) != expected {
			failure := runtimecontract.RuntimeOperationInfeasibleFailure(command.Operation, "operation_admissibility_stale", "runtime Git capability binding and File workspace-tree dependency differ")
			return failure, nil
		}
	}
	observed, err := m.acquireFileWorkspaceTreeRevision(
		ctx, command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID,
		"git-feasibility-workspace-tree/"+strings.TrimSpace(command.Operation.ID), command.WorkspaceDeliverableDirectory,
	)
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(command.Operation, "git_workspace_tree_unavailable", err.Error())
		return failure, nil
	}
	if observed == expected {
		return nil, nil
	}
	return nil, &runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: expected, ObservedRevision: observed,
		Reason: "File workspace tree changed after Git operation admissibility was sealed",
	}
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.ProbeRuntimeSemanticProofFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := gitevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	snapshot, err := m.runtimeAdapter.CapabilityState(event.Context(), command.OwnerCatalog.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory)
	if err != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, command.OwnerCatalog.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "git_repository_state_unavailable", err.Error(), command.OwnerCatalog.ObservedRevision, "", nil))
		return
	}
	if snapshot.ObservedRevision != command.OwnerCatalog.ObservedRevision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, snapshot.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime Git semantic catalog is stale", command.OwnerCatalog.ObservedRevision, snapshot.ObservedRevision, nil))
		return
	}
	if strings.TrimSpace(command.Proposal.ValidationRecipeID) != "" || len(command.Proposal.RequiredCheckRefs) != 0 {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, snapshot.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "git_semantic_proof_unavailable", "Git repository state is not an independent semantic validation proof", "", snapshot.ObservedRevision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
		return
	}
	setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityFeasible, snapshot.ObservedRevision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal), nil)
}

func setRuntimeSemanticProofFeasibilityResult(result magicevent.Result, command gitevents.ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, revision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) {
	response, err := gitevents.NewRuntimeSemanticProofFeasibilityResult(command, status, revision, targetRefs, diagnostic)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func gitCapabilityStateObservation(prerequisiteID string, snapshot runtimecontract.RuntimeCapabilityStateSnapshot) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: ModuleID, ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{
			{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: snapshot.SubjectRef, Revision: snapshot.ObservedRevision},
			// CapabilityState reads git status or the root directory to derive the
			// required sequence. A File effect can therefore invalidate this
			// observation before Git executes its next sealed operation.
			{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: snapshot.WorkspaceTreeRevision},
		},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func gitAdmissibilityObservation(prerequisiteID string, inventory runtimecontract.RuntimeOperationAdmissibility, workspaceTreeRevision string) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{
			{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision},
			{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: strings.TrimSpace(workspaceTreeRevision)},
		},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func gitAdmissibilityStale(observation runtimecontract.RuntimePrerequisiteObservation, inventory runtimecontract.RuntimeOperationAdmissibility, observedRevision string) *runtimecontract.RuntimePrerequisiteStale {
	observation = observation.Normalized()
	for _, dependency := range observation.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceRepositoryState && dependency.OwnerModuleID == ModuleID && dependency.SubjectRef == "workspace_deliverable" && dependency.Revision == inventory.ObservedRevision {
			return &runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime git operation admissibility snapshot is stale"}
		}
	}
	return nil
}

// runtimeOperationFeasibility is a read-only candidate-admission check.  It
// never initializes a repository, runs a write operation, or commits an
// owner receipt.  The actual effect still occurs only after sealed dispatch.
func (m *Biz) runtimeOperationFeasibility(ctx context.Context, command gitevents.ProbeRuntimeOperationFeasibilityCommand) *runtimecontract.FailureFact {
	operation := command.Operation.Normalized()
	inventory := command.Admissibility.Normalized()
	if binding := strings.TrimSpace(command.CapabilityStateBinding); binding != "" {
		if binding != inventory.AcquisitionBinding {
			return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime git operation admissibility acquisition binding is stale")
		}
		if m.capabilityReceipts == nil {
			return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_binding_unavailable", "runtime git capability acquisition binding is unavailable")
		}
		snapshot, found, err := m.capabilityReceipts.Load(command.WorkspaceID, binding)
		if err != nil || !found || snapshot.AcquisitionBinding != binding || snapshot.ObservedRevision != inventory.ObservedRevision {
			return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime git operation admissibility acquisition binding is stale")
		}
	}
	snapshot, err := m.runtimeAdapter.CapabilityState(ctx, inventory.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory)
	if err != nil {
		return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "git_repository_unavailable", err.Error())
	}
	if snapshot.ObservedRevision != inventory.ObservedRevision {
		return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime git operation admissibility snapshot is stale")
	}
	// A repository snapshot can require a specific write sequence (for example
	// init -> add -> commit).  The generic candidate validator verifies the
	// complete dependency graph; the owner gate ensures this concrete operation
	// is at least one currently declared Git variant before it can be sealed.
	if operation.Kind == StepKindWrite && len(snapshot.RequiredOperationSequence) > 0 {
		contract, found := runtimecontract.RuntimeOperationContractForKind(operation.Kind)
		if !found {
			return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "git_operation_contract_missing", "runtime git operation has no contract")
		}
		variant, matchErr := runtimecontract.MatchRuntimeOperationVariant(operation, contract)
		if matchErr != nil {
			return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "git_operation_variant_invalid", matchErr.Error())
		}
		for _, required := range snapshot.RequiredOperationSequence {
			if required.Kind == operation.Kind && required.VariantID == variant.ID {
				return nil
			}
		}
		return runtimecontract.RuntimeOperationInfeasibleFailure(operation, "git_operation_variant_not_admissible", "runtime git operation is absent from the current repository-state sequence")
	}
	return nil
}

func (m *Biz) handleGitOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.ExecuteOperationCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	operation, err := validateSealedRuntimeGitOperation(event, command.RuntimeOperationExecutionContext, command.SealedOperation)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	data, err := gitevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := validateGitOperationTargetBinding(operation, data); err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		result.Set(gitevents.ExecuteOperationResult{Receipt: receipt}, nil)
		return
	}
	// 未命中的回执查询不能借 drain 准入产生新 effect 或新 receipt。
	leaveFresh, admissionErr := m.lifecycle.Enter()
	if admissionErr != nil {
		setCapabilityError(result, admissionErr)
		return
	}
	defer leaveFresh()

	if stale, staleErr := m.runtimePrerequisiteStaleReceipt(event.Context(), command, operation.RequiredOperationRef); staleErr != nil {
		setCapabilityError(result, staleErr)
		return
	} else if stale != nil {
		committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, *stale)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		result.Set(gitevents.ExecuteOperationResult{Receipt: committed}, nil)
		return
	}
	if data.Operation == modulecommon.OperationCommit {
		if err := validateGitCommitDependencyScope(command.RuntimeOperationExecutionContext); err != nil {
			setCapabilityError(result, err)
			return
		}
		if command.WorkspaceInternalDirectory != "" {
			if err := gitCommitSnapshotScopeForExecution(command.RuntimeOperationExecutionContext).Validate(); err != nil {
				setCapabilityError(result, err)
				return
			}
		}
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: operation.Kind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory,
		Operation:                  data.Operation, TimeoutMillis: data.TimeoutMillis, Pathspecs: data.Pathspecs,
		Ref: data.Ref, BaseRef: data.BaseRef, Limit: data.Limit,
		Message: data.Message, ExpectedHead: data.ExpectedHead, EvidencePolicies: command.EvidencePolicies,
	})
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	if executeErr == nil {
		ownerEvidence, dependencyErr := m.runtimeAdapter.commitOwnerEvidence(event.Context(), command.RuntimeOperationExecutionContext, operationResult)
		if dependencyErr != nil {
			// effect 可能已经完成；不伪造 rejected/no-op receipt，交由原 unknown 对账路径处理。
			setCapabilityError(result, dependencyErr)
			return
		}
		evidence = ownerEvidence
	}
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, operation.Kind, operationResult.ErrorCode, operationResult.ErrorMessage, command.TargetRefs)
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{
		ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		BeforeHash:          operationResult.BeforeHash,
		AfterHash:           operationResult.AfterHash,
	}
	if operation.Kind == runtimecontract.StepKindGitWrite {
		observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		if status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
			observation.ObservedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
			observation.Changed = !operationResult.ZeroWork
			observation.NoOp = operationResult.ZeroWork
			if observation.NoOp {
				observation.ObservedEffectScope = runtimecontract.RuntimeEffectNone
			} else if after, afterErr := m.runtimeAdapter.CapabilityState(event.Context(), "runtime-receipt/"+command.OperationID, "workspace_deliverable", command.WorkspaceDeliverableDirectory); afterErr == nil {
				observation.ResourceChanges = gitReceiptResourceChanges(command.CapabilityStates, after)
			}
		}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, command.TargetRefs, evidence, failure, nil, observation)
	if receipt.Feedback != nil && receipt.Feedback.OriginalDiagnostic != nil && operationResult.OutputBytes+operationResult.ErrorOutputBytes > 0 {
		receipt.Feedback.OriginalDiagnostic.DetailsBytes = operationResult.OutputBytes + operationResult.ErrorOutputBytes
		receipt.Feedback.OriginalDiagnostic.ContentType = "text/plain"
		receipt.Feedback.OriginalDiagnostic.OmittedReason = "raw_git_command_output"
		receipt.Feedback.OriginalDiagnostic.Truncated = operationResult.Truncated
		receipt.Feedback.OriginalDiagnostic.Redacted = true
	}
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(gitevents.ExecuteOperationResult{Receipt: committed}, nil)
}

func validateSealedRuntimeGitOperation(event magicevent.Event, context runtimecontract.RuntimeOperationExecutionContext, sealed gitevents.SealedRuntimeOperation) (runtimecontract.RuntimeOperation, error) {
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("runtime Git operation requires Orchestrator source")
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(context.WorkspaceID, context.RunID, context.DispatchID, context.Transaction, context.OperationID, context.EvidenceKind, context.WorkspaceDeliverableDirectory); err != nil {
		return runtimecontract.RuntimeOperation{}, err
	}
	return sealed.ValidateForExecution(context)
}

func validateGitOperationTargetBinding(operation runtimecontract.RuntimeOperation, data gitevents.OperationData) error {
	if operation.Kind != runtimecontract.StepKindGitWrite || strings.TrimSpace(data.Operation) != modulecommon.OperationAdd {
		return nil
	}
	pathspecs, err := requiredPathspecs(data.Pathspecs)
	if err != nil {
		return err
	}
	authorized := map[string]struct{}{}
	for _, target := range operation.TargetRefs {
		target = strings.TrimSpace(target)
		if target == gitWorkspaceRepositorySelector {
			return nil
		}
		normalized, normalizeErr := normalizePathspec(target, false)
		if normalizeErr != nil {
			return fmt.Errorf("sealed Git add target %q is invalid: %w", target, normalizeErr)
		}
		authorized[normalized] = struct{}{}
	}
	for _, pathspec := range pathspecs {
		if _, found := authorized[pathspec]; !found {
			return fmt.Errorf("%w: sealed Git add pathspec %q is outside operation target refs", common.ErrConflict, pathspec)
		}
	}
	return nil
}

// gitReceiptResourceChanges reports only revisions directly observed by the
// Git owner after a durable Git operation. Orchestrator performs the neutral
// intersection; it never derives repository state from operation kind.
func gitReceiptResourceChanges(before []runtimecontract.RuntimeCapabilityStateSnapshot, after runtimecontract.RuntimeCapabilityStateSnapshot) []runtimecontract.RuntimeResourceChange {
	if after.OwnerModuleID != ModuleID || after.CapabilityStateKind != "repository_state" || after.SubjectRef == "" || after.ObservedRevision == "" {
		return nil
	}
	beforeRevision := ""
	for _, snapshot := range before {
		if snapshot.OwnerModuleID == ModuleID && snapshot.CapabilityStateKind == "repository_state" && snapshot.SubjectRef == after.SubjectRef {
			beforeRevision = snapshot.ObservedRevision
			break
		}
	}
	if beforeRevision == after.ObservedRevision {
		return nil
	}
	return []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: after.SubjectRef,
		BeforeRevision: beforeRevision, AfterRevision: after.ObservedRevision,
	}}
}

func (m *Biz) runtimePrerequisiteStaleReceipt(ctx context.Context, command gitevents.ExecuteOperationCommand, requiredOperationRef string) (*runtimecontract.RuntimeOperationReceipt, error) {
	expectedWorkspaceTree := ""
	var workspaceTreeObservation runtimecontract.RuntimePrerequisiteObservation
	if command.WorkspaceTreeObservation != nil {
		observation := command.WorkspaceTreeObservation.Normalized()
		if err := observation.Validate(); err != nil || observation.Kind != runtimecontract.PrerequisiteKindManifest || observation.OwnerModuleID != runtimecontract.RuntimeModuleFile {
			return nil, fmt.Errorf("%w: sealed Git execution has an invalid File workspace-tree observation", common.ErrConflict)
		}
		for _, dependency := range observation.Dependencies {
			if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree && dependency.OwnerModuleID == runtimecontract.RuntimeModuleFile && dependency.SubjectRef == "workspace_deliverable" {
				if expectedWorkspaceTree != "" && expectedWorkspaceTree != dependency.Revision {
					return nil, fmt.Errorf("%w: sealed Git execution has conflicting File workspace-tree revisions", common.ErrConflict)
				}
				expectedWorkspaceTree = dependency.Revision
			}
		}
		if expectedWorkspaceTree == "" || observation.ContentHash != expectedWorkspaceTree {
			return nil, fmt.Errorf("%w: sealed Git execution File observation does not bind its workspace-tree revision", common.ErrConflict)
		}
		workspaceTreeObservation = observation
	}
	for _, snapshot := range command.CapabilityStates {
		if snapshot.OwnerModuleID != ModuleID || snapshot.CapabilityStateKind != "repository_state" {
			continue
		}
		snapshotRevision := strings.TrimSpace(snapshot.WorkspaceTreeRevision)
		if snapshotRevision == "" {
			return nil, fmt.Errorf("%w: sealed Git capability state omitted File workspace-tree revision", common.ErrConflict)
		}
		if expectedWorkspaceTree != "" && expectedWorkspaceTree != snapshotRevision {
			return nil, fmt.Errorf("%w: sealed Git capability state and File workspace-tree observation differ", common.ErrConflict)
		}
		expectedWorkspaceTree = snapshotRevision
		if workspaceTreeObservation.ObservationID == "" {
			workspaceTreeObservation = gitCapabilityStateObservation(snapshot.RequirementID, snapshot)
		}
	}
	if expectedWorkspaceTree != "" {
		observed, err := m.acquireFileWorkspaceTreeRevision(
			ctx, command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID,
			"git-execution-workspace-tree/"+command.OperationID, command.WorkspaceDeliverableDirectory,
		)
		if err != nil {
			return nil, err
		}
		if observed != expectedWorkspaceTree {
			message := "File workspace tree changed after the Git operation was sealed"
			failure := &runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassExecutionFailure, Code: "prerequisite_stale",
				Signature: "workspace-tree-stale:" + workspaceTreeObservation.ObservationID, Message: message,
				TargetRefs: append([]string(nil), command.TargetRefs...),
			}
			receipt := m.newRuntimeOperationReceipt(
				command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected,
				command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: message}, failure, nil,
				orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone},
			)
			receipt.PrerequisiteStale = &runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: workspaceTreeObservation.PrerequisiteID, ObservationID: workspaceTreeObservation.ObservationID,
				Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				SubjectRef: "workspace_deliverable", ExpectedRevision: expectedWorkspaceTree,
				ObservedRevision: observed, Reason: message,
			}
			return &receipt, nil
		}
	}
	if strings.TrimSpace(requiredOperationRef) == "" {
		return nil, nil
	}
	for _, snapshot := range command.CapabilityStates {
		if snapshot.OwnerModuleID != ModuleID || snapshot.CapabilityStateKind != "repository_state" || snapshot.SnapshotID == "" || snapshot.ContentHash == "" {
			continue
		}
		var required *runtimecontract.RuntimeRequiredOperationVariant
		for idx := range snapshot.RequiredOperationSequence {
			if snapshot.RequiredOperationSequence[idx].RequirementID == strings.TrimSpace(requiredOperationRef) {
				value := snapshot.RequiredOperationSequence[idx]
				required = &value
				break
			}
		}
		if required == nil {
			continue
		}
		// A predecessor only suppresses re-observation when the command carries
		// the exact completed receipt binding for every declared predecessor.
		// Dependency presence by itself is not authority: a missing, mismatched,
		// rejected, or no-op predecessor must never hide an external state drift.
		if len(required.DependsOnRequirementIDs) != 0 {
			if err := validateGitCapabilityStatePredecessorReceipts(snapshot, *required, command.CompletedRequiredOperationReceipts); err != nil {
				return nil, err
			}
			continue
		}
		observed, err := m.runtimeAdapter.CapabilityState(ctx, snapshot.RequirementID, snapshot.SubjectRef, command.WorkspaceDeliverableDirectory)
		if err != nil {
			return nil, err
		}
		if observed.ContentHash == snapshot.ContentHash {
			continue
		}
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "prerequisite_stale", Signature: "capability-state-stale:" + snapshot.SnapshotID, Message: "owner capability state changed after sealed snapshot", TargetRefs: append([]string(nil), command.TargetRefs...)}
		receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: failure.Message}, failure, nil, orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone})
		observation := gitCapabilityStateObservation(snapshot.RequirementID, snapshot)
		receipt.PrerequisiteStale = &runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID,
			Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: snapshot.SubjectRef,
			ExpectedRevision: snapshot.ObservedRevision, ObservedRevision: observed.ObservedRevision, Reason: failure.Message,
		}
		return &receipt, nil
	}
	return nil, nil
}

func validateGitCapabilityStatePredecessorReceipts(snapshot runtimecontract.RuntimeCapabilityStateSnapshot, required runtimecontract.RuntimeRequiredOperationVariant, bindings []runtimecontract.RuntimeRequiredOperationReceiptBinding) error {
	requirements := map[string]runtimecontract.RuntimeRequiredOperationVariant{}
	for _, item := range snapshot.RequiredOperationSequence {
		requirements[item.RequirementID] = item
	}
	byRequirement := map[string]runtimecontract.RuntimeRequiredOperationReceiptBinding{}
	for _, binding := range runtimecontract.NormalizeRuntimeRequiredOperationReceiptBindings(bindings) {
		byRequirement[binding.RequirementID] = binding
	}
	for _, dependencyID := range required.DependsOnRequirementIDs {
		dependency, known := requirements[dependencyID]
		binding, found := byRequirement[dependencyID]
		if !known || !found || binding.Validate() != nil || binding.Kind != dependency.Kind || binding.VariantID != dependency.VariantID ||
			binding.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || binding.ObservedScope != runtimecontract.RuntimeEffectWorkspaceDurable || !binding.Changed {
			return fmt.Errorf("capability-state predecessor receipt %q is not an accepted durable binding", dependencyID)
		}
	}
	return nil
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := gitRuntimeIngressCommand[gitevents.AcquireRuntimeCapabilitiesCommand](event, result)
	if !ok {
		return
	}
	response, err := gitevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

// gitRuntimeIngressCommand is the owner-local strong-call gate for every
// Runtime topic subscribed by Git. It deliberately rejects Post before
// inspecting the DTO, and rejects every source except the exact Orchestrator
// module before any repository, receipt-store, or File-owner query can run.
func gitRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) {
	var zero T
	if result == nil {
		return zero, false
	}
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setGitRuntimeIngressInvalid(result, "runtime Git ingress requires exact Orchestrator source")
		return zero, false
	}
	command, ok := event.Data().(T)
	if !ok {
		setGitRuntimeIngressInvalid(result, "runtime Git ingress payload has an invalid concrete type")
		return zero, false
	}
	return command, true
}

func setGitRuntimeIngressInvalid(result magicevent.Result, message string) {
	if result == nil {
		return
	}
	err := fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
	result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: err.Error(), Cause: err})
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}

// BeginShutdown 关停新 root，不取消已接纳 handler；其 nested receipt/query
// 仍保留真实调用 context，只有实际返回时才释放在途记账。
func (m *Biz) BeginShutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.lifecycle.BeginShutdown()

	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (m *Biz) Quiesce(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if err := m.lifecycle.Quiesce(ctx); err != nil {
		return err
	}

	return nil
}

// Execute 的 drain 入口仅执行 sealed command 校验与 exact persisted receipt
// 查询；receipt 未命中必须另取 fresh 票，不能回退扩大停机期间的执行权限。
func (m *Biz) lifecycleHandler(handler magicevent.ObserverFunc, receiptDrain bool) magicevent.ObserverFunc {
	return func(ev magicevent.Event, result magicevent.Result) {
		if result == nil || ev == nil {
			return
		}
		if ev.Source() != runtimecontract.RuntimeModuleOrchestrator {
			setGitRuntimeIngressInvalid(result, "runtime command source must be kernel/orchestrator")
			return
		}
		var leave func()
		var err error
		if receiptDrain {
			leave, err = m.lifecycle.EnterDrain()
		} else {
			leave, err = m.lifecycle.Enter()
		}
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		defer leave()
		handler(ev, result)
	}
}
