package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 只映射本次实际 owner 执行事实。新 adapter 尚未公开；raw bytes 留在
// 原 owner 事实中，不进入 Runtime aggregate，也不补造持久原文读取回执。
func shellProjectConjunctionObservation(fact shellProjectConjunctionFact, expected []shellProjectStageSpec, diagnosticSpecs ...shellProjectStageSpec) (runtimecontract.RuntimeValidationExecutionConjunction, error) {
	if len(expected) != 4 || len(fact.Stages) == 0 || len(fact.Stages) > runtimecontract.RuntimeValidationExecutionMaxStages || !shellProjectObservationProtocol(fact.ProtocolRef) {
		return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("复合验证缺少准确的四阶段合同或实际执行事实")
	}
	var identities []runtimecontract.RuntimeValidationExecutionStageIdentity
	for index, spec := range expected {
		identities = append(identities, runtimecontract.RuntimeValidationExecutionStageIdentity{ID: spec.ID, Scope: spec.Scope, RequireTests: index > 0, RequireBehavior: index == 3})
	}
	ret := runtimecontract.RuntimeValidationExecutionConjunction{ProtocolRef: fact.ProtocolRef, RequiredStagesHash: runtimecontract.StableRuntimeValidationExecutionStagesHash(identities), Complete: fact.Complete, BehaviorTargetRefs: append([]string(nil), fact.BehaviorTargetRefs...)}
	for index, stage := range fact.Stages {
		identity, spec := identities[min(index, 3)], expected[min(index, 3)]
		if strings.HasPrefix(stage.ID, "project-diagnostic-") {
			if fact.ProtocolRef != shellProjectMultiFailureDiagnosticProtocolRef || fact.Complete || index < 2 || index != 2 && index != 3 && !strings.HasPrefix(fact.Stages[index-1].ID, "project-diagnostic-") {
				return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("multi-source diagnostic lacks failed project prefix")
			}
			prefix := index
			for prefix > 0 && strings.HasPrefix(fact.Stages[prefix-1].ID, "project-diagnostic-") {
				prefix--
			}
			if prefix != 2 && prefix != 3 || !fact.Stages[prefix-1].KnownFailure || fact.Stages[prefix-1].ID != "project-test" && fact.Stages[prefix-1].ID != "project-race" {
				return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("multi-source diagnostic lacks exact failure")
			}
			found := false
			for _, candidate := range diagnosticSpecs {
				if candidate.ID == stage.ID {
					if found {
						return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("ambiguous diagnostic stage")
					}
					spec, found = candidate, true
				}
			}
			if !found {
				return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("diagnostic stage has no pre-execution specification")
			}
			identity = runtimecontract.RuntimeValidationExecutionStageIdentity{ID: spec.ID, Scope: spec.Scope, RequireTests: true, RequireBehavior: true}
		} else if index >= len(expected) {
			return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("unknown additional execution stage")
		}
		if stage.ID == shellProjectFailureDiagnosticStageID {
			if fact.ProtocolRef != shellProjectFailureDiagnosticProtocolRef || fact.Complete || index != len(fact.Stages)-1 || index != 2 && index != 3 || !fact.Stages[index-1].KnownFailure || fact.Stages[index-1].ID != "project-test" && fact.Stages[index-1].ID != "project-race" {
				return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("diagnostic stage lacks exact failed project prefix")
			}
			identity, spec = identities[3], expected[3]
			identity.ID = shellProjectFailureDiagnosticStageID
		}
		if stage.ID != identity.ID || stage.Scope != identity.Scope || !slices.Equal(stage.Argv, append([]string{"go"}, spec.Args...)) {
			return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("实际阶段身份与原四阶段合同不一致")
		}
		for _, raw := range []shellProjectRawFact{stage.Stdout, stage.Stderr} {
			digest := sha256.Sum256(raw.Raw)
			if raw.SavedBytes != int64(len(raw.Raw)) || raw.Complete && !shellProjectRawFactValid(raw) || raw.Bytes == raw.SavedBytes && raw.SHA256 != hex.EncodeToString(digest[:]) {
				return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("实际 raw bytes 与输出身份不一致")
			}
		}
		ret.Stages = append(ret.Stages, runtimecontract.RuntimeValidationExecutionStage{
			Identity: identity, Argv: append([]string(nil), stage.Argv...), StartedAt: stage.StartedAt, FinishedAt: stage.FinishedAt,
			ExitCode: int64(stage.ExitCode), Started: stage.Started, TimedOut: stage.TimedOut, CleanupFailed: stage.CleanupFailed,
			Stdout: shellProjectOutputIdentity(stage.Stdout), Stderr: shellProjectOutputIdentity(stage.Stderr),
			SourceBefore: stage.SourceBefore, SourceAfter: stage.SourceAfter, Tests: stage.Tests, PassedTests: stage.PassedTests,
			TerminalComplete: stage.TerminalComplete, KnownFailure: stage.KnownFailure, FailureCode: stage.FailureCode,
			DiagnosticSourceRef: stage.DiagnosticSourceRef, BehaviorTargetRefs: slices.Clone(stage.BehaviorTargetRefs),
		})
	}
	if err := ret.Validate(); err != nil {
		return runtimecontract.RuntimeValidationExecutionConjunction{}, err
	}
	return ret, nil
}

func shellProjectOutputIdentity(raw shellProjectRawFact) runtimecontract.RuntimeValidationOutputIdentity {
	return runtimecontract.RuntimeValidationOutputIdentity{Bytes: raw.Bytes, SHA256: raw.SHA256, SavedBytes: raw.SavedBytes, Complete: raw.Complete, OmittedReason: raw.OmittedReason}
}
