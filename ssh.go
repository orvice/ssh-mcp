package main

import (
	"bytes"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	signer ssh.Signer
}

func NewSSHClient(privateKeyPath string) (*SSHClient, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key %s: %w", privateKeyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	return &SSHClient{signer: signer}, nil
}

func (c *SSHClient) Connect(conn Connection) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: conn.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(c.signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	addr := fmt.Sprintf("%s:%d", conn.Server, conn.Port)
	return ssh.Dial("tcp", addr, config)
}

func (c *SSHClient) Exec(conn Connection, cmd string) (string, error) {
	client, err := c.Connect(conn)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session failed: %w", err)
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf

	err = session.Run(cmd)
	output := buf.String()
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return output, fmt.Errorf("command exited with code %d: %s", exitErr.ExitStatus(), output)
		}
		return output, err
	}
	return output, nil
}
