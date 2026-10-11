package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 只映射本次实际 owner 执行事实。新 adapter 尚未公开；raw bytes 留在
// 原 owner 事实中，不进入 Runtime aggregate，也不补造持久原文读取回执。
func shellProjectConjunctionObservation(fact shellProjectConjunctionFact, expected []shellProjectStageSpec) (runtimecontract.RuntimeValidationExecutionConjunction, error) {
	if len(expected) != 4 || len(fact.Stages) == 0 || len(fact.Stages) > len(expected) || fact.ProtocolRef != shellProjectConjunctionProtocolRef {
		return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("复合验证缺少准确的四阶段合同或实际执行事实")
	}
	var identities []runtimecontract.RuntimeValidationExecutionStageIdentity
	for index, spec := range expected {
		identities = append(identities, runtimecontract.RuntimeValidationExecutionStageIdentity{ID: spec.ID, Scope: spec.Scope, RequireTests: index > 0, RequireBehavior: index == 3})
	}
	ret := runtimecontract.RuntimeValidationExecutionConjunction{ProtocolRef: fact.ProtocolRef, RequiredStagesHash: runtimecontract.StableRuntimeValidationExecutionStagesHash(identities), Complete: fact.Complete, BehaviorTargetRefs: append([]string(nil), fact.BehaviorTargetRefs...)}
	for index, stage := range fact.Stages {
		if stage.ID != identities[index].ID || stage.Scope != identities[index].Scope || !slices.Equal(stage.Argv, append([]string{"go"}, expected[index].Args...)) {
			return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("实际阶段身份与原四阶段合同不一致")
		}
		for _, raw := range []shellProjectRawFact{stage.Stdout, stage.Stderr} {
			digest := sha256.Sum256(raw.Raw)
			if raw.SavedBytes != int64(len(raw.Raw)) || raw.Complete && !shellProjectRawFactValid(raw) || raw.Bytes == raw.SavedBytes && raw.SHA256 != hex.EncodeToString(digest[:]) {
				return runtimecontract.RuntimeValidationExecutionConjunction{}, fmt.Errorf("实际 raw bytes 与输出身份不一致")
			}
		}
		ret.Stages = append(ret.Stages, runtimecontract.RuntimeValidationExecutionStage{
			Identity: identities[index], Argv: append([]string(nil), stage.Argv...), StartedAt: stage.StartedAt, FinishedAt: stage.FinishedAt,
			ExitCode: int64(stage.ExitCode), Started: stage.Started, TimedOut: stage.TimedOut, CleanupFailed: stage.CleanupFailed,
			Stdout: shellProjectOutputIdentity(stage.Stdout), Stderr: shellProjectOutputIdentity(stage.Stderr),
			SourceBefore: stage.SourceBefore, SourceAfter: stage.SourceAfter, Tests: stage.Tests, PassedTests: stage.PassedTests,
			TerminalComplete: stage.TerminalComplete, KnownFailure: stage.KnownFailure, FailureCode: stage.FailureCode,
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
