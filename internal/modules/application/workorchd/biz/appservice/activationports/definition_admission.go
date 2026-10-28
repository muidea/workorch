package activationports

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	definitionadmission "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// WorkspaceDefinitionAdmissionSnapshot is the exact owner snapshot admitted
// for one bootstrap compilation. The Template instance must be reused by every
// downstream compiler in that transaction; rereading a live Template would
// allow its profile policy to drift away from the Workspace-owner startup seal.
type WorkspaceDefinitionAdmissionSnapshot struct {
	Workspace *workspacemodel.Workspace
	Template  *workspacemodel.WorkspaceTemplate
	Candidate workspacemodel.DefinitionAdmissionCandidate
}

// EnsureWorkspaceDefinitionAdmission 为已确认创作核对 exact owner 准入并复用原候选；
// 其他定义路径提交 immutable intent，由 Workspace 独占 persistence、promotion 和 eligibility。
func EnsureWorkspaceDefinitionAdmission(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (WorkspaceDefinitionAdmissionSnapshot, error) {
	if hub == nil || workspace == nil || goal == nil || dod == nil || workspace.ID == "" || goal.WorkspaceID != workspace.ID || dod.WorkspaceID != workspace.ID {
		return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrInvalidArgument
	}
	definitionRevision := workspacecontracts.ExecutionDefinitionRevision(string(goal.ID), goal.DefinitionRevision, string(dod.ID), dod.DefinitionRevision)
	if definitionRevision == "" {
		return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: current Goal/DoD revision is incomplete", common.ErrConflict)
	}
	template, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, strings.TrimSpace(workspace.TemplateID))
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: Workspace Template snapshot is required for Definition admission: %v", common.ErrConflict, err)
	}
	// Fresh activation compiles current configuration, not historical policy.
	workspace, err = workspacemodel.WorkspaceForCurrentTemplate(workspace, template)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	for _, candidate := range workspace.DefinitionAdmissionCandidates {
		if candidate.AuthoringRef == nil {
			continue
		}
		command := workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: workspace.ID}
		ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		ownerevent.BindLane(ev, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, "workspace", string(workspace.ID), workspace.ID, "", "")
		view, err := ownerevent.WorkspaceOwner[workspaceevents.QueryDefinitionAuthoringResult](hub.Send(ev), workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
		if err != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, err
		}
		if err := view.ValidateFor(command); err != nil || view.WorkspaceRevision != workspace.Revision || view.Record == nil || view.Confirmation == nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: authoring activation observation changed or lacks confirmation", common.ErrConflict)
		}
		record, err := view.Record.Model()
		if err != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, err
		}
		history, err := workspaceevents.AuthoringHistoryModels(view.History)
		if err != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, err
		}
		workspace.DefinitionAuthoring = &record
		workspace.DefinitionAuthoringConfirmation = view.Confirmation
		workspace.DefinitionAuthoringHistory = history
		break
	}
	if workspace.DefinitionAuthoring != nil || workspace.DefinitionAuthoringConfirmation != nil {
		candidate, present, err := workspace.ValidateActivationAdmissionView()
		journal := workspace.DefinitionAuthoringConfirmation
		if err != nil || !present || journal == nil || workspace.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible ||
			journal.TemplateSnapshotHash != workspacemodel.WorkspaceTemplateSnapshotHash(template) || workspace.DeliverableDirectory != candidate.ResolvedProjectWorkspaceDirectory ||
			goal.ID != journal.Goal.ID || goal.DefinitionRevision != journal.Goal.DefinitionRevision || goal.Text != journal.Goal.Text ||
			dod.ID != journal.DoD.ID || dod.DefinitionRevision != journal.DoD.DefinitionRevision || !slices.Equal(dod.Items, journal.DoD.Items) ||
			definitionRevision != workspace.StartupInput.DefinitionRevision {
			return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: authoring activation requires exact confirmed admission and unchanged input", common.ErrConflict)
		}
		bindings, err := definitionconfiguration.Compile(workspace, template)
		if err != nil || bindings.ProfileContractsHash != candidate.RuntimeProfileContractsHash || !reflect.DeepEqual(bindings.PolicyBindings, candidate.AdmissionPolicyBindings) {
			return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: authoring activation policy changed after confirmation", common.ErrConflict)
		}
		if err := validateCurrentWorkspaceDefinitionAdmission(workspace); err != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, invalidCurrentWorkspaceStartupInput(err)
		}
		return WorkspaceDefinitionAdmissionSnapshot{Workspace: workspace, Template: template, Candidate: candidate}, nil
	}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	return SubmitWorkspaceDefinitionAdmission(ctx, hub, workspace, template, candidate)
}

