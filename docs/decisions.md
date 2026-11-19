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

---

## ADR-007: The Linux service runs as its own user, so Linux updates only notify
**Status:** Accepted, 2026-11-19

**Context:** The service has to create a tun device and change routes. Nebula handles UDP from the internet, and the API takes requests from every member of the `nebula-gui` group. A bug in either runs with whatever the service has. ADR-006 assumed the service would install updates itself, which on Linux means running `apt-get` or `dnf` as root.

**Options considered:**
1. **Root**, sandboxed with systemd directives, keeping self-updates.
2. **A dedicated `nebula-guid` user with only `CAP_NET_ADMIN` and `CAP_NET_RAW`**, which can't install packages.
3. Option 2 plus a **root helper unit**, triggered by a `.path` unit, that installs a package the service has verified and spooled.

**Decision:** Option 2. The unit is `packaging/linux/nebula-guid.service`; [platforms.md](platforms.md#linux) lists its hardening.

**Why:**
- A compromised service can change this machine's networking, but can't read users' files, write system files or install software. Root, even sandboxed, still owns most of the system.
- Option 3 brings back a root process that installs whatever it's handed, which is the risk option 2 removes, and adds code that has to get the hand-off exactly right.
- The people using the app install it by double-clicking a package and can type their password again for an update.

**Consequences:**
- **Linux installs only notify, whatever their source** (M4.5). The service still checks, downloads and verifies the update (signature, hash, no downgrade) into a directory only it can write and the `nebula-gui` group can read, and the GUI's **Download update** opens that file in the desktop's software installer, where the user confirms with their password. This replaces the self-update row for Linux in ADR-006's plan ([updates.md](updates.md#install-mechanics-per-platform)).
- Windows and macOS keep installing updates themselves: Wintun needs SYSTEM and utun needs root, so there's no equivalent choice there.
- Adding someone to `nebula-gui` takes effect after a restart on Linux, which the app explains.

---

## ADR-008: Inno Setup for the Windows installer
**Status:** Accepted, 2026-11-19

**Context:** The Windows installer has to install both programs and Wintun, add a firewall rule, create the `nebula-gui` group, register and start the service, close a running app in every session, and support silent installs for M4.5's updater. [updates.md](updates.md) assumed an MSI.

**Options considered:**
- **An MSI built with WiX.** The format for centrally managed machines (Group Policy, Intune), with transactional rollback.
- **Inno Setup**, which builds a setup `.exe` from a Pascal-scripted `.iss` file.

**Decision:** Inno Setup (`packaging/windows/nebula-gui.iss`).

**Why:**
- It's preinstalled on GitHub's Windows runners, so CI needs no extra tooling.
- Groups, firewall rules and service registration take a few lines of script, mostly calls to `nebula-guid.exe install`/`uninstall` and `netsh`. In WiX each needs custom actions.
- It supports silent installs (`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`) and Restart Manager, and its setup `.exe` can be Authenticode-signed in M5.
- Nobody on this network manages machines centrally, which is what an MSI is best at.

**Consequences:**
- The M4.5 updater runs the setup `.exe` silently instead of `msiexec`, and the platform-signature check covers the setup `.exe` ([updates.md](updates.md)).
- Inno Setup undoes its own file changes when setup fails, but not what the script did (the service, the group, the firewall rule). The updater's check that the new service answers with the new version catches a failed install.
- An administrator who wants an MSI for deployment would have to wrap the `.exe`.
