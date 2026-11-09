package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"reflect"
	"slices"
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

// 引用由 validation owner 发布；不包含目录或原文，也不授予调用方读取权限。
// 未保存原因与输出容量不足是不同事实，不能把完整接收误标为完整持久保存。
type RuntimeValidationOutputStorage struct {
	Reference     string
	OmittedReason string
}

type RuntimeValidationOutputIdentity struct {
	Bytes         int64
	SHA256        string
	SavedBytes    int64
	Complete      bool
	OmittedReason string
	Storage       *RuntimeValidationOutputStorage `json:",omitempty"`
}

type RuntimeValidationExecutionStage struct {
	Identity            RuntimeValidationExecutionStageIdentity
	Argv                []string
	StartedAt           string
	FinishedAt          string
	ExitCode            int64
	Started             bool
	TimedOut            bool
	CleanupFailed       bool
	Stdout              RuntimeValidationOutputIdentity
	Stderr              RuntimeValidationOutputIdentity
	SourceBefore        string
	SourceAfter         string
	Tests               int
	PassedTests         int
	TerminalComplete    bool
	KnownFailure        bool
	FailureCode         string
	DiagnosticSourceRef string `json:",omitempty"`
	// Paired owner provenance permits overlapping diagnostic copies after an
	// exact failed original. It never completes or reorders ordinary all-of.
	DiagnosticOriginStageID string                             `json:",omitempty"`
	DiagnosticIsolationRef  string                             `json:",omitempty"`
	BehaviorTargetRefs      []string                           `json:",omitempty"`
	NativeExecutions        []RuntimeValidationNativeExecution `json:",omitempty"`
	NativeSourceFailure     string                             `json:",omitempty"`
}

// Capability-owner controlled child execution, bound before output is shared.
// These are observations within the original stage, not additional all-of
// stages, dispatches, proof credit or retry budgets.
type RuntimeValidationNativeExecution struct {
	SourceRef                                 string
	NativeCaseRef                             string
	ParentExecutableSHA256                    string
	Argv                                      []string
	StartedAt, FinishedAt                     string
	ExitCode                                  int64
	TimedOut, CleanupFailed, TerminalComplete bool
	Stdout, Stderr                            RuntimeValidationOutputIdentity
}

func (n RuntimeValidationNativeExecution) Clone() RuntimeValidationNativeExecution {
	n.Argv = slices.Clone(n.Argv)
	n.Stdout = n.Stdout.Clone()
	n.Stderr = n.Stderr.Clone()
	return n
}

func (n RuntimeValidationNativeExecution) Validate() error {
	start, e1 := time.Parse(time.RFC3339Nano, n.StartedAt)
	end, e2 := time.Parse(time.RFC3339Nano, n.FinishedAt)
	if !boundedValidationExecutionRef(n.SourceRef, 512) || !boundedValidationExecutionRef(n.NativeCaseRef, 2048) || !validationExecutionDigest(n.ParentExecutableSHA256) || len(n.Argv) == 0 || len(n.Argv) > 64 || e1 != nil || e2 != nil || end.Before(start) || n.ExitCode < -1 || n.Stdout.Validate() != nil || n.Stderr.Validate() != nil {
		return fmt.Errorf("native execution identity or output invalid")
	}
	bytes := 0
	for _, arg := range n.Argv {
		bytes += len(arg)
		if !boundedValidationExecutionRef(arg, 4096) {
			return fmt.Errorf("native execution argv invalid")
		}
	}
	if bytes > 16*1024 {
		return fmt.Errorf("native execution argv over capacity")
	}
	return nil
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
			c.Stages[index].NativeExecutions = slices.Clone(c.Stages[index].NativeExecutions)
			for child := range c.Stages[index].NativeExecutions {
				c.Stages[index].NativeExecutions[child] = c.Stages[index].NativeExecutions[child].Clone()
			}
			c.Stages[index].BehaviorTargetRefs = slices.Clone(c.Stages[index].BehaviorTargetRefs)
			if c.Stages[index].Argv != nil {
				c.Stages[index].Argv = append(make([]string, 0, len(c.Stages[index].Argv)), c.Stages[index].Argv...)
			}
			c.Stages[index].Stdout = c.Stages[index].Stdout.Clone()
			c.Stages[index].Stderr = c.Stages[index].Stderr.Clone()
		}
	}
	if c.BehaviorTargetRefs != nil {
		c.BehaviorTargetRefs = append(make([]string, 0, len(c.BehaviorTargetRefs)), c.BehaviorTargetRefs...)
	}
	return c
}

