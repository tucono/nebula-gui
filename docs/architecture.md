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
| `nebula-guid` | Linux: the `nebula-guid` system user with `CAP_NET_ADMIN` and `CAP_NET_RAW`; macOS: root (launchd daemon); Windows: LocalSystem | Starts at boot and keeps running when the GUI is closed | Owns the Nebula instance. Single source of truth. |
| `nebula-gui` | Logged-in user | Opened by the user or at sign-in. Sits in the system tray when the window is closed. One copy per user. | Display and user input only. Holds no VPN state. |

The GUI can be closed and reopened while the tunnel stays up. Each time it connects, it reads the current state from the service. What the installers set up on each OS is in [platforms.md](platforms.md).

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

  Nebula (1.12+) logs through `log/slog`. Its logs and the service's own go into a ring buffer so the GUI can show them, and into a rotating log file in the state directory (`logs/nebula-guid.jsonl`) for troubleshooting reports. See [security.md](security.md#service-log-file).

- **Peers**: read from `ctrl.ListHostmapHosts(false)` (established tunnels) and `ctrl.ListHostmapHosts(true)` (pending handshakes), then mapped to the API's `Peer` type.
- **Pinger**: sends ICMP echo requests to a peer's overlay IP. The packet goes through the tun device, which makes Nebula do a lighthouse lookup and a handshake. The service keeps the last average RTT per peer in memory, cleared on disconnect. It uses `github.com/prometheus-community/pro-bing` in privileged mode, since the service already runs privileged (`cap_net_raw` in development). Pings are limited in count, timeout and rate, and only go to addresses inside this host's cert networks ([security.md](security.md)).

