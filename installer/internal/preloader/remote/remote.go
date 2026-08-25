package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/progress"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

type SSHOptions struct {
	Host           string
	User           string
	PrivateKeyPath string
	Port           int
}

func NewSSHClient(opts SSHOptions) (*SSHClient, error) {
	privateKey, err := os.ReadFile(opts.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read ssh private key %q: %w", opts.PrivateKeyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse ssh private key %q: %w", opts.PrivateKeyPath, err)
	}

	sshConfig := &ssh.ClientConfig{
		User: opts.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},

		// TODO: Replace with known_hosts verification if host keys are available.
		// This is acceptable for first-contact provisioning only if the installer
		// already trusts the target network.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),

		Timeout: 20 * time.Second,
	}

	address := net.JoinHostPort(opts.Host, fmt.Sprintf("%d", opts.Port))

	sshClient, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("connect to ssh host %s: %w", address, err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("open sftp client for %s: %w", address, err)
	}

	return &SSHClient{
		sshClient:  sshClient,
		sftpClient: sftpClient,
	}, nil
}

func (s *SSHClient) Run(ctx context.Context, command Command) (stdout string, stderr string, err error) {
	if s == nil || s.sshClient == nil {
		return "", "", fmt.Errorf("ssh client is not initialized")
	}

	session, err := s.sshClient.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("open ssh session: %w", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command.String())
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return stdoutBuf.String(), stderrBuf.String(), ctx.Err()
	case err := <-done:
		if err != nil {
			return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("run remote command %q: %w", command, err)
		}

		return stdoutBuf.String(), stderrBuf.String(), nil
	}
}

func (s *SSHClient) Upload(
	ctx context.Context,
	localPath string,
	remotePath string,
	mode os.FileMode,
	tracker *progress.Tracker,
) error {
	if s == nil || s.sshClient == nil {
		return fmt.Errorf("ssh client is not initialized")
	}

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %q: %w", localPath, err)
	}
	defer src.Close()

	remoteDir := path.Dir(remotePath)
	if err := s.sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("create remote directory %q: %w", remoteDir, err)
	}

	dst, err := s.sftpClient.OpenFile(remotePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return fmt.Errorf("open remote file %q: %w", remotePath, err)
	}
	defer dst.Close()

	done := make(chan error, 1)
	go func() {
		reader := tracker.Reader(src)

		_, copyErr := io.Copy(dst, reader)
		if copyErr == nil && mode != 0 {
			copyErr = dst.Chmod(mode)
		}
		done <- copyErr
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case err := <-done:
		if err != nil {
			return fmt.Errorf("upload %q to %q: %w", localPath, remotePath, err)
		}
		return nil
	}
}

func (s *SSHClient) Close() error {
	var errs []error

	if s.sftpClient != nil {
		errs = append(errs, s.sftpClient.Close())
	}
	if s.sshClient != nil {
		errs = append(errs, s.sshClient.Close())
	}

	return errors.Join(errs...)
}
