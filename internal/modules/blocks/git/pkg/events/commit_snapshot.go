package events

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const RuntimeCommitSnapshotQueryProtocol = "git/commit-object-graph-query"
const RuntimeCommitSnapshotInputQueryProtocol = "git/commit-input-membership-query"
const RuntimeCommitSnapshotTerminalQueryProtocol = "git/terminal-commit-snapshot-query"
const RuntimeCommitSnapshotProtocol = "git/commit-object-snapshot/v1"

// This is an original-receipt read, not another execution authorization. Directory
// authority is obtained by Git from Workspace, never from this caller's context.
type QueryRuntimeCommitSnapshotCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	ProtocolRef           string
	RequestID             string
	IssuedAtStateRevision uint64
	SealedOperation       SealedRuntimeOperation
	SourceReceiptHash     string
	SnapshotAddress       runtimecontract.RuntimeEvidenceContentAddress
	OriginalDependencies  runtimecontract.RuntimeOwnerRevisionSet
	// V2 is a bounded byte-membership request, not a review/credit authority.
	// Orchestrator derives this original binding from its accepted semantic batch.
	InputBinding           *runtimecontract.RuntimeValidationProofRequirementBinding `json:",omitempty"`
	ValidationDependencies *runtimecontract.RuntimeOwnerRevisionSet                  `json:",omitempty"`
	// V3 joins the original receipt's exact commit/root identity, not merely
	// two arbitrary IDs that happen to belong to the same archived graph.
	OriginalSnapshot *runtimecontract.RuntimeOwnerSnapshotIdentity `json:",omitempty"`
}

type RuntimeCommitSnapshotInputMembership struct {
	TargetRef   string
	ContentHash string
	BlobID      string
}

type QueryRuntimeCommitSnapshotResult struct {
	ProtocolRef           string
	RequestID             string
	IssuedAtStateRevision uint64
	SourceReceiptHash     string
	SnapshotAddress       runtimecontract.RuntimeEvidenceContentAddress
	SnapshotHead          string
	SnapshotRootTree      string
	ObservedHead          string
	ObjectCount           int
	ObjectBytes           int
	// Present only after a complete fresh graph read enclosed by equal HEAD
	// observations, with the original bytes/dependencies independently matched.
	CurrentDependencies   *runtimecontract.RuntimeOwnerRevisionSet
	InputSubjectHash      string                                 `json:",omitempty"`
	InputMemberships      []RuntimeCommitSnapshotInputMembership `json:",omitempty"`
	WorkspaceClean        *bool                                  `json:",omitempty"`
	ValidationMemberships []RuntimeCommitSnapshotInputMembership `json:",omitempty"`
}

