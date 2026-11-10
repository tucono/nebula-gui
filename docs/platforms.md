<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Platform Notes

## Linux
- **Tun device:** needs `CAP_NET_ADMIN` and `/dev/net/tun`. ICMP ping needs `CAP_NET_RAW`, or unprivileged ICMP through `net.ipv4.ping_group_range`.
- **Service:** systemd unit `nebula-guid.service` with `Type=simple`, `RuntimeDirectory=nebula-gui`, and `StateDirectory=nebula-gui`. Run it as a dedicated user with `AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW` rather than full root if possible.
- **Socket access:** the installer creates a `nebula-gui` group. Users are added to it to control the VPN.
- **Packaging:** `.deb` and `.rpm` through [nfpm](https://nfpm.goreleaser.com/). Flatpak is a poor fit because the service needs host privileges, so leave it for later.
- **Conflicts:** if the system `nebula` package (`nebula.service`) is running on the same port or tun name, starting fails. Detect this and show a clear error.

## macOS
- **Tun device:** Nebula uses `utun`, which needs root.
- **Service:** a launchd daemon at `/Library/LaunchDaemons/<reverse-dns>.nebula-guid.plist` running as root.
- **GUI:** a `.app` bundle built with `fyne package -os darwin`.
- **Socket access:** create the socket in a root-owned directory and give access to group `staff` or a dedicated group. The service can also check the peer's credentials (`LOCAL_PEERCRED`) for each connection.
- **Distribution:** to avoid Gatekeeper warnings you need a Developer ID certificate, code signing, and notarization. Ship a `.pkg` installer that installs both the daemon and the app. A later option is `SMAppService` to register the daemon from inside the app.
- **Future:** a NetworkExtension (`NEPacketTunnelProvider`) would remove the need for a root daemon, but it needs Apple entitlements and a different architecture. Treat it like the iOS work.

## Windows
- **Tun device:** Nebula uses the **Wintun** driver. `wintun.dll` (matching the CPU architecture) must be in the same directory as `nebula-guid.exe`. It is downloaded from wintun.net and the license allows redistribution.
- **Service:** a Windows Service running as `LocalSystem`, using `golang.org/x/sys/windows/svc`. The service binary handles `install`, `uninstall`, and `run` subcommands.
- **IPC:** a named pipe through `github.com/Microsoft/go-winio`, with an explicit security descriptor. Don't use the default, which grants too much.
- **Firewall:** Nebula's UDP port needs an inbound rule. The installer adds one for `nebula-guid.exe`.
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

To keep mobile possible, keep `internal/gui` free of desktop-only assumptions (put tray code behind a `desktop.App` type assertion), and keep the "engine" interface narrow enough to back with an in-process mobile engine instead of the IPC client.
