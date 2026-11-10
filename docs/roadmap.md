# Roadmap

## M0: Skeleton
- [ ] `go mod init`, Makefile, golangci-lint config, CI running vet, lint, and tests on all three OSes
- [ ] `internal/api` types for `version` and `status`
- [ ] `internal/ipc` listener and dialer (Unix socket plus Windows named pipe)
- [ ] Service answers `GET /v1/version`. The GUI shows it in a window.

## M1: Service runs Nebula
- [ ] `internal/binstore`: bundled/custom binary detection, `nebula -version`, supported range
- [ ] Runtime config generation (inject `sshd` and JSON logging, keep `pki` inline)
- [ ] `internal/nebulaproc`: spawn, ready check, stop (SIGTERM / CTRL_BREAK), crash backoff, orphan cleanup, Job Object / Pdeathsig
- [ ] `internal/nebulactl`: SSH console client + parsers + golden files for each supported version
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
- [ ] `GET /v1/peers`, `POST /v1/peers/{ip}/tunnel`, `POST /v1/peers/{ip}/close`
- [ ] `POST /v1/ping` (pro-bing, limits, subnet check)
- [ ] Peer table with auto-refresh, a ping dialog, and per-row connect, ping and close actions
- [ ] CI Nebula version matrix

## M4: Desktop integration and packaging
- [ ] System tray menu, plus closing the window keeps the app running in the tray
- [ ] Service install and uninstall: systemd unit, launchd plist, Windows Service
- [ ] Installers: `.deb`/`.rpm` (nfpm), `.pkg` (macOS), MSI or Inno Setup (Windows, bundling wintun.dll)
- [ ] Autoconnect at boot setting

## M4.5: Nebula binary updates (can start before M4, since no installers are needed)
- [ ] Curated channel manifest + minisign signing + CI job that watches upstream releases and runs the version matrix
- [ ] Service: check, download, verify (signature, sequence, hashes, platform signature, `-version`, `-test`)
- [ ] Install side by side, switch, health check, automatic rollback, `bad`/`revoked` handling
- [ ] `upstream` and `pinned` channels, auto-install setting (`security-only` by default)
- [ ] GUI: Nebula update section, rollback button, rollback notification
- [ ] Windows firewall rule follows version switches

## M4.6: App updates (needs M4 installers + signing)
- [ ] Same as the `monolithic` branch's M4.5. The installer updates only the bundled Nebula, never managed versions.
- [ ] After an app update, check that the active Nebula is still in the supported range

## M5: Polish (v1.0)
- [ ] Warnings for `routes`/`unsafe_routes` before connecting
- [ ] Cert expiry warnings
- [ ] Code signing and notarization
- [ ] User docs and screenshots

## Later / maybe
- Several saved profiles
- Push updates (SSE) instead of polling
- Mobile (see [platforms.md](platforms.md#mobile)). First decide whether recommending Mobile Nebula is enough.
