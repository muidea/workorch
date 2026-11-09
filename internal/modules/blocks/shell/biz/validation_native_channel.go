package shell

// Native subprocess output belongs to the process actually executed by Shell,
// never to the Test label of a parent test2json stream. This is private managed
// session instrumentation, not a new Runtime command or execution authority.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellNativeRunnerEnv = "WORKORCH_NATIVE_SOURCE_RUNNER"
const shellNativeSocketEnv = "WORKORCH_NATIVE_SOURCE_SOCKET"
const shellNativeChildEnv = "WORKORCH_NATIVE_SOURCE_CHILD"

func shellNativeChannelMethodRequired(files []rc.RuntimeValidationInputReviewFile) bool {
	for _, source := range files {
		if !strings.HasSuffix(source.TargetRef, ".go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source.TargetRef, source.Content, 0)
		if err != nil {
			continue
		}
		aliases := map[string]bool{}
		for _, im := range file.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if p == "os" {
				name := "os"
				if im.Name != nil {
					name = im.Name.Name
				}
				aliases[name] = true
			}
		}
		found := false
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Getenv" && selector.Sel.Name != "LookupEnv") {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok || receiver.Obj != nil || !aliases[receiver.Name] {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if ok && literal.Kind == token.STRING {
				value, _ := strconv.Unquote(literal.Value)
				found = found || value == shellNativeRunnerEnv
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// The client cannot submit an executable, package, argv, environment, outcome or
// diagnostic. SO_PEERCRED and the live parent's compiled Go build identity bind
// the request to this stage. Shell chooses the executable and exact selector.
const shellNativeClientSource = `package main
import("encoding/json";"net";"os")
func main(){
 if len(os.Args)!=2 {os.Exit(125)}
 c,e:=net.Dial("unix",os.Getenv("WORKORCH_NATIVE_SOURCE_SOCKET"));if e!=nil{os.Exit(125)};defer c.Close()
 if json.NewEncoder(c).Encode(os.Args[1])!=nil{os.Exit(125)}
 if c.(*net.UnixConn).CloseWrite()!=nil{os.Exit(125)}
 var result int
 if json.NewDecoder(c).Decode(&result)!=nil || result<0 || result>125 {os.Exit(125)}
 os.Exit(result)
}
`

func prepareShellNativeRunner(ctx context.Context, scratch string, env []string) (string, error) {
	dir, err := os.MkdirTemp(scratch, "native-runner-")
	if err != nil {
		return "", err
	}
	source, binary := filepath.Join(dir, "main.go"), filepath.Join(dir, "observe")
	if err = os.WriteFile(source, []byte(shellNativeClientSource), 0600); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, source)
	cmd.Dir, cmd.Env = dir, shellBehaviorEnvironment(env)
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
	err = cmd.Run()
	if cmd.Process != nil {
		if cleanup := terminateShellProcessGroup(cmd.Process.Pid); err == nil {
			err = cleanup
		}
	}
	if err != nil || output.Truncated() {
		return "", fmt.Errorf("native source client preparation failed: %w", err)
	}
	return binary, nil
}

type shellNativeExecutionFact struct {
	Identity       rc.RuntimeValidationNativeExecution
	Stdout, Stderr shellProjectRawFact
	// Detached projection of this exact raw stream, computed at acquisition.
	Facts *shellGoTestJSONFacts `json:"-"`
}

type shellNativeChannel struct {
	ctx         context.Context
	cancel      context.CancelFunc
	directory   string
	env         []string
	sources     []rc.RuntimeValidationDiagnosticSource
	budget      *shellProjectRawBudget
	listener    *net.UnixListener
	stagePID    int
	mu          sync.Mutex
	closed      bool
	seen        map[string]bool
	connections map[*net.UnixConn]bool
	joined      sync.WaitGroup
	facts       []shellNativeExecutionFact
	code        string
}

func startShellNativeChannel(ctx context.Context, directory string, env []string, spec shellProjectStageSpec, budget *shellProjectRawBudget) (*shellNativeChannel, []string, error) {
	if len(spec.NativeSources) == 0 {
		return nil, env, nil
	}
	client := ""
	for _, value := range env {
		if v, ok := strings.CutPrefix(value, shellNativeRunnerEnv+"="); ok {
			client = v
		}
	}
	if client == "" {
		return nil, env, fmt.Errorf("native source client unavailable")
	}
	// Linux abstract addresses remain private to this session, do not depend
	// on long workspace paths, and disappear on close. Peers are still checked.
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, env, err
	}
	address := "@workorch-native-" + hex.EncodeToString(nonce[:])
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		return nil, env, err
	}
	childCtx, cancel := context.WithCancel(ctx)
	c := &shellNativeChannel{ctx: childCtx, cancel: cancel, directory: directory, env: env, sources: slices.Clone(spec.NativeSources), budget: budget, listener: listener, seen: map[string]bool{}, connections: map[*net.UnixConn]bool{}}
	ret := make([]string, 0, len(env)+1)
	for _, v := range env {
		if !strings.HasPrefix(v, shellNativeSocketEnv+"=") && !strings.HasPrefix(v, shellNativeChildEnv+"=") {
			ret = append(ret, v)
		}
	}
	ret = append(ret, shellNativeSocketEnv+"="+address)
	return c, ret, nil
}

