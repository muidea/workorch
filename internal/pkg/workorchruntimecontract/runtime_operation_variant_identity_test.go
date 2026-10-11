package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func referenceRuntimeSemanticVariantExists(contract RuntimeOperationContract, variantID string) bool {
	variantID = strings.TrimSpace(variantID)
	for _, variant := range contract.Variants {
		if variant.ID == variantID {
			return true
		}
	}
	return false
}

func referenceRuntimeOperationAdmissibilityVariantValidate(v RuntimeOperationAdmissibilityVariant, owner string, operationKinds, selectors, constraints []string) error {
	rawPolicies := append([]RuntimeOperationTargetPolicy(nil), v.AllowedTargetPolicies...)
	v = v.Normalized()
	owner = strings.TrimSpace(owner)
	if v.Kind == "" || !runtimeOperationKindsContain(operationKinds, v.Kind) {
		return fmt.Errorf("runtime operation admissibility variant %q is absent from owner kind inventory", v.Kind)
	}
	descriptor, ok := RuntimeOperationDescriptorForKind(v.Kind)
	if !ok || descriptor.OwnerModuleID != owner {
		return fmt.Errorf("runtime operation admissibility variant %q does not belong to owner %q", v.Kind, owner)
	}
	contract, found := RuntimeOperationContractForKind(v.Kind)
	if found && len(contract.Variants) != 0 && v.VariantID == "" {
		return fmt.Errorf("runtime operation admissibility kind %q requires a concrete variant fact", v.Kind)
	}
	if v.VariantID != "" && (!found || !referenceRuntimeSemanticVariantExists(contract, v.VariantID)) {
		return fmt.Errorf("runtime operation admissibility variant %q/%q is outside owner operation contract", v.Kind, v.VariantID)
	}
	for _, raw := range rawPolicies {
		switch RuntimeOperationTargetPolicy(strings.TrimSpace(string(raw))) {
		case RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		default:
			return fmt.Errorf("runtime operation admissibility variant %q/%q declares unknown target policy %q", v.Kind, v.VariantID, raw)
		}
	}
	if len(v.AllowedTargetPolicies) == 0 {
		return fmt.Errorf("runtime operation admissibility variant %q/%q requires target policies", v.Kind, v.VariantID)
	}
	if !allStringsContained(v.SelectorRefs, selectors) || !allStringsContained(v.ConstraintRefs, constraints) {
		return fmt.Errorf("runtime operation admissibility variant %q/%q references undeclared selector or constraint", v.Kind, v.VariantID)
	}
	if runtimeOperationAdmissibilityVariantAllowsTargetPolicy(v, RuntimeOperationTargetPolicyProspectiveOwnerTarget) &&
		(len(v.SelectorRefs) == 0 || len(v.ConstraintRefs) == 0) {
		return fmt.Errorf("runtime operation admissibility variant %q/%q prospective target policy requires owner selector and constraint", v.Kind, v.VariantID)
	}
	return nil
}

