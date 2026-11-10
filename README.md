# nebula-gui

A cross-platform desktop app for managing [Nebula](https://github.com/slackhq/nebula) overlay-network connections.

A user loads a Nebula config. The app starts and stops the tunnel, shows which peers are connected, and can ping a peer to open a tunnel to it.

> **Status:** pre-alpha, design phase. No code yet. See [docs/roadmap.md](docs/roadmap.md).

## Goals

- Import a Nebula config, including inline or referenced certs and keys.
- Start and stop the Nebula connection with one click.
- Show active and pending peer tunnels: name, overlay IP, real endpoint, and whether the tunnel is direct or relayed.
- Ping a peer's overlay IP to open a tunnel and measure round-trip time.
- Check for updates and install them, including new Nebula versions. Updates are signed and verified ([docs/updates.md](docs/updates.md)).
- Run on **Windows, Linux, and macOS**. Android and iOS are stretch goals (see [docs/platforms.md](docs/platforms.md#mobile)).

## Non-goals (for now)

- Acting as a certificate authority or managing a fleet. Use `nebula-cert` or Defined Networking for that.
- Running more than one Nebula network at the same time. This may be added after v1.

## Tech stack

| Concern | Choice |
|---|---|
| Language | Go |
| GUI | [Fyne v2](https://fyne.io) |
| VPN engine | Nebula, **embedded as a Go library** (`github.com/slackhq/nebula`) |
| Privilege model | Separate privileged service (`nebula-guid`) plus an unprivileged GUI (`nebula-gui`) |
| GUI ↔ service IPC | HTTP + JSON over a Unix socket (Linux/macOS) or named pipe (Windows) |

The reasons for each choice are in [docs/decisions.md](docs/decisions.md).

## Documentation

| Doc | Contents |
|---|---|
| [docs/architecture.md](docs/architecture.md) | Components, process model, data flow |
| [docs/ipc-api.md](docs/ipc-api.md) | The GUI ↔ service API contract |
| [docs/development.md](docs/development.md) | Toolchain setup, repo layout, building and running |
| [docs/testing.md](docs/testing.md) | Unit tests and a local multi-node Nebula test lab |
| [docs/platforms.md](docs/platforms.md) | Per-OS notes (Wintun, systemd, launchd, mobile) |
| [docs/updates.md](docs/updates.md) | Update checks, the signed manifest, installing on each OS |
| [docs/security.md](docs/security.md) | Threat model and service hardening rules |
| [docs/decisions.md](docs/decisions.md) | Architecture decision records |
| [docs/roadmap.md](docs/roadmap.md) | Milestones |
