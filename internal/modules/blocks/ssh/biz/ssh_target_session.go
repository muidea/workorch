package ssh

import (
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

type sshCommandSpec struct {
	TargetRef          string
	ObservationRef     string
	Host               string
	Port               int
	HostKeyAlias       string
	KnownHosts         []byte
	Username           string
	Password           string
	PrivateKey         string
	Command            string
	ObservationCommand string
	Timeout            time.Duration
}

func (a *RuntimeAdapter) sshCommandSpecFromRequest(request RuntimeCommandRequest) (sshCommandSpec, error) {
	if a == nil {
		return sshCommandSpec{}, common.ErrInvalidArgument
	}
	targetRef := strings.TrimSpace(request.TargetRef)
	sessionRef := strings.TrimSpace(request.SessionRef)
	credentialRef := strings.TrimSpace(request.CredentialRef)
	target, targetFound := a.targets[targetRef]
	session, sessionFound := a.sessions[sessionRef]
	credential, credentialFound := a.credentials[credentialRef]
	if !targetFound || !sessionFound || !credentialFound || session.targetRef != targetRef || !containsSSHRef(session.credentialRefs, credentialRef) {
		return sshCommandSpec{}, fmt.Errorf("%w: SSH owner refs are unavailable or not bound", common.ErrInvalidArgument)
	}
	command := strings.TrimSpace(request.Command)
	if command == "" || len(command) > 16*1024 {
		return sshCommandSpec{}, fmt.Errorf("%w: SSH command is empty or exceeds 16384 bytes", common.ErrInvalidArgument)
	}
	timeout, err := sshCommandTimeout(request.TimeoutMillis, a.defaultTimeout())
	if err != nil {
		return sshCommandSpec{}, err
	}
	return sshCommandSpec{
		TargetRef: targetRef, ObservationRef: target.observationRef,
		Host: target.host, Port: target.port, HostKeyAlias: target.hostKeyAlias,
		KnownHosts: append([]byte(nil), target.knownHosts...), Username: session.username,
		Password: credential.password, PrivateKey: credential.privateKey,
		Command: command, ObservationCommand: target.observationCommand, Timeout: timeout,
	}, nil
}

func sshCommandTimeout(timeoutMillis int64, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}
	if timeoutMillis == 0 {
		return defaultTimeout, nil
	}
	if timeoutMillis < 1000 || timeoutMillis > 300000 {
		return 0, fmt.Errorf("%w: timeoutMillis must be between 1000 and 300000", common.ErrInvalidArgument)
	}
	return time.Duration(timeoutMillis) * time.Millisecond, nil
}

func containsSSHRef(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
