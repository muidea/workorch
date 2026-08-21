package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestRuntimeAdapterConfigRejectsSecretsDirectorySymlinkEscape(t *testing.T) {
	configDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(configDir, "ssh")); err != nil {
		t.Fatal(err)
	}
	_, err := NewRuntimeAdapterFromConfig(testSSHOwnerConfig(workorchconfig.SSHCredentialConfig{PasswordEnv: "SSH_PASSWORD_A"}), configDir, func(string) (string, bool) {
		return "secret", true
	})
	if err == nil || !strings.Contains(err.Error(), "escapes configuration directory") {
		t.Fatalf("err = %v, want secrets directory escape rejection", err)
	}
}

func TestRuntimeAdapterConfigRejectsPrivateKeyPermissionAndFileSymlinkEscape(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(t *testing.T, configDir, secretsDir string)
		want      string
	}{
		{
			name: "group-readable private key",
			configure: func(t *testing.T, _, secretsDir string) {
				if err := os.WriteFile(filepath.Join(secretsDir, "identity"), []byte("key"), 0o640); err != nil {
					t.Fatal(err)
				}
			},
			want: "group/world accessible",
		},
		{
			name: "private key symlink escape",
			configure: func(t *testing.T, configDir, secretsDir string) {
				outside := filepath.Join(configDir, "outside-key")
				if err := os.WriteFile(outside, []byte("key"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(secretsDir, "identity")); err != nil {
					t.Fatal(err)
				}
			},
			want: "escapes owner directory",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			configDir := t.TempDir()
			secretsDir := filepath.Join(configDir, "ssh")
			if err := os.Mkdir(secretsDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(secretsDir, "known_hosts"), []byte("example.internal ssh-ed25519 AAAATEST\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			test.configure(t, configDir, secretsDir)
			_, err := NewRuntimeAdapterFromConfig(testSSHOwnerConfig(workorchconfig.SSHCredentialConfig{PrivateKeyFile: "identity"}), configDir, os.LookupEnv)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRuntimeAdapterConfigRejectsWritableKnownHostsFile(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := filepath.Join(configDir, "ssh")
	if err := os.Mkdir(secretsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	knownHostsPath := filepath.Join(secretsDir, "known_hosts")
	if err := os.WriteFile(knownHostsPath, []byte("example.internal ssh-ed25519 AAAATEST\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(knownHostsPath, 0o666); err != nil {
		t.Fatal(err)
	}
	_, err := NewRuntimeAdapterFromConfig(testSSHOwnerConfig(workorchconfig.SSHCredentialConfig{PasswordEnv: "SSH_PASSWORD_A"}), configDir, func(string) (string, bool) {
		return "secret", true
	})
	if err == nil || !strings.Contains(err.Error(), "known-hosts file must not be group/world writable") {
		t.Fatalf("err = %v, want writable known-hosts rejection", err)
	}
}

func testSSHOwnerConfig(credential workorchconfig.SSHCredentialConfig) workorchconfig.SSHConfig {
	return workorchconfig.SSHConfig{
		DefaultTimeout: "5s", SecretsDirectory: "ssh",
		Targets: map[string]workorchconfig.SSHTargetConfig{
			"target-a": {Host: "example.internal", KnownHostsFile: "known_hosts", ObservationRef: "state", ObservationCommand: "observe-state"},
		},
		Sessions: map[string]workorchconfig.SSHSessionConfig{
			"session-a": {TargetRef: "target-a", Username: "operator", CredentialRefs: []string{"credential-a"}},
		},
		Credentials: map[string]workorchconfig.SSHCredentialConfig{"credential-a": credential},
	}
}
