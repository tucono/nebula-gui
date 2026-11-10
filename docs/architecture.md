# Architecture (service branch: separate Nebula binary)

## Overview

There are three processes:

```
┌──────────────────────┐   HTTP+JSON over     ┌───────────────────────────────────┐
│ nebula-gui  (user)   │   unix socket /      │ nebula-guid  (privileged)         │
│                      │   named pipe         │                                   │
│ Fyne window + tray   │ ───────────────────► │ API server                        │
│ - config import      │                      │ Config store ──writes──► run/config.yml
│ - start / stop       │ ◄─────────────────── │ Supervisor ──spawn/signal──┐      │
│ - peer table, ping   │                      │ Control client (SSH) ──┐   │      │
│ - updates            │                      │ Binary manager/updater │   │      │
└──────────────────────┘                      └────────────────────────┼───┼──────┘
                                                   ssh 127.0.0.1:<rand>│   │ child process
                                                                       ▼   ▼
                                                            ┌───────────────────────┐
                                                            │ nebula -config …      │
                                                            │  (upstream binary)    │
                                                            │  sshd debug console   │
                                                            └──────────┬────────────┘
                                                                       │ tun + UDP 4242
                                                                       ▼
                                                               Nebula overlay network
```

| Binary | Runs as | Lifetime | Responsibility |
|---|---|---|---|
| `nebula-guid` | root / LocalSystem / launchd daemon | Starts at boot and keeps running | Supervisor and the single source of truth. Never handles packets itself. |
| `nebula` | Child of `nebula-guid`, inheriting its privileges | Started and stopped by the service | The upstream VPN engine, unmodified |
| `nebula-gui` | Logged-in user | Opened by the user. Can sit in the system tray. | Display and user input only |

The GUI is the same as in the `monolithic` design. **Only the service's internals and the update flow are different.**

## Service components

### Config store
- Stores the user's imported YAML. Like the monolithic design, it accepts only inline PEM (ADR-005).
- On every start it **generates `run/config.yml`** (mode `0600`) by taking the user's config and **overriding these keys**:
  - `sshd`: enabled, `listen: 127.0.0.1:<random free port>`, a service-generated `host_key`, and one `authorized_users` entry with the service's own ed25519 public key. Any `sshd` block in the user's config is **replaced**.
  - `logging.format: json`, `logging.level: info` (or `debug` with `--dev`), so the supervisor can parse logs.
  - `pki.*` stays inline, so no other files are involved.
- The SSH host key and client key are created on first run and kept in the state directory (`0600`). The service pins the host key when it connects.

### Binary manager
- Keeps installed Nebula versions side by side:
  ```
  <state>/nebula/
    1.9.3/nebula[.exe]   (+ wintun.dll on Windows)
    1.10.0/nebula[.exe]
    active.json          {"version":"1.10.0","previous":"1.9.3","source":"managed"}
  ```
- Where the binary comes from:
  - `bundled`: the copy shipped with the installer, so the app works offline the first time.
  - `managed`: downloaded by the updater.
  - `custom`: an admin-set path in the service's config file, for example a distro's `/usr/bin/nebula`. Auto-updates are turned off. **This can't be set through the IPC API** (see [security.md](security.md)).
- At startup it runs `nebula -version` to find the version, then checks it against the **supported range** compiled into the service (for example `>=1.8.0 <1.11.0`). Versions outside the range produce a warning, or are refused when below the minimum.

### Supervisor
State machine:

```
Stopped ──start──► Spawning ──ssh ready──► Running ──stop──► Stopping ──exit──► Stopped
                      │                      │ unexpected exit
                      │ timeout / exit       ▼
                      └────────────────► Crashed ──backoff (1s,2s,4s… max 60s)──► Spawning
                                            └── 5 crashes in 5 min ──► Error (needs user)
```

- **Spawn:** `nebula -config <state>/run/config.yml`. Stdout and stderr are captured and each line is parsed as JSON into the log ring buffer.
- **Ready:** the SSH console accepts a connection and `version` responds within 15 s.
- **Stop:** on Unix send `SIGTERM`. On Windows send `CTRL_BREAK_EVENT` to the child's process group, which needs the process to be started with `CREATE_NEW_PROCESS_GROUP`. After 10 s, kill it.
- **Config reload:** a config change while running uses the console's `reload` command (or `SIGHUP` on Unix). If the change can't be reloaded (tun settings, listen port), restart instead.
- **Orphan cleanup:** the child's PID is stored in `<state>/run/nebula.pid`. At startup, the service kills any leftover child from a previous crash. On Linux it also sets `Pdeathsig` so the child exits when the service dies.

### Control client (SSH)
- Uses `golang.org/x/crypto/ssh` to keep one session to the console and run commands:

  | Purpose | Console command |
  |---|---|
  | Established peers | `list-hostmap -json` |
  | Pending handshakes | `list-pending-hostmap -json` |
  | Open a tunnel without ICMP | `create-tunnel <vpn-ip>` |
  | Close a tunnel | `close-tunnel <vpn-ip>` |
  | Details for one peer | `print-tunnel -json <vpn-ip>` |
  | Own cert | `print-cert -json` |
  | Version / health | `version` |

  Check this command list against each supported Nebula version. It's tested in the version matrix ([testing.md](testing.md)).
- **Parsers for each version range.** For example, `vpnIp` (≤1.9) became `vpnAddrs` (1.10+). They live in `internal/nebulactl/parse_*.go` and map to the API's `Peer` type.

### Pinger
- ICMP echo through the overlay, as in the monolithic design.
- The `create-tunnel` command can also open a tunnel without sending ICMP. The GUI's "Connect to peer" action uses it, and "Ping" measures RTT.

### Updater
- There are **two independent update streams**: the Nebula binary (frequent, with rollback) and the app itself (GUI + service). See [updates.md](updates.md).

## GUI (`cmd/nebula-gui`, `internal/gui`)
Same as the monolithic design, with these additions:
- The status header shows the Nebula version and where it came from (bundled, managed, or custom).
- **Settings → Updates** has separate sections for Nebula and the app, plus a "Roll back Nebula" button.
- A crash or restart indicator, for example "Nebula restarted 2× in the last hour", with a link to the logs.

## Key flows

### Connect
1. Generate `run/config.yml`.
2. Spawn `active` Nebula.
3. Wait for the SSH console.
4. The state becomes `running`.

### Peer list
The GUI polls `GET /v1/peers`. The service runs `list-hostmap -json` and `list-pending-hostmap -json` and caches the result for 1 s so several GUIs don't cause extra SSH calls.

### Nebula update
1. Download and verify the new version into `nebula/<ver>/`.
2. Stop the current process, update `active.json`, and spawn the new version.
3. Health check (see [updates.md](updates.md#switch-health-check-rollback)). On failure, switch back to `previous` automatically.

Downtime is only the time it takes to restart, about 1–3 s, and nothing is reinstalled.
