package workspace

import (
	"bytes"
	"slices"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparationSkillContextVersion = "workorch-preparation-skill-context-compilation/v1"

// A pure preview of the existing bounded context contract, never a sealed Run
// context or availability proof. Selected retains the entire source material;
// only this separately identified projection may be explicitly bounded.
type PreparationSkillContextCompilation struct {
	Version                  string
	Projection               []runtimecontract.RuntimeSkillContext
	Budget                   *runtimecontract.RuntimeSkillContextBudget
	FailureCode              string
	Diagnostic               []byte
	DiagnosticBytes          uint64
	DiagnosticSHA256         string
	DiagnosticNotSavedReason string
}

func (c PreparationSkillContextCompilation) Summary() string {
	if c.FailureCode != "" {
		return "rejected"
	}
	if c.Budget != nil && c.Budget.TruncatedSkills != 0 {
		return "bounded_checks_pending"
	}
	return "compiled_checks_pending"
}

func (c PreparationSkillContextCompilation) Clone() PreparationSkillContextCompilation {
	c.Diagnostic = bytes.Clone(c.Diagnostic)
	if c.Budget != nil {
		b := *c.Budget
		c.Budget = &b
	}
	c.Projection = slices.Clone(c.Projection)
	for i := range c.Projection {
		p := &c.Projection[i]
		p.TriggerRules = slices.Clone(p.TriggerRules)
		p.ReferenceRefs = slices.Clone(p.ReferenceRefs)
		p.RequiredToolKinds = slices.Clone(p.RequiredToolKinds)
		p.RequiredMCPServerRefs = slices.Clone(p.RequiredMCPServerRefs)
		p.RequiredCapabilityRefs = slices.Clone(p.RequiredCapabilityRefs)
		p.Constraints = slices.Clone(p.Constraints)
	}
	return c
}

// CompilePreparationSkillContext uses the same pure policy as activation. It
// neither selects again nor reads owners/provider/files. Independent source
// recomputation is performed by the containing selection persistence validator.
func CompilePreparationSkillContext(selected []PreparationSelectedSkill) PreparationSkillContextCompilation {
	c := PreparationSkillContextCompilation{Version: PreparationSkillContextVersion}
	values := make([]runtimecontract.RuntimeSkillContext, 0, len(selected))
	for _, s := range selected {
		v := s.Content
		values = append(values, runtimecontract.RuntimeSkillContext{
			ID: v.ID, Name: v.Name, Version: v.Version, Description: v.Description,
			Instruction: v.Instruction, TriggerRules: slices.Clone(v.TriggerRules), ReferenceRefs: slices.Clone(v.ReferenceRefs),
			RequiredToolKinds: slices.Clone(v.RequiredToolKinds), RequiredMCPServerRefs: slices.Clone(v.RequiredMCPServerRefs),
			RequiredCapabilityRefs: slices.Clone(v.RequiredCapabilityRefs), Constraints: slices.Clone(v.Constraints), Provenance: s.Provenance,
		})
	}
	var err error
	c.Projection, c.Budget, err = runtimecontract.SealRuntimeSkillContexts(values)
	if err != nil {
		c.FailureCode = "context_contract_rejected"
		raw := []byte(err.Error())
		c.DiagnosticBytes, c.DiagnosticSHA256 = uint64(len(raw)), fileobservation.Digest(raw)
		if len(raw) > PreparationSkillHubErrorLimit {
			c.DiagnosticNotSavedReason = "error_exceeds_storage_limit"
		} else {
			c.Diagnostic = raw
		}
	}
	return c.Clone()
}
