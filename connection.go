package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Connection struct {
	Name                 string `json:"name" yaml:"name"`
	User                 string `json:"user" yaml:"user"`
	Server               string `json:"server" yaml:"server"`
	Port                 int    `json:"port" yaml:"port"`
	PrivateKey           string `json:"private_key,omitempty" yaml:"private_key,omitempty"`
	PrivateKeyPassphrase string `json:"-" yaml:"private_key_passphrase,omitempty"`
}

type ConnectionStore interface {
	Add(Connection) error
	Delete(string) error
	Get(string) (Connection, error)
	List() []Connection
}

type MemoryConnectionStore struct {
	mu    sync.RWMutex
	conns map[string]Connection
}

func NewConnectionStore() *MemoryConnectionStore {
	return NewMemoryConnectionStore()
}

func NewMemoryConnectionStore() *MemoryConnectionStore {
	return &MemoryConnectionStore{
		conns: make(map[string]Connection),
	}
}

func NewConfiguredConnectionStore(cfg ConnectionStoreConfig) (ConnectionStore, error) {
	switch cfg.Type {
	case "", "memory":
		return NewMemoryConnectionStore(), nil
	case "file":
		if cfg.Path == "" {
			return nil, fmt.Errorf("connection_store.path is required when connection_store.type is file")
		}
		return NewFileConnectionStore(cfg.Path)
	default:
		return nil, fmt.Errorf("unsupported connection_store.type %q", cfg.Type)
	}
}

func (s *MemoryConnectionStore) Add(c Connection) error {
	if err := c.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.conns[c.Name]; exists {
		return fmt.Errorf("connection %q already exists", c.Name)
	}
	s.conns[c.Name] = c
	return nil
}

func (s *MemoryConnectionStore) Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("connection name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.conns[name]; !exists {
		return fmt.Errorf("connection %q not found", name)
	}
	delete(s.conns, name)
	return nil
}

func (s *MemoryConnectionStore) Get(name string) (Connection, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Connection{}, fmt.Errorf("connection name is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c, exists := s.conns[name]
	if !exists {
		return Connection{}, fmt.Errorf("connection %q not found", name)
	}
	return c, nil
}

func (s *MemoryConnectionStore) List() []Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedConnections(s.conns)
}

type FileConnectionStore struct {
	mu    sync.RWMutex
	path  string
	conns map[string]Connection
}

type connectionStoreFile struct {
	Connections []Connection `yaml:"connections" json:"connections"`
}

func NewFileConnectionStore(path string) (*FileConnectionStore, error) {
	store := &FileConnectionStore{
		path:  ExpandPath(path),
		conns: make(map[string]Connection),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileConnectionStore) Add(c Connection) error {
	if err := c.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.conns[c.Name]; exists {
		return fmt.Errorf("connection %q already exists", c.Name)
	}
	s.conns[c.Name] = c
	return s.saveLocked()
}

func (s *FileConnectionStore) Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("connection name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.conns[name]; !exists {
		return fmt.Errorf("connection %q not found", name)
	}
	delete(s.conns, name)
	return s.saveLocked()
}

func (s *FileConnectionStore) Get(name string) (Connection, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Connection{}, fmt.Errorf("connection name is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c, exists := s.conns[name]
	if !exists {
		return Connection{}, fmt.Errorf("connection %q not found", name)
	}
	return c, nil
}

func (s *FileConnectionStore) List() []Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedConnections(s.conns)
}

func (s *FileConnectionStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read connection store %s: %w", s.path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var file connectionStoreFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("failed to parse connection store %s: %w", s.path, err)
	}
	for _, conn := range file.Connections {
		if err := conn.Validate(); err != nil {
			return fmt.Errorf("invalid connection %q in store: %w", conn.Name, err)
		}
		if _, exists := s.conns[conn.Name]; exists {
			return fmt.Errorf("duplicate connection %q in store", conn.Name)
		}
		s.conns[conn.Name] = conn
	}
	return nil
}

func (s *FileConnectionStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("failed to create connection store directory: %w", err)
	}

	file := connectionStoreFile{Connections: sortedConnections(s.conns)}
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to encode connection store: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write connection store %s: %w", s.path, err)
	}
	return nil
}

func AddConfiguredConnections(store ConnectionStore, conns []Connection) error {
	for _, conn := range conns {
		if _, err := store.Get(conn.Name); err == nil {
			continue
		}
		if err := store.Add(conn); err != nil {
			return fmt.Errorf("failed to add configured connection %q: %w", conn.Name, err)
		}
	}
	return nil
}

func sortedConnections(conns map[string]Connection) []Connection {
	result := make([]Connection, 0, len(conns))
	for _, c := range conns {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (c Connection) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("connection name is required")
	}
	if strings.TrimSpace(c.User) == "" {
		return fmt.Errorf("connection user is required")
	}
	if strings.TrimSpace(c.Server) == "" {
		return fmt.Errorf("connection server is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("connection port must be between 1 and 65535")
	}
	if strings.ContainsAny(c.Name, " \t\n\r/") {
		return fmt.Errorf("connection name must not contain whitespace or slash characters")
	}
	return nil
}
