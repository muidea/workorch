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
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	definitionadmission "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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

// EnsureWorkspaceDefinitionAdmission converges current owner snapshots into an
// immutable pre-seal candidate and an eligible all-of startup input. It runs
// before RuntimeBootstrap compilation and performs no provider/capability
// call. Invalid candidates stop after a dedicated Decision ticket is opened.
func EnsureWorkspaceDefinitionAdmission(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (WorkspaceDefinitionAdmissionSnapshot, error) {
	if hub == nil || workspace == nil || goal == nil || dod == nil || workspace.ID == "" || goal.WorkspaceID != workspace.ID || dod.WorkspaceID != workspace.ID {
		return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrInvalidArgument
	}
	definitionRevision := workspacecontracts.ExecutionDefinitionRevision(string(goal.ID), goal.DefinitionRevision, string(dod.ID), dod.DefinitionRevision)
	if definitionRevision == "" {
		return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: current Goal/DoD revision is incomplete", common.ErrConflict)
	}
	if err := validateCurrentWorkspaceDefinitionAdmission(workspace); err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, invalidCurrentWorkspaceStartupInput(err)
	}
	template, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, strings.TrimSpace(workspace.TemplateID))
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: Workspace Template snapshot is required for Definition admission: %v", common.ErrConflict, err)
	}
	currentCandidate, currentMatches, err := currentDefinitionAdmissionMatchesOwnerInputs(workspace, goal, dod, template, definitionRevision)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	if currentMatches && workspaceDefinitionAdmissionEligible(workspace, currentCandidate) {
		return WorkspaceDefinitionAdmissionSnapshot{Workspace: workspace, Template: template, Candidate: currentCandidate}, nil
	}
	if currentMatches && workspace.ActivationAdmission != nil && workspace.ActivationAdmission.State == workspacemodel.WorkspaceActivationPendingClarificationResolution {
		if err := completePendingWorkspaceDefinitionClarification(ctx, hub, workspace); err != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, err
		}
		updated, err := workspaceports.ReadWorkspaceRecord(ctx, hub, workspace.ID)
		if err != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, err
		}
		if err := validateCurrentWorkspaceDefinitionAdmission(updated); err != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, invalidCurrentWorkspaceStartupInput(err)
		}
		if workspaceDefinitionAdmissionEligible(updated, currentCandidate) {
			return WorkspaceDefinitionAdmissionSnapshot{Workspace: updated, Template: template, Candidate: currentCandidate}, nil
		}
		return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
	}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	result, err := workspaceports.SubmitDefinitionAdmissionCandidate(ctx, hub, candidate, candidate.CandidateRevision-1)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	admission := result.AdmissionResult
	if admission.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		command := workspaceClarificationCommand(result.Candidate, admission)
		previousCandidate, previousAdmission, hasPrevious := latestPriorInvalidDefinitionCandidate(workspace, result.Candidate)
		if hasPrevious {
			oldCommand := workspaceClarificationCommand(previousCandidate, previousAdmission)
			if _, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, oldCommand); openErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, openErr
			}
			reconciled, reconcileErr := governanceports.ReconcileWorkspaceDefinitionClarificationCandidate(ctx, hub, decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: oldCommand, NewRequest: command})
			if reconcileErr != nil {
				return WorkspaceDefinitionAdmissionSnapshot{}, reconcileErr
			}
			return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: %s; blocking ticket %s", common.ErrConflict, admission.ViolationCode, reconciled.NewTicketID)
		}
		opened, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, command)
		if openErr != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, openErr
		}
		return WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: %s; blocking ticket %s", common.ErrConflict, admission.ViolationCode, opened.TicketID)
	}
	previousCandidate, previousAdmission, clarified := latestPriorInvalidDefinitionCandidate(workspace, result.Candidate)
	var blockingTicketID common.TicketID
	if clarified {
		oldCommand := workspaceClarificationCommand(previousCandidate, previousAdmission)
		opened, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, hub, oldCommand)
		if openErr != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, openErr
		}
		blockingTicketID = opened.TicketID
	}
	promoted, err := workspaceports.PromoteDefinitionAdmissionCandidate(ctx, hub, result.Candidate, blockingTicketID)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	if clarified {
		resolved, resolveErr := governanceports.ResolveWorkspaceDefinitionClarificationByAdmission(ctx, hub, decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
			TicketID: blockingTicketID, WorkspaceID: workspace.ID,
			OldCandidateID: previousCandidate.CandidateID, OldCandidateRevision: previousCandidate.CandidateRevision, OldCandidateHash: previousCandidate.CandidateHash,
			NewCandidateID: result.Candidate.CandidateID, NewCandidateRevision: result.Candidate.CandidateRevision, NewCandidateHash: result.Candidate.CandidateHash,
			AdmissionResultHash: admission.AdmissionResultHash, DefinitionRevision: promoted.DefinitionRevision, StartupInputHash: promoted.StartupInputHash,
		})
		if resolveErr != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, resolveErr
		}
		if _, ackErr := workspaceports.AcknowledgeWorkspaceDefinitionClarification(ctx, hub, workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand{
			WorkspaceID: workspace.ID, CandidateID: result.Candidate.CandidateID, CandidateRevision: result.Candidate.CandidateRevision, CandidateHash: result.Candidate.CandidateHash,
			BlockingTicketID: blockingTicketID, ResolutionID: resolved.ResolutionID, ResolutionBindingHash: resolved.ResolutionBindingHash,
			DefinitionRevision: promoted.DefinitionRevision, StartupInputHash: promoted.StartupInputHash,
		}); ackErr != nil {
			return WorkspaceDefinitionAdmissionSnapshot{}, ackErr
		}
	}
	updated, err := workspaceports.ReadWorkspaceRecord(ctx, hub, workspace.ID)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	if err := validateCurrentWorkspaceDefinitionAdmission(updated); err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, invalidCurrentWorkspaceStartupInput(err)
	}
	if !workspaceDefinitionAdmissionEligible(updated, result.Candidate) {
		return WorkspaceDefinitionAdmissionSnapshot{}, common.ErrConflict
	}
	return WorkspaceDefinitionAdmissionSnapshot{Workspace: updated, Template: template, Candidate: result.Candidate.Clone()}, nil
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
	candidate, err := definitionAdmissionCandidateForIdentity(workspace, goal, dod, template, "workspace-definition", revision, baseRevision, baseHash)
	if err != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, err
	}
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

