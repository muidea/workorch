package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrRuntimeOperationContractRevisionMismatch identifies a persisted Runtime
// aggregate whose executable operation catalog is not the current contract.
var ErrRuntimeOperationContractRevisionMismatch = errors.New("runtime operation contract revision mismatch")

// RuntimeOperationContractRevisionMismatchError preserves both sides of the
// executable-catalog fence. It is deliberately an error rather than a
// default projection: a nonterminal Run must be activated again under an
// explicitly sealed current contract.
type RuntimeOperationContractRevisionMismatchError struct {
	PersistedRevision string
	CurrentRevision   string
}

func (e *RuntimeOperationContractRevisionMismatchError) Error() string {
	if e == nil {
		return ErrRuntimeOperationContractRevisionMismatch.Error()
	}
	return fmt.Sprintf("%s: persisted=%q current=%q", ErrRuntimeOperationContractRevisionMismatch, strings.TrimSpace(e.PersistedRevision), strings.TrimSpace(e.CurrentRevision))
}

func (e *RuntimeOperationContractRevisionMismatchError) Unwrap() error {
	return ErrRuntimeOperationContractRevisionMismatch
}

// ValidateRuntimeOperationContractRevision checks the persisted revision
// against this binary's executable operation catalog. An empty revision is a
// mismatch as well; Runtime accepts only the explicitly sealed current
// operation contract.
func ValidateRuntimeOperationContractRevision(persistedRevision string) error {
	persistedRevision = strings.TrimSpace(persistedRevision)
	currentRevision := RuntimeOperationContractRevision()
	if persistedRevision == currentRevision {
		return nil
	}
	return &RuntimeOperationContractRevisionMismatchError{
		PersistedRevision: persistedRevision,
		CurrentRevision:   currentRevision,
	}
}

