package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/sftp"
)

func registerTools(s *mcp.Server, store *ConnectionStore, sshClient *SSHClient) {
	registerConnectionTools(s, store)
	registerExecTool(s, store, sshClient)
	registerFileTools(s, store, sshClient)
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

func registerConnectionTools(s *mcp.Server, store *ConnectionStore) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_connection",
		Description: "Add a new SSH connection",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddConnectionInput) (*mcp.CallToolResult, any, error) {
		err := store.Add(Connection{
			Name:   input.Name,
			User:   input.User,
			Server: input.Server,
			Port:   input.Port,
		})
		if err != nil {
			return errResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Connection %q added", input.Name)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_connection",
		Description: "Delete an SSH connection",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteConnectionInput) (*mcp.CallToolResult, any, error) {
		if err := store.Delete(input.Name); err != nil {
			return errResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Connection %q deleted", input.Name)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_connections",
		Description: "List all SSH connections",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListConnectionsInput) (*mcp.CallToolResult, any, error) {
		conns := store.List()
		data, _ := json.Marshal(conns)
		return textResult(string(data)), nil, nil
	})
}

// --- Exec Tool ---

type ExecCommandInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
	Cmd        string `json:"cmd" jsonschema:"command to execute"`
}

func registerExecTool(s *mcp.Server, store *ConnectionStore, sshClient *SSHClient) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "exec_command",
		Description: "Execute a command on a remote server via SSH",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExecCommandInput) (*mcp.CallToolResult, any, error) {
		conn, err := store.Get(input.Connection)
		if err != nil {
			return errResult(err), nil, nil
		}
		output, err := sshClient.Exec(conn, input.Cmd)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textResult(output), nil, nil
	})
}

// --- File Tools ---

type ReadFileInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
	File       string `json:"file" jsonschema:"remote file path"`
}

type WriteFileInput struct {
	Connection string `json:"connection" jsonschema:"connection name"`
	File       string `json:"file" jsonschema:"remote file path"`
	Content    string `json:"content" jsonschema:"file content to write"`
}

func registerFileTools(s *mcp.Server, store *ConnectionStore, sshClient *SSHClient) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "read_file",
		Description: "Read a file from a remote server via SSH",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReadFileInput) (*mcp.CallToolResult, any, error) {
		conn, err := store.Get(input.Connection)
		if err != nil {
			return errResult(err), nil, nil
		}

		client, err := sshClient.Connect(conn)
		if err != nil {
			return errResult(fmt.Errorf("SSH connection failed: %w", err)), nil, nil
		}
		defer client.Close()

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return errResult(fmt.Errorf("SFTP session failed: %w", err)), nil, nil
		}
		defer sftpClient.Close()

		f, err := sftpClient.Open(input.File)
		if err != nil {
			return errResult(fmt.Errorf("failed to open file: %w", err)), nil, nil
		}
		defer f.Close()

		data, err := io.ReadAll(f)
		if err != nil {
			return errResult(fmt.Errorf("failed to read file: %w", err)), nil, nil
		}
		return textResult(string(data)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "write_file",
		Description: "Write content to a file on a remote server via SSH",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WriteFileInput) (*mcp.CallToolResult, any, error) {
		conn, err := store.Get(input.Connection)
		if err != nil {
			return errResult(err), nil, nil
		}

		client, err := sshClient.Connect(conn)
		if err != nil {
			return errResult(fmt.Errorf("SSH connection failed: %w", err)), nil, nil
		}
		defer client.Close()

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return errResult(fmt.Errorf("SFTP session failed: %w", err)), nil, nil
		}
		defer sftpClient.Close()

		f, err := sftpClient.Create(input.File)
		if err != nil {
			return errResult(fmt.Errorf("failed to create file: %w", err)), nil, nil
		}
		defer f.Close()

		if _, err := f.Write([]byte(input.Content)); err != nil {
			return errResult(fmt.Errorf("failed to write file: %w", err)), nil, nil
		}
		return textResult(fmt.Sprintf("Written %d bytes to %s", len(input.Content), input.File)), nil, nil
	})
}

// --- Helpers ---

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: err.Error()},
		},
		IsError: true,
	}
}