// SubmitWorkspaceDefinitionAdmission 保留调用方已完成审查的确切候选，复用 owner 准入及 clarification 事务。
// 不重建候选、不调用 provider，也不创建 Run。
func SubmitWorkspaceDefinitionAdmission(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate, candidate workspacemodel.DefinitionAdmissionCandidate) (WorkspaceDefinitionAdmissionSnapshot, error) {
	if ctx == nil || hub == nil || workspace == nil || template == nil || workspace.ID == "" ||
		candidate.WorkspaceID != workspace.ID || template.ID != workspace.TemplateID ||
		candidate.WorkspaceTemplateSnapshot.TemplateRef != template.ID ||
		candidate.WorkspaceTemplateSnapshot.SnapshotHash != workspacemodel.WorkspaceTemplateSnapshotHash(template) {
		return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrInvalidArgument
	}
	if err := candidate.Validate(); err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	candidate = candidate.Clone()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate, ExpectedCandidateRevision: candidate.CandidateRevision - 1,
		CurrentTemplate: true, ExpectedWorkspaceRevision: workspace.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(workspace)}
	var opened *decisionevents.OpenWorkspaceDefinitionClarificationRequestResult
	for attempt := 0; attempt < 4; attempt++ {
		result, submitErr := workspaceports.SubmitDefinitionAdmissionIntent(ctx, hub, intent)
		if submitErr != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, submitErr
		}
		intent.ClarificationTicket = nil
		intent.ClarificationResolution = nil
		switch result.Disposition {
		case workspaceevents.DefinitionAdmissionIntentClarificationRequired:
			ticketID, coordinateErr := coordinateInvalidWorkspaceDefinitionClarification(ctx, hub, result)
			if coordinateErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, coordinateErr
			}
			return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: %s; blocking ticket %s", common.ErrConflict, result.AdmissionResult.ViolationCode, ticketID)
		case workspaceevents.DefinitionAdmissionIntentClarificationTicketRequired:
			if result.PriorClarification == nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
			}
			command, commandErr := result.PriorClarification.DecisionRequestIdentity()
			if commandErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
			}
			value, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(command))
			if openErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, openErr
			}
			fact, factErr := workspaceClarificationTicketFact(*result.PriorClarification, value)
			if factErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, factErr
			}
			opened = &value
			intent.ClarificationTicket = &fact
		case workspaceevents.DefinitionAdmissionIntentClarificationResolutionRequired:
			if result.PriorClarification == nil || result.StartupInput == nil || result.ActivationAdmission == nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
			}
			if opened == nil {
				command, commandErr := result.PriorClarification.DecisionRequestIdentity()
				if commandErr != nil {
					return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
				}
				value, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(command))
				if openErr != nil {
					return WorkspaceDefinitionAdmissionSnapshot{}, openErr
				}
				opened = &value
			}
			if opened.TicketID != result.ActivationAdmission.BlockingTicketID {
				return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
			}
			resolveCommand := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
				TicketID: opened.TicketID, WorkspaceID: workspace.ID,
				OldCandidateID: result.PriorClarification.Candidate.CandidateID, OldCandidateRevision: result.PriorClarification.Candidate.CandidateRevision, OldCandidateHash: result.PriorClarification.Candidate.CandidateHash,
				NewCandidateID: result.Candidate.CandidateID, NewCandidateRevision: result.Candidate.CandidateRevision, NewCandidateHash: result.Candidate.CandidateHash,
				AdmissionResultHash: result.AdmissionResult.AdmissionResultHash, DefinitionRevision: result.StartupInput.DefinitionRevision, StartupInputHash: result.StartupInput.StartupInputHash,
			}
			resolved, resolveErr := governanceports.ResolveWorkspaceDefinitionClarificationByAdmission(ctx, hub, resolveCommand)
			if resolveErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, resolveErr
			}
			fact := workspaceevents.WorkspaceDefinitionClarificationResolutionFact{
				ResolutionIdentity: definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity(resolveCommand),
				ResolutionBinding:  resolved.BindingValue(),
			}
			if err := fact.ValidateFor(*result.PriorClarification, result.Candidate, result.AdmissionResult, *result.StartupInput, *result.ActivationAdmission); err != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
			}
			intent.ClarificationResolution = &fact
		case workspaceevents.DefinitionAdmissionIntentEligible:
			updated, readErr := workspaceports.ReadWorkspaceRecord(ctx, hub, workspace.ID)
			if readErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, readErr
			}
			if err := validateDefinitionAdmissionOwnerSnapshot(updated, result); err != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, err
			}
			return WorkspaceDefinitionAdmissionSnapshot{Workspace: updated, Template: template, Candidate: result.Candidate.Clone()}, nil
		default:
			return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
		}
	}
	return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: Workspace Definition admission did not converge", common.ErrConflict)
}

