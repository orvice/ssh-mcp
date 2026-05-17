package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/sftp"
)

func registerTools(s *mcp.Server, store ConnectionStore, sshClient *SSHClient, cfg *Config, policy *Policy, audit *AuditLogger, metrics *Metrics) {
	registerConnectionTools(s, store, sshClient, policy, audit, metrics)
	registerExecTool(s, store, sshClient, cfg, policy, audit, metrics)
	registerFileTools(s, store, sshClient, cfg, policy, audit, metrics)
}

// --- Connection Management Tools ---

type AddConnectionInput struct {
	Name   string `json:"name" jsonschema:"connection name"`
	User   string `json:"user" jsonschema:"SSH username"`
	Server string `json:"server" jsonschema:"server hostname or IP"`
	Port   int    `json:"port" jsonschema:"SSH port"`
}

type DeleteConnectionInput struct {
	Name string `json:"name" jsonschema:"connection name to delete"`
}

type ListConnectionsInput struct{}

type TestConnectionInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
}

func registerConnectionTools(s *mcp.Server, store ConnectionStore, sshClient *SSHClient, policy *Policy, audit *AuditLogger, metrics *Metrics) {
	mcp.AddTool(s, &mcp.Tool{Name: "add_connection", Description: "Add a new SSH connection"},
		func(ctx context.Context, req *mcp.CallToolRequest, input AddConnectionInput) (*mcp.CallToolResult, any, error) {
			tool := "add_connection"
			if err := policy.CheckConnection(input.Name); err != nil {
				return toolErr(tool, metrics, err)
			}
			err := store.Add(Connection{Name: input.Name, User: input.User, Server: input.Server, Port: input.Port})
			if err != nil {
				return toolErr(tool, metrics, err)
			}
			return toolText(tool, metrics, fmt.Sprintf("Connection %q added", input.Name))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "delete_connection", Description: "Delete an SSH connection"},
		func(ctx context.Context, req *mcp.CallToolRequest, input DeleteConnectionInput) (*mcp.CallToolResult, any, error) {
			tool := "delete_connection"
			if err := policy.CheckConnection(input.Name); err != nil {
				return toolErr(tool, metrics, err)
			}
			if err := store.Delete(input.Name); err != nil {
				return toolErr(tool, metrics, err)
			}
			return toolText(tool, metrics, fmt.Sprintf("Connection %q deleted", input.Name))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "list_connections", Description: "List all SSH connections"},
		func(ctx context.Context, req *mcp.CallToolRequest, input ListConnectionsInput) (*mcp.CallToolResult, any, error) {
			tool := "list_connections"
			conns := store.List()
			if len(policy.allowedConnections) > 0 {
				filtered := make([]Connection, 0, len(conns))
				for _, conn := range conns {
					if err := policy.CheckConnection(conn.Name); err == nil {
						filtered = append(filtered, conn)
					}
				}
				conns = filtered
			}
			return toolJSON(tool, metrics, conns)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "test_connection", Description: "Test whether an SSH connection can be established"},
		func(ctx context.Context, req *mcp.CallToolRequest, input TestConnectionInput) (*mcp.CallToolResult, any, error) {
			tool := "test_connection"
			if err := policy.CheckConnection(input.Connection); err != nil {
				return toolErr(tool, metrics, err)
			}
			conn, err := store.Get(input.Connection)
			if err != nil {
				return toolErr(tool, metrics, err)
			}
			started := time.Now()
			auditEvent := AuditEvent{Tool: tool, Connection: conn.Name, Target: conn.Server}
			if err := sshClient.TestConnection(conn); err != nil {
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			finishAudit(audit, auditEvent, started, nil)
			return toolJSON(tool, metrics, map[string]any{"connection": conn.Name, "ok": true, "duration_ms": time.Since(started).Milliseconds()})
		})
}

// --- Exec Tool ---

type ExecCommandInput struct {
	Connection     string `json:"connection" jsonschema:"connection name"`
	Cmd            string `json:"cmd" jsonschema:"command to execute"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"optional command timeout in seconds"`
}

func registerExecTool(s *mcp.Server, store ConnectionStore, sshClient *SSHClient, cfg *Config, policy *Policy, audit *AuditLogger, metrics *Metrics) {
	mcp.AddTool(s, &mcp.Tool{Name: "exec_command", Description: "Execute a command on a remote server via SSH and return structured stdout/stderr/exit status"},
		func(ctx context.Context, req *mcp.CallToolRequest, input ExecCommandInput) (*mcp.CallToolResult, any, error) {
			tool := "exec_command"
			if input.Cmd == "" {
				return toolErr(tool, metrics, fmt.Errorf("cmd is required"))
			}
			if err := policy.CheckExec(input.Connection, input.Cmd); err != nil {
				return toolErr(tool, metrics, err)
			}
			conn, err := store.Get(input.Connection)
			if err != nil {
				return toolErr(tool, metrics, err)
			}

			timeout := cfg.CommandTimeout()
			if input.TimeoutSeconds > 0 {
				timeout = time.Duration(input.TimeoutSeconds) * time.Second
			}
			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			auditEvent := AuditEvent{Tool: tool, Connection: conn.Name, Target: input.Cmd}
			result, err := sshClient.Exec(cmdCtx, conn, input.Cmd)
			finishAudit(audit, auditEvent, started, err)
			if err != nil {
				return recordTool(metrics, tool, jsonErrorResult(result, err)), nil, nil
			}
			return toolJSON(tool, metrics, result)
		})
}

// --- File Tools ---

type ReadFileInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
	File       string `json:"file" jsonschema:"remote file path"`
	MaxBytes   int64  `json:"max_bytes,omitempty" jsonschema:"maximum bytes to read"`
}

type WriteFileInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
	File       string `json:"file" jsonschema:"remote file path"`
	Content    string `json:"content" jsonschema:"file content to write"`
	Overwrite  bool   `json:"overwrite,omitempty" jsonschema:"must be true to overwrite an existing remote file"`
}

type StatFileInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
	File       string `json:"file" jsonschema:"remote file path"`
}

type ListDirectoryInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
	Path       string `json:"path" jsonschema:"remote directory path"`
}

type FileInfoResult struct {
	Name    string    `json:"name"`
	Path    string    `json:"path,omitempty"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

func registerFileTools(s *mcp.Server, store ConnectionStore, sshClient *SSHClient, cfg *Config, policy *Policy, audit *AuditLogger, metrics *Metrics) {
	mcp.AddTool(s, &mcp.Tool{Name: "read_file", Description: "Read a file from a remote server via SSH"},
		func(ctx context.Context, req *mcp.CallToolRequest, input ReadFileInput) (*mcp.CallToolResult, any, error) {
			tool := "read_file"
			if input.File == "" {
				return toolErr(tool, metrics, fmt.Errorf("file is required"))
			}
			if err := policy.CheckRead(input.Connection, input.File); err != nil {
				return toolErr(tool, metrics, err)
			}
			conn, err := store.Get(input.Connection)
			if err != nil {
				return toolErr(tool, metrics, err)
			}
			started := time.Now()
			auditEvent := AuditEvent{Tool: tool, Connection: conn.Name, Target: input.File}

			sftpClient, closeFn, err := openSFTP(sshClient, conn)
			if err != nil {
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			defer closeFn()

			f, err := sftpClient.Open(input.File)
			if err != nil {
				err = fmt.Errorf("failed to open file: %w", err)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			defer f.Close()

			maxBytes := cfg.MaxReadBytes
			if input.MaxBytes > 0 {
				maxBytes = input.MaxBytes
			}
			data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
			if err != nil {
				err = fmt.Errorf("failed to read file: %w", err)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			if int64(len(data)) > maxBytes {
				err = fmt.Errorf("file exceeds read limit of %d bytes", maxBytes)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			auditEvent.Bytes = len(data)
			finishAudit(audit, auditEvent, started, nil)
			return toolText(tool, metrics, string(data))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "write_file", Description: "Write content to a file on a remote server via SSH"},
		func(ctx context.Context, req *mcp.CallToolRequest, input WriteFileInput) (*mcp.CallToolResult, any, error) {
			tool := "write_file"
			if input.File == "" {
				return toolErr(tool, metrics, fmt.Errorf("file is required"))
			}
			if err := policy.CheckWrite(input.Connection, input.File); err != nil {
				return toolErr(tool, metrics, err)
			}
			conn, err := store.Get(input.Connection)
			if err != nil {
				return toolErr(tool, metrics, err)
			}
			started := time.Now()
			auditEvent := AuditEvent{Tool: tool, Connection: conn.Name, Target: input.File, Bytes: len(input.Content)}

			sftpClient, closeFn, err := openSFTP(sshClient, conn)
			if err != nil {
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			defer closeFn()

			if _, err := sftpClient.Stat(input.File); err == nil && !input.Overwrite {
				err = fmt.Errorf("remote file %q already exists; set overwrite=true to replace it", input.File)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			f, err := sftpClient.Create(input.File)
			if err != nil {
				err = fmt.Errorf("failed to create file: %w", err)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			defer f.Close()

			if _, err := f.Write([]byte(input.Content)); err != nil {
				err = fmt.Errorf("failed to write file: %w", err)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			finishAudit(audit, auditEvent, started, nil)
			return toolJSON(tool, metrics, map[string]any{"file": input.File, "bytes_written": len(input.Content), "overwritten": input.Overwrite})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "stat_file", Description: "Return metadata for a remote file or directory via SFTP"},
		func(ctx context.Context, req *mcp.CallToolRequest, input StatFileInput) (*mcp.CallToolResult, any, error) {
			tool := "stat_file"
			if input.File == "" {
				return toolErr(tool, metrics, fmt.Errorf("file is required"))
			}
			if err := policy.CheckRead(input.Connection, input.File); err != nil {
				return toolErr(tool, metrics, err)
			}
			conn, err := store.Get(input.Connection)
			if err != nil {
				return toolErr(tool, metrics, err)
			}
			started := time.Now()
			auditEvent := AuditEvent{Tool: tool, Connection: conn.Name, Target: input.File}
			sftpClient, closeFn, err := openSFTP(sshClient, conn)
			if err != nil {
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			defer closeFn()
			info, err := sftpClient.Stat(input.File)
			if err != nil {
				err = fmt.Errorf("failed to stat file: %w", err)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			finishAudit(audit, auditEvent, started, nil)
			return toolJSON(tool, metrics, fileInfoResult(input.File, info))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "list_directory", Description: "List entries in a remote directory via SFTP"},
		func(ctx context.Context, req *mcp.CallToolRequest, input ListDirectoryInput) (*mcp.CallToolResult, any, error) {
			tool := "list_directory"
			if input.Path == "" {
				return toolErr(tool, metrics, fmt.Errorf("path is required"))
			}
			if err := policy.CheckRead(input.Connection, input.Path); err != nil {
				return toolErr(tool, metrics, err)
			}
			conn, err := store.Get(input.Connection)
			if err != nil {
				return toolErr(tool, metrics, err)
			}
			started := time.Now()
			auditEvent := AuditEvent{Tool: tool, Connection: conn.Name, Target: input.Path}
			sftpClient, closeFn, err := openSFTP(sshClient, conn)
			if err != nil {
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			defer closeFn()
			entries, err := sftpClient.ReadDir(input.Path)
			if err != nil {
				err = fmt.Errorf("failed to list directory: %w", err)
				finishAudit(audit, auditEvent, started, err)
				return toolErr(tool, metrics, err)
			}
			results := make([]FileInfoResult, 0, len(entries))
			for _, entry := range entries {
				results = append(results, fileInfoResult("", entry))
			}
			finishAudit(audit, auditEvent, started, nil)
			return toolJSON(tool, metrics, map[string]any{"path": input.Path, "entries": results})
		})
}

func openSFTP(sshClient *SSHClient, conn Connection) (*sftp.Client, func(), error) {
	client, err := sshClient.Connect(conn)
	if err != nil {
		return nil, nil, fmt.Errorf("SSH connection failed: %w", err)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("SFTP session failed: %w", err)
	}

	return sftpClient, func() { _ = sftpClient.Close(); _ = client.Close() }, nil
}

func fileInfoResult(path string, info os.FileInfo) FileInfoResult {
	return FileInfoResult{Name: info.Name(), Path: path, Size: info.Size(), Mode: info.Mode().String(), ModTime: info.ModTime(), IsDir: info.IsDir()}
}

// --- Helpers ---

func recordTool(metrics *Metrics, tool string, result *mcp.CallToolResult) *mcp.CallToolResult {
	if metrics != nil {
		metrics.IncTool(tool, result != nil && result.IsError)
	}
	return result
}

func finishAudit(audit *AuditLogger, event AuditEvent, started time.Time, err error) {
	if audit == nil {
		return
	}
	event.DurationMS = time.Since(started).Milliseconds()
	event.Success = err == nil
	if err != nil {
		event.Error = err.Error()
	}
	audit.Log(event)
}

func toolErr(tool string, metrics *Metrics, err error) (*mcp.CallToolResult, any, error) {
	return recordTool(metrics, tool, errResult(err)), nil, nil
}

func toolJSON(tool string, metrics *Metrics, value any) (*mcp.CallToolResult, any, error) {
	return recordTool(metrics, tool, jsonResult(value)), nil, nil
}

func toolText(tool string, metrics *Metrics, text string) (*mcp.CallToolResult, any, error) {
	return recordTool(metrics, tool, textResult(text)), nil, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func jsonResult(value any) *mcp.CallToolResult {
	data, err := json.Marshal(value)
	if err != nil {
		return errResult(fmt.Errorf("failed to encode JSON result: %w", err))
	}
	return textResult(string(data))
}

func jsonErrorResult(value any, err error) *mcp.CallToolResult {
	payload := map[string]any{"error": err.Error()}
	if value != nil {
		payload["result"] = value
	}
	result := jsonResult(payload)
	result.IsError = true
	return result
}

func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}
}
