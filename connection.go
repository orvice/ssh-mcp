package main

import (
	"fmt"
	"sync"
)

type Connection struct {
	Name   string `json:"name"`
	User   string `json:"user"`
	Server string `json:"server"`
	Port   int    `json:"port"`
}

type ConnectionStore struct {
	mu    sync.RWMutex
	conns map[string]Connection
}

func NewConnectionStore() *ConnectionStore {
	return &ConnectionStore{
		conns: make(map[string]Connection),
	}
}

func (s *ConnectionStore) Add(c Connection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.conns[c.Name]; exists {
		return fmt.Errorf("connection %q already exists", c.Name)
	}
	s.conns[c.Name] = c
	return nil
}

func (s *ConnectionStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.conns[name]; !exists {
		return fmt.Errorf("connection %q not found", name)
	}
	delete(s.conns, name)
	return nil
}

func (s *ConnectionStore) Get(name string) (Connection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, exists := s.conns[name]
	if !exists {
		return Connection{}, fmt.Errorf("connection %q not found", name)
	}
	return c, nil
}

func (s *ConnectionStore) List() []Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Connection, 0, len(s.conns))
	for _, c := range s.conns {
		result = append(result, c)
	}
	return result
}
