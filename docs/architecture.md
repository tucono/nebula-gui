<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Architecture

## Overview

Nebula has to create a tun device and change routes, which needs root, Administrator, or `CAP_NET_ADMIN`. GUI toolkits should not run with those privileges. The app is therefore split into **two processes**:

```
┌──────────────────────────┐        local IPC         ┌──────────────────────────────┐
│  nebula-gui  (user)      │  HTTP+JSON over          │  nebula-guid  (privileged)   │
│                          │  unix socket / named pipe│                              │
│  Fyne window + tray      │ ───────────────────────► │  API server                  │
│  - config import         │                          │  Config store                │
│  - start / stop          │ ◄─────────────────────── │  Nebula manager ──► nebula.Main()
│  - peer table            │     status / peers       │    └─ *nebula.Control        │
│  - ping                  │                          │  Pinger (ICMP via overlay)   │
└──────────────────────────┘                          └──────────────┬───────────────┘
                                                                     │ tun device + UDP 4242
                                                                     ▼
                                                             Nebula overlay network
```

| Binary | Runs as | Lifetime | Responsibility |
|---|---|---|---|
| `nebula-guid` | root / LocalSystem / launchd daemon | Starts at boot and keeps running when the GUI is closed | Owns the Nebula instance. Single source of truth. |
| `nebula-gui` | Logged-in user | Opened by the user. Can sit in the system tray. | Display and user input only. Holds no VPN state. |

The GUI can be closed and reopened while the tunnel stays up. Each time it connects, it reads the current state from the service.

## Components

### Service (`cmd/nebula-guid`, `internal/service`)

- **API server**: serves the endpoints in [ipc-api.md](ipc-api.md) on the platform's IPC transport (`internal/ipc`).
- **Config store**: keeps the imported config, with certs and keys inlined, in a service-owned directory with `0600` permissions:
  - Linux: `/var/lib/nebula-gui/`
  - macOS: `/Library/Application Support/nebula-gui/`
  - Windows: `%ProgramData%\nebula-gui\`
- **Nebula manager**: a state machine that wraps the embedded Nebula instance:

  ```
  Stopped ──start──► Starting ──ok──► Running ──stop──► Stopping ──► Stopped
                         │                                  ▲
                         └──error──► Error ──start/clear────┘
  ```

  Starting Nebula follows this pattern. Check the exact signatures against the pinned version in `go.mod`:

  ```go
  c := config.NewC(logger)
  if err := c.LoadString(yamlWithInlinedPKI); err != nil { ... }
  ctrl, err := nebula.Main(c, false, buildVersion, logger, nil) // nil = default tun device
  if err != nil { ... }
  ctrl.Start()
  // later: ctrl.Stop()
  ```

  Logs from the Nebula `logrus` logger go into a ring buffer so the GUI can show them.

- **Peers**: read from `ctrl.ListHostmapHosts(false)` (established tunnels) and `ctrl.ListHostmapHosts(true)` (pending handshakes), then mapped to the API's `Peer` type.
- **Pinger**: sends ICMP echo requests to a peer's overlay IP. The packet goes through the tun device, which makes Nebula do a lighthouse lookup and a handshake. The service records the RTT. It uses `github.com/prometheus-community/pro-bing` in privileged mode, since the service already runs privileged.

- **Updater** (`internal/updater`): checks the signed release manifest, downloads and verifies the installer, and runs the platform installer. Details in [updates.md](updates.md).

### GUI (`cmd/nebula-gui`, `internal/gui`)

- **Main window**:
  - status header showing state, overlay IP, cert name, and cert expiry;
  - Connect/Disconnect button;
  - peer table;
  - "Ping…" dialog that takes an overlay IP, or a context-menu ping on a peer row;
  - log view.
- **Config import**: a file picker for `config.yml`. Relative `pki.*` paths are resolved and the files are read **in the GUI process**, as the user. Their contents are inlined into the YAML before it is sent to the service. See [security.md](security.md).
- **System tray** (`fyne/driver/desktop`): Connect/Disconnect, status, Open, and Quit.
- **Poller**: calls `GET /v1/status` and `GET /v1/peers` every 2 s while the window is visible, and less often when hidden. It updates Fyne data bindings.

### Shared (`internal/api`)

Request and response types plus a typed client. Both binaries import this package, so the API contract is defined in one place.

## Key flows

### Import + connect
1. The user picks `config.yml`. The GUI parses it and inlines `pki.ca`, `pki.cert`, and `pki.key` if they are paths.
2. `PUT /v1/config` with the YAML. The service validates it with a dry-run parse and stores it.
3. `POST /v1/connect`. The service runs `nebula.Main` and `ctrl.Start()`, and the state becomes `running`.

### Ping a peer
1. `POST /v1/ping {"vpn_ip":"192.168.100.5","count":3}`.
2. The service sends ICMP echo requests to that IP. Nebula sees traffic for an unknown host, asks the lighthouse, and handshakes.
3. The service returns the RTT and loss. The next `/v1/peers` poll shows the new tunnel.

### Service restart / boot
If `autoconnect: true` is set in service settings, the service starts Nebula with the stored config at boot, even before any user logs in.

## Out-of-scope details to decide later
- Push updates (SSE or WebSocket) instead of polling. Polling is good enough for the MVP.
- Several saved profiles, with one active at a time.
- Using Nebula's SSH debug console instead of the Control API for diagnostics.