// RuntimeOperationContractRevision identifies the executable catalog compiled
// into this binary. It intentionally excludes prose descriptions and Rules:
// only fields consumed by matcher/effect/idempotency/correction authority participate.
func RuntimeOperationContractRevision() string {
	kinds := []string{StepKindFileList, StepKindFileRead, StepKindFileWrite, StepKindFileEdit, RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview, StepKindShellCommand, RuntimeBlockKindManagedValidationSession, StepKindGitRead, StepKindGitWrite, StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet, StepKindSSHCommand, StepKindWebSearch, StepKindWebFetch}
	h := sha256.New()
	for _, kind := range kinds {
		contract, ok := RuntimeOperationContractForKind(kind)
		if !ok {
			continue
		}
		descriptor, ok := RuntimeOperationDescriptorForKind(kind)
		if !ok {
			continue
		}
		writeHashString(h, contract.Kind)
		// Descriptor fields are consumed by reducer feasibility and sealed
		// dispatch admission.  They must advance the executable contract revision
		// together with parameter rules so an already-persisted Run never changes
		// its create/effect semantics after a binary replacement.
		writeHashString(h, descriptor.OwnerModuleID)
		writeHashString(h, string(descriptor.DefaultEffectScope))
		writeHashString(h, string(descriptor.CapabilityClass))
		writeHashString(h, string(descriptor.IdempotencyClass))
		writeHashString(h, fmt.Sprintf("%t", descriptor.RequiresOwnerReceipt))
		writeHashString(h, string(descriptor.TargetCardinality))
		writeHashString(h, string(descriptor.FailureTargetProtocol))
		writeHashString(h, string(descriptor.RepairSurfaceConfirmationProtocol))
		for _, state := range descriptor.AllowedStates {
			writeHashString(h, string(state))
		}
		for _, p := range contract.Parameters {
			writeHashString(h, p.Name)
			writeHashString(h, string(p.Type))
			writeHashString(h, fmt.Sprintf("%t", p.Required))
			writeHashString(h, string(p.CorrectionRole))
		}
		writeHashStrings(h, contract.RequireOneOfParameters)
		writeHashString(h, fmt.Sprintf("%t", contract.RequireTargetRefs))
		writeHashStrings(h, contract.ForbiddenParameters)
		writeHashString(h, runtimeOperationDynamicParameterPolicy(contract.Kind))
		writeHashString(h, runtimeOperationValidationPolicy(contract.Kind))
		for _, v := range contract.Variants {
			writeHashString(h, v.ID)
			for _, d := range v.Discriminators {
				writeHashString(h, d.Name)
				writeHashString(h, d.EqualsString)
			}
			writeHashStrings(h, v.RequiredParameters)
			writeHashStrings(h, v.RequiredNonEmpty)
			writeHashStrings(h, v.ForbiddenParameters)
			for _, pair := range v.DistinctParameterPairs {
				writeHashString(h, pair.Left)
				writeHashString(h, pair.Right)
			}
			for _, constraint := range NormalizeRuntimeOperationCandidateConstraints(v.CandidateConstraints) {
				writeHashString(h, constraint.Ref)
				writeHashString(h, string(constraint.Kind))
				writeHashString(h, constraint.ParameterName)
				writeHashString(h, string(constraint.RequiredShape))
			}
			writeHashString(h, string(v.EffectScope))
			writeHashString(h, string(v.IdempotencyClass))
		}
	}
	return "operation-contract-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeOperationParameterType string

const (
	RuntimeOperationParameterTypeString     RuntimeOperationParameterType = "string"
	RuntimeOperationParameterTypeInteger    RuntimeOperationParameterType = "integer"
	RuntimeOperationParameterTypeBoolean    RuntimeOperationParameterType = "boolean"
	RuntimeOperationParameterTypeStringList RuntimeOperationParameterType = "string_list"
)

type RuntimeOperationParameterContract struct {
	Name           string
	Type           RuntimeOperationParameterType
	Required       bool
	CorrectionRole RuntimeOperationCorrectionRole
	Description    string
}

// RuntimeOperationParameterPredicate selects a concrete operation variant from
// a typed operation parameter. The matcher compares string values
// case-insensitively so provider formatting cannot create a second variant.
type RuntimeOperationParameterPredicate struct {
	Name         string
	EqualsString string
}

// RuntimeOperationParameterPair expresses a generic value relation required by
// a variant. It deliberately contains no domain terminology: concrete owners
// define their own parameter names in their descriptor.
type RuntimeOperationParameterPair struct {
	Left  string
	Right string
}

// RuntimeOperationVariantContract is the single executable authority for a
// discriminated sub-operation. Provider schema rendering, candidate validation,
// dispatch seal and concrete owner preflight must derive from this descriptor.
type RuntimeOperationVariantContract struct {
	ID                     string
	Discriminators         []RuntimeOperationParameterPredicate
	RequiredParameters     []string
	RequiredNonEmpty       []string
	ForbiddenParameters    []string
	DistinctParameterPairs []RuntimeOperationParameterPair
	CandidateConstraints   []RuntimeOperationCandidateConstraint
	EffectScope            RuntimeEffectScope
	IdempotencyClass       RuntimeOperationIdempotencyClass
}

// RuntimeOperationVariantRef is the typed identity used by focus and required
// output contracts. A variant is always scoped by its operation kind.
type RuntimeOperationVariantRef struct {
	Kind      string
	VariantID string
}

// RuntimeOperationVariantRuleID is the stable, descriptor-derived identifier
// shared by provider schema projection and final seal diagnostics. It carries
// no owner/domain policy: kind, variant and generic rule shape are all the
// authority needed to correlate a rejected candidate with its executable
// contract.
func RuntimeOperationVariantRuleID(kind, variantID, rule, parameter string) string {
	parts := []string{
		"operation_variant",
		CanonicalRuntimeBlockKind(kind),
		strings.TrimSpace(variantID),
		strings.TrimSpace(rule),
	}
	if parameter = strings.TrimSpace(parameter); parameter != "" {
		parts = append(parts, parameter)
	}
	return strings.Join(parts, "/")
}

func NormalizeRuntimeOperationVariantRefs(values []RuntimeOperationVariantRef) []RuntimeOperationVariantRef {
	byKey := map[string]RuntimeOperationVariantRef{}
	for _, value := range values {
		value.Kind = CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		if value.Kind != "" && value.VariantID != "" {
			byKey[value.Kind+"|"+value.VariantID] = value
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeOperationVariantRef, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byKey[key])
	}
	return ret
}

func SameRuntimeOperationVariantRefs(left []RuntimeOperationVariantRef, right []RuntimeOperationVariantRef) bool {
	left = NormalizeRuntimeOperationVariantRefs(left)
	right = NormalizeRuntimeOperationVariantRefs(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

type RuntimeOperationContract struct {
	Kind                   string
	Parameters             []RuntimeOperationParameterContract
	RequireOneOfParameters []string
	RequireTargetRefs      bool
	ForbiddenParameters    []string
	Variants               []RuntimeOperationVariantContract
	Rules                  []string
}

func RuntimeOperationContractForKind(kind string) (RuntimeOperationContract, bool) {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindFileList:
		return RuntimeOperationContract{
			Kind: StepKindFileList,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "path", Type: RuntimeOperationParameterTypeString, Description: "workspace-relative directory path; defaults to workspace root"},
				{Name: "recursive", Type: RuntimeOperationParameterTypeBoolean, CorrectionRole: RuntimeOperationCorrectionRoleClosedParameter, Description: "recursively enumerate descendants"},
				{Name: "maxEntries", Type: RuntimeOperationParameterTypeInteger, Description: "bounded maximum entry count"},
			},
			ForbiddenParameters: []string{"input", "output", "payload"},
			Rules:               []string{"maxEntries must be between 1 and 500 when provided"},
		}, true
	case StepKindFileRead:
		return RuntimeOperationContract{
			Kind: StepKindFileRead,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "optional", Type: RuntimeOperationParameterTypeBoolean, CorrectionRole: RuntimeOperationCorrectionRoleClosedParameter, Description: "return exists=false when the target is absent"},
				{Name: "maxBytes", Type: RuntimeOperationParameterTypeInteger, Description: "bounded maximum returned content bytes"},
				{Name: "startLine", Type: RuntimeOperationParameterTypeInteger, Description: "optional 1-based inclusive start line"},
				{Name: "endLine", Type: RuntimeOperationParameterTypeInteger, Description: "optional 1-based inclusive end line"},
			},
			RequireTargetRefs:   true,
			ForbiddenParameters: []string{"input", "output", "payload", "path", "targetRef", "targetRefs"},
			Rules:               []string{"targetRefs must contain exactly one workspace-relative file path", "maxBytes must be between 1 and 65536 when provided", "line bounds must be positive and ordered when provided"},
		}, true
	case StepKindFileWrite:
		return RuntimeOperationContract{
			Kind: StepKindFileWrite,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "content", Type: RuntimeOperationParameterTypeString, Required: true, Description: "complete file content; an empty string is allowed"},
			},
			RequireTargetRefs:   true,
			ForbiddenParameters: []string{"input", "output", "payload", "path", "targetRef", "targetRefs"},
			Rules:               []string{"targetRefs must contain exactly one workspace-relative target", "target must not already exist; replacing content requires patch/apply with an owner-observed baseHash"},
		}, true
	case StepKindFileEdit:
		return RuntimeOperationContract{
			Kind: StepKindFileEdit,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "oldText", Type: RuntimeOperationParameterTypeString, Required: true, Description: "exact current text to replace"},
				{Name: "newText", Type: RuntimeOperationParameterTypeString, Required: true, Description: "replacement text; an empty string is allowed"},
				{Name: "baseHash", Type: RuntimeOperationParameterTypeString, Description: "optional current content hash"},
				{Name: "occurrence", Type: RuntimeOperationParameterTypeInteger, Description: "1-based match index when oldText is not unique"},
			},
			RequireTargetRefs:   true,
			ForbiddenParameters: []string{"filePath", "fileContent", "path", "targetRef", "targetRefs"},
			Rules: []string{
				"oldText must be non-empty and different from newText",
				"targetRefs must contain exactly one workspace-relative target",
			},
		}, true
	case RuntimeBlockKindPatchApply:
		return RuntimeOperationContract{
			Kind: RuntimeBlockKindPatchApply,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "patchType", Type: RuntimeOperationParameterTypeString, Required: true, Description: "file_write, text_replace, or unified_diff"},
				{Name: "content", Type: RuntimeOperationParameterTypeString, Description: "complete file_write content; an empty string is allowed"},
				{Name: "diff", Type: RuntimeOperationParameterTypeString, Description: "unified_diff content"},
				{Name: "oldText", Type: RuntimeOperationParameterTypeString, Description: "exact text_replace source text"},
				{Name: "newText", Type: RuntimeOperationParameterTypeString, Description: "text_replace replacement; an empty string is allowed"},
				{Name: "baseHash", Type: RuntimeOperationParameterTypeString, Description: "owner-observed current content hash; required when file_write replaces an existing target"},
				{Name: "occurrence", Type: RuntimeOperationParameterTypeInteger, Description: "1-based match index when oldText is not unique"},
				{Name: "coverageTargets", Type: RuntimeOperationParameterTypeStringList, Description: "coverage targets addressed by this state change"},
				{Name: "summary", Type: RuntimeOperationParameterTypeString, Description: "bounded change summary"},
			},
			RequireTargetRefs:   true,
			ForbiddenParameters: []string{"filePath", "fileContent", "path", "targetRef", "targetRefs"},
			Variants: []RuntimeOperationVariantContract{
				{ID: "file_write", Discriminators: []RuntimeOperationParameterPredicate{{Name: "patchType", EqualsString: "file_write"}}, RequiredParameters: []string{"content"}, ForbiddenParameters: []string{"diff", "oldText", "newText", "occurrence"}, EffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
				{ID: "text_replace", Discriminators: []RuntimeOperationParameterPredicate{{Name: "patchType", EqualsString: "text_replace"}}, RequiredParameters: []string{"oldText", "newText"}, RequiredNonEmpty: []string{"oldText"}, ForbiddenParameters: []string{"content", "diff"}, DistinctParameterPairs: []RuntimeOperationParameterPair{{Left: "oldText", Right: "newText"}}, EffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
				{ID: "unified_diff", Discriminators: []RuntimeOperationParameterPredicate{{Name: "patchType", EqualsString: "unified_diff"}}, RequiredParameters: []string{"diff"}, RequiredNonEmpty: []string{"diff"}, ForbiddenParameters: []string{"content", "oldText", "newText", "occurrence"}, CandidateConstraints: []RuntimeOperationCandidateConstraint{
					{Ref: "patch/unified_diff/diff/single_file", Kind: RuntimeOperationCandidateConstraintParameterShape, ParameterName: "diff", RequiredShape: RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile},
					{Ref: "patch/unified_diff/diff/target_mode", Kind: RuntimeOperationCandidateConstraintParameterShape, ParameterName: "diff", RequiredShape: RuntimeOperationCandidateRequiredShapeUnifiedDiffAuthorizedTargetMode},
					{Ref: "patch/unified_diff/diff/observed_target", Kind: RuntimeOperationCandidateConstraintTargetApplicability, ParameterName: "diff", RequiredShape: RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget},
				}, EffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
			},
			Rules: []string{
				"file_write must provide content",
				"file_write replacing an existing target must provide the owner-observed baseHash",
				"text_replace must provide non-empty oldText and a distinct newText",
				"unified_diff must provide non-empty diff",
				"unified_diff must apply to the current owner-observed target content",
				"targetRefs must contain exactly one workspace-relative target",
			},
		}, true
	case RuntimeBlockKindPatchPreview:
		contract, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
		contract.Kind = RuntimeBlockKindPatchPreview
		for idx := range contract.Variants {
			contract.Variants[idx].EffectScope = RuntimeEffectNone
		}
		return contract, true
	case StepKindShellCommand:
		return RuntimeOperationContract{
			Kind: StepKindShellCommand,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "command", Type: RuntimeOperationParameterTypeString, Required: true, Description: "executable or shell binary"},
				{Name: "args", Type: RuntimeOperationParameterTypeStringList, Description: "ordered command arguments"},
				{Name: "timeoutMillis", Type: RuntimeOperationParameterTypeInteger, Description: "bounded execution timeout in milliseconds"},
				{Name: "stateChanging", Type: RuntimeOperationParameterTypeBoolean, Required: true, Description: "must be true; non-mutating validation uses validation/managed_session"},
				{Name: "verificationScope", Type: RuntimeOperationParameterTypeString, Description: "bounded verification scope"},
				{Name: "boundedReason", Type: RuntimeOperationParameterTypeString, Description: "reason the command is bounded"},
			},
			ForbiddenParameters: []string{"input", "output", "payload", "targetRef", "targetRefs"},
			Rules: []string{
				"command must be non-empty",
				"timeoutMillis must be between 1000 and 600000 when provided",
				"stateChanging must be true and targetRefs must identify at least one workspace target",
			},
		}, true
	case RuntimeBlockKindManagedValidationSession:
		return RuntimeOperationContract{
			Kind: RuntimeBlockKindManagedValidationSession,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "recipeId", Type: RuntimeOperationParameterTypeString, Required: true, CorrectionRole: RuntimeOperationCorrectionRoleValidationRecipe, Description: "activation-sealed Shell validation recipe identity"},
			},
			ForbiddenParameters: []string{"input", "output", "payload", "command", "args", "timeoutMillis", "stateChanging", "targetRef", "targetRefs", "verificationScope", "boundedReason", "validationScratchDir", "validationScratchAutoCleanup"},
			Rules:               []string{"recipeId must select an activation-sealed owner-confirmed validation recipe"},
		}, true
	case StepKindGitRead, StepKindGitWrite:
		contract := RuntimeOperationContract{
			Kind: CanonicalRuntimeBlockKind(kind),
			Parameters: []RuntimeOperationParameterContract{
				{Name: "operation", Type: RuntimeOperationParameterTypeString, Required: true, Description: "git sub-operation"},
				{Name: "timeoutMillis", Type: RuntimeOperationParameterTypeInteger, Description: "bounded execution timeout in milliseconds"},
				{Name: "pathspecs", Type: RuntimeOperationParameterTypeStringList, Description: "workspace-relative git pathspecs"},
				{Name: "ref", Type: RuntimeOperationParameterTypeString, Description: "git reference"},
				{Name: "baseRef", Type: RuntimeOperationParameterTypeString, Description: "base git reference"},
				{Name: "limit", Type: RuntimeOperationParameterTypeInteger, Description: "bounded log result limit"},
				{Name: "message", Type: RuntimeOperationParameterTypeString, Description: "commit message"},
				{Name: "expectedHead", Type: RuntimeOperationParameterTypeString, Description: "optional optimistic HEAD value"},
			},
			ForbiddenParameters: []string{"command", "args", "input", "payload"},
			Rules: []string{
				"git/read accepts status, diff, log, show, or branch",
				"git/write accepts init, add, or commit",
				"add requires pathspecs and commit requires message",
			},
		}
		if contract.Kind == StepKindGitRead {
			contract.Variants = []RuntimeOperationVariantContract{
				{ID: "status", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "status"}}, ForbiddenParameters: []string{"ref", "baseRef", "limit", "message", "expectedHead"}, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
				{ID: "diff", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "diff"}}, ForbiddenParameters: []string{"limit", "message", "expectedHead"}, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
				{ID: "log", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "log"}}, ForbiddenParameters: []string{"baseRef", "message", "expectedHead"}, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
				{ID: "show", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "show"}}, ForbiddenParameters: []string{"baseRef", "limit", "message", "expectedHead"}, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
				{ID: "branch", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "branch"}}, ForbiddenParameters: []string{"pathspecs", "ref", "baseRef", "limit", "message", "expectedHead"}, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
			}
		} else {
			contract.Variants = []RuntimeOperationVariantContract{
				{ID: "init", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "init"}}, ForbiddenParameters: []string{"pathspecs", "message", "ref", "baseRef", "limit", "expectedHead"}, EffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyOwnerManaged},
				{ID: "add", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "add"}}, RequiredParameters: []string{"pathspecs"}, RequiredNonEmpty: []string{"pathspecs"}, ForbiddenParameters: []string{"message", "ref", "baseRef", "limit", "expectedHead"}, EffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
				{ID: "commit", Discriminators: []RuntimeOperationParameterPredicate{{Name: "operation", EqualsString: "commit"}}, RequiredParameters: []string{"message"}, RequiredNonEmpty: []string{"message"}, ForbiddenParameters: []string{"pathspecs", "ref", "baseRef", "limit"}, EffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce},
			}
		}
		return contract, true
	case StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet:
		return RuntimeOperationContract{
			Kind: CanonicalRuntimeBlockKind(kind),
			Parameters: []RuntimeOperationParameterContract{
				{Name: "serverId", Type: RuntimeOperationParameterTypeString, Required: true, Description: "configured MCP server identity"},
				{Name: "tool", Type: RuntimeOperationParameterTypeString, Description: "tool name for mcp/tool_call"},
				{Name: "uri", Type: RuntimeOperationParameterTypeString, Description: "resource URI for mcp/resource_read"},
				{Name: "resourceUri", Type: RuntimeOperationParameterTypeString, Description: "resource URI alias"},
				{Name: "prompt", Type: RuntimeOperationParameterTypeString, Description: "prompt name for mcp/prompt_get"},
				{Name: "name", Type: RuntimeOperationParameterTypeString, Description: "prompt name alias"},
				{Name: "summary", Type: RuntimeOperationParameterTypeString, Description: "bounded operation summary"},
				{Name: "stream", Type: RuntimeOperationParameterTypeBoolean, CorrectionRole: RuntimeOperationCorrectionRoleClosedParameter, Description: "requests streaming external protocol handling"},
			},
			ForbiddenParameters: []string{"input", "arguments", "payload"},
			Rules: []string{
				"tool calls require tool and may use scalar input.<name> fields",
				"resource reads require uri or resourceUri",
				"prompt gets require prompt or name and may use scalar arguments.<name> fields",
			},
		}, true
	case StepKindSSHCommand:
		return RuntimeOperationContract{
			Kind: StepKindSSHCommand,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "sessionRef", Type: RuntimeOperationParameterTypeString, Required: true, Description: "opaque SSH owner session identity selected from an ssh-binding constraint fact"},
				{Name: "credentialRef", Type: RuntimeOperationParameterTypeString, Required: true, Description: "opaque SSH owner credential identity selected from the same ssh-binding constraint fact"},
				{Name: "command", Type: RuntimeOperationParameterTypeString, Required: true, Description: "bounded remote command"},
				{Name: "timeoutMillis", Type: RuntimeOperationParameterTypeInteger, Description: "bounded timeout in milliseconds"},
			},
			ForbiddenParameters: []string{"args", "host", "input", "password", "payload", "port", "privateKey", "privateKeyPath", "username"},
			Rules:               []string{"targetRefs carries one opaque owner target ref", "targetRef/sessionRef/credentialRef must match one owner-published ssh-binding/<targetRef>/<sessionRef>/<credentialRef> constraint", "command is bounded to 16384 bytes", "timeoutMillis must be between 1000 and 300000"},
		}, true
	case StepKindWebSearch:
		return RuntimeOperationContract{
			Kind: StepKindWebSearch,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "providerId", Type: RuntimeOperationParameterTypeString, Description: "configured provider identity"},
				{Name: "query", Type: RuntimeOperationParameterTypeString, Required: true, Description: "search query"},
				{Name: "locale", Type: RuntimeOperationParameterTypeString, Description: "search locale"},
				{Name: "recency", Type: RuntimeOperationParameterTypeString, Description: "search recency filter"},
				{Name: "domains", Type: RuntimeOperationParameterTypeStringList, Description: "included domains"},
				{Name: "excludeDomains", Type: RuntimeOperationParameterTypeStringList, Description: "excluded domains"},
				{Name: "maxResults", Type: RuntimeOperationParameterTypeInteger, Description: "bounded search result count"},
				{Name: "safeSearch", Type: RuntimeOperationParameterTypeBoolean, CorrectionRole: RuntimeOperationCorrectionRoleClosedParameter, Description: "safe search policy"},
			},
			ForbiddenParameters: []string{"approvalId", "extractMode", "input", "maxBytes", "payload", "result", "url"},
			Rules: []string{
				"query is bounded to 4096 bytes",
				"provider/filter metadata and result count are owner-bounded",
				"governance authorization is carried only by the sealed execution grant",
			},
		}, true
	case StepKindWebFetch:
		return RuntimeOperationContract{
			Kind: StepKindWebFetch,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "providerId", Type: RuntimeOperationParameterTypeString, Description: "configured policy identity"},
				{Name: "url", Type: RuntimeOperationParameterTypeString, Required: true, Description: "fetch URL"},
				{Name: "maxBytes", Type: RuntimeOperationParameterTypeInteger, Description: "bounded fetch bytes"},
				{Name: "extractMode", Type: RuntimeOperationParameterTypeString, Description: "bounded owner extraction mode"},
			},
			ForbiddenParameters: []string{"approvalId", "domains", "excludeDomains", "input", "locale", "maxResults", "payload", "query", "recency", "result", "safeSearch"},
			Rules: []string{
				"url is bounded to 4096 bytes and must pass owner network preflight",
				"provider/extraction metadata and response bytes are owner-bounded",
				"governance authorization is carried only by the sealed execution grant",
			},
		}, true
	default:
		return RuntimeOperationContract{}, false
	}
}

