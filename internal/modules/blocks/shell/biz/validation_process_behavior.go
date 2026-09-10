package shell

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxShellProcessCoverageFiles = 128

func shellRequiresEntrypointBehavior(request RuntimeCommandRequest) bool {
	return len(request.ValidationCheckAttestations) != 0 && shellEntrypointObservationProtocol(request.ValidationCheckAttestations[0].ObservationProtocolRef)
}

func shellEntrypointHasSupplement(request RuntimeCommandRequest) bool {
	if !shellRequiresEntrypointBehavior(request) {
		return false
	}
	for _, check := range request.ValidationCheckAttestations {
		if check.SupplementalNativeCheckRef != "" {
			return true
		}
	}
	return false
}

// The fresh directory and conversion output belong to this one managed
// session. Neither the provider nor the test chooses an evidence artifact to
// import. GOFLAGS instruments Go children built by the harness; GOCOVERDIR
// collects executed binaries, not successful builds or stdout declarations.
func prepareShellProcessBehavior(request RuntimeCommandRequest, directory string) (string, error) {
	if !shellRequiresEntrypointBehavior(request) {
		return "", nil
	}
	if inspection := inspectShellNativeValidationChecks(directory, request.ValidationScopeTargetRefs, request.ValidationCheckAttestations); inspection.Code != "" {
		return "", fmt.Errorf("native entrypoint input is not ready: %s", inspection.Code)
	}
	directory, err := os.MkdirTemp(request.ValidationScratchDirectory, "process-behavior-")
	if err != nil {
		return "", err
	}
	// go test sets its own GOCOVERDIR immediately before starting each test
	// binary. Restore the session-owned child directory at that exact boundary,
	// without injecting a TestMain/init or changing any verification source.
	wrapper := filepath.Join(directory, "test-exec")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexec env GOCOVERDIR=\"${WORKORCH_PROCESS_COVERDIR:?}\" \"$@\"\n"), 0700); err != nil {
		return "", err
	}
	return directory, nil
}

func shellProcessBehaviorArgs(args []string, directory string) []string {
	if directory == "" {
		return args
	}
	return append([]string{args[0], "-exec=" + strconv.Quote(filepath.Join(directory, "test-exec"))}, args[1:]...)
}

func shellProcessBehaviorEnvironment(env []string, directory string, testArgs []string) []string {
	if directory == "" {
		return env
	}
	ret := make([]string, 0, len(env)+2)
	for _, value := range env {
		if strings.HasPrefix(value, "GOFLAGS=") || strings.HasPrefix(value, "GOCOVERDIR=") || strings.HasPrefix(value, "WORKORCH_PROCESS_COVERDIR=") {
			continue
		}
		ret = append(ret, value)
	}
	coverage := "-coverpkg=./..."
	for _, arg := range testArgs {
		if strings.HasPrefix(arg, "-coverpkg=") {
			coverage = arg
			break
		}
	}
	return append(ret, "GOFLAGS=-cover -covermode=count "+coverage, "GOCOVERDIR="+directory, "WORKORCH_PROCESS_COVERDIR="+directory)
}

// This observation supplies execution grounding only. The native test must
// still pass its actual protocol/semantic assertions. Non-Go backends may run
// normally but their execution is not Go workspace implementation coverage.
func shellProcessBehaviorTargets(ctx context.Context, command, workspace, directory string, env, refs []string) ([]string, error) {
	if directory == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	if len(entries) == 1 && entries[0].Name() == "test-exec" {
		return nil, nil // An in-process test need not create a child process.
	}
	if len(entries) > maxShellProcessCoverageFiles {
		return nil, fmt.Errorf("process behavior observation exceeds file bound")
	}
	var bytes int64
	metadata, counters := false, false
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("process behavior observation contains a non-regular artifact")
		}
		if entry.Name() == "test-exec" {
			continue
		}
		bytes += info.Size()
		if bytes > maxNativeCheckSourceBytes {
			return nil, fmt.Errorf("process behavior observation exceeds byte bound")
		}
		switch {
		case strings.HasPrefix(entry.Name(), "covmeta."):
			metadata = true
		case strings.HasPrefix(entry.Name(), "covcounters."):
			counters = true
		default:
			return nil, fmt.Errorf("process behavior observation contains an unknown artifact")
		}
	}
	if !metadata || !counters {
		return nil, fmt.Errorf("process behavior observation is incomplete")
	}
	// Conversion is a bounded Shell-private substep of the same operation,
	// sharing its deadline, process cleanup and scratch lifetime. It is not a
	// new Runtime operation, receipt, recovery lane or state machine.
	convertCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	profile := filepath.Join(directory, "observed.cover")
	cmd := exec.CommandContext(convertCtx, command, "tool", "covdata", "textfmt", "-i="+directory, "-o="+profile)
	cmd.Dir = workspace
	cmd.Env = shellBehaviorEnvironment(env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return terminateShellProcessGroup(cmd.Process.Pid)
	}
	cmd.WaitDelay = 2 * time.Second
	output := newLimitedOutputBuffer(MaxCommandOutputBytes)
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil {
		if cmd.Process != nil {
			_ = terminateShellProcessGroup(cmd.Process.Pid)
		}
		return nil, fmt.Errorf("process behavior coverage conversion failed")
	}
	if err := terminateShellProcessGroup(cmd.Process.Pid); err != nil || output.Truncated() || strings.TrimSpace(output.String()) != "" {
		// covdata can report an incomplete/mismatched input as a warning with
		// exit zero. Do not silently accept a partial conversion.
		return nil, fmt.Errorf("process behavior coverage conversion is incomplete")
	}
	targets, err := shellWorkspaceBehaviorTargets(workspace, profile, refs)
	return canonicalShellBehaviorTargets(targets), err
}

func canonicalShellBehaviorTargets(refs []string) []string {
	refs = normalizeShellStrings(refs)
	slices.Sort(refs)
	return slices.Compact(refs)
}

func shellEntrypointSourceSnapshot(directory string) (map[string]string, error) {
	observed, err := shellWorkspaceFileRevisions(directory, true)
	if err != nil {
		return nil, err
	}
	for ref := range observed {
		base := filepath.Base(ref)
		if !strings.HasSuffix(ref, ".go") && base != "go.mod" && base != "go.sum" && base != "go.work" {
			delete(observed, ref)
		}
	}
	return observed, nil
}
