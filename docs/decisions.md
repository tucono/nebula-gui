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
**Status:** Accepted, 2026-11-10

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
**Status:** Accepted, 2026-11-10

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