func ValidateRuntimeOperationContract(operation RuntimeOperation) error {
	operation = operation.Normalized()
	operation.Kind = CanonicalRuntimeBlockKind(operation.Kind)
	contract, known := RuntimeOperationContractForKind(operation.Kind)
	if !known {
		return nil
	}
	if descriptor, found := RuntimeOperationDescriptorForKind(operation.Kind); found {
		if err := validateRuntimeOperationTargetCardinality(operation, descriptor.TargetCardinality); err != nil {
			return err
		}
	}
	allowed := make(map[string]RuntimeOperationParameterContract, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		allowed[parameter.Name] = parameter
	}
	values := make(map[string]OperationParameter, len(operation.Parameters))
	for _, parameter := range operation.Parameters {
		definition, ok := allowed[parameter.Name]
		if !ok && isMCPDynamicOperationParameter(operation.Kind, parameter.Name) {
			if !isMCPDynamicOperationParameterScalar(parameter) {
				return fmt.Errorf("kind %q dynamic parameter %q requires exactly one scalar string, integer, or boolean value", operation.Kind, parameter.Name)
			}
			values[parameter.Name] = parameter
			continue
		}
		if !ok {
			return fmt.Errorf("%s", invalidRuntimeOperationParameterError(operation.Kind, parameter.Name, contract))
		}
		if !operationParameterMatchesType(parameter, definition.Type) {
			return fmt.Errorf("kind %q parameter %q requires %s", operation.Kind, parameter.Name, definition.Type)
		}
		values[parameter.Name] = parameter
	}
	for _, parameter := range contract.Parameters {
		if parameter.Required {
			if _, ok := values[parameter.Name]; !ok {
				return fmt.Errorf("kind %q requires parameter %q", operation.Kind, parameter.Name)
			}
		}
	}
	if len(contract.Variants) > 0 {
		variant, err := MatchRuntimeOperationVariant(operation, contract)
		if err != nil {
			return err
		}
		if err := ValidateRuntimeOperationVariant(operation, variant); err != nil {
			return err
		}
	}
	switch operation.Kind {
	case StepKindFileList:
		if limit, ok := parameterInteger(values["maxEntries"]); ok && (limit <= 0 || limit > 500) {
			return fmt.Errorf("kind %q maxEntries must be between 1 and 500", operation.Kind)
		}
	case StepKindFileRead:
		if limit, ok := parameterInteger(values["maxBytes"]); ok && (limit <= 0 || limit > 65536) {
			return fmt.Errorf("kind %q maxBytes must be between 1 and 65536", operation.Kind)
		}
		start, hasStart := parameterInteger(values["startLine"])
		end, hasEnd := parameterInteger(values["endLine"])
		if (hasStart && start <= 0) || (hasEnd && end <= 0) || (hasStart && hasEnd && end < start) {
			return fmt.Errorf("kind %q requires positive ordered line bounds", operation.Kind)
		}
	case StepKindFileWrite:
	case StepKindFileEdit:
		oldText := parameterRawString(values["oldText"])
		newText := parameterRawString(values["newText"])
		if oldText == "" || oldText == newText {
			return fmt.Errorf("kind %q requires non-empty oldText with a distinct newText", operation.Kind)
		}
		if occurrence, ok := parameterInteger(values["occurrence"]); ok && occurrence <= 0 {
			return fmt.Errorf("kind %q occurrence must be positive", operation.Kind)
		}
	case RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview:
		if occurrence, ok := parameterInteger(values["occurrence"]); ok && occurrence <= 0 {
			return fmt.Errorf("kind %q occurrence must be positive", operation.Kind)
		}
	case StepKindShellCommand:
		if parameterString(values["command"]) == "" {
			return fmt.Errorf("kind %q requires a non-empty command", operation.Kind)
		}
		if timeout, ok := parameterInteger(values["timeoutMillis"]); ok && (timeout < 1000 || timeout > 600000) {
			return fmt.Errorf("kind %q timeoutMillis must be between 1000 and 600000", operation.Kind)
		}
		stateChanging, present := parameterBoolean(values["stateChanging"])
		if !present || !stateChanging {
			return fmt.Errorf("kind %q requires stateChanging=true; use managed validation for non-mutating execution", operation.Kind)
		}
		if len(operation.TargetRefs) == 0 {
			return fmt.Errorf("kind %q state-changing command requires target refs", operation.Kind)
		}
	case RuntimeBlockKindManagedValidationSession:
		if parameterString(values["recipeId"]) == "" {
			return fmt.Errorf("kind %q requires a non-empty recipeId", operation.Kind)
		}
	case StepKindGitRead, StepKindGitWrite:
		if timeout, ok := parameterInteger(values["timeoutMillis"]); ok && (timeout < 1000 || timeout > 600000) {
			return fmt.Errorf("kind %q timeoutMillis must be between 1000 and 600000", operation.Kind)
		}
		if limit, ok := parameterInteger(values["limit"]); ok && limit <= 0 {
			return fmt.Errorf("kind %q limit must be positive", operation.Kind)
		}
	case StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet:
		if parameterString(values["serverId"]) == "" {
			return fmt.Errorf("kind %q requires serverId", operation.Kind)
		}
		if err := validateRuntimeMCPParameters(operation, values); err != nil {
			return err
		}
		switch operation.Kind {
		case StepKindMCPToolCall:
			if parameterString(values["tool"]) == "" {
				return fmt.Errorf("kind %q requires tool", operation.Kind)
			}
		case StepKindMCPResourceRead:
			if parameterString(values["uri"]) == "" && parameterString(values["resourceUri"]) == "" {
				return fmt.Errorf("kind %q requires uri", operation.Kind)
			}
		case StepKindMCPPromptGet:
			if parameterString(values["prompt"]) == "" && parameterString(values["name"]) == "" {
				return fmt.Errorf("kind %q requires prompt", operation.Kind)
			}
		}
	case StepKindSSHCommand:
		sessionRef := parameterString(values["sessionRef"])
		credentialRef := parameterString(values["credentialRef"])
		command := parameterString(values["command"])
		if !validRuntimeOpaqueSSHRef(sessionRef) || !validRuntimeOpaqueSSHRef(credentialRef) || len(command) == 0 || len(command) > 16*1024 || !validRuntimeOpaqueSSHRef(operation.TargetRefs[0]) {
			return fmt.Errorf("kind %q requires canonical owner refs and a bounded command", operation.Kind)
		}
		if timeout, ok := parameterInteger(values["timeoutMillis"]); ok && (timeout < 1000 || timeout > 300000) {
			return fmt.Errorf("kind %q timeoutMillis must be between 1000 and 300000", operation.Kind)
		}
	case StepKindWebSearch:
		if query := parameterString(values["query"]); query == "" || len(query) > 4096 {
			return fmt.Errorf("kind %q requires a query of at most 4096 bytes", operation.Kind)
		}
		if err := validateRuntimeWebCommonParameters(operation.Kind, values); err != nil {
			return err
		}
		if limit, ok := parameterInteger(values["maxResults"]); ok && (limit <= 0 || limit > 20) {
			return fmt.Errorf("kind %q maxResults must be between 1 and 20", operation.Kind)
		}
	case StepKindWebFetch:
		if rawURL := parameterString(values["url"]); rawURL == "" || len(rawURL) > 4096 {
			return fmt.Errorf("kind %q requires a URL of at most 4096 bytes", operation.Kind)
		}
		if err := validateRuntimeWebCommonParameters(operation.Kind, values); err != nil {
			return err
		}
		if limit, ok := parameterInteger(values["maxBytes"]); ok && (limit <= 0 || limit > 1024*1024) {
			return fmt.Errorf("kind %q maxBytes must be between 1 and 1048576", operation.Kind)
		}
	}
	return nil
}

