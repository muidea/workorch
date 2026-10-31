package shell

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
)

// 此文件只提供尚未接线的 Shell-private 执行事实生产器。它不发布
// Runtime proof、receipt 或新 adapter availability；生产接线必须另行完成。
const shellProjectConjunctionProtocolRef = "shell/go-test-json/project-conjunction-behavior"

const shellProjectConjunctionReviewScopeRequirements = "Shell 在一个隔离副本和一个原 operation deadline 内按固定 all_of 执行 go build ./...、go test -json -count=1 ./...、go test -race -json -count=1 ./...，最后独立执行当前 CWI 准确评审的原始/补充 InputV4 检查及实际实现行为观测。前三阶段覆盖整个根 Go module 的 ./...，包括旧输入和相邻 CWI harness；它们不是当前 CWI 语义证明，也不从当前 CWI 的评审推断相邻输入已评审。任何失败、跳过的测试、未结束事件、零实际测试或不完整输出均不能完成合取。完整 source/member set 的执行前后 hash 必须一致；GOWORK=off。评审字节仍必须包含当前 CWI 完整 package/helpers/fixtures 和根 module/workspace 定义，不提高 16 files / 64 KiB 边界。依赖未评审外部 harness/fixtures 才能解释当前准则的输入不能判为 consistent；必须明确缺失上下文或超限不可用。所有原 Goal/DoD 准则保持 mandatory，完整项目执行不代替逐 CWI 行为断言、一致性判断或验收 credit。"

type shellProjectStageSpec struct {
	ID    string
	Scope string
	Args  []string
	JSON  bool
}

// facts 中 argv 是实际 OS 进程的准确参数，不是 template/provider 的命令标签。
// 原 bytes 仅留在 owner-local 返回值；公开接线只能传有界事实及原始身份。
type shellProjectStageFact struct {
	ID, Scope                    string
	Argv                         []string
	StartedAt, FinishedAt        string
	ExitCode                     int
	Started, TimedOut            bool
	CleanupFailed                bool
	Stdout, Stderr               shellProjectRawFact
	Packages, Tests, PassedTests int
	TerminalComplete             bool
	KnownFailure                 bool
	FailureCode                  string
	SourceBefore, SourceAfter    string
	DiagnosticSourceRef          string   `json:",omitempty"`
	BehaviorTargetRefs           []string `json:",omitempty"`
}

type shellProjectRawFact struct {
	Bytes         int64
	SHA256        string
	SavedBytes    int64
	Complete      bool
	OmittedReason string
	Raw           []byte
}

type shellProjectConjunctionFact struct {
	ProtocolRef        string
	Stages             []shellProjectStageFact
	BehaviorTargetRefs []string
	Complete           bool
	FailureCode        string
	CommitmentHash     string
}

// 原 bytes 不清洗、不摘要；hash 涵盖全部接收 bytes，即使存储边界已满。
// 超界只保留明确标记的前缀，绝不把它标成完整诊断或通过事实。
type shellProjectRawBuffer struct {
	buf   bytes.Buffer
	hash  hash.Hash
	bytes int64
}

func newShellProjectRawBuffer() *shellProjectRawBuffer {
	return &shellProjectRawBuffer{hash: sha256.New()}
}

func (b *shellProjectRawBuffer) Write(p []byte) (int, error) {
	n := len(p)
	b.bytes += int64(n)
	_, _ = b.hash.Write(p)
	if remaining := MaxCommandOutputBytes - b.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buf.Write(p)
	}
	return n, nil
}

func (b *shellProjectRawBuffer) fact() shellProjectRawFact {
	fact := shellProjectRawFact{Bytes: b.bytes, SHA256: hex.EncodeToString(b.hash.Sum(nil)), SavedBytes: int64(b.buf.Len()), Complete: b.bytes == int64(b.buf.Len()), Raw: append([]byte(nil), b.buf.Bytes()...)}
	if !fact.Complete {
		fact.OmittedReason = "shell_stage_raw_output_capacity_exceeded"
	}
	return fact
}

