package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

const RuntimeValidationExecutionMaxStages = 8

// 阶段身份由 validation owner 声明；Core 不解释命令、语言或业务判据。
// 顺序也是 all-of 合同的一部分，不允许把集合排序后替换原执行次序。
type RuntimeValidationExecutionStageIdentity struct {
	ID              string
	Scope           string
	RequireTests    bool
	RequireBehavior bool
}

type RuntimeValidationOutputIdentity struct {
	Bytes         int64
	SHA256        string
	SavedBytes    int64
	Complete      bool
	OmittedReason string
}

type RuntimeValidationExecutionStage struct {
	Identity         RuntimeValidationExecutionStageIdentity
	Argv             []string
	StartedAt        string
	FinishedAt       string
	ExitCode         int64
	Started          bool
	TimedOut         bool
	CleanupFailed    bool
	Stdout           RuntimeValidationOutputIdentity
	Stderr           RuntimeValidationOutputIdentity
	SourceBefore     string
	SourceAfter      string
	Tests            int
	PassedTests      int
	TerminalComplete bool
	KnownFailure     bool
	FailureCode      string
}

// 仅传有界实际执行事实和原输出身份，不传原诊断、句柄或第二份 credit。
// 不完整事实可保全在 rejected receipt；Complete 必须独立反证所有成员。
type RuntimeValidationExecutionConjunction struct {
	ProtocolRef        string
	RequiredStagesHash string
	Stages             []RuntimeValidationExecutionStage
	BehaviorTargetRefs []string
	Complete           bool
}

func boundedValidationExecutionRef(value string, bound int) bool {
	return value != "" && len(value) <= bound && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) < 0
}

func validationExecutionDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func StableRuntimeValidationExecutionStagesHash(stages []RuntimeValidationExecutionStageIdentity) string {
	h := sha256.New()
	writeHashString(h, "runtime-validation-execution-stages/v1")
	for _, stage := range stages {
		writeHashString(h, stage.ID)
		writeHashString(h, stage.Scope)
		writeHashString(h, fmt.Sprint(stage.RequireTests))
		writeHashString(h, fmt.Sprint(stage.RequireBehavior))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c RuntimeValidationExecutionConjunction) Clone() RuntimeValidationExecutionConjunction {
	if c.Stages != nil {
		c.Stages = append(make([]RuntimeValidationExecutionStage, 0, len(c.Stages)), c.Stages...)
		for index := range c.Stages {
			if c.Stages[index].Argv != nil {
				c.Stages[index].Argv = append(make([]string, 0, len(c.Stages[index].Argv)), c.Stages[index].Argv...)
			}
		}
	}
	if c.BehaviorTargetRefs != nil {
		c.BehaviorTargetRefs = append(make([]string, 0, len(c.BehaviorTargetRefs)), c.BehaviorTargetRefs...)
	}
	return c
}

func (o RuntimeValidationOutputIdentity) Validate() error {
	if o.Bytes < 0 || o.SavedBytes < 0 || o.SavedBytes > o.Bytes || !validationExecutionDigest(o.SHA256) ||
		(o.Complete && (o.Bytes != o.SavedBytes || o.OmittedReason != "")) ||
		(!o.Complete && !boundedValidationExecutionRef(o.OmittedReason, 512)) {
		return fmt.Errorf("validation execution output identity or completeness invalid")
	}
	return nil
}

func (c RuntimeValidationExecutionConjunction) Validate() error {
	if !boundedValidationExecutionRef(c.ProtocolRef, 512) || !validationExecutionDigest(c.RequiredStagesHash) || len(c.Stages) == 0 || len(c.Stages) > RuntimeValidationExecutionMaxStages || len(c.BehaviorTargetRefs) > 64 || !reflect.DeepEqual(c.BehaviorTargetRefs, normalizeStrings(c.BehaviorTargetRefs)) {
		return fmt.Errorf("validation execution conjunction identity or bounds invalid")
	}
	for _, ref := range c.BehaviorTargetRefs {
		if !boundedValidationExecutionRef(ref, 1024) {
			return fmt.Errorf("validation execution behavior target identity invalid")
		}
	}
	seen := make(map[string]bool)
	var identities []RuntimeValidationExecutionStageIdentity
	var previousEnd time.Time
	for index, stage := range c.Stages {
		argvBytes := 0
		for _, arg := range stage.Argv {
			argvBytes += len(arg)
		}
		if argvBytes > 16*1024 || len(stage.StartedAt) > 64 || len(stage.FinishedAt) > 64 {
			return fmt.Errorf("validation execution stage payload bound exceeded")
		}
		if !boundedValidationExecutionRef(stage.Identity.ID, 512) || !boundedValidationExecutionRef(stage.Identity.Scope, 1024) || seen[stage.Identity.ID] || len(stage.Argv) == 0 || len(stage.Argv) > 64 || stage.Tests < 0 || stage.PassedTests < 0 || stage.PassedTests > stage.Tests {
			return fmt.Errorf("validation execution stage identity, argv or counts invalid")
		}
		seen[stage.Identity.ID] = true
		identities = append(identities, stage.Identity)
		for _, arg := range stage.Argv {
			if !boundedValidationExecutionRef(arg, 4096) {
				return fmt.Errorf("validation execution stage argv invalid")
			}
		}
		if err := stage.Stdout.Validate(); err != nil {
			return err
		}
		if err := stage.Stderr.Validate(); err != nil {
			return err
		}
		if stage.FailureCode != "" && !boundedValidationExecutionRef(stage.FailureCode, 512) {
			return fmt.Errorf("validation execution stage failure code invalid")
		}
		if stage.SourceBefore != "" && !validationExecutionDigest(stage.SourceBefore) || stage.SourceAfter != "" && !validationExecutionDigest(stage.SourceAfter) {
			return fmt.Errorf("validation execution source identity invalid")
		}
		if !c.Complete {
			continue
		}
		start, startErr := time.Parse(time.RFC3339Nano, stage.StartedAt)
		end, endErr := time.Parse(time.RFC3339Nano, stage.FinishedAt)
		if startErr != nil || endErr != nil || end.Before(start) || index > 0 && start.Before(previousEnd) || !stage.Started || stage.ExitCode != 0 || stage.TimedOut || stage.CleanupFailed || stage.KnownFailure || stage.FailureCode != "" || !stage.TerminalComplete || !stage.Stdout.Complete || !stage.Stderr.Complete || stage.SourceBefore == "" || stage.SourceBefore != stage.SourceAfter || index > 0 && stage.SourceBefore != c.Stages[index-1].SourceAfter || stage.Identity.RequireTests && (stage.Tests == 0 || stage.PassedTests != stage.Tests) {
			return fmt.Errorf("complete validation execution conjunction has an incomplete stage")
		}
		if c.Complete && stage.Identity.RequireBehavior && len(c.BehaviorTargetRefs) == 0 {
			return fmt.Errorf("complete validation execution stage lacks required behavior")
		}
		previousEnd = end
	}
	if c.Complete && StableRuntimeValidationExecutionStagesHash(identities) != c.RequiredStagesHash {
		return fmt.Errorf("complete validation execution conjunction lacks exact all-of members or behavior")
	}
	return nil
}

func writeRuntimeValidationExecutionHash(h hash.Hash, observation RuntimeValidationObservation) {
	if observation.ExecutionConjunction != nil {
		writeHashString(h, "validation-execution-conjunction/v1")
		writeHashString(h, stableFormalValueHash("runtime-validation-execution-conjunction", *observation.ExecutionConjunction))
	}
}

func writeRuntimeValidationExecutionRequirementHash(h hash.Hash, attestation RuntimeValidationCheckAttestationBinding) {
	if attestation.ExecutionStagesHash != "" {
		writeHashString(h, "validation-execution-stages/v1")
		writeHashString(h, attestation.ExecutionStagesHash)
	}
}
