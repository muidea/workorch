package definitionconfiguration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/bootstrapcompiler"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const executionLimitsAdmissionRef = "execution-limits:effective"

// Bindings is the immutable, normalized result of compiling a Workspace's
// effective Definition configuration from its current Workspace and Template.
type Bindings struct {
	PolicyBindings        []workspacemodel.DefinitionAdmissionPolicyBinding
	ProfileContracts      []runtimecontract.RuntimeProfileContract
	RequiredOwnerFacts    []workspacemodel.DefinitionRequiredOwnerFactBinding
	OwnerFactConvergences []workspacemodel.DefinitionOwnerFactConvergenceBinding
	ProfileContractsHash  string
}

// Compile validates both raw source layers before merge, then derives the
// effective profile contracts and policy snapshot bindings. It is pure: neither
// source model is mutated and it performs no I/O.
func Compile(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) (Bindings, error) {
	items, err := configurationDoDItems(workspace, template)
	if err != nil {
		return Bindings{}, err
	}
	return CompileForItems(workspace, template, items)
}

func CompileForItems(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate, items []workspacemodel.DefinitionCandidateItem) (Bindings, error) {
	if workspace == nil || template == nil {
		return Bindings{}, common.ErrInvalidArgument
	}
	workspaceHints, templateHints := workspace.ContextHints.Map(), template.ContextHints.Map()
	for _, input := range []map[string]any{templateHints, workspaceHints} {
		if err := validateExecutionLimitsInput(input); err != nil {
			return Bindings{}, err
		}
		if err := runtimeprofiledecode.ValidateInputShape(input); err != nil {
			return Bindings{}, err
		}
	}
	if err := validateEvidencePolicies("Template "+strings.TrimSpace(template.ID), template.EvidencePolicies); err != nil {
		return Bindings{}, err
	}
	if err := validateEvidencePolicies("Workspace "+strings.TrimSpace(string(workspace.ID)), workspace.EvidencePolicies); err != nil {
		return Bindings{}, err
	}

	effective := runtimeprofiledecode.MergeTemplateWorkspaceContextHintsForExecutionContract(templateHints, workspaceHints)
	rawProfiles := runtimeprofiledecode.Map(effective["profileContracts"])
	var err error
	rawProfiles, err = compileTemplateProofBindings(template, workspaceHints, rawProfiles, items)
	if err != nil {
		return Bindings{}, err
	}
	profiles := runtimeprofiledecode.Decode(rawProfiles)
	maxFocuses := runtimecontract.RuntimeAcceptanceFocusMaxCount
	if policy := runtimeprofiledecode.Map(effective["acceptanceSlicePolicy"]); len(policy) > 0 {
		maxFocuses = integer(policy["maxFocuses"])
	}
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}
	capacityGoal := ""
	capacityProfiles := runtimecontract.NormalizeRuntimeProfileContracts(profiles)
	if workspace.DefinitionAuthoring != nil {
		capacityGoal = workspace.DefinitionAuthoring.Input.GoalText
	} else {
		// Without a Goal this is a lower-bound preview, never recipe selection.
		for i := range capacityProfiles {
			capacityProfiles[i].BootstrapRecipes = nil
		}
	}
	if len(texts) > 0 {
		if _, err := bootstrapcompiler.CompileCapacity(capacityGoal, texts, capacityProfiles, maxFocuses); err != nil {
			return Bindings{}, err
		}
	}

	policies := make([]workspacemodel.DefinitionAdmissionPolicyBinding, 0, len(profiles)+len(template.EvidencePolicies)+len(workspace.EvidencePolicies)+1)
	for _, profile := range profiles {
		encoded, err := json.Marshal(rawProfiles[profile.ID])
		if err != nil {
			return Bindings{}, fmt.Errorf("%w: profile %q cannot be canonically sealed", common.ErrConflict, profile.ID)
		}
		h := sha256.New()
		_, _ = fmt.Fprintf(h, "%08x%s%08x%s", len(profile.ID), profile.ID, len(encoded), encoded)
		policies = append(policies, workspacemodel.DefinitionAdmissionPolicyBinding{
			PolicyRef: "profile:" + profile.ID, Revision: "runtime-profile-contract-v1",
			SnapshotHash: "profile-snapshot-sha256:" + hex.EncodeToString(h.Sum(nil)),
		})
	}
	policies = append(policies, evidencePolicyBindings("template", template.EvidencePolicies)...)
	policies = append(policies, evidencePolicyBindings("workspace", workspace.EvidencePolicies)...)
	if limits := executionLimits(effective); limits != nil {
		policies = append(policies, executionLimitsBinding(*limits))
	}
	required, convergences := definitionadmission.CompileProfileOwnerFactBindings(profiles)
	return Bindings{
		PolicyBindings: policies, ProfileContracts: profiles,
		RequiredOwnerFacts: required, OwnerFactConvergences: convergences,
		ProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(profiles),
	}, nil
}