func validRuntimeOpaqueSSHRef(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 64 || value[0] == '-' {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || (index > 0 && (character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return true
}

func validateRuntimeOperationTargetCardinality(operation RuntimeOperation, cardinality RuntimeOperationTargetCardinality) error {
	switch cardinality {
	case "", RuntimeOperationTargetCardinalityZero:
		if cardinality == RuntimeOperationTargetCardinalityZero && len(operation.TargetRefs) != 0 {
			return fmt.Errorf("kind %q does not accept targetRefs", operation.Kind)
		}
	case RuntimeOperationTargetCardinalityOne:
		if len(operation.TargetRefs) != 1 || strings.TrimSpace(operation.TargetRefs[0]) == "" {
			return fmt.Errorf("kind %q requires exactly one targetRefs entry", operation.Kind)
		}
	case RuntimeOperationTargetCardinalityMany:
		// many permits zero for non-mutating shell/session operations. The
		// state-changing branch below requires one-or-more outer targets.
	default:
		return fmt.Errorf("kind %q has an invalid target cardinality %q", operation.Kind, cardinality)
	}
	return nil
}

// MatchRuntimeOperationVariant returns the one descriptor variant selected by a
// concrete operation. Kinds without variants intentionally return an empty
// variant and nil error so existing non-discriminated descriptors remain valid.
func MatchRuntimeOperationVariant(operation RuntimeOperation, contract RuntimeOperationContract) (RuntimeOperationVariantContract, error) {
	operation = operation.Normalized()
	operation.Kind = CanonicalRuntimeBlockKind(operation.Kind)
	if operation.Kind == "" || CanonicalRuntimeBlockKind(contract.Kind) != operation.Kind {
		return RuntimeOperationVariantContract{}, fmt.Errorf("operation kind %q does not match variant contract kind %q", operation.Kind, contract.Kind)
	}
	if len(contract.Variants) == 0 {
		return RuntimeOperationVariantContract{}, nil
	}
	values := operationParameterValues(operation.Parameters)
	matches := make([]RuntimeOperationVariantContract, 0, 1)
	for _, variant := range contract.Variants {
		if runtimeOperationVariantMatches(values, variant) {
			matches = append(matches, variant)
		}
	}
	if len(matches) == 0 {
		return RuntimeOperationVariantContract{}, fmt.Errorf("operation variant rule %q: kind %q does not match an operation variant", RuntimeOperationVariantRuleID(operation.Kind, "match", "no_match", ""), operation.Kind)
	}
	if len(matches) != 1 {
		return RuntimeOperationVariantContract{}, fmt.Errorf("operation variant rule %q: kind %q matches multiple operation variants", RuntimeOperationVariantRuleID(operation.Kind, "match", "multiple", ""), operation.Kind)
	}
	return matches[0], nil
}

// ValidateRuntimeOperationVariant validates the generic executable rules of a
// matched variant. Domain-specific execution still belongs to the concrete
// owner, but parameter shape rules must not be duplicated there.
func ValidateRuntimeOperationVariant(operation RuntimeOperation, variant RuntimeOperationVariantContract) error {
	operation = operation.Normalized()
	values := operationParameterValues(operation.Parameters)
	variantID := strings.TrimSpace(variant.ID)
	if variantID == "" {
		return nil
	}
	for _, name := range variant.RequiredParameters {
		name = strings.TrimSpace(name)
		if _, ok := values[name]; !ok {
			return fmt.Errorf("operation variant rule %q: kind %q variant %q requires parameter %q", RuntimeOperationVariantRuleID(operation.Kind, variantID, "required", name), operation.Kind, variantID, name)
		}
	}
	for _, name := range variant.RequiredNonEmpty {
		name = strings.TrimSpace(name)
		value, ok := values[name]
		if !ok || !operationParameterHasValue(value) {
			return fmt.Errorf("operation variant rule %q: kind %q variant %q requires non-empty parameter %q", RuntimeOperationVariantRuleID(operation.Kind, variantID, "required_non_empty", name), operation.Kind, variantID, name)
		}
	}
	for _, name := range variant.ForbiddenParameters {
		name = strings.TrimSpace(name)
		if _, ok := values[name]; ok {
			return fmt.Errorf("operation variant rule %q: kind %q variant %q forbids parameter %q", RuntimeOperationVariantRuleID(operation.Kind, variantID, "forbidden", name), operation.Kind, variantID, name)
		}
	}
	for _, pair := range variant.DistinctParameterPairs {
		left, leftOK := values[strings.TrimSpace(pair.Left)]
		right, rightOK := values[strings.TrimSpace(pair.Right)]
		if leftOK && rightOK && operationParameterRawValue(left) == operationParameterRawValue(right) {
			return fmt.Errorf("operation variant rule %q: kind %q variant %q requires distinct parameters %q and %q", RuntimeOperationVariantRuleID(operation.Kind, variantID, "distinct", strings.TrimSpace(pair.Left)+"-"+strings.TrimSpace(pair.Right)), operation.Kind, variantID, pair.Left, pair.Right)
		}
	}
	return nil
}

func operationParameterValues(parameters []OperationParameter) map[string]OperationParameter {
	values := make(map[string]OperationParameter, len(parameters))
	for _, parameter := range parameters {
		values[strings.TrimSpace(parameter.Name)] = parameter
	}
	return values
}

func runtimeOperationVariantMatches(values map[string]OperationParameter, variant RuntimeOperationVariantContract) bool {
	if strings.TrimSpace(variant.ID) == "" || len(variant.Discriminators) == 0 {
		return false
	}
	for _, predicate := range variant.Discriminators {
		value, ok := values[strings.TrimSpace(predicate.Name)]
		if !ok || value.String == nil || !strings.EqualFold(strings.TrimSpace(*value.String), strings.TrimSpace(predicate.EqualsString)) {
			return false
		}
	}
	return true
}

func operationParameterHasValue(value OperationParameter) bool {
	switch {
	case value.String != nil:
		return strings.TrimSpace(*value.String) != ""
	case value.Integer != nil, value.Boolean != nil:
		return true
	default:
		return len(value.StringList) > 0
	}
}

func operationParameterRawValue(value OperationParameter) string {
	switch {
	case value.String != nil:
		return *value.String
	case value.Integer != nil:
		return fmt.Sprintf("%d", *value.Integer)
	case value.Boolean != nil:
		return fmt.Sprintf("%t", *value.Boolean)
	default:
		return strings.Join(value.StringList, "\x00")
	}
}

// RuntimeOperationRequiresCurrentTargetSnapshot reports whether an operation's
// technical contract depends on exact current target content. Repair planning
// uses it to reacquire stale targets after a conflict; it does not encode
// business or profile semantics.
func RuntimeOperationRequiresCurrentTargetSnapshot(operation RuntimeOperation) bool {
	operation = operation.Normalized()
	switch operation.Kind {
	case StepKindFileEdit:
		return true
	case RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview:
		patchType := operationStringParameter(operation.Parameters, "patchType")
		return patchType == "text_replace" || patchType == "unified_diff"
	default:
		return false
	}
}

func invalidRuntimeOperationParameterError(kind string, name string, contract RuntimeOperationContract) string {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	allowedNames := make([]string, 0, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		allowedNames = append(allowedNames, parameter.Name)
	}
	message := fmt.Sprintf("kind %q does not accept parameter %q", kind, name)
	switch kind {
	case StepKindMCPToolCall:
		suggestion := "input." + name
		if strings.HasPrefix(name, "input.") {
			suggestion = "a declared tool field or a different input.<name> key"
		}
		message += fmt.Sprintf("; for mcp/tool_call use fixed parameters %s or dynamic input.<name> fields (example: %q)", strings.Join(allowedNames, ", "), suggestion)
	case StepKindMCPPromptGet:
		suggestion := "arguments." + name
		if strings.HasPrefix(name, "arguments.") {
			suggestion = "a declared prompt field or a different arguments.<name> key"
		}
		message += fmt.Sprintf("; for mcp/prompt_get use fixed parameters %s or dynamic arguments.<name> fields (example: %q)", strings.Join(allowedNames, ", "), suggestion)
	case StepKindMCPResourceRead:
		message += fmt.Sprintf("; for mcp/resource_read use fixed parameters %s", strings.Join(allowedNames, ", "))
	default:
		if len(allowedNames) > 0 {
			message += fmt.Sprintf("; allowed parameters: %s", strings.Join(allowedNames, ", "))
		}
	}
	if len(contract.ForbiddenParameters) > 0 {
		message += fmt.Sprintf("; forbidden parameters: %s", strings.Join(contract.ForbiddenParameters, ", "))
	}
	return message
}

func isMCPDynamicOperationParameter(kind string, name string) bool {
	switch kind {
	case StepKindMCPToolCall:
		return strings.HasPrefix(name, "input.") && strings.TrimSpace(strings.TrimPrefix(name, "input.")) != ""
	case StepKindMCPPromptGet:
		return strings.HasPrefix(name, "arguments.") && strings.TrimSpace(strings.TrimPrefix(name, "arguments.")) != ""
	default:
		return false
	}
}

// runtimeOperationDynamicParameterPolicy is included in the executable
// operation-catalog revision. It makes a change to dynamic MCP parameter
// semantics invalidate a contract-mismatched sealed Runtime activation instead of silently
// applying a different contract to an already sealed Run.
func runtimeOperationDynamicParameterPolicy(kind string) string {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindMCPToolCall:
		return "input.scalar"
	case StepKindMCPPromptGet:
		return "arguments.scalar"
	default:
		return "none"
	}
}

// runtimeOperationValidationPolicy is part of the executable operation
// revision. Unlike RuntimeOperationContract.Rules, this value represents
// limits actively enforced by ValidateRuntimeOperationContract.
func runtimeOperationValidationPolicy(kind string) string {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet:
		return "mcp.v2:fixed=4096,dynamic=64x16384,dynamic-name=128,total-string=131072,sealed-owner-catalog"
	case StepKindWebSearch, StepKindWebFetch:
		return "web.v2:provider=128,query=4096,url=4096,metadata=64,domains=32x253,max-results=20,max-bytes=1048576,governance=execution-grant-only"
	default:
		return "default"
	}
}

func validateRuntimeMCPParameters(operation RuntimeOperation, values map[string]OperationParameter) error {
	const (
		maxDynamicCount = 64
		maxDynamicName  = 128
		maxDynamicValue = 16 * 1024
		maxTotalStrings = 128 * 1024
	)
	if len(operation.Parameters) > maxDynamicCount+8 {
		return fmt.Errorf("kind %q exceeds the bounded MCP parameter count", operation.Kind)
	}
	fixedLimits := map[string]int{"serverId": 256, "tool": 256, "prompt": 256, "name": 256, "uri": 4096, "resourceUri": 4096, "summary": 4096}
	totalStrings := 0
	for name, limit := range fixedLimits {
		if value := parameterRawString(values[name]); len(value) > limit {
			return fmt.Errorf("kind %q parameter %q exceeds %d bytes", operation.Kind, name, limit)
		} else {
			totalStrings += len(value)
		}
	}
	dynamicCount := 0
	for _, parameter := range operation.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if !isMCPDynamicOperationParameter(operation.Kind, name) {
			continue
		}
		dynamicCount++
		var suffix string
		if operation.Kind == StepKindMCPToolCall {
			suffix = strings.TrimSpace(strings.TrimPrefix(name, "input."))
		} else {
			suffix = strings.TrimSpace(strings.TrimPrefix(name, "arguments."))
		}
		if suffix == "" || len(suffix) > maxDynamicName {
			return fmt.Errorf("kind %q dynamic parameter %q has an invalid bounded name", operation.Kind, name)
		}
		if parameter.String != nil {
			if len(*parameter.String) > maxDynamicValue {
				return fmt.Errorf("kind %q dynamic parameter %q exceeds %d bytes", operation.Kind, name, maxDynamicValue)
			}
			totalStrings += len(*parameter.String)
		}
	}
	if dynamicCount > maxDynamicCount || totalStrings > maxTotalStrings {
		return fmt.Errorf("kind %q exceeds bounded MCP input budget", operation.Kind)
	}
	return nil
}

