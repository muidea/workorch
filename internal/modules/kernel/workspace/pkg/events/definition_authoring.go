package events

import (
	"fmt"
	"reflect"
	"strings"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceAuthoringDiagnosticQuery = "/workspace/definition-authoring/diagnostic/query"

type QueryAuthoringDiagnosticCommand struct {
	WorkspaceID common.WorkspaceID
	PermitID    string
}

const CommandSource = "application/workorchd"

// 派生阶段只用于诊断观察，不扩展规格请求 ledger 的合法阶段。
const AuthoringDiagnosticDerivation workspace.DefinitionSpecificationPhase = "derivation"

type QueryAuthoringDiagnosticResult struct {
	WorkspaceID common.WorkspaceID
	PermitID    string
	Phase       workspace.DefinitionSpecificationPhase
	FailureCode string
	Diagnostic  workspace.DefinitionAuthoringDiagnostic
}

func (c QueryAuthoringDiagnosticCommand) Validate() error {
	if c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256 {
		return common.ErrInvalidArgument
	}
	return nil
}

func (r QueryAuthoringDiagnosticResult) ValidateFor(c QueryAuthoringDiagnosticCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.PermitID != c.PermitID || r.Diagnostic.Validate() != nil {
		return common.ErrConflict
	}
	if r.FailureCode == "provider_failed" && r.Diagnostic.ProviderCode != "" && (r.Phase == AuthoringDiagnosticDerivation || r.Phase == workspace.DefinitionSpecificationProposal || r.Phase == workspace.DefinitionSpecificationCritic) {
		return nil
	}
	if (r.Phase == AuthoringDiagnosticDerivation || r.Phase == workspace.DefinitionSpecificationProposal) && r.FailureCode == "invalid_proposal" || r.Phase == workspace.DefinitionSpecificationCritic && r.FailureCode == "invalid_review" {
		return nil
	}
	return common.ErrConflict
}

type SaveDefinitionAuthoringCommand struct {
	RequestID                 string
	ExpectedInputHash         string
	Input                     workspace.DefinitionAuthoringInput
	ExpectedWorkspaceRevision uint64
	DeliverableDirectory      *string
}

func (c SaveDefinitionAuthoringCommand) Validate() error {
	if err := c.Input.Validate(); err != nil {
		return err
	}
	if c.DeliverableDirectory != nil && (strings.TrimSpace(*c.DeliverableDirectory) == "" || len(*c.DeliverableDirectory) > 32*1024 || strings.ContainsRune(*c.DeliverableDirectory, 0)) {
		return fmt.Errorf("authoring directory is invalid")
	}
	if strings.TrimSpace(c.RequestID) != c.RequestID || c.RequestID == "" || len(c.RequestID) > 256 || strings.ContainsRune(c.RequestID, 0) {
		return fmt.Errorf("authoring save requires request identity")
	}
	if c.ExpectedWorkspaceRevision == 0 {
		return fmt.Errorf("authoring save requires workspace revision")
	}
	return nil
}

type SaveDefinitionAuthoringResult struct {
	Receipt workspace.DefinitionAuthoringSaveReceipt
	Record  DefinitionAuthoringRecord
}

type QueryDefinitionAuthoringCommand struct {
	WorkspaceID common.WorkspaceID
	// InputHash 显式选择原输入或独立待审输入；空值仍只查询原输入。
	InputHash string
}

type QueryDefinitionAuthoringResult struct {
	WorkspaceID                 common.WorkspaceID
	WorkspaceRevision           uint64
	Record                      *DefinitionAuthoringRecord
	History                     []DefinitionAuthoringConfirmedRevision
	Confirmation                *workspace.DefinitionAuthoringConfirmation
	PriorClarification          *DefinitionAdmissionClarificationBinding
	AdmissionCandidate          *workspace.DefinitionAdmissionCandidate
	AdmissionResult             *workspace.DefinitionAdmissionResult
	ActivationAdmission         *workspace.WorkspaceActivationAdmission
	DeliverableDirectory        string
	CurrentTemplateSnapshotHash string
	CurrentTemplateAvailable    bool
}

func (r QueryDefinitionAuthoringResult) ValidateFor(c QueryDefinitionAuthoringCommand) error {
	if c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.CurrentTemplateAvailable != (r.CurrentTemplateSnapshotHash != "") {
		return fmt.Errorf("authoring query result scope is invalid")
	}
	if c.InputHash != "" && (len(c.InputHash) > 256 || r.Record == nil || r.Record.InputHash != c.InputHash || r.Confirmation != nil || r.AdmissionCandidate != nil || r.ActivationAdmission != nil || len(r.History) != 0) {
		return common.ErrConflict
	}
	if r.Record != nil && r.Record.Input.WorkspaceID != c.WorkspaceID {
		return fmt.Errorf("authoring query record scope mismatch")
	}
	var record *workspace.DefinitionAuthoringRecord
	if r.Record != nil {
		model, err := r.Record.Model()
		if err != nil {
			return err
		}
		record = &model
	}
	history, err := AuthoringHistoryModels(r.History)
	if err != nil {
		return err
	}
	if (&workspace.Workspace{ID: c.WorkspaceID, Revision: r.WorkspaceRevision, DefinitionAuthoring: record, DefinitionAuthoringHistory: history}).ValidatePreparationDiagnosticHistoryView() != nil {
		return common.ErrConflict
	}
	var goal workspace.DefinitionCandidateItem
	var dod []workspace.DefinitionCandidateItem
	var spec workspace.GoalSpecification
	var ref workspace.DefinitionAuthoringAdmissionRef
	if r.Confirmation != nil {
		sequence := r.Confirmation.FormalSequence
		if sequence == 0 && r.Confirmation.PreviousInputHash == "" {
			sequence = 1
		}
		if sequence != uint64(len(r.History)+1) {
			return fmt.Errorf("authoring query confirmation lacks exact history sequence")
		}
		if r.AdmissionCandidate != nil {
			var err error
			goal, dod, spec, ref, err = workspace.ConfirmedAuthoringAdmissionSourceView(record, *r.Confirmation, c.WorkspaceID)
			if err != nil {
				return fmt.Errorf("authoring query confirmation lacks exact record: %w", err)
			}
		} else if r.Confirmation.ValidateView(record, c.WorkspaceID) != nil {
			return fmt.Errorf("authoring query confirmation lacks exact record")
		}
	} else if r.Record != nil {
		if err := record.ValidateView(); err != nil {
			return err
		}
	}
	if len(r.History) > 0 {
		if r.Record == nil || r.Record.Input.Revision <= r.History[len(r.History)-1].Record.Input.Revision {
			return fmt.Errorf("authoring query lacks successor input")
		}
		previousRevision := uint64(0)
		for index, archived := range history {
			if archived.Record.Input.Revision <= previousRevision || archived.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || archived.Confirmation.ValidateView(&archived.Record, c.WorkspaceID) != nil {
				return fmt.Errorf("authoring query history is invalid")
			}
			previousRevision = archived.Record.Input.Revision
			if index == 0 && archived.Confirmation.FormalSequence > 1 || index > 0 && archived.Confirmation.FormalSequence != uint64(index+1) {
				return fmt.Errorf("authoring query history formal sequence is invalid")
			}
			if index > 0 && archived.Confirmation.ValidateSuccessor(r.History[index-1].Confirmation) != nil {
				return fmt.Errorf("authoring query history lineage is invalid")
			}
		}
		if r.Confirmation != nil && r.Confirmation.ValidateSuccessor(r.History[len(r.History)-1].Confirmation) != nil {
			return fmt.Errorf("authoring query confirmation lineage is invalid")
		}
	}
	if r.PriorClarification != nil {
		if r.PriorClarification.Validate() != nil || r.PriorClarification.Candidate.WorkspaceID != c.WorkspaceID || r.PriorClarification.Candidate.CandidateRevision > uint64(len(r.History)) {
			return fmt.Errorf("authoring query prior clarification is invalid")
		}
	}
	if r.AdmissionCandidate != nil {
		if r.Confirmation == nil || r.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || r.AdmissionCandidate.Validate() != nil || r.AdmissionCandidate.AuthoringRef == nil || r.AdmissionCandidate.WorkspaceID != c.WorkspaceID || r.AdmissionResult == nil || r.AdmissionResult.ValidateFor(*r.AdmissionCandidate) != nil {
			return fmt.Errorf("authoring query admission lacks reviewed source")
		}
		if r.ActivationAdmission == nil && r.AdmissionResult.Disposition != workspace.DefinitionAdmissionClarificationRequired && r.PriorClarification == nil {
			return fmt.Errorf("authoring query has an admitted candidate without prior clarification")
		}
		if r.PriorClarification != nil && (r.PriorClarification.Candidate.CandidateID != r.AdmissionCandidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.AdmissionCandidate.CandidateRevision) {
			return fmt.Errorf("authoring query prior clarification does not precede candidate")
		}
		if r.AdmissionCandidate.CandidateRevision != uint64(len(r.History)+1) {
			return fmt.Errorf("authoring query candidate revision differs from history")
		}
		if *r.AdmissionCandidate.AuthoringRef != ref {
			return fmt.Errorf("authoring query admission source mismatch")
		}
		if r.AdmissionCandidate.GoalInput != goal || !reflect.DeepEqual(r.AdmissionCandidate.DoDInputs, workspace.DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || r.AdmissionCandidate.Specification == nil || !reflect.DeepEqual(r.AdmissionCandidate.Specification.Normalized(), spec.Normalized()) {
			return fmt.Errorf("authoring query admission reviews mismatch")
		}
		if r.ActivationAdmission != nil && (r.AdmissionResult.Disposition != workspace.DefinitionAdmissionAdmittedAllOf || r.ActivationAdmission.CandidateHash != r.AdmissionCandidate.CandidateHash || r.ActivationAdmission.State != workspace.WorkspaceActivationEligible && r.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution || r.ActivationAdmission.State == workspace.WorkspaceActivationPendingClarificationResolution && r.PriorClarification == nil) {
			return fmt.Errorf("authoring query admission has invalid activation")
		}
		if r.ActivationAdmission != nil && r.PriorClarification != nil {
			identity, err := r.PriorClarification.DecisionRequestIdentity()
			if err != nil || r.ActivationAdmission.BlockingTicketID != identity.TicketID {
				return fmt.Errorf("authoring query activation does not bind prior ticket")
			}
		}
	} else if r.AdmissionResult != nil || r.ActivationAdmission != nil {
		return fmt.Errorf("authoring query has admission without candidate")
	}
	return nil
}
