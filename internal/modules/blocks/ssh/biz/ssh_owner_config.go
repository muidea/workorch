package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type sshTargetSnapshot struct {
	host               string
	port               int
	hostKeyAlias       string
	knownHosts         []byte
	knownHostsHash     string
	observationRef     string
	observationCommand string
}

type sshSessionSnapshot struct {
	targetRef      string
	username       string
	credentialRefs []string
}

type sshCredentialSnapshot struct {
	password   string
	privateKey string
	secretHash string
}

func newConfiguredRuntimeAdapter() (*RuntimeAdapter, error) {
	cfg, err := workorchconfig.CurrentSSH()
	if err != nil {
		return nil, err
	}
	configPath, err := workorchconfig.ApplicationConfigPath()
	if err != nil {
		return nil, err
	}
	return NewRuntimeAdapterFromConfig(cfg, filepath.Dir(configPath), os.LookupEnv)
}

// NewRuntimeAdapterFromConfig freezes endpoint and credential resolution for
// the process lifetime. Reconfiguration therefore changes only after restart.
func NewRuntimeAdapterFromConfig(cfg workorchconfig.SSHConfig, configDirectory string, lookupEnv func(string) (string, bool)) (*RuntimeAdapter, error) {
	cfg, err := workorchconfig.NormalizeSSHConfig(cfg)
	if err != nil {
		return nil, err
	}
	timeout, err := time.ParseDuration(cfg.DefaultTimeout)
	if err != nil || timeout < time.Second || timeout > 5*time.Minute {
		return nil, fmt.Errorf("%w: ssh defaultTimeout must be between 1s and 5m", common.ErrInvalidArgument)
	}
	adapter := &RuntimeAdapter{
		timeout: timeout, targets: map[string]sshTargetSnapshot{}, sessions: map[string]sshSessionSnapshot{}, credentials: map[string]sshCredentialSnapshot{},
	}
	if len(cfg.Targets) == 0 && len(cfg.Sessions) == 0 && len(cfg.Credentials) == 0 {
		adapter.revision = stableSSHOwnerPolicyRevision(cfg, adapter.targets, adapter.sessions, adapter.credentials)
		return adapter, nil
	}
	root, err := resolveSSHSecretsRoot(configDirectory, cfg.SecretsDirectory)
	if err != nil {
		return nil, err
	}
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	for _, ref := range sortedSSHMapKeys(cfg.Credentials) {
		value := cfg.Credentials[ref]
		snapshot := sshCredentialSnapshot{}
		switch {
		case value.PasswordEnv != "":
			secret, found := lookupEnv(value.PasswordEnv)
			if !found || secret == "" || len(secret) > 64*1024 || strings.ContainsAny(secret, "\r\n\x00") {
				adapter.Close()
				return nil, fmt.Errorf("%w: ssh credential %q secret source is unavailable", common.ErrInvalidArgument, ref)
			}
			snapshot.password = secret
			snapshot.secretHash = hashSSHBytes([]byte(secret))
		case value.PrivateKeyEnv != "":
			secret, found := lookupEnv(value.PrivateKeyEnv)
			if !found || strings.TrimSpace(secret) == "" || len(secret) > 1024*1024 || strings.ContainsRune(secret, '\x00') {
				adapter.Close()
				return nil, fmt.Errorf("%w: ssh credential %q secret source is unavailable", common.ErrInvalidArgument, ref)
			}
			snapshot.privateKey = secret
			snapshot.secretHash = hashSSHBytes([]byte(secret))
		case value.PrivateKeyFile != "":
			payload, err := readSSHContainedFile(root, value.PrivateKeyFile, true)
			if err != nil {
				adapter.Close()
				return nil, fmt.Errorf("ssh credential %q: %w", ref, err)
			}
			snapshot.privateKey = string(payload)
			snapshot.secretHash = hashSSHBytes(payload)
		}
		adapter.credentials[ref] = snapshot
	}
	for _, ref := range sortedSSHMapKeys(cfg.Targets) {
		value := cfg.Targets[ref]
		knownHosts, err := readSSHContainedFile(root, value.KnownHostsFile, false)
		if err != nil {
			adapter.Close()
			return nil, fmt.Errorf("ssh target %q: %w", ref, err)
		}
		adapter.targets[ref] = sshTargetSnapshot{
			host: value.Host, port: value.Port, hostKeyAlias: value.HostKeyAlias,
			knownHosts: append([]byte(nil), knownHosts...), knownHostsHash: hashSSHBytes(knownHosts),
			observationRef: value.ObservationRef, observationCommand: value.ObservationCommand,
		}
	}
	for _, ref := range sortedSSHMapKeys(cfg.Sessions) {
		value := cfg.Sessions[ref]
		adapter.sessions[ref] = sshSessionSnapshot{targetRef: value.TargetRef, username: value.Username, credentialRefs: append([]string(nil), value.CredentialRefs...)}
	}
	adapter.revision = stableSSHOwnerPolicyRevision(cfg, adapter.targets, adapter.sessions, adapter.credentials)
	return adapter, nil
}

