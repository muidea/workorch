package workorchruntimecontract

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

const RuntimeSourceObligationInventorySchemaCurrent = "workorch.runtime_source_obligation_inventory/v1"

// RuntimeSourceObligationInventory 是 exact Workspace candidate 的不可变来源投影。
// 它没有 target、recipe、binding 或闭包状态；存在映射不代表已经取得证明。
type RuntimeSourceObligationInventory struct {
	SchemaVersion           string
	Hash                    string
	CandidateHash           string
	SpecificationHash       string
	DefinitionRevision      string
	StartupInputHash        string
	Sources                 []RuntimeDefinitionSource
	Resources               []RuntimeSourceResource
	Requirements            []RuntimeSourceRequirement
	Obligations             []RuntimeSourceObligation
	Verifications           []RuntimeSourceVerification
	ExecutionProofMappings  []RuntimeExecutionProofMapping  `json:",omitempty"`
	EvidencePurposeMappings []RuntimeEvidencePurposeMapping `json:",omitempty"`
}

type RuntimeExecutionProofMapping struct {
	ContractRef      string
	ConsumerItemRef  string
	RuleRef          string
	SourceItemRef    string
	ConsumerGoalText string `json:",omitempty"`
}

type RuntimeDefinitionSource struct {
	CandidateItemRef string
	ItemRef          string
	SourceScope      string
	SourceID         string
	Text             string
}

type RuntimeSourceResource struct {
	ID           string
	Ref          string
	SnapshotHash string
	Required     bool
	Available    bool
}

type RuntimeSourceRequirement struct {
	ID             string
	DimensionID    string
	Required       bool
	SourceKind     string
	SourceItemRefs []string
	ResourceIDs    []string
	Text           string
}

type RuntimeSourceObligation struct {
	ID            string
	RequirementID string
	DoDItemRefs   []string
	Text          string
}

type RuntimeSourceVerification struct {
	ID                    string
	ObligationID          string
	Declaration           string
	ProfileID             string
	EvidenceRequirementID string
	Scope                 string
}

// Clone 不执行正文规范化，也不让调用方与来源数组共享底层存储。
func (i RuntimeSourceObligationInventory) Clone() RuntimeSourceObligationInventory {
	i.EvidencePurposeMappings = slices.Clone(i.EvidencePurposeMappings)
	i.ExecutionProofMappings = slices.Clone(i.ExecutionProofMappings)
	i.Sources = slices.Clone(i.Sources)
	i.Resources = slices.Clone(i.Resources)
	i.Requirements = slices.Clone(i.Requirements)
	for n := range i.Requirements {
		i.Requirements[n].SourceItemRefs = slices.Clone(i.Requirements[n].SourceItemRefs)
		i.Requirements[n].ResourceIDs = slices.Clone(i.Requirements[n].ResourceIDs)
	}
	i.Obligations = slices.Clone(i.Obligations)
	for n := range i.Obligations {
		i.Obligations[n].DoDItemRefs = slices.Clone(i.Obligations[n].DoDItemRefs)
	}
	i.Verifications = slices.Clone(i.Verifications)
	return i
}

// WithHash 仅排序集合。原判据、要求与验证声明逐字节参与 hash，空白不能被擦除。
func (i RuntimeSourceObligationInventory) WithHash() RuntimeSourceObligationInventory {
	i = i.Clone()
	sort.Slice(i.Sources, func(a, b int) bool { return i.Sources[a].CandidateItemRef < i.Sources[b].CandidateItemRef })
	sort.Slice(i.Resources, func(a, b int) bool { return i.Resources[a].ID < i.Resources[b].ID })
	sort.Slice(i.Requirements, func(a, b int) bool { return i.Requirements[a].ID < i.Requirements[b].ID })
	for n := range i.Requirements {
		sort.Strings(i.Requirements[n].SourceItemRefs)
		sort.Strings(i.Requirements[n].ResourceIDs)
	}
	sort.Slice(i.Obligations, func(a, b int) bool { return i.Obligations[a].ID < i.Obligations[b].ID })
	for n := range i.Obligations {
		sort.Strings(i.Obligations[n].DoDItemRefs)
	}
	sort.Slice(i.Verifications, func(a, b int) bool { return i.Verifications[a].ID < i.Verifications[b].ID })
	sort.Slice(i.ExecutionProofMappings, func(a, b int) bool {
		left, right := i.ExecutionProofMappings[a], i.ExecutionProofMappings[b]
		if left.ConsumerItemRef != right.ConsumerItemRef {
			return left.ConsumerItemRef < right.ConsumerItemRef
		}
		return left.RuleRef < right.RuleRef
	})
	sort.Slice(i.EvidencePurposeMappings, func(a, b int) bool {
		return i.EvidencePurposeMappings[a].HistoricalItemRef < i.EvidencePurposeMappings[b].HistoricalItemRef
	})
	i.Hash = StableRuntimeSourceObligationInventoryHash(i)
	return i
}

