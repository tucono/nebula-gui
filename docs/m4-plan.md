<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# M4 plan: Desktop integration and packaging

Detailed plan for the [M4 roadmap items](roadmap.md#m4-desktop-integration-and-packaging).

## What M4 delivers

At the end of M4, someone on the network gets a **download, not a build**: they install one package, and the app works like any other desktop app.

- **One installer per platform** puts both binaries in place, registers the background service, starts it, and gives the installing user access to it. Uninstalling removes all of it.
- **The service starts at boot** and, with **Connect automatically** turned on, brings the connection up before anyone logs in.
- **A tray icon** shows the connection state at a glance and has Connect/Disconnect, Open and Quit. Closing the window leaves the app in the tray; the tunnel stays up either way.
- **Opening the app twice** shows the window that's already running instead of a second copy.
- **A Settings tab** holds the two new choices: connect automatically, and start the app when you sign in.
- **A release workflow** builds the packages on a version tag and attaches them, with a `SHA256SUMS` file, to a draft GitHub release.

The people using the app are the same as in M2 (see [Who this is for](m2-plan.md#who-this-is-for)). For M4 that means: installing must not need a terminal, and anything that can't be done for them (such as signing out after being added to a group) is said once, in plain words, at the moment it matters.

**Left for later:** code signing and notarization (M5), so M4 installers show the platform's "unknown publisher" warnings; the updater, manifest and signatures (M4.5); arm64 Linux and Windows packages; Flatpak.

## Starting point

- `nebula-guid` runs in the foreground only. It already takes `--listen`, `--socket-owner`, `--socket-group` and `--state-dir`, stops Nebula on SIGTERM (`Server.Close` → `manager.shutdown`), and has default addresses and state directories per OS (`internal/ipc/address_*.go`, `internal/service/statedir_*.go`).
- `internal/ipc` already gives the socket a group and mode `0660` (Unix), and appends owner and group ACEs to a protected DACL (Windows).
- The GUI's problem catalogue already explains a missing service and a missing `nebula-gui` group membership, with the `usermod`/`dseditgroup` command. Today those steps say "restart your computer" or "reinstall", because there's no installer yet.
- The GUI is created with `app.NewWithID("io.github.tucono.nebulagui")` and quits when the window closes. Fyne 2.8's `desktop.App` has `SetSystemTrayMenu`, `SetSystemTrayIcon` and `SetSystemTrayWindow` (shows the window on a left click). `fyne.io/systray` is already an indirect dependency, and `godbus` a direct one (the file-picker portal).
- `Makefile` lists `make package-{linux,macos,windows}` as planned. CI runs vet, lint and tests on all three OSes but builds no packages.
- There's no app icon yet.

## The tray

### What it shows

| Nebula state | Icon | Tooltip and first menu line |
|---|---|---|
| running | filled, "on" | "Connected · 192.168.100.20" |
| starting / stopping | outline, dimmed | "Connecting…" / "Disconnecting…" |
| stopped | outline | "Not connected" |
| error, or service unreachable | outline with a mark | "Not connected · needs attention" |

The first menu line is a disabled item with the same text as the tooltip, because some Linux trays don't show tooltips.

Menu:

1. *(status line)*
2. **Connect** or **Disconnect**, depending on the state. Disabled while starting or stopping, or with no config. A failed Connect opens the window on the Status tab, where the problem banner explains it.
3. **Open nebula-gui**
4. **Quit**: quits the app only. The first time, a one-line notice says "The connection stays on. Disconnect first if you want it off."

`trayModel(st *api.StatusResponse, reachErr error) trayView` is a pure function with a table test, like `describeStatus`. It returns the icon, the status text and which actions are enabled.

### Closing the window

- **With a tray**, closing the window hides it (`SetCloseIntercept`) and the app keeps running. The first time, a system notification says "nebula-gui is still running in the tray." Quit is in the tray menu and in File → Quit.
- **Without a tray**, closing the window quits the app, as it does today. Otherwise the app would be running with no way to reach it.
- **Finding out whether there's a tray:** on Linux, check that `org.kde.StatusNotifierWatcher` has an owner on the session bus (COSMIC, KDE, and GNOME with the AppIndicator extension, which Ubuntu ships). Windows and macOS always have one. The check runs at start-up; a tray that appears later is picked up on the next start.
- The window remembers that it was hidden only for this run. Starting the app always shows the window, except when it's started at sign-in (see [Start at sign-in](#start-at-sign-in)).

### Polling while hidden

While the window is hidden, the poller keeps calling `GET /v1/status` (the tray needs it) but every 10 s instead of every 2 s, and stops calling `GET /v1/peers` and `GET /v1/logs`. Showing the window pokes the poller so it's current at once. This is the change M3 left for M4.

### One copy at a time

With close-to-tray, people will click the app's icon again to "open" it. That has to show the running window, not start a second app with its own poller and tray icon.

- On start, the GUI listens on a per-user socket or pipe: `$XDG_RUNTIME_DIR/nebula-gui/gui.sock` (Linux), `os.UserCacheDir()/nebula-gui/gui.sock` (macOS), `\\.\pipe\nebula-gui-<user SID>` with a DACL for that user only (Windows). Directory `0700`, socket `0600`. It reuses `internal/ipc`.
- If something already answers there, the new copy sends `show` and exits. The running copy shows and raises its window.
- The only message is `show`. Anything else is ignored, so the socket can't be used to drive the app.
- A stale socket from a crash is removed, as the service already does.

## Settings

A fourth tab, **Settings**, after Logs. M4.5 adds an **Updates** section here (see [updates.md](updates.md#what-the-user-sees)).

| Setting | Default | Stored by | Effect |
|---|---|---|---|
| **Connect automatically when this computer starts** | off | service | The service connects at start-up if a config is imported. Applies to every user of the computer, so the text under it says so. |
| **Open nebula-gui when I sign in** | on, if there's a tray | GUI, per user | Starts the app hidden in the tray at sign-in. Shown only when there's a tray. |

### Connect automatically

- **Off by default.** A fresh install doesn't connect at boot. Only someone turning it on in Settings changes that; installers, upgrades and importing a config never turn it on.
- Stored in `<state dir>/settings.json` (`0600`), next to the config: `{ "autoconnect": true }`. A missing or damaged file means off, with a warning in the service log for a damaged one.
- New endpoints, documented in [ipc-api.md](ipc-api.md): `GET /v1/settings` and `PUT /v1/settings` with `{ "autoconnect": bool }`. Unknown fields are refused (`bad_request`), so M4.5 can add update settings to its own endpoint without confusion.
- At start-up, if `autoconnect` is on and a config is stored, the service connects exactly as `POST /v1/connect` does and logs "connecting automatically". A failure leaves the state `error` with its reason, as a manual connect does: the tray shows "needs attention" and the window explains it when opened.
- Removing the config leaves the setting alone; there's simply nothing to connect.
- **Boot timing:** the systemd unit waits for `network-online.target`. Windows and macOS have no reliable equivalent, so a lighthouse given by DNS name may not resolve at the first try. Nebula looks up `static_host_map` names again periodically, so it should recover on its own; the lab test below checks this, and if it doesn't, the service retries the start after 30 s, at most three times.
- Who can change it: anyone who can use the socket, the same people who can connect and disconnect.

### Start at sign-in

- Linux: `~/.config/autostart/nebula-gui.desktop` with `Exec=nebula-gui --hidden`.
- macOS: a LaunchAgent in `~/Library/LaunchAgents/` running the app with `--hidden`. (`SMAppService` needs a signed app; M5.)
- Windows: `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` with `"…\nebula-gui.exe" --hidden`.
- `--hidden` starts the app in the tray without showing the window. If there's no tray, it shows the window anyway.
- The checkbox reflects whether the file or registry value exists, so removing it by hand is respected. The installers don't create it; the GUI creates it on first run when the default is on, and says so once on the Settings tab.

## Installing the service

### Linux (systemd)

The service runs as a **dedicated system user** with only the capabilities Nebula and ping need, not as root:

```ini
# packaging/linux/nebula-guid.service
[Unit]
Description=nebula-gui background service
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=nebula-guid
Group=nebula-guid
SupplementaryGroups=nebula-gui
ExecStart=/usr/libexec/nebula-gui/nebula-guid --socket-group nebula-gui
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
NoNewPrivileges=yes
RuntimeDirectory=nebula-gui
RuntimeDirectoryMode=0755
StateDirectory=nebula-gui
StateDirectoryMode=0700
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
DeviceAllow=/dev/net/tun rw
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

- `sysusers.d/nebula-gui.conf` creates the `nebula-guid` user and the `nebula-gui` group.
- The socket is `/run/nebula-gui/nebula-guid.sock`, owned by `nebula-guid:nebula-gui`, mode `0660`, matching [ipc-api.md](ipc-api.md).
- `nebula-guid` lives in `/usr/libexec/nebula-gui/` so it isn't on anyone's `PATH`; the GUI is `/usr/bin/nebula-gui`.
- Hardening is tried one directive at a time in the lab; any that stops Nebula creating the tun device or routes comes out, with a note why.
- **This shapes M4.5:** a non-root service can't run `apt-get install`, so on Linux the updater only notifies. See [Risks and decisions](#risks-and-decisions).

### Windows (Service Control Manager)

- `nebula-guid.exe` gains subcommands, through `golang.org/x/sys/windows/svc` and `svc/mgr`:
  - `install --allow-user <name>`: creates the `nebula-gui` group if it's missing, adds `<name>` to it, and registers the `nebula-guid` service (automatic start, LocalSystem, restart on failure), with its command line `run --socket-group nebula-gui --socket-owner <name>`. The installer uses `--allow-session-user` instead, which picks the person signed in to the session the installer runs in (WTS): when someone else types an administrator's password into the elevation prompt, the elevated installer runs as the administrator but in the session of the person at the screen, who's the one who'll use the app. With no one signed in (session 0, as for a process a service starts), it grants no one and says to add people to the group.
  - `uninstall`: stops it and removes it.
  - `run`: what the SCM starts. Inside the SCM it runs the `svc.Handler`; a Stop or Shutdown request cancels the context `Serve` already uses, so Nebula stops cleanly.
  - With no subcommand it runs in the foreground, as today.
- The pipe's DACL grants the installing user's SID directly, as well as the `nebula-gui` local group. Group membership on Windows only takes effect after signing out, and granting the user directly means the person who installed it can use the app at once. Others are added to the group.
- Logs go to the existing log file; Event Log entries only for start, stop and fatal errors.

### macOS (launchd)

- `/Library/LaunchDaemons/io.github.tucono.nebula-guid.plist`, running `/usr/local/libexec/nebula-gui/nebula-guid --socket-group nebula-gui` as root, `RunAtLoad` and `KeepAlive`.
- The binary isn't inside the `.app`, so dragging the app to the Trash doesn't break a running daemon. An uninstall script removes both.
- Socket `/var/run/nebula-guid.sock`, `root:nebula-gui`, `0660`.

### Giving the user access

| | Group created | Installing user added | Takes effect |
|---|---|---|---|
| Linux | `nebula-gui` (sysusers) | on a fresh install, `postinst` adds `$SUDO_USER` or the user behind `$PKEXEC_UID`; failing both (a software centre installs through PackageKit, which doesn't say who asked), the user of the active session on `seat0` | after a restart |
| macOS | `nebula-gui` (`dseditgroup`) | `postinstall` adds the console user (`stat -f%Su /dev/console`) | at the next app start |
| Windows | `nebula-gui` (`nebula-guid install`, run by the installer) | on a fresh install, the person signed in to the installer's session is granted directly, and added to the group | at once |

On Linux, the GUI's **no permission** problem already gives the command. It gains a check: if the user is in `nebula-gui` according to `/etc/group` but not in this session (`os.Getgroups`), it says "Restart your computer to finish" instead. Signing out isn't enough on every desktop: on COSMIC the systemd user manager, which starts the apps, outlived the sign-out and kept the old groups in the M4 smoke test, and only a restart applied the new one.

### Uninstalling

- Stops the service first, which stops Nebula and removes the tun device.
- Keeps the state directory (the imported config, settings and logs) by default, so a reinstall picks up where it left off. On Linux even a purge keeps it, as do the `nebula-guid` user and `nebula-gui` group. `apt purge` / `dnf remove` followed by deleting the directory, a question in the Windows uninstaller ("Also remove your Nebula settings?", default No; `/REMOVESETTINGS` for a silent uninstall), which also removes the `nebula-gui` group,, and a flag to the macOS uninstall script remove it.
- Doesn't touch users' own settings (`peers.json`, `mask-names.json`, the GUI log, sign-in entries) beyond what the OS uninstaller conventionally removes. The docs list where they are.

## Packages

| Platform | Format | Tool | Contents |
|---|---|---|---|
| Linux amd64 | `.deb`, `.rpm` | [nfpm](https://nfpm.goreleaser.com/) | both binaries, the unit, sysusers file, `.desktop` file, icons, `postinst`/`prerm` scripts |
| macOS (universal) | `.pkg` | `pkgbuild` + `productbuild` | `nebula-gui.app` (`fyne package`), the daemon binary and plist, `postinstall`, an uninstall script |
| Windows amd64 | `.exe` setup | Inno Setup | both binaries, `wintun.dll`, Start Menu shortcut, firewall rule, service install, group |

- **Linux build baseline:** packages are built on the oldest Ubuntu LTS runner still offered, so the GUI's glibc requirement is low enough for Debian stable and Fedora. Package dependencies are the runtime libraries Fyne needs (`libgl1`, `libxkbcommon0` or the Fedora names).
- **Conflicts:** the packages don't declare a conflict with the distro `nebula` package. Both can be installed; running both on the same port fails with the existing `port_in_use` message.
- **wintun.dll** is downloaded in CI from wintun.net at a pinned version and checked against a SHA-256 in `packaging/windows/wintun.sha256`. It's never committed.
- **Windows firewall:** an inbound UDP rule for `nebula-guid.exe` on any port (the port comes from the config), added and removed by the installer.
- **The macOS app bundle** is built on the macOS runner, as a universal binary (`lipo` of amd64 and arm64). The service cross-compiles; the GUI needs CGO, so each arch is built natively or with the SDK's `-arch` flags.
- **The Windows GUI** is linked with `-ldflags -H=windowsgui`, so neither the Start Menu shortcut nor start at sign-in opens a console window next to it.
- **Icons:** one app icon (SVG source plus PNG and ICO/ICNS sizes) and three tray icons (on, off, attention) in `internal/gui/assets/`, embedded with `go:embed`. Tray icons are monochrome templates on macOS.
- **Version:** packages take their version from the tag (`v0.1.0` → `0.1.0`). Builds from untagged commits get `0.0.0~dev.<n>.<hash>` and aren't released.

### Release workflow

`.github/workflows/release.yml`, on a pushed tag `v*`:

1. The existing test job.
2. One build job per OS, producing the packages above.
3. A job that collects them, writes `SHA256SUMS`, and creates a **draft** release with all of it attached. Publishing the draft stays a manual step.

The workflow needs only `contents: write` for the release and no secrets. M4.5 adds the manifest and its signature here.

Release URLs must work without signing in, which GitHub release assets on a public repo do.

## Steps

### 1. Service: settings and autoconnect

**Files:** `internal/service/settings.go`, `settings_test.go`, `server.go`, `manager.go`; `internal/api` types and client.

- `settings.json` load and save, `GET`/`PUT /v1/settings`, autoconnect at start-up with the retry described above.
- Tests with the fake engine: off with no settings file, off after importing a config, a damaged file, autoconnect with and without a config, a failed autoconnect leaving `error` with its reason, unknown fields refused.

### 2. Service: running under the system's service manager

**Files:** `cmd/nebula-guid/main.go`, `service_windows.go`, `service_other.go`.

- Windows `install`/`uninstall`/`run` subcommands and the `svc.Handler`.
- Linux and macOS need no code: systemd and launchd run the foreground binary. `main` gains a check that a release build isn't run as an ordinary user without `CAP_NET_ADMIN` (a clear message instead of a tun error on connect).
- Tests: argument handling, the handler's stop path with a fake `Serve`.

### 3. GUI: tray, close-to-tray and polling while hidden

**Files:** `internal/gui/tray.go`, `tray_test.go`, `tray_linux.go` (watcher check), `tray_other.go`, `main_window.go`, `poller.go`, `gui.go`; `internal/gui/assets/`.

- `trayModel` and its table test; wiring to `desktop.App` behind a type assertion, so non-desktop builds still compile (see [platforms.md](platforms.md#mobile)).
- `SetCloseIntercept`, the first-time notice and notification, `--hidden`.
- The poller's hidden-window rates.
- Placeholder icons (see [Icons](#risks-and-decisions)): simple original shapes drawn as SVG, rendered to the PNG, ICO and ICNS sizes the packages need by a small script kept in the repo, so replacing the SVGs later regenerates everything.

### 4. GUI: one copy at a time

**Files:** `internal/gui/instance.go`, `instance_unix.go`, `instance_windows.go` and tests.

- Listen-or-send-`show`, the stale socket, the DACL on Windows. Tests with two in-process instances on a temp path.

### 5. GUI: Settings tab and start at sign-in

**Files:** `internal/gui/settings_view.go`; `internal/autostart/` (`autostart_linux.go`, `autostart_darwin.go`, `autostart_windows.go`) and tests. The entries are a package of their own, without Fyne, so they build and vet for every OS from one machine.

- The two settings, with the service one disabled and explained when the service can't be reached.
- Autostart files written to a temp home in tests; on Windows, the Run value under a test key in HKCU. A Run value turned off in Task Manager's Startup apps counts as off.

### 6. GUI: problems that know about the installer

**Files:** `internal/gui/problems.go`, `connerr_*.go` and tests.

- **Not installed** vs **not running**: the GUI checks for the service's files (`/usr/libexec/nebula-gui/nebula-guid`, the launchd plist, the Windows service) to tell them apart. Not installed points to the download page; not running gives the platform's way to start it, with a **Start the service** button where the OS can ask for the password itself (`systemctl start nebula-guid` on Linux, an elevated `sc start` on Windows). On Linux systemd asks polkit, which has the desktop ask for the password and allows only starting units; `pkexec` would run a whole program as root. If that fails (no polkit agent, say), a dialog gives `sudo systemctl start nebula-guid` instead. macOS gets the `launchctl kickstart` command, not a button.
- **Signed out needed** for the group, as above.

### 7. Linux package

**Files:** `packaging/linux/` (`nfpm.yaml`, unit, sysusers, `.desktop`, scripts), `Makefile` (`package-linux`).

- `make package-linux` builds both binaries with the release flags and runs nfpm.
- CI: `systemd-analyze verify` on the unit, and installing the `.deb` in a container to check files, the user and group, and modes.

### 8. Windows installer

**Files:** `packaging/windows/` (`nebula-gui.iss`, `wintun.sha256`, `fetch-wintun.ps1`, `check-install.ps1`), `cmd/nebula-guid/access_windows.go`, `service_windows.go`, `Makefile` (`package-windows`), `.github/workflows/ci.yml`.

- `make package-windows` (from bash on Windows) fetches and checks Wintun, builds both binaries (the GUI with `-H=windowsgui`), and runs ISCC. The setup is `dist/nebula-gui-<version>-windows-amd64-setup.exe`.
- Fresh install: files, Start Menu shortcut, firewall rule, then `nebula-guid install --allow-session-user` and `net start`. Upgrade: `net stop` before the files are replaced; the service's registration is kept, so an upgrade never grants access, and it's started again only if it was running. The app is closed through Restart Manager (`CloseApplications=force`).
- Uninstall: asks about the settings first, then `nebula-guid uninstall` (stops Nebula, removes the service), removes the firewall rule and ends the app in every session.
- CI (`package-windows` job): builds the setup on `windows-latest`, installs it silently and checks the service, group, firewall rule and the API over the pipe (`check-install.ps1`), then uninstalls twice: keeping the settings, then with `/REMOVESETTINGS`.
- A service registered by hand from another directory (`docs/development.md`) is stopped and deleted, and the install carries on as a fresh one.

### 9. macOS package

**Files:** `packaging/macos/` (`io.github.tucono.nebula-guid.plist`, `Info.plist`, `Distribution.xml`, `scripts/preinstall`, `scripts/postinstall`, `uninstall.sh`, `build-pkg.sh`, `check-install.sh`), `Makefile` (`package-macos`), `.github/workflows/ci.yml`.

- `make package-macos` (on a Mac) builds both binaries for amd64 and arm64 (the GUI with cgo, `MACOSX_DEPLOYMENT_TARGET=11.0`), and `build-pkg.sh` joins them with `lipo`, makes the app bundle by hand (`Info.plist`, the ICNS from `make icons`; no `fyne package`), signs the app and service ad hoc (Apple silicon runs only signed code; a Developer ID is M5), and runs `pkgbuild` and `productbuild`. The package is `dist/nebula-gui-<version>-macos-universal.pkg`, for macOS 11 or later.
- Payload: `/Applications/nebula-gui.app`, `/usr/local/libexec/nebula-gui/{nebula-guid,uninstall.sh}`, `/Library/LaunchDaemons/io.github.tucono.nebula-guid.plist`. The app isn't relocatable, so the installer never updates a development build found elsewhere.
- `preinstall`: on an upgrade (the daemon's plist exists) leaves a note for `postinstall` and boots the daemon out; it quits the app in every session either way. `postinstall`: creates the `nebula-gui` group (`dseditgroup`), on a fresh install adds the console user, and bootstraps the daemon. Its stderr goes to `/var/log/nebula-guid.log`.
- `sudo /usr/local/libexec/nebula-gui/uninstall.sh [--remove-settings]`: boots the daemon out, quits the app, removes the files, each user's start-at-sign-in LaunchAgent (it would point at the removed app) and the package receipt. `--remove-settings` also removes the state directory and the group.
- CI (`package-macos` job): builds on `macos-latest`, installs with `installer`, checks the files, universal binaries, signature, daemon, socket owner and mode, and the API (as root, as the console user, refused to `nobody`), then uninstalls twice: keeping the settings, then with `--remove-settings`.

### 10. Release workflow

**Files:** `.github/workflows/release.yml`.

- First run with a `v0.1.0-rc.1` tag, kept as a draft and deleted afterwards.

### 11. Docs

- `docs/architecture.md`: tray, single instance, settings, autoconnect.
- `docs/platforms.md`: what each installer does, as built.
- `docs/ipc-api.md`: `/v1/settings`.
- `docs/security.md`: the service user and capabilities, socket access per platform, the single-instance socket, wintun pinning, unsigned installers and `SHA256SUMS`.
- `docs/development.md`: packaging targets and how to test them; the repository layout loses "(planned)" where it's now true.
- `docs/testing.md`: the M4 smoke tests below.
- `docs/decisions.md`: ADR-007 for the Linux service user and Linux updates, and ADR-008 for Inno Setup (below).
- `docs/updates.md` and `docs/roadmap.md` (M4.5): Linux installs only notify, whatever their source; the install table and the Linux self-update item change to match. On Windows the updater runs the Inno Setup `.exe` silently instead of `msiexec`, and the platform-signature check covers the setup `.exe`.
- `README.md`: an **Install** section per platform, including how to get past the unsigned-installer warnings and how to check `SHA256SUMS`. `docs/roadmap.md`: tick the M4 items.

## Smoke tests

Each GUI commit gets its operator check as usual. The installers are tested on the developer's COSMIC desktop (Linux), a Windows VM reverted to a snapshot before each run, and a Mac, where the uninstall script returns it to a clean state between runs.

**Linux** (the `.deb` on the developer's COSMIC desktop; stop `make dev-service` first, since both use Nebula's port and tun name. The `.rpm` is checked only by CI's install test):

1. Install the `.deb` (or `.rpm`) by double-clicking it. `systemctl status nebula-guid` is active, running as `nebula-guid`. Before restarting, the app shows **Restart your computer to finish**. Restart; the app opens without a permission problem.
2. Import a config made with `testlab/` (lab running on the host; the VM reaches it over the bridge), connect. The tray shows Connected.
3. Close the window: the app stays in the tray. Click the app's icon in the launcher: the same window comes back, and `pgrep -c nebula-gui` is 1.
4. Reboot with the config imported and the settings untouched: not connected, and Connect automatically is unticked. Then turn it on, reboot, don't sign in for a minute, then sign in: already connected, and the service log shows "connecting automatically" before the sign-in. Repeat with the lighthouse given by a DNS name.
5. Quit from the tray: the tunnel stays up (`ip link`). Open the app: Connected.
6. Without a tray (COSMIC with the status area applet removed from the panel): closing the window quits the app.
7. Uninstall: the service and tun device are gone, `/var/lib/nebula-gui` remains. Purge and delete it.

**Windows** and **macOS**: the same steps, with the platform's install and uninstall, the unsigned-installer warning documented in the README, and on Windows a second, non-admin account added to the `nebula-gui` group.

## Risks and decisions

The open choices are decided; the remaining items are risks to watch. The service user and the Windows installer become ADRs (step 11).

- **Linux service user (step 7). Decided:** a dedicated `nebula-guid` user with only `CAP_NET_ADMIN` and `CAP_NET_RAW`, as above. Nebula handles UDP from the internet and the API takes requests from every member of `nebula-gui`; a bug in either then reaches a process that can change networking but can't read other users' files, install software or take over the machine. Root, even sandboxed, still owns most system files.

  **Consequence for M4.5:** the service can't install packages, so **Linux updates only notify**. The service still checks, downloads and verifies the update (signature, hash, no downgrade); the GUI then offers **Download update**, which opens the verified package in the desktop's software installer, where the user confirms with their password. The verified package is put in a directory only the service can write and the `nebula-gui` group can read (for example `/var/cache/nebula-gui/`, via systemd's `CacheDirectory`), so the file the user installs is the one that was checked; M4.5 settles the details. No root helper unit is added. Windows and macOS keep installing updates themselves (Wintun needs SYSTEM, utun needs root, so there's no equivalent choice there). Rejected: running as root to keep self-updates, and a root `.path`-triggered helper that installs the spooled package.
- **Windows installer (step 8). Decided:** Inno Setup. It's preinstalled on GitHub's Windows runners, scripting groups, firewall rules and services is short, it supports silent installs (`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`) and Restart Manager, and the setup `.exe` can be Authenticode-signed in M5. Rejected: an MSI built with WiX, which [updates.md](updates.md#install-mechanics-per-platform) assumes today and which suits centrally managed machines, which this network doesn't have. updates.md changes to match (step 11).
- **Test machines.** A Windows VM and a Mac are available, so every platform's smoke test runs by hand. Linux is tested on COSMIC only: GNOME, KDE and Fedora stay untested, and the `.rpm` gets CI's install check but no desktop test.
- **Icons. Decided:** placeholders made with the code in step 3: an original, simple app icon and three tray states (on, off, attention), not based on Nebula's or anyone else's logo. They can ship in releases until real ones replace them; replacing them means swapping the SVG sources and running the render script.
- **Group restart on Linux.** Adding the user to `nebula-gui` only takes effect after a restart (signing out isn't reliably enough, see [Giving the user access](#giving-the-user-access)), which is the most likely point for someone to get stuck after installing. Granting the installing user directly (a POSIX ACL on the socket, set by the service from a list in its settings) would avoid it. Not planned for M4; revisit if it trips people up.
- **Tray support varies on Linux.** GNOME needs an extension, and trays differ in how they show menus and tooltips. GNOME isn't tested; the no-tray fallback (checked on COSMIC with the applet removed) should keep the app usable there, and the status line in the menu covers trays without tooltips.
- **Fyne's tray on COSMIC/Wayland.** Not tested yet on the developer's desktop. Step 3's operator check covers it; the layout quirk in [the M3 risks](m3-plan.md#risks-and-decisions) doesn't apply to the tray, but applies to the new Settings tab.
- **Unsigned installers until M5.** SmartScreen and Gatekeeper warn, and the README has to walk people past that. `SHA256SUMS` lets the careful ones check, and the person who runs the network can tell everyone the expected hash.

## Suggested commits

1. Service: settings endpoint and autoconnect (step 1)
2. Service: Windows service subcommands (step 2)
3. GUI: tray, close to tray, polling while hidden (step 3)
4. GUI: one copy at a time (step 4)
5. GUI: Settings tab and start at sign-in (step 5)
6. GUI: installer-aware service problems (step 6)
7. Packaging: Linux `.deb`/`.rpm` (step 7)
8. Packaging: Windows installer (step 8)
9. Packaging: macOS `.pkg` (step 9)
10. CI: release workflow (step 10)
11. Docs, roadmap and README install notes (step 11)