// 与旧 parser 独立，不改变已封存旧协议的语义。完整性严格绑定 package/test
// identity：每个 package 必须 start 后 terminal，每个实际 test 必须 run 后
// terminal。无测试 package 的 skip 可结束 package，但任何 test skip 均拒绝。
func shellProjectJSONCompletion(stdout []byte, complete bool) (packages, tests, passed int, terminal, failed bool, code string) {
	type member struct {
		started bool
		ended   bool
	}
	pkgs, cases := map[string]member{}, map[string]member{}
	invalid, skipped := !complete, false
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 4096), MaxCommandOutputBytes+1)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event shellGoTestJSONEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			invalid = true
			continue
		}
		if event.Action == "fail" || event.Action == "build-fail" {
			failed = true
		}
		pkg := event.Package
		if pkg == "" && (event.Action == "build-output" || event.Action == "build-fail") {
			pkg = event.ImportPath
		}
		if pkg == "" || strings.TrimSpace(pkg) != pkg || len(pkg) > 1024 || len(event.Test) > 2048 {
			invalid = true
			continue
		}
		if len(pkgs) > maxNativeCheckSourceFiles || len(cases) > maxNativeCheckWalkNodes {
			invalid = true
			continue
		}
		p := pkgs[pkg]
		if event.Test == "" {
			switch event.Action {
			case "start":
				if p.started || p.ended {
					invalid = true
				}
				p.started = true
			case "pass", "fail", "skip":
				if !p.started || p.ended {
					invalid = true
				}
				p.ended = true
			case "output":
				if !p.started || p.ended {
					invalid = true
				}
			case "build-output", "build-fail":
				// 编译失败仍是已知失败，不能被缺少 package start 覆盖。
			default:
				invalid = true
			}
			pkgs[pkg] = p
			continue
		}
		if !p.started || p.ended {
			invalid = true
		}
		key := pkg + "\x00" + event.Test
		c := cases[key]
		switch event.Action {
		case "run":
			if c.started || c.ended {
				invalid = true
			}
			c.started = true
		case "pass", "fail", "skip":
			if !c.started || c.ended {
				invalid = true
			}
			if event.Action == "pass" && c.started && !c.ended {
				passed++
			}
			c.ended = true
			skipped = skipped || event.Action == "skip"
		case "pause", "cont", "output":
			if !c.started || c.ended {
				invalid = true
			}
		default:
			invalid = true
		}
		cases[key] = c
	}
	invalid = invalid || scanner.Err() != nil || len(pkgs) > maxNativeCheckSourceFiles || len(cases) > maxNativeCheckWalkNodes
	for _, p := range pkgs {
		invalid = invalid || !p.started || !p.ended
	}
	for _, c := range cases {
		invalid = invalid || !c.started || !c.ended
		if c.started {
			tests++
		}
	}
	packages = len(pkgs)
	terminal = !invalid && packages > 0
	switch {
	case failed:
		code = "validation_project_test_failed"
	case invalid || packages == 0:
		code = "validation_project_test_transcript_incomplete"
	case skipped:
		code = "validation_project_test_skipped"
	case tests == 0 || passed == 0:
		code = "validation_project_test_empty"
	}
	return
}

func shellProjectFixedStages(exactArgs []string, pkg string) []shellProjectStageSpec {
	return []shellProjectStageSpec{
		{ID: "project-build", Scope: "root-module:./...", Args: []string{"build", "./..."}},
		{ID: "project-test", Scope: "root-module:./...", Args: []string{"test", "-json", "-count=1", "./..."}, JSON: true},
		{ID: "project-race", Scope: "root-module:./...", Args: []string{"test", "-race", "-json", "-count=1", "./..."}, JSON: true},
		{ID: "reviewed-cwi-behavior", Scope: "reviewed-package:" + pkg, Args: append([]string(nil), exactArgs...), JSON: true},
	}
}

// caller 只能供应同一 operation 的 deadline，禁止无 deadline 或超过旧 60s
// 总预算。每次启动一个 owner process group；正常退出也清理后代。
func runShellProjectStage(ctx context.Context, directory string, env []string, spec shellProjectStageSpec) (shellProjectStageFact, error) {
	fact := shellProjectStageFact{ID: spec.ID, Scope: spec.Scope, Argv: append([]string{"go"}, spec.Args...), ExitCode: -1,
		Stdout: newShellProjectRawBuffer().fact(), Stderr: newShellProjectRawBuffer().fact()}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > DefaultCommandTimeout {
		fact.FailureCode = "validation_project_deadline_invalid"
		return fact, fmt.Errorf("复合验证要求不超过原 60000ms 的单一 deadline")
	}
	if err := ctx.Err(); err != nil {
		fact.TimedOut = err == context.DeadlineExceeded
		fact.FailureCode = "validation_project_context_expired"
		return fact, err
	}
	before, sourceErr := shellProjectSourceSetHash(directory)
	if sourceErr != nil {
		fact.FailureCode = "validation_project_source_unavailable"
		return fact, sourceErr
	}
	fact.SourceBefore = before
	stdout, stderr := newShellProjectRawBuffer(), newShellProjectRawBuffer()
	cmd := exec.CommandContext(ctx, "go", spec.Args...)
	cmd.Dir, cmd.Env = directory, env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return terminateShellProcessGroup(cmd.Process.Pid)
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdout, cmd.Stderr = stdout, stderr
	fact.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	err := cmd.Start()
	if err == nil {
		fact.Started = true
		err = cmd.Wait()
	}
	if cmd.Process != nil {
		cleanupErr := terminateShellProcessGroup(cmd.Process.Pid)
		fact.CleanupFailed = cleanupErr != nil
		if err == nil {
			err = cleanupErr
		}
	}
	fact.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if cmd.ProcessState != nil {
		fact.ExitCode = cmd.ProcessState.ExitCode()
	}
	fact.Stdout, fact.Stderr = stdout.fact(), stderr.fact()
	fact.SourceAfter, sourceErr = shellProjectSourceSetHash(directory)
	fact.TimedOut = ctx.Err() == context.DeadlineExceeded
	fact.KnownFailure = fact.Started && fact.ExitCode != 0
	if spec.JSON {
		var jsonFailed bool
		fact.Packages, fact.Tests, fact.PassedTests, fact.TerminalComplete, jsonFailed, fact.FailureCode = shellProjectJSONCompletion(fact.Stdout.Raw, fact.Stdout.Complete)
		fact.KnownFailure = fact.KnownFailure || jsonFailed
	} else {
		fact.TerminalComplete = fact.Started && cmd.ProcessState != nil
	}
	switch {
	case fact.TimedOut:
		fact.FailureCode = "validation_project_timeout"
	case fact.KnownFailure:
		fact.FailureCode = "validation_project_stage_failed"
	case !fact.Started:
		fact.FailureCode = "validation_project_process_unavailable"
	case fact.CleanupFailed:
		fact.FailureCode = "validation_project_cleanup_failed"
	case !fact.Stdout.Complete || !fact.Stderr.Complete:
		fact.FailureCode = "validation_project_output_incomplete"
	case sourceErr != nil || fact.SourceBefore != fact.SourceAfter:
		fact.FailureCode = "validation_project_source_changed"
	}
	if err == nil && fact.FailureCode != "" {
		err = fmt.Errorf("复合验证阶段 %s 拒绝：%s", fact.ID, fact.FailureCode)
	}
	return fact, err
}