func coordinateInvalidWorkspaceDefinitionClarification(ctx context.Context, hub magicevent.Hub, result workspaceevents.DefinitionAdmissionIntentResult) (common.TicketID, error) {
	binding := workspaceevents.DefinitionAdmissionClarificationBinding{Candidate: result.Candidate, AdmissionResult: result.AdmissionResult}
	command, err := binding.DecisionRequestIdentity()
	if err != nil {
		return "", common.ErrConflict
	}
	if result.PriorClarification == nil {
		opened, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(command))
		return opened.TicketID, openErr
	}
	oldCommand, err := result.PriorClarification.DecisionRequestIdentity()
	if err != nil {
		return "", common.ErrConflict
	}
	if _, err := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(oldCommand)); err != nil {
		return "", err
	}
	reconciled, err := governanceports.ReconcileWorkspaceDefinitionClarificationCandidate(ctx, hub, decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{
		OldRequest: decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(oldCommand),
		NewRequest: decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(command),
	})
	if err != nil {
		return "", err
	}
	return reconciled.NewTicketID, nil
}

func workspaceClarificationTicketFact(binding workspaceevents.DefinitionAdmissionClarificationBinding, opened decisionevents.OpenWorkspaceDefinitionClarificationRequestResult) (workspaceevents.WorkspaceDefinitionClarificationTicketFact, error) {
	command, err := binding.DecisionRequestIdentity()
	if err != nil {
		return workspaceevents.WorkspaceDefinitionClarificationTicketFact{}, common.ErrConflict
	}
	fact := workspaceevents.WorkspaceDefinitionClarificationTicketFact{
		RequestIdentity: command,
		TicketBinding:   opened.BindingValue(),
	}
	if err := fact.ValidateFor(binding); err != nil {
		return workspaceevents.WorkspaceDefinitionClarificationTicketFact{}, common.ErrConflict
	}
	return fact, nil
}

func validateDefinitionAdmissionOwnerSnapshot(workspace *workspacemodel.Workspace, result workspaceevents.DefinitionAdmissionIntentResult) error {
	if workspace == nil || result.StartupInput == nil || result.ActivationAdmission == nil {
		return common.ErrConflict
	}
	if err := validateCurrentWorkspaceDefinitionAdmission(workspace); err != nil {
		return invalidCurrentWorkspaceStartupInput(err)
	}
	candidate, present, err := workspace.ValidateActivationAdmissionView()
	if err != nil || !present || candidate.CandidateHash != result.Candidate.CandidateHash || workspace.StartupInput.StartupInputHash != result.StartupInput.StartupInputHash || workspace.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible {
		return common.ErrConflict
	}
	return nil
}

