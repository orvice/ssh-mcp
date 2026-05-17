package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PolicyConfig struct {
	AllowedConnections []string `yaml:"allowed_connections"`
	DeniedCommands     []string `yaml:"denied_commands"`
	AllowedPaths       []string `yaml:"allowed_paths"`
	ReadOnly           bool     `yaml:"readonly"`
	DisableExec        bool     `yaml:"disable_exec"`
	DisableWrite       bool     `yaml:"disable_write"`
}

type AuditConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type Policy struct {
	cfg                PolicyConfig
	allowedConnections map[string]struct{}
}

func NewPolicy(cfg PolicyConfig) *Policy {
	allowed := make(map[string]struct{}, len(cfg.AllowedConnections))
	for _, name := range cfg.AllowedConnections {
		name = strings.TrimSpace(name)
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	return &Policy{cfg: cfg, allowedConnections: allowed}
}

func (p *Policy) CheckConnection(name string) error {
	if len(p.allowedConnections) == 0 {
		return nil
	}
	if _, ok := p.allowedConnections[name]; !ok {
		return fmt.Errorf("connection %q is not allowed by policy", name)
	}
	return nil
}

func (p *Policy) CheckExec(conn, cmd string) error {
	if err := p.CheckConnection(conn); err != nil {
		return err
	}
	if p.cfg.DisableExec {
		return fmt.Errorf("exec_command is disabled by policy")
	}
	for _, denied := range p.cfg.DeniedCommands {
		denied = strings.TrimSpace(denied)
		if denied != "" && strings.Contains(cmd, denied) {
			return fmt.Errorf("command is denied by policy")
		}
	}
	return nil
}

func (p *Policy) CheckRead(conn, path string) error {
	if err := p.CheckConnection(conn); err != nil {
		return err
	}
	return p.checkPath(path)
}

func (p *Policy) CheckWrite(conn, path string) error {
	if err := p.CheckConnection(conn); err != nil {
		return err
	}
	if p.cfg.ReadOnly || p.cfg.DisableWrite {
		return fmt.Errorf("write operations are disabled by policy")
	}
	return p.checkPath(path)
}

func (p *Policy) checkPath(path string) error {
	if len(p.cfg.AllowedPaths) == 0 {
		return nil
	}
	cleanPath := filepath.Clean(path)
	for _, allowed := range p.cfg.AllowedPaths {
		allowed = filepath.Clean(allowed)
		if cleanPath == allowed || strings.HasPrefix(cleanPath, allowed+string(os.PathSeparator)) {
			return nil
		}
	}
	return fmt.Errorf("path %q is not allowed by policy", path)
}

type AuditLogger struct {
	mu      sync.Mutex
	enabled bool
	path    string
}

type AuditEvent struct {
	Time       time.Time `json:"time"`
	Tool       string    `json:"tool"`
	Connection string    `json:"connection,omitempty"`
	Target     string    `json:"target,omitempty"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
	Bytes      int       `json:"bytes,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

func NewAuditLogger(cfg AuditConfig) (*AuditLogger, error) {
	logger := &AuditLogger{enabled: cfg.Enabled, path: ExpandPath(cfg.Path)}
	if !logger.enabled {
		return logger, nil
	}
	if logger.path == "" {
		return nil, fmt.Errorf("audit.path is required when audit.enabled is true")
	}
	if err := os.MkdirAll(filepath.Dir(logger.path), 0o700); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %w", err)
	}
	return logger, nil
}

func (l *AuditLogger) Log(event AuditEvent) {
	if l == nil || !l.enabled {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

type Metrics struct {
	mu           sync.Mutex
	startedAt    time.Time
	HTTPRequests map[string]int64 `json:"http_requests"`
	ToolCalls    map[string]int64 `json:"tool_calls"`
	ToolErrors   map[string]int64 `json:"tool_errors"`
}

func NewMetrics() *Metrics {
	return &Metrics{
		startedAt:    time.Now().UTC(),
		HTTPRequests: make(map[string]int64),
		ToolCalls:    make(map[string]int64),
		ToolErrors:   make(map[string]int64),
	}
}

func (m *Metrics) IncTool(tool string, isError bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ToolCalls[tool]++
	if isError {
		m.ToolErrors[tool]++
	}
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.HTTPRequests[r.URL.Path]++
		m.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

func (m *Metrics) Snapshot() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]any{
		"started_at":     m.startedAt,
		"uptime_seconds": int64(time.Since(m.startedAt).Seconds()),
		"http_requests": cloneMap(m.HTTPRequests),
		"tool_calls":    cloneMap(m.ToolCalls),
		"tool_errors":   cloneMap(m.ToolErrors),
	}
}

func cloneMap(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
