package workorchconfig

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const DefaultSSHCommandTimeout = "30s"

// SSHConfig is read once when the SSH owner starts. Runtime authority carries
// only the opaque map keys below; endpoint and credential material stay inside
// this owner-private configuration boundary.
type SSHConfig struct {
	DefaultTimeout   string                         `json:"defaultTimeout" toml:"defaultTimeout"`
	SecretsDirectory string                         `json:"secretsDirectory" toml:"secretsDirectory"`
	Targets          map[string]SSHTargetConfig     `json:"targets,omitempty" toml:"targets,omitempty"`
	Sessions         map[string]SSHSessionConfig    `json:"sessions,omitempty" toml:"sessions,omitempty"`
	Credentials      map[string]SSHCredentialConfig `json:"credentials,omitempty" toml:"credentials,omitempty"`
}

type SSHTargetConfig struct {
	Host               string `json:"host" toml:"host"`
	Port               int    `json:"port" toml:"port"`
	KnownHostsFile     string `json:"knownHostsFile" toml:"knownHostsFile"`
	HostKeyAlias       string `json:"hostKeyAlias,omitempty" toml:"hostKeyAlias,omitempty"`
	ObservationRef     string `json:"observationRef" toml:"observationRef"`
	ObservationCommand string `json:"observationCommand" toml:"observationCommand"`
}

type SSHSessionConfig struct {
	TargetRef      string   `json:"targetRef" toml:"targetRef"`
	Username       string   `json:"username" toml:"username"`
	CredentialRefs []string `json:"credentialRefs" toml:"credentialRefs"`
}

// SSHCredentialConfig contains only secret locations. Inline passwords and
// private keys are intentionally absent from the persisted configuration type.
type SSHCredentialConfig struct {
	PasswordEnv    string `json:"passwordEnv,omitempty" toml:"passwordEnv,omitempty"`
	PrivateKeyEnv  string `json:"privateKeyEnv,omitempty" toml:"privateKeyEnv,omitempty"`
	PrivateKeyFile string `json:"privateKeyFile,omitempty" toml:"privateKeyFile,omitempty"`
}

var sshOpaqueRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var sshEnvironmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func NormalizeSSHConfig(cfg SSHConfig) (SSHConfig, error) {
	cfg.DefaultTimeout = strings.TrimSpace(cfg.DefaultTimeout)
	if cfg.DefaultTimeout == "" {
		cfg.DefaultTimeout = DefaultSSHCommandTimeout
	}
	cfg.SecretsDirectory = strings.TrimSpace(cfg.SecretsDirectory)
	if cfg.SecretsDirectory == "" {
		cfg.SecretsDirectory = "ssh"
	}
	if err := validateSSHRelativePath("secretsDirectory", cfg.SecretsDirectory); err != nil {
		return SSHConfig{}, err
	}

	var err error
	cfg.Credentials, err = normalizeSSHCredentials(cfg.Credentials)
	if err != nil {
		return SSHConfig{}, err
	}
	cfg.Targets, err = normalizeSSHTargets(cfg.Targets)
	if err != nil {
		return SSHConfig{}, err
	}
	cfg.Sessions, err = normalizeSSHSessions(cfg.Sessions, cfg.Targets, cfg.Credentials)
	if err != nil {
		return SSHConfig{}, err
	}
	if err := validateSSHConfigReachability(cfg); err != nil {
		return SSHConfig{}, err
	}
	return cfg, nil
}

