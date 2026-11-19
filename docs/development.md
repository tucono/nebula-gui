<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Development Guide

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Go | the version in `go.mod` (latest stable) | https://go.dev/dl |
| C compiler | — | Fyne uses CGO for OpenGL |
| `golangci-lint` | latest | Linting |

Packaging needs a few more tools; see [Packages](#packages). The `fyne` CLI isn't used: the icons and app bundle are made by scripts in `packaging/`.

### Per-OS packages

**Linux (Debian/Ubuntu/Pop!_OS)**
```sh
sudo apt install gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
```
Install Go from the official tarball at https://go.dev/dl, not from apt. The apt `golang` package is usually too old (Ubuntu 24.04 ships Go 1.22). Once any Go 1.21+ is installed, the `toolchain` line in `go.mod` makes `go` download the exact version the project uses.
**Fedora**
```sh
sudo dnf install golang gcc libXcursor-devel libXrandr-devel mesa-libGL-devel libXi-devel libXinerama-devel libXxf86vm-devel libxkbcommon-devel wayland-devel
```
**macOS:** `xcode-select --install`, then `brew install go`.

**Windows:** install Go, then a GCC toolchain from [MSYS2](https://www.msys2.org/) (`pacman -S mingw-w64-ucrt-x86_64-gcc`) and add its `bin` directory to `PATH`. You also need the zip from https://www.wintun.net: extract it into `bin\dist\windows\`, so the DLL ends up at `bin\dist\windows\wintun\bin\amd64\wintun.dll` next to the service binary (see [platforms.md](platforms.md#windows)).

In a VM or remote desktop session without OpenGL (the GUI says the driver doesn't support it), use Mesa's software renderer: `pacman -S mingw-w64-ucrt-x86_64-mesa`, copy `opengl32.dll`, `libgallium_wgl.dll` and the MSYS2 DLLs they need (`ldd`) next to the built `nebula-gui.exe` (`go run` won't do: Windows loads `opengl32.dll` from the program's directory before `System32`, and `PATH` after it), and set `GALLIUM_DRIVER=llvmpipe`. Mesa's default Direct3D 12 driver crashes on the first draw on a VM's basic display adapter. For the installed app, copy the same DLLs into `C:\Program Files\nebula-gui\` and set `GALLIUM_DRIVER` machine-wide ([platforms.md](platforms.md#windows)). In PowerShell, use `sc.exe` for the service commands: `sc` is `Set-Content`.

## Repository layout

```
cmd/
  nebula-gui/        # GUI entrypoint (unprivileged)
  nebula-guid/       # service entrypoint (privileged); Windows service subcommands
internal/
  api/               # shared request/response types + typed HTTP client
  ipc/               # transport: listener/dialer per OS (unix socket, named pipe), default addresses
  service/           # API handlers, state machine, config store, settings, pinger, log buffer and file
  nebulactl/         # thin wrapper over nebula.Main / *nebula.Control (only package importing nebula)
  gui/               # Fyne windows, widgets, tray, poller, problems, reports, native file pickers
  autostart/         # start at sign-in: XDG autostart file, LaunchAgent, Run registry value
packaging/
  version.sh         # the release version from the tag, or 0.0.0-dev.<n>.<hash>
  icons/             # renders internal/gui/assets/app.svg to PNG, ICO and ICNS (make icons)
  notices/           # writes THIRD_PARTY_NOTICES.txt from the modules built in (make notices)
  linux/             # nfpm config, systemd unit, sysusers file, .desktop file, package scripts, install check
  macos/             # build-pkg.sh, Info.plist, launchd plist, Distribution.xml, scripts, uninstall.sh, install check
  windows/           # Inno Setup script, Wintun hash and fetch script, version resources, install check
tools/netadmin/      # config templates and make-config.sh for whoever runs the network
docs/
testlab/             # scripts + configs for a local multi-node Nebula network
.github/workflows/   # ci.yml (tests, packages, install checks), release.yml (draft release from a tag)
Makefile
```

`internal/updater/` comes with M4.5 ([updates.md](updates.md)).

Rules:
- **Only `internal/nebulactl` imports `github.com/slackhq/nebula`.** This keeps Nebula API changes, which do happen between minor versions, in one place.
- **`internal/gui` never imports `internal/service`.** The GUI and service communicate only through `internal/api`.
- Platform-specific code uses `_linux.go`, `_darwin.go`, and `_windows.go` filename suffixes rather than runtime checks.

## Dependencies

The module is `github.com/tucono/nebula-gui`. Fyne and go-winio were added in M0, Nebula in M1, and pro-bing (the service's ICMP ping) in M3.

Pin the Nebula version on purpose and update it on purpose. Read Nebula's CHANGELOG before each bump. `go.mod` also lists Nebula's `nebula` and `nebula-cert` commands as `tool` dependencies, so the test lab uses the same version as the service (`go tool nebula-cert …` works too). To bump:

```sh
go get github.com/slackhq/nebula@vX.Y.Z github.com/slackhq/nebula/cmd/nebula@vX.Y.Z github.com/slackhq/nebula/cmd/nebula-cert@vX.Y.Z
go mod tidy
```

## Running in development

Run the service and the GUI in separate terminals. Use a socket in your home directory so the GUI doesn't need special group membership.

### Linux
Instead of running the service as root, give the built binary `CAP_NET_ADMIN`:
```sh
make dev-service   # build with -tags dev, setcap, run with --dev on $XDG_RUNTIME_DIR/nebula-guid.sock, state in .dev-state/

# other terminal
make gui
```
`setcap` has to be run again after every rebuild, so `make dev-service` does it each time (it asks for your sudo password).

### macOS
```sh
go build -tags dev -o bin/nebula-guid-dev ./cmd/nebula-guid
sudo ./bin/nebula-guid-dev --listen /tmp/nebula-guid.sock --state-dir .dev-state --dev
NEBULA_GUI_SOCKET=/tmp/nebula-guid.sock go run ./cmd/nebula-gui
```

### Windows (Administrator PowerShell for the service)
```powershell
go build -tags dev -o bin\nebula-guid-dev.exe .\cmd\nebula-guid
Expand-Archive path\to\wintun-*.zip bin\dist\windows   # once
.\bin\nebula-guid-dev.exe --listen \\.\pipe\nebula-guid-dev --state-dir .dev-state --dev
# normal PowerShell
$env:NEBULA_GUI_SOCKET="\\.\pipe\nebula-guid-dev"; go run .\cmd\nebula-gui
```

### Windows, as a service
The release build registers itself with the Service Control Manager; the installer runs the same commands, with `--allow-session-user` (the person signed in at the screen). From an Administrator PowerShell, with Wintun extracted as above:
```powershell
go build -o bin\nebula-guid.exe .\cmd\nebula-guid
.\bin\nebula-guid.exe install --allow-user $env:USERNAME
sc.exe start nebula-guid
# normal PowerShell: the GUI uses the installed pipe by default
go run .\cmd\nebula-gui
# remove it again (the state directory stays)
.\bin\nebula-guid.exe uninstall
```
`install` creates the `nebula-gui` group if it doesn't exist, and adds the `--allow-user` user to it as well as giving them access at once. It points the service at the binary where it is, so don't move or rebuild it while installed: stop the service first. Start, stop and fatal errors go to the Application event log under the source `nebula-guid`; everything else goes to the service's log file.

`--state-dir` is where the service keeps the imported config. It defaults to the installed location (see [architecture.md](architecture.md#service-cmdnebula-guid-internalservice)), so in development point it at the gitignored `.dev-state/`.

`--dev` makes the service:
- log at debug level to stderr;
- skip the start-up permission check. A release build stops at once, with a message, unless it has `CAP_NET_ADMIN` (Linux), runs as root (macOS) or runs elevated (Windows);
- give the user who launched it access to the socket or pipe (`$SUDO_USER` on Linux and macOS, the current user on Windows). `--socket-owner` overrides this.

**Release builds don't accept `--dev`.** The flag exists only in binaries built with `-tags dev` (`cmd/nebula-guid/dev_on.go`). The dev binary is named `nebula-guid-dev` so it can't be mistaken for a release build.

## Common tasks (Makefile targets)

| Target | Does |
|---|---|
| `make build` | Build both binaries into `bin/` |
| `make service` / `make gui-bin` | Build one of them into `bin/`. `make gui-bin` gives a GUI that can be started at sign-in, which `go run` can't. |
| `make dev-service` | Build the service, `setcap` it (Linux), and run it with dev flags |
| `make gui` | `go run ./cmd/nebula-gui` with the dev socket |
| `make test` | `go test ./...` |
| `make lint` | `golangci-lint run` |
| `make fmt` | `golangci-lint fmt` (gofmt + goimports) |
| `make lab-up` / `make lab-down` | Start or stop the local Nebula test lab ([testing.md](testing.md#local-test-lab-integration)) |
| `make lab-import` | Send the lab's dev config to the dev service |
| `make icons` | Render the app icon to `dist/icons/` |
| `make notices` | Write `dist/THIRD_PARTY_NOTICES.txt` for this OS |
| `make package-linux` | Build the `.deb` and `.rpm` into `dist/` (on Linux) |
| `make package-macos` | Build the universal `.pkg` into `dist/` (on a Mac) |
| `make package-windows` | Build the setup `.exe` into `dist/` (on Windows, from bash) |

## Packages

Each package is built on its own OS. The version comes from `packaging/version.sh`: the tag's (`v0.1.0` → `0.1.0`) when `HEAD` is tagged, else `0.0.0-dev.<commits>.<hash>`, with `.dirty` for uncommitted changes. Linux packages write the pre-release part with `~` (`0.1.0~rc.1`), so it sorts before the release. What each package installs is in [platforms.md](platforms.md).

| Target | Needs | Makes |
|---|---|---|
| `make package-linux` | the Linux build packages above; nfpm runs with `go run` at a pinned version | `dist/nebula-gui_<v>-1_amd64.deb`, `dist/nebula-gui-<v>-1.x86_64.rpm` |
| `make package-macos` | Xcode command line tools (`lipo`, `codesign`, `pkgbuild`, `productbuild`) | `dist/nebula-gui-<v>-macos-universal.pkg` |
| `make package-windows` | Inno Setup 6 (`ISCC`, override its path with `ISCC=`), PowerShell, bash (Git Bash or MSYS2); go-winres runs with `go run` | `dist/nebula-gui-<v>-windows-amd64-setup.exe` |

All three run `make icons` and `make notices` first. Release builds are stripped (`-s -w`) except on Windows, where Defender's machine-learning scan flags stripped, unsigned Go programs; `go-winres` adds version details, the icon and a manifest from `packaging/windows/*.winres.json` instead. `make package-windows` downloads Wintun and checks it against `packaging/windows/wintun.sha256`.

### Testing packages

CI builds every package on each push and checks it:

- **Linux** (`ubuntu-22.04`, the oldest runner, so the GUI's glibc requirement stays low): `shellcheck` on the package scripts, then installs the `.deb`, runs `packaging/linux/check-install.sh --running` (files, modes, user, group, `systemd-analyze verify` on the unit, the service running and answering), removes it and checks the service is gone and `/var/lib/nebula-gui` is kept. The `.rpm` is installed and checked in a `fedora:latest` container, without systemd.
- **macOS:** installs the `.pkg` with `installer` and runs `packaging/macos/check-install.sh` (files, universal binaries, signatures, the daemon, the socket's owner and mode, the API as root and as the console user, refused to `nobody`), then uninstalls twice: keeping the settings, then with `--remove-settings`.
- **Windows:** installs the setup silently and runs `packaging/windows/check-install.ps1 -State Installed` (files, version details, the service, group, firewall rule, the API over the pipe), then uninstalls twice: `-State Removed`, then `/REMOVESETTINGS` and `-State Purged`.

What CI can't check (warnings when downloading, the tray, the group taking effect, autoconnect at boot) is in the smoke tests in [testing.md](testing.md#installer-smoke-tests-m4).

## Releases

1. Make sure the commit you'll tag is green in CI.
2. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`. A tag with a `-` (`v0.1.0-rc.1`) makes a pre-release.
3. `.github/workflows/release.yml` runs CI's jobs, checks each package carries the tag's version, renames `~` to `-` in file names (GitHub would anyway), writes `SHA256SUMS`, and creates a **draft** release with the files, generated notes and a pointer to the README's install steps.
4. Download the files from the draft, try them (at least one install per OS for a release candidate), then publish the draft by hand.

The workflow uses no secrets: release files must download without signing in. To throw away a release candidate: `gh release delete v0.1.0-rc.1 --cleanup-tag --yes`, then `git tag -d v0.1.0-rc.1`.

## Cross-compiling

Because Fyne needs CGO, plain `GOOS=… go build` won't work for the GUI. Options:
- **[fyne-cross](https://github.com/fyne-io/fyne-cross)**: Docker-based. Builds Linux and Windows from Linux. macOS targets need an SDK, which in practice means building on a Mac.
- **CI matrix**: native builds on GitHub Actions runners. This is what the packages and releases use.

The service doesn't need CGO and cross-compiles normally.

## Code conventions
- Format with `gofmt` and `goimports`. Lint with the `golangci-lint` defaults plus `errorlint` and `gosec`.
- Wrap errors with context (`fmt.Errorf("start nebula: %w", err)`). API handlers map errors to the codes in [ipc-api.md](ipc-api.md).
- Use `log/slog` for app logging. The service's logger, which Nebula also uses, writes to stderr, to the log ring buffer (info and above) and to the log file in the state directory (debug and above).
- Never log private keys or the full config.
