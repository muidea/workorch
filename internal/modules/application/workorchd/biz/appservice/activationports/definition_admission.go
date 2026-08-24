package activationports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionadmissioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// EnsureWorkspaceDefinitionAdmission converges current owner snapshots into an
// immutable pre-seal candidate and an eligible all-of startup input. It runs
// before RuntimeBootstrap compilation and performs no provider/capability
// call. Invalid candidates stop after a dedicated Decision ticket is opened.
func EnsureWorkspaceDefinitionAdmission(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (*workspacemodel.Workspace, error) {
	if hub == nil || workspace == nil || goal == nil || dod == nil || workspace.ID == "" || goal.WorkspaceID != workspace.ID || dod.WorkspaceID != workspace.ID {
		return nil, common.ErrInvalidArgument
	}
	definitionRevision := workspacecontracts.ExecutionDefinitionRevision(string(goal.ID), goal.DefinitionRevision, string(dod.ID), dod.DefinitionRevision)
	if definitionRevision == "" {
		return nil, fmt.Errorf("%w: current Goal/DoD revision is incomplete", common.ErrConflict)
	}
	if err := validateCurrentWorkspaceDefinitionAdmission(workspace); err != nil {
		return nil, invalidCurrentWorkspaceStartupInput(err)
	}
	if workspaceDefinitionAdmissionEligible(workspace, goal, dod, definitionRevision) {
		return workspace, nil
	}
	if workspace.ActivationAdmission != nil && workspace.ActivationAdmission.State == workspacemodel.WorkspaceActivationPendingClarificationResolution {
		if err := completePendingWorkspaceDefinitionClarification(ctx, hub, workspace); err != nil {
			return nil, err
		}
		updated, err := workspaceports.ReadWorkspaceRecord(ctx, hub, workspace.ID)
		if err != nil {
			return nil, err
		}
		if err := validateCurrentWorkspaceDefinitionAdmission(updated); err != nil {
			return nil, invalidCurrentWorkspaceStartupInput(err)
		}
		if workspaceDefinitionAdmissionEligible(updated, goal, dod, definitionRevision) {
			return updated, nil
		}
		return nil, common.ErrConflict
	}
	template, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, strings.TrimSpace(workspace.TemplateID))
	if err != nil {
		return nil, fmt.Errorf("%w: Workspace Template snapshot is required for Definition admission: %v", common.ErrConflict, err)
	}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		return nil, err
	}
	result, err := workspaceports.SubmitDefinitionAdmissionCandidate(ctx, hub, candidate, candidate.CandidateRevision-1)
	if err != nil {
		return nil, err
	}
	admission, err := definitionadmissioncore.Compile(result.Candidate)
	if err != nil {
		return nil, err
	}
	if admission.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		command := workspaceClarificationCommand(result.Candidate, admission)
		previousCandidate, previousAdmission, hasPrevious := latestPriorInvalidDefinitionCandidate(workspace, result.Candidate)
		if hasPrevious {
			oldCommand := workspaceClarificationCommand(previousCandidate, previousAdmission)
			if _, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, oldCommand); openErr != nil {
				return nil, openErr
			}
			reconciled, reconcileErr := governanceports.ReconcileWorkspaceDefinitionClarificationCandidate(ctx, hub, decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: oldCommand, NewRequest: command})
			if reconcileErr != nil {
				return nil, reconcileErr
			}
			return nil, fmt.Errorf("%w: %s; blocking ticket %s", common.ErrConflict, admission.ViolationCode, reconciled.NewTicketID)
		}
		opened, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, command)
		if openErr != nil {
			return nil, openErr
		}
		return nil, fmt.Errorf("%w: %s; blocking ticket %s", common.ErrConflict, admission.ViolationCode, opened.TicketID)
	}
	previousCandidate, previousAdmission, clarified := latestPriorInvalidDefinitionCandidate(workspace, result.Candidate)
	var blockingTicketID common.TicketID
	if clarified {
		oldCommand := workspaceClarificationCommand(previousCandidate, previousAdmission)
		opened, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, oldCommand)
		if openErr != nil {
			return nil, openErr
		}
		blockingTicketID = opened.TicketID
	}
	promoted, err := workspaceports.PromoteDefinitionAdmissionCandidate(ctx, hub, result.Candidate, admission, blockingTicketID)
	if err != nil {
		return nil, err
	}
	if clarified {
		resolved, resolveErr := governanceports.ResolveWorkspaceDefinitionClarificationByAdmission(ctx, hub, decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
			TicketID: blockingTicketID, WorkspaceID: workspace.ID,
			OldCandidateID: previousCandidate.CandidateID, OldCandidateRevision: previousCandidate.CandidateRevision, OldCandidateHash: previousCandidate.CandidateHash,
			NewCandidateID: result.Candidate.CandidateID, NewCandidateRevision: result.Candidate.CandidateRevision, NewCandidateHash: result.Candidate.CandidateHash,
			AdmissionResultHash: admission.AdmissionResultHash, DefinitionRevision: promoted.DefinitionRevision, StartupInputHash: promoted.StartupInputHash,
		})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if _, ackErr := workspaceports.AcknowledgeWorkspaceDefinitionClarification(ctx, hub, workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand{
			WorkspaceID: workspace.ID, CandidateID: result.Candidate.CandidateID, CandidateRevision: result.Candidate.CandidateRevision, CandidateHash: result.Candidate.CandidateHash,
			BlockingTicketID: blockingTicketID, ResolutionID: resolved.ResolutionID, ResolutionBindingHash: resolved.ResolutionBindingHash,
			DefinitionRevision: promoted.DefinitionRevision, StartupInputHash: promoted.StartupInputHash,
		}); ackErr != nil {
			return nil, ackErr
		}
	}
	updated, err := workspaceports.ReadWorkspaceRecord(ctx, hub, workspace.ID)
	if err != nil {
		return nil, err
	}
	if err := validateCurrentWorkspaceDefinitionAdmission(updated); err != nil {
		return nil, invalidCurrentWorkspaceStartupInput(err)
	}
	if !workspaceDefinitionAdmissionEligible(updated, goal, dod, definitionRevision) {
		return nil, common.ErrConflict
	}
	return updated, nil
}

