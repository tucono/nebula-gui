<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Platform Notes

What each installer does, as built in M4. The files are in `packaging/`; [development.md](development.md#packages) says how to build and test them.

| | Linux | macOS | Windows |
|---|---|---|---|
| Package | `.deb`, `.rpm` (amd64) | `.pkg` (universal, macOS 11+) | Inno Setup `.exe` (amd64, Windows 10+) |
| Service | systemd unit, user `nebula-guid` | launchd daemon, root | Windows service, LocalSystem |
| Socket or pipe | `/run/nebula-gui/nebula-guid.sock` | `/var/run/nebula-guid.sock` | `\\.\pipe\nebula-guid` |
| State directory | `/var/lib/nebula-gui` | `/Library/Application Support/nebula-gui` | `%ProgramData%\nebula-gui` |
| Who may use it | group `nebula-gui` | group `nebula-gui` | group `nebula-gui`, plus the installing user directly |
| Installing user's access | after a restart | at the next app start | at once |
| Uninstall keeps | the state directory, user and group | the state directory and group (`--remove-settings` removes them) | the state directory and group (asks; `/REMOVESETTINGS`) |

Uninstalling leaves each user's own files alone: `peers.json`, `mask-names.json` and the GUI log in `os.UserConfigDir()/nebula-gui/` (`~/.config` on Linux, `~/Library/Application Support` on macOS, `%AppData%` on Windows), and the start-at-sign-in entry. The one exception is the macOS uninstall script, which removes the start-at-sign-in LaunchAgents because they'd point at the removed app. The entries are `~/.config/autostart/nebula-gui.desktop` (Linux), `~/Library/LaunchAgents/io.github.tucono.nebulagui.plist` (macOS) and the `nebula-gui` value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` (Windows).

## Linux
- **Tun device:** needs `CAP_NET_ADMIN` and `/dev/net/tun`. ICMP ping needs `CAP_NET_RAW`.
- **Files:** `/usr/bin/nebula-gui`, the service at `/usr/libexec/nebula-gui/nebula-guid` (off everyone's `PATH`), the unit at `/usr/lib/systemd/system/nebula-guid.service`, `/usr/lib/sysusers.d/nebula-gui.conf`, the `.desktop` file and icons as `io.github.tucono.nebulagui`, and `/usr/share/doc/nebula-gui/THIRD_PARTY_NOTICES.txt`.
- **Service:** runs as the system user `nebula-guid` with only `CAP_NET_ADMIN` and `CAP_NET_RAW` (ambient and bounding), `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp` and only `/dev/net/tun` allowed; `StateDirectory` and `RuntimeDirectory` give it `/var/lib/nebula-gui` (`0700`) and `/run/nebula-gui`. It waits for `network-online.target` and restarts on failure. See ADR-007 in [decisions.md](decisions.md).
- **Install** (`postinstall.sh`): creates the user and the `nebula-gui` group (`systemd-sysusers`, or `useradd`/`groupadd` without it). On a fresh install it adds the installing user to the group: `$SUDO_USER`, else the user behind `$PKEXEC_UID`, else the user of the active session on `seat0` (a software centre installs through PackageKit, which doesn't say who asked). Then `systemctl enable --now`. An upgrade only runs `try-restart`, so a service someone stopped or disabled stays that way, and grants no one.
- **Group membership takes a restart.** Signing out isn't enough on every desktop: on COSMIC the systemd user manager, which starts the apps, outlived the sign-out and kept the old groups. The app notices a user who's in `nebula-gui` in `/etc/group` but not in this session and says **Restart your computer to finish**. Others are added with `sudo usermod -aG nebula-gui <name>`.
- **Remove:** `preremove.sh` disables and stops the service (which stops Nebula and removes the tun device). `/var/lib/nebula-gui`, the `nebula-guid` user and the `nebula-gui` group stay even on purge; delete the directory by hand.
- **Updates (M4.5):** a service that isn't root can't install packages, so Linux installs only notify. See ADR-007 and [updates.md](updates.md).
- **Tray:** needs a StatusNotifier host (`org.kde.StatusNotifierWatcher` on the session bus): KDE, COSMIC with the Notification Tray applet, and GNOME with the AppIndicator extension (Ubuntu ships it). Without one, closing the window quits the app, and start at sign-in isn't offered.
- **Packaging:** `.deb` and `.rpm` through [nfpm](https://nfpm.goreleaser.com/), built on the oldest Ubuntu runner so the GUI's glibc requirement stays low. They depend on the GL, Wayland, X11 and xkbcommon libraries Fyne loads. Flatpak is a poor fit because the service needs host privileges. Only amd64 is built.
- **Conflicts:** the packages don't conflict with the distro `nebula` package. If both run on the same port, connecting fails with "Another program is using Nebula's port".
- **Tested on:** COSMIC (Pop!_OS) by hand; the `.deb` on Ubuntu and the `.rpm` on Fedora in CI's install checks only. GNOME, KDE and Fedora desktops are untested.
- **File picker:** the GUI uses the desktop's file chooser through the XDG Desktop Portal, and falls back to Fyne's picker without one (see `internal/gui/portal_linux.go`).
  - **Known issue on COSMIC:** if the app quits while the picker is open, the picker stays open. The GUI closes the portal request (`Request.Close`), but `xdg-desktop-portal-cosmic` (seen with 1.11.0) doesn't register a request object for its file chooser, so the call does nothing. Its dialog only closes when the user picks a file or cancels. The leftover picker is harmless: nothing it returns reaches the app. GNOME and KDE close the picker. Not yet reported upstream.
  - The picker opens as a separate window rather than on top of the app: under XWayland the portal gets an X11 handle that COSMIC doesn't attach to, and Fyne offers no Wayland handle.

## macOS
- **Tun device:** Nebula uses `utun`, which needs root.
- **Files:** `/Applications/nebula-gui.app` (a universal binary; the bundle is made by `packaging/macos/build-pkg.sh`, not `fyne package`), the service and the uninstall script in `/usr/local/libexec/nebula-gui/`, and `/Library/LaunchDaemons/io.github.tucono.nebula-guid.plist`. The service isn't inside the app, so dragging the app to the Trash doesn't break a running service.
- **Service:** a launchd daemon running `nebula-guid --socket-group nebula-gui` as root, with `RunAtLoad` and `KeepAlive`. Its stderr goes to `/var/log/nebula-guid.log`; its own log is in the state directory as on every platform.
- **Install:** `preinstall` notes an upgrade and boots the old daemon out, and quits the app in every session. `postinstall` creates the `nebula-gui` group (`dseditgroup`), on a fresh install adds the console user (`stat -f%Su /dev/console`), and bootstraps the daemon. The package isn't relocatable, so it never updates a development build found elsewhere.
- **Group membership** takes effect the next time the app starts. Others are added with `sudo dseditgroup -o edit -a <name> -t user nebula-gui`.
- **Uninstall:** `sudo /usr/local/libexec/nebula-gui/uninstall.sh [--remove-settings]` boots the daemon out, quits the app, removes the files, each user's start-at-sign-in LaunchAgent (it would point at the removed app) and the package receipt. `--remove-settings` also removes the state directory and the group.
- **Signing:** the app and the service are signed ad hoc (Apple silicon runs only signed code); the package isn't signed. Gatekeeper blocks it until the user clicks **Open Anyway** in System Settings → Privacy & Security; the README walks through it. Developer ID signing and notarization are M5. `SMAppService`, which could register the daemon from inside the app, needs a signed app too.
- **Future:** a NetworkExtension (`NEPacketTunnelProvider`) would remove the need for a root daemon, but it needs Apple entitlements and a different architecture. Treat it like the iOS work.

## Windows
- **Tun device:** Nebula uses the **Wintun** driver. Nebula looks for `wintun.dll` (matching the CPU architecture) at `dist\windows\wintun\bin\<arch>\wintun.dll` under the directory of `nebula-guid.exe` (`<arch>` is `amd64`, `arm64` or `x86`), the layout of the zip from wintun.net, and fails to create the adapter if it isn't there. The installer puts it there, with Wintun's license. Wintun also needs an adapter name, which Nebula doesn't default on Windows, so the service names it `nebula1` when the config has no `tun.dev`.
- **Files:** both programs, `nebula-gui.ico`, Wintun and `THIRD_PARTY_NOTICES.txt` in `C:\Program Files\nebula-gui\`, and a Start Menu shortcut for all users. The programs carry version details, an icon and a manifest (`packaging/windows/*.winres.json`), and aren't stripped: Defender's machine-learning scan quarantined an unsigned, stripped build with no version details as `Program:Win32/Wacapew.A!ml`.
- **Service:** `nebula-guid`, a Windows service running as LocalSystem, starting automatically and restarting on failure, through `golang.org/x/sys/windows/svc`. `nebula-guid.exe install --allow-user <name>` / `--allow-session-user`, `uninstall` and `run` manage it ([development.md](development.md#windows-as-a-service)). Start, stop and fatal errors go to the Application event log under `nebula-guid`.
- **IPC:** a named pipe through `github.com/Microsoft/go-winio`, with an explicit security descriptor: SYSTEM, Administrators, the `nebula-gui` group and the installing user.
- **Install:** adds an inbound UDP firewall rule `nebula-gui` for `nebula-guid.exe` on any port (the port comes from the config), then `nebula-guid install --allow-session-user`: it creates the `nebula-gui` group and grants the person signed in to the installer's session, both directly (at once) and through the group. When someone else types an administrator's password into the elevation prompt, that's still the person at the screen. An upgrade stops the service, replaces the files, keeps the registration (so it grants no one) and starts the service again only if it was running. The app is ended in every session first (`CloseApplications=force`).
- **Group membership** takes effect at the next sign-in. Others are added from an administrator's Command Prompt with `net localgroup nebula-gui <name> /add`.
- **Uninstall:** asks "Also remove your Nebula settings?" (default No; `/REMOVESETTINGS` for a silent uninstall), runs `nebula-guid uninstall` (stops Nebula and removes the service), removes the firewall rule and ends the app in every session. Removing the settings also removes the `nebula-gui` group.
- **Silent install:** `setup.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART`; the uninstaller (`unins000.exe` in the install folder) takes the same flags.
- **Signing:** not signed until M5. Edge and SmartScreen warn; the README walks through it. See ADR-008 in [decisions.md](decisions.md) for why Inno Setup rather than an MSI.
- **Without OpenGL** (a VM, or a remote desktop session without a GPU), the GUI can't draw its window. Mesa's software renderer works: copy `opengl32.dll`, `libgallium_wgl.dll` and the DLLs they need from MSYS2's `mingw-w64-ucrt-x86_64-mesa` into `C:\Program Files\nebula-gui\`, and set the system environment variable `GALLIUM_DRIVER=llvmpipe` (Mesa's default Direct3D 12 driver crashes on a VM's basic display adapter). The installer doesn't bundle Mesa. The service is unaffected. See [development.md](development.md#per-os-packages) for building in such a VM.

## Mobile

Mobile is a **stretch goal**, and it **does not reuse the desktop service design**:

| | Android | iOS |
|---|---|---|
| OS VPN API | `VpnService` gives the app a tun file descriptor | `NEPacketTunnelProvider` app extension |
| Privileged service | Not possible or needed | Not possible or needed |
| Nebula integration | gomobile-bound library given the tun fd | gomobile `.xcframework` running inside the extension, with a memory limit (~50 MB) |
| Fyne support | Fyne apps run, but the `VpnService` binding needs Java/Kotlin glue | Fyne apps run, but the Network Extension target has to be native Swift |
| Extra requirements | — | Paid Apple developer account and the Network Extension entitlement |

**Before doing any mobile work:** Defined Networking's open-source [Mobile Nebula](https://github.com/DefinedNet/mobile_nebula) app (Flutter + gomobile) already supports custom configs on both platforms. Recommending it may be better than building a new app. If we do build one, its native code is the reference for the tun and extension integration.

To keep mobile possible, keep `internal/gui` free of desktop-only assumptions (put tray code behind a `desktop.App` type assertion), and keep the "engine" interface narrow enough to back with an in-process mobile engine instead of the IPC client.
