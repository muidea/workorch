package ssh

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

type sshCommandSpec struct {
	Host           string
	Port           int
	Username       string
	Password       string
	PrivateKey     string
	privateKeyPath string
	Command        string
	Timeout        time.Duration
}

func (s sshCommandSpec) authMethod() string {
	if strings.TrimSpace(s.PrivateKey) != "" || strings.TrimSpace(s.privateKeyPath) != "" {
		return "private_key"
	}
	if strings.TrimSpace(s.Password) != "" {
		return "password"
	}
	return ""
}

func sshCommandSpecFromRequest(request RuntimeCommandRequest, defaultTimeout time.Duration) (sshCommandSpec, error) {
	spec := sshCommandSpec{
		Host:     strings.TrimSpace(request.Host),
		Username: strings.TrimSpace(request.Username),
		Password: strings.TrimSpace(request.Password),
		Command:  strings.TrimSpace(request.Command),
		Port:     22,
		Timeout:  defaultTimeout,
	}
	if request.Port > 0 {
		spec.Port = int(request.Port)
	}
	if spec.Host == "" || spec.Username == "" || spec.Command == "" || spec.Port <= 0 || spec.Port > 65535 {
		return sshCommandSpec{}, common.ErrInvalidArgument
	}
	privateKey, privateKeyPath, err := sshPrivateKeyInput(request.PrivateKey, request.PrivateKeyPath, request.WorkspaceDeliverableDirectory)
	if err != nil {
		return sshCommandSpec{}, err
	}
	spec.PrivateKey = privateKey
	spec.privateKeyPath = privateKeyPath
	if strings.TrimSpace(spec.Password) == "" && strings.TrimSpace(spec.PrivateKey) == "" && strings.TrimSpace(spec.privateKeyPath) == "" {
		return sshCommandSpec{}, fmt.Errorf("%w: ssh/command requires password or privateKey/privateKeyPath", common.ErrInvalidArgument)
	}
	timeout, err := sshCommandTimeout(request.TimeoutMillis, defaultTimeout)
	if err != nil {
		return sshCommandSpec{}, err
	}
	spec.Timeout = timeout
	return spec, nil
}

func sshPrivateKeyInput(privateKey string, privateKeyPath string, workspaceDeliverableDirectory string) (string, string, error) {
	if value := strings.TrimSpace(privateKey); value != "" {
		return value, "", nil
	}
	path := strings.TrimSpace(privateKeyPath)
	if path == "" {
		return "", "", nil
	}
	if !filepath.IsAbs(path) {
		base := strings.TrimSpace(workspaceDeliverableDirectory)
		if base == "" {
			return "", "", common.ErrInvalidArgument
		}
		path = filepath.Join(base, path)
	}
	return "", path, nil
}

func sshCommandTimeout(timeoutMillis int64, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}
	if timeoutMillis == 0 {
		return defaultTimeout, nil
	}
	if timeoutMillis < 0 {
		return 0, common.ErrInvalidArgument
	}
	timeout := time.Duration(timeoutMillis) * time.Millisecond
	if timeout < time.Second {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeout > 5*time.Minute {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed 300000", common.ErrInvalidArgument)
	}
	return timeout, nil
}