func definitionAdmissionCandidate(workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, template *workspacemodel.WorkspaceTemplate) (workspacemodel.DefinitionAdmissionCandidate, error) {
	revision := uint64(1)
	for _, existing := range workspace.DefinitionAdmissionCandidates {
		if existing.CandidateID == "workspace-definition" && existing.CandidateRevision >= revision {
			revision = existing.CandidateRevision + 1
		}
	}
	baseRevision, baseHash := "", ""
	if workspace.StartupInput != nil {
		baseRevision, baseHash = workspace.StartupInput.DefinitionRevision, workspace.StartupInput.StartupInputHash
	}
	templateHash := workspaceTemplateSnapshotHash(template)
	policyBindings, requiredOwnerFacts, ownerFactConvergences, profileContractsHash, err := definitionAdmissionProfileBindings(workspace, template)
	if err != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, err
	}
	dodInputs := make([]workspacemodel.DefinitionCandidateItem, 0, len(dod.Items))
	dodRefs := make([]string, 0, len(dod.Items))
	for idx, item := range dod.Items {
		ref := fmt.Sprintf("dod:%s:item:%06d", dod.ID, idx+1)
		dodRefs = append(dodRefs, ref)
		dodInputs = append(dodInputs, workspacemodel.DefinitionCandidateItem{CandidateItemRef: ref, Text: item, SourceIdentity: "dod:" + string(dod.ID) + "@" + dod.DefinitionRevision})
	}
	candidate := workspacemodel.DefinitionAdmissionCandidate{SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: workspace.ID, CandidateID: "workspace-definition", CandidateRevision: revision, BaseDefinitionRevision: baseRevision, BaseStartupInputHash: baseHash,
		GoalInput: workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal:" + string(goal.ID), Text: goal.Text, SourceIdentity: "goal:" + string(goal.ID) + "@" + goal.DefinitionRevision}, DoDInputs: dodInputs,
		WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{TemplateRef: template.ID, Revision: "template:" + templateHash, SnapshotHash: templateHash}, AdmissionCompilerVersion: workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: workspace.DeliverableDirectory,
		AdmissionPolicyBindings: policyBindings, RequiredOwnerFactBindings: requiredOwnerFacts, OwnerFactConvergenceBindings: ownerFactConvergences,
		RuntimeProfileContractsHash: profileContractsHash,
		ProposedRelationInputs:      []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: dodRefs, TargetItemRefs: []string{"goal:" + string(goal.ID)}}}}
	candidate = candidate.WithHash()
	for idx := len(workspace.DefinitionAdmissionCandidates) - 1; idx >= 0; idx-- {
		existing := workspace.DefinitionAdmissionCandidates[idx]
		if existing.CandidateID != candidate.CandidateID || existing.CandidateRevision+1 != candidate.CandidateRevision {
			continue
		}
		equivalent := candidate
		equivalent.CandidateRevision = existing.CandidateRevision
		equivalent = equivalent.WithHash()
		if equivalent.CandidateHash == existing.CandidateHash {
			return existing.Clone(), nil
		}
		break
	}
	return candidate, nil
}