func referenceRuntimeOwnerFactConvergenceValidate(c RuntimeOwnerFactConvergence) error {
	c = c.Normalized()
	if err := c.Requirement.Validate(); err != nil {
		return err
	}
	if c.RequirePreexisting {
		if c.ProspectiveSatisfactionContractRef != "" || c.MissingTrigger != "" || c.AcquisitionOwnerModuleID != "" || c.AcquisitionOperationKind != "" ||
			c.AcquisitionOperationVariantID != "" || c.AcquisitionTargetPolicy != "" || c.AcquisitionTargetSelectorRef != "" ||
			c.TargetSelectorRef != "" || c.TargetSelectorOwnerRef != "" || c.TargetSelectorSubjectRef != "" ||
			c.ReobserveOwnerModuleID != "" || c.ReobserveFactKind != "" || c.MaxAcquisitionAttempts != 0 || len(c.DependsOnRequirementRefs) != 0 {
			return fmt.Errorf("preexisting owner-fact convergence cannot declare acquisition or re-observation")
		}
		return nil
	}
	if c.MissingTrigger != RuntimeOwnerFactMissingTrigger || c.AcquisitionOwnerModuleID == "" || c.AcquisitionOperationKind == "" ||
		c.AcquisitionTargetPolicy == "" || c.AcquisitionTargetSelectorRef == "" ||
		c.TargetSelectorRef == "" || c.TargetSelectorOwnerRef == "" || c.TargetSelectorSubjectRef == "" ||
		c.ReobserveOwnerModuleID != c.Requirement.OwnerModuleID || c.ReobserveFactKind != c.Requirement.FactKind || c.MaxAcquisitionAttempts != 1 {
		return fmt.Errorf("owner-fact acquisition requires exact missing trigger, owner operation, selector, re-observation and one attempt")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(c.AcquisitionOperationKind)
	if !found || descriptor.OwnerModuleID != c.AcquisitionOwnerModuleID {
		return fmt.Errorf("owner-fact acquisition operation %q does not belong to owner %q", c.AcquisitionOperationKind, c.AcquisitionOwnerModuleID)
	}
	if contract, found := RuntimeOperationContractForKind(c.AcquisitionOperationKind); found {
		if len(contract.Variants) == 0 && c.AcquisitionOperationVariantID != "" {
			return fmt.Errorf("owner-fact acquisition operation %q does not declare a static variant", c.AcquisitionOperationKind)
		}
		if len(contract.Variants) != 0 {
			matched := false
			for _, variant := range contract.Variants {
				if variant.ID == c.AcquisitionOperationVariantID {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("owner-fact acquisition operation %q requires one exact static variant", c.AcquisitionOperationKind)
			}
		}
	}
	switch c.AcquisitionTargetPolicy {
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if c.ProspectiveSatisfactionContractRef != "" {
			return fmt.Errorf("existing-scope owner-fact acquisition cannot declare a prospective satisfaction contract")
		}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if c.ProspectiveSatisfactionContractRef == "" {
			return fmt.Errorf("prospective owner-fact acquisition requires a prospective satisfaction contract")
		}
	default:
		return fmt.Errorf("owner-fact acquisition requires a selector-backed target policy, got %q", c.AcquisitionTargetPolicy)
	}
	return nil
}

func operationVariantIdentityConvergenceFixture(kind, variantID string) RuntimeOwnerFactConvergence {
	descriptor, _ := RuntimeOperationDescriptorForKind(kind)
	requirement := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
	return RuntimeOwnerFactConvergence{
		Requirement: requirement, MissingTrigger: RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID: descriptor.OwnerModuleID, AcquisitionOperationKind: kind, AcquisitionOperationVariantID: variantID,
		AcquisitionTargetPolicy: RuntimeOperationTargetPolicyExistingOwnerScope, AcquisitionTargetSelectorRef: "selector/op",
		TargetSelectorRef: "selector/definition", TargetSelectorOwnerRef: "owner", TargetSelectorSubjectRef: "subject",
		ReobserveOwnerModuleID: requirement.OwnerModuleID, ReobserveFactKind: requirement.FactKind, MaxAcquisitionAttempts: 1,
	}
}

func assertOperationVariantIdentityReference(t testing.TB, kind, variantID string) {
	t.Helper()
	contract, known := RuntimeOperationContractForKind(kind)
	gotKnown, gotVariants, gotContains := runtimeOperationVariantIdentity(kind, variantID)
	if gotKnown != known || gotVariants != (len(contract.Variants) != 0) || gotContains != referenceRuntimeSemanticVariantExists(contract, variantID) {
		t.Fatalf("静态 variant 身份偏离公开完整合同：kind=%q variant=%q", kind, variantID)
	}
	descriptor, _ := RuntimeOperationDescriptorForKind(kind)
	value := RuntimeOperationAdmissibilityVariant{Kind: kind, VariantID: variantID, AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope}, SelectorRefs: []string{"selector"}, ConstraintRefs: []string{"constraint"}}
	before := value.Normalized()
	assertProofCollectionIdentityErrors(t, value.Validate(descriptor.OwnerModuleID, []string{kind}, []string{"selector"}, []string{"constraint"}), referenceRuntimeOperationAdmissibilityVariantValidate(value, descriptor.OwnerModuleID, []string{kind}, []string{"selector"}, []string{"constraint"}))
	if !reflect.DeepEqual(before, value.Normalized()) {
		t.Fatal("静态身份查询改写 owner variant")
	}
	convergence := operationVariantIdentityConvergenceFixture(kind, variantID)
	assertProofCollectionIdentityErrors(t, convergence.Validate(), referenceRuntimeOwnerFactConvergenceValidate(convergence))
}

func TestOperationVariantIdentityMatchesAllOriginalContracts(t *testing.T) {
	for _, descriptor := range RuntimeOperationDescriptors() {
		contract, _ := RuntimeOperationContractForKind(descriptor.Kind)
		variants := []string{"", " unknown ", "\x00", "file_write", "FILE_WRITE"}
		for _, variant := range contract.Variants {
			variants = append(variants, variant.ID, " "+variant.ID+" ")
		}
		for _, variantID := range variants {
			assertOperationVariantIdentityReference(t, descriptor.Kind, variantID)
			assertOperationVariantIdentityReference(t, " "+descriptor.Kind+" ", variantID)
		}
	}
	for _, kind := range []string{"", "unknown", "file.read", "patch_apply", string([]byte{0xff, 0x80})} {
		assertOperationVariantIdentityReference(t, kind, "file_write")
	}
}

func TestOperationVariantIdentityKeepsOtherOwnerAndPolicyGates(t *testing.T) {
	for _, scenario := range []string{"valid", "owner", "kind", "policy", "selector", "constraint", "prospective"} {
		value := RuntimeOperationAdmissibilityVariant{Kind: RuntimeBlockKindPatchApply, VariantID: "file_write", AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope}, SelectorRefs: []string{"selector"}, ConstraintRefs: []string{"constraint"}}
		owner := RuntimeModuleFile
		switch scenario {
		case "owner":
			owner = RuntimeModuleShell
		case "kind":
			value.Kind = "unknown"
		case "policy":
			value.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{"unknown"}
		case "selector":
			value.SelectorRefs = []string{"other"}
		case "constraint":
			value.ConstraintRefs = []string{"other"}
		case "prospective":
			value.AllowedTargetPolicies, value.SelectorRefs = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}, nil
		}
		want := referenceRuntimeOperationAdmissibilityVariantValidate(value, owner, []string{RuntimeBlockKindPatchApply}, []string{"selector"}, []string{"constraint"})
		if (want == nil) != (scenario == "valid") {
			t.Fatal("owner/policy 反例未命中")
		}
		assertProofCollectionIdentityErrors(t, value.Validate(owner, []string{RuntimeBlockKindPatchApply}, []string{"selector"}, []string{"constraint"}), want)
	}
	value := operationVariantIdentityConvergenceFixture(RuntimeBlockKindPatchApply, "file_write")
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.AcquisitionOperationVariantID = "unknown"
	if value.Validate() == nil {
		t.Fatal("后续 owner-fact variant 漂移借用先前成功")
	}
	assertProofCollectionIdentityErrors(t, value.Validate(), referenceRuntimeOwnerFactConvergenceValidate(value))
}

