package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultListen                = "127.0.0.1:8080"
	defaultSSHTimeoutSeconds     = 15
	defaultCommandTimeoutSeconds = 60
	defaultMaxReadBytes          = 1 << 20 // 1 MiB
)

type Config struct {
	PrivateKey            string                `yaml:"private_key"`
	PrivateKeyPassphrase  string                `yaml:"private_key_passphrase"`
	Listen                string                `yaml:"listen"`
	AuthToken             string                `yaml:"auth_token"`
	KnownHosts            string                `yaml:"known_hosts"`
	InsecureIgnoreHostKey bool                  `yaml:"insecure_ignore_host_key"`
	SSHTimeoutSeconds     int                   `yaml:"ssh_timeout_seconds"`
	CommandTimeoutSeconds int                   `yaml:"command_timeout_seconds"`
	MaxReadBytes          int64                 `yaml:"max_read_bytes"`
	ConnectionStore       ConnectionStoreConfig `yaml:"connection_store"`
	Connections           []Connection          `yaml:"connections"`
	Policy                PolicyConfig          `yaml:"policy"`
	Audit                 AuditConfig           `yaml:"audit"`
}

type ConnectionStoreConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

func DefaultConfig() *Config {
	return &Config{
		Listen:                defaultListen,
		KnownHosts:            "~/.ssh/known_hosts",
		SSHTimeoutSeconds:     defaultSSHTimeoutSeconds,
		CommandTimeoutSeconds: defaultCommandTimeoutSeconds,
		MaxReadBytes:          defaultMaxReadBytes,
		ConnectionStore: ConnectionStoreConfig{
			Type: "memory",
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(ExpandPath(path))
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.Normalize()
	return cfg, nil
}

func (c *Config) Normalize() {
	if c.Listen == "" {
		c.Listen = defaultListen
	}
	if c.KnownHosts == "" {
		c.KnownHosts = "~/.ssh/known_hosts"
	}
	if c.SSHTimeoutSeconds <= 0 {
		c.SSHTimeoutSeconds = defaultSSHTimeoutSeconds
	}
	if c.CommandTimeoutSeconds <= 0 {
		c.CommandTimeoutSeconds = defaultCommandTimeoutSeconds
	}
	if c.MaxReadBytes <= 0 {
		c.MaxReadBytes = defaultMaxReadBytes
	}
	if c.ConnectionStore.Type == "" {
		c.ConnectionStore.Type = "memory"
	}
	c.ConnectionStore.Type = strings.ToLower(c.ConnectionStore.Type)
}

func (c *Config) SSHTimeout() time.Duration {
	return time.Duration(c.SSHTimeoutSeconds) * time.Second
}

func (c *Config) CommandTimeout() time.Duration {
	return time.Duration(c.CommandTimeoutSeconds) * time.Second
}

func ExpandPath(path string) string {
	if path == "" {
		return path
	}

	path = os.ExpandEnv(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
