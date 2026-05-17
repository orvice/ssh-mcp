# ssh-mcp

An MCP (Model Context Protocol) server that exposes SSH remote operations as MCP tools, including command execution and SFTP-based file operations.

## Features

- Execute commands on remote servers via SSH.
- Read and write files on remote servers via SFTP.
- Manage multiple SSH connections.
- Load SSH keys from local files, config file, 1Password, or Consul KV.
- Support passphrase-protected private keys.
- Support connection-level SSH keys.
- Optional file-backed connection store for persistence.
- Optional Bearer token authentication for the HTTP/SSE MCP endpoint.
- SSH host key verification through `known_hosts` by default.
- Optional policy guardrails for connections, commands, and file paths.
- Optional JSONL audit logging.
- Health and JSON metrics endpoints.

## Installation

```bash
go install github.com/orvice/ssh-mcp@latest
```

## Quick Start

### Local private key

```bash
ssh-mcp --ssh-key ~/.ssh/id_ed25519
```

By default the server listens on `127.0.0.1:8080` and verifies remote host keys with `~/.ssh/known_hosts`.

### With HTTP Bearer authentication

```bash
ssh-mcp \
  --ssh-key ~/.ssh/id_ed25519 \
  --auth-token "$SSH_MCP_TOKEN"
```

Clients must send:

```http
Authorization: Bearer <token>
```

### With 1Password

Set the `OP_SERVICE_ACCOUNT_TOKEN` environment variable and pass a secret reference:

```bash
export OP_SERVICE_ACCOUNT_TOKEN="your-token"
ssh-mcp --op-ssh-key "op://vault/item/private_key"
```

### With Consul KV

Store your SSH key in Consul KV and pass the key path. Consul address is configured via `CONSUL_HTTP_ADDR` and defaults to `127.0.0.1:8500`:

```bash
ssh-mcp --consul-ssh-key "ssh/keys/my-server"
```

### With config file

```bash
ssh-mcp --config config.yaml
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to YAML config file | |
| `--ssh-key` | Path to SSH private key file | |
| `--op-ssh-key` | 1Password secret reference, e.g. `op://vault/item/private_key` | |
| `--consul-ssh-key` | Consul KV path, e.g. `ssh/keys/my-server` | |
| `--listen` | Listen address | `127.0.0.1:8080` |
| `--auth-token` | Bearer token required for MCP HTTP requests | |
| `--known-hosts` | `known_hosts` file for SSH host key verification | `~/.ssh/known_hosts` |
| `--insecure-ignore-host-key` | Disable SSH host key verification. Unsafe. | `false` |

SSH key source priority:

1. `--op-ssh-key`
2. `--consul-ssh-key`
3. `--ssh-key`
4. config file `private_key`
5. connection-level `private_key`

## Configuration

Example:

```yaml
listen: "127.0.0.1:8080"
auth_token: ""

private_key: ~/.ssh/id_rsa
private_key_passphrase: ""

known_hosts: ~/.ssh/known_hosts
insecure_ignore_host_key: false
ssh_timeout_seconds: 15
command_timeout_seconds: 60
max_read_bytes: 1048576

connection_store:
  type: memory # or "file"
  # path: ~/.config/ssh-mcp/connections.yaml

connections:
  - name: example
    user: ubuntu
    server: 127.0.0.1
    port: 22
    # Optional per-connection key override:
    # private_key: ~/.ssh/example_ed25519
    # private_key_passphrase: ""

policy:
  allowed_connections: []
  denied_commands:
    - "rm -rf /"
  allowed_paths: []
  readonly: false
  disable_exec: false
  disable_write: false

audit:
  enabled: false
  path: ~/.config/ssh-mcp/audit.jsonl
```

### Path expansion

The following paths support `~` and environment variable expansion:

- `private_key`
- `known_hosts`
- `connection_store.path`
- connection-level `private_key`

Examples:

```yaml
private_key: ~/.ssh/id_ed25519
known_hosts: $HOME/.ssh/known_hosts
```

### Connection store

By default, connections are stored in memory and are lost after restart:

```yaml
connection_store:
  type: memory
```

Use a file-backed store to persist `add_connection` and `delete_connection` changes:

```yaml
connection_store:
  type: file
  path: ~/.config/ssh-mcp/connections.yaml
```

The store file is written with `0600` permissions. Its parent directory is created with `0700` permissions.

### Connection-level SSH keys

A connection can override the global key:

```yaml
connections:
  - name: prod
    user: ubuntu
    server: prod.example.com
    port: 22
    private_key: ~/.ssh/prod_ed25519
    private_key_passphrase: ""
```

If every connection has its own `private_key`, a global `private_key` is not required.

## Security Notes

### Configure HTTP authentication

This server can execute commands and read/write files on remote hosts. If the HTTP endpoint is reachable by others, configure `auth_token` or `--auth-token`.

```yaml
auth_token: "use-a-long-random-token"
```

