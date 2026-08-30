package ssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterExecutesPasswordSSHWithStrictHostIdentityAndObservation(t *testing.T) {
	toolsDir := t.TempDir()
	captureFile := filepath.Join(toolsDir, "capture.txt")
	stateFile := filepath.Join(toolsDir, "remote-state")
	writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), fakeObservedSSHScript(captureFile, stateFile, true))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_PASSWORD", "test-password")
	result, err := adapter.ExecuteOperation(context.Background(), testRuntimeCommandRequest("mutate-state"))
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if result.Status != "completed" || !result.Changed || result.ObservedScope != runtimecontract.RuntimeEffectRemoteDurable ||
		result.BeforeHash == result.AfterHash || result.OutputHash == "" || result.StdoutBytes != int64(len("remote-output\n")) {
		t.Fatalf("result = %+v, want observed remote change and bounded output metadata", result)
	}
	captured := readFile(t, captureFile)
	for _, want := range []string{
		"-F /dev/null", "StrictHostKeyChecking=yes", "UserKnownHostsFile=", "GlobalKnownHostsFile=/dev/null", "IdentitiesOnly=yes",
		"-o BatchMode=no", "askpass_require=force", "password=test-password", "operator@example.internal mutate-state",
	} {
		if !strings.Contains(captured, want) {
			t.Fatalf("captured = %q, want %q", captured, want)
		}
	}
	if strings.Contains(captured, "StrictHostKeyChecking=no") {
		t.Fatalf("captured = %q, strict host identity was disabled", captured)
	}
}

func TestRuntimeAdapterExecutesPrivateKeyWithoutExposingConfiguredPath(t *testing.T) {
	toolsDir := t.TempDir()
	captureFile := filepath.Join(toolsDir, "capture.txt")
	stateFile := filepath.Join(toolsDir, "remote-state")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), fakeObservedSSHScript(captureFile, stateFile, false))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := configuredPrivateKeyRuntimeAdapter(t)
	result, err := adapter.ExecuteOperation(context.Background(), testRuntimeCommandRequest("mutate-state"))
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if result.Status != "completed" || !result.Changed || result.OutputHash == "" {
		t.Fatalf("result = %+v, want observed private-key execution", result)
	}
	captured := readFile(t, captureFile)
	for _, want := range []string{"-i ", "-o BatchMode=yes", "PreferredAuthentications=publickey", "operator@example.internal mutate-state"} {
		if !strings.Contains(captured, want) {
			t.Fatalf("captured = %q, want %q", captured, want)
		}
	}
	if strings.Contains(captured, "identity-source") {
		t.Fatalf("configured private-key path crossed into ssh argv: %q", captured)
	}
}

func TestRuntimeAdapterBoundsAndHashesFullSSHCommandOutput(t *testing.T) {
	toolsDir := t.TempDir()
	stateFile := filepath.Join(toolsDir, "remote-state")
	writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\nif [ \"$last\" = \"observe-state\" ]; then if [ -f \""+stateFile+"\" ]; then echo after; else echo before; fi; exit 0; fi\ntouch \""+stateFile+"\"\nyes x | head -c 70000\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_PASSWORD", "test-password")
	result, err := adapter.ExecuteOperation(context.Background(), testRuntimeCommandRequest("large-output"))
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if !result.OutputTruncated || !result.StdoutTruncated || result.StdoutBytes != 70000 || result.OutputHash == "" {
		t.Fatalf("result = %+v, want full byte count/hash with bounded transport", result)
	}
	if strings.Contains(result.Evidence.Summary, strings.Repeat("x", 32)) {
		t.Fatalf("raw output entered evidence: %+v", result.Evidence)
	}
}

