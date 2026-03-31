# ssh-mcp

An MCP (Model Context Protocol) server that provides SSH remote operations as tools, including command execution and file read/write via SFTP.

## Features

- Execute commands on remote servers via SSH
- Read and write files on remote servers via SFTP
- Manage multiple SSH connections
- Support SSH key from file path, config file, or 1Password

## Installation

```bash
go install github.com/orvice/ssh-mcp@latest
```

## Usage

### With SSH key path (no config file needed)

```bash
ssh-mcp --ssh-key ~/.ssh/id_ed25519
```

### With 1Password

Set the `OP_SERVICE_ACCOUNT_TOKEN` environment variable and pass a secret reference:

```bash
export OP_SERVICE_ACCOUNT_TOKEN="your-token"
ssh-mcp --op-ssh-key "op://vault/item/private_key"
```

### With config file

```bash
ssh-mcp --config config.yaml
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--ssh-key` | Path to SSH private key file | |
| `--op-ssh-key` | 1Password secret reference for SSH key (e.g. `op://vault/item/private_key`) | |
| `--config` | Path to YAML config file | |
| `--listen` | Listen address | `:8080` |

Priority: `--op-ssh-key` > `--ssh-key` / config file `private_key`.

### Config file format

```yaml
private_key: ~/.ssh/id_rsa
listen: ":8080"
```

## MCP Tools

| Tool | Description |
|------|-------------|
| `add_connection` | Add a new SSH connection (name, user, server, port) |
| `delete_connection` | Delete a connection by name |
| `list_connections` | List all stored connections |
| `exec_command` | Execute a command on a remote server |
| `read_file` | Read a file from a remote server via SFTP |
| `write_file` | Write a file to a remote server via SFTP |

## License

MIT
