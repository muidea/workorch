package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
)

const DefinitionDerivationRequestLimit = 16
const DefinitionDerivationAttemptLimit = 2

// DefinitionDerivationRequest 记录一次派生调用，nil 结果表示已记账但结果未决。
// Input/Template 为调用时快照，编辑当前草稿不能覆盖历史预算。
type DefinitionDerivationRequest struct {
	RequestID            string
	Input                DefinitionAuthoringInput
	Template             WorkspaceTemplate
	Resources            []DefinitionAuthoringResourceSnapshot
	ResourceSnapshotHash string
	ProviderBindingHash  string
	PermitID             string
	Proposal             *DefinitionDerivedAcceptance
	FailureCode          string
}

func (r DefinitionDerivationRequest) Clone() DefinitionDerivationRequest {
	r.Input = r.Input.Clone()
	r.Template = r.Template.Clone()
	r.Resources = CloneDefinitionAuthoringResources(r.Resources)
	if r.Proposal != nil {
		p := *r.Proposal
		if p.AdditionalItems != nil {
			p.AdditionalItems = append([]string{}, p.AdditionalItems...)
		}
		r.Proposal = &p
	}
	return r
}

func (r DefinitionDerivationRequest) ExpectedPermitID() string {
	h := sha256.New()
	writeDefinitionHash(h, "workspace-derivation-permit-v2", r.RequestID, r.Input.InputHash(), WorkspaceTemplateSnapshotHash(&r.Template), r.ResourceSnapshotHash, r.ProviderBindingHash)
	return "derivation-permit-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (r DefinitionDerivationRequest) Validate() error {
	if r.Input.Validate() != nil || r.Template.ID != r.Input.Normalized().TemplateID || r.RequestID == "" || len(r.RequestID) > 128 || r.ProviderBindingHash == "" || len(r.ProviderBindingHash) > 256 || ValidateDefinitionAuthoringResources(r.Input, r.Resources) != nil || r.ResourceSnapshotHash != DefinitionAuthoringResourcesHash(r.Resources) || r.PermitID != r.ExpectedPermitID() {
		return fmt.Errorf("derivation request identity invalid")
	}
	if r.Proposal != nil {
		if r.FailureCode != "" {
			return fmt.Errorf("derivation has conflicting outcomes")
		}
		return r.Proposal.ValidateFor(r.Input, &r.Template)
	}
	switch r.FailureCode {
	case "", "provider_failed", "invalid_proposal":
		return nil
	default:
		return fmt.Errorf("derivation failure code invalid")
	}
}

func validateDerivationRequests(records []DefinitionDerivationRequest, input DefinitionAuthoringInput) error {
	if len(records) > DefinitionDerivationRequestLimit {
		return fmt.Errorf("derivation request budget exceeded")
	}
	seen := map[string]bool{}
	attempts := map[string]int{}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
		if record.Input.WorkspaceID != input.WorkspaceID || record.Input.Revision > input.Revision || seen[record.RequestID] {
			return fmt.Errorf("derivation history scope or identity invalid")
		}
		key := record.Input.InputHash() + "\x00" + WorkspaceTemplateSnapshotHash(&record.Template)
		attempts[key]++
		if attempts[key] > DefinitionDerivationAttemptLimit {
			return fmt.Errorf("derivation input attempt budget exceeded")
		}
		seen[record.RequestID] = true
	}
	return nil
}

// ReserveDerivation 仅计算下一份记录；调用方必须 CAS 成功后才能使用 fresh=true。
func (r DefinitionAuthoringRecord) ReserveDerivation(requestID, providerBindingHash string, resources []DefinitionAuthoringResourceSnapshot) (next DefinitionAuthoringRecord, request DefinitionDerivationRequest, fresh bool, err error) {
	if err = r.Validate(); err != nil {
		return
	}
	if err = ValidateDefinitionAuthoringResources(r.Input, resources); err != nil {
		return
	}
	request = DefinitionDerivationRequest{RequestID: requestID, Input: r.Input.Clone(), Template: r.TemplateSnapshot.Clone(), Resources: CloneDefinitionAuthoringResources(resources), ResourceSnapshotHash: DefinitionAuthoringResourcesHash(resources), ProviderBindingHash: providerBindingHash}
	request.PermitID = request.ExpectedPermitID()
	if err = request.Validate(); err != nil {
		return
	}
	for _, previous := range r.DerivationRequests {
		if previous.RequestID == requestID {
			if previous.PermitID != request.PermitID {
				err = fmt.Errorf("derivation request identity conflict")
				return
			}
			return r.Clone(), previous.Clone(), false, nil
		}
	}
	if len(r.DerivationRequests) == DefinitionDerivationRequestLimit {
		err = fmt.Errorf("derivation request budget exhausted")
		return
	}
	attempts := 0
	for _, previous := range r.DerivationRequests {
		if previous.Input.InputHash() == r.InputHash && WorkspaceTemplateSnapshotHash(&previous.Template) == r.TemplateSnapshotHash {
			attempts++
		}
	}
	if attempts >= DefinitionDerivationAttemptLimit {
		err = fmt.Errorf("derivation input attempt budget exhausted")
		return
	}
	next = r.Clone()
	next.DerivationRequests = append(next.DerivationRequests, request.Clone())
	return next, request, true, nil
}

func (r DefinitionAuthoringRecord) CompleteDerivation(permitID string, proposal *DefinitionDerivedAcceptance, failureCode string) (DefinitionAuthoringRecord, error) {
	if err := r.Validate(); err != nil {
		return DefinitionAuthoringRecord{}, err
	}
	for n, previous := range r.DerivationRequests {
		if previous.PermitID != permitID {
			continue
		}
		completed := previous.Clone()
		completed.Proposal, completed.FailureCode = proposal, failureCode
		completed = completed.Clone()
		if proposal == nil && failureCode == "" {
			return DefinitionAuthoringRecord{}, fmt.Errorf("derivation completion requires outcome")
		}
		if err := completed.Validate(); err != nil {
			return DefinitionAuthoringRecord{}, err
		}
		if previous.Proposal != nil || previous.FailureCode != "" {
			if !reflect.DeepEqual(previous, completed) {
				return DefinitionAuthoringRecord{}, fmt.Errorf("derivation completion conflict")
			}
			return r.Clone(), nil
		}
		// 旧版本回执只能封存其原请求结果，不能改写当前草稿或授予准入。
		next := r.Clone()
		next.DerivationRequests[n] = completed
		return next, nil
	}
	return DefinitionAuthoringRecord{}, fmt.Errorf("derivation permit not found")
}