func (c QueryRuntimeCommitSnapshotCommand) Validate() error {
	if (c.ProtocolRef != RuntimeCommitSnapshotQueryProtocol && c.ProtocolRef != RuntimeCommitSnapshotInputQueryProtocol && c.ProtocolRef != RuntimeCommitSnapshotTerminalQueryProtocol) || !commitSnapshotQueryIdentity(c.RequestID) ||
		c.IssuedAtStateRevision == 0 || !commitSnapshotDigest(c.SourceReceiptHash, 64) ||
		c.WorkspaceDeliverableDirectory != "" || c.WorkspaceInternalDirectory != "" {
		return fmt.Errorf("Git snapshot query requires exact bounded original-source identity and no caller directories")
	}
	if c.ProtocolRef == RuntimeCommitSnapshotTerminalQueryProtocol {
		identity := c.OriginalSnapshot
		if identity == nil || identity.Validate() != nil || !commitSnapshotObjectID(identity.SnapshotID) || !commitSnapshotObjectID(identity.RootSnapshotID) || len(identity.SnapshotID) != len(identity.RootSnapshotID) || identity.SnapshotID == identity.RootSnapshotID {
			return fmt.Errorf("Git terminal snapshot requires exact original commit/root identity")
		}
		for _, id := range []string{identity.SnapshotID, identity.RootSnapshotID} {
			matches := 0
			for _, entry := range c.OriginalDependencies.Entries {
				if entry.ResourceID == id {
					matches++
				}
			}
			if matches != 1 {
				return fmt.Errorf("Git terminal snapshot identity is absent or ambiguous in original object graph")
			}
		}
		set := c.ValidationDependencies
		if set == nil || set.DefinitionRevision != c.Transaction.DefinitionRevision || len(set.Entries) == 0 || len(set.Entries) > 1024 || set.Validate() != nil || !reflect.DeepEqual(*set, set.Normalized()) {
			return fmt.Errorf("Git terminal snapshot requires complete original validation read set")
		}
		for _, entry := range set.Entries {
			if entry.OwnerKind != "capability" || !strings.HasSuffix(entry.OwnerInstanceID, "/"+string(c.WorkspaceID)) || entry.ResourceKind != string(runtimecontract.RuntimeResourceWorkspaceTarget) || !commitSnapshotInputPath(entry.ResourceID) || !commitSnapshotDigest(entry.Revision, 64) {
				return fmt.Errorf("Git terminal snapshot supports exact workspace-byte validation reads only")
			}
		}
	} else if c.ValidationDependencies != nil || c.OriginalSnapshot != nil {
		return fmt.Errorf("graph-only Git snapshot cannot carry terminal read set or snapshot identity")
	}
	if c.ProtocolRef == RuntimeCommitSnapshotQueryProtocol {
		if c.InputBinding != nil {
			return fmt.Errorf("Git snapshot V1 cannot carry input membership extension")
		}
	} else {
		binding := c.InputBinding
		if binding == nil || binding.InputReview == nil || len(binding.RequiredCheckRefs) > 512 || len(binding.CheckAttestations) != len(binding.RequiredCheckRefs) || binding.DefinitionRevision != c.Transaction.DefinitionRevision {
			return fmt.Errorf("Git snapshot V2 requires bounded exact input binding")
		}
		for _, value := range []string{binding.RequirementHash, string(binding.CoverageWorkItemID), binding.DeclarationRef, binding.ValidationRecipeID} {
			if !commitSnapshotQueryIdentity(value) {
				return fmt.Errorf("Git snapshot input binding identity is not bounded/canonical")
			}
		}
		for _, check := range binding.CheckAttestations {
			for _, value := range []string{check.RequiredCheckRef, check.ObservationProtocolRef, check.NativeCheckRef} {
				if !commitSnapshotQueryIdentity(value) {
					return fmt.Errorf("Git snapshot input native mapping is not bounded/canonical")
				}
			}
			if check.SupplementalNativeCheckRef != "" && !commitSnapshotQueryIdentity(check.SupplementalNativeCheckRef) {
				return fmt.Errorf("Git snapshot supplemental input mapping is not bounded/canonical")
			}
		}
		if binding.Validate() != nil ||
			!reflect.DeepEqual(*binding, binding.Normalized()) {
			return fmt.Errorf("Git snapshot V2 requires complete exact input binding")
		}
		for _, file := range binding.InputReview.Files {
			if !commitSnapshotInputPath(file.TargetRef) {
				return fmt.Errorf("Git snapshot input requires exact repository-relative path")
			}
		}
	}
	for _, value := range []string{string(c.WorkspaceID), string(c.RunID), c.DispatchID, c.OperationID,
		c.Transaction.DefinitionRevision, c.Transaction.AuthoritySnapshotID, c.Transaction.RequiredOutputHash} {
		if !commitSnapshotQueryIdentity(value) {
			return fmt.Errorf("Git snapshot query requires exact original execution/Definition scope")
		}
	}
	if c.EvidenceKind != runtimecontract.EvidenceKindStateChange {
		return fmt.Errorf("Git snapshot query requires original state-change evidence scope")
	}
	operation, err := c.SealedOperation.ValidateForExecution(c.RuntimeOperationExecutionContext)
	if err != nil {
		return err
	}
	data, err := OperationDataFromRuntimeOperation(operation)
	if err != nil || operation.Kind != runtimecontract.StepKindGitWrite || data.Operation != gitcommon.OperationCommit ||
		c.Execution.ExpectedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		return fmt.Errorf("Git snapshot query requires original sealed durable commit")
	}
	if c.SnapshotAddress.Validate() != nil || !reflect.DeepEqual(c.SnapshotAddress, c.SnapshotAddress.Normalized()) ||
		c.SnapshotAddress.Ref != RuntimeCommitSnapshotProtocol+"/"+c.SnapshotAddress.Digest {
		return fmt.Errorf("Git snapshot query requires exact original archive address")
	}
	return validateCommitSnapshotDependencies(c.RuntimeOperationExecutionContext, &c.OriginalDependencies)
}

