# lrc

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev)

Lightweight reverse-tunnel client for Locrest. Runs as a foreground process, a background supervisor, or via `curl | bash` one-liners.

## Install

Run the installer. It detects your OS and architecture, downloads the matching
binary from GitHub Releases, and installs it to `~/.local/bin` (or
`/usr/local/bin` when run as root).

```bash
curl -fsSL https://raw.githubusercontent.com/quonaro/locrest-client/main/install.sh | bash
```

## Quick start

### Foreground mode (legacy)

```bash
lrc -server wss://locrest.example.com/tunnel -port 3000 -subdomain myapp -key <hex>
```

The tunnel runs in the terminal. Press `Ctrl+C` to stop.

### Background supervisor mode

Start the supervisor (keeps running after terminal closes):

```bash
nohup lrc -supervisor >~/.cache/locrest/supervisor.log 2>&1 &
```

Add a tunnel:

```bash
lrc add -server wss://locrest.example.com/tunnel -port 3000 -subdomain myapp -key <hex>
```

Manage tunnels:

```bash
lrc list                # show all tunnels
lrc status <id>         # tunnel details
lrc logs <id>           # recent log lines
lrc kill <id>           # stop a tunnel
```

Tunnels auto-reconnect with exponential backoff. Each reconnect performs a fresh auth handshake, so token invalidation by the server is handled automatically.

### One-liner with daemon mode

The server can generate a script that automatically starts the supervisor and registers the tunnel:

```bash
curl -fsSL "https://locrest.example.com/3000?daemon=true" | bash
```

This downloads the client binary, starts the supervisor in the background, adds the tunnel, and prints management commands.

## CLI reference

### Global flags

| Flag | Description |
|------|-------------|
| `-server` | Chisel server URL (`wss://host/tunnel`) |
| `-port` | Local port to forward |
| `-subdomain` | Requested subdomain |
| `-host` | Target host (default: `localhost`) |
| `-key` | Hex-encoded ed25519 private key |
| `-keyfile` | Path to key file (read once, then deleted) |
| `-setup-token` | Server-issued setup token |
| `-insecure-url` | Optional insecure server URL |
| `-fingerprint` | Expected SSH host-key fingerprint |
| `-insecure` | Skip TLS certificate verification |
| `-debug` | Enable verbose debug output |
| `-token-ttl` | Token lifetime (informational) |
| `-supervisor` | Run as background supervisor |

### Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `add` | same as flags | Start a new background tunnel |
| `list` | none | List all active tunnels |
| `kill` | `<id>` | Stop a tunnel by ID |
| `status` | `<id>` | Show tunnel details |
| `logs` | `<id>` | Show recent log lines |

### Tunnel ID

The tunnel ID is a 16-character hex prefix of `SHA256(server + subdomain + port)`. It is shown after `lrc add` and in `lrc list`.

## Files

| Path | Description |
|------|-------------|
| `~/.cache/locrest/lrsv.sock` | Unix socket for supervisor communication |
| `~/.cache/locrest/supervisor.log` | Supervisor and tunnel logs |
| `~/.cache/locrest/` | Downloaded client binaries |

## Environment variables

| Variable | Used by |
|----------|---------|
| `LOCREST_SUBDOMAIN` | `-subdomain` fallback |
| `LOCREST_SETUP_TOKEN` | `-setup-token` fallback |
| `LOCREST_KEY` | `-key` fallback |
| `XDG_CACHE_HOME` | Cache directory override |

## Architecture

The client uses a single binary in three modes:

- **Foreground**: direct `auth.Run()` + `tunnel.Start()`, same as before.
- **Supervisor** (`-supervisor`): Unix socket HTTP server that holds an in-memory registry of tunnels. Each tunnel runs in its own goroutine with a reconnect loop.
- **CLI command**: talks to the supervisor over the Unix socket.

The reconnect loop performs a full `auth.Run()` on every disconnect, so token expiration or server-side invalidation is handled transparently.

## License

See [LICENSE](./LICENSE) for the full MIT license text, including [third-party software notices](./LICENSE#third-party-software-notices).