func definitionAdmissionCandidate(workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, template *workspacemodel.WorkspaceTemplate) (workspacemodel.DefinitionAdmissionCandidate, error) {
	var latest *workspacemodel.DefinitionAdmissionCandidate
	for _, existing := range workspace.DefinitionAdmissionCandidates {
		if existing.CandidateID == "workspace-definition" && (latest == nil || existing.CandidateRevision > latest.CandidateRevision) {
			value := existing.Clone()
			latest = &value
		}
	}
	if latest != nil {
		exact, err := definitionAdmissionCandidateForIdentity(workspace, goal, dod, template, latest.CandidateID, latest.CandidateRevision, latest.BaseDefinitionRevision, latest.BaseStartupInputHash)
		if err != nil {
			return workspacemodel.DefinitionAdmissionCandidate{}, err
		}
		if latest.Specification != nil {
			// A reviewed specification is bound to its complete authoring input.
			// Reuse only the exact input; a configuration or definition change
			// requires another review, never a silent return to legacy admission.
			withoutSpecification := latest.Clone()
			withoutSpecification.Specification = nil
			withoutSpecification.CompilationRef = nil
			withoutSpecification = withoutSpecification.WithHash()
			if exact.CandidateHash != withoutSpecification.CandidateHash {
				return workspacemodel.DefinitionAdmissionCandidate{}, fmt.Errorf("%w: reviewed specification requires recompilation for changed definition or template", common.ErrConflict)
			}
			return latest.Clone(), nil
		}
		if exact.CandidateHash == latest.CandidateHash {
			return latest.Clone(), nil
		}
	}
	revision := uint64(1)
	if latest != nil {
		revision = latest.CandidateRevision + 1
	}
	baseRevision, baseHash := "", ""
	if workspace.StartupInput != nil {
		baseRevision, baseHash = workspace.StartupInput.DefinitionRevision, workspace.StartupInput.StartupInputHash
	}
	return definitionAdmissionCandidateForIdentity(workspace, goal, dod, template, "workspace-definition", revision, baseRevision, baseHash)
}

func definitionAdmissionCandidateForIdentity(workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, template *workspacemodel.WorkspaceTemplate, candidateID string, revision uint64, baseRevision, baseHash string) (workspacemodel.DefinitionAdmissionCandidate, error) {
	if workspace == nil || goal == nil || dod == nil || template == nil || strings.TrimSpace(candidateID) == "" || revision == 0 {
		return workspacemodel.DefinitionAdmissionCandidate{}, common.ErrInvalidArgument
	}
	templateHash := workspaceTemplateSnapshotHash(template)
	dodInputs := make([]workspacemodel.DefinitionCandidateItem, 0, len(dod.Items))
	dodRefs := make([]string, 0, len(dod.Items))
	for idx, item := range dod.Items {
		ref := fmt.Sprintf("dod:%s:item:%06d", dod.ID, idx+1)
		dodRefs = append(dodRefs, ref)
		dodInputs = append(dodInputs, workspacemodel.DefinitionCandidateItem{CandidateItemRef: ref, Text: item, SourceIdentity: "dod:" + string(dod.ID) + "@" + dod.DefinitionRevision})
	}
	compiled, err := definitionconfiguration.CompileForItems(workspace, template, dodInputs)
	if err != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, err
	}
	policyBindings, profileContracts := compiled.PolicyBindings, compiled.ProfileContracts
	requiredOwnerFacts, ownerFactConvergences, profileContractsHash := compiled.RequiredOwnerFacts, compiled.OwnerFactConvergences, compiled.ProfileContractsHash
	candidate := workspacemodel.DefinitionAdmissionCandidate{SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: workspace.ID, CandidateID: strings.TrimSpace(candidateID), CandidateRevision: revision, BaseDefinitionRevision: baseRevision, BaseStartupInputHash: baseHash,
		GoalInput: workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal:" + string(goal.ID), Text: goal.Text, SourceIdentity: "goal:" + string(goal.ID) + "@" + goal.DefinitionRevision}, DoDInputs: dodInputs,
		WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{TemplateRef: template.ID, Revision: "template:" + templateHash, SnapshotHash: templateHash}, AdmissionCompilerVersion: workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: workspace.DeliverableDirectory,
		AdmissionPolicyBindings: policyBindings, RuntimeProfileContracts: profileContracts, RequiredOwnerFactBindings: requiredOwnerFacts, OwnerFactConvergenceBindings: ownerFactConvergences,
		RuntimeProfileContractsHash: profileContractsHash,
		ProposedRelationInputs:      []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: dodRefs, TargetItemRefs: []string{"goal:" + string(goal.ID)}}}}
	return candidate.WithHash(), nil
}