func definitionAdmissionCandidateForIdentity(workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, template *workspacemodel.WorkspaceTemplate, candidateID string, revision uint64, baseRevision, baseHash string) (workspacemodel.DefinitionAdmissionCandidate, error) {
	if workspace == nil || goal == nil || dod == nil || template == nil || strings.TrimSpace(candidateID) == "" || revision == 0 {
		return workspacemodel.DefinitionAdmissionCandidate{}, common.ErrInvalidArgument
	}
	templateHash := workspaceTemplateSnapshotHash(template)
	policyBindings, profileContracts, requiredOwnerFacts, ownerFactConvergences, profileContractsHash, err := definitionAdmissionProfileBindings(workspace, template)
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
	candidate := workspacemodel.DefinitionAdmissionCandidate{SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: workspace.ID, CandidateID: strings.TrimSpace(candidateID), CandidateRevision: revision, BaseDefinitionRevision: baseRevision, BaseStartupInputHash: baseHash,
		GoalInput: workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal:" + string(goal.ID), Text: goal.Text, SourceIdentity: "goal:" + string(goal.ID) + "@" + goal.DefinitionRevision}, DoDInputs: dodInputs,
		WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{TemplateRef: template.ID, Revision: "template:" + templateHash, SnapshotHash: templateHash}, AdmissionCompilerVersion: workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: workspace.DeliverableDirectory,
		AdmissionPolicyBindings: policyBindings, RuntimeProfileContracts: profileContracts, RequiredOwnerFactBindings: requiredOwnerFacts, OwnerFactConvergenceBindings: ownerFactConvergences,
		RuntimeProfileContractsHash: profileContractsHash,
		ProposedRelationInputs:      []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: dodRefs, TargetItemRefs: []string{"goal:" + string(goal.ID)}}}}
	return candidate.WithHash(), nil
}

func definitionAdmissionProfileBindings(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) ([]workspacemodel.DefinitionAdmissionPolicyBinding, []runtimecontract.RuntimeProfileContract, []workspacemodel.DefinitionRequiredOwnerFactBinding, []workspacemodel.DefinitionOwnerFactConvergenceBinding, string, error) {
	if workspace == nil || template == nil {
		return nil, nil, nil, nil, "", common.ErrInvalidArgument
	}
	effective := activationcore.MergeTemplateWorkspaceContextHintsForExecutionContract(template.ContextHints.Map(), workspace.ContextHints.Map())
	rawProfiles := runtimeMap(effective["profileContracts"])
	if err := validateRuntimePresealTargetDeclarations(map[string]any{"profileContracts": rawProfiles}, nil); err != nil {
		return nil, nil, nil, nil, "", err
	}
	evidencePolicyBindings, err := definitionAdmissionEvidencePolicyBindings(workspace, template)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	contract := runtimeExecutionContract(map[string]any{"profileContracts": rawProfiles})
	profiles := runtimecontract.NormalizeRuntimeProfileContracts(contract.ProfileContracts)
	policies := make([]workspacemodel.DefinitionAdmissionPolicyBinding, 0, len(profiles)+len(evidencePolicyBindings))
	for _, profile := range profiles {
		encoded, err := json.Marshal(rawProfiles[profile.ID])
		if err != nil {
			return nil, nil, nil, nil, "", fmt.Errorf("%w: profile %q cannot be canonically sealed", common.ErrConflict, profile.ID)
		}
		h := sha256.New()
		_, _ = fmt.Fprintf(h, "%08x%s%08x%s", len(profile.ID), profile.ID, len(encoded), encoded)
		policies = append(policies, workspacemodel.DefinitionAdmissionPolicyBinding{
			PolicyRef: "profile:" + profile.ID, Revision: "runtime-profile-contract-v1",
			SnapshotHash: "profile-snapshot-sha256:" + hex.EncodeToString(h.Sum(nil)),
		})
	}
	policies = append(policies, evidencePolicyBindings...)
	required, convergences := definitionadmission.CompileProfileOwnerFactBindings(profiles)
	return policies, profiles, required, convergences, runtimecontract.StableRuntimeProfileContractsHash(profiles), nil
}

