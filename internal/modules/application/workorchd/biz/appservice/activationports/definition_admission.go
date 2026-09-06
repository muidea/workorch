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
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
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

// EnsureWorkspaceDefinitionAdmission compiles one immutable authoring intent,
// submits it to the Workspace owner and composes only the Decision-owner work
// requested by the factual owner outcome. Workspace alone selects candidate
// persistence, promotion and eligibility through its private reducer.
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
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		return WorkspaceDefinitionAdmissionSnapshot{}, err
	}
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
	candidate, present, err := workspace.ValidateActivationAdmissionPersistence()
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
	return fmt.Errorf("%w: Workspace admission does not match the current input; start a new execution with the current Template: %v", common.ErrConflict, cause)
}

func workspaceTemplateSnapshotHash(template *workspacemodel.WorkspaceTemplate) string {
	return workspacemodel.WorkspaceTemplateSnapshotHash(template)
}