func validateSSHConfigReachability(cfg SSHConfig) error {
	if len(cfg.Targets) == 0 && len(cfg.Sessions) == 0 && len(cfg.Credentials) == 0 {
		return nil
	}
	if len(cfg.Targets) == 0 || len(cfg.Sessions) == 0 || len(cfg.Credentials) == 0 {
		return fmt.Errorf("%w: ssh targets, sessions and credentials must be configured as one closed catalog", common.ErrInvalidArgument)
	}
	if len(cfg.Targets) > runtimecontract.RuntimeOwnerInventoryItemsMaxCount ||
		len(cfg.Sessions) > runtimecontract.RuntimeOwnerInventoryItemsMaxCount ||
		len(cfg.Credentials) > runtimecontract.RuntimeOwnerInventoryItemsMaxCount {
		return fmt.Errorf("%w: ssh owner catalog exceeds %d targets, sessions or credentials", common.ErrInvalidArgument, runtimecontract.RuntimeOwnerInventoryItemsMaxCount)
	}
	usedTargets := map[string]struct{}{}
	usedCredentials := map[string]struct{}{}
	bindingCount := 0
	for _, session := range cfg.Sessions {
		usedTargets[session.TargetRef] = struct{}{}
		bindingCount += len(session.CredentialRefs)
		for _, ref := range session.CredentialRefs {
			usedCredentials[ref] = struct{}{}
		}
	}
	if bindingCount > runtimecontract.RuntimeOwnerInventoryItemsMaxCount {
		return fmt.Errorf("%w: ssh owner catalog exceeds %d target/session/credential bindings", common.ErrInvalidArgument, runtimecontract.RuntimeOwnerInventoryItemsMaxCount)
	}
	for ref := range cfg.Targets {
		if _, found := usedTargets[ref]; !found {
			return fmt.Errorf("%w: ssh target %q is not bound by a session", common.ErrInvalidArgument, ref)
		}
	}
	for ref := range cfg.Credentials {
		if _, found := usedCredentials[ref]; !found {
			return fmt.Errorf("%w: ssh credential %q is not bound by a session", common.ErrInvalidArgument, ref)
		}
	}
	return nil
}

func normalizeSSHCredentials(values map[string]SSHCredentialConfig) (map[string]SSHCredentialConfig, error) {
	ret := make(map[string]SSHCredentialConfig, len(values))
	for _, rawRef := range sortedSSHConfigKeys(values) {
		ref := strings.TrimSpace(rawRef)
		if !sshOpaqueRefPattern.MatchString(ref) || ref != rawRef {
			return nil, fmt.Errorf("%w: ssh credential ref %q is not a canonical opaque ref", common.ErrInvalidArgument, rawRef)
		}
		value := values[rawRef]
		value.PasswordEnv = strings.TrimSpace(value.PasswordEnv)
		value.PrivateKeyEnv = strings.TrimSpace(value.PrivateKeyEnv)
		value.PrivateKeyFile = strings.TrimSpace(value.PrivateKeyFile)
		count := 0
		if value.PasswordEnv != "" {
			count++
			if !sshEnvironmentNamePattern.MatchString(value.PasswordEnv) {
				return nil, fmt.Errorf("%w: ssh credential %q has invalid passwordEnv", common.ErrInvalidArgument, ref)
			}
		}
		if value.PrivateKeyEnv != "" {
			count++
			if !sshEnvironmentNamePattern.MatchString(value.PrivateKeyEnv) {
				return nil, fmt.Errorf("%w: ssh credential %q has invalid privateKeyEnv", common.ErrInvalidArgument, ref)
			}
		}
		if value.PrivateKeyFile != "" {
			count++
			if err := validateSSHRelativePath("privateKeyFile", value.PrivateKeyFile); err != nil {
				return nil, fmt.Errorf("ssh credential %q: %w", ref, err)
			}
		}
		if count != 1 {
			return nil, fmt.Errorf("%w: ssh credential %q requires exactly one secret source", common.ErrInvalidArgument, ref)
		}
		ret[ref] = value
	}
	return ret, nil
}