func StableRuntimeSourceObligationInventoryHash(i RuntimeSourceObligationInventory) string {
	h := newRuntimeHashWriter()
	writeHashString(h, i.SchemaVersion)
	writeHashString(h, i.CandidateHash)
	writeHashString(h, i.SpecificationHash)
	writeHashString(h, i.DefinitionRevision)
	writeHashString(h, i.StartupInputHash)
	writeHashString(h, fmt.Sprintf("%d", len(i.Sources)))
	for _, source := range i.Sources {
		writeHashString(h, source.CandidateItemRef)
		writeHashString(h, source.ItemRef)
		writeHashString(h, source.SourceScope)
		writeHashString(h, source.SourceID)
		writeHashString(h, source.Text)
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Resources)))
	for _, resource := range i.Resources {
		writeHashString(h, resource.ID)
		writeHashString(h, resource.Ref)
		writeHashString(h, resource.SnapshotHash)
		writeHashString(h, fmt.Sprintf("%t", resource.Required))
		writeHashString(h, fmt.Sprintf("%t", resource.Available))
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Requirements)))
	for _, requirement := range i.Requirements {
		writeHashString(h, requirement.ID)
		writeHashString(h, requirement.DimensionID)
		writeHashString(h, fmt.Sprintf("%t", requirement.Required))
		writeHashString(h, requirement.SourceKind)
		writeHashString(h, requirement.Text)
		writeHashStrings(h, requirement.SourceItemRefs)
		writeHashStrings(h, requirement.ResourceIDs)
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Obligations)))
	for _, obligation := range i.Obligations {
		writeHashString(h, obligation.ID)
		writeHashString(h, obligation.RequirementID)
		writeHashString(h, obligation.Text)
		writeHashStrings(h, obligation.DoDItemRefs)
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Verifications)))
	for _, verification := range i.Verifications {
		writeHashString(h, verification.ID)
		writeHashString(h, verification.ObligationID)
		writeHashString(h, verification.Declaration)
		writeHashString(h, verification.ProfileID)
		writeHashString(h, verification.EvidenceRequirementID)
		writeHashString(h, verification.Scope)
	}
	if len(i.ExecutionProofMappings) != 0 {
		writeHashString(h, RuntimeCrossSourceExecutionProofV1)
		writeHashString(h, fmt.Sprint(len(i.ExecutionProofMappings)))
		for _, mapping := range i.ExecutionProofMappings {
			writeHashString(h, mapping.ContractRef)
			writeHashString(h, mapping.ConsumerItemRef)
			writeHashString(h, mapping.RuleRef)
			writeHashString(h, mapping.SourceItemRef)
			if mapping.ConsumerGoalText != "" {
				writeHashString(h, RuntimeGoalSourceExecutionProofV2)
				writeHashString(h, mapping.ConsumerGoalText)
			}
		}
	}
	if len(i.EvidencePurposeMappings) != 0 {
		writeHashString(h, RuntimeCrossCWIEvidencePurposeV1)
		writeHashString(h, fmt.Sprint(len(i.EvidencePurposeMappings)))
		for _, mapping := range i.EvidencePurposeMappings {
			writeHashString(h, mapping.ContractRef)
			writeHashString(h, mapping.HistoricalItemRef)
			writeHashString(h, mapping.AcceptanceItemRef)
			if runtimeEvidencePurposeHasProfileFields(mapping) {
				writeHashString(h, "profile-deliverable-source/v2")
				writeHashString(h, mapping.HistoricalProfileID)
				writeHashString(h, mapping.HistoricalDeliverableID)
				writeHashString(h, mapping.HistoricalProfileHash)
				writeHashString(h, mapping.HistoricalText)
			}
		}
	}
	return "runtime-source-obligation-inventory-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Validate 只反证结构与身份；完整性必须由 eligible candidate 的确定性重编译核对。