func definitionAdmissionProfileBindings(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) ([]workspacemodel.DefinitionAdmissionPolicyBinding, []workspacemodel.DefinitionRequiredOwnerFactBinding, []workspacemodel.DefinitionOwnerFactConvergenceBinding, string, error) {
	if workspace == nil || template == nil {
		return nil, nil, nil, "", common.ErrInvalidArgument
	}
	effective := activationcore.MergeTemplateWorkspaceContextHintsForExecutionContract(template.ContextHints.Map(), workspace.ContextHints.Map())
	rawProfiles := runtimeMap(effective["profileContracts"])
	contract := runtimeExecutionContract(map[string]any{"profileContracts": rawProfiles})
	profiles := runtimecontract.NormalizeRuntimeProfileContracts(contract.ProfileContracts)
	policies := make([]workspacemodel.DefinitionAdmissionPolicyBinding, 0, len(profiles))
	required := make([]workspacemodel.DefinitionRequiredOwnerFactBinding, 0)
	convergences := make([]workspacemodel.DefinitionOwnerFactConvergenceBinding, 0)
	for _, profile := range profiles {
		encoded, err := json.Marshal(rawProfiles[profile.ID])
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("%w: profile %q cannot be canonically sealed", common.ErrConflict, profile.ID)
		}
		h := sha256.New()
		_, _ = fmt.Fprintf(h, "%08x%s%08x%s", len(profile.ID), profile.ID, len(encoded), encoded)
		policies = append(policies, workspacemodel.DefinitionAdmissionPolicyBinding{
			PolicyRef: "profile:" + profile.ID, Revision: "runtime-profile-contract-v1",
			SnapshotHash: "profile-snapshot-sha256:" + hex.EncodeToString(h.Sum(nil)),
		})
		for _, evidence := range profile.EvidenceRequirements {
			sourceByRequirement := make(map[string]string, len(evidence.RequiredOwnerFacts))
			for _, fact := range evidence.RequiredOwnerFacts {
				sourceRef := definitionOwnerFactSourceRef(profile.ID, evidence.Scope, evidence.ID, fact)
				required = append(required, workspacemodel.DefinitionRequiredOwnerFactBinding{SourceRef: sourceRef, Requirement: fact})
				sourceByRequirement[runtimecontract.RuntimeOwnerFactRequirementRef(fact)] = sourceRef
			}
			for _, convergence := range evidence.OwnerFactConvergences {
				sourceRef := definitionOwnerFactSourceRef(profile.ID, evidence.Scope, evidence.ID, convergence.Requirement)
				convergence = convergence.Normalized()
				for idx, dependency := range convergence.DependsOnRequirementRefs {
					if mapped := sourceByRequirement[dependency]; mapped != "" {
						convergence.DependsOnRequirementRefs[idx] = mapped
					}
				}
				convergences = append(convergences, workspacemodel.DefinitionOwnerFactConvergenceBinding{SourceRef: sourceRef, Convergence: convergence})
			}
		}
	}
	return policies, required, convergences, runtimecontract.StableRuntimeProfileContractsHash(profiles), nil
}

func definitionOwnerFactSourceRef(profileID, scope, requirementID string, fact runtimecontract.RuntimeOwnerFactRequirement) string {
	fact = fact.Normalized()
	return strings.Join([]string{"profile", strings.TrimSpace(profileID), strings.TrimSpace(scope), strings.TrimSpace(requirementID), fact.OwnerModuleID, string(fact.FactKind), fact.OperationKind}, "/")
}

func latestPriorInvalidDefinitionCandidate(workspace *workspacemodel.Workspace, current workspacemodel.DefinitionAdmissionCandidate) (workspacemodel.DefinitionAdmissionCandidate, workspacemodel.DefinitionAdmissionResult, bool) {
	if workspace == nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, workspacemodel.DefinitionAdmissionResult{}, false
	}
	// An eligible activation closes only candidates at or before the admitted
	// revision. Invalid authoring attempts after that baseline start a new
	// clarification lineage and must still be reconciled by a later candidate.
	eligibleBaseline := uint64(0)
	if workspace.ActivationAdmission != nil {
		admission := workspace.ActivationAdmission.Normalized()
		if admission.State == workspacemodel.WorkspaceActivationEligible && admission.CandidateID == current.CandidateID {
			eligibleBaseline = admission.CandidateRevision
		}
	}
	var selected workspacemodel.DefinitionAdmissionCandidate
	var selectedAdmission workspacemodel.DefinitionAdmissionResult
	for _, candidate := range workspace.DefinitionAdmissionCandidates {
		if candidate.CandidateID != current.CandidateID || candidate.CandidateRevision <= eligibleBaseline || candidate.CandidateRevision >= current.CandidateRevision || candidate.CandidateRevision <= selected.CandidateRevision {
			continue
		}
		admission, err := definitionadmissioncore.Compile(candidate)
		if err != nil || admission.Disposition == workspacemodel.DefinitionAdmissionAdmittedAllOf {
			continue
		}
		selected, selectedAdmission = candidate.Clone(), admission
	}
	return selected, selectedAdmission, selected.CandidateRevision != 0
}

