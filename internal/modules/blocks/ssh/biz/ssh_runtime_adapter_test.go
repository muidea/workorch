package ssh

import (
	"context"
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