func validateExecutionLimitsInput(input map[string]any) error {
	raw, present := input["executionLimits"]
	if !present {
		return nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: executionLimits requires an object", common.ErrInvalidArgument)
	}
	for key := range value {
		switch key {
		case "protocolRef", "maxProviderCalls", "maxCapabilityOperations", "maxProofBatchesPerCWI", "maxDurableDispatchesPerCWI":
		default:
			return fmt.Errorf("%w: unknown executionLimits field %q", common.ErrInvalidArgument, key)
		}
	}
	for _, key := range []string{"maxProofBatchesPerCWI", "maxDurableDispatchesPerCWI"} {
		if bound, exists := value[key]; exists {
			if !runtimeprofiledecode.IntegerHasExactShape(bound) || integer(bound) < 1 {
				return fmt.Errorf("%w: executionLimits.%s must be a positive exact integer", common.ErrInvalidArgument, key)
			}
		}
	}
	protocol, ok := value["protocolRef"].(string)
	if !ok || protocol != runtimecontract.RuntimeExecutionLimitsV1 {
		return fmt.Errorf("%w: executionLimits.protocolRef must select the current protocol", common.ErrInvalidArgument)
	}
	for _, key := range []string{"maxProviderCalls", "maxCapabilityOperations"} {
		if !runtimeprofiledecode.IntegerHasExactShape(value[key]) {
			return fmt.Errorf("%w: executionLimits.%s must be an exact integer", common.ErrInvalidArgument, key)
		}
	}
	if err := executionLimits(input).Validate(); err != nil {
		return fmt.Errorf("%w: executionLimits: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func executionLimits(input map[string]any) *runtimecontract.RuntimeExecutionLimits {
	raw, present := input["executionLimits"]
	if !present {
		return nil
	}
	value := runtimeprofiledecode.Map(raw)
	return &runtimecontract.RuntimeExecutionLimits{
		ProtocolRef:                runtimeprofiledecode.String(value["protocolRef"]),
		MaxProviderCalls:           integer(value["maxProviderCalls"]),
		MaxCapabilityOperations:    integer(value["maxCapabilityOperations"]),
		MaxProofBatchesPerCWI:      integer(value["maxProofBatchesPerCWI"]),
		MaxDurableDispatchesPerCWI: integer(value["maxDurableDispatchesPerCWI"]),
	}
}

func integer(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func executionLimitsBinding(limits runtimecontract.RuntimeExecutionLimits) workspacemodel.DefinitionAdmissionPolicyBinding {
	return workspacemodel.DefinitionAdmissionPolicyBinding{
		PolicyRef: executionLimitsAdmissionRef, Revision: limits.ProtocolRef,
		SnapshotHash: runtimecontract.StableRuntimeExecutionLimitsHash(limits),
	}
}

func evidencePolicyBindings(owner string, policies []workorcheventpayload.Payload) []workspacemodel.DefinitionAdmissionPolicyBinding {
	ret := make([]workspacemodel.DefinitionAdmissionPolicyBinding, 0, len(policies))
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
	return ret
}

func validateEvidencePolicies(owner string, items []workorcheventpayload.Payload) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "Runtime"
	}
	seen := make(map[string]struct{}, len(items))
	for idx, item := range items {
		if payloadContainsField(item.Fields, "prospectiveTargetRef") {
			return fmt.Errorf("%s evidence policies: %w: pre-seal evidence policy rejects prospectiveTargetRef; declare targetSelectorCandidateRefs instead", owner, common.ErrInvalidArgument)
		}
		id, ok := item.String("id")
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			return fmt.Errorf("%w: %s evidence policy %d requires a non-empty string id", common.ErrInvalidArgument, owner, idx+1)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: %s evidence policies duplicate id %q", common.ErrInvalidArgument, owner, id)
		}
		seen[id] = struct{}{}
		if err := evidencePolicy(item).Validate(); err != nil {
			return fmt.Errorf("%w: %s evidence policy %q is invalid: %v", common.ErrInvalidArgument, owner, id, err)
		}
	}
	return nil
}

func payloadContainsField(fields []workorcheventpayload.Field, fieldName string) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.Name), fieldName) || payloadValueContainsField(field.Value, fieldName) {
			return true
		}
	}
	return false
}

func payloadValueContainsField(value workorcheventpayload.Value, fieldName string) bool {
	switch value.Kind {
	case workorcheventpayload.ValueObject:
		return payloadContainsField(value.Object, fieldName)
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			if payloadValueContainsField(item, fieldName) {
				return true
			}
		}
	}
	return false
}

