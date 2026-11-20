<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Roadmap

## M0: Skeleton
- [x] `go mod init`, Makefile, golangci-lint config, CI running vet, lint, and tests on all three OSes
- [x] `internal/api` types for `version` and `status`
- [x] `internal/ipc` listener and dialer (Unix socket plus Windows named pipe)
- [x] Service answers `GET /v1/version`. The GUI shows it in a window.

## M1: Service runs Nebula
- [x] `internal/nebulactl` wrapping `nebula.Main` and `Control`
- [x] Config store plus `PUT/GET/DELETE /v1/config` with inline-PKI validation
- [x] `connect`, `disconnect`, and `status` with the state machine
- [x] Log ring buffer plus `GET /v1/logs`
- [x] Test lab (`testlab/`, `make lab-up`). The service connects to it, tested with `curl --unix-socket`.

## M2: GUI basics
Detailed plan: [m2-plan.md](m2-plan.md).

- [x] Config import with PEM inlining
- [x] Status header plus Connect/Disconnect
- [x] Log view
- [x] Clear errors when the service isn't installed or running, or the user lacks permission
- [x] Plain-language errors with direct fixes, based on error reasons from the service
- [x] Persistent service log plus `GET /v1/diagnostics`
- [x] "Get help" troubleshooting report for AI assistants, with private network details masked

## M3: Peers and ping
Detailed plan: [m3-plan.md](m3-plan.md).

- [x] `GET /v1/peers`, `POST /v1/peers/{ip}/close`
- [x] `POST /v1/ping` (pro-bing, limits, subnet check, last RTT per peer)
- [x] Peers tab with auto-refresh and a ⋯ menu per row: ping, copy IP, rename, close connection
- [x] Saved peers: the user's own names for peers, and **Add peer…** for peers without a tunnel yet
- [x] Peers in the troubleshooting report (masked; saved names never included)

## M4: Desktop integration and packaging
Detailed plan: [m4-plan.md](m4-plan.md).

- [x] System tray menu, plus closing the window keeps the app running in the tray
- [x] One copy of the app at a time; opening it again shows the running window
- [x] Service install and uninstall: systemd unit (its own user, ADR-007), launchd plist, Windows Service
- [x] Installers: `.deb`/`.rpm` (nfpm), `.pkg` (macOS), Inno Setup (Windows, bundling wintun.dll; ADR-008)
- [x] Settings tab: autoconnect at boot, and start at sign-in
- [x] Release workflow: a draft GitHub release with the packages and `SHA256SUMS` from a version tag
- [x] Third-party license notices in every package, and an About dialog

## M4.5: Full uninstall
Detailed plan: [m4.5-plan.md](m4.5-plan.md).

- [ ] An option to remove everything: the service's state (imported config, logs, settings) and each user's own files (saved peer names, masking names, the app log, app preferences, the start-at-sign-in entry). Today every uninstaller leaves users' files behind, and on Linux even a purge keeps `/var/lib/nebula-gui`.
- [ ] macOS: a cleaner uninstall than a Terminal script, if the platform allows one
- [ ] Tests: what each uninstall removes and keeps, on all three platforms

## M5: Polish (v1.0)
- [ ] Update check: the app checks GitHub for a newer release and says so, with a link to the release page. Installing stays manual (ADR-009, [updates.md](updates.md)).
- [ ] Warnings for `routes`/`unsafe_routes` before connecting
- [ ] Cert expiry warnings
- [ ] Import a `.zip` with the config and its key files
- [ ] User docs and screenshots

## Not planned
- Code signing and notarization. Installers stay unsigned, with the warnings the README walks through (ADR-009).
- Installing updates from the app (ADR-009)
- Testing on desktops other than COSMIC (Pop!_OS)
- 32-bit builds, and processors other than 64-bit Intel/AMD on Windows and Linux (macOS stays universal)

## Later / maybe
- Several saved profiles
- Push updates (SSE) instead of polling
- Mobile (see [platforms.md](platforms.md#mobile)). First decide whether recommending Mobile Nebula is enough.