func TestRuntimeAdapterNoOpUsesActualRemoteObservation(t *testing.T) {
	toolsDir := t.TempDir()
	captureFile := filepath.Join(toolsDir, "capture.txt")
	stateFile := filepath.Join(toolsDir, "remote-state")
	writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), fakeObservedSSHScript(captureFile, stateFile, true))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_PASSWORD", "test-password")
	result, err := adapter.ExecuteOperation(context.Background(), testRuntimeCommandRequest("true"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || result.ObservedScope != runtimecontract.RuntimeEffectNone || !result.Evidence.ZeroWork || result.BeforeHash != result.AfterHash {
		t.Fatalf("result = %+v, want factual no-op", result)
	}
}

func TestRuntimeAdapterTimeoutAndCancellationTerminateLocalSSHAndRemoveCredentialMaterial(t *testing.T) {
	newFixture := func(t *testing.T, marker string) *RuntimeAdapter {
		t.Helper()
		toolsDir := t.TempDir()
		writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
		writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\nif [ \"$last\" = \"observe-state\" ]; then echo before; exit 0; fi\nprintf '%s' \"$WORKORCH_SSH_PASSWORD_FILE\" > \""+marker+"\"\nexec sleep 10\n")
		t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		return configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_CANCEL_PASSWORD", "test-password")
	}
	assertCredentialRemoved := func(t *testing.T, marker string) {
		t.Helper()
		path, err := os.ReadFile(marker)
		if err != nil || strings.TrimSpace(string(path)) == "" {
			t.Fatalf("read staged credential path: path=%q err=%v", string(path), err)
		}
		if _, err := os.Stat(strings.TrimSpace(string(path))); !os.IsNotExist(err) {
			t.Fatalf("temporary SSH credential material remains at %q: %v", string(path), err)
		}
	}

	t.Run("sealed timeout", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "credential-path")
		adapter := newFixture(t, marker)
		request := testRuntimeCommandRequest("slow")
		request.TimeoutMillis = 1000
		started := time.Now()
		result, err := adapter.ExecuteOperation(context.Background(), request)
		if !errors.Is(err, context.DeadlineExceeded) || result.FailureCode != "ssh_command_timeout" || result.ObservedScope != runtimecontract.RuntimeEffectUnknown || time.Since(started) > 3*time.Second {
			t.Fatalf("timeout result=%+v err=%v duration=%s", result, err, time.Since(started))
		}
		assertCredentialRemoved(t, marker)
	})

	t.Run("caller cancellation", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "credential-path")
		adapter := newFixture(t, marker)
		ctx, cancel := context.WithCancel(context.Background())
		type outcome struct {
			result RuntimeCommandResult
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, err := adapter.ExecuteOperation(ctx, testRuntimeCommandRequest("slow"))
			done <- outcome{result: result, err: err}
		}()
		deadline := time.Now().Add(2 * time.Second)
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("SSH child did not start before cancellation")
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		select {
		case got := <-done:
			if !errors.Is(got.err, context.Canceled) || got.result.ObservedScope != runtimecontract.RuntimeEffectUnknown {
				t.Fatalf("cancel result=%+v err=%v", got.result, got.err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("local SSH child survived caller cancellation")
		}
		assertCredentialRemoved(t, marker)
	})
}

func TestRuntimeAdapterFailedCommandClassifiesObservedRemoteEffect(t *testing.T) {
	for _, tc := range []struct {
		name      string
		command   string
		changed   bool
		wantScope runtimecontract.RuntimeEffectScope
	}{
		{name: "failed after remote change", command: "fail-after-change", changed: true, wantScope: runtimecontract.RuntimeEffectRemoteDurable},
		{name: "failed without remote change", command: "fail-without-change", changed: false, wantScope: runtimecontract.RuntimeEffectNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toolsDir := t.TempDir()
			stateFile := filepath.Join(toolsDir, "remote-state")
			writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
			writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\nif [ \"$last\" = \"observe-state\" ]; then if [ -f \""+stateFile+"\" ]; then echo after; else echo before; fi; exit 0; fi\nif [ \"$last\" = \"fail-after-change\" ]; then touch \""+stateFile+"\"; echo changed-but-failed; exit 2; fi\nif [ \"$last\" = \"fail-without-change\" ]; then echo unchanged-failure >&2; exit 2; fi\nexit 3\n")
			t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_FAILURE_PASSWORD", "test-password")
			result, err := adapter.ExecuteOperation(context.Background(), testRuntimeCommandRequest(tc.command))
			if err == nil {
				t.Fatal("failed SSH command returned nil error")
			}
			if result.Status != "failed" || result.FailureCode != "ssh_command_failed" || result.Changed != tc.changed || result.ObservedScope != tc.wantScope || result.BeforeHash == "" || result.AfterHash == "" || result.OutputHash == "" {
				t.Fatalf("failed SSH result = %+v, want changed=%t scope=%q", result, tc.changed, tc.wantScope)
			}
			if tc.changed == (result.BeforeHash == result.AfterHash) {
				t.Fatalf("failed SSH observation hashes do not match changed=%t: before=%q after=%q", tc.changed, result.BeforeHash, result.AfterHash)
			}
		})
	}
}