func (o RuntimeValidationOutputIdentity) Clone() RuntimeValidationOutputIdentity {
	if o.Storage != nil {
		value := *o.Storage
		o.Storage = &value
	}
	return o
}

func (o RuntimeValidationOutputIdentity) Validate() error {
	if o.Bytes < 0 || o.SavedBytes < 0 || o.SavedBytes > o.Bytes || !validationExecutionDigest(o.SHA256) ||
		(o.Complete && (o.Bytes != o.SavedBytes || o.OmittedReason != "")) ||
		(!o.Complete && !boundedValidationExecutionRef(o.OmittedReason, 512)) {
		return fmt.Errorf("validation execution output identity or completeness invalid")
	}
	if o.Storage != nil {
		if o.Storage.Reference == "" {
			if !boundedValidationExecutionRef(o.Storage.OmittedReason, 512) {
				return fmt.Errorf("validation execution output storage requires reference or omission reason")
			}
		} else if !boundedValidationExecutionRef(o.Storage.Reference, 1024) || o.Storage.OmittedReason != "" {
			return fmt.Errorf("validation execution output storage reference invalid")
		}
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
	storageDeclared := false
	for _, stage := range c.Stages {
		storageDeclared = storageDeclared || stage.Stdout.Storage != nil || stage.Stderr.Storage != nil
	}
	seen := make(map[string]bool)
	diagnosticSources := make(map[string]bool)
	isolations := map[string]bool{}
	origins := map[string]RuntimeValidationExecutionStage{}
	var identities []RuntimeValidationExecutionStageIdentity
	var previousEnd time.Time
	for index, stage := range c.Stages {
		if len(stage.NativeExecutions) > RuntimeValidationDiagnosticSourceLimit || stage.NativeSourceFailure != "" && !boundedValidationExecutionRef(stage.NativeSourceFailure, 512) {
			return fmt.Errorf("native execution capacity or failure invalid")
		}
		children := map[string]bool{}
		for _, child := range stage.NativeExecutions {
			if child.Validate() != nil || children[child.SourceRef] {
				return fmt.Errorf("native execution invalid or duplicate")
			}
			children[child.SourceRef] = true
			start, e1 := time.Parse(time.RFC3339Nano, stage.StartedAt)
			end, e2 := time.Parse(time.RFC3339Nano, stage.FinishedAt)
			a, _ := time.Parse(time.RFC3339Nano, child.StartedAt)
			b, _ := time.Parse(time.RFC3339Nano, child.FinishedAt)
			if e1 != nil || e2 != nil || a.Before(start) || b.After(end) {
				return fmt.Errorf("native execution outside parent stage")
			}
			if c.Complete && (child.ExitCode != 0 || child.TimedOut || child.CleanupFailed || !child.TerminalComplete || !child.Stdout.Complete || !child.Stderr.Complete) {
				return fmt.Errorf("complete execution has incomplete native child")
			}
			if c.Complete && storageDeclared && (child.Stdout.Storage == nil || child.Stderr.Storage == nil || child.Stdout.Storage.Reference == "" || child.Stderr.Storage.Reference == "") {
				return fmt.Errorf("native execution raw storage missing")
			}
		}
		if c.Complete && stage.NativeSourceFailure != "" {
			return fmt.Errorf("complete execution has native source failure")
		}
		if stage.DiagnosticIsolationRef != "" || stage.DiagnosticOriginStageID != "" {
			origin, found := origins[stage.DiagnosticOriginStageID]
			start, e1 := time.Parse(time.RFC3339Nano, stage.StartedAt)
			end, e2 := time.Parse(time.RFC3339Nano, origin.FinishedAt)
			finished, e3 := time.Parse(time.RFC3339Nano, stage.FinishedAt)
			if !boundedValidationExecutionRef(stage.DiagnosticIsolationRef, 512) || isolations[stage.DiagnosticIsolationRef] || !found || !stage.Started || origin.DiagnosticOriginStageID != "" || origin.DiagnosticSourceRef != "" || origin.BehaviorTargetRefs != nil || !origin.KnownFailure || !origin.Started || !origin.TerminalComplete || origin.CleanupFailed || !origin.Stdout.Complete || !origin.Stderr.Complete || origin.SourceBefore == "" || origin.SourceBefore != origin.SourceAfter || origin.ExitCode == 0 || origin.TimedOut || origin.SourceAfter != stage.SourceBefore || e1 != nil || e2 != nil || e3 != nil || finished.Before(start) || start.Before(end) || c.Complete {
				return fmt.Errorf("isolated diagnostic requires a unique copy after the exact original failure")
			}
			isolations[stage.DiagnosticIsolationRef] = true
		}
		origins[stage.Identity.ID] = stage
		if stage.DiagnosticSourceRef != "" || stage.BehaviorTargetRefs != nil {
			if c.Complete || !stage.Started || stage.ExitCode == 0 || !stage.KnownFailure || stage.TimedOut || stage.CleanupFailed || !stage.TerminalComplete || !stage.Stdout.Complete || !stage.Stderr.Complete || stage.SourceBefore == "" || stage.SourceBefore != stage.SourceAfter || !boundedValidationExecutionRef(stage.DiagnosticSourceRef, 512) || len(stage.BehaviorTargetRefs) == 0 || len(stage.BehaviorTargetRefs) > 64 || !reflect.DeepEqual(stage.BehaviorTargetRefs, normalizeStrings(stage.BehaviorTargetRefs)) {
				return fmt.Errorf("diagnostic execution requires bounded source-specific behavior, never complete proof")
			}
			for _, ref := range stage.BehaviorTargetRefs {
				if !boundedValidationExecutionRef(ref, 1024) {
					return fmt.Errorf("invalid diagnostic behavior target")
				}
			}
			start, startErr := time.Parse(time.RFC3339Nano, stage.StartedAt)
			end, endErr := time.Parse(time.RFC3339Nano, stage.FinishedAt)
			if diagnosticSources[stage.DiagnosticSourceRef] || startErr != nil || endErr != nil || end.Before(start) || index == 0 {
				return fmt.Errorf("diagnostic execution lacks unique source or ordered original failure")
			}
			prior := c.Stages[index-1]
			priorEnd, priorErr := time.Parse(time.RFC3339Nano, prior.FinishedAt)
			if stage.DiagnosticIsolationRef == "" && (priorErr != nil || start.Before(priorEnd) || stage.SourceBefore != prior.SourceAfter) {
				return fmt.Errorf("diagnostic execution changed original source or stage order")
			}
			// Publication may fail after real attribution. Preserve that history
			// with explicit output omission; the failure-authority consumer below
			// independently requires every original stream to be durably stored.
			diagnosticSources[stage.DiagnosticSourceRef] = true
		}
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
		if storageDeclared && (stage.Stdout.Storage == nil || stage.Stderr.Storage == nil || stage.Stdout.Storage.Reference == "" || stage.Stderr.Storage.Reference == "") {
			return fmt.Errorf("complete validation execution conjunction lacks declared output storage")
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
