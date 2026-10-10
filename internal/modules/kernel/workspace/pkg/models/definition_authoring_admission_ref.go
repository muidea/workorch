package workspace

import (
	"fmt"
	"strings"
)

// DefinitionAuthoringAdmissionRef is a provenance link, not an approval. The
// Workspace owner reconstructs it from persisted provider outcomes before
// admitting a candidate; the four hashes alone cannot establish review facts.
type DefinitionAuthoringAdmissionRef struct {
	InputHash          string
	DerivationPermitID string
	ProposalHash       string
	SpecificationHash  string
}

func (r DefinitionAuthoringAdmissionRef) Normalized() DefinitionAuthoringAdmissionRef {
	r.InputHash = strings.TrimSpace(r.InputHash)
	r.DerivationPermitID = strings.TrimSpace(r.DerivationPermitID)
	r.ProposalHash = strings.TrimSpace(r.ProposalHash)
	r.SpecificationHash = strings.TrimSpace(r.SpecificationHash)
	return r
}

func (r DefinitionAuthoringAdmissionRef) Validate() error {
	r = r.Normalized()
	for _, field := range []string{r.InputHash, r.DerivationPermitID, r.ProposalHash, r.SpecificationHash} {
		if field == "" || len(field) > 256 {
			return fmt.Errorf("authoring admission reference requires bounded identities")
		}
	}
	return nil
}
