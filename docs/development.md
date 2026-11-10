<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Development Guide

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Go | the version in `go.mod` (latest stable) | https://go.dev/dl |
| C compiler | — | Fyne uses CGO for OpenGL |
| `fyne` CLI | latest | `go install fyne.io/tools/cmd/fyne@latest`, used for packaging and icons |
| `nebula` + `nebula-cert` | a version inside the supported range | For the test lab. Distro packages are often old (for example 1.6.x). Prefer the GitHub release binary. |
| `golangci-lint` | latest | Linting |

### Per-OS packages

**Linux (Debian/Ubuntu/Pop!_OS)**
```sh
sudo apt install gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev
```
Install Go from the official tarball at https://go.dev/dl, not from apt. The apt `golang` package is usually too old (Ubuntu 24.04 ships Go 1.22). Once any Go 1.21+ is installed, the `toolchain` line in `go.mod` makes `go` download the exact version the project uses.
**Fedora**
```sh
sudo dnf install golang gcc libXcursor-devel libXrandr-devel mesa-libGL-devel libXi-devel libXinerama-devel libXxf86vm-devel libxkbcommon-devel
```
**macOS:** `xcode-select --install`, then `brew install go`.

**Windows:** install Go, then a GCC toolchain from [MSYS2](https://www.msys2.org/) (`pacman -S mingw-w64-ucrt-x86_64-gcc`) and add its `bin` directory to `PATH`. You also need `wintun.dll` from https://www.wintun.net, placed next to the service binary (see [platforms.md](platforms.md#windows)).

## Repository layout (planned)

```
cmd/
  nebula-gui/        # GUI entrypoint (unprivileged)
  nebula-guid/       # service entrypoint (privileged)
internal/
  api/               # shared request/response types + typed HTTP client
  ipc/               # transport: listener/dialer per OS (unix socket, named pipe)
    ipc_unix.go
    ipc_windows.go
  service/           # API handlers, state machine, config store, pinger, log buffer
  nebulaproc/        # spawn/stop/supervise the nebula child process (per-OS signal + job/pdeathsig code)
  nebulactl/         # SSH console client + per-version output parsers (testdata/<version>/ golden files)
  binstore/          # versioned nebula binary store, active.json, hash re-verification
  updater/           # manifest fetch + minisign verify, download, per-OS installer launch
  gui/               # Fyne windows, widgets, tray, poller
  pki/               # inline-PEM helpers, cert parsing for display
packaging/
  linux/             # systemd unit, polkit/udev bits, .desktop file, nfpm config
  macos/             # launchd plist, app bundle Info.plist
  windows/           # service install scripts / installer config
docs/
testlab/             # scripts + configs for a local multi-node Nebula network
Makefile
```

Rules:
- **The project does not import `github.com/slackhq/nebula` at all.** All Nebula knowledge lives in `internal/nebulactl` (console commands and parsers) and `internal/config` (the generated runtime config).
- **`internal/gui` never imports `internal/service`.** The GUI and service communicate only through `internal/api`.
- Platform-specific code uses `_linux.go`, `_darwin.go`, and `_windows.go` filename suffixes rather than runtime checks.

## Bootstrapping the module (first time)

```sh
go mod init github.com/<you>/nebula-gui
go get fyne.io/fyne/v2@latest
go get golang.org/x/crypto/ssh@latest              # SSH console client
go get github.com/jedisct1/go-minisign@latest     # manifest signatures
go get github.com/Masterminds/semver/v3@latest    # supported-range checks
go get github.com/prometheus-community/pro-bing@latest
go get github.com/Microsoft/go-winio@latest   # Windows named pipes
```

There is no Nebula Go dependency to pin. Instead, the **supported Nebula range** is a constant in `internal/nebulactl` (`SupportedRange = ">=1.8.0 <1.11.0"`). Raise it only after the version matrix passes ([testing.md](testing.md#nebula-version-matrix)).

## Running in development

Run the service and the GUI in separate terminals. Use a socket in your home directory so the GUI doesn't need special group membership.

### Linux
Instead of running the service as root, give the built binary `CAP_NET_ADMIN`:
```sh
go build -o bin/nebula-guid ./cmd/nebula-guid
sudo setcap cap_net_raw+ep bin/nebula-guid                    # for ICMP ping
mkdir -p .dev-nebula && cp "$(command -v nebula)" .dev-nebula/  # or extract an upstream release here
sudo setcap cap_net_admin+ep .dev-nebula/nebula               # file caps don't pass to children; give them to nebula itself
./bin/nebula-guid --listen "$XDG_RUNTIME_DIR/nebula-guid.sock" --state-dir ./.dev-state \
    --nebula-binary ./.dev-nebula/nebula --dev

# other terminal
NEBULA_GUI_SOCKET="$XDG_RUNTIME_DIR/nebula-guid.sock" go run ./cmd/nebula-gui
```
`setcap` has to be run again after every rebuild. `--nebula-binary` is **dev-only**. In release builds the binary comes from the binary store or the admin config file. A `make dev-service` target should do both steps.

### macOS
```sh
go build -o bin/nebula-guid ./cmd/nebula-guid
sudo ./bin/nebula-guid --listen /tmp/nebula-guid.sock --socket-owner "$USER" --state-dir ./.dev-state \
    --nebula-binary ./.dev-nebula/nebula --dev   # unzip nebula-darwin.zip into .dev-nebula/
NEBULA_GUI_SOCKET=/tmp/nebula-guid.sock go run ./cmd/nebula-gui
```

### Windows (Administrator PowerShell for the service)
```powershell
go build -o bin\nebula-guid.exe .\cmd\nebula-guid
.\bin\nebula-guid.exe --listen \\.\pipe\nebula-guid-dev --nebula-binary .\.dev-nebula\nebula.exe --dev  # unzip nebula-windows-amd64.zip (incl. wintun.dll) into .dev-nebula\
# normal PowerShell
$env:NEBULA_GUI_SOCKET="\\.\pipe\nebula-guid-dev"; go run .\cmd\nebula-gui
```

`--dev` makes the service:
- log at debug level to stderr;
- run in the foreground instead of as a system service;
- loosen socket ownership to the user who launched it.

**Release builds must not accept `--dev`.** Enforce this with a build tag.

## Common tasks (planned Makefile targets)

| Target | Does |
|---|---|
| `make build` | Build both binaries into `bin/` |
| `make dev-service` | Build the service, `setcap` it (Linux), and run it with dev flags |
| `make gui` | `go run ./cmd/nebula-gui` with the dev socket |
| `make test` | `go test ./...` |
| `make lint` | `golangci-lint run` |
| `make lab-up` / `make lab-down` | Start or stop the local Nebula test lab ([testing.md](testing.md)) |
| `make package-{linux,macos,windows}` | Build platform packages |

## Cross-compiling

Because Fyne needs CGO, plain `GOOS=… go build` won't work for the GUI. Options:
- **[fyne-cross](https://github.com/fyne-io/fyne-cross)**: Docker-based. Builds Linux and Windows from Linux. macOS targets need an SDK, which in practice means building on a Mac.
- **CI matrix**: native builds on GitHub Actions `ubuntu-latest`, `windows-latest`, and `macos-latest`. This is the recommended approach for releases.

The service doesn't need CGO and cross-compiles normally.

## Code conventions
- Format with `gofmt` and `goimports`. Lint with the `golangci-lint` defaults plus `errorlint` and `gosec`.
- Wrap errors with context (`fmt.Errorf("start nebula: %w", err)`). API handlers map errors to the codes in [ipc-api.md](ipc-api.md).
- Use `log/slog` for app logging. Nebula's `logrus` output goes through an adapter into the service log ring buffer.
- Never log private keys or the full config.
