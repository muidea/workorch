package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
)

// RuntimeValidationExecutionPlan is the validation owner's declared method at
// the input observation boundary. It is not execution evidence or a scheduler.
// Args exclude private scratch paths; Instrumentation names those additional
// owner observations explicitly instead of pretending they are the same run.
type RuntimeValidationExecutionPlan struct {
	TimeoutMillis int64
	Stages        []RuntimeValidationPlannedStage
}

type RuntimeValidationPlannedStage struct {
	Identity          RuntimeValidationExecutionStageIdentity
	Command           string
	Args              []string
	Instrumentation   []string
	RequiredCheckRefs []string
}

func (p RuntimeValidationExecutionPlan) Clone() RuntimeValidationExecutionPlan {
	p.Stages = slices.Clone(p.Stages)
	for i := range p.Stages {
		p.Stages[i].Args = slices.Clone(p.Stages[i].Args)
		p.Stages[i].Instrumentation = slices.Clone(p.Stages[i].Instrumentation)
		p.Stages[i].RequiredCheckRefs = slices.Clone(p.Stages[i].RequiredCheckRefs)
	}
	return p
}

func (p RuntimeValidationExecutionPlan) validate(b RuntimeValidationProofRequirementBinding, recipe *RuntimeValidationRecipe) error {
	if len(p.Stages) == 0 || len(p.Stages) > RuntimeValidationExecutionMaxStages || p.TimeoutMillis < 0 || recipe == nil && p.TimeoutMillis != 0 || recipe != nil && p.TimeoutMillis != recipe.TimeoutMillis {
		return fmt.Errorf("validation execution plan lacks bounded stages or exact declared deadline")
	}
	var identities []RuntimeValidationExecutionStageIdentity
	var checks []string
	seen := map[string]bool{}
	for _, s := range p.Stages {
		if seen[s.Identity.ID] || !boundedValidationExecutionRef(s.Identity.ID, 512) || !boundedValidationExecutionRef(s.Identity.Scope, 1024) || !boundedValidationExecutionRef(s.Command, 512) || len(s.Args) == 0 || len(s.Args) > 64 || len(s.Instrumentation) > 8 || !reflect.DeepEqual(s.Instrumentation, normalizeStrings(s.Instrumentation)) || !reflect.DeepEqual(s.RequiredCheckRefs, normalizeStrings(s.RequiredCheckRefs)) {
			return fmt.Errorf("validation execution plan has invalid stage method or membership")
		}
		bytes := 0
		for _, arg := range s.Args {
			bytes += len(arg)
			if !boundedValidationExecutionRef(arg, 4096) {
				return fmt.Errorf("validation execution plan has invalid argv")
			}
		}
		if bytes > 16*1024 {
			return fmt.Errorf("validation execution plan argv exceeds capacity")
		}
		for _, mode := range s.Instrumentation {
			if !boundedValidationExecutionRef(mode, 512) {
				return fmt.Errorf("validation execution plan has invalid instrumentation")
			}
		}
		for _, ref := range s.RequiredCheckRefs {
			if !slices.Contains(b.RequiredCheckRefs, ref) || !s.Identity.RequireTests {
				return fmt.Errorf("validation execution plan claims an unbound check")
			}
		}
		checks = append(checks, s.RequiredCheckRefs...)
		identities = append(identities, s.Identity)
		seen[s.Identity.ID] = true
	}
	if !reflect.DeepEqual(normalizeStrings(checks), normalizeStrings(b.RequiredCheckRefs)) {
		return fmt.Errorf("validation execution plan omits required checks")
	}
	hash := StableRuntimeValidationExecutionStagesHash(identities)
	for _, check := range b.CheckAttestations {
		if check.ExecutionStagesHash != hash {
			return fmt.Errorf("validation execution plan differs from sealed all-of")
		}
	}
	return nil
}
