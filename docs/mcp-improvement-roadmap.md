# SSH MCP Improvement Roadmap

This document captures the current implementation review and a phased plan to improve safety, reliability, and MCP usability.

## Current State

The project currently implements a minimal SSH MCP server with:

- In-memory SSH connection management.
- SSH command execution.
- SFTP file read/write.
- SSH private key loading from local path, 1Password, or Consul KV.
- MCP SSE transport over HTTP.

The implementation is a good v1 MVP, but remote command execution and file writes are high-impact operations. Before broader use, the server should establish safer defaults and better operational controls.

## Key Issues

### Security

1. **Host key verification is disabled**
   - Current code uses `ssh.InsecureIgnoreHostKey()`.
   - This makes connections vulnerable to man-in-the-middle attacks.

2. **MCP HTTP/SSE endpoint has no authentication**
   - Anyone who can reach the listen address can call tools.
   - This allows remote command execution and file read/write through the MCP server.

3. **Default listen address is too broad**
   - Default `:8080` binds on all interfaces.
   - Safer default should be `127.0.0.1:8080`.

4. **No command/file operation policy**
   - No allowlist/denylist for commands, paths, or connections.
   - Destructive operations are not guarded.

5. **No audit log**
   - Remote commands and file writes are not recorded.

### Reliability

1. **No timeouts**
   - SSH dial, command execution, and SFTP operations can hang.

2. **Unbounded file reads**
   - `read_file` reads the entire remote file into memory.
   - Large files can exhaust memory or create oversized MCP responses.

3. **Unsafe writes by default**
   - `write_file` truncates/overwrites files without requiring explicit confirmation.

4. **No structured command result**
   - stdout and stderr are merged.
   - exit code is embedded in an error string.

### Usability

1. **Connection list order is unstable**
   - Connections are returned from a map without sorting.

2. **Input validation is minimal**
   - Empty names, invalid ports, and missing fields are not rejected clearly.

3. **Path expansion is missing**
   - `~/.ssh/id_rsa` in config examples does not work with `os.ReadFile` directly.

4. **Only one global SSH key is supported**
   - Different hosts cannot use different keys.

5. **Encrypted private keys are unsupported**
   - Passphrase-protected keys fail with `ssh.ParsePrivateKey`.

### Testability / Maintainability

1. **No test coverage**
   - Connection store, config loading, validation, and tool behavior should be covered.

2. **Concrete SSH client dependency**
   - Tool handlers depend directly on `*SSHClient`, making tests harder.

3. **Version is duplicated and hard-coded**
   - `1.0.0` appears in multiple files.

## Phased Implementation Plan

### Phase 1 — Safety Baseline

Goal: make the server safer by default without changing its core architecture.

- [x] Default listen address to `127.0.0.1:8080`.
- [x] Add optional Bearer token authentication for HTTP/SSE.
- [x] Add `known_hosts` support for SSH host key verification.
- [x] Keep insecure host key mode available only via explicit config/flag.
- [x] Add SSH dial timeout.
- [x] Add command timeout.
- [x] Add `read_file` size limits.
- [x] Add explicit `overwrite` flag for `write_file`.
- [x] Add input validation for connection fields.
- [x] Sort `list_connections` output.

### Phase 2 — Better MCP Tool Results

Goal: make tool results easier for agents and clients to consume.

- [x] Return structured JSON from `exec_command`.
- [x] Split stdout and stderr.
- [x] Include exit code, duration, and timed-out status.
- [x] Return structured JSON from file operations.
- [x] Add `test_connection`.
- [x] Add `stat_file`.
- [x] Add `list_directory`.

### Phase 3 — Configuration and Persistence

Goal: improve long-term usability across restarts and environments.

- [x] Expand `~` and environment variables in config paths.
- [x] Support connection definitions in config.
- [x] Add optional file-backed connection store.
- [x] Support connection-level SSH keys.
- [x] Support passphrase-protected private keys.
- [ ] Optionally import entries from `~/.ssh/config`.

### Phase 4 — Production Hardening

Goal: make the server safe and observable in production-like environments.

- [x] Add policy controls for commands and file paths.
- [x] Add audit logging for tool calls.
- [x] Add health endpoint.
- [x] Add metrics.
- [ ] Add CI.
- [x] Add baseline unit tests for config, stores, auth, policy, and metrics.
- [ ] Add mocked SSH/SFTP tool handler tests.

## Immediate Implementation Scope

The first implementation batch should focus on Phase 1:

1. Safer listen default.
2. Bearer token authentication.
3. Host key verification with `known_hosts`.
4. Timeouts.
5. File read/write safety limits.
6. Validation and deterministic connection listing.