func completePendingWorkspaceDefinitionClarification(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace) error {
	if workspace == nil || workspace.StartupInput == nil || workspace.ActivationAdmission == nil {
		return common.ErrInvalidArgument
	}
	current := workspace.ActivationAdmission.Normalized()
	var promoted workspacemodel.DefinitionAdmissionCandidate
	for _, candidate := range workspace.DefinitionAdmissionCandidates {
		if candidate.CandidateID == current.CandidateID && candidate.CandidateRevision == current.CandidateRevision && candidate.CandidateHash == current.CandidateHash {
			promoted = candidate.Clone()
			break
		}
	}
	admission, err := definitionadmissioncore.Compile(promoted)
	if err != nil || admission.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		return common.ErrConflict
	}
	previous, _, ok := latestPriorInvalidDefinitionCandidate(workspace, promoted)
	if !ok {
		return common.ErrConflict
	}
	resolved, err := governanceports.ResolveWorkspaceDefinitionClarificationByAdmission(ctx, hub, decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: current.BlockingTicketID, WorkspaceID: workspace.ID,
		OldCandidateID: previous.CandidateID, OldCandidateRevision: previous.CandidateRevision, OldCandidateHash: previous.CandidateHash,
		NewCandidateID: promoted.CandidateID, NewCandidateRevision: promoted.CandidateRevision, NewCandidateHash: promoted.CandidateHash,
		AdmissionResultHash: admission.AdmissionResultHash, DefinitionRevision: current.DefinitionRevision, StartupInputHash: current.StartupInputHash,
	})
	if err != nil {
		return err
	}
	_, err = workspaceports.AcknowledgeWorkspaceDefinitionClarification(ctx, hub, workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand{
		WorkspaceID: workspace.ID, CandidateID: promoted.CandidateID, CandidateRevision: promoted.CandidateRevision, CandidateHash: promoted.CandidateHash,
		BlockingTicketID: current.BlockingTicketID, ResolutionID: resolved.ResolutionID, ResolutionBindingHash: resolved.ResolutionBindingHash,
		DefinitionRevision: current.DefinitionRevision, StartupInputHash: current.StartupInputHash,
	})
	return err
}

func workspaceDefinitionAdmissionEligible(workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string) bool {
	if workspace == nil || workspace.StartupInput == nil || workspace.ActivationAdmission == nil {
		return false
	}
	startup := workspace.StartupInput.Normalized()
	admission := workspace.ActivationAdmission.Normalized()
	if startup.Validate() != nil || admission.Validate(startup) != nil || admission.State != workspacemodel.WorkspaceActivationEligible || startup.DefinitionRevision != definitionRevision || startup.GoalTextHash != workspacemodel.StableDefinitionTextHash(goal.Text) || startup.DoDItemSetHash != workspacemodel.StableDefinitionStringSetHash(dod.Items) || startup.ResolvedProjectWorkspaceDirectory != strings.TrimSpace(workspace.DeliverableDirectory) {
		return false
	}
	return true
}

func validateCurrentWorkspaceDefinitionAdmission(workspace *workspacemodel.Workspace) error {
	candidate, present, err := workspace.ValidateActivationAdmissionPersistence()
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	result, err := definitionadmissioncore.Compile(candidate)
	if err != nil {
		return err
	}
	if result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		return fmt.Errorf("activation admission candidate is not admitted_all_of")
	}
	return nil
}

func invalidCurrentWorkspaceStartupInput(cause error) error {
	return fmt.Errorf("%w: current Workspace startup input does not match the only supported shape; recreate the Workspace from the current Template: %v", common.ErrConflict, cause)
}

func workspaceTemplateSnapshotHash(template *workspacemodel.WorkspaceTemplate) string {
	h := sha256.New()
	values := []string{template.ID, template.Name, template.Description, template.GoalText, strings.Join(template.DoDItems, "\x00"), template.DefaultSoulID, template.ContextHints.CanonicalString()}
	for _, policy := range template.EvidencePolicies {
		values = append(values, policy.CanonicalString())
	}
	for _, value := range values {
		_, _ = fmt.Fprintf(h, "%08x%s", len(value), value)
	}
	return "template-snapshot-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func workspaceClarificationCommand(candidate workspacemodel.DefinitionAdmissionCandidate, admission workspacemodel.DefinitionAdmissionResult) decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand {
	command := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand{SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent, WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash, ViolationCode: admission.ViolationCode, OffendingRefs: append([]string(nil), admission.OffendingRefs...), CanonicalSignatureVersion: "workspace-definition-clarification-v1", CanonicalSignature: admission.CanonicalViolation, RequiredActionKind: decisionevents.DefinitionClarificationActionRewriteAllOf}
	command.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(command)
	return command
}
