<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Architecture Decision Records

Short records of significant decisions. Add new ones at the bottom. Don't edit accepted ones. Replace them with a new ADR instead.

---

## ADR-001: Go + Fyne for the GUI
**Status:** Accepted, 2026-11-10

**Context:** Nebula is written in Go. We need Windows, Linux, and macOS, and Android/iOS would be nice.

**Options considered:**
- Fyne (pure Go)
- Wails (Go + web UI; mobile unproven)
- Flutter (Dart + gomobile; strongest mobile support, two languages)
- Tauri/Electron (would run the `nebula` binary as a subprocess)

**Decision:** Fyne. It's one language and one toolchain, it can import Nebula directly, and it reaches every target platform. The UI needs are modest: forms, a table, a tray.

**Consequences:** Fyne widgets look plain, and Fyne needs CGO, so cross-compiling is harder. We can move to Wails later if UI polish becomes a priority. The service and API would stay the same.

---

## ADR-002: Embed Nebula as a library, not a subprocess
**Status:** Superseded by ADR-007 on the `service` branch (still accepted on `monolithic`)

**Decision:** Call `nebula.Main` and use `*nebula.Control` from inside the service.

**Why:** Structured peer data (`ListHostmapHosts`), `CloseTunnel`, and clean start and stop, without parsing logs or using the SSH console.

**Consequences:** Nebula's Go API isn't guaranteed stable. All use of it goes through `internal/nebulactl`. Upgrade on purpose and pin the version.

---

## ADR-003: Separate privileged service and unprivileged GUI
**Status:** Accepted, 2026-11-10

**Decision:** There are two binaries: `nebula-guid` (privileged, long-running) and `nebula-gui` (user).

**Why:** Running a GUI toolkit as root is a security and usability problem. The tunnel should also stay up when the window is closed, and should be able to start at boot.

**Consequences:** Every OS needs its own installer and service integration. The IPC API becomes a security boundary ([security.md](security.md)).

---

## ADR-004: HTTP+JSON over Unix socket / named pipe for IPC
**Status:** Accepted, 2026-11-10

**Options considered:**
- gRPC
- JSON-RPC
- HTTP+JSON

**Decision:** HTTP+JSON using `net/http`, served on a Unix listener (Linux/macOS) or a go-winio named pipe listener (Windows).

**Why:** It uses only the standard library and is easy to debug (`curl --unix-socket`). There's no codegen step, and it is fast enough for polling every 2 s.

**Consequences:** No push updates in v1. Server-Sent Events can be added later if polling isn't good enough.

---

## ADR-005: Service only accepts inline PKI material
**Status:** Accepted, 2026-11-10

**Decision:** `PUT /v1/config` rejects configs where `pki.ca`, `pki.cert`, or `pki.key` are file paths. The GUI inlines them.

**Why:** It stops the privileged service from being used to read arbitrary files.

---

## ADR-006: Nebula updates ship as signed app updates
**Status:** Superseded by ADR-009 on the `service` branch (still accepted on `monolithic`)

**Context:** Users want to check for and install Nebula client updates from the app. Nebula is built into `nebula-guid` (ADR-002).

**Options considered:**
1. **Update the app as a whole.** Each Nebula bump is a new nebula-gui release, installed through the platform's own installer (MSI, pkg, deb/rpm), with a signed manifest.
2. **Run Nebula as a subprocess** and download upstream `nebula` binaries from slackhq/nebula separately.
3. **Notify only** and leave updating to the user.

**Decision:** Option 1, plus an informational notice when upstream Nebula has a newer release than the one we embed.

**Why:**
- Option 2 would give up ADR-002 (the structured Control API) and require parsing logs or using the SSH console.
- Option 2 would also have the service download and run executables we never tested with our code.
- Option 1 updates the GUI and service together, so their API versions always match.
- Each platform's installer already handles stopping and restarting services, locked files, and rollback.

**Consequences:**
- We have to release promptly when Nebula publishes a security fix. The upstream notice makes any delay visible to users.
- We need signing infrastructure: a minisign key plus Authenticode and Developer ID certificates.
- Linux installs from a package repo don't self-update.
- See [updates.md](updates.md).

---

## ADR-007: Run the upstream `nebula` binary as a supervised child process
**Status:** Proposed (`service` branch), 2026-11-10. Replaces ADR-002 on this branch.

**Context:** We want to deliver Nebula updates, especially security fixes, without making a new app release for each one. We also want admins to be able to use a `nebula` they already trust, such as a distro package.

**Decision:** `nebula-guid` spawns and supervises an unmodified upstream `nebula` binary. The service generates the runtime config file, captures JSON logs, restarts Nebula on crashes with backoff, and stops it with SIGTERM on Unix or CTRL_BREAK on Windows.

**Consequences:**
- Nebula can be updated and rolled back separately from the app.
- We only run upstream's own release builds, never a fork or rebuild.
- The service needs process supervision, orphan cleanup, and stop signals for each platform.
- There's no typed Go API, so control goes through ADR-008.
- Mobile isn't possible with this design, since mobile OSes don't let apps run separate binaries.

---

## ADR-008: Control Nebula through its SSH debug console
**Status:** Proposed (`service` branch), 2026-11-10

**Options considered:**
- **SSH console:** `list-hostmap -json`, `create-tunnel`, `close-tunnel`, and so on
- **Parsing log lines:** unreliable, and gives no on-demand queries
- **Prometheus `stats` endpoint:** only overall counters, no hostmap

**Decision:** The service always injects an `sshd` block into the runtime config. It listens on `127.0.0.1:<random port>`, uses a service-generated host key that the client pins, and allows only the service's own ed25519 client key. The user's `sshd` settings are always replaced.

**Consequences:**
- The output format can change between Nebula versions. We need parsers for each version range, a supported-range check, and a test matrix over versions.
- Any local process can reach the localhost TCP listener, so security depends entirely on SSH key authentication (see security.md).

---

## ADR-009: Separate, curated, signed Nebula binary updates with automatic rollback
**Status:** Proposed (`service` branch), 2026-11-10. Replaces ADR-006 for Nebula updates on this branch. App updates still follow ADR-006.

**Decision:**
- Binaries are downloaded **directly from upstream GitHub releases**.
- By default they're only installed if listed, with their hashes, in **our minisign-signed channel manifest** (`curated`). That manifest is updated separately from app releases.
- An opt-in `upstream` channel trusts GitHub HTTPS plus upstream checksums.
- Versions are installed side by side. A health check runs after switching, with automatic rollback on failure.

**Why:**
- Upstream release checksums aren't signed separately, so `curated` adds a trust anchor we control, plus a compatibility test, without us having to host binaries.
- `upstream` exists for users who want fixes the moment they're released.

**Consequences:**
- We have to keep up a CI job that watches upstream releases and tests and approves new versions.
- The service has to enforce a supported version range.
- See updates.md.