- **Settings** (`settings.go`): `settings.json` in the state directory, read and written through `GET`/`PUT /v1/settings`. Its one setting, `autoconnect`, applies to every user of the computer. See [Service start and boot](#service-start-and-boot).
- **OS service integration** (`cmd/nebula-guid`): on Linux and macOS, systemd and launchd run the foreground binary. On Windows, `nebula-guid.exe` has `install`, `uninstall` and `run` subcommands; `run` serves the Service Control Manager, and its Stop or Shutdown request cancels the same context a Ctrl-C does, so Nebula stops cleanly. A release build stops at once with a message if it lacks the privileges to make a tun device.
- **No updater:** the service never downloads or installs updates. From v1.0 the GUI checks GitHub for a newer release and says so ([updates.md](updates.md), ADR-009).

### GUI (`cmd/nebula-gui`, `internal/gui`)

- **Main window**, with four tabs (Status, Peers, Logs, Settings):
  - **Status**: a problem banner when something is wrong, otherwise the status header (state, overlay IP, cert name and expiry, time connected) with Connect/Disconnect, **Import config…** and **Remove config…**; the app, service and Nebula versions in small print.
  - **Logs**: the service log from `GET /v1/logs`, read every 2 s while the tab shows and every 10 s otherwise, with Follow and a warnings-only filter.
  - **Peers**: peers with a tunnel from `GET /v1/peers`, merged with the user's **saved peers**, under the user's name for each peer, else the cert name, else the IP. States are Connected (direct or relayed, with the last ping time), Connecting… and Not connected. A ⋯ menu per row has Ping, Copy IP, Rename…, Remove from list and Close connection. **Add peer…** saves a name and overlay IP for a peer with no tunnel yet, and can ping it straight away. Ping results show above the list, one line per peer (at most three, newest first). See [m3-plan.md](m3-plan.md).
  - **Settings** (M4): **Connect automatically when this computer starts** (the service's `autoconnect`, disabled and explained while the service can't be reached) and **Open nebula-gui when I sign in** (shown only with a tray). See [m4-plan.md](m4-plan.md#settings).
- **Saved peers** (`peerstore.go`): the user's names for peers, keyed by overlay IP, in `os.UserConfigDir()/nebula-gui/peers.json` (`0600`). They belong to the GUI user, not the service, and are never written to the GUI log or a troubleshooting report.
- **Problems** (`problems.go`, `problem_view.go`): every error is mapped to an entry in a catalogue, from the service's error code and reason or from the transport error (service not installed, not running, no permission). Each has a plain-language title and steps, a fix button where the app can fix it, collapsed technical details, and **Get help…**. See "Friendly errors" in [m2-plan.md](m2-plan.md).
- **GUI log** (`applog.go`): what the user did and what happened (service reachability, imports, Connect/Disconnect and their results, problems shown, and peer actions by IP only), kept in memory and in a rotating JSON Lines file under `os.UserConfigDir()/nebula-gui/logs/`. It covers the time the service couldn't be reached.
- **Get help** (`report.go`, `mask.go`, `help_dialog.go`): a Markdown troubleshooting report for an AI assistant or the network's admin. It combines the problem shown, `GET /v1/diagnostics` (status, config summary, the last 300 lines of the service's log file), the peers from `GET /v1/peers` and the GUI log. Saved peer names are never in it. Private network details are masked by default; see [security.md](security.md#troubleshooting-reports). Get help… is in every problem view, on the Logs tab and under Help in the menu.
- **Config import**: a file picker for `config.yml`, the desktop's own: the XDG Desktop Portal's on Linux (Fyne's without a portal), the Finder's open panel on macOS, and File Explorer's Open dialog on Windows. Missing key files are asked for one by one, and fixable problems (such as an enabled `sshd`) offer a fix in the import dialog. Relative `pki.*` paths are resolved and the files are read **in the GUI process**, as the user. Their contents are inlined into the YAML before it is sent to the service. See [security.md](security.md).
- **System tray** (`tray.go`, through Fyne's `desktop.App`): an icon for connected, not connected and needs attention, and a menu with a status line (some Linux trays show no tooltips), Connect or Disconnect, **Open nebula-gui** and **Quit**. `trayModel` turns the status into what the tray shows, as plain data with a table test. On Linux the GUI checks for a StatusNotifier host at start-up (`tray_linux.go`); Windows and macOS always have a tray.
- **Closing the window** hides it in the tray while there is one, with a notification the first time; without a tray, closing quits. **Quit** (tray or File → Quit) ends only the app: the connection stays up, and the first time while connected a dialog says so.
- **One copy at a time** (`instance*.go`): the GUI listens on a per-user socket (`$XDG_RUNTIME_DIR/nebula-gui/gui.sock` on Linux, `os.UserCacheDir()/nebula-gui/gui.sock` on macOS; directory `0700`, socket `0600`) or a pipe named after the user's SID with a DACL for that user only (Windows). A second copy sends `show`, the only message, and exits; the first shows its window.
- **Start at sign-in** (`internal/autostart`, no Fyne, so it builds and is tested for every OS from one machine): an XDG autostart file on Linux, a LaunchAgent on macOS, a `Run` registry value on Windows, each running `nebula-gui --hidden`, which starts in the tray. The GUI turns it on the first time it runs with a tray and says so once on the Settings tab; the checkbox reflects whether the entry exists, so removing it by hand (or turning it off in Task Manager) is respected.
- **Each user's files** (`internal/userfiles`): the one list of what the app keeps per user (its settings folder, Fyne's preferences, its cache folder, the start-at-sign-in entry). The GUI finds them through it for the user running it; uninstallers find them for every user from their home folders (M4.5).
- **About** (`about_dialog.go`, Help → About nebula-gui): the version, the project page, the note that this isn't an official Nebula project, and a button that opens the installed `THIRD_PARTY_NOTICES.txt`.
- **Poller**: calls `GET /v1/status` every 2 s, backing off to 10 s while the service can't be reached, and checks `GET /v1/version` again whenever the service's instance ID changes. Results reach the widgets on the UI goroutine through `fyne.Do`. While Nebula runs it also calls `GET /v1/peers`, every 2 s while the Peers tab shows and every 10 s otherwise (M3). While the window is hidden in the tray it reads only the status, every 10 s, for the tray; showing the window polls at once.

### Shared (`internal/api`)

Request and response types plus a typed client. Both binaries import this package, so the API contract is defined in one place.

## Key flows

### Import + connect
1. The user picks `config.yml`. The GUI parses it and inlines `pki.ca`, `pki.cert`, and `pki.key` if they are paths.
2. `PUT /v1/config` with the YAML. The service validates it with a dry-run parse and stores it.
3. `POST /v1/connect`. The service runs `nebula.Main` and `ctrl.Start()`, and the state becomes `running`.

### Add and ping a peer
1. The user clicks **Add peer…**, types a name and `192.168.100.5`, and clicks **Add and ping**. The GUI checks the IP against `overlay_ips` from the status and saves the peer in `peers.json`. The row shows Not connected.
2. The GUI reads `GET /v1/peers` to note the peer's tunnel, then sends `POST /v1/ping {"vpn_ip":"192.168.100.5"}` (3 echo requests, 5 s timeout by default). The service checks the address and the rate limits.
3. The service sends ICMP echo requests to that IP. Nebula sees traffic for an unknown host, asks the lighthouse, and handshakes.
4. The service returns the replies and RTT. The GUI reads `/v1/peers` again and judges the result with the tunnel state before and after (`describePing`): replies show an RTT; no replies but a new tunnel shows "Connecting…" and pings once more; no replies with an established tunnel says its firewall may not allow ping; anything else, including a tunnel still pending, shows "No answer…". The result goes in a line above the list, and the poller is poked so the row catches up.

### Service start and boot
1. The OS starts `nebula-guid` at boot (systemd after `network-online.target`, launchd, the Service Control Manager), before anyone signs in.
2. If `autoconnect` is on and a config is stored, the service connects as `POST /v1/connect` would and logs "connecting automatically". It's off by default; only the Settings tab turns it on.
3. If Nebula fails to start (the network may not be up yet at boot), the service tries again after 30 s, at most three times, and stops as soon as someone connects or disconnects, turns the setting off or deletes the config. A rejected config isn't retried.
4. When someone signs in, `nebula-gui --hidden` starts in the tray (if start at sign-in is on) and shows the state the service already has.

## Out-of-scope details to decide later
- Push updates (SSE or WebSocket) instead of polling. Polling is good enough for the MVP.
- Several saved profiles, with one active at a time.
- Using Nebula's SSH debug console instead of the Control API for diagnostics.
