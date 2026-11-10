<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Roadmap

## M0: Skeleton
- [ ] `go mod init`, Makefile, golangci-lint config, CI running vet, lint, and tests on all three OSes
- [ ] `internal/api` types for `version` and `status`
- [ ] `internal/ipc` listener and dialer (Unix socket plus Windows named pipe)
- [ ] Service answers `GET /v1/version`. The GUI shows it in a window.

## M1: Service runs Nebula
- [ ] `internal/nebulactl` wrapping `nebula.Main` and `Control`
- [ ] Config store plus `PUT/GET/DELETE /v1/config` with inline-PKI validation
- [ ] `connect`, `disconnect`, and `status` with the state machine
- [ ] Log ring buffer plus `GET /v1/logs`
- [ ] Test lab (`testlab/`, `make lab-up`). The service connects to it, tested with `curl --unix-socket`.

## M2: GUI basics
- [ ] Config import with PEM inlining
- [ ] Status header plus Connect/Disconnect
- [ ] Log view
- [ ] Clear errors when the service isn't installed or running, or the user lacks permission

## M3: Peers and ping
- [ ] `GET /v1/peers`, `POST /v1/peers/{ip}/close`
- [ ] `POST /v1/ping` (pro-bing, limits, subnet check)
- [ ] Peer table with auto-refresh, a ping dialog, and per-row ping and close actions

## M4: Desktop integration and packaging
- [ ] System tray menu, plus closing the window keeps the app running in the tray
- [ ] Service install and uninstall: systemd unit, launchd plist, Windows Service
- [ ] Installers: `.deb`/`.rpm` (nfpm), `.pkg` (macOS), MSI or Inno Setup (Windows, bundling wintun.dll)
- [ ] Autoconnect at boot setting

## M4.5: Updates (needs M4 installers + signing)
- [ ] Release pipeline produces `manifest.json` + `.minisig` and attaches them to GitHub releases
- [ ] Service: scheduled and manual checks, upstream slackhq/nebula notice
- [ ] Service: download, verify (signature, hash, no downgrade, platform signature), run the installer
- [ ] Resume the connection after an update (`resume-after-update` marker)
- [ ] Detect package-manager installs on Linux and only notify
- [ ] GUI: Updates settings, banner and tray notification, install now or when disconnected, skip version
- [ ] Tests: manifest signing and verification, downgrade refusal, a fake feed server in the test lab

## M5: Polish (v1.0)
- [ ] Warnings for `routes`/`unsafe_routes` before connecting
- [ ] Cert expiry warnings
- [ ] Code signing and notarization
- [ ] User docs and screenshots

## Later / maybe
- Several saved profiles
- Push updates (SSE) instead of polling
- Mobile (see [platforms.md](platforms.md#mobile)). First decide whether recommending Mobile Nebula is enough.