func validateRuntimeWebCommonParameters(kind string, values map[string]OperationParameter) error {
	for _, field := range []string{"providerId", "locale", "recency", "extractMode"} {
		value := parameterString(values[field])
		limit := 64
		if field == "providerId" {
			limit = 128
		}
		if len(value) > limit {
			return fmt.Errorf("kind %q parameter %q exceeds %d bytes", kind, field, limit)
		}
	}
	for _, field := range []string{"domains", "excludeDomains"} {
		items := values[field].StringList
		if len(items) > 32 {
			return fmt.Errorf("kind %q parameter %q exceeds 32 entries", kind, field)
		}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" || len(item) > 253 {
				return fmt.Errorf("kind %q parameter %q requires non-empty entries of at most 253 bytes", kind, field)
			}
		}
	}
	return nil
}

// isMCPDynamicOperationParameterScalar keeps the pure operation contract in
// lockstep with the provider-facing closed schema. Dynamic MCP tool/prompt
// fields are intentionally scalar: structured payloads belong to the owner
// protocol, not the generic Runtime candidate DTO.
func isMCPDynamicOperationParameterScalar(parameter OperationParameter) bool {
	valueCount := 0
	if parameter.String != nil {
		valueCount++
	}
	if parameter.Integer != nil {
		valueCount++
	}
	if parameter.Boolean != nil {
		valueCount++
	}
	if len(parameter.StringList) > 0 {
		return false
	}
	return valueCount == 1
}

func operationParameterMatchesType(parameter OperationParameter, expected RuntimeOperationParameterType) bool {
	switch expected {
	case RuntimeOperationParameterTypeString:
		return parameter.String != nil && parameter.Integer == nil && parameter.Boolean == nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeInteger:
		return parameter.String == nil && parameter.Integer != nil && parameter.Boolean == nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeBoolean:
		return parameter.String == nil && parameter.Integer == nil && parameter.Boolean != nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeStringList:
		return parameter.String == nil && parameter.Integer == nil && parameter.Boolean == nil && len(parameter.StringList) > 0
	default:
		return false
	}
}

func parameterString(parameter OperationParameter) string {
	if parameter.String == nil {
		return ""
	}
	return strings.TrimSpace(*parameter.String)
}

func parameterRawString(parameter OperationParameter) string {
	if parameter.String == nil {
		return ""
	}
	return *parameter.String
}

func parameterInteger(parameter OperationParameter) (int64, bool) {
	if parameter.Integer == nil {
		return 0, false
	}
	return *parameter.Integer, true
}

func parameterBoolean(parameter OperationParameter) (bool, bool) {
	if parameter.Boolean == nil {
		return false, false
	}
	return *parameter.Boolean, true
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
