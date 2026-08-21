package decision

import (
	"github.com/muidea/workorch/internal/pkg/common"
)

type DefinitionClarificationClass string

const (
	DefinitionClarificationWorkspacePreSeal DefinitionClarificationClass = "workspace_definition_pre_seal"
	DefinitionClarificationRuntime          DefinitionClarificationClass = "runtime_definition"
)

// DefinitionClarificationBinding is Decision-owner durable ticket identity.
// It stores only bounded canonical identity/signature data and never raw LLM
// text or a Runtime continuation action.
type DefinitionClarificationBinding struct {
	Class                       DefinitionClarificationClass
	OpenBindingHash             string
	RequiredActionKind          string
	CandidateID                 string
	CandidateRevision           uint64
	CandidateHash               string
	ViolationOrGapCode          string
	CanonicalSignatureVersion   string
	CanonicalSignature          string
	CanonicalSignatureHash      string
	OffendingRefs               []string
	DefinitionRevision          string
	StartupInputHash            string
	AuthoritySnapshotID         string
	ClarificationDispatchID     string
	FocusRef                    string
	LogicalRequestID            string
	SemanticDocumentHash        string
	ResolutionID                common.ResolutionID
	ResolutionBindingHash       string
	ReplacementIntentID         string
	ReplacementIntentHash       string
	SuccessorDefinitionRevision string
	SuccessorStartupInputHash   string
}

func (b DefinitionClarificationBinding) Clone() *DefinitionClarificationBinding {
	ret := b
	ret.OffendingRefs = append([]string(nil), b.OffendingRefs...)
	return &ret
}