func (i RuntimeSourceObligationInventory) Validate() error {
	if i.SchemaVersion != RuntimeSourceObligationInventorySchemaCurrent ||
		!runtimeSourceIdentity(i.CandidateHash, 256) || !runtimeSourceIdentity(i.SpecificationHash, 256) ||
		!runtimeSourceIdentity(i.DefinitionRevision, 1024) || !runtimeSourceIdentity(i.StartupInputHash, 256) ||
		len(i.Sources) < 2 || len(i.Sources) > 513 || len(i.Resources) > 128 ||
		len(i.Requirements) == 0 || len(i.Requirements) > 256 || len(i.Obligations) == 0 || len(i.Obligations) > 512 ||
		len(i.Verifications) == 0 || len(i.Verifications) > 512 {
		return fmt.Errorf("runtime source obligation inventory lacks bounded exact startup identity or source graph")
	}
	identities := make(map[string]bool, len(i.Sources)+len(i.Resources)+len(i.Requirements)+len(i.Obligations)+len(i.Verifications))
	claimIdentity := func(id string, limit int) bool {
		if !runtimeSourceIdentity(id, limit) || identities[id] {
			return false
		}
		identities[id] = true
		return true
	}
	sources := make(map[string]RuntimeDefinitionSource, len(i.Sources))
	formalRefs, sourceKeys := make(map[string]bool), make(map[string]bool)
	goalRef, goalCount := "", 0
	for _, source := range i.Sources {
		key := source.SourceScope + "\x00" + source.SourceID
		if !claimIdentity(source.CandidateItemRef, 1024) || !runtimeSourceIdentity(source.ItemRef, 1024) ||
			!runtimeSourceIdentity(source.SourceID, 128) || strings.TrimSpace(source.Text) == "" || len(source.Text) > 32*1024 ||
			sources[source.CandidateItemRef].CandidateItemRef != "" || formalRefs[source.ItemRef] || sourceKeys[key] {
			return fmt.Errorf("runtime source obligation inventory has invalid or duplicate source %q", source.CandidateItemRef)
		}
		switch source.SourceScope {
		case "goal":
			goalRef, goalCount = source.CandidateItemRef, goalCount+1
		case "dodItem":
		default:
			return fmt.Errorf("runtime source obligation inventory has unsupported source scope %q", source.SourceScope)
		}
		sources[source.CandidateItemRef], formalRefs[source.ItemRef], sourceKeys[key] = source, true, true
	}
	if goalCount != 1 {
		return fmt.Errorf("runtime source obligation inventory requires exactly one Goal source")
	}
	if len(i.ExecutionProofMappings) > 512 {
		return fmt.Errorf("runtime execution proof mappings exceed bound")
	}
	proofKeys := map[string]bool{}
	goalProofSources := map[string]string{}
	for _, mapping := range i.ExecutionProofMappings {
		key := mapping.ConsumerItemRef + "\x00" + mapping.RuleRef
		if validateRuntimeExecutionProofMappingSource(mapping, sources[mapping.ConsumerItemRef], sources[mapping.SourceItemRef]) != nil || proofKeys[key] {
			return fmt.Errorf("runtime execution proof mapping requires unique exact original consumer/rule/DoD source")
		}
		proofKeys[key] = true
		if mapping.ContractRef == RuntimeGoalSourceExecutionProofV2 {
			if prior := goalProofSources[mapping.ConsumerItemRef]; prior != "" && prior != mapping.SourceItemRef {
				return fmt.Errorf("Goal mappings have ambiguous execution sources")
			}
			goalProofSources[mapping.ConsumerItemRef] = mapping.SourceItemRef
		}
	}
	resources := make(map[string]bool, len(i.Resources))
	if err := validateRuntimeEvidencePurposeMappingSources(i); err != nil {
		return err
	}
	for _, resource := range i.Resources {
		if !claimIdentity(resource.ID, 128) || !runtimeSourceIdentity(resource.Ref, 1024) || resources[resource.ID] ||
			len(resource.SnapshotHash) > 256 || (resource.Required && (!resource.Available || resource.SnapshotHash == "")) {
			return fmt.Errorf("runtime source obligation inventory has invalid resource %q", resource.ID)
		}
		resources[resource.ID] = true
	}
	requirements := make(map[string]RuntimeSourceRequirement, len(i.Requirements))
	for _, requirement := range i.Requirements {
		if !claimIdentity(requirement.ID, 128) || !runtimeSourceIdentity(requirement.DimensionID, 128) ||
			!runtimeSourceIdentity(requirement.SourceKind, 128) || requirements[requirement.ID].ID != "" ||
			strings.TrimSpace(requirement.Text) == "" || len(requirement.Text) > 32*1024 ||
			!runtimeSourceRefs(requirement.SourceItemRefs, 32, false, func(ref string) bool { return sources[ref].CandidateItemRef != "" }) ||
			!runtimeSourceRefs(requirement.ResourceIDs, 128, true, func(ref string) bool { return resources[ref] }) {
			return fmt.Errorf("runtime source obligation inventory has invalid requirement %q", requirement.ID)
		}
		switch requirement.SourceKind {
		case "USER_EXPLICIT", "TEMPLATE_REQUIRED", "TEMPLATE_CONDITIONAL", "DERIVED", "DISCOVERED":
		default:
			return fmt.Errorf("runtime source obligation inventory has unsupported requirement source %q", requirement.SourceKind)
		}
		requirements[requirement.ID] = requirement
	}
	obligations := make(map[string]bool, len(i.Obligations))
	mappedRequirements, mappedDoD := make(map[string]bool), make(map[string]bool)
	for _, obligation := range i.Obligations {
		if !claimIdentity(obligation.ID, 128) || obligations[obligation.ID] || requirements[obligation.RequirementID].ID == "" ||
			strings.TrimSpace(obligation.Text) == "" || len(obligation.Text) > 4096 ||
			!runtimeSourceRefs(obligation.DoDItemRefs, 32, false, func(ref string) bool { return sources[ref].SourceScope == "dodItem" }) {
			return fmt.Errorf("runtime source obligation inventory has invalid obligation %q", obligation.ID)
		}
		obligations[obligation.ID], mappedRequirements[obligation.RequirementID] = true, true
		for _, ref := range obligation.DoDItemRefs {
			mappedDoD[ref] = true
		}
	}
	for _, requirement := range i.Requirements {
		independentGoal := len(requirement.SourceItemRefs) == 1 && requirement.SourceItemRefs[0] == goalRef &&
			requirement.SourceKind == "USER_EXPLICIT" && requirement.Text == sources[goalRef].Text
		if requirement.Required && !mappedRequirements[requirement.ID] && !independentGoal {
			return fmt.Errorf("runtime source obligation inventory omits required obligation for %q", requirement.ID)
		}
	}
	for _, source := range i.Sources {
		if source.SourceScope == "dodItem" && !mappedDoD[source.CandidateItemRef] {
			return fmt.Errorf("runtime source obligation inventory omits DoD source %q", source.CandidateItemRef)
		}
	}
	verificationIDs, verified := make(map[string]bool), make(map[string]bool)
	for _, verification := range i.Verifications {
		if !claimIdentity(verification.ID, 128) || verificationIDs[verification.ID] || !obligations[verification.ObligationID] ||
			strings.TrimSpace(verification.Declaration) == "" || len(verification.Declaration) > 4096 ||
			!runtimeSourceIdentity(verification.ProfileID, 128) || !runtimeSourceIdentity(verification.EvidenceRequirementID, 128) ||
			!runtimeSourceIdentity(verification.Scope, 128) {
			return fmt.Errorf("runtime source obligation inventory has invalid verification %q", verification.ID)
		}
		verificationIDs[verification.ID], verified[verification.ObligationID] = true, true
	}
	for id := range obligations {
		if !verified[id] {
			return fmt.Errorf("runtime source obligation inventory omits verification for %q", id)
		}
	}
	canonical := i.WithHash()
	if i.Hash != canonical.Hash || StableRuntimeSourceObligationInventoryHash(i) != canonical.Hash {
		return fmt.Errorf("runtime source obligation inventory hash or canonical order mismatch")
	}
	return nil
}

func SameRuntimeSourceObligationInventory(left, right *RuntimeSourceObligationInventory) bool {
	return reflect.DeepEqual(left, right)
}

func runtimeSourceIdentity(value string, limit int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= limit && !strings.ContainsRune(value, '\x00')
}

func runtimeSourceRefs(refs []string, limit int, allowEmpty bool, exists func(string) bool) bool {
	if len(refs) > limit || (!allowEmpty && len(refs) == 0) {
		return false
	}
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if !runtimeSourceIdentity(ref, 1024) || seen[ref] || !exists(ref) {
			return false
		}
		seen[ref] = true
	}
	return true
}