func validateCommitSnapshotDependencies(c runtimecontract.RuntimeOperationExecutionContext, set *runtimecontract.RuntimeOwnerRevisionSet) error {
	if set == nil || len(set.Entries) < 2 || len(set.Entries) > 1024 || set.DefinitionRevision != c.Transaction.DefinitionRevision ||
		!reflect.DeepEqual(*set, set.Normalized()) {
		return fmt.Errorf("Git snapshot query requires complete raw-canonical object dependencies")
	}
	if err := set.Validate(); err != nil {
		return err
	}
	for _, entry := range set.Entries {
		if entry.OwnerKind != "capability" || entry.OwnerInstanceID != runtimecontract.RuntimeModuleGit+"/"+string(c.WorkspaceID) ||
			entry.ResourceKind != "git_object" || !commitSnapshotObjectID(entry.ResourceID) || entry.SnapshotID != entry.ResourceID ||
			!commitSnapshotDigest(entry.Revision, 64) || !commitSnapshotDigest(entry.DependencyHash, 64) {
			return fmt.Errorf("Git snapshot query dependency is not an exact Git-owned object")
		}
	}
	return nil
}

func (r QueryRuntimeCommitSnapshotResult) Clone() QueryRuntimeCommitSnapshotResult {
	r.CurrentDependencies = runtimecontract.CloneRuntimeOwnerRevisionSet(r.CurrentDependencies)
	r.InputMemberships = slices.Clone(r.InputMemberships)
	r.ValidationMemberships = slices.Clone(r.ValidationMemberships)
	if r.WorkspaceClean != nil {
		value := *r.WorkspaceClean
		r.WorkspaceClean = &value
	}
	return r
}