func TestOperationVariantIdentityIsIndependentOfPublicCopiesAndConcurrent(t *testing.T) {
	assertOperationVariantIdentityReference(t, RuntimeBlockKindPatchApply, "file_write")
	contract, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
	contract.Variants[0].ID = "tampered"
	if known, hasVariants, contains := runtimeOperationVariantIdentity(RuntimeBlockKindPatchApply, "file_write"); !known || !hasVariants || !contains {
		t.Fatal("公开合同副本修改影响静态身份")
	}
	if _, _, contains := runtimeOperationVariantIdentity(RuntimeBlockKindPatchApply, "tampered"); contains {
		t.Fatal("公开合同副本污染静态身份")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 32 {
				assertOperationVariantIdentityReference(t, RuntimeBlockKindPatchApply, "unified_diff")
			}
		})
	}
	wg.Wait()
	original := testing.AllocsPerRun(20, func() { _, _ = RuntimeOperationContractForKind(RuntimeBlockKindPatchApply) })
	current := testing.AllocsPerRun(20, func() { _, _, _ = runtimeOperationVariantIdentity(RuntimeBlockKindPatchApply, "file_write") })
	if current != 0 || current >= original {
		t.Fatalf("纯静态身份查询仍复制完整合同：current=%v original=%v", current, original)
	}
}

func FuzzOperationVariantIdentityReference(f *testing.F) {
	f.Add(RuntimeBlockKindPatchApply, " file_write ")
	f.Add("unknown", string([]byte{0xff, 0x80}))
	f.Add(RuntimeBlockKindManagedValidationSession, "")
	f.Fuzz(func(t *testing.T, kind, variantID string) {
		assertOperationVariantIdentityReference(t, kind, variantID)
	})
}

func BenchmarkOperationVariantIdentity(b *testing.B) {
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					contract, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
					_ = referenceRuntimeSemanticVariantExists(contract, "file_write")
				} else {
					_, _, _ = runtimeOperationVariantIdentity(RuntimeBlockKindPatchApply, "file_write")
				}
			}
		})
	}
}