func definitionAdmissionProfileBindings(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) ([]workspacemodel.DefinitionAdmissionPolicyBinding, []runtimecontract.RuntimeProfileContract, []workspacemodel.DefinitionRequiredOwnerFactBinding, []workspacemodel.DefinitionOwnerFactConvergenceBinding, string, error) {
	bindings, err := definitionconfiguration.Compile(workspace, template)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	return bindings.PolicyBindings, bindings.ProfileContracts, bindings.RequiredOwnerFacts, bindings.OwnerFactConvergences, bindings.ProfileContractsHash, nil
}

func AuthoringAdmissionCandidate(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) (workspacemodel.DefinitionAdmissionCandidate, error) {
	if workspace == nil || template == nil || workspace.DefinitionAuthoring == nil || workspace.DefinitionAuthoringConfirmation == nil || workspace.ID == "" || template.ID != workspace.TemplateID || workspace.StartupInput != nil || workspace.ActivationAdmission != nil || len(workspace.DefinitionAdmissionCandidates) != len(workspace.DefinitionAuthoringHistory) || workspace.ValidateDefinitionAuthoringHistoryView() != nil || workspace.DefinitionSpecificationLedger != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, common.ErrConflict
	}
	if len(workspace.DefinitionAuthoringHistory) > 0 && workspace.DefinitionAuthoringConfirmation.ValidateSuccessor(workspace.DefinitionAuthoringHistory[len(workspace.DefinitionAuthoringHistory)-1].Confirmation) != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, common.ErrConflict
	}
	journal := *workspace.DefinitionAuthoringConfirmation
	if journal.ConfigurationHash != workspacemodel.WorkspaceConfigurationHash(workspace) || journal.TemplateSnapshotHash != workspacemodel.WorkspaceTemplateSnapshotHash(template) {
		return workspacemodel.DefinitionAdmissionCandidate{}, common.ErrConflict
	}
	goal, dod, spec, ref, err := workspacemodel.ConfirmedAuthoringAdmissionSourceView(workspace.DefinitionAuthoring, journal, workspace.ID)
	if err != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, err
	}
	policyBindings, profileContracts, requiredFacts, convergences, profileHash, err := definitionAdmissionProfileBindings(workspace, template)
	if err != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, err
	}
	refs := make([]string, len(dod))
	for index := range dod {
		refs[index] = dod[index].CandidateItemRef
	}
	sort.Strings(refs)
	lineageID := "authoring-" + workspacemodel.StableDefinitionAuthoringConfirmationID(workspace.ID, journal.InputHash, journal.DerivationPermitID, journal.SpecificationHash)
	if len(workspace.DefinitionAuthoringHistory) > 0 {
		lineageID = workspace.DefinitionAdmissionCandidates[0].CandidateID
	}
	candidate := workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent,
		WorkspaceID:   workspace.ID, CandidateID: lineageID, CandidateRevision: uint64(len(workspace.DefinitionAuthoringHistory) + 1),
		GoalInput: goal, DoDInputs: dod,
		WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{TemplateRef: template.ID, Revision: "template:" + journal.TemplateSnapshotHash, SnapshotHash: journal.TemplateSnapshotHash},
		AdmissionPolicyBindings:   policyBindings, RuntimeProfileContracts: profileContracts, RequiredOwnerFactBindings: requiredFacts, OwnerFactConvergenceBindings: convergences, RuntimeProfileContractsHash: profileHash,
		AdmissionCompilerVersion: workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: workspace.DeliverableDirectory,
		ProposedRelationInputs: []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: refs, TargetItemRefs: []string{goal.CandidateItemRef}}},
		Specification:          &spec, AuthoringRef: &ref,
	}
	candidate = candidate.WithHash()
	if err := candidate.Validate(); err != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, err
	}
	return candidate, nil
}

func validateCurrentWorkspaceDefinitionAdmission(workspace *workspacemodel.Workspace) error {
	candidate, present, err := workspace.ValidateActivationAdmissionView()
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	result, err := definitionadmission.Compile(candidate)
	if err != nil {
		return err
	}
	if result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		return fmt.Errorf("activation admission candidate is not admitted_all_of")
	}
	return nil
}

func invalidCurrentWorkspaceStartupInput(cause error) error {
	return fmt.Errorf("%w: Workspace admission does not match the current input; start a new execution with the current Template: %v", common.ErrConflict, cause)
}

func workspaceTemplateSnapshotHash(template *workspacemodel.WorkspaceTemplate) string {
	return workspacemodel.WorkspaceTemplateSnapshotHash(template)
}
