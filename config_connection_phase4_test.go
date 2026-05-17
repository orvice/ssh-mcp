package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigNormalize(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	if cfg.Listen != defaultListen {
		t.Fatalf("Listen = %q, want %q", cfg.Listen, defaultListen)
	}
	if cfg.ConnectionStore.Type != "memory" {
		t.Fatalf("ConnectionStore.Type = %q, want memory", cfg.ConnectionStore.Type)
	}
	if cfg.MaxReadBytes != defaultMaxReadBytes {
		t.Fatalf("MaxReadBytes = %d, want %d", cfg.MaxReadBytes, defaultMaxReadBytes)
	}
}

func TestMemoryConnectionStoreValidationAndSorting(t *testing.T) {
	store := NewMemoryConnectionStore()
	if err := store.Add(Connection{Name: "b", User: "u", Server: "host", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Connection{Name: "a", User: "u", Server: "host", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Connection{Name: "bad name", User: "u", Server: "host", Port: 22}); err == nil {
		t.Fatal("expected invalid connection name error")
	}
	list := store.List()
	if len(list) != 2 || list[0].Name != "a" || list[1].Name != "b" {
		t.Fatalf("unexpected sorted list: %+v", list)
	}
}

func TestFileConnectionStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.yaml")
	store, err := NewFileConnectionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Connection{Name: "prod", User: "ubuntu", Server: "example.com", Port: 22}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewFileConnectionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := reloaded.Get("prod")
	if err != nil {
		t.Fatal(err)
	}
	if conn.Server != "example.com" {
		t.Fatalf("Server = %q", conn.Server)
	}
}

func TestPolicy(t *testing.T) {
	policy := NewPolicy(PolicyConfig{
		AllowedConnections: []string{"prod"},
		DeniedCommands:     []string{"rm -rf"},
		AllowedPaths:       []string{"/var/log"},
		ReadOnly:           true,
	})
	if err := policy.CheckConnection("dev"); err == nil {
		t.Fatal("expected disallowed connection error")
	}
	if err := policy.CheckExec("prod", "rm -rf /"); err == nil {
		t.Fatal("expected denied command error")
	}
	if err := policy.CheckRead("prod", "/etc/passwd"); err == nil {
		t.Fatal("expected denied path error")
	}
	if err := policy.CheckRead("prod", "/var/log/syslog"); err != nil {
		t.Fatalf("unexpected allowed path error: %v", err)
	}
	if err := policy.CheckWrite("prod", "/var/log/app.log"); err == nil {
		t.Fatal("expected readonly write error")
	}
}

func TestAuditLoggerWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := NewAuditLogger(AuditConfig{Enabled: true, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	logger.Log(AuditEvent{Tool: "exec_command", Connection: "prod", Success: true})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var event AuditEvent
	if err := json.Unmarshal(data[:len(data)-1], &event); err != nil {
		t.Fatal(err)
	}
	if event.Tool != "exec_command" || !event.Success {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestMetrics(t *testing.T) {
	metrics := NewMetrics()
	metrics.IncTool("exec_command", false)
	metrics.IncTool("exec_command", true)
	snapshot := metrics.Snapshot()
	toolCalls := snapshot["tool_calls"].(map[string]int64)
	toolErrors := snapshot["tool_errors"].(map[string]int64)
	if toolCalls["exec_command"] != 2 || toolErrors["exec_command"] != 1 {
		t.Fatalf("unexpected metrics: %+v", snapshot)
	}
}

func TestBearerAuth(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := bearerAuth(next, "secret")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rr.Code)
	}
}
