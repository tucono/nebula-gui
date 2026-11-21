<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# nebula-gui

A cross-platform desktop app for managing [Nebula](https://github.com/slackhq/nebula) overlay-network connections.

A user loads a Nebula config. The app starts and stops the tunnel, shows which peers are connected, and can ping a peer to open a tunnel to it.

> **Personal project.** Built for a private Nebula network shared with friends and family. The repo is public so they can download releases and updates. You're welcome to use it, but there's no support, and issues or pull requests may go unanswered.

> **Not an official Nebula project.** nebula-gui is an independent project. It is not affiliated with or endorsed by Slack or Defined Networking. Nebula is copyright Slack Technologies, Inc. and is MIT licensed. The installers include the licenses of Nebula and every other open-source part built in, in `THIRD_PARTY_NOTICES.txt`; the app opens it from Help → About nebula-gui.

> **Status:** pre-alpha. The service embeds Nebula, and the GUI can import a config, connect and disconnect, show the service log, explain problems in plain language, and make a masked troubleshooting report (M2). It also lists peers under names the user chooses, pings them to open a tunnel, and closes stuck connections (M3). Installers for Linux, Windows and macOS put the service in place and start it at boot; the app sits in the tray and can connect automatically when the computer starts (M4). Uninstalling can remove everything the app saved, for every user of the computer (M4.5). Next, in v1.0, comes a notice when a new version is out (M5). See [docs/roadmap.md](docs/roadmap.md).

## Install

Download the file for your computer from the [releases page](https://github.com/tucono/nebula-gui/releases), install it, then open nebula-gui and import the config the person who runs your network gave you.

| Computer | File |
|---|---|
| Windows 10 or 11 (64-bit Intel/AMD) | `nebula-gui-<version>-windows-amd64-setup.exe` |
| Mac, Apple silicon or Intel, macOS 11 or later | `nebula-gui-<version>-macos-universal.pkg` |
| Ubuntu, Debian, Pop!_OS, Mint (64-bit Intel/AMD) | `nebula-gui_<version>-1_amd64.deb` |
| Fedora (64-bit Intel/AMD) | `nebula-gui-<version>-1.x86_64.rpm` |

The installers aren't signed, and there are no plans to sign them, so Windows and macOS warn that they don't know who made them. The steps below get past those warnings. If you'd like to make sure the file is the one that was published, see [Checking a download](#checking-a-download) first.

### Windows

1. **Download.** Edge may say the file isn't commonly downloaded. Open the downloads list, click **⋯** next to the file, choose **Keep**, then **Keep anyway**.
2. **Run it.** SmartScreen says "Windows protected your PC". Click **More info**, then **Run anyway**. Windows then asks for permission to make changes: click **Yes** (or ask someone with an administrator account to type their password).
3. Follow the installer. At the end it opens nebula-gui. You can use the app straight away; other people who share the computer need to be added to the `nebula-gui` group (see [docs/platforms.md](docs/platforms.md#windows)).

**To uninstall:** Settings → Apps, find nebula-gui, and choose Uninstall. It asks whether to remove everything nebula-gui saved on this computer: the imported config, and every user's saved peer names, app log and settings. The default, No, keeps them for a reinstall.

### macOS

1. **Download and open the `.pkg`.** macOS says it was "Not Opened" because "Apple could not verify" it "is free of malware". Click **Done** (not Move to Trash).
2. Open **System Settings → Privacy & Security**. Near the bottom, next to a line about the nebula-gui package being blocked, click **Open Anyway**, then confirm with your password or Touch ID.
   On macOS 14 and earlier you can instead right-click the `.pkg` in Finder, choose **Open**, then **Open** again.
3. Follow the installer. It installs nebula-gui in Applications, starts the background service, and gives you access to it.

**To uninstall:** open **Uninstall nebula-gui** in Applications, or choose **Help → Uninstall nebula-gui…** in the app. Choose **Keep settings and uninstall** to use them again after a reinstall, or **Remove everything and uninstall** to also remove the imported config and every user's saved peer names, app log and settings. From Terminal it's `sudo /usr/local/libexec/nebula-gui/uninstall.sh`, adding `--remove-everything` for the second. Dragging the app to the Trash leaves the background service running.

### Linux

1. **Install the package.** Double-clicking it opens your software centre, or use a terminal:
   ```sh
   sudo apt install ./nebula-gui_<version>-1_amd64.deb     # Ubuntu, Debian, Pop!_OS, Mint
   sudo dnf install ./nebula-gui-<version>-1.x86_64.rpm    # Fedora
   ```
2. **Restart the computer.** The installer adds you to the `nebula-gui` group, which takes effect after a restart (signing out isn't always enough). Until then the app says "Restart your computer to finish".
3. Open nebula-gui from your applications menu. The tray icon needs a desktop with a system tray; on GNOME that means the AppIndicator extension (Ubuntu has it already). Without one the app works as a normal window.

Other people who share the computer: `sudo usermod -aG nebula-gui <their login name>`, then they restart too.

**To uninstall:** `sudo apt remove nebula-gui` or `sudo dnf remove nebula-gui`. This keeps the imported config and each user's settings for a reinstall.

**To remove everything** (the imported config, every user's saved peer names, app log and settings, and the `nebula-gui` group):
```sh
sudo apt purge nebula-gui        # Ubuntu, Debian, Pop!_OS, Mint (also after apt remove)

# Fedora: rpm has no purge, so remove everything first, then the package
sudo systemctl disable --now nebula-guid
sudo /usr/libexec/nebula-gui/nebula-guid remove-everything
sudo dnf remove nebula-gui
```

On every system, removing everything reaches a user's files only when their home folder is there to read: one that's encrypted or on a network server while they're signed out is skipped.

### Checking a download

Each release has a `SHA256SUMS` file listing the SHA-256 hash of every file in it. Download it into the same folder as the installer, then:

| Computer | Command | Look for |
|---|---|---|
| Linux | `sha256sum -c SHA256SUMS --ignore-missing` | `<file>: OK` |
| macOS (Terminal) | `shasum -a 256 -c SHA256SUMS --ignore-missing` | `<file>: OK` |
| Windows (PowerShell) | `Get-FileHash .\nebula-gui-<version>-windows-amd64-setup.exe` | the same hash as the `.exe`'s line in `SHA256SUMS` (case doesn't matter) |

This shows the file wasn't damaged or swapped after it was published. Since `SHA256SUMS` comes from the same page, it can't protect against someone who controls the releases page; for that, compare the hash with one the person who runs your network sends you another way.

### After installing

- **Import config…** on the Status tab, then **Connect**.
- Closing the window leaves nebula-gui in the tray; the connection stays up even when the app is quit. To end it, **Disconnect** first.
- The **Settings** tab has **Connect automatically when this computer starts** (off until you turn it on) and **Open nebula-gui when I sign in**.
- If something's wrong, the app explains it. **Get help…** makes a report you can paste into an AI assistant or send to the person who runs your network.

## Goals

- Import a Nebula config, including inline or referenced certs and keys.
- Start and stop the Nebula connection with one click.
- Show active and pending peer tunnels: name, overlay IP, real endpoint, and whether the tunnel is direct or relayed.
- Ping a peer's overlay IP to open a tunnel and measure round-trip time.
- Say when a new version is on GitHub, including one with a newer Nebula. Installing it stays a manual download ([docs/updates.md](docs/updates.md)).
- Run on **Windows, Linux, and macOS**. Android and iOS are stretch goals (see [docs/platforms.md](docs/platforms.md#mobile)).

## Non-goals (for now)

- Acting as a certificate authority or managing a fleet. Use `nebula-cert` or Defined Networking for that.
- Running more than one Nebula network at the same time. This may be added after v1.
- Installing updates from the app, and signed installers (ADR-009 in [docs/decisions.md](docs/decisions.md)).
- Builds for 32-bit or ARM Windows and Linux, and testing on desktops other than COSMIC (Pop!_OS).

## Tech stack

| Concern | Choice |
|---|---|
| Language | Go |
| GUI | [Fyne v2](https://fyne.io) |
| VPN engine | Nebula, **embedded as a Go library** (`github.com/slackhq/nebula`) |
| Privilege model | Separate privileged service (`nebula-guid`) plus an unprivileged GUI (`nebula-gui`) |
| GUI ↔ service IPC | HTTP + JSON over a Unix socket (Linux/macOS) or named pipe (Windows) |

The reasons for each choice are in [docs/decisions.md](docs/decisions.md).

## Documentation

| Doc | Contents |
|---|---|
| [docs/network-setup.md](docs/network-setup.md) | Running a network for app users: CA, lighthouse, configs to hand out |
| [docs/architecture.md](docs/architecture.md) | Components, process model, data flow |
| [docs/ipc-api.md](docs/ipc-api.md) | The GUI ↔ service API contract |
| [docs/development.md](docs/development.md) | Toolchain setup, repo layout, building and running, packages and releases |
| [docs/testing.md](docs/testing.md) | Unit tests and a local multi-node Nebula test lab |
| [docs/platforms.md](docs/platforms.md) | What each installer sets up per OS, known issues, mobile |
| [docs/updates.md](docs/updates.md) | Installing new versions, and the new-version check |
| [docs/security.md](docs/security.md) | Threat model and service hardening rules |
| [docs/decisions.md](docs/decisions.md) | Architecture decision records |
| [docs/roadmap.md](docs/roadmap.md) | Milestones |

## Authorship

Each file starts with two tags recording how it was made:

- `@authorship`: who wrote it.
  - `Human-Primary`: written by the maintainer, with at most minor AI help.
  - `AI-Assisted`: an AI assistant wrote a substantial part of it.
- `@architect`: who designed it.
  - `Philip Thomas`: the maintainer designed its structure, and reviewed and refined any AI-written parts.
  - `AI (<model>)`: an AI assistant proposed the design as well as writing it; the maintainer requested, reviewed and integrated it.

Generated files, files that can't hold comments, and repo configuration files (such as `.gitignore`) aren't tagged.