func evidencePolicy(item workorcheventpayload.Payload) runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: payloadString(item, "id"), Profile: payloadString(item, "profile"), Rule: payloadString(item, "rule"),
		AppliesTo: payloadString(item, "appliesTo"), CommandName: payloadString(item, "commandName"),
		RecipeAdapterRef: payloadString(item, "recipeAdapterRef"), RecipeInputReviewProtocolRef: payloadString(item, "recipeInputReviewProtocolRef"),
		ArgsContain: payloadStrings(item, "argsContain"), RecipeDeclared: payloadBool(item, "recipeDeclared"), RecipeArgs: payloadStrings(item, "recipeArgs"),
		RecipeTimeoutMillis: int64(payloadInt(item, "recipeTimeoutMillis")), RequiredArgs: payloadStrings(item, "requiredArgs"),
		ZeroWorkMarkers: payloadStrings(item, "zeroWorkMarkers"), WorkEvidenceMarkers: payloadStrings(item, "workEvidenceMarkers"),
		QualityOnZeroWork: payloadString(item, "qualityOnZeroWork"), MarksZeroWork: payloadBool(item, "zeroWork"),
		EvidenceTags: payloadStrings(item, "evidenceTags"), EvidenceReason: payloadString(item, "evidenceReason"),
		CoverageRequirements: payloadCoverageRequirements(item, "coverageRequirements"), Description: payloadString(item, "description"),
	}
}

func payloadString(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return value
}

func payloadBool(input workorcheventpayload.Payload, key string) bool {
	value, _ := input.Bool(key)
	return value
}

func payloadInt(input workorcheventpayload.Payload, key string) int64 {
	value, _ := input.Int64(key)
	return value
}

func payloadStrings(input workorcheventpayload.Payload, key string) []string {
	value, _ := input.StringSlice(key)
	return value
}

func payloadCoverageRequirements(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeCoverageEvidenceRequirement {
	value, ok := input.Field(key)
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]runtimecontract.RuntimeCoverageEvidenceRequirement, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != workorcheventpayload.ValueObject {
			continue
		}
		payload := workorcheventpayload.Payload{Fields: item.Object}
		ret = append(ret, runtimecontract.RuntimeCoverageEvidenceRequirement{
			ID: payloadString(payload, "id"), Scope: payloadString(payload, "scope"), EvidenceRole: payloadString(payload, "evidenceRole"),
			TargetRole: payloadString(payload, "targetRole"), TargetRefs: payloadStrings(payload, "targetRefs"),
			TargetDiscoverySources: payloadStrings(payload, "targetDiscoverySources"), TargetSelectorRef: payloadString(payload, "targetSelectorRef"),
			TargetSelectorOwnerRef: payloadString(payload, "targetSelectorOwnerRef"), TargetSelectorSubjectRef: payloadString(payload, "targetSelectorSubjectRef"),
			TargetSelectorCandidateRefs:          payloadStrings(payload, "targetSelectorCandidateRefs"),
			ProspectiveTargetAllOfConstraintRefs: payloadStrings(payload, "prospectiveTargetAllOfConstraintRefs"), RequireTargetGrounding: payloadBool(payload, "requireTargetGrounding"),
			RequiredEvidenceKinds: payloadStrings(payload, "requiredEvidenceKinds"), RequireDirectEvidence: payloadBool(payload, "requireDirectEvidence"),
			RequireClaimEvidence: payloadBool(payload, "requireClaimEvidence"), RequireExecutableBehaviorEvidence: payloadBool(payload, "requireExecutableBehaviorEvidence"),
			RequireCoverageTargets: payloadBool(payload, "requireCoverageTargets"), AcceptedStepKinds: payloadStrings(payload, "acceptedStepKinds"),
			RejectedStepKinds: payloadStrings(payload, "rejectedStepKinds"), AcceptedOperationVariants: operationVariants(payload, "acceptedOperationVariants"),
			ValidationRecipeID: payloadString(payload, "validationRecipeId"), RequiredCheckRefs: payloadStrings(payload, "requiredCheckRefs"),
			MetricConstraints: payloadStrings(payload, "metricConstraints"), ArtifactRefs: payloadStrings(payload, "artifactRefs"),
			Description: payloadString(payload, "description"),
		})
	}
	return ret
}

func operationVariants(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeOperationVariantRef {
	value, ok := input.Field(key)
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationVariantRef, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != workorcheventpayload.ValueObject {
			continue
		}
		payload := workorcheventpayload.Payload{Fields: item.Object}
		ret = append(ret, runtimecontract.RuntimeOperationVariantRef{Kind: payloadString(payload, "kind"), VariantID: payloadString(payload, "variantId")})
	}
	return runtimecontract.NormalizeRuntimeOperationVariantRefs(ret)
}