func normalizeSSHTargets(values map[string]SSHTargetConfig) (map[string]SSHTargetConfig, error) {
	ret := make(map[string]SSHTargetConfig, len(values))
	for _, rawRef := range sortedSSHConfigKeys(values) {
		ref := strings.TrimSpace(rawRef)
		if !sshOpaqueRefPattern.MatchString(ref) || ref != rawRef {
			return nil, fmt.Errorf("%w: ssh target ref %q is not a canonical opaque ref", common.ErrInvalidArgument, rawRef)
		}
		value := values[rawRef]
		value.Host = strings.TrimSpace(value.Host)
		value.KnownHostsFile = strings.TrimSpace(value.KnownHostsFile)
		value.HostKeyAlias = strings.TrimSpace(value.HostKeyAlias)
		value.ObservationRef = strings.TrimSpace(value.ObservationRef)
		value.ObservationCommand = strings.TrimSpace(value.ObservationCommand)
		if value.Port == 0 {
			value.Port = 22
		}
		if !validSSHHost(value.Host) || value.Port < 1 || value.Port > 65535 {
			return nil, fmt.Errorf("%w: ssh target %q requires a safe host and valid port", common.ErrInvalidArgument, ref)
		}
		if err := validateSSHRelativePath("knownHostsFile", value.KnownHostsFile); err != nil {
			return nil, fmt.Errorf("ssh target %q: %w", ref, err)
		}
		if value.HostKeyAlias != "" && !validSSHHost(value.HostKeyAlias) {
			return nil, fmt.Errorf("%w: ssh target %q has invalid hostKeyAlias", common.ErrInvalidArgument, ref)
		}
		if !sshOpaqueRefPattern.MatchString(value.ObservationRef) || value.ObservationCommand == "" || len(value.ObservationCommand) > 16*1024 {
			return nil, fmt.Errorf("%w: ssh target %q requires bounded observationRef and observationCommand", common.ErrInvalidArgument, ref)
		}
		ret[ref] = value
	}
	return ret, nil
}

func normalizeSSHSessions(values map[string]SSHSessionConfig, targets map[string]SSHTargetConfig, credentials map[string]SSHCredentialConfig) (map[string]SSHSessionConfig, error) {
	ret := make(map[string]SSHSessionConfig, len(values))
	for _, rawRef := range sortedSSHConfigKeys(values) {
		ref := strings.TrimSpace(rawRef)
		if !sshOpaqueRefPattern.MatchString(ref) || ref != rawRef {
			return nil, fmt.Errorf("%w: ssh session ref %q is not a canonical opaque ref", common.ErrInvalidArgument, rawRef)
		}
		value := values[rawRef]
		value.TargetRef = strings.TrimSpace(value.TargetRef)
		value.Username = strings.TrimSpace(value.Username)
		if _, ok := targets[value.TargetRef]; !ok || !validSSHArgumentAtom(value.Username) || len(value.Username) > 64 || strings.Contains(value.Username, "@") {
			return nil, fmt.Errorf("%w: ssh session %q has invalid target or username", common.ErrInvalidArgument, ref)
		}
		seen := map[string]struct{}{}
		refs := make([]string, 0, len(value.CredentialRefs))
		for _, rawCredentialRef := range value.CredentialRefs {
			credentialRef := strings.TrimSpace(rawCredentialRef)
			if _, ok := credentials[credentialRef]; !ok {
				return nil, fmt.Errorf("%w: ssh session %q references unknown credential", common.ErrInvalidArgument, ref)
			}
			if _, duplicate := seen[credentialRef]; duplicate {
				continue
			}
			seen[credentialRef] = struct{}{}
			refs = append(refs, credentialRef)
		}
		sort.Strings(refs)
		if len(refs) == 0 {
			return nil, fmt.Errorf("%w: ssh session %q requires a credential ref", common.ErrInvalidArgument, ref)
		}
		value.CredentialRefs = refs
		ret[ref] = value
	}
	return ret, nil
}

func validateSSHRelativePath(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) || filepath.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: ssh %s must be a canonical relative path", common.ErrInvalidArgument, field)
	}
	return nil
}

func validSSHHost(value string) bool {
	return len(value) <= 255 && validSSHArgumentAtom(value) && !strings.ContainsAny(value, "@/\\")
}

func validSSHArgumentAtom(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, " \t\r\n\x00")
}

func sortedSSHConfigKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