func TestRuntimeAdapterProbeResolvesOwnerRefsWithoutSSHExecution(t *testing.T) {
	toolsDir := t.TempDir()
	marker := filepath.Join(toolsDir, "ssh-called")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\ntouch \""+marker+"\"\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_PASSWORD", "test-password")
	if err := adapter.ProbeOperation(testRuntimeCommandRequest("hostname")); err != nil {
		t.Fatalf("probe valid ssh command: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("probe started ssh command: stat err=%v", err)
	}
	invalid := testRuntimeCommandRequest("hostname")
	invalid.CredentialRef = "other-credential"
	if err := adapter.ProbeOperation(invalid); err == nil {
		t.Fatal("probe accepted an unbound credential ref")
	}
}

func configuredPasswordRuntimeAdapter(t *testing.T, envName, secret string) *RuntimeAdapter {
	t.Helper()
	t.Setenv(envName, secret)
	return configuredRuntimeAdapter(t, workorchconfig.SSHCredentialConfig{PasswordEnv: envName})
}

func configuredPrivateKeyRuntimeAdapter(t *testing.T) *RuntimeAdapter {
	t.Helper()
	return configuredRuntimeAdapter(t, workorchconfig.SSHCredentialConfig{PrivateKeyFile: "identity-source"})
}

func configuredRuntimeAdapter(t *testing.T, credential workorchconfig.SSHCredentialConfig) *RuntimeAdapter {
	t.Helper()
	configDir := t.TempDir()
	secretsDir := filepath.Join(configDir, "ssh")
	if err := os.Mkdir(secretsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "known_hosts"), []byte("example.internal ssh-ed25519 AAAATEST\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if credential.PrivateKeyFile != "" {
		if err := os.WriteFile(filepath.Join(secretsDir, credential.PrivateKeyFile), []byte("test-private-key"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	adapter, err := NewRuntimeAdapterFromConfig(workorchconfig.SSHConfig{
		DefaultTimeout: "5s", SecretsDirectory: "ssh",
		Targets: map[string]workorchconfig.SSHTargetConfig{
			"target-a": {Host: "example.internal", Port: 22, KnownHostsFile: "known_hosts", ObservationRef: "state", ObservationCommand: "observe-state"},
		},
		Sessions: map[string]workorchconfig.SSHSessionConfig{
			"session-a": {TargetRef: "target-a", Username: "operator", CredentialRefs: []string{"credential-a"}},
		},
		Credentials: map[string]workorchconfig.SSHCredentialConfig{"credential-a": credential},
	}, configDir, os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adapter.Close)
	return adapter
}

func testRuntimeCommandRequest(command string) RuntimeCommandRequest {
	return RuntimeCommandRequest{
		WorkspaceID: "ws-ssh", RunID: "run-ssh", StepID: "run-ssh/ssh-1",
		TargetRef: "target-a", SessionRef: "session-a", CredentialRef: "credential-a", Command: command, TimeoutMillis: 5000,
	}
}

func fakeObservedSSHScript(captureFile, stateFile string, password bool) string {
	passwordCapture := ""
	if password {
		passwordCapture = "echo \"askpass_require=$SSH_ASKPASS_REQUIRE\" >> \"" + captureFile + "\"\necho \"password=$($SSH_ASKPASS)\" >> \"" + captureFile + "\"\n"
	}
	return "#!/bin/sh\n" +
		"echo \"args=$*\" >> \"" + captureFile + "\"\n" + passwordCapture +
		"last=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\n" +
		"if [ \"$last\" = \"observe-state\" ]; then if [ -f \"" + stateFile + "\" ]; then echo after; else echo before; fi; exit 0; fi\n" +
		"if [ \"$last\" = \"mutate-state\" ]; then touch \"" + stateFile + "\"; echo remote-output; exit 0; fi\n" +
		"if [ \"$last\" = \"true\" ]; then exit 0; fi\nexit 2\n"
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

var _ = time.Second