// definitionAdmissionEvidencePolicyBindings seals both Template input and the
// Workspace-owned effective copy before promotion. Runtime activation consumes
// the Workspace policies, while the Template remains a formal startup input;
// neither surface may hide the removed exact prospective-target declaration.
func definitionAdmissionEvidencePolicyBindings(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) ([]workspacemodel.DefinitionAdmissionPolicyBinding, error) {
	if workspace == nil || template == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRuntimeEvidencePolicyInputs("Template "+strings.TrimSpace(template.ID), template.EvidencePolicies); err != nil {
		return nil, err
	}
	if err := validateRuntimeEvidencePolicyInputs("Workspace "+strings.TrimSpace(string(workspace.ID)), workspace.EvidencePolicies); err != nil {
		return nil, err
	}
	ret := make([]workspacemodel.DefinitionAdmissionPolicyBinding, 0, len(template.EvidencePolicies)+len(workspace.EvidencePolicies))
	appendBindings := func(owner string, policies []workorcheventpayload.Payload) {
		for idx, policy := range policies {
			policyID, _ := policy.String("id")
			policyID = strings.TrimSpace(policyID)
			canonical := policy.CanonicalString()
			h := sha256.New()
			_, _ = fmt.Fprintf(h, "%08x%s", len(canonical), canonical)
			ret = append(ret, workspacemodel.DefinitionAdmissionPolicyBinding{
				PolicyRef:    fmt.Sprintf("%s-evidence-policy:%06d:%s", owner, idx+1, policyID),
				Revision:     "runtime-evidence-policy-v1",
				SnapshotHash: "evidence-policy-snapshot-sha256:" + hex.EncodeToString(h.Sum(nil)),
			})
		}
	}
	appendBindings("template", template.EvidencePolicies)
	appendBindings("workspace", workspace.EvidencePolicies)
	return ret, nil
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
		admission, err := definitionadmission.Compile(candidate)
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
	admission, err := definitionadmission.Compile(promoted)
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

func currentDefinitionAdmissionMatchesOwnerInputs(workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, template *workspacemodel.WorkspaceTemplate, definitionRevision string) (workspacemodel.DefinitionAdmissionCandidate, bool, error) {
	if workspace == nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, false, nil
	}
	candidate, present, err := workspace.ValidateActivationAdmissionPersistence()
	if err != nil || !present {
		return workspacemodel.DefinitionAdmissionCandidate{}, false, err
	}
	expected, err := definitionAdmissionCandidateForIdentity(workspace, goal, dod, template, candidate.CandidateID, candidate.CandidateRevision, candidate.BaseDefinitionRevision, candidate.BaseStartupInputHash)
	if err != nil {
		return workspacemodel.DefinitionAdmissionCandidate{}, false, err
	}
	if candidate.CandidateHash != expected.CandidateHash || workspace.StartupInput == nil || workspace.StartupInput.DefinitionRevision != strings.TrimSpace(definitionRevision) {
		return candidate, false, nil
	}
	return candidate, true, nil
}

func workspaceDefinitionAdmissionEligible(workspace *workspacemodel.Workspace, candidate workspacemodel.DefinitionAdmissionCandidate) bool {
	if workspace == nil || workspace.StartupInput == nil || workspace.ActivationAdmission == nil {
		return false
	}
	startup := workspace.StartupInput.Normalized()
	admission := workspace.ActivationAdmission.Normalized()
	if startup.ValidateCandidate(candidate) != nil || admission.ValidateCandidate(startup, candidate) != nil || admission.State != workspacemodel.WorkspaceActivationEligible {
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
	command := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand{SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent, WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash, ViolationCode: admission.ViolationCode, OffendingRefs: append([]string(nil), admission.OffendingRefs...), CanonicalSignatureVersion: "workspace-definition-clarification-v1", CanonicalSignature: admission.CanonicalViolation, RequiredActionKind: workspaceDefinitionClarificationAction(admission.ViolationCode)}
	command.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(command)
	return command
}

func workspaceDefinitionClarificationAction(violationCode string) string {
	switch strings.TrimSpace(violationCode) {
	case workspacemodel.DefinitionAdmissionProfileContractInvalid,
		workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved:
		return decisionevents.DefinitionClarificationActionFixTemplateProfile
	default:
		return decisionevents.DefinitionClarificationActionRewriteAllOf
	}
}