func (r QueryRuntimeCommitSnapshotResult) ValidateFor(c QueryRuntimeCommitSnapshotCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if (c.ProtocolRef == RuntimeCommitSnapshotTerminalQueryProtocol) != (r.WorkspaceClean != nil) {
		return fmt.Errorf("Git snapshot cleanliness requires exact V3 observation")
	}
	if c.OriginalSnapshot != nil && (r.SnapshotHead != c.OriginalSnapshot.SnapshotID || r.SnapshotRootTree != c.OriginalSnapshot.RootSnapshotID) {
		return fmt.Errorf("Git terminal snapshot response differs from original receipt commit/root identity")
	}
	if c.ValidationDependencies == nil {
		if r.ValidationMemberships != nil {
			return fmt.Errorf("graph-only Git snapshot cannot claim validation membership")
		}
	} else {
		if len(r.ValidationMemberships) != len(c.ValidationDependencies.Entries) {
			return fmt.Errorf("Git terminal validation membership incomplete")
		}
		for i, entry := range c.ValidationDependencies.Entries {
			member := r.ValidationMemberships[i]
			if member.TargetRef != entry.ResourceID || member.ContentHash != entry.Revision || !commitSnapshotObjectID(member.BlobID) {
				return fmt.Errorf("Git validation member differs from original read dependency")
			}
			matches := 0
			for _, object := range c.OriginalDependencies.Entries {
				if object.ResourceID == member.BlobID && object.Revision == member.ContentHash {
					matches++
				}
			}
			if matches != 1 {
				return fmt.Errorf("Git validation member absent from original byte graph")
			}
		}
	}
	if r.ProtocolRef != c.ProtocolRef || r.RequestID != c.RequestID || r.IssuedAtStateRevision != c.IssuedAtStateRevision ||
		r.SourceReceiptHash != c.SourceReceiptHash || r.SnapshotAddress != c.SnapshotAddress ||
		!commitSnapshotObjectID(r.SnapshotHead) || !commitSnapshotObjectID(r.SnapshotRootTree) || !commitSnapshotObjectID(r.ObservedHead) ||
		len(r.SnapshotHead) != len(r.SnapshotRootTree) || len(r.SnapshotHead) != len(r.ObservedHead) ||
		r.ObjectCount != len(c.OriginalDependencies.Entries) || r.ObjectBytes < 1 || r.ObjectBytes > 4*1024*1024 {
		return fmt.Errorf("Git snapshot query observation identity or complete bounds mismatch")
	}
	for _, id := range []string{r.SnapshotHead, r.SnapshotRootTree} {
		found := false
		for _, entry := range c.OriginalDependencies.Entries {
			found = found || entry.ResourceID == id
		}
		if !found {
			return fmt.Errorf("Git snapshot head/tree absent from original object dependencies")
		}
	}
	if c.InputBinding == nil {
		if r.InputSubjectHash != "" || r.InputMemberships != nil {
			return fmt.Errorf("Git snapshot V1 response cannot claim input membership")
		}
	} else {
		subject := c.InputBinding.InputReview
		if r.InputSubjectHash != subject.Hash || len(r.InputMemberships) != len(subject.Files) {
			return fmt.Errorf("Git snapshot input membership is missing or differs from original subject")
		}
		for i, file := range subject.Files {
			member := r.InputMemberships[i]
			if member.TargetRef != file.TargetRef || member.ContentHash != file.ContentHash || !commitSnapshotObjectID(member.BlobID) || len(member.BlobID) != len(r.SnapshotHead) {
				return fmt.Errorf("Git snapshot input membership identity mismatch")
			}
			found := false
			for _, entry := range c.OriginalDependencies.Entries {
				found = found || (entry.ResourceID == member.BlobID && entry.Revision == file.ContentHash)
			}
			if !found {
				return fmt.Errorf("Git input blob bytes are absent from original object dependencies")
			}
		}
	}
	if r.ObservedHead != r.SnapshotHead {
		if r.CurrentDependencies != nil {
			return fmt.Errorf("changed Git HEAD cannot claim current original snapshot dependencies")
		}
		return nil
	}
	if err := validateCommitSnapshotDependencies(c.RuntimeOperationExecutionContext, r.CurrentDependencies); err != nil {
		return err
	}
	if !reflect.DeepEqual(r.CurrentDependencies, &c.OriginalDependencies) {
		return fmt.Errorf("Git current object dependencies differ from original complete snapshot")
	}
	return nil
}

func commitSnapshotInputPath(value string) bool {
	if !commitSnapshotQueryIdentity(value) || path.IsAbs(value) || path.Clean(value) != value || value == "." || strings.HasPrefix(value, "../") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if strings.EqualFold(component, ".git") {
			return false
		}
	}
	return true
}

func commitSnapshotQueryIdentity(value string) bool {
	return value != "" && len(value) <= 2048 && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func commitSnapshotDigest(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func commitSnapshotObjectID(value string) bool {
	return commitSnapshotDigest(value, 40) || commitSnapshotDigest(value, 64)
}
