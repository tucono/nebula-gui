# Platform Notes

## Linux
- **Tun device:** needs `CAP_NET_ADMIN` and `/dev/net/tun`. ICMP ping needs `CAP_NET_RAW`, or unprivileged ICMP through `net.ipv4.ping_group_range`.
- **Service:** systemd unit `nebula-guid.service` with `Type=simple`, `RuntimeDirectory=nebula-gui`, and `StateDirectory=nebula-gui`. Run it as a dedicated user with `AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW` rather than full root if possible.
- **Socket access:** the installer creates a `nebula-gui` group. Users are added to it to control the VPN.
- **Packaging:** `.deb` and `.rpm` through [nfpm](https://nfpm.goreleaser.com/). Flatpak is a poor fit because the service needs host privileges, so leave it for later.
- **Conflicts:** if the system `nebula` package (`nebula.service`) is running on the same port or tun name, starting fails. Detect this and show a clear error.
- **Child process (service branch):** the `nebula` child inherits the service's ambient capabilities through `execve`, so a non-root service user with `AmbientCapabilities` works. Set `SysProcAttr.Pdeathsig = SIGTERM` so the child exits if the service dies. Use `KillMode=mixed` in the unit so systemd also stops the child.
- **Binaries:** upstream publishes `nebula-linux-{amd64,arm64,arm-7,…}.tar.gz`, all statically linked. Pick the artifact from `runtime.GOARCH` (and `GOARM`).
- **Custom binary:** admins can set `nebula_binary = /usr/bin/nebula` in `/etc/nebula-gui/service.toml` to use the distro package. Managed updates are then turned off.

## macOS
- **Tun device:** Nebula uses `utun`, which needs root.
- **Service:** a launchd daemon at `/Library/LaunchDaemons/<reverse-dns>.nebula-guid.plist` running as root.
- **GUI:** a `.app` bundle built with `fyne package -os darwin`.
- **Socket access:** create the socket in a root-owned directory and give access to group `staff` or a dedicated group. The service can also check the peer's credentials (`LOCAL_PEERCRED`) for each connection.
- **Distribution:** to avoid Gatekeeper warnings you need a Developer ID certificate, code signing, and notarization. Ship a `.pkg` installer that installs both the daemon and the app. A later option is `SMAppService` to register the daemon from inside the app.
- **Child process (service branch):** stop with `SIGTERM`. macOS has no `Pdeathsig`, so the service records the child's PID and kills orphans at startup. A kqueue `NOTE_EXIT` watcher in a small helper could be added later.
- **Binaries:** upstream publishes a universal `nebula-darwin.zip`. When it's code-signed, check it with `codesign --verify --strict` and pin the Team ID. Files downloaded by the service through Go's HTTP client don't get the `com.apple.quarantine` attribute, so Gatekeeper doesn't block a daemon from running them. Our own verification is therefore the only check.
- **Future:** a NetworkExtension (`NEPacketTunnelProvider`) would remove the need for a root daemon, but it needs Apple entitlements and a different architecture. Treat it like the iOS work.

## Windows
- **Tun device:** Nebula uses the **Wintun** driver. `wintun.dll` (matching the CPU architecture) must be in the same directory as **`nebula.exe`**. That's each version directory, and upstream's zip includes it (see below).
- **Service:** a Windows Service running as `LocalSystem`, using `golang.org/x/sys/windows/svc`. The service binary handles `install`, `uninstall`, and `run` subcommands.
- **IPC:** a named pipe through `github.com/Microsoft/go-winio`, with an explicit security descriptor. Don't use the default, which grants too much.
- **Child process (service branch):**
  - Start Nebula with `CREATE_NEW_PROCESS_GROUP` so it can be stopped gracefully with `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`. Go programs receive CTRL_BREAK as `os.Interrupt`.
  - **Verify on Windows that Nebula shuts down cleanly and removes its Wintun adapter.** If that fails, use `TerminateProcess` after a 10 s timeout.
  - Put the child in a **Job Object** with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, so it exits whenever the service does.
- **Binaries:** upstream's `nebula-windows-{amd64,arm64}.zip` includes `wintun.dll`. Extract `nebula.exe` and the matching DLL into the version directory. The installer's bundled copy covers offline first runs.
- **Firewall:** the inbound UDP rule has to be for **each `nebula.exe` path**, which changes with every version directory. Instead, make the rule for the UDP port, or have the service update the rule (`INetFwPolicy2`) when it switches versions.
- **Packaging:** an MSI (WiX) or Inno Setup installer that installs the service, adds the firewall rule, and creates the GUI's Start Menu shortcut. Sign the binaries to reduce SmartScreen warnings.

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

**Service branch:** iOS and Android don't allow running separate executables, so this design can't run on mobile. Mobile would use the library approach from the `monolithic` branch, or Mobile Nebula.

To keep mobile possible, keep `internal/gui` free of desktop-only assumptions (put tray code behind a `desktop.App` type assertion), and keep the "engine" interface narrow enough to back with an in-process mobile engine instead of the IPC client.
