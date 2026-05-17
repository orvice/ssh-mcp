package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SSHClient struct {
	defaultSigner   ssh.Signer
	defaultKeyPath  string
	defaultPass     string
	hostKeyCallback ssh.HostKeyCallback
	timeout         time.Duration
}

type ExecResult struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out"`
}

func NewSSHClient(privateKeyPath string, opts SSHClientOptions) (*SSHClient, error) {
	keyData, err := os.ReadFile(ExpandPath(privateKeyPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read private key %s: %w", privateKeyPath, err)
	}
	client, err := NewSSHClientFromKeyData(keyData, opts)
	if err != nil {
		return nil, err
	}
	client.defaultKeyPath = privateKeyPath
	client.defaultPass = opts.PrivateKeyPassphrase
	return client, nil
}

type SSHClientOptions struct {
	KnownHosts            string
	InsecureIgnoreHostKey bool
	Timeout               time.Duration
	PrivateKeyPassphrase  string
}

func NewSSHClientFromKeyData(keyData []byte, opts SSHClientOptions) (*SSHClient, error) {
	var signer ssh.Signer
	var err error
	if len(keyData) > 0 {
		signer, err = parsePrivateKey(keyData, opts.PrivateKeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	}

	hostKeyCallback, err := hostKeyCallback(opts)
	if err != nil {
		return nil, err
	}

	return &SSHClient{
		defaultSigner:   signer,
		defaultPass:     opts.PrivateKeyPassphrase,
		hostKeyCallback: hostKeyCallback,
		timeout:         opts.Timeout,
	}, nil
}

func parsePrivateKey(keyData []byte, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(keyData)
}

func hostKeyCallback(opts SSHClientOptions) (ssh.HostKeyCallback, error) {
	if opts.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsPath := ExpandPath(opts.KnownHosts)
	if knownHostsPath == "" {
		return nil, errors.New("known_hosts path is required unless insecure_ignore_host_key is enabled")
	}

	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load known_hosts %s: %w", knownHostsPath, err)
	}
	return callback, nil
}

func (c *SSHClient) Connect(conn Connection) (*ssh.Client, error) {
	signer := c.defaultSigner
	if conn.PrivateKey != "" {
		keyData, err := os.ReadFile(ExpandPath(conn.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("failed to read connection private key %s: %w", conn.PrivateKey, err)
		}
		signer, err = parsePrivateKey(keyData, conn.PrivateKeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("failed to parse connection private key %s: %w", conn.PrivateKey, err)
		}
	}
	if signer == nil {
		return nil, errors.New("no SSH private key configured")
	}

	config := &ssh.ClientConfig{
		User: conn.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: c.hostKeyCallback,
		Timeout:         c.timeout,
	}
	addr := fmt.Sprintf("%s:%d", conn.Server, conn.Port)
	return ssh.Dial("tcp", addr, config)
}

func (c *SSHClient) TestConnection(conn Connection) error {
	client, err := c.Connect(conn)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	return client.Close()
}

func (c *SSHClient) Exec(ctx context.Context, conn Connection, cmd string) (*ExecResult, error) {
	started := time.Now()
	result := &ExecResult{ExitCode: 0}

	client, err := c.Connect(conn)
	if err != nil {
		result.DurationMS = time.Since(started).Milliseconds()
		return result, fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		result.DurationMS = time.Since(started).Milliseconds()
		return result, fmt.Errorf("SSH session failed: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	var runErr error
	select {
	case runErr = <-done:
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		result.ExitCode = -1
		result.DurationMS = time.Since(started).Milliseconds()
		result.TimedOut = true
		return result, fmt.Errorf("command timed out or was cancelled: %w", ctx.Err())
	}

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.DurationMS = time.Since(started).Milliseconds()
	if runErr != nil {
		if exitErr, ok := runErr.(*ssh.ExitError); ok {
			result.ExitCode = exitErr.ExitStatus()
			return result, fmt.Errorf("command exited with code %d", exitErr.ExitStatus())
		}
		result.ExitCode = -1
		return result, runErr
	}
	return result, nil
}
