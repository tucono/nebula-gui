# nebula-gui

A cross-platform desktop app for managing [Nebula](https://github.com/slackhq/nebula) overlay-network connections.

A user loads a Nebula config. The app starts and stops the tunnel, shows which peers are connected, and can ping a peer to open a tunnel to it.

> **Status:** pre-alpha, design phase. No code yet. See [docs/roadmap.md](docs/roadmap.md).
>
> **Branch `service`:** this design runs the **upstream `nebula` binary as a separate, supervised process** that can be updated on its own. The other design, with Nebula built in as a Go library, is on branch [`monolithic`](../../tree/monolithic). See [Design comparison](#design-comparison) below.

## Goals

- Import a Nebula config, including inline or referenced certs and keys.
- Start and stop the Nebula connection with one click.
- Show active and pending peer tunnels: name, overlay IP, real endpoint, and whether the tunnel is direct or relayed.
- Ping a peer's overlay IP, or ask Nebula directly to open a tunnel, and measure round-trip time.
- **Check for and install Nebula updates separately from the app**, with automatic rollback ([docs/updates.md](docs/updates.md)).
- Run on **Windows, Linux, and macOS**. Android and iOS are stretch goals (see [docs/platforms.md](docs/platforms.md#mobile)).

## Non-goals (for now)

- Acting as a certificate authority or managing a fleet. Use `nebula-cert` or Defined Networking for that.
- Running more than one Nebula network at the same time.

## Tech stack

| Concern | Choice |
|---|---|
| Language | Go |
| GUI | [Fyne v2](https://fyne.io) |
| VPN engine | **Upstream `nebula` binary**, run as a child process of the service |
| Engine control | Nebula's **SSH debug console**, bound to localhost with a service-generated key (`list-hostmap -json` and similar) |
| Privilege model | A privileged service (`nebula-guid`) that supervises `nebula`, plus an unprivileged GUI (`nebula-gui`) |
| GUI ↔ service IPC | HTTP + JSON over a Unix socket (Linux/macOS) or named pipe (Windows) |

## Design comparison

| | `monolithic` (Nebula built in) | `service` (separate binary, this branch) |
|---|---|---|
| Updating Nebula | A new app release is needed for every Nebula bump | Download the upstream release on its own; no app release needed |
| Getting a Nebula security fix to users | Waits until we publish a release | As soon as it's approved in the curated channel (or right away on the `upstream` channel) |
| Peer and status data | Typed Go structs from `nebula.Control` | JSON parsed from the SSH console; field names change between Nebula versions |
| Moving parts | One process | Service, child process, and SSH control channel |
| Getting both pieces to agree | Built together, so it can't go wrong | Needs a tested version range and a parser per version |
| Use the distro's or user's own `nebula` | No | Yes (set by an admin) |
| Rollback | Reinstall the old app | Switch back to the previous Nebula binary automatically |
| Attack surface | Smaller | Adds a localhost SSH listener and downloaded executables |
| Mobile | Closer to how mobile apps must work | Not possible on mobile (you can't run separate binaries there). Mobile would have to use the library approach. |

## Documentation

| Doc | Contents |
|---|---|
| [docs/architecture.md](docs/architecture.md) | Components, process model, process supervision, data flow |
| [docs/ipc-api.md](docs/ipc-api.md) | The GUI ↔ service API contract |
| [docs/updates.md](docs/updates.md) | Nebula binary updates (channels, verification, rollback) and app updates |
| [docs/development.md](docs/development.md) | Toolchain setup, repo layout, building and running |
| [docs/testing.md](docs/testing.md) | Unit tests, Nebula version matrix, local test lab |
| [docs/platforms.md](docs/platforms.md) | Per-OS notes (process signals, Wintun, systemd, launchd, mobile) |
| [docs/security.md](docs/security.md) | Threat model and service hardening rules |
| [docs/decisions.md](docs/decisions.md) | Architecture decision records |
| [docs/roadmap.md](docs/roadmap.md) | Milestones |
