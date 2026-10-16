package workorchruntimecontract

import (
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeSourceObligationCheck 将同一 CWI 上每项源义务的验证声明分开标识。
// 一个共同 operation 可以提供多项证明，但不能让 A 的检查冒充 B。
type RuntimeSourceObligationCheck struct {
	InventoryHash  string
	ObligationID   string
	RequirementID  string
	VerificationID string
	Criterion      string
	Declaration    string
	CheckRef       string
}

func CloneRuntimeSourceObligationChecks(values []RuntimeSourceObligationCheck) []RuntimeSourceObligationCheck {
	return slices.Clone(values)
}

func StableRuntimeSourceObligationCheckRef(definitionRevision string, coverageID common.CoverageWorkItemID, value RuntimeSourceObligationCheck) string {
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-source-obligation-check/v1")
	writeHashString(h, definitionRevision)
	writeHashString(h, string(coverageID))
	writeHashString(h, value.InventoryHash)
	writeHashString(h, value.ObligationID)
	writeHashString(h, value.RequirementID)
	writeHashString(h, value.VerificationID)
	writeHashString(h, value.Criterion)
	writeHashString(h, value.Declaration)
	return "workorch-obligation-check-" + hex.EncodeToString(h.Sum(nil))
}

func ValidateRuntimeSourceObligationChecks(definitionRevision string, coverageID common.CoverageWorkItemID, values []RuntimeSourceObligationCheck) error {
	if len(values) > 512 {
		return fmt.Errorf("runtime source obligation checks exceed specification verification bound")
	}
	seen := make(map[string]bool, len(values))
	last := ""
	for n, value := range values {
		if !runtimeSourceIdentity(value.InventoryHash, 256) || !runtimeSourceIdentity(value.ObligationID, 128) ||
			!runtimeSourceIdentity(value.RequirementID, 128) || !runtimeSourceIdentity(value.VerificationID, 128) ||
			strings.TrimSpace(value.Criterion) == "" || len(value.Criterion) > 4096 ||
			strings.TrimSpace(value.Declaration) == "" || len(value.Declaration) > 4096 ||
			value.CheckRef != StableRuntimeSourceObligationCheckRef(definitionRevision, coverageID, value) ||
			seen[value.VerificationID] || (n > 0 && value.VerificationID <= last) {
			return fmt.Errorf("runtime source obligation check %q has invalid exact source identity", value.VerificationID)
		}
		last, seen[value.VerificationID] = value.VerificationID, true
	}
	return nil
}

// CompileRuntimeSourceObligationChecks 只沿已封存来源边映射，不从 DoD 文本猜测义务。
func CompileRuntimeSourceObligationChecks(inventory RuntimeSourceObligationInventory, item RuntimeCoverageItem) ([]RuntimeSourceObligationCheck, error) {
	inventory = inventory.Clone()
	if err := inventory.Validate(); err != nil {
		return nil, err
	}
	return compileRuntimeSourceObligationChecksValidated(inventory, item)
}

// Invocation-local use only after the caller has fully validated this exact
// detached inventory. Public calls never trust a supplied hash or cached proof.
func compileRuntimeSourceObligationChecksValidated(inventory RuntimeSourceObligationInventory, item RuntimeCoverageItem) ([]RuntimeSourceObligationCheck, error) {
	var sourceRef string
	for _, source := range RuntimeEvidencePurposeSources(inventory) {
		if source.SourceScope == item.SourceScope && source.SourceID == item.SourceID {
			if strings.TrimSpace(source.Text) != item.TargetText {
				return nil, fmt.Errorf("runtime source obligations coverage %q source text drift", item.ID)
			}
			sourceRef = source.CandidateItemRef
		}
	}
	if sourceRef == "" {
		if item.SourceScope == "dodItem" {
			return nil, fmt.Errorf("runtime source obligations omit coverage source %q", item.SourceID)
		}
		return nil, nil
	}
	// An explicitly mapped ordinary acceptance source must prove its own
	// obligations AND every original obligation of each mapped historical
	// source. Keep the historical row unchanged; only the receiving check
	// identity is compiled for this exact consumer CWI. The inventory hash
	// binds the mapping and original source graph, so runtime text or order
	// cannot select an additional source.
	sourceRefs := map[string]bool{sourceRef: true}
	for _, mapping := range inventory.EvidencePurposeMappings {
		if mapping.AcceptanceItemRef == sourceRef {
			sourceRefs[mapping.HistoricalItemRef] = true
		}
	}
	var ret []RuntimeSourceObligationCheck
	for _, obligation := range inventory.Obligations {
		contributes := false
		for _, ref := range obligation.DoDItemRefs {
			contributes = contributes || sourceRefs[ref]
		}
		if !contributes {
			continue
		}
		for _, verification := range inventory.Verifications {
			if verification.ObligationID != obligation.ID {
				continue
			}
			check := RuntimeSourceObligationCheck{
				InventoryHash: inventory.Hash, ObligationID: obligation.ID, RequirementID: obligation.RequirementID,
				VerificationID: verification.ID, Criterion: obligation.Text, Declaration: verification.Declaration,
			}
			check.CheckRef = StableRuntimeSourceObligationCheckRef(inventory.DefinitionRevision, item.ID, check)
			ret = append(ret, check)
		}
	}
	for _, mapping := range inventory.EvidencePurposeMappings {
		if mapping.ContractRef == RuntimeProfileDeliverableEvidencePurposeV2 && sourceRefs[mapping.HistoricalItemRef] {
			ret = append(ret, runtimeProfileEvidencePurposeCheck(inventory, mapping, item))
		}
	}
	if check := runtimeGoalExecutionCheck(inventory, item); check != nil {
		ret = append(ret, *check)
	}
	sort.Slice(ret, func(a, b int) bool { return ret[a].VerificationID < ret[b].VerificationID })
	if err := ValidateRuntimeSourceObligationChecks(inventory.DefinitionRevision, item.ID, ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func sameRuntimeSourceObligationChecks(left, right []RuntimeSourceObligationCheck) bool {
	return slices.Equal(left, right)
}

// ValidateRuntimeSourceObligationSeed 重新编译完整源图，不能用自洽 hash 掩盖遗漏的 B。
func ValidateRuntimeSourceObligationSeed(seed RuntimeDefinitionSeed) error {
	if seed.SourceObligations == nil {
		return fmt.Errorf("runtime source obligation seed requires inventory")
	}
	inventory := seed.SourceObligations.Clone()
	if err := inventory.Validate(); err != nil {
		return err
	}
	if err := validateRuntimeGoalExecutionProjection(inventory, seed.CoverageItems); err != nil {
		return err
	}
	if inventory.DefinitionRevision != seed.DefinitionRevision {
		return fmt.Errorf("runtime source obligation seed definition revision mismatch")
	}
	if err := validateRuntimeEvidencePurposeMappingCoverageValidated(inventory, seed.CoverageItems); err != nil {
		return err
	}
	seenSources := make(map[string]bool)
	for _, item := range seed.CoverageItems {
		checks, err := compileRuntimeSourceObligationChecksValidated(inventory, item)
		if err != nil {
			return err
		}
		if !sameRuntimeSourceObligationChecks(checks, item.SourceObligationChecks) {
			return fmt.Errorf("runtime coverage %q source obligation checks do not match sealed inventory", item.ID)
		}
		if item.SourceScope == "dodItem" {
			if seenSources[item.SourceID] {
				return fmt.Errorf("runtime source obligation seed duplicates DoD coverage %q", item.SourceID)
			}
			seenSources[item.SourceID] = true
		}
	}
	for _, source := range inventory.Sources {
		if source.SourceScope == "dodItem" && !seenSources[source.SourceID] {
			return fmt.Errorf("runtime source obligation seed omits DoD coverage %q", source.SourceID)
		}
	}
	return nil
}

func writeRuntimeSourceObligationChecksHash(h hashStringWriter, values []RuntimeSourceObligationCheck) {
	writeHashString(h, "runtime-source-obligation-checks/v1")
	writeHashString(h, fmt.Sprintf("%d", len(values)))
	for _, value := range values {
		writeHashString(h, value.InventoryHash)
		writeHashString(h, value.ObligationID)
		writeHashString(h, value.RequirementID)
		writeHashString(h, value.VerificationID)
		writeHashString(h, value.Criterion)
		writeHashString(h, value.Declaration)
		writeHashString(h, value.CheckRef)
	}
}