// commit 仅对固定四阶段全部真实成功签署合取，包含实际 argv/scope、原 bytes
// 身份和 source pre/post。没有 stage 字符串/marker 的替代通过路径。
// 此私有事实不提供 dispatch/replay 权威；恢复必须使用原 Runtime aggregate/receipt。
func sealShellProjectConjunction(stages []shellProjectStageFact, expected []shellProjectStageSpec, behavior []string) shellProjectConjunctionFact {
	fact := shellProjectConjunctionFact{ProtocolRef: shellProjectConjunctionProtocolRef, Stages: stages, BehaviorTargetRefs: canonicalShellBehaviorTargets(behavior)}
	if len(stages) != 4 || len(expected) != 4 || len(fact.BehaviorTargetRefs) == 0 {
		fact.FailureCode = "validation_project_conjunction_incomplete"
	} else {
		for index, stage := range stages {
			if stage.ID != []string{"project-build", "project-test", "project-race", "reviewed-cwi-behavior"}[index] || stage.ID != expected[index].ID || stage.Scope != expected[index].Scope || strings.Join(stage.Argv, "\x00") != strings.Join(append([]string{"go"}, expected[index].Args...), "\x00") || !shellProjectRawFactValid(stage.Stdout) || !shellProjectRawFactValid(stage.Stderr) || (index > 0 && stage.SourceBefore != stages[index-1].SourceAfter) || !stage.Started || stage.ExitCode != 0 || stage.KnownFailure || stage.TimedOut || stage.CleanupFailed || !stage.Stdout.Complete || !stage.Stderr.Complete || !stage.TerminalComplete || stage.FailureCode != "" || stage.SourceBefore == "" || stage.SourceBefore != stage.SourceAfter || (index > 0 && (stage.Tests == 0 || stage.PassedTests == 0)) {
				fact.FailureCode = "validation_project_conjunction_incomplete"
				break
			}
		}
	}
	fact.Complete = fact.FailureCode == ""
	// Raw 不进入可公开 commitment，身份 bytes/hash 与 completeness 全部保留。
	fact.CommitmentHash = shellProjectConjunctionCommitmentHash(fact)
	return fact
}

func shellProjectConjunctionCommitmentHash(fact shellProjectConjunctionFact) string {
	preimage := fact
	preimage.CommitmentHash = ""
	preimage.Stages = append([]shellProjectStageFact(nil), fact.Stages...)
	for index := range preimage.Stages {
		preimage.Stages[index].Stdout.Raw = nil
		preimage.Stages[index].Stderr.Raw = nil
	}
	body, _ := json.Marshal(preimage)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func shellProjectRawFactValid(fact shellProjectRawFact) bool {
	if !fact.Complete || fact.Bytes != int64(len(fact.Raw)) || fact.SavedBytes != fact.Bytes || fact.OmittedReason != "" {
		return false
	}
	digest := sha256.Sum256(fact.Raw)
	return fact.SHA256 == hex.EncodeToString(digest[:])
}

// 完整副本的 member/content 身份包含旧输入、相邻 harness、fixtures 及
// 非 Go source，避免只核对当前 CWI 或漏掉 go.work.sum。沿用现 owner 上界，
// 不读其它 owner、不跳过现副本中参与执行的普通文件。
func shellProjectSourceSetHash(directory string) (string, error) {
	members, err := shellWorkspaceFileRevisions(directory, true)
	if err != nil {
		return "", err
	}
	refs := make([]string, 0, len(members))
	for ref := range members {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	h := sha256.New()
	_, _ = h.Write([]byte("shell-project-source-set/v1\x00"))
	for _, ref := range refs {
		_, _ = fmt.Fprintf(h, "%d:%s%d:%s", len(ref), ref, len(members[ref]), members[ref])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