The default listen address is `127.0.0.1:8080`. Avoid binding to `:8080` unless you understand the exposure and have authentication in place.

### Keep SSH host key verification enabled

By default, SSH host keys are verified with `known_hosts`:

```yaml
known_hosts: ~/.ssh/known_hosts
insecure_ignore_host_key: false
```

Only use insecure mode for local testing or controlled environments:

```bash
ssh-mcp --ssh-key ~/.ssh/id_ed25519 --insecure-ignore-host-key
```

### Avoid plaintext secrets in config

`private_key_passphrase` is supported, but plaintext passphrases in YAML are risky. Prefer environment injection from your process manager or external secret stores.

### Use policy guardrails

Example: allow only the `prod` connection, deny destructive commands, and restrict file access to `/var/log`:

```yaml
policy:
  allowed_connections:
    - prod
  denied_commands:
    - "rm -rf"
    - "shutdown"
    - "reboot"
  allowed_paths:
    - /var/log
  readonly: true
```

## HTTP Endpoints

| Endpoint | Description |
|----------|-------------|
| `/` | MCP SSE handler |
| `/healthz` | JSON health check, returns `{ "ok": true }` |
| `/metrics` | JSON runtime counters for HTTP requests and MCP tool calls/errors |

Example metrics response:

```json
{
  "started_at": "2026-05-17T10:00:00Z",
  "uptime_seconds": 3600,
  "http_requests": {
    "/": 10,
    "/healthz": 2,
    "/metrics": 1
  },
  "tool_calls": {
    "exec_command": 3
  },
  "tool_errors": {
    "exec_command": 1
  }
}
```

## MCP Tools

| Tool | Description |
|------|-------------|
| `add_connection` | Add a new SSH connection. |
| `delete_connection` | Delete a connection by name. |
| `list_connections` | List all stored connections, sorted by name. |
| `test_connection` | Test whether an SSH connection can be established. |
| `exec_command` | Execute a command and return structured stdout/stderr/exit status. |
| `read_file` | Read a remote file via SFTP, with a configurable byte limit. |
| `write_file` | Write a remote file via SFTP. Existing files require `overwrite: true`. |
| `stat_file` | Return metadata for a remote file or directory. |
| `list_directory` | List entries in a remote directory. |

### `add_connection`

Input:

```json
{
  "name": "prod",
  "user": "ubuntu",
  "server": "prod.example.com",
  "port": 22
}
```

Notes:

- `name`, `user`, and `server` are required.
- `port` must be between `1` and `65535`.
- `name` must not contain whitespace or slash characters.

### `exec_command`

Input:

```json
{
  "connection": "prod",
  "cmd": "uptime",
  "timeout_seconds": 10
}
```

Success output:

```json
{
  "stdout": " 10:00:00 up 1 day...",
  "stderr": "",
  "exit_code": 0,
  "duration_ms": 123,
  "timed_out": false
}
```

Failure output is still structured and marked as an MCP tool error:

```json
{
  "error": "command exited with code 1",
  "result": {
    "stdout": "",
    "stderr": "command error details",
    "exit_code": 1,
    "duration_ms": 100,
    "timed_out": false
  }
}
```

### `read_file`

Input:

```json
{
  "connection": "prod",
  "file": "/var/log/syslog",
  "max_bytes": 4096
}
```

If `max_bytes` is omitted, `max_read_bytes` from config is used. The default is `1048576` bytes.

### `write_file`

Input:

```json
{
  "connection": "prod",
  "file": "/tmp/example.txt",
  "content": "hello\n",
  "overwrite": true
}
```

Output:

```json
{
  "file": "/tmp/example.txt",
  "bytes_written": 6,
  "overwritten": true
}
```

If the remote file already exists, `overwrite` must be explicitly set to `true`.

### `stat_file`

Input:

```json
{
  "connection": "prod",
  "file": "/var/log/syslog"
}
```

Output:

```json
{
  "name": "syslog",
  "path": "/var/log/syslog",
  "size": 123456,
  "mode": "-rw-r--r--",
  "mod_time": "2026-05-17T10:00:00Z",
  "is_dir": false
}
```

### `list_directory`

Input:

```json
{
  "connection": "prod",
  "path": "/var/log"
}
```

Output:

```json
{
  "path": "/var/log",
  "entries": [
    {
      "name": "syslog",
      "size": 123456,
      "mode": "-rw-r--r--",
      "mod_time": "2026-05-17T10:00:00Z",
      "is_dir": false
    }
  ]
}
```

## Audit Log

When enabled, audit events are written as JSON Lines:

```json
{"time":"2026-05-17T10:00:00Z","tool":"exec_command","connection":"prod","target":"uptime","success":true,"duration_ms":42}
```

Audited operations include:

- `test_connection`
- `exec_command`
- `read_file`
- `write_file`
- `stat_file`
- `list_directory`

## Development

Run checks:

```bash
go test ./...
go vet ./...
```

## License

MIT
