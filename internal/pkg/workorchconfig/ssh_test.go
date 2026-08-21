package workorchconfig

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeSSHConfigAcceptsOnlyOpaqueOwnerRefsAndSecretLocations(t *testing.T) {
	cfg, err := NormalizeSSHConfig(SSHConfig{
		Targets: map[string]SSHTargetConfig{
			"target-a": {Host: "example.internal", KnownHostsFile: "known_hosts", ObservationRef: "state", ObservationCommand: "sha256sum /srv/app/revision"},
		},
		Sessions: map[string]SSHSessionConfig{
			"session-a": {TargetRef: "target-a", Username: "operator", CredentialRefs: []string{"credential-a", "credential-a"}},
		},
		Credentials: map[string]SSHCredentialConfig{
			"credential-a": {PasswordEnv: "WORKORCH_SSH_PASSWORD_A"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTimeout != DefaultSSHCommandTimeout || cfg.SecretsDirectory != "ssh" || cfg.Targets["target-a"].Port != 22 ||
		len(cfg.Sessions["session-a"].CredentialRefs) != 1 {
		t.Fatalf("normalized SSH config = %+v", cfg)
	}
}

func TestNormalizeSSHConfigRejectsUnboundedOwnerBindingCatalog(t *testing.T) {
	credentials := make(map[string]SSHCredentialConfig, 17)
	credentialRefs := make([]string, 0, 17)
	for idx := 0; idx < 17; idx++ {
		ref := fmt.Sprintf("credential-%02d", idx)
		credentials[ref] = SSHCredentialConfig{PasswordEnv: fmt.Sprintf("SSH_PASSWORD_%02d", idx)}
		credentialRefs = append(credentialRefs, ref)
	}
	_, err := NormalizeSSHConfig(SSHConfig{
		Targets: map[string]SSHTargetConfig{
			"target-a": {Host: "example.internal", KnownHostsFile: "known_hosts", ObservationRef: "state", ObservationCommand: "observe"},
		},
		Sessions: map[string]SSHSessionConfig{
			"session-a": {TargetRef: "target-a", Username: "operator-a", CredentialRefs: credentialRefs},
			"session-b": {TargetRef: "target-a", Username: "operator-b", CredentialRefs: credentialRefs},
		},
		Credentials: credentials,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds 32 target/session/credential bindings") {
		t.Fatalf("err = %v, want bounded binding catalog rejection", err)
	}
}

func TestNormalizeSSHConfigRejectsCredentialAndPathEscapeSurfaces(t *testing.T) {
	valid := func() SSHConfig {
		return SSHConfig{
			SecretsDirectory: "ssh",
			Targets: map[string]SSHTargetConfig{
				"target-a": {Host: "example.internal", KnownHostsFile: "known_hosts", ObservationRef: "state", ObservationCommand: "observe"},
			},
			Sessions: map[string]SSHSessionConfig{
				"session-a": {TargetRef: "target-a", Username: "operator", CredentialRefs: []string{"credential-a"}},
			},
			Credentials: map[string]SSHCredentialConfig{"credential-a": {PasswordEnv: "SSH_PASSWORD_A"}},
		}
	}
	tests := map[string]func(*SSHConfig){
		"absolute secrets directory": func(cfg *SSHConfig) { cfg.SecretsDirectory = "/tmp/ssh" },
		"known hosts traversal": func(cfg *SSHConfig) {
			value := cfg.Targets["target-a"]
			value.KnownHostsFile = "../known_hosts"
			cfg.Targets["target-a"] = value
		},
		"private key absolute": func(cfg *SSHConfig) {
			cfg.Credentials["credential-a"] = SSHCredentialConfig{PrivateKeyFile: "/tmp/id_ed25519"}
		},
		"ambiguous credential": func(cfg *SSHConfig) {
			cfg.Credentials["credential-a"] = SSHCredentialConfig{PasswordEnv: "SSH_PASSWORD_A", PrivateKeyEnv: "SSH_KEY_A"}
		},
		"host option injection": func(cfg *SSHConfig) {
			value := cfg.Targets["target-a"]
			value.Host = "-oProxyCommand=bad"
			cfg.Targets["target-a"] = value
		},
		"username injection": func(cfg *SSHConfig) {
			value := cfg.Sessions["session-a"]
			value.Username = "operator@example"
			cfg.Sessions["session-a"] = value
		},
		"unbounded username": func(cfg *SSHConfig) {
			value := cfg.Sessions["session-a"]
			value.Username = strings.Repeat("u", 65)
			cfg.Sessions["session-a"] = value
		},
		"unknown credential": func(cfg *SSHConfig) {
			value := cfg.Sessions["session-a"]
			value.CredentialRefs = []string{"missing"}
			cfg.Sessions["session-a"] = value
		},
		"endpoint-shaped target ref": func(cfg *SSHConfig) {
			cfg.Targets["https://example.internal"] = cfg.Targets["target-a"]
			delete(cfg.Targets, "target-a")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := valid()
			mutate(&cfg)
			if _, err := NormalizeSSHConfig(cfg); err == nil || !strings.Contains(err.Error(), "invalid argument") {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}