func resolveSSHSecretsRoot(configDirectory, relative string) (string, error) {
	configDirectory = strings.TrimSpace(configDirectory)
	if configDirectory == "" {
		return "", common.ErrInvalidArgument
	}
	base, err := filepath.Abs(configDirectory)
	if err != nil {
		return "", err
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(filepath.Join(base, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, root)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: ssh secretsDirectory escapes configuration directory", common.ErrInvalidArgument)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: ssh secretsDirectory is not a directory", common.ErrInvalidArgument)
	}
	return root, nil
}

func readSSHContainedFile(root, relative string, requirePrivateMode bool) ([]byte, error) {
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("%w: ssh secret path escapes owner directory", common.ErrInvalidArgument)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: ssh owner file is not regular", common.ErrInvalidArgument)
	}
	if requirePrivateMode && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: ssh private key file must not be group/world accessible", common.ErrInvalidArgument)
	}
	if !requirePrivateMode && info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("%w: ssh known-hosts file must not be group/world writable", common.ErrInvalidArgument)
	}
	payload, err := os.ReadFile(candidate)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || len(payload) > 1024*1024 {
		return nil, fmt.Errorf("%w: ssh owner file is empty or exceeds 1 MiB", common.ErrInvalidArgument)
	}
	return payload, nil
}

func stableSSHOwnerPolicyRevision(cfg workorchconfig.SSHConfig, targets map[string]sshTargetSnapshot, sessions map[string]sshSessionSnapshot, credentials map[string]sshCredentialSnapshot) string {
	type targetRevision struct {
		Ref, Host, HostKeyAlias, KnownHostsHash, ObservationRef, ObservationCommand string
		Port                                                                        int
	}
	type sessionRevision struct {
		Ref, TargetRef, Username string
		CredentialRefs           []string
	}
	type credentialRevision struct{ Ref, SecretHash string }
	value := struct {
		Timeout     string
		Targets     []targetRevision
		Sessions    []sessionRevision
		Credentials []credentialRevision
	}{Timeout: cfg.DefaultTimeout}
	for _, ref := range sortedSSHMapKeys(targets) {
		target := targets[ref]
		value.Targets = append(value.Targets, targetRevision{ref, target.host, target.hostKeyAlias, target.knownHostsHash, target.observationRef, target.observationCommand, target.port})
	}
	for _, ref := range sortedSSHMapKeys(sessions) {
		session := sessions[ref]
		value.Sessions = append(value.Sessions, sessionRevision{Ref: ref, TargetRef: session.targetRef, Username: session.username, CredentialRefs: append([]string(nil), session.credentialRefs...)})
	}
	for _, ref := range sortedSSHMapKeys(credentials) {
		value.Credentials = append(value.Credentials, credentialRevision{Ref: ref, SecretHash: credentials[ref].secretHash})
	}
	payload, _ := json.Marshal(value)
	return "ssh-policy-" + hashSSHBytes(payload)[:20]
}

func (a *RuntimeAdapter) targetRefs(maxItems int) []string {
	if a == nil || maxItems <= 0 {
		return nil
	}
	refs := sortedSSHMapKeys(a.targets)
	if len(refs) > maxItems {
		refs = refs[:maxItems]
	}
	return refs
}

// bindingConstraintRefs publishes only the opaque, owner-valid tuple needed
// to assemble a feasible operation. Endpoint, username and secret material
// remain exclusively in the frozen snapshots above.
func (a *RuntimeAdapter) bindingConstraintRefs(maxItems int) []string {
	if a == nil || maxItems <= 0 {
		return nil
	}
	refs := make([]string, 0)
	for _, sessionRef := range sortedSSHMapKeys(a.sessions) {
		session := a.sessions[sessionRef]
		for _, credentialRef := range session.credentialRefs {
			refs = append(refs, sshBindingConstraintRef(session.targetRef, sessionRef, credentialRef))
		}
	}
	if len(refs) > maxItems {
		refs = refs[:maxItems]
	}
	return refs
}

func sshBindingConstraintRef(targetRef, sessionRef, credentialRef string) string {
	return "ssh-binding/" + targetRef + "/" + sessionRef + "/" + credentialRef
}

func hashSSHBytes(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func sortedSSHMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