func (c *shellNativeChannel) start(pid int) {
	c.stagePID = pid
	c.joined.Add(1)
	go func() {
		defer c.joined.Done()
		for {
			conn, err := c.listener.AcceptUnix()
			if err != nil {
				return
			}
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				_ = conn.Close()
				return
			}
			c.connections[conn] = true
			c.joined.Add(1)
			c.mu.Unlock()
			go func() {
				defer c.joined.Done()
				defer conn.Close()
				c.observe(conn)
				c.mu.Lock()
				delete(c.connections, conn)
				c.mu.Unlock()
			}()
		}
	}()
}

func shellNativeParent(pid int) (int, error) {
	body, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			return strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
		}
	}
	return 0, fmt.Errorf("native parent unavailable")
}

func shellNativePeer(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credential *syscall.Ucred
	var credentialErr error
	err = raw.Control(func(fd uintptr) {
		credential, credentialErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return 0, err
	}
	if credentialErr != nil {
		return 0, credentialErr
	}
	if credential.Uid != uint32(os.Getuid()) {
		return 0, fmt.Errorf("native peer owner mismatch")
	}
	return int(credential.Pid), nil
}

func (c *shellNativeChannel) reject(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.code == "" || code < c.code {
		c.code = code
	}
}

func (c *shellNativeChannel) observe(conn *net.UnixConn) {
	if deadline, ok := c.ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	peer, err := shellNativePeer(conn)
	if err != nil {
		c.reject("validation_native_source_peer_invalid")
		return
	}
	parent, err := shellNativeParent(peer)
	if err != nil {
		c.reject("validation_native_source_parent_unavailable")
		return
	}
	// Only a test binary launched directly by this exact owner go-test process
	// may acquire a child. Merely being a descendant would let a test substitute
	// another prebuilt .test binary or recursively create observations.
	grandparent, parentErr := shellNativeParent(parent)
	if parentErr != nil || grandparent != c.stagePID {
		c.reject("validation_native_source_parent_invalid")
		return
	}
	// Pin the parent's live executable inode. A filesystem path can be
	// replaced after observation; the inherited descriptor is the exact file
	// whose build identity and bytes we inspect, including an unlinked binary.
	binary, err := os.Open(fmt.Sprintf("/proc/%d/exe", parent))
	if err != nil {
		c.reject("validation_native_source_executable_unavailable")
		return
	}
	defer binary.Close()
	info, err := buildinfo.Read(binary)
	if err != nil || !strings.HasSuffix(info.Path, ".test") {
		c.reject("validation_native_source_executable_invalid")
		return
	}
	pkg := strings.TrimSuffix(info.Path, ".test")
	var native string
	decoder := json.NewDecoder(io.LimitReader(conn, 1025))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&native) != nil || len(native) > 512 || strings.Contains(native, "/") {
		c.reject("validation_native_source_identity_invalid")
		return
	}
	var trailing json.RawMessage
	if decoder.Decode(&trailing) != io.EOF {
		c.reject("validation_native_source_identity_invalid")
		return
	}
	var source rc.RuntimeValidationDiagnosticSource
	for _, candidate := range c.sources {
		if candidate.NativeCaseRef == pkg+"/"+native {
			source = candidate
			break
		}
	}
	if source.SourceRef == "" {
		c.reject("validation_native_source_out_of_scope")
		return
	}
	digest := sha256.New()
	_, hashErr := io.Copy(digest, binary)
	if hashErr != nil {
		c.reject("validation_native_source_executable_unavailable")
		return
	}
	c.mu.Lock()
	if c.seen[source.SourceRef] || c.closed {
		c.mu.Unlock()
		c.reject("validation_native_source_duplicate")
		return
	}
	c.seen[source.SourceRef] = true
	c.mu.Unlock()
	stdout, stderr := newShellProjectRawBuffer(), newShellProjectRawBuffer()
	stdout.budget, stderr.budget = c.budget, c.budget
	deadline, _ := c.ctx.Deadline()
	args := []string{"-test.v=true", "-test.count=1", "-test.run=^" + regexp.QuoteMeta(native) + "$", "-test.timeout=" + time.Until(deadline).String()}
	// A covered test executable publishes its final in-process counters only
	// with the test flag as well as GOCOVERDIR. Use this stage's owner-created
	// directory, never a caller-supplied artifact or another source's profile.
	for _, value := range c.env {
		if dir, ok := strings.CutPrefix(value, "WORKORCH_PROCESS_COVERDIR="); ok && dir != "" {
			args = append(args, "-test.gocoverdir="+dir)
		}
	}
	executable := "/proc/self/fd/3"
	cmd := exec.CommandContext(c.ctx, executable, args...)
	cmd.ExtraFiles = []*os.File{binary}
	cmd.Dir = c.directory
	// A test binary runs from its package directory, just like go test.
	parentDirectory, dirErr := os.Readlink(fmt.Sprintf("/proc/%d/cwd", parent))
	if dirErr != nil {
		c.reject("validation_native_source_parent_unavailable")
		return
	}
	if parentDirectory != filepath.Join(c.directory, filepath.Dir(filepath.FromSlash(source.InputTargetRef))) {
		c.reject("validation_native_source_parent_directory_changed")
		return
	}
	cmd.Dir = parentDirectory
	// Use the actual parent's environment, including owner coverage settings.
	// No caller-supplied execution options cross the channel.
	parentEnv, envErr := os.ReadFile(fmt.Sprintf("/proc/%d/environ", parent))
	if envErr != nil {
		c.reject("validation_native_source_parent_unavailable")
		return
	}
	for _, v := range strings.Split(strings.TrimSuffix(string(parentEnv), "\x00"), "\x00") {
		if !strings.HasPrefix(v, shellNativeSocketEnv+"=") && !strings.HasPrefix(v, shellNativeChildEnv+"=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, shellNativeChildEnv+"="+native)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return terminateShellProcessGroup(cmd.Process.Pid)
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdout, cmd.Stderr = stdout, stderr
	fact := shellNativeExecutionFact{Identity: rc.RuntimeValidationNativeExecution{SourceRef: source.SourceRef, NativeCaseRef: source.NativeCaseRef, ParentExecutableSHA256: hex.EncodeToString(digest.Sum(nil)), Argv: append([]string{executable}, args...), StartedAt: time.Now().UTC().Format(time.RFC3339Nano), ExitCode: -1}}
	err = cmd.Start()
	if err == nil {
		err = cmd.Wait()
	}
	if cmd.Process != nil {
		fact.Identity.CleanupFailed = terminateShellProcessGroup(cmd.Process.Pid) != nil
	}
	fact.Identity.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if cmd.ProcessState != nil {
		fact.Identity.ExitCode = int64(cmd.ProcessState.ExitCode())
	}
	fact.Identity.TimedOut = c.ctx.Err() == context.DeadlineExceeded
	fact.Stdout, fact.Stderr = stdout.fact(), stderr.fact()
	fact.Identity.Stdout, fact.Identity.Stderr = shellProjectOutputIdentity(fact.Stdout), shellProjectOutputIdentity(fact.Stderr)
	// test2json's command mode combines stderr into stdout. Capture the actual
	// child streams first; run its deterministic converter on saved stdout only.
	projection, convertErr := shellNativeJSONProjection(c.ctx, pkg, cmd.Env, fact.Stdout)
	_, _, _, complete, _, code := shellProjectJSONCompletion(projection, convertErr == nil)
	fact.Identity.TerminalComplete = complete && (code == "" || code == "validation_project_test_failed") && shellNativeOutputHasExactSource(projection, pkg, native)
	if fact.Identity.TerminalComplete {
		facts := parseGoTestJSONFacts(string(projection), string(fact.Stderr.Raw))
		fact.Facts = &facts
	}
	fact.Identity.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !fact.Stdout.Complete || !fact.Stderr.Complete {
		c.reject("validation_native_source_raw_capacity_exceeded")
	} else if !fact.Identity.TerminalComplete || fact.Identity.ExitCode < 0 || fact.Identity.TimedOut || fact.Identity.CleanupFailed {
		c.reject("validation_native_source_execution_incomplete")
	}
	c.mu.Lock()
	c.facts = append(c.facts, fact)
	c.mu.Unlock()
	status := 0
	if err != nil || fact.Identity.ExitCode != 0 {
		status = 1
	}
	_ = json.NewEncoder(conn).Encode(status)
}

func shellNativeJSONProjection(ctx context.Context, pkg string, env []string, raw shellProjectRawFact) ([]byte, error) {
	if !shellProjectRawFactValid(raw) {
		return nil, fmt.Errorf("native raw output incomplete")
	}
	cmd := exec.CommandContext(ctx, "go", "tool", "test2json", "-p", pkg)
	cmd.Env = shellBehaviorEnvironment(env)
	cmd.Stdin = bytes.NewReader(raw.Raw)
	output, errors := newShellProjectRawBuffer(), newShellProjectRawBuffer()
	cmd.Stdout, cmd.Stderr = output, errors
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return terminateShellProcessGroup(cmd.Process.Pid)
	}
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if cmd.Process != nil {
		if cleanup := terminateShellProcessGroup(cmd.Process.Pid); err == nil {
			err = cleanup
		}
	}
	if err != nil || !output.fact().Complete || errors.bytes != 0 {
		return nil, fmt.Errorf("native output conversion incomplete")
	}
	return output.fact().Raw, nil
}

func shellNativeOutputHasExactSource(raw []byte, pkg, native string) bool {
	scan := bufio.NewScanner(bytes.NewReader(raw))
	scan.Buffer(make([]byte, 4096), shellProjectRawOutputMaxBytes+1)
	found := false
	for scan.Scan() {
		var event shellGoTestJSONEvent
		if json.Unmarshal(scan.Bytes(), &event) != nil || event.Package != pkg {
			return false
		}
		if event.Test != "" {
			root, _, _ := strings.Cut(event.Test, "/")
			if root != native {
				return false
			}
			found = true
		}
	}
	return found && scan.Err() == nil
}

func (c *shellNativeChannel) finish() ([]shellNativeExecutionFact, string) {
	if c == nil {
		return nil, ""
	}
	c.mu.Lock()
	c.closed = true
	_ = c.listener.Close()
	for conn := range c.connections {
		_ = conn.Close()
	}
	c.mu.Unlock()
	c.cancel()
	c.joined.Wait()
	slices.SortFunc(c.facts, func(a, b shellNativeExecutionFact) int {
		return strings.Compare(a.Identity.SourceRef, b.Identity.SourceRef)
	})
	return c.facts, c.code
}
